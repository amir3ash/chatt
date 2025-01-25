package repo

import (
	"chat-system/core/messages"
	"context"
	"encoding/binary"
	"fmt"
	"log/slog"
	"time"

	"github.com/kamva/mgm/v3"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.opentelemetry.io/contrib/instrumentation/go.mongodb.org/mongo-driver/mongo/otelmongo"
)

type MongoConf struct {
	MongoHost string `env:"MONGO_HOST" required:"true"`
	MongoUser string `env:"MONGO_USER" required:"true"`
	MongoPass string `env:"MONGO_PASSWORD"`
	MongoPort int    `env:"MONGO_PORT"`
}

func NewInsecureMongoCli(conf *MongoConf) *mongo.Client {
	if conf.MongoPort == 0 {
		conf.MongoPort = 27017
	}
	mongoOptions := &options.ClientOptions{}
	mongoOptions.Monitor = otelmongo.NewMonitor()
	mongoOptions.ApplyURI(fmt.Sprintf("mongodb://%s:%s@%s:%d", conf.MongoUser, conf.MongoPass, conf.MongoHost, conf.MongoPort))

	mongoCli, err := mongo.Connect(context.TODO(), mongoOptions)
	if err != nil {
		panic(fmt.Errorf("can't create mongodb client: %w", err))
	}

	return mongoCli
}

type Message struct {
	// DefaultModel adds _id, created_at and updated_at fields to the Model.
	mgm.DefaultModel `bson:",inline"`
	TopicID          string              `bson:"topicID" json:"topicId"`
	Version          uint                `bson:"v" json:"v"`
	SenderId         string              `bson:"senderID" json:"senderId"`
	Timestamp        primitive.Timestamp `bson:"ts" json:"-"`
	Text             string              `bson:"text" json:"text"`
	Deleted          bool                `bson:"deleted" json:"deleted"`
}

func (m *Message) ToApiMessage() *messages.Message {
	return &messages.Message{
		SenderId: m.SenderId,
		ID:       m.ID.Hex(),
		Version:  m.Version,
		TopicID:  m.TopicID,
		SentAt:   m.CreatedAt,
		Text:     m.Text,
	}
}

type pagination struct {
	Limit    *int64
	AfterId  primitive.ObjectID
	BeforeID primitive.ObjectID
}

func NewMPaginatin(c messages.Pagination) (p pagination) {
	p.BeforeID, _ = primitive.ObjectIDFromHex("ffffffffffffffffffffffff")
	p.AfterId = [12]byte{}

	if c.Limit > 0 {
		l := int64(c.Limit)
		p.Limit = &l
	}

	if id, err := primitive.ObjectIDFromHex(c.AfterID); err == nil {
		p.AfterId = id
	}

	if id, err := primitive.ObjectIDFromHex(c.BeforeID); err == nil {
		p.BeforeID = id
	}

	return
}

type Repo struct {
	messages *mgm.Collection
	db       *mongo.Database
}

func NewMongoRepo(cli *mongo.Client) (*Repo, error) {

	db := cli.Database("chatting")
	repo := &Repo{
		messages: mgm.NewCollection(db, mgm.CollName(&Message{})),
		db:       db,
	}
	err := db.CreateCollection(context.Background(), "hist")
	if err != nil {
		slog.Error("cant create collection \"hist\"", "err", err)
		return nil, err
	}

	unique := true
	_, err = db.Collection("hist").Indexes().CreateOne(context.Background(),
		mongo.IndexModel{
			Keys:    bson.D{{Key: "topicID", Value: 1}, {Key: "min", Value: 1}},
			Options: &options.IndexOptions{Unique: &unique},
		})
	if err != nil {
		slog.Warn("cant create index for collection \"hist\"", "err", err)
	}

	go func() {
		for range time.Tick(2 * time.Minute) {
			slog.Debug("agregating messages into hist")
			if _, err := repo.writeToBucket(context.Background()); err != nil {
				slog.Error("can't aggregate messages", "err", err)
				fmt.Println("hello from aggrgeage")
			}
		}
	}()

	return repo, nil
}

func (r Repo) ListMessages(ctx context.Context, topicID string, pg messages.Pagination) ([]messages.Message, error) {
	return r.readFromBucket(ctx, topicID, pg)
}

func (r Repo) SendMsgToTopic(ctx context.Context, sender messages.Sender, topicID string, message string) (messages.Message, error) {
	msg := &Message{
		SenderId: sender.ID,
		Text:     message,
		TopicID:  topicID,
		Version:  1,
	}

	err := r.messages.CreateWithCtx(ctx, msg)
	if err != nil {
		return messages.Message{}, err
	}

	return messages.Message{
		SenderId: msg.SenderId,
		ID:       msg.ID.Hex(),
		TopicID:  topicID,
		SentAt:   msg.CreatedAt,
		Text:     msg.Text,
	}, err
}

func (r Repo) createTopic(ctx context.Context, topicID string) error {
	topic := bson.M{"_id": topicID}

	upsert := true
	_, err := r.db.Collection("topicIds").UpdateByID(ctx, topicID, topic,
		&options.UpdateOptions{Upsert: &upsert})
	if err != nil {
		return err
	}
	return nil
}

func (r Repo) topicExist(ctx context.Context, topicID string) (bool, error) {
	returnKey := true
	err := r.db.Collection("topicIds").FindOne(ctx, bson.M{"_id": topicID},
		&options.FindOneOptions{ReturnKey: &returnKey}).Err()

	if err == mongo.ErrNoDocuments {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (r Repo) writeToBucket(ctx context.Context) (bool, error) {
	maxBucketCount := 20
	timeRange := time.Now().Add(-1 * time.Minute)
	filterMsgs := bson.M{
		"_id": bson.M{
			"$lt": tsOnlyObjectID(timeRange),
		},
	}

	cur, err := r.messages.Aggregate(ctx, bson.A{
		bson.M{
			"$match": filterMsgs,
		},
		bson.M{"$sort": bson.M{"ts": 1}},
		bson.M{
			"$group": bson.M{
				"_id": "$topicID",
				"msg": bson.M{
					"$push": "$$ROOT",
				},
			},
		},
		bson.M{
			"$addFields": bson.M{
				"msg": bson.M{
					"$map": bson.M{
						"input": bson.M{
							"$range": bson.A{
								0,
								bson.M{
									"$size": "$msg",
								},
								maxBucketCount,
							},
						},
						"as": "index",
						"in": bson.M{
							"$slice": bson.A{
								"$msg",
								"$$index",
								maxBucketCount,
							},
						},
					},
				},
			},
		},
		bson.M{
			"$unwind": "$msg",
		},
		bson.M{
			"$addFields": bson.M{
				"topicID": "$_id",
				"count":   bson.M{"$size": "$msg"},
				"max":     bson.M{"$max": "$msg._id"},
				"min":     bson.M{"$min": "$msg._id"},
			},
		},
		bson.M{
			"$project": bson.M{
				"_id":     0,
				"topicID": 1,
				"count":   1,
				"min":     1,
				"max":     1,
				"msg":     1,
			},
		},
		bson.M{
			"$merge": bson.M{
				"into":           "hist",
				"on":             bson.A{"topicID", "min"},
				"whenMatched":    "merge",
				"whenNotMatched": "insert",
			},
		},
	})

	if err != nil {
		return false, err
	}

	cur.Close(context.Background())

	_, err = r.messages.DeleteMany(ctx, filterMsgs)
	if err != nil {
		return false, err
	}

	return true, nil
}

func (r Repo) readFromBucket(ctx context.Context, topicID string, pg messages.Pagination) ([]messages.Message, error) {
	p := NewMPaginatin(pg)
	sortStage := bson.M{
		"$sort": bson.M{
			"_id": 1,
		},
	}
	limitStage := bson.M{
		"$limit": p.Limit,
	}

	cur, err := r.db.Collection("hist").Aggregate(ctx, bson.A{
		bson.M{
			"$match": bson.M{
				"min":     bson.M{"$gt": p.AfterId, "$lt": p.BeforeID},
				"topicID": topicID,
			},
		},
		sortStage,
		limitStage,
		bson.M{
			"$project": bson.M{
				"msg": 1,
				"_id": 0,
			},
		},
		bson.M{
			"$unwind": "$msg",
		},
		bson.M{
			"$replaceRoot": bson.M{
				"newRoot": "$msg",
			},
		},
		bson.M{
			"$unionWith": bson.M{
				"coll": "messages",
				"pipeline": bson.A{
					bson.M{
						"$match": bson.M{
							"topicID": topicID,
							"_id":     bson.M{"$gt": p.AfterId, "$lt": p.BeforeID},
						},
					},
					sortStage,
					limitStage,
				},
			},
		},
		bson.M{"$match": bson.M{"_id": bson.M{"$gt": p.AfterId, "$lt": p.BeforeID}}},
		sortStage,
		limitStage,
	},
	)

	if err != nil {
		return nil, err
	}

	defer cur.Close(context.Background())

	res := make([]messages.Message, 0, (pg.Limit))

	for cur.Next(ctx) {
		m := Message{}
		if err := cur.Decode(&m); err != nil {
			return nil, fmt.Errorf("cant decode cursor's element into Message{}, err:%w", err)
		}

		res = append(res, messages.Message{
			SenderId: m.SenderId,
			ID:       m.ID.Hex(),
			TopicID:  m.TopicID,
			SentAt:   m.CreatedAt,
			Text:     m.Text,
		})
	}

	return res, nil
}

type ChangeStream struct {
	DocumentKey   string
	OperationType string            // insert or delete
	Msg           *messages.Message // will be nil for "delete" operation type
}

func (r Repo) watchMessagesChangeStream(ctx context.Context, msgChan chan<- *ChangeStream) {
	stream, err := r.messages.Watch(context.TODO(), mongo.Pipeline{})
	if err != nil {
		slog.Error("can't watch messages collection", "err", err)
		return
	}
	defer func() {
		err := recover()
		if err != nil {
			slog.Error("recovering panic while watching change stream", "err", err)
		}
	}()

	defer stream.Close(context.TODO())
	defer close(msgChan)

	for stream.Next(ctx) {
		docKey := stream.Current.Lookup("documentKey").Document().Lookup("_id").ObjectID().Hex()
		opType := stream.Current.Lookup("operationType").StringValue()

		changeStream := ChangeStream{
			DocumentKey:   docKey,
			OperationType: opType,
		}

		if opType == "insert" {
			doc, err := stream.Current.LookupErr("fullDocument")
			if err != nil {
				slog.Error("can't lookup 'fullDocument' from changeStream's document", "err", err)
				continue
			}

			msg := &Message{}
			err = doc.Unmarshal(msg)
			if err != nil {
				slog.Error("can't unmarshal changeStream's document", "err", err)
				continue
			}

			changeStream.Msg = msg.ToApiMessage()
		}

		msgChan <- &changeStream
	}
}

// Watch for mongodb messages collection changes (using change data capture).
// NOTE: cancel func MUST be called.
func (r Repo) WatchMessages() (stream <-chan *ChangeStream, cancel func()) {
	msgChan := make(chan *ChangeStream, 1)
	ctx, cancelCtx := context.WithCancel(context.TODO())

	go r.watchMessagesChangeStream(ctx, msgChan)

	return msgChan, func() {
		cancelCtx()
	}
}

// returns ObjectID from time.Time for filtering based on creation date
func tsOnlyObjectID(timestamp time.Time) primitive.ObjectID {
	var b [12]byte

	binary.BigEndian.PutUint32(b[0:4], uint32(timestamp.Unix()))
	return b
}
