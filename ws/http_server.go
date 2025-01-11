package ws

import (
	"chat-system/authz"
	"chat-system/ws/presence"
	"context"
	"errors"
	"log/slog"
	"net/http"

	nettyws "github.com/go-netty/go-netty-ws"
)

type errorHandledConn struct {
	conn  Conn
	onErr func(error)
}

// calls the function onErr in a new goroutine in case of none nil error.
func (c errorHandledConn) Write(b []byte) error {
	err := c.conn.Write(b)
	if err != nil {
		slog.Warn("cant write to websocket", "err", err)
		go c.onErr(err)
	}
	return nil
}
func (c *errorHandledConn) onError(f func(error)) {
	c.onErr = f
}

// wsHandler manages all online clients and upgrading http requests to websocket.
type wsHandler struct {
	onlineClients *presence.MemService[Client]
	dispatcher    *roomDispatcher
	websocket     *nettyws.Websocket
	getUserId     func(http.Header) string // gets userId from [http.Header]
}

func newWsHandler(presence *presence.MemService[Client], dispatcher *roomDispatcher) wsHandler {
	wsh := nettyws.NewWebsocket(
		nettyws.WithAsyncWrite(512, false),
		// nettyws.WithBufferSize(2048, 2048),
		nettyws.WithNoDelay(true),
	)
	s := wsHandler{
		presence,
		dispatcher,
		wsh,
		authz.UserIdFromCookieHeader,
	}

	s.setupWsHandler()

	return s
}

// implements [http.Handler] to upgrade requests to websocket.
func (s wsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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

func (s *wsHandler) setupWsHandler() {
	s.websocket.OnOpen = func(conn nettyws.Conn) {
		userId := s.getUserId(conn.Header())

		errHConn := &errorHandledConn{conn, func(err error) {}}
		client := Client{userId + randomClientIdSuffix(), userId, errHConn}

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

// adds conn's [Client] to s.onlineClients and dispatches an event.
func (s *wsHandler) onConnect(conn nettyws.Conn) {
	client := conn.Userdata().(Client)

	s.onlineClients.Connect(context.TODO(), client)
	s.dispatcher.dispatch(clientEvent{clientConnected, client})
}

// removes conn's [Client] from s.onlineClients and dispatches an event.
func (s *wsHandler) onClose(conn nettyws.Conn, err error) {
	client := conn.Userdata().(Client)

	s.onlineClients.Disconnected(context.TODO(), client)
	s.dispatcher.dispatch(clientEvent{clientDisconnected, client})

	slog.Debug("client closed the connection", "remoteAddr", conn.RemoteAddr(), "userId", client.UserId(), "err", err)
}
