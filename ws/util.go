package ws

import (
	"chat-system/authz"
	"chat-system/ws/presence"
	"hash/fnv"
	"log/slog"
	"math/rand/v2"
)

func Run(watcher MessageWatcher, authz *authz.Authoriz) {
	onlineUsersPresence := presence.NewMemService[Client]()

	roomServer := NewRoomServer(onlineUsersPresence, NewWSAuthorizer(authz))

	dispatcher := NewRoomDispatcher()

	dispatcher.SubscribeOnClientEvents(func(e clientEvent) {
		if e.EventType() == clientConnected {
			roomServer.onClientConnected(e.client)
		} else {
			roomServer.onClientDisconnected(e.client)
		}
	})

	httpServer := newHttpServer(onlineUsersPresence, dispatcher)
	httpServer.RunServer()

	ReadChangeStream(watcher, roomServer)
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
	j := make([]chan workerJob, num)
	for i := range j {
		j[i] = make(chan workerJob, 1)
	}

	return shardedWriter{num: num, workerJobs: j}
}

// Writes to client's [Conn] in a seperate goroutine.
// It logs errors.
//
//  1. It prevent DoS in case of slow connection (ex. full tcp queues) by sharding
//     clients between goroutines.
//  2. It prevents race conditions (ex. unordered messages or unexpected closed connection errors)
func (w shardedWriter) writeTo(cli Client, msg []byte) {
	index := getHash(cli.userId) % w.num
	w.workerJobs[index] <- workerJob{cli: cli, message: msg}
}

func (w shardedWriter) run() {
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
	if err := client.Conn().Write(bytes); err != nil {
		// never here
		slog.Error("can not write to client's connection",
			slog.String("userId", client.UserId()),
			slog.String("clientId", client.ClientId()),
			"err", err)
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
