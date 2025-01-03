package ws

import (
	"chat-system/authz"
	"chat-system/ws/presence"
	"hash/fnv"
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
