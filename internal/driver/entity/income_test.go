package entity

import (
	"testing"

	"github.com/BwCloudWeGo/bw-cli/pkg/contracts"
)

// 覆盖资金五桶：可用桶不足时不能预占，流转后总额守恒。
func TestFundsTransfer(t *testing.T) {
	f := Funds{Available: 1000}
	if !f.CanTransfer(BucketAvailable, 1000) {
		t.Fatal("可用 1000 分应允许预占 1000 分")
	}
	if f.CanTransfer(BucketAvailable, 1001) {
		t.Fatal("超出可用余额不应允许预占")
	}
	if err := f.Add(BucketAvailable, -1000); err != nil {
		t.Fatalf("扣减可用桶失败: %v", err)
	}
	if err := f.Add(BucketWithdraw, 1000); err != nil {
		t.Fatalf("增加提现预占桶失败: %v", err)
	}
	if f.Available != 0 || f.Withdraw != 1000 {
		t.Fatalf("预占后桶金额不正确: A=%d W=%d", f.Available, f.Withdraw)
	}
}

// 覆盖提现审批：双人审批需两位不同审批人。
func TestWithdrawalDualApproval(t *testing.T) {
	w, err := NewWithdrawal("driver-1", "wd-key-1", 60000, true)
	if err != nil {
		t.Fatalf("创建提现单失败: %v", err)
	}
	if w.Status != WithdrawStatusReviewing || !w.DualApproval {
		t.Fatalf("初始状态或双人审批标记不正确: %s/%v", w.Status, w.DualApproval)
	}
	if err := w.Approve("admin01", "ok"); err != nil {
		t.Fatalf("第一位审批人应通过: %v", err)
	}
	if err := w.Approve("admin01", "again"); err != ErrDuplicateApprover {
		t.Fatalf("同一审批人重复应返回 ErrDuplicateApprover，实际 %v", err)
	}
	if w.ApprovalsEnough() {
		t.Fatal("仅一位审批人时不应满足双人审批")
	}
	if err := w.Approve("admin02", "ok"); err != nil {
		t.Fatalf("第二位审批人应通过: %v", err)
	}
	if !w.ApprovalsEnough() {
		t.Fatal("两位不同审批人应满足双人审批")
	}
}

// 覆盖反向流水：退款生成负数凭证并指向原凭证，原凭证保持不变。
func TestReverseFlow(t *testing.T) {
	origin, err := NewIncomeFlow("driver-1", "order-1", "task-1", "evt-pay-1", "key-1", contracts.TaskAmount{
		PassengerPaidFen: 3000, CommissionFen: 600, DriverIncomeFen: 2400,
	}, "2026-10-22")
	if err != nil {
		t.Fatalf("创建收益凭证失败: %v", err)
	}
	originIncome := origin.DriverIncomeFen

	reverse, err := NewReverseFlow(origin, FlowTypeRefund, "evt-refund-1", "key-2", "2026-10-23", 1000, "乘客退款")
	if err != nil {
		t.Fatalf("创建反向流水失败: %v", err)
	}
	if reverse.ReverseOf != origin.ID {
		t.Fatalf("反向流水应指向原凭证 %s，实际 %s", origin.ID, reverse.ReverseOf)
	}
	if reverse.DriverIncomeFen != -1000 {
		t.Fatalf("反向金额应为 -1000，实际 %d", reverse.DriverIncomeFen)
	}
	if origin.DriverIncomeFen != originIncome {
		t.Fatalf("原凭证金额被修改: %d", origin.DriverIncomeFen)
	}
	if reverse.AccountingDate != "2026-10-23" {
		t.Fatalf("跨日退款应记入退款当日，实际 %s", reverse.AccountingDate)
	}
}
