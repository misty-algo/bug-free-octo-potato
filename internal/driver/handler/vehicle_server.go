package handler

import (
	"context"
	"time"

	driverv1 "github.com/BwCloudWeGo/bw-cli/api/gen/driver/v1"
	"github.com/BwCloudWeGo/bw-cli/internal/driver/dto"
	"github.com/BwCloudWeGo/bw-cli/internal/driver/entity"
	"github.com/BwCloudWeGo/bw-cli/internal/driver/service"
	"github.com/BwCloudWeGo/bw-cli/pkg/authn"
	apperrors "github.com/BwCloudWeGo/bw-cli/pkg/errors"
)

// VehicleServer 暴露草稿、证件、绑定与受控审核命令。
type VehicleServer struct {
	driverv1.UnimplementedVehicleServiceServer
	vehicles    *service.VehicleService
	reviews     *service.ReviewService
	eligibility *service.EligibilityService
	reminders   *service.ReminderService
	auth        authn.Config
}

func NewVehicleServer(
	v *service.VehicleService,
	r *service.ReviewService,
	e *service.EligibilityService,
	rem *service.ReminderService,
	cfg authn.Config,
) *VehicleServer {
	return &VehicleServer{vehicles: v, reviews: r, eligibility: e, reminders: rem, auth: cfg}
}

func (s *VehicleServer) SaveDraft(ctx context.Context, r *driverv1.SaveDraftRequest) (*driverv1.DraftResult, error) {
	principal, err := principalFrom(ctx, s.auth)
	if err != nil {
		return nil, apperrors.ToGRPC(err)
	}
	result, err := s.vehicles.SaveDraft(ctx, dto.SaveDraftCommand{
		Principal: principal, VIN: r.GetVin(), Plate: r.GetPlate(), CityCode: r.GetCityCode(),
		ModelCode: r.GetModelCode(), Seats: int(r.GetSeats()), Color: r.GetColor(),
		FileIDs: r.GetFileIds(), ExpectedDraftVersion: r.GetExpectedDraftVersion(),
	})
	if err != nil {
		return nil, apperrors.ToGRPC(err)
	}
	return &driverv1.DraftResult{
		DraftId: result.DraftID, Version: result.Version, VehicleId: result.VehicleID,
		VinMask: result.VinMask, Plate: result.Plate, CityCode: result.CityCode,
		ModelCode: result.ModelCode, Seats: int32(result.Seats), Color: result.Color, FileIds: result.FileIDs,
	}, nil
}

func (s *VehicleServer) SubmitApplication(ctx context.Context, r *driverv1.SubmitApplicationRequest) (*driverv1.ApplicationResult, error) {
	principal, err := principalFrom(ctx, s.auth)
	if err != nil {
		return nil, apperrors.ToGRPC(err)
	}
	result, err := s.reviews.Submit(ctx, dto.SubmitApplicationCommand{
		Principal: principal, ApplicationID: r.GetApplicationId(),
		ExpectedDraftVersion:  r.GetExpectedDraftVersion(),
		ExpectedRevision:      r.GetExpectedRevision(),
		ExpectedReviewVersion: r.GetExpectedReviewVersion(),
	})
	if err != nil {
		return nil, apperrors.ToGRPC(err)
	}
	return toApplicationResult(result), nil
}

func (s *VehicleServer) WithdrawApplication(ctx context.Context, r *driverv1.WithdrawApplicationRequest) (*driverv1.ApplicationResult, error) {
	principal, err := principalFrom(ctx, s.auth)
	if err != nil {
		return nil, apperrors.ToGRPC(err)
	}
	result, err := s.reviews.WithdrawApplication(ctx, dto.WithdrawCommand{
		Principal: principal, ApplicationID: r.GetApplicationId(), Revision: r.GetRevision(),
		ExpectedReviewVersion: r.GetExpectedReviewVersion(),
	})
	if err != nil {
		return nil, apperrors.ToGRPC(err)
	}
	return toApplicationResult(result), nil
}

func (s *VehicleServer) ApplicationDetail(ctx context.Context, r *driverv1.ApplicationDetailRequest) (*driverv1.ApplicationDetailResponse, error) {
	principal, err := principalFrom(ctx, s.auth)
	if err != nil {
		return nil, apperrors.ToGRPC(err)
	}
	detail, err := s.reviews.ApplicationDetail(ctx, dto.ApplicationDetailCommand{
		Principal: principal, ApplicationID: r.GetApplicationId(),
	})
	if err != nil {
		return nil, apperrors.ToGRPC(err)
	}
	out := &driverv1.ApplicationDetailResponse{
		ApplicationId: detail.ApplicationID, DriverId: detail.DriverID, VehicleId: detail.VehicleID,
		State: detail.State, ActiveRevision: detail.ActiveRevision, Revision: detail.Revision,
		ReviewVersion: detail.ReviewVersion, ReviewerId: detail.ReviewerID, ReviewFlags: detail.ReviewFlags,
	}
	for _, rev := range detail.Revisions {
		view := &driverv1.RevisionView{
			Revision: rev.Revision, State: rev.State, PayloadDigest: rev.PayloadDigest,
			VehicleId: rev.VehicleID, VinMask: rev.VinMask, Plate: rev.Plate, CityCode: rev.CityCode,
			ModelCode: rev.ModelCode, Seats: int32(rev.Seats), Color: rev.Color,
			FileIds: rev.FileIDs, DocumentIds: rev.DocumentIDs, CreatedAt: rev.CreatedAt.Unix(),
		}
		for _, d := range rev.Decisions {
			dv := &driverv1.DecisionView{
				Action: d.Action, ActorId: d.ActorID, ReviewVersion: d.ReviewVersion,
				Summary: d.Summary, At: d.At.Unix(),
			}
			for _, item := range d.Items {
				dv.Items = append(dv.Items, &driverv1.RejectItemView{
					TargetType: item.TargetType, TargetKey: item.TargetKey,
					Code: item.Code, Reason: item.Reason,
				})
			}
			view.Decisions = append(view.Decisions, dv)
		}
		for _, a := range rev.Assignments {
			view.Assignments = append(view.Assignments, &driverv1.AssignmentView{
				Action: a.Action, FromId: a.FromID, ToId: a.ToID,
				Reason: a.Reason, ReviewVersion: a.ReviewVersion, At: a.At.Unix(),
			})
		}
		out.Revisions = append(out.Revisions, view)
	}
	return out, nil
}

func (s *VehicleServer) SaveDocument(ctx context.Context, r *driverv1.SaveDocumentRequest) (*driverv1.DocumentItem, error) {
	principal, err := principalFrom(ctx, s.auth)
	if err != nil {
		return nil, apperrors.ToGRPC(err)
	}
	result, err := s.vehicles.SaveDocument(ctx, dto.SaveDocumentCommand{
		Principal: principal, DocType: r.GetDocType(), DocNo: r.GetDocNo(),
		IssuedAt: timePtr(r.GetIssuedAt()), ValidUntil: timePtr(r.GetValidUntil()),
		ValidUntilDate: r.GetValidUntilDate(), IssuedOn: r.GetIssuedOn(),
		FileIDs: r.GetFileIds(), ExpectedVersion: r.GetExpectedVersion(),
	})
	if err != nil {
		return nil, apperrors.ToGRPC(err)
	}
	return &driverv1.DocumentItem{
		DocumentId: result.DocumentID, DocType: result.DocType, DocNoMask: result.DocNoMask,
		IssuedAt: unix(result.IssuedAt), ValidUntil: unix(result.ValidUntil),
		ValidUntilDate: result.ValidUntilDate, ExpiresAt: unix(result.ExpiresAt),
		FileIds: result.FileIDs, Version: result.Version, State: result.State,
	}, nil
}

func (s *VehicleServer) CurrentBinding(ctx context.Context, _ *driverv1.CurrentBindingRequest) (*driverv1.BindingView, error) {
	principal, err := principalFrom(ctx, s.auth)
	if err != nil {
		return nil, apperrors.ToGRPC(err)
	}
	view, err := s.vehicles.CurrentBinding(ctx, principal)
	if err != nil {
		return nil, apperrors.ToGRPC(err)
	}
	out := &driverv1.BindingView{Snapshot: toSnapshot(view.Snapshot)}
	if view.Binding != nil {
		out.Binding = toBinding(view.Binding)
	}
	for i := range view.History {
		out.History = append(out.History, toBinding(&view.History[i]))
	}
	return out, nil
}

func (s *VehicleServer) BindingSnapshot(ctx context.Context, r *driverv1.BindingSnapshotRequest) (*driverv1.VehicleSnapshot, error) {
	principal, err := principalFrom(ctx, s.auth)
	if err != nil {
		return nil, apperrors.ToGRPC(err)
	}
	snapshot, err := s.vehicles.BindingSnapshot(ctx, principal, r.GetDriverId())
	if err != nil {
		return nil, apperrors.ToGRPC(err)
	}
	return toSnapshot(snapshot), nil
}

func (s *VehicleServer) ClaimApplication(ctx context.Context, r *driverv1.ReviewCommandRequest) (*driverv1.ApplicationResult, error) {
	principal, err := principalFrom(ctx, s.auth)
	if err != nil {
		return nil, apperrors.ToGRPC(err)
	}
	result, err := s.reviews.ClaimApplication(ctx, dto.ReviewCommand{
		Principal: principal, ApplicationID: r.GetApplicationId(), Revision: r.GetRevision(),
		ExpectedReviewVersion: r.GetExpectedReviewVersion(),
	})
	if err != nil {
		return nil, apperrors.ToGRPC(err)
	}
	return toApplicationResult(result), nil
}

func (s *VehicleServer) TransferApplication(ctx context.Context, r *driverv1.TransferApplicationRequest) (*driverv1.ApplicationResult, error) {
	principal, err := principalFrom(ctx, s.auth)
	if err != nil {
		return nil, apperrors.ToGRPC(err)
	}
	result, err := s.reviews.Transfer(ctx, dto.TransferCommand{
		ReviewCommand: dto.ReviewCommand{
			Principal: principal, ApplicationID: r.GetApplicationId(), Revision: r.GetRevision(),
			ExpectedReviewVersion: r.GetExpectedReviewVersion(),
		},
		ToReviewerID: r.GetToReviewerId(), Reason: r.GetReason(),
	})
	if err != nil {
		return nil, apperrors.ToGRPC(err)
	}
	return toApplicationResult(result), nil
}

func (s *VehicleServer) ApproveApplication(ctx context.Context, r *driverv1.ReviewCommandRequest) (*driverv1.ApplicationResult, error) {
	principal, err := principalFrom(ctx, s.auth)
	if err != nil {
		return nil, apperrors.ToGRPC(err)
	}
	result, err := s.reviews.ApproveApplication(ctx, dto.ReviewCommand{
		Principal: principal, ApplicationID: r.GetApplicationId(), Revision: r.GetRevision(),
		ExpectedReviewVersion: r.GetExpectedReviewVersion(),
	})
	if err != nil {
		return nil, apperrors.ToGRPC(err)
	}
	return toApplicationResult(result), nil
}

func (s *VehicleServer) RejectApplication(ctx context.Context, r *driverv1.RejectApplicationRequest) (*driverv1.ApplicationResult, error) {
	principal, err := principalFrom(ctx, s.auth)
	if err != nil {
		return nil, apperrors.ToGRPC(err)
	}
	items := make([]dto.RejectItemInput, 0, len(r.GetItems()))
	for _, item := range r.GetItems() {
		items = append(items, dto.RejectItemInput{
			TargetType: item.GetTargetType(), TargetKey: item.GetTargetKey(),
			Code: item.GetCode(), Reason: item.GetReason(),
		})
	}
	result, err := s.reviews.RejectApplication(ctx, dto.RejectCommand{
		ReviewCommand: dto.ReviewCommand{
			Principal: principal, ApplicationID: r.GetApplicationId(), Revision: r.GetRevision(),
			ExpectedReviewVersion: r.GetExpectedReviewVersion(),
		},
		Items: items, Summary: r.GetSummary(),
	})
	if err != nil {
		return nil, apperrors.ToGRPC(err)
	}
	return toApplicationResult(result), nil
}

func (s *VehicleServer) PendingAudits(ctx context.Context, r *driverv1.PendingAuditsRequest) (*driverv1.PendingAuditsResponse, error) {
	principal, err := principalFrom(ctx, s.auth)
	if err != nil {
		return nil, apperrors.ToGRPC(err)
	}
	items, err := s.reviews.PendingAudits(ctx, principal, r.GetState())
	if err != nil {
		return nil, apperrors.ToGRPC(err)
	}
	out := &driverv1.PendingAuditsResponse{}
	for _, item := range items {
		out.Items = append(out.Items, &driverv1.PendingAuditItem{
			ApplicationId: item.ApplicationID, Revision: item.Revision, DriverId: item.DriverID,
			VehicleId: item.VehicleID, Plate: item.Plate, FileIds: item.FileIDs,
			CreatedAt: item.CreatedAt.Unix(), ReviewVersion: item.ReviewVersion,
			ReviewerId: item.ReviewerID, State: item.State,
		})
	}
	return out, nil
}

func (s *VehicleServer) DrainOutbox(ctx context.Context, r *driverv1.DrainOutboxRequest) (*driverv1.DrainOutboxResponse, error) {
	principal, err := principalFrom(ctx, s.auth)
	if err != nil {
		return nil, apperrors.ToGRPC(err)
	}
	result, err := s.reviews.DrainOutbox(ctx, principal, r.GetLimit())
	if err != nil {
		return nil, apperrors.ToGRPC(err)
	}
	return &driverv1.DrainOutboxResponse{
		Delivered: int32(result.Delivered), Failed: int32(result.Failed),
	}, nil
}

// CheckEligibility 只允许司机查自己；审核/安全角色可查任意司机。
func (s *VehicleServer) CheckEligibility(ctx context.Context, r *driverv1.CheckEligibilityRequest) (*driverv1.EligibilityView, error) {
	principal, err := principalFrom(ctx, s.auth)
	if err != nil {
		return nil, apperrors.ToGRPC(err)
	}
	driverID := r.GetDriverId()
	if driverID == "" {
		driverID = principal.UserID
	}
	if driverID != principal.UserID && !isStaff(principal.Role) {
		return nil, apperrors.ToGRPC(apperrors.Forbidden("FORBIDDEN", "driver role is required"))
	}
	view, err := s.eligibility.Check(ctx, driverID)
	if err != nil {
		return nil, apperrors.ToGRPC(err)
	}
	return toEligibility(view), nil
}

func (s *VehicleServer) TickReminders(ctx context.Context, r *driverv1.TickRemindersRequest) (*driverv1.TickRemindersResponse, error) {
	principal, err := principalFrom(ctx, s.auth)
	if err != nil {
		return nil, apperrors.ToGRPC(err)
	}
	if !isStaff(principal.Role) {
		return nil, apperrors.ToGRPC(apperrors.Forbidden("FORBIDDEN", "auditor role is required"))
	}
	now := time.Now().UTC()
	if r.GetNow() > 0 {
		now = time.Unix(r.GetNow(), 0).UTC()
	}
	result, err := s.reminders.Tick(ctx, now, int(r.GetLimit()))
	if err != nil {
		return nil, apperrors.ToGRPC(err)
	}
	return &driverv1.TickRemindersResponse{
		Claimed: result.Claimed, Sent: result.Sent, Merged: result.Merged,
		Cancelled: result.Cancelled, Failed: result.Failed,
	}, nil
}

func (s *VehicleServer) AssignTask(ctx context.Context, r *driverv1.AssignTaskRequest) (*driverv1.AssignTaskResponse, error) {
	principal, err := principalFrom(ctx, s.auth)
	if err != nil {
		return nil, apperrors.ToGRPC(err)
	}
	result, err := s.eligibility.AssignTask(ctx, dto.AssignTaskCommand{
		Principal: principal, DriverID: r.GetDriverId(), OrderIDs: r.GetOrderIds(),
		ExpectedSourceRevision: r.GetExpectedSourceRevision(), TaskNo: r.GetTaskNo(),
	})
	if err != nil {
		return nil, apperrors.ToGRPC(err)
	}
	return &driverv1.AssignTaskResponse{
		TaskId: result.TaskID, TaskNo: result.TaskNo,
		SourceRevision: result.SourceRevision, VehicleId: result.VehicleID,
		Snapshot: toSnapshot(result.Snapshot),
	}, nil
}

func toEligibility(v *dto.EligibilityView) *driverv1.EligibilityView {
	out := &driverv1.EligibilityView{
		DriverId: v.DriverID, CanAcceptNewTask: v.CanAcceptNewTask,
		SourceRevision: v.SourceRevision, ValidUntil: unix(v.ValidUntil),
		AllowedActions: v.AllowedActions, BlockedActions: v.BlockedActions,
	}
	for _, r := range v.Restrictions {
		out.Restrictions = append(out.Restrictions, &driverv1.RestrictionView{
			Code: r.Code, Source: r.Source, TargetKey: r.TargetKey,
			Detail: r.Detail, Until: unix(r.Until),
		})
	}
	return out
}

func isStaff(role string) bool {
	return role == "auditor" || role == "safety_handler" || role == "admin"
}

func toApplicationResult(r *dto.ApplicationResult) *driverv1.ApplicationResult {
	if r == nil {
		return nil
	}
	return &driverv1.ApplicationResult{
		ApplicationId: r.ApplicationID, Revision: r.Revision, ActiveRevision: r.ActiveRevision,
		State: r.State, VehicleId: r.VehicleID, FileIds: r.FileIDs, DocumentIds: r.DocumentIDs,
		ReviewVersion: r.ReviewVersion, ReviewerId: r.ReviewerID, PayloadDigest: r.PayloadDigest,
		Flags: r.Flags,
	}
}

func toBinding(b *dto.BindingResult) *driverv1.BindingResult {
	if b == nil {
		return nil
	}
	return &driverv1.BindingResult{
		DriverId: b.DriverID, VehicleId: b.VehicleID,
		ActiveVehicleRevisionId: b.ActiveVehicleRevisionID,
		QualificationVersion:    b.QualificationVersion, BindingVersion: b.BindingVersion,
		State: b.State, Snapshot: toSnapshot(&b.Snapshot),
	}
}

func toSnapshot(s *entity.VehicleSnapshot) *driverv1.VehicleSnapshot {
	if s == nil {
		return nil
	}
	return &driverv1.VehicleSnapshot{
		VehicleId: s.VehicleID, RevisionId: s.RevisionID, Plate: s.Plate,
		ModelCode: s.ModelCode, Seats: int32(s.Seats), Color: s.Color, BindingVersion: s.BindingVersion,
	}
}

func timePtr(unixSeconds int64) *time.Time {
	if unixSeconds <= 0 {
		return nil
	}
	t := time.Unix(unixSeconds, 0).UTC()
	return &t
}

func unix(t *time.Time) int64 {
	if t == nil {
		return 0
	}
	return t.Unix()
}
