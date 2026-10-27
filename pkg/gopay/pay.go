// Package gopay 封装 github.com/go-pay/gopay 支付宝支付能力，
// 对外仅暴露 Init / Pay / Notify 三个核心函数，直调底层 API，不做多层抽象。
package gopay

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"

	"github.com/BwCloudWeGo/bw-cli/pkg/alipayx"
	"github.com/go-pay/gopay"
	"github.com/go-pay/gopay/alipay"
)

// Config 支付宝应用配置，字段与 alipayx 保持一致以便复用。
type Config struct {
	AppID           string // 支付宝应用 ID
	PrivateKey      string // 应用私钥（PKCS1/PKCS8 均可，go-pay 内部自动识别）
	AlipayPublicKey string // 支付宝公钥，用于异步通知验签
	Production      bool   // true=线上网关，false=沙箱网关
	NotifyURL       string // 异步通知地址
	ReturnURL       string // 同步跳转地址
	EncryptKey      string // 敏感字段 AES 解密密钥（可选）
}

// Client 持有 go-pay 底层客户端与配置。
type Client struct {
	cfg Config
	raw *alipay.Client
}

// PayRequest 描述一笔支付订单。
type PayRequest struct {
	OutTradeNo  string // 商户订单号（必填）
	Subject     string // 订单标题（必填）
	TotalAmount string // 订单金额，单位元，如 "9.90"（必填）
	Body        string // 订单描述（可选）
	NotifyURL   string // 覆盖默认异步通知地址（可选）
	ReturnURL   string // 覆盖默认同步跳转地址（可选）
}

// PayResponse 支付结果，PayURL 为可直接跳转的支付链接。
type PayResponse struct {
	PayURL string // WAP/网页支付跳转链接
}

// RefundRequest 退款请求参数。
type RefundRequest struct {
	OutTradeNo   string // 商户订单号（与 TradeNo 二选一）
	TradeNo      string // 支付宝交易号（与 OutTradeNo 二选一）
	RefundAmount string // 退款金额，如 "0.01"（必填）
	RefundReason string // 退款原因（可选，≤256 字符）
	OutRequestNo string // 退款请求号，用于幂等（必填）
}

// RefundResult 退款结果。
type RefundResult struct {
	TradeNo      string // 支付宝交易号
	OutTradeNo   string // 商户订单号
	FundChange   string // Y=退款成功，N=退款失败
	RefundFee    string // 退款总金额
	GmtRefundPay string // 退款支付时间
}

// Init 创建并校验支付宝客户端。
func Init(cfg Config) (*Client, error) {
	if cfg.AppID == "" {
		return nil, errors.New("alipay app id is required")
	}
	if cfg.PrivateKey == "" {
		return nil, errors.New("alipay private key is required")
	}
	if cfg.AlipayPublicKey == "" {
		return nil, errors.New("alipay public key is required for notify verify")
	}

	// NewClient 返回 (*Client, error)，需接收两个值。
	raw, err := alipay.NewClient(cfg.AppID, cfg.PrivateKey, cfg.Production)
	if err != nil {
		return nil, err
	}
	// 公钥不通过 Client 方法设置，验签时直接传入 VerifySign。
	if cfg.EncryptKey != "" {
		raw.SetAESKey(cfg.EncryptKey) // go-pay 中 SetEncryptKey 已更名为 SetAESKey
	}
	return &Client{cfg: cfg, raw: raw}, nil
}

// InitFromAlipay 桥接函数：将本地 alipayx.Config 映射为 gopay.Config 后调用 Init，
// 便于在已使用 alipayx 的项目中无缝切换到 go-pay 实现。
func InitFromAlipay(cfg *alipayx.Config) (*Client, error) {
	if cfg == nil {
		return nil, errors.New("alipayx config is nil")
	}
	// 仅映射 7 个公共字段，证书相关字段（cert 模式）在 go-pay 中暂未使用。
	gc := Config{
		AppID:           cfg.AppID,
		PrivateKey:      cfg.PrivateKey,
		AlipayPublicKey: cfg.AlipayPublicKey,
		Production:      cfg.Production,
		NotifyURL:       cfg.NotifyURL,
		ReturnURL:       cfg.ReturnURL,
		EncryptKey:      cfg.EncryptKey,
	}
	return Init(gc)
}

// Pay 发起支付，scene 取值 "wap"（手机网站支付）或 "page"（PC 网页支付）。
// 返回可直接 302 跳转的支付链接字符串。
func (c *Client) Pay(req PayRequest, scene string) (*PayResponse, error) {
	if c == nil || c.raw == nil {
		return nil, errors.New("gopay client is nil")
	}
	if req.OutTradeNo == "" || req.Subject == "" || req.TotalAmount == "" {
		return nil, errors.New("out_trade_no, subject and total_amount are required")
	}

	// 组装统一下单参数。
	bm := make(gopay.BodyMap)
	bm.Set("out_trade_no", req.OutTradeNo)
	bm.Set("total_amount", req.TotalAmount)
	bm.Set("subject", req.Subject)
	if req.Body != "" {
		bm.Set("body", req.Body)
	}
	bm.Set("notify_url", firstNonEmpty(req.NotifyURL, c.cfg.NotifyURL))
	bm.Set("return_url", firstNonEmpty(req.ReturnURL, c.cfg.ReturnURL))

	var payURL string
	var err error
	switch scene {
	case "wap":
		// TradeWapPay 需要 (ctx, bm)，返回 (string, error)。
		payURL, err = c.raw.TradeWapPay(context.Background(), bm)
	case "page":
		// TradePagePay 需要 (ctx, bm)，返回 (string, error)。
		payURL, err = c.raw.TradePagePay(context.Background(), bm)
	default:
		return nil, errors.New("scene must be 'wap' or 'page'")
	}
	if err != nil {
		return nil, err
	}
	return &PayResponse{PayURL: payURL}, nil
}

// Notify 校验支付宝异步通知签名，返回解析后的参数 BodyMap 供业务层读取。
// 入参为 HTTP 请求（来自回调 handler），验签使用包级函数
// alipay.VerifySign(publicKey, notifyBean)，而非 Client 方法。
func (c *Client) Notify(req *http.Request) (gopay.BodyMap, error) {
	if c == nil {
		return nil, errors.New("gopay client is nil")
	}
	// 解析异步通知参数到 BodyMap。
	bean, err := alipay.ParseNotifyToBodyMap(req)
	if err != nil {
		return nil, err
	}
	// 验签：传入支付宝公钥与通知体，失败返回 error。
	ok, err := alipay.VerifySign(c.cfg.AlipayPublicKey, bean)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errors.New("alipay notify sign verify failed")
	}
	return bean, nil
}

// firstNonEmpty 返回第一个非空字符串，用于合并请求级与默认级配置。
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func (r *RefundRequest) validate() error {
	if r.OutTradeNo == "" && r.TradeNo == "" {
		return errors.New("out_trade_no or trade_no is required")
	}
	if r.RefundAmount == "" {
		return errors.New("refund_amount is required")
	}
	if matched, _ := regexp.MatchString(`^\d+\.\d{2}$`, r.RefundAmount); !matched {
		return errors.New("refund_amount 格式错误，应为 '0.01' 两位小数")
	}
	if r.OutRequestNo == "" {
		return errors.New("out_request_no is required")
	}
	if len([]rune(r.RefundReason)) > 256 {
		return errors.New("退款原因不能超过256")
	}
	return nil
}

// Refund 发起退款，返回支付宝原始退款响应。
func (c *Client) Refund(ctx context.Context, req RefundRequest) (*alipay.TradeRefundResponse, error) {
	if c == nil || c.raw == nil {
		return nil, errors.New("gopay client is nil")
	}
	if err := req.validate(); err != nil {
		return nil, fmt.Errorf("validate refund request: %w", err)
	}

	bm := make(gopay.BodyMap)
	if req.OutTradeNo != "" {
		bm.Set("out_trade_no", req.OutTradeNo)
	}
	if req.TradeNo != "" {
		bm.Set("trade_no", req.TradeNo)
	}
	bm.Set("refund_amount", req.RefundAmount)
	bm.Set("out_request_no", req.OutRequestNo)
	if req.RefundReason != "" {
		bm.Set("refund_reason", req.RefundReason)
	}

	rsp, err := c.raw.TradeRefund(ctx, bm)
	if err != nil {
		return nil, fmt.Errorf("alipay trade refund: %w", err)
	}
	if rsp.Response == nil {
		return nil, errors.New("alipay trade refund response is nil")
	}
	if rsp.Response.Code != "10000" {
		return nil, fmt.Errorf("alipay trade refund biz error: code=%s msg=%s sub_code=%s",
			rsp.Response.Code, rsp.Response.Msg, rsp.Response.SubCode)
	}
	return rsp, nil
}

// QueryRequest 支付宝订单查询请求参数。
type QueryRequest struct {
	OutTradeNo string // 商户订单号（与 TradeNo 二选一）
	TradeNo    string // 支付宝交易号（与 OutTradeNo 二选一）
}

// QueryResult 支付宝订单查询结果。
type QueryResult struct {
	TradeNo      string // 支付宝交易号
	OutTradeNo   string // 商户订单号
	TradeStatus  string // WAIT_BUYER_PAY / TRADE_CLOSED / TRADE_SUCCESS / TRADE_FINISHED
	BuyerLogonId string // 买家支付宝账号
	SendPayDate  string // 支付时间
}

func (r *QueryRequest) validate() error {
	if r.OutTradeNo == "" && r.TradeNo == "" {
		return errors.New("out_trade_no or trade_no is required")
	}
	return nil
}

// Query 查询支付宝订单状态，用于主动查单与掉单补偿。
func (c *Client) Query(ctx context.Context, req QueryRequest) (*QueryResult, error) {
	if c == nil || c.raw == nil {
		return nil, errors.New("gopay client is nil")
	}
	if err := req.validate(); err != nil {
		return nil, fmt.Errorf("validate query request: %w", err)
	}

	bm := make(gopay.BodyMap)
	if req.OutTradeNo != "" {
		bm.Set("out_trade_no", req.OutTradeNo)
	}
	if req.TradeNo != "" {
		bm.Set("trade_no", req.TradeNo)
	}

	rsp, err := c.raw.TradeQuery(ctx, bm)
	if err != nil {
		return nil, fmt.Errorf("alipay trade query: %w", err)
	}
	if rsp.Response == nil {
		return nil, errors.New("alipay trade query response is nil")
	}
	if rsp.Response.Code != "10000" {
		return nil, fmt.Errorf("alipay trade query biz error: code=%s msg=%s sub_code=%s",
			rsp.Response.Code, rsp.Response.Msg, rsp.Response.SubCode)
	}
	return &QueryResult{
		TradeNo:      rsp.Response.TradeNo,
		OutTradeNo:   rsp.Response.OutTradeNo,
		TradeStatus:  rsp.Response.TradeStatus,
		BuyerLogonId: rsp.Response.BuyerLogonId,
		SendPayDate:  rsp.Response.SendPayDate,
	}, nil
}

// CancelRequest 订单撤销请求参数。仅未支付或已关闭的订单可撤销。
type CancelRequest struct {
	OutTradeNo string // 商户订单号（与 TradeNo 二选一）
	TradeNo    string // 支付宝交易号（与 OutTradeNo 二选一）
}

// CancelResult 撤销结果。
type CancelResult struct {
	TradeNo    string // 支付宝交易号
	OutTradeNo string // 商户订单号
	RetryFlag  string // Y=可重试，N=不可重试
	Action     string // close=关闭的订单撤销，refund=支付的订单退款
}

func (r *CancelRequest) validate() error {
	if r.OutTradeNo == "" && r.TradeNo == "" {
		return errors.New("out_trade_no or trade_no is required")
	}
	return nil
}

// Cancel 撤销订单。支付宝返回 retry_flag 指示是否需要换号重试。
func (c *Client) Cancel(ctx context.Context, req CancelRequest) (*CancelResult, error) {
	if c == nil || c.raw == nil {
		return nil, errors.New("gopay client is nil")
	}
	if err := req.validate(); err != nil {
		return nil, fmt.Errorf("validate cancel request: %w", err)
	}

	bm := make(gopay.BodyMap)
	if req.OutTradeNo != "" {
		bm.Set("out_trade_no", req.OutTradeNo)
	}
	if req.TradeNo != "" {
		bm.Set("trade_no", req.TradeNo)
	}

	rsp, err := c.raw.TradeCancel(ctx, bm)
	if err != nil {
		return nil, fmt.Errorf("alipay trade cancel: %w", err)
	}
	if rsp.Response == nil {
		return nil, errors.New("alipay trade cancel response is nil")
	}
	if rsp.Response.Code != "10000" {
		return nil, fmt.Errorf("alipay trade cancel biz error: code=%s msg=%s sub_code=%s",
			rsp.Response.Code, rsp.Response.Msg, rsp.Response.SubCode)
	}
	return &CancelResult{
		TradeNo:    rsp.Response.TradeNo,
		OutTradeNo: rsp.Response.OutTradeNo,
		RetryFlag:  rsp.Response.RetryFlag,
		Action:     rsp.Response.Action,
	}, nil
}

// PrecreateRequest 扫码支付（当面付）请求参数。
type PrecreateRequest struct {
	OutTradeNo  string // 商户订单号（必填）
	Subject     string // 订单标题（必填）
	TotalAmount string // 订单金额，单位元，如 "19.90"（必填）
	Body        string // 订单描述（可选）
	NotifyURL   string // 覆盖默认异步通知地址（可选）
}

// PrecreateResult 扫码支付结果，QrCode 给前端生成二维码。
type PrecreateResult struct {
	OutTradeNo string // 商户订单号
	QrCode     string // 二维码链接，前端用此生成二维码图片
}

// TradePayRequest 付款码支付（商家扫用户付款码）请求参数。
type TradePayRequest struct {
	OutTradeNo  string // 商户订单号（必填）
	Subject     string // 订单标题（必填）
	TotalAmount string // 订单金额，单位元，如 "19.90"（必填）
	AuthCode    string // 用户付款码，28位数字串（必填）
	Body        string // 订单描述（可选）
	NotifyURL   string // 覆盖默认异步通知地址（可选）
}

// TradePayResult 付款码支付结果。TradeNo 在同步扣款时即返回，与 Precreate 不同。
// 注：alipay_trade_pay_response 不含 GmtPayment，支付时间可后续通过 TradeQuery 获取。
type TradePayResult struct {
	TradeNo      string // 支付宝交易号
	OutTradeNo   string // 商户订单号
	TotalAmount  string // 交易金额
	BuyerLogonId string // 买家支付宝账号
}

func (r *TradePayRequest) validate() error {
	if r.OutTradeNo == "" {
		return errors.New("out_trade_no is required")
	}
	if r.Subject == "" {
		return errors.New("subject is required")
	}
	if r.TotalAmount == "" {
		return errors.New("total_amount is required")
	}
	if r.AuthCode == "" {
		return errors.New("auth_code is required")
	}
	matched, _ := regexp.MatchString(`^\d+\.\d{2}$`, r.TotalAmount)
	if !matched {
		return errors.New("total_amount 格式错误，应为 '0.01' 两位小数")
	}
	return nil
}

// TradePay 发起付款码支付（商家扫用户付款码），同步扣款，调用即扣钱。
func (c *Client) TradePay(ctx context.Context, req TradePayRequest) (*TradePayResult, error) {
	if c == nil || c.raw == nil {
		return nil, errors.New("gopay client is nil")
	}
	if err := req.validate(); err != nil {
		return nil, fmt.Errorf("validate trade_pay request: %w", err)
	}

	bm := make(gopay.BodyMap)
	bm.Set("out_trade_no", req.OutTradeNo)
	bm.Set("subject", req.Subject)
	bm.Set("total_amount", req.TotalAmount)
	bm.Set("scene", "bar_code")
	bm.Set("auth_code", req.AuthCode)
	bm.Set("notify_url", firstNonEmpty(req.NotifyURL, c.cfg.NotifyURL))
	if req.Body != "" {
		bm.Set("body", req.Body)
	}

	rsp, err := c.raw.TradePay(ctx, bm)
	if err != nil {
		return nil, fmt.Errorf("alipay trade pay: %w", err)
	}
	if rsp.Response == nil {
		return nil, errors.New("alipay trade pay response is nil")
	}
	if rsp.Response.Code != "10000" {
		return nil, fmt.Errorf("alipay trade pay biz error: code=%s msg=%s sub_code=%s",
			rsp.Response.Code, rsp.Response.Msg, rsp.Response.SubCode)
	}
	return &TradePayResult{
		TradeNo:      rsp.Response.TradeNo,
		OutTradeNo:   rsp.Response.OutTradeNo,
		TotalAmount:  rsp.Response.TotalAmount,
		BuyerLogonId: rsp.Response.BuyerLogonId,
	}, nil
}

func (r *PrecreateRequest) validate() error {
	if r.OutTradeNo == "" {
		return errors.New("out_trade_no is required")
	}
	if r.Subject == "" {
		return errors.New("subject is required")
	}
	if r.TotalAmount == "" {
		return errors.New("total_amount is required")
	}
	matched, _ := regexp.MatchString(`^\d+\.\d{2}$`, r.TotalAmount)
	if !matched {
		return errors.New("total_amount 格式错误，应为 '0.01' 两位小数")
	}
	return nil
}

// Precreate 发起扫码支付（当面付），返回二维码链接。
func (c *Client) Precreate(req PrecreateRequest) (*PrecreateResult, error) {
	if c == nil || c.raw == nil {
		return nil, errors.New("gopay client is nil")
	}
	if err := req.validate(); err != nil {
		return nil, fmt.Errorf("validate precreate request: %w", err)
	}

	bm := make(gopay.BodyMap)
	bm.Set("out_trade_no", req.OutTradeNo)
	bm.Set("total_amount", req.TotalAmount)
	bm.Set("subject", req.Subject)
	if req.Body != "" {
		bm.Set("body", req.Body)
	}
	bm.Set("notify_url", firstNonEmpty(req.NotifyURL, c.cfg.NotifyURL))

	rsp, err := c.raw.TradePrecreate(context.Background(), bm)
	if err != nil {
		return nil, fmt.Errorf("alipay trade precreate: %w", err)
	}
	if rsp.Response == nil {
		return nil, errors.New("alipay precreate response is nil")
	}
	if rsp.Response.Code != "10000" {
		return nil, fmt.Errorf("alipay precreate biz error: code=%s msg=%s sub_code=%s",
			rsp.Response.Code, rsp.Response.Msg, rsp.Response.SubCode)
	}
	return &PrecreateResult{
		OutTradeNo: rsp.Response.OutTradeNo,
		QrCode:     rsp.Response.QrCode,
	}, nil
}
