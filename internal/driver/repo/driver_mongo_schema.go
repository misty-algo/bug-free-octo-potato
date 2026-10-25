package repo

import (
	"context"
	"fmt"

	"github.com/BwCloudWeGo/bw-cli/internal/driver/entity"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// DriverCollections 是司机资料与审核域使用的集合。
// vehicle_audits 是 D14 的审核版本表，D15 起由 vehicle_applications 聚合根取代：
// 集合与索引保留（历史数据不重写），不再写入新数据。
var DriverCollections = []string{
	"vehicle_profiles", "vehicle_drafts", "vehicle_audits", "driver_documents",
	"vehicle_bindings", "binding_history", "files", "file_delete_tasks",
	// D15 审核域
	"vehicle_applications", "driver_rejection_counters", "driver_admission",
	// D16 到期提醒
	"document_reminder_tasks",
	// D17 安全事件
	"safety_events",
}

type driverIndex struct {
	collection string
	name       string
	keys       bson.D
	unique     bool
	// partial 是 partialFilterExpression：用于"只在活动期间唯一"这类约束。
	partial bson.M
}

// driverIndexes 表达「一辆车只绑一个有效司机、一名司机只有一辆有效车、
// 同一司机只有一个活动变更、每 revision 至多一个终态裁决」等不变式。
var driverIndexes = []driverIndex{
	{"vehicle_profiles", "uk_vehicle", bson.D{{Key: "vehicleId", Value: 1}}, true, nil},
	{"vehicle_drafts", "uk_driver_draft", bson.D{{Key: "driverId", Value: 1}}, true, nil},
	{"vehicle_audits", "uk_app_revision", bson.D{{Key: "applicationId", Value: 1}, {Key: "revision", Value: 1}}, true, nil},
	{"vehicle_audits", "idx_driver_state", bson.D{{Key: "driverId", Value: 1}, {Key: "state", Value: 1}}, false, nil},
	{"driver_documents", "uk_subject_type_version", bson.D{{Key: "subjectId", Value: 1}, {Key: "docType", Value: 1}, {Key: "version", Value: 1}}, true, nil},
	// 有效绑定：对 driver_id 与 vehicle_id 分别唯一，历史绑定不占用唯一位。
	{"vehicle_bindings", "uk_binding_driver", bson.D{{Key: "driverId", Value: 1}}, true, nil},
	{"vehicle_bindings", "uk_binding_vehicle", bson.D{{Key: "vehicleId", Value: 1}}, true, nil},
	{"binding_history", "uk_history_version", bson.D{{Key: "driverId", Value: 1}, {Key: "bindingVersion", Value: 1}}, true, nil},
	{"files", "uk_file", bson.D{{Key: "fileId", Value: 1}}, true, nil},
	{"files", "idx_cleanup", bson.D{{Key: "state", Value: 1}, {Key: "stagingExpiresAt", Value: 1}}, false, nil},
	{"file_delete_tasks", "uk_task", bson.D{{Key: "taskId", Value: 1}}, true, nil},
	{"file_delete_tasks", "uk_task_file", bson.D{{Key: "fileId", Value: 1}}, true, nil},
	// 申请聚合根
	{"vehicle_applications", "uk_application", bson.D{{Key: "applicationId", Value: 1}}, true, nil},
	// 同一司机只允许一个 pending/reviewing 变更：进入终态即 $unset activeSlot。
	{"vehicle_applications", "uk_active_slot", bson.D{{Key: "activeSlot", Value: 1}}, true, bson.M{"activeSlot": bson.M{"$exists": true}}},
	{"vehicle_applications", "idx_app_driver_created", bson.D{{Key: "driverId", Value: 1}, {Key: "createdAt", Value: -1}}, false, nil},
	{"vehicle_applications", "idx_app_state_updated", bson.D{{Key: "state", Value: 1}, {Key: "updatedAt", Value: 1}}, false, nil},
	{"vehicle_applications", "idx_app_outbox", bson.D{{Key: "outbox.state", Value: 1}}, false, nil},
	// 连续驳回计数：键与证件号无关，改号/撤回/新建申请都无法绕过。
	{"driver_rejection_counters", "uk_counter", bson.D{
		{Key: "driverId", Value: 1}, {Key: "targetType", Value: 1}, {Key: "targetKey", Value: 1},
	}, true, nil},
	{"driver_admission", "uk_admission", bson.D{{Key: "driverId", Value: 1}}, true, nil},
	// 每张证件的每个版本、每个档位只生成一条任务；重启补跑不重复。
	{"document_reminder_tasks", "uk_reminder_slot", bson.D{
		{Key: "driverId", Value: 1}, {Key: "documentId", Value: 1},
		{Key: "documentVersion", Value: 1}, {Key: "level", Value: 1},
	}, true, nil},
	{"document_reminder_tasks", "idx_reminder_due", bson.D{{Key: "state", Value: 1}, {Key: "dueAt", Value: 1}}, false, nil},
	{"document_reminder_tasks", "idx_reminder_driver", bson.D{{Key: "driverId", Value: 1}, {Key: "dueAt", Value: 1}}, false, nil},
	// 同一司机同一个请求号只受理一次：重试命中已有事件，不重复建单。
	{"safety_events", "uk_safety_request", bson.D{
		{Key: "driverId", Value: 1}, {Key: "raw.clientRequestId", Value: 1},
	}, true, nil},
	{"safety_events", "uk_safety_event", bson.D{{Key: "eventId", Value: 1}}, true, nil},
	{"safety_events", "idx_safety_queue", bson.D{
		{Key: "state", Value: 1}, {Key: "raw.receivedAt", Value: -1},
	}, false, nil},
	{"safety_events", "idx_safety_outbox", bson.D{{Key: "outbox.state", Value: 1}}, false, nil},
	{"safety_events", "idx_safety_driver_state", bson.D{
		{Key: "driverId", Value: 1}, {Key: "state", Value: 1},
	}, false, nil},
}

// driverOptionalIndexes 作用于本域不负责建集合的集合（订单域已建）。
// 集合不存在时跳过，不替别的域建表。
var driverOptionalIndexes = []driverIndex{
	{"dispatch_tasks", "idx_dispatch_driver_state", bson.D{{Key: "driverId", Value: 1}, {Key: "state", Value: 1}}, false, nil},
}

// ValidateMongoDriverSchema 校验（可选创建）集合与索引。
func ValidateMongoDriverSchema(ctx context.Context, db *mongo.Database, apply bool) error {
	existing, err := db.ListCollectionNames(ctx, bson.M{})
	if err != nil {
		return fmt.Errorf("list collections: %w", err)
	}
	found := map[string]bool{}
	for _, name := range existing {
		found[name] = true
	}
	for _, name := range DriverCollections {
		if found[name] {
			continue
		}
		if !apply {
			return fmt.Errorf("missing collection %q (run driver-schema -apply)", name)
		}
		if err := db.CreateCollection(ctx, name); err != nil {
			return fmt.Errorf("create collection %q: %w", name, err)
		}
		found[name] = true
	}
	// vehicle_audits 的历史数据可能使用中文状态；这一版不重写历史值。
	_ = entity.AuditStatePending
	for _, idx := range driverIndexes {
		if err := ensureDriverIndex(ctx, db, found, idx, apply); err != nil {
			return err
		}
	}
	for _, idx := range driverOptionalIndexes {
		if !found[idx.collection] {
			continue
		}
		if err := ensureDriverIndex(ctx, db, found, idx, apply); err != nil {
			return err
		}
	}
	return nil
}

func ensureDriverIndex(ctx context.Context, db *mongo.Database, found map[string]bool, idx driverIndex, apply bool) error {
	if !found[idx.collection] {
		return fmt.Errorf("missing collection %q", idx.collection)
	}
	var indexNames []string
	cursor, err := db.Collection(idx.collection).Indexes().List(ctx)
	if err != nil {
		return fmt.Errorf("list indexes of %q: %w", idx.collection, err)
	}
	for cursor.Next(ctx) {
		var spec bson.M
		if err := cursor.Decode(&spec); err != nil {
			continue
		}
		if name, ok := spec["name"].(string); ok {
			indexNames = append(indexNames, name)
		}
	}
	cursor.Close(ctx)
	if containsString(indexNames, idx.name) {
		return nil
	}
	if !apply {
		return fmt.Errorf("missing index %q on %q (run driver-schema -apply)", idx.name, idx.collection)
	}
	opt := options.Index().SetName(idx.name).SetUnique(idx.unique)
	if len(idx.partial) > 0 {
		opt = opt.SetPartialFilterExpression(idx.partial)
	}
	if _, err := db.Collection(idx.collection).Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    idx.keys,
		Options: opt,
	}); err != nil {
		return fmt.Errorf("create index %q on %q: %w", idx.name, idx.collection, err)
	}
	return nil
}

func containsString(values []string, target string) bool {
	for _, v := range values {
		if v == target {
			return true
		}
	}
	return false
}
