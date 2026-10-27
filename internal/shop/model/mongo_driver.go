package model

import "time"

const driversMongoCollectionName = "drivers"

// DriversDocument 是 drivers 集合（司机账户）的 MongoDB 文档模型。
type DriversDocument struct {
	ID                      string    `bson:"_id"`
	DriverNo                string    `bson:"driver_no"`
	PhoneEnc                string    `bson:"phone_enc"`
	PhoneHash               string    `bson:"phone_hash"`
	PhoneMask               string    `bson:"phone_mask"`
	RealName                string    `bson:"real_name"`
	IDCardEnc               string    `bson:"id_card_enc"`
	Gender                  string    `bson:"gender"`
	Birthday                time.Time `bson:"birthday"`
	AvatarURL               string    `bson:"avatar_url"`
	CityCode                string    `bson:"city_code"`
	ServiceCityCodes        []string  `bson:"service_city_codes"`
	DriverType              string    `bson:"driver_type"`
	PartnerID               string    `bson:"partner_id"`
	FleetID                 string    `bson:"fleet_id"`
	InviteCode              string    `bson:"invite_code"`
	Status                  string    `bson:"status"`
	OnlineStatus            string    `bson:"online_status"`
	WorkStatus              string    `bson:"work_status"`
	ServiceScore            int64     `bson:"service_score"`
	StarLevel               int64     `bson:"star_level"`
	AcceptRate              float64   `bson:"accept_rate"`
	CompleteRate            float64   `bson:"complete_rate"`
	CancelRate              float64   `bson:"cancel_rate"`
	ComplaintRate           float64   `bson:"complaint_rate"`
	TotalOrderCount         int64     `bson:"total_order_count"`
	TodayOrderCount         int64     `bson:"today_order_count"`
	TodayIncome             int64     `bson:"today_income"`
	TotalIncome             int64     `bson:"total_income"`
	Balance                 int64     `bson:"balance"`
	FrozenBalance           int64     `bson:"frozen_balance"`
	CreditScore             int64     `bson:"credit_score"`
	LastLocation            GeoPoint  `bson:"last_location"`
	LastLocationAt          time.Time `bson:"last_location_at"`
	LastFaceVerifyAt        time.Time `bson:"last_face_verify_at"`
	TodayOnlineMinutes      int64     `bson:"today_online_minutes"`
	ContinuousOnlineMinutes int64     `bson:"continuous_online_minutes"`
	RestUntil               time.Time `bson:"rest_until"`
	CurrentOrderID          string    `bson:"current_order_id"`
	BankCardEnc             string    `bson:"bank_card_enc"`
	BankName                string    `bson:"bank_name"`
	ContractStatus          string    `bson:"contract_status"`
	ContractSignedAt        time.Time `bson:"contract_signed_at"`
	ActivatedAt             time.Time `bson:"activated_at"`
	QuitAt                  time.Time `bson:"quit_at"`
	Tags                    []string  `bson:"tags"`
	DeviceID                string    `bson:"device_id"`

	// 16.6 增补字段（听单设置）
	ListenRadius       int64                   `bson:"listen_radius"`
	ListenPreferences  DriverListenPreferences `bson:"listen_preferences"`
	PreferProductCodes []string                `bson:"prefer_product_codes"`
	CarpoolEnabled     bool                    `bson:"carpool_enabled"`

	// 全局审计字段
	CreatedAt time.Time `bson:"created_at"`
	UpdatedAt time.Time `bson:"updated_at"`
	CreatedBy string    `bson:"created_by"`
	UpdatedBy string    `bson:"updated_by"`
	IsDeleted bool      `bson:"is_deleted"`
	Version   int64     `bson:"version"`
}

func (DriversDocument) MongoCollectionName() string {
	return driversMongoCollectionName
}

// DriverListenPreferences 表示 drivers.listenPreferences 听单偏好。
type DriverListenPreferences struct {
	DestKeyword       string   `bson:"dest_keyword"`
	PreferAreas       []string `bson:"prefer_areas"`
	AvoidAreas        []string `bson:"avoid_areas"`
	MaxPickupDistance int64    `bson:"max_pickup_distance"`
}

const driverQualificationsMongoCollectionName = "driver_qualifications"

// DriverQualificationsDocument 是 driver_qualifications 集合（司机资质证件）的 MongoDB 文档模型。
type DriverQualificationsDocument struct {
	ID               string                 `bson:"_id"`
	DriverID         string                 `bson:"driver_id"`
	Type             string                 `bson:"type"`
	CertNo           string                 `bson:"cert_no"`
	RealName         string                 `bson:"real_name"`
	IssueDate        time.Time              `bson:"issue_date"`
	ExpireDate       time.Time              `bson:"expire_date"`
	FrontImageURL    string                 `bson:"front_image_url"`
	BackImageURL     string                 `bson:"back_image_url"`
	OcrData          map[string]interface{} `bson:"ocr_data"`
	OcrStatus        string                 `bson:"ocr_status"`
	VerifyStatus     string                 `bson:"verify_status"`
	RejectReason     string                 `bson:"reject_reason"`
	VerifiedBy       string                 `bson:"verified_by"`
	VerifiedAt       time.Time              `bson:"verified_at"`
	FaceCompareScore float64                `bson:"face_compare_score"`
	IsExpired        bool                   `bson:"is_expired"`
	RemindLevel      string                 `bson:"remind_level"`

	CreatedAt time.Time `bson:"created_at"`
	UpdatedAt time.Time `bson:"updated_at"`
	CreatedBy string    `bson:"created_by"`
	UpdatedBy string    `bson:"updated_by"`
	IsDeleted bool      `bson:"is_deleted"`
	Version   int64     `bson:"version"`
}

func (DriverQualificationsDocument) MongoCollectionName() string {
	return driverQualificationsMongoCollectionName
}

const driverAuditsMongoCollectionName = "driver_audits"

// DriverAuditsDocument 是 driver_audits 集合（司机审核记录）的 MongoDB 文档模型。
type DriverAuditsDocument struct {
	ID            string                   `bson:"_id"`
	DriverID      string                   `bson:"driver_id"`
	Round         int64                    `bson:"round"`
	SubmitAt      time.Time                `bson:"submit_at"`
	MachineResult string                   `bson:"machine_result"`
	MachineDetail DriverAuditMachineDetail `bson:"machine_detail"`
	ManualResult  string                   `bson:"manual_result"`
	ManualRemark  string                   `bson:"manual_remark"`
	OperatorID    string                   `bson:"operator_id"`
	OperateAt     time.Time                `bson:"operate_at"`
	FinalStatus   string                   `bson:"final_status"`
	RejectReasons []string                 `bson:"reject_reasons"`
	SlaMinutes    int64                    `bson:"sla_minutes"`

	CreatedAt time.Time `bson:"created_at"`
	UpdatedAt time.Time `bson:"updated_at"`
	CreatedBy string    `bson:"created_by"`
	UpdatedBy string    `bson:"updated_by"`
	IsDeleted bool      `bson:"is_deleted"`
	Version   int64     `bson:"version"`
}

func (DriverAuditsDocument) MongoCollectionName() string {
	return driverAuditsMongoCollectionName
}

// DriverAuditMachineDetail 表示 driver_audits.machineDetail 机审明细。
type DriverAuditMachineDetail struct {
	OcrPass       bool `bson:"ocr_pass"`
	FacePass      bool `bson:"face_pass"`
	BlacklistPass bool `bson:"blacklist_pass"`
	LicenseValid  bool `bson:"license_valid"`
}

const driverTrainingsMongoCollectionName = "driver_trainings"

// DriverTrainingsDocument 是 driver_trainings 集合（培训与考试记录）的 MongoDB 文档模型。
type DriverTrainingsDocument struct {
	ID            string    `bson:"_id"`
	DriverID      string    `bson:"driver_id"`
	CourseCode    string    `bson:"course_code"`
	CourseName    string    `bson:"course_name"`
	CourseType    string    `bson:"course_type"`
	StudyProgress int64     `bson:"study_progress"`
	ExamScore     int64     `bson:"exam_score"`
	PassScore     int64     `bson:"pass_score"`
	IsPassed      bool      `bson:"is_passed"`
	AttemptCount  int64     `bson:"attempt_count"`
	CompletedAt   time.Time `bson:"completed_at"`
	ExpireAt      time.Time `bson:"expire_at"`

	CreatedAt time.Time `bson:"created_at"`
	UpdatedAt time.Time `bson:"updated_at"`
	CreatedBy string    `bson:"created_by"`
	UpdatedBy string    `bson:"updated_by"`
	IsDeleted bool      `bson:"is_deleted"`
	Version   int64     `bson:"version"`
}

func (DriverTrainingsDocument) MongoCollectionName() string {
	return driverTrainingsMongoCollectionName
}

const vehiclesMongoCollectionName = "vehicles"

// VehiclesDocument 是 vehicles 集合（车辆档案）的 MongoDB 文档模型。
type VehiclesDocument struct {
	ID                    string        `bson:"_id"`
	PlateNo               string        `bson:"plate_no"`
	CityCode              string        `bson:"city_code"`
	Brand                 string        `bson:"brand"`
	Model                 string        `bson:"model"`
	Color                 string        `bson:"color"`
	SeatCount             int64         `bson:"seat_count"`
	VehicleType           string        `bson:"vehicle_type"`
	EnergyType            string        `bson:"energy_type"`
	Vin                   string        `bson:"vin"`
	EngineNo              string        `bson:"engine_no"`
	PurchaseDate          time.Time     `bson:"purchase_date"`
	VehicleAge            int64         `bson:"vehicle_age"`
	Mileage               int64         `bson:"mileage"`
	OwnerName             string        `bson:"owner_name"`
	OwnerType             string        `bson:"owner_type"`
	UsageType             string        `bson:"usage_type"`
	InsuranceExpireAt     time.Time     `bson:"insurance_expire_at"`
	AnnualInspectExpireAt time.Time     `bson:"annual_inspect_expire_at"`
	NetcarPermitExpireAt  time.Time     `bson:"netcar_permit_expire_at"`
	Images                VehicleImages `bson:"images"`
	Status                string        `bson:"status"`
	PartnerID             string        `bson:"partner_id"`
	GpsDeviceNo           string        `bson:"gps_device_no"`

	CreatedAt time.Time `bson:"created_at"`
	UpdatedAt time.Time `bson:"updated_at"`
	CreatedBy string    `bson:"created_by"`
	UpdatedBy string    `bson:"updated_by"`
	IsDeleted bool      `bson:"is_deleted"`
	Version   int64     `bson:"version"`
}

func (VehiclesDocument) MongoCollectionName() string {
	return vehiclesMongoCollectionName
}

// VehicleImages 表示 vehicles.images 车辆照片。
type VehicleImages struct {
	Front    string `bson:"front"`
	Back     string `bson:"back"`
	Interior string `bson:"interior"`
	License  string `bson:"license"`
}

const driverVehicleBindsMongoCollectionName = "driver_vehicle_binds"

// DriverVehicleBindsDocument 是 driver_vehicle_binds 集合（司机车辆绑定）的 MongoDB 文档模型。
type DriverVehicleBindsDocument struct {
	ID         string    `bson:"_id"`
	DriverID   string    `bson:"driver_id"`
	VehicleID  string    `bson:"vehicle_id"`
	IsCurrent  bool      `bson:"is_current"`
	BindAt     time.Time `bson:"bind_at"`
	UnbindAt   time.Time `bson:"unbind_at"`
	BindStatus string    `bson:"bind_status"`

	CreatedAt time.Time `bson:"created_at"`
	UpdatedAt time.Time `bson:"updated_at"`
	CreatedBy string    `bson:"created_by"`
	UpdatedBy string    `bson:"updated_by"`
	IsDeleted bool      `bson:"is_deleted"`
	Version   int64     `bson:"version"`
}

func (DriverVehicleBindsDocument) MongoCollectionName() string {
	return driverVehicleBindsMongoCollectionName
}

const driverShiftsMongoCollectionName = "driver_shifts"

// DriverShiftsDocument 是 driver_shifts 集合（出车/收车记录）的 MongoDB 文档模型。
type DriverShiftsDocument struct {
	ID               string    `bson:"_id"`
	DriverID         string    `bson:"driver_id"`
	Date             string    `bson:"date"`
	StartAt          time.Time `bson:"start_at"`
	EndAt            time.Time `bson:"end_at"`
	OnlineMinutes    int64     `bson:"online_minutes"`
	ServiceMinutes   int64     `bson:"service_minutes"`
	OrderCount       int64     `bson:"order_count"`
	Income           int64     `bson:"income"`
	FaceVerifyPassed bool      `bson:"face_verify_passed"`
	RestCount        int64     `bson:"rest_count"`
	CityCode         string    `bson:"city_code"`

	CreatedAt time.Time `bson:"created_at"`
	UpdatedAt time.Time `bson:"updated_at"`
	CreatedBy string    `bson:"created_by"`
	UpdatedBy string    `bson:"updated_by"`
	IsDeleted bool      `bson:"is_deleted"`
	Version   int64     `bson:"version"`
}

func (DriverShiftsDocument) MongoCollectionName() string {
	return driverShiftsMongoCollectionName
}
