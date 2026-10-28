package repo

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/BwCloudWeGo/bw-cli/internal/user/entity"
	"github.com/BwCloudWeGo/bw-cli/internal/user/model"
)

// MongoConfigRepository 读线上 config_dictionaries 现值（P22，A18 参数字典）。
// 只读不写：配置维护由 A18 管理后台负责。
type MongoConfigRepository struct {
	db *mongo.Database
}

var _ entity.ServiceConfigRepository = (*MongoConfigRepository)(nil)

// NewMongoConfigRepository 创建参数字典仓储。
func NewMongoConfigRepository(db *mongo.Database) *MongoConfigRepository {
	return &MongoConfigRepository{db: db}
}

// GetPublishedValues 按 dictKey 批量读取现值，state=已发布 才生效；
// 无记录 key 不出现在结果里。
func (r *MongoConfigRepository) GetPublishedValues(ctx context.Context, keys []string) (map[string]string, error) {
	cursor, err := r.db.Collection(model.CollectionConfigDictionaries).Find(ctx, bson.M{
		"dictKey": bson.M{"$in": keys},
		"state":   "已发布",
	})
	if err != nil {
		return nil, err
	}
	var docs []model.ConfigDictionaryDocument
	if err := cursor.All(ctx, &docs); err != nil {
		return nil, err
	}
	out := make(map[string]string, len(docs))
	for _, doc := range docs {
		out[doc.Key] = doc.Value
	}
	return out, nil
}
