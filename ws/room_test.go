package ws

import (
	"chat-system/core/messages"
	"encoding/json"
	"sync"
	"testing"
	"time"
)

type MockConnection struct {
	received []byte
	userId   string
	sync.Mutex
}

func (m *MockConnection) getUserId() string {
	m.Lock()
	defer m.Unlock()

	return m.userId
}
func (m *MockConnection) sendBytes(b []byte) {
	m.Lock()
	defer m.Unlock()

	m.received = b
}
func (m *MockConnection) getReceived() []byte {
	m.Lock()
	defer m.Unlock()
	return m.received
}

func Test_SendMessageTo(t *testing.T) {

	wsServer := NewWSServer()
	mockAuthz := NewMockTestAuthz()
	authorizedUser := mockAuthz.topics["t_5"][0]

	roomServer := NewRoomServer(wsServer, mockAuthz)

	msg := &messages.Message{ID: "2323"}
	expectedBytes, _ := json.Marshal(msg)

	mockConn := &MockConnection{[]byte("not_called"), authorizedUser, sync.Mutex{}}

	wsServer.AddConnByPersonID(mockConn, mockConn.userId)

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
