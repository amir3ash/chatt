package ws

import (
	"chat-system/core/messages"
	"chat-system/ws/presence"
	"context"
	"encoding/json"
	"log/slog"
	"runtime"
	"slices"
	"sync"
)

// a sharded goroutine worker pool for writing to users' [Conn]
// to reduce all generated goroutine size.
var workerIns shardedWriter

type whoCanReadTopic interface {
	WhoCanWatchTopic(topicId string) ([]string, error)

	// returns authorized topics for the input userId
	// among all input topics.
	TopicsWhichUserCanWatch(userId string, topics []string) (topicIds []string, err error)
}

type devicesGetter[T presence.Device] interface {
	// gets devices for online users
	GetDevicesForUsers(userIds ...string) []T
}

func init() {
	workerIns = newShardedWriter(min(2, uint32(runtime.NumCPU())))
	go workerIns.run()
}

// roomServer manages all rooms
type roomServer struct {
	onlinePersons devicesGetter[Client]
	authz         whoCanReadTopic
	rooms         map[string]*room
	clientsRooms  *mapList[string, *room] // clientId -> []*room. rooms which the user connected to.
	sync.RWMutex
}

func NewRoomServer(b devicesGetter[Client], authz whoCanReadTopic) *roomServer {
	server := roomServer{
		b, authz, make(map[string]*room), &mapList[string, *room]{},
		sync.RWMutex{},
	}
	return &server
}

// Adds client to room and remembers rooms which the client connected to.
func (r *roomServer) joinClientToRooms(c Client, rooms ...*room) {
	for _, room := range rooms {
		r.clientsRooms.insert(c.ClientId(), room)
		room.addClient(c)
	}
}

// Removes client from room and remove the room from user's websocket rooms.
func (r *roomServer) leaveClientFromRoom(c Client, rooms ...*room) {
	for _, room := range rooms {
		r.clientsRooms.deleteValue(c.ClientId(), room)
		room.removeClient(c)
	}
}

// onClientConnected gets authorzided topics
// by calling [whoCanReadTopic]'s function and
// adds the [Client] c to authorized rooms.
func (r *roomServer) onClientConnected(c Client) {
	serverTopics := make([]string, 0, len(r.rooms))
	for roomId := range r.rooms {
		serverTopics = append(serverTopics, roomId)
	}

	topics, err := r.authz.TopicsWhichUserCanWatch(c.UserId(), serverTopics)
	if err != nil {
		slog.Error("can't get topics which the user can read", "err", err)
		return
	}

	rooms := make([]*room, 0, 1)
	for _, topic := range topics {
		rooms = append(rooms, r.rooms[topic])
	}

	r.joinClientToRooms(c, rooms...)
}

// onClientDisconnected remove [Client] c from rooms which connected before.
// then deletes rooms with zero clients.
func (r *roomServer) onClientDisconnected(c Client) {
	rooms := r.clientsRooms.cloneValues(c.ClientId())
	r.leaveClientFromRoom(c, rooms...)

	r.Lock()
	defer r.Unlock()

	for _, room := range rooms {
		if room.IsEmpty() {
			delete(r.rooms, room.ID)
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

	room = newRoom(topicId, userConnections)
	r.rooms[topicId] = room

	return room
}

var _ presence.Device = Client{}

// --------------

// room contains online clients for a particular topic.
type room struct {
	ID            string
	onlinePersons *presence.MemService[Client]
}

func newRoom(id string, connections []Client) *room {
	slog.Debug("creating new room", slog.String("id", id))
	persons := presence.NewMemService[Client]()
	ctx := context.Background()

	for _, c := range connections { // TODO: optimize it
		persons.Connect(ctx, c)
	}

	return &room{id, persons}
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

	for client := range clients {
		workerIns.writeTo(client, bytes)
	}
}

// simple safe listContainer
type listContainer[T comparable] struct {
	list []T
	mu sync.Mutex
}
// used in [roomServer]
type mapList[K, V comparable] struct {
	m sync.Map ///map[K]*listContainer[V]
}

func (ml *mapList[K, V]) insert(key K, val V) {
	c, _ := ml.m.LoadOrStore(key, &listContainer[V]{})
	container := c.(*listContainer[V])

	container.mu.Lock()
	container.list = append(container.list, val)
	container.mu.Unlock()
}

func (ml *mapList[K, V]) deleteValue(key K, val V) {
	c, ok := ml.m.Load(key)
	if !ok {
		return
	}

	container := c.(*listContainer[V])
	
	container.mu.Lock()

	list := container.list
	lastIndex := len(list) - 1
	var nilVal V

	for i := 0; i <= lastIndex; i++ {
		if list[i] == val {
			list[i], list[lastIndex] = list[lastIndex], nilVal
			list = list[:lastIndex]
			break
		}
	}

	if len(list) == 0 {
		ml.m.Delete(key)
	} else {
		container.list = list
	}

	container.mu.Unlock()
}

func (ml *mapList[K, V]) cloneValues(key K) (values []V) {
	c, ok  := ml.m.Load(key)
	if !ok {
		return nil
	}

	container := c.(*listContainer[V])
	
	container.mu.Lock()
	values = slices.Clone(container.list)
	container.mu.Unlock()

	return
}
