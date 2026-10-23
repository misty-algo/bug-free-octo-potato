package repo

import (
	"context"
	"crypto/rand"

	"errors"
	"math/big"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/BwCloudWeGo/bw-cli/internal/passenger/entity"
	"github.com/BwCloudWeGo/bw-cli/internal/passenger/service"
)

// BcryptHasher 用 bcrypt 保存登录密码摘要。
//
// 明文密码不落库也不进日志。bcrypt 自带盐值，摘要不可逆推。
type BcryptHasher struct {
	cost int
}

// NewBcryptHasher 创建 bcrypt 哈希器。
func NewBcryptHasher() *BcryptHasher {
	return &BcryptHasher{cost: bcrypt.DefaultCost}
}

// Hash 返回 bcrypt 摘要。
func (h *BcryptHasher) Hash(password string) (string, error) {
	if password == "" {
		return "", errors.New("password is empty")
	}
	cost := h.cost
	if cost <= 0 {
		cost = bcrypt.DefaultCost
	}
	digest, err := bcrypt.GenerateFromPassword([]byte(password), cost)
	if err != nil {
		return "", err
	}
	return string(digest), nil
}

// Verify 校验密码。
func (h *BcryptHasher) Verify(hash string, password string) bool {
	if hash == "" || password == "" {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// IDGenerator 生成各类业务标识。
type IDGenerator struct {
	seq int64
}

// NewIDGenerator 创建标识生成器。
func NewIDGenerator() *IDGenerator {
	return &IDGenerator{}
}

// NewID 生成通用标识。
func (g *IDGenerator) NewID() string {
	value, err := rand.Int(rand.Reader, big.NewInt(1<<62))
	if err != nil {
		g.seq++
		return "id-" + strconv.FormatInt(time.Now().UnixNano(), 10) + "-" + strconv.FormatInt(g.seq, 10)
	}
	return "id-" + strconv.FormatInt(value.Int64(), 16)
}

// NewOrderNo 生成订单号。
func (g *IDGenerator) NewOrderNo(now time.Time) string {
	value, err := rand.Int(rand.Reader, big.NewInt(1<<30))
	if err != nil {
		value = big.NewInt(0)
	}
	return "HXZ" + now.Format("20060102150405") + strconv.FormatInt(value.Int64(), 36)
}

// NewEstimateNo 生成估价单号。
func (g *IDGenerator) NewEstimateNo(now time.Time) string {
	value, err := rand.Int(rand.Reader, big.NewInt(1<<30))
	if err != nil {
		value = big.NewInt(0)
	}
	return "EST" + now.Format("20060102150405") + strconv.FormatInt(value.Int64(), 36)
}

// LocalPOIRepository 是地点检索的本地实现。
//
// ADAPTER: mock —— 未配置地图服务密钥时使用，接口契约与真实实现一致。
type LocalPOIRepository struct {
	pois []*entity.POI
}

// NewLocalPOIRepository 创建本地地点检索实现。
func NewLocalPOIRepository() *LocalPOIRepository {
	return &LocalPOIRepository{pois: []*entity.POI{
		{ID: "poi_0001", Name: "张江镇科苑路", Addr: "上海市浦东新区科苑路 88 号", City: "上海", Lng: 121.601, Lat: 31.2054},
		{ID: "poi_0002", Name: "虹桥火车站", Addr: "上海市闵行区申贵路 1500 号", City: "上海", Lng: 121.3208, Lat: 31.1979},
		{ID: "poi_0003", Name: "陆家嘴地铁站", Addr: "上海市浦东新区浦东南路 1 号", City: "上海", Lng: 121.5050, Lat: 31.2397},
	}}
}

// Search 按关键词匹配名称与地址。
func (r *LocalPOIRepository) Search(ctx context.Context, keyword, city string, lng, lat float64, limit int) ([]*entity.POI, error) {
	matched := make([]*entity.POI, 0, len(r.pois))
	for _, poi := range r.pois {
		if keyword == "" || contains(poi.Name, keyword) || contains(poi.Addr, keyword) {
			matched = append(matched, poi)
		}
	}
	if limit > 0 && len(matched) > limit {
		matched = matched[:limit]
	}
	return matched, nil
}

// Recommend 返回附近上车点。
func (r *LocalPOIRepository) Recommend(ctx context.Context, lng, lat float64, limit int) ([]*entity.POI, error) {
	if limit > 0 && len(r.pois) > limit {
		return r.pois[:limit], nil
	}
	return r.pois, nil
}

// LocalCodeSender 是短信通道的本地实现。
//
// ADAPTER: mock —— 保留验证码而不真正发送，供本地联调。
type LocalCodeSender struct {
	codes map[string]string
}

// NewLocalCodeSender 创建本地短信通道。
func NewLocalCodeSender() *LocalCodeSender {
	return &LocalCodeSender{codes: map[string]string{}}
}

// Send 保留验证码。
func (s *LocalCodeSender) Send(ctx context.Context, phone string, code string) (string, error) {
	s.codes[phone] = code
	return "local-" + phone, nil
}

// LastCode 返回发给该号码的验证码。
func (s *LocalCodeSender) LastCode(phone string) (string, bool) {
	code, ok := s.codes[phone]
	return code, ok
}

// LocalChannelAuthenticator 是第三方渠道的本地实现。
//
// ADAPTER: mock —— 返回确定性渠道标识，便于无凭据环境联调。
type LocalChannelAuthenticator struct{}

// NewLocalChannelAuthenticator 创建本地渠道实现。
func NewLocalChannelAuthenticator() *LocalChannelAuthenticator {
	return &LocalChannelAuthenticator{}
}

// Code2Session 解析授权码为渠道标识。
func (a *LocalChannelAuthenticator) Code2Session(ctx context.Context, channel entity.Channel, code string) (string, error) {
	if code == "" {
		return "", errors.New("authorization code is empty")
	}
	return string(channel) + "_uid_" + code, nil
}

// ensure the local adapters satisfy the service ports.
var (
	_ entity.POIRepository         = (*LocalPOIRepository)(nil)
	_ service.CodeSender           = (*LocalCodeSender)(nil)
	_ service.CodeInspector        = (*LocalCodeSender)(nil)
	_ service.ChannelAuthenticator = (*LocalChannelAuthenticator)(nil)
	_ service.IDGenerator          = (*IDGenerator)(nil)
	_ service.PasswordHasher       = (*BcryptHasher)(nil)
)

// contains 报告文本是否包含目标子串，忽略大小写。
func contains(text, target string) bool {
	return strings.Contains(strings.ToLower(text), strings.ToLower(target))
}
