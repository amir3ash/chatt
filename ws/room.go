package ws

import (
	"chat-system/core"
	"encoding/json"
	"log/slog"
	"sync"

	"github.com/gofiber/contrib/websocket"
)

type whoCanReadTopic interface {
	WhoCanWatchTopic(topicId string) ([]string, error)
	TopicsWhichUserCanWatch(userId string, topics []string) (topicIds []string, err error)
}

type roomServer struct {
	broker *wsServer
	authz  whoCanReadTopic
	rooms  map[string]*room
}

func NewRoomServer(b *wsServer, authz whoCanReadTopic) *roomServer {
	server := roomServer{b, authz, make(map[string]*room)}

	b.OnConnect(func(c conn) {
		serverTopics := make([]string, 0, len(server.rooms))
		for roomId := range server.rooms {
			serverTopics = append(serverTopics, roomId)
		}
		slog.Debug("OnConnect in roomserver", "rooms", server.rooms)

		topics, err := authz.TopicsWhichUserCanWatch(c.getUserId(), serverTopics)
		if err != nil {
			slog.Error("can't get topics which the user can read", "err", err)
			return
		}

		for _, topicId := range topics {
			server.rooms[topicId].addConn(c)
		}
	})
	return &server
}

func (r *roomServer) SendMessageTo(topicId string, msg *core.Message) {
	room, ok := r.rooms[topicId]
	if !ok {
		room = r.createRoom(topicId)
	}

	room.SendMessage(msg)
}

func (r *roomServer) createRoom(topicId string) *room {
	userIds, err := r.authz.WhoCanWatchTopic(topicId)
	if err != nil {
		slog.Error("error in calling WhoCanWatchTopic", slog.String("topicId", topicId), "err", err)
	}

	userConnections := r.broker.findConnectionsForUsers(userIds)
	slog.Debug("roomServer.createRoom", "topicId", topicId, "userConns", userConnections)
	room := newRoom(topicId, userConnections)
	r.rooms[topicId] = room
	r.broker.registerOnDisconnect(room)

	room.subscribeOnDestruct(func(roomId string) {
		r.broker.unRegisterOnDisconnect(r.rooms[roomId])
		delete(r.rooms, roomId)
	})

	return room
}

// --------------
type room struct {
	ID               string
	onlinePersons    map[string][]conn
	destroyObservers []func(roomId string)
	mu               sync.Mutex
}

func newRoom(id string, users []userWithConns) *room {
	slog.Debug("creating new room", slog.String("id", id), "users", users)
	persons := make(map[string][]conn, len(users))
	for _, u := range users {
		persons[u.userId] = u.connections
	}
	return &room{id, persons, make([]func(string), 0), sync.Mutex{}}
}

func (r *room) subscribeOnDestruct(f func(roomeId string)) {
	slog.Debug("room.onDestruct called", slog.String("roomId", r.ID))
	r.destroyObservers = append(r.destroyObservers, f)
}

func (r *room) addConn(c conn) {
	slog.Debug("room.addConn called.", slog.String("userId", c.getUserId()))
	r.mu.Lock()

	userId := c.getUserId()
	connections := r.onlinePersons[userId]
	connections = append(connections, c)
	r.onlinePersons[userId] = connections
	r.mu.Unlock()
}

func (r *room) onDisconnect(c conn) { // called when client disconnected
	slog.Debug("room.onDisconnect called.", slog.String("userId", c.getUserId()))
	userId := c.getUserId()
	r.mu.Lock()
	connections := r.onlinePersons[userId]

	connections = findAndDelete(connections, c)

	if len(connections) == 0 {
		delete(r.onlinePersons, userId)
	} else {
		r.onlinePersons[userId] = connections
	}
	r.mu.Unlock()
}
func (r *room) SendMessage(m *core.Message) { // maybe message will be inconsistence with DB
	slog.Debug("room.SendMessage called", slog.String("topicId", m.TopicID), "onlinepersions", r.onlinePersons)
	bytes, _ := json.Marshal(m)

	for _, connections := range r.onlinePersons {
		for _, v := range connections {
			v.sendBytes(bytes)
		}
	}
}

type conn interface {
	getUserId() string
	sendBytes(b []byte)
}

// ----------

type connection struct {
	websocket.Conn
	userId string
}

func (c connection) getUserId() string {
	return c.userId
}
func (c *connection) sendBytes(b []byte) {
	err := c.WriteMessage(websocket.TextMessage, b)
	if err != nil {
		slog.Error("can't write to websocket", slog.String("userId", c.getUserId()), "err", err)
	}
}

type disconnectSubscriber interface {
	onDisconnect(c conn)
}

type wsServer struct {
	onlineClients  map[string][]conn // connections for a userId
	connectSubs    []func(conn)
	disconnectSubs []disconnectSubscriber
	mu             sync.Mutex
}

func NewWSServer() *wsServer {
	return &wsServer{make(map[string][]conn), []func(conn){}, []disconnectSubscriber{}, sync.Mutex{}}
}

func (b *wsServer) OnConnect(f func(conn)) {
	b.connectSubs = append(b.connectSubs, f)
}
func (b *wsServer) registerOnDisconnect(o disconnectSubscriber) {
	b.disconnectSubs = append(b.disconnectSubs, o)
}
func (b *wsServer) unRegisterOnDisconnect(o disconnectSubscriber) {
	b.disconnectSubs = findAndDelete(b.disconnectSubs, o)
}
func (b *wsServer) AddConnByPersonID(c conn, id string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	conns := b.onlineClients[id]

	conns = append(conns, c)
	b.onlineClients[id] = conns

	for _, function := range b.connectSubs {
		function(c)
	}
}
func (b *wsServer) RemoveConn(c conn) {
	func() {
		b.mu.Lock()
		defer b.mu.Unlock()

		userId := c.getUserId()
		conns4person := b.onlineClients[userId]

		conns4person = findAndDelete(conns4person, c)

		if len(conns4person) == 0 {
			delete(b.onlineClients, userId)
		} else {
			b.onlineClients[userId] = conns4person
		}
	}()

	for _, s := range b.disconnectSubs {
		s.onDisconnect(c)
	}
}

type userWithConns struct {
	userId      string
	connections []conn
}

func (b *wsServer) findConnectionsForUsers(userIds []string) (res []userWithConns) {
	slog.Debug("findConnectionsForUsers", "connections", b.onlineClients, "userIds", userIds)
	for _, u := range userIds {
		if conns := b.onlineClients[u]; conns != nil {
			newConns := make([]conn, len(conns))
			copy(newConns, conns)
			res = append(res, userWithConns{u, newConns})
		}
	}
	return
}

// Deletes item from slice then insert zero value at end (for GC).
// Be careful, it reorders the slice
func findAndDelete[T comparable](list []T, elem T) []T {
	var zero T
	lastIdx := len(list) - 1
	for i := range list {
		if list[i] == elem {
			list[i] = list[lastIdx]
			list[lastIdx] = zero
			list = list[:lastIdx]
			return list
		}
	}
	return list
}
