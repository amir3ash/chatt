package main

import (
	"chat-system/authz"
	"chat-system/config"
	"chat-system/core/api"
	"chat-system/core/messages"
	"chat-system/core/repo"
	"chat-system/ws"
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/healthcheck"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	conf, err := config.New()
	if err != nil {
		panic(err)
	}

	app, err := prepare(conf)
	if err != nil {
		panic(err)
	}

	err = app.Listen(":8888")
	panic(err)
}

func prepare(conf *config.Confing) (*fiber.App, error) {

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

	authzed, err := authz.NewInsecureAuthZedCli(authz.Conf{BearerToken: conf.SpiceDBSecret, ApiUrl: conf.SpiceDbUrl})
	if err != nil {
		return nil, fmt.Errorf("can't create authzed client: %w", err)
	}

	authoriz := authz.NewAuthoriz(authzed)

	roomServer := ws.NewRoomServer(broker, ws.NewWSAuthorizer(authoriz))
	msgSvc := messages.NewService(mongoRepo, roomServer, authoriz)

	fiberApp, err := api.Initialize(msgSvc)
	if err != nil {
		return nil, err
	}
	
	fiberApp.Use(healthcheck.New(healthcheck.Config{
		ReadinessProbe: func(c *fiber.Ctx) bool {
			ctx, cancel := context.WithTimeout(context.Background(), 700*time.Millisecond)
			defer cancel()
			err := mongoCli.Ping(ctx, nil)
			return err == nil
		},
	}))

	return fiberApp, nil
}
