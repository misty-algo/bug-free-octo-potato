package entity

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// 固定时刻，保证分时段与夜间判定可复现。
var (
	earlyMorning = time.Date(2026, 10, 27, 23, 30, 0, 0, time.UTC)
	workday      = time.Date(2026, 10, 27, 10, 30, 0, 0, time.UTC)
	noon         = time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
)

func TestCalculateChargesOnlyBaseFeeWithinBaseDistance(t *testing.T) {
	rule := PricingRule{
		BaseFee:       1000,
		BaseDistanceM: 3000,
		MileageSegments: []MileageSegment{
			{FromM: 0, ToM: 0, PricePerKm: 200},
		},
	}

	got := Calculate(rule, FareInput{DistanceM: 3000})

	require.Equal(t, int64(1000), got.Total)
	require.Equal(t, int64(1000), got.BaseFee)
}

func TestCalculateAddsMileageFeeBeyondBaseDistance(t *testing.T) {
	rule := PricingRule{
		BaseFee:         1000,
		BaseDistanceM:   3000,
		MileageSegments: []MileageSegment{{FromM: 0, ToM: 0, PricePerKm: 200}},
	}

	// 超出起步里程 5000m，5000m × 200 分/km = 1000 分。
	got := Calculate(rule, FareInput{DistanceM: 8000})

	require.Equal(t, int64(2000), got.Total)
}

func TestCalculateSplitsMileageAcrossSegments(t *testing.T) {
	rule := PricingRule{
		BaseFee:       1000,
		BaseDistanceM: 3000,
		MileageSegments: []MileageSegment{
			{FromM: 0, ToM: 10000, PricePerKm: 200},
			{FromM: 10000, ToM: 0, PricePerKm: 300},
		},
	}

	// 首段 7000m×200 + 次段 13000m×300 = 5300 分。
	got := Calculate(rule, FareInput{DistanceM: 23000})

	require.Equal(t, int64(6300), got.Total)
}

func TestCalculateAddsDurationFeeBeyondBaseDuration(t *testing.T) {
	rule := PricingRule{
		BaseFee:         1000,
		BaseDistanceM:   3000,
		BaseDurationSec: 600,
		MileageSegments: []MileageSegment{{FromM: 0, ToM: 0, PricePerKm: 200}},
		DurationRules:   []DurationRule{{Start: "00:00", End: "24:00", PricePerMin: 50}},
	}

	// 超出起步时长 900 秒 = 15 分钟 × 50 分。
	got := Calculate(rule, FareInput{DistanceM: 3000, DurationSec: 1500, StartAt: workday})

	require.Equal(t, int64(1750), got.Total)
	require.Equal(t, int64(750), got.DurationFee)
}

func TestCalculateRoundsDurationUpToWholeMinutes(t *testing.T) {
	rule := PricingRule{
		BaseFee:         1000,
		BaseDistanceM:   3000,
		BaseDurationSec: 600,
		DurationRules:   []DurationRule{{Start: "00:00", End: "24:00", PricePerMin: 50}},
	}

	// 超出 90 秒不足 2 分钟，按分钟向上取整为 2 分钟。
	got := Calculate(rule, FareInput{DistanceM: 1000, DurationSec: 690, StartAt: workday})

	require.Equal(t, int64(1100), got.Total)
}

func TestCalculateMatchesDurationRuleSpanningMidnight(t *testing.T) {
	rule := PricingRule{
		BaseFee:         1000,
		BaseDistanceM:   3000,
		BaseDurationSec: 600,
		DurationRules:   []DurationRule{{Start: "22:00", End: "06:00", PricePerMin: 80}},
	}

	got := Calculate(rule, FareInput{DistanceM: 1000, DurationSec: 1500, StartAt: earlyMorning})

	require.Equal(t, int64(1200), got.DurationFee)
	require.Equal(t, int64(2200), got.Total)
}

func TestCalculateSkipsDurationFeeWhenNoPeriodMatches(t *testing.T) {
	rule := PricingRule{
		BaseFee:         1000,
		BaseDistanceM:   3000,
		BaseDurationSec: 600,
		DurationRules:   []DurationRule{{Start: "07:00", End: "09:00", PricePerMin: 80}},
	}

	// 12:00 落在早高峰之外，无可匹配时段则不收时长费，避免静默套用错误单价。
	got := Calculate(rule, FareInput{DistanceM: 1000, DurationSec: 1500, StartAt: noon})

	require.Equal(t, int64(0), got.DurationFee)
	require.Equal(t, int64(1000), got.Total)
}

func TestCalculateAddsWaitingFeeAfterFreeMinutes(t *testing.T) {
	rule := PricingRule{
		BaseFee:            1000,
		BaseDistanceM:      3000,
		WaitingFreeMinutes: 5,
		WaitingFeePerMin:   100,
		MileageSegments:    []MileageSegment{{FromM: 0, ToM: 0, PricePerKm: 200}},
	}

	// 等待 8 分钟，免 5 分钟后按 3 分钟计费。
	got := Calculate(rule, FareInput{DistanceM: 3000, WaitSec: 480})

	require.Equal(t, int64(300), got.WaitingFee)
	require.Equal(t, int64(1300), got.Total)
}

func TestCalculateChargesNoWaitingFeeWithinFreeMinutes(t *testing.T) {
	rule := PricingRule{
		BaseFee:            1000,
		BaseDistanceM:      3000,
		WaitingFreeMinutes: 5,
		WaitingFeePerMin:   100,
	}

	got := Calculate(rule, FareInput{DistanceM: 1000, WaitSec: 300})

	require.Equal(t, int64(0), got.WaitingFee)
	require.Equal(t, int64(1000), got.Total)
}

func TestCalculateAddsLongDistanceFeeBeyondThreshold(t *testing.T) {
	rule := PricingRule{
		BaseFee:          1000,
		BaseDistanceM:    3000,
		MileageSegments:  []MileageSegment{{FromM: 0, ToM: 0, PricePerKm: 200}},
		LongDistanceRule: LongDistanceRule{FromKm: 20, PricePerKm: 100},
	}

	// 25km 超出 20km 阈值 5km × 100 分。
	got := Calculate(rule, FareInput{DistanceM: 25000})

	require.Equal(t, int64(500), got.LongDistanceFee)
	require.Equal(t, int64(5900), got.Total)
}

func TestCalculateAddsNightSurchargeDuringNightHours(t *testing.T) {
	rule := PricingRule{
		BaseFee:       1000,
		BaseDistanceM: 3000,
		NightRule:     NightRule{Start: "23:00", End: "06:00", ExtraPerKm: 100},
	}

	// 8km × 100 分/km = 800 分夜间加价。
	got := Calculate(rule, FareInput{DistanceM: 8000, StartAt: earlyMorning})

	require.Equal(t, int64(800), got.NightFee)
	require.Equal(t, int64(1800), got.Total)
}

func TestCalculateSkipsNightSurchargeOutsideNightHours(t *testing.T) {
	rule := PricingRule{
		BaseFee:       1000,
		BaseDistanceM: 3000,
		NightRule:     NightRule{Start: "23:00", End: "06:00", ExtraPerKm: 100},
	}

	got := Calculate(rule, FareInput{DistanceM: 8000, StartAt: noon})

	require.Equal(t, int64(0), got.NightFee)
	require.Equal(t, int64(1000), got.Total)
}

func TestCalculateAddsLowSpeedFeeBelowSpeedThreshold(t *testing.T) {
	rule := PricingRule{
		BaseFee:         1000,
		BaseDistanceM:   3000,
		BaseDurationSec: 600,
		LowSpeedRule:    LowSpeedRule{SpeedThreshold: 12, PricePerMin: 80},
	}

	// 3km 用 1200 秒，均速 9km/h 低于 12km/h 阈值，按 20 分钟 × 80 分。
	got := Calculate(rule, FareInput{DistanceM: 3000, DurationSec: 1200, StartAt: workday})

	require.Equal(t, int64(1600), got.LowSpeedFee)
	require.Equal(t, int64(2600), got.Total)
}

func TestCalculateSkipsLowSpeedFeeWhenSpeedIsNormal(t *testing.T) {
	rule := PricingRule{
		BaseFee:       1000,
		BaseDistanceM: 3000,
		LowSpeedRule:  LowSpeedRule{SpeedThreshold: 12, PricePerMin: 80},
	}

	// 3km 用 600 秒，均速 18km/h，不触发。
	got := Calculate(rule, FareInput{DistanceM: 3000, DurationSec: 600, StartAt: workday})

	require.Equal(t, int64(0), got.LowSpeedFee)
	require.Equal(t, int64(1000), got.Total)
}

func TestCalculateClampsTotalToMaxFee(t *testing.T) {
	rule := PricingRule{
		BaseFee:         1000,
		BaseDistanceM:   0,
		MileageSegments: []MileageSegment{{FromM: 0, ToM: 0, PricePerKm: 200}},
		MaxFee:          1500,
	}

	got := Calculate(rule, FareInput{DistanceM: 10000})

	require.Equal(t, int64(3000), got.RawTotal)
	require.Equal(t, int64(1500), got.Total)
}

func TestCalculateClampsTotalUpToMinFee(t *testing.T) {
	rule := PricingRule{
		BaseFee:       100,
		BaseDistanceM: 3000,
		MinFee:        500,
	}

	got := Calculate(rule, FareInput{DistanceM: 1000})

	require.Equal(t, int64(100), got.RawTotal)
	require.Equal(t, int64(500), got.Total)
}

func TestCalculateKeepsBreakdownSumEqualToRawTotal(t *testing.T) {
	rule := PricingRule{
		BaseFee:            1000,
		BaseDistanceM:      3000,
		BaseDurationSec:    600,
		MileageSegments:    []MileageSegment{{FromM: 0, ToM: 10000, PricePerKm: 200}, {FromM: 10000, ToM: 0, PricePerKm: 300}},
		DurationRules:      []DurationRule{{Start: "00:00", End: "24:00", PricePerMin: 50}},
		LongDistanceRule:   LongDistanceRule{FromKm: 20, PricePerKm: 100},
		NightRule:          NightRule{Start: "23:00", End: "06:00", ExtraPerKm: 100},
		WaitingFreeMinutes: 5,
		WaitingFeePerMin:   100,
	}

	got := Calculate(rule, FareInput{
		DistanceM:   25000,
		DurationSec: 1500,
		WaitSec:     480,
		StartAt:     earlyMorning,
	})

	// 25km 行程：首段 [3000,10000) 7000m×200，次段 [10000,25000) 15000m×300。
	sum := got.BaseFee + got.MileageFee + got.DurationFee + got.LongDistanceFee +
		got.LowSpeedFee + got.NightFee + got.WaitingFee
	require.Equal(t, sum, got.RawTotal)
	require.Equal(t, int64(5900), got.MileageFee)
	require.Equal(t, int64(10950), got.Total)
}

func TestCalculateReturnsZeroForEmptyRule(t *testing.T) {
	got := Calculate(PricingRule{}, FareInput{})

	require.Equal(t, int64(0), got.Total)
}
