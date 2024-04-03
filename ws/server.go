package ws

import (
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
		// When the function returns, unregister the client and close the connection
		defer func() {
			c.Close()
		}()

		userId := c.Locals("userId").(string)

		conn := &connection{*c, userId}
		broker.AddConnByPersonID(conn, conn.getUserId())

		defer broker.RemoveConn(conn)

		for {
			messageType, message, err := c.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					slog.Warn("websocket read error:", err)
				}
				return // Calls the deferred function, i.e. closes the connection on error
			}

			if messageType == websocket.TextMessage {
				fmt.Println(string(message))
			}
		}
	}, wsConf))
}
