package main

import (
	"chat-system/authz"
	"chat-system/config"
	"chat-system/core/repo"
	kafkarep "chat-system/core/repo/kafkaRep"
	"chat-system/pkg/observe"
	"chat-system/ws"
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/segmentio/kafka-go"
)

type Config struct {
	MongoDB      *repo.MongoConf
	KafkaReader  *kafkarep.ReaderConf
	SpiceDbUrl   string `env:"AUTHZED_URL"`
	SpiceDBToken string `env:"AUTHZED_TOKEN"`
}

func getMessageWatcher(conf *Config) (ws.MessageWatcher, error) {
	const wType = "kafka"

	switch wType {
	case "mongo":
		mongoCli := repo.NewInsecureMongoCli(conf.MongoDB)

		mongoRepo, err := repo.NewMongoRepo(mongoCli)
		if err != nil {
			return nil, fmt.Errorf("can't create mongodb repo: %w", err)
		}
		return mongoRepo, nil

	case "kafka":
		kafkaReader := kafka.NewReader(kafka.ReaderConfig{
			Brokers:  []string{conf.KafkaReader.KafkaHost},
			Topic:    conf.KafkaReader.Topic,
			MaxBytes: conf.KafkaReader.MaxBytes,
			MaxWait:  conf.KafkaReader.MaxWait,
			GroupID:  "chat-messages-watcher",
		})
		watcher := kafkarep.NewMessageWatcher(kafkaReader)

		return watcher, nil
	}

	return nil, fmt.Errorf("watcher type %s not found", wType)
}

func prepare(conf *Config) error {
	authzed, err := authz.NewInsecureAuthZedCli(authz.Conf{BearerToken: conf.SpiceDBToken, ApiUrl: conf.SpiceDbUrl})
	if err != nil {
		return fmt.Errorf("can't create authzed client: %w", err)
	}

	authoriz := authz.NewAuthoriz(authzed)
	msgWatcher, err := getMessageWatcher(conf)
	if err != nil {
		return err
	}

	ws.Run(msgWatcher, authoriz)
	return nil
}

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	observeOpts := observe.Options().
		WithService("ws-server", "chatting").
		EnableTraceProvider().
		EnableLoggerProvider()

	otelShutdown, err := observe.SetupOTelSDK(context.TODO(), observeOpts)
	if err != nil {
		panic(fmt.Errorf("can't setup opentelementry: %w", err))
	}
	defer otelShutdown(context.Background())

	conf := &Config{}
	if err := config.Parse(conf); err != nil {
		panic(err)
	}

	err = prepare(conf)
	if err != nil {
		panic(err)
	}
}
