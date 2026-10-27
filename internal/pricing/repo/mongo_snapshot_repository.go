package repo

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.uber.org/zap"

	"github.com/BwCloudWeGo/bw-cli/internal/pricing/entity"
	shopmodel "github.com/BwCloudWeGo/bw-cli/internal/shop/model"
	"github.com/BwCloudWeGo/bw-cli/pkg/mongox"
)

// MongoRuleSnapshotRepository 从 pricing_snapshots 集合读取下单时锁定的计价规则快照。
//
// 计价必须取快照而不是当前规则：否则规则在行程期间被运营修改，
// 乘客看到的预估价与最终实付会对不上。快照的创建方是下单链路，
// 本仓储只负责按订单号还原成类型化的领域规则。
type MongoRuleSnapshotRepository struct {
	snapshots *mongox.DocumentStore[shopmodel.PricingSnapshotsDocument]
	log       *zap.Logger
}

// NewMongoRuleSnapshotRepository 创建计价规则快照仓储。
func NewMongoRuleSnapshotRepository(db *mongo.Database, loggers ...*zap.Logger) *MongoRuleSnapshotRepository {
	log := zap.NewNop()
	if len(loggers) > 0 && loggers[0] != nil {
		log = loggers[0]
	}
	return &MongoRuleSnapshotRepository{
		snapshots: mongox.NewDocumentStore[shopmodel.PricingSnapshotsDocument](db, log),
		log:       log,
	}
}

// SnapshotByOrder 按订单号返回该单锁定的计价规则；不存在返回 ErrRuleSnapshotNotFound。
func (r *MongoRuleSnapshotRepository) SnapshotByOrder(ctx context.Context, orderNo string) (entity.PricingRule, error) {
	start := time.Now()
	document, err := r.snapshots.FindOne(ctx, bson.M{"order_id": orderNo, "is_deleted": false})
	if errors.Is(err, mongox.ErrNotFound) {
		err = entity.ErrRuleSnapshotNotFound
	}
	r.logOperation("SnapshotByOrder", orderNo, start, err)
	if err != nil {
		return entity.PricingRule{}, err
	}
	return ruleFromDocument(document.Snapshot), nil
}

func (r *MongoRuleSnapshotRepository) logOperation(operation string, id string, start time.Time, err error) {
	fields := []zap.Field{
		zap.String("repository", "pricing_mongo_rule_snapshot"),
		zap.String("operation", operation),
		zap.String("order_no", id),
		zap.Float64("latency_ms", float64(time.Since(start).Microseconds())/1000),
	}
	if err != nil {
		fields = append(fields, zap.Error(err))
		r.log.Warn("mongodb repository operation completed with error", fields...)
		return
	}
	r.log.Info("mongodb repository operation completed", fields...)
}
