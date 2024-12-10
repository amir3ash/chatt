package ws

import (
	"chat-system/ws/presence"
	"fmt"
	"slices"
	"testing"
)

type mockDeviceGetter struct {
	clients []Client
}

// GetDevicesForUsers implements devicesGetter.
func (m mockDeviceGetter) GetDevicesForUsers(userIds ...string) []presence.Device {
	return nil
}

var _ devicesGetter = mockDeviceGetter{}

func TestRoomServer_getRoom(t *testing.T) {
	r := NewRoomServer(mockDeviceGetter{}, &TestAuthz{})

	room := r.getRoom("not_exist")

	if room == nil {
		t.Error("it should create new room if not exists")
		return
	}

	if room.ID != "not_exist" {
		t.Errorf("room's ID is not what requested, got: %s", room.ID)
	}
}

func TestRoomServer_joinClientToRoom(t *testing.T) {
	r := NewRoomServer(mockDeviceGetter{}, &TestAuthz{})

	room := r.getRoom("roomId")
	cli := Client{"cID", "uID", nil}

	r.joinClientToRoom(cli, room)

	if !roomContainsClient(room, cli) {
		t.Error("it should add client to the room")
	}

	if r.clientsRooms[cli.ClientId()][0] != "roomId" {
		t.Errorf("it should remember websocket rooms for every client, clientRooms: %v", r.clientsRooms)
	}
}

func TestRoomServer_leaveClientFromRoom(t *testing.T) {
	r := NewRoomServer(mockDeviceGetter{}, &TestAuthz{})

	room := r.getRoom("roomId")
	client := Client{"cID", "uID", nil}

	r.joinClientToRoom(client, room)

	if !roomContainsClient(room, client) {
		t.Error("joinClientToRoom should add the client to the room")
	}

	r.leaveClientFromRoom(client, room)

	if roomContainsClient(room, client) {
		t.Error("it should remove client from the room")
	}

	if len(r.clientsRooms[client.ClientId()]) > 0 {
		t.Errorf("it should remove websocket room for the client")
	}
}

type mockAuthorizedTopics struct {
	err error
}

// TopicsWhichUserCanWatch implements whoCanReadTopic.
func (m mockAuthorizedTopics) TopicsWhichUserCanWatch(userId string, topics []string) (topicIds []string, err error) {
	return topics, m.err
}

// WhoCanWatchTopic implements whoCanReadTopic.
func (m mockAuthorizedTopics) WhoCanWatchTopic(topicId string) ([]string, error) {
	return nil, nil
}

var _ whoCanReadTopic = mockAuthorizedTopics{}

func TestRoomServer_onClientConnected(t *testing.T) {

	cli := Client{"client", "user", nil}
	r := NewRoomServer(mockDeviceGetter{}, mockAuthorizedTopics{})
	rooms := []*room{newRoom("room1", nil), newRoom("room2", nil), newRoom("room3", nil)}
	for _, room := range rooms {
		r.rooms[room.ID] = room
	}

	r.onClientConnected(cli)

	for _, room := range rooms {
		if !roomContainsClient(room, cli) {
			t.Errorf("room server should add client to every online authorized rooms")
		}
	}
}

func TestRoomServer_onClientConnected_withError(t *testing.T) {
	cli := Client{"client", "user", nil}
	r := NewRoomServer(mockDeviceGetter{}, mockAuthorizedTopics{fmt.Errorf("mock error")})
	room := newRoom("roomId", nil)
	r.rooms[room.ID] = room

	r.onClientConnected(cli)

	if roomContainsClient(room, cli) {
		t.Errorf("room server should not add client to the room")
	}
}

func TestRoomServer_onClientDisconnected(t *testing.T) {
	cli := Client{"clientID", "userId", nil}
	other_client := Client{"client22", "other_user", nil}
	rooms := []*room{newRoom("room1", nil), newRoom("room2", nil), newRoom("room3", nil)}

	r := NewRoomServer(mockDeviceGetter{}, mockAuthorizedTopics{})

	for _, room := range rooms {
		r.rooms[room.ID] = room
	}
	rooms[0].addClient(other_client) // room "room1" has two clients
	r.onClientConnected(cli)

	clientRooms := r.clientsRooms[cli.ClientId()]
	slices.Sort(clientRooms)

	if !slices.Equal(clientRooms, []string{"room1", "room2", "room3"}) {
		t.Errorf("clients must connected to authorized rooms, clientRooms: %v", r.clientsRooms[cli.ClientId()])
		return
	}

	r.onClientDisconnected(cli)

	for _, room := range rooms {
		if roomContainsClient(room, cli) {
			t.Errorf("room server should remove client from his rooms, roomId: %s", room.ID)
		}
	}

	if len(r.rooms) != 1 {
		t.Errorf("it should remove empty rooms, rooms: %v", r.rooms)
		for _, room := range rooms {
			if !room.IsEmpty() {
				t.Errorf("room is not empty, roomId: %s", room.ID)
			}
		}
	}

	if _, found := r.rooms["room1"]; !found {
		t.Errorf("it should not remove none empty rooms")
	}
}
