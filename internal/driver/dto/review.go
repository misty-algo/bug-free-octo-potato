package dto

import (
	"time"

	"github.com/BwCloudWeGo/bw-cli/pkg/authn"
)

// WithdrawCommand 由司机发起：撤回必须携带当前 revision 与审核代数，
// 与批准/驳回竞争时只有一方能成功。
type WithdrawCommand struct {
	Principal             authn.Principal `json:"-"`
	ApplicationID         string          `json:"application_id"`
	Revision              int64           `json:"revision"`
	ExpectedReviewVersion int64           `json:"expected_review_version"`
}

// ReviewCommand 是审核命令的并发凭据。
type ReviewCommand struct {
	Principal             authn.Principal `json:"-"`
	ApplicationID         string          `json:"application_id"`
	Revision              int64           `json:"revision"`
	ExpectedReviewVersion int64           `json:"expected_review_version"`
}

// TransferCommand 转派必须说明原因，且只追加分配记录，不产生裁决。
type TransferCommand struct {
	ReviewCommand
	ToReviewerID string `json:"to_reviewer_id"`
	Reason       string `json:"reason"`
}

type RejectItemInput struct {
	TargetType string `json:"target_type"`
	TargetKey  string `json:"target_key"`
	Code       string `json:"code"`
	Reason     string `json:"reason"`
}

// RejectCommand 是逐条驳回：整包不通过，不做部分替换。
type RejectCommand struct {
	ReviewCommand
	Items   []RejectItemInput `json:"items"`
	Summary string            `json:"summary"`
}

type ApplicationDetailCommand struct {
	Principal     authn.Principal `json:"-"`
	ApplicationID string          `json:"application_id"`
}

type RejectItemView struct {
	TargetType string `json:"target_type"`
	TargetKey  string `json:"target_key"`
	Code       string `json:"code"`
	Reason     string `json:"reason"`
}

type DecisionView struct {
	Action        string           `json:"action"`
	ActorID       string           `json:"actor_id"`
	ReviewVersion int64            `json:"review_version"`
	Summary       string           `json:"summary"`
	Items         []RejectItemView `json:"items"`
	At            time.Time        `json:"at"`
}

type AssignmentView struct {
	Action        string    `json:"action"`
	FromID        string    `json:"from_id"`
	ToID          string    `json:"to_id"`
	Reason        string    `json:"reason"`
	ReviewVersion int64     `json:"review_version"`
	At            time.Time `json:"at"`
}

// RevisionView 是历史版本的只读视图：被驳回的 revision 仍可查。
type RevisionView struct {
	Revision      int64            `json:"revision"`
	State         string           `json:"state"`
	PayloadDigest string           `json:"payload_digest"`
	VehicleID     string           `json:"vehicle_id"`
	VinMask       string           `json:"vin_mask"`
	Plate         string           `json:"plate"`
	CityCode      string           `json:"city_code"`
	ModelCode     string           `json:"model_code"`
	Seats         int              `json:"seats"`
	Color         string           `json:"color"`
	FileIDs       []string         `json:"file_ids"`
	DocumentIDs   []string         `json:"document_ids"`
	CreatedAt     time.Time        `json:"created_at"`
	Decisions     []DecisionView   `json:"decisions"`
	Assignments   []AssignmentView `json:"assignments"`
}

type ApplicationDetail struct {
	ApplicationID  string         `json:"application_id"`
	DriverID       string         `json:"driver_id"`
	VehicleID      string         `json:"vehicle_id"`
	State          string         `json:"state"`
	ActiveRevision int64          `json:"active_revision"`
	Revision       int64          `json:"revision"`
	ReviewVersion  int64          `json:"review_version"`
	ReviewerID     string         `json:"reviewer_id"`
	ReviewFlags    []string       `json:"review_flags"`
	Revisions      []RevisionView `json:"revisions"`
}

// OutboxDrainResult 是内嵌 outbox 的投递结果。
type OutboxDrainResult struct {
	Delivered int `json:"delivered"`
	Failed    int `json:"failed"`
}
