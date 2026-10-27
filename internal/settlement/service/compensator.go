package service

import (
	"context"
	"time"

	"go.uber.org/zap"
)

// ReconcileWorker 掉单补偿工人：周期扫描已支付但未结算的账单并补分账。
//
// 存在的原因：支付链路 MarkBillPaid → SplitBill 是 fire-and-forget，
// 分账失败只记日志，没有补偿的话钱会永远滞留在账单里、进不了司机钱包。
// 补偿保证的是最终一致，因此用周期扫描而非即时重试。
type ReconcileWorker struct {
	svc        *Service
	log        *zap.Logger
	interval   time.Duration
	batchLimit int64

	cancel context.CancelFunc
	done   chan struct{}
}

// NewReconcileWorker 创建补偿工人。interval 为扫描间隔，batchLimit 为单批最多处理的单数。
func NewReconcileWorker(svc *Service, log *zap.Logger, interval time.Duration, batchLimit int64) *ReconcileWorker {
	return &ReconcileWorker{
		svc:        svc,
		log:        log,
		interval:   interval,
		batchLimit: batchLimit,
		done:       make(chan struct{}),
	}
}

// Start 启动后台补偿循环，非阻塞。启动时先跑一批，把进程重启期间积压的掉单尽快清掉。
func (w *ReconcileWorker) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	w.cancel = cancel
	go func() {
		defer close(w.done)
		ticker := time.NewTicker(w.interval)
		defer ticker.Stop()
		for {
			w.runOnce(ctx)
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

// Stop 停止补偿循环并等待当前批次结束。
func (w *ReconcileWorker) Stop() {
	if w.cancel == nil {
		return
	}
	w.cancel()
	<-w.done
	w.cancel = nil
}

func (w *ReconcileWorker) runOnce(ctx context.Context) {
	// 单批超时独立于扫描间隔：扫描失败或卡住不能拖垮下一轮。
	batchCtx, cancel := context.WithTimeout(ctx, w.interval)
	defer cancel()
	compensated, err := w.svc.ReconcilePendingSettlements(batchCtx, w.batchLimit)
	switch {
	case err != nil:
		w.log.Warn("reconcile pending settlements failed", zap.Error(err))
	case compensated > 0:
		w.log.Info("reconcile pending settlements compensated", zap.Int("compensated", compensated))
	}
}
