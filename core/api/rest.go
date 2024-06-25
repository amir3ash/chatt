package api

import (
	"chat-system/authz"
	"chat-system/core/messages"
	"context"

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
	app.Use(fiberRecover.New())
	app.Use(authz.NewFiberAuthMiddleware())
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

	api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
		next(fiberHumaCtx{ctx}) // to use fiber's Ctx.Context() and Ctx.UserContext()
	})

	handler := Handler{
		messageSVC,
		"http://127.0.0.1:8888",
	}

	registerEndpoints(api, handler)

	return app, nil
}

type humaCtx = huma.Context
type fiberHumaCtx struct {
	humaCtx
}

func (c fiberHumaCtx) Context() context.Context {
	return ctx{c.humaCtx.Context()}
}

// Overrides Value function to merge values from fiber's UserContext() and context.Context
type ctx struct {
	context.Context
}

func (c ctx) Value(key any) any {
	// userContextKey define the key name for storing context.Context in *fasthttp.RequestCtx
	const userContextKey = "__local_user_context__"

	v := c.Context.Value(key)
	if v != nil {
		return v
	}

	fiberUserCtx, ok := c.Context.Value(userContextKey).(context.Context)
	if ok {
		return fiberUserCtx.Value(key)
	}

	return nil
}
