package ws

import (
	"chat-system/core"
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
func (r *roomServer) getRoom(topicID string) *room {
	var roomIns *room
	r.RLock()
	roomIns = r.rooms[topicID]
	r.RUnlock()
	return roomIns
}
func (r *roomServer) SendMessageTo(topicId string, msg *core.Message) {
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
	onlinePersons    *sync.Map // map<string, []conn>
	destroyObservers []func(roomId string)
	mu               sync.RWMutex
}

func newRoom(id string, users []userWithConns) *room {
	slog.Debug("creating new room", slog.String("id", id), "users", users)
	persons := &sync.Map{}
	for _, u := range users {
		persons.Store(u.userId, u.connections)
	}
	return &room{id, persons, make([]func(string), 0), sync.RWMutex{}}
}

func (r *room) subscribeOnDestruct(f func(roomeId string)) {
	slog.Debug("room.onDestruct called", slog.String("roomId", r.ID))
	r.destroyObservers = append(r.destroyObservers, f)
}

func (r *room) addConn(c conn) {
	slog.Debug("room.addConn called.", slog.String("userId", c.getUserId()))
	r.mu.Lock()
	defer r.mu.Unlock()

	userId := c.getUserId()
	connections, ok := r.onlinePersons.Load(userId)
	if ok {
		connections = append(connections.([]conn), c)
	} else {
		connections = []conn{c}
	}

	r.onlinePersons.Store(userId, connections)
}

func (r *room) onDisconnect(c conn) { // called when client disconnected
	slog.Debug("room.onDisconnect called.", slog.String("userId", c.getUserId()))
	userId := c.getUserId()

	r.mu.Lock()
	defer r.mu.Unlock()

	v, ok := r.onlinePersons.Load(userId)
	if !ok {
		return
	}
	connections := v.([]conn)

	connections = findAndDelete(connections, c)

	if len(connections) == 0 {
		r.onlinePersons.Delete(userId)
	} else {
		r.onlinePersons.Store(userId, connections)
	}
}
func (r *room) SendMessage(m *core.Message) { // maybe message will be inconsistence with DB
	slog.Debug("room.SendMessage called", slog.String("topicId", m.TopicID), "onlinepersions", r.onlinePersons)
	bytes, _ := json.Marshal(m)

	r.onlinePersons.Range(func(key, value any) bool {
		connections := value.([]conn)
		for i := range connections {
			v := connections[i]
			if v == nil {
				continue
			}
			workerIns.do(v.getUserId(), func() {
				if v == nil {
					return
				}
				v.sendBytes(bytes)
			})
		}
		return true
	})

}

type conn interface {
	getUserId() string
	sendBytes(b []byte)
}

type disconnectSubscriber interface {
	onDisconnect(c conn)
}

type wsServer struct {
	onlineClients  map[string][]conn // connections for a userId
	connectSubs    []func(conn)
	disconnectSubs []disconnectSubscriber
	mu             sync.RWMutex
}

func NewWSServer() *wsServer {
	s := &wsServer{make(map[string][]conn), []func(conn){}, []disconnectSubscriber{}, sync.RWMutex{}}
	// go s.distbuteMessages()
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
