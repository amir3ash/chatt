package ws

import "testing"

func TestReadChangeStream_withConnectedClients(t *testing.T) {
	msgWatcher := newMockWatcher()
	roomserver := NewRoomServer(mockDeviceGetter{}, NewMockTestAuthz())

	go ReadChangeStream(msgWatcher, roomserver)

	conn := &mockConn{}
	roomserver.getRoom("topic").addClient(Client{"cli-id", "user", conn})

	msgWatcher.sendMockMessageTo("topic")

	if len(conn.getReceived()) == 0 {
		t.Error("got empty message bytes")
	}
}
