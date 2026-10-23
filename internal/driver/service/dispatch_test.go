package service

import (
	"testing"
	"time"

	"github.com/BwCloudWeGo/bw-cli/internal/driver/entity"
	"github.com/BwCloudWeGo/bw-cli/pkg/biztime"
)

// 派单候选排序规则：本月累计收入升序（挣得少的优先）→ 综合评分降序 → 司机编号升序。
func TestRankCandidates(t *testing.T) {
	svc := &Service{cfg: BusinessConfig{Weights: DispatchWeights{Distance: 4000, Rating: 2500, Online: 1500, Success: 2000}}}
	order := &entity.HallOrder{Origin: entity.Location{Lat: 30.657, Lng: 104.081}}

	poor := &entity.Driver{ID: "d-poor", DriverNo: "D000001", Status: entity.DriverStatusIdle,
		MonthlyIncomeFen: 1000, Rating: 4.0, SuccessRate: 0.8, OnlineSinceAt: biztime.UTC().Add(-time.Hour),
		LastLat: 30.660, LastLng: 104.085}
	rich := &entity.Driver{ID: "d-rich", DriverNo: "D000002", Status: entity.DriverStatusIdle,
		MonthlyIncomeFen: 9000, Rating: 5.0, SuccessRate: 1.0, OnlineSinceAt: biztime.UTC().Add(-time.Hour),
		LastLat: 30.657, LastLng: 104.081}

	ranked := svc.rankCandidates([]*entity.Driver{rich, poor}, order)
	if ranked[0].ID != "d-poor" {
		t.Fatalf("收入更低的司机应排在前面，实际首位 %s", ranked[0].ID)
	}

	// 收入相同时，评分高者（距离更近）优先。
	near := &entity.Driver{ID: "d-near", DriverNo: "D000003", MonthlyIncomeFen: 1000,
		Rating: 5.0, SuccessRate: 1.0, OnlineSinceAt: biztime.UTC().Add(-2 * time.Hour),
		LastLat: 30.6570, LastLng: 104.0810}
	far := &entity.Driver{ID: "d-far", DriverNo: "D000004", MonthlyIncomeFen: 1000,
		Rating: 3.0, SuccessRate: 0.5, OnlineSinceAt: biztime.UTC().Add(-time.Minute),
		LastLat: 30.700, LastLng: 104.150}
	ranked = svc.rankCandidates([]*entity.Driver{far, near}, order)
	if ranked[0].ID != "d-near" {
		t.Fatalf("评分更高的司机应排在前面，实际首位 %s", ranked[0].ID)
	}
	if svc.scoreCandidate(near, order) <= svc.scoreCandidate(far, order) {
		t.Fatal("距离更近的司机得分应更高")
	}
}

// 候选过滤：车型/座位/城市/接单区域任一不满足即淘汰。
func TestFilterCandidate(t *testing.T) {
	svc := &Service{}
	order := &entity.HallOrder{CityCode: "510100", RequireSeats: 5, RequireCarType: "comfort",
		Origin: entity.Location{Lat: 30.657, Lng: 104.081}}
	d, _ := entity.NewDriver("候选司机", "13800000002")
	d.CityCode, d.Seats, d.CarType = "510100", 4, "comfort"
	if svc.filterCandidate(d, order) {
		t.Fatal("座位数不满足要求时不应入选")
	}
	d.Seats = 5
	if !svc.filterCandidate(d, order) {
		t.Fatal("满足车型/座位/区域时应入选")
	}
	d.AreaLimitSwitch = true
	d.Areas = []entity.AreaBox{{Name: "北京", MinLat: 39.8, MinLng: 116.3, MaxLat: 40.0, MaxLng: 116.5}}
	if svc.filterCandidate(d, order) {
		t.Fatal("起点不在接单区域时不应入选")
	}
}
