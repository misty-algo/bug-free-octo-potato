package repo

import (
	"context"
	"errors"
	"time"

	"github.com/BwCloudWeGo/bw-cli/internal/order/entity"
	"github.com/BwCloudWeGo/bw-cli/internal/order/model"
	"github.com/BwCloudWeGo/bw-cli/pkg/mongox"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.uber.org/zap"
)

// MongoRepository 是 order 实体的 MongoDB 仓储实现（线上库 huazhu）。
// 约定：Save 统一走 UpsertByID（有则整文档替换，无则插入），避免新建订单被静默丢弃。
type MongoRepository struct {
	db         *mongo.Database
	store      *mongox.DocumentStore[model.OrderDocument]
	ticketSrv  *mongox.DocumentStore[model.TicketDocument]
	collection *mongox.Collection[model.OrderDocument]
	logger     *zap.Logger
}

// NewMongoRepository 基于现有 Mongo 连接创建订单仓储。
// 建议测试环境按需设置 serverSelectionTimeoutMS，避免连接不可用时长时间阻塞。
func NewMongoRepository(database *mongo.Database, loggers ...*zap.Logger) *MongoRepository {
	var logger *zap.Logger
	if len(loggers) > 0 {
		logger = loggers[0]
	}
	return &MongoRepository{
		db:         database,
		store:      mongox.NewDocumentStore[model.OrderDocument](database, loggers...),
		ticketSrv:  mongox.NewDocumentStore[model.TicketDocument](database, loggers...),
		collection: mongox.NewCollection[model.OrderDocument](database, model.OrderDocument{}.MongoCollectionName(), loggers...),
		logger:     logger,
	}
}

var (
	_ entity.Repository           = (*MongoRepository)(nil)
	_ entity.OrderQueryRepository = (*MongoRepository)(nil)
)

// Save 新建或更新（UpsertByID：有则整文档替换，无则插入）。
func (r *MongoRepository) Save(ctx context.Context, item *entity.Order) error {
	document, err := toDocument(item)
	if err != nil {
		return err
	}
	if document.ID.IsZero() {
		document.ID = bson.NewObjectID()
	}
	_, err = r.store.UpsertByID(ctx, document.ID, document)
	return err
}

// FindByID 按 _id 查询单条订单。
func (r *MongoRepository) FindByID(ctx context.Context, id string) (*entity.Order, error) {
	docID, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return nil, entity.ErrInvalidOrder
	}
	document, err := r.store.FindByID(ctx, docID)
	if err != nil {
		return nil, mapNotFound(err)
	}
	return toDomainFromDocument(document), nil
}

// FindByOrderNo 按业务号 orderNo 查询单条订单（契约 id 口径，P18 详情）。
func (r *MongoRepository) FindByOrderNo(ctx context.Context, orderNo string) (*entity.Order, error) {
	if orderNo == "" {
		return nil, entity.ErrInvalidOrder
	}
	document, err := r.store.FindOne(ctx, bson.M{"orderNo": orderNo})
	if err != nil {
		return nil, mapNotFound(err)
	}
	return toDomainFromDocument(document), nil
}

// MarkRated 原子标记已评（P19）：filter 带 _id+version 乐观锁且 rated!=true，
// 单文档一次写入 rated=true + 内嵌 rating + version+1；matchedCount=0 即已被抢先（唯一键兜底）。
func (r *MongoRepository) MarkRated(ctx context.Context, id string, version int64, rating *entity.Rating) (bool, error) {
	if id == "" || rating == nil {
		return false, entity.ErrInvalidOrder
	}
	objectID, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return false, entity.ErrInvalidOrder
	}
	filter := bson.M{"_id": objectID, "version": version, "rated": bson.M{"$ne": true}}
	update := bson.M{
		"$set": bson.M{
			"rated": true,
			"rating": &model.RatingDoc{
				Score:     rating.Score,
				Content:   rating.Content,
				Anonymous: rating.Anonymous,
				RatedAt:   rating.RatedAt,
			},
			"updatedAt": time.Now().UTC(),
		},
		"$inc": bson.M{"version": 1},
	}
	result, err := r.store.UpdateOne(ctx, filter, update)
	if err != nil {
		return false, err
	}
	return result.MatchedCount > 0, nil
}

var _ entity.RatingRepository = (*MongoRepository)(nil)

// mapNotFound 把 mongox.ErrNotFound 映射为领域错误，避免 404 落进 500。
func mapNotFound(err error) error {
	if errors.Is(err, mongox.ErrNotFound) {
		return entity.ErrOrderNotFound
	}
	return err
}

// List 按创建时间倒序分页返回订单。
func (r *MongoRepository) List(ctx context.Context, offset int, limit int) ([]*entity.Order, int64, error) {
	if limit <= 0 {
		limit = 10
	}
	if offset < 0 {
		offset = 0
	}
	total, err := r.collection.Count(ctx, bson.M{})
	if err != nil {
		return nil, 0, err
	}
	documents, err := r.collection.FindMany(ctx, bson.M{},
		options.Find().SetSkip(int64(offset)).SetLimit(int64(limit)).SetSort(bson.D{{Key: "createdAt", Value: -1}}))
	if err != nil {
		return nil, 0, err
	}
	return toDomainListFromDocuments(documents), total, nil
}

// ListFiltered 按乘客/状态过滤分页（P17）。
func (r *MongoRepository) ListFiltered(ctx context.Context, filter *entity.OrderFilter) ([]*entity.Order, int64, error) {
	if filter == nil || filter.PassengerID == "" {
		return nil, 0, entity.ErrInvalidOrder
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 10
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	mongoFilter := buildOrderFilter(filter)
	total, err := r.collection.Count(ctx, mongoFilter)
	if err != nil {
		return nil, 0, err
	}
	documents, err := r.collection.FindMany(ctx, mongoFilter,
		options.Find().SetSkip(int64(offset)).SetLimit(int64(limit)).SetSort(bson.D{{Key: "createdAt", Value: -1}}))
	if err != nil {
		return nil, 0, err
	}
	return toDomainListFromDocuments(documents), total, nil
}

// Delete 按 _id 删除订单；不存在时返回 ErrOrderNotFound（与 Gorm 版语义一致）。
func (r *MongoRepository) Delete(ctx context.Context, id string) error {
	docID, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return entity.ErrInvalidOrder
	}
	_, err = r.store.DeleteByID(ctx, docID)
	return mapNotFound(err)
}

// buildOrderFilter 组装订单列表过滤条件。
// View 口径（计划附录 E）与 entity.MatchesFilter 必须一致：
// 进行中/已完成/已取消走 state $in；「待支付」=UNPAID 或（CANCELLED 且取消费未付）。
func buildOrderFilter(filter *entity.OrderFilter) bson.M {
	mongoFilter := bson.M{}
	if filter.PassengerID != "" {
		if oid, err := bson.ObjectIDFromHex(filter.PassengerID); err == nil {
			mongoFilter["passengerId"] = oid
		} else {
			mongoFilter["passengerId"] = filter.PassengerID
		}
	}
	switch filter.View {
	case "", entity.OrderViewAll:
	case entity.OrderViewActive:
		mongoFilter["state"] = bson.M{"$in": bson.A{
			entity.OrderPending, entity.OrderAccepted, entity.OrderInService, entity.OrderInTrip,
		}}
	case entity.OrderViewDone:
		mongoFilter["state"] = bson.M{"$in": bson.A{entity.OrderEnded, entity.OrderCompleted}}
	case entity.OrderViewCancelled:
		mongoFilter["state"] = bson.M{"$in": bson.A{entity.OrderCancelled, entity.OrderNoDriver}}
	case entity.OrderViewUnpaid:
		mongoFilter["$or"] = bson.A{
			bson.M{"state": entity.OrderUnpaid},
			bson.M{
				"state": entity.OrderCancelled,
				"bills": bson.M{"$elemMatch": bson.M{
					"kind":  entity.BillKindCancelFee,
					"state": entity.BillStateUnpaid,
				}},
			},
		}
	}
	return mongoFilter
}

// toDocument entity → MongoDB 文档（旧 mapping 用 snake_case 字段，现按线上 camelCase）。
func toDocument(item *entity.Order) (*model.OrderDocument, error) {
	if item == nil {
		return nil, entity.ErrInvalidOrder
	}
	docID, err := objectIDOrZero(item.ID)
	if err != nil {
		return nil, entity.ErrInvalidOrder
	}
	doc := &model.OrderDocument{
		ID:         docID,
		OrderNo:    item.OrderNo,
		Type:       item.Type,
		TypeName:   item.TypeName,
		DistM:      item.DistM,
		DurS:       item.DurS,
		Seats:      item.Seats,
		State:      item.State,
		Version:    item.Version,
		CpFen:      item.CpFen,
		CdFen:      item.CdFen,
		Fixed:      item.Fixed,
		Rated:      item.Rated,
		Changes:    item.Changes,
		BizType:    item.BizType,
		CityCode:   item.CityCode,
		PricingVer: item.PricingVer,
		RiskLevel:  item.RiskLevel,
		SurgeRatio: item.SurgeRatio,
		CreatedAt:  item.CreatedAt,
		UpdatedAt:  item.UpdatedAt,
	}
	if item.StartLoc != nil {
		doc.StartLoc = &model.GeoPointDoc{
			Type:        item.StartLoc.Type,
			Coordinates: item.StartLoc.Coordinates,
		}
	}
	if item.Matching != nil {
		doc.Matching = bson.M(item.Matching)
	}
	if item.DispatchID != "" {
		oid, err := bson.ObjectIDFromHex(item.DispatchID)
		if err != nil {
			return nil, entity.ErrInvalidOrder
		}
		doc.DispatchID = oid
	}
	if item.PricingRule != "" {
		oid, err := bson.ObjectIDFromHex(item.PricingRule)
		if err != nil {
			return nil, entity.ErrInvalidOrder
		}
		doc.PricingRule = oid
	}
	if item.PricingSnap != "" {
		oid, err := bson.ObjectIDFromHex(item.PricingSnap)
		if err != nil {
			return nil, entity.ErrInvalidOrder
		}
		doc.PricingSnap = oid
	}
	if item.PassengerID != "" {
		oid, err := bson.ObjectIDFromHex(item.PassengerID)
		if err != nil {
			return nil, entity.ErrInvalidOrder
		}
		doc.PassengerID = oid
	}
	if item.DriverID != "" {
		oid, err := bson.ObjectIDFromHex(item.DriverID)
		if err != nil {
			return nil, entity.ErrInvalidOrder
		}
		doc.DriverID = oid
	}
	if item.StartID != "" {
		oid, err := bson.ObjectIDFromHex(item.StartID)
		if err != nil {
			return nil, entity.ErrInvalidOrder
		}
		doc.StartID = oid
	}
	if item.EndID != "" {
		oid, err := bson.ObjectIDFromHex(item.EndID)
		if err != nil {
			return nil, entity.ErrInvalidOrder
		}
		doc.EndID = oid
	}
	if item.DriverName != "" {
		doc.DriverSnap = &model.DriverSnapDoc{Name: item.DriverName}
	}
	if item.PriceSnap != nil {
		doc.PriceSnap = &model.PriceSnapshotDoc{
			T:     item.PriceSnap.T,
			Q:     item.PriceSnap.Q,
			H:     item.PriceSnap.H,
			F:     item.PriceSnap.F,
			K:     item.PriceSnap.K,
			Fixed: item.PriceSnap.Fixed,
		}
	}
	if item.AcceptedAt != nil {
		doc.AcceptedAt = item.AcceptedAt
	}
	if item.ArrivedAt != nil {
		doc.ArrivedAt = item.ArrivedAt
	}
	if item.EndedAt != nil {
		doc.EndedAt = item.EndedAt
	}
	if item.Rating != nil {
		doc.Rating = &model.RatingDoc{
			Score:     item.Rating.Score,
			Content:   item.Rating.Content,
			Anonymous: item.Rating.Anonymous,
			RatedAt:   item.Rating.RatedAt,
		}
	}
	for _, b := range item.Bills {
		doc.Bills = append(doc.Bills, billToDocument(b))
	}
	for _, rf := range item.Refunds {
		doc.Refunds = append(doc.Refunds, refundToDocument(rf))
	}
	for _, tl := range item.Timeline {
		doc.Timeline = append(doc.Timeline, model.TimelineDoc{Name: tl.Name, T: tl.T})
	}
	return doc, nil
}

// toDomainFromDocument MongoDB 文档 → entity。
func toDomainFromDocument(document *model.OrderDocument) *entity.Order {
	if document == nil {
		return nil
	}
	item := &entity.Order{
		ID:         document.ID.Hex(),
		OrderNo:    document.OrderNo,
		Type:       document.Type,
		TypeName:   document.TypeName,
		DistM:      document.DistM,
		DurS:       document.DurS,
		Seats:      document.Seats,
		State:      document.State,
		Version:    document.Version,
		CpFen:      document.CpFen,
		CdFen:      document.CdFen,
		Fixed:      document.Fixed,
		Rated:      document.Rated,
		Changes:    document.Changes,
		BizType:    document.BizType,
		CityCode:   document.CityCode,
		PricingVer: document.PricingVer,
		RiskLevel:  document.RiskLevel,
		SurgeRatio: document.SurgeRatio,
		AcceptedAt: document.AcceptedAt,
		ArrivedAt:  document.ArrivedAt,
		Timeline:   []entity.TimelineItem{},
		Refunds:    []entity.Refund{},
		Bills:      []entity.Bill{},
		CreatedAt:  document.CreatedAt,
		UpdatedAt:  document.UpdatedAt,
	}
	if document.PriceSnap != nil {
		item.PriceSnap = &entity.PriceSnapshot{
			T:     document.PriceSnap.T,
			Q:     document.PriceSnap.Q,
			H:     document.PriceSnap.H,
			F:     document.PriceSnap.F,
			K:     document.PriceSnap.K,
			Fixed: document.PriceSnap.Fixed,
		}
	}
	// I1：零值 ObjectID 往返映射为空串，避免 "000000000000000000000000" 污染领域对象。
	if !document.StartID.IsZero() {
		item.StartID = document.StartID.Hex()
	}
	if !document.EndID.IsZero() {
		item.EndID = document.EndID.Hex()
	}
	if !document.DispatchID.IsZero() {
		item.DispatchID = document.DispatchID.Hex()
	}
	if !document.PricingRule.IsZero() {
		item.PricingRule = document.PricingRule.Hex()
	}
	if !document.PricingSnap.IsZero() {
		item.PricingSnap = document.PricingSnap.Hex()
	}
	if document.StartLoc != nil {
		item.StartLoc = &entity.GeoPoint{
			Type:        document.StartLoc.Type,
			Coordinates: document.StartLoc.Coordinates,
		}
	}
	if document.Matching != nil {
		item.Matching = map[string]any(document.Matching)
	}
	if !document.PassengerID.IsZero() {
		item.PassengerID = document.PassengerID.Hex()
	}
	if !document.DriverID.IsZero() {
		item.DriverID = document.DriverID.Hex()
	}
	if document.DriverSnap != nil {
		item.DriverName = document.DriverSnap.Name
	}
	item.EndedAt = document.EndedAt
	if document.Rating != nil {
		item.Rating = &entity.Rating{
			Score:     document.Rating.Score,
			Content:   document.Rating.Content,
			Anonymous: document.Rating.Anonymous,
			RatedAt:   document.Rating.RatedAt,
		}
	}
	for _, b := range document.Bills {
		item.Bills = append(item.Bills, billToDomain(b))
	}
	for _, rf := range document.Refunds {
		item.Refunds = append(item.Refunds, refundToDomain(rf))
	}
	for _, tl := range document.Timeline {
		item.Timeline = append(item.Timeline, entity.TimelineItem{Name: tl.Name, T: tl.T})
	}
	return item
}

func toDomainListFromDocuments(documents []model.OrderDocument) []*entity.Order {
	items := make([]*entity.Order, 0, len(documents))
	for i := range documents {
		items = append(items, toDomainFromDocument(&documents[i]))
	}
	return items
}

// ticketToDocument 工单 entity → 文档（非法 ID 显式报错，不静默吞掉）。
func ticketToDocument(item *entity.Ticket) (*model.TicketDocument, error) {
	if item == nil {
		return nil, entity.ErrInvalidOrder
	}
	docID, err := objectIDOrZero(item.ID)
	if err != nil {
		return nil, entity.ErrInvalidOrder
	}
	doc := &model.TicketDocument{
		ID:        docID,
		TicketNo:  item.TicketNo,
		Content:   item.Content,
		Evidences: item.Evidences,
		State:     item.State,
		Round:     item.Round,
		Rounds:    []model.TicketRound{},
		MergedNos: []string{},
		CreatedAt: item.CreatedAt,
	}
	if item.OrderID != "" {
		oid, err := bson.ObjectIDFromHex(item.OrderID)
		if err != nil {
			return nil, entity.ErrInvalidOrder
		}
		doc.OrderID = oid
	}
	doc.OrderNo = item.OrderNo
	if item.ComplainID != "" {
		oid, err := bson.ObjectIDFromHex(item.ComplainID)
		if err != nil {
			return nil, entity.ErrInvalidOrder
		}
		doc.ComplainID = oid
	}
	if item.DriverID != "" {
		oid, err := bson.ObjectIDFromHex(item.DriverID)
		if err != nil {
			return nil, entity.ErrInvalidOrder
		}
		doc.DriverID = oid
	}
	doc.Type = item.Type
	for _, ev := range item.Timeline {
		at := ev.At
		doc.Timeline = append(doc.Timeline, model.TicketEvent{
			Seq: ev.Seq, Actor: ev.Actor, Action: ev.Action,
			From: ev.From, To: ev.To, Message: ev.Message, At: &at,
		})
	}
	return doc, nil
}

// ticketToDomainFromDocument 工单文档 → entity。
func ticketToDomainFromDocument(document *model.TicketDocument) *entity.Ticket {
	if document == nil {
		return nil
	}
	item := &entity.Ticket{
		ID:        document.ID.Hex(),
		TicketNo:  document.TicketNo,
		OrderNo:   document.OrderNo,
		Type:      document.Type,
		Content:   document.Content,
		Evidences: document.Evidences,
		State:     document.State,
		Round:     document.Round,
		CreatedAt: document.CreatedAt,
	}
	// I1：零值 ObjectID 映射为空串，保证往返一致。
	if !document.OrderID.IsZero() {
		item.OrderID = document.OrderID.Hex()
	}
	if !document.ComplainID.IsZero() {
		item.ComplainID = document.ComplainID.Hex()
	}
	if !document.DriverID.IsZero() {
		item.DriverID = document.DriverID.Hex()
	}
	for _, ev := range document.Timeline {
		e := entity.TicketEvent{
			Seq: ev.Seq, Actor: ev.Actor, Action: ev.Action,
			From: ev.From, To: ev.To, Message: ev.Message,
		}
		if ev.At != nil {
			e.At = *ev.At
		}
		item.Timeline = append(item.Timeline, e)
	}
	return item
}

func billToDocument(b entity.Bill) model.BillDoc {
	return model.BillDoc{
		Kind:                  b.Kind,
		TotalFee:              b.TotalFee,
		DriverGrossFee:        b.DriverGrossFee,
		PlatformServiceFee:    b.PlatformServiceFee,
		SurgeFee:              b.SurgeFee,
		CouponFee:             b.CouponFee,
		PlatformCouponBearFee: b.PlatformCouponBearFee,
		DriverCouponBearFee:   b.DriverCouponBearFee,
		PayableFee:            b.PayableFee,
		DriverIncomeFee:       b.DriverIncomeFee,
		PlatformNetFee:        b.PlatformNetFee,
		State:                 b.State,
		PaidAt:                b.PaidAt,
		Channel:               b.Channel,
	}
}

func billToDomain(d model.BillDoc) entity.Bill {
	return entity.Bill{
		Kind:                  d.Kind,
		TotalFee:              d.TotalFee,
		DriverGrossFee:        d.DriverGrossFee,
		PlatformServiceFee:    d.PlatformServiceFee,
		SurgeFee:              d.SurgeFee,
		CouponFee:             d.CouponFee,
		PlatformCouponBearFee: d.PlatformCouponBearFee,
		DriverCouponBearFee:   d.DriverCouponBearFee,
		PayableFee:            d.PayableFee,
		DriverIncomeFee:       d.DriverIncomeFee,
		PlatformNetFee:        d.PlatformNetFee,
		State:                 d.State,
		PaidAt:                d.PaidAt,
		Channel:               d.Channel,
	}
}

func refundToDocument(r entity.Refund) model.RefundDoc {
	return model.RefundDoc{
		RefundFee:       r.RefundFee,
		DriverRefundFee: r.DriverRefundFee,
		Mode:            r.Mode,
		State:           r.State,
		T:               r.T,
	}
}

func refundToDomain(d model.RefundDoc) entity.Refund {
	return entity.Refund{
		RefundFee:       d.RefundFee,
		DriverRefundFee: d.DriverRefundFee,
		Mode:            d.Mode,
		State:           d.State,
		T:               d.T,
	}
}

// objectIDOrZero 空串 → 零值 ObjectID（新建）；非法 hex → 错误。
func objectIDOrZero(id string) (bson.ObjectID, error) {
	if id == "" {
		return bson.NilObjectID, nil
	}
	return bson.ObjectIDFromHex(id)
}
