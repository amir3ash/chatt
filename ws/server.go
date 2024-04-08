package ws

import (
	"chat-system/authz"
	"fmt"
	"log/slog"
	"time"

	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
)

func RunServer(app *fiber.App, broker *wsServer) {
	wsConf := websocket.Config{
		HandshakeTimeout: time.Second,
		// TODO add allowed origins to prevent CSRF
	}

	app.Use("/ws", func(c *fiber.Ctx) error {
		if !websocket.IsWebSocketUpgrade(c) {
			return c.SendStatus(fiber.StatusUpgradeRequired)
		}
		return c.Next()
	})

	app.Get("/ws", websocket.New(func(c *websocket.Conn) {
		defer c.Close()

		userId, ok := c.Locals(authz.UserIdCtxKey).(string)
		if !ok {
			return
		}

		conn := &connection{*c, userId}
		broker.AddConnByPersonID(conn, conn.getUserId())

		defer broker.RemoveConn(conn)

		for {
			messageType, message, err := c.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					slog.Warn("websocket read error:", err)
				}
				return
			}

			if messageType == websocket.TextMessage {
				fmt.Println(string(message))
			}
		}
	}, wsConf))
}
