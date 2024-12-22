package ws

import (
	"chat-system/core/messages"
	"chat-system/ws/presence"
	"context"
	"encoding/json"
	"log/slog"
	"slices"
	"sync"
)

// a sharded goroutine worker pool for writing to users' [Conn]
// to reduce all generated goroutine size.
var workerIns shardedWorker

type whoCanReadTopic interface {
	WhoCanWatchTopic(topicId string) ([]string, error)

	// returns authorized topics for the input userId
	// among all input topics.
	TopicsWhichUserCanWatch(userId string, topics []string) (topicIds []string, err error)
}

type devicesGetter interface {
	// gets devices for online users
	GetDevicesForUsers(userIds ...string) []presence.Device
}

func init() {
	workerIns = newShardedWorker(16)
	go workerIns.run()
}

// roomServer manages all rooms
type roomServer struct {
	onlinePersons devicesGetter
	authz         whoCanReadTopic
	rooms         map[string]*room
	clientsRooms  map[string][]string // clientId -> []roomId. rooms which the user connected to.
	clientsMutex  sync.RWMutex
	sync.RWMutex
}

func NewRoomServer(b devicesGetter, authz whoCanReadTopic) *roomServer {
	server := roomServer{
		b, authz, make(map[string]*room), make(map[string][]string),
		sync.RWMutex{}, sync.RWMutex{},
	}
	return &server
}

// Adds client to room and remembers rooms which the client connected to.
func (r *roomServer) joinClientToRoom(c Client, room *room) {
	r.clientsMutex.Lock()
	defer r.clientsMutex.Unlock()

	rooms, ok := r.clientsRooms[c.ClientId()]
	if ok {
		rooms = append(rooms, room.ID)
	} else {
		rooms = []string{room.ID}
	}

	r.clientsRooms[c.ClientId()] = rooms

	room.addClient(c)
}

// Removes client from room and remove the room from user's websocket rooms.
func (r *roomServer) leaveClientFromRoom(c Client, room *room) {
	r.clientsMutex.Lock()
	defer r.clientsMutex.Unlock()

	rooms, ok := r.clientsRooms[c.ClientId()]
	if !ok {
		return
	}

	rooms = findAndDelete(rooms, room.ID)

	if len(rooms) == 0 {
		delete(r.clientsRooms, c.ClientId())
	} else {
		r.clientsRooms[c.ClientId()] = rooms
	}

	room.removeClient(c)
}

// onClientConnected gets authorzided topics
// by calling [whoCanReadTopic]'s function and
// adds the [Client] c to authorized rooms.
func (r *roomServer) onClientConnected(c Client) {
	serverTopics := make([]string, 0, len(r.rooms))
	for roomId := range r.rooms {
		serverTopics = append(serverTopics, roomId)
	}
	slog.Debug("OnConnect in roomserver", "rooms", r.rooms)

	topics, err := r.authz.TopicsWhichUserCanWatch(c.UserId(), serverTopics)
	if err != nil {
		slog.Error("can't get topics which the user can read", "err", err)
		return
	}

	for _, topicId := range topics {
		room := r.rooms[topicId]
		r.joinClientToRoom(c, room)
	}
}

// onClientDisconnected remove [Client] c from rooms which connected before.
// then deletes rooms with zero clients.
func (r *roomServer) onClientDisconnected(c Client) {
	r.clientsMutex.RLock()
	rooms := slices.Clone(r.clientsRooms[c.ClientId()])
	r.clientsMutex.RUnlock()

	r.Lock()
	defer r.Unlock()

	for _, roomId := range rooms {
		room, ok := r.rooms[roomId]
		if !ok {
			slog.Warn("client's rooms is not synchronized with rooms",
				slog.String("clientId", c.ClientId()), slog.String("roomId", roomId))
			continue
		}

		r.leaveClientFromRoom(c, room)

		if room.IsEmpty() {
			delete(r.rooms, roomId)
		}
	}
}

// returns [room] with specified topicID.
// if not found, it creates new room and returns it.
func (r *roomServer) getRoom(topicID string) *room {
	r.RLock()
	roomIns, found := r.rooms[topicID]
	r.RUnlock()

	if !found {
		roomIns = r.createRoom(topicID)
	}
	return roomIns
}

// SendMessageTo sends [messages.Message] msg to the room with specified topicId.
//
// it gets existing [room] or creates new room, and calls room's SendMessage func.
func (r *roomServer) SendMessageTo(topicId string, msg *messages.Message) {
	room := r.getRoom(topicId)
	if room == nil {
		room = r.createRoom(topicId)
	}

	room.SendMessage(msg)
}

// createRoom creates new [room] with online authorzed users.
//
//  1. it gets all authrized users by calling [whoCanReadTopic]'s WhoCanWatchTopic
//  2. filter online users
//  3. adds all online authrized users to the room.
func (r *roomServer) createRoom(topicId string) *room {
	r.Lock()
	defer r.Unlock()

	room, ok := r.rooms[topicId]
	if ok {
		return room
	}

	userIds, err := r.authz.WhoCanWatchTopic(topicId)
	if err != nil {
		slog.Error("error in calling WhoCanWatchTopic", slog.String("topicId", topicId), "err", err)
	}

	userConnections := r.onlinePersons.GetDevicesForUsers(userIds...)
	slog.Debug("roomServer.createRoom", "topicId", topicId, "userConns", userConnections)

	room = newRoom(topicId, devicesToClients(userConnections))
	r.rooms[topicId] = room

	return room
}

var _ presence.Device = Client{}

func devicesToClients(devs []presence.Device) []Client {
	res := make([]Client, 0, len(devs))
	for i := range devs {
		res = append(res, devs[i].(Client))
	}
	return res
}

// --------------

// room contains online clients for a particular topic.
type room struct {
	ID               string
	onlinePersons    *presence.MemService
	destroyObservers []func(roomId string)
	mu               sync.RWMutex
}

func newRoom(id string, connections []Client) *room {
	slog.Debug("creating new room", slog.String("id", id))
	persons := presence.NewMemService()
	ctx := context.Background()

	for _, c := range connections { // TODO: optimize it
		persons.Connect(ctx, c)
	}

	return &room{id, persons, make([]func(string), 0), sync.RWMutex{}}
}

// adds the [Client] c to online users of the [room] r.
func (r *room) addClient(c Client) {
	slog.Debug("room.addConn called.", slog.String("userId", c.UserId()))
	r.onlinePersons.Connect(context.Background(), c)
}

// removes the [Client] c from online users of the [room] r.
func (r *room) removeClient(c Client) {
	slog.Debug("room.removeClient called.", slog.String("clientId", c.ClientId()))
	r.onlinePersons.Disconnected(context.Background(), c)
}

func (r *room) IsEmpty() bool {
	return r.onlinePersons.IsEmpty()
}

// Sends [*messages.Message] to online users of the [room] r.
//
// SendMessage encodes the messages to json and send the encoded messages
// to all client's [Conn].
func (r *room) SendMessage(m *messages.Message) { // maybe message will be inconsistence with DB
	slog.Debug("room.SendMessage called", slog.String("topicId", m.TopicID), "onlinepersions", r.onlinePersons)
	bytes, _ := json.Marshal(m)

	clients, _ := r.onlinePersons.GetOnlineClients(context.Background())

	for dev := range clients {
		client, ok := dev.(Client)
		if !ok {
			slog.Warn("connection is nil")
			continue
		}

		workerIns.do(client.UserId(), func() {
			if err := client.Conn().Write(bytes); err != nil {
				// never here
				slog.Error("can not write to client's connection",
					slog.String("userId", client.UserId()),
					slog.String("clientId", client.ClientId()),
					"err", err)
			}
		})
	}
}
