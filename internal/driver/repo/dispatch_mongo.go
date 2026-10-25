package repo

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/BwCloudWeGo/bw-cli/internal/driver/entity"
)

// dispatchTaskStateAssigned 与订单域 dispatch_tasks 的状态取值保持一致。
const dispatchTaskStateAssigned = "ASSIGNED"

// MongoDispatchRepository 写 dispatch_tasks，把准入 source_revision 固化在任务上。
// 真实派单域落地后，这里应改为调用派单服务，语义不变。
type MongoDispatchRepository struct{ db *mongo.Database }

func NewMongoDispatchRepository(db *mongo.Database) *MongoDispatchRepository {
	return &MongoDispatchRepository{db: db}
}

func (r *MongoDispatchRepository) CreateAssignment(ctx context.Context, driverID string, orderIDs []string, revision int64, taskNo string, at time.Time) (string, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	driverOID, err := objectID(driverID)
	if err != nil {
		return "", err
	}
	orders := make([]bson.ObjectID, 0, len(orderIDs))
	for _, raw := range orderIDs {
		oid, err := bson.ObjectIDFromHex(raw)
		if err != nil {
			return "", err
		}
		orders = append(orders, oid)
	}
	id := bson.NewObjectID()
	_, err = r.db.Collection("dispatch_tasks").InsertOne(ctx, bson.M{
		"_id": id, "taskNo": taskNo, "state": dispatchTaskStateAssigned,
		"driverId": driverOID, "orderIds": orders,
		// assignmentRevision 与准入源版本同源，限制变更后旧授权自然失效。
		"assignmentRevision": revision, "createdAt": at,
	})
	if err != nil {
		return "", err
	}
	return id.Hex(), nil
}

var _ entity.DispatchRepository = (*MongoDispatchRepository)(nil)
