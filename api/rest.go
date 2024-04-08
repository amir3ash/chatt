package api

import (
	"chat-system/authz"
	"chat-system/core"

	"github.com/danielgtaylor/huma/v2"
	"github.com/gofiber/fiber/v2"

	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/pprof"
	fiberRecover "github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/fiber/v2/middleware/requestid"
)

// Options for the CLI.
type Options struct {
	Port int `help:"Port to listen on" short:"p" default:"8888"`
}

// -----

type sendMessageInput struct {
	TopicID string `path:"TopicID" maxLength:"30" example:"456" required:"true"`
	Body    struct {
		Message string `json:"message" maxLength:"300" required:"true"`
	}
}

type getMessagesInput struct {
	TopicID  string `path:"TopicID" maxLength:"30" example:"456" required:"true"`
	Limit    int    `query:"limit" max:"50" default:"20"`
	BeforeID string `query:"before_id" maxLength:"30"`
	AfterID  string `query:"after_id" maxLength:"30"`
}

type getMessagesOutput struct {
	Body struct {
		Messages []core.Message `json:"messages"`
		Next     string         `json:"next"`
		Prev     string         `json:"prev"`
	}
}

type ResBody[T any] struct {
	Body T
}

func setFiberMiddleWares(app *fiber.App) {
	app.Use(requestid.New())
	app.Use(logger.New())
	app.Use(pprof.New())
	app.Use(authz.NewAuthMiddleware())
	app.Use(fiberRecover.New())
}

func registerEndpoints(api huma.API, handler Handler) {
	huma.Register(api, huma.Operation{
		OperationID: "list-messages",
		Method:      "GET",
		Path:        "/topics/{TopicID}/messages",
	}, handler.listMessages)

	huma.Register(api, huma.Operation{
		OperationID:   "send-message",
		Summary:       "Sending new message",
		Method:        "POST",
		Path:          "/topics/{TopicID}/messages",
		DefaultStatus: 201,
	}, handler.sendMessage)
}
