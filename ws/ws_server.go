package ws

import (
	"chat-system/ws/presence"
	"context"
	"log/slog"
	"sync"
)

type disconnectSubscriber interface {
	onDisconnect(c Client)
}

type wsServer struct {
	onlineClients  *presence.MemService
	connectSubs    []func(Client)
	disconnectSubs []disconnectSubscriber
	mu             sync.RWMutex
}

func NewWSServer() *wsServer {
	s := &wsServer{
		presence.NewMemService(),
		[]func(Client){},
		[]disconnectSubscriber{},
		sync.RWMutex{},
	}
	return s
}

func (b *wsServer) OnConnect(f func(Client)) {
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

func (b *wsServer) AddConn(c Client) {
	b.onlineClients.Connect(context.Background(), c)

	for _, function := range b.connectSubs {
		function(c)
	}
}

func (b *wsServer) RemoveConn(c Client) {
	b.onlineClients.Disconnected(context.Background(), c)

	for _, s := range b.disconnectSubs {
		s.onDisconnect(c)
	}
}

func (b *wsServer) findConnectionsForUsers(userIds []string) (res []Client) {
	slog.Debug("findConnectionsForUsers", "connections", b.onlineClients, "userIds", userIds)

	for _, u := range userIds {
		if conns := b.onlineClients.GetClientsForUserId(u); len(conns) != 0 {
			for _, c := range conns {
				res = append(res, c.(Client))
			}
		}
	}
	return
}
