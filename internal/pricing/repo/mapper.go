package repo

import (
	"github.com/BwCloudWeGo/bw-cli/internal/pricing/entity"
	"github.com/BwCloudWeGo/bw-cli/internal/shop/model"
)

// 存储模型的距离单位口径：base_distance 与 mileage_segments 的分段端点以「公里」表达，
// 领域实体内部一律换算为「米」，避免单位混用。base_duration 以「秒」表达。

// ruleFromDocument 把计价规则的存储文档转换为领域实体。
// 它只做单位换算与字段搬运，不含任何业务判断。
func ruleFromDocument(doc model.PricingRulesDocument) entity.PricingRule {
	rule := entity.PricingRule{
		BaseFee:                 doc.BaseFee,
		BaseDistanceM:           kilometersToMeters(doc.BaseDistance),
		BaseDurationSec:         doc.BaseDuration,
		MinFee:                  doc.MinFee,
		MaxFee:                  doc.MaxFee,
		WaitingFreeMinutes:      doc.WaitingFreeMinutes,
		WaitingFeePerMin:        doc.WaitingFeePerMin,
		CommissionRateBP:        doc.CommissionRateBP,
		DriverMinIncomePerOrder: doc.DriverMinIncomePerOrder,
	}
	rule.MileageSegments = mileageSegmentsFromDocument(doc.MileageSegments)
	rule.DurationRules = durationRulesFromDocument(doc.DurationRules)
	rule.LongDistanceRule = entity.LongDistanceRule{
		FromKm:     doc.LongDistanceRule.FromKm,
		PricePerKm: doc.LongDistanceRule.PricePerKm,
	}
	rule.LowSpeedRule = entity.LowSpeedRule{
		SpeedThreshold: doc.LowSpeedRule.SpeedThreshold,
		PricePerMin:    doc.LowSpeedRule.PricePerMin,
	}
	rule.NightRule = entity.NightRule{
		Start:      doc.NightRule.Start,
		End:        doc.NightRule.End,
		ExtraPerKm: doc.NightRule.ExtraPerKm,
	}
	return rule
}

// mileageSegmentsFromDocument 把公里分段的端点换算为米。ToKm 为 0 表示上不封顶，换算后保持 0。
func mileageSegmentsFromDocument(segments []model.MileageSegment) []entity.MileageSegment {
	if len(segments) == 0 {
		return nil
	}
	out := make([]entity.MileageSegment, 0, len(segments))
	for _, seg := range segments {
		out = append(out, entity.MileageSegment{
			FromM:      kilometersToMeters(seg.FromKm),
			ToM:        kilometersToMeters(seg.ToKm),
			PricePerKm: seg.PricePerKm,
		})
	}
	return out
}

// durationRulesFromDocument 搬运分时段时长费规则。
func durationRulesFromDocument(rules []model.DurationRule) []entity.DurationRule {
	if len(rules) == 0 {
		return nil
	}
	out := make([]entity.DurationRule, 0, len(rules))
	for _, r := range rules {
		out = append(out, entity.DurationRule{
			Start:       r.Start,
			End:         r.End,
			PricePerMin: r.PricePerMin,
		})
	}
	return out
}

// kilometersToMeters 把公里换算为米；0 保持 0，以延续「上不封顶」的哨兵语义。
func kilometersToMeters(km int64) int64 {
	if km == 0 {
		return 0
	}
	return km * 1000
}
