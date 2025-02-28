package ws

import (
	"chat-system/ws/presence"
	"context"
	"fmt"
	"hash/fnv"
	"log/slog"
	"math/rand/v2"
	"net"
	"net/http"
)

func NewServer(watcher MessageWatcher, authz whoCanReadTopic) *Server {
	onlineUsersPresence := presence.NewMemService[Client]()

	r := &Server{
		Watcher:             watcher,
		Authz:               authz,
		onlineUsersPresence: onlineUsersPresence,
		roomServer:          NewRoomServer(onlineUsersPresence, authz),
		roomDispatcher:      NewRoomDispatcher(),
	}

	r.registerEventHandlers()
	r.setupWsHandler()

	return r
}

type Server struct {
	Watcher        MessageWatcher
	Authz          whoCanReadTopic
	AllowedOrigins []string

	onlineUsersPresence *presence.MemService[Client]
	roomServer          *roomServer
	roomDispatcher      *roomDispatcher
	wsHandler           wsHandler
	httpServer          *http.Server
	wsURL               string
}

func (m *Server) ListenAndServe(addr string) error {
	go ReadChangeStream(m.Watcher, m.roomServer)

	m.httpServer = &http.Server{Addr: addr}
	m.httpServer.RegisterOnShutdown(func() {
		err := m.wsHandler.shutdown()
		if err != nil {
			slog.Error("can not shutdown websocket", "err", err)
		}
	})

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	m.wsURL = "ws://" + ln.Addr().String()

	return m.httpServer.Serve(ln)
}

func (s *Server) setupWsHandler() {
	// TODO add allowed origins to prevent CSRF
	s.wsHandler = newWsHandler(s.onlineUsersPresence, s.roomDispatcher)
	handler := wsHandler.setupHttpMiddlewares(s.wsHandler)

	http.Handle("/ws", handler)
}

func (s *Server) registerEventHandlers() {
	s.roomDispatcher.SubscribeOnClientEvents(func(e clientEvent) {
		if e.EventType() == clientConnected {
			err := s.roomServer.onClientConnected(e.client)
			if err != nil {
				slog.Error("onClientConneted fails", slog.String("clientId", e.client.ClientId()), "err", err)
				s.wsHandler.closeClient(e.client, InternalError)
			}
		} else {
			err := s.roomServer.onClientDisconnected(e.client)
			if err != nil {
				slog.Error("onClientDisconneted fails", slog.String("clientId", e.client.ClientId()), "err", err)
			}
		}
	})
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

// return websocket endpoint (like ws://127.0.0.1:7100/ws).
//
// Note: it must be called after calling listen.
func (s *Server) WsEndpoint() string {
	if s.wsURL == "" {
		panic("Manager: WsEndpoint(): unkown addr")
	}
	return s.wsURL + "/ws"
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

type workerJob struct {
	cli     Client
	message []byte
}
type shardedWriter struct {
	num        uint32
	workerJobs []chan workerJob
}

// Creates multiple job chans and goroutins
// to shard jobs by the hash of the client's userId.
func newShardedWriter(num uint32) shardedWriter {
	if num == 0 {
		panic(fmt.Errorf("can not create new shardedWriter with zero workers"))
	}

	j := make([]chan workerJob, num)
	for i := range j {
		j[i] = make(chan workerJob, 30)
	}

	return shardedWriter{num: num, workerJobs: j}
}

// Writes to client's [Conn] in a seperate goroutine.
// It logs errors.
//
//  1. It prevent DoS in case of slow connection (ex. full tcp queues) by sharding
//     clients between goroutines.
//  2. It prevents race conditions (ex. unordered messages or unexpected closed connection errors)
func (w *shardedWriter) writeTo(cli Client, msg []byte) {
	index := getHash(cli.userId) % w.num
	w.workerJobs[index] <- workerJob{cli: cli, message: msg}
}

func (w *shardedWriter) run() {
	for i := uint32(0); i < w.num; i++ {
		go func(workerIdx uint32) {
			jobs := w.workerJobs[workerIdx]
			for job := range jobs {
				w.write(&job.cli, job.message)
			}
		}(i)
	}
}

func (w shardedWriter) write(client *Client, bytes []byte) {
	conn := client.Conn()
	if conn == nil {
		slog.Error("client's connection is nil")
		return
	}

	if err := conn.Write(bytes); err != nil {
		// never here
		slog.Error("can not write to client's connection",
			slog.String("userId", client.UserId()),
			slog.String("clientId", client.ClientId()),
			"err", err)
	}
}

func (w shardedWriter) Close() {
	for _, ch := range w.workerJobs {
		close(ch)
	}
}

func getHash(s string) uint32 {
	h := fnv.New32a()
	h.Write([]byte(s))
	return h.Sum32()
}

// returns "-A8df" like random string which starts with '-'.
func randomClientIdSuffix() string {
	const s = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	const all = uint32(len(s))
	const numChars = 4

	suffix := make([]byte, numChars+1)
	suffix[0] = byte('-')
	for i := 1; i < numChars+1; i++ {
		b := s[rand.Uint32N(all)]
		suffix[i] = b
	}

	return string(suffix)
}
