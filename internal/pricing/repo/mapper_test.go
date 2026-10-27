package repo

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/BwCloudWeGo/bw-cli/internal/pricing/entity"
	"github.com/BwCloudWeGo/bw-cli/internal/shop/model"
)

func TestRuleFromDocumentConvertsKilometersToMeters(t *testing.T) {
	doc := model.PricingRulesDocument{
		BaseFee:          1000,
		BaseDistance:     3,
		BaseDuration:     600,
		MileageSegments:  []model.MileageSegment{{FromKm: 0, ToKm: 10, PricePerKm: 200}, {FromKm: 10, ToKm: 0, PricePerKm: 300}},
		LongDistanceRule: model.LongDistanceRule{FromKm: 20, PricePerKm: 100},
	}

	got := ruleFromDocument(doc)

	require.Equal(t, int64(3000), got.BaseDistanceM)
	require.Equal(t, int64(0), got.MileageSegments[0].FromM)
	require.Equal(t, int64(10000), got.MileageSegments[0].ToM)
	require.Equal(t, int64(10000), got.MileageSegments[1].FromM)
	// ToKm 为 0 表示上不封顶，换算后仍保持 0。
	require.Equal(t, int64(0), got.MileageSegments[1].ToM)
	require.Equal(t, int64(20000), got.LongDistanceRule.FromKm*1000)
}

func TestRuleFromDocumentCarriesCommissionRateInBasisPoints(t *testing.T) {
	doc := model.PricingRulesDocument{CommissionRateBP: 2000}

	got := ruleFromDocument(doc)

	require.Equal(t, int64(2000), got.CommissionRateBP)
}

func TestRuleFromDocumentMapsTimeBasedRules(t *testing.T) {
	doc := model.PricingRulesDocument{
		BaseDuration:       600,
		DurationRules:      []model.DurationRule{{Start: "07:00", End: "09:00", PricePerMin: 80}},
		LowSpeedRule:       model.LowSpeedRule{SpeedThreshold: 12, PricePerMin: 80},
		NightRule:          model.NightRule{Start: "23:00", End: "06:00", ExtraPerKm: 100},
		WaitingFreeMinutes: 5,
		WaitingFeePerMin:   100,
		MinFee:             500,
		MaxFee:             30000,
	}

	got := ruleFromDocument(doc)

	require.Equal(t, int64(600), got.BaseDurationSec)
	require.Equal(t, "07:00", got.DurationRules[0].Start)
	require.Equal(t, int64(80), got.DurationRules[0].PricePerMin)
	require.Equal(t, int64(12), got.LowSpeedRule.SpeedThreshold)
	require.Equal(t, "23:00", got.NightRule.Start)
	require.Equal(t, int64(100), got.NightRule.ExtraPerKm)
	require.Equal(t, int64(5), got.WaitingFreeMinutes)
	require.Equal(t, int64(100), got.WaitingFeePerMin)
	require.Equal(t, int64(500), got.MinFee)
	require.Equal(t, int64(30000), got.MaxFee)
}

func TestRuleFromDocumentReturnsZeroRuleForEmptyDocument(t *testing.T) {
	got := ruleFromDocument(model.PricingRulesDocument{})

	require.Equal(t, entity.PricingRule{}, got)
}
