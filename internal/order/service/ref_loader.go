package service

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/BwCloudWeGo/bw-cli/internal/order/entity"
)

// 到达域对外接口的参数名是 *_id，但服务内部历史上一直按业务单号（taskNo / orderNo）
// 检索，于是调用方按字面含义传入主键时只会拿到 NOT_FOUND。下面两个函数让主键与业务
// 单号都可以作为入参：先判定是否为 24 位 ObjectId 并精确命中，未命中再按单号回落。

// taskLoader 是到达与行程服务加载任务所需的最小能力。
type taskLoader interface {
	TaskByNo(context.Context, string) (*entity.DispatchTask, error)
	TaskByID(context.Context, bson.ObjectID) (*entity.DispatchTask, error)
}

// loadTaskByRef 按 task_id 或 taskNo 加载任务。
func loadTaskByRef(ctx context.Context, repo taskLoader, ref string) (*entity.DispatchTask, error) {
	if oid, err := bson.ObjectIDFromHex(ref); err == nil {
		if task, err := repo.TaskByID(ctx, oid); err == nil {
			return task, nil
		}
	}
	return repo.TaskByNo(ctx, ref)
}

// orderLoader 是行程与联系服务加载行程订单所需的最小能力。
type orderLoader interface {
	OrderByNo(context.Context, string) (*entity.RideOrder, error)
	OrderByID(context.Context, bson.ObjectID) (*entity.RideOrder, error)
}

// loadOrderByRef 按 order_id 或 orderNo 加载行程订单。
func loadOrderByRef(ctx context.Context, repo orderLoader, ref string) (*entity.RideOrder, error) {
	if oid, err := bson.ObjectIDFromHex(ref); err == nil {
		if order, err := repo.OrderByID(ctx, oid); err == nil {
			return order, nil
		}
	}
	return repo.OrderByNo(ctx, ref)
}
