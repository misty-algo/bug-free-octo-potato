package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/BwCloudWeGo/bw-cli/internal/driver/dto"
	"github.com/BwCloudWeGo/bw-cli/internal/driver/entity"
	"github.com/BwCloudWeGo/bw-cli/pkg/authn"
	"github.com/BwCloudWeGo/bw-cli/pkg/clock"
	apperrors "github.com/BwCloudWeGo/bw-cli/pkg/errors"
)

// SafetyService 承载 D17 安全事件。
// 受理的判据只有一条：事件文档原子写入成功，才有 event_id 返回。
type SafetyService struct {
	events   entity.SafetyRepository
	tasks    entity.TaskReader
	location entity.DriverLocationReader
	files    entity.FileRepository
	logs     entity.EventWriter
	evidence entity.SafetyEvidenceReader
	clock    clock.Clock
	log      *zap.Logger
}

func NewSafetyService(
	events entity.SafetyRepository,
	tasks entity.TaskReader,
	location entity.DriverLocationReader,
	files entity.FileRepository,
	logs entity.EventWriter,
	evidence entity.SafetyEvidenceReader,
	clk clock.Clock,
	log *zap.Logger,
) *SafetyService {
	if log == nil {
		log = zap.NewNop()
	}
	return &SafetyService{
		events: events, tasks: tasks, location: location,
		files: files, logs: logs, evidence: evidence, clock: clk, log: log,
	}
}

// Report 受理求助。相同 client_request_id 且内容一致时返回原事件编号，
// 内容不同则拒绝：不允许用同一个请求号提交另外一个险情。
func (s *SafetyService) Report(ctx context.Context, cmd dto.ReportEventCommand) (*dto.SafetyEventView, error) {
	if err := requireRole(cmd.Principal, RoleDriver); err != nil {
		return nil, err
	}
	if err := validateRequestID(cmd.ClientRequestID); err != nil {
		return nil, err
	}
	if !entity.IsKnownEventType(cmd.EventType) {
		return nil, apperrors.InvalidArgument("INVALID_EVENT_TYPE", "event type is not in the published dictionary")
	}
	if len([]rune(cmd.Description)) > entity.MaxSafetyDescription {
		return nil, apperrors.InvalidArgument("DESCRIPTION_TOO_LONG",
			fmt.Sprintf("description must be at most %d characters", entity.MaxSafetyDescription))
	}
	now := s.clock.Now().UTC()

	// 位置证据：本次采集优先，回落服务端最后已知位置，都没有才记 UNKNOWN。
	position, positionNote := s.resolvePosition(ctx, cmd.Principal.UserID, cmd, now)

	// 任务引用：无任务时允许为空（收车也能求助），非空必须是本人当前任务。
	taskID := strings.TrimSpace(cmd.TaskID)
	if taskID != "" && s.tasks != nil {
		ids, err := s.tasks.ActiveTaskIDs(ctx, cmd.Principal.UserID)
		if err != nil {
			return nil, fmt.Errorf("load active tasks: %w", err)
		}
		if !contains(ids, taskID) {
			return nil, apperrors.InvalidArgument("TASK_NOT_OWNED", "task_id does not belong to the current driver")
		}
	}

	event := entity.SafetyEvent{
		EventID:  "SE:" + uuid.NewString(),
		DriverID: cmd.Principal.UserID,
		State:    entity.SafetyStateReceived,
		Version:  1,
		Raw: entity.SafetyRaw{
			EventType: cmd.EventType, Description: cmd.Description, TaskID: taskID,
			Position: position, ClientRequestID: cmd.ClientRequestID,
			RequestHash: reportHash(cmd, taskID, position), ReceivedAt: now,
		},
		Attachments: []string{},
		Supplements: []entity.SafetySupplement{},
		Actions:     []entity.SafetyAction{},
		Escalations: []entity.SafetyEscalation{},
		AuditTrail: []entity.SafetyAudit{{
			Seq: 1, Action: "report", ActorID: cmd.Principal.UserID,
			Role: cmd.Principal.Role, Detail: positionNote, At: now,
		}},
		// 通知意图与受理同一次写入：外部投递失败不影响受理本身。
		Outbox: []entity.SafetyOutbox{{
			ID: "1", Kind: entity.OutboxKindSafetyReported,
			Payload: reportPayload(cmd.EventType, cmd.Principal.UserID, position, now),
			State:   entity.OutboxPending,
		}},
		CreatedAt: now,
		UpdatedAt: now,
	}
	created, err := s.events.CreateReport(ctx, event)
	if err != nil {
		return nil, fmt.Errorf("create safety event: %w", err)
	}
	if !created {
		return s.replayByRequest(ctx, cmd.Principal.UserID, cmd.ClientRequestID, event.Raw.RequestHash)
	}
	return toSafetyView(&event), nil
}

// ByRequest 用于响应丢失后的恢复：有就是原事件，没有就是明确未受理，
// 不因为"第一次可能失败"而生成新的编号。
func (s *SafetyService) ByRequest(ctx context.Context, cmd dto.ByRequestCommand) (*dto.SafetyEventView, error) {
	if err := requireRole(cmd.Principal, RoleDriver); err != nil {
		return nil, err
	}
	if err := validateRequestID(cmd.ClientRequestID); err != nil {
		return nil, err
	}
	event, err := s.events.ByRequest(ctx, cmd.Principal.UserID, cmd.ClientRequestID)
	if err != nil {
		return nil, fmt.Errorf("load safety event: %w", err)
	}
	if event == nil {
		return nil, apperrors.NotFound("REPORT_NOT_RECEIVED",
			"no report was received for this client_request_id; retry with the same id")
	}
	return toSafetyView(event), nil
}

// AddSupplement 追加补充：定位与说明都只追加，不改冻结的原始内容。
func (s *SafetyService) AddSupplement(ctx context.Context, cmd dto.AddSupplementCommand) (*dto.SafetyEventView, error) {
	if err := requireRole(cmd.Principal, RoleDriver); err != nil {
		return nil, err
	}
	if cmd.Kind != "position" && cmd.Kind != "note" {
		return nil, apperrors.InvalidArgument("INVALID_SUPPLEMENT_KIND", "kind must be position or note")
	}
	event, err := s.ownEvent(ctx, cmd.Principal.UserID, cmd.EventID)
	if err != nil {
		return nil, err
	}
	if len(event.Supplements) >= entity.MaxSafetySupplements {
		return nil, apperrors.Conflict("TOO_MANY_SUPPLEMENTS",
			fmt.Sprintf("at most %d supplements per event", entity.MaxSafetySupplements))
	}
	now := s.clock.Now().UTC()
	position := supplementPosition(cmd, now)
	sup := entity.SafetySupplement{
		Seq: event.NextSupplementSeq(), Kind: cmd.Kind, Note: cmd.Note,
		Position: position, ActorID: cmd.Principal.UserID, At: now,
	}
	updated, err := s.events.AddSupplement(ctx, cmd.EventID, sup)
	if err != nil {
		return nil, fmt.Errorf("add supplement: %w", err)
	}
	if updated == nil {
		return nil, apperrors.NotFound("EVENT_NOT_FOUND", "safety event does not exist")
	}
	return toSafetyView(updated), nil
}

// AddAttachment 只绑定已经上传完成的文件，附件后补不影响受理。
func (s *SafetyService) AddAttachment(ctx context.Context, cmd dto.AddAttachmentCommand) (*dto.SafetyEventView, error) {
	if err := requireRole(cmd.Principal, RoleDriver); err != nil {
		return nil, err
	}
	if strings.TrimSpace(cmd.FileID) == "" {
		return nil, apperrors.InvalidArgument("INVALID_FILE", "file_id is required")
	}
	event, err := s.ownEvent(ctx, cmd.Principal.UserID, cmd.EventID)
	if err != nil {
		return nil, err
	}
	if len(event.Attachments) >= entity.MaxSafetyAttachments {
		return nil, apperrors.Conflict("TOO_MANY_ATTACHMENTS",
			fmt.Sprintf("at most %d attachments per event", entity.MaxSafetyAttachments))
	}
	if s.files != nil {
		rec, err := s.files.ByID(ctx, cmd.FileID)
		if err != nil {
			return nil, fmt.Errorf("load file: %w", err)
		}
		if rec == nil {
			return nil, apperrors.NotFound("FILE_NOT_FOUND", "file does not exist")
		}
		if rec.OwnerID != cmd.Principal.UserID {
			return nil, apperrors.Forbidden("FILE_NOT_OWNED", "file does not belong to the current driver")
		}
	}
	updated, err := s.events.AddAttachment(ctx, cmd.EventID, cmd.FileID)
	if err != nil {
		return nil, fmt.Errorf("add attachment: %w", err)
	}
	if updated == nil {
		return nil, apperrors.NotFound("EVENT_NOT_FOUND", "safety event does not exist")
	}
	return toSafetyView(updated), nil
}

// Hotline 返回受控已发布内容。它是指引，不是受理通道。
func (s *SafetyService) Hotline() *dto.HotlineView {
	items := make([]dto.HotlineItem, 0, len(entity.PublishedHotlines))
	for _, h := range entity.PublishedHotlines {
		items = append(items, dto.HotlineItem{Name: h.Name, Number: h.Number, Note: h.Note})
	}
	return &dto.HotlineView{
		Items: items, Version: entity.HotlineVersion,
		PublishedAt:      entity.HotlinePublishedAt,
		AutoDial:         false,
		Demo:             true,
		OfflineCacheable: true,
	}
}

// Pending 是后台可查询队列。
func (s *SafetyService) Pending(ctx context.Context, cmd dto.PendingEventsCommand) ([]dto.SafetyEventView, error) {
	if err := requireRole(cmd.Principal, RoleAuditor, RoleSafetyHandler); err != nil {
		return nil, err
	}
	limit := cmd.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	events, err := s.events.Pending(ctx, cmd.DriverID, cmd.State, limit)
	if err != nil {
		return nil, fmt.Errorf("list safety events: %w", err)
	}
	out := make([]dto.SafetyEventView, 0, len(events))
	for i := range events {
		out = append(out, *toSafetyView(&events[i]))
	}
	return out, nil
}

// DrainOutbox 投递通知意图。投递失败只累加尝试次数，不影响已受理的事件。
func (s *SafetyService) DrainOutbox(ctx context.Context, principal authn.Principal, limit int) (int32, int32, error) {
	if err := requireRole(principal, RoleAuditor, RoleSafetyHandler); err != nil {
		return 0, 0, err
	}
	now := s.clock.Now().UTC()
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	refs, err := s.events.PendingOutbox(ctx, limit)
	if err != nil {
		return 0, 0, fmt.Errorf("load pending outbox: %w", err)
	}
	var delivered, failed int32
	for _, ref := range refs {
		// eventId 唯一：重复投递不会给下游产生第二条相同事件。
		eventID := ref.EventID + ":" + ref.Entry.ID
		if err := s.logs.AppendEvent(ctx, eventID, ref.Entry.Kind, "safety", ref.Entry.Payload, now); err != nil {
			s.log.Error("deliver safety outbox failed", zap.String("entry", eventID), zap.Error(err))
			if err := s.events.BumpOutboxAttempt(ctx, ref.EventID, ref.Entry.ID); err != nil {
				s.log.Error("bump outbox attempt failed", zap.String("entry", eventID), zap.Error(err))
			}
			failed++
			continue
		}
		if err := s.events.MarkOutboxSent(ctx, ref.EventID, ref.Entry.ID, now); err != nil {
			s.log.Error("mark outbox sent failed", zap.String("entry", eventID), zap.Error(err))
			failed++
			continue
		}
		delivered++
	}
	return delivered, failed, nil
}

func (s *SafetyService) replayByRequest(ctx context.Context, driverID, requestID, hash string) (*dto.SafetyEventView, error) {
	existing, err := s.events.ByRequest(ctx, driverID, requestID)
	if err != nil {
		return nil, fmt.Errorf("load existing safety event: %w", err)
	}
	if existing == nil {
		// 撞唯一索引却读不到，说明并发受理尚未可见：宁可明确未确认，也不编造编号。
		return nil, apperrors.Conflict("REPORT_UNAVAILABLE", "the report is not confirmed as received")
	}
	if existing.Raw.RequestHash != hash {
		return nil, apperrors.Conflict("IDEMPOTENCY_CONFLICT",
			"client_request_id was used with different content")
	}
	return toSafetyView(existing), nil
}

func (s *SafetyService) ownEvent(ctx context.Context, driverID, eventID string) (*entity.SafetyEvent, error) {
	event, err := s.events.ByEventID(ctx, eventID)
	if err != nil {
		return nil, fmt.Errorf("load safety event: %w", err)
	}
	if event == nil || event.DriverID != driverID {
		// 不区分"不存在"与"不是本人的"，避免用 404 探测他人事件。
		return nil, apperrors.NotFound("EVENT_NOT_FOUND", "safety event does not exist")
	}
	return event, nil
}

// resolvePosition 决定位置证据的档位。0,0 与越界坐标一律降级为 UNKNOWN，
// 但绝不因此拒绝求助。
func (s *SafetyService) resolvePosition(
	ctx context.Context,
	driverID string,
	cmd dto.ReportEventCommand,
	now time.Time,
) (entity.SafetyPosition, string) {
	if cmd.HasPosition {
		if !inGeoRange(cmd.Lng, cmd.Lat) {
			return unknownPosition(), "position rejected: coordinates out of range"
		}
		if cmd.Lng == 0 && cmd.Lat == 0 {
			return unknownPosition(), "position rejected: zero coordinates are not evidence"
		}
		if cmd.CapturedAt != nil && positionFresh(*cmd.CapturedAt, now) {
			lng, lat := cmd.Lng, cmd.Lat
			capturedAt := cmd.CapturedAt.UTC()
			return entity.SafetyPosition{
				Kind: entity.PositionAvailable, Lng: &lng, Lat: &lat,
				CapturedAt: &capturedAt, Source: entity.PositionSourceClient,
			}, "position from client"
		}
	}
	if s.location != nil {
		lng, lat, at, err := s.location.LastKnownLocation(ctx, driverID)
		if err != nil {
			s.log.Error("load last known location failed", zap.String("driver_id", driverID), zap.Error(err))
		} else if at != nil && inGeoRange(lng, lat) && !(lng == 0 && lat == 0) {
			return entity.SafetyPosition{
				Kind: entity.PositionLastKnown, Lng: &lng, Lat: &lat,
				CapturedAt: at, Source: entity.PositionSourceLastKnown,
			}, "position from last known location"
		}
	}
	return unknownPosition(), "position unavailable"
}

func supplementPosition(cmd dto.AddSupplementCommand, now time.Time) entity.SafetyPosition {
	if !cmd.HasPosition || !inGeoRange(cmd.Lng, cmd.Lat) || (cmd.Lng == 0 && cmd.Lat == 0) {
		return unknownPosition()
	}
	lng, lat := cmd.Lng, cmd.Lat
	capturedAt := now
	if cmd.CapturedAt != nil {
		capturedAt = cmd.CapturedAt.UTC()
	}
	return entity.SafetyPosition{
		Kind: entity.PositionAvailable, Lng: &lng, Lat: &lat,
		CapturedAt: &capturedAt, Source: entity.PositionSourceClient,
	}
}

func unknownPosition() entity.SafetyPosition {
	return entity.SafetyPosition{Kind: entity.PositionUnknown, Source: entity.PositionSourceNone}
}

func inGeoRange(lng, lat float64) bool {
	return lng >= -180 && lng <= 180 && lat >= -90 && lat <= 90
}

func positionFresh(capturedAt, now time.Time) bool {
	return !capturedAt.Before(now.Add(-entity.PositionFreshWindow)) && !capturedAt.After(now.Add(time.Minute))
}

func toSafetyView(e *entity.SafetyEvent) *dto.SafetyEventView {
	receivedAt := e.Raw.ReceivedAt
	return &dto.SafetyEventView{
		EventID: e.EventID, DriverID: e.DriverID, State: e.State,
		Version: e.Version, Handler: e.Handler, ClaimedAt: e.ClaimedAt,
		EventType: e.Raw.EventType, Description: e.Raw.Description, TaskID: e.Raw.TaskID,
		Position: e.Raw.Position, ReceivedAt: &receivedAt,
		Attachments: e.Attachments, Supplements: len(e.Supplements),
		Actions: len(e.Actions), Escalations: len(e.Escalations),
	}
}

// reportHash 绑定内容与身份：身份参与哈希，避免同号内容被他人复用。
func reportHash(cmd dto.ReportEventCommand, taskID string, position entity.SafetyPosition) string {
	type snapshot struct {
		DriverID    string
		EventType   string
		Description string
		TaskID      string
		Kind        string
		Lng         *float64
		Lat         *float64
		CapturedAt  *time.Time
	}
	raw, err := json.Marshal(snapshot{
		DriverID: cmd.Principal.UserID, EventType: cmd.EventType,
		Description: cmd.Description, TaskID: taskID,
		Kind: position.Kind, Lng: position.Lng, Lat: position.Lat, CapturedAt: position.CapturedAt,
	})
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func reportPayload(eventType, driverID string, position entity.SafetyPosition, now time.Time) string {
	return fmt.Sprintf(
		`{"driver_id":%q,"event_type":%q,"position":%q,"received_at":%q}`,
		driverID, eventType, position.Kind, now.UTC().Format(time.RFC3339))
}

// validateRequestID 与请求号约定一致：1~128 个可打印 ASCII。
func validateRequestID(id string) error {
	if len(id) == 0 || len(id) > 128 {
		return apperrors.InvalidArgument("INVALID_REQUEST_ID", "client_request_id must be 1..128 characters")
	}
	for _, r := range id {
		if r < 32 || r > 126 {
			return apperrors.InvalidArgument("INVALID_REQUEST_ID", "client_request_id must be printable ASCII")
		}
	}
	return nil
}

func contains(values []string, target string) bool {
	for _, v := range values {
		if v == target {
			return true
		}
	}
	return false
}

// staffOnly 是后台处置角色的统一门禁。
func (s *SafetyService) staffOnly(p authn.Principal) error {
	return requireRole(p, RoleAuditor, RoleSafetyHandler)
}

// staffEvent 读取事件并校验后台角色；nil 事件按未找到返回。
func (s *SafetyService) staffEvent(ctx context.Context, principal authn.Principal, eventID string) (*entity.SafetyEvent, error) {
	if err := s.staffOnly(principal); err != nil {
		return nil, err
	}
	event, err := s.events.ByEventID(ctx, eventID)
	if err != nil {
		return nil, fmt.Errorf("load safety event: %w", err)
	}
	if event == nil {
		return nil, apperrors.NotFound("EVENT_NOT_FOUND", "safety event does not exist")
	}
	return event, nil
}

// versionConflict 统一返回版本/状态冲突：expected_version 条件更新未命中。
func versionConflict() error {
	return apperrors.Conflict("VERSION_CONFLICT", "event version or state changed; reload and retry")
}

func (s *SafetyService) newAction(event *entity.SafetyEvent, principal authn.Principal, typ, note, result, target, refID, requestID string, version int64, now time.Time) entity.SafetyAction {
	return entity.SafetyAction{
		Seq: event.NextActionSeq(), Type: typ, Note: note, Result: result,
		Target: target, RefID: refID, ActorID: principal.UserID,
		ActorRole: principal.Role, RequestID: requestID, Version: version, At: now,
	}
}

// Claim 领取事件：RECEIVED → CLAIMED。expected_version 条件更新，仅一个处理员成功。
func (s *SafetyService) Claim(ctx context.Context, cmd dto.SafetyClaimCommand) (*dto.SafetyEventView, error) {
	event, err := s.staffEvent(ctx, cmd.Principal, cmd.EventID)
	if err != nil {
		return nil, err
	}
	if cmd.ExpectedVersion <= 0 {
		return nil, apperrors.InvalidArgument("INVALID_VERSION", "expected_version is required")
	}
	now := s.clock.Now().UTC()
	action := s.newAction(event, cmd.Principal, entity.SafetyActionClaim, "", "", "", "", cmd.RequestID, cmd.ExpectedVersion+1, now)
	updated, err := s.events.Claim(ctx, cmd.EventID, cmd.ExpectedVersion, cmd.Principal.UserID, action, now)
	if err != nil {
		return nil, fmt.Errorf("claim safety event: %w", err)
	}
	if updated == nil {
		return nil, versionConflict()
	}
	return toSafetyView(updated), nil
}

// Start 实际开始处理：CLAIMED → PROCESSING。只有当前处理人才能推进。
func (s *SafetyService) Start(ctx context.Context, cmd dto.SafetyStartCommand) (*dto.SafetyEventView, error) {
	event, err := s.staffEvent(ctx, cmd.Principal, cmd.EventID)
	if err != nil {
		return nil, err
	}
	if cmd.ExpectedVersion <= 0 {
		return nil, apperrors.InvalidArgument("INVALID_VERSION", "expected_version is required")
	}
	now := s.clock.Now().UTC()
	action := s.newAction(event, cmd.Principal, entity.SafetyActionStart, "", "", "", "", cmd.RequestID, cmd.ExpectedVersion+1, now)
	updated, err := s.events.Start(ctx, cmd.EventID, cmd.ExpectedVersion, cmd.Principal.UserID, action, now)
	if err != nil {
		return nil, fmt.Errorf("start safety event: %w", err)
	}
	if updated == nil {
		return nil, versionConflict()
	}
	return toSafetyView(updated), nil
}

// Transfer 转派：写原因与交接动作，不改 state、不改 claimedAt。
func (s *SafetyService) Transfer(ctx context.Context, cmd dto.SafetyTransferCommand) (*dto.SafetyEventView, error) {
	event, err := s.staffEvent(ctx, cmd.Principal, cmd.EventID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(cmd.To) == "" {
		return nil, apperrors.InvalidArgument("INVALID_TRANSFER_TARGET", "to is required")
	}
	if strings.TrimSpace(cmd.Reason) == "" {
		return nil, apperrors.InvalidArgument("TRANSFER_REASON_REQUIRED", "transfer reason is required")
	}
	now := s.clock.Now().UTC()
	action := s.newAction(event, cmd.Principal, entity.SafetyActionTransfer, cmd.Reason, "", cmd.To, "", cmd.RequestID, cmd.ExpectedVersion+1, now)
	updated, err := s.events.Transfer(ctx, cmd.EventID, cmd.ExpectedVersion, cmd.Principal.UserID, cmd.To, action, now)
	if err != nil {
		return nil, fmt.Errorf("transfer safety event: %w", err)
	}
	if updated == nil {
		return nil, versionConflict()
	}
	return toSafetyView(updated), nil
}

// AddAction 追加处置/附证据：保持状态，只增不改。
func (s *SafetyService) AddAction(ctx context.Context, cmd dto.SafetyAddActionCommand) (*dto.SafetyEventView, error) {
	event, err := s.staffEvent(ctx, cmd.Principal, cmd.EventID)
	if err != nil {
		return nil, err
	}
	if cmd.Kind != entity.SafetyActionNote && cmd.Kind != entity.SafetyActionAttachEvidence {
		return nil, apperrors.InvalidArgument("INVALID_ACTION_KIND", "kind must be note or attach_evidence")
	}
	now := s.clock.Now().UTC()
	action := s.newAction(event, cmd.Principal, cmd.Kind, cmd.Note, "", "", cmd.RefID, cmd.RequestID, cmd.ExpectedVersion+1, now)
	updated, err := s.events.AddAction(ctx, cmd.EventID, cmd.ExpectedVersion, action, now)
	if err != nil {
		return nil, fmt.Errorf("add safety action: %w", err)
	}
	if updated == nil {
		return nil, versionConflict()
	}
	return toSafetyView(updated), nil
}

// Resolve 办结：必填结果与处置说明；不做任何自动关闭。
func (s *SafetyService) Resolve(ctx context.Context, cmd dto.SafetyResolveCommand) (*dto.SafetyEventView, error) {
	event, err := s.staffEvent(ctx, cmd.Principal, cmd.EventID)
	if err != nil {
		return nil, err
	}
	if !entity.IsKnownResolveResult(cmd.Result) {
		return nil, apperrors.InvalidArgument("INVALID_RESOLVE_RESULT", "result is not in the published dictionary")
	}
	if strings.TrimSpace(cmd.Note) == "" {
		return nil, apperrors.InvalidArgument("RESOLVE_NOTE_REQUIRED", "disposition note is required on resolve")
	}
	now := s.clock.Now().UTC()
	action := s.newAction(event, cmd.Principal, entity.SafetyActionResolve, cmd.Note, cmd.Result, "", "", cmd.RequestID, cmd.ExpectedVersion+1, now)
	updated, err := s.events.Resolve(ctx, cmd.EventID, cmd.ExpectedVersion, cmd.Principal.UserID, action, now)
	if err != nil {
		return nil, fmt.Errorf("resolve safety event: %w", err)
	}
	if updated == nil {
		return nil, versionConflict()
	}
	return toSafetyView(updated), nil
}

// GetEvidence 读取必要证据：仅本人是当前处理人；读取写审计。
func (s *SafetyService) GetEvidence(ctx context.Context, cmd dto.SafetyGetEvidenceCommand) (*dto.EvidenceView, error) {
	event, err := s.staffEvent(ctx, cmd.Principal, cmd.EventID)
	if err != nil {
		return nil, err
	}
	if event.Handler != cmd.Principal.UserID {
		return nil, apperrors.Forbidden("NOT_HANDLER", "only the current handler may read evidence")
	}
	now := s.clock.Now().UTC()
	view := &dto.EvidenceView{
		EventID: event.EventID, TaskID: event.Raw.TaskID,
		Recording: dto.RecordingView{Available: false, Note: "无录音证据"},
	}
	if event.Raw.TaskID != "" && s.evidence != nil {
		from := now.Add(-24 * time.Hour)
		view.Trace, _ = s.evidence.TracePoints(ctx, event.Raw.TaskID, from, now, 200)
		view.Messages, _ = s.evidence.Messages(ctx, event.Raw.TaskID, 100)
		view.VirtualCall, _ = s.evidence.VirtualCall(ctx, event.Raw.TaskID)
	}
	// 读取也审计：eventId 用纳秒时间戳，保证每次读取都留痕，不去重。
	auditID := fmt.Sprintf("%s:evidence:%s:%d", event.EventID, cmd.Principal.UserID, now.UnixNano())
	payload := fmt.Sprintf(`{"event_id":%q,"handler":%q,"task_id":%q}`, event.EventID, cmd.Principal.UserID, event.Raw.TaskID)
	if s.logs != nil {
		if err := s.logs.AppendEvent(ctx, auditID, "SafetyEvidenceRead", "safety", payload, now); err != nil {
			s.log.Error("audit evidence read failed", zap.String("event_id", event.EventID), zap.Error(err))
		}
	}
	return view, nil
}

// EscalateTick 扫描超过升级阈值仍未领取的事件。按 received_at 判定，
// 不依赖任何内存定时器：服务重启/重分配不重新计时。
func (s *SafetyService) EscalateTick(ctx context.Context, cmd dto.SafetyEscalateTickCommand) (dto.EscalateTickResult, error) {
	if err := s.staffOnly(cmd.Principal); err != nil {
		return dto.EscalateTickResult{}, err
	}
	now := cmd.Now
	if now.IsZero() {
		now = s.clock.Now().UTC()
	}
	limit := cmd.Limit
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	candidates, err := s.events.EscalateCandidates(ctx, now, limit)
	if err != nil {
		return dto.EscalateTickResult{}, fmt.Errorf("load escalation candidates: %w", err)
	}
	result := dto.EscalateTickResult{}
	for i := range candidates {
		event := &candidates[i]
		escalation := entity.SafetyEscalation{Level: entity.EscalationLevelUnclaimed, At: now}
		outbox := entity.SafetyOutbox{
			ID:   event.NextOutboxID(),
			Kind: entity.OutboxKindSafetyEscalated,
			Payload: fmt.Sprintf(`{"event_id":%q,"level":%d,"received_at":%q}`,
				event.EventID, entity.EscalationLevelUnclaimed, event.Raw.ReceivedAt.UTC().Format(time.RFC3339)),
			State: entity.OutboxPending,
		}
		ok, err := s.events.TryEscalate(ctx, event.EventID, escalation, outbox)
		if err != nil {
			s.log.Error("try escalate failed", zap.String("event_id", event.EventID), zap.Error(err))
			result.Failed++
			continue
		}
		if ok {
			result.Escalated++
		} else {
			result.Skipped++
		}
	}
	return result, nil
}
