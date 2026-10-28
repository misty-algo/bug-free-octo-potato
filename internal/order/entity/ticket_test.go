package entity

import (
	"testing"
	"time"
)

func TestNewTicketInitialState(t *testing.T) {
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	tk, err := NewTicket("id1", "TK1", "oid", "P0001", "pax1", "drv1",
		TicketTypeRouteDetour, "司机绕路", []string{"img1"}, now)
	if err != nil {
		t.Fatalf("NewTicket: %v", err)
	}
	if tk.State != TicketPending {
		t.Fatalf("初始状态 = %q, want %q", tk.State, TicketPending)
	}
	if len(tk.Timeline) != 1 || tk.Timeline[0].Action != "create" {
		t.Fatalf("初始时间线错误: %+v", tk.Timeline)
	}
}

func TestNewTicketValidation(t *testing.T) {
	now := time.Now()
	if _, err := NewTicket("i", "n", "o", "P", "p", "d", "BAD_TYPE", "x", nil, now); err == nil {
		t.Fatal("非法类型应报错")
	}
	if _, err := NewTicket("i", "n", "o", "P", "p", "d", TicketTypeLostItem, "", nil, now); err == nil {
		t.Fatal("空内容应报错")
	}
	if _, err := NewTicket("i", "n", "o", "P", "p", "d", TicketTypeLostItem, "x",
		[]string{"1", "2", "3", "4", "5", "6", "7"}, now); err == nil {
		t.Fatal("超 6 图应报错")
	}
}

func TestTicketStateNameMapping(t *testing.T) {
	cases := map[string]string{
		TicketPending:    "待处理",
		TicketProcessing: "处理中",
		TicketResolved:   "已完结",
	}
	for state, want := range cases {
		if got := TicketStateName(state); got != want {
			t.Fatalf("TicketStateName(%s) = %q, want %q", state, got, want)
		}
	}
	if TicketStateName("WHATEVER") != "" {
		t.Fatal("未知状态应返回空串")
	}
}
