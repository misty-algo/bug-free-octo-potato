package dto

import (
	"time"

	"github.com/BwCloudWeGo/bw-cli/internal/driver/entity"
	"github.com/BwCloudWeGo/bw-cli/pkg/authn"
)

// SaveDraftCommand 建立或更新车辆草稿。ExpectedDraftVersion=0 表示新建。
type SaveDraftCommand struct {
	Principal            authn.Principal `json:"-"`
	VIN                  string          `json:"vin"`
	Plate                string          `json:"plate"`
	CityCode             string          `json:"city_code"`
	ModelCode            string          `json:"model_code"`
	Seats                int             `json:"seats"`
	Color                string          `json:"color"`
	FileIDs              []string        `json:"file_ids"`
	ExpectedDraftVersion int64           `json:"expected_draft_version"`
}

type DraftResult struct {
	DraftID   string   `json:"draft_id"`
	Version   int64    `json:"version"`
	VehicleID string   `json:"vehicle_id"`
	VinMask   string   `json:"vin_mask"`
	Plate     string   `json:"plate"`
	CityCode  string   `json:"city_code"`
	ModelCode string   `json:"model_code"`
	Seats     int      `json:"seats"`
	Color     string   `json:"color"`
	FileIDs   []string `json:"file_ids"`
}

// SaveDocumentCommand 是受权编辑命令：必须传完整证件号，不接受脱敏值回写。
type SaveDocumentCommand struct {
	Principal  authn.Principal `json:"-"`
	DocType    string          `json:"doc_type"`
	DocNo      string          `json:"doc_no"`
	IssuedAt   *time.Time      `json:"issued_at"`
	ValidUntil *time.Time      `json:"valid_until"`
	// ValidUntilDate 是"最后有效自然日"（YYYY-MM-DD，Asia/Shanghai），
	// 优先于 valid_until 的秒级口径。
	ValidUntilDate  string   `json:"valid_until_date"`
	IssuedOn        string   `json:"issued_on"`
	FileIDs         []string `json:"file_ids"`
	ExpectedVersion int64    `json:"expected_version"`
}

type DocumentItem struct {
	DocumentID     string     `json:"document_id"`
	DocType        string     `json:"doc_type"`
	DocNoMask      string     `json:"doc_no_mask"`
	IssuedAt       *time.Time `json:"issued_at"`
	ValidUntil     *time.Time `json:"valid_until"`
	ValidUntilDate string     `json:"valid_until_date"`
	ExpiresAt      *time.Time `json:"expires_at"`
	FileIDs        []string   `json:"file_ids"`
	Version        int64      `json:"version"`
	State          string     `json:"state"`
}

// SubmitApplicationCommand 提交或追加 revision。
// 首次提交 expected_revision/expected_review_version 均为 0；
// 驳回或撤回后追加时，两者必须与当前申请一致，否则视为陈旧请求。
type SubmitApplicationCommand struct {
	Principal             authn.Principal `json:"-"`
	ApplicationID         string          `json:"application_id"`
	ExpectedDraftVersion  int64           `json:"expected_draft_version"`
	ExpectedRevision      int64           `json:"expected_revision"`
	ExpectedReviewVersion int64           `json:"expected_review_version"`
}

type ApplicationResult struct {
	ApplicationID  string   `json:"application_id"`
	Revision       int64    `json:"revision"`
	ActiveRevision int64    `json:"active_revision"`
	State          string   `json:"state"`
	VehicleID      string   `json:"vehicle_id"`
	FileIDs        []string `json:"file_ids"`
	DocumentIDs    []string `json:"document_ids"`
	ReviewVersion  int64    `json:"review_version"`
	ReviewerID     string   `json:"reviewer_id"`
	PayloadDigest  string   `json:"payload_digest"`
	Flags          []string `json:"flags"`
}

type BindingResult struct {
	DriverID                string                 `json:"driver_id"`
	VehicleID               string                 `json:"vehicle_id"`
	ActiveVehicleRevisionID string                 `json:"active_vehicle_revision_id"`
	QualificationVersion    int64                  `json:"qualification_version"`
	BindingVersion          int64                  `json:"binding_version"`
	State                   string                 `json:"state"`
	Snapshot                entity.VehicleSnapshot `json:"snapshot"`
}

type BindingView struct {
	Binding  *BindingResult          `json:"binding"`
	History  []BindingResult         `json:"history"`
	Snapshot *entity.VehicleSnapshot `json:"snapshot"`
}

type PendingAuditItem struct {
	ApplicationID string    `json:"application_id"`
	Revision      int64     `json:"revision"`
	DriverID      string    `json:"driver_id"`
	VehicleID     string    `json:"vehicle_id"`
	Plate         string    `json:"plate"`
	FileIDs       []string  `json:"file_ids"`
	CreatedAt     time.Time `json:"created_at"`
	ReviewVersion int64     `json:"review_version"`
	ReviewerID    string    `json:"reviewer_id"`
	State         string    `json:"state"`
}
