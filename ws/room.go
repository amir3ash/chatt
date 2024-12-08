package ws

import (
	"chat-system/core/messages"
	"chat-system/ws/presence"
	"context"
	"encoding/json"
	"log/slog"
	"sync"
)

var workerIns shardedWorker

type whoCanReadTopic interface {
	WhoCanWatchTopic(topicId string) ([]string, error)
	TopicsWhichUserCanWatch(userId string, topics []string) (topicIds []string, err error)
}

func init() {
	workerIns = newShardedWorker(16)
	go workerIns.run()
}

type roomServer struct {
	broker *wsServer
	authz  whoCanReadTopic
	rooms  map[string]*room
	sync.RWMutex
}

func NewRoomServer(b *wsServer, authz whoCanReadTopic) *roomServer {
	server := roomServer{b, authz, make(map[string]*room), sync.RWMutex{}}

	b.OnConnect(func(c Client) {
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
			server.rooms[topicId].addClient(c)
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

func newRoom(id string, connections []Client) *room {
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

func (r *room) addClient(c Client) {
	slog.Debug("room.addConn called.", slog.String("userId", c.UserId()))
	r.onlinePersons.Connect(context.Background(), c)
}

func (r *room) removeClient(c Client) {
	slog.Debug("room.removeClient called.", slog.String("clientId", c.ClientId()))
	r.onlinePersons.Disconnected(context.Background(), c)
}

func (r *room) onDisconnect(c Client) { // called when client disconnected
	r.removeClient(c)
}

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
