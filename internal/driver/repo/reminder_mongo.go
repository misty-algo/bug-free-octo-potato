package repo

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/BwCloudWeGo/bw-cli/internal/driver/entity"
)

const colReminderTasks = "document_reminder_tasks"

// MongoReminderRepository 只用条件更新：领取靠租约 CAS，发送靠唯一 eventId 幂等。
type MongoReminderRepository struct{ db *mongo.Database }

func NewMongoReminderRepository(db *mongo.Database) *MongoReminderRepository {
	return &MongoReminderRepository{db: db}
}

func (r *MongoReminderRepository) Upsert(ctx context.Context, task entity.ReminderTask) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	_, err := r.db.Collection(colReminderTasks).UpdateOne(ctx,
		bson.M{
			"driverId": task.DriverID, "documentId": task.DocumentID,
			"documentVersion": task.DocumentVersion, "level": task.Level,
		},
		bson.M{"$setOnInsert": task},
		options.UpdateOne().SetUpsert(true),
	)
	return err
}

func (r *MongoReminderRepository) CancelByDocument(ctx context.Context, documentID string, exceptVersion int64, at time.Time) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	_, err := r.db.Collection(colReminderTasks).UpdateMany(ctx,
		bson.M{
			"documentId":      documentID,
			"documentVersion": bson.M{"$ne": exceptVersion},
			"state":           bson.M{"$in": []string{entity.ReminderPending, entity.ReminderFailed}},
		},
		bson.M{"$set": bson.M{"state": entity.ReminderCancelled, "updatedAt": at}},
	)
	return err
}

func (r *MongoReminderRepository) ClaimDue(ctx context.Context, now time.Time, limit int, owner string) ([]entity.ReminderTask, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	col := r.db.Collection(colReminderTasks)
	out := make([]entity.ReminderTask, 0, limit)
	for i := 0; i < limit; i++ {
		var task entity.ReminderTask
		err := col.FindOneAndUpdate(ctx,
			bson.M{
				"state":    bson.M{"$in": []string{entity.ReminderPending, entity.ReminderFailed}},
				"dueAt":    bson.M{"$lte": now},
				"attempts": bson.M{"$lt": entity.ReminderMaxAttempts},
				// 租约过期或从未领取过才能抢，进程重启后自然补跑。
				"$or": []bson.M{
					{"leaseUntil": nil},
					{"leaseUntil": bson.M{"$lte": now}},
				},
			},
			bson.M{
				"$set": bson.M{
					"state": entity.ReminderLeased, "leaseOwner": owner,
					"leaseUntil": now.Add(entity.ReminderLeaseTTL), "updatedAt": now,
				},
				"$inc": bson.M{"attempts": 1},
			},
			options.FindOneAndUpdate().SetReturnDocument(options.After),
		).Decode(&task)
		if err == mongo.ErrNoDocuments {
			break
		}
		if err != nil {
			return out, err
		}
		out = append(out, task)
	}
	return out, nil
}

func (r *MongoReminderRepository) MarkSent(ctx context.Context, taskID, noticeID string, at time.Time) error {
	return r.updateTask(ctx, taskID, bson.M{
		"state": entity.ReminderSent, "noticeId": noticeID,
		"notifiedAt": at, "leaseOwner": "", "updatedAt": at,
	})
}

func (r *MongoReminderRepository) MarkMerged(ctx context.Context, taskID, into string, at time.Time) error {
	return r.updateTask(ctx, taskID, bson.M{
		"state": entity.ReminderMerged, "mergedInto": into, "updatedAt": at,
	})
}

func (r *MongoReminderRepository) MarkCancelled(ctx context.Context, taskID string, at time.Time) error {
	return r.updateTask(ctx, taskID, bson.M{"state": entity.ReminderCancelled, "updatedAt": at})
}

func (r *MongoReminderRepository) MarkFailed(ctx context.Context, taskID string, retryAt time.Time) error {
	return r.updateTask(ctx, taskID, bson.M{
		"state": entity.ReminderFailed, "leaseUntil": nil,
		"dueAt": retryAt, "updatedAt": retryAt,
	})
}

func (r *MongoReminderRepository) updateTask(ctx context.Context, taskID string, fields bson.M) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	_, err := r.db.Collection(colReminderTasks).UpdateOne(ctx, bson.M{"taskId": taskID}, bson.M{"$set": fields})
	return err
}

func (r *MongoReminderRepository) InsertNotice(ctx context.Context, eventID, name, driverID, detail string, at time.Time) (bool, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	_, err := r.db.Collection(colEventLogs).InsertOne(ctx, bson.M{
		"eventId": eventID, "t": at, "name": name, "kind": "driver",
		"subjectId": driverID, "detail": detail,
	})
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

var _ entity.ReminderRepository = (*MongoReminderRepository)(nil)
