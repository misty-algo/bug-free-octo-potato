// Package repo 实现乘客端领域仓储的 MongoDB 版本。
package repo

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/BwCloudWeGo/bw-cli/internal/passenger/entity"
	"github.com/BwCloudWeGo/bw-cli/internal/passenger/model"
	"github.com/BwCloudWeGo/bw-cli/pkg/mongox"
)

// Store 汇集乘客端全部集合句柄。
type Store struct {
	accounts    *mongox.Collection[model.AccountDocument]
	channels    *mongox.Collection[model.ChannelBindingDocument]
	sessions    *mongox.Collection[model.SessionDocument]
	codes       *mongox.Collection[model.VerificationCodeDocument]
	estimates   *mongox.Collection[model.EstimateDocument]
	addresses   *mongox.Collection[model.AddressDocument]
	orders      *mongox.Collection[model.OrderDocument]
	orderLogs   *mongox.Collection[model.OrderStatusLogDocument]
	quick       *mongox.Collection[model.QuickEntryDocument]
	usedQuick   *mongox.Collection[model.UsedQuickEntryDocument]
}

// NewStore 基于数据库创建乘客端仓储。
func NewStore(db *mongo.Database) *Store {
	return &Store{
		accounts:  mongox.NewCollection[model.AccountDocument](db, model.CollectionAccounts),
		channels:  mongox.NewCollection[model.ChannelBindingDocument](db, model.CollectionChannelBindings),
		sessions:  mongox.NewCollection[model.SessionDocument](db, model.CollectionSessions),
		codes:     mongox.NewCollection[model.VerificationCodeDocument](db, model.CollectionVerificationCodes),
		estimates: mongox.NewCollection[model.EstimateDocument](db, model.CollectionEstimates),
		addresses: mongox.NewCollection[model.AddressDocument](db, model.CollectionAddresses),
		orders:    mongox.NewCollection[model.OrderDocument](db, model.CollectionOrders),
		orderLogs: mongox.NewCollection[model.OrderStatusLogDocument](db, model.CollectionOrderStatusLogs),
		quick:     mongox.NewCollection[model.QuickEntryDocument](db, model.CollectionQuickEntries),
		usedQuick: mongox.NewCollection[model.UsedQuickEntryDocument](db, model.CollectionUsedQuickEntries),
	}
}

// Accounts 返回账号仓储。
func (s *Store) Accounts() *AccountRepository { return &AccountRepository{store: s} }

// Channels 返回渠道绑定仓储。
func (s *Store) Channels() *ChannelRepository { return &ChannelRepository{store: s} }

// Sessions 返回会话仓储。
func (s *Store) Sessions() *SessionRepository { return &SessionRepository{store: s} }

// Codes 返回验证码仓储。
func (s *Store) Codes() *CodeRepository { return &CodeRepository{store: s} }

// Estimates 返回估价单仓储。
func (s *Store) Estimates() *EstimateRepository { return &EstimateRepository{store: s} }

// Addresses 返回地址仓储。
func (s *Store) Addresses() *AddressRepository { return &AddressRepository{store: s} }

// Orders 返回订单仓储。
func (s *Store) Orders() *OrderRepository { return &OrderRepository{store: s} }

// QuickEntries 返回快速入口仓储。
func (s *Store) QuickEntries() *QuickEntryRepository { return &QuickEntryRepository{store: s} }

// AccountRepository 是账号的 MongoDB 仓储实现。
type AccountRepository struct {
	store *Store
}

// FindByID 按标识查账号。
func (r *AccountRepository) FindByID(ctx context.Context, id string) (*entity.Account, error) {
	document, err := r.store.accounts.FindOne(ctx, bson.M{"_id": id, "is_deleted": false})
	if err != nil {
		if errors.Is(err, mongox.ErrNotFound) {
			return nil, entity.ErrAccountNotFound
		}
		return nil, err
	}
	return toAccountEntity(document), nil
}

// FindByPhone 按手机号哈希查账号。手机号以密文落库、等值查询走哈希字段。
func (r *AccountRepository) FindByPhone(ctx context.Context, phone string) (*entity.Account, error) {
	document, err := r.store.accounts.FindOne(ctx, bson.M{
		"phone_hash": HashPhone(phone),
		"is_deleted": false,
	})
	if err != nil {
		if errors.Is(err, mongox.ErrNotFound) {
			return nil, entity.ErrAccountNotFound
		}
		return nil, err
	}
	return toAccountEntity(document), nil
}

// Save 保存账号。
func (r *AccountRepository) Save(ctx context.Context, account *entity.Account) error {
	document := toAccountDocument(account)
	if _, err := r.store.accounts.ReplaceByID(ctx, account.ID, document); err != nil {
		return err
	}
	return nil
}

// UpdatePhone 在乐观锁校验通过后更新手机号。
func (r *AccountRepository) UpdatePhone(ctx context.Context, account *entity.Account) error {
	return r.Save(ctx, account)
}

// ChannelRepository 是渠道绑定的 MongoDB 仓储实现。
type ChannelRepository struct {
	store *Store
}

// FindByChannelUID 按渠道与渠道标识查绑定。
func (r *ChannelRepository) FindByChannelUID(ctx context.Context, channel entity.Channel, channelUID string) (*entity.ChannelBinding, error) {
	document, err := r.store.channels.FindOne(ctx, bson.M{
		"channel":     string(channel),
		"channel_uid": channelUID,
		"is_deleted":  false,
	})
	if err != nil {
		if errors.Is(err, mongox.ErrNotFound) {
			return nil, entity.ErrAccountNotFound
		}
		return nil, err
	}
	return toChannelEntity(document), nil
}

// Save 保存渠道绑定。
func (r *ChannelRepository) Save(ctx context.Context, binding *entity.ChannelBinding) error {
	document := &model.ChannelBindingDocument{
		ID:         binding.ID,
		AccountID:  binding.AccountID,
		Channel:    string(binding.Channel),
		ChannelUID: binding.ChannelUID,
		CreatedAt:  binding.CreatedAt,
		UpdatedAt:  binding.UpdatedAt,
	}
	if _, err := r.store.channels.ReplaceByID(ctx, binding.ID, document); err != nil {
		return err
	}
	return nil
}

// Delete 软删除渠道绑定。业务数据 MUST NOT 物理删除。
func (r *ChannelRepository) Delete(ctx context.Context, id string) error {
	_, err := r.store.channels.UpdateOne(ctx, bson.M{"_id": id}, bson.M{
		"$set": bson.M{"is_deleted": true, "updated_at": time.Now().UTC()},
	})
	return err
}

// ListByAccount 列出账号的全部渠道绑定。
func (r *ChannelRepository) ListByAccount(ctx context.Context, accountID string) ([]*entity.ChannelBinding, error) {
	documents, err := r.store.channels.FindMany(ctx, bson.M{"account_id": accountID, "is_deleted": false})
	if err != nil {
		return nil, err
	}
	list := make([]*entity.ChannelBinding, 0, len(documents))
	for i := range documents {
		list = append(list, toChannelEntity(&documents[i]))
	}
	return list, nil
}

// SessionRepository 是会话的 MongoDB 仓储实现。
type SessionRepository struct {
	store *Store
}

// Save 保存会话。
func (r *SessionRepository) Save(ctx context.Context, session *entity.Session) error {
	document := &model.SessionDocument{
		ID:               session.ID,
		AccountID:        session.AccountID,
		Audience:         string(session.Audience),
		Role:             string(session.Role),
		RefreshTokenHash: session.RefreshTokenHash,
		ExpiresAt:        session.ExpiresAt,
		CreatedAt:        session.CreatedAt,
		UpdatedAt:        session.UpdatedAt,
	}
	if _, err := r.store.sessions.ReplaceByID(ctx, session.ID, document); err != nil {
		return err
	}
	return nil
}

// FindByID 按标识查会话。
func (r *SessionRepository) FindByID(ctx context.Context, id string) (*entity.Session, error) {
	document, err := r.store.sessions.FindOne(ctx, bson.M{"_id": id, "is_deleted": false})
	if err != nil {
		if errors.Is(err, mongox.ErrNotFound) {
			return nil, entity.ErrAccountNotFound
		}
		return nil, err
	}
	return &entity.Session{
		ID:               document.ID,
		AccountID:        document.AccountID,
		Audience:         entity.Audience(document.Audience),
		Role:             entity.Role(document.Role),
		RefreshTokenHash: document.RefreshTokenHash,
		ExpiresAt:        document.ExpiresAt,
		CreatedAt:        document.CreatedAt,
		UpdatedAt:        document.UpdatedAt,
	}, nil
}

// RevokeByAccount 撤销账号全部刷新令牌。
//
// 退出登录必须让服务端令牌失效，只让前端删令牌是不够的，
// 刷新令牌七天有效，原主人必须能踢掉持有者。
func (r *SessionRepository) RevokeByAccount(ctx context.Context, accountID string, now time.Time) error {
	_, err := r.store.sessions.UpdateOne(ctx, bson.M{
		"account_id":  accountID,
		"is_deleted":  false,
		"revoked_at":  bson.M{"$exists": false},
	}, bson.M{
		"$set": bson.M{"revoked_at": now, "updated_at": now},
	})
	return err
}

// CodeRepository 是验证码的 MongoDB 仓储实现。
type CodeRepository struct {
	store *Store
}

// Save 覆盖保存该号码的验证码。
func (r *CodeRepository) Save(ctx context.Context, code *entity.VerificationCode) error {
	document := &model.VerificationCodeDocument{
		ID:        HashPhone(code.Phone),
		PhoneHash: HashPhone(code.Phone),
		Code:      code.Code,
		SentAt:    code.SentAt,
		ExpiresAt: code.ExpiresAt,
		Used:      code.Used,
		FailCount: code.FailCount,
		CreatedAt: code.SentAt,
		UpdatedAt: time.Now().UTC(),
	}
	if _, err := r.store.codes.ReplaceByID(ctx, document.ID, document); err != nil {
		return err
	}
	return nil
}

// Load 按手机号查验证码。
func (r *CodeRepository) Load(ctx context.Context, phone string) (*entity.VerificationCode, error) {
	document, err := r.store.codes.FindOne(ctx, bson.M{"_id": HashPhone(phone), "is_deleted": false})
	if err != nil {
		if errors.Is(err, mongox.ErrNotFound) {
			return nil, entity.ErrAccountNotFound
		}
		return nil, err
	}
	return &entity.VerificationCode{
		Phone:     phone,
		Code:      document.Code,
		SentAt:    document.SentAt,
		ExpiresAt: document.ExpiresAt,
		Used:      document.Used,
		FailCount: document.FailCount,
	}, nil
}

// CountRecentHourly 统计该号码近一小时的下发次数。
func (r *CodeRepository) CountRecentHourly(ctx context.Context, phone string, since time.Time) (int, error) {
	count, err := r.store.codes.Count(ctx, bson.M{
		"phone_hash": HashPhone(phone),
		"sent_at":    bson.M{"$gte": since},
	})
	if err != nil {
		return 0, err
	}
	return int(count), nil
}

// EstimateRepository 是估价单的 MongoDB 仓储实现。
type EstimateRepository struct {
	store *Store
}

// Save 保存估价单。
func (r *EstimateRepository) Save(ctx context.Context, estimate *entity.Estimate) error {
	document := &model.EstimateDocument{
		ID:             estimate.ID,
		No:             estimate.No,
		AccountID:      estimate.AccountID,
		CarType:        estimate.CarType,
		BaseFee:        estimate.BaseFee,
		ServiceFee:     estimate.ServiceFee,
		DiscountFee:    estimate.DiscountFee,
		PayFee:         estimate.PayFee,
		DistanceM:      estimate.DistanceM,
		DurationS:      estimate.DurationS,
		PricingVersion: estimate.PricingVersion,
		CouponID:       estimate.CouponID,
		Used:           estimate.Used,
		ExpiresAt:      estimate.ExpiresAt,
		CreatedAt:      estimate.CreatedAt,
		UpdatedAt:      estimate.UpdatedAt,
	}
	if _, err := r.store.estimates.ReplaceByID(ctx, estimate.ID, document); err != nil {
		return err
	}
	return nil
}

// FindByNo 按单号查估价单。
func (r *EstimateRepository) FindByNo(ctx context.Context, no string) (*entity.Estimate, error) {
	document, err := r.store.estimates.FindOne(ctx, bson.M{"no": no, "is_deleted": false})
	if err != nil {
		if errors.Is(err, mongox.ErrNotFound) {
			return nil, entity.ErrEstimateNotFound
		}
		return nil, err
	}
	return toEstimateEntity(document), nil
}

// Consume 在乐观锁校验通过后把估价单置为已消费。
//
// 用条件更新而不是先查后改，避免并发下同一张估价单创建两笔订单。
func (r *EstimateRepository) Consume(ctx context.Context, estimate *entity.Estimate) error {
	result, err := r.store.estimates.UpdateOne(ctx, bson.M{
		"no":         estimate.No,
		"used":       false,
		"is_deleted": false,
	}, bson.M{
		"$set": bson.M{"used": true, "updated_at": time.Now().UTC()},
	})
	if err != nil {
		return err
	}
	if result == nil || result.MatchedCount == 0 {
		return entity.ErrEstimateUsed
	}
	return nil
}

// AddressRepository 是地址簿的 MongoDB 仓储实现。
type AddressRepository struct {
	store *Store
}

// Save 保存地址。
func (r *AddressRepository) Save(ctx context.Context, address *entity.Address) error {
	document := &model.AddressDocument{
		ID:         address.ID,
		AccountID:  address.AccountID,
		Type:       string(address.Type),
		Tag:        address.Tag,
		Name:       address.Name,
		Addr:       address.Addr,
		POIID:      address.POIID,
		Lng:        address.Lng,
		Lat:        address.Lat,
		UseCount:   address.UseCount,
		LastUsedAt: address.LastUsedAt,
		CreatedAt:  address.CreatedAt,
		UpdatedAt:  address.UpdatedAt,
	}
	if _, err := r.store.addresses.ReplaceByID(ctx, address.ID, document); err != nil {
		return err
	}
	return nil
}

// FindByID 按标识查地址。
func (r *AddressRepository) FindByID(ctx context.Context, id string) (*entity.Address, error) {
	document, err := r.store.addresses.FindOne(ctx, bson.M{"_id": id, "is_deleted": false})
	if err != nil {
		if errors.Is(err, mongox.ErrNotFound) {
			return nil, entity.ErrAddressNotFound
		}
		return nil, err
	}
	return toAddressEntity(document), nil
}

// Delete 软删除地址。历史订单的地址快照不受影响。
func (r *AddressRepository) Delete(ctx context.Context, id string) error {
	_, err := r.store.addresses.UpdateOne(ctx, bson.M{"_id": id}, bson.M{
		"$set": bson.M{"is_deleted": true, "updated_at": time.Now().UTC()},
	})
	return err
}

// ListByType 列出该账号指定类型的地址。
func (r *AddressRepository) ListByType(ctx context.Context, accountID string, addrType entity.AddressType, limit int) ([]*entity.Address, error) {
	filter := bson.M{"account_id": accountID, "type": string(addrType), "is_deleted": false}
	opts := options.Find().SetSort(bson.D{{Key: "use_count", Value: -1}, {Key: "last_used_at", Value: -1}})
	if limit > 0 {
		opts.SetLimit(int64(limit))
	}
	documents, err := r.store.addresses.FindMany(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	list := make([]*entity.Address, 0, len(documents))
	for i := range documents {
		list = append(list, toAddressEntity(&documents[i]))
	}
	return list, nil
}

// CountByType 统计该账号指定类型的地址条数。
func (r *AddressRepository) CountByType(ctx context.Context, accountID string, addrType entity.AddressType) (int, error) {
	count, err := r.store.addresses.Count(ctx, bson.M{
		"account_id": accountID,
		"type":       string(addrType),
		"is_deleted": false,
	})
	if err != nil {
		return 0, err
	}
	return int(count), nil
}

// ClearHistory 清空该账号的历史地址。用软删除，不物理删除。
func (r *AddressRepository) ClearHistory(ctx context.Context, accountID string) error {
	_, err := r.store.addresses.UpdateOne(ctx, bson.M{
		"account_id": accountID,
		"type":       string(entity.AddressTypeHistory),
		"is_deleted": false,
	}, bson.M{
		"$set": bson.M{"is_deleted": true, "updated_at": time.Now().UTC()},
	})
	return err
}

// OrderRepository 是订单的 MongoDB 仓储实现。
type OrderRepository struct {
	store *Store
}

// Save 保存订单。
func (r *OrderRepository) Save(ctx context.Context, order *entity.Order) error {
	document := toOrderDocument(order)
	if _, err := r.store.orders.ReplaceByID(ctx, order.ID, document); err != nil {
		return err
	}
	return nil
}

// FindByID 按标识查订单。
func (r *OrderRepository) FindByID(ctx context.Context, id string) (*entity.Order, error) {
	document, err := r.store.orders.FindOne(ctx, bson.M{"_id": id, "is_deleted": false})
	if err != nil {
		if errors.Is(err, mongox.ErrNotFound) {
			return nil, entity.ErrOrderNotFound
		}
		return nil, err
	}
	return toOrderEntity(document), nil
}

// FindByEstimateNo 按估价单号查订单。
func (r *OrderRepository) FindByEstimateNo(ctx context.Context, estimateNo string) (*entity.Order, error) {
	document, err := r.store.orders.FindOne(ctx, bson.M{"estimate_no": estimateNo, "is_deleted": false})
	if err != nil {
		if errors.Is(err, mongox.ErrNotFound) {
			return nil, entity.ErrOrderNotFound
		}
		return nil, err
	}
	return toOrderEntity(document), nil
}

// FindByIdempotencyKey 按幂等键查订单。
func (r *OrderRepository) FindByIdempotencyKey(ctx context.Context, accountID, key string) (*entity.Order, error) {
	document, err := r.store.orders.FindOne(ctx, bson.M{
		"account_id":      accountID,
		"idempotency_key": key,
		"is_deleted":      false,
	})
	if err != nil {
		if errors.Is(err, mongox.ErrNotFound) {
			return nil, entity.ErrOrderNotFound
		}
		return nil, err
	}
	return toOrderEntity(document), nil
}

// FindActive 查该账号进行中的订单。
func (r *OrderRepository) FindActive(ctx context.Context, accountID string) (*entity.Order, error) {
	terminal := []string{
		string(entity.OrderStateCompleted),
		string(entity.OrderStateCancelled),
	}
	document, err := r.store.orders.FindOne(ctx, bson.M{
		"account_id": accountID,
		"is_deleted": false,
		"state":      bson.M{"$nin": terminal},
	})
	if err != nil {
		if errors.Is(err, mongox.ErrNotFound) {
			return nil, entity.ErrOrderNotFound
		}
		return nil, err
	}
	return toOrderEntity(document), nil
}

// ListByAccount 分页列出该账号的订单，可按状态过滤。
func (r *OrderRepository) ListByAccount(ctx context.Context, accountID string, state entity.OrderState, offset int, limit int) ([]*entity.Order, error) {
	return r.store.listOrders(ctx, accountID, state, offset, limit)
}

// CountByAccount 统计该账号符合条件的订单数。
func (r *OrderRepository) CountByAccount(ctx context.Context, accountID string, state entity.OrderState) (int, error) {
	return r.store.countOrders(ctx, accountID, state)
}

// UpdateState 在乐观锁校验通过后更新状态。
//
// 用条件更新而不是先查后改：过滤条件带 version，
// 并发下只有一个请求能改成功，失败方重试或报冲突。
func (r *OrderRepository) UpdateState(ctx context.Context, order *entity.Order) error {
	result, err := r.store.orders.UpdateOne(ctx, bson.M{
		"_id":         order.ID,
		"version":     order.Version,
		"is_deleted":  false,
	}, bson.M{
		"$set": bson.M{
			"state":      string(order.State),
			"cancel_fee": order.CancelFee,
			"updated_at": time.Now().UTC(),
		},
		"$inc": bson.M{"version": 1},
	})
	if err != nil {
		return err
	}
	if result == nil || result.MatchedCount == 0 {
		return entity.ErrOrderStateConflict
	}
	return nil
}

// AppendStatusLog 写一条状态留痕。该集合只追加，不修改不删除。
func (r *OrderRepository) AppendStatusLog(ctx context.Context, log *entity.OrderStatusLog) error {
	document := &model.OrderStatusLogDocument{
		ID:        log.ID,
		OrderID:   log.OrderID,
		From:      string(log.From),
		To:        string(log.To),
		Operator:  log.Operator,
		Reason:    log.Reason,
		CreatedAt: log.CreatedAt,
	}
	if _, err := r.store.orderLogs.Insert(ctx, document); err != nil {
		return err
	}
	return nil
}

// QuickEntryRepository 是快速入口的 MongoDB 仓储实现。
type QuickEntryRepository struct {
	store *Store
}

// ListEnabled 返回启用中的入口配置。
func (r *QuickEntryRepository) ListEnabled(ctx context.Context, now time.Time) ([]*entity.QuickEntry, error) {
	documents, err := r.store.quick.FindMany(ctx, bson.M{
		"enabled":    true,
		"is_deleted": false,
		"start_at":   bson.M{"$lte": now},
		"end_at":     bson.M{"$gt": now},
	}, options.Find().SetSort(bson.D{{Key: "sort", Value: 1}}))
	if err != nil {
		return nil, err
	}
	list := make([]*entity.QuickEntry, 0, len(documents))
	for i := range documents {
		list = append(list, &entity.QuickEntry{
			EntryID: documents[i].EntryID,
			Title:   documents[i].Title,
			Icon:    documents[i].Icon,
			Action:  documents[i].Action,
			Target:  documents[i].Target,
			Sort:    documents[i].Sort,
		})
	}
	return list, nil
}

// ListUsedEntryIDs 返回该账号已用过的入口。
func (r *QuickEntryRepository) ListUsedEntryIDs(ctx context.Context, accountID string) ([]string, error) {
	documents, err := r.store.usedQuick.FindMany(ctx, bson.M{"account_id": accountID})
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(documents))
	for i := range documents {
		ids = append(ids, documents[i].EntryID)
	}
	return ids, nil
}

// MarkUsed 记录该账号用过某入口。唯一索引保证不会写重复。
func (r *QuickEntryRepository) MarkUsed(ctx context.Context, accountID, entryID string, now time.Time) error {
	document := &model.UsedQuickEntryDocument{
		ID:        accountID + ":" + entryID,
		AccountID: accountID,
		EntryID:   entryID,
		UsedAt:    now,
		CreatedAt: now,
	}
	_, err := r.store.usedQuick.UpdateOne(ctx, bson.M{"_id": document.ID}, bson.M{
		"$setOnInsert": bson.M{
			"account_id": accountID,
			"entry_id":   entryID,
			"used_at":    now,
			"created_at": now,
		},
	}, options.UpdateOne().SetUpsert(true))
	return err
}

// ensure the repositories keep compile-time contracts with the entity ports.
var (
	_ entity.AccountRepository    = (*AccountRepository)(nil)
	_ entity.ChannelRepository    = (*ChannelRepository)(nil)
	_ entity.SessionRepository    = (*SessionRepository)(nil)
	_ entity.CodeRepository       = (*CodeRepository)(nil)
	_ entity.EstimateRepository   = (*EstimateRepository)(nil)
	_ entity.AddressRepository    = (*AddressRepository)(nil)
	_ entity.OrderRepository      = (*OrderRepository)(nil)
	_ entity.QuickEntryRepository = (*QuickEntryRepository)(nil)
)

// keep the mongo driver import referenced for future index helpers.
var _ = mongo.ErrNoDocuments
