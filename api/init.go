package api

import (
	"chat-system/authz"
	"chat-system/config"
	"chat-system/core"
	"chat-system/repo"
	"chat-system/ws"
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
)

func Initialize(conf *config.Confing) (huma.CLI, error) {
	app := fiber.New()

	setFiberMiddleWares(app)
	servePromMetrics("/metrics", app)

	api := humafiber.New(app, huma.DefaultConfig("Chat API", "0.0.0-alpha-1"))

	otelShutdown, err := setupOTelSDK(context.TODO())
	if err != nil {
		return nil, fmt.Errorf("can't setup opentelementry: %w", err)
	}
	defer otelShutdown(context.Background())

	mongoOptions := &options.ClientOptions{}
	mongoOptions.ApplyURI(fmt.Sprintf("mongodb://%s:%s@%s:%d", conf.MongoUser, conf.MongoPass, conf.MongoHost, conf.MongoPort))
	mongoCli, err := mongo.Connect(context.TODO(), mongoOptions)
	if err != nil {
		return nil, fmt.Errorf("can't create mongodb client: %w", err)
	}

	mongoRepo, err := repo.NewMongoRepo(mongoCli)
	if err != nil {
		return nil, fmt.Errorf("can't create mongodb repo: %w", err)
	}

	broker := ws.NewWSServer()
	ws.RunServer(app, broker)

	authzed, err := authz.NewInsecureAuthZedCli(authz.Conf{BearerToken: conf.SpiceDBSecret, ApiUrl: conf.SpiceDbUrl})
	if err != nil {
		return nil, fmt.Errorf("can't create authzed client: %w", err)
	}

	authoriz := authz.NewAuthoriz(authzed)

	roomServer := ws.NewRoomServer(broker, ws.NewWSAuthorizer(authoriz))
	service := core.NewService(mongoRepo, roomServer, *authoriz)
	handler := Handler{
		service,
		"http://127.0.0.1:8888",
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

	return client, nil
}
