package filex

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"
)

// 授权读取的用途。对象 key 不可预测不等于已授权，每次读取都要重新判定用途。
const (
	// PurposeDriverSelf 只允许本人读取自己的材料。
	PurposeDriverSelf = "driver_self"
	// PurposeAudit 审核职责范围内调阅。
	PurposeAudit = "audit"
	// PurposeSafety 安全处理职责范围内调阅。
	PurposeSafety = "safety"
)

// DefaultTicketTTL 是票据默认有效期，属于技术建议值。
const DefaultTicketTTL = 60 * time.Second

// LocalConfig 本地私有目录适配器配置。
// 这是课程可控实现：对象落在服务端私有目录，读取只能走下面的授权代理，
// 不经过 CDN、不签发公开 URL。
type LocalConfig struct {
	RootDir string `mapstructure:"root_dir" yaml:"root_dir"`
	SignKey string `mapstructure:"sign_key" yaml:"sign_key"`
}

// GrantRequest 描述一次授权下载申请。
type GrantRequest struct {
	Key       string
	Purpose   string
	SubjectID string
	TTL       time.Duration
	MaxBytes  int64
}

// DownloadGrant 是签发结果。Ticket 是一次用途的短期凭证，
// 这里刻意不返回对象存储直签 URL。
type DownloadGrant struct {
	Provider  string    `json:"provider"`
	Key       string    `json:"key"`
	Ticket    string    `json:"ticket"`
	ExpiresAt time.Time `json:"expires_at"`
}

// ObjectStream 是代理读取到的对象内容。
type ObjectStream struct {
	Body        io.ReadCloser
	ContentType string
	Size        int64
}

// PrivateReader 是私有对象的授权读取能力。
// 现有 Uploader 只产出公开 URL，不能满足证件材料的授权读取，因此单独增补。
type PrivateReader interface {
	// GrantDownload 校验 stroage key 后签发短期一次用途票据。
	GrantDownload(ctx context.Context, req GrantRequest) (DownloadGrant, error)
	// Inspect 只校验签名与有效期，不消耗一次用途，用于代理读取前再次鉴权。
	Inspect(ctx context.Context, ticket string) (TicketClaims, error)
	// Open 校验票据有效且未被使用后返回对象内容。
	Open(ctx context.Context, ticket string) (ObjectStream, error)
	// Delete 供受控清理任务回收孤儿对象。
	Delete(ctx context.Context, key string) error
}

// TicketClaims 是票据内声明的内容。代理读取前必须再次据此判定用途与主体，
// 避免撤权后仍能用旧直签 URL 访问。
type TicketClaims struct {
	Key       string
	Purpose   string
	Subject   string
	ExpiresAt time.Time
	MaxBytes  int64
}

type ticketPayload struct {
	Key      string `json:"k"`
	Purpose  string `json:"p"`
	Subject  string `json:"s"`
	Exp      int64  `json:"e"`
	MaxBytes int64  `json:"m"`
	Nonce    string `json:"n"`
}

type privateBackend interface {
	Open(ctx context.Context, key string) (ObjectStream, error)
	Delete(ctx context.Context, key string) error
}

type privateReader struct {
	cfg      Config
	provider string
	backend  privateBackend
	used     map[string]time.Time
	mu       sync.Mutex
}

// NewPrivateReader 在当前 provider 支持私有读取时返回授权读取器。
// 现有四个云 provider 尚未实现 Open/Delete，会明确报错而不是退化为公开 URL。
func NewPrivateReader(cfg Config) (PrivateReader, error) {
	cfg = normalizeConfig(cfg)
	backend, err := newBackend(cfg)
	if err != nil {
		return nil, err
	}
	private, ok := backend.(privateBackend)
	if !ok {
		return nil, fmt.Errorf("file storage provider %q does not support private authorized reads yet", cfg.Provider)
	}
	return &privateReader{cfg: cfg, provider: cfg.Provider, backend: private, used: make(map[string]time.Time)}, nil
}

func signingKey(cfg Config) []byte {
	if cfg.Local.SignKey != "" {
		return []byte(cfg.Local.SignKey)
	}
	return []byte("filex-local-dev-key")
}

func (p *privateReader) GrantDownload(_ context.Context, req GrantRequest) (DownloadGrant, error) {
	if req.Key == "" {
		return DownloadGrant{}, errors.New("object key is required")
	}
	if req.Purpose == "" {
		return DownloadGrant{}, errors.New("download purpose is required")
	}
	ttl := req.TTL
	if ttl <= 0 {
		ttl = DefaultTicketTTL
	}
	now := time.Now().UTC()
	payload := ticketPayload{
		Key: req.Key, Purpose: req.Purpose, Subject: req.SubjectID,
		Exp: now.Add(ttl).Unix(), MaxBytes: req.MaxBytes, Nonce: newNonce(),
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return DownloadGrant{}, err
	}
	encoded := base64.RawURLEncoding.EncodeToString(raw)
	ticket := encoded + "." + signTicket(signingKey(p.cfg), encoded)
	p.mu.Lock()
	p.gc(now)
	p.mu.Unlock()
	return DownloadGrant{
		Provider:  p.provider,
		Key:       req.Key,
		Ticket:    ticket,
		ExpiresAt: time.Unix(payload.Exp, 0).UTC(),
	}, nil
}

// verifyTicket 校验签名与有效期，返回票据内声明。
func (p *privateReader) verifyTicket(ticket string) (*ticketPayload, error) {
	encoded, sig, ok := splitTicket(ticket)
	if !ok {
		return nil, errors.New("malformed download ticket")
	}
	if !hmac.Equal([]byte(signTicket(signingKey(p.cfg), encoded)), []byte(sig)) {
		return nil, errors.New("download ticket signature mismatch")
	}
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, errors.New("malformed download ticket")
	}
	var payload ticketPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, errors.New("malformed download ticket")
	}
	if time.Now().UTC().After(time.Unix(payload.Exp, 0).UTC()) {
		return nil, errors.New("download ticket expired")
	}
	return &payload, nil
}

// Inspect 只做校验，不消耗一次用途。
func (p *privateReader) Inspect(_ context.Context, ticket string) (TicketClaims, error) {
	payload, err := p.verifyTicket(ticket)
	if err != nil {
		return TicketClaims{}, err
	}
	p.mu.Lock()
	_, used := p.used[payload.Nonce]
	p.mu.Unlock()
	if used {
		return TicketClaims{}, errors.New("download ticket already used")
	}
	return TicketClaims{
		Key: payload.Key, Purpose: payload.Purpose, Subject: payload.Subject,
		ExpiresAt: time.Unix(payload.Exp, 0).UTC(), MaxBytes: payload.MaxBytes,
	}, nil
}

func (p *privateReader) Open(_ context.Context, ticket string) (ObjectStream, error) {
	payload, err := p.verifyTicket(ticket)
	if err != nil {
		return ObjectStream{}, err
	}
	// 一次用途：已用过的 nonce 直接拒绝，不因仍在有效期内而放行。
	p.mu.Lock()
	if _, seen := p.used[payload.Nonce]; seen {
		p.mu.Unlock()
		return ObjectStream{}, errors.New("download ticket already used")
	}
	p.used[payload.Nonce] = time.Unix(payload.Exp, 0).UTC()
	p.gc(time.Now().UTC())
	p.mu.Unlock()

	private := p.backend
	stream, err := private.Open(context.Background(), payload.Key)
	if err != nil {
		return ObjectStream{}, err
	}
	if payload.MaxBytes > 0 && stream.Size > payload.MaxBytes {
		stream.Body.Close()
		return ObjectStream{}, fmt.Errorf("object exceeds authorized size %d bytes", payload.MaxBytes)
	}
	return stream, nil
}

func (p *privateReader) Delete(ctx context.Context, key string) error {
	return p.backend.Delete(ctx, key)
}

// gc 清理过期 nonce，避免内存无界增长。
func (p *privateReader) gc(now time.Time) {
	for nonce, exp := range p.used {
		if now.After(exp) {
			delete(p.used, nonce)
		}
	}
}

func splitTicket(ticket string) (string, string, bool) {
	for i := len(ticket) - 1; i > 0; i-- {
		if ticket[i] == '.' {
			return ticket[:i], ticket[i+1:], true
		}
	}
	return "", "", false
}

func signTicket(key []byte, payload string) string {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil))
}

// newNonce 生成不可预测的一次用途标识。
func newNonce() string {
	var seed [32]byte
	if _, err := rand.Read(seed[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	sum := sha256.Sum256(seed[:])
	return hex.EncodeToString(sum[:16])
}

// SniffImageKind 只依据 magic bytes 判定图片真实类型，
// 不信扩展名、Content-Type 或前端声明。
func SniffImageKind(sample []byte) string {
	if len(sample) >= 3 && sample[0] == 0xFF && sample[1] == 0xD8 && sample[2] == 0xFF {
		return "image/jpeg"
	}
	if len(sample) >= 8 && sample[0] == 0x89 && sample[1] == 0x50 && sample[2] == 0x4E && sample[3] == 0x47 &&
		sample[4] == 0x0D && sample[5] == 0x0A && sample[6] == 0x1A && sample[7] == 0x0A {
		return "image/png"
	}
	return ""
}
