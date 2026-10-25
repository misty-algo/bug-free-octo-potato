package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/BwCloudWeGo/bw-cli/internal/order/dto"
	"github.com/BwCloudWeGo/bw-cli/internal/order/entity"
	"github.com/BwCloudWeGo/bw-cli/pkg/clock"
	apperrors "github.com/BwCloudWeGo/bw-cli/pkg/errors"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"
)

const maxMessageLength = 500

// ContactService 承载 D07 的乘客联系。撤权边界在行程域：授权判定与意图
// 持久化在同一边界内完成，不存在「先查权限、释放锁、再无条件接收」的窗口。
type ContactService struct {
	repo     entity.ContactRepository
	provider entity.VirtualCallProvider
	clock    clock.Clock
}

func NewContactService(r entity.ContactRepository, p entity.VirtualCallProvider, c clock.Clock) *ContactService {
	return &ContactService{repo: r, provider: p, clock: c}
}

// normalize 生成规范化内容摘要用的文本：去掉首尾空白并折叠内部空白。
func normalize(text string) string {
	return strings.Join(strings.Fields(strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return ' '
		}
		return r
	}, text)), " ")
}

func digestOf(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return hex.EncodeToString(sum[:])
}

func nextSeq(messages []entity.OrderMessage) int64 {
	var max int64
	for _, m := range messages {
		if m.Seq > max {
			max = m.Seq
		}
	}
	return max + 1
}

// SendMessage 是司机发消息。仅 ACCEPTED/ARRIVED/IN_TRIP 允许；拼车按 order_id 选目标。
func (s *ContactService) SendMessage(ctx context.Context, cmd dto.SendMessageCommand) (*dto.SendMessageResult, error) {
	if err := requireRole(cmd.Principal, "driver"); err != nil {
		return nil, err
	}
	if err := validateKey(cmd.IdempotencyKey); err != nil {
		return nil, err
	}
	cmd.OrderID = strings.TrimSpace(cmd.OrderID)
	cmd.Text = strings.TrimSpace(cmd.Text)
	cmd.ClientMessageID = strings.TrimSpace(cmd.ClientMessageID)
	if cmd.OrderID == "" || cmd.Text == "" || cmd.ClientMessageID == "" {
		return nil, apperrors.InvalidArgument("INVALID_ARGUMENT", "order_id, text and client_message_id are required")
	}
	if len([]rune(cmd.Text)) > maxMessageLength {
		return nil, apperrors.InvalidArgument("MESSAGE_TOO_LONG", fmt.Sprintf("text must not exceed %d characters", maxMessageLength))
	}
	hash, err := requestHash(cmd)
	if err != nil {
		return nil, err
	}
	order, err := loadOrderByRef(ctx, s.repo, cmd.OrderID)
	if err != nil {
		return nil, err
	}
	if order.DriverID.Hex() != cmd.Principal.UserID {
		return nil, apperrors.New(apperrors.Kind("FORBIDDEN"), "FORBIDDEN", "driver does not own this order")
	}
	// 立即撤权：状态离开联系窗口即拒绝新请求，不看缓存。
	if !entity.ContactWindowOpen(strings.ToUpper(order.State)) {
		return nil, apperrors.Conflict("CONTACT_NOT_AUTHORIZED", "order is not in a contactable state")
	}
	key := entity.CommandKey{ActorID: cmd.Principal.UserID, CommandType: "SEND_MESSAGE", ResourceID: order.OrderNo, IdempotencyKey: cmd.IdempotencyKey}
	saved, err := s.repo.FindCommand(ctx, key)
	if err != nil {
		return nil, err
	}
	if result, err := replay[dto.SendMessageResult](saved, hash); err != nil || result != nil {
		return result, err
	}

	revision := order.Version
	if order.DispatchID != bson.NilObjectID {
		if task, err := s.repo.TaskByID(ctx, order.DispatchID); err == nil && task != nil && task.AssignmentRevision > 0 {
			revision = task.AssignmentRevision
		}
	}
	now := utcMillis(s.clock.Now())
	seq := nextSeq(order.Messages)
	intent := entity.ContactIntent{
		IntentID:        "CI:" + uuid.NewString(),
		OrderNo:         order.OrderNo,
		DriverID:        order.DriverID.Hex(),
		PassengerID:     order.PassengerID.Hex(),
		Revision:        revision,
		Action:          entity.IntentSendMessage,
		Digest:          digestOf(order.OrderNo, normalize(cmd.Text)),
		RequestNo:       cmd.IdempotencyKey,
		ClientMessageID: cmd.ClientMessageID,
		Seq:             seq,
		// 意图受理不等于消息已保存，状态停在 ACCEPTED。
		State:     entity.MessageAccepted,
		CreatedAt: now,
	}
	message := entity.OrderMessage{Seq: seq, From: "drv", Text: cmd.Text, T: now, ClientMessageID: cmd.ClientMessageID, State: entity.MessageAccepted}
	mutation := entity.ContactMutation{Order: *order, Intent: intent, Message: &message}
	mutation.Order.Version++
	mutation.Events = append(mutation.Events, entity.EventLog{EventID: "CONTACT:" + uuid.NewString(), T: now, Name: "ContactIntentAccepted", Detail: order.OrderNo + " 发消息意图已受理", Kind: "audit"})
	mutation.Command, err = commandRecord(key, hash, intent.IntentID, &dto.SendMessageResult{IntentID: intent.IntentID, Seq: seq, State: intent.State}, now)
	if err != nil {
		return nil, err
	}
	stored, _, err := s.repo.SaveContact(ctx, mutation)
	if err != nil {
		return nil, err
	}
	// 崩溃窗口内的重试：返回已存在的意图，不重复落库。
	return &dto.SendMessageResult{IntentID: stored.IntentID, Seq: stored.Seq, State: stored.State}, nil
}

// ListMessages 是消息拉取（游标分页 + 短轮询）。结束后仍可按授权只读历史。
func (s *ContactService) ListMessages(ctx context.Context, cmd dto.ListMessagesCommand) (*dto.ListMessagesResult, error) {
	if err := requireRole(cmd.Principal, "driver"); err != nil {
		return nil, err
	}
	cmd.OrderID = strings.TrimSpace(cmd.OrderID)
	if cmd.OrderID == "" {
		return nil, apperrors.InvalidArgument("INVALID_ARGUMENT", "order_id is required")
	}
	order, err := loadOrderByRef(ctx, s.repo, cmd.OrderID)
	if err != nil {
		return nil, err
	}
	if order.DriverID.Hex() != cmd.Principal.UserID {
		return nil, apperrors.New(apperrors.Kind("FORBIDDEN"), "FORBIDDEN", "driver does not own this order")
	}
	const pageSize = 50
	result := &dto.ListMessagesResult{}
	var maxSeq int64 = cmd.Cursor
	count := 0
	for _, m := range order.Messages {
		if m.Seq <= cmd.Cursor {
			continue
		}
		if count >= pageSize {
			break
		}
		result.Messages = append(result.Messages, dto.ChatMessage{Seq: m.Seq, From: m.From, Text: m.Text, T: m.T, State: m.State, ClientMessageID: m.ClientMessageID})
		if m.Seq > maxSeq {
			maxSeq = m.Seq
		}
		count++
	}
	result.NextCursor = maxSeq
	// 对端已拉取（不是已读）：只推进来自乘客且已落库的消息。
	if len(result.Messages) > 0 {
		if err := s.repo.MarkDelivered(ctx, order.OrderNo, "pax", maxSeq); err != nil {
			return nil, err
		}
	}
	return result, nil
}

// MarkStored 供通讯域回写「已保存」，行程域不冒充该事实。
func (s *ContactService) MarkStored(ctx context.Context, orderNo string, seq int64) (*dto.MessageReceipt, error) {
	order, err := s.repo.OrderByNo(ctx, orderNo)
	if err != nil {
		return nil, err
	}
	if err := s.repo.SetMessageState(ctx, order.OrderNo, seq, entity.MessageStored); err != nil {
		return nil, err
	}
	return &dto.MessageReceipt{OrderID: order.OrderNo, Seq: seq, State: entity.MessageStored}, nil
}

// RequestVirtualCall 申请虚拟号。绑定 order_id、轮次与业务请求号，
// 不返回真实手机号；外部失败只给客服指引，不假称接通。
func (s *ContactService) RequestVirtualCall(ctx context.Context, cmd dto.VirtualCallCommand) (*dto.VirtualCallResult, error) {
	if err := requireRole(cmd.Principal, "driver"); err != nil {
		return nil, err
	}
	if err := validateKey(cmd.IdempotencyKey); err != nil {
		return nil, err
	}
	cmd.OrderID = strings.TrimSpace(cmd.OrderID)
	if cmd.OrderID == "" {
		return nil, apperrors.InvalidArgument("INVALID_ARGUMENT", "order_id is required")
	}
	hash, err := requestHash(cmd)
	if err != nil {
		return nil, err
	}
	order, err := loadOrderByRef(ctx, s.repo, cmd.OrderID)
	if err != nil {
		return nil, err
	}
	if order.DriverID.Hex() != cmd.Principal.UserID {
		return nil, apperrors.New(apperrors.Kind("FORBIDDEN"), "FORBIDDEN", "driver does not own this order")
	}
	if !entity.ContactWindowOpen(strings.ToUpper(order.State)) {
		return nil, apperrors.Conflict("CONTACT_NOT_AUTHORIZED", "order is not in a contactable state")
	}
	key := entity.CommandKey{ActorID: cmd.Principal.UserID, CommandType: "VIRTUAL_CALL", ResourceID: order.OrderNo, IdempotencyKey: cmd.IdempotencyKey}
	saved, err := s.repo.FindCommand(ctx, key)
	if err != nil {
		return nil, err
	}
	if result, err := replay[dto.VirtualCallResult](saved, hash); err != nil || result != nil {
		return result, err
	}
	now := utcMillis(s.clock.Now())
	// 超时查原请求：同一业务请求号返回已绑定的号码，不重复申请。
	if order.VirtualCall != nil && order.VirtualCall.RequestNo == cmd.IdempotencyKey && order.VirtualCall.RevokedAt == nil {
		return &dto.VirtualCallResult{VirtualNumber: order.VirtualCall.Number, ExpiresAt: order.VirtualCall.ExpiresAt, Simulated: order.VirtualCall.Simulated, Notice: simulatedNotice}, nil
	}
	revision := order.Version
	if order.DispatchID != bson.NilObjectID {
		if task, err := s.repo.TaskByID(ctx, order.DispatchID); err == nil && task != nil && task.AssignmentRevision > 0 {
			revision = task.AssignmentRevision
		}
	}
	binding, err := s.provider.Bind(ctx, entity.VirtualCallRequest{OrderNo: order.OrderNo, RequestNo: cmd.IdempotencyKey, Revision: revision})
	if err != nil {
		// 不假称接通：只给客服指引。
		return nil, apperrors.Unavailable("VIRTUAL_CALL_UNAVAILABLE", "虚拟号服务暂时不可用，请通过客服热线联系乘客")
	}
	binding.Revision = revision
	binding.RequestNo = cmd.IdempotencyKey

	intent := entity.ContactIntent{
		IntentID:    "CI:" + uuid.NewString(),
		OrderNo:     order.OrderNo,
		DriverID:    order.DriverID.Hex(),
		PassengerID: order.PassengerID.Hex(),
		Revision:    revision,
		Action:      entity.IntentVirtualCall,
		Digest:      digestOf(order.OrderNo, binding.Number),
		RequestNo:   cmd.IdempotencyKey,
		State:       entity.MessageAccepted,
		CreatedAt:   now,
	}
	result := &dto.VirtualCallResult{VirtualNumber: binding.Number, ExpiresAt: binding.ExpiresAt, Simulated: binding.Simulated, Notice: simulatedNotice}
	mutation := entity.ContactMutation{Order: *order, Intent: intent, VirtualCall: &binding}
	mutation.Order.Version++
	mutation.Events = append(mutation.Events, entity.EventLog{EventID: "CONTACT:" + uuid.NewString(), T: now, Name: "VirtualCallBound", Detail: order.OrderNo + " 虚拟号已绑定（不含真实号码）", Kind: "audit"})
	mutation.Command, err = commandRecord(key, hash, intent.IntentID, result, now)
	if err != nil {
		return nil, err
	}
	if _, _, err := s.repo.SaveContact(ctx, mutation); err != nil {
		return nil, err
	}
	return result, nil
}

const simulatedNotice = "课程模拟，未实际接通"

// SimulatedVirtualCallProvider 是课程可控适配器：明确标记模拟，
// 便于验证「呼叫时按当前轮次再授权」与撤销行为。
type SimulatedVirtualCallProvider struct {
	Clock clock.Clock
}

func (p SimulatedVirtualCallProvider) Bind(_ context.Context, r entity.VirtualCallRequest) (entity.VirtualCallBinding, error) {
	sum := sha256.Sum256([]byte(r.OrderNo + ":" + r.RequestNo))
	digits := hex.EncodeToString(sum[:])[:8]
	return entity.VirtualCallBinding{
		Number:    "170-" + digits[:4] + "-" + digits[4:],
		ExpiresAt: p.Clock.Now().Add(30 * time.Minute),
		Simulated: true,
	}, nil
}

func (p SimulatedVirtualCallProvider) Revoke(_ context.Context, _ string) error { return nil }
