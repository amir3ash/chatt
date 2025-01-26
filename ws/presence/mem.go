package presence

import (
	"context"
	"iter"
	"slices"
	"sync"
	"sync/atomic"
)

type deviceList[T Device] struct {
	list []T
	mu   sync.Mutex
}

func newDevList[T Device](devs ...T) *deviceList[T] {
	return &deviceList[T]{devs, sync.Mutex{}}
}

func (l *deviceList[T]) insert(d T) {
	l.mu.Lock()
	l.list = append(l.list, d)
	l.mu.Unlock()
}

func (l *deviceList[T]) delete(d T) (empty bool) {
	l.mu.Lock()
	defer l.mu.Unlock()

	var nilDev T
	lastIndex := len(l.list) - 1
	clientID := d.ClientId()

	for i := 0; i <= lastIndex; i++ {
		if l.list[i].ClientId() == clientID {
			l.list[i], l.list[lastIndex] = l.list[lastIndex], nilDev
			l.list = l.list[:lastIndex]
			break
		}
	}
	return len(l.list) == 0
}

func (l *deviceList[T]) getUserDevices() (res []T) {
	l.mu.Lock()
	res = slices.Clone(l.list)
	l.mu.Unlock()

	return
}


// A util func for testing existance of device.
func (l *deviceList[T]) deviceExists(dev T) bool {
	cId := dev.ClientId()

	for _, c := range l.list {
		if c.ClientId() == cId{
			return c.UserId() == dev.UserId()
		}	
	}

	return false
}

type Person interface {
	UserId() string
}

type Device interface {
	Person
	ClientId() string // ClientId must be unique for all devices
}

type MemService[T Device] struct {
	onlinePersons *sync.Map // map<string, []Device>
	len           atomic.Int32
}

func NewMemService[T Device]() *MemService[T] {
	return &MemService[T]{&sync.Map{}, atomic.Int32{}}
}

// appends the device and returns nil.
func (s *MemService[T]) Connect(ctx context.Context, dev T) error {
	userId := dev.UserId()
	val, exists := s.onlinePersons.LoadOrStore(userId, newDevList(dev))
	if exists {
		val.(*deviceList[T]).insert(dev)
	}

	s.len.Add(1)
	return nil
}

// removes the device and returns nil.
func (s *MemService[T]) Disconnected(_ context.Context, dev T) error {
	userId := dev.UserId()

	v, ok := s.onlinePersons.Load(userId)
	if !ok {
		return nil
	}

	empty := v.(*deviceList[T]).delete(dev)

	if empty {
		s.onlinePersons.Delete(userId)
	}

	s.len.Add(-1)
	return nil
}

// func (s MemService) IsUserOnline(_ context.Context, p Person) (bool, error)

// func (s MemService) GetOnlineUsers(_ context.Context, limit int) (iter.Seq[Person], error)

// return an iterator of devices and nil.
func (s *MemService[T]) GetOnlineClients(_ context.Context) (iter.Seq[T], error) {
	return func(yield func(T) bool) {
		s.onlinePersons.Range(func(key, value any) bool {
			clients := value.(*deviceList[T]).getUserDevices()
			for _, c := range clients {
				if !yield(c) {
					return false
				}
			}

			return true
		})
	}, nil
}

// A util func for testing
func (s *MemService[T]) deviceExists(dev T) bool {
	val, ok := s.onlinePersons.Load(dev.UserId())
	if !ok {
		return false
	}
	
	return val.(*deviceList[T]).deviceExists(dev)
}

func (s *MemService[T]) GetClientsForUserId(user string) []T {
	v, ok := s.onlinePersons.Load(user)
	if !ok {
		return nil
	}
	return v.(*deviceList[T]).getUserDevices()
}

func (s *MemService[T]) IsEmpty() bool {
	empty := s.len.Load() == 0
	return empty
}

func (s *MemService[T]) GetDevicesForUsers(userIds ...string) (res []T) {
	for i := range userIds {
		clients := s.GetClientsForUserId(userIds[i])
		res = append(res, clients...)
	}
	return
}
