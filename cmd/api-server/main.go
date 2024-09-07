package main

import (
	"chat-system/authz"
	"chat-system/config"
	"chat-system/core/api"
	"chat-system/core/messages"
	"chat-system/core/repo"
	kafkarep "chat-system/core/repo/kafkaRep"
	"chat-system/pkg/observe"
	"context"
	"fmt"

	"github.com/gofiber/fiber/v2/middleware/healthcheck"
	"go.mongodb.org/mongo-driver/mongo"
)

type Config struct {
	MongoDB      *repo.MongoConf
	KafkaWriter  *kafkarep.WriterConf
	SpiceDbUrl   string `env:"AUTHZED_URL"`
	SpiceDBToken string `env:"AUTHZED_TOKEN"`
}

func getMessageRepository(conf *Config) messages.Repository {
	type messageRepoType int

	const (
		mongoRepoT messageRepoType = iota
		kafkaRepoT
	)
	const repoType = kafkaRepoT

	var mongoCli *mongo.Client

	if repoType == mongoRepoT || repoType == kafkaRepoT {
		mongoCli = repo.NewInsecureMongoCli(conf.MongoDB)
	}

	switch repoType {
	case mongoRepoT:
		mongoRepo, err := repo.NewMongoRepo(mongoCli)
		if err != nil {
			panic(fmt.Errorf("can't create mongodb repo: %w", err))
		}

		return mongoRepo

	case kafkaRepoT:
		kafkaWriter := kafkarep.NewInsecureWriter(conf.KafkaWriter)

		db := mongoCli.Database("chatting2")
		repo := kafkarep.NewKafkaRepo(kafkaWriter, db)
		return repo
	}

	return nil
}

func main() {
	conf := &Config{}
	if err := config.Parse(conf); err != nil {
		panic(err)
	}

	observeOpts := observe.Options().
		WithService("api-server", "chatting").
		EnableTraceProvider()

	otelShutdown, err := observe.SetupOTelSDK(context.TODO(), observeOpts)
	if err != nil {
		panic(fmt.Errorf("can't setup opentelementry: %w", err))
	}
	defer otelShutdown(context.Background())

	authzed, err := authz.NewInsecureAuthZedCli(authz.Conf{BearerToken: conf.SpiceDBToken, ApiUrl: conf.SpiceDbUrl})
	if err != nil {
		panic(fmt.Errorf("can't create authzed client: %w", err))
	}

	authoriz := authz.NewAuthoriz(authzed)

	messageRepo := getMessageRepository(conf)

	fiberApp, err := api.Initialize(messages.NewService(messageRepo, authoriz))
	if err != nil {
		panic(err)
	}

	fiberApp.Use(healthcheck.New())

	fiberApp.Listen(":8888")

}
