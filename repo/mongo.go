package repo

import (
	"chat-system/core"
	"context"
	"fmt"

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

	return &Repo{
		messages: mgm.NewCollection(db, mgm.CollName(&Message{})),
		db:       db}, nil
}

func (r Repo) ListMessages(ctx context.Context, topicID string, pg core.Pagination) ([]core.Message, error) {
	p := newMPaginatin(pg)

	cur, err := r.messages.Find(ctx,
		bson.M{"topicID": topicID, "_id": bson.M{"$gt": p.AfterId, "$lt": p.BeforeID}},
		&options.FindOptions{Limit: p.Limit, Sort: bson.M{"ts": 1}},
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
