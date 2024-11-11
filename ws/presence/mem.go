package presence

import (
	"context"
	"iter"
	"sync"
)

type Person interface {
	UserId() string
}

type Device interface {
	Person
	ClientId() string
}

type MemService struct {
	onlinePersons *sync.Map // map<string, []Device>
	mu            sync.Mutex
}

func NewMemService() *MemService {
	return &MemService{&sync.Map{}, sync.Mutex{}}
}

// appends the device and returns nil.
func (s *MemService) Connect(ctx context.Context, dev Device) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	userId := dev.UserId()
	connections, ok := s.onlinePersons.Load(userId)
	if ok {
		connections = append(connections.([]Device), dev)
	} else {
		connections = []Device{dev}
	}

	s.onlinePersons.Store(userId, connections)

	return nil
}

// removes the device and returns nil.
func (s *MemService) Disconnected(_ context.Context, dev Device) error {
	userId := dev.UserId()

	s.mu.Lock()
	defer s.mu.Unlock()

	v, ok := s.onlinePersons.Load(userId)
	if !ok {
		return nil
	}
	connections := v.([]Device)

	connections = findAndDelete(connections, dev)

	if len(connections) == 0 {
		s.onlinePersons.Delete(userId)
	} else {
		s.onlinePersons.Store(userId, connections)
	}

	return nil
}

// func (s MemService) IsUserOnline(_ context.Context, p Person) (bool, error)

// func (s MemService) GetOnlineUsers(_ context.Context, limit int) (iter.Seq[Person], error)

// return an iterator of devices and nil.
func (s *MemService) GetOnlineClients(_ context.Context) (iter.Seq[Device], error) {
	return func(yield func(Device) bool) {
		s.onlinePersons.Range(func(key, value any) bool {
			clients := value.([]Device)
			for _, c := range clients {
				if !yield(c) {
					return false
				}
			}

			return true
		})
	}, nil
}

func (s *MemService) GetClientsForUserId(user string) []Device {
	v, ok := s.onlinePersons.Load(user)
	if !ok {
		return nil
	}
	return v.([]Device)
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
