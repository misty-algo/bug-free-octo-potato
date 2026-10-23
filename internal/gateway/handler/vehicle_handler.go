package handler

import (
	"strings"

	driverv1 "github.com/BwCloudWeGo/bw-cli/api/gen/driver/v1"
	"github.com/BwCloudWeGo/bw-cli/internal/gateway/request"
	"github.com/BwCloudWeGo/bw-cli/pkg/authn"
	apperrors "github.com/BwCloudWeGo/bw-cli/pkg/errors"
	"github.com/BwCloudWeGo/bw-cli/pkg/httpx"
	"github.com/gin-gonic/gin"
)

// VehicleHandler 是司机端车辆资料与审核命令入口。
// 审核状态只能经由受控命令推进，service 层校验 revision 与 review_version。
type VehicleHandler struct {
	client driverv1.VehicleServiceClient
	auth   authn.Config
}

func NewVehicleHandler(c driverv1.VehicleServiceClient, cfg authn.Config) *VehicleHandler {
	return &VehicleHandler{client: c, auth: cfg}
}

func (h *VehicleHandler) DriverGuard() gin.HandlerFunc {
	return materialGuard(h.auth, "driver")
}

func (h *VehicleHandler) AuditorGuard() gin.HandlerFunc {
	return materialGuard(h.auth, "auditor", "safety_handler")
}

// SaveDraft: POST /api/drv/vehicle/draft
func (h *VehicleHandler) SaveDraft(c *gin.Context) {
	var r struct {
		VIN                  string   `json:"vin"`
		Plate                string   `json:"plate"`
		CityCode             string   `json:"city_code"`
		ModelCode            string   `json:"model_code"`
		Seats                int32    `json:"seats"`
		Color                string   `json:"color"`
		FileIDs              []string `json:"file_ids"`
		ExpectedDraftVersion int64    `json:"expected_draft_version"`
	}
	if err := request.DecodeArrivalJSON(c.Request.Body, &r); err != nil {
		httpx.Error(c, apperrors.InvalidArgument("INVALID_REQUEST", err.Error()))
		return
	}
	ctx, cancel := requestContext(c)
	defer cancel()
	out, err := h.client.SaveDraft(ctx, &driverv1.SaveDraftRequest{
		Vin: r.VIN, Plate: r.Plate, CityCode: r.CityCode, ModelCode: r.ModelCode,
		Seats: r.Seats, Color: r.Color, FileIds: r.FileIDs, ExpectedDraftVersion: r.ExpectedDraftVersion,
	})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	httpx.OK(c, out)
}

// SubmitApplication: POST /api/drv/vehicle/application
// 首次提交 expected_revision/expected_review_version 传 0；
// 驳回或撤回后追加时必须带上当前值。
func (h *VehicleHandler) SubmitApplication(c *gin.Context) {
	var r struct {
		ApplicationID         string `json:"application_id"`
		ExpectedDraftVersion  int64  `json:"expected_draft_version"`
		ExpectedRevision      int64  `json:"expected_revision"`
		ExpectedReviewVersion int64  `json:"expected_review_version"`
	}
	if err := request.DecodeArrivalJSON(c.Request.Body, &r); err != nil {
		httpx.Error(c, apperrors.InvalidArgument("INVALID_REQUEST", err.Error()))
		return
	}
	ctx, cancel := requestContext(c)
	defer cancel()
	out, err := h.client.SubmitApplication(ctx, &driverv1.SubmitApplicationRequest{
		ApplicationId: r.ApplicationID, ExpectedDraftVersion: r.ExpectedDraftVersion,
		ExpectedRevision: r.ExpectedRevision, ExpectedReviewVersion: r.ExpectedReviewVersion,
	})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	httpx.OK(c, out)
}

// WithdrawApplication: POST /api/drv/vehicle/application/withdraw
func (h *VehicleHandler) WithdrawApplication(c *gin.Context) {
	var r struct {
		ApplicationID         string `json:"application_id"`
		Revision              int64  `json:"revision"`
		ExpectedReviewVersion int64  `json:"expected_review_version"`
	}
	if err := request.DecodeArrivalJSON(c.Request.Body, &r); err != nil {
		httpx.Error(c, apperrors.InvalidArgument("INVALID_REQUEST", err.Error()))
		return
	}
	ctx, cancel := requestContext(c)
	defer cancel()
	out, err := h.client.WithdrawApplication(ctx, &driverv1.WithdrawApplicationRequest{
		ApplicationId: r.ApplicationID, Revision: r.Revision,
		ExpectedReviewVersion: r.ExpectedReviewVersion,
	})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	httpx.OK(c, out)
}

// ApplicationDetail: GET /api/drv/vehicle/application?application_id=
// 司机只读自己的申请；审核角色可读任意申请，用于展示历史驳回原因。
func (h *VehicleHandler) ApplicationDetail(c *gin.Context) {
	ctx, cancel := requestContext(c)
	defer cancel()
	out, err := h.client.ApplicationDetail(ctx, &driverv1.ApplicationDetailRequest{
		ApplicationId: strings.TrimSpace(c.Query("application_id")),
	})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	httpx.OK(c, out)
}

// SaveDocument: POST /api/drv/vehicle/document
func (h *VehicleHandler) SaveDocument(c *gin.Context) {
	var r struct {
		DocType         string   `json:"doc_type"`
		DocNo           string   `json:"doc_no"`
		IssuedAt        int64    `json:"issued_at"`
		ValidUntil      int64    `json:"valid_until"`
		FileIDs         []string `json:"file_ids"`
		ExpectedVersion int64    `json:"expected_version"`
	}
	if err := request.DecodeArrivalJSON(c.Request.Body, &r); err != nil {
		httpx.Error(c, apperrors.InvalidArgument("INVALID_REQUEST", err.Error()))
		return
	}
	ctx, cancel := requestContext(c)
	defer cancel()
	out, err := h.client.SaveDocument(ctx, &driverv1.SaveDocumentRequest{
		DocType: r.DocType, DocNo: r.DocNo, IssuedAt: r.IssuedAt, ValidUntil: r.ValidUntil,
		FileIds: r.FileIDs, ExpectedVersion: r.ExpectedVersion,
	})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	httpx.OK(c, out)
}

// CurrentBinding: GET /api/drv/vehicle/binding
func (h *VehicleHandler) CurrentBinding(c *gin.Context) {
	ctx, cancel := requestContext(c)
	defer cancel()
	out, err := h.client.CurrentBinding(ctx, &driverv1.CurrentBindingRequest{})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	httpx.OK(c, out)
}

// BindingSnapshot: GET /api/drv/vehicle/snapshot?driver_id=
// 派单域在分配任务时调用它锁定车辆版本；写入订单快照后旧订单不再改写。
func (h *VehicleHandler) BindingSnapshot(c *gin.Context) {
	driverID := strings.TrimSpace(c.Query("driver_id"))
	ctx, cancel := requestContext(c)
	defer cancel()
	out, err := h.client.BindingSnapshot(ctx, &driverv1.BindingSnapshotRequest{DriverId: driverID})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	httpx.OK(c, out)
}

type reviewCommandBody struct {
	ApplicationID         string `json:"application_id"`
	Revision              int64  `json:"revision"`
	ExpectedReviewVersion int64  `json:"expected_review_version"`
}

// ClaimApplication: POST /api/admin/driver-audit/claim
func (h *VehicleHandler) ClaimApplication(c *gin.Context) {
	var r reviewCommandBody
	if err := request.DecodeArrivalJSON(c.Request.Body, &r); err != nil {
		httpx.Error(c, apperrors.InvalidArgument("INVALID_REQUEST", err.Error()))
		return
	}
	ctx, cancel := requestContext(c)
	defer cancel()
	out, err := h.client.ClaimApplication(ctx, &driverv1.ReviewCommandRequest{
		ApplicationId: r.ApplicationID, Revision: r.Revision,
		ExpectedReviewVersion: r.ExpectedReviewVersion,
	})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	httpx.OK(c, out)
}

// TransferApplication: POST /api/admin/driver-audit/transfer
func (h *VehicleHandler) TransferApplication(c *gin.Context) {
	var r struct {
		reviewCommandBody
		ToReviewerID string `json:"to_reviewer_id"`
		Reason       string `json:"reason"`
	}
	if err := request.DecodeArrivalJSON(c.Request.Body, &r); err != nil {
		httpx.Error(c, apperrors.InvalidArgument("INVALID_REQUEST", err.Error()))
		return
	}
	ctx, cancel := requestContext(c)
	defer cancel()
	out, err := h.client.TransferApplication(ctx, &driverv1.TransferApplicationRequest{
		ApplicationId: r.ApplicationID, Revision: r.Revision,
		ExpectedReviewVersion: r.ExpectedReviewVersion,
		ToReviewerId:          r.ToReviewerID, Reason: r.Reason,
	})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	httpx.OK(c, out)
}

// ApproveApplication: POST /api/admin/driver-audit/approve
func (h *VehicleHandler) ApproveApplication(c *gin.Context) {
	var r reviewCommandBody
	if err := request.DecodeArrivalJSON(c.Request.Body, &r); err != nil {
		httpx.Error(c, apperrors.InvalidArgument("INVALID_REQUEST", err.Error()))
		return
	}
	ctx, cancel := requestContext(c)
	defer cancel()
	out, err := h.client.ApproveApplication(ctx, &driverv1.ReviewCommandRequest{
		ApplicationId: r.ApplicationID, Revision: r.Revision,
		ExpectedReviewVersion: r.ExpectedReviewVersion,
	})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	httpx.OK(c, out)
}

// RejectApplication: POST /api/admin/driver-audit/reject
// items 必须逐条给出目标与原因；不通过的包不部分替换有效版本。
func (h *VehicleHandler) RejectApplication(c *gin.Context) {
	var r struct {
		reviewCommandBody
		Items []struct {
			TargetType string `json:"target_type"`
			TargetKey  string `json:"target_key"`
			Code       string `json:"code"`
			Reason     string `json:"reason"`
		} `json:"items"`
		Summary string `json:"summary"`
	}
	if err := request.DecodeArrivalJSON(c.Request.Body, &r); err != nil {
		httpx.Error(c, apperrors.InvalidArgument("INVALID_REQUEST", err.Error()))
		return
	}
	items := make([]*driverv1.RejectItemInput, 0, len(r.Items))
	for _, item := range r.Items {
		items = append(items, &driverv1.RejectItemInput{
			TargetType: item.TargetType, TargetKey: item.TargetKey,
			Code: item.Code, Reason: item.Reason,
		})
	}
	ctx, cancel := requestContext(c)
	defer cancel()
	out, err := h.client.RejectApplication(ctx, &driverv1.RejectApplicationRequest{
		ApplicationId: r.ApplicationID, Revision: r.Revision,
		ExpectedReviewVersion: r.ExpectedReviewVersion,
		Items:                 items, Summary: r.Summary,
	})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	httpx.OK(c, out)
}

// PendingAudits: GET /api/admin/driver-audit/pending?state=pending|reviewing|mine
func (h *VehicleHandler) PendingAudits(c *gin.Context) {
	ctx, cancel := requestContext(c)
	defer cancel()
	out, err := h.client.PendingAudits(ctx, &driverv1.PendingAuditsRequest{
		State: strings.TrimSpace(c.Query("state")),
	})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	httpx.OK(c, out)
}

// DrainOutbox: POST /api/admin/outbox/drain
func (h *VehicleHandler) DrainOutbox(c *gin.Context) {
	var r struct {
		Limit int64 `json:"limit"`
	}
	if c.Request.ContentLength > 0 {
		if err := request.DecodeArrivalJSON(c.Request.Body, &r); err != nil {
			httpx.Error(c, apperrors.InvalidArgument("INVALID_REQUEST", err.Error()))
			return
		}
	}
	ctx, cancel := requestContext(c)
	defer cancel()
	out, err := h.client.DrainOutbox(ctx, &driverv1.DrainOutboxRequest{Limit: r.Limit})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	httpx.OK(c, out)
}

// Eligibility: GET /api/drv/eligibility
// 实时计算，不接受客户端传入的"是否合格"，也不把结果当长期授权。
func (h *VehicleHandler) Eligibility(c *gin.Context) {
	ctx, cancel := requestContext(c)
	defer cancel()
	out, err := h.client.CheckEligibility(ctx, &driverv1.CheckEligibilityRequest{})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	httpx.OK(c, out)
}

// TickReminders: POST /api/admin/reminder/tick
// now 用于演练与补跑；为 0 时取服务器时间。
func (h *VehicleHandler) TickReminders(c *gin.Context) {
	var r struct {
		Now   int64 `json:"now"`
		Limit int64 `json:"limit"`
	}
	if c.Request.ContentLength > 0 {
		if err := request.DecodeArrivalJSON(c.Request.Body, &r); err != nil {
			httpx.Error(c, apperrors.InvalidArgument("INVALID_REQUEST", err.Error()))
			return
		}
	}
	ctx, cancel := requestContext(c)
	defer cancel()
	out, err := h.client.TickReminders(ctx, &driverv1.TickRemindersRequest{Now: r.Now, Limit: r.Limit})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	httpx.OK(c, out)
}

// AssignTask: POST /api/admin/dispatch/assign
func (h *VehicleHandler) AssignTask(c *gin.Context) {
	var r struct {
		DriverID               string   `json:"driver_id"`
		OrderIDs               []string `json:"order_ids"`
		ExpectedSourceRevision int64    `json:"expected_source_revision"`
		TaskNo                 string   `json:"task_no"`
	}
	if err := request.DecodeArrivalJSON(c.Request.Body, &r); err != nil {
		httpx.Error(c, apperrors.InvalidArgument("INVALID_REQUEST", err.Error()))
		return
	}
	ctx, cancel := requestContext(c)
	defer cancel()
	out, err := h.client.AssignTask(ctx, &driverv1.AssignTaskRequest{
		DriverId: r.DriverID, OrderIds: r.OrderIDs,
		ExpectedSourceRevision: r.ExpectedSourceRevision, TaskNo: r.TaskNo,
	})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	httpx.OK(c, out)
}
