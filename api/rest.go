package api

import (
	"chat-system/core"
	"fmt"
	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humafiber"
	"github.com/gofiber/fiber/v2"
	"strconv"
)

var Client huma.CLI

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
		Next     string    `json:"next"`
		Prev     string    `json:"prev"`
	}
}

func init() {
	app := fiber.New()
	api := humafiber.New(app, huma.DefaultConfig("Chat API", "0.0.0-pre"))
	service := core.NewService(nil)
	handler := Handler{service}

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

	Client = huma.NewCLI(func(hooks huma.Hooks, options *Options) {
		hooks.OnStart(func() {
			fmt.Printf("Starting server on port %d...\n", options.Port)
			app.Listen(":" + strconv.Itoa(options.Port))
		})
	})

}
