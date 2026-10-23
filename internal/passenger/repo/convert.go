package repo

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"

	"github.com/BwCloudWeGo/bw-cli/internal/passenger/entity"
	"github.com/BwCloudWeGo/bw-cli/internal/passenger/model"
)

// 敏感字段加密相关错误。
var (
	ErrEncryptionKeyMissing = errors.New("phone encryption key is not configured")
	ErrEncryptionFailed     = errors.New("phone encryption failed")
)

// phoneKey 保存手机号加密密钥。必须在服务启动时通过 SetPhoneKey 注入。
var phoneKey []byte

// SetPhoneKey 注入手机号加密密钥。
//
// 密钥 MUST NOT 硬编码在代码里，MUST NOT 提交到仓库，
// 从配置中心或环境变量读入。
func SetPhoneKey(key string) error {
	if key == "" {
		return ErrEncryptionKeyMissing
	}
	phoneKey = []byte(key)
	return nil
}

// HashPhone 计算手机号哈希，用于等值查询与唯一索引。
//
// 哈希不带盐，保证同一手机号的哈希稳定，可直接建唯一索引。
func HashPhone(phone string) string {
	sum := sha256.Sum256([]byte(phone))
	return hex.EncodeToString(sum[:])
}

// EncryptPhone 加密手机号，落库用密文。
//
// 使用 AES-256-GCM。密钥未配置时返回错误而不是静默降级为明文。
func EncryptPhone(phone string) (string, error) {
	if len(phoneKey) == 0 {
		return "", ErrEncryptionKeyMissing
	}
	block, err := aes.NewCipher(phoneKey)
	if err != nil {
		return "", ErrEncryptionFailed
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", ErrEncryptionFailed
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", ErrEncryptionFailed
	}
	sealed := gcm.Seal(nonce, nonce, []byte(phone), nil)
	return hex.EncodeToString(sealed), nil
}

// DecryptPhone 解密手机号。仅在确有需要时调用，接口返回一律用脱敏值。
func DecryptPhone(encrypted string) (string, error) {
	if len(phoneKey) == 0 {
		return "", ErrEncryptionKeyMissing
	}
	raw, err := hex.DecodeString(encrypted)
	if err != nil {
		return "", ErrEncryptionFailed
	}
	block, err := aes.NewCipher(phoneKey)
	if err != nil {
		return "", ErrEncryptionFailed
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", ErrEncryptionFailed
	}
	if len(raw) < gcm.NonceSize() {
		return "", ErrEncryptionFailed
	}
	nonce, cipherText := raw[:gcm.NonceSize()], raw[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, cipherText, nil)
	if err != nil {
		return "", ErrEncryptionFailed
	}
	return string(plain), nil
}

// toAccountDocument 把账号实体转成落库文档。
func toAccountDocument(account *entity.Account) *model.AccountDocument {
	encrypted, err := EncryptPhone(account.Phone)
	if err != nil {
		// 密钥未配置时落空值并置未删除标记，由启动校验兜住。
		encrypted = ""
	}
	return &model.AccountDocument{
		ID:              account.ID,
		PhoneEnc:        encrypted,
		PhoneHash:       HashPhone(account.Phone),
		PhoneMask:       entity.MaskPhone(account.Phone),
		Status:          string(account.Status),
		Nickname:        account.Nickname,
		Avatar:          account.Avatar,
		PasswordHash:    account.PasswordHash,
		RealNameVerified: account.RealNameVerified,
		CreatedAt:       account.CreatedAt,
		UpdatedAt:       account.UpdatedAt,
	}
}

// toAccountEntity 把落库文档转成账号实体。
func toAccountEntity(document *model.AccountDocument) *entity.Account {
	return &entity.Account{
		ID:               document.ID,
		Phone:            document.PhoneMask,
		Status:           entity.AccountStatus(document.Status),
		Nickname:         document.Nickname,
		Avatar:           document.Avatar,
		PasswordHash:     document.PasswordHash,
		RealNameVerified: document.RealNameVerified,
		CreatedAt:        document.CreatedAt,
		UpdatedAt:        document.UpdatedAt,
	}
}

// toChannelEntity 把落库文档转成渠道绑定实体。
func toChannelEntity(document *model.ChannelBindingDocument) *entity.ChannelBinding {
	return &entity.ChannelBinding{
		ID:         document.ID,
		AccountID:  document.AccountID,
		Channel:    entity.Channel(document.Channel),
		ChannelUID: document.ChannelUID,
		CreatedAt:  document.CreatedAt,
		UpdatedAt:  document.UpdatedAt,
	}
}

// toEstimateEntity 把落库文档转成估价单实体。
func toEstimateEntity(document *model.EstimateDocument) *entity.Estimate {
	return &entity.Estimate{
		ID:             document.ID,
		No:             document.No,
		AccountID:      document.AccountID,
		CarType:        document.CarType,
		BaseFee:        document.BaseFee,
		ServiceFee:     document.ServiceFee,
		DiscountFee:    document.DiscountFee,
		PayFee:         document.PayFee,
		DistanceM:      document.DistanceM,
		DurationS:      document.DurationS,
		PricingVersion: document.PricingVersion,
		CouponID:       document.CouponID,
		Used:           document.Used,
		ExpiresAt:      document.ExpiresAt,
		CreatedAt:      document.CreatedAt,
		UpdatedAt:      document.UpdatedAt,
	}
}

// toAddressEntity 把落库文档转成地址实体。
func toAddressEntity(document *model.AddressDocument) *entity.Address {
	return &entity.Address{
		ID:         document.ID,
		AccountID:  document.AccountID,
		Type:       entity.AddressType(document.Type),
		Tag:        document.Tag,
		Name:       document.Name,
		Addr:       document.Addr,
		POIID:      document.POIID,
		Lng:        document.Lng,
		Lat:        document.Lat,
		UseCount:   document.UseCount,
		LastUsedAt: document.LastUsedAt,
		CreatedAt:  document.CreatedAt,
		UpdatedAt:  document.UpdatedAt,
	}
}

// toOrderDocument 把订单实体转成落库文档。
func toOrderDocument(order *entity.Order) *model.OrderDocument {
	return &model.OrderDocument{
		ID:              order.ID,
		OrderNo:         order.OrderNo,
		AccountID:       order.AccountID,
		State:           string(order.State),
		CarType:         order.CarType,
		StartName:       order.StartName,
		StartAddr:       order.StartAddr,
		StartLng:        order.StartLng,
		StartLat:        order.StartLat,
		EndName:         order.EndName,
		EndAddr:         order.EndAddr,
		EndLng:          order.EndLng,
		EndLat:          order.EndLat,
		EstimateNo:      order.EstimateNo,
		TotalFee:        order.TotalFee,
		CouponFee:       order.CouponFee,
		CancelFee:       order.CancelFee,
		Remark:          order.Remark,
		PricingSnapshot: order.PricingSnapshot,
		IdempotencyKey:  order.IdempotencyKey,
		Version:         order.Version,
		CreatedAt:       order.CreatedAt,
		UpdatedAt:       order.UpdatedAt,
	}
}

// toOrderEntity 把落库文档转成订单实体。
func toOrderEntity(document *model.OrderDocument) *entity.Order {
	return &entity.Order{
		ID:              document.ID,
		OrderNo:         document.OrderNo,
		AccountID:       document.AccountID,
		State:           entity.OrderState(document.State),
		CarType:         document.CarType,
		StartName:       document.StartName,
		StartAddr:       document.StartAddr,
		StartLng:        document.StartLng,
		StartLat:        document.StartLat,
		EndName:         document.EndName,
		EndAddr:         document.EndAddr,
		EndLng:          document.EndLng,
		EndLat:          document.EndLat,
		EstimateNo:      document.EstimateNo,
		TotalFee:        document.TotalFee,
		CouponFee:       document.CouponFee,
		CancelFee:       document.CancelFee,
		Remark:          document.Remark,
		PricingSnapshot: document.PricingSnapshot,
		IdempotencyKey:  document.IdempotencyKey,
		Version:         document.Version,
		CreatedAt:       document.CreatedAt,
		UpdatedAt:       document.UpdatedAt,
	}
}
