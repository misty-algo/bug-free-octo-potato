package service

import (
	"context"
	"sync"
	"time"

	"github.com/BwCloudWeGo/bw-cli/internal/passenger/entity"
)

// fakeAccountRepository 是账号仓储的内存实现，仅用于测试。
type fakeAccountRepository struct {
	mutex    sync.Mutex
	accounts map[string]*entity.Account
	phones   map[string]string
}

func newFakeAccountRepository() *fakeAccountRepository {
	return &fakeAccountRepository{accounts: map[string]*entity.Account{}, phones: map[string]string{}}
}

func (r *fakeAccountRepository) FindByID(ctx context.Context, id string) (*entity.Account, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	account, ok := r.accounts[id]
	if !ok {
		return nil, entity.ErrAccountNotFound
	}
	copied := *account
	return &copied, nil
}

func (r *fakeAccountRepository) FindByPhone(ctx context.Context, phone string) (*entity.Account, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	id, ok := r.phones[phone]
	if !ok {
		return nil, entity.ErrAccountNotFound
	}
	copied := *r.accounts[id]
	return &copied, nil
}

func (r *fakeAccountRepository) Save(ctx context.Context, account *entity.Account) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	copied := *account
	r.accounts[account.ID] = &copied
	if account.Phone != "" {
		r.phones[account.Phone] = account.ID
	}
	return nil
}

func (r *fakeAccountRepository) UpdatePhone(ctx context.Context, account *entity.Account) error {
	return r.Save(ctx, account)
}

// fakeChannelRepository 是渠道绑定仓储的内存实现。
type fakeChannelRepository struct {
	mutex    sync.Mutex
	bindings map[string]*entity.ChannelBinding
}

func newFakeChannelRepository() *fakeChannelRepository {
	return &fakeChannelRepository{bindings: map[string]*entity.ChannelBinding{}}
}

func (r *fakeChannelRepository) FindByChannelUID(ctx context.Context, channel entity.Channel, channelUID string) (*entity.ChannelBinding, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	for _, binding := range r.bindings {
		if binding.Channel == channel && binding.ChannelUID == channelUID {
			copied := *binding
			return &copied, nil
		}
	}
	return nil, entity.ErrAccountNotFound
}

func (r *fakeChannelRepository) Save(ctx context.Context, binding *entity.ChannelBinding) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	copied := *binding
	r.bindings[binding.ID] = &copied
	return nil
}

func (r *fakeChannelRepository) Delete(ctx context.Context, id string) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	delete(r.bindings, id)
	return nil
}

func (r *fakeChannelRepository) ListByAccount(ctx context.Context, accountID string) ([]*entity.ChannelBinding, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	list := make([]*entity.ChannelBinding, 0)
	for _, binding := range r.bindings {
		if binding.AccountID == accountID {
			copied := *binding
			list = append(list, &copied)
		}
	}
	return list, nil
}

// seed 预置一条渠道绑定，供测试构造多渠道场景。
func (r *fakeChannelRepository) seed(binding *entity.ChannelBinding) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	copied := *binding
	r.bindings[binding.ID] = &copied
}

// fakeSessionRepository 是会话仓储的内存实现。
type fakeSessionRepository struct {
	mutex    sync.Mutex
	sessions map[string]*entity.Session
}

func newFakeSessionRepository() *fakeSessionRepository {
	return &fakeSessionRepository{sessions: map[string]*entity.Session{}}
}

func (r *fakeSessionRepository) Save(ctx context.Context, session *entity.Session) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	copied := *session
	r.sessions[session.ID] = &copied
	return nil
}

func (r *fakeSessionRepository) FindByID(ctx context.Context, id string) (*entity.Session, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	session, ok := r.sessions[id]
	if !ok {
		return nil, entity.ErrAccountNotFound
	}
	copied := *session
	return &copied, nil
}

func (r *fakeSessionRepository) RevokeByAccount(ctx context.Context, accountID string, now time.Time) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	for _, session := range r.sessions {
		if session.AccountID == accountID {
			session.UpdatedAt = now
			session.ExpiresAt = now
		}
	}
	return nil
}

// fakeCodeRepository 是验证码仓储的内存实现。
type fakeCodeRepository struct {
	mutex sync.Mutex
	codes map[string]*entity.VerificationCode
	// history 记录下发时间，用于小时级限流统计。
	history map[string][]time.Time
}

func newFakeCodeRepository() *fakeCodeRepository {
	return &fakeCodeRepository{
		codes:   map[string]*entity.VerificationCode{},
		history: map[string][]time.Time{},
	}
}

func (r *fakeCodeRepository) Save(ctx context.Context, code *entity.VerificationCode) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	copied := *code
	r.codes[code.Phone] = &copied
	return nil
}

func (r *fakeCodeRepository) Load(ctx context.Context, phone string) (*entity.VerificationCode, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	code, ok := r.codes[phone]
	if !ok {
		return nil, entity.ErrAccountNotFound
	}
	copied := *code
	return &copied, nil
}

func (r *fakeCodeRepository) CountRecentHourly(ctx context.Context, phone string, since time.Time) (int, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	count := 0
	for _, at := range r.history[phone] {
		if at.After(since) {
			count++
		}
	}
	return count, nil
}

// recordSend 记录一次下发，供限流统计。
func (r *fakeCodeRepository) recordSend(phone string, at time.Time) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.history[phone] = append(r.history[phone], at)
}

// fakeEstimateRepository 是估价单仓储的内存实现。
type fakeEstimateRepository struct {
	mutex    sync.Mutex
	estimates map[string]*entity.Estimate
}

func newFakeEstimateRepository() *fakeEstimateRepository {
	return &fakeEstimateRepository{estimates: map[string]*entity.Estimate{}}
}

func (r *fakeEstimateRepository) Save(ctx context.Context, estimate *entity.Estimate) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	copied := *estimate
	r.estimates[estimate.No] = &copied
	return nil
}

func (r *fakeEstimateRepository) FindByNo(ctx context.Context, no string) (*entity.Estimate, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	estimate, ok := r.estimates[no]
	if !ok {
		return nil, entity.ErrEstimateNotFound
	}
	copied := *estimate
	return &copied, nil
}

func (r *fakeEstimateRepository) Consume(ctx context.Context, estimate *entity.Estimate) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	current, ok := r.estimates[estimate.No]
	if !ok {
		return entity.ErrEstimateNotFound
	}
	if current.Used {
		return entity.ErrEstimateUsed
	}
	current.Used = true
	current.UpdatedAt = estimate.UpdatedAt
	return nil
}

// fakeAddressRepository 是地址仓储的内存实现。
type fakeAddressRepository struct {
	mutex    sync.Mutex
	addresses map[string]*entity.Address
}

func newFakeAddressRepository() *fakeAddressRepository {
	return &fakeAddressRepository{addresses: map[string]*entity.Address{}}
}

func (r *fakeAddressRepository) Save(ctx context.Context, address *entity.Address) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	copied := *address
	r.addresses[address.ID] = &copied
	return nil
}

func (r *fakeAddressRepository) FindByID(ctx context.Context, id string) (*entity.Address, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	address, ok := r.addresses[id]
	if !ok {
		return nil, entity.ErrAddressNotFound
	}
	copied := *address
	return &copied, nil
}

func (r *fakeAddressRepository) Delete(ctx context.Context, id string) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	delete(r.addresses, id)
	return nil
}

func (r *fakeAddressRepository) ListByType(ctx context.Context, accountID string, addrType entity.AddressType, limit int) ([]*entity.Address, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	list := make([]*entity.Address, 0)
	for _, address := range r.addresses {
		if address.AccountID == accountID && address.Type == addrType {
			copied := *address
			list = append(list, &copied)
		}
	}
	return list, nil
}

func (r *fakeAddressRepository) CountByType(ctx context.Context, accountID string, addrType entity.AddressType) (int, error) {
	list, err := r.ListByType(ctx, accountID, addrType, 0)
	return len(list), err
}

func (r *fakeAddressRepository) ClearHistory(ctx context.Context, accountID string) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	for id, address := range r.addresses {
		if address.AccountID == accountID && address.Type == entity.AddressTypeHistory {
			delete(r.addresses, id)
		}
	}
	return nil
}

// fakeOrderRepository 是订单仓储的内存实现。
type fakeOrderRepository struct {
	mutex  sync.Mutex
	orders map[string]*entity.Order
	logs   []*entity.OrderStatusLog
}

func newFakeOrderRepository() *fakeOrderRepository {
	return &fakeOrderRepository{orders: map[string]*entity.Order{}}
}

func (r *fakeOrderRepository) Save(ctx context.Context, order *entity.Order) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	copied := *order
	r.orders[order.ID] = &copied
	return nil
}

func (r *fakeOrderRepository) FindByID(ctx context.Context, id string) (*entity.Order, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	order, ok := r.orders[id]
	if !ok {
		return nil, entity.ErrOrderNotFound
	}
	copied := *order
	return &copied, nil
}

func (r *fakeOrderRepository) FindByEstimateNo(ctx context.Context, estimateNo string) (*entity.Order, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	for _, order := range r.orders {
		if order.EstimateNo == estimateNo {
			copied := *order
			return &copied, nil
		}
	}
	return nil, entity.ErrOrderNotFound
}

func (r *fakeOrderRepository) FindByIdempotencyKey(ctx context.Context, accountID, key string) (*entity.Order, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	for _, order := range r.orders {
		if order.AccountID == accountID && order.IdempotencyKey == key {
			copied := *order
			return &copied, nil
		}
	}
	return nil, entity.ErrOrderNotFound
}

func (r *fakeOrderRepository) FindActive(ctx context.Context, accountID string) (*entity.Order, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	for _, order := range r.orders {
		if order.AccountID == accountID && !order.State.Terminal() {
			copied := *order
			return &copied, nil
		}
	}
	return nil, entity.ErrOrderNotFound
}

func (r *fakeOrderRepository) ListByAccount(ctx context.Context, accountID string, state entity.OrderState, offset int, limit int) ([]*entity.Order, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	matched := make([]*entity.Order, 0, len(r.orders))
	for _, order := range r.orders {
		if order.AccountID != accountID {
			continue
		}
		if state != "" && order.State != state {
			continue
		}
		copied := *order
		matched = append(matched, &copied)
	}
	if offset > 0 {
		if offset >= len(matched) {
			return nil, nil
		}
		matched = matched[offset:]
	}
	if limit > 0 && len(matched) > limit {
		matched = matched[:limit]
	}
	return matched, nil
}

func (r *fakeOrderRepository) CountByAccount(ctx context.Context, accountID string, state entity.OrderState) (int, error) {
	list, err := r.ListByAccount(ctx, accountID, state, 0, 0)
	return len(list), err
}

func (r *fakeOrderRepository) UpdateState(ctx context.Context, order *entity.Order) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.orders[order.ID] = order
	return nil
}

func (r *fakeOrderRepository) AppendStatusLog(ctx context.Context, log *entity.OrderStatusLog) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.logs = append(r.logs, log)
	return nil
}

// fakePOIRepository 是地点仓储的内存实现。
type fakePOIRepository struct {
	pois []*entity.POI
}

func (r *fakePOIRepository) Search(ctx context.Context, keyword, city string, lng, lat float64, limit int) ([]*entity.POI, error) {
	return r.pois, nil
}

func (r *fakePOIRepository) Recommend(ctx context.Context, lng, lat float64, limit int) ([]*entity.POI, error) {
	return r.pois, nil
}

// fakeQuickEntryRepository 是快速入口仓储的内存实现。
type fakeQuickEntryRepository struct {
	mutex   sync.Mutex
	entries []*entity.QuickEntry
	used    map[string][]string
}

func newFakeQuickEntryRepository() *fakeQuickEntryRepository {
	return &fakeQuickEntryRepository{used: map[string][]string{}}
}

func (r *fakeQuickEntryRepository) ListEnabled(ctx context.Context, now time.Time) ([]*entity.QuickEntry, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	return r.entries, nil
}

func (r *fakeQuickEntryRepository) ListUsedEntryIDs(ctx context.Context, accountID string) ([]string, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	return r.used[accountID], nil
}

func (r *fakeQuickEntryRepository) MarkUsed(ctx context.Context, accountID, entryID string, now time.Time) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	for _, id := range r.used[accountID] {
		if id == entryID {
			return nil
		}
	}
	r.used[accountID] = append(r.used[accountID], entryID)
	return nil
}

// ensure the fakes keep compile-time contracts with the entity ports.
var (
	_ entity.AccountRepository    = (*fakeAccountRepository)(nil)
	_ entity.ChannelRepository    = (*fakeChannelRepository)(nil)
	_ entity.SessionRepository    = (*fakeSessionRepository)(nil)
	_ entity.CodeRepository       = (*fakeCodeRepository)(nil)
	_ entity.EstimateRepository   = (*fakeEstimateRepository)(nil)
	_ entity.AddressRepository    = (*fakeAddressRepository)(nil)
	_ entity.OrderRepository      = (*fakeOrderRepository)(nil)
	_ entity.POIRepository        = (*fakePOIRepository)(nil)
	_ entity.QuickEntryRepository = (*fakeQuickEntryRepository)(nil)
)
