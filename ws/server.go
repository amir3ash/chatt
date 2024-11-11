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
	conn   nettyws.Conn
	userId string
	onErr  func(error)
}

func (c nettyConnection) getUserId() string {
	return c.userId
}

func (c nettyConnection) sendBytes(b []byte) {
	err := c.conn.Write(b)
	if err != nil {
		slog.Error("cant write to websocket", "err", err)
		go c.onErr(err)
	}
}

type serv struct {
	// chatSvc   api.MessageService
	broker    *wsServer
	websocket *nettyws.Websocket
	ch        chan *nettyConnection
	OnConnect func(nettyws.Conn)
}

func newServ(broker *wsServer) serv {
	wsh := nettyws.NewWebsocket(
		nettyws.WithAsyncWrite(512, false),
		// nettyws.WithBufferSize(2048, 2048),
		nettyws.WithNoDelay(true),
	)
	s := serv{broker, wsh, make(chan *nettyConnection, 15), nil}

	s.setupWsHandler()
	go s.connectingHandler()

	return s
}

func RunServer(broker *wsServer) {
	// TODO add allowed origins to prevent CSRF
	go func() {
		fmt.Println("listen on ws://:7100")

		wsHandler := newServ(broker)

		handler := authz.NewHttpAuthMiddleware(wsHandler)
		http.Handle("/ws", handler)

		if err := http.ListenAndServe(":7100", handler); err != nil {
			slog.Error("http can't listen on port 7100", "err", err)
		}
	}()
}

func (wsHandler serv) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := wsHandler.websocket.UpgradeHTTP(w, r)
	if err != nil {
		if errors.Is(err, nettyws.ErrServerClosed) {
			http.Error(w, "http: server shutdown", http.StatusNotAcceptable)
		} else {
			http.Error(w, err.Error(), http.StatusNotAcceptable)
		}
		return
	}

	userId := authz.UserIdFromCtx(r.Context())

	nettyConn := &nettyConnection{conn, userId, func(err error) {}}
	conn.SetUserdata(nettyConn)

	wsHandler.OnConnect(conn)
}

func (wsHandler serv) connectingHandler() {
	for conn := range wsHandler.ch {
		wsHandler.OnConnect(conn.conn)
	}
}

func (wsHandler *serv) setupWsHandler() {
	broker := wsHandler.broker

	wsHandler.OnConnect = func(conn nettyws.Conn) {
		nettyConn, ok := conn.Userdata().(*nettyConnection)
		if !ok {
			slog.Error("cant get userdata in websocket.onOpen")
			conn.WriteClose(1011, "Internal Error")
			conn.Close()
			return
		}

		nettyConn.onErr = func(_ error) {
			broker.RemoveConn(nettyConn)
			conn.WriteClose(1001, "going away")
			conn.Close()
		}

		broker.AddConn(nettyConn)
	}

	wsHandler.websocket.OnClose = func(conn nettyws.Conn, err error) {
		nettyConn, ok := conn.Userdata().(*nettyConnection)
		if ok {
			broker.RemoveConn(nettyConn)
		}
		fmt.Println("OnClose: ", conn.RemoteAddr(), ", error: ", err)
	}
}
