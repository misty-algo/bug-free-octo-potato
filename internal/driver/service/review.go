package service

import (
	"context"
	"encoding/json"
	"errors"
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

// ReviewService 是审核域的受控命令入口。审核状态只能经由这里推进，
// 每次推进都带 revision + reviewVersion 双凭据并追加审计。
type ReviewService struct {
	apps      entity.ApplicationRepository
	counters  entity.RejectionCounterRepository
	admission entity.AdmissionRepository
	tasks     entity.TaskReader
	vehicles  entity.VehicleRepository
	files     entity.FileRepository
	events    entity.EventWriter
	clock     clock.Clock
	log       *zap.Logger
}

func NewReviewService(
	apps entity.ApplicationRepository,
	counters entity.RejectionCounterRepository,
	admission entity.AdmissionRepository,
	tasks entity.TaskReader,
	vehicles entity.VehicleRepository,
	files entity.FileRepository,
	events entity.EventWriter,
	clk clock.Clock,
	log *zap.Logger,
) *ReviewService {
	return &ReviewService{
		apps: apps, counters: counters, admission: admission, tasks: tasks,
		vehicles: vehicles, files: files, events: events, clock: clk, log: log,
	}
}

// Submit 冻结草稿正文：首次创建申请，或在其被驳回/撤回后追加新 revision。
// 被驳回的版本永不修改，重新提交只会追加。
func (s *ReviewService) Submit(ctx context.Context, cmd dto.SubmitApplicationCommand) (*dto.ApplicationResult, error) {
	if err := requireRole(cmd.Principal, RoleDriver); err != nil {
		return nil, err
	}
	appID := strings.TrimSpace(cmd.ApplicationID)
	if appID == "" {
		return nil, apperrors.InvalidArgument("INVALID_REQUEST", "application_id is required")
	}
	driverID := cmd.Principal.UserID
	now := s.clock.Now().UTC()

	// 活动变更优先判断：重复提交应得到"在审中"而不是"草稿不存在"。
	active, err := s.apps.ActiveByDriver(ctx, driverID)
	if err != nil {
		return nil, err
	}
	if active != nil {
		return nil, apperrors.Conflict("APPLICATION_IN_REVIEW", "another vehicle change is under review")
	}
	draft, err := s.vehicles.DraftByDriver(ctx, driverID)
	if err != nil {
		return nil, err
	}
	if draft == nil {
		return nil, apperrors.NotFound("DRAFT_NOT_FOUND", "no vehicle draft to submit")
	}
	if cmd.ExpectedDraftVersion != draft.Version {
		return nil, apperrors.Conflict("VERSION_MISMATCH",
			fmt.Sprintf("expected draft version %d, got %d", draft.Version, cmd.ExpectedDraftVersion))
	}
	docs, err := s.vehicles.DocumentsBySubject(ctx, driverID)
	if err != nil {
		return nil, err
	}
	documentIDs, missing := checkRequiredDocuments(docs, now)
	if len(missing) > 0 {
		return nil, apperrors.InvalidArgument("DRIVER_DOC_INCOMPLETE",
			"missing or expired documents: "+strings.Join(missing, ","))
	}
	if len(draft.FileIDs) == 0 {
		return nil, apperrors.InvalidArgument("FILE_REQUIRED", "at least one material file is required")
	}
	if err := ensureFilesUsable(ctx, s.files, driverID, draft.FileIDs); err != nil {
		return nil, err
	}
	payload := entity.RevisionPayload{
		VehicleID: draft.VehicleID, VinMask: draft.VinMask, Plate: draft.Plate,
		CityCode: draft.CityCode, ModelCode: draft.ModelCode, Seats: draft.Seats, Color: draft.Color,
		DocumentIDs: documentIDs, FileIDs: draft.FileIDs,
		DraftID: draft.DraftID, DraftVersion: draft.Version,
	}
	digest := entity.PayloadDigest(payload)

	latest, err := s.apps.LatestByDriver(ctx, driverID)
	if err != nil {
		return nil, err
	}
	if latest != nil && latest.ApplicationID == appID {
		if latest.State != entity.AppStateRejected && latest.State != entity.AppStateWithdrawn {
			return nil, apperrors.Conflict("APPLICATION_ALREADY_DECIDED",
				"application is not open for a new revision")
		}
		return s.appendRevision(ctx, driverID, appID, payload, digest, latest,
			cmd.ExpectedRevision, cmd.ExpectedReviewVersion, now)
	}
	// 被驳回或撤回的申请必须沿用原 application_id 追加 revision，
	// 不允许另起新申请绕过历史驳回。批准后才允许发起新的续期/换车申请。
	if latest != nil && (latest.State == entity.AppStateRejected || latest.State == entity.AppStateWithdrawn) {
		return nil, apperrors.Conflict("USE_EXISTING_APPLICATION",
			"rejected or withdrawn application must be resubmitted with application_id "+latest.ApplicationID)
	}
	return s.createApplication(ctx, driverID, appID, payload, digest, now)
}

func (s *ReviewService) createApplication(ctx context.Context, driverID, appID string, payload entity.RevisionPayload, digest string, now time.Time) (*dto.ApplicationResult, error) {
	// 内嵌数组必须初始化为空数组而不是 null：后续 $push/$addToSet 只能作用于数组。
	rev := entity.ApplicationRevision{
		Revision: 1, PayloadDigest: digest, Payload: payload, State: entity.AppStatePending,
		Decisions: []entity.Decision{}, Assignments: []entity.Assignment{},
		CreatedAt: now, UpdatedAt: now,
	}
	entry := entity.AuditEntry{
		Seq: 1, Action: entity.ActionSubmit, ActorID: driverID, ActorRole: RoleDriver,
		ReviewVersion: 0, Detail: "revision 1 submitted", At: now,
	}
	out, err := outboxEvent(entity.OutboxKindApplication, map[string]any{
		"applicationId": appID, "driverId": driverID, "revision": 1, "state": entity.AppStatePending,
	}, now)
	if err != nil {
		return nil, err
	}
	app := &entity.VehicleApplication{
		ApplicationID: appID, DriverID: driverID, VehicleID: payload.VehicleID,
		State: entity.AppStatePending, ActiveRevision: 1, Revision: 1, ReviewVersion: 0,
		ActiveSlot: driverID, Revisions: []entity.ApplicationRevision{rev},
		AuditTrail: []entity.AuditEntry{entry}, ReviewFlags: []string{},
		Outbox:    []entity.OutboxEntry{out},
		CreatedAt: now, UpdatedAt: now,
	}
	attachedTo := attachmentRef(appID, 1)
	if err := s.attachFiles(ctx, driverID, attachedTo, payload.FileIDs); err != nil {
		return nil, err
	}
	if err := s.apps.Create(ctx, app); err != nil {
		s.releaseFiles(ctx, attachedTo)
		if errors.Is(err, entity.ErrDuplicateKey) {
			return nil, apperrors.Conflict("APPLICATION_IN_REVIEW",
				"another application is already active for this driver, or the application id is taken")
		}
		return nil, err
	}
	s.dropDraft(ctx, driverID)
	return applicationResult(app, &app.Revisions[0]), nil
}

func (s *ReviewService) appendRevision(ctx context.Context, driverID, appID string, payload entity.RevisionPayload, digest string, latest *entity.VehicleApplication, expectedRevision, expectedReviewVersion int64, now time.Time) (*dto.ApplicationResult, error) {
	if latest.Revision >= entity.MaxRevisionsPerApplication {
		return nil, apperrors.Conflict("TOO_MANY_REVISIONS",
			fmt.Sprintf("at most %d revisions per application", entity.MaxRevisionsPerApplication))
	}
	// 新提交与审批、撤回共同校验当前 revision 与审核代数。
	if latest.Revision != expectedRevision {
		return nil, apperrors.Conflict("STALE_REVISION",
			fmt.Sprintf("expected revision %d, got %d", latest.Revision, expectedRevision))
	}
	if latest.ReviewVersion != expectedReviewVersion {
		return nil, apperrors.Conflict("REVIEW_VERSION_MISMATCH",
			fmt.Sprintf("expected review version %d, got %d", latest.ReviewVersion, expectedReviewVersion))
	}
	next := latest.Revision + 1
	rev := entity.ApplicationRevision{
		Revision: next, PayloadDigest: digest, Payload: payload, State: entity.AppStatePending,
		Decisions: []entity.Decision{}, Assignments: []entity.Assignment{},
		CreatedAt: now, UpdatedAt: now,
	}
	entry := entity.AuditEntry{
		Seq: latest.NextAuditSeq(), Action: entity.ActionSubmit, ActorID: driverID, ActorRole: RoleDriver,
		ReviewVersion: 0, Detail: fmt.Sprintf("revision %d submitted", next), At: now,
	}
	out, err := outboxEvent(entity.OutboxKindApplication, map[string]any{
		"applicationId": appID, "driverId": driverID, "revision": next, "state": entity.AppStatePending,
	}, now)
	if err != nil {
		return nil, err
	}
	attachedTo := attachmentRef(appID, next)
	if err := s.attachFiles(ctx, driverID, attachedTo, payload.FileIDs); err != nil {
		return nil, err
	}
	ok, err := s.apps.AppendRevision(ctx, entity.AppendRevisionInput{
		ApplicationID: appID, DriverID: driverID,
		ExpectedRevision: latest.Revision, ExpectedReviewVersion: latest.ReviewVersion,
		Revision: rev, Entry: entry, Outbox: []entity.OutboxEntry{out},
	})
	if err != nil {
		s.releaseFiles(ctx, attachedTo)
		return nil, err
	}
	if !ok {
		s.releaseFiles(ctx, attachedTo)
		return nil, apperrors.Conflict("REVIEW_VERSION_MISMATCH",
			"application changed while resubmitting; reload before trying again")
	}
	s.dropDraft(ctx, driverID)
	result := &dto.ApplicationResult{
		ApplicationID: appID, Revision: next, ActiveRevision: next, State: entity.AppStatePending,
		VehicleID: payload.VehicleID, FileIDs: payload.FileIDs, DocumentIDs: payload.DocumentIDs,
		ReviewVersion: 0, PayloadDigest: digest, Flags: latest.ReviewFlags,
	}
	return result, nil
}

// ClaimApplication 是领取命令：并发下只有一个审核员抢到。
func (s *ReviewService) ClaimApplication(ctx context.Context, cmd dto.ReviewCommand) (*dto.ApplicationResult, error) {
	if err := requireRole(cmd.Principal, RoleAuditor, RoleSafetyHandler); err != nil {
		return nil, err
	}
	now := s.clock.Now().UTC()
	app, err := s.apps.ByID(ctx, cmd.ApplicationID)
	if err != nil {
		return nil, err
	}
	if app == nil {
		return nil, apperrors.NotFound("APPLICATION_NOT_FOUND", "application not found")
	}
	if app.ActiveRevision != cmd.Revision {
		return nil, apperrors.Conflict("STALE_REVISION", "this revision is no longer active")
	}
	if app.State != entity.AppStatePending {
		return nil, apperrors.Conflict("ALREADY_CLAIMED", "application is not pending")
	}
	if app.ReviewVersion != cmd.ExpectedReviewVersion {
		return nil, apperrors.Conflict("REVIEW_VERSION_MISMATCH",
			fmt.Sprintf("expected review version %d, got %d", app.ReviewVersion, cmd.ExpectedReviewVersion))
	}
	if app.DriverID == cmd.Principal.UserID {
		return nil, apperrors.Forbidden("FORBIDDEN", "submitter cannot review own materials")
	}
	entry := entity.AuditEntry{
		Seq: app.NextAuditSeq(), Action: entity.ActionClaim, ActorID: cmd.Principal.UserID,
		ActorRole: cmd.Principal.Role, ReviewVersion: cmd.ExpectedReviewVersion + 1, At: now,
	}
	out, err := outboxEvent(entity.OutboxKindApplication, map[string]any{
		"applicationId": app.ApplicationID, "revision": app.ActiveRevision, "state": entity.AppStateReviewing,
		"reviewerId": cmd.Principal.UserID,
	}, now)
	if err != nil {
		return nil, err
	}
	ok, err := s.apps.Claim(ctx, entity.TransitionInput{
		ApplicationID: app.ApplicationID, Revision: app.ActiveRevision,
		ReviewVersion: cmd.ExpectedReviewVersion, ActorID: cmd.Principal.UserID,
		ActorRole: cmd.Principal.Role, At: now, Entry: entry, Outbox: []entity.OutboxEntry{out},
	})
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, apperrors.Conflict("ALREADY_CLAIMED", "another reviewer already claimed this revision")
	}
	app.State = entity.AppStateReviewing
	app.ReviewerID = cmd.Principal.UserID
	app.ReviewVersion = cmd.ExpectedReviewVersion + 1
	app.AuditTrail = append(app.AuditTrail, entry)
	if rev := app.ActiveRevisionView(); rev != nil {
		rev.State = entity.AppStateReviewing
	}
	return applicationResult(app, app.ActiveRevisionView()), nil
}

// Transfer 转派：只追加分配记录与原因，不伪造另一次裁决。
func (s *ReviewService) Transfer(ctx context.Context, cmd dto.TransferCommand) (*dto.ApplicationResult, error) {
	if err := requireRole(cmd.Principal, RoleAuditor, RoleSafetyHandler); err != nil {
		return nil, err
	}
	to := strings.TrimSpace(cmd.ToReviewerID)
	if to == "" {
		return nil, apperrors.InvalidArgument("INVALID_REQUEST", "to_reviewer_id is required")
	}
	if to == cmd.Principal.UserID {
		return nil, apperrors.InvalidArgument("INVALID_REQUEST", "cannot transfer to yourself")
	}
	reason := strings.TrimSpace(cmd.Reason)
	if reason == "" {
		return nil, apperrors.InvalidArgument("TRANSFER_REASON_REQUIRED", "transfer reason is required")
	}
	now := s.clock.Now().UTC()
	app, err := s.apps.ByID(ctx, cmd.ApplicationID)
	if err != nil {
		return nil, err
	}
	if app == nil {
		return nil, apperrors.NotFound("APPLICATION_NOT_FOUND", "application not found")
	}
	if err := assertReviewer(app, cmd.ReviewCommand); err != nil {
		return nil, err
	}
	entry := entity.AuditEntry{
		Seq: app.NextAuditSeq(), Action: entity.ActionTransfer, ActorID: cmd.Principal.UserID,
		ActorRole: cmd.Principal.Role, ReviewVersion: cmd.ExpectedReviewVersion + 1,
		Detail: "transfer to " + to + ": " + reason, At: now,
	}
	out, err := outboxEvent(entity.OutboxKindApplication, map[string]any{
		"applicationId": app.ApplicationID, "revision": app.ActiveRevision,
		"state": entity.AppStateReviewing, "reviewerId": to,
	}, now)
	if err != nil {
		return nil, err
	}
	ok, err := s.apps.Transfer(ctx, entity.TransitionInput{
		ApplicationID: app.ApplicationID, Revision: app.ActiveRevision,
		ReviewVersion: cmd.ExpectedReviewVersion, ActorID: cmd.Principal.UserID,
		ActorRole: cmd.Principal.Role, At: now, Entry: entry, Outbox: []entity.OutboxEntry{out},
	}, to, reason)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, apperrors.Conflict("REVIEW_VERSION_MISMATCH", "assignment changed; reload before transferring")
	}
	app.ReviewerID = to
	app.ReviewVersion = cmd.ExpectedReviewVersion + 1
	return applicationResult(app, app.ActiveRevisionView()), nil
}

// WithdrawApplication 仅限本人当前 pending/reviewing 版本。
func (s *ReviewService) WithdrawApplication(ctx context.Context, cmd dto.WithdrawCommand) (*dto.ApplicationResult, error) {
	if err := requireRole(cmd.Principal, RoleDriver); err != nil {
		return nil, err
	}
	now := s.clock.Now().UTC()
	app, err := s.apps.ByID(ctx, cmd.ApplicationID)
	if err != nil {
		return nil, err
	}
	if app == nil {
		return nil, apperrors.NotFound("APPLICATION_NOT_FOUND", "application not found")
	}
	if app.DriverID != cmd.Principal.UserID {
		return nil, apperrors.Forbidden("FORBIDDEN", "driver may only withdraw own application")
	}
	if app.ActiveRevision != cmd.Revision {
		return nil, apperrors.Conflict("STALE_REVISION", "this revision is no longer active")
	}
	if !app.IsActive() {
		return nil, apperrors.Conflict("NOT_WITHDRAWABLE", "application already reached a final state")
	}
	if app.ReviewVersion != cmd.ExpectedReviewVersion {
		return nil, apperrors.Conflict("REVIEW_VERSION_MISMATCH",
			fmt.Sprintf("expected review version %d, got %d", app.ReviewVersion, cmd.ExpectedReviewVersion))
	}
	entry := entity.AuditEntry{
		Seq: app.NextAuditSeq(), Action: entity.ActionWithdraw, ActorID: cmd.Principal.UserID,
		ActorRole: RoleDriver, ReviewVersion: cmd.ExpectedReviewVersion,
		Detail: "withdrawn by driver", At: now,
	}
	out, err := outboxEvent(entity.OutboxKindApplication, map[string]any{
		"applicationId": app.ApplicationID, "revision": app.ActiveRevision, "state": entity.AppStateWithdrawn,
	}, now)
	if err != nil {
		return nil, err
	}
	ok, err := s.apps.Withdraw(ctx, entity.TransitionInput{
		ApplicationID: app.ApplicationID, Revision: app.ActiveRevision,
		ReviewVersion: cmd.ExpectedReviewVersion, ActorID: cmd.Principal.UserID,
		ActorRole: RoleDriver, At: now, Entry: entry, Outbox: []entity.OutboxEntry{out},
	})
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, apperrors.Conflict("NOT_WITHDRAWABLE", "application changed; reload before withdrawing")
	}
	app.State = entity.AppStateWithdrawn
	app.ActiveSlot = ""
	if rev := app.ActiveRevisionView(); rev != nil {
		rev.State = entity.AppStateWithdrawn
	}
	return applicationResult(app, app.ActiveRevisionView()), nil
}

// RejectApplication 逐条记录原因并一次终态裁决；不通过的包不部分替换有效版本。
func (s *ReviewService) RejectApplication(ctx context.Context, cmd dto.RejectCommand) (*dto.ApplicationResult, error) {
	if err := requireRole(cmd.Principal, RoleAuditor, RoleSafetyHandler); err != nil {
		return nil, err
	}
	items, err := normalizeRejectItems(cmd.Items)
	if err != nil {
		return nil, err
	}
	now := s.clock.Now().UTC()
	app, err := s.apps.ByID(ctx, cmd.ApplicationID)
	if err != nil {
		return nil, err
	}
	if app == nil {
		return nil, apperrors.NotFound("APPLICATION_NOT_FOUND", "application not found")
	}
	if err := assertReviewer(app, cmd.ReviewCommand); err != nil {
		return nil, err
	}
	flags := s.repeatFlags(ctx, app.DriverID, items)
	entry := entity.AuditEntry{
		Seq: app.NextAuditSeq(), Action: entity.ActionReject, ActorID: cmd.Principal.UserID,
		ActorRole: cmd.Principal.Role, ReviewVersion: cmd.ExpectedReviewVersion + 1,
		Detail: "rejected: " + summarizeItems(items), At: now,
	}
	decision := entity.Decision{
		Action: entity.ActionReject, ActorID: cmd.Principal.UserID,
		ReviewVersion: cmd.ExpectedReviewVersion + 1, Summary: strings.TrimSpace(cmd.Summary),
		Items: items, At: now,
	}
	outbox, err := s.decisionOutbox(app, entity.AppStateRejected, items, now)
	if err != nil {
		return nil, err
	}
	ok, err := s.apps.Decide(ctx, entity.DecideInput{
		TransitionInput: entity.TransitionInput{
			ApplicationID: app.ApplicationID, Revision: app.ActiveRevision,
			ReviewVersion: cmd.ExpectedReviewVersion, ActorID: cmd.Principal.UserID,
			ActorRole: cmd.Principal.Role, At: now, Entry: entry, Outbox: outbox,
		},
		ToState: entity.AppStateRejected, Decision: decision, Flags: flags,
	})
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, apperrors.Conflict("AUDIT_ALREADY_DECIDED",
			"this revision already reached a final state or the reviewer changed")
	}
	// 计数是派生数据：裁决已生效，失败时由 COUNTER_SYNC outbox 事件幂等重放。
	if err := s.counters.ApplyReject(ctx, app.DriverID, items, app.ApplicationID, app.ActiveRevision, now); err != nil {
		s.log.Error("apply rejection counter failed; outbox will replay",
			zap.String("application_id", app.ApplicationID),
			zap.Int64("revision", app.ActiveRevision), zap.Error(err))
	}
	app.State = entity.AppStateRejected
	app.ActiveSlot = ""
	app.ReviewVersion = cmd.ExpectedReviewVersion + 1
	app.ReviewFlags = append(app.ReviewFlags, flags...)
	if rev := app.ActiveRevisionView(); rev != nil {
		rev.State = entity.AppStateRejected
		rev.Decisions = append(rev.Decisions, decision)
	}
	return applicationResult(app, app.ActiveRevisionView()), nil
}

// ApproveApplication 在批准前重新检查有效期、必填资料、绑定唯一性与任务占用，
// 全部通过才切换有效版本并广播 QualificationChanged。
func (s *ReviewService) ApproveApplication(ctx context.Context, cmd dto.ReviewCommand) (*dto.ApplicationResult, error) {
	if err := requireRole(cmd.Principal, RoleAuditor, RoleSafetyHandler); err != nil {
		return nil, err
	}
	now := s.clock.Now().UTC()
	app, err := s.apps.ByID(ctx, cmd.ApplicationID)
	if err != nil {
		return nil, err
	}
	if app == nil {
		return nil, apperrors.NotFound("APPLICATION_NOT_FOUND", "application not found")
	}
	if err := assertReviewer(app, cmd); err != nil {
		return nil, err
	}
	rev := app.ActiveRevisionView()
	if rev == nil {
		return nil, apperrors.Conflict("STALE_REVISION", "active revision is missing")
	}
	if _, err := s.recheckDocuments(ctx, app.DriverID, rev.Payload.DocumentIDs, now); err != nil {
		return nil, err
	}

	previous, err := s.vehicles.ActiveBindingByDriver(ctx, app.DriverID)
	if err != nil {
		return nil, err
	}
	changingVehicle := previous == nil || previous.VehicleID != rev.Payload.VehicleID
	lockKey := fmt.Sprintf("review:%s:%d", app.ApplicationID, rev.Revision)
	if changingVehicle {
		locked, err := s.admission.TryLock(ctx, app.DriverID, lockKey, now)
		if err != nil {
			return nil, err
		}
		if !locked {
			return nil, apperrors.Conflict("ACTIVE_TASK",
				"driver has an active task or another admission change is in progress")
		}
		has, err := s.tasks.HasActiveTask(ctx, app.DriverID)
		if err != nil {
			s.unlock(ctx, app.DriverID, lockKey)
			return nil, fmt.Errorf("check active task: %w", err)
		}
		if has {
			s.unlock(ctx, app.DriverID, lockKey)
			return nil, apperrors.Conflict("ACTIVE_TASK",
				"driver has an active task; approve again after the task finishes")
		}
	}

	var previousCopy *entity.VehicleBinding
	if previous != nil {
		cp := *previous
		previousCopy = &cp
	}
	bindingVersion, qualificationVersion := int64(1), int64(1)
	if previous != nil {
		bindingVersion = previous.BindingVersion + 1
		qualificationVersion = previous.QualificationVersion + 1
		// ArchiveBinding 会修改传入对象的状态，必须传副本。
		if err := s.vehicles.ArchiveBinding(ctx, previousCopy); err != nil {
			s.unlockIf(ctx, changingVehicle, app.DriverID, lockKey)
			return nil, err
		}
	}
	revisionID := fmt.Sprintf("%s:%d", app.ApplicationID, rev.Revision)
	binding := &entity.VehicleBinding{
		BindingID: "VB:" + uuid.NewString(), DriverID: app.DriverID, VehicleID: rev.Payload.VehicleID,
		ActiveVehicleRevisionID: revisionID, QualificationVersion: qualificationVersion,
		BindingVersion: bindingVersion, State: entity.BindingStateActive, CreatedAt: now,
	}
	if err := s.vehicles.SaveBinding(ctx, binding); err != nil {
		s.restoreBinding(ctx, app.DriverID, bindingsToRestore{bindingVersion: bindingVersion, previous: previousCopy})
		s.unlockIf(ctx, changingVehicle, app.DriverID, lockKey)
		if errors.Is(err, entity.ErrDuplicateKey) {
			return nil, apperrors.Conflict("VEHICLE_ALREADY_BOUND", "this vehicle is bound to another driver")
		}
		return nil, err
	}

	decision := entity.Decision{
		Action: entity.ActionApprove, ActorID: cmd.Principal.UserID,
		ReviewVersion: cmd.ExpectedReviewVersion + 1, Summary: "approved", At: now,
	}
	outbox, err := s.decisionOutbox(app, entity.AppStateApproved, nil, now)
	if err != nil {
		s.restoreBinding(ctx, app.DriverID, bindingsToRestore{bindingVersion: bindingVersion, previous: previousCopy})
		s.unlockIf(ctx, changingVehicle, app.DriverID, lockKey)
		return nil, err
	}
	entry := entity.AuditEntry{
		Seq: app.NextAuditSeq(), Action: entity.ActionApprove, ActorID: cmd.Principal.UserID,
		ActorRole: cmd.Principal.Role, ReviewVersion: cmd.ExpectedReviewVersion + 1,
		Detail: fmt.Sprintf("approved revision %d, binding v%d", rev.Revision, bindingVersion), At: now,
	}
	ok, err := s.apps.Decide(ctx, entity.DecideInput{
		TransitionInput: entity.TransitionInput{
			ApplicationID: app.ApplicationID, Revision: app.ActiveRevision,
			ReviewVersion: cmd.ExpectedReviewVersion, ActorID: cmd.Principal.UserID,
			ActorRole: cmd.Principal.Role, At: now, Entry: entry, Outbox: outbox,
		},
		ToState: entity.AppStateApproved, Decision: decision,
	})
	if err != nil || !ok {
		// 与撤回/其他裁决竞争失败：回退已写入的绑定，保持"旧版本仍有效"。
		s.restoreBinding(ctx, app.DriverID, bindingsToRestore{bindingVersion: bindingVersion, previous: previousCopy})
		s.unlockIf(ctx, changingVehicle, app.DriverID, lockKey)
		if err != nil {
			return nil, err
		}
		return nil, apperrors.Conflict("AUDIT_ALREADY_DECIDED",
			"this revision already reached a final state or the reviewer changed")
	}

	if changingVehicle {
		if _, err := s.admission.BumpVersion(ctx, app.DriverID, now); err != nil {
			s.log.Error("bump admission version failed",
				zap.String("driver_id", app.DriverID), zap.Error(err))
		}
	}
	s.unlock(ctx, app.DriverID, lockKey)
	// 车辆档案只在批准后刷新：它是车辆主数据，不能被未批准的提交改写。
	if err := s.upsertProfile(ctx, rev.Payload, now); err != nil {
		s.log.Error("refresh vehicle profile failed",
			zap.String("vehicle_id", rev.Payload.VehicleID), zap.Error(err))
	}
	snapshot := entity.VehicleSnapshot{
		VehicleID: rev.Payload.VehicleID, RevisionID: revisionID, Plate: rev.Payload.Plate,
		ModelCode: rev.Payload.ModelCode, Seats: rev.Payload.Seats, Color: rev.Payload.Color,
		BindingVersion: bindingVersion,
	}
	// drivers.vehicle 只是投影；真相在 vehicle_bindings。
	if err := s.vehicles.UpdateDriverVehicleProjection(ctx, app.DriverID, snapshot); err != nil {
		s.log.Error("refresh driver vehicle projection failed",
			zap.String("driver_id", app.DriverID), zap.Error(err))
	}
	s.clearCounters(ctx, app.DriverID, rev.Payload, app.ApplicationID, rev.Revision, now)

	app.State = entity.AppStateApproved
	app.ActiveSlot = ""
	app.ReviewVersion = cmd.ExpectedReviewVersion + 1
	if rev := app.ActiveRevisionView(); rev != nil {
		rev.State = entity.AppStateApproved
		rev.Decisions = append(rev.Decisions, decision)
	}
	return applicationResult(app, app.ActiveRevisionView()), nil
}

// ApplicationDetail 返回申请全历史：被驳回的 revision 与逐条原因都可查。
func (s *ReviewService) ApplicationDetail(ctx context.Context, cmd dto.ApplicationDetailCommand) (*dto.ApplicationDetail, error) {
	if strings.TrimSpace(cmd.Principal.UserID) == "" {
		return nil, apperrors.Unauthorized("UNAUTHENTICATED", "a valid principal is required")
	}
	var (
		app *entity.VehicleApplication
		err error
	)
	if id := strings.TrimSpace(cmd.ApplicationID); id != "" {
		app, err = s.apps.ByID(ctx, id)
	} else {
		app, err = s.apps.LatestByDriver(ctx, cmd.Principal.UserID)
	}
	if err != nil {
		return nil, err
	}
	if app == nil {
		return nil, apperrors.NotFound("APPLICATION_NOT_FOUND", "no application found")
	}
	if !s.canRead(cmd.Principal, app) {
		return nil, apperrors.Forbidden("FORBIDDEN", "not allowed to read this application")
	}
	return toApplicationDetail(app), nil
}

// PendingAudits 供审核工作台按状态拉取；未指派与"只看自己"分别用 pending/mine。
func (s *ReviewService) PendingAudits(ctx context.Context, principal authn.Principal, state string) ([]dto.PendingAuditItem, error) {
	if err := requireRole(principal, RoleAuditor, RoleSafetyHandler); err != nil {
		return nil, err
	}
	states := []string{entity.AppStatePending}
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "", "pending":
		states = []string{entity.AppStatePending}
	case "reviewing", "mine":
		states = []string{entity.AppStateReviewing}
	default:
		return nil, apperrors.InvalidArgument("INVALID_REQUEST", "state must be pending, reviewing or mine")
	}
	apps, err := s.apps.ListByState(ctx, states)
	if err != nil {
		return nil, err
	}
	mine := strings.EqualFold(strings.TrimSpace(state), "mine")
	items := make([]dto.PendingAuditItem, 0, len(apps))
	for _, app := range apps {
		if mine && app.ReviewerID != principal.UserID {
			continue
		}
		rev := app.ActiveRevisionView()
		if rev == nil {
			continue
		}
		items = append(items, dto.PendingAuditItem{
			ApplicationID: app.ApplicationID, Revision: app.ActiveRevision, DriverID: app.DriverID,
			VehicleID: app.VehicleID, Plate: rev.Payload.Plate, FileIDs: rev.Payload.FileIDs,
			CreatedAt: rev.CreatedAt, ReviewVersion: app.ReviewVersion, ReviewerID: app.ReviewerID,
			State: app.State,
		})
	}
	return items, nil
}

// DrainOutbox 投递内嵌事件：成功置 sent，失败累计尝试次数，下轮重试。
func (s *ReviewService) DrainOutbox(ctx context.Context, principal authn.Principal, limit int64) (*dto.OutboxDrainResult, error) {
	if err := requireRole(principal, RoleAuditor, RoleSafetyHandler); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	refs, err := s.apps.PendingOutbox(ctx, limit)
	if err != nil {
		return nil, err
	}
	result := &dto.OutboxDrainResult{}
	now := s.clock.Now().UTC()
	for _, ref := range refs {
		if err := s.deliver(ctx, ref, now); err != nil {
			s.log.Error("deliver outbox event failed",
				zap.String("application_id", ref.ApplicationID),
				zap.String("outbox_id", ref.Entry.ID), zap.Error(err))
			if err := s.apps.BumpOutboxAttempt(ctx, ref.ApplicationID, ref.Entry.ID); err != nil {
				s.log.Error("bump outbox attempt failed",
					zap.String("outbox_id", ref.Entry.ID), zap.Error(err))
			}
			result.Failed++
			continue
		}
		if _, err := s.apps.MarkOutboxSent(ctx, ref.ApplicationID, ref.Entry.ID, now); err != nil {
			s.log.Error("mark outbox sent failed",
				zap.String("outbox_id", ref.Entry.ID), zap.Error(err))
			result.Failed++
			continue
		}
		result.Delivered++
	}
	return result, nil
}

func (s *ReviewService) deliver(ctx context.Context, ref entity.OutboxRef, now time.Time) error {
	if ref.Entry.Kind == entity.OutboxKindCounterSync {
		var payload counterSyncPayload
		if err := json.Unmarshal([]byte(ref.Entry.Payload), &payload); err != nil {
			return fmt.Errorf("decode counter sync payload: %w", err)
		}
		return s.counters.ApplyReject(ctx, payload.DriverID, payload.Items,
			payload.ApplicationID, payload.Revision, now)
	}
	return s.events.AppendEvent(ctx, ref.ApplicationID+":"+ref.Entry.ID, ref.Entry.Kind, "driver",
		ref.Entry.Payload, now)
}

type counterSyncPayload struct {
	DriverID      string              `json:"driverId"`
	ApplicationID string              `json:"applicationId"`
	Revision      int64               `json:"revision"`
	Items         []entity.RejectItem `json:"items"`
}

type bindingsToRestore struct {
	bindingVersion int64
	previous       *entity.VehicleBinding
}

// restoreBinding 回退失败的批准：删掉新绑定、恢复旧绑定与历史行。
func (s *ReviewService) restoreBinding(ctx context.Context, driverID string, in bindingsToRestore) {
	if err := s.vehicles.DeleteBinding(ctx, driverID, in.bindingVersion); err != nil {
		s.log.Error("delete uncommitted binding failed",
			zap.String("driver_id", driverID), zap.Int64("binding_version", in.bindingVersion), zap.Error(err))
	}
	if in.previous == nil {
		return
	}
	if err := s.vehicles.DeleteHistory(ctx, driverID, in.previous.BindingVersion); err != nil {
		s.log.Error("delete archived binding history failed",
			zap.String("driver_id", driverID), zap.Int64("binding_version", in.previous.BindingVersion), zap.Error(err))
	}
	restore := *in.previous
	restore.State = entity.BindingStateActive
	if err := s.vehicles.SaveBinding(ctx, &restore); err != nil {
		s.log.Error("restore previous binding failed",
			zap.String("driver_id", driverID), zap.Int64("binding_version", restore.BindingVersion), zap.Error(err))
	}
}

func (s *ReviewService) unlock(ctx context.Context, driverID, lockKey string) {
	if _, err := s.admission.Unlock(ctx, driverID, lockKey); err != nil {
		s.log.Error("release admission lock failed",
			zap.String("driver_id", driverID), zap.String("lock", lockKey), zap.Error(err))
	}
}

func (s *ReviewService) unlockIf(ctx context.Context, locked bool, driverID, lockKey string) {
	if locked {
		s.unlock(ctx, driverID, lockKey)
	}
}

func (s *ReviewService) clearCounters(ctx context.Context, driverID string, payload entity.RevisionPayload, appID string, revision int64, now time.Time) {
	docs, err := s.vehicles.DocumentsByIDs(ctx, payload.DocumentIDs)
	if err != nil {
		s.log.Error("load documents for counter clear failed",
			zap.String("driver_id", driverID), zap.Error(err))
		return
	}
	seen := map[string]bool{}
	for _, doc := range docs {
		if seen[doc.DocType] {
			continue
		}
		seen[doc.DocType] = true
		if err := s.counters.Clear(ctx, driverID, entity.CounterTargetDoc, doc.DocType, appID, revision, now); err != nil {
			s.log.Error("clear document rejection counter failed",
				zap.String("driver_id", driverID), zap.String("doc_type", doc.DocType), zap.Error(err))
		}
	}
	if err := s.counters.Clear(ctx, driverID, entity.CounterTargetVehicle, payload.VehicleID, appID, revision, now); err != nil {
		s.log.Error("clear vehicle rejection counter failed",
			zap.String("driver_id", driverID), zap.String("vehicle_id", payload.VehicleID), zap.Error(err))
	}
}

// repeatFlags 计算本次驳回后是否触发重点复核标记。读取失败不阻断裁决，
// 由 outbox 重放计数后仍会在下一次驳回时体现。
func (s *ReviewService) repeatFlags(ctx context.Context, driverID string, items []entity.RejectItem) []string {
	existing, err := s.counters.ByDriver(ctx, driverID)
	if err != nil {
		s.log.Error("load rejection counters failed", zap.String("driver_id", driverID), zap.Error(err))
		return nil
	}
	index := make(map[string]int, len(existing))
	for _, c := range existing {
		index[c.TargetType+"|"+c.TargetKey] = c.Consecutive
	}
	seen := map[string]bool{}
	for _, item := range items {
		key := item.TargetType + "|" + item.TargetKey
		if seen[key] {
			continue
		}
		seen[key] = true
		if index[key]+1 >= entity.RepeatRejectThreshold {
			return []string{entity.RejectFlagRepeat}
		}
	}
	return nil
}

func (s *ReviewService) recheckDocuments(ctx context.Context, driverID string, ids []string, now time.Time) ([]entity.DriverDocument, error) {
	if len(ids) == 0 {
		return nil, apperrors.InvalidArgument("DRIVER_DOC_INCOMPLETE", "no documents were submitted")
	}
	docs, err := s.vehicles.DocumentsByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	if len(docs) != len(ids) {
		return nil, apperrors.InvalidArgument("DRIVER_DOC_INCOMPLETE", "some submitted documents no longer exist")
	}
	valid := make(map[string]bool, len(docs))
	var missing []string
	for _, doc := range docs {
		if doc.State != entity.DocStateActive {
			missing = append(missing, doc.DocType+"(archived)")
			continue
		}
		if expiry := doc.Expiry(); expiry != nil && entity.IsExpired(now, *expiry) {
			missing = append(missing, doc.DocType+"(expired)")
			continue
		}
		valid[doc.DocType] = true
	}
	for _, docType := range requiredDocTypes {
		if !valid[docType] {
			missing = append(missing, docType)
		}
	}
	if len(missing) > 0 {
		return nil, apperrors.InvalidArgument("DRIVER_DOC_INCOMPLETE",
			"documents are incomplete or expired: "+strings.Join(dedupe(missing), ","))
	}
	return docs, nil
}

func (s *ReviewService) attachFiles(ctx context.Context, ownerID, attachedTo string, ids []string) error {
	attached, err := s.files.Attach(ctx, ownerID, attachedTo, ids)
	if err != nil {
		return err
	}
	if attached != int64(len(ids)) {
		s.releaseFiles(ctx, attachedTo)
		return apperrors.Conflict("FILE_STALE", "some files were reclaimed by cleanup; please upload them again")
	}
	return nil
}

func (s *ReviewService) releaseFiles(ctx context.Context, attachedTo string) {
	if _, err := s.files.Release(ctx, attachedTo); err != nil {
		s.log.Error("release bound files failed", zap.String("attached_to", attachedTo), zap.Error(err))
	}
}

func (s *ReviewService) upsertProfile(ctx context.Context, payload entity.RevisionPayload, now time.Time) error {
	return s.vehicles.UpsertProfile(ctx, &entity.VehicleProfile{
		VehicleID: payload.VehicleID, VinMask: payload.VinMask, Plate: payload.Plate,
		CityCode: payload.CityCode, ModelCode: payload.ModelCode, Seats: payload.Seats,
		Color: payload.Color, CreatedAt: now, UpdatedAt: now,
	})
}

func (s *ReviewService) dropDraft(ctx context.Context, driverID string) {
	if err := s.vehicles.DeleteDraft(ctx, driverID); err != nil {
		s.log.Error("delete submitted draft failed", zap.String("driver_id", driverID), zap.Error(err))
	}
}

func (s *ReviewService) canRead(principal authn.Principal, app *entity.VehicleApplication) bool {
	switch {
	case strings.EqualFold(principal.Role, RoleDriver):
		return app.DriverID == principal.UserID
	case strings.EqualFold(principal.Role, RoleAuditor), strings.EqualFold(principal.Role, RoleSafetyHandler):
		return true
	default:
		return false
	}
}

func (s *ReviewService) decisionOutbox(app *entity.VehicleApplication, state string, items []entity.RejectItem, now time.Time) ([]entity.OutboxEntry, error) {
	changed, err := outboxEvent(entity.OutboxKindApplication, map[string]any{
		"applicationId": app.ApplicationID, "driverId": app.DriverID,
		"revision": app.ActiveRevision, "state": state,
	}, now)
	if err != nil {
		return nil, err
	}
	out := []entity.OutboxEntry{changed}
	switch state {
	case entity.AppStateApproved:
		qual, err := outboxEvent(entity.OutboxKindQualification, map[string]any{
			"applicationId": app.ApplicationID, "driverId": app.DriverID,
			"vehicleId": app.VehicleID, "revision": app.ActiveRevision,
		}, now)
		if err != nil {
			return nil, err
		}
		out = append(out, qual)
	case entity.AppStateRejected:
		sync, err := outboxEvent(entity.OutboxKindCounterSync, counterSyncPayload{
			DriverID: app.DriverID, ApplicationID: app.ApplicationID,
			Revision: app.ActiveRevision, Items: items,
		}, now)
		if err != nil {
			return nil, err
		}
		out = append(out, sync)
	}
	return out, nil
}

func assertReviewer(app *entity.VehicleApplication, cmd dto.ReviewCommand) error {
	if app.ActiveRevision != cmd.Revision {
		return apperrors.Conflict("STALE_REVISION", "this revision is no longer active")
	}
	if app.State != entity.AppStateReviewing {
		return apperrors.Conflict("AUDIT_ALREADY_DECIDED", "revision is not under review")
	}
	if app.ReviewerID != cmd.Principal.UserID {
		return apperrors.Forbidden("NOT_ASSIGNEE", "only the assigned reviewer may decide")
	}
	if app.ReviewVersion != cmd.ExpectedReviewVersion {
		return apperrors.Conflict("REVIEW_VERSION_MISMATCH",
			fmt.Sprintf("expected review version %d, got %d", app.ReviewVersion, cmd.ExpectedReviewVersion))
	}
	return nil
}

func normalizeRejectItems(in []dto.RejectItemInput) ([]entity.RejectItem, error) {
	if len(in) == 0 {
		return nil, apperrors.InvalidArgument("REJECT_REASON_REQUIRED",
			"at least one per-item reason is required")
	}
	out := make([]entity.RejectItem, 0, len(in))
	for _, item := range in {
		targetType := strings.TrimSpace(item.TargetType)
		if targetType != entity.CounterTargetDoc && targetType != entity.CounterTargetVehicle {
			return nil, apperrors.InvalidArgument("INVALID_REQUEST",
				"target_type must be doc_type or vehicle")
		}
		targetKey := strings.TrimSpace(item.TargetKey)
		if targetKey == "" {
			return nil, apperrors.InvalidArgument("INVALID_REQUEST", "target_key is required")
		}
		code := strings.TrimSpace(item.Code)
		if code == "" {
			return nil, apperrors.InvalidArgument("REJECT_REASON_REQUIRED", "reason code is required")
		}
		reason := strings.TrimSpace(item.Reason)
		if reason == "" {
			return nil, apperrors.InvalidArgument("REJECT_REASON_REQUIRED", "reason text is required")
		}
		out = append(out, entity.RejectItem{
			TargetType: targetType, TargetKey: targetKey, Code: code, Reason: reason,
		})
	}
	return out, nil
}

func summarizeItems(items []entity.RejectItem) string {
	parts := make([]string, 0, len(items))
	for _, item := range items {
		parts = append(parts, item.TargetKey+":"+item.Code)
	}
	return strings.Join(parts, ",")
}

func attachmentRef(applicationID string, revision int64) string {
	return fmt.Sprintf("%s:%d", applicationID, revision)
}

func dedupe(values []string) []string {
	seen := make(map[string]bool, len(values))
	out := make([]string, 0, len(values))
	for _, v := range values {
		if seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}

func outboxEvent(kind string, payload any, now time.Time) (entity.OutboxEntry, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return entity.OutboxEntry{}, fmt.Errorf("encode outbox payload: %w", err)
	}
	return entity.OutboxEntry{
		ID: "OB:" + uuid.NewString(), Kind: kind, State: entity.OutboxPending,
		Payload: string(raw), CreatedAt: now,
	}, nil
}

func applicationResult(app *entity.VehicleApplication, rev *entity.ApplicationRevision) *dto.ApplicationResult {
	result := &dto.ApplicationResult{
		ApplicationID: app.ApplicationID, Revision: app.Revision, ActiveRevision: app.ActiveRevision,
		State: app.State, VehicleID: app.VehicleID, ReviewVersion: app.ReviewVersion,
		ReviewerID: app.ReviewerID, Flags: app.ReviewFlags,
	}
	if rev != nil {
		result.VehicleID = rev.Payload.VehicleID
		result.FileIDs = rev.Payload.FileIDs
		result.DocumentIDs = rev.Payload.DocumentIDs
		result.PayloadDigest = rev.PayloadDigest
	}
	return result
}

func toApplicationDetail(app *entity.VehicleApplication) *dto.ApplicationDetail {
	detail := &dto.ApplicationDetail{
		ApplicationID: app.ApplicationID, DriverID: app.DriverID, VehicleID: app.VehicleID,
		State: app.State, ActiveRevision: app.ActiveRevision, Revision: app.Revision,
		ReviewVersion: app.ReviewVersion, ReviewerID: app.ReviewerID,
		ReviewFlags: app.ReviewFlags,
	}
	for _, rev := range app.Revisions {
		view := dto.RevisionView{
			Revision: rev.Revision, State: rev.State, PayloadDigest: rev.PayloadDigest,
			VehicleID: rev.Payload.VehicleID, VinMask: rev.Payload.VinMask, Plate: rev.Payload.Plate,
			CityCode: rev.Payload.CityCode, ModelCode: rev.Payload.ModelCode, Seats: rev.Payload.Seats,
			Color: rev.Payload.Color, FileIDs: rev.Payload.FileIDs, DocumentIDs: rev.Payload.DocumentIDs,
			CreatedAt: rev.CreatedAt,
		}
		for _, d := range rev.Decisions {
			dv := dto.DecisionView{
				Action: d.Action, ActorID: d.ActorID, ReviewVersion: d.ReviewVersion,
				Summary: d.Summary, At: d.At,
			}
			for _, item := range d.Items {
				dv.Items = append(dv.Items, dto.RejectItemView{
					TargetType: item.TargetType, TargetKey: item.TargetKey,
					Code: item.Code, Reason: item.Reason,
				})
			}
			view.Decisions = append(view.Decisions, dv)
		}
		for _, a := range rev.Assignments {
			view.Assignments = append(view.Assignments, dto.AssignmentView{
				Action: a.Action, FromID: a.FromID, ToID: a.ToID,
				Reason: a.Reason, ReviewVersion: a.ReviewVersion, At: a.At,
			})
		}
		detail.Revisions = append(detail.Revisions, view)
	}
	return detail
}
