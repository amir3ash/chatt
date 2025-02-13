package main

import (
	"chat-system/config"
	"chat-system/core/repo"
	kafkarep "chat-system/core/repo/kafkaRep"
	"chat-system/pkg/observe"
	"context"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

type Config struct {
	KafkaReader *kafkarep.ReaderConf
	MongoDB     *repo.MongoConf
}

func main() {
	log.SetFlags(log.Lmicroseconds)

	conf := &Config{}
	if err := config.Parse(conf); err != nil {
		panic(err)
	}

	slog.Info("Started")

	observeOpts := observe.Options().
		WithService("chat-mongo-kafka-connect", "chatting.streams").
		EnableTraceProvider().
		EnableLoggerProvider()
	otelShutdown, err := observe.SetupOTelSDK(context.TODO(), observeOpts)
	if err != nil {
		panic(fmt.Errorf("can't setup opentelementry: %w", err))
	}
	defer otelShutdown(context.Background())

	mongoCli := repo.NewInsecureMongoCli(conf.MongoDB)
	defer mongoCli.Disconnect(context.Background())

	kafkaReader := kafkarep.NewInsecureReader(conf.KafkaReader)
	defer kafkaReader.Close()

	mongoDB := mongoCli.Database("chatting2")
	mongoKConnect := kafkarep.NewMongoConnect(context.Background(), mongoDB, kafkaReader)
	defer mongoKConnect.Close()

	s := make(chan os.Signal, 1)
	defer close(s)
	signal.Notify(s, syscall.SIGTERM, syscall.SIGINT)

	<-s
}
