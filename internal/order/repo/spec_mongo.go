package repo

import (
	"context"
	"errors"
	"time"

	"github.com/BwCloudWeGo/bw-cli/internal/order/entity"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// configDictionaryCollection 属 A18 后台配置域；行程域只读消费，
// 不创建、不迁移、不写（未登记进 arrival  schema 清单）。
const configDictionaryCollection = "config_dictionary"

type MongoSpecRepository struct{ db *mongo.Database }

func NewMongoSpecRepository(db *mongo.Database) *MongoSpecRepository {
	return &MongoSpecRepository{db: db}
}

// PublishedEntries 只取已发布条目。集合缺失（A18 尚未交付）时返回空，
// 由服务层翻译成「暂无已发布规范」，绝不造默认规范冒充已发布。
func (r *MongoSpecRepository) PublishedEntries(ctx context.Context, key string) ([]entity.SpecEntry, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	cursor, err := r.db.Collection(configDictionaryCollection).Find(ctx,
		bson.M{"key": key, "state": entity.SpecStatePublished},
		options.Find().SetSort(bson.D{{Key: "category", Value: 1}}))
	if err != nil {
		return nil, mongoArrivalError(err)
	}
	defer cursor.Close(ctx)
	var entries []entity.SpecEntry
	for cursor.Next(ctx) {
		var entry entity.SpecEntry
		if errors.Is(cursor.Decode(&entry), nil) {
			entries = append(entries, entry)
		}
	}
	return entries, mongoArrivalError(cursor.Err())
}

var _ entity.SpecRepository = (*MongoSpecRepository)(nil)
