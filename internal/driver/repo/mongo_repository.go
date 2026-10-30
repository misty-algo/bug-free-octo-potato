package repo

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.uber.org/zap"

	"github.com/BwCloudWeGo/bw-cli/internal/driver/entity"
	dbmodel "github.com/BwCloudWeGo/bw-cli/internal/driver/model"
	"github.com/BwCloudWeGo/bw-cli/pkg/biztime"
	"github.com/BwCloudWeGo/bw-cli/pkg/contracts"
	"github.com/BwCloudWeGo/bw-cli/pkg/mongox"
)

// MongoRepositories 基于 day1 库的司机端仓储。
// 关键唯一性由数据库唯一索引兜底：一个司机一个活动任务（active_task_id 唯一）。
type MongoRepositories struct {
	db  *mongo.Database
	log *zap.Logger
}

// NewMongoRepositories 创建司机端全部仓储。
func NewMongoRepositories(db *mongo.Database, loggers ...*zap.Logger) *MongoRepositories {
	log := zap.NewNop()
	if len(loggers) > 0 && loggers[0] != nil {
		log = loggers[0]
	}
	return &MongoRepositories{db: db, log: log}
}

func (r *MongoRepositories) Drivers() entity.DriverRepository { return &driverRepo{db: r.db, log: r.log} }
func (r *MongoRepositories) Orders() entity.HallOrderRepository { return &orderRepo{db: r.db, log: r.log} }
func (r *MongoRepositories) Incomes() entity.IncomeRepository { return &incomeRepo{db: r.db, log: r.log} }
func (r *MongoRepositories) Support() entity.SupportRepository { return &supportRepo{db: r.db, log: r.log} }

func isNoDocuments(err error) bool {
	return errors.Is(err, mongo.ErrNoDocuments) || errors.Is(err, mongox.ErrNotFound)
}

func logRepoOp(log *zap.Logger, op, id string, err error) {
	if log == nil || err == nil {
		return
	}
	log.Warn("driver repo operation failed", zap.String("op", op), zap.String("id", id), zap.Error(err))
}

// ─────────────────────────── 司机档案 ───────────────────────────

type driverRepo struct {
	db  *mongo.Database
	log *zap.Logger
}

func (r *driverRepo) coll() *mongo.Collection { return r.db.Collection(dbmodel.DriverCollectionName) }

func (r *driverRepo) Create(ctx context.Context, d *entity.Driver) error {
	_, err := r.coll().InsertOne(ctx, toDriverDocument(d))
	logRepoOp(r.log, "driver.Create", d.ID, err)
	return err
}

func (r *driverRepo) Get(ctx context.Context, id string) (*entity.Driver, error) {
	var doc dbmodel.DriverDocument
	err := r.coll().FindOne(ctx, bson.M{"_id": id}).Decode(&doc)
	if isNoDocuments(err) {
		return nil, entity.ErrDriverNotFound
	}
	if err != nil {
		logRepoOp(r.log, "driver.Get", id, err)
		return nil, err
	}
	return driverFromDocument(&doc), nil
}

func (r *driverRepo) UpdateStatus(ctx context.Context, id, status string, version int64, updatedAt time.Time) error {
	filter := bson.M{"_id": id}
	if version > 0 {
		filter["version"] = version
	}
	res, err := r.coll().UpdateOne(ctx, filter, bson.M{
		"$set": bson.M{"status": status, "updated_at": updatedAt},
		"$inc": bson.M{"version": int64(1)},
	})
	logRepoOp(r.log, "driver.UpdateStatus", id, err)
	if err == nil && res.MatchedCount == 0 {
		return entity.ErrVersionConflict
	}
	return err
}

func (r *driverRepo) UpdateMode(ctx context.Context, id, mode string, version, modeVersion int64, updatedAt time.Time) error {
	filter := bson.M{"_id": id}
	if version > 0 {
		filter["version"] = version
	}
	res, err := r.coll().UpdateOne(ctx, filter, bson.M{
		"$set": bson.M{"mode": mode, "mode_version": modeVersion, "updated_at": updatedAt},
		"$inc": bson.M{"version": int64(1)},
	})
	logRepoOp(r.log, "driver.UpdateMode", id, err)
	if err == nil && res.MatchedCount == 0 {
		return entity.ErrVersionConflict
	}
	return err
}

func (r *driverRepo) UpdateAreaConfig(ctx context.Context, id string, switchOn bool, areas []entity.AreaBox, version int64, updatedAt time.Time) error {
	boxes := make([]dbmodel.AreaBoxDoc, 0, len(areas))
	for _, a := range areas {
		boxes = append(boxes, dbmodel.AreaBoxDoc{Name: a.Name, MinLat: a.MinLat, MinLng: a.MinLng, MaxLat: a.MaxLat, MaxLng: a.MaxLng})
	}
	filter := bson.M{"_id": id}
	if version > 0 {
		filter["version"] = version
	}
	res, err := r.coll().UpdateOne(ctx, filter, bson.M{
		"$set": bson.M{"area_limit_switch": switchOn, "areas": boxes, "updated_at": updatedAt},
		"$inc": bson.M{"version": int64(1)},
	})
	logRepoOp(r.log, "driver.UpdateAreaConfig", id, err)
	if err == nil && res.MatchedCount == 0 {
		return entity.ErrVersionConflict
	}
	return err
}

func (r *driverRepo) UpdateTaskBinding(ctx context.Context, id, status, activeTaskID string, version int64, updatedAt time.Time) error {
	filter := bson.M{"_id": id}
	if version > 0 {
		filter["version"] = version
	}
	if status == entity.DriverStatusIdle && activeTaskID != "" {
		// 兜底防御：企图在已有活动任务上再绑定，交由唯一索引/上层业务拒绝。
		filter["active_task_id"] = bson.M{"$in": []any{"", activeTaskID}}
	}
	res, err := r.coll().UpdateOne(ctx, filter, bson.M{
		"$set": bson.M{"status": status, "active_task_id": activeTaskID, "updated_at": updatedAt},
		"$inc": bson.M{"version": int64(1)},
	})
	logRepoOp(r.log, "driver.UpdateTaskBinding", id, err)
	if err == nil && res.MatchedCount == 0 {
		return entity.ErrVersionConflict
	}
	return err
}

func (r *driverRepo) UpdateHeartbeat(ctx context.Context, id string, lastHeartbeatAt time.Time, lat, lng float64, version int64, updatedAt time.Time) error {
	filter := bson.M{"_id": id}
	if version > 0 {
		filter["version"] = version
	}
	res, err := r.coll().UpdateOne(ctx, filter, bson.M{
		"$set": bson.M{"last_heartbeat_at": lastHeartbeatAt, "last_lat": lat, "last_lng": lng, "updated_at": updatedAt},
		"$inc": bson.M{"version": int64(1)},
	})
	logRepoOp(r.log, "driver.UpdateHeartbeat", id, err)
	if err == nil && res.MatchedCount == 0 {
		return entity.ErrVersionConflict
	}
	return err
}

func (r *driverRepo) UpdateOfflineAfterTrip(ctx context.Context, id string, on bool, version int64, updatedAt time.Time) error {
	filter := bson.M{"_id": id}
	if version > 0 {
		filter["version"] = version
	}
	res, err := r.coll().UpdateOne(ctx, filter, bson.M{
		"$set": bson.M{"offline_after_trip": on, "updated_at": updatedAt},
		"$inc": bson.M{"version": int64(1)},
	})
	logRepoOp(r.log, "driver.UpdateOfflineAfterTrip", id, err)
	if err == nil && res.MatchedCount == 0 {
		return entity.ErrVersionConflict
	}
	return err
}

func (r *driverRepo) FindListenCandidates(ctx context.Context, f entity.ListenFilter) ([]*entity.Driver, error) {
	filter := bson.M{
		"status":    entity.DriverStatusIdle,
		"mode":      entity.ModeListen,
		"qualified": true,
		"can_accept": true,
	}
	if len(f.ExcludeIDs) > 0 {
		filter["_id"] = bson.M{"$nin": f.ExcludeIDs}
	}
	if f.CityCode != "" {
		filter["city_code"] = f.CityCode
	}
	// 区域/车型/座位在候选粗筛后再由服务层精算，这里先按座位数粗筛。
	limit := f.Limit
	if limit <= 0 {
		limit = 50
	}
	opts := options.Find().SetSort(bson.D{{Key: "driver_no", Value: 1}}).SetLimit(int64(limit))
	cur, err := r.coll().Find(ctx, filter, opts)
	if err != nil {
		logRepoOp(r.log, "driver.FindListenCandidates", "", err)
		return nil, err
	}
	defer cur.Close(ctx)
	out := make([]*entity.Driver, 0, limit)
	for cur.Next(ctx) {
		var doc dbmodel.DriverDocument
		if err := cur.Decode(&doc); err != nil {
			continue
		}
		out = append(out, driverFromDocument(&doc))
	}
	return out, nil
}

func (r *driverRepo) FindIdleHeartbeatExpired(ctx context.Context, before time.Time, limit int) ([]*entity.Driver, error) {
	if limit <= 0 {
		limit = 50
	}
	cur, err := r.coll().Find(ctx, bson.M{
		"status":             entity.DriverStatusIdle,
		"last_heartbeat_at":  bson.M{"$lt": before},
	}, options.Find().SetLimit(int64(limit)))
	if err != nil {
		logRepoOp(r.log, "driver.FindIdleHeartbeatExpired", "", err)
		return nil, err
	}
	defer cur.Close(ctx)
	out := make([]*entity.Driver, 0, limit)
	for cur.Next(ctx) {
		var doc dbmodel.DriverDocument
		if err := cur.Decode(&doc); err != nil {
			continue
		}
		out = append(out, driverFromDocument(&doc))
	}
	return out, nil
}

// MoveFunds 资金五桶原子转移：源桶余额不足则失败（不会透支）。
func (r *driverRepo) MoveFunds(ctx context.Context, id, fromBucket, toBucket string, amountFen int64) error {
	if amountFen <= 0 {
		return entity.ErrInvalidBucket
	}
	fromField, err := bucketField(fromBucket)
	if err != nil {
		return err
	}
	toField, err := bucketField(toBucket)
	if err != nil {
		return err
	}
	if fromField == toField {
		return nil
	}
	res, err := r.coll().UpdateOne(ctx, bson.M{
		"_id":       id,
		fromField:   bson.M{"$gte": amountFen},
	}, bson.M{
		"$inc": bson.M{fromField: -amountFen, toField: amountFen, "version": int64(1)},
		"$set": bson.M{"updated_at": biztime.UTC()},
	})
	logRepoOp(r.log, "driver.MoveFunds", id, err)
	if err == nil && res.MatchedCount == 0 {
		return entity.ErrInsufficientFunds
	}
	return err
}

// DeductFunds 从指定桶原子扣款出账（提现打款）；余额不足返回 ErrInsufficientFunds。
func (r *driverRepo) DeductFunds(ctx context.Context, id, bucket string, amountFen int64) error {
	if amountFen <= 0 {
		return entity.ErrInvalidBucket
	}
	field, err := bucketField(bucket)
	if err != nil {
		return err
	}
	res, err := r.coll().UpdateOne(ctx, bson.M{
		"_id":     id,
		field:     bson.M{"$gte": amountFen},
	}, bson.M{
		"$inc": bson.M{field: -amountFen, "version": int64(1)},
		"$set": bson.M{"updated_at": biztime.UTC()},
	})
	logRepoOp(r.log, "driver.DeductFunds", id, err)
	if err == nil && res.MatchedCount == 0 {
		return entity.ErrInsufficientFunds
	}
	return err
}

func bucketField(bucket string) (string, error) {
	switch bucket {
	case entity.BucketPending:
		return "funds.pending_fen", nil
	case entity.BucketAvailable:
		return "funds.available_fen", nil
	case entity.BucketFrozen:
		return "funds.frozen_fen", nil
	case entity.BucketWithdraw:
		return "funds.withdraw_fen", nil
	case entity.BucketDebt:
		return "funds.debt_fen", nil
	}
	return "", entity.ErrInvalidBucket
}

// AddFunds 外部资金入账到指定桶（乘客支付 → 待结算 U）。
func (r *driverRepo) AddFunds(ctx context.Context, id, bucket string, amountFen int64) error {
	if amountFen <= 0 {
		return nil
	}
	field, err := bucketField(bucket)
	if err != nil {
		return err
	}
	_, err = r.coll().UpdateOne(ctx, bson.M{"_id": id}, bson.M{
		"$inc": bson.M{field: amountFen, "version": int64(1)},
		"$set": bson.M{"updated_at": biztime.UTC()},
	})
	logRepoOp(r.log, "driver.AddFunds", id, err)
	return err
}

func (r *driverRepo) AddMonthlyIncome(ctx context.Context, id string, amountFen int64) error {
	_, err := r.coll().UpdateOne(ctx, bson.M{"_id": id},
		bson.M{"$inc": bson.M{"monthly_income_fen": amountFen}})
	logRepoOp(r.log, "driver.AddMonthlyIncome", id, err)
	return err
}

func (r *driverRepo) AddSuccessRate(ctx context.Context, id string, sample bool) error {
	d, err := r.Get(ctx, id)
	if err != nil {
		return err
	}
	// 简单移动平均：历史权重 0.9 + 新样本 0.1。
	var target float64 = 0
	if sample {
		target = 1
	}
	next := d.SuccessRate*0.9 + target*0.1
	_, err = r.coll().UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": bson.M{"success_rate": next}})
	logRepoOp(r.log, "driver.AddSuccessRate", id, err)
	return err
}

func (r *driverRepo) SetAcceptPermission(ctx context.Context, id string, canAccept bool) error {
	_, err := r.coll().UpdateOne(ctx, bson.M{"_id": id},
		bson.M{"$set": bson.M{"can_accept": canAccept, "updated_at": biztime.UTC()}})
	logRepoOp(r.log, "driver.SetAcceptPermission", id, err)
	return err
}

// RecordRelease 记录退单（含责任方），用于每日 3 次上限统计。
func (r *driverRepo) RecordRelease(ctx context.Context, id, party string, at time.Time) error {
	_, err := r.coll().UpdateOne(ctx, bson.M{"_id": id}, bson.M{
		"$push": bson.M{"recent_releases": bson.M{
			"$each":     []bson.M{{"at": at, "party": party}},
			"$slice":    -100,
		}},
		"$set": bson.M{"updated_at": at},
	})
	logRepoOp(r.log, "driver.RecordRelease", id, err)
	return err
}

// CountDriverReleasesToday 统计当日司机责任退单次数（排除乘客取消与平台责任）。
func (r *driverRepo) CountDriverReleasesToday(ctx context.Context, id string, dayStart time.Time) (int64, error) {
	d, err := r.Get(ctx, id)
	if err != nil {
		return 0, err
	}
	var count int64
	for _, rec := range d.RecentReleases {
		if rec.Party == contracts.PartyDriver && !rec.At.Before(dayStart) {
			count++
		}
	}
	return count, nil
}

// ─────────────────────────── 接单大厅 ───────────────────────────

type orderRepo struct {
	db  *mongo.Database
	log *zap.Logger
}

func (r *orderRepo) coll() *mongo.Collection { return r.db.Collection(dbmodel.HallOrderCollectionName) }

func (r *orderRepo) Create(ctx context.Context, o *entity.HallOrder) error {
	_, err := r.coll().InsertOne(ctx, toHallOrderDocument(o))
	logRepoOp(r.log, "order.Create", o.ID, err)
	return err
}

func (r *orderRepo) Get(ctx context.Context, id string) (*entity.HallOrder, error) {
	var doc dbmodel.HallOrderDocument
	err := r.coll().FindOne(ctx, bson.M{"_id": id}).Decode(&doc)
	if isNoDocuments(err) {
		return nil, entity.ErrOrderNotFound
	}
	if err != nil {
		logRepoOp(r.log, "order.Get", id, err)
		return nil, err
	}
	return hallOrderFromDocument(&doc), nil
}

func (r *orderRepo) GetByOrderID(ctx context.Context, orderID string) (*entity.HallOrder, error) {
	var doc dbmodel.HallOrderDocument
	err := r.coll().FindOne(ctx, bson.M{"order_id": orderID}).Decode(&doc)
	if isNoDocuments(err) {
		return nil, entity.ErrOrderNotFound
	}
	if err != nil {
		logRepoOp(r.log, "order.GetByOrderID", orderID, err)
		return nil, err
	}
	return hallOrderFromDocument(&doc), nil
}

// Assign 原子邀请/抢占：只有等待中且无人占用（或上一轮邀请已超时）才能成功。
func (r *orderRepo) Assign(ctx context.Context, orderID, driverID, driverName string, version int64,
	acceptDeadline time.Time, distanceMeters int64) (*entity.HallOrder, error) {
	now := biztime.UTC()
	filter := bson.M{
		"_id":    orderID,
		"status": entity.OrderStatusWaiting,
		"$or":    []bson.M{{"driver_id": ""}, {"accept_deadline": bson.M{"$lt": now}}},
	}
	if version > 0 {
		filter["version"] = version
	}
	update := bson.M{
		"$set": bson.M{
			"driver_id":              driverID,
			"driver_name":            driverName,
			"assigned_at":            now,
			"accept_deadline":        acceptDeadline,
			"pickup_distance_meters": distanceMeters,
			"updated_at":             now,
		},
		"$addToSet": bson.M{"round_invited": driverID},
		"$inc":      bson.M{"version": int64(1), "invite_round": int64(1)},
	}
	var doc dbmodel.HallOrderDocument
	err := r.coll().FindOneAndUpdate(ctx, filter, update, options.FindOneAndUpdate().SetReturnDocument(options.After)).Decode(&doc)
	if isNoDocuments(err) {
		return nil, entity.ErrOrderNotAvailable
	}
	if err != nil {
		logRepoOp(r.log, "order.Assign", orderID, err)
		return nil, err
	}
	return hallOrderFromDocument(&doc), nil
}

// ConfirmAccept 确认接单：必须在邀请有效期内且是自己被邀请的任务。
func (r *orderRepo) ConfirmAccept(ctx context.Context, orderID, driverID string, version int64) (*entity.HallOrder, error) {
	now := biztime.UTC()
	filter := bson.M{
		"_id":             orderID,
		"status":          entity.OrderStatusWaiting,
		"driver_id":       driverID,
		"accept_deadline": bson.M{"$gte": now},
	}
	if version > 0 {
		filter["version"] = version
	}
	update := bson.M{
		"$set": bson.M{"status": entity.OrderStatusAccepted, "accepted_at": now, "updated_at": now},
		"$inc": bson.M{"version": int64(1)},
	}
	var doc dbmodel.HallOrderDocument
	err := r.coll().FindOneAndUpdate(ctx, filter, update, options.FindOneAndUpdate().SetReturnDocument(options.After)).Decode(&doc)
	if isNoDocuments(err) {
		return nil, entity.ErrOrderNotAvailable
	}
	if err != nil {
		logRepoOp(r.log, "order.ConfirmAccept", orderID, err)
		return nil, err
	}
	return hallOrderFromDocument(&doc), nil
}

// ReleaseToWaiting 退单/超时回收：回到等待并记入排除名单。
func (r *orderRepo) ReleaseToWaiting(ctx context.Context, orderID, releasedBy, responsibleParty, reason string) (*entity.HallOrder, error) {
	now := biztime.UTC()
	zero := time.Time{}
	filter := bson.M{
		"_id":       orderID,
		"driver_id": releasedBy,
		"status":    bson.M{"$in": []string{entity.OrderStatusWaiting, entity.OrderStatusAccepted}},
	}
	update := bson.M{
		"$set": bson.M{
			"status":            entity.OrderStatusWaiting,
			"driver_id":         "",
			"driver_name":       "",
			"assigned_at":       zero,
			"accept_deadline":   zero,
			"responsible_party": responsibleParty,
			"cancel_reason":     reason,
			"updated_at":        now,
		},
		"$addToSet": bson.M{"assign_exclude": releasedBy},
		"$inc":      bson.M{"version": int64(1)},
	}
	var doc dbmodel.HallOrderDocument
	err := r.coll().FindOneAndUpdate(ctx, filter, update, options.FindOneAndUpdate().SetReturnDocument(options.After)).Decode(&doc)
	if isNoDocuments(err) {
		return nil, entity.ErrOrderNotAvailable
	}
	if err != nil {
		logRepoOp(r.log, "order.ReleaseToWaiting", orderID, err)
		return nil, err
	}
	return hallOrderFromDocument(&doc), nil
}

// StartTrip 行程开始：ACCEPTED → IN_TRIP（由行程域/司机端调用）。
func (r *orderRepo) StartTrip(ctx context.Context, orderID, driverID string) (*entity.HallOrder, error) {
	now := biztime.UTC()
	filter := bson.M{"_id": orderID, "driver_id": driverID, "status": entity.OrderStatusAccepted}
	update := bson.M{
		"$set": bson.M{"status": entity.OrderStatusInTrip, "started_at": now, "updated_at": now},
		"$inc": bson.M{"version": int64(1)},
	}
	var doc dbmodel.HallOrderDocument
	err := r.coll().FindOneAndUpdate(ctx, filter, update, options.FindOneAndUpdate().SetReturnDocument(options.After)).Decode(&doc)
	if isNoDocuments(err) {
		return nil, entity.ErrOrderNotAvailable
	}
	if err != nil {
		logRepoOp(r.log, "order.StartTrip", orderID, err)
		return nil, err
	}
	return hallOrderFromDocument(&doc), nil
}

// Complete 送达结束：释放运力，行程进入 ENDED（与付款无关）。
func (r *orderRepo) Complete(ctx context.Context, orderID, driverID string, version int64, distanceMeters, durationSec int64) (*entity.HallOrder, error) {
	now := biztime.UTC()
	filter := bson.M{
		"_id":       orderID,
		"driver_id": driverID,
		"status":    bson.M{"$in": []string{entity.OrderStatusAccepted, entity.OrderStatusArrived, entity.OrderStatusInTrip}},
	}
	if version > 0 {
		filter["version"] = version
	}
	update := bson.M{
		"$set": bson.M{
			"status":          entity.OrderStatusEnded,
			"ended_at":        now,
			"distance_meters": distanceMeters,
			"duration_sec":    durationSec,
			"updated_at":      now,
		},
		"$inc": bson.M{"version": int64(1)},
	}
	var doc dbmodel.HallOrderDocument
	err := r.coll().FindOneAndUpdate(ctx, filter, update, options.FindOneAndUpdate().SetReturnDocument(options.After)).Decode(&doc)
	if isNoDocuments(err) {
		return nil, entity.ErrOrderNotAvailable
	}
	if err != nil {
		logRepoOp(r.log, "order.Complete", orderID, err)
		return nil, err
	}
	return hallOrderFromDocument(&doc), nil
}

// Cancel 取消：乘客取消 / 平台责任取消。已结束与已取消的任务不可再取消。
func (r *orderRepo) Cancel(ctx context.Context, orderID, responsibleParty, reason string) (*entity.HallOrder, error) {
	now := biztime.UTC()
	filter := bson.M{
		"_id":    orderID,
		"status": bson.M{"$nin": []string{entity.OrderStatusEnded, entity.OrderStatusCancelled}},
	}
	update := bson.M{
		"$set": bson.M{
			"status":            entity.OrderStatusCancelled,
			"cancelled_at":      now,
			"responsible_party": responsibleParty,
			"cancel_reason":     reason,
			"updated_at":        now,
		},
		"$inc": bson.M{"version": int64(1)},
	}
	var doc dbmodel.HallOrderDocument
	err := r.coll().FindOneAndUpdate(ctx, filter, update, options.FindOneAndUpdate().SetReturnDocument(options.After)).Decode(&doc)
	if isNoDocuments(err) {
		return nil, entity.ErrOrderNotAvailable
	}
	if err != nil {
		logRepoOp(r.log, "order.Cancel", orderID, err)
		return nil, err
	}
	return hallOrderFromDocument(&doc), nil
}

// OpenToGrabPool 听单窗口结束后转抢单池曝光：清空本轮邀请记录，进入新一轮。
func (r *orderRepo) OpenToGrabPool(ctx context.Context, orderID string) (*entity.HallOrder, error) {
	now := biztime.UTC()
	filter := bson.M{"_id": orderID, "status": entity.OrderStatusWaiting, "visible_in_pool": false}
	update := bson.M{
		"$set": bson.M{
			"visible_in_pool": true,
			"dispatch_channel": entity.DispatchChannelGrabPool,
			"round_invited":    []string{},
			"updated_at":       now,
		},
		"$inc": bson.M{"version": int64(1)},
	}
	var doc dbmodel.HallOrderDocument
	err := r.coll().FindOneAndUpdate(ctx, filter, update, options.FindOneAndUpdate().SetReturnDocument(options.After)).Decode(&doc)
	if isNoDocuments(err) {
		return nil, entity.ErrOrderNotAvailable
	}
	if err != nil {
		logRepoOp(r.log, "order.OpenToGrabPool", orderID, err)
		return nil, err
	}
	return hallOrderFromDocument(&doc), nil
}

func (r *orderRepo) ListPool(ctx context.Context, driver *entity.Driver, lat, lng float64, limit int) ([]*entity.HallOrder, error) {
	now := biztime.UTC()
	if limit <= 0 {
		limit = 50
	}
	filter := bson.M{
		"status":            entity.OrderStatusWaiting,
		"visible_in_pool":   true,
		"dispatch_deadline": bson.M{"$gt": now},
		"$or":               []bson.M{{"driver_id": ""}, {"accept_deadline": bson.M{"$lt": now}}},
	}
	cur, err := r.coll().Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}}).SetLimit(int64(limit)))
	if err != nil {
		logRepoOp(r.log, "order.ListPool", "", err)
		return nil, err
	}
	defer cur.Close(ctx)
	out := make([]*entity.HallOrder, 0, limit)
	for cur.Next(ctx) {
		var doc dbmodel.HallOrderDocument
		if err := cur.Decode(&doc); err != nil {
			continue
		}
		out = append(out, hallOrderFromDocument(&doc))
	}
	return out, nil
}

func (r *orderRepo) findOrders(ctx context.Context, filter bson.M, limit int) ([]*entity.HallOrder, error) {
	if limit <= 0 {
		limit = 50
	}
	cur, err := r.coll().Find(ctx, filter, options.Find().SetLimit(int64(limit)))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	out := make([]*entity.HallOrder, 0, limit)
	for cur.Next(ctx) {
		var doc dbmodel.HallOrderDocument
		if err := cur.Decode(&doc); err != nil {
			continue
		}
		out = append(out, hallOrderFromDocument(&doc))
	}
	return out, nil
}

func (r *orderRepo) FindPendingInviteExpired(ctx context.Context, now time.Time, limit int) ([]*entity.HallOrder, error) {
	filter := bson.M{
		"status":            entity.OrderStatusWaiting,
		"driver_id":         bson.M{"$ne": ""},
		"accept_deadline":   bson.M{"$lt": now},
		"dispatch_deadline": bson.M{"$gt": now},
	}
	out, err := r.findOrders(ctx, filter, limit)
	logRepoOp(r.log, "order.FindPendingInviteExpired", "", err)
	return out, err
}

func (r *orderRepo) FindListeningTasks(ctx context.Context, now time.Time, limit int) ([]*entity.HallOrder, error) {
	filter := bson.M{
		"status":            entity.OrderStatusWaiting,
		"dispatch_channel":  entity.DispatchChannelListen,
		"listen_deadline":   bson.M{"$gt": now},
		"dispatch_deadline": bson.M{"$gt": now},
		"$or":               []bson.M{{"driver_id": ""}, {"accept_deadline": bson.M{"$lt": now}}},
	}
	out, err := r.findOrders(ctx, filter, limit)
	logRepoOp(r.log, "order.FindListeningTasks", "", err)
	return out, err
}

func (r *orderRepo) FindWindowSwitchable(ctx context.Context, now time.Time, limit int) ([]*entity.HallOrder, error) {
	filter := bson.M{
		"status":            entity.OrderStatusWaiting,
		"visible_in_pool":   false,
		"listen_deadline":   bson.M{"$lte": now},
		"dispatch_deadline": bson.M{"$gt": now},
	}
	out, err := r.findOrders(ctx, filter, limit)
	logRepoOp(r.log, "order.FindWindowSwitchable", "", err)
	return out, err
}

func (r *orderRepo) FindDispatchExpired(ctx context.Context, now time.Time, limit int) ([]*entity.HallOrder, error) {
	filter := bson.M{
		"status":            entity.OrderStatusWaiting,
		"dispatch_deadline": bson.M{"$lte": now},
	}
	out, err := r.findOrders(ctx, filter, limit)
	logRepoOp(r.log, "order.FindDispatchExpired", "", err)
	return out, err
}

// ReleasePendingForDriver 释放司机名下所有待确认的推送任务（模式切换时清空队列）。
func (r *orderRepo) ReleasePendingForDriver(ctx context.Context, driverID string) (int64, error) {
	now := biztime.UTC()
	zero := time.Time{}
	res, err := r.coll().UpdateMany(ctx, bson.M{
		"status":    entity.OrderStatusWaiting,
		"driver_id": driverID,
	}, bson.M{
		"$set": bson.M{
			"driver_id": "", "driver_name": "", "assigned_at": zero,
			"accept_deadline": zero, "updated_at": now,
		},
		"$addToSet": bson.M{"assign_exclude": driverID},
		"$inc":      bson.M{"version": int64(1)},
	})
	logRepoOp(r.log, "order.ReleasePendingForDriver", driverID, err)
	if err != nil {
		return 0, err
	}
	return res.ModifiedCount, nil
}

// SumEndedByDay 按行程结束日统计：完单量与乘客数（拼车两乘客算 2 单）。
func (r *orderRepo) SumEndedByDay(ctx context.Context, driverID string, dayStart, dayEnd time.Time) (int64, int64, error) {
	cur, err := r.coll().Find(ctx, bson.M{
		"driver_id": driverID,
		"status":    entity.OrderStatusEnded,
		"ended_at":  bson.M{"$gte": dayStart, "$lt": dayEnd},
	})
	if err != nil {
		logRepoOp(r.log, "order.SumEndedByDay", driverID, err)
		return 0, 0, err
	}
	defer cur.Close(ctx)
	var orders, passengers int64
	for cur.Next(ctx) {
		var doc dbmodel.HallOrderDocument
		if err := cur.Decode(&doc); err != nil {
			continue
		}
		orders++
		if doc.PassengerCount > 0 {
			passengers += int64(doc.PassengerCount)
		} else {
			passengers++
		}
	}
	return orders, passengers, nil
}

// ─────────────────────────── 文档转换 ───────────────────────────

func toDriverDocument(d *entity.Driver) dbmodel.DriverDocument {
	areas := make([]dbmodel.AreaBoxDoc, 0, len(d.Areas))
	for _, a := range d.Areas {
		areas = append(areas, dbmodel.AreaBoxDoc{Name: a.Name, MinLat: a.MinLat, MinLng: a.MinLng, MaxLat: a.MaxLat, MaxLng: a.MaxLng})
	}
	rels := make([]dbmodel.ReleaseRecordDoc, 0, len(d.RecentReleases))
	for _, r := range d.RecentReleases {
		rels = append(rels, dbmodel.ReleaseRecordDoc{At: r.At, Party: r.Party})
	}
	return dbmodel.DriverDocument{
		ID: d.ID, DriverNo: d.DriverNo, Name: d.Name, Phone: d.Phone,
		Status: d.Status, Mode: d.Mode, ModeVersion: d.ModeVersion,
		AreaLimitSwitch: d.AreaLimitSwitch, Areas: areas,
		CityCode: d.CityCode, CarType: d.CarType, Seats: d.Seats,
		Qualified: d.Qualified, CanAccept: d.CanAccept,
		OfflineAfterTrip: d.OfflineAfterTrip,
		LastHeartbeatAt:  d.LastHeartbeatAt, ActiveTaskID: d.ActiveTaskID,
		LastLat: d.LastLat, LastLng: d.LastLng,
		OnlineSinceAt: d.OnlineSinceAt,
		MonthlyIncomeFen: d.MonthlyIncomeFen, Rating: d.Rating, SuccessRate: d.SuccessRate,
		Funds: dbmodel.FundsDoc{
			PendingFen: d.Funds.Pending, AvailableFen: d.Funds.Available,
			FrozenFen: d.Funds.Frozen, WithdrawFen: d.Funds.Withdraw, DebtFen: d.Funds.Debt,
		},
		RecentReleases: rels,
		Version:        d.Version,
		CreatedAt:      d.CreatedAt, UpdatedAt: d.UpdatedAt,
	}
}

func driverFromDocument(doc *dbmodel.DriverDocument) *entity.Driver {
	areas := make([]entity.AreaBox, 0, len(doc.Areas))
	for _, a := range doc.Areas {
		areas = append(areas, entity.AreaBox{Name: a.Name, MinLat: a.MinLat, MinLng: a.MinLng, MaxLat: a.MaxLat, MaxLng: a.MaxLng})
	}
	rels := make([]entity.ReleaseRecord, 0, len(doc.RecentReleases))
	for _, r := range doc.RecentReleases {
		rels = append(rels, entity.ReleaseRecord{At: r.At, Party: r.Party})
	}
	return &entity.Driver{
		ID: doc.ID, DriverNo: doc.DriverNo, Name: doc.Name, Phone: doc.Phone,
		Status: doc.Status, Mode: doc.Mode, ModeVersion: doc.ModeVersion,
		AreaLimitSwitch: doc.AreaLimitSwitch, Areas: areas,
		CityCode: doc.CityCode, CarType: doc.CarType, Seats: doc.Seats,
		Qualified: doc.Qualified, CanAccept: doc.CanAccept,
		OfflineAfterTrip: doc.OfflineAfterTrip,
		LastHeartbeatAt:  doc.LastHeartbeatAt, ActiveTaskID: doc.ActiveTaskID,
		LastLat: doc.LastLat, LastLng: doc.LastLng,
		OnlineSinceAt: doc.OnlineSinceAt,
		MonthlyIncomeFen: doc.MonthlyIncomeFen, Rating: doc.Rating, SuccessRate: doc.SuccessRate,
		Funds: entity.Funds{
			Pending: doc.Funds.PendingFen, Available: doc.Funds.AvailableFen,
			Frozen: doc.Funds.FrozenFen, Withdraw: doc.Funds.WithdrawFen, Debt: doc.Funds.DebtFen,
		},
		RecentReleases: rels,
		Version:        doc.Version,
		CreatedAt:      doc.CreatedAt, UpdatedAt: doc.UpdatedAt,
	}
}

func toHallOrderDocument(o *entity.HallOrder) dbmodel.HallOrderDocument {
	copyIDs := func(in []string) []string {
		out := make([]string, 0, len(in))
		out = append(out, in...)
		return out
	}
	return dbmodel.HallOrderDocument{
		ID: o.ID, TaskID: o.TaskID, OrderID: o.OrderID,
		PassengerName: o.PassengerName, PassengerCount: o.PassengerCount,
		CityCode: o.CityCode, OrderType: o.OrderType,
		RequireSeats: o.RequireSeats, RequireCarType: o.RequireCarType,
		Origin:      dbmodel.LocationDoc{Name: o.Origin.Name, Lat: o.Origin.Lat, Lng: o.Origin.Lng},
		Destination: dbmodel.LocationDoc{Name: o.Destination.Name, Lat: o.Destination.Lat, Lng: o.Destination.Lng},
		OrigFareFen: o.OrigFareFen, PassengerPaidFen: o.PassengerPaidFen,
		PlatformCouponFen: o.PlatformCouponFen, DriverCouponFen: o.DriverCouponFen,
		CommissionFen: o.CommissionFen, SubsidyFen: o.SubsidyFen, DriverIncomeFen: o.DriverIncomeFen,
		Status: o.Status, DriverID: o.DriverID, DriverName: o.DriverName,
		DispatchChannel: o.DispatchChannel, VisibleInPool: o.VisibleInPool,
		PickupDistanceMeters: o.PickupDistanceMeters,
		InviteRound: o.InviteRound, RoundInvited: copyIDs(o.RoundInvited), AssignExclude: copyIDs(o.AssignExclude),
		AcceptDeadline: o.AcceptDeadline, ListenDeadline: o.ListenDeadline,
		PoolVisibleAt: o.PoolVisibleAt, DispatchDeadline: o.DispatchDeadline,
		AssignedAt: o.AssignedAt, AcceptedAt: o.AcceptedAt, ArrivedAt: o.ArrivedAt,
		StartedAt: o.StartedAt, EndedAt: o.EndedAt, CancelledAt: o.CancelledAt,
		DurationSec: o.DurationSec, DistanceMeters: o.DistanceMeters,
		CancelReason: o.CancelReason, ResponsibleParty: o.ResponsibleParty,
		Version: o.Version, CreatedAt: o.CreatedAt, UpdatedAt: o.UpdatedAt,
	}
}

func hallOrderFromDocument(doc *dbmodel.HallOrderDocument) *entity.HallOrder {
	return &entity.HallOrder{
		ID: doc.ID, TaskID: doc.TaskID, OrderID: doc.OrderID,
		PassengerName: doc.PassengerName, PassengerCount: doc.PassengerCount,
		CityCode: doc.CityCode, OrderType: doc.OrderType,
		RequireSeats: doc.RequireSeats, RequireCarType: doc.RequireCarType,
		Origin:      entity.Location{Name: doc.Origin.Name, Lat: doc.Origin.Lat, Lng: doc.Origin.Lng},
		Destination: entity.Location{Name: doc.Destination.Name, Lat: doc.Destination.Lat, Lng: doc.Destination.Lng},
		OrigFareFen: doc.OrigFareFen, PassengerPaidFen: doc.PassengerPaidFen,
		PlatformCouponFen: doc.PlatformCouponFen, DriverCouponFen: doc.DriverCouponFen,
		CommissionFen: doc.CommissionFen, SubsidyFen: doc.SubsidyFen, DriverIncomeFen: doc.DriverIncomeFen,
		Status: doc.Status, DriverID: doc.DriverID, DriverName: doc.DriverName,
		DispatchChannel: doc.DispatchChannel, VisibleInPool: doc.VisibleInPool,
		PickupDistanceMeters: doc.PickupDistanceMeters,
		InviteRound: doc.InviteRound, RoundInvited: doc.RoundInvited, AssignExclude: doc.AssignExclude,
		AcceptDeadline: doc.AcceptDeadline, ListenDeadline: doc.ListenDeadline,
		PoolVisibleAt: doc.PoolVisibleAt, DispatchDeadline: doc.DispatchDeadline,
		AssignedAt: doc.AssignedAt, AcceptedAt: doc.AcceptedAt, ArrivedAt: doc.ArrivedAt,
		StartedAt: doc.StartedAt, EndedAt: doc.EndedAt, CancelledAt: doc.CancelledAt,
		DurationSec: doc.DurationSec, DistanceMeters: doc.DistanceMeters,
		CancelReason: doc.CancelReason, ResponsibleParty: doc.ResponsibleParty,
		Version: doc.Version, CreatedAt: doc.CreatedAt, UpdatedAt: doc.UpdatedAt,
	}
}
