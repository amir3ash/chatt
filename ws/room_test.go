package ws

import (
	"chat-system/core"
	"encoding/json"
	"testing"
	"time"
)

type MockConnection struct {
	received []byte
}

func (m MockConnection) getUserId() string {
	return "343"
}
func (m *MockConnection) sendBytes(b []byte) {
	m.received = b
}

func Test_SendMessageTo(t *testing.T) {
	broker := NewWSServer()
	roomServer := NewRoomServer(broker, &TestAuthz{})

	msg := &core.Message{ID: "2323"}
	expectedBytes, _ := json.Marshal(msg)

	mockConn := &MockConnection{[]byte("not_called")}
	broker.AddConnByPersonID(mockConn, "343")

	wait := make(chan bool)

	go func() {
		roomServer.SendMessageTo("hell", msg)
		time.Sleep(200 * time.Millisecond)
		wait <- false
	}()

	<-wait
	if string(mockConn.received) != string(expectedBytes) {
		t.Errorf("message bytes not equal, got %s, expected %s", mockConn.received, expectedBytes)
	}
}
