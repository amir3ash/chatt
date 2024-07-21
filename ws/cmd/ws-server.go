package main

import (
	"chat-system/authz"
	"chat-system/config"
	"chat-system/core/repo"
	kafkarep "chat-system/core/repo/kafkaRep"
	"chat-system/ws"
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/segmentio/kafka-go"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func getMessageWatcher(conf *config.Confing) (ws.MessageWatcher, error) {
	const wType = "kafka"

	switch wType {
	case "mongo":
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
		return mongoRepo, nil

	case "kafka":
		kafkaReader := kafka.NewReader(kafka.ReaderConfig{
			Brokers:  []string{conf.KafkaHost},
			Topic:    "chat-messages",
			MaxBytes: 2e6, // 2MB
			GroupID:  "chat-messages-watcher",
		})
		watcher := kafkarep.NewMessageWatcher(kafkaReader)

		return watcher, nil
	}

	return nil, fmt.Errorf("watcher type %s not found", wType)
}

func prepare(conf *config.Confing) error {
	broker := ws.NewWSServer()
	ws.RunServer(broker)

	authzed, err := authz.NewInsecureAuthZedCli(authz.Conf{BearerToken: conf.SpiceDBToken, ApiUrl: conf.SpiceDbUrl})
	if err != nil {
		return fmt.Errorf("can't create authzed client: %w", err)
	}

	authoriz := authz.NewAuthoriz(authzed)

	roomServer := ws.NewRoomServer(broker, ws.NewWSAuthorizer(authoriz))

	msgWatcher ,err := getMessageWatcher(conf)
	if err != nil {
		return err
	}

	ws.ReadChangeStream(msgWatcher, roomServer)

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
