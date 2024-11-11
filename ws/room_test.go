package ws

import (
	"chat-system/core/messages"
	"encoding/json"
	"sync"
	"testing"
	"time"
)

type mockConn struct {
	sync.Mutex
	received []byte
}

func (m *mockConn) Write(b []byte) error {
	m.Lock()
	defer m.Unlock()

	m.received = b
	return nil
}
func (m *mockConn) getReceived() []byte {
	m.Lock()
	defer m.Unlock()
	return m.received
}

type MockClient struct {
	userId   string
	clientId string
	conn mockConn
	sync.Mutex
}

func (m *MockClient) UserId() string {
	m.Lock()
	defer m.Unlock()

	return m.userId
}
func (m *MockClient) ClientId() string {
	m.Lock()
	defer m.Unlock()

	return m.clientId
}
func (m *MockClient) Conn() Conn {
	return &m.conn
}

func Test_SendMessageTo(t *testing.T) {

	wsServer := NewWSServer()
	mockAuthz := NewMockTestAuthz()
	authorizedUser := mockAuthz.topics["t_5"][0]

	roomServer := NewRoomServer(wsServer, mockAuthz)

	msg := &messages.Message{ID: "2323"}
	expectedBytes, _ := json.Marshal(msg)

	mockConn := &mockConn{sync.Mutex{}, []byte("not_called")}
	client := Client{"empty-clientID", authorizedUser, mockConn}

	wsServer.AddConn(client)

	wait := make(chan bool)

	go func() {
		roomServer.SendMessageTo("t_5", msg)
		time.Sleep(20 * time.Millisecond)
		wait <- false
	}()

	<-wait
	if string(mockConn.getReceived()) != string(expectedBytes) {
		t.Errorf("message bytes not equal, got %s, expected %s", mockConn.received, expectedBytes)
	}
}
