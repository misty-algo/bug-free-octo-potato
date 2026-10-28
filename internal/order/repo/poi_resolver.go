package repo

import (
	"context"

	"github.com/BwCloudWeGo/bw-cli/internal/order/service"
	"github.com/BwCloudWeGo/bw-cli/pkg/mongox"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.uber.org/zap"
)

// poiDoc 对齐线上 pois 集合（名称字段照抄数据字典：poiId 三方地图 POI ID / name）。
type poiDoc struct {
	ID    bson.ObjectID `bson:"_id"`
	PoiID string        `bson:"poiId"`
	Name  string        `bson:"name"`
}

// POINameResolver 按 POI ID 批量解析名称（P17 列表 start_name/end_name，涉及表 pois）。
// 兼容两种引用形态：orders.startId/endId 的 ObjectID(_id) 与三方 poiId 字符串。
type POINameResolver struct {
	pois *mongox.Collection[poiDoc]
}

// NewPOINameResolver 基于现有 Mongo 连接创建 POI 名称解析器。
func NewPOINameResolver(database *mongo.Database, loggers ...*zap.Logger) *POINameResolver {
	return &POINameResolver{pois: mongox.NewCollection[poiDoc](database, "pois", loggers...)}
}

var _ service.POINameResolver = (*POINameResolver)(nil)

// ResolveNames 批量解析 POI 名称；缺失的 ID 不出现在结果中（调用方降级为空串）。
func (r *POINameResolver) ResolveNames(ctx context.Context, ids []string) (map[string]string, error) {
	names := make(map[string]string, len(ids))
	if len(ids) == 0 {
		return names, nil
	}
	oids := bson.A{}
	strs := bson.A{}
	for _, id := range ids {
		if oid, err := bson.ObjectIDFromHex(id); err == nil {
			oids = append(oids, oid)
		} else {
			strs = append(strs, id)
		}
	}
	or := bson.A{}
	if len(oids) > 0 {
		or = append(or, bson.M{"_id": bson.M{"$in": oids}})
	}
	if len(strs) > 0 {
		or = append(or, bson.M{"poiId": bson.M{"$in": strs}})
	}
	documents, err := r.pois.FindMany(ctx, bson.M{"$or": or}, options.Find().SetProjection(bson.M{"name": 1}))
	if err != nil {
		return nil, err
	}
	for _, document := range documents {
		if !document.ID.IsZero() {
			names[document.ID.Hex()] = document.Name
		}
		if document.PoiID != "" {
			names[document.PoiID] = document.Name
		}
	}
	return names, nil
}
