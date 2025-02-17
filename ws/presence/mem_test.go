package presence

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

type mockDevice struct {
	userId   string
	clientId string
}

// ClientId implements Device.
func (m mockDevice) ClientId() string {
	return m.clientId
}

// UserId implements Device.
func (m mockDevice) UserId() string {
	return m.userId
}

var _ Device = mockDevice{}

func TestMemService_Connect(t *testing.T) {
	ctx := context.Background()
	memService := NewMemService[mockDevice]()

	tests := []struct {
		name string
		devs []mockDevice
	}{
		{"one client", []mockDevice{{"me", "client1"}}},
		{"two client", []mockDevice{{"user2", "cli_21"}, {"user2", "cli_22"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := memService

			for _, device := range tt.devs {

				if err := s.Connect(ctx, device); err != nil {
					t.Errorf("MemService.Connect() error = %v", err)
				}

				if !s.deviceExists(device) {
					t.Errorf("MemService does store device with specific clientId, wantsClientId = %v", device.ClientId())
				}
			}
		})
	}
}

func TestMemService_Disconnected(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name         string
		devs         []mockDevice
		disconnected mockDevice
		wantErr      bool
	}{
		{"empty", []mockDevice{}, mockDevice{"user", "cli"}, false},
		{"one client", []mockDevice{{"user", "cli"}}, mockDevice{"user", "cli"}, false},
		{"three client", []mockDevice{
			{"u2", "c_21"}, {"u2", "c_22"}, {"u2", "c_23"},
			{"other", "oCli"},
		}, mockDevice{"u2", "c_22"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewMemService[mockDevice]()

			// pre connected devices
			for _, device := range tt.devs {
				s.Connect(ctx, device)
			}

			if err := s.Disconnected(ctx, tt.disconnected); (err != nil) != tt.wantErr {
				t.Errorf("MemService.Disconnected() error = %v, wantErr %v", err, tt.wantErr)
			}

			if s.deviceExists(tt.disconnected) {
				t.Errorf("device must be deleted but exists")
			}

			for _, dev := range tt.devs {
				if dev.ClientId() == tt.disconnected.ClientId() {
					continue
				}

				if !s.deviceExists(dev) {
					t.Errorf("other devices must be present")
				}
			}
		})
	}
}

func TestMemService_GetOnlineClients(t *testing.T) {
	ctx := context.Background()

	genDevices := func(usersNum, devsNum int) (devs []mockDevice) {
		cli := usersNum * devsNum
		for u := range usersNum {
			devs = append(devs, mockDevice{"u" + fmt.Sprint(u), "d" + fmt.Sprint(cli)})
			cli--
		}
		return devs
	}

	t.Run("tt.name", func(t *testing.T) {
		s := NewMemService[mockDevice]()

		devices := genDevices(10, 3)

		for _, d := range devices {
			s.Connect(ctx, d)
		}

		got, err := s.GetOnlineClients(ctx)
		if err != nil {
			t.Errorf("MemService.GetOnlineClients() error = %v, wantErr %v", err, nil)
			return
		}

		cmp := func(a, b mockDevice) int { return strings.Compare(a.ClientId(), b.ClientId()) }

		gotSlice := slices.Collect(got)

		slices.SortFunc(gotSlice, cmp)
		slices.SortFunc(devices, cmp)

		if !reflect.DeepEqual(gotSlice, devices) {
			t.Errorf("all devices are not equal, expect %v, got %v", devices, gotSlice)
		}
	})
}

func TestMemService_GetDevicesForUsers(t *testing.T) {
	tests := []struct {
		name            string
		connectedDevs   []mockDevice
		usersArg        []string
		expectedClients []string
	}{
		{"empty", nil, nil, nil},
		{"all devs", []mockDevice{{"u1", "cli_11"}, {"u2", "cli_22"}},
			[]string{"u1", "u2"}, []string{"cli_11", "cli_22"}},
		{"all devs", []mockDevice{{"u1", "cli_11"}, {"u1", "cli_12"}, {"u2", "cli_22"}},
			[]string{"u1"}, []string{"cli_11", "cli_12"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			s := NewMemService[mockDevice]()

			for _, dev := range tt.connectedDevs {
				s.Connect(ctx, dev)
			}

			devices := s.GetDevicesForUsers(tt.usersArg...)

			for _, dev := range devices {
				if !slices.Contains(tt.expectedClients, dev.ClientId()) {
					t.Errorf("clientId not exists in users' devices, got: %v, shouldContainsClient: %s, users: %v",
						devices, dev.ClientId(), tt.usersArg)
				}
			}
		})
	}
}

func TestMemService_IsEmpty(t *testing.T) {
	tests := []struct {
		name          string
		connectedDevs int
		expected      bool
	}{
		{"empty", 0, true},
		{"low num _ not empty", 10, false},
		{"low num _ empty", 10, true},
		{"med num _ empty", 150, true},
	}

	for _, tt := range tests {
		if tt.expected == false {
			t.Run(tt.name, func(t *testing.T) {
				s := NewMemService[mockDevice]()
				wg := sync.WaitGroup{}
				wg.Add(tt.connectedDevs)

				for range tt.connectedDevs {
					go func() {
						if err := s.Connect(context.Background(), mockDevice{}); err != nil {
							t.Error("can not add device to memservice")
						}
						wg.Done()
					}()
				}
				wg.Wait()

				if int(s.len.Load()) != tt.connectedDevs {
					t.Errorf("memService.len is %d, expected: %d", s.len.Load(), tt.connectedDevs)
				}
			})
		}

		if tt.expected == true {
			t.Run(tt.name, func(t *testing.T) {
				s := NewMemService[mockDevice]()
				wg := sync.WaitGroup{}
				wg.Add(tt.connectedDevs)

				for i := range tt.connectedDevs {
					go func(i int) {
						userId := fmt.Sprint(i)
						if err := s.Connect(context.Background(), mockDevice{userId: userId}); err != nil {
							t.Error("can not add device to memservice")
						}

						if err := s.Disconnected(context.Background(), mockDevice{userId: userId}); err != nil {
							t.Error("can not remove device from memservice")
						}
						wg.Done()
					}(i)
				}
				wg.Wait()

				if s.len.Load() != 0 {
					t.Errorf("memService.len is %d, expected: %d", s.len.Load(), tt.connectedDevs)
				}

				if s.IsEmpty() != tt.expected {
					t.Errorf("memService.IsEmpty is %v, expected: %v", s.IsEmpty(), tt.expected)
				}
			})
		}
	}
}

func BenchmarkConnectSameUser(b *testing.B) {
	ctx := context.Background()
	s := NewMemService[mockDevice]()
	b.SetParallelism(400)
	i := atomic.Uint32{}

	b.RunParallel(func(p *testing.PB) {
		for p.Next() {
			clientId := fmt.Sprint(i.Load())
			err := s.Connect(ctx, mockDevice{userId: "user", clientId: clientId})
			if err != nil {
				b.Fatal(err)
			}
		}
		i.Add(1)
	})
}

func BenchmarkConnectDiffrentUsers(b *testing.B) {
	ctx := context.Background()
	s := NewMemService[mockDevice]()
	b.SetParallelism(400)
	i := atomic.Uint32{}

	b.RunParallel(func(p *testing.PB) {
		for p.Next() {
			userId := fmt.Sprint(i.Load())
			err := s.Connect(ctx, mockDevice{userId: userId, clientId: "cli"})
			if err != nil {
				b.Fatal(err)
			}
			i.Add(1)
		}
	})
}

func BenchmarkMultipleOperations(b *testing.B) {
	ctx := context.Background()
	s := NewMemService[mockDevice]()
	b.SetParallelism(400)
	i := atomic.Uint32{}

	b.RunParallel(func(p *testing.PB) {
		for p.Next() {
			userId := fmt.Sprint(i.Load())
			dev := mockDevice{userId: userId, clientId: "cli"}
			err := s.Connect(ctx, dev)
			if err != nil {
				b.Fatal(err)
			}

			i.Add(1)

			err = s.Disconnected(ctx, dev)
			if err != nil {
				b.Fatal(err)
			}

			s.GetOnlineClients(ctx)
		}
	})
}

func BenchmarkGetOnlineClients(b *testing.B) {
	ctx := context.Background()
	s := NewMemService[mockDevice]()

	for i := range 1000 {
		uId := fmt.Sprint(i + 1)
		for j := range 4 {
			s.Connect(ctx, mockDevice{userId: uId, clientId: fmt.Sprint(j * i)})
		}
	}

	b.ResetTimer()
	b.RunParallel(func(p *testing.PB) {
		for p.Next() {
			devices, _ := s.GetOnlineClients(ctx)
			for a := range devices {
				if a.userId == a.UserId() {
					continue
				}
			}
		}
	})
}
