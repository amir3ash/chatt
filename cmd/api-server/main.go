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
	"time"

	"github.com/gofiber/fiber/v2/middleware/healthcheck"
	"github.com/segmentio/kafka-go"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.opentelemetry.io/contrib/instrumentation/go.mongodb.org/mongo-driver/mongo/otelmongo"
)

func getMessageRepository(conf *config.Confing) messages.Repository {
	type messageRepoType int

	const (
		mongoRepoT messageRepoType = iota
		kafkaRepoT
	)
	const repoType = kafkaRepoT

	var mongoCli *mongo.Client

	if repoType == mongoRepoT || repoType == kafkaRepoT {
		mongoOptions := &options.ClientOptions{}
		mongoOptions.Monitor = otelmongo.NewMonitor()
		mongoOptions.ApplyURI(fmt.Sprintf("mongodb://%s:%s@%s:%d", conf.MongoUser, conf.MongoPass, conf.MongoHost, conf.MongoPort))

		var err error
		mongoCli, err = mongo.Connect(context.TODO(), mongoOptions)
		if err != nil {
			panic(fmt.Errorf("can't create mongodb client: %w", err))
		}
	}

	switch repoType {
	case mongoRepoT:
		mongoRepo, err := repo.NewMongoRepo(mongoCli)
		if err != nil {
			panic(fmt.Errorf("can't create mongodb repo: %w", err))
		}

		return mongoRepo

	case kafkaRepoT:
		kafkaWriter := &kafka.Writer{
			Addr:                   kafka.TCP(conf.KafkaHost),
			Topic:                  "chat-messages",
			AllowAutoTopicCreation: true,
			// Balancer:               &kafka.LeastBytes{},
			RequiredAcks: kafka.RequireOne,
			BatchTimeout: 50 * time.Millisecond,
			Compression:  kafka.Snappy,

			// Transport: kafka.DefaultTransport,
		}

		db := mongoCli.Database("chatting2")
		repo := kafkarep.NewKafkaRepo(kafkaWriter, db)
		return repo
	}

	return nil
}

func main() {
	conf, err := config.New()
	if err != nil {
		panic(err)
	}

	otelShutdown, err := observe.SetupOTelSDK(context.TODO())
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
