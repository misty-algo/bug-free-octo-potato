package service

import (
	"context"
	"time"

	"go.uber.org/zap"
)

// BillReconcileWorker 建账补偿工人：周期扫描已结束但未建账的行程并补建账。
//
// 存在的原因：EndTrip 里行程计价与建账是两步，中间失败会留下「行程已结束但账单不存在」
// 的中间态——乘客付不了款，整条资金链卡死，且没有任何告警信号。
// 补偿保证的是最终一致，因此用周期扫描而非即时重试。
//
// 与 settlement 域的 ReconcileWorker 是上下游关系：这里补账单，那里补分账。
// 账单补出来后，行程才能被支付，PAID 之后的对账由 settlement 侧接手。
type BillReconcileWorker struct {
	svc        *Service
	log        *zap.Logger
	interval   time.Duration
	batchLimit int

	cancel context.CancelFunc
	done   chan struct{}
}

// NewBillReconcileWorker 创建建账补偿工人。interval 为扫描间隔，batchLimit 为单批最多处理的单数。
func NewBillReconcileWorker(svc *Service, log *zap.Logger, interval time.Duration, batchLimit int) *BillReconcileWorker {
	return &BillReconcileWorker{
		svc:        svc,
		log:        log,
		interval:   interval,
		batchLimit: batchLimit,
		done:       make(chan struct{}),
	}
}

// Start 启动后台补偿循环，非阻塞。启动时先跑一批，把进程重启期间的积压尽快清掉。
func (w *BillReconcileWorker) Start() {
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
func (w *BillReconcileWorker) Stop() {
	if w.cancel == nil {
		return
	}
	w.cancel()
	<-w.done
	w.cancel = nil
}

func (w *BillReconcileWorker) runOnce(ctx context.Context) {
	// 单批超时独立于扫描间隔：扫描失败或卡住不能拖垮下一轮。
	batchCtx, cancel := context.WithTimeout(ctx, w.interval)
	defer cancel()
	compensated, err := w.svc.ReconcilePendingBills(batchCtx, w.batchLimit)
	switch {
	case err != nil:
		w.log.Warn("reconcile pending bills failed", zap.Error(err))
	case compensated > 0:
		w.log.Info("reconcile pending bills compensated", zap.Int("compensated", compensated))
	}
}
