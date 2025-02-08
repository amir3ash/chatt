package repo

import (
	"chat-system/core/messages"
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/mongodb"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func startMongo(t *testing.T, ctx context.Context) (cli *mongo.Client) {
	t.Helper()

	mongodbContainer, err := mongodb.Run(ctx, "mongo:noble")
	t.Cleanup(func() {
		if err := testcontainers.TerminateContainer(mongodbContainer); err != nil {
			t.Errorf("failed to terminate container: %s", err)
		}
	})

	if err != nil {
		t.Fatalf("failed to start container: %s", err)
	}

	endpoint, err := mongodbContainer.ConnectionString(ctx)
	if err != nil {
		t.Fatalf("failed to get connection string: %s", err)
	}
	mongoCli, err := mongo.Connect(ctx, options.Client().ApplyURI(endpoint))
	if err != nil {
		t.Fatalf("failed to connect to MongoDB: %s", err)
	}

	return mongoCli
}

func TestRepo_SendMsgToTopic(t *testing.T) {
	if testing.Short() {
		t.Skip("mongo test container skipped")
	}

	ctx := context.Background()
	mongoCli := startMongo(t, ctx)

	repo, err := NewMongoRepo(mongoCli)
	if err != nil {
		t.Fatal(err)
	}

	sentMsg, err := repo.SendMsgToTopic(ctx, messages.Sender{ID: "test-user"}, "test-topic", "test-text")
	if err != nil {
		t.Errorf("can not send message: %v", err)
	}

	_, err = repo.SendMsgToTopic(ctx, messages.Sender{ID: "test-user"}, "other-topic", "test-text")
	if err != nil {
		t.Errorf("can not send message: %v", err)
	}

	messages, err := repo.ListMessages(ctx, "test-topic", messages.Pagination{
		Limit: 10,
	})
	if err != nil {
		t.Errorf("can not list messages %v", err)
	}

	if len(messages) != 1 {
		t.Fatalf("messages len is %d, expected 1", len(messages))
	}

	msg := messages[0]

	if !cmp.Equal(msg, sentMsg) {
		t.Errorf("messages are not equal, diff: %s", cmp.Diff(msg, sentMsg))
	}
}

func TestRepo_SendMsgToTopic_afterAggr(t *testing.T) {
	if testing.Short() {
		t.Skip("mongo test container skipped")
	}

	ctx := context.Background()
	mongoCli := startMongo(t, ctx)

	repo, err := NewMongoRepo(mongoCli)
	if err != nil {
		t.Fatal(err)
	}

	msgsSent := make([]messages.Message, 0, 110)
	for range 100 {
		m, err := repo.SendMsgToTopic(ctx, messages.Sender{ID: "test-user"}, "test-topic", "test-text")
		if err != nil {
			t.Errorf("can not send message: %v", err)
		}
		msgsSent = append(msgsSent, m)
	}

	// send to another topic
	_, err = repo.SendMsgToTopic(ctx, messages.Sender{ID: "test-user"}, "other-topic", "test-text")
	if err != nil {
		t.Errorf("can not send message: %v", err)
	}

	_, err = repo.writeToBucket(ctx)
	if err != nil {
		t.Errorf("can not write to bucket: %v", err)
	}

	// send after aggregate (bucket pattern)
	for range 10 {
		m, err := repo.SendMsgToTopic(ctx, messages.Sender{ID: "test-user"}, "test-topic", "test-text")
		if err != nil {
			t.Errorf("can not send message: %v", err)
		}
		msgsSent = append(msgsSent, m)
	}

	messages, err := repo.ListMessages(ctx, "test-topic", messages.Pagination{
		Limit: 150,
	})
	if err != nil {
		t.Errorf("can not list messages %v", err)
	}

	if len(messages) != 100+10 {
		t.Fatalf("messages len is %d, expected 110", len(messages))
	}

	if !cmp.Equal(msgsSent, messages) {
		t.Errorf("messages not equal: %s", cmp.Diff(msgsSent, messages))
	}
}
