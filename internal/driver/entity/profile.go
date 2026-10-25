package entity

import "time"

// VehicleProfile 是车辆的持久身份：由受保护车辆识别信息派生，
// 车牌等可变属性只是 profile 上的字段。
type VehicleProfile struct {
	VehicleID string    `bson:"vehicleId"`
	VinMask   string    `bson:"vinMask"`
	Plate     string    `bson:"plate"`
	CityCode  string    `bson:"cityCode"`
	ModelCode string    `bson:"modelCode"`
	Seats     int       `bson:"seats"`
	Color     string    `bson:"color"`
	CreatedAt time.Time `bson:"createdAt"`
	UpdatedAt time.Time `bson:"updatedAt"`
}

// VehicleDraft 保存影响准入的字段。未提交前不改当前有效车。
type VehicleDraft struct {
	DraftID     string    `bson:"draftId"`
	DriverID    string    `bson:"driverId"`
	Version     int64     `bson:"version"`
	VehicleID   string    `bson:"vehicleId"`
	VinMask     string    `bson:"vinMask"`
	Plate       string    `bson:"plate"`
	CityCode    string    `bson:"cityCode"`
	ModelCode   string    `bson:"modelCode"`
	Seats       int       `bson:"seats"`
	Color       string    `bson:"color"`
	DocumentIDs []string  `bson:"documentIds"`
	FileIDs     []string  `bson:"fileIds"`
	State       string    `bson:"state"`
	UpdatedAt   time.Time `bson:"updatedAt"`
}

// DriverDocument 是证件条目。响应必须脱敏，
// 更新只能走受权编辑命令，不允许把掩码当作完整值回写。
type DriverDocument struct {
	DocumentID  string     `bson:"documentId"`
	SubjectType string     `bson:"subjectType"` // driver | vehicle
	SubjectID   string     `bson:"subjectId"`
	DocType     string     `bson:"docType"`
	DocNoMask   string     `bson:"docNoMask"`
	IssuedAt    *time.Time `bson:"issuedAt"`
	// ExpiresAt 是 exclusive 失效时刻（当地次日 00:00 的 UTC instant）。
	ExpiresAt *time.Time `bson:"expiresAt"`
	// ExpiresOn 是最后有效自然日（Asia/Shanghai），仅用于展示与审计。
	ExpiresOn string `bson:"expiresOn"`
	// ValidUntil 是 D16 之前的旧口径字段，只做兼容读取，不再作为判定依据。
	ValidUntil *time.Time `bson:"validUntil"`
	FileIDs    []string   `bson:"fileIds"`
	Version    int64      `bson:"version"`
	State      string     `bson:"state"`
	UpdatedAt  time.Time  `bson:"updatedAt"`
}

// Expiry 返回该证件的失效时刻：优先新口径 expiresAt，存量数据按旧字段归一。
func (d DriverDocument) Expiry() *time.Time {
	if d.ExpiresAt != nil {
		return d.ExpiresAt
	}
	if d.ValidUntil != nil {
		normalized := NormalizeLegacyValidUntil(*d.ValidUntil)
		return &normalized
	}
	return nil
}

// VehicleAudit 是不可变审核版本。重新提交追加 revision，不覆盖历史驳回。
type VehicleAudit struct {
	ApplicationID string     `bson:"applicationId"`
	Revision      int64      `bson:"revision"`
	DriverID      string     `bson:"driverId"`
	VehicleID     string     `bson:"vehicleId"`
	Plate         string     `bson:"plate"`
	CityCode      string     `bson:"cityCode"`
	ModelCode     string     `bson:"modelCode"`
	Seats         int        `bson:"seats"`
	Color         string     `bson:"color"`
	DocumentIDs   []string   `bson:"documentIds"`
	FileIDs       []string   `bson:"fileIds"`
	State         string     `bson:"state"`
	RejectReason  string     `bson:"rejectReason"`
	RejectCount   int        `bson:"rejectCount"`
	CreatedAt     time.Time  `bson:"createdAt"`
	DecidedAt     *time.Time `bson:"decidedAt"`
	DecidedBy     string     `bson:"decidedBy"`
}

// VehicleBinding 是有效绑定。mongo 侧对 driverId、vehicleId 分别唯一，
// 历史绑定进 binding_history，不占用这两个唯一位。
type VehicleBinding struct {
	BindingID               string     `bson:"bindingId"`
	DriverID                string     `bson:"driverId"`
	VehicleID               string     `bson:"vehicleId"`
	ActiveVehicleRevisionID string     `bson:"activeVehicleRevisionId"`
	QualificationVersion    int64      `bson:"qualificationVersion"`
	BindingVersion          int64      `bson:"bindingVersion"`
	State                   string     `bson:"state"`
	CreatedAt               time.Time  `bson:"createdAt"`
	ReleasedAt              *time.Time `bson:"releasedAt"`
}

// VehicleSnapshot 是分配任务时锁定的车辆版本快照，
// 写入订单后不因续证、改车或注销而变化。
type VehicleSnapshot struct {
	VehicleID      string `bson:"vehicleId"`
	RevisionID     string `bson:"revisionId"`
	Plate          string `bson:"plate"`
	ModelCode      string `bson:"modelCode"`
	Seats          int    `bson:"seats"`
	Color          string `bson:"color"`
	BindingVersion int64  `bson:"bindingVersion"`
}
