package ws

import (
	"chat-system/authz"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	nettyws "github.com/go-netty/go-netty-ws"
)

type nettyConnection struct {
	conn  nettyws.Conn
	onErr func(error)
}

func (c nettyConnection) Write(b []byte) error {
	err := c.conn.Write(b)
	if err != nil {
		slog.Error("cant write to websocket", "err", err)
		go c.onErr(err)
	}
	return nil
}
func (c *nettyConnection) onError(f func(error)) {
	c.onErr = f
}

type httpServer struct {
	// chatSvc   api.MessageService
	broker    *wsServer
	websocket *nettyws.Websocket
	ch        chan *nettyConnection
	OnConnect func(nettyws.Conn)
}

func newHttpServer(broker *wsServer) httpServer {
	wsh := nettyws.NewWebsocket(
		nettyws.WithAsyncWrite(512, false),
		// nettyws.WithBufferSize(2048, 2048),
		nettyws.WithNoDelay(true),
	)
	s := httpServer{broker, wsh, make(chan *nettyConnection, 15), nil}

	s.setupWsHandler()
	go s.connectingHandler()

	return s
}

func RunServer(broker *wsServer) {
	// TODO add allowed origins to prevent CSRF
	go func() {
		fmt.Println("listen on ws://:7100")

		server := newHttpServer(broker)

		handler := authz.NewHttpAuthMiddleware(server)
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

	nettyConn := &nettyConnection{conn, func(err error) {}}
	conn.SetUserdata(Client{"test-clientID", userId, nettyConn})

	s.OnConnect(conn)
}

func (s httpServer) connectingHandler() {
	for conn := range s.ch {
		s.OnConnect(conn.conn)
	}
}

func (s *httpServer) setupWsHandler() {
	broker := s.broker

	s.OnConnect = func(conn nettyws.Conn) {
		client, ok := conn.Userdata().(Client)
		if !ok {
			slog.Error("cant get userdata in websocket.onOpen")
			conn.WriteClose(1011, "Internal Error")
			conn.Close()
			return
		}

		nettyConn := client.Conn().(nettyConnection)
		nettyConn.onError(func(_ error) {
			broker.RemoveConn(client)
			conn.WriteClose(1001, "going away")
			conn.Close()
		})

		broker.AddConn(client)
	}

	s.websocket.OnClose = func(conn nettyws.Conn, err error) {
		client, ok := conn.Userdata().(Client)
		if ok {
			broker.RemoveConn(client)
		}
		fmt.Println("OnClose: ", conn.RemoteAddr(), ", error: ", err)
	}
}
