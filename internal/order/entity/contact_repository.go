package entity

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// ContactRepository 持久化 D07 的联系命令。部署的 MongoDB 是 standalone，
// 因此授权判定与意图持久化必须在同一边界内完成：先以 uk(orderNo, requestNo)
// 落唯一意图，再对订单文档做 version 守卫的单文档原子更新，最后落幂等命令记录。
type ContactRepository interface {
	OrderByNo(context.Context, string) (*RideOrder, error)
	// OrderByID 按订单主键加载：对外参数名是 order_id，传入订单 ObjectId 时也应命中。
	OrderByID(context.Context, bson.ObjectID) (*RideOrder, error)
	TaskByID(context.Context, bson.ObjectID) (*DispatchTask, error)
	FindCommand(context.Context, CommandKey) (*CommandResult, error)

	// SaveContact 返回已存在的意图以覆盖崩溃窗口内的重试，避免重复落库。
	SaveContact(context.Context, ContactMutation) (ContactIntent, bool, error)
	// SetMessageState 供通讯域回写 STORED，避免行程域冒充「通讯域已保存」。
	SetMessageState(context.Context, string, int64, string) error
	// MarkDelivered 标记对端已拉取（不是已读）。
	MarkDelivered(context.Context, string, string, int64) error
}

// VirtualCallRequest 是向虚拟号提供方申请绑定的入参，不含真实手机号。
type VirtualCallRequest struct {
	OrderNo   string
	RequestNo string
	Revision  int64
}

// VirtualCallProvider 是虚拟号提供方适配器。课程使用可控适配器验证
// 「实际呼叫时按当前轮次再授权」与撤销能力；无法验证真实通道时不声称立即失效。
type VirtualCallProvider interface {
	Bind(context.Context, VirtualCallRequest) (VirtualCallBinding, error)
	Revoke(context.Context, string) error
}
