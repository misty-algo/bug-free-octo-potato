package entity

import (
	"testing"
)

// 覆盖司机四态状态机：出车 → 预占 → 忙碌 → 释放，以及收车与掉线保留任务。
func TestDriverStateMachine(t *testing.T) {
	d, err := NewDriver("测试司机", "13800000000")
	if err != nil {
		t.Fatalf("注册司机失败: %v", err)
	}
	if d.Status != DriverStatusIdle || d.Mode != ModeGrab {
		t.Fatalf("默认状态应为空闲+抢单，实际 %s/%s", d.Status, d.Mode)
	}

	if err := d.ToPreOccupied("task-1"); err != nil {
		t.Fatalf("空闲 → 接单预占应允许: %v", err)
	}
	if d.Status != DriverStatusPreOccupied || d.ActiveTaskID != "task-1" {
		t.Fatalf("预占态或活动任务不正确: %s / %s", d.Status, d.ActiveTaskID)
	}

	if err := d.ToBusy("task-1"); err != nil {
		t.Fatalf("预占 → 忙碌应允许: %v", err)
	}
	if d.Status != DriverStatusBusy {
		t.Fatalf("应为忙碌态，实际 %s", d.Status)
	}

	if err := d.SetStatus(DriverStatusOffline); err != ErrBusyCannotOffline {
		t.Fatalf("忙碌中收车应返回 ErrBusyCannotOffline，实际 %v", err)
	}

	d.SetOfflineAfterTrip(true)
	if !d.ReleaseTask() {
		t.Fatalf("完单应命中「本单后收车」")
	}
	d.Status = DriverStatusOffline
	if d.ActiveTaskID != "" {
		t.Fatalf("完单后必须清空活动任务")
	}
}

// 覆盖区域偏好：非法包围盒必须拒绝，合法片区按包含边界过滤订单。
func TestDriverAreaConfig(t *testing.T) {
	d, _ := NewDriver("区域司机", "13800000001")
	if err := d.UpdateAreaConfig(true, []AreaBox{{Name: "错误", MinLat: 40, MinLng: 116, MaxLat: 39, MaxLng: 115}}); err != ErrInvalidDriver {
		t.Fatalf("min>max 的片区应被拒绝，实际 %v", err)
	}
	if err := d.UpdateAreaConfig(true, []AreaBox{{Name: "成都", MinLat: 30.6, MinLng: 104.0, MaxLat: 30.75, MaxLng: 104.15}}); err != nil {
		t.Fatalf("合法片区应允许: %v", err)
	}
	if !d.InArea(30.6, 104.0) {
		t.Fatal("区域边界上的点应视为包含")
	}
	if d.InArea(31.5, 104.0) {
		t.Fatal("片区外的点不应包含")
	}
}

// 覆盖金额口径与坐标校验：司机收入 = 乘客实付 - 抽成 - 司机券 + 补贴。
func TestHallOrderAmount(t *testing.T) {
	order, err := NewHallOrder(CreateHallOrderParam{
		PassengerName:     "乘客甲",
		Origin:            Location{Name: "春熙路", Lat: 30.657, Lng: 104.081},
		Destination:       Location{Name: "天府广场", Lat: 30.657, Lng: 104.065},
		PassengerPaidFen:  3000,
		PlatformCouponFen: 300,
		CommissionFen:     600,
		SubsidyFen:        200,
		ListenWindowSec:   60,
		TotalWindowSec:    120,
	})
	if err != nil {
		t.Fatalf("创建订单失败: %v", err)
	}
	// 平台券由平台承担，不计入司机收入。
	if got := order.Amount().DriverIncomeFen; got != 3000-600+200 {
		t.Fatalf("司机收入应为 2600 分，实际 %d", got)
	}

	if _, err := NewHallOrder(CreateHallOrderParam{
		PassengerName: "无坐标",
		PassengerPaidFen: 1000,
	}); err != ErrInvalidOrder {
		t.Fatalf("缺少坐标应返回 ErrInvalidOrder，实际 %v", err)
	}
}
