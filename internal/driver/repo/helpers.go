package repo

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// withTimeout 限制单次 Mongo 操作时长，避免跨域调用拖住请求。
func withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, 3*time.Second)
}

// objectID 把字符串主键还原为 Mongo ObjectId。
func objectID(id string) (bson.ObjectID, error) {
	return bson.ObjectIDFromHex(id)
}
