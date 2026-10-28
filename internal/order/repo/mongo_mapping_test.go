package repo

import (
	"testing"
	"time"

	"github.com/BwCloudWeGo/bw-cli/internal/order/entity"
	"github.com/BwCloudWeGo/bw-cli/internal/order/model"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// oid 断言用 ObjectID 构造。
func oid(t *testing.T, hex string) bson.ObjectID {
	t.Helper()
	id, err := bson.ObjectIDFromHex(hex)
	require.NoError(t, err)
	return id
}

func TestOrderDocumentRoundTrip(t *testing.T) {
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	in := &entity.Order{
		ID:          "6720f1a2b3c4d5e6f7a8b9c0",
		OrderNo:     "P0001",
		PassengerID: "6720f1a2b3c4d5e6f7a8b9c1",
		Type:        1,
		TypeName:    "快车",
		StartID:     "6720f1a2b3c4d5e6f7a8b9c2",
		EndID:       "6720f1a2b3c4d5e6f7a8b9c3",
		DistM:       12000,
		DurS:        1800,
		Seats:       4,
		State:       "ACCEPTED",
		Version:     3,
		CpFen:       500,
		CdFen:       300,
		Fixed:       true,
		PriceSnap:   &entity.PriceSnapshot{T: 1800, Q: 500, H: 200, F: 1200},
		DriverID:    "6720f1a2b3c4d5e6f7a8b9c4",
		DriverName:  "王师傅",
		Rated:       true,
		Changes:     1,
		CreatedAt:   now,
		AcceptedAt:  &now,
		ArrivedAt:   &now,
		UpdatedAt:   now,
		CityCode:    "310100",
		SurgeRatio:  1.5,
		Rating:      &entity.Rating{Score: 5, Content: "好评", Anonymous: true, RatedAt: now},
		Timeline:    []entity.TimelineItem{{Name: "created", T: now}},
		Bills:       []entity.Bill{{TotalFee: 1000, PayableFee: 900}},
		Refunds:     []entity.Refund{{RefundFee: 100, T: now}},
	}

	doc, err := toDocument(in)
	require.NoError(t, err)
	require.Equal(t, "P0001", doc.OrderNo)
	require.Equal(t, oid(t, "6720f1a2b3c4d5e6f7a8b9c0"), doc.ID)
	require.Equal(t, oid(t, "6720f1a2b3c4d5e6f7a8b9c4"), doc.DriverID)
	require.Equal(t, "王师傅", doc.DriverSnap.Name)
	require.NotNil(t, doc.Rating)
	require.Equal(t, int32(5), doc.Rating.Score)
	require.Len(t, doc.Bills, 1)
	require.Equal(t, int64(900), doc.Bills[0].PayableFee)
	require.Len(t, doc.Refunds, 1)
	require.Len(t, doc.Timeline, 1)

	out := toDomainFromDocument(doc)
	require.Equal(t, in.ID, out.ID)
	require.Equal(t, in.OrderNo, out.OrderNo)
	require.Equal(t, in.DriverID, out.DriverID)
	require.Equal(t, "王师傅", out.DriverName)
	require.Equal(t, in.State, out.State)
	require.Equal(t, in.Version, out.Version)
	require.Equal(t, in.CpFen, out.CpFen)
	require.Equal(t, in.SurgeRatio, out.SurgeRatio)
	require.NotNil(t, out.Rating)
	require.Equal(t, int32(5), out.Rating.Score)
	require.Len(t, out.Bills, 1)
	require.Len(t, out.Refunds, 1)
	require.Len(t, out.Timeline, 1)
}

func TestToDocumentInvalidID(t *testing.T) {
	_, err := toDocument(&entity.Order{ID: "not-an-objectid"})
	require.Error(t, err)
}

func TestBuildOrderFilter(t *testing.T) {
	pid := bson.NewObjectID()
	// 「进行中」视图：state $in 四态
	filter := buildOrderFilter(&entity.OrderFilter{PassengerID: pid.Hex(), View: entity.OrderViewActive, Limit: 20, Offset: 10})
	require.Equal(t, pid, filter["passengerId"])
	require.Equal(t, bson.M{"$in": bson.A{
		entity.OrderPending, entity.OrderAccepted, entity.OrderInService, entity.OrderInTrip,
	}}, filter["state"])

	// 「待支付」视图：UNPAID 或（CANCELLED 且取消费未付），口径与 entity.MatchesFilter 一致
	filter = buildOrderFilter(&entity.OrderFilter{PassengerID: pid.Hex(), View: entity.OrderViewUnpaid})
	require.Equal(t, bson.A{
		bson.M{"state": entity.OrderUnpaid},
		bson.M{
			"state": entity.OrderCancelled,
			"bills": bson.M{"$elemMatch": bson.M{"kind": entity.BillKindCancelFee, "state": entity.BillStateUnpaid}},
		},
	}, filter["$or"])

	// 「已取消」视图：CANCELLED/NO_DRIVER
	filter = buildOrderFilter(&entity.OrderFilter{PassengerID: pid.Hex(), View: entity.OrderViewCancelled})
	require.Equal(t, bson.M{"$in": bson.A{entity.OrderCancelled, entity.OrderNoDriver}}, filter["state"])

	// 「已完成」视图：ENDED/COMPLETED
	filter = buildOrderFilter(&entity.OrderFilter{PassengerID: pid.Hex(), View: entity.OrderViewDone})
	require.Equal(t, bson.M{"$in": bson.A{entity.OrderEnded, entity.OrderCompleted}}, filter["state"])

	// 视图不传/「全部」时不限定 state
	filter = buildOrderFilter(&entity.OrderFilter{PassengerID: pid.Hex()})
	_, hasState := filter["state"]
	require.False(t, hasState)
}

// TestOrderDocumentFullRoundTrip 全结构往返保真（含 C3 补齐的 8 个线上字段）。
func TestOrderDocumentFullRoundTrip(t *testing.T) {
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	in := &entity.Order{
		ID:          "6720f1a2b3c4d5e6f7a8b9c0",
		OrderNo:     "P0001",
		PassengerID: "6720f1a2b3c4d5e6f7a8b9c1",
		Type:        1,
		TypeName:    "快车",
		StartID:     "6720f1a2b3c4d5e6f7a8b9c2",
		EndID:       "6720f1a2b3c4d5e6f7a8b9c3",
		StartLoc:    &entity.GeoPoint{Type: "Point", Coordinates: []float64{121.47, 31.23}},
		DistM:       12000,
		DurS:        1800,
		Seats:       4,
		State:       "ACCEPTED",
		Version:     3,
		CpFen:       500,
		CdFen:       300,
		Fixed:       true,
		PriceSnap:   &entity.PriceSnapshot{T: 1800, Q: 500, H: 200, F: 1200, K: 0, Fixed: true},
		DriverID:    "6720f1a2b3c4d5e6f7a8b9c4",
		DriverName:  "王师傅",
		Rated:       true,
		Changes:     1,
		DispatchID:  "6720f1a2b3c4d5e6f7a8b9c5",
		BizType:     2,
		CityCode:    "310100",
		PricingRule: "6720f1a2b3c4d5e6f7a8b9c6",
		PricingSnap: "6720f1a2b3c4d5e6f7a8b9c7",
		PricingVer:  "v3",
		RiskLevel:   "LOW",
		SurgeRatio:  1.5,
		Matching:    map[string]any{"batchId": "b1"},
		CreatedAt:   now,
		AcceptedAt:  &now,
		ArrivedAt:   &now,
		UpdatedAt:   now,
		Rating:      &entity.Rating{Score: 5, Content: "好评", Anonymous: true, RatedAt: now},
		Timeline:    []entity.TimelineItem{{Name: "created", T: now}},
		Bills:       []entity.Bill{{TotalFee: 1000, PayableFee: 900}},
		Refunds:     []entity.Refund{{RefundFee: 100, T: now}},
	}

	doc, err := toDocument(in)
	require.NoError(t, err)
	out := toDomainFromDocument(doc)
	require.Equal(t, in, out)
}

// TestOrderDocumentBSONKeys 校验线上 camelCase 字段名未被改动。
func TestOrderDocumentBSONKeys(t *testing.T) {
	doc := &model.OrderDocument{
		StartLoc:    &model.GeoPointDoc{Type: "Point", Coordinates: []float64{1, 2}},
		DispatchID:  bson.NewObjectID(),
		BizType:     1,
		PricingRule: bson.NewObjectID(),
		PricingSnap: bson.NewObjectID(),
		PricingVer:  "v1",
		RiskLevel:   "LOW",
		Matching:    bson.M{"a": 1},
	}
	raw, err := bson.Marshal(doc)
	require.NoError(t, err)
	var m bson.M
	require.NoError(t, bson.Unmarshal(raw, &m))
	for _, key := range []string{"startLocation", "dispatchId", "bizType", "pricingRuleId", "pricingSnapshotId", "pricingVersion", "riskLevel", "matching"} {
		require.Contains(t, m, key)
	}
}

// TestOrderZeroObjectIDRoundTrip 零值 ObjectID 往返为空串（I1）。
func TestOrderZeroObjectIDRoundTrip(t *testing.T) {
	in := &entity.Order{OrderNo: "P0002"}
	doc, err := toDocument(in)
	require.NoError(t, err)
	out := toDomainFromDocument(doc)
	require.Equal(t, "", out.StartID)
	require.Equal(t, "", out.EndID)
	require.Equal(t, "", out.DispatchID)
	require.Equal(t, "", out.PricingRule)
	require.Equal(t, "", out.PricingSnap)
	require.Equal(t, in.OrderNo, out.OrderNo)
}

func TestTicketDocumentRoundTrip(t *testing.T) {
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	in := &entity.Ticket{
		ID:         "6720f1a2b3c4d5e6f7a8b9d0",
		TicketNo:   "TK0001",
		OrderID:    "6720f1a2b3c4d5e6f7a8b9c0",
		OrderNo:    "P0001",
		ComplainID: "6720f1a2b3c4d5e6f7a8b9c1",
		DriverID:   "6720f1a2b3c4d5e6f7a8b9c4",
		Type:       entity.TicketTypeRouteDetour,
		Content:    "司机绕路",
		Evidences:  []string{"http://img/1.jpg"},
		State:      entity.TicketPending,
		Timeline:   []entity.TicketEvent{{Seq: 1, Actor: "pax", Action: "create", At: now}},
		CreatedAt:  now,
	}

	doc, err := ticketToDocument(in)
	require.NoError(t, err)
	require.Equal(t, "TK0001", doc.TicketNo)
	require.Equal(t, oid(t, "6720f1a2b3c4d5e6f7a8b9d0"), doc.ID)
	require.Equal(t, entity.TicketPending, doc.State)

	out := ticketToDomainFromDocument(doc)
	require.Equal(t, in.ID, out.ID)
	require.Equal(t, in.TicketNo, out.TicketNo)
	require.Equal(t, in.OrderID, out.OrderID)
	require.Equal(t, in.ComplainID, out.ComplainID)
	require.Equal(t, in.DriverID, out.DriverID)
	require.Equal(t, in.Content, out.Content)
	require.Equal(t, in.State, out.State)
	require.Len(t, out.Timeline, 1)
	require.Equal(t, int32(1), out.Timeline[0].Seq)
}

func TestTicketToDocumentInvalidID(t *testing.T) {
	_, err := ticketToDocument(&entity.Ticket{ID: "not-an-objectid", Content: "x"})
	require.Error(t, err)
	_, err = ticketToDocument(&entity.Ticket{OrderID: "bad", Content: "x"})
	require.Error(t, err)
	_, err = ticketToDocument(nil)
	require.Error(t, err)
}
