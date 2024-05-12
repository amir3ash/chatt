package ws

import (
	"fmt"
	"log/slog"
	"net/http"

	nettyws "github.com/go-netty/go-netty-ws"
	"github.com/gofiber/fiber/v2"
)

func RunServer(app *fiber.App, broker *wsServer) {
	// TODO add allowed origins to prevent CSRF
	go func() {
		fmt.Println("listen on ws://:7100")
		wsH := setupWsHandler(broker)

		http.Handle("/ws", wsH)

		if err := http.ListenAndServe(":7100", wsH); err != nil {
			slog.Error("http can't listen on port 7100", "err", err)
		}
	}()
}

type nettyConnection struct {
	conn   nettyws.Conn
	userId string
	onErr  func(error)
}

func (c nettyConnection) getUserId() string {
	return c.userId
}

func (c nettyConnection) sendBytes(b []byte) {
	err := c.conn.Write(b)
	if err != nil {
		slog.Error("cant write to websocket", "err", err)
		go c.onErr(err)
	}
}

func setupWsHandler(broker *wsServer) *nettyws.Websocket {
	wsh := nettyws.NewWebsocket(
		nettyws.WithAsyncWrite(512, false),
		// nettyws.WithBufferSize(2048, 2048),
		nettyws.WithNoDelay(true),
	)

	wsh.OnOpen = func(conn nettyws.Conn) {
		nettyConn := &nettyConnection{conn, randomUserId(), func(err error) {}}
		conn.SetUserdata(nettyConn)

		nettyConn.onErr = func(_ error) {
			broker.RemoveConn(nettyConn)
			conn.WriteClose(1001, "going away")
			conn.Close()
		}

		broker.AddConnByPersonID(nettyConn, nettyConn.userId)
	}

	wsh.OnClose = func(conn nettyws.Conn, err error) {
		nettyConn, ok := conn.Userdata().(*nettyConnection)
		if ok {
			broker.RemoveConn(nettyConn)
		}
		// fmt.Println("OnClose: ", conn.RemoteAddr(), ", error: ", err)
	}

	return wsh
}
