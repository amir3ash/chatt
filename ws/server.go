package ws

import (
	"chat-system/ws/presence"
	"context"
	"log/slog"
	"net"
	"net/http"
	"slices"
	"strings"
)

type ServerOpt func(s *Server)

func WithAllowedOrigin(origins ...string) ServerOpt {
	return func(s *Server) {
		for _, orig := range origins {
			if strings.HasSuffix(orig, "*") {
				panic("ws.Server: wildcard origin not supported yet")
			}
		}
		s.AllowedOrigins = append(s.AllowedOrigins, origins...)
	}
}

func NewServer(watcher MessageWatcher, authz whoCanReadTopic, opts ...ServerOpt) *Server {
	onlineUsersPresence := presence.NewMemService[Client]()

	s := &Server{
		Watcher:             watcher,
		Authz:               authz,
		onlineUsersPresence: onlineUsersPresence,
		roomServer:          NewRoomServer(onlineUsersPresence, authz),
		roomDispatcher:      NewRoomDispatcher(),
		httpHandler:         http.NewServeMux(),
		listenDone:          make(chan struct{}),
	}

	for _, opt := range opts {
		opt(s)
	}

	s.registerEventHandlers()
	s.setupWsHandler()

	return s
}

type Server struct {
	Watcher        MessageWatcher
	Authz          whoCanReadTopic
	AllowedOrigins []string

	onlineUsersPresence *presence.MemService[Client]
	roomServer          *roomServer
	roomDispatcher      *roomDispatcher
	wsHandler           wsHandler
	httpHandler         *http.ServeMux
	httpServer          *http.Server

	listenDone chan struct{} // used for synchronization of readig wsURL.
	wsURL      string
}

// ListenAndServe listens and serves websocket handshakes on path "/ws".
func (s *Server) ListenAndServe(addr string) error {
	go ReadChangeStream(s.Watcher, s.roomServer)

	s.httpServer = &http.Server{Addr: addr, Handler: s.httpHandler}
	s.httpServer.RegisterOnShutdown(func() {
		err := s.wsHandler.shutdown()
		if err != nil {
			slog.Error("can not shutdown websocket", "err", err)
		}
	})

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		close(s.listenDone)
		return err
	}

	s.wsURL = "ws://" + ln.Addr().String()
	close(s.listenDone)

	return s.httpServer.Serve(ln)
}

func (s *Server) setupWsHandler() {
	s.wsHandler = newWsHandler(s.onlineUsersPresence, s.roomDispatcher)
	handler := wsHandler.setupHttpMiddlewares(s.wsHandler)

	handler = AllowedOriginsMiddleware(handler, s.AllowedOrigins)

	s.httpHandler.Handle("/ws", handler)
}

func (s *Server) registerEventHandlers() {
	s.roomDispatcher.SubscribeOnClientEvents(func(e clientEvent) {
		if e.EventType() == clientConnected {
			err := s.roomServer.onClientConnected(e.client)
			if err != nil {
				slog.Error("onClientConneted fails", slog.String("clientId", e.client.ClientId()), "err", err)
				s.wsHandler.closeClient(e.client, InternalError)
			}
		} else {
			err := s.roomServer.onClientDisconnected(e.client)
			if err != nil {
				slog.Error("onClientDisconneted fails", slog.String("clientId", e.client.ClientId()), "err", err)
			}
		}
	})
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

// return websocket endpoint (like ws://127.0.0.1:7100/ws).
func (s *Server) WsEndpoint() string {
	<-s.listenDone
	return s.wsURL + "/ws"
}

// A middleware to check Origin http header to prevent CSRF attack in websocket handshakes in browsers.
//
// If Origin header is empty or is allowed, it will pass.
// Buf if origin is not allowed returns 403 response.
func AllowedOriginsMiddleware(h http.Handler, allowedOrigins []string) http.Handler {
	if len(allowedOrigins) == 0 {
		return h
	}

	errBody := []byte("Origin is not allowed.")
	slices.Sort(allowedOrigins)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")

		_, found := slices.BinarySearch(allowedOrigins, origin)
		if found || origin == "" {
			h.ServeHTTP(w, r)
		} else {
			w.WriteHeader(403)
			w.Write(errBody)
		}
	})
}
