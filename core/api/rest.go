package api

import (
	"chat-system/authz"
	"chat-system/core/messages"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humafiber"
	"github.com/gofiber/contrib/otelfiber"
	"github.com/gofiber/fiber/v2"
	"go.opentelemetry.io/otel"
	// "github.com/felixge/fgprof"
	// "github.com/gofiber/fiber/v2/middleware/adaptor"
	// "github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/pprof"
	fiberRecover "github.com/gofiber/fiber/v2/middleware/recover"
	// "github.com/maruel/panicparse/v2/stack/webstack"
)

type sendMessageInput struct {
	TopicID string `path:"TopicID" maxLength:"30" example:"456" required:"true"`
	Body    struct {
		Message string `json:"message" minLength:"1" maxLength:"300" required:"true"`
	}
}

type getMessagesInput struct {
	TopicID  string `path:"TopicID" maxLength:"30" example:"456" required:"true"`
	Limit    int    `query:"limit" minimum:"1" maximum:"50" default:"20"`
	BeforeID string `query:"before_id" maxLength:"30"`
	AfterID  string `query:"after_id" maxLength:"30"`
}

type getMessagesOutput struct {
	Body struct {
		Messages []messages.Message `json:"messages"`
		Next     string             `json:"next"`
		Prev     string             `json:"prev"`
	}
}

type ResBody[T any] struct {
	Body T
}

func setFiberMiddleWares(app *fiber.App) {
	// app.Use(requestid.New())
	// app.Use(logger.New())
	app.Use(pprof.New())
	app.Use(authz.NewFiberAuthMiddleware())
	app.Use(fiberRecover.New())
	// app.Get("/debug/fgprof", adaptor.HTTPHandler(fgprof.Handler()))
	// app.Get("go", adaptor.HTTPHandlerFunc(webstack.SnapshotHandler))
	app.Use(otelfiber.Middleware(otelfiber.WithTracerProvider(otel.GetTracerProvider())))
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

func Initialize(messageSVC MessageService) (*fiber.App, error) {
	app := fiber.New()

	setFiberMiddleWares(app)
	// otel.ServeFiberPromMetrics("/metrics", app)

	api := humafiber.New(app, huma.DefaultConfig("Chat API", "0.0.0-alpha-0"))

	handler := Handler{
		messageSVC,
		"http://127.0.0.1:8888",
	}

	registerEndpoints(api, handler)

	return app, nil
}
