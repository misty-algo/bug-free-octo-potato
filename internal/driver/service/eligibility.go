package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/BwCloudWeGo/bw-cli/internal/driver/dto"
	"github.com/BwCloudWeGo/bw-cli/internal/driver/entity"
	"github.com/BwCloudWeGo/bw-cli/pkg/clock"
	apperrors "github.com/BwCloudWeGo/bw-cli/pkg/errors"

	"github.com/google/uuid"
)

// eligibilitySources 是重算需要的权威事实来源，收窄便于单测替换。
type eligibilitySources interface {
	DocumentsBySubject(ctx context.Context, subjectID string) ([]entity.DriverDocument, error)
	ActiveBindingByDriver(ctx context.Context, driverID string) (*entity.VehicleBinding, error)
}

// EligibilityService 每次都从权威事实实时计算，没有投影表，
// 因此不存在"按旧缓存放行"的路径。
type EligibilityService struct {
	vehicles  eligibilitySources
	apps      entity.ApplicationRepository
	admission entity.AdmissionRepository
	dispatch  entity.DispatchRepository
	clock     clock.Clock
	log       *zap.Logger
}

func NewEligibilityService(
	vehicles eligibilitySources,
	apps entity.ApplicationRepository,
	admission entity.AdmissionRepository,
	dispatch entity.DispatchRepository,
	clk clock.Clock,
	log *zap.Logger,
) *EligibilityService {
	if log == nil {
		log = zap.NewNop()
	}
	return &EligibilityService{
		vehicles: vehicles, apps: apps, admission: admission,
		dispatch: dispatch, clock: clk, log: log,
	}
}

// Check 返回司机当前能否接新单，以及每一条限制的具体原因。
func (s *EligibilityService) Check(ctx context.Context, driverID string) (*dto.EligibilityView, error) {
	now := s.clock.Now().UTC()
	view := &dto.EligibilityView{
		DriverID:       driverID,
		Restrictions:   []dto.RestrictionView{},
		AllowedActions: []string{},
		BlockedActions: []string{},
	}

	docs, err := s.vehicles.DocumentsBySubject(ctx, driverID)
	if err != nil {
		return nil, fmt.Errorf("load documents: %w", err)
	}
	latest := make(map[string]entity.DriverDocument, len(docs))
	for _, doc := range docs {
		if doc.State != entity.DocStateActive {
			continue
		}
		if current, ok := latest[doc.DocType]; !ok || current.Version < doc.Version {
			latest[doc.DocType] = doc
		}
	}

	var validUntil *time.Time
	for _, docType := range requiredDocTypes {
		doc, ok := latest[docType]
		if !ok {
			view.Restrictions = append(view.Restrictions, dto.RestrictionView{
				Code: entity.RestrictionDocMissing, Source: entity.SourceDocType,
				TargetKey: docType, Detail: "required document is missing",
			})
			continue
		}
		expiry := doc.Expiry()
		if expiry == nil {
			view.Restrictions = append(view.Restrictions, dto.RestrictionView{
				Code: entity.RestrictionDocMissing, Source: entity.SourceDocType,
				TargetKey: docType, Detail: "document has no valid_until",
			})
			continue
		}
		if entity.IsExpired(now, *expiry) {
			view.Restrictions = append(view.Restrictions, dto.RestrictionView{
				Code: entity.RestrictionDocExpired, Source: entity.SourceDocType,
				TargetKey: docType, Detail: "document expired", Until: expiry,
			})
			continue
		}
		if validUntil == nil || expiry.Before(*validUntil) {
			validUntil = expiry
		}
	}
	view.ValidUntil = validUntil

	binding, err := s.vehicles.ActiveBindingByDriver(ctx, driverID)
	if err != nil {
		return nil, fmt.Errorf("load binding: %w", err)
	}
	if binding == nil {
		view.Restrictions = append(view.Restrictions, dto.RestrictionView{
			Code: entity.RestrictionBinding, Source: entity.SourceBinding,
			TargetKey: driverID, Detail: "no active vehicle binding",
		})
	}

	admission, err := s.admission.ByDriver(ctx, driverID)
	if err != nil {
		return nil, fmt.Errorf("load admission: %w", err)
	}
	if admission != nil {
		view.SourceRevision = admission.AdmissionVersion
	}

	view.CanAcceptNewTask = len(view.Restrictions) == 0
	if view.CanAcceptNewTask {
		view.AllowedActions = []string{entity.ActionAcceptTask, entity.ActionGoOnline}
	} else {
		view.BlockedActions = []string{entity.ActionAcceptTask, entity.ActionGoOnline}
	}
	return view, nil
}

// CanAcceptNewTask 是行程域收车判定使用的窄接口：
// 由权威事实实时计算，不接受任何缓存输入。
func (s *EligibilityService) CanAcceptNewTask(ctx context.Context, driverID string) (bool, error) {
	view, err := s.Check(ctx, driverID)
	if err != nil {
		return false, err
	}
	return view.CanAcceptNewTask, nil
}

// AssignTask 是"最终确认"的权威入口：与换车批准共用 driver_admission 锁，
// 锁内校验最新准入并把 source_revision 固化到任务上。
func (s *EligibilityService) AssignTask(ctx context.Context, cmd dto.AssignTaskCommand) (*dto.AssignTaskResult, error) {
	if err := requireRole(cmd.Principal, RoleSafetyHandler, RoleAuditor); err != nil {
		return nil, err
	}
	if strings.TrimSpace(cmd.DriverID) == "" {
		return nil, apperrors.InvalidArgument("INVALID_DRIVER", "driver_id is required")
	}
	now := s.clock.Now().UTC()
	taskNo := cmd.TaskNo
	if taskNo == "" {
		taskNo = "T:" + uuid.NewString()
	}
	lockKey := "assign:" + taskNo
	locked, err := s.admission.TryLock(ctx, cmd.DriverID, lockKey, now)
	if err != nil {
		return nil, fmt.Errorf("lock admission: %w", err)
	}
	if !locked {
		return nil, apperrors.Conflict("ACTIVE_TASK",
			"driver has an active task or another admission change is in progress")
	}
	release := func() { s.unlock(ctx, cmd.DriverID, lockKey) }

	view, err := s.Check(ctx, cmd.DriverID)
	if err != nil {
		release()
		return nil, err
	}
	if !view.CanAcceptNewTask {
		release()
		return nil, apperrors.Conflict("NOT_ELIGIBLE", "driver cannot accept new task: "+restrictionCodes(view))
	}
	if cmd.ExpectedSourceRevision > 0 && cmd.ExpectedSourceRevision != view.SourceRevision {
		release()
		return nil, apperrors.Conflict("ELIGIBILITY_STALE",
			fmt.Sprintf("eligibility changed: expected revision %d, current %d",
				cmd.ExpectedSourceRevision, view.SourceRevision))
	}

	taskID, err := s.dispatch.CreateAssignment(ctx, cmd.DriverID, cmd.OrderIDs, view.SourceRevision, taskNo, now)
	if err != nil {
		release()
		return nil, fmt.Errorf("create assignment: %w", err)
	}
	// 任务落地后登记活动任务，换车批准才会真正互斥。
	if _, err := s.admission.SetActiveTask(ctx, cmd.DriverID, taskID, now); err != nil {
		s.log.Error("set active task failed",
			zap.String("driver_id", cmd.DriverID), zap.String("task_id", taskID), zap.Error(err))
	}
	release()

	result := &dto.AssignTaskResult{
		TaskID: taskID, TaskNo: taskNo, SourceRevision: view.SourceRevision,
	}
	binding, err := s.vehicles.ActiveBindingByDriver(ctx, cmd.DriverID)
	if err != nil {
		s.log.Error("load binding for snapshot failed", zap.String("driver_id", cmd.DriverID), zap.Error(err))
		return result, nil
	}
	if binding != nil {
		result.VehicleID = binding.VehicleID
		if snap, err := s.bindingSnapshot(ctx, binding); err == nil {
			result.Snapshot = snap
		} else {
			s.log.Error("build binding snapshot failed", zap.String("driver_id", cmd.DriverID), zap.Error(err))
		}
	}
	return result, nil
}

func (s *EligibilityService) unlock(ctx context.Context, driverID, lockKey string) {
	if _, err := s.admission.Unlock(ctx, driverID, lockKey); err != nil {
		s.log.Error("unlock admission failed", zap.String("driver_id", driverID), zap.Error(err))
	}
}

func (s *EligibilityService) bindingSnapshot(ctx context.Context, binding *entity.VehicleBinding) (*entity.VehicleSnapshot, error) {
	snap := &entity.VehicleSnapshot{
		VehicleID: binding.VehicleID, RevisionID: binding.ActiveVehicleRevisionID,
		BindingVersion: binding.BindingVersion,
	}
	appID, rev, ok := splitRevisionRef(binding.ActiveVehicleRevisionID)
	if !ok || s.apps == nil {
		return snap, nil
	}
	app, err := s.apps.ByID(ctx, appID)
	if err != nil {
		return nil, err
	}
	if app == nil {
		return snap, nil
	}
	if r := app.RevisionByNumber(rev); r != nil {
		snap.Plate, snap.ModelCode, snap.Seats, snap.Color =
			r.Payload.Plate, r.Payload.ModelCode, r.Payload.Seats, r.Payload.Color
	}
	return snap, nil
}

func restrictionCodes(view *dto.EligibilityView) string {
	parts := make([]string, 0, len(view.Restrictions))
	for _, r := range view.Restrictions {
		parts = append(parts, r.Code+":"+r.TargetKey)
	}
	if len(parts) == 0 {
		return "unknown"
	}
	return strings.Join(parts, ",")
}
