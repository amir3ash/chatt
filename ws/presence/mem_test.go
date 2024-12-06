package presence

import "fmt"

import (
	"context"
	"reflect"
	"slices"
	"strings"
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
