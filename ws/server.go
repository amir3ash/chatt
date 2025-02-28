package ws

import (
	"chat-system/ws/presence"
	"context"
	"log/slog"
	"net"
	"net/http"
)

func NewServer(watcher MessageWatcher, authz whoCanReadTopic) *Server {
	onlineUsersPresence := presence.NewMemService[Client]()

	r := &Server{
		Watcher:             watcher,
		Authz:               authz,
		onlineUsersPresence: onlineUsersPresence,
		roomServer:          NewRoomServer(onlineUsersPresence, authz),
		roomDispatcher:      NewRoomDispatcher(),
	}

	r.registerEventHandlers()
	r.setupWsHandler()

	return r
}

type Server struct {
	Watcher        MessageWatcher
	Authz          whoCanReadTopic
	AllowedOrigins []string

	onlineUsersPresence *presence.MemService[Client]
	roomServer          *roomServer
	roomDispatcher      *roomDispatcher
	wsHandler           wsHandler
	httpServer          *http.Server
	wsURL               string
}

func (m *Server) ListenAndServe(addr string) error {
	go ReadChangeStream(m.Watcher, m.roomServer)

	m.httpServer = &http.Server{Addr: addr}
	m.httpServer.RegisterOnShutdown(func() {
		err := m.wsHandler.shutdown()
		if err != nil {
			slog.Error("can not shutdown websocket", "err", err)
		}
	})

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	m.wsURL = "ws://" + ln.Addr().String()

	return m.httpServer.Serve(ln)
}

func (s *Server) setupWsHandler() {
	// TODO add allowed origins to prevent CSRF
	s.wsHandler = newWsHandler(s.onlineUsersPresence, s.roomDispatcher)
	handler := wsHandler.setupHttpMiddlewares(s.wsHandler)

	http.Handle("/ws", handler)
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
//
// Note: it must be called after calling listen.
func (s *Server) WsEndpoint() string {
	if s.wsURL == "" {
		panic("Manager: WsEndpoint(): unkown addr")
	}
	return s.wsURL + "/ws"
}
