package ws

import (
	"chat-system/ws/presence"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	nettyws "github.com/go-netty/go-netty-ws"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type mockNettyConn struct {
	mock.Mock
	userData any
}

// Request implements nettyws.Conn.
func (m mockNettyConn) Request() *http.Request {
	panic("unimplemented")
}

// Close implements nettyws.Conn.
func (m mockNettyConn) Close() error {
	args := m.Called()
	return args.Error(0)
}

// Context implements nettyws.Conn.
func (m mockNettyConn) Context() context.Context {
	panic("unimplemented")
}

// Header implements nettyws.Conn.
func (m mockNettyConn) Header() http.Header {
	return nil
}

// LocalAddr implements nettyws.Conn.
func (m mockNettyConn) LocalAddr() string {
	panic("unimplemented")
}

// RemoteAddr implements nettyws.Conn.
func (m mockNettyConn) RemoteAddr() string {
	return "remoteAddr"
}

// SetDeadline implements nettyws.Conn.
func (m mockNettyConn) SetDeadline(t time.Time) error {
	panic("unimplemented")
}

// SetReadDeadline implements nettyws.Conn.
func (m mockNettyConn) SetReadDeadline(t time.Time) error {
	panic("unimplemented")
}

// SetUserdata implements nettyws.Conn.
func (m mockNettyConn) SetUserdata(userdata interface{}) {
	m.userData = userdata
}

// SetWriteDeadline implements nettyws.Conn.
func (m mockNettyConn) SetWriteDeadline(t time.Time) error {
	panic("unimplemented")
}

// Userdata implements nettyws.Conn.
func (m mockNettyConn) Userdata() interface{} {
	return m.userData
}

// Write implements nettyws.Conn.
func (m mockNettyConn) Write(message []byte) error {
	panic("unimplemented")
}

// WriteClose implements nettyws.Conn.
func (m mockNettyConn) WriteClose(code int, reason string) error {
	args := m.Called(code, reason)
	return args.Error(0)
}

var _ nettyws.Conn = mockNettyConn{}

func toWsSchema(url string) string {
	return strings.Replace(url, "http", "ws", 1)
}

func TestHttpServer_onConnect(t *testing.T) {
	presence := presence.NewMemService[Client]()
	dispatcher := NewRoomDispatcher()
	wsHandler := newWsHandler(presence, dispatcher)

	cli := Client{"clientId", "userId", &errorHandledConn{}}

	clientEvCalled := make(chan bool, 1)
	dispatcher.SubscribeOnClientEvents(func(e clientEvent) {
		assert.Equal(t, e.EventType(), clientConnected, "it should sent an event with type clientConnected")

		assert.Equal(t, e.client, cli, "the event should contain the client")

		clientEvCalled <- true
	})

	wsHandler.onConnect(mockNettyConn{userData: cli})

	devices := presence.GetDevicesForUsers(cli.UserId())
	assert.Len(t, devices, 1)
	assert.Equal(t, devices[0].ClientId(), cli.ClientId(), "it should add the client to online users")

	assert.True(t, <-clientEvCalled, "onClientEvent subscriber must be called")
}

func TestHttpServer_OnClose_called(t *testing.T) {
	wsHandler := newWsHandler(presence.NewMemService[Client](), NewRoomDispatcher())

	onClosedCalled := make(chan bool, 1)
	wsHandler.websocket.OnClose = func(conn nettyws.Conn, err error) {
		_, ok := conn.Userdata().(Client)
		assert.True(t, ok, "connection's userdata must be Client")

		assert.NotNil(t, err, "websocket OnClose must not be nil")

		onClosedCalled <- true
	}

	server := httptest.NewServer(wsHandler)
	defer server.Close()

	ws := nettyws.NewWebsocket()

	conn, err := ws.Open(toWsSchema(server.URL))
	assert.NoError(t, err, "can not connect to server")

	err = conn.Close()
	assert.NoError(t, err, "client's conn.Close should returns error")

	assert.True(t, <-onClosedCalled, "websocket's OnClosed should be called")
}

func TestHttpServer_OnClose(t *testing.T) {
	presence := presence.NewMemService[Client]()
	dispatcher := NewRoomDispatcher()
	wsHandler := newWsHandler(presence, dispatcher)

	cli := Client{"cId", "uId", &errorHandledConn{}}
	conn := mockNettyConn{userData: cli}
	wsHandler.onConnect(conn)

	devices := presence.GetDevicesForUsers(cli.UserId())
	assert.Len(t, devices, 1)
	assert.Equal(t, devices[0].ClientId(), cli.ClientId(), "when connected, the client must be added to online clients")

	clientEvCalled := make(chan bool, 1)
	dispatcher.SubscribeOnClientEvents(func(e clientEvent) {
		assert.Equal(t, e.EventType(), clientDisconnected, "it should sent an event with type clientDisconnected")

		assert.Equal(t, e.client, cli, "the event should contain the client")

		clientEvCalled <- true
	})

	wsHandler.websocket.OnClose(conn, fmt.Errorf("mock error"))

	devices = presence.GetDevicesForUsers(cli.UserId())
	assert.Len(t, devices, 0, "when disconnected, the client must be removed from online clients")

	assert.True(t, <-clientEvCalled, "event subscriber must be called")
}

func TestHttpServer_closeClient(t *testing.T) {
	presence := presence.NewMemService[Client]()
	dispatcher := NewRoomDispatcher()
	wsHandler := newWsHandler(presence, dispatcher)

	cli := &Client{"cliId", "userId", nil}
	conn := &mockNettyConn{userData: *cli}
	cli.conn = &errorHandledConn{conn: conn}

	conn.On("WriteClose", int(InternalError), InternalError.GetCloseReason()).
		Return(fmt.Errorf("mockErr"))

	conn.On("Close").Return(nil)

	wsHandler.onConnect(conn)

	if presence.IsEmpty() {
		t.Error("client must be added to presence instance")
		return
	}

	mockDispatch := mock.Mock{}
	dispatcher.SubscribeOnClientEvents(func(e clientEvent) {
		mockDispatch.MethodCalled("mockSub", e.EventType())
	})
	mockDispatch.On("mockSub", clientDisconnected).Once()

	wsHandler.closeClient(*cli, InternalError)

	if !presence.IsEmpty() {
		t.Error("closeClient should delete client from presence service")
	}

	if !mockDispatch.AssertExpectations(t) {
		t.Error("it should dispatch [clientDisconnected] event")
	}

	if !conn.AssertExpectations(t) {
		t.Error("it should write ws close frame and close the connection")
	}
}

var _ http.Handler = wsHandler{}
