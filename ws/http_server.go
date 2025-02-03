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

// calls the function onErr in a new goroutine in case of none nil error.
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

// httpServer manages all online clients and upgrading http requests to websocket.
type httpServer struct {
	onlineClients *presence.MemService[Client]
	dispatcher    *roomDispatcher
	websocket     *nettyws.Websocket
	ch            chan *errorHandledConn
	getUserId     func(http.Header) string // gets userId from [http.Header]
}

func newHttpServer(presence *presence.MemService[Client], dispatcher *roomDispatcher) httpServer {
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

// listens on port 7100 and handles websockets on "/ws" path.
func (s httpServer) RunServer() {
	// TODO add allowed origins to prevent CSRF
	go func() {
		fmt.Println("listen on ws://:7100")

		if err := http.ListenAndServe(":7100", s.setupHandler()); err != nil {
			slog.Error("http can't listen on port 7100", "err", err)
		}
	}()
}

func (s httpServer) setupHandler() http.Handler {
	handler := authz.NewHttpAuthMiddleware(s)
	// http.Handle("/ws", handler)

	return handler
}

// implements [http.Handler] to upgrade requests to websocket.
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
func (s *httpServer) onConnect(conn nettyws.Conn) {
	client := conn.Userdata().(Client)

	s.onlineClients.Connect(context.TODO(), client)
	s.dispatcher.dispatch(clientEvent{clientConnected, client})
}

// removes conn's [Client] from s.onlineClients and dispatches an event.
func (s *httpServer) onClose(conn nettyws.Conn, err error) {
	client := conn.Userdata().(Client)

	s.onlineClients.Disconnected(context.TODO(), client)
	s.dispatcher.dispatch(clientEvent{clientDisconnected, client})

	slog.Debug("client closed the connection", "remoteAddr", conn.RemoteAddr(), "userId", client.UserId(), "err", err)
}

// closeClient disconnect the client from server
// and writes websocket close frame with code and reason.
func (s *httpServer) closeClient(client Client, code WsCode) {
	conn, ok := client.Conn().(*errorHandledConn).conn.(nettyws.Conn)
	if !ok {
		slog.Error("can not cast client's conn top nettyws.Conn", "conn", conn)
		return
	}

	s.onlineClients.Disconnected(context.TODO(), client)

	err := conn.WriteClose(int(code), code.GetCloseReason())
	if err != nil {
		slog.Warn("can not write close frame into ws connection", "err", err)
	}

	conn.Close()
	s.dispatcher.dispatch(clientEvent{clientDisconnected, client})
}

func (s *httpServer) shutdown() error {
	return s.websocket.Close()
}

type WsCode int

const (
	// The connection successfully completed the purpose for which it was created.
	NormalClosure WsCode = 1000
	// The endpoint is going away, either because of a server failure or because the
	// browser is navigating away from the page that opened the connection.
	GoAway WsCode = 1001
	// The endpoint is terminating the connection due to a protocol error.
	ProtocolError WsCode = 1002
	// The connection is being terminated because the endpoint received data of a type it cannot accept.
	UnsupportedData WsCode = 1003
	// Indicates that no status code was provided even though one was expected.
	NoStatusRecived WsCode = 1005
	// Indicates that a connection was closed abnormally (that is, with no close frame being sent) when a status code is expected.
	AbnormalClosure WsCode = 1006
	// The endpoint is terminating the connection because a message was received that contained inconsistent data (e.g., non-UTF-8 data within a text message).
	InvalideFramePayload WsCode = 1007
	// The endpoint is terminating the connection because it received a message that violates its policy. This is a generic status code, used when codes 1003 and 1009 are not suitable.
	PolicyVoiolation WsCode = 1008
	// The endpoint is terminating the connection because a data frame was received that is too large.
	MessageTooBig WsCode = 1009
	// The client is terminating the connection because it expected the server to negotiate one or more extension, but the server didn't.
	MandatoryExtension WsCode = 1010
	// The server is terminating the connection because it encountered an unexpected condition that prevented it from fulfilling the request.
	InternalError WsCode = 1011
	// The server is terminating the connection because it is restarting.
	ServiceRestart WsCode = 1012
	// The server is terminating the connection due to a temporary condition, e.g. it is overloaded and is casting off some of its clients.
	TryAgainLater WsCode = 1013
	// The server was acting as a gateway or proxy and received an invalid response from the upstream server. This is similar to 502 HTTP Status Code.
	BadGateway WsCode = 1014
	// Indicates that the connection was closed due to a failure to perform a TLS handshake (e.g., the server certificate can't be verified).
	TLSHandshake WsCode = 1015

	// Endpoint must be authorized to perform application-based request. Equivalent to HTTP 401
	UnAuthorized WsCode = 3000
	// Endpoint is authorized but has no permissions to perform application-based request. Equivalent to HTTP 403
	Forbidden WsCode = 3003
	// Endpoint took too long to respond to application-based request. Equivalent to HTTP 408
	Timeout WsCode = 3008
)

func (c WsCode) GetCloseReason() string {
	var r string

	switch c {
	case NormalClosure:
		r = "Normal Closure"
	case GoAway:
		r = "Going Away"
	case ProtocolError:
		r = "Protocol Error"
	case UnsupportedData:
		r = "Unsupported Data"
	case NoStatusRecived:
		r = "No Status Rcvd"
	case AbnormalClosure:
		r = "Abnormal Closure"
	case InvalideFramePayload:
		r = "Invalid Frame Payload Data"
	case PolicyVoiolation:
		r = "Policy Violation"
	case MessageTooBig:
		r = "Message Too Big"
	case MandatoryExtension:
		r = "Mandatory Ext."
	case InternalError:
		r = "Internal Error"
	case ServiceRestart:
		r = "Service Restart"
	case TryAgainLater:
		r = "Try Again Later"
	case BadGateway:
		r = "Bad Gateway"
	case TLSHandshake:
		r = "TLS handshake"

	case UnAuthorized:
		r = "Unauthorized"
	case Forbidden:
		r = "Forbidden"
	case Timeout:
		r = "Timeout"
	}

	return r
}
