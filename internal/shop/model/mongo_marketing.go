package model

import "time"

const couponTemplatesMongoCollectionName = "coupon_templates"

// CouponTemplatesDocument 是 coupon_templates 集合（优惠券模板）的 MongoDB 文档模型。
type CouponTemplatesDocument struct {
	ID           string            `bson:"_id"`
	TemplateNo   string            `bson:"template_no"`
	Name         string            `bson:"name"`
	CouponType   string            `bson:"coupon_type"`
	FaceValue    int64             `bson:"face_value"`
	MinCharge    int64             `bson:"min_charge"`
	MaxDiscount  int64             `bson:"max_discount"`
	CityCodes    []string          `bson:"city_codes"`
	ProductCodes []string          `bson:"product_codes"`
	TimeRanges   []CouponTimeRange `bson:"time_ranges"`
	ValidType    string            `bson:"valid_type"`
	ValidStart   time.Time         `bson:"valid_start"`
	ValidEnd     time.Time         `bson:"valid_end"`
	ValidDays    int64             `bson:"valid_days"`
	Stackable    bool              `bson:"stackable"`
	MutualCodes  []string          `bson:"mutual_codes"`
	PerLimit     int64             `bson:"per_limit"`
	SettleMode   string            `bson:"settle_mode"`
	ShareRatio   float64           `bson:"share_ratio"`
	Status       string            `bson:"status"`
	Description  string            `bson:"description"`

	CreatedAt time.Time `bson:"created_at"`
	UpdatedAt time.Time `bson:"updated_at"`
	CreatedBy string    `bson:"created_by"`
	UpdatedBy string    `bson:"updated_by"`
	IsDeleted bool      `bson:"is_deleted"`
	Version   int64     `bson:"version"`
}

func (CouponTemplatesDocument) MongoCollectionName() string {
	return couponTemplatesMongoCollectionName
}

// CouponTimeRange 表示 coupon_templates.timeRanges 可用时段。
type CouponTimeRange struct {
	Start string `bson:"start"`
	End   string `bson:"end"`
}

const couponBatchesMongoCollectionName = "coupon_batches"

// CouponBatchesDocument 是 coupon_batches 集合（券批次）的 MongoDB 文档模型。
type CouponBatchesDocument struct {
	ID            string    `bson:"_id"`
	BatchNo       string    `bson:"batch_no"`
	TemplateID    string    `bson:"template_id"`
	TotalCount    int64     `bson:"total_count"`
	IssuedCount   int64     `bson:"issued_count"`
	UsedCount     int64     `bson:"used_count"`
	Channel       string    `bson:"channel"`
	TargetUserIds []string  `bson:"target_user_ids"`
	Budget        int64     `bson:"budget"`
	Status        string    `bson:"status"`
	StartAt       time.Time `bson:"start_at"`
	EndAt         time.Time `bson:"end_at"`
	OperatorID    string    `bson:"operator_id"`

	CreatedAt time.Time `bson:"created_at"`
	UpdatedAt time.Time `bson:"updated_at"`
	CreatedBy string    `bson:"created_by"`
	UpdatedBy string    `bson:"updated_by"`
	IsDeleted bool      `bson:"is_deleted"`
	Version   int64     `bson:"version"`
}

func (CouponBatchesDocument) MongoCollectionName() string {
	return couponBatchesMongoCollectionName
}

const userCouponsMongoCollectionName = "user_coupons"

// UserCouponsDocument 是 user_coupons 集合（用户优惠券）的 MongoDB 文档模型。
type UserCouponsDocument struct {
	ID             string    `bson:"_id"`
	CouponNo       string    `bson:"coupon_no"`
	UserID         string    `bson:"user_id"`
	TemplateID     string    `bson:"template_id"`
	BatchID        string    `bson:"batch_id"`
	Status         string    `bson:"status"`
	ObtainAt       time.Time `bson:"obtain_at"`
	ValidStart     time.Time `bson:"valid_start"`
	ValidEnd       time.Time `bson:"valid_end"`
	UsedAt         *time.Time `bson:"used_at"`
	OrderID        string    `bson:"order_id"`
	LockOrderID    string    `bson:"lock_order_id"`
	DiscountAmount int64     `bson:"discount_amount"`
	Source         string    `bson:"source"`

	CreatedAt time.Time `bson:"created_at"`
	UpdatedAt time.Time `bson:"updated_at"`
	CreatedBy string    `bson:"created_by"`
	UpdatedBy string    `bson:"updated_by"`
	IsDeleted bool      `bson:"is_deleted"`
	Version   int64     `bson:"version"`
}

func (UserCouponsDocument) MongoCollectionName() string {
	return userCouponsMongoCollectionName
}

const campaignsMongoCollectionName = "campaigns"

// CampaignsDocument 是 campaigns 集合（营销活动）的 MongoDB 文档模型。
type CampaignsDocument struct {
	ID                      string                 `bson:"_id"`
	Code                    string                 `bson:"code"`
	Name                    string                 `bson:"name"`
	Type                    string                 `bson:"type"`
	TargetAudience          string                 `bson:"target_audience"`
	Rules                   map[string]interface{} `bson:"rules"`
	RewardCouponTemplateIds []string               `bson:"reward_coupon_template_ids"`
	RewardAmount            int64                  `bson:"reward_amount"`
	CityCodes               []string               `bson:"city_codes"`
	StartAt                 time.Time              `bson:"start_at"`
	EndAt                   time.Time              `bson:"end_at"`
	Status                  string                 `bson:"status"`
	ParticipantCount        int64                  `bson:"participant_count"`
	Budget                  int64                  `bson:"budget"`
	UsedBudget              int64                  `bson:"used_budget"`
	PageURL                 string                 `bson:"page_url"`

	// 16.6 增补字段
	TaskType   string `bson:"task_type"`
	NeedEnroll bool   `bson:"need_enroll"`

	// 全局审计字段
	CreatedAt time.Time `bson:"created_at"`
	UpdatedAt time.Time `bson:"updated_at"`
	CreatedBy string    `bson:"created_by"`
	UpdatedBy string    `bson:"updated_by"`
	IsDeleted bool      `bson:"is_deleted"`
	Version   int64     `bson:"version"`
}

func (CampaignsDocument) MongoCollectionName() string {
	return campaignsMongoCollectionName
}

const invitationsMongoCollectionName = "invitations"

// InvitationsDocument 是 invitations 集合（邀请关系）的 MongoDB 文档模型。
type InvitationsDocument struct {
	ID            string           `bson:"_id"`
	InviterID     string           `bson:"inviter_id"`
	InviteeID     string           `bson:"invitee_id"`
	InviteCode    string           `bson:"invite_code"`
	Channel       string           `bson:"channel"`
	RegisteredAt  time.Time        `bson:"registered_at"`
	FirstOrderAt  time.Time        `bson:"first_order_at"`
	RewardStatus  string           `bson:"reward_status"`
	InviterReward InvitationReward `bson:"inviter_reward"`
	InviteeReward InvitationReward `bson:"invitee_reward"`

	CreatedAt time.Time `bson:"created_at"`
	UpdatedAt time.Time `bson:"updated_at"`
	CreatedBy string    `bson:"created_by"`
	UpdatedBy string    `bson:"updated_by"`
	IsDeleted bool      `bson:"is_deleted"`
	Version   int64     `bson:"version"`
}

func (InvitationsDocument) MongoCollectionName() string {
	return invitationsMongoCollectionName
}

// InvitationReward 表示 invitations.inviterReward / inviteeReward 奖励。
type InvitationReward struct {
	CouponID string `bson:"coupon_id"`
	Amount   int64  `bson:"amount"`
}
