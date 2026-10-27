package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/BwCloudWeGo/bw-cli/internal/pricing/entity"
)

type fakeSnapshots struct {
	rule entity.PricingRule
	err  error
}

func (f fakeSnapshots) SnapshotByOrder(_ context.Context, _ string) (entity.PricingRule, error) {
	return f.rule, f.err
}

func TestCalculateBillUsesRuleSnapshotOfOrder(t *testing.T) {
	rule := entity.PricingRule{
		BaseFee:          1000,
		BaseDistanceM:    3000,
		MileageSegments:  []entity.MileageSegment{{FromM: 0, ToM: 0, PricePerKm: 200}},
		CommissionRateBP: 2000,
	}
	svc := New(fakeSnapshots{rule: rule})

	got, err := svc.CalculateBill(context.Background(), CalculateBillCommand{
		OrderNo:   "ORDER-1",
		DistanceM: 8000,
	})

	require.NoError(t, err)
	require.Equal(t, "ORDER-1", got.OrderNo)
	require.Equal(t, int64(2000), got.PassengerPaid)
}

func TestCalculateBillFailsWhenSnapshotMissing(t *testing.T) {
	svc := New(fakeSnapshots{err: entity.ErrRuleSnapshotNotFound})

	_, err := svc.CalculateBill(context.Background(), CalculateBillCommand{OrderNo: "ORDER-1"})

	require.ErrorIs(t, err, entity.ErrRuleSnapshotNotFound)
}

func TestCalculateBillCarriesCommissionRateForSettlement(t *testing.T) {
	rule := entity.PricingRule{BaseFee: 1000, CommissionRateBP: 2000}
	svc := New(fakeSnapshots{rule: rule})

	got, err := svc.CalculateBill(context.Background(), CalculateBillCommand{OrderNo: "ORDER-1"})

	require.NoError(t, err)
	// 计价域不做分账，只把万分比佣金率透传给 settlement 域。
	require.Equal(t, int64(2000), got.CommissionRateBP)
}

func TestCalculateBillKeepsBillingRequestIDForTraceability(t *testing.T) {
	svc := New(fakeSnapshots{rule: entity.PricingRule{BaseFee: 1000}})

	got, err := svc.CalculateBill(context.Background(), CalculateBillCommand{
		OrderNo:          "ORDER-1",
		BillingRequestID: "BR:abc",
	})

	require.NoError(t, err)
	require.Equal(t, "BR:abc", got.BillingRequestID)
}
