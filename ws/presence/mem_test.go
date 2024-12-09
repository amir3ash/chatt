package presence

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"sync"
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
	memService := NewMemService()

	tests := []struct {
		name string
		devs []Device
	}{
		{"one client", []Device{mockDevice{"me", "client1"}}},
		{"two client", []Device{mockDevice{"user2", "cli_21"}, mockDevice{"user2", "cli_22"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := memService

			for _, device := range tt.devs {

				if err := s.Connect(ctx, device); err != nil {
					t.Errorf("MemService.Connect() error = %v", err)
				}

				if !deviceExits(t, s, device) {
					t.Errorf("MemService does store device with specific clientId, wantsClientId = %v", device.ClientId())

				}
			}
		})
	}
}

func deviceExits(t *testing.T, s *MemService, dev Device) bool {
	t.Helper()

	// load devices for ther user with his userId
	valuse, exists := s.onlinePersons.Load(dev.UserId())
	if !exists {
		return false
	}

	devices, ok := valuse.([]Device)
	if !ok {
		t.Errorf("MemService.onlinePersons not stores Device[], values: %+v", valuse)
	}

	return slices.ContainsFunc(devices, func(d Device) bool {
		return d.ClientId() == dev.ClientId() &&
			d.UserId() == dev.UserId()
	})
}

func TestMemService_Disconnected(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name         string
		devs         []Device
		disconnected Device
		wantErr      bool
	}{
		{"empty", []Device{}, mockDevice{"user", "cli"}, false},
		{"one client", []Device{mockDevice{"user", "cli"}}, mockDevice{"user", "cli"}, false},
		{"three client", []Device{
			mockDevice{"u2", "c_21"}, mockDevice{"u2", "c_22"}, mockDevice{"u2", "c_23"},
			mockDevice{"other", "oCli"},
		}, mockDevice{"u2", "c_22"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewMemService()

			// pre connected devices
			for _, device := range tt.devs {
				s.Connect(ctx, device)
			}

			if err := s.Disconnected(ctx, tt.disconnected); (err != nil) != tt.wantErr {
				t.Errorf("MemService.Disconnected() error = %v, wantErr %v", err, tt.wantErr)
			}

			if deviceExits(t, s, tt.disconnected) {
				t.Errorf("device must be deleted but exists")
			}

			for _, dev := range tt.devs {
				if dev.ClientId() == tt.disconnected.ClientId() {
					continue
				}

				if !deviceExits(t, s, dev) {
					t.Errorf("other devices must be present")
				}
			}
		})
	}
}

func TestMemService_GetOnlineClients(t *testing.T) {
	ctx := context.Background()

	genDevices := func(usersNum, devsNum int) (devs []Device) {
		cli := usersNum * devsNum
		for u := range usersNum {
			devs = append(devs, mockDevice{"u" + fmt.Sprint(u), "d" + fmt.Sprint(cli)})
			cli--
		}
		return devs
	}

	t.Run("tt.name", func(t *testing.T) {
		s := NewMemService()

		devices := genDevices(10, 3)

		for _, d := range devices {
			s.Connect(ctx, d)
		}

		got, err := s.GetOnlineClients(ctx)
		if err != nil {
			t.Errorf("MemService.GetOnlineClients() error = %v, wantErr %v", err, nil)
			return
		}

		cmp := func(a, b Device) int { return strings.Compare(a.ClientId(), b.ClientId()) }

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
		connectedDevs   []Device
		usersArg        []string
		expectedClients []string
	}{
		{"empty", nil, nil, nil},
		{"all devs", []Device{mockDevice{"u1", "cli_11"}, mockDevice{"u2", "cli_22"}},
			[]string{"u1", "u2"}, []string{"cli_11", "cli_22"}},
		{"all devs", []Device{mockDevice{"u1", "cli_11"}, mockDevice{"u1", "cli_12"}, mockDevice{"u2", "cli_22"}},
			[]string{"u1"}, []string{"cli_11", "cli_12"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			s := NewMemService()

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
				s := NewMemService()
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

				if s.len != tt.connectedDevs {
					t.Errorf("memService.len is %d, expected: %d", s.len, tt.connectedDevs)
				}
			})
		}

		if tt.expected == true {
			t.Run(tt.name, func(t *testing.T) {
				s := NewMemService()
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

				if s.len != 0 {
					t.Errorf("memService.len is %d, expected: %d", s.len, tt.connectedDevs)
				}

				if s.IsEmpty() != tt.expected {
					t.Errorf("memService.IsEmpty is %v, expected: %v", s.IsEmpty(), tt.expected)
				}
			})
		}
	}
}
