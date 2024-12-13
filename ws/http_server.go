package ws

import (
	"chat-system/authz"
	"chat-system/ws/presence"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	nettyws "github.com/go-netty/go-netty-ws"
)

type errorHandledConn struct {
	conn  Conn
	onErr func(error)
}

func (c errorHandledConn) Write(b []byte) error {
	err := c.conn.Write(b)
	if err != nil {
		slog.Error("cant write to websocket", "err", err)
		go c.onErr(err)
	}
	return nil
}
func (c *errorHandledConn) onError(f func(error)) {
	c.onErr = f
}

type httpServer struct {
	onlineClients *presence.MemService
	dispatcher    *roomDispatcher
	websocket     *nettyws.Websocket
	ch            chan *errorHandledConn
	getUserId     func(http.Header) string
}

func newHttpServer(presence *presence.MemService, dispatcher *roomDispatcher) httpServer {
	wsh := nettyws.NewWebsocket(
		nettyws.WithAsyncWrite(512, false),
		// nettyws.WithBufferSize(2048, 2048),
		nettyws.WithNoDelay(true),
	)
	s := httpServer{
		presence,
		dispatcher,
		wsh,
		make(chan *errorHandledConn, 15),
		authz.UserIdFromCookieHeader,
	}

	s.setupWsHandler()

	return s
}

func (s httpServer) RunServer() {
	// TODO add allowed origins to prevent CSRF
	go func() {
		fmt.Println("listen on ws://:7100")

		handler := authz.NewHttpAuthMiddleware(s)
		http.Handle("/ws", handler)

		if err := http.ListenAndServe(":7100", handler); err != nil {
			slog.Error("http can't listen on port 7100", "err", err)
		}
	}()
}

func (s httpServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	_, err := s.websocket.UpgradeHTTP(w, r)
	if err != nil {
		if errors.Is(err, nettyws.ErrServerClosed) {
			http.Error(w, "http: server shutdown", http.StatusNotAcceptable)
		} else {
			http.Error(w, err.Error(), http.StatusNotAcceptable)
		}
		return
	}
}

func (s *httpServer) setupWsHandler() {
	s.websocket.OnOpen = func(conn nettyws.Conn) {
		userId := s.getUserId(conn.Header())

		errHConn := &errorHandledConn{conn, func(err error) {}}
		client := Client{"test-clientID", userId, errHConn}

		conn.SetUserdata(client)

		errHConn.onError(func(_ error) {
			s.onlineClients.Disconnected(context.TODO(), client)
			conn.WriteClose(1001, "going away")
			conn.Close()
			s.dispatcher.dispatch(clientEvent{clientDisconnected, client})
		})

		s.onConnect(conn)
	}

	s.websocket.OnClose = s.onClose
}

func (s *httpServer) onConnect(conn nettyws.Conn) {
	client := conn.Userdata().(Client)

	s.onlineClients.Connect(context.TODO(), client)
	s.dispatcher.dispatch(clientEvent{clientConnected, client})
}

func (s *httpServer) onClose(conn nettyws.Conn, err error) {
	client := conn.Userdata().(Client)

	s.onlineClients.Disconnected(context.TODO(), client)
	s.dispatcher.dispatch(clientEvent{clientDisconnected, client})

	slog.Debug("client closed the connection", "remoteAddr", conn.RemoteAddr(), "userId", client.UserId(), "err", err)
}
