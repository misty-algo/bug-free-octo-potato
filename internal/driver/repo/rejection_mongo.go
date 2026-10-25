package repo

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/BwCloudWeGo/bw-cli/internal/driver/entity"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// ByDriver 返回司机全部计数目标，供批准后清零与重点复核标记判断。
func (r *MongoVehicleRepository) ByDriver(ctx context.Context, driverID string) ([]entity.RejectionCounter, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	cursor, err := r.db.Collection(colCounters).Find(ctx, bson.M{"driverId": driverID})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var out []entity.RejectionCounter
	for cursor.Next(ctx) {
		var counter entity.RejectionCounter
		if errors.Is(cursor.Decode(&counter), nil) {
			out = append(out, counter)
		}
	}
	return out, cursor.Err()
}

// ApplyReject 按"每次终态驳回只累计一次"执行：同一 revision 的重复调用被 processed 幂等键挡住。
func (r *MongoVehicleRepository) ApplyReject(ctx context.Context, driverID string, items []entity.RejectItem, applicationID string, revision int64, at time.Time) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	ref := applicationID + ":" + strconv.FormatInt(revision, 10)
	coll := r.db.Collection(colCounters)
	for _, item := range items {
		if item.TargetType == "" || item.TargetKey == "" {
			continue
		}
		event := entity.CounterEvent{
			Action: entity.ActionReject, ApplicationID: applicationID, Revision: revision, At: at,
		}
		res, err := coll.UpdateOne(ctx,
			bson.M{"driverId": driverID, "targetType": item.TargetType, "targetKey": item.TargetKey,
				"processed": bson.M{"$ne": ref}},
			bson.M{
				"$inc":      bson.M{"consecutive": int64(1)},
				"$addToSet": bson.M{"processed": ref},
				"$push":     bson.M{"history": event},
				"$set":      bson.M{"updatedAt": at},
			})
		if err != nil {
			return err
		}
		if res.MatchedCount == 0 {
			// 文档不存在 → 建首条；已被处理过 → 唯一索引冲突，幂等忽略。
			event.Value = 1
			_, err := coll.InsertOne(ctx, entity.RejectionCounter{
				DriverID: driverID, TargetType: item.TargetType, TargetKey: item.TargetKey,
				Consecutive: 1, Processed: []string{ref}, History: []entity.CounterEvent{event}, UpdatedAt: at,
			})
			if err != nil && !mongo.IsDuplicateKeyError(err) {
				return err
			}
		}
		if _, err := coll.UpdateOne(ctx,
			bson.M{"driverId": driverID, "targetType": item.TargetType, "targetKey": item.TargetKey,
				"consecutive": bson.M{"$gte": entity.RepeatRejectThreshold}},
			bson.M{"$set": bson.M{"flagged": true, "updatedAt": at}}); err != nil {
			return err
		}
	}
	return nil
}

// Clear 在批准后清零连续驳回次数，历史与标记痕迹保留。
func (r *MongoVehicleRepository) Clear(ctx context.Context, driverID, targetType, targetKey, applicationID string, revision int64, at time.Time) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	_, err := r.db.Collection(colCounters).UpdateOne(ctx,
		bson.M{"driverId": driverID, "targetType": targetType, "targetKey": targetKey},
		bson.M{
			"$set": bson.M{"consecutive": int64(0), "flagged": false, "updatedAt": at},
			"$push": bson.M{"history": entity.CounterEvent{
				Action: entity.ActionApprove, ApplicationID: applicationID, Revision: revision, At: at,
			}},
		})
	return err
}

// MongoAdmissionRepository 是准入协调点的独立实现：
// 与计数仓储同名方法签名不同，不能挂在同一个类型上。
type MongoAdmissionRepository struct{ db *mongo.Database }

func NewMongoAdmissionRepository(db *mongo.Database) *MongoAdmissionRepository {
	return &MongoAdmissionRepository{db: db}
}

// ByDriver 返回准入协调点，缺失时返回 nil（视为没有活动任务、未被锁）。
func (r *MongoAdmissionRepository) ByDriver(ctx context.Context, driverID string) (*entity.DriverAdmission, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	var adm entity.DriverAdmission
	err := r.db.Collection(colAdmission).FindOne(ctx, bson.M{"driverId": driverID}).Decode(&adm)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, err
	}
	return &adm, nil
}

// TryLock 仅在无活动任务且无人持锁时成功。这是与任务分配共用的协调边界。
func (r *MongoAdmissionRepository) TryLock(ctx context.Context, driverID, lockedBy string, at time.Time) (bool, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	coll := r.db.Collection(colAdmission)
	if _, err := coll.UpdateOne(ctx,
		bson.M{"driverId": driverID},
		bson.M{"$setOnInsert": bson.M{"driverId": driverID, "admissionVersion": int64(0), "activeTaskId": ""}},
		options.UpdateOne().SetUpsert(true)); err != nil && !mongo.IsDuplicateKeyError(err) {
		return false, err
	}
	res, err := coll.UpdateOne(ctx,
		bson.M{"driverId": driverID, "activeTaskId": "", "lockedBy": bson.M{"$in": []any{"", nil}}},
		bson.M{"$set": bson.M{"lockedBy": lockedBy, "lockedAt": at, "updatedAt": at}})
	if err != nil {
		return false, err
	}
	return res.MatchedCount == 1, nil
}

func (r *MongoAdmissionRepository) Unlock(ctx context.Context, driverID, lockedBy string) (bool, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	res, err := r.db.Collection(colAdmission).UpdateOne(ctx,
		bson.M{"driverId": driverID, "lockedBy": lockedBy},
		bson.M{"$set": bson.M{"lockedBy": "", "lockedAt": nil}})
	if err != nil {
		return false, err
	}
	return res.MatchedCount == 1, nil
}

// BumpVersion 递增准入源版本，派单与计费消费方据此感知资格变化。
func (r *MongoAdmissionRepository) BumpVersion(ctx context.Context, driverID string, at time.Time) (bool, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	res, err := r.db.Collection(colAdmission).UpdateOne(ctx,
		bson.M{"driverId": driverID},
		bson.M{"$inc": bson.M{"admissionVersion": int64(1)}, "$set": bson.M{"updatedAt": at}})
	if err != nil {
		return false, err
	}
	return res.MatchedCount == 1, nil
}

// SetActiveTask 由派单域写入；本域只读它做换车前的互斥检查。
func (r *MongoAdmissionRepository) SetActiveTask(ctx context.Context, driverID, taskID string, at time.Time) (bool, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	res, err := r.db.Collection(colAdmission).UpdateOne(ctx,
		bson.M{"driverId": driverID},
		bson.M{"$set": bson.M{"activeTaskId": taskID, "updatedAt": at},
			"$setOnInsert": bson.M{"admissionVersion": int64(0)}},
		options.UpdateOne().SetUpsert(true))
	if err != nil {
		return false, err
	}
	return res.MatchedCount == 1 || res.UpsertedCount == 1, nil
}

// DispatchTaskReader 对 dispatch_tasks 做只读检查，判断司机是否有未结束任务。
// 派单域提供正式接口后替换此实现（联调项）。
type DispatchTaskReader struct{ db *mongo.Database }

func NewDispatchTaskReader(db *mongo.Database) *DispatchTaskReader {
	return &DispatchTaskReader{db: db}
}

func (r *DispatchTaskReader) HasActiveTask(ctx context.Context, driverID string) (bool, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	oid, err := objectID(driverID)
	if err != nil {
		return false, fmt.Errorf("parse driver id %q: %w", driverID, err)
	}
	count, err := r.db.Collection("dispatch_tasks").CountDocuments(ctx,
		bson.M{"driverId": oid, "state": bson.M{"$in": []string{"ASSIGNED", "ACTIVE"}}},
		options.Count().SetLimit(1))
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// ActiveTaskIDs 返回本人当前活动的任务标识（_id 与 taskNo 都算）：
// 安全事件引用 task_id 时按这个集合核对，不属于本人就无法引用。
func (r *DispatchTaskReader) ActiveTaskIDs(ctx context.Context, driverID string) ([]string, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	oid, err := objectID(driverID)
	if err != nil {
		return nil, fmt.Errorf("parse driver id %q: %w", driverID, err)
	}
	cursor, err := r.db.Collection("dispatch_tasks").Find(ctx,
		bson.M{"driverId": oid, "state": bson.M{"$in": []string{"ASSIGNED", "ACTIVE"}}},
		options.Find().SetLimit(20))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	ids := make([]string, 0, 2)
	for cursor.Next(ctx) {
		var task struct {
			ID     bson.ObjectID `bson:"_id"`
			TaskNo string        `bson:"taskNo"`
		}
		if err := cursor.Decode(&task); err != nil {
			return ids, err
		}
		ids = append(ids, task.ID.Hex())
		if task.TaskNo != "" {
			ids = append(ids, task.TaskNo)
		}
	}
	return ids, cursor.Err()
}

var (
	_ entity.ApplicationRepository      = (*MongoVehicleRepository)(nil)
	_ entity.RejectionCounterRepository = (*MongoVehicleRepository)(nil)
	_ entity.AdmissionRepository        = (*MongoAdmissionRepository)(nil)
	_ entity.TaskReader                 = (*DispatchTaskReader)(nil)
)
