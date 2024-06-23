package main

import (
	"chat-system/authz"
	"chat-system/config"
	"chat-system/core/repo"
	"chat-system/ws"
	"context"
	"fmt"
	"log/slog"
	"os"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func prepare(conf *config.Confing) error {
	broker := ws.NewWSServer()
	ws.RunServer(broker)

	mongoOptions := &options.ClientOptions{}
	mongoOptions.ApplyURI(fmt.Sprintf("mongodb://%s:%s@%s:%d", conf.MongoUser, conf.MongoPass, conf.MongoHost, conf.MongoPort))
	mongoCli, err := mongo.Connect(context.TODO(), mongoOptions)
	if err != nil {
		return fmt.Errorf("can't create mongodb client: %w", err)
	}

	mongoRepo, err := repo.NewMongoRepo(mongoCli)
	if err != nil {
		return fmt.Errorf("can't create mongodb repo: %w", err)
	}

	authzed, err := authz.NewInsecureAuthZedCli(authz.Conf{BearerToken: conf.SpiceDBToken, ApiUrl: conf.SpiceDbUrl})
	if err != nil {
		return fmt.Errorf("can't create authzed client: %w", err)
	}

	authoriz := authz.NewAuthoriz(authzed)

	roomServer := ws.NewRoomServer(broker, ws.NewWSAuthorizer(authoriz))

	ws.ReadChangeStream(mongoRepo, roomServer)

	return nil
}

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	conf, err := config.New()
	if err != nil {
		panic(err)
	}

	err = prepare(conf)
	if err != nil {
		panic(err)
	}
}
