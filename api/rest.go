package api

import (
	"chat-system/config"
	"chat-system/core"
	"chat-system/repo"
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humafiber"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/healthcheck"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/monitor"
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

func Initialize(conf *config.Confing) huma.CLI {
	app := fiber.New()
	setFiberMiddleWares(app)

	api := humafiber.New(app, huma.DefaultConfig("Chat API", "0.0.0-alpha-1"))

	mongoOptions := &options.ClientOptions{}
	mongoOptions.ApplyURI(fmt.Sprintf("mongodb://%s:%s@%s:%d", conf.MongoUser, conf.MongoPass, conf.MongoHost, conf.MongoPort))
	mongoCli, err := mongo.Connect(context.TODO(), mongoOptions)
	if err != nil {
		panic(err)
	}

	mongoRepo, _ := repo.NewMongoRepo(mongoCli)
	service := core.NewService(mongoRepo)
	handler := Handler{
		service,
		"http://127.0.01:8888",
	}

	registerEndpoints(api, handler)

	app.Use(healthcheck.New(healthcheck.Config{
		ReadinessProbe: func(c *fiber.Ctx) bool {
			ctx, cancel := context.WithTimeout(context.Background(), 700*time.Millisecond)
			defer cancel()
			err := mongoCli.Ping(ctx, nil)
			return err == nil
		},
	}))

	client := huma.NewCLI(func(hooks huma.Hooks, options *Options) {
		hooks.OnStart(func() {
			fmt.Printf("Starting server on port %d...\n", options.Port)
			app.Listen(":" + strconv.Itoa(options.Port))
		})
	})

	return client
}

func setFiberMiddleWares(app *fiber.App) {

	app.Use(requestid.New())
	app.Use(logger.New())
	app.Use(fiberRecover.New())
	app.Get("/metrics", monitor.New())

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
