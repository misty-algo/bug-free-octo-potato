package handler

import (
	"context"
	"testing"
	"time"

	passengerv1 "github.com/BwCloudWeGo/bw-cli/api/gen/passenger/v1"
)

// TestGRPCLoginFlow 走完整 gRPC 链路：发码 → 登录 → 首页。
func TestGRPCLoginFlow(t *testing.T) {
	client, _ := newTestServer(t)
	ctx := context.Background()

	// 发验证码。
	codeResp, err := client.SendCode(ctx, &passengerv1.SendCodeRequest{Phone: "13800000000"})
	if err != nil {
		t.Fatalf("SendCode() error = %v", err)
	}
	if codeResp.GetDebugCode() == "" {
		t.Fatal("DebugCode is empty, want the local channel to hand the code back")
	}

	// 验证码登录。
	loginResp, err := client.Login(ctx, &passengerv1.LoginRequest{
		Channel: "sms", Phone: "13800000000", Code: codeResp.GetDebugCode(), Role: "passenger",
	})
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if loginResp.GetAccessToken() == "" {
		t.Fatal("AccessToken is empty, want a session")
	}
	if !loginResp.GetIsNewUser() {
		t.Fatal("IsNewUser = false, want true for a first login")
	}
	if loginResp.GetRestoreTarget() == "" {
		t.Fatal("RestoreTarget is empty, want the server-computed target")
	}

	// 首页。
	homeResp, err := client.Home(ctx, &passengerv1.HomeRequest{
		AccountId: loginResp.GetAccountId(), Lng: 121.601, Lat: 31.2054, HasLocation: true,
	})
	if err != nil {
		t.Fatalf("Home() error = %v", err)
	}
	if len(homeResp.GetCarTypes()) != 3 {
		t.Fatalf("car types = %d, want 3", len(homeResp.GetCarTypes()))
	}
	if len(homeResp.GetQuickEntries()) != 2 {
		t.Fatalf("quick entries = %d, want 2", len(homeResp.GetQuickEntries()))
	}
}

// TestGRPCChannelLogin 验证微信与 QQ 渠道登录。
func TestGRPCChannelLogin(t *testing.T) {
	client, _ := newTestServer(t)
	ctx := context.Background()

	for _, channel := range []string{"wechat", "qq"} {
		resp, err := client.Login(ctx, &passengerv1.LoginRequest{Channel: channel, Code: "code-1", Role: "passenger"})
		if err != nil {
			t.Fatalf("Login(%s) error = %v", channel, err)
		}
		if resp.GetAccessToken() == "" {
			t.Fatalf("Login(%s) returned no token", channel)
		}
	}
}

// TestGRPCQuotePriceForVisitor 验证游客看价拿不到估价单。
func TestGRPCQuotePriceForVisitor(t *testing.T) {
	client, _ := newTestServer(t)

	resp, err := client.QuotePrice(context.Background(), &passengerv1.EstimateRequest{
		CarType: 1, StartLng: 121.601, StartLat: 31.2054, EndLng: 121.3208, EndLat: 31.1979,
	})
	if err != nil {
		t.Fatalf("QuotePrice() error = %v", err)
	}
	if resp.GetEstimateNo() != "" {
		t.Fatalf("EstimateNo = %q, want empty for a visitor", resp.GetEstimateNo())
	}
	if resp.GetPayFee() <= 0 {
		t.Fatalf("PayFee = %d, want a positive amount", resp.GetPayFee())
	}
}

// TestGRPCEstimateAndCreateOrder 验证估价 → 下单的链路。
func TestGRPCEstimateAndCreateOrder(t *testing.T) {
	client, store := newTestServer(t)
	ctx := context.Background()

	// 建一个已实名通过的账号。
	loginResp, err := client.Login(ctx, &passengerv1.LoginRequest{
		Channel: "wechat", Code: "code-1", Role: "passenger",
	})
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	accountID := loginResp.GetAccountId()
	store.accounts[accountID].RealNameVerified = true

	// 估价。
	estResp, err := client.Estimate(ctx, &passengerv1.EstimateRequest{
		AccountId: accountID, CarType: 1,
		StartLng: 121.601, StartLat: 31.2054, EndLng: 121.3208, EndLat: 31.1979,
	})
	if err != nil {
		t.Fatalf("Estimate() error = %v", err)
	}
	if estResp.GetEstimateNo() == "" {
		t.Fatal("EstimateNo is empty, want an estimate for a signed-in account")
	}

	// 下单。
	orderResp, err := client.CreateOrder(ctx, &passengerv1.CreateOrderRequest{
		AccountId: accountID, EstimateNo: estResp.GetEstimateNo(), CarType: 1,
		StartName: "张江", StartAddr: "科苑路", StartLng: 121.601, StartLat: 31.2054,
		EndName: "虹桥", EndAddr: "申贵路", EndLng: 121.3208, EndLat: 31.1979,
		IdempotencyKey: "key-1",
	})
	if err != nil {
		t.Fatalf("CreateOrder() error = %v", err)
	}
	if orderResp.GetOrder().GetState() != "PENDING" {
		t.Fatalf("state = %q, want PENDING", orderResp.GetOrder().GetState())
	}

	// 幂等：相同键重试返回同一订单。
	again, err := client.CreateOrder(ctx, &passengerv1.CreateOrderRequest{
		AccountId: accountID, EstimateNo: estResp.GetEstimateNo(), CarType: 1,
		StartLng: 121.601, StartLat: 31.2054, EndLng: 121.3208, EndLat: 31.1979,
		IdempotencyKey: "key-1",
	})
	if err != nil {
		t.Fatalf("second CreateOrder() error = %v", err)
	}
	if again.GetOrder().GetOrderId() != orderResp.GetOrder().GetOrderId() {
		t.Fatalf("order ids differ: %q vs %q", again.GetOrder().GetOrderId(), orderResp.GetOrder().GetOrderId())
	}
}

// TestGRPCAddressManagement 验证常用地址与历史地址的增删查。
func TestGRPCAddressManagement(t *testing.T) {
	client, _ := newTestServer(t)
	ctx := context.Background()

	saveResp, err := client.SaveAddress(ctx, &passengerv1.SaveAddressRequest{
		AccountId: "account-1", Type: "home", Tag: "家", Name: "家", Addr: "科苑路 88 号",
		Lng: 121.601, Lat: 31.2054,
	})
	if err != nil {
		t.Fatalf("SaveAddress() error = %v", err)
	}
	if saveResp.GetAddress().GetId() == "" {
		t.Fatal("address id is empty")
	}

	listResp, err := client.ListAddresses(ctx, &passengerv1.ListAddressesRequest{
		AccountId: "account-1", Type: "home",
	})
	if err != nil {
		t.Fatalf("ListAddresses() error = %v", err)
	}
	if len(listResp.GetList()) != 1 {
		t.Fatalf("addresses = %d, want 1", len(listResp.GetList()))
	}

	// 上报历史地址。
	if _, err := client.RecordHistory(ctx, &passengerv1.RecordHistoryRequest{
		AccountId: "account-1", PoiId: "poi_1", Name: "张江", Addr: "科苑路",
		Lng: 121.601, Lat: 31.2054,
	}); err != nil {
		t.Fatalf("RecordHistory() error = %v", err)
	}
	historyResp, err := client.ListAddresses(ctx, &passengerv1.ListAddressesRequest{
		AccountId: "account-1", Type: "history",
	})
	if err != nil {
		t.Fatalf("ListAddresses(history) error = %v", err)
	}
	if len(historyResp.GetList()) != 1 {
		t.Fatalf("history = %d, want 1", len(historyResp.GetList()))
	}

	// 删除常用地址。
	if _, err := client.DeleteAddress(ctx, &passengerv1.DeleteAddressRequest{
		AccountId: "account-1", AddressId: saveResp.GetAddress().GetId(),
	}); err != nil {
		t.Fatalf("DeleteAddress() error = %v", err)
	}
	after, err := client.ListAddresses(ctx, &passengerv1.ListAddressesRequest{
		AccountId: "account-1", Type: "home",
	})
	if err != nil {
		t.Fatalf("ListAddresses() error = %v", err)
	}
	if len(after.GetList()) != 0 {
		t.Fatalf("addresses = %d, want 0 after the delete", len(after.GetList()))
	}
}

// TestGRPCSearchAndRecommend 验证地点检索与上车点推荐。
func TestGRPCSearchAndRecommend(t *testing.T) {
	client, _ := newTestServer(t)
	ctx := context.Background()

	searchResp, err := client.SearchPOI(ctx, &passengerv1.SearchPOIRequest{
		Keyword: "张江", HasCoord: true, Lng: 121.601, Lat: 31.2054, Limit: 10,
	})
	if err != nil {
		t.Fatalf("SearchPOI() error = %v", err)
	}
	if len(searchResp.GetList()) != 1 {
		t.Fatalf("results = %d, want 1", len(searchResp.GetList()))
	}
	if !searchResp.GetList()[0].GetHasDistance() {
		t.Fatal("HasDistance = false, want the distance to be present with coordinates")
	}

	recommendResp, err := client.RecommendPickup(ctx, &passengerv1.RecommendPickupRequest{
		Lng: 121.601, Lat: 31.2054, Limit: 10,
	})
	if err != nil {
		t.Fatalf("RecommendPickup() error = %v", err)
	}
	if len(recommendResp.GetList()) != 1 {
		t.Fatalf("recommendations = %d, want 1", len(recommendResp.GetList()))
	}
}

// TestGRPCLogoutRevokesTheSession 验证退出登录撤销刷新令牌。
func TestGRPCLogoutRevokesTheSession(t *testing.T) {
	client, store := newTestServer(t)
	ctx := context.Background()

	loginResp, err := client.Login(ctx, &passengerv1.LoginRequest{
		Channel: "wechat", Code: "code-1", Role: "passenger",
	})
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	accountID := loginResp.GetAccountId()
	if len(store.sessions) == 0 {
		t.Fatal("no session was saved")
	}
	if _, err := client.Logout(ctx, &passengerv1.LogoutRequest{AccountId: accountID}); err != nil {
		t.Fatalf("Logout() error = %v", err)
	}
	for _, session := range store.sessions {
		if session.AccountID == accountID && session.ExpiresAt.After(time.Now().Add(time.Hour)) {
			t.Fatal("the session was not revoked by the logout")
		}
	}
}

// TestGRPCBindPhoneReturnsMaskedNumber 验证手机号脱敏。
func TestGRPCBindPhoneReturnsMaskedNumber(t *testing.T) {
	client, _ := newTestServer(t)
	ctx := context.Background()

	// 先登录建号，才能绑手机号。
	loginResp, err := client.Login(ctx, &passengerv1.LoginRequest{
		Channel: "wechat", Code: "code-1", Role: "passenger",
	})
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	resp, err := client.BindPhone(ctx, &passengerv1.BindPhoneRequest{
		AccountId: loginResp.GetAccountId(), Phone: "13712345678",
	})
	if err != nil {
		t.Fatalf("BindPhone() error = %v", err)
	}
	if resp.GetPhone() != "137****5678" {
		t.Fatalf("Phone = %q, want the masked form", resp.GetPhone())
	}
}
