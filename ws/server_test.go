package ws

import (
	"fmt"
	"net/http"
	"testing"

	nettyws "github.com/go-netty/go-netty-ws"
)

// returns new connection. used for testing.
func newWsConnection(endpoint, userId string) (nettyws.Conn, error) {
	ws := nettyws.NewWebsocket(nettyws.WithClientHeader(
		http.Header{
			"Cookie": []string{fmt.Sprintf("userId=%s", userId)},
		},
	))
	return ws.Open(endpoint)
}

func TestConnectingDisconnecting_handling_events(t *testing.T) {
	watcher := newMockWatcher()
	mockAuthz := NewMockTestAuthz()

	wsServer := NewServer(watcher, mockAuthz)

	go func() {
		err := wsServer.ListenAndServe("127.0.0.1:") // listen on random port
		t.Error(err)
	}()

	wsEndpoint := wsServer.WsEndpoint()

	for range 2 {
		conns := make([]nettyws.Conn, 0, len(mockAuthz.authorizedUserIds))

		for _, userId := range mockAuthz.authorizedUserIds {
			conn, err := newWsConnection(wsEndpoint, userId)
			if err != nil {
				t.Fatal(err)
			}
			conns = append(conns, conn)
		}

		if usersLen := wsServer.onlineUsersPresence.Len(); usersLen != len(conns) {
			t.Errorf("all clients should be connected, onlineUsersLen=%d, expected=%d", usersLen, len(conns))
		}

		for _, conn := range conns {
			if err := conn.WriteClose(5006, ""); err != nil {
				t.Error("can not write close frame", err)
			}
			if err := conn.Close(); err != nil {
				t.Error("can not close conn", err)
			}
		}

		if usersLen := wsServer.onlineUsersPresence.Len(); usersLen != 0 {
			t.Errorf("all clients should be disconnected, onlineUsersLen=%d, expected=0", usersLen)
		}
	}
}
