package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	settlementv1 "github.com/BwCloudWeGo/bw-cli/api/gen/settlement/v1"
	"github.com/BwCloudWeGo/bw-cli/internal/shop/dto"
	"github.com/BwCloudWeGo/bw-cli/internal/shop/entity"
	"github.com/BwCloudWeGo/bw-cli/pkg/gopay"
	"github.com/BwCloudWeGo/bw-cli/pkg/wechatpayx"
)

// PayResult 网页/WAP 支付用例结果。
type PayResult struct {
	OrderNo string
	PayURL  string
}

// ScanPayResult 扫码支付用例结果。
type ScanPayResult struct {
	OrderNo     string
	QrCode      string
	TotalAmount int64
}

// BarCodePayResult 付款码支付用例结果。
type BarCodePayResult struct {
	TradeNo      string
	OrderNo      string
	TotalAmount  int64
	BuyerLogonID string
}

// RefundResult 退款用例结果。
type RefundResult struct {
	Success      bool
	TradeNo      string
	OrderNo      string
	RefundAmount int64
	Duplicate    bool // 幂等命中：同一订单已退款，未重复调用渠道
}

// SetAlipayClient 注入支付宝支付客户端。为 nil 时支付用例返回 alipay not configured。
func (s *Service) SetAlipayClient(client *gopay.Client) {
	s.alipay = client
}

// requireAlipay 检查支付宝客户端是否可用，避免 nil client panic。
func (s *Service) requireAlipay() error {
	if s.alipay == nil {
		return entity.ErrAlipayNotConfigured
	}
	return nil
}

// requireWechat 检查微信支付客户端是否可用，避免 nil client panic。
func (s *Service) requireWechat() error {
	if s.wechat == nil {
		return entity.ErrWechatNotConfigured
	}
	return nil
}

// requirePayments 检查支付单仓储是否注入。
func (s *Service) requirePayments() error {
	if s.payments == nil {
		return fmt.Errorf("%w: payment repository not injected", entity.ErrInvalidPaymentRequest)
	}
	return nil
}

// createOrGetPayment 幂等落库：order_no 已存在则返回已有单与 duplicate=true，
// 不重复调用渠道；并发冲突（唯一索引）同样返回已有单。
func (s *Service) createOrGetPayment(ctx context.Context, orderNo string, amount int64, channel entity.PaymentChannel) (*entity.Payment, bool, error) {
	if err := s.requirePayments(); err != nil {
		return nil, false, err
	}
	if existing, err := s.payments.FindByOrderNo(ctx, orderNo); err == nil {
		// P0-1 重复下单防金额分裂：同一 order_no 必须金额、渠道一致，
		// 否则拒绝并在渠道侧不会生成新支付，避免本地与渠道金额不一致。
		if existing.Amount != amount {
			return nil, false, fmt.Errorf("%w: order_no %s local amount %d != requested %d", entity.ErrPaymentAmountMismatch, orderNo, existing.Amount, amount)
		}
		if channel != "" && existing.Channel != channel {
			return nil, false, fmt.Errorf("%w: order_no %s local channel %s != requested %s", entity.ErrPaymentExists, orderNo, existing.Channel, channel)
		}
		return existing, true, nil
	} else if err != entity.ErrPaymentNotFound {
		return nil, false, err
	}

	now := time.Now()
	item := &entity.Payment{
		OrderNo:       orderNo,
		Amount:        amount,
		Status:        entity.PaymentStatusPending,
		Channel:       channel,
		IdempotentKey: orderNo, // P1：幂等键与 payment 服务口径一致，避免空串撞 idx_payments_idempotent_key 唯一索引
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := s.payments.Create(ctx, item); err != nil {
		if err == entity.ErrPaymentExists {
			existing, findErr := s.payments.FindByOrderNo(ctx, orderNo)
			if findErr != nil {
				return nil, false, findErr
			}
			return existing, true, nil
		}
		return nil, false, err
	}
	return item, false, nil
}

// markFailed 将支付单标记为 failed。
func (s *Service) markFailed(ctx context.Context, orderNo string, cause string) {
	if s.payments == nil {
		return
	}
	item, err := s.payments.FindByOrderNo(ctx, orderNo)
	if err != nil || item.Status != entity.PaymentStatusPending {
		return
	}
	item.Status = entity.PaymentStatusFailed
	item.UpdatedAt = time.Now()
	if err := s.payments.Update(ctx, item); err != nil {
		s.log.Warn("mark payment failed error", zap.String("order_no", orderNo), zap.Error(err))
	}
}

// Pay 发起 WAP/PC 网页支付，scene 取 wap / page。
// 先落库 pending（order_no 幂等），重复下单直接返回已有单，不重复调用支付宝。
func (s *Service) Pay(ctx context.Context, cmd dto.PayCommand) (*PayResult, error) {
	if err := s.requireAlipay(); err != nil {
		return nil, err
	}
	if cmd.Subject == "" || cmd.TotalAmount <= 0 {
		return nil, fmt.Errorf("%w: subject and positive total_amount are required", entity.ErrInvalidPaymentRequest)
	}
	if cmd.Scene != "wap" && cmd.Scene != "page" {
		return nil, fmt.Errorf("%w: scene must be 'wap' or 'page'", entity.ErrInvalidPaymentRequest)
	}
	orderNo := firstNonEmptyOrderNo(cmd.OrderNo, generateOrderNo())

	item, duplicate, err := s.createOrGetPayment(ctx, orderNo, cmd.TotalAmount, entity.PaymentChannelAlipay)
	if err != nil {
		return nil, err
	}
	if duplicate {
		s.log.Info("duplicate pay request, return existing pending payment", zap.String("order_no", orderNo))
		if item.Channel != entity.PaymentChannelAlipay || item.Status != entity.PaymentStatusPending {
			return nil, fmt.Errorf("%w: order_no exists with status %s channel %s", entity.ErrPaymentExists, item.Status, item.Channel)
		}
	}

	resp, err := s.alipay.Pay(gopay.PayRequest{
		OutTradeNo:  orderNo,
		Subject:     cmd.Subject,
		TotalAmount: formatCents(cmd.TotalAmount),
		Body:        cmd.Body,
	}, cmd.Scene)
	if err != nil {
		s.markFailed(ctx, orderNo, "alipay pay")
		return nil, fmt.Errorf("alipay pay: %w", err)
	}
	s.log.Info("alipay pay requested", zap.String("order_no", orderNo), zap.String("scene", cmd.Scene), zap.Int64("total_amount_cents", cmd.TotalAmount))
	return &PayResult{OrderNo: orderNo, PayURL: resp.PayURL}, nil
}

// ScanPay 发起扫码支付，channel 区分 alipay / wechat（空值默认 alipay）。
// alipay 返回当面付二维码链接，wechat 走 NativeScanPay 返回 code_url。
func (s *Service) ScanPay(ctx context.Context, cmd dto.ScanPayCommand) (*ScanPayResult, error) {
	if cmd.Subject == "" || cmd.TotalAmount <= 0 {
		return nil, fmt.Errorf("%w: subject and positive total_amount are required", entity.ErrInvalidPaymentRequest)
	}
	channel := entity.PaymentChannel(firstNonEmptyOrderNo(cmd.Channel, string(entity.PaymentChannelAlipay)))
	orderNo := firstNonEmptyOrderNo(cmd.OrderNo, generateOrderNo())

	item, duplicate, err := s.createOrGetPayment(ctx, orderNo, cmd.TotalAmount, channel)
	if err != nil {
		return nil, err
	}
	if duplicate {
		s.log.Info("duplicate scan pay request, return existing payment", zap.String("order_no", orderNo))
		// P12：显式校验渠道一致性，避免跨渠道状态混乱（createOrGetPayment 已拦截，此处加固）。
		if item.Channel != channel {
			return nil, fmt.Errorf("%w: order_no %s local channel %s != requested %s", entity.ErrPaymentExists, orderNo, item.Channel, channel)
		}
		if item.Status != entity.PaymentStatusPending {
			return nil, fmt.Errorf("%w: order_no exists with status %s", entity.ErrPaymentExists, item.Status)
		}
	}

	switch channel {
	case entity.PaymentChannelAlipay:
		if err := s.requireAlipay(); err != nil {
			return nil, err
		}
		resp, err := s.alipay.Precreate(gopay.PrecreateRequest{
			OutTradeNo:  orderNo,
			Subject:     cmd.Subject,
			TotalAmount: formatCents(cmd.TotalAmount),
			Body:        cmd.Body,
		})
		if err != nil {
			s.markFailed(ctx, orderNo, "alipay precreate")
			return nil, fmt.Errorf("alipay precreate: %w", err)
		}
		s.log.Info("alipay scan pay requested", zap.String("order_no", orderNo), zap.Int64("total_amount_cents", cmd.TotalAmount))
		return &ScanPayResult{OrderNo: orderNo, QrCode: resp.QrCode, TotalAmount: cmd.TotalAmount}, nil
	case entity.PaymentChannelWechat:
		if err := s.requireWechat(); err != nil {
			return nil, err
		}
		resp, err := s.wechat.NativeScanPay(ctx, wechatpayx.NativeScanPayRequest{
			OutTradeNo:  orderNo,
			Description: cmd.Subject,
			Amount:      cmd.TotalAmount,
		})
		if err != nil {
			s.markFailed(ctx, orderNo, "wechat native scan pay")
			return nil, fmt.Errorf("wechat native scan pay: %w", err)
		}
		s.log.Info("wechat native scan pay requested", zap.String("order_no", orderNo), zap.Int64("total_amount_cents", cmd.TotalAmount))
		return &ScanPayResult{OrderNo: orderNo, QrCode: resp.CodeURL, TotalAmount: cmd.TotalAmount}, nil
	default:
		return nil, fmt.Errorf("%w: %s", entity.ErrPaymentChannelUnsupported, channel)
	}
}

// BarCodePay 发起付款码支付（商家扫用户付款码），同步扣款。
func (s *Service) BarCodePay(ctx context.Context, cmd dto.BarCodePayCommand) (*BarCodePayResult, error) {
	if err := s.requireAlipay(); err != nil {
		return nil, err
	}
	if cmd.Subject == "" || cmd.TotalAmount <= 0 || cmd.AuthCode == "" {
		return nil, fmt.Errorf("%w: subject, positive total_amount and auth_code are required", entity.ErrInvalidPaymentRequest)
	}
	orderNo := firstNonEmptyOrderNo(cmd.OrderNo, generateOrderNo())

	item, duplicate, err := s.createOrGetPayment(ctx, orderNo, cmd.TotalAmount, entity.PaymentChannelAlipay)
	if err != nil {
		return nil, err
	}
	if duplicate {
		// P0-2 条码支付防重复扣款：命中已有单时绝不直接重调渠道扣款。
		if item.Status == entity.PaymentStatusSuccess {
			s.log.Info("duplicate bar code pay, return existing success payment", zap.String("order_no", orderNo), zap.String("trade_no", item.TradeNo))
			return &BarCodePayResult{TradeNo: item.TradeNo, OrderNo: orderNo, TotalAmount: item.Amount}, nil
		}
		if item.Status != entity.PaymentStatusPending {
			return nil, fmt.Errorf("%w: order_no exists with status %s", entity.ErrPaymentExists, item.Status)
		}
		// pending 但本地单已存在：上一次 TradePay 可能已扣款成功仅响应丢失。
		// 先查渠道，渠道已成功则补记成功返回；未成功则拒绝重试（付款码一次性，无法安全重放）。
		queryResp, qErr := s.alipay.Query(ctx, gopay.QueryRequest{OutTradeNo: orderNo})
		if qErr == nil && (queryResp.TradeStatus == "TRADE_SUCCESS" || queryResp.TradeStatus == "TRADE_FINISHED") {
			paidAt := time.Now()
			if t, err := time.ParseInLocation("2006-01-02 15:04:05", queryResp.SendPayDate, time.Local); err == nil && !t.IsZero() {
				paidAt = t
			}
			if err := s.markSuccess(ctx, orderNo, queryResp.TradeNo, paidAt); err != nil {
				s.log.Warn("bar code pay duplicate mark success rejected", zap.String("order_no", orderNo), zap.Error(err))
			}
			s.log.Info("bar code pay duplicate resolved by channel query", zap.String("order_no", orderNo), zap.String("trade_no", queryResp.TradeNo))
			return &BarCodePayResult{TradeNo: queryResp.TradeNo, OrderNo: orderNo, TotalAmount: item.Amount}, nil
		}
		return nil, fmt.Errorf("%w: order_no %s is pending with one-time auth_code, query payment status instead of retrying", entity.ErrPaymentExists, orderNo)
	}

	resp, err := s.alipay.TradePay(ctx, gopay.TradePayRequest{
		OutTradeNo:  orderNo,
		Subject:     cmd.Subject,
		TotalAmount: formatCents(cmd.TotalAmount),
		AuthCode:    cmd.AuthCode,
		Body:        cmd.Body,
	})
	if err != nil {
		s.markFailed(ctx, orderNo, "alipay trade pay")
		return nil, fmt.Errorf("alipay trade pay: %w", err)
	}
	if err := s.markSuccess(ctx, orderNo, resp.TradeNo, time.Now()); err != nil {
		s.log.Warn("bar code pay mark success rejected", zap.String("order_no", orderNo), zap.Error(err))
	}
	s.log.Info("alipay bar code pay completed", zap.String("order_no", orderNo), zap.String("trade_no", resp.TradeNo), zap.Int64("total_amount_cents", cmd.TotalAmount))
	return &BarCodePayResult{
		TradeNo:      resp.TradeNo,
		OrderNo:      orderNo,
		TotalAmount:  cmd.TotalAmount,
		BuyerLogonID: resp.BuyerLogonId,
	}, nil
}

// markSuccess 将支付单标记为 success 并记录渠道交易号与支付时间。
// P5：终态保护——仅 pending 可成功；已 success 幂等返回；CLOSED/FAILED/REFUNDED 拒绝迁移。
// P3：成功后触发结算服务 MarkBillPaid，打通支付成功→账单置 PAID 链路（方案 A，无 MQ）。
func (s *Service) markSuccess(ctx context.Context, orderNo, tradeNo string, paidAt time.Time) error {
	if s.payments == nil {
		return nil
	}
	item, err := s.payments.FindByOrderNo(ctx, orderNo)
	if err != nil {
		s.log.Warn("mark payment success: payment not found", zap.String("order_no", orderNo), zap.Error(err))
		return err
	}
	if item.Status == entity.PaymentStatusSuccess {
		// 幂等：重复回调/查单补记直接返回，不重复触发结算。
		return nil
	}
	if item.Status != entity.PaymentStatusPending {
		return fmt.Errorf("%w: order_no %s cannot transit from %s to success", entity.ErrPaymentInvalidState, orderNo, item.Status)
	}
	item.Status = entity.PaymentStatusSuccess
	item.TradeNo = tradeNo
	item.PaidAt = &paidAt
	item.UpdatedAt = time.Now()
	if err := s.payments.Update(ctx, item); err != nil {
		s.log.Warn("mark payment success error", zap.String("order_no", orderNo), zap.Error(err))
		return err
	}
	s.triggerMarkBillPaid(orderNo, item.Amount)
	return nil
}

// triggerMarkBillPaid 支付成功后异步通知结算服务将账单置为 PAID（方案 A，无需 MQ）。
// 异步 + 短超时：失败不阻断支付响应，由账单查询/掉单补偿与对账兜底；
// 幂等由 settlement.MarkBillPaid 的 UNPAID→PAID 状态机保证。
func (s *Service) triggerMarkBillPaid(orderNo string, payFee int64) {
	if s.settlement == nil {
		s.log.Warn("settlement client not injected, skip mark bill paid", zap.String("order_no", orderNo))
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, err := s.settlement.MarkBillPaid(ctx, &settlementv1.MarkBillPaidRequest{
			OrderNo: orderNo,
			PayFee:  payFee,
		})
		if err != nil {
			s.log.Warn("settlement mark bill paid failed", zap.String("order_no", orderNo), zap.Error(err))
			return
		}
		s.log.Info("settlement bill marked paid", zap.String("order_no", orderNo), zap.Int64("pay_fee", payFee))
	}()
}

// Refund 发起退款，channel 区分 alipay / wechat（空值默认 alipay）。
// 退款成功后更新本地支付单 status=refunded。
func (s *Service) Refund(ctx context.Context, cmd dto.RefundCommand) (*RefundResult, error) {
	if cmd.OrderNo == "" && cmd.TradeNo == "" {
		return nil, fmt.Errorf("%w: order_no or trade_no is required", entity.ErrInvalidPaymentRequest)
	}
	if cmd.RefundAmount <= 0 {
		return nil, fmt.Errorf("%w: positive refund_amount is required", entity.ErrInvalidPaymentRequest)
	}
	channel := entity.PaymentChannel(firstNonEmptyOrderNo(cmd.Channel, string(entity.PaymentChannelAlipay)))
	outRequestNo := firstNonEmptyOrderNo(cmd.OutRequestNo, generateOrderNo())

	switch channel {
	case entity.PaymentChannelAlipay:
		if err := s.requireAlipay(); err != nil {
			return nil, err
		}
		// P6：幂等 + 防超退——同一订单已退款则直接返回历史结果，不重复调渠道；
		// 退款金额超过实付则拒绝（复用 payment AddRefundedAmount 的防超退思路）。
		if cmd.OrderNo != "" {
			if item, findErr := s.payments.FindByOrderNo(ctx, cmd.OrderNo); findErr == nil {
				if item.Status == entity.PaymentStatusRefunded {
					s.log.Info("alipay refund duplicate", zap.String("order_no", cmd.OrderNo), zap.String("out_request_no", outRequestNo))
					return &RefundResult{Success: true, OrderNo: cmd.OrderNo, RefundAmount: cmd.RefundAmount, Duplicate: true}, nil
				}
				if item.Status != entity.PaymentStatusSuccess {
					return nil, fmt.Errorf("%w: only success payment can be refunded, current=%s", entity.ErrInvalidPaymentRequest, item.Status)
				}
				if cmd.RefundAmount > item.Amount {
					return nil, fmt.Errorf("%w: refund %d exceeds paid %d", entity.ErrRefundExceedsPaid, cmd.RefundAmount, item.Amount)
				}
			} else if !errors.Is(findErr, entity.ErrPaymentNotFound) {
				return nil, findErr
			}
		}
		resp, err := s.alipay.Refund(ctx, gopay.RefundRequest{
			OutTradeNo:   cmd.OrderNo,
			TradeNo:      cmd.TradeNo,
			RefundAmount: formatCents(cmd.RefundAmount),
			RefundReason: cmd.RefundReason,
			OutRequestNo: outRequestNo,
		})
		if err != nil {
			return nil, fmt.Errorf("alipay refund: %w", err)
		}
		orderNo := cmd.OrderNo
		if orderNo == "" {
			orderNo = resp.Response.OutTradeNo
		}
		// P6：校验渠道资金变更标志，FundChange != Y 表示"已受理但未实际退款"，拒绝标记退款成功。
		if resp.Response.FundChange != "Y" {
			return nil, fmt.Errorf("%w: alipay refund not funded, fund_change=%q order_no=%s out_request_no=%s",
				entity.ErrInvalidPaymentRequest, resp.Response.FundChange, orderNo, outRequestNo)
		}
		s.markRefunded(ctx, orderNo)
		s.triggerRefundSettlement(orderNo, outRequestNo, cmd.RefundAmount)
		s.log.Info("alipay refund completed", zap.String("order_no", orderNo), zap.String("trade_no", resp.Response.TradeNo), zap.String("out_request_no", outRequestNo), zap.Int64("refund_amount_cents", cmd.RefundAmount))
		return &RefundResult{
			Success:      true,
			TradeNo:      resp.Response.TradeNo,
			OrderNo:      orderNo,
			RefundAmount: cmd.RefundAmount,
		}, nil
	case entity.PaymentChannelWechat:
		if err := s.requireWechat(); err != nil {
			return nil, err
		}
		if cmd.OrderNo == "" {
			return nil, fmt.Errorf("%w: order_no is required for wechat refund", entity.ErrInvalidPaymentRequest)
		}
		item, err := s.payments.FindByOrderNo(ctx, cmd.OrderNo)
		if err != nil {
			return nil, err
		}
		resp, err := s.wechat.Refund(ctx, wechatpayx.RefundRequest{
			OutTradeNo:    cmd.OrderNo,
			TransactionID: item.TradeNo,
			OutRefundNo:   outRequestNo,
			RefundAmount:  cmd.RefundAmount,
			TotalAmount:   item.Amount,
			Reason:        cmd.RefundReason,
		})
		if err != nil {
			return nil, fmt.Errorf("wechat refund: %w", err)
		}
		s.markRefunded(ctx, cmd.OrderNo)
		s.triggerRefundSettlement(cmd.OrderNo, outRequestNo, cmd.RefundAmount)
		s.log.Info("wechat refund completed", zap.String("order_no", cmd.OrderNo), zap.String("refund_id", resp.RefundID), zap.Int64("refund_amount_cents", cmd.RefundAmount))
		return &RefundResult{
			Success:      true,
			TradeNo:      resp.RefundID,
			OrderNo:      cmd.OrderNo,
			RefundAmount: cmd.RefundAmount,
		}, nil
	default:
		return nil, fmt.Errorf("%w: %s", entity.ErrPaymentChannelUnsupported, channel)
	}
}

// markRefunded 将支付单标记为 refunded。
func (s *Service) markRefunded(ctx context.Context, orderNo string) {
	if s.payments == nil {
		return
	}
	item, err := s.payments.FindByOrderNo(ctx, orderNo)
	if err != nil {
		s.log.Warn("mark payment refunded: payment not found", zap.String("order_no", orderNo), zap.Error(err))
		return
	}
	item.Status = entity.PaymentStatusRefunded
	item.UpdatedAt = time.Now()
	if err := s.payments.Update(ctx, item); err != nil {
		s.log.Warn("mark payment refunded error", zap.String("order_no", orderNo), zap.Error(err))
	}
}

// triggerRefundSettlement 退款成功后异步通知结算服务冲正司机已结算入账。
// 异步 + 短超时：冲正失败不阻断退款响应，由 refund_success 事件消费端与对账兜底；
// 冲正幂等由 settlement.ApplyRefund 的冲正流水锚点保证。
func (s *Service) triggerRefundSettlement(orderNo, refundNo string, amount int64) {
	if s.settlement == nil {
		s.log.Warn("settlement client not injected, skip refund settlement", zap.String("order_no", orderNo))
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, err := s.settlement.ApplyRefund(ctx, &settlementv1.ApplyRefundRequest{
			OrderNo:      orderNo,
			RefundNo:     refundNo,
			RefundAmount: amount,
		})
		if err != nil {
			s.log.Warn("settlement refund reversal failed", zap.String("order_no", orderNo), zap.Error(err))
			return
		}
		s.log.Info("settlement refund reversal applied",
			zap.String("order_no", orderNo),
			zap.String("refund_no", refundNo),
			zap.Int64("amount", amount))
	}()
}

// NotifyPayment 处理支付渠道异步通知：验签后更新本地支付单状态。
// alipay 通知为 x-www-form-urlencoded 表单（cmd.Form），wechat 通知为 JSON 原文（cmd.Payload）。
func (s *Service) NotifyPayment(ctx context.Context, cmd dto.NotifyCommand) (*dto.PaymentQueryResult, error) {
	channel := entity.PaymentChannel(cmd.Channel)
	switch channel {
	case entity.PaymentChannelAlipay:
		if err := s.requireAlipay(); err != nil {
			return nil, err
		}
		if err := s.requirePayments(); err != nil {
			return nil, err
		}
		if cmd.Form == "" {
			return nil, fmt.Errorf("%w: alipay notify form is empty", entity.ErrInvalidPaymentRequest)
		}
		req, err := http.NewRequest(http.MethodPost, "/pay/notify", bytes.NewBufferString(cmd.Form))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		bean, err := s.alipay.Notify(req)
		if err != nil {
			return nil, fmt.Errorf("alipay notify verify: %w", err)
		}
		orderNo := bean.GetString("out_trade_no")
		tradeNo := bean.GetString("trade_no")
		tradeStatus := bean.GetString("trade_status")
		if orderNo == "" {
			return nil, fmt.Errorf("%w: out_trade_no missing in alipay notify", entity.ErrInvalidPaymentRequest)
		}
		if tradeStatus == "TRADE_SUCCESS" || tradeStatus == "TRADE_FINISHED" {
			// P4：校验回调金额与本地单一致，防金额篡改/不一致（T12）。
			localItem, findErr := s.payments.FindByOrderNo(ctx, orderNo)
			if findErr != nil {
				return nil, findErr
			}
			callbackAmount, parseErr := parseYuanToCents(bean.GetString("total_amount"))
			if parseErr != nil {
				return nil, fmt.Errorf("%w: invalid total_amount %q in alipay notify", entity.ErrInvalidPaymentRequest, bean.GetString("total_amount"))
			}
			if callbackAmount != localItem.Amount {
				return nil, fmt.Errorf("%w: order_no %s callback amount %d != local amount %d", entity.ErrPaymentAmountMismatch, orderNo, callbackAmount, localItem.Amount)
			}
			paidAt := time.Now()
			if t, err := time.ParseInLocation("2006-01-02 15:04:05", bean.GetString("gmt_payment"), time.Local); err == nil && !t.IsZero() {
				paidAt = t
			}
			if err := s.markSuccess(ctx, orderNo, tradeNo, paidAt); err != nil {
				// P5：终态保护失败（CLOSED/FAILED 收到成功）返回错误，让渠道侧重试/对账，绝不把终态单改回 SUCCESS。
				return nil, err
			}
			s.log.Info("alipay notify success", zap.String("order_no", orderNo), zap.String("trade_no", tradeNo), zap.String("trade_status", tradeStatus))
		} else if tradeStatus == "TRADE_CLOSED" {
			s.markFailed(ctx, orderNo, "alipay notify trade closed")
		}
		return s.queryLocal(ctx, orderNo)
	case entity.PaymentChannelWechat:
		if err := s.requireWechat(); err != nil {
			return nil, err
		}
		if cmd.Payload == "" {
			return nil, fmt.Errorf("%w: wechat notify payload is empty", entity.ErrInvalidPaymentRequest)
		}
		req, err := http.NewRequest(http.MethodPost, "/pay/notify", bytes.NewBufferString(cmd.Payload))
		if err != nil {
			return nil, err
		}
		for k, v := range cmd.Headers {
			req.Header.Set(k, v)
		}
		result, err := s.wechat.Notify(req)
		if err != nil {
			return nil, fmt.Errorf("wechat notify verify: %w", err)
		}
		if result.OutTradeNo == "" {
			return nil, fmt.Errorf("%w: out_trade_no missing in wechat notify", entity.ErrInvalidPaymentRequest)
		}
		if result.TradeState == "SUCCESS" {
			paidAt := time.Now()
			if t, err := time.Parse(time.RFC3339, result.SuccessTime); err == nil {
				paidAt = t
			}
			s.markSuccess(ctx, result.OutTradeNo, result.TransactionID, paidAt)
			s.log.Info("wechat notify success", zap.String("order_no", result.OutTradeNo), zap.String("transaction_id", result.TransactionID))
		}
		return s.queryLocal(ctx, result.OutTradeNo)
	default:
		return nil, fmt.Errorf("%w: %s", entity.ErrPaymentChannelUnsupported, channel)
	}
}

// QueryPayment 按 order_no 查询本地支付单；若仍为 pending 且渠道客户端可用，
// 主动调用渠道查单刷新状态（掉单补偿核心）。
func (s *Service) QueryPayment(ctx context.Context, orderNo string) (*dto.PaymentQueryResult, error) {
	if orderNo == "" {
		return nil, fmt.Errorf("%w: order_no is required", entity.ErrInvalidPaymentRequest)
	}
	item, err := s.payments.FindByOrderNo(ctx, orderNo)
	if err != nil {
		return nil, err
	}
	if item.Status != entity.PaymentStatusPending {
		return s.toQueryResult(item), nil
	}

	switch item.Channel {
	case entity.PaymentChannelAlipay:
		if s.alipay == nil {
			return s.toQueryResult(item), nil
		}
		result, err := s.alipay.Query(ctx, gopay.QueryRequest{OutTradeNo: orderNo})
		if err != nil {
			s.log.Warn("alipay query order failed", zap.String("order_no", orderNo), zap.Error(err))
			return s.toQueryResult(item), nil
		}
		if result.TradeStatus == "TRADE_SUCCESS" || result.TradeStatus == "TRADE_FINISHED" {
			paidAt := time.Now()
			if t, err := time.ParseInLocation("2006-01-02 15:04:05", result.SendPayDate, time.Local); err == nil && !t.IsZero() {
				paidAt = t
			}
			s.markSuccess(ctx, orderNo, result.TradeNo, paidAt)
			s.log.Info("alipay query refreshed payment success", zap.String("order_no", orderNo), zap.String("trade_no", result.TradeNo))
		} else if result.TradeStatus == "TRADE_CLOSED" {
			s.markFailed(ctx, orderNo, "alipay query trade closed")
		}
	case entity.PaymentChannelWechat:
		if s.wechat == nil {
			return s.toQueryResult(item), nil
		}
		result, err := s.wechat.QueryOrder(ctx, orderNo)
		if err != nil {
			s.log.Warn("wechat query order failed", zap.String("order_no", orderNo), zap.Error(err))
			return s.toQueryResult(item), nil
		}
		if result.TradeState == "SUCCESS" {
			paidAt := time.Now()
			if t, err := time.Parse(time.RFC3339, result.SuccessTime); err == nil {
				paidAt = t
			}
			s.markSuccess(ctx, orderNo, result.TransactionID, paidAt)
			s.log.Info("wechat query refreshed payment success", zap.String("order_no", orderNo), zap.String("transaction_id", result.TransactionID))
		}
	}

	refreshed, err := s.payments.FindByOrderNo(ctx, orderNo)
	if err != nil {
		return nil, err
	}
	return s.toQueryResult(refreshed), nil
}

// CompensatePending 扫描超过超时阈值的 pending 支付单并逐个刷新状态。
// 用于 cmd/shop/main.go 的后台补偿 goroutine。
func (s *Service) CompensatePending(ctx context.Context, olderThan time.Duration) (int, error) {
	if s.payments == nil {
		return 0, fmt.Errorf("%w: payment repository not injected", entity.ErrInvalidPaymentRequest)
	}
	items, err := s.payments.ListPendingBefore(ctx, time.Now().Add(-olderThan), 200)
	if err != nil {
		return 0, err
	}
	checked := 0
	for _, item := range items {
		if _, err := s.QueryPayment(ctx, item.OrderNo); err != nil {
			s.log.Warn("compensate query failed", zap.String("order_no", item.OrderNo), zap.Error(err))
			continue
		}
		checked++
	}
	if checked > 0 {
		s.log.Info("pending payment compensation done", zap.Int("checked", checked), zap.Int("total_pending", len(items)))
	}
	return checked, nil
}

// queryLocal 返回本地支付单的查询结果。
func (s *Service) queryLocal(ctx context.Context, orderNo string) (*dto.PaymentQueryResult, error) {
	item, err := s.payments.FindByOrderNo(ctx, orderNo)
	if err != nil {
		return nil, err
	}
	return s.toQueryResult(item), nil
}

// toQueryResult 将领域支付单转换为查询结果 DTO。
func (s *Service) toQueryResult(item *entity.Payment) *dto.PaymentQueryResult {
	result := &dto.PaymentQueryResult{
		OrderNo:     item.OrderNo,
		Status:      string(item.Status),
		Channel:     string(item.Channel),
		TradeNo:     item.TradeNo,
		TotalAmount: item.Amount,
	}
	if item.PaidAt != nil {
		result.PaidAtUnix = item.PaidAt.Unix()
	}
	return result
}

// formatCents 将 int64 分金额转换为两位小数元字符串，如 990 -> "9.90"。
func formatCents(cents int64) string {
	if cents <= 0 {
		return "0.00"
	}
	return fmt.Sprintf("%d.%02d", cents/100, cents%100)
}

// parseYuanToCents 将两位小数元字符串解析为 int64 分，如 "9.90" -> 990。
// 非法格式（非数字、超过两位小数）返回错误。
func parseYuanToCents(s string) (int64, error) {
	body := s
	neg := false
	if strings.HasPrefix(body, "-") {
		neg = true
		body = strings.TrimPrefix(body, "-")
	}
	parts := strings.Split(body, ".")
	if len(parts) == 0 || len(parts) > 2 {
		return 0, fmt.Errorf("invalid yuan format %q", s)
	}
	intPart := parts[0]
	fracPart := ""
	if len(parts) == 2 {
		fracPart = parts[1]
	}
	if intPart == "" {
		intPart = "0"
	}
	if len(fracPart) > 2 {
		return 0, fmt.Errorf("invalid yuan format %q", s)
	}
	for len(fracPart) < 2 {
		fracPart += "0"
	}
	full := intPart + fracPart
	if full == "" {
		return 0, fmt.Errorf("invalid yuan format %q", s)
	}
	v, err := strconv.ParseInt(full, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid yuan format %q", s)
	}
	if neg {
		v = -v
	}
	return v, nil
}

// generateOrderNo 生成商户订单号：毫秒时间戳 + 6 位随机后缀。
func generateOrderNo() string {
	return fmt.Sprintf("%d%06d", time.Now().UnixMilli(), rand.Intn(1000000))
}

func firstNonEmptyOrderNo(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
