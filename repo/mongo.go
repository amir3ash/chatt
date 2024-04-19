package repo

import (
	"chat-system/core"
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
)

type Message struct {
	// DefaultModel adds _id, created_at and updated_at fields to the Model.
	mgm.DefaultModel `bson:",inline"`
	TopicID          string              `bson:"topicID"`
	SenderId         string              `bson:"senderID"`
	Timestamp        primitive.Timestamp `bson:"ts"`
	Text             string              `bson:"text"`
}

type pagination struct {
	Limit    *int64
	AfterId  primitive.ObjectID
	BeforeID primitive.ObjectID
}

func newMPaginatin(c core.Pagination) (p pagination) {
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
	db.CreateCollection(context.Background(), "hist")
	db.Collection("hist").Indexes().CreateOne(context.Background(),
		mongo.IndexModel{Keys: bson.D{{"topicID", 1}, {"min", 1}}})

	go func() {
		for range time.Tick(2 * time.Minute) {
			slog.Debug("agregating messages into hist")
			if _, err := repo.writeToBucket(context.Background()); err != nil {
				slog.Error("can't aggregate messages", "err", err)
			}
		}
	}()

	return repo, nil
}

func (r Repo) ListMessages(ctx context.Context, topicID string, pg core.Pagination) ([]core.Message, error) {
	return r.readFromBucket(ctx, topicID, pg)
}

func (r Repo) SendMsgToTopic(ctx context.Context, sender core.Sender, topicID string, message string) (core.Message, error) {
	msg := &Message{
		SenderId: sender.ID,
		Text:     message,
		TopicID:  topicID,
	}

	err := r.messages.CreateWithCtx(ctx, msg)
	if err != nil {
		return core.Message{}, err
	}

	return core.Message{
		SenderId: msg.SenderId,
		ID:       msg.ID.Hex(),
		SentAt:   msg.CreatedAt,
		Text:     msg.Text,
	}, nil
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

func (r Repo) readFromBucket(ctx context.Context, topicID string, pg core.Pagination) ([]core.Message, error) {
	p := newMPaginatin(pg)
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

	messages := make([]core.Message, 0, (pg.Limit))

	for cur.Next(ctx) {
		m := Message{}
		if err := cur.Decode(&m); err != nil {
			return nil, fmt.Errorf("cant decode cursor's element into Message{}, err:%w", err)
		}

		messages = append(messages, core.Message{
			SenderId: m.SenderId,
			ID:       m.ID.Hex(),
			SentAt:   m.CreatedAt,
			Text:     m.Text,
		})
	}

	return messages, nil
}

// returns ObjectID from time.Time for filtering based on creation date
func tsOnlyObjectID(timestamp time.Time) primitive.ObjectID {
	var b [12]byte

	binary.BigEndian.PutUint32(b[0:4], uint32(timestamp.Unix()))
	return b
}
