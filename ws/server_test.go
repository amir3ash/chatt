package ws

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
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
	defer func() {
		err := wsServer.Shutdown(context.Background())
		if err != nil {
			t.Errorf("shutdown failed: %v", err)
		}
	}()

	go func() {
		err := wsServer.ListenAndServe("127.0.0.1:") // listen on random port
		if err != http.ErrServerClosed {
			t.Error(err)
		}
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

func TestAllowedOriginMiddleware(t *testing.T) {
	handler404 := http.HandlerFunc(http.NotFound)

	tests := []struct {
		name           string
		origin         string
		allowedOrigins []string
		expectPass     bool
	}{
		{"empty origin", "", []string{"example.com"}, true},
		{"empty allowed origins", "attacker.com", []string{}, true},
		{"allowed origin", "example.com", []string{"example.com"}, true},
		{"not-exists", "attacker.com", []string{"example.com"}, false},
		{"subdomain-not-exists", "sub.example.com", []string{"example.com"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			req.Header.Set("Origin", tt.origin)

			recorder := httptest.NewRecorder()

			handler := AllowedOriginsMiddleware(handler404, tt.allowedOrigins)
			handler.ServeHTTP(recorder, req)

			if tt.expectPass {
				if recorder.Code != 404 {
					t.Errorf("it should pass to next handler: code=%d, expected=404", recorder.Code)
				}
			} else {
				if recorder.Code != 403 {
					t.Errorf("it should prohibit the request, code=%d, expected=403", recorder.Code)
				}

				expectedBody := "Origin is not allowed."
				if body := recorder.Body.String(); body != expectedBody {
					t.Errorf("it should write reason message, body=%s, expected=%s", body, expectedBody)
				}
			}
		})
	}
}
