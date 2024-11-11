package ws

import (
	"chat-system/core/messages"
	"chat-system/ws/presence"
	"context"
	"encoding/json"
	"hash/fnv"
	"log/slog"
	"sync"
)

type whoCanReadTopic interface {
	WhoCanWatchTopic(topicId string) ([]string, error)
	TopicsWhichUserCanWatch(userId string, topics []string) (topicIds []string, err error)
}

type roomServer struct {
	broker *wsServer
	authz  whoCanReadTopic
	rooms  map[string]*room
	sync.RWMutex
}

var workerIns = newShardedWorker(16)

func NewRoomServer(b *wsServer, authz whoCanReadTopic) *roomServer {
	go workerIns.run()
	server := roomServer{b, authz, make(map[string]*room), sync.RWMutex{}}

	b.OnConnect(func(c conn) {
		serverTopics := make([]string, 0, len(server.rooms))
		for roomId := range server.rooms {
			serverTopics = append(serverTopics, roomId)
		}
		slog.Debug("OnConnect in roomserver", "rooms", server.rooms)

		topics, err := authz.TopicsWhichUserCanWatch(c.UserId(), serverTopics)
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
func (r *roomServer) getRoom(topicID string) *room {
	var roomIns *room
	r.RLock()
	roomIns = r.rooms[topicID]
	r.RUnlock()
	return roomIns
}
func (r *roomServer) SendMessageTo(topicId string, msg *messages.Message) {
	room := r.getRoom(topicId)
	if room == nil {
		room = r.createRoom(topicId)
	}

	room.SendMessage(msg)
}

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

	userConnections := r.broker.findConnectionsForUsers(userIds)
	slog.Debug("roomServer.createRoom", "topicId", topicId, "userConns", userConnections)
	room = newRoom(topicId, userConnections)
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
	onlinePersons    *presence.MemService
	destroyObservers []func(roomId string)
	mu               sync.RWMutex
}

func newRoom(id string, connections []conn) *room {
	slog.Debug("creating new room", slog.String("id", id))
	persons := presence.NewMemService()
	ctx := context.Background()

	for _, c := range connections { // TODO: optimize it
		persons.Connect(ctx, c)
	}

	return &room{id, persons, make([]func(string), 0), sync.RWMutex{}}
}

func (r *room) subscribeOnDestruct(f func(roomeId string)) {
	slog.Debug("room.onDestruct called", slog.String("roomId", r.ID))
	r.destroyObservers = append(r.destroyObservers, f)
}

func (r *room) addConn(c conn) {
	slog.Debug("room.addConn called.", slog.String("userId", c.UserId()))
	r.onlinePersons.Connect(context.Background(), c)
}

func (r *room) onDisconnect(c conn) { // called when client disconnected
	slog.Debug("room.onDisconnect called.", slog.String("userId", c.UserId()))
	r.onlinePersons.Disconnected(context.Background(), c)
}

func (r *room) SendMessage(m *messages.Message) { // maybe message will be inconsistence with DB
	slog.Debug("room.SendMessage called", slog.String("topicId", m.TopicID), "onlinepersions", r.onlinePersons)
	bytes, _ := json.Marshal(m)

	clients, _ := r.onlinePersons.GetOnlineClients(context.Background())

	for dev := range clients {
		client, ok := dev.(conn)
		if !ok {
			slog.Warn("connection is nil")
			continue
		}

		workerIns.do(client.UserId(), func() {
			client.SendBytes(bytes)
		})
	}
}

type conn interface {
	UserId() string
	ClientId() string
	SendBytes(b []byte)
}

type disconnectSubscriber interface {
	onDisconnect(c conn)
}

type wsServer struct {
	onlineClients  *presence.MemService
	connectSubs    []func(conn)
	disconnectSubs []disconnectSubscriber
	mu             sync.RWMutex
}

func NewWSServer() *wsServer {
	s := &wsServer{presence.NewMemService(), []func(conn){}, []disconnectSubscriber{}, sync.RWMutex{}}
	return s
}

func (b *wsServer) OnConnect(f func(conn)) {
	b.mu.Lock()
	b.connectSubs = append(b.connectSubs, f)
	b.mu.Unlock()
}
func (b *wsServer) registerOnDisconnect(o disconnectSubscriber) {
	b.mu.Lock()
	b.disconnectSubs = append(b.disconnectSubs, o)
	b.mu.Unlock()
}
func (b *wsServer) unRegisterOnDisconnect(o disconnectSubscriber) {
	b.mu.Lock()
	b.disconnectSubs = findAndDelete(b.disconnectSubs, o)
	b.mu.Unlock()
}
func (b *wsServer) AddConn(c conn) {
	b.onlineClients.Connect(context.Background(), c)

	for _, function := range b.connectSubs {
		function(c)
	}
}
func (b *wsServer) RemoveConn(c conn) {
	b.onlineClients.Disconnected(context.Background(), c)

	for _, s := range b.disconnectSubs {
		s.onDisconnect(c)
	}
}

func (b *wsServer) findConnectionsForUsers(userIds []string) (res []conn) {
	slog.Debug("findConnectionsForUsers", "connections", b.onlineClients, "userIds", userIds)

	for _, u := range userIds {
		if conns := b.onlineClients.GetClientsForUserId(u); len(conns) != 0 {
			for _, c := range conns {
				res = append(res, c.(conn))
			}
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

// -----------------

type workerJob func()
type shardedWorker struct {
	num        uint32
	workerJobs []chan workerJob
}

// Creates multiple job chans and goroutins to shard jobs by the hash of the string
func newShardedWorker(num uint32) shardedWorker {
	j := make([]chan workerJob, num)
	for i := range j {
		j[i] = make(chan workerJob, 1)
	}

	return shardedWorker{num: num, workerJobs: j}
}

func (w shardedWorker) do(id string, f func()) {
	index := getHash(id) % w.num
	w.workerJobs[index] <- f
}

func (w shardedWorker) run() {
	for i := uint32(0); i < w.num; i++ {
		go func(workerIdx uint32) {
			jobs := w.workerJobs[workerIdx]
			for job := range jobs {
				job()
			}
		}(i)
	}
}

func getHash(s string) uint32 {
	h := fnv.New32a()
	h.Write([]byte(s))
	return h.Sum32()
}
