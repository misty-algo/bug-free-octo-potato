package model

import "time"

const partnersMongoCollectionName = "partners"

// PartnersDocument 是 partners 集合（加盟商 CP）的 MongoDB 文档模型。
type PartnersDocument struct {
	ID              string    `bson:"_id"`
	PartnerNo       string    `bson:"partner_no"`
	CompanyName     string    `bson:"company_name"`
	LicenseNo       string    `bson:"license_no"`
	LegalPerson     string    `bson:"legal_person"`
	ContactName     string    `bson:"contact_name"`
	ContactPhoneEnc string    `bson:"contact_phone_enc"`
	CityCodes       []string  `bson:"city_codes"`
	Deposit         int64     `bson:"deposit"`
	CommissionRate  float64   `bson:"commission_rate"`
	SettleCycle     string    `bson:"settle_cycle"`
	Status          string    `bson:"status"`
	ContractStartAt time.Time `bson:"contract_start_at"`
	ContractEndAt   time.Time `bson:"contract_end_at"`
	InviteCode      string    `bson:"invite_code"`

	CreatedAt time.Time `bson:"created_at"`
	UpdatedAt time.Time `bson:"updated_at"`
	CreatedBy string    `bson:"created_by"`
	UpdatedBy string    `bson:"updated_by"`
	IsDeleted bool      `bson:"is_deleted"`
	Version   int64     `bson:"version"`
}

func (PartnersDocument) MongoCollectionName() string {
	return partnersMongoCollectionName
}

const settlementsMongoCollectionName = "settlements"

// SettlementsDocument 是 settlements 集合（结算单）的 MongoDB 文档模型。
type SettlementsDocument struct {
	ID            string    `bson:"_id"`
	SettleNo      string    `bson:"settle_no"`
	TargetType    string    `bson:"target_type"`
	TargetID      string    `bson:"target_id"`
	PeriodStart   time.Time `bson:"period_start"`
	PeriodEnd     time.Time `bson:"period_end"`
	OrderCount    int64     `bson:"order_count"`
	GrossAmount   int64     `bson:"gross_amount"`
	Commission    int64     `bson:"commission"`
	RewardAmount  int64     `bson:"reward_amount"`
	PenaltyAmount int64     `bson:"penalty_amount"`
	NetAmount     int64     `bson:"net_amount"`
	InvoiceStatus string    `bson:"invoice_status"`
	Status        string    `bson:"status"`
	PaidAt        time.Time `bson:"paid_at"`
	Remark        string    `bson:"remark"`

	CreatedAt time.Time `bson:"created_at"`
	UpdatedAt time.Time `bson:"updated_at"`
	CreatedBy string    `bson:"created_by"`
	UpdatedBy string    `bson:"updated_by"`
	IsDeleted bool      `bson:"is_deleted"`
	Version   int64     `bson:"version"`
}

func (SettlementsDocument) MongoCollectionName() string {
	return settlementsMongoCollectionName
}
