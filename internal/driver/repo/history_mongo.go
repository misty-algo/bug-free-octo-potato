package repo

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/BwCloudWeGo/bw-cli/internal/driver/entity"
	dbmodel "github.com/BwCloudWeGo/bw-cli/internal/driver/model"
)

// historyRepo 跨域只读：从本域 hall_orders 取司机接单记录，
// 再从订单域 orders / tickets 与乘客域 passenger_reviews 取乘客评价与投诉。
// 乘客端页面的评价接口写入 passenger_reviews，而不是 orders.rating，
// 两处都要读才能覆盖全部评价来源（同一物理库，直接读集合）。
type historyRepo struct {
	db *mongo.Database
}

// NewHistoryRepository 创建接单记录仓储。
func NewHistoryRepository(db *mongo.Database) entity.HistoryRepository {
	return &historyRepo{db: db}
}

// hallOrderLite hall_orders 的精简投影。
type hallOrderLite struct {
	OrderID        string  `bson:"order_id"`
	PassengerName  string  `bson:"passenger_name"`
	PassengerCount int     `bson:"passenger_count"`
	Origin         struct {
		Name string  `bson:"name"`
		Lat  float64 `bson:"lat"`
		Lng  float64 `bson:"lng"`
	} `bson:"origin"`
	Destination struct {
		Name string  `bson:"name"`
		Lat  float64 `bson:"lat"`
		Lng  float64 `bson:"lng"`
	} `bson:"destination"`
	Status          string    `bson:"status"`
	DriverIncomeFen int64     `bson:"driver_income_fen"`
	CreatedAt       time.Time `bson:"created_at"`
	EndedAt         time.Time `bson:"ended_at"`
}

// orderRatingLite orders 中 rating 子文档的精简投影。
type orderRatingLite struct {
	OrderNo string `bson:"orderNo"`
	Rating  *struct {
		Score     int32     `bson:"score"`
		Content   string    `bson:"content"`
		Anonymous bool      `bson:"anonymous"`
		RatedAt   time.Time `bson:"ratedAt"`
	} `bson:"rating"`
}

// ticketLite tickets 的精简投影。
type ticketLite struct {
	OrderNo string    `bson:"orderNo"`
	Type    string    `bson:"type"`
	Content string    `bson:"content"`
	State   string    `bson:"state"`
	CreatedAt time.Time `bson:"createdAt"`
}

// passengerReviewLite passenger_reviews 的精简投影。
// 乘客域用 snake_case 字段名，与订单域 orders / tickets 的 camelCase 不同。
type passengerReviewLite struct {
	OrderNo       string    `bson:"order_no"`
	Rating        int32     `bson:"rating"`
	Comment       string    `bson:"comment"`
	IsComplaint   bool      `bson:"is_complaint"`
	ComplaintType string    `bson:"complaint_type"`
	ComplaintDesc string    `bson:"complaint_desc"`
	CreatedAt     time.Time `bson:"created_at"`
}

// mergePassengerReviews 把乘客域评价/投诉并入接单记录的评分与投诉映射。
// orders.rating / tickets 已有值时不覆盖，避免同一订单派生出两条重复通知。
func mergePassengerReviews(
	docs []passengerReviewLite,
	ratings map[string]*entity.HistoryRating,
	complaints map[string]*entity.HistoryComplaint,
) {
	for _, d := range docs {
		if d.OrderNo == "" {
			continue
		}
		if _, ok := ratings[d.OrderNo]; !ok {
			ratings[d.OrderNo] = &entity.HistoryRating{
				Score:   d.Rating,
				Content: d.Comment,
				RatedAt: d.CreatedAt,
			}
		}
		if !d.IsComplaint {
			continue
		}
		if _, ok := complaints[d.OrderNo]; !ok {
			complaints[d.OrderNo] = &entity.HistoryComplaint{
				Type:    d.ComplaintType,
				Content: d.ComplaintDesc,
				State:   "待处理",
				At:      d.CreatedAt,
			}
		}
	}
}

// ListTakenOrders 返回司机接过的订单（ACCEPTED/ARRIVED/IN_TRIP/ENDED），并批量补上评价/投诉。
func (r *historyRepo) ListTakenOrders(ctx context.Context, driverID string, limit int) ([]*entity.HistoryOrder, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	cur, err := r.db.Collection(dbmodel.HallOrderCollectionName).Find(ctx, bson.M{
		"driver_id": driverID,
		"status":    bson.M{"$in": []string{"ACCEPTED", "ARRIVED", "IN_TRIP", "ENDED"}},
	}, options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}}).SetLimit(int64(limit)))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var docs []hallOrderLite
	if err := cur.All(ctx, &docs); err != nil {
		return nil, err
	}
	if len(docs) == 0 {
		return []*entity.HistoryOrder{}, nil
	}

	orderNos := make([]string, 0, len(docs))
	for _, d := range docs {
		orderNos = append(orderNos, d.OrderID)
	}

	// 批量取评价（只取有 rating 子文档的订单）。
	ratingMap := make(map[string]*entity.HistoryRating)
	rcur, err := r.db.Collection("orders").Find(ctx, bson.M{
		"orderNo": orderNos,
		"rating":  bson.M{"$exists": true},
	}, options.Find().SetProjection(bson.M{"orderNo": 1, "rating": 1}))
	if err == nil {
		defer rcur.Close(ctx)
		var rdocs []orderRatingLite
		if rErr := rcur.All(ctx, &rdocs); rErr == nil {
			for _, d := range rdocs {
				if d.Rating != nil {
					ratingMap[d.OrderNo] = &entity.HistoryRating{
						Score:     d.Rating.Score,
						Content:   d.Rating.Content,
						Anonymous: d.Rating.Anonymous,
						RatedAt:   d.Rating.RatedAt,
					}
				}
			}
		}
	}

	// 批量取投诉（按该司机 + 订单号）。
	complaintMap := make(map[string]*entity.HistoryComplaint)
	tcur, err := r.db.Collection("tickets").Find(ctx, bson.M{
		"orderNo":  orderNos,
		"driverId": driverID,
	}, options.Find().SetProjection(bson.M{"orderNo": 1, "type": 1, "content": 1, "state": 1, "createdAt": 1}).
		SetSort(bson.D{{Key: "createdAt", Value: -1}}))
	if err == nil {
		defer tcur.Close(ctx)
		var tdocs []ticketLite
		if tErr := tcur.All(ctx, &tdocs); tErr == nil {
			// 同一订单可能有多条工单，取最新一条。
			for _, d := range tdocs {
				if _, ok := complaintMap[d.OrderNo]; ok {
					continue
				}
				complaintMap[d.OrderNo] = &entity.HistoryComplaint{
					Type:    d.Type,
					Content: d.Content,
					State:   d.State,
					At:      d.CreatedAt,
				}
			}
		}
	}

	// 乘客端页面的评价/投诉写入 passenger_reviews，按同一批订单号批量补齐。
	// 排序后按「先到先留」合并，同一订单即使有多条也只派生一次通知。
	pcur, err := r.db.Collection("passenger_reviews").Find(ctx, bson.M{
		"order_no": bson.M{"$in": orderNos},
	}, options.Find().SetSort(bson.D{{Key: "created_at", Value: 1}}))
	if err == nil {
		defer pcur.Close(ctx)
		var pdocs []passengerReviewLite
		if pErr := pcur.All(ctx, &pdocs); pErr == nil {
			mergePassengerReviews(pdocs, ratingMap, complaintMap)
		}
	}

	out := make([]*entity.HistoryOrder, 0, len(docs))
	for _, d := range docs {
		ho := &entity.HistoryOrder{
			OrderID:        d.OrderID,
			PassengerName:  d.PassengerName,
			PassengerCount: d.PassengerCount,
			Origin:         entity.Location{Name: d.Origin.Name, Lat: d.Origin.Lat, Lng: d.Origin.Lng},
			Destination:    entity.Location{Name: d.Destination.Name, Lat: d.Destination.Lat, Lng: d.Destination.Lng},
			Status:         d.Status,
			DriverIncomeFen: d.DriverIncomeFen,
			CreatedAt:      d.CreatedAt,
			EndedAt:        d.EndedAt,
		}
		if rt, ok := ratingMap[d.OrderID]; ok {
			ho.Rating = rt
		}
		if cp, ok := complaintMap[d.OrderID]; ok {
			ho.Complaint = cp
		}
		out = append(out, ho)
	}
	return out, nil
}
