package entity

import "context"

// Repository 定义 order 服务层需要的持久化行为。
// OrderFilter 订单列表过滤与分页（P17）。
// View 为契约过滤视图（空等价「全部」），判定口径见 MatchesFilter；
// Mongo 侧翻译见 repo.buildOrderFilter，两侧必须保持一致（计划附录 E）。
type OrderFilter struct {
	PassengerID string // 必填（归属校验）
	View        string // 全部/进行中/待支付/已完成/已取消（空=全部）
	Limit       int
	Offset      int
}

// OrderQueryRepository 扩展只读查询（P17-P18）。
// Gorm 实现不需要它；Mongo 实现（*repo.MongoRepository）在 Task 2 实现。
type OrderQueryRepository interface {
	ListFiltered(ctx context.Context, filter *OrderFilter) ([]*Order, int64, error)
	// FindByOrderNo 按业务号 orderNo 精确查找（契约 id 口径，P18 详情）。
	FindByOrderNo(ctx context.Context, orderNo string) (*Order, error)
}

// TicketRepository 投诉工单写入（P19）。
// CreateTicket 幂等建单：同 ticketNo 已存在时不重复建（created=false），调用方读回原单返回。
type TicketRepository interface {
	CreateTicket(ctx context.Context, ticket *Ticket) (created bool, err error)
	FindTicketByTicketNo(ctx context.Context, ticketNo string) (*Ticket, error)
}

// TicketQueryRepository 工单只读查询（P22 客服页我的工单）。
// 按投诉人归属过滤（D4：只出本人单），createdAt 倒序。
type TicketQueryRepository interface {
	ListTicketsByComplainant(ctx context.Context, complainantID string) ([]*Ticket, error)
}

// SafetyEventRepository 危险驾驶投诉的安全侧记录（P19）。
// RecordDangerousDriving 记两条：safety_events 安全事件 + event_logs 事件日志。
type SafetyEventRepository interface {
	RecordDangerousDriving(ctx context.Context, report *SafetyEventReport, log *EventLog) error
}

// RatingRepository 评价原子写（P19）。
// MarkRated 以 _id+version 乐观锁且 rated!=true 为条件原子标记已评（rated=true + 内嵌 rating），
// matched=false 表示已被抢先写入/版本漂移（唯一键兜底，调用方按重复评价处理）。
type RatingRepository interface {
	MarkRated(ctx context.Context, id string, version int64, rating *Rating) (bool, error)
}

type Repository interface {
	Save(ctx context.Context, item *Order) error
	FindByID(ctx context.Context, id string) (*Order, error)
	List(ctx context.Context, offset int, limit int) ([]*Order, int64, error)
	Delete(ctx context.Context, id string) error
}
