package repo

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/BwCloudWeGo/bw-cli/internal/order/entity"
)

const safetyEventsCollection = "safety_events"

// dangerousDrivingEventType 是安全事件 raw.eventType（危险驾驶投诉桥接）。
// TODO(D17 联调): driver 域 knownEventTypes 需加入该值（现有字典无危险驾驶项）。
const dangerousDrivingEventType = "dangerous_driving"

// CreateTicket 幂等建单（P19）：按 ticketNo upsert（$setOnInsert 整文档），
// 同键同号重试不重复建单（created=false 表示已存在，调用方读回原单返回）。
func (r *MongoRepository) CreateTicket(ctx context.Context, ticket *entity.Ticket) (bool, error) {
	if ticket == nil || ticket.TicketNo == "" {
		return false, entity.ErrInvalidOrder
	}
	doc, err := ticketToDocument(ticket)
	if err != nil {
		return false, err
	}
	result, err := r.ticketSrv.UpdateOne(ctx,
		bson.M{"ticketNo": ticket.TicketNo},
		bson.M{"$setOnInsert": doc},
		options.UpdateOne().SetUpsert(true),
	)
	if err != nil {
		return false, err
	}
	return result.UpsertedCount > 0, nil
}

// FindTicketByTicketNo 按业务号 ticketNo 查询工单（幂等重试读回原单）。
func (r *MongoRepository) FindTicketByTicketNo(ctx context.Context, ticketNo string) (*entity.Ticket, error) {
	if ticketNo == "" {
		return nil, entity.ErrInvalidOrder
	}
	doc, err := r.ticketSrv.FindOne(ctx, bson.M{"ticketNo": ticketNo})
	if err != nil {
		return nil, mapNotFound(err)
	}
	return ticketToDomainFromDocument(doc), nil
}

// ListTicketsByComplainant 按投诉人查工单（P22，D4 归属过滤），createdAt 倒序。
func (r *MongoRepository) ListTicketsByComplainant(ctx context.Context, complainantID string) ([]*entity.Ticket, error) {
	oid, err := bson.ObjectIDFromHex(complainantID)
	if err != nil {
		return nil, entity.ErrInvalidOrder
	}
	docs, err := r.ticketSrv.FindMany(ctx,
		bson.M{"complainantId": oid},
		options.Find().SetSort(bson.D{{Key: "createdAt", Value: -1}}),
	)
	if err != nil {
		return nil, err
	}
	out := make([]*entity.Ticket, 0, len(docs))
	for i := range docs {
		out = append(out, ticketToDomainFromDocument(&docs[i]))
	}
	return out, nil
}

// RecordDangerousDriving 记安全事件 + 事件日志（P19 危险驾驶投诉）：
// safety_events 按 eventId 去重，state=RECEIVED 入值班队列（处置流转归 driver 域 D17）；
// event_logs 按 eventId 去重，与 trip 域事件日志同集合同口径。
func (r *MongoRepository) RecordDangerousDriving(ctx context.Context, report *entity.SafetyEventReport, log *entity.EventLog) error {
	if report == nil || log == nil || report.EventID == "" {
		return entity.ErrInvalidOrder
	}
	doc := bson.M{
		"eventId":  report.EventID,
		"driverId": report.DriverID,
		"state":    "RECEIVED",
		"version":  int64(1),
		"handler":  "",
		"raw": bson.M{
			"eventType":       dangerousDrivingEventType,
			"description":     report.Description,
			"taskId":          "",
			"position":        bson.M{"source": "UNKNOWN"},
			"clientRequestId": report.ClientRequestID,
			"requestHash":     "",
			"receivedAt":      report.ReceivedAt,
			"ticketNo":        report.TicketNo,
			"orderNo":         report.OrderNo,
		},
		"actions":     []bson.M{},
		"escalations": []bson.M{},
		"outbox":      []bson.M{},
		"supplements": []bson.M{},
		"attachments": []bson.M{},
		"auditTrail":  []bson.M{},
	}
	if _, err := r.db.Collection(safetyEventsCollection).InsertOne(ctx, doc); err != nil && !mongo.IsDuplicateKeyError(err) {
		return err
	}
	if _, err := r.db.Collection(eventLogsCollection).InsertOne(ctx, log); err != nil && !mongo.IsDuplicateKeyError(err) {
		return err
	}
	return nil
}

var (
	_ entity.TicketRepository      = (*MongoRepository)(nil)
	_ entity.SafetyEventRepository = (*MongoRepository)(nil)
)
