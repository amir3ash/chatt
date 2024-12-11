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
	OnConnect     func(nettyws.Conn)
}

func newHttpServer(presence *presence.MemService, dispatcher *roomDispatcher) httpServer {
	wsh := nettyws.NewWebsocket(
		nettyws.WithAsyncWrite(512, false),
		// nettyws.WithBufferSize(2048, 2048),
		nettyws.WithNoDelay(true),
	)
	s := httpServer{presence, dispatcher, wsh, make(chan *errorHandledConn, 15), nil}

	s.setupWsHandler()
	go s.connectingHandler()

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
	conn, err := s.websocket.UpgradeHTTP(w, r)
	if err != nil {
		if errors.Is(err, nettyws.ErrServerClosed) {
			http.Error(w, "http: server shutdown", http.StatusNotAcceptable)
		} else {
			http.Error(w, err.Error(), http.StatusNotAcceptable)
		}
		return
	}

	userId := authz.UserIdFromCtx(r.Context())

	nettyConn := &errorHandledConn{conn, func(err error) {}}
	conn.SetUserdata(Client{"test-clientID", userId, nettyConn})

	s.OnConnect(conn)
}

func (s httpServer) connectingHandler() {
	for conn := range s.ch {
		s.OnConnect(conn.conn.(nettyws.Conn))
	}
}

func (s *httpServer) setupWsHandler() {
	s.OnConnect = func(conn nettyws.Conn) {
		client, ok := conn.Userdata().(Client)
		if !ok {
			slog.Error("cant get userdata in websocket.onOpen")
			conn.WriteClose(1011, "Internal Error")
			conn.Close()
			return
		}

		nettyConn := client.Conn().(errorHandledConn)
		nettyConn.onError(func(_ error) {
			s.onlineClients.Disconnected(context.TODO(), client)
			conn.WriteClose(1001, "going away")
			conn.Close()
			s.dispatcher.dispatch(clientEvent{clientDisconnected, client})
		})

		s.onlineClients.Connect(context.TODO(), client)
		s.dispatcher.dispatch(clientEvent{clientConnected, client})
	}

	s.websocket.OnClose = func(conn nettyws.Conn, err error) {
		client, ok := conn.Userdata().(Client)
		if ok {
			s.onlineClients.Disconnected(context.TODO(), client)
			s.dispatcher.dispatch(clientEvent{clientDisconnected, client})
		} else {
			slog.Warn("can not cast connection's userdata to Client sturct", "userData", conn.Userdata())
		}

		slog.Debug("client closed the connection", "remoteAddr", conn.RemoteAddr(), "userId", client.UserId(), "err", err)
	}
}
