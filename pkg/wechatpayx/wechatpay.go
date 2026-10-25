// Package wechatpayx 使用标准库 net/http + crypto 封装微信支付 v3 核心能力，
// 不引入任何第三方依赖。签名使用商户私钥 RSA-SHA256 生成 Authorization 头，
// 回调使用微信平台证书公钥验签，敏感字段使用 APIv3Key AES-256-GCM 解密。
package wechatpayx

import (
	"bytes"
	"context"
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	// BaseURL 微信支付 v3 API 网关。
	BaseURL = "https://api.mch.weixin.qq.com"
	// SandboxBaseURL 微信支付沙箱网关（Production=false 时使用）。
	SandboxBaseURL = "https://api.mch.weixin.qq.com/sandboxnew"
	// SignType 平台签名类型，固定 SHA256-RSA2048。
	SignType = "WECHATPAY2-SHA256-RSA2048"
)

// Config 微信支付商户配置。
type Config struct {
	MchID             string `mapstructure:"mch_id" yaml:"mch_id"`                           // 商户号
	AppID             string `mapstructure:"app_id" yaml:"app_id"`                           // 应用 AppID
	APIv3Key          string `mapstructure:"api_v3_key" yaml:"api_v3_key"`                   // APIv3 密钥（32 字节，用于回调解密）
	SerialNo          string `mapstructure:"serial_no" yaml:"serial_no"`                     // 商户 API 证书序列号
	PrivateKey        string `mapstructure:"private_key" yaml:"private_key"`                 // 商户 API 私钥（PKCS1/PKCS8 PEM）
	PlatformPublicKey string `mapstructure:"platform_public_key" yaml:"platform_public_key"` // 微信平台证书公钥 PEM（回调验签用，可后续从平台获取）
	NotifyURL         string `mapstructure:"notify_url" yaml:"notify_url"`                   // 异步通知地址
	Production        bool   `mapstructure:"production" yaml:"production"`                   // true=线上，false=沙箱
}

// Client 持有微信支付 v3 凭证与 HTTP 客户端。
type Client struct {
	cfg        Config
	privateKey *rsa.PrivateKey
	httpClient *http.Client
}

// DefaultConfig 返回微信支付的默认配置（全部为空占位）。
func DefaultConfig() Config {
	return Config{}
}

// Init 校验配置并构造微信支付客户端。
func Init(cfg Config) (*Client, error) {
	if cfg.MchID == "" {
		return nil, errors.New("wechat mch_id is required")
	}
	if cfg.AppID == "" {
		return nil, errors.New("wechat app_id is required")
	}
	if cfg.APIv3Key == "" {
		return nil, errors.New("wechat api_v3_key is required")
	}
	if cfg.SerialNo == "" {
		return nil, errors.New("wechat serial_no is required")
	}
	if cfg.PrivateKey == "" {
		return nil, errors.New("wechat private key is required")
	}

	key, err := parsePrivateKey(cfg.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("parse wechat private key: %w", err)
	}
	return &Client{
		cfg:        cfg,
		privateKey: key,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}, nil
}

// base 返回当前环境使用的 API 根地址。
func (c *Client) base() string {
	if c.cfg.Production {
		return BaseURL
	}
	return SandboxBaseURL
}

// NativeScanPayRequest 扫码支付（Native）下单请求。
type NativeScanPayRequest struct {
	OutTradeNo  string // 商户订单号（必填）
	Description string // 商品描述（必填）
	Amount      int64  // 订单金额，单位分（必填）
	NotifyURL   string // 覆盖默认异步通知地址（可选）
}

// NativeScanPayResult 扫码支付下单结果，CodeURL 供前端生成二维码。
type NativeScanPayResult struct {
	CodeURL string
}

// NativeScanPay 调用 /v3/pay/transactions/native 下单，返回 code_url。
func (c *Client) NativeScanPay(ctx context.Context, req NativeScanPayRequest) (*NativeScanPayResult, error) {
	if c == nil || c.privateKey == nil {
		return nil, errors.New("wechatpayx client is nil")
	}
	if req.OutTradeNo == "" || req.Description == "" || req.Amount <= 0 {
		return nil, errors.New("out_trade_no, description and amount are required")
	}

	body := map[string]interface{}{
		"appid":        c.cfg.AppID,
		"mchid":        c.cfg.MchID,
		"description":  req.Description,
		"out_trade_no": req.OutTradeNo,
		"notify_url":   firstNonEmpty(req.NotifyURL, c.cfg.NotifyURL),
		"amount": map[string]interface{}{
			"total":    req.Amount,
			"currency": "CNY",
		},
	}
	raw, err := c.doJSON(ctx, http.MethodPost, "/v3/pay/transactions/native", body)
	if err != nil {
		return nil, err
	}
	var rsp struct {
		CodeURL string `json:"code_url"`
	}
	if err := json.Unmarshal(raw, &rsp); err != nil {
		return nil, fmt.Errorf("parse native scan pay response: %w", err)
	}
	if rsp.CodeURL == "" {
		return nil, errors.New("wechat native scan pay returned empty code_url")
	}
	return &NativeScanPayResult{CodeURL: rsp.CodeURL}, nil
}

// JsapiPayRequest JSAPI 支付下单请求。
type JsapiPayRequest struct {
	OutTradeNo  string // 商户订单号（必填）
	Description string // 商品描述（必填）
	Amount      int64  // 订单金额，单位分（必填）
	OpenID      string // 用户 openid（必填）
	NotifyURL   string // 覆盖默认异步通知地址（可选）
}

// JsapiPayResult JSAPI 支付下单结果，含调起支付所需参数。
type JsapiPayResult struct {
	PrepayID  string
	AppID     string
	TimeStamp string
	NonceStr  string
	Package   string
	PaySign   string
}

// JsapiPay 调用 /v3/pay/transactions/jsapi 下单，并生成调起支付参数。
func (c *Client) JsapiPay(ctx context.Context, req JsapiPayRequest) (*JsapiPayResult, error) {
	if c == nil || c.privateKey == nil {
		return nil, errors.New("wechatpayx client is nil")
	}
	if req.OutTradeNo == "" || req.Description == "" || req.Amount <= 0 || req.OpenID == "" {
		return nil, errors.New("out_trade_no, description, amount and openid are required")
	}

	body := map[string]interface{}{
		"appid":        c.cfg.AppID,
		"mchid":        c.cfg.MchID,
		"description":  req.Description,
		"out_trade_no": req.OutTradeNo,
		"notify_url":   firstNonEmpty(req.NotifyURL, c.cfg.NotifyURL),
		"amount": map[string]interface{}{
			"total":    req.Amount,
			"currency": "CNY",
		},
		"payer": map[string]string{
			"openid": req.OpenID,
		},
	}
	raw, err := c.doJSON(ctx, http.MethodPost, "/v3/pay/transactions/jsapi", body)
	if err != nil {
		return nil, err
	}
	var rsp struct {
		PrepayID string `json:"prepay_id"`
	}
	if err := json.Unmarshal(raw, &rsp); err != nil {
		return nil, fmt.Errorf("parse jsapi pay response: %w", err)
	}
	if rsp.PrepayID == "" {
		return nil, errors.New("wechat jsapi pay returned empty prepay_id")
	}

	ts := fmt.Sprintf("%d", time.Now().Unix())
	nonce := randomString(32)
	packageValue := "prepay_id=" + rsp.PrepayID
	signMsg := c.cfg.AppID + "\n" + ts + "\n" + nonce + "\n" + packageValue + "\n"
	paySign, err := signSHA256WithRSA(c.privateKey, signMsg)
	if err != nil {
		return nil, fmt.Errorf("sign jsapi pay params: %w", err)
	}
	return &JsapiPayResult{
		PrepayID:  rsp.PrepayID,
		AppID:     c.cfg.AppID,
		TimeStamp: ts,
		NonceStr:  nonce,
		Package:   packageValue,
		PaySign:   paySign,
	}, nil
}

// QueryOrder 按商户订单号查询微信支付订单状态。
// TradeState 取值：SUCCESS / REFUND / NOTPAY / CLOSED / REVOKED / USERPAYING / PAYERROR。
func (c *Client) QueryOrder(ctx context.Context, outTradeNo string) (*OrderQueryResult, error) {
	if c == nil || c.privateKey == nil {
		return nil, errors.New("wechatpayx client is nil")
	}
	if outTradeNo == "" {
		return nil, errors.New("out_trade_no is required")
	}

	path := "/v3/pay/transactions/out-trade-no/" + urlPathEscape(outTradeNo) + "?mchid=" + urlQueryEscape(c.cfg.MchID)
	raw, err := c.doRaw(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	var rsp struct {
		AppID          string `json:"appid"`
		MchID          string `json:"mchid"`
		OutTradeNo     string `json:"out_trade_no"`
		TransactionID  string `json:"transaction_id"`
		TradeType      string `json:"trade_type"`
		TradeState     string `json:"trade_state"`
		TradeStateDesc string `json:"trade_state_desc"`
		SuccessTime    string `json:"success_time"`
		Payer          struct {
			OpenID string `json:"openid"`
		} `json:"payer"`
		Amount struct {
			Total         int64  `json:"total"`
			PayerTotal    int64  `json:"payer_total"`
			Currency      string `json:"currency"`
			PayerCurrency string `json:"payer_currency"`
		} `json:"amount"`
	}
	if err := json.Unmarshal(raw, &rsp); err != nil {
		return nil, fmt.Errorf("parse query order response: %w", err)
	}
	return &OrderQueryResult{
		OutTradeNo:     rsp.OutTradeNo,
		TransactionID:  rsp.TransactionID,
		TradeState:     rsp.TradeState,
		TradeStateDesc: rsp.TradeStateDesc,
		SuccessTime:    rsp.SuccessTime,
		TotalAmount:    rsp.Amount.Total,
	}, nil
}

// OrderQueryResult 微信支付订单查询结果。
type OrderQueryResult struct {
	OutTradeNo     string
	TransactionID  string
	TradeState     string
	TradeStateDesc string
	SuccessTime    string
	TotalAmount    int64
}

// RefundRequest 微信支付退款请求。
type RefundRequest struct {
	OutTradeNo    string // 商户订单号（与 TransactionID 二选一）
	TransactionID string // 微信支付订单号（与 OutTradeNo 二选一）
	OutRefundNo   string // 商户退款单号（必填，用于幂等）
	RefundAmount  int64  // 退款金额，单位分（必填）
	TotalAmount   int64  // 原订单金额，单位分（必填）
	Reason        string // 退款原因（可选）
	NotifyURL     string // 退款结果回调地址（可选）
}

// RefundResult 微信支付退款结果。
type RefundResult struct {
	RefundID      string
	OutTradeNo    string
	TransactionID string
	Status        string // SUCCESS / CLOSED / PROCESSING / ABNORMAL
	RefundAmount  int64
}

// Refund 调用 /v3/refund/domestic/refunds 发起退款。
func (c *Client) Refund(ctx context.Context, req RefundRequest) (*RefundResult, error) {
	if c == nil || c.privateKey == nil {
		return nil, errors.New("wechatpayx client is nil")
	}
	if req.OutTradeNo == "" && req.TransactionID == "" {
		return nil, errors.New("out_trade_no or transaction_id is required")
	}
	if req.OutRefundNo == "" {
		return nil, errors.New("out_refund_no is required")
	}
	if req.RefundAmount <= 0 || req.TotalAmount <= 0 {
		return nil, errors.New("refund_amount and total_amount must be positive")
	}

	body := map[string]interface{}{
		"out_refund_no": req.OutRefundNo,
		"amount": map[string]interface{}{
			"refund":   req.RefundAmount,
			"total":    req.TotalAmount,
			"currency": "CNY",
		},
	}
	if req.OutTradeNo != "" {
		body["out_trade_no"] = req.OutTradeNo
	}
	if req.TransactionID != "" {
		body["transaction_id"] = req.TransactionID
	}
	if req.Reason != "" {
		body["reason"] = req.Reason
	}
	if req.NotifyURL != "" {
		body["notify_url"] = req.NotifyURL
	}

	raw, err := c.doJSON(ctx, http.MethodPost, "/v3/refund/domestic/refunds", body)
	if err != nil {
		return nil, err
	}
	var rsp struct {
		RefundID      string `json:"refund_id"`
		OutTradeNo    string `json:"out_trade_no"`
		TransactionID string `json:"transaction_id"`
		Status        string `json:"status"`
		Amount        struct {
			Refund      int64 `json:"refund"`
			PayerRefund int64 `json:"payer_refund"`
			Total       int64 `json:"total"`
		} `json:"amount"`
	}
	if err := json.Unmarshal(raw, &rsp); err != nil {
		return nil, fmt.Errorf("parse refund response: %w", err)
	}
	return &RefundResult{
		RefundID:      rsp.RefundID,
		OutTradeNo:    rsp.OutTradeNo,
		TransactionID: rsp.TransactionID,
		Status:        rsp.Status,
		RefundAmount:  rsp.Amount.Refund,
	}, nil
}

// Notify 校验微信支付回调签名并解密订单结果。
// 入参为 HTTP 请求（来自回调 handler），验签需要 Config.PlatformPublicKey；
// 未配置平台证书公钥时返回明确错误。
func (c *Client) Notify(req *http.Request) (*NotifyResult, error) {
	if c == nil || c.privateKey == nil {
		return nil, errors.New("wechatpayx client is nil")
	}
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, fmt.Errorf("read wechat notify body: %w", err)
	}

	// 验签串：时间戳\n随机串\n请求体\n
	signMsg := req.Header.Get("Wechatpay-Timestamp") + "\n" +
		req.Header.Get("Wechatpay-Nonce") + "\n" +
		string(body) + "\n"

	publicKey, err := parsePublicKey(c.cfg.PlatformPublicKey)
	if err != nil {
		return nil, fmt.Errorf("parse wechat platform public key: %w", err)
	}
	signature, err := base64.StdEncoding.DecodeString(req.Header.Get("Wechatpay-Signature"))
	if err != nil {
		return nil, fmt.Errorf("decode wechat notify signature: %w", err)
	}
	if err := verifySHA256WithRSA(publicKey, signMsg, signature); err != nil {
		return nil, fmt.Errorf("wechat notify verify failed: %w", err)
	}

	// 解密 resource 字段（AES-256-GCM）。
	var envelope struct {
		Resource struct {
			Algorithm      string `json:"algorithm"`
			Ciphertext     string `json:"ciphertext"`
			AssociatedData string `json:"associated_data"`
			Nonce          string `json:"nonce"`
			OriginalType   string `json:"original_type"`
		} `json:"resource"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("parse wechat notify envelope: %w", err)
	}
	plain, err := decryptAESGCM(c.cfg.APIv3Key, envelope.Resource.Ciphertext, envelope.Resource.Nonce, envelope.Resource.AssociatedData)
	if err != nil {
		return nil, fmt.Errorf("decrypt wechat notify resource: %w", err)
	}
	var result NotifyResult
	if err := json.Unmarshal(plain, &result); err != nil {
		return nil, fmt.Errorf("parse wechat notify resource: %w", err)
	}
	result.Raw = string(plain)
	return &result, nil
}

// NotifyResult 微信支付回调解密后的订单结果。
type NotifyResult struct {
	AppID          string `json:"appid"`
	MchID          string `json:"mchid"`
	OutTradeNo     string `json:"out_trade_no"`
	TransactionID  string `json:"transaction_id"`
	TradeType      string `json:"trade_type"`
	TradeState     string `json:"trade_state"`
	TradeStateDesc string `json:"trade_state_desc"`
	SuccessTime    string `json:"success_time"`
	Payer          struct {
		OpenID string `json:"openid"`
	} `json:"payer"`
	Amount struct {
		Total      int64  `json:"total"`
		PayerTotal int64  `json:"payer_total"`
		Currency   string `json:"currency"`
	} `json:"amount"`
	Raw string `json:"-"`
}

// doJSON 发送带 JSON 请求体的 v3 请求并返回响应体。
func (c *Client) doJSON(ctx context.Context, method, path string, payload interface{}) ([]byte, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal request body: %w", err)
	}
	return c.doRaw(ctx, method, path, data)
}

// doRaw 发送 v3 请求并返回响应体（成功时）。
func (c *Client) doRaw(ctx context.Context, method, path string, body []byte) ([]byte, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base()+path, reader)
	if err != nil {
		return nil, fmt.Errorf("build wechat request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	// 签名串使用请求的 path（含 query），body 为空时为空字符串。
	signPath := path
	auth, err := c.buildAuthorization(method, signPath, string(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", auth)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "wechatpayx/1.0")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("wechat request failed: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read wechat response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("wechat api error: status=%d body=%s", resp.StatusCode, truncate(string(raw), 512))
	}
	return raw, nil
}

// buildAuthorization 使用商户私钥对请求签名并生成 Authorization 头。
func (c *Client) buildAuthorization(method, path string, body string) (string, error) {
	ts := fmt.Sprintf("%d", time.Now().Unix())
	nonce := randomString(32)
	msg := method + "\n" + path + "\n" + ts + "\n" + nonce + "\n" + body + "\n"
	signature, err := signSHA256WithRSA(c.privateKey, msg)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(
		`%s mchid="%s",nonce_str="%s",signature="%s",timestamp="%s",serial_no="%s"`,
		SignType, c.cfg.MchID, nonce, signature, ts, c.cfg.SerialNo,
	), nil
}

// signSHA256WithRSA 使用 RSA 私钥对消息做 SHA256-RSA 签名，返回 base64。
func signSHA256WithRSA(key *rsa.PrivateKey, msg string) (string, error) {
	digest := sha256.Sum256([]byte(msg))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(sig), nil
}

// verifySHA256WithRSA 使用 RSA 公钥验证 SHA256-RSA 签名。
func verifySHA256WithRSA(key *rsa.PublicKey, msg string, sig []byte) error {
	digest := sha256.Sum256([]byte(msg))
	return rsa.VerifyPKCS1v15(key, crypto.SHA256, digest[:], sig)
}

// decryptAESGCM 使用 APIv3Key 对微信回调 resource 做 AES-256-GCM 解密。
func decryptAESGCM(apiV3Key, ciphertextB64, nonceB64, associatedData string) ([]byte, error) {
	key := []byte(apiV3Key)
	if len(key) != 32 {
		return nil, errors.New("api_v3_key must be 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce, err := base64.StdEncoding.DecodeString(nonceB64)
	if err != nil {
		return nil, err
	}
	ciphertext, err := base64.StdEncoding.DecodeString(ciphertextB64)
	if err != nil {
		return nil, err
	}
	var adata []byte
	if associatedData != "" {
		adata = []byte(associatedData)
	}
	plain, err := gcm.Open(nil, nonce, ciphertext, adata)
	if err != nil {
		return nil, err
	}
	return plain, nil
}

// parsePrivateKey 解析 PKCS1 或 PKCS8 格式的 PEM 私钥。
func parsePrivateKey(pemText string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(strings.TrimSpace(pemText)))
	if block == nil {
		return nil, errors.New("invalid private key pem")
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	if key, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		rsaKey, ok := key.(*rsa.PrivateKey)
		if !ok {
			return nil, errors.New("private key is not rsa")
		}
		return rsaKey, nil
	}
	return nil, errors.New("unsupported private key format")
}

// parsePublicKey 解析 PEM 格式的 RSA 公钥（PKIX 或 PKCS1）。
func parsePublicKey(pemText string) (*rsa.PublicKey, error) {
	block, _ := pem.Decode([]byte(strings.TrimSpace(pemText)))
	if block == nil {
		return nil, errors.New("invalid public key pem")
	}
	if key, err := x509.ParsePKIXPublicKey(block.Bytes); err == nil {
		rsaKey, ok := key.(*rsa.PublicKey)
		if !ok {
			return nil, errors.New("public key is not rsa")
		}
		return rsaKey, nil
	}
	if key, err := x509.ParsePKCS1PublicKey(block.Bytes); err == nil {
		return key, nil
	}
	return nil, errors.New("unsupported public key format")
}

// randomString 生成指定长度的随机十六进制字符串。
func randomString(n int) string {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		// 理论上 crypto/rand 不会失败，退化为时间戳+固定串。
		return fmt.Sprintf("%d%s", time.Now().UnixNano(), strings.Repeat("0", n))
	}
	return hex.EncodeToString(buf)[:n]
}

func urlPathEscape(s string) string {
	return strings.ReplaceAll(s, "/", "%2F")
}

func urlQueryEscape(s string) string {
	return strings.ReplaceAll(s, "&", "%26")
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
