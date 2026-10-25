package entity

import (
	"math"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

const (
	// VerifyMaxAttempts 连续第 5 次有效错误即锁定（接口文档 3.9.2）。
	VerifyMaxAttempts = 5
	// VerifyLockDuration 锁定时长，服务端当前时间 + 10 分钟。
	VerifyLockDuration = 10 * time.Minute
	// VerifyCodeLength 手机尾号后四位。
	VerifyCodeLength = 4
	// BoardWaitLimit 等待满 5 分钟仍未上车者需先通过有证据的取消/爽约处理，
	// 不能隐藏一个乘客卡直接发车。
	BoardWaitLimit = 5 * time.Minute
	// BillingUploadWindow 结束后轨迹补传窗口（技术建议值 120 秒）。
	BillingUploadWindow = 120 * time.Second
)

// 计费输入的两阶段状态：结束事务先落 COLLECTING，轨迹齐且质检通过后冻结
// 不可变输入快照并置 READY，计价域只消费 READY 快照。
const (
	BillingCollecting     = "COLLECTING"
	BillingReady          = "READY"
	BillingReviewRequired = "REVIEW_REQUIRED"
)

const (
	SessionActive = "ACTIVE"
	SessionClosed = "CLOSED"
)

// 轨迹样本质量结论。阈值由配置提供，不在代码里凭空固定生产阈值。
const (
	SampleOK       = "OK"
	SampleSuspect  = "SUSPECT"
	SampleRejected = "REJECTED"
)

// SampleThresholds 由地图/计价契约配置注入。
type SampleThresholds struct {
	MaxAccuracyMeters float64
	MaxSpeedMPS       float64
}

func DefaultSampleThresholds() SampleThresholds {
	return SampleThresholds{MaxAccuracyMeters: 100, MaxSpeedMPS: 50}
}

// TripSession 采集会话：多标签页通过会话登记与主采集租约避免重复累计里程。
type TripSession struct {
	ID         bson.ObjectID `bson:"_id,omitempty"`
	TaskNo     string        `bson:"taskNo"`
	SessionID  string        `bson:"sessionId"`
	DriverID   bson.ObjectID `bson:"driverId"`
	State      string        `bson:"state"`
	LastSeq    int64         `bson:"lastSeq"`
	Watermark  int64         `bson:"watermark"`
	LeaseUntil *time.Time    `bson:"leaseUntil,omitempty"`
	CreatedAt  time.Time     `bson:"createdAt"`
	UpdatedAt  time.Time     `bson:"updatedAt"`
}

// TripPoint 一个轨迹样本。服务端以 taskNo+sessionId+seq 唯一去重，
// 重放不重复累计里程。
type TripPoint struct {
	TaskNo      string    `bson:"taskNo"`
	SessionID   string    `bson:"sessionId"`
	Seq         int64     `bson:"seq"`
	CollectedAt time.Time `bson:"collectedAt"`
	ReceivedAt  time.Time `bson:"receivedAt"`
	CRS         string    `bson:"crs"`
	Accuracy    *float64  `bson:"accuracy,omitempty"`
	Location    GeoPoint  `bson:"loc"`
	Quality     string    `bson:"quality"`
	Note        string    `bson:"note,omitempty"`
}

// VerifyMutation 是核验命令写入的全部事实。成功与失败都只原子更新订单文档，
// 不产生需要回滚的中间授权票据。
type VerifyMutation struct {
	Order       RideOrder
	Verified    bool
	Failures    int
	LockedUntil *time.Time
	Timeline    TimelineItem
	Events      []EventLog
	Command     CommandResult
}

// StartMutation 是开始行程写入的全部事实。
type StartMutation struct {
	TaskNo     string
	DepartedAt time.Time
	Orders     []RideOrder
	Session    TripSession
	Events     []EventLog
	Command    CommandResult
}

// EndMutation 是送达写入的全部事实。ReleaseDriver 由行程域判定：
// 任务上最后一个活动子单终结时才释放一次。
// DriverBlocked 表示释放时该司机已不符合准入，收车而不是置空闲。
type EndMutation struct {
	Order            RideOrder
	EndedAt          time.Time
	ActualDist       int64
	ActualDur        int64
	EndReason        string
	BillingRequestID string
	InputState       string
	Timeline         TimelineItem
	Events           []EventLog
	Command          CommandResult
	ReleaseDriver    bool
	DriverBlocked    bool
	DriverID         bson.ObjectID
}

// SampleMutation 是轨迹样本写入的全部事实。
type SampleMutation struct {
	Point     TripPoint
	SessionID string
	TaskNo    string
	// UpdateLatest 仅在该点比会话最新序号更新时置位，避免乱序旧点回退司机最新位置。
	UpdateLatest bool
	DriverID     bson.ObjectID
	LatestAt     time.Time
}

const (
	pi    = 3.1415926535897932384626
	aAxis = 6378245.0
	eESq  = 0.00669342162296594323
)

func outOfChina(lng, lat float64) bool {
	return lng < 72.004 || lng > 137.8347 || lat < 0.8293 || lat > 55.8271
}

func transformLat(x, y float64) float64 {
	ret := -100.0 + 2.0*x + 3.0*y + 0.2*y*y + 0.1*x*y + 0.2*math.Sqrt(math.Abs(x))
	ret += (20.0*math.Sin(6.0*x*pi) + 20.0*math.Sin(2.0*x*pi)) * 2.0 / 3.0
	ret += (20.0*math.Sin(y*pi) + 40.0*math.Sin(y/3.0*pi)) * 2.0 / 3.0
	ret += (160.0*math.Sin(y/12.0*pi) + 320*math.Sin(y*pi/30.0)) * 2.0 / 3.0
	return ret
}

func transformLng(x, y float64) float64 {
	ret := 300.0 + x + 2.0*y + 0.1*x*x + 0.1*x*y + 0.1*math.Sqrt(math.Abs(x))
	ret += (20.0*math.Sin(6.0*x*pi) + 20.0*math.Sin(2.0*x*pi)) * 2.0 / 3.0
	ret += (20.0*math.Sin(x*pi) + 40.0*math.Sin(x/3.0*pi)) * 2.0 / 3.0
	ret += (150.0*math.Sin(x/12.0*pi) + 300.0*math.Sin(x/30.0*pi)) * 2.0 / 3.0
	return ret
}

// WGS84ToGCJ02 把浏览器定位（通常 WGS84）统一为 GCJ02。转换只在服务端完成，
// 转换不可靠的样本拒绝用于到达及计费。
func WGS84ToGCJ02(lng, lat float64) (float64, float64, bool) {
	if math.IsNaN(lng) || math.IsNaN(lat) || outOfChina(lng, lat) {
		return lng, lat, false
	}
	dLat := transformLat(lng-105.0, lat-35.0)
	dLng := transformLng(lng-105.0, lat-35.0)
	radLat := lat / 180.0 * pi
	magic := math.Sin(radLat)
	magic = 1 - eESq*magic*magic
	sqrtMagic := math.Sqrt(magic)
	mLat := (dLat * 180.0) / ((aAxis * (1 - eESq)) / (magic * sqrtMagic) * pi)
	mLng := (dLng * 180.0) / (aAxis / sqrtMagic * math.Cos(radLat) * pi)
	return lng + mLng, lat + mLat, true
}

// EvaluateSample 对样本做时间顺序、精度与速度/连续性检查。结论为
// OK / SUSPECT / REJECTED；进入复核的样本仍保留证据，不静默丢点。
func EvaluateSample(previous *TripPoint, next TripPoint, th SampleThresholds) string {
	if next.Accuracy != nil && th.MaxAccuracyMeters > 0 && *next.Accuracy > th.MaxAccuracyMeters {
		return SampleRejected
	}
	if previous == nil {
		return SampleOK
	}
	dt := next.CollectedAt.Sub(previous.CollectedAt)
	if dt <= 0 {
		// 时间倒流或重复采集：不用于计费，但保留证据。
		return SampleRejected
	}
	from, okFrom := previous.Location.Coordinate()
	to, okTo := next.Location.Coordinate()
	if !okFrom || !okTo {
		return SampleRejected
	}
	meters, err := DistanceMeters(from, to)
	if err != nil {
		return SampleRejected
	}
	if th.MaxSpeedMPS > 0 && meters/dt.Seconds() > th.MaxSpeedMPS {
		return SampleSuspect
	}
	return SampleOK
}
