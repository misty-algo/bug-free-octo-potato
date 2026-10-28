package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"unicode/utf8"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/BwCloudWeGo/bw-cli/internal/order/dto"
	"github.com/BwCloudWeGo/bw-cli/internal/order/entity"
)

// 投诉输入限制（契约 3.6.4）。
const (
	complaintDescMax   = 500 // 描述 ≤500 字
	complaintImagesMax = 6   // 图片 ≤6 张
)

// dangerousDrivingEventName 是危险驾驶投诉的事件日志名（event_logs.name）。
const dangerousDrivingEventName = "dangerous_driving_complaint"

// complaintTypeLabels 契约中文五类 → tickets.type 机器码。
var complaintTypeLabels = map[string]string{
	"绕路":   entity.TicketTypeRouteDetour,
	"服务态度": entity.TicketTypeDriverService,
	"危险驾驶": entity.TicketTypeDangerousDriving,
	"费用异议": entity.TicketTypePriceDispute,
	"其他":   entity.TicketTypeOther,
}

// SubmitComplaint 是 P19 提交投诉（POST /api/pax/order/complaint，契约 3.6.4）。
// 提交即建单：ticketNo 由幂等键派生（同键同号，重试 upsert 同一文档不重复建单）；
// 危险驾驶额外写 safety_events+event_logs（值班队列告警不因展示审核阻断）。
// TODO(孙源): 敏感词展示审核与超 7 日转人工归客服域，此处不实现。
func (s *Service) SubmitComplaint(ctx context.Context, cmd dto.SubmitComplaintCommand) (*dto.SubmitComplaintResult, error) {
	if cmd.PassengerID == "" || cmd.OrderID == "" || cmd.IdempotencyKey == "" {
		return nil, entity.ErrInvalidOrder
	}
	if strings.TrimSpace(cmd.Desc) == "" || utf8.RuneCountInString(cmd.Desc) > complaintDescMax {
		return nil, entity.ErrInvalidOrder // 描述非空且 ≤500 字
	}
	if len(cmd.Images) > complaintImagesMax {
		return nil, entity.ErrInvalidOrder // 图片 ≤6 张
	}
	typ, ok := normalizeComplaintType(cmd.Type)
	if !ok {
		return nil, entity.ErrInvalidOrder
	}
	queryRepo, ok := s.repo.(entity.OrderQueryRepository)
	if !ok {
		return nil, errors.New("order query repository not available")
	}
	order, err := queryRepo.FindByOrderNo(ctx, cmd.OrderID)
	if err != nil {
		return nil, err
	}
	if order.PassengerID != cmd.PassengerID {
		return nil, entity.ErrOrderNotFound // 归属校验，不泄露存在性（D4）
	}

	now := s.clock.Now().UTC()
	ticket, err := entity.NewTicket(
		bson.NewObjectID().Hex(),
		ticketNoFromKey(cmd.PassengerID, cmd.IdempotencyKey),
		order.ID,
		order.OrderNo,
		cmd.PassengerID,
		order.DriverID,
		typ,
		cmd.Desc,
		append([]string(nil), cmd.Images...),
		now,
	)
	if err != nil {
		return nil, entity.ErrInvalidOrder
	}

	ticketRepo, ok := s.repo.(entity.TicketRepository)
	if !ok {
		return nil, errors.New("ticket repository not available")
	}
	created, err := ticketRepo.CreateTicket(ctx, ticket)
	if err != nil {
		return nil, err
	}
	if !created {
		// 幂等重试：同键同号，读回原单返回（不重复建单）。
		existing, err := ticketRepo.FindTicketByTicketNo(ctx, ticket.TicketNo)
		if err != nil {
			return nil, err
		}
		return complaintResult(existing), nil
	}

	// 危险驾驶：额外记安全事件 + 事件日志（1 分钟提示/5 分钟升级归司机端安全域 D17）。
	if typ == entity.TicketTypeDangerousDriving {
		safety, ok := s.repo.(entity.SafetyEventRepository)
		if !ok {
			return nil, errors.New("safety event repository not available")
		}
		eventID := "SAFETY:" + ticket.TicketNo
		if err := safety.RecordDangerousDriving(ctx,
			&entity.SafetyEventReport{
				EventID:         eventID,
				TicketNo:        ticket.TicketNo,
				OrderNo:         order.OrderNo,
				DriverID:        order.DriverID,
				Description:     cmd.Desc,
				ClientRequestID: cmd.IdempotencyKey,
				ReceivedAt:      now,
			},
			&entity.EventLog{
				EventID: eventID,
				T:       now,
				Name:    dangerousDrivingEventName,
				Detail:  ticket.TicketNo + " 危险驾驶投诉已受理",
				Kind:    "safety",
			}); err != nil {
			return nil, err
		}
	}
	return complaintResult(ticket), nil
}

// complaintResult 映射 P19 出参：ticket_id 取工单号（幂等派生，同键同号）。
func complaintResult(ticket *entity.Ticket) *dto.SubmitComplaintResult {
	return &dto.SubmitComplaintResult{
		TicketID:  ticket.TicketNo,
		State:     ticket.State,
		StateName: entity.TicketStateName(ticket.State),
	}
}

// normalizeComplaintType 接受契约中文五类或机器码，归一到 tickets.type 机器码。
func normalizeComplaintType(typ string) (string, bool) {
	if code, ok := complaintTypeLabels[strings.TrimSpace(typ)]; ok {
		return code, true
	}
	if entity.ValidTicketType(typ) {
		return strings.ToUpper(typ), true
	}
	return "", false
}

// ticketNoFromKey 由幂等键派生工单号：同键同号，重试 upsert 同一文档；按投诉人隔离防键撞车。
func ticketNoFromKey(passengerID, key string) string {
	sum := sha256.Sum256([]byte(passengerID + ":" + key))
	return "TK" + hex.EncodeToString(sum[:])[:16]
}
