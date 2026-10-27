package handler

import (
	"context"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	passengerv1 "github.com/BwCloudWeGo/bw-cli/api/gen/passenger/v1"
	"github.com/BwCloudWeGo/bw-cli/internal/passenger/dto"
	"github.com/BwCloudWeGo/bw-cli/internal/passenger/entity"
	"github.com/BwCloudWeGo/bw-cli/internal/passenger/service"
	"go.uber.org/zap"
)

// bufconnSize 是内存 gRPC 连接的缓冲区大小。
const bufconnSize = 1024 * 1024

// memStore 是集成测试用的内存仓储集合。
type memStore struct {
	accounts  map[string]*entity.Account
	phones    map[string]string
	bindings  map[string]*entity.ChannelBinding
	sessions  map[string]*entity.Session
	codes     map[string]*entity.VerificationCode
	estimates map[string]*entity.Estimate
	addresses map[string]*entity.Address
	orders    map[string]*entity.Order
	logs      []*entity.OrderStatusLog
	quick     []*entity.QuickEntry
	used      map[string][]string
}

func newMemStore() *memStore {
	return &memStore{
		accounts:  map[string]*entity.Account{},
		phones:    map[string]string{},
		bindings:  map[string]*entity.ChannelBinding{},
		sessions:  map[string]*entity.Session{},
		codes:     map[string]*entity.VerificationCode{},
		estimates: map[string]*entity.Estimate{},
		addresses: map[string]*entity.Address{},
		orders:    map[string]*entity.Order{},
		used:      map[string][]string{},
		quick: []*entity.QuickEntry{
			{EntryID: "scan_ride", Title: "扫码乘车", Icon: "scan", Action: "scan", Sort: 1},
			{EntryID: "reserve_ride", Title: "预约用车", Icon: "clock", Action: "navigate", Target: "/reserve", Sort: 2},
		},
	}
}

func (s *memStore) FindByID(ctx context.Context, id string) (*entity.Account, error) {
	account, ok := s.accounts[id]
	if !ok {
		return nil, entity.ErrAccountNotFound
	}
	copied := *account
	return &copied, nil
}

func (s *memStore) FindByPhone(ctx context.Context, phone string) (*entity.Account, error) {
	id, ok := s.phones[phone]
	if !ok {
		return nil, entity.ErrAccountNotFound
	}
	copied := *s.accounts[id]
	return &copied, nil
}

func (s *memStore) Save(ctx context.Context, account *entity.Account) error {
	copied := *account
	s.accounts[account.ID] = &copied
	if account.Phone != "" {
		s.phones[account.Phone] = account.ID
	}
	return nil
}

func (s *memStore) UpdatePhone(ctx context.Context, account *entity.Account) error {
	return s.Save(ctx, account)
}

func (s *memStore) FindByChannelUID(ctx context.Context, channel entity.Channel, channelUID string) (*entity.ChannelBinding, error) {
	for _, binding := range s.bindings {
		if binding.Channel == channel && binding.ChannelUID == channelUID {
			copied := *binding
			return &copied, nil
		}
	}
	return nil, entity.ErrAccountNotFound
}

func (s *memStore) SaveBinding(ctx context.Context, binding *entity.ChannelBinding) error {
	copied := *binding
	s.bindings[binding.ID] = &copied
	return nil
}

func (s *memStore) DeleteBinding(ctx context.Context, id string) error {
	delete(s.bindings, id)
	return nil
}

func (s *memStore) ListBindings(ctx context.Context, accountID string) ([]*entity.ChannelBinding, error) {
	list := make([]*entity.ChannelBinding, 0)
	for _, binding := range s.bindings {
		if binding.AccountID == accountID {
			copied := *binding
			list = append(list, &copied)
		}
	}
	return list, nil
}

func (s *memStore) SaveSession(ctx context.Context, session *entity.Session) error {
	copied := *session
	s.sessions[session.ID] = &copied
	return nil
}

func (s *memStore) FindSession(ctx context.Context, id string) (*entity.Session, error) {
	session, ok := s.sessions[id]
	if !ok {
		return nil, entity.ErrAccountNotFound
	}
	copied := *session
	return &copied, nil
}

func (s *memStore) RevokeSessions(ctx context.Context, accountID string, now time.Time) error {
	for _, session := range s.sessions {
		if session.AccountID == accountID {
			session.ExpiresAt = now
		}
	}
	return nil
}

func (s *memStore) SaveCode(ctx context.Context, code *entity.VerificationCode) error {
	copied := *code
	s.codes[code.Phone] = &copied
	return nil
}

func (s *memStore) LoadCode(ctx context.Context, phone string) (*entity.VerificationCode, error) {
	code, ok := s.codes[phone]
	if !ok {
		return nil, entity.ErrAccountNotFound
	}
	copied := *code
	return &copied, nil
}

func (s *memStore) CountCodes(ctx context.Context, phone string, since time.Time) (int, error) {
	return 0, nil
}

func (s *memStore) SaveEstimate(ctx context.Context, estimate *entity.Estimate) error {
	copied := *estimate
	s.estimates[estimate.No] = &copied
	return nil
}

func (s *memStore) FindEstimate(ctx context.Context, no string) (*entity.Estimate, error) {
	estimate, ok := s.estimates[no]
	if !ok {
		return nil, entity.ErrEstimateNotFound
	}
	copied := *estimate
	return &copied, nil
}

func (s *memStore) ConsumeEstimate(ctx context.Context, estimate *entity.Estimate) error {
	current, ok := s.estimates[estimate.No]
	if !ok {
		return entity.ErrEstimateNotFound
	}
	if current.Used {
		return entity.ErrEstimateUsed
	}
	current.Used = true
	return nil
}

func (s *memStore) SaveAddress(ctx context.Context, address *entity.Address) error {
	copied := *address
	s.addresses[address.ID] = &copied
	return nil
}

func (s *memStore) FindAddress(ctx context.Context, id string) (*entity.Address, error) {
	address, ok := s.addresses[id]
	if !ok {
		return nil, entity.ErrAddressNotFound
	}
	copied := *address
	return &copied, nil
}

func (s *memStore) DeleteAddress(ctx context.Context, id string) error {
	delete(s.addresses, id)
	return nil
}

func (s *memStore) ListAddresses(ctx context.Context, accountID string, addrType entity.AddressType, limit int) ([]*entity.Address, error) {
	list := make([]*entity.Address, 0)
	for _, address := range s.addresses {
		if address.AccountID == accountID && address.Type == addrType {
			copied := *address
			list = append(list, &copied)
		}
	}
	return list, nil
}

func (s *memStore) CountAddresses(ctx context.Context, accountID string, addrType entity.AddressType) (int, error) {
	list, err := s.ListAddresses(ctx, accountID, addrType, 0)
	return len(list), err
}

func (s *memStore) ClearHistory(ctx context.Context, accountID string) error {
	for id, address := range s.addresses {
		if address.AccountID == accountID && address.Type == entity.AddressTypeHistory {
			delete(s.addresses, id)
		}
	}
	return nil
}

func (s *memStore) SaveOrder(ctx context.Context, order *entity.Order) error {
	copied := *order
	s.orders[order.ID] = &copied
	return nil
}

func (s *memStore) FindOrder(ctx context.Context, id string) (*entity.Order, error) {
	order, ok := s.orders[id]
	if !ok {
		return nil, entity.ErrOrderNotFound
	}
	copied := *order
	return &copied, nil
}

func (s *memStore) FindOrderByEstimate(ctx context.Context, estimateNo string) (*entity.Order, error) {
	for _, order := range s.orders {
		if order.EstimateNo == estimateNo {
			copied := *order
			return &copied, nil
		}
	}
	return nil, entity.ErrOrderNotFound
}

func (s *memStore) FindOrderByIdempotency(ctx context.Context, accountID, key string) (*entity.Order, error) {
	for _, order := range s.orders {
		if order.AccountID == accountID && order.IdempotencyKey == key {
			copied := *order
			return &copied, nil
		}
	}
	return nil, entity.ErrOrderNotFound
}

func (s *memStore) FindActiveOrder(ctx context.Context, accountID string) (*entity.Order, error) {
	for _, order := range s.orders {
		if order.AccountID == accountID && !order.State.Terminal() {
			copied := *order
			return &copied, nil
		}
	}
	return nil, entity.ErrOrderNotFound
}

func (s *memStore) ListOrders(ctx context.Context, accountID string, state entity.OrderState, offset, limit int) ([]*entity.Order, error) {
	list := make([]*entity.Order, 0)
	for _, order := range s.orders {
		if order.AccountID != accountID {
			continue
		}
		if state != "" && order.State != state {
			continue
		}
		copied := *order
		list = append(list, &copied)
	}
	return list, nil
}

func (s *memStore) CountOrders(ctx context.Context, accountID string, state entity.OrderState) (int, error) {
	list, err := s.ListOrders(ctx, accountID, state, 0, 0)
	return len(list), err
}

func (s *memStore) UpdateOrderState(ctx context.Context, order *entity.Order) error {
	s.orders[order.ID] = order
	return nil
}

func (s *memStore) AppendLog(ctx context.Context, log *entity.OrderStatusLog) error {
	s.logs = append(s.logs, log)
	return nil
}

func (s *memStore) ListQuick(ctx context.Context, now time.Time) ([]*entity.QuickEntry, error) {
	return s.quick, nil
}

func (s *memStore) ListUsed(ctx context.Context, accountID string) ([]string, error) {
	return s.used[accountID], nil
}

func (s *memStore) MarkUsed(ctx context.Context, accountID, entryID string, now time.Time) error {
	s.used[accountID] = append(s.used[accountID], entryID)
	return nil
}

// memCoupons 是集成测试用的优惠券仓储。
type memCoupons struct {
	items map[string]*entity.Coupon
}

func (c *memCoupons) FindByID(ctx context.Context, id string) (*entity.Coupon, error) {
	item, ok := c.items[id]
	if !ok {
		return nil, entity.ErrCouponNotFound
	}
	copied := *item
	return &copied, nil
}

func (c *memCoupons) ListUsable(ctx context.Context, accountID string, now time.Time) ([]*entity.Coupon, error) {
	list := make([]*entity.Coupon, 0)
	for _, item := range c.items {
		if item.AccountID == accountID {
			copied := *item
			list = append(list, &copied)
		}
	}
	return list, nil
}

func (c *memCoupons) Lock(ctx context.Context, id string, orderID string, now time.Time) error {
	return nil
}

func (c *memCoupons) Unlock(ctx context.Context, id string, now time.Time) error { return nil }

// memDrivers 是集成测试用的司机仓储。
type memDrivers struct{}

func (d *memDrivers) FindByAccountID(ctx context.Context, accountID string) (*entity.DriverProfile, error) {
	return nil, entity.ErrDriverNotRegistered
}

func (d *memDrivers) FindByOrderID(ctx context.Context, orderID string) (*entity.DriverProfile, error) {
	return nil, entity.ErrDriverNotAssigned
}

// memPOIs 是集成测试用的地点仓储。
type memPOIs struct{}

func (p *memPOIs) Search(ctx context.Context, keyword, city string, lng, lat float64, limit int) ([]*entity.POI, error) {
	return []*entity.POI{
		{ID: "poi_1", Name: "张江镇科苑路", Addr: "科苑路 88 号", City: "上海", Lng: 121.601, Lat: 31.2054, DistanceM: 120},
	}, nil
}

func (p *memPOIs) Recommend(ctx context.Context, lng, lat float64, limit int) ([]*entity.POI, error) {
	return []*entity.POI{
		{ID: "poi_1", Name: "张江镇科苑路", Addr: "科苑路 88 号", City: "上海", Lng: 121.601, Lat: 31.2054, DistanceM: 120},
	}, nil
}

// memSender 是集成测试用的短信通道。
type memSender struct {
	codes map[string]string
}

func (s *memSender) Send(ctx context.Context, phone string, code string) (string, error) {
	if s.codes == nil {
		s.codes = map[string]string{}
	}
	s.codes[phone] = code
	return "msg-1", nil
}

func (s *memSender) LastCode(phone string) (string, bool) {
	code, ok := s.codes[phone]
	return code, ok
}

// memAuth 是集成测试用的第三方渠道。
type memAuth struct{}

func (a *memAuth) Code2Session(ctx context.Context, channel entity.Channel, code string) (string, error) {
	return string(channel) + "_uid_" + code, nil
}

// memIDs 是集成测试用的标识生成器。
type memIDs struct {
	seq int
}

func (g *memIDs) NewID() string {
	g.seq++
	return "id-" + string(rune('a'+g.seq))
}

func (g *memIDs) NewOrderNo(now time.Time) string {
	g.seq++
	return "HXZ" + now.Format("20060102150405")
}

func (g *memIDs) NewEstimateNo(now time.Time) string {
	g.seq++
	return "EST" + now.Format("20060102150405")
}

// memHasher 是集成测试用的密码哈希器。
type memHasher struct{}

func (h memHasher) Hash(password string) (string, error) { return "hashed:" + password, nil }

func (h memHasher) Verify(hash string, password string) bool {
	return hash == "hashed:"+password
}

// newTestServer 在内存 gRPC 连接上起一个乘客端服务。
func newTestServer(t *testing.T) (passengerv1.PassengerServiceClient, *memStore) {
	t.Helper()
	store := newMemStore()
	sender := &memSender{}
	svc := service.NewService(
		&memAccounts{store},
		&memChannels{store},
		&memSessions{store},
		&memCodes{store},
		&memEstimates{store},
		&memAddresses{store},
		&memPOIs{},
		&memOrders{store},
		&memQuick{store},
		&memIssuer{},
		memHasher{},
		sender,
		&memAuth{},
		&memIDs{},
	)
	svc.WithCouponRepository(&memCoupons{items: map[string]*entity.Coupon{}})
	svc.WithDriverRepository(&memDrivers{})
	svc.WithCancelFee(500, 1000)

	listener := bufconn.Listen(bufconnSize)
	server := grpc.NewServer()
	passengerv1.RegisterPassengerServiceServer(server, NewServer(svc, zap.NewNop()))
	go func() {
		_ = server.Serve(listener)
	}()
	t.Cleanup(server.Stop)

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return listener.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial bufconn error = %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return passengerv1.NewPassengerServiceClient(conn), store
}

// memIssuer 是集成测试用的令牌签发器。
type memIssuer struct{}

func (m *memIssuer) Issue(accountID string, audience entity.Audience, role entity.Role, now time.Time) (string, string, time.Duration, error) {
	return "access-" + accountID, "refresh-" + accountID, 30 * time.Minute, nil
}

func (m *memIssuer) ParseAccess(token string, expect entity.Audience, now time.Time) (string, entity.Role, error) {
	return "account-1", entity.RolePassenger, nil
}

func (m *memIssuer) ParseRefresh(token string, expect entity.Audience, now time.Time) (string, entity.Role, error) {
	return "account-1", entity.RolePassenger, nil
}

// 下列适配器把 memStore 的方法透传给业务端口。
type memAccounts struct{ store *memStore }

func (m *memAccounts) FindByID(ctx context.Context, id string) (*entity.Account, error) {
	return m.store.FindByID(ctx, id)
}
func (m *memAccounts) FindByPhone(ctx context.Context, phone string) (*entity.Account, error) {
	return m.store.FindByPhone(ctx, phone)
}
func (m *memAccounts) Save(ctx context.Context, account *entity.Account) error {
	return m.store.Save(ctx, account)
}
func (m *memAccounts) UpdatePhone(ctx context.Context, account *entity.Account) error {
	return m.store.UpdatePhone(ctx, account)
}

type memChannels struct{ store *memStore }

func (m *memChannels) FindByChannelUID(ctx context.Context, channel entity.Channel, uid string) (*entity.ChannelBinding, error) {
	return m.store.FindByChannelUID(ctx, channel, uid)
}
func (m *memChannels) Save(ctx context.Context, binding *entity.ChannelBinding) error {
	return m.store.SaveBinding(ctx, binding)
}
func (m *memChannels) Delete(ctx context.Context, id string) error {
	return m.store.DeleteBinding(ctx, id)
}
func (m *memChannels) ListByAccount(ctx context.Context, accountID string) ([]*entity.ChannelBinding, error) {
	return m.store.ListBindings(ctx, accountID)
}

type memSessions struct{ store *memStore }

func (m *memSessions) Save(ctx context.Context, session *entity.Session) error {
	return m.store.SaveSession(ctx, session)
}
func (m *memSessions) FindByID(ctx context.Context, id string) (*entity.Session, error) {
	return m.store.FindSession(ctx, id)
}
func (m *memSessions) RevokeByAccount(ctx context.Context, accountID string, now time.Time) error {
	return m.store.RevokeSessions(ctx, accountID, now)
}

type memCodes struct{ store *memStore }

func (m *memCodes) Save(ctx context.Context, code *entity.VerificationCode) error {
	return m.store.SaveCode(ctx, code)
}
func (m *memCodes) Load(ctx context.Context, phone string) (*entity.VerificationCode, error) {
	return m.store.LoadCode(ctx, phone)
}
func (m *memCodes) CountRecentHourly(ctx context.Context, phone string, since time.Time) (int, error) {
	return m.store.CountCodes(ctx, phone, since)
}

type memEstimates struct{ store *memStore }

func (m *memEstimates) Save(ctx context.Context, estimate *entity.Estimate) error {
	return m.store.SaveEstimate(ctx, estimate)
}
func (m *memEstimates) FindByNo(ctx context.Context, no string) (*entity.Estimate, error) {
	return m.store.FindEstimate(ctx, no)
}
func (m *memEstimates) Consume(ctx context.Context, estimate *entity.Estimate) error {
	return m.store.ConsumeEstimate(ctx, estimate)
}

type memAddresses struct{ store *memStore }

func (m *memAddresses) Save(ctx context.Context, address *entity.Address) error {
	return m.store.SaveAddress(ctx, address)
}
func (m *memAddresses) FindByID(ctx context.Context, id string) (*entity.Address, error) {
	return m.store.FindAddress(ctx, id)
}
func (m *memAddresses) Delete(ctx context.Context, id string) error {
	return m.store.DeleteAddress(ctx, id)
}
func (m *memAddresses) ListByType(ctx context.Context, accountID string, t entity.AddressType, limit int) ([]*entity.Address, error) {
	return m.store.ListAddresses(ctx, accountID, t, limit)
}
func (m *memAddresses) CountByType(ctx context.Context, accountID string, t entity.AddressType) (int, error) {
	return m.store.CountAddresses(ctx, accountID, t)
}
func (m *memAddresses) ClearHistory(ctx context.Context, accountID string) error {
	return m.store.ClearHistory(ctx, accountID)
}

type memOrders struct{ store *memStore }

func (m *memOrders) Save(ctx context.Context, order *entity.Order) error {
	return m.store.SaveOrder(ctx, order)
}
func (m *memOrders) FindByID(ctx context.Context, id string) (*entity.Order, error) {
	return m.store.FindOrder(ctx, id)
}
func (m *memOrders) FindByEstimateNo(ctx context.Context, no string) (*entity.Order, error) {
	return m.store.FindOrderByEstimate(ctx, no)
}
func (m *memOrders) FindByIdempotencyKey(ctx context.Context, accountID, key string) (*entity.Order, error) {
	return m.store.FindOrderByIdempotency(ctx, accountID, key)
}
func (m *memOrders) FindActive(ctx context.Context, accountID string) (*entity.Order, error) {
	return m.store.FindActiveOrder(ctx, accountID)
}
func (m *memOrders) ListByAccount(ctx context.Context, accountID string, state entity.OrderState, offset, limit int) ([]*entity.Order, error) {
	return m.store.ListOrders(ctx, accountID, state, offset, limit)
}
func (m *memOrders) CountByAccount(ctx context.Context, accountID string, state entity.OrderState) (int, error) {
	return m.store.CountOrders(ctx, accountID, state)
}
func (m *memOrders) UpdateState(ctx context.Context, order *entity.Order) error {
	return m.store.UpdateOrderState(ctx, order)
}
func (m *memOrders) AppendStatusLog(ctx context.Context, log *entity.OrderStatusLog) error {
	return m.store.AppendLog(ctx, log)
}

type memQuick struct{ store *memStore }

func (m *memQuick) ListEnabled(ctx context.Context, now time.Time) ([]*entity.QuickEntry, error) {
	return m.store.ListQuick(ctx, now)
}
func (m *memQuick) ListUsedEntryIDs(ctx context.Context, accountID string) ([]string, error) {
	return m.store.ListUsed(ctx, accountID)
}
func (m *memQuick) MarkUsed(ctx context.Context, accountID, entryID string, now time.Time) error {
	return m.store.MarkUsed(ctx, accountID, entryID, now)
}

// ensure the in-memory adapters satisfy the entity ports.
var (
	_ entity.AccountRepository    = (*memAccounts)(nil)
	_ entity.ChannelRepository    = (*memChannels)(nil)
	_ entity.SessionRepository    = (*memSessions)(nil)
	_ entity.CodeRepository       = (*memCodes)(nil)
	_ entity.EstimateRepository   = (*memEstimates)(nil)
	_ entity.AddressRepository    = (*memAddresses)(nil)
	_ entity.OrderRepository      = (*memOrders)(nil)
	_ entity.QuickEntryRepository = (*memQuick)(nil)
	_ entity.CouponRepository     = (*memCoupons)(nil)
	_ entity.DriverRepository     = (*memDrivers)(nil)
	_ entity.POIRepository        = (*memPOIs)(nil)
	_ service.CodeSender          = (*memSender)(nil)
	_ service.ChannelAuthenticator = (*memAuth)(nil)
	_ service.IDGenerator         = (*memIDs)(nil)
	_ service.PasswordHasher      = memHasher{}
)

// keep the dto import referenced.
var _ = dto.LoginCommand{}
