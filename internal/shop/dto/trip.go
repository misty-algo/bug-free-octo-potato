package dto

import (
	"time"

	"github.com/BwCloudWeGo/bw-cli/internal/shop/entity"
)

// AppendTrajectoryCommand 包含上报单个行程轨迹点的入参（P08）。
type AppendTrajectoryCommand struct {
	OrderID    string
	DriverID   string
	Location   entity.GeoPoint
	Speed      float64
	Bearing    float64
	Accuracy   float64
	Mileage    float64
	PointType  string
	ReportedAt time.Time
	Source     string
}

// ListTrajectoriesCommand 包含查询订单轨迹列表的入参。
type ListTrajectoriesCommand struct {
	OrderID string
	Limit   int // <=0 返回全部
}

// ListTrajectoriesDTO 包含轨迹点分页列表出参。
type ListTrajectoriesDTO struct {
	Items []*TrajectoryDTO
	Total int64
}

// TrajectoryDTO 是轨迹点的 service 响应 DTO。
type TrajectoryDTO struct {
	ID          string
	OrderID     string
	DriverID    string
	Location    entity.GeoPoint
	Speed       float64
	Bearing     float64
	Accuracy    float64
	Mileage     float64
	PointType   string
	ReportedAt  time.Time
	CollectedAt time.Time
	Source      string
	Quality     string
}

// FromTrajectoryPoint 将轨迹点聚合转换为 service 响应 DTO。
func FromTrajectoryPoint(point *entity.TrajectoryPoint) *TrajectoryDTO {
	if point == nil {
		return nil
	}
	return &TrajectoryDTO{
		ID:          point.ID,
		OrderID:     point.OrderID,
		DriverID:    point.DriverID,
		Location:    point.Location,
		Speed:       point.Speed,
		Bearing:     point.Bearing,
		Accuracy:    point.Accuracy,
		Mileage:     point.Mileage,
		PointType:   point.PointType,
		ReportedAt:  point.ReportedAt,
		CollectedAt: point.CollectedAt,
		Source:      point.Source,
		Quality:     point.Quality,
	}
}

// BindVirtualNumberCommand 包含绑定虚拟号的入参（P09）。
// OrderStatus 传入订单当前状态，按 CanContact 规则校验可联系窗口。
type BindVirtualNumberCommand struct {
	OrderID     string
	PassengerID string
	DriverID    string
	VirtualNo   string
	OrderStatus string
	ExpireAt    time.Time
}

// GetVirtualNumberCommand 包含按订单查询虚拟号的入参。
type GetVirtualNumberCommand struct {
	OrderID string
}

// ReleaseVirtualNumberCommand 包含释放虚拟号的入参。
type ReleaseVirtualNumberCommand struct {
	OrderID string
}

// VirtualNumberDTO 是虚拟号的 service 响应 DTO。
type VirtualNumberDTO struct {
	ID          string
	OrderID     string
	PassengerID string
	DriverID    string
	VirtualNo   string
	Status      string
	BoundAt     time.Time
	ReleasedAt  time.Time
	ExpireAt    time.Time
}

// FromVirtualNumber 将虚拟号聚合转换为 service 响应 DTO。
func FromVirtualNumber(v *entity.VirtualNumber) *VirtualNumberDTO {
	if v == nil {
		return nil
	}
	return &VirtualNumberDTO{
		ID:          v.ID,
		OrderID:     v.OrderID,
		PassengerID: v.PassengerID,
		DriverID:    v.DriverID,
		VirtualNo:   v.VirtualNo,
		Status:      v.Status,
		BoundAt:     v.BoundAt,
		ReleasedAt:  v.ReleasedAt,
		ExpireAt:    v.ExpireAt,
	}
}

// RequestDestinationChangeCommand 包含发起中途改点申请的入参（P10）。
type RequestDestinationChangeCommand struct {
	OrderID        string
	NewEndLocation entity.GeoPoint
	NewEndAddress  string
	NewEndPoiID    string
	NewEstimateFee int64
}

// ConfirmDestinationChangeCommand 包含司机确认改点的入参。
type ConfirmDestinationChangeCommand struct {
	ID string
}

// RejectDestinationChangeCommand 包含司机拒绝改点的入参。
type RejectDestinationChangeCommand struct {
	ID string
}

// CancelDestinationChangeCommand 包含乘客撤销改点的入参。
type CancelDestinationChangeCommand struct {
	ID string
}

// ListDestinationChangesCommand 包含查询订单全部改点申请的入参。
type ListDestinationChangesCommand struct {
	OrderID string
}

// ListDestinationChangesDTO 包含改点申请列表出参。
type ListDestinationChangesDTO struct {
	Items []*DestinationChangeDTO
	Total int64
}

// DestinationChangeDTO 是中途改点的 service 响应 DTO。
type DestinationChangeDTO struct {
	ID             string
	OrderID        string
	NewEndLocation entity.GeoPoint
	NewEndAddress  string
	NewEndPoiID    string
	NewEstimateFee int64
	Status         string
	RequestedAt    time.Time
	ConfirmedAt    time.Time
	ExpiresAt      time.Time
	Version        int64
}

// FromDestinationChange 将改点聚合转换为 service 响应 DTO。
func FromDestinationChange(c *entity.DestinationChange) *DestinationChangeDTO {
	if c == nil {
		return nil
	}
	return &DestinationChangeDTO{
		ID:             c.ID,
		OrderID:        c.OrderID,
		NewEndLocation: c.NewEndLocation,
		NewEndAddress:  c.NewEndAddress,
		NewEndPoiID:    c.NewEndPoiID,
		NewEstimateFee: c.NewEstimateFee,
		Status:         c.Status,
		RequestedAt:    c.RequestedAt,
		ConfirmedAt:    c.ConfirmedAt,
		ExpiresAt:      c.ExpiresAt,
		Version:        c.Version,
	}
}

// CreateTripShareCommand 包含创建行程分享的入参（P11）。
// OrderEndedAt 为空表示行程进行中，按 24 小时上限；非空按结束时间 +30 分钟提前失效。
type CreateTripShareCommand struct {
	OrderID      string
	CreatorID    string
	OrderEndedAt *time.Time
}

// AccessTripShareCommand 包含通过令牌访问分享的入参。
type AccessTripShareCommand struct {
	Token string
}

// RevokeTripShareCommand 包含撤销分享的入参。
type RevokeTripShareCommand struct {
	Token string
}

// TripShareDTO 是行程分享的 service 响应 DTO。
type TripShareDTO struct {
	ID          string
	OrderID     string
	ShareToken  string
	CreatorID   string
	ExpireAt    time.Time
	Revoked     bool
	AccessCount int64
	CreatedAt   time.Time
}

// FromTripShare 将行程分享聚合转换为 service 响应 DTO。
func FromTripShare(s *entity.TripShare) *TripShareDTO {
	if s == nil {
		return nil
	}
	return &TripShareDTO{
		ID:          s.ID,
		OrderID:     s.OrderID,
		ShareToken:  s.ShareToken,
		CreatorID:   s.CreatorID,
		ExpireAt:    s.ExpireAt,
		Revoked:     s.Revoked,
		AccessCount: s.AccessCount,
		CreatedAt:   s.CreatedAt,
	}
}

// QuoteCancelCommand 包含生成取消报价的入参（P12）。
// OrderVersion 为绑定订单版本；AcceptedAt 为司机接单时间，用于免费窗口判定。
type QuoteCancelCommand struct {
	OrderID      string
	QuoteNo      string
	OrderVersion int64
	AcceptedAt   time.Time
	ReasonCode   string
}

// GetLatestCancelQuoteCommand 包含查询订单最新报价的入参。
type GetLatestCancelQuoteCommand struct {
	OrderID string
}

// ConsumeCancelQuoteCommand 包含消费取消报价的入参。
type ConsumeCancelQuoteCommand struct {
	QuoteNo      string
	OrderVersion int64
}

// CancelQuoteDTO 是取消报价的 service 响应 DTO。
type CancelQuoteDTO struct {
	ID           string
	OrderID      string
	QuoteNo      string
	CancelFee    int64
	BoundVersion int64
	ReasonCode   string
	ExpiresAt    time.Time
	Used         bool
	CreatedAt    time.Time
}

// FromCancelQuote 将取消报价聚合转换为 service 响应 DTO。
func FromCancelQuote(q *entity.CancelQuote) *CancelQuoteDTO {
	if q == nil {
		return nil
	}
	return &CancelQuoteDTO{
		ID:           q.ID,
		OrderID:      q.OrderID,
		QuoteNo:      q.QuoteNo,
		CancelFee:    q.CancelFee,
		BoundVersion: q.BoundVersion,
		ReasonCode:   q.ReasonCode,
		ExpiresAt:    q.ExpiresAt,
		Used:         q.Used,
		CreatedAt:    q.CreatedAt,
	}
}
