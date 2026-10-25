package entity

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// ArrivalRepository persists arrival commands. The deployed MongoDB is a
// standalone server, so multi-document transactions are unavailable
// ("Transaction numbers are only allowed on a replica set member or mongos").
// Every write is therefore a single-document atomic update guarded by the
// document version, and cross-document convergence relies on the idempotent
// command record instead of a transaction.
type ArrivalRepository interface {
	TaskByNo(context.Context, string) (*DispatchTask, error)
	// TaskByID 按任务主键加载。对外接口的参数名是 task_id，调用方按字面含义
	// 传入任务 ObjectId 时必须能命中，见服务层的 loadTaskByRef。
	TaskByID(context.Context, bson.ObjectID) (*DispatchTask, error)
	OrdersByIDs(context.Context, []bson.ObjectID) ([]RideOrder, error)
	DriverByID(context.Context, bson.ObjectID) (*Driver, error)
	// Pickup resolves the boarding coordinate from the embedded GeoJSON point,
	// falling back to the referenced POI (orders.startId → pois._id).
	Pickup(context.Context, *RideOrder) (Coordinate, error)
	FindCommand(context.Context, CommandKey) (*CommandResult, error)
	RecordPosition(context.Context, bson.ObjectID, Coordinate, time.Time) error
	SaveArrival(context.Context, ArrivalMutation) error
}
