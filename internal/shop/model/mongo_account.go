package model

import "time"

const usersMongoCollectionName = "users"

// UsersDocument 是 users 集合（乘客账户）的 MongoDB 文档模型。
type UsersDocument struct {
	ID                string          `bson:"_id"`
	PhoneEnc          string          `bson:"phone_enc"`
	PhoneHash         string          `bson:"phone_hash"`
	PhoneMask         string          `bson:"phone_mask"`
	Nickname          string          `bson:"nickname"`
	AvatarURL         string          `bson:"avatar_url"`
	Gender            string          `bson:"gender"`
	Birthday          time.Time       `bson:"birthday"`
	CityCode          string          `bson:"city_code"`
	RealNameStatus    string          `bson:"real_name_status"`
	RealName          string          `bson:"real_name"`
	IDCardEnc         string          `bson:"id_card_enc"`
	IDCardMask        string          `bson:"id_card_mask"`
	FaceVerifyAt      time.Time       `bson:"face_verify_at"`
	Status            string          `bson:"status"`
	BanReason         string          `bson:"ban_reason"`
	BanUntil          time.Time       `bson:"ban_until"`
	CreditScore       int64           `bson:"credit_score"`
	IsNewUser         bool            `bson:"is_new_user"`
	FirstOrderAt      time.Time       `bson:"first_order_at"`
	TotalOrderCount   int64           `bson:"total_order_count"`
	CancelOrderCount  int64           `bson:"cancel_order_count"`
	NoShowCount       int64           `bson:"no_show_count"`
	WalletBalance     int64           `bson:"wallet_balance"`
	WalletGiftBalance int64           `bson:"wallet_gift_balance"`
	MemberLevel       string          `bson:"member_level"`
	MemberExpireAt    time.Time       `bson:"member_expire_at"`
	InviteCode        string          `bson:"invite_code"`
	InvitedBy         string          `bson:"invited_by"`
	EnterpriseID      string          `bson:"enterprise_id"`
	PartnerID         string          `bson:"partner_id"`
	LastLoginAt       time.Time       `bson:"last_login_at"`
	LastLoginIP       string          `bson:"last_login_ip"`
	RegisterChannel   string          `bson:"register_channel"`
	RegisterSource    string          `bson:"register_source"`
	Preferences       UserPreferences `bson:"preferences"`
	Tags              []string        `bson:"tags"`

	// 16.6 增补字段
	DefaultInvoiceTitleID string `bson:"default_invoice_title_id"`
	NoPassPayEnabled      bool   `bson:"no_pass_pay_enabled"`

	// 全局审计字段
	CreatedAt time.Time `bson:"created_at"`
	UpdatedAt time.Time `bson:"updated_at"`
	CreatedBy string    `bson:"created_by"`
	UpdatedBy string    `bson:"updated_by"`
	IsDeleted bool      `bson:"is_deleted"`
	Version   int64     `bson:"version"`
}

func (UsersDocument) MongoCollectionName() string {
	return usersMongoCollectionName
}

// UserPreferences 表示 users.preferences 乘车偏好。
type UserPreferences struct {
	Quiet       bool  `bson:"quiet"`
	NoSmoking   bool  `bson:"no_smoking"`
	Temperature int64 `bson:"temperature"`
	Luggage     int64 `bson:"luggage"`
}

const userAddressesMongoCollectionName = "user_addresses"

// UserAddressesDocument 是 user_addresses 集合（乘客常用地址）的 MongoDB 文档模型。
type UserAddressesDocument struct {
	ID          string   `bson:"_id"`
	UserID      string   `bson:"user_id"`
	Label       string   `bson:"label"`
	Name        string   `bson:"name"`
	Address     string   `bson:"address"`
	PoiID       string   `bson:"poi_id"`
	Location    GeoPoint `bson:"location"`
	HouseNumber string   `bson:"house_number"`
	CityCode    string   `bson:"city_code"`
	UseCount    int64    `bson:"use_count"`
	IsDefault   bool     `bson:"is_default"`

	CreatedAt time.Time `bson:"created_at"`
	UpdatedAt time.Time `bson:"updated_at"`
	CreatedBy string    `bson:"created_by"`
	UpdatedBy string    `bson:"updated_by"`
	IsDeleted bool      `bson:"is_deleted"`
	Version   int64     `bson:"version"`
}

func (UserAddressesDocument) MongoCollectionName() string {
	return userAddressesMongoCollectionName
}

const userEmergencyContactsMongoCollectionName = "user_emergency_contacts"

// UserEmergencyContactsDocument 是 user_emergency_contacts 集合（紧急联系人）的 MongoDB 文档模型。
type UserEmergencyContactsDocument struct {
	ID            string `bson:"_id"`
	UserID        string `bson:"user_id"`
	Name          string `bson:"name"`
	PhoneEnc      string `bson:"phone_enc"`
	Relation      string `bson:"relation"`
	IsPrimary     bool   `bson:"is_primary"`
	ShareToken    string `bson:"share_token"`
	NotifyEnabled bool   `bson:"notify_enabled"`

	CreatedAt time.Time `bson:"created_at"`
	UpdatedAt time.Time `bson:"updated_at"`
	CreatedBy string    `bson:"created_by"`
	UpdatedBy string    `bson:"updated_by"`
	IsDeleted bool      `bson:"is_deleted"`
	Version   int64     `bson:"version"`
}

func (UserEmergencyContactsDocument) MongoCollectionName() string {
	return userEmergencyContactsMongoCollectionName
}

const userDevicesMongoCollectionName = "user_devices"

// UserDevicesDocument 是 user_devices 集合（设备指纹）的 MongoDB 文档模型。
type UserDevicesDocument struct {
	ID           string    `bson:"_id"`
	UserID       string    `bson:"user_id"`
	DeviceID     string    `bson:"device_id"`
	Platform     string    `bson:"platform"`
	DeviceModel  string    `bson:"device_model"`
	OSVersion    string    `bson:"os_version"`
	AppVersion   string    `bson:"app_version"`
	IP           string    `bson:"ip"`
	IsRooted     bool      `bson:"is_rooted"`
	IsEmulator   bool      `bson:"is_emulator"`
	RiskLevel    string    `bson:"risk_level"`
	LastActiveAt time.Time `bson:"last_active_at"`

	CreatedAt time.Time `bson:"created_at"`
	UpdatedAt time.Time `bson:"updated_at"`
	CreatedBy string    `bson:"created_by"`
	UpdatedBy string    `bson:"updated_by"`
	IsDeleted bool      `bson:"is_deleted"`
	Version   int64     `bson:"version"`
}

func (UserDevicesDocument) MongoCollectionName() string {
	return userDevicesMongoCollectionName
}

const smsCodesMongoCollectionName = "sms_codes"

// SmsCodesDocument 是 sms_codes 集合（短信验证码）的 MongoDB 文档模型。
type SmsCodesDocument struct {
	ID        string    `bson:"_id"`
	PhoneHash string    `bson:"phone_hash"`
	Scene     string    `bson:"scene"`
	Code      string    `bson:"code"`
	ExpireAt  time.Time `bson:"expire_at"`
	UsedAt    time.Time `bson:"used_at"`
	SendCount int64     `bson:"send_count"`
	IP        string    `bson:"ip"`

	CreatedAt time.Time `bson:"created_at"`
	UpdatedAt time.Time `bson:"updated_at"`
	CreatedBy string    `bson:"created_by"`
	UpdatedBy string    `bson:"updated_by"`
	IsDeleted bool      `bson:"is_deleted"`
	Version   int64     `bson:"version"`
}

func (SmsCodesDocument) MongoCollectionName() string {
	return smsCodesMongoCollectionName
}
