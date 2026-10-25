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
	colVehicleProfiles = "vehicle_profiles"
	colVehicleDrafts   = "vehicle_drafts"
	colVehicleAudits   = "vehicle_audits"
	colDocuments       = "driver_documents"
	colBindings        = "vehicle_bindings"
	colBindingHistory  = "binding_history"
	colFiles           = "files"
	colFileTasks       = "file_delete_tasks"
)

type MongoVehicleRepository struct{ db *mongo.Database }

func NewMongoVehicleRepository(db *mongo.Database) *MongoVehicleRepository {
	return &MongoVehicleRepository{db: db}
}

func (r *MongoVehicleRepository) DraftByDriver(ctx context.Context, driverID string) (*entity.VehicleDraft, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	var draft entity.VehicleDraft
	err := r.db.Collection(colVehicleDrafts).FindOne(ctx, bson.M{"driverId": driverID}).Decode(&draft)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, err
	}
	return &draft, nil
}

func (r *MongoVehicleRepository) SaveDraft(ctx context.Context, draft *entity.VehicleDraft) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	_, err := r.db.Collection(colVehicleDrafts).UpdateOne(ctx,
		bson.M{"driverId": draft.DriverID, "version": draft.Version},
		bson.M{"$set": draftToDoc(draft), "$inc": bson.M{"version": int64(1)}},
		options.UpdateOne().SetUpsert(true))
	return err
}

func (r *MongoVehicleRepository) DeleteDraft(ctx context.Context, driverID string) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	_, err := r.db.Collection(colVehicleDrafts).DeleteOne(ctx, bson.M{"driverId": driverID})
	return err
}

func (r *MongoVehicleRepository) ProfileByID(ctx context.Context, vehicleID string) (*entity.VehicleProfile, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	var profile entity.VehicleProfile
	err := r.db.Collection(colVehicleProfiles).FindOne(ctx, bson.M{"vehicleId": vehicleID}).Decode(&profile)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, err
	}
	return &profile, nil
}

func (r *MongoVehicleRepository) UpsertProfile(ctx context.Context, profile *entity.VehicleProfile) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	_, err := r.db.Collection(colVehicleProfiles).UpdateOne(ctx,
		bson.M{"vehicleId": profile.VehicleID},
		bson.M{"$set": bson.M{
			"vinMask": profile.VinMask, "plate": profile.Plate, "cityCode": profile.CityCode,
			"modelCode": profile.ModelCode, "seats": profile.Seats, "color": profile.Color,
			"updatedAt": profile.UpdatedAt,
		}, "$setOnInsert": bson.M{"vehicleId": profile.VehicleID, "createdAt": profile.CreatedAt}},
		options.UpdateOne().SetUpsert(true))
	return err
}

// PendingAuditByDriver 检索在审版本。历史数据可能使用中文状态，
// 这里一并识别，避免改不定装的旧数据。
func (r *MongoVehicleRepository) PendingAuditByDriver(ctx context.Context, driverID string) (*entity.VehicleAudit, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	var audit entity.VehicleAudit
	err := r.db.Collection(colVehicleAudits).FindOne(ctx,
		bson.M{"driverId": driverID, "state": bson.M{"$in": []string{entity.AuditStatePending, "待审"}}},
		options.FindOne().SetSort(bson.D{{Key: "revision", Value: -1}})).Decode(&audit)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, err
	}
	return &audit, nil
}

// NextRevision 追加版本链：同一 applicationId 的 revision 单调递增，
// 不覆盖历史驳回。
func (r *MongoVehicleRepository) NextRevision(ctx context.Context, applicationID string) (int64, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	var result struct {
		MaxRevision int64 `bson:"maxRevision"`
	}
	cursor, err := r.db.Collection(colVehicleAudits).Aggregate(ctx, []bson.M{
		{"$match": bson.M{"applicationId": applicationID}},
		{"$group": bson.M{"_id": nil, "maxRevision": bson.M{"$max": "$revision"}}},
	})
	if err != nil {
		return 0, err
	}
	defer cursor.Close(ctx)
	if cursor.Next(ctx) {
		if err := cursor.Decode(&result); err != nil {
			return 0, err
		}
	}
	return result.MaxRevision + 1, cursor.Err()
}

func (r *MongoVehicleRepository) InsertAudit(ctx context.Context, audit *entity.VehicleAudit) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	_, err := r.db.Collection(colVehicleAudits).InsertOne(ctx, audit)
	return err
}

func (r *MongoVehicleRepository) AuditByKey(ctx context.Context, applicationID string, revision int64) (*entity.VehicleAudit, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	var audit entity.VehicleAudit
	err := r.db.Collection(colVehicleAudits).FindOne(ctx, bson.M{"applicationId": applicationID, "revision": revision}).Decode(&audit)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, err
	}
	return &audit, nil
}

func (r *MongoVehicleRepository) ListPendingAudits(ctx context.Context) ([]entity.VehicleAudit, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	cursor, err := r.db.Collection(colVehicleAudits).Find(ctx,
		bson.M{"state": bson.M{"$in": []string{entity.AuditStatePending, "待审"}}},
		options.Find().SetSort(bson.D{{Key: "createdAt", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var out []entity.VehicleAudit
	for cursor.Next(ctx) {
		var audit entity.VehicleAudit
		if errors.Is(cursor.Decode(&audit), nil) {
			out = append(out, audit)
		}
	}
	return out, cursor.Err()
}

// SetAuditDecision 只在 pending 上成功，终态裁决唯一。
func (r *MongoVehicleRepository) SetAuditDecision(ctx context.Context, applicationID string, revision int64, state, reason, decidedBy string, at time.Time) (bool, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	res, err := r.db.Collection(colVehicleAudits).UpdateOne(ctx,
		bson.M{"applicationId": applicationID, "revision": revision,
			"state": bson.M{"$in": []string{entity.AuditStatePending, "待审"}}},
		bson.M{"$set": bson.M{"state": state, "rejectReason": reason, "decidedBy": decidedBy, "decidedAt": at},
			"$inc": bson.M{"rejectCount": int64(1)}})
	if err != nil {
		return false, err
	}
	return res.MatchedCount == 1, nil
}

func (r *MongoVehicleRepository) ActiveDocument(ctx context.Context, subjectID, docType string) (*entity.DriverDocument, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	var doc entity.DriverDocument
	err := r.db.Collection(colDocuments).FindOne(ctx,
		bson.M{"subjectId": subjectID, "docType": docType, "state": entity.DocStateActive}).Decode(&doc)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, err
	}
	return &doc, nil
}

func (r *MongoVehicleRepository) DocumentsByIDs(ctx context.Context, ids []string) ([]entity.DriverDocument, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	if len(ids) == 0 {
		return nil, nil
	}
	cursor, err := r.db.Collection(colDocuments).Find(ctx, bson.M{"documentId": bson.M{"$in": ids}})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var out []entity.DriverDocument
	for cursor.Next(ctx) {
		var doc entity.DriverDocument
		if errors.Is(cursor.Decode(&doc), nil) {
			out = append(out, doc)
		}
	}
	return out, cursor.Err()
}

func (r *MongoVehicleRepository) DocumentsBySubject(ctx context.Context, subjectID string) ([]entity.DriverDocument, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	cursor, err := r.db.Collection(colDocuments).Find(ctx,
		bson.M{"subjectId": subjectID, "state": entity.DocStateActive})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var out []entity.DriverDocument
	for cursor.Next(ctx) {
		var doc entity.DriverDocument
		if errors.Is(cursor.Decode(&doc), nil) {
			out = append(out, doc)
		}
	}
	return out, cursor.Err()
}

func (r *MongoVehicleRepository) ArchiveDocument(ctx context.Context, subjectID, docType string) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	_, err := r.db.Collection(colDocuments).UpdateOne(ctx,
		bson.M{"subjectId": subjectID, "docType": docType, "state": entity.DocStateActive},
		bson.M{"$set": bson.M{"state": entity.DocStateArchived}})
	return err
}

func (r *MongoVehicleRepository) SaveDocument(ctx context.Context, doc *entity.DriverDocument) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	_, err := r.db.Collection(colDocuments).InsertOne(ctx, doc)
	return err
}

func (r *MongoVehicleRepository) ActiveBindingByDriver(ctx context.Context, driverID string) (*entity.VehicleBinding, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	var binding entity.VehicleBinding
	err := r.db.Collection(colBindings).FindOne(ctx, bson.M{"driverId": driverID, "state": entity.BindingStateActive}).Decode(&binding)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, err
	}
	return &binding, nil
}

func (r *MongoVehicleRepository) ActiveBindingByVehicle(ctx context.Context, vehicleID string) (*entity.VehicleBinding, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	var binding entity.VehicleBinding
	err := r.db.Collection(colBindings).FindOne(ctx, bson.M{"vehicleId": vehicleID, "state": entity.BindingStateActive}).Decode(&binding)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, err
	}
	return &binding, nil
}

// SaveBinding 依赖 uk_binding_driver 与 uk_binding_vehicle 两个唯一索引兜底，
// 冲突即代表"一车一有效司机"或"一司机一有效车"被占用。
func (r *MongoVehicleRepository) SaveBinding(ctx context.Context, binding *entity.VehicleBinding) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	_, err := r.db.Collection(colBindings).UpdateOne(ctx,
		bson.M{"driverId": binding.DriverID, "state": entity.BindingStateActive},
		bson.M{"$set": binding}, options.UpdateOne().SetUpsert(true))
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return ErrDuplicateKey
		}
		return err
	}
	return nil
}

// DeleteBinding 在批准失败时回退刚写入的绑定，避免占用唯一位。
func (r *MongoVehicleRepository) DeleteBinding(ctx context.Context, driverID string, bindingVersion int64) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	_, err := r.db.Collection(colBindings).DeleteOne(ctx,
		bson.M{"driverId": driverID, "bindingVersion": bindingVersion})
	return err
}

// DeleteHistory 回退刚归档的历史行，保证同一版本不会同时存在于主表与历史表。
func (r *MongoVehicleRepository) DeleteHistory(ctx context.Context, driverID string, bindingVersion int64) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	_, err := r.db.Collection(colBindingHistory).DeleteOne(ctx,
		bson.M{"driverId": driverID, "bindingVersion": bindingVersion})
	return err
}

// ArchiveBinding 把旧绑定移出 vehicle_bindings（腾出唯一位），
// 历史独立保存到 binding_history。
func (r *MongoVehicleRepository) ArchiveBinding(ctx context.Context, binding *entity.VehicleBinding) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	binding.State = entity.BindingStateReleased
	_, err := r.db.Collection(colBindingHistory).UpdateOne(ctx,
		bson.M{"driverId": binding.DriverID, "bindingVersion": binding.BindingVersion},
		bson.M{"$set": binding}, options.UpdateOne().SetUpsert(true))
	if err != nil {
		return err
	}
	_, err = r.db.Collection(colBindings).DeleteOne(ctx,
		bson.M{"driverId": binding.DriverID, "bindingVersion": binding.BindingVersion})
	return err
}

func (r *MongoVehicleRepository) HistoryByDriver(ctx context.Context, driverID string) ([]entity.VehicleBinding, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	cursor, err := r.db.Collection(colBindingHistory).Find(ctx, bson.M{"driverId": driverID},
		options.Find().SetSort(bson.D{{Key: "bindingVersion", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var out []entity.VehicleBinding
	for cursor.Next(ctx) {
		var binding entity.VehicleBinding
		if errors.Is(cursor.Decode(&binding), nil) {
			out = append(out, binding)
		}
	}
	return out, cursor.Err()
}

// UpdateDriverVehicleProjection 只刷新 drivers 上的车辆投影（Mongo 侧）。
// 这是给派单/快照消费的只读副本，不是第二份真相。
func (r *MongoVehicleRepository) UpdateDriverVehicleProjection(ctx context.Context, driverID string, snap entity.VehicleSnapshot) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	oid, err := objectID(driverID)
	if err != nil {
		return err
	}
	_, err = r.db.Collection("drivers").UpdateOne(ctx, bson.M{"_id": oid}, bson.M{"$set": bson.M{
		"vehicle.plate": snap.Plate, "vehicle.model": snap.ModelCode, "vehicle.seats": snap.Seats,
		"vehicle.color": snap.Color, "vehicle.vehicleId": snap.VehicleID, "vehicle.revisionId": snap.RevisionID,
	}})
	return err
}

func draftToDoc(d *entity.VehicleDraft) bson.M {
	return bson.M{
		"draftId": d.DraftID, "driverId": d.DriverID, "vehicleId": d.VehicleID, "vinMask": d.VinMask,
		"plate": d.Plate, "cityCode": d.CityCode, "modelCode": d.ModelCode, "seats": d.Seats,
		"color": d.Color, "documentIds": d.DocumentIDs, "fileIds": d.FileIDs,
		"state": d.State, "updatedAt": d.UpdatedAt,
	}
}

var _ entity.VehicleRepository = (*MongoVehicleRepository)(nil)
