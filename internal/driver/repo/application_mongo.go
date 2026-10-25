package repo

import (
	"context"
	"errors"
	"time"

	"github.com/BwCloudWeGo/bw-cli/internal/driver/entity"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const (
	colApplications = "vehicle_applications"
	colCounters     = "driver_rejection_counters"
	colAdmission    = "driver_admission"
)

// ErrDuplicateKey 是 entity 层哨兵的别名：调用方据此映射成业务冲突码，
// 而不是先查询再插入——索引才是并发下的唯一真相。
var ErrDuplicateKey = entity.ErrDuplicateKey

// Create 插入申请聚合根。activeSlot 上的唯一索引保证同一司机只有一个活动变更。
func (r *MongoVehicleRepository) Create(ctx context.Context, app *entity.VehicleApplication) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	if _, err := r.db.Collection(colApplications).InsertOne(ctx, app); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return ErrDuplicateKey
		}
		return err
	}
	return nil
}

func (r *MongoVehicleRepository) ByID(ctx context.Context, applicationID string) (*entity.VehicleApplication, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	var app entity.VehicleApplication
	err := r.db.Collection(colApplications).FindOne(ctx, bson.M{"applicationId": applicationID}).Decode(&app)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, err
	}
	return &app, nil
}

// ActiveByDriver 只返回占用"活动变更"名额的申请（pending/reviewing）。
func (r *MongoVehicleRepository) ActiveByDriver(ctx context.Context, driverID string) (*entity.VehicleApplication, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	var app entity.VehicleApplication
	err := r.db.Collection(colApplications).FindOne(ctx, bson.M{"activeSlot": driverID}).Decode(&app)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, err
	}
	return &app, nil
}

// LatestByDriver 返回司机最近一条申请（含终态），用于展示历史 revision。
func (r *MongoVehicleRepository) LatestByDriver(ctx context.Context, driverID string) (*entity.VehicleApplication, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	var app entity.VehicleApplication
	err := r.db.Collection(colApplications).FindOne(ctx, bson.M{"driverId": driverID},
		options.FindOne().SetSort(bson.D{{Key: "createdAt", Value: -1}})).Decode(&app)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, err
	}
	return &app, nil
}

// AppendRevision 只在"当前版本与审核代数同时匹配、且处于可重新提交的终态"时成功。
func (r *MongoVehicleRepository) AppendRevision(ctx context.Context, in entity.AppendRevisionInput) (bool, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	filter := bson.M{
		"applicationId": in.ApplicationID,
		"driverId":      in.DriverID,
		"state":         bson.M{"$in": []string{entity.AppStateRejected, entity.AppStateWithdrawn}},
		"revision":      in.ExpectedRevision,
		"reviewVersion": in.ExpectedReviewVersion,
	}
	update := bson.M{
		"$set": bson.M{
			"state":          entity.AppStatePending,
			"activeRevision": in.Revision.Revision,
			// 新 revision 没有任何审核动作，代数归零。
			"reviewVersion": int64(0),
			"reviewerId":    "",
			"vehicleId":     in.Revision.Payload.VehicleID,
			"activeSlot":    in.DriverID,
			"updatedAt":     in.Revision.CreatedAt,
		},
		"$inc": bson.M{"revision": int64(1)},
		"$push": bson.M{
			"revisions":  in.Revision,
			"auditTrail": in.Entry,
			"outbox":     bson.M{"$each": in.Outbox},
		},
	}
	res, err := r.db.Collection(colApplications).UpdateOne(ctx, filter, update)
	if err != nil {
		return false, err
	}
	return res.MatchedCount == 1, nil
}

// Claim 是领取命令：并发下只有一个审核员能匹配 pending + 代数。
func (r *MongoVehicleRepository) Claim(ctx context.Context, in entity.TransitionInput) (bool, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	filter := bson.M{
		"applicationId":  in.ApplicationID,
		"activeRevision": in.Revision,
		"reviewVersion":  in.ReviewVersion,
		"state":          entity.AppStatePending,
	}
	next := in.ReviewVersion + 1
	update := bson.M{
		"$set": bson.M{
			"state":                    entity.AppStateReviewing,
			"reviewerId":               in.ActorID,
			"revisions.$[r].state":     entity.AppStateReviewing,
			"revisions.$[r].updatedAt": in.At,
			"updatedAt":                in.At,
		},
		"$inc": bson.M{"reviewVersion": int64(1)},
		"$push": bson.M{
			"revisions.$[r].assignments": entity.Assignment{
				Action: entity.ActionClaim, ToID: in.ActorID, ReviewVersion: next, At: in.At,
			},
			"auditTrail": in.Entry,
			"outbox":     bson.M{"$each": in.Outbox},
		},
	}
	opts := options.UpdateOne().SetArrayFilters([]any{
		bson.M{"r.revision": in.Revision, "r.state": entity.AppStatePending},
	})
	res, err := r.db.Collection(colApplications).UpdateOne(ctx, filter, update, opts)
	if err != nil {
		return false, err
	}
	return res.MatchedCount == 1, nil
}

// Transfer 只换领取人并留痕，不产生裁决。
func (r *MongoVehicleRepository) Transfer(ctx context.Context, in entity.TransitionInput, to, reason string) (bool, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	filter := bson.M{
		"applicationId":  in.ApplicationID,
		"activeRevision": in.Revision,
		"reviewVersion":  in.ReviewVersion,
		"state":          entity.AppStateReviewing,
		"reviewerId":     in.ActorID,
	}
	next := in.ReviewVersion + 1
	update := bson.M{
		"$set": bson.M{
			"reviewerId":               to,
			"revisions.$[r].updatedAt": in.At,
			"updatedAt":                in.At,
		},
		"$inc": bson.M{"reviewVersion": int64(1)},
		"$push": bson.M{
			"revisions.$[r].assignments": entity.Assignment{
				Action: entity.ActionTransfer, FromID: in.ActorID, ToID: to,
				Reason: reason, ReviewVersion: next, At: in.At,
			},
			"auditTrail": in.Entry,
			"outbox":     bson.M{"$each": in.Outbox},
		},
	}
	opts := options.UpdateOne().SetArrayFilters([]any{
		bson.M{"r.revision": in.Revision, "r.state": entity.AppStateReviewing},
	})
	res, err := r.db.Collection(colApplications).UpdateOne(ctx, filter, update, opts)
	if err != nil {
		return false, err
	}
	return res.MatchedCount == 1, nil
}

// Withdraw 仅限本人当前 pending/reviewing 版本，与裁决竞争只能一方生效。
func (r *MongoVehicleRepository) Withdraw(ctx context.Context, in entity.TransitionInput) (bool, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	filter := bson.M{
		"applicationId":  in.ApplicationID,
		"driverId":       in.ActorID,
		"activeRevision": in.Revision,
		"reviewVersion":  in.ReviewVersion,
		"state":          bson.M{"$in": []string{entity.AppStatePending, entity.AppStateReviewing}},
	}
	update := bson.M{
		"$set": bson.M{
			"state":                    entity.AppStateWithdrawn,
			"revisions.$[r].state":     entity.AppStateWithdrawn,
			"revisions.$[r].updatedAt": in.At,
			"updatedAt":                in.At,
		},
		"$unset": bson.M{"activeSlot": ""},
		"$push": bson.M{
			"auditTrail": in.Entry,
			"outbox":     bson.M{"$each": in.Outbox},
		},
	}
	opts := options.UpdateOne().SetArrayFilters([]any{
		bson.M{"r.revision": in.Revision, "r.state": bson.M{"$in": []string{entity.AppStatePending, entity.AppStateReviewing}}},
	})
	res, err := r.db.Collection(colApplications).UpdateOne(ctx, filter, update, opts)
	if err != nil {
		return false, err
	}
	return res.MatchedCount == 1, nil
}

// Decide 是一次终态裁决：领取人、revision、reviewVersion 必须同时匹配，
// 且该 revision 尚无裁决记录，因此"每 revision 至多一个终态裁决"由条件更新保证。
func (r *MongoVehicleRepository) Decide(ctx context.Context, in entity.DecideInput) (bool, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	filter := bson.M{
		"applicationId":  in.ApplicationID,
		"activeRevision": in.Revision,
		"reviewVersion":  in.ReviewVersion,
		"state":          entity.AppStateReviewing,
		"reviewerId":     in.ActorID,
		"revisions": bson.M{"$elemMatch": bson.M{
			"revision":  in.Revision,
			"state":     entity.AppStateReviewing,
			"decisions": bson.M{"$size": 0},
		}},
	}
	update := bson.M{
		"$set": bson.M{
			"state":                    in.ToState,
			"revisions.$[r].state":     in.ToState,
			"revisions.$[r].updatedAt": in.At,
			"updatedAt":                in.At,
		},
		"$inc":   bson.M{"reviewVersion": int64(1)},
		"$unset": bson.M{"activeSlot": ""},
		"$push": bson.M{
			"revisions.$[r].decisions": in.Decision,
			"auditTrail":               in.Entry,
			"outbox":                   bson.M{"$each": in.Outbox},
		},
	}
	if len(in.Flags) > 0 {
		update["$addToSet"] = bson.M{"reviewFlags": bson.M{"$each": in.Flags}}
	}
	opts := options.UpdateOne().SetArrayFilters([]any{
		bson.M{"r.revision": in.Revision},
	})
	res, err := r.db.Collection(colApplications).UpdateOne(ctx, filter, update, opts)
	if err != nil {
		return false, err
	}
	return res.MatchedCount == 1, nil
}

// ListByState 供审核工作台按状态拉取。
func (r *MongoVehicleRepository) ListByState(ctx context.Context, states []string) ([]entity.VehicleApplication, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	cursor, err := r.db.Collection(colApplications).Find(ctx,
		bson.M{"state": bson.M{"$in": states}},
		options.Find().SetSort(bson.D{{Key: "updatedAt", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var out []entity.VehicleApplication
	for cursor.Next(ctx) {
		var app entity.VehicleApplication
		if errors.Is(cursor.Decode(&app), nil) {
			out = append(out, app)
		}
	}
	return out, cursor.Err()
}

// PendingOutbox 拉取内嵌待投递事件，按创建时间先到先投。
func (r *MongoVehicleRepository) PendingOutbox(ctx context.Context, limit int64) ([]entity.OutboxRef, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	cursor, err := r.db.Collection(colApplications).Aggregate(ctx, []bson.M{
		{"$match": bson.M{"outbox.state": entity.OutboxPending}},
		{"$unwind": "$outbox"},
		{"$match": bson.M{"outbox.state": entity.OutboxPending}},
		{"$sort": bson.M{"outbox.createdAt": 1}},
		{"$limit": limit},
		{"$project": bson.M{"applicationId": 1, "driverId": 1, "outbox": 1}},
	})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var out []entity.OutboxRef
	for cursor.Next(ctx) {
		var row struct {
			ApplicationID string             `bson:"applicationId"`
			DriverID      string             `bson:"driverId"`
			Outbox        entity.OutboxEntry `bson:"outbox"`
		}
		if err := cursor.Decode(&row); err != nil {
			return nil, err
		}
		out = append(out, entity.OutboxRef{ApplicationID: row.ApplicationID, DriverID: row.DriverID, Entry: row.Outbox})
	}
	return out, cursor.Err()
}

func (r *MongoVehicleRepository) MarkOutboxSent(ctx context.Context, applicationID, outboxID string, at time.Time) (bool, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	res, err := r.db.Collection(colApplications).UpdateOne(ctx,
		bson.M{"applicationId": applicationID, "outbox": bson.M{"$elemMatch": bson.M{"id": outboxID, "state": entity.OutboxPending}}},
		bson.M{"$set": bson.M{"outbox.$[o].state": entity.OutboxSent, "outbox.$[o].sentAt": at}},
		options.UpdateOne().SetArrayFilters([]any{bson.M{"o.id": outboxID}}))
	if err != nil {
		return false, err
	}
	return res.MatchedCount == 1, nil
}

func (r *MongoVehicleRepository) BumpOutboxAttempt(ctx context.Context, applicationID, outboxID string) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	_, err := r.db.Collection(colApplications).UpdateOne(ctx,
		bson.M{"applicationId": applicationID, "outbox": bson.M{"$elemMatch": bson.M{"id": outboxID, "state": entity.OutboxPending}}},
		bson.M{"$inc": bson.M{"outbox.$[o].attempts": 1}},
		options.UpdateOne().SetArrayFilters([]any{bson.M{"o.id": outboxID}}))
	return err
}
