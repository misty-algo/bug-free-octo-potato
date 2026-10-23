package repo

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/BwCloudWeGo/bw-cli/internal/driver/entity"
)

// config_dictionary 属 A18 后台配置域，本域只读、不建表、不写。
const colConfigDictionary = "config_dictionary"

type MongoSafetyNoticeReader struct{ db *mongo.Database }

func NewMongoSafetyNoticeReader(db *mongo.Database) *MongoSafetyNoticeReader {
	return &MongoSafetyNoticeReader{db: db}
}

// PublishedNotice 读 A18 已发布条目；key 不存在返回 (nil, nil)，调用方回退内置。
func (r *MongoSafetyNoticeReader) PublishedNotice(ctx context.Context, key string) (*entity.SafetyNotice, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	var doc struct {
		Key     string `bson:"key"`
		Version int64  `bson:"version"`
		Title   string `bson:"title"`
		Blocks  []struct {
			Kind string `bson:"kind"`
			Text string `bson:"text"`
		} `bson:"blocks"`
	}
	err := r.db.Collection(colConfigDictionary).FindOne(ctx, bson.M{"key": "safety.notice." + key}).Decode(&doc)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	notice := &entity.SafetyNotice{Key: key, Version: doc.Version, Title: doc.Title}
	for _, b := range doc.Blocks {
		notice.Blocks = append(notice.Blocks, entity.NoticeBlock{Kind: b.Kind, Text: b.Text})
	}
	return notice, nil
}

var _ entity.SafetyNoticeReader = (*MongoSafetyNoticeReader)(nil)
