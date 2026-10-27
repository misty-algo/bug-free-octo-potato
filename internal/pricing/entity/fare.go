package entity

// 计价单位约定（团队红线）：
//   金额一律为「分」整数，距离一律为「米」，时长一律为「秒」，比率一律为「万分比」。
// 任何 float64 都不得进入金额计算路径。
//
// 取整口径（业务口径未确认前采用的默认值，变更需同步修改本注释与对应测试）：
//   按里程计费的项（里程费、远途费、夜间加价）→ 四舍五入到分；
//   按时长计费的项（时长费、等待费、低速费）→ 向上取整到整分钟。

import (
	"strconv"
	"time"
)

const (
	metersPerKm    = 1000
	secondsPerMin  = 60
	secondsPerHour = 3600
)

// MileageSegment 是一段里程区间及其单价。ToM 为 0 表示该段上不封顶。
type MileageSegment struct {
	FromM      int64
	ToM        int64
	PricePerKm int64
}

// DurationRule 是分时段时长费规则，Start/End 为 "HH:MM"，End 可用 "24:00"。
type DurationRule struct {
	Start       string
	End         string
	PricePerMin int64
}

// LongDistanceRule 是远途费规则，超过 FromKm 的部分按 PricePerKm 加收。
type LongDistanceRule struct {
	FromKm     int64
	PricePerKm int64
}

// LowSpeedRule 是低速费规则，均速低于 SpeedThreshold 时按 PricePerMin 加收。
type LowSpeedRule struct {
	SpeedThreshold int64
	PricePerMin    int64
}

// NightRule 是夜间加价规则，行程起点落在 [Start, End) 内时按 ExtraPerKm 加收。
type NightRule struct {
	Start      string
	End        string
	ExtraPerKm int64
}

// PricingRule 是计价时使用的规则投影。
type PricingRule struct {
	BaseFee          int64
	BaseDistanceM    int64
	BaseDurationSec  int64
	MileageSegments  []MileageSegment
	DurationRules    []DurationRule
	LongDistanceRule LongDistanceRule
	LowSpeedRule     LowSpeedRule
	NightRule        NightRule
	MinFee           int64
	MaxFee           int64

	WaitingFreeMinutes int64
	WaitingFeePerMin   int64

	// 佣金率，万分比整数（2000 = 20%）。计价域不做分账，只把它透传给 settlement 域。
	CommissionRateBP int64
	// 司机单笔保底收入，分。
	DriverMinIncomePerOrder int64
}

// FareInput 是单次计价的输入。
// StartAt 必须是已转换到业务时区（Asia/Shanghai）的时刻；分时段与夜间判定直接取它的时分。
type FareInput struct {
	DistanceM   int64
	DurationSec int64
	WaitSec     int64
	StartAt     time.Time
}

// FareBreakdown 是计价明细，各字段单位均为「分」。
// RawTotal 是钳制前的各分项之和，Total 是钳制后的乘客实付。
type FareBreakdown struct {
	BaseFee         int64
	MileageFee      int64
	DurationFee     int64
	LongDistanceFee int64
	LowSpeedFee     int64
	NightFee        int64
	WaitingFee      int64
	RawTotal        int64
	Total           int64
}

// Calculate 按规则计算车费明细，全程整数运算。
func Calculate(rule PricingRule, in FareInput) FareBreakdown {
	breakdown := FareBreakdown{
		BaseFee:         rule.BaseFee,
		MileageFee:      mileageFee(rule, in.DistanceM),
		DurationFee:     durationFee(rule, in.DurationSec, in.StartAt),
		LongDistanceFee: longDistanceFee(rule, in.DistanceM),
		LowSpeedFee:     lowSpeedFee(rule, in.DistanceM, in.DurationSec),
		NightFee:        nightFee(rule, in.DistanceM, in.StartAt),
		WaitingFee:      waitingFee(rule, in.WaitSec),
	}
	breakdown.RawTotal = breakdown.BaseFee + breakdown.MileageFee + breakdown.DurationFee +
		breakdown.LongDistanceFee + breakdown.LowSpeedFee + breakdown.NightFee + breakdown.WaitingFee
	breakdown.Total = clampFee(breakdown.RawTotal, rule.MinFee, rule.MaxFee)
	return breakdown
}

// mileageFee 计算超出起步里程部分的累进里程费。
func mileageFee(rule PricingRule, distanceM int64) int64 {
	if distanceM <= rule.BaseDistanceM {
		return 0
	}
	from := rule.BaseDistanceM
	to := distanceM

	// 单价是「每公里」，因此先累计「米·分」，最后统一除一次 1000，
	// 避免每个分段各自取整导致误差累积。
	var metersPriced int64
	for _, seg := range rule.MileageSegments {
		start := seg.FromM
		if start < from {
			start = from
		}
		end := to
		if seg.ToM != 0 && seg.ToM < to {
			end = seg.ToM
		}
		if end > start {
			metersPriced += (end - start) * seg.PricePerKm
		}
	}
	return roundDiv(metersPriced, metersPerKm)
}

// durationFee 计算超出起步时长部分的时长费，单价按行程开始时刻匹配分时段规则。
// 无匹配时段时不收费，避免静默套用错误单价。
func durationFee(rule PricingRule, durationSec int64, startAt time.Time) int64 {
	rule0, ok := matchDurationRule(rule.DurationRules, startAt)
	if !ok {
		return 0
	}
	billable := durationSec - rule.BaseDurationSec
	if billable <= 0 {
		return 0
	}
	return ceilDiv(billable, secondsPerMin) * rule0.PricePerMin
}

// longDistanceFee 计算超过远途阈值部分的远途费。
func longDistanceFee(rule PricingRule, distanceM int64) int64 {
	thresholdM := rule.LongDistanceRule.FromKm * metersPerKm
	if thresholdM <= 0 || distanceM <= thresholdM {
		return 0
	}
	return roundDiv((distanceM-thresholdM)*rule.LongDistanceRule.PricePerKm, metersPerKm)
}

// lowSpeedFee 在平均时速低于阈值时按整段时长加收低速费。
func lowSpeedFee(rule PricingRule, distanceM int64, durationSec int64) int64 {
	if rule.LowSpeedRule.SpeedThreshold <= 0 || durationSec <= 0 {
		return 0
	}
	if averageKmh(distanceM, durationSec) >= rule.LowSpeedRule.SpeedThreshold {
		return 0
	}
	return ceilDiv(durationSec, secondsPerMin) * rule.LowSpeedRule.PricePerMin
}

// nightFee 在行程起点处于夜间区间时按全程里程加收夜间费。
func nightFee(rule PricingRule, distanceM int64, startAt time.Time) int64 {
	if rule.NightRule.ExtraPerKm <= 0 || startAt.IsZero() {
		return 0
	}
	start, okStart := parseMinutesOfDay(rule.NightRule.Start)
	end, okEnd := parseMinutesOfDay(rule.NightRule.End)
	if !okStart || !okEnd {
		return 0
	}
	if !inMinuteRange(int64(startAt.Hour()*60+startAt.Minute()), start, end) {
		return 0
	}
	return roundDiv(distanceM*rule.NightRule.ExtraPerKm, metersPerKm)
}

// waitingFee 计算超出免费分钟后的等待费。
func waitingFee(rule PricingRule, waitSec int64) int64 {
	freeSec := rule.WaitingFreeMinutes * secondsPerMin
	billable := waitSec - freeSec
	if billable <= 0 || rule.WaitingFeePerMin <= 0 {
		return 0
	}
	return ceilDiv(billable, secondsPerMin) * rule.WaitingFeePerMin
}

// clampFee 把总额钳制到 [MinFee, MaxFee]；0 表示该侧不钳制。
func clampFee(total int64, minFee int64, maxFee int64) int64 {
	if maxFee > 0 && total > maxFee {
		return maxFee
	}
	if minFee > 0 && total < minFee {
		return minFee
	}
	return total
}

// matchDurationRule 找出覆盖该时刻的分时段规则。
func matchDurationRule(rules []DurationRule, at time.Time) (DurationRule, bool) {
	if at.IsZero() {
		return DurationRule{}, false
	}
	minuteOfDay := int64(at.Hour()*60 + at.Minute())
	for _, r := range rules {
		start, okStart := parseMinutesOfDay(r.Start)
		end, okEnd := parseMinutesOfDay(r.End)
		if !okStart || !okEnd {
			continue
		}
		if inMinuteRange(minuteOfDay, start, end) {
			return r, true
		}
	}
	return DurationRule{}, false
}

// parseMinutesOfDay 把 "HH:MM" 解析为当日分钟数，支持 "24:00"。
func parseMinutesOfDay(hhmm string) (int64, bool) {
	if len(hhmm) != 5 || hhmm[2] != ':' {
		return 0, false
	}
	hh, err := strconv.ParseInt(hhmm[0:2], 10, 64)
	if err != nil {
		return 0, false
	}
	mm, err := strconv.ParseInt(hhmm[3:5], 10, 64)
	if err != nil {
		return 0, false
	}
	return hh*60 + mm, true
}

// inMinuteRange 判断时刻是否落在 [start, end) 内，start > end 视为跨午夜。
func inMinuteRange(minuteOfDay int64, start int64, end int64) bool {
	if start <= end {
		return minuteOfDay >= start && minuteOfDay < end
	}
	return minuteOfDay >= start || minuteOfDay < end
}

// averageKmh 计算平均时速（km/h）。
func averageKmh(distanceM int64, durationSec int64) int64 {
	if durationSec <= 0 {
		return 0
	}
	return (distanceM * secondsPerHour) / (durationSec * metersPerKm)
}

// roundDiv 返回 numerator/denominator 的四舍五入结果。
func roundDiv(numerator int64, denominator int64) int64 {
	if numerator <= 0 {
		return 0
	}
	return (numerator + denominator/2) / denominator
}

// ceilDiv 返回 numerator/denominator 的向上取整结果。
func ceilDiv(numerator int64, denominator int64) int64 {
	if numerator <= 0 {
		return 0
	}
	return (numerator + denominator - 1) / denominator
}
