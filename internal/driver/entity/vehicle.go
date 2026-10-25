package entity

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// 证件类型。必填集合由 A18 发布配置提供，这里只定义编码。
const (
	DocTypeDrivingLicense    = "driving_license"
	DocTypeVehicleLicense    = "vehicle_license"
	DocTypeInsurance         = "insurance"
	DocTypeRideHailingPermit = "ride_hailing_permit"
)

// 草稿与审核版本链状态。
const (
	DraftStateDraft = "draft"

	AuditStatePending  = "pending"
	AuditStateApproved = "approved"
	AuditStateRejected = "rejected"

	BindingStateActive   = "active"
	BindingStateReleased = "released"

	DocStateActive   = "active"
	DocStateArchived = "archived"
)

// 上传约束属于实现建议值，但服务端必须同样执行。
const (
	MaxFilesPerRevision = 6
	MaxFileBytes        = 5 * 1024 * 1024
)

// 许可证类型白名单。
var AllowedImageTypes = map[string]bool{"image/jpeg": true, "image/png": true}

// NormalizeVIN 规范化车辆识别信息后参与派生与自增幂比较。
func NormalizeVIN(vin string) string {
	return strings.ToUpper(strings.TrimSpace(vin))
}

// DeriveVehicleID 由受保护的车辆识别信息派生 vehicle_id。
// 车牌只是可变属性，不能作为唯一永久车辆身份，
// 否则同一辆车换个牌照就能绕过「一辆车只绑一个有效司机」。
func DeriveVehicleID(secret, vin string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte("vehicle-id|" + NormalizeVIN(vin)))
	return "V:" + hex.EncodeToString(mac.Sum(nil))[:24]
}

// MaskVIN 只保留可核对的少量字符，库内不留存可还原的完整识别信息。
func MaskVIN(vin string) string {
	n := NormalizeVIN(vin)
	if len(n) <= 6 {
		return strings.Repeat("*", len(n))
	}
	return n[:3] + strings.Repeat("*", len(n)-7) + n[len(n)-4:]
}

// MaskDocNo 对司机响应脱敏；掩码不可再当作完整证件号入库。
func MaskDocNo(no string) string {
	n := strings.TrimSpace(no)
	if len(n) <= 4 {
		return strings.Repeat("*", len(n))
	}
	return strings.Repeat("*", len(n)-4) + n[len(n)-4:]
}

// LooksMasked 判断入参是否疑似脱敏值——这类值不能被当成完整证件号写库。
func LooksMasked(no string) bool {
	return strings.Contains(strings.TrimSpace(no), "*")
}

// NormalizePlate 规范化车牌（去分隔符与空白、统一大写）。
func NormalizePlate(plate string) string {
	return strings.ToUpper(strings.NewReplacer("·", "", "-", "", " ", "").Replace(strings.TrimSpace(plate)))
}
