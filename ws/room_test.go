package ws

import (
	"bytes"
	"chat-system/core/messages"
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"slices"
	"sync"
	"testing"
)

type mockConn struct {
	sync.Mutex
	ch  chan []byte
	err error
}

func (m *mockConn) Write(b []byte) error {
	m._getCh() <- b
	return m.err
}
func (m *mockConn) getReceived() []byte {
	b := <-m._getCh()
	return b
}

func (m *mockConn) _getCh() chan []byte {
	m.Lock()
	defer m.Unlock()

	if m.ch == nil {
		m.ch = make(chan []byte, 1)
	}
	return m.ch
}

type MockClient struct {
	userId   string
	clientId string
	conn     mockConn
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

func roomContainsClient(r *room, cli Client) bool {
	return slices.ContainsFunc(
		r.onlinePersons.GetClientsForUserId(cli.UserId()),
		func(dev Client) bool {
			return dev.ClientId() == cli.clientId && dev.UserId() == cli.UserId()
		})
}

func Test_addClient(t *testing.T) {
	room := newRoom("room_id", []Client{})
	cli := Client{"cli", "userid", &mockConn{}}

	room.addClient(cli)

	if !roomContainsClient(room, cli) {
		t.Errorf("the room should contains the client")
	}
}

func Test_removeClient(t *testing.T) {
	cli := Client{"cli", "userid", &mockConn{}}
	room := newRoom("room_id", []Client{cli})

	if !roomContainsClient(room, cli) {
		t.Error("it should initialized with clients")
		return
	}

	room.removeClient(cli)

	if roomContainsClient(room, cli) {
		t.Error("room should remove the client")
	}
}

func Test_removeNoneExistanceClient(t *testing.T) {
	// it should not panic when the client not exists

	cli := Client{"cli", "userid", &mockConn{}}
	room := newRoom("room_id", []Client{})

	room.removeClient(cli)
}

func Test_SendMessage_writingToConn(t *testing.T) {
	conn := mockConn{}
	cli := Client{"cli", "userid", &conn}
	room := newRoom("room_id", []Client{cli})
	msg := messages.Message{ID: "msgId"}

	room.SendMessage(context.Background(), &msg)

	expected, _ := json.Marshal(&msg)
	gotBytes := conn.getReceived()

	if !bytes.Equal(expected, gotBytes) {
		t.Errorf("it should send serialized message, got %s, expected %s", gotBytes, expected)
	}
}

func TestSendMessageWithError(t *testing.T) {
	conn := mockConn{err: fmt.Errorf("mock error")}
	cli := Client{"cli", "userid", &conn}
	room := newRoom("room_id", []Client{cli})
	msg := messages.Message{ID: "msgId"}

	room.SendMessage(context.Background(), &msg)
}	

func Benchmark(b *testing.B) {
	s := NewRoomServer(mockDeviceGetter{}, mockAuthorizedTopics{})
	for i := range 10000 {
		s.createRoom(fmt.Sprint(i))
	}
	b.ResetTimer()
	b.SetParallelism(30_000)

	b.RunParallel(func(p *testing.PB) {
		i := rand.Int64N(1000000)
		for p.Next() {
			cId := fmt.Sprint(i)
			s.onClientConnected(Client{cId, cId, nil})
			i++
			i = i % 5000
		}
	})
}
