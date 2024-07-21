package kafkarep

import (
	"chat-system/core/repo"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/kamva/mgm/v3"
	"github.com/segmentio/kafka-go"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type mongoAggr struct {
	mgm.IDField `bson:",inline"`
	Topic       string             `bson:"topicID"`
	MinId       primitive.ObjectID `bson:"minID"`
	MaxId       primitive.ObjectID `bson:"maxID"`
	Len         int                `bson:"size"`
	Messages    []repo.Message     `bson:"messages"`
}

type MongoTransactionErr struct {
	error
}

type MongoConnect struct {
	reader       *kafka.Reader
	mongoCli     *mongo.Client
	coll         mgm.Collection
	ctx          context.Context
	cancel       context.CancelFunc
	msgMap       map[string][]repo.Message // topicId -> messages
	lastKafkaMsg kafka.Message
	mu           sync.Mutex
}

func NewMongoConnect(ctx context.Context, mongoCli *mongo.Client, kafkaReader *kafka.Reader) *MongoConnect {
	db := mongoCli.Database("chatting2")

	coll := mgm.NewCollection(db, mgm.CollName(&mongoAggr{}))
	ctx, cancel := context.WithCancel(ctx)
	c := MongoConnect{
		reader:   kafkaReader,
		mongoCli: mongoCli,
		coll:     *coll,
		ctx:      ctx,
		cancel:   cancel,
		msgMap:   make(map[string][]repo.Message),
		mu:       sync.Mutex{},
	}

	go c.run()
	go c.readKafka()

	return &c
}

func (c *MongoConnect) readKafka() {
	for {
		kafkaMsg, err := c.reader.FetchMessage(context.TODO())
		if err != nil {
			if err == io.EOF {
				slog.Info("kafka fetch EOF: reader closed")
				return
			}
			slog.Error("get error while reading meassageas", "err", err)
			break
		}

		msg := repo.Message{}
		err = json.Unmarshal(kafkaMsg.Value, &msg)
		if err != nil {
			slog.Error("can not unmarshal kafka message", "msg", kafkaMsg.Value, "err", err)
			continue
		}

		c.appendMessage(msg, kafkaMsg)
	}

}

func (c *MongoConnect) appendMessage(msg repo.Message, kakaMsg kafka.Message) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.msgMap[msg.TopicID] = append(c.msgMap[msg.TopicID], msg)
	c.lastKafkaMsg = kakaMsg
}

func (c *MongoConnect) run() {
	ticker := time.NewTicker(150 * time.Millisecond)
	var err error
	for _ = range ticker.C {
		for i := 0; i < 3; i++ {

			select {
			case <-c.ctx.Done():
			default:
				err = c.doTransaction(c.ctx)
				if err != nil {
					slog.Error("transaction failed", "retry", i, "err", err)
				}
			}
			if err == nil {
				break
			}
		}

		if err != nil {
			break
		}
	}

}

func (c *MongoConnect) doTransaction(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.msgMap) == 0 {
		return nil
	}

	err := c.mongoCli.UseSession(ctx, func(sc mongo.SessionContext) (err error) {
		var merged bool
		for topic, msgList := range c.msgMap {
			agrr := mongoAggr{
				Topic:    topic,
				MinId:    msgList[0].ID,
				MaxId:    msgList[len(msgList)-1].ID,
				Len:      len(msgList),
				Messages: msgList,
			}

			merged, err = c.mergeToLastMessage(sc, &agrr)
			if err != nil {
				break
			}

			if merged {
				_, err = c.coll.ReplaceOne(sc, bson.M{"_id": agrr.ID}, agrr)
			} else {
				_, err = c.coll.InsertOne(sc, agrr)
			}
			if err != nil {
				break
			}
		}

		if err != nil {
			slog.Warn("mongo command failed", "err", err)
			return MongoTransactionErr{fmt.Errorf("mongo command failed: %w", err)}
		}

		err = c.reader.CommitMessages(ctx, c.lastKafkaMsg)
		if err != nil {
			slog.Warn("kafka messages commit failed", "err", err)
			return MongoTransactionErr{fmt.Errorf("kafka messages commit failed: %w", err)}
		}

		return
	})

	if err != nil {
		if _, ok := err.(MongoTransactionErr); !ok {
			slog.Error("posiblity of data loss: transaction failed but commited kafka message", "lastKafkaMsg", c.lastKafkaMsg, "err", err)
		}
	} else {
		clear(c.msgMap)
	}
	return err
}

// find last item, if len was low append appends old messages to input *agrr*
func (c *MongoConnect) mergeToLastMessage(ctx context.Context, agrr *mongoAggr) (merged bool, err error) {
	getAgrr := mongoAggr{}
	err = c.coll.FirstWithCtx(ctx, bson.M{"topicID": agrr.Topic}, &getAgrr, &options.FindOneOptions{Sort: bson.M{"id": 1}})
	if err != nil {
		if err != mongo.ErrNoDocuments {
			return false, err
		}
	}

	if getAgrr.Len >= 20 || getAgrr.Len == 0 {
		return false, nil
	}

	agrr.Messages = append(getAgrr.Messages, agrr.Messages...)
	agrr.Len = len(agrr.Messages)
	agrr.MinId = getAgrr.MinId
	agrr.ID = getAgrr.ID

	return true, nil
}

func (c *MongoConnect) Close() {
	c.cancel()
}