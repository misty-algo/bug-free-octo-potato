package service

import (
	"context"
	"fmt"
	"strconv"
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

// 城市与车型字典。真实取值应来自 A18 发布配置（config_dictionary），
// 这里是待接 A18 的受控常量，服务端字典校验逻辑本身不变。
var (
	allowedCities = map[string]bool{"shanghai": true, "beijing": true, "hangzhou": true}
	// modelCode -> 最大核载。车型与核载矛盾在此被拒。
	allowedModels = map[string]int{"compact": 4, "sedan": 4, "business": 5, "suv": 7, "mpv": 7}
	knownDocTypes = map[string]bool{
		entity.DocTypeDrivingLicense:    true,
		entity.DocTypeVehicleLicense:    true,
		entity.DocTypeInsurance:         true,
		entity.DocTypeRideHailingPermit: true,
	}
	// requiredDocTypes 是必填集合。即使已上传若干证件，也逐项校验完整性。
	requiredDocTypes = []string{
		entity.DocTypeDrivingLicense,
		entity.DocTypeVehicleLicense,
		entity.DocTypeInsurance,
		entity.DocTypeRideHailingPermit,
	}
)

// VehicleService 提供草稿、证件与有效绑定的读写能力。
// 提交与审核命令在 ReviewService，二者共用同一套仓储与文件状态机。
type VehicleService struct {
	repo      entity.VehicleRepository
	files     entity.FileRepository
	apps      entity.ApplicationRepository
	clock     clock.Clock
	secret    string
	log       *zap.Logger
	reminders DocumentReminder
}

func NewVehicleService(repo entity.VehicleRepository, files entity.FileRepository, apps entity.ApplicationRepository, clk clock.Clock, secret string) *VehicleService {
	return &VehicleService{
		repo: repo, files: files, apps: apps, clock: clk, secret: secret,
		log: zap.NewNop(),
	}
}

// WithReminders 注入提醒依赖。为 nil 时证件变更不生成提醒任务，
// 提醒失败也不应影响证件保存本身。
func (s *VehicleService) WithReminders(log *zap.Logger, reminders DocumentReminder) *VehicleService {
	if log != nil {
		s.log = log
	}
	s.reminders = reminders
	return s
}

// SaveDraft 建立或更新车辆草稿。影响准入的字段只进草稿，提交前不改当前有效车。
func (s *VehicleService) SaveDraft(ctx context.Context, cmd dto.SaveDraftCommand) (*dto.DraftResult, error) {
	if err := requireRole(cmd.Principal, RoleDriver); err != nil {
		return nil, err
	}
	now := s.clock.Now().UTC()
	plate := entity.NormalizePlate(cmd.Plate)
	if plate == "" {
		return nil, apperrors.InvalidArgument("INVALID_PLATE", "plate is required")
	}
	if strings.TrimSpace(cmd.VIN) == "" {
		return nil, apperrors.InvalidArgument("INVALID_VIN", "vehicle identification is required")
	}
	if !allowedCities[cmd.CityCode] {
		return nil, apperrors.InvalidArgument("INVALID_CITY", "city is not in the published dictionary")
	}
	maxSeats, ok := allowedModels[cmd.ModelCode]
	if !ok {
		return nil, apperrors.InvalidArgument("INVALID_MODEL", "model is not in the published dictionary")
	}
	if cmd.Seats <= 0 || cmd.Seats > maxSeats {
		return nil, apperrors.InvalidArgument("SEATS_MODEL_MISMATCH", fmt.Sprintf("seats must be 1..%d for %s", maxSeats, cmd.ModelCode))
	}
	// 同一司机只允许一个活动变更；活动期间不能再开新的草稿流程。
	if s.apps != nil {
		active, err := s.apps.ActiveByDriver(ctx, cmd.Principal.UserID)
		if err != nil {
			return nil, err
		}
		if active != nil {
			return nil, apperrors.Conflict("APPLICATION_IN_REVIEW", "another vehicle change is under review")
		}
	}
	if len(cmd.FileIDs) > entity.MaxFilesPerRevision {
		return nil, apperrors.InvalidArgument("TOO_MANY_FILES", fmt.Sprintf("at most %d files per revision", entity.MaxFilesPerRevision))
	}
	draft, err := s.repo.DraftByDriver(ctx, cmd.Principal.UserID)
	if err != nil {
		return nil, err
	}
	switch {
	case draft == nil && cmd.ExpectedDraftVersion != 0:
		return nil, apperrors.Conflict("VERSION_MISMATCH", "no draft exists; expected_draft_version must be 0")
	case draft != nil && cmd.ExpectedDraftVersion != draft.Version:
		return nil, apperrors.Conflict("VERSION_MISMATCH", fmt.Sprintf("expected draft version %d, got %d", draft.Version, cmd.ExpectedDraftVersion))
	}
	if draft == nil {
		draft = &entity.VehicleDraft{DraftID: "VD:" + uuid.NewString(), DriverID: cmd.Principal.UserID, Version: 0}
	}
	if len(cmd.FileIDs) > 0 {
		if err := ensureFilesUsable(ctx, s.files, cmd.Principal.UserID, cmd.FileIDs); err != nil {
			return nil, err
		}
	}
	draft.VehicleID = entity.DeriveVehicleID(s.secret, cmd.VIN)
	draft.VinMask = entity.MaskVIN(cmd.VIN)
	draft.Plate = plate
	draft.CityCode = cmd.CityCode
	draft.ModelCode = cmd.ModelCode
	draft.Seats = cmd.Seats
	draft.Color = cmd.Color
	draft.FileIDs = cmd.FileIDs
	draft.State = entity.DraftStateDraft
	draft.UpdatedAt = now
	// SaveDraft 以 expected version 为条件更新并自增，这里的写入是原子的。
	if err := s.repo.SaveDraft(ctx, draft); err != nil {
		return nil, err
	}
	return &dto.DraftResult{
		DraftID: draft.DraftID, Version: draft.Version + 1, VehicleID: draft.VehicleID, VinMask: draft.VinMask,
		Plate: draft.Plate, CityCode: draft.CityCode, ModelCode: draft.ModelCode, Seats: draft.Seats,
		Color: draft.Color, FileIDs: draft.FileIDs,
	}, nil
}

// SaveDocument 是受权编辑命令：只接受完整证件号，绝不把掩码当完整值回写。
func (s *VehicleService) SaveDocument(ctx context.Context, cmd dto.SaveDocumentCommand) (*dto.DocumentItem, error) {
	if err := requireRole(cmd.Principal, RoleDriver); err != nil {
		return nil, err
	}
	now := s.clock.Now().UTC()
	if !knownDocTypes[cmd.DocType] {
		return nil, apperrors.InvalidArgument("INVALID_DOC_TYPE", "unknown document type")
	}
	if strings.TrimSpace(cmd.DocNo) == "" {
		return nil, apperrors.InvalidArgument("INVALID_DOC_NO", "document number is required")
	}
	if entity.LooksMasked(cmd.DocNo) {
		return nil, apperrors.InvalidArgument("MASKED_DOC_NO", "masked value cannot be stored as a full document number")
	}
	expiresAt, expiresOn, issuedAt, err := documentValidity(cmd)
	if err != nil {
		return nil, err
	}
	if expiresAt != nil && entity.IsExpired(now, *expiresAt) {
		return nil, apperrors.InvalidArgument("DOC_EXPIRED", "document is already expired")
	}
	if expiresAt != nil && issuedAt != nil && expiresAt.Before(*issuedAt) {
		return nil, apperrors.InvalidArgument("INVALID_DOC_RANGE", "valid_until must not precede issued_at")
	}
	if len(cmd.FileIDs) > entity.MaxFilesPerRevision {
		return nil, apperrors.InvalidArgument("TOO_MANY_FILES", fmt.Sprintf("at most %d files per document", entity.MaxFilesPerRevision))
	}
	if len(cmd.FileIDs) > 0 {
		if err := ensureFilesUsable(ctx, s.files, cmd.Principal.UserID, cmd.FileIDs); err != nil {
			return nil, err
		}
	}
	active, err := s.repo.ActiveDocument(ctx, cmd.Principal.UserID, cmd.DocType)
	if err != nil {
		return nil, err
	}
	version := int64(1)
	if active != nil {
		if cmd.ExpectedVersion != active.Version {
			return nil, apperrors.Conflict("VERSION_MISMATCH", fmt.Sprintf("expected document version %d, got %d", active.Version, cmd.ExpectedVersion))
		}
		version = active.Version + 1
		if err := s.repo.ArchiveDocument(ctx, cmd.Principal.UserID, cmd.DocType); err != nil {
			return nil, err
		}
	}
	doc := &entity.DriverDocument{
		DocumentID: "DOC:" + uuid.NewString(), SubjectType: "driver", SubjectID: cmd.Principal.UserID,
		DocType: cmd.DocType, DocNoMask: entity.MaskDocNo(cmd.DocNo),
		IssuedAt: issuedAt, ExpiresAt: expiresAt, ExpiresOn: expiresOn,
		// validUntil 只作旧字段镜像，判定一律走 ExpiresAt / Expiry()。
		ValidUntil: expiresAt, FileIDs: cmd.FileIDs,
		Version: version, State: entity.DocStateActive, UpdatedAt: now,
	}
	if err := s.repo.SaveDocument(ctx, doc); err != nil {
		return nil, err
	}
	if s.reminders != nil {
		// 新证生效后旧版本的未发提醒立即失效，避免给已作废的证件发提醒。
		if err := s.reminders.EnsureForDocument(ctx, doc, now); err != nil {
			s.log.Error("ensure reminders failed",
				zap.String("document_id", doc.DocumentID), zap.Error(err))
		}
	}
	return &dto.DocumentItem{
		DocumentID: doc.DocumentID, DocType: doc.DocType, DocNoMask: doc.DocNoMask,
		IssuedAt: doc.IssuedAt, ValidUntil: doc.ValidUntil, ValidUntilDate: doc.ExpiresOn,
		ExpiresAt: doc.ExpiresAt, FileIDs: doc.FileIDs,
		Version: doc.Version, State: doc.State,
	}, nil
}

// documentValidity 统一解析证件有效期：valid_until_date 优先，
// 否则把旧秒级口径归一到自然日边界，保证内部只有一种失效语义。
func documentValidity(cmd dto.SaveDocumentCommand) (expiresAt *time.Time, expiresOn string, issuedAt *time.Time, err error) {
	issuedAt = cmd.IssuedAt
	if cmd.ValidUntilDate != "" {
		parsed, perr := entity.ExpiresAtFromDate(cmd.ValidUntilDate)
		if perr != nil {
			return nil, "", issuedAt, apperrors.InvalidArgument("INVALID_VALID_UNTIL_DATE", "valid_until_date must be YYYY-MM-DD")
		}
		return &parsed, entity.LastValidDate(parsed), issuedAt, nil
	}
	if cmd.ValidUntil != nil {
		normalized := entity.NormalizeLegacyValidUntil(*cmd.ValidUntil)
		return &normalized, entity.LastValidDate(normalized), issuedAt, nil
	}
	return nil, "", issuedAt, nil
}

// CurrentBinding 返回司机本人当前有效绑定与历史绑定。
func (s *VehicleService) CurrentBinding(ctx context.Context, principal authn.Principal) (*dto.BindingView, error) {
	if err := requireRole(principal, RoleDriver); err != nil {
		return nil, err
	}
	active, err := s.repo.ActiveBindingByDriver(ctx, principal.UserID)
	if err != nil {
		return nil, err
	}
	history, err := s.repo.HistoryByDriver(ctx, principal.UserID)
	if err != nil {
		return nil, err
	}
	view := &dto.BindingView{}
	if active != nil {
		snapshot := entity.VehicleSnapshot{
			VehicleID: active.VehicleID, RevisionID: active.ActiveVehicleRevisionID,
			BindingVersion: active.BindingVersion,
		}
		// 车辆快照只能来自被批准并冻结的那一版 revision：
		// 未批准的草稿/提交改动绝不能出现在当前有效版本视图里。
		if s.apps != nil {
			if appID, rev, ok := splitRevisionRef(active.ActiveVehicleRevisionID); ok {
				app, err := s.apps.ByID(ctx, appID)
				if err != nil {
					return nil, err
				}
				if app != nil {
					if r := app.RevisionByNumber(rev); r != nil {
						snapshot.Plate, snapshot.ModelCode, snapshot.Seats, snapshot.Color =
							r.Payload.Plate, r.Payload.ModelCode, r.Payload.Seats, r.Payload.Color
					}
				}
			}
		}
		view.Binding = &dto.BindingResult{
			DriverID: active.DriverID, VehicleID: active.VehicleID,
			ActiveVehicleRevisionID: active.ActiveVehicleRevisionID,
			QualificationVersion:    active.QualificationVersion,
			BindingVersion:          active.BindingVersion, State: active.State, Snapshot: snapshot,
		}
		view.Snapshot = &snapshot
	}
	for _, b := range history {
		view.History = append(view.History, dto.BindingResult{
			DriverID: b.DriverID, VehicleID: b.VehicleID,
			ActiveVehicleRevisionID: b.ActiveVehicleRevisionID,
			QualificationVersion:    b.QualificationVersion,
			BindingVersion:          b.BindingVersion, State: b.State,
		})
	}
	return view, nil
}

// BindingSnapshot 供派单域在分配任务时锁定车辆版本：
// 写入订单快照后，续证、改车或注销都不再改写旧订单。
func (s *VehicleService) BindingSnapshot(ctx context.Context, principal authn.Principal, driverID string) (*entity.VehicleSnapshot, error) {
	if err := requireRole(principal, RoleDriver, RoleAuditor); err != nil {
		return nil, err
	}
	if strings.EqualFold(principal.Role, RoleDriver) && driverID != principal.UserID {
		return nil, apperrors.Forbidden("FORBIDDEN", "driver may only read own binding")
	}
	active, err := s.repo.ActiveBindingByDriver(ctx, driverID)
	if err != nil {
		return nil, err
	}
	if active == nil {
		return nil, apperrors.NotFound("BINDING_NOT_FOUND", "no active vehicle binding")
	}
	snapshot := &entity.VehicleSnapshot{
		VehicleID: active.VehicleID, RevisionID: active.ActiveVehicleRevisionID,
		BindingVersion: active.BindingVersion,
	}
	// 派单写入订单的快照必须是被批准的那一版，而不是车辆档案的最新值。
	if s.apps != nil {
		if appID, rev, ok := splitRevisionRef(active.ActiveVehicleRevisionID); ok {
			app, err := s.apps.ByID(ctx, appID)
			if err != nil {
				return nil, err
			}
			if app != nil {
				if r := app.RevisionByNumber(rev); r != nil {
					snapshot.Plate, snapshot.ModelCode, snapshot.Seats, snapshot.Color =
						r.Payload.Plate, r.Payload.ModelCode, r.Payload.Seats, r.Payload.Color
				}
			}
		}
	}
	return snapshot, nil
}

// splitRevisionRef 拆分 "applicationID:revision"。
func splitRevisionRef(ref string) (string, int64, bool) {
	idx := strings.LastIndex(ref, ":")
	if idx <= 0 || idx == len(ref)-1 {
		return "", 0, false
	}
	rev, err := strconv.ParseInt(ref[idx+1:], 10, 64)
	if err != nil {
		return "", 0, false
	}
	return ref[:idx], rev, true
}

// checkRequiredDocuments 按发布配置的必填集合逐项校验完整性。
func checkRequiredDocuments(docs []entity.DriverDocument, now time.Time) ([]string, []string) {
	byType := make(map[string]entity.DriverDocument)
	for _, doc := range docs {
		byType[doc.DocType] = doc
	}
	var ids, missing []string
	for _, docType := range requiredDocTypes {
		doc, ok := byType[docType]
		if !ok {
			missing = append(missing, docType)
			continue
		}
		if expiry := doc.Expiry(); expiry != nil && entity.IsExpired(now, *expiry) {
			missing = append(missing, docType)
			continue
		}
		ids = append(ids, doc.DocumentID)
	}
	return ids, missing
}

// ensureFilesUsable 校验材料归属、状态与数量，未完成文件不能进入申请。
// 草稿阶段与提交阶段共用同一份判断，前端提示不能替代。
func ensureFilesUsable(ctx context.Context, files entity.FileRepository, ownerID string, ids []string) error {
	if len(ids) > entity.MaxFilesPerRevision {
		return apperrors.InvalidArgument("TOO_MANY_FILES", fmt.Sprintf("at most %d files per revision", entity.MaxFilesPerRevision))
	}
	records, err := files.ListByIDs(ctx, ids)
	if err != nil {
		return err
	}
	if len(records) != len(ids) {
		return apperrors.NotFound("FILE_NOT_FOUND", "some files do not exist")
	}
	for _, rec := range records {
		if rec.OwnerID != ownerID {
			return apperrors.Forbidden("FORBIDDEN", "file belongs to another account")
		}
		if rec.State != entity.FileStateReady {
			return apperrors.InvalidArgument("FILE_NOT_READY", "file "+rec.FileID+" is not ready for submission")
		}
	}
	return nil
}
