package repo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/BwCloudWeGo/bw-cli/internal/driver/entity"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type MongoFileRepository struct{ db *mongo.Database }

func NewMongoFileRepository(db *mongo.Database) *MongoFileRepository {
	return &MongoFileRepository{db: db}
}

func (r *MongoFileRepository) Insert(ctx context.Context, rec *entity.FileRecord) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	_, err := r.db.Collection(colFiles).InsertOne(ctx, rec)
	return err
}

func (r *MongoFileRepository) ByID(ctx context.Context, fileID string) (*entity.FileRecord, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	var rec entity.FileRecord
	err := r.db.Collection(colFiles).FindOne(ctx, bson.M{"fileId": fileID}).Decode(&rec)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, err
	}
	return &rec, nil
}

func (r *MongoFileRepository) ByKey(ctx context.Context, key string) (*entity.FileRecord, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	var rec entity.FileRecord
	err := r.db.Collection(colFiles).FindOne(ctx, bson.M{"key": key}).Decode(&rec)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, err
	}
	return &rec, nil
}

// MarkUploaded 只在 UPLOADING 行上成功，并写入服务端实测的字节数与真实类型。
// 但不信调用方声明的 Size/Content-Type——这两个值由存储层统计与嗅探得出。
func (r *MongoFileRepository) MarkUploaded(ctx context.Context, fileID, provider, bucket, contentType, checksum string, size int64) (bool, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	res, err := r.db.Collection(colFiles).UpdateOne(ctx,
		bson.M{"fileId": fileID, "state": entity.FileStateUploading},
		bson.M{"$set": bson.M{
			"state": entity.FileStateReady, "provider": provider, "bucket": bucket,
			"contentType": contentType, "size": size, "checksum": checksum,
		}})
	if err != nil {
		return false, err
	}
	return res.MatchedCount == 1, nil
}

func (r *MongoFileRepository) Fail(ctx context.Context, fileID, reason string) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	_, err := r.db.Collection(colFiles).UpdateOne(ctx,
		bson.M{"fileId": fileID, "state": entity.FileStateUploading},
		bson.M{"$set": bson.M{"state": entity.FileStateFailed, "failureReason": reason}})
	return err
}

// Attach 逐行抢占 READY 行并绑定到审核版本。
// 若清理已把某行置为 DELETING，这里 MatchedCount=0，提交必须失败并要求重新上传。
func (r *MongoFileRepository) Attach(ctx context.Context, ownerID, attachedTo string, ids []string) (int64, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	var attached int64
	for _, id := range ids {
		res, err := r.db.Collection(colFiles).UpdateOne(ctx,
			bson.M{"fileId": id, "ownerId": ownerID, "state": entity.FileStateReady},
			bson.M{"$set": bson.M{"state": entity.FileStateAttached, "attachedTo": attachedTo}})
		if err != nil {
			return attached, err
		}
		attached += res.MatchedCount
	}
	return attached, nil
}

// Release 在提交失败时把本次绑定的文件退回 READY，允许重新提交。
// 只回退仍为 ATTACHED 且 attachedTo 匹配的行，已进入清理流程的文件不动。
func (r *MongoFileRepository) Release(ctx context.Context, attachedTo string) (int64, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	res, err := r.db.Collection(colFiles).UpdateMany(ctx,
		bson.M{"attachedTo": attachedTo, "state": entity.FileStateAttached},
		bson.M{"$set": bson.M{"state": entity.FileStateReady, "attachedTo": ""}})
	if err != nil {
		return 0, err
	}
	return res.ModifiedCount, nil
}

func (r *MongoFileRepository) CountByOwnerPurpose(ctx context.Context, ownerID, attachedTo string) (int64, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return r.db.Collection(colFiles).CountDocuments(ctx, bson.M{"ownerId": ownerID, "attachedTo": attachedTo,
		"state": bson.M{"$in": []string{entity.FileStateReady, entity.FileStateAttached}}})
}

func (r *MongoFileRepository) ListByIDs(ctx context.Context, ids []string) ([]entity.FileRecord, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	if len(ids) == 0 {
		return nil, nil
	}
	cursor, err := r.db.Collection(colFiles).Find(ctx, bson.M{"fileId": bson.M{"$in": ids}})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var out []entity.FileRecord
	for cursor.Next(ctx) {
		var rec entity.FileRecord
		if errors.Is(cursor.Decode(&rec), nil) {
			out = append(out, rec)
		}
	}
	return out, cursor.Err()
}

// ClaimStale 抢占暂存到期且未被绑定的 READY 行。
// attachedTo 为空意味着没有审核版本引用它，已提交证据不会被回收。
func (r *MongoFileRepository) ClaimStale(ctx context.Context, now time.Time, limit int64) ([]entity.FileRecord, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	if limit <= 0 {
		limit = 20
	}
	cursor, err := r.db.Collection(colFiles).Find(ctx, bson.M{
		"state":            entity.FileStateReady,
		"attachedTo":       bson.M{"$in": []any{"", nil}},
		"stagingExpiresAt": bson.M{"$lte": now},
	}, options.Find().SetLimit(limit))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var candidates []entity.FileRecord
	for cursor.Next(ctx) {
		var rec entity.FileRecord
		if errors.Is(cursor.Decode(&rec), nil) {
			candidates = append(candidates, rec)
		}
	}
	if err := cursor.Err(); err != nil {
		return nil, err
	}
	var claimed []entity.FileRecord
	for _, rec := range candidates {
		taskID := "FT:" + rec.FileID
		res, err := r.db.Collection(colFiles).UpdateOne(ctx,
			bson.M{"fileId": rec.FileID, "state": entity.FileStateReady, "attachedTo": bson.M{"$in": []any{"", nil}}},
			bson.M{"$set": bson.M{"state": entity.FileStateDeleting, "deleteTaskId": taskID}})
		if err != nil {
			return claimed, err
		}
		if res.MatchedCount != 1 {
			// 被提交事务抢先绑定：清理跳过，不删除正在引用的证据。
			continue
		}
		// 删除任务按 fileId 幂等：重复创建撞唯一索引属正常，其它错误必须上抛。
		if _, err := r.db.Collection(colFileTasks).InsertOne(ctx, &entity.FileDeleteTask{
			TaskID: taskID, FileID: rec.FileID, Key: rec.Key, Provider: rec.Provider,
			State: entity.DeleteTaskPending, CreatedAt: now,
		}); err != nil && !mongo.IsDuplicateKeyError(err) {
			return claimed, fmt.Errorf("create delete task for file %s: %w", rec.FileID, err)
		}
		rec.State = entity.FileStateDeleting
		rec.DeleteTaskID = taskID
		claimed = append(claimed, rec)
	}
	return claimed, nil
}

func (r *MongoFileRepository) InsertDeleteTask(ctx context.Context, task *entity.FileDeleteTask) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	_, err := r.db.Collection(colFileTasks).InsertOne(ctx, task)
	return err
}

func (r *MongoFileRepository) PendingDeleteTasks(ctx context.Context, limit int64) ([]entity.FileDeleteTask, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	cursor, err := r.db.Collection(colFileTasks).Find(ctx, bson.M{"state": entity.DeleteTaskPending},
		options.Find().SetLimit(limit))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var out []entity.FileDeleteTask
	for cursor.Next(ctx) {
		var task entity.FileDeleteTask
		if errors.Is(cursor.Decode(&task), nil) {
			out = append(out, task)
		}
	}
	return out, cursor.Err()
}

func (r *MongoFileRepository) BumpDeleteAttempt(ctx context.Context, taskID string) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	_, err := r.db.Collection(colFileTasks).UpdateOne(ctx, bson.M{"taskId": taskID}, bson.M{"$inc": bson.M{"attempts": int64(1)}})
	return err
}

func (r *MongoFileRepository) MarkDeleteTaskDone(ctx context.Context, taskID string, at time.Time) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	_, err := r.db.Collection(colFileTasks).UpdateOne(ctx, bson.M{"taskId": taskID},
		bson.M{"$set": bson.M{"state": entity.DeleteTaskDone, "doneAt": at}})
	return err
}

// MarkFileDeleted 只在仍处于 DELETING 时成功：
// 已删除的文件不会被改回可提交状态。
func (r *MongoFileRepository) MarkFileDeleted(ctx context.Context, fileID string, at time.Time) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	_, err := r.db.Collection(colFiles).UpdateOne(ctx,
		bson.M{"fileId": fileID, "state": entity.FileStateDeleting},
		bson.M{"$set": bson.M{"state": entity.FileStateDeleted, "deletedAt": at}})
	return err
}

var _ entity.FileRepository = (*MongoFileRepository)(nil)
