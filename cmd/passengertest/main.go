// Command passengertest 直连乘客端 gRPC 服务跑一遍真实链路，用于联调验证。
//
// 运行方式：
//   go run ./cmd/passengertest -target 127.0.0.1:9005
//
// 可选参数 -mongo 用于联调时把测试账号的实名状态置为通过。
// 实名认证本身属于「乘客端-我的」模块，不在本端范围内，
// 联调时直接改库是临时手段，正式流程应由实名域写回。
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	passengerv1 "github.com/BwCloudWeGo/bw-cli/api/gen/passenger/v1"
	"github.com/BwCloudWeGo/bw-cli/pkg/mongox"
)

func main() {
	target := flag.String("target", "127.0.0.1:9005", "乘客端 gRPC 地址")
	mongoURI := flag.String("mongo", "", "MongoDB 连接串，留空则跳过实名置位")
	mongoUser := flag.String("mongo-user", "", "MongoDB 用户名")
	mongoPassword := flag.String("mongo-password", "", "MongoDB 密码")
	mongoDB := flag.String("mongo-db", "huaxiaozhu", "MongoDB 数据库名")
	flag.Parse()

	conn, err := grpc.NewClient(*target, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		fail("dial %s: %v", *target, err)
	}
	defer func() { _ = conn.Close() }()
	client := passengerv1.NewPassengerServiceClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 用时间戳派生手机号，避免重复运行时撞上已占用的号码。
	suffix := time.Now().UnixNano() % 100000000
	phone := fmt.Sprintf("138%08d", suffix)
	step := 1

	// 1. 发验证码。
	codeResp, err := client.SendCode(ctx, &passengerv1.SendCodeRequest{Phone: phone})
	if err != nil {
		fail("SendCode: %v", err)
	}
	code := codeResp.GetDebugCode()
	fmt.Printf("[1] SendCode ... OK code=%s\n", code)
	step++

	// 2. 验证码登录。
	loginResp, err := client.Login(ctx, &passengerv1.LoginRequest{
		Channel: "sms", Phone: phone, Code: code, Role: "passenger",
	})
	if err != nil {
		fail("Login: %v", err)
	}
	accountID := loginResp.GetAccountId()
	fmt.Printf("[2] Login ... OK accountId=%s isNew=%v tokenLen=%d\n",
		accountID, loginResp.GetIsNewUser(), len(loginResp.GetAccessToken()))
	step++

	// 3. 游客看价。
	quoteResp, err := client.QuotePrice(ctx, &passengerv1.EstimateRequest{
		CarType: 1, StartLng: 121.601, StartLat: 31.2054, EndLng: 121.3208, EndLat: 31.1979,
	})
	if err != nil {
		fail("QuotePrice: %v", err)
	}
	if quoteResp.GetEstimateNo() != "" {
		fail("QuotePrice returned an estimate number for a visitor: %s", quoteResp.GetEstimateNo())
	}
	fmt.Printf("[3] QuotePrice(visitor) ... OK payFee=%d estimateNo=<empty as expected>\n", quoteResp.GetPayFee())
	step++

	// 4. 登录用户估价。
	estResp, err := client.Estimate(ctx, &passengerv1.EstimateRequest{
		AccountId: accountID, CarType: 1,
		StartLng: 121.601, StartLat: 31.2054, EndLng: 121.3208, EndLat: 31.1979,
	})
	if err != nil {
		fail("Estimate: %v", err)
	}
	fmt.Printf("[4] Estimate ... OK estimateNo=%s payFee=%d\n", estResp.GetEstimateNo(), estResp.GetPayFee())
	step++

	// 5. 首页。
	homeResp, err := client.Home(ctx, &passengerv1.HomeRequest{
		AccountId: accountID, Lng: 121.601, Lat: 31.2054, HasLocation: true,
	})
	if err != nil {
		fail("Home: %v", err)
	}
	fmt.Printf("[5] Home ... OK carTypes=%d quickEntries=%d restoreTarget=%s\n",
		len(homeResp.GetCarTypes()), len(homeResp.GetQuickEntries()), homeResp.GetRestoreTarget())
	step++

	// 6. 地点搜索。
	searchResp, err := client.SearchPOI(ctx, &passengerv1.SearchPOIRequest{
		AccountId: accountID, Keyword: "张江", HasCoord: true, Lng: 121.601, Lat: 31.2054, Limit: 10,
	})
	if err != nil {
		fail("SearchPOI: %v", err)
	}
	fmt.Printf("[6] SearchPOI ... OK results=%d\n", len(searchResp.GetList()))
	step++

	// 7. 保存常用地址。
	saveResp, err := client.SaveAddress(ctx, &passengerv1.SaveAddressRequest{
		AccountId: accountID, Type: "home", Tag: "家", Name: "家", Addr: "上海市浦东新区科苑路 88 号",
		Lng: 121.601, Lat: 31.2054,
	})
	if err != nil {
		fail("SaveAddress: %v", err)
	}
	addressID := saveResp.GetAddress().GetId()
	fmt.Printf("[7] SaveAddress ... OK addressId=%s\n", addressID)
	step++

	// 8. 地址列表。
	listResp, err := client.ListAddresses(ctx, &passengerv1.ListAddressesRequest{
		AccountId: accountID, Type: "home",
	})
	if err != nil {
		fail("ListAddresses: %v", err)
	}
	fmt.Printf("[8] ListAddresses ... OK count=%d\n", len(listResp.GetList()))
	step++

	// 9. 上报历史地址。
	if _, err := client.RecordHistory(ctx, &passengerv1.RecordHistoryRequest{
		AccountId: accountID, PoiId: "poi_1", Name: "张江镇科苑路", Addr: "科苑路 88 号",
		Lng: 121.601, Lat: 31.2054,
	}); err != nil {
		fail("RecordHistory: %v", err)
	}
	fmt.Printf("[9] RecordHistory ... OK\n")
	step++

	// 10. 标记快速入口已用过。
	if _, err := client.UseQuickEntry(ctx, &passengerv1.UseQuickEntryRequest{
		AccountId: accountID, EntryId: "scan_ride",
	}); err != nil {
		fail("UseQuickEntry: %v", err)
	}
	homeAfter, err := client.Home(ctx, &passengerv1.HomeRequest{AccountId: accountID, HasLocation: true})
	if err != nil {
		fail("Home after use: %v", err)
	}
	fmt.Printf("[10] UseQuickEntry ... OK quickEntries %d -> %d\n",
		len(homeResp.GetQuickEntries()), len(homeAfter.GetQuickEntries()))
	step++

	// 11. 绑定手机号（返回掩码）。
	bindResp, err := client.BindPhone(ctx, &passengerv1.BindPhoneRequest{
		AccountId: accountID, Phone: fmt.Sprintf("139%08d", suffix+1),
	})
	if err != nil {
		fail("BindPhone: %v", err)
	}
	fmt.Printf("[11] BindPhone ... OK phone=%s (masked)\n", bindResp.GetPhone())
	step++

	// 12. 设置密码。
	codeResp2, err := client.SendCode(ctx, &passengerv1.SendCodeRequest{Phone: fmt.Sprintf("139%08d", suffix+1)})
	if err != nil {
		fail("SendCode for password: %v", err)
	}
	if _, err := client.SetPassword(ctx, &passengerv1.SetPasswordRequest{
		AccountId: accountID, Phone: fmt.Sprintf("139%08d", suffix+1), Code: codeResp2.GetDebugCode(), Password: "abc123",
	}); err != nil {
		fail("SetPassword: %v", err)
	}
	fmt.Printf("[12] SetPassword ... OK\n")
	step++

	// 13. 用密码登录。
	pwdResp, err := client.Login(ctx, &passengerv1.LoginRequest{
		Channel: "password", Phone: fmt.Sprintf("139%08d", suffix+1), Password: "abc123", Role: "passenger",
	})
	if err != nil {
		fail("password Login: %v", err)
	}
	fmt.Printf("[13] Login(password) ... OK accountId=%s hasPassword=%v\n",
		pwdResp.GetAccountId(), pwdResp.GetHasPassword())
	step++

	// 14. 微信渠道登录。
	wxResp, err := client.Login(ctx, &passengerv1.LoginRequest{
		Channel: "wechat", Code: "wx-code-1", Role: "passenger",
	})
	if err != nil {
		fail("wechat Login: %v", err)
	}
	fmt.Printf("[14] Login(wechat) ... OK accountId=%s\n", wxResp.GetAccountId())
	step++

	// 15. QQ 渠道登录。
	qqResp, err := client.Login(ctx, &passengerv1.LoginRequest{
		Channel: "qq", Code: "qq-code-1", Role: "passenger",
	})
	if err != nil {
		fail("qq Login: %v", err)
	}
	fmt.Printf("[15] Login(qq) ... OK accountId=%s\n", qqResp.GetAccountId())
	step++

	// 16. 游客下单必须被拒（需要账号）。
	if _, err := client.CreateOrder(ctx, &passengerv1.CreateOrderRequest{
		AccountId: "", EstimateNo: "EST-NOT-EXIST", CarType: 1,
		StartLng: 121.601, StartLat: 31.2054, EndLng: 121.3208, EndLat: 31.1979,
		IdempotencyKey: "key-guest",
	}); err == nil {
		fail("CreateOrder for a visitor was accepted, want it rejected")
	}
	fmt.Printf("[16] CreateOrder(visitor) ... OK rejected as expected\n")
	step++

	// 17. 实名未通过不能下单。
	if _, err := client.CreateOrder(ctx, &passengerv1.CreateOrderRequest{
		AccountId: accountID, EstimateNo: estResp.GetEstimateNo(), CarType: 1,
		StartLng: 121.601, StartLat: 31.2054, EndLng: 121.3208, EndLat: 31.1979,
		IdempotencyKey: "key-unverified",
	}); err == nil {
		fail("CreateOrder for an unverified account was accepted, want it rejected")
	}
	fmt.Printf("[17] CreateOrder(unverified) ... OK rejected as expected\n")
	step++

	// 18. 实名置位后下单成功。
	if *mongoURI != "" {
		markRealNameVerified(ctx, *mongoURI, *mongoUser, *mongoPassword, *mongoDB, accountID)
	}
	orderResp, err := client.CreateOrder(ctx, &passengerv1.CreateOrderRequest{
		AccountId: accountID, EstimateNo: estResp.GetEstimateNo(), CarType: 1,
		StartName: "张江镇科苑路", StartAddr: "上海市浦东新区科苑路 88 号",
		StartLng: 121.601, StartLat: 31.2054,
		EndName: "虹桥火车站", EndAddr: "上海市闵行区申贵路 1500 号",
		EndLng: 121.3208, EndLat: 31.1979,
		IdempotencyKey: "key-order-1",
	})
	if err != nil {
		fail("CreateOrder: %v", err)
	}
	orderID := orderResp.GetOrder().GetOrderId()
	fmt.Printf("[18] CreateOrder ... OK orderId=%s orderNo=%s state=%s totalFee=%d\n",
		orderID, orderResp.GetOrder().GetOrderNo(), orderResp.GetOrder().GetState(), orderResp.GetOrder().GetTotalFee())
	step++

	// 19. 幂等：相同键重试返回同一订单。
	again, err := client.CreateOrder(ctx, &passengerv1.CreateOrderRequest{
		AccountId: accountID, EstimateNo: estResp.GetEstimateNo(), CarType: 1,
		StartLng: 121.601, StartLat: 31.2054, EndLng: 121.3208, EndLat: 31.1979,
		IdempotencyKey: "key-order-1",
	})
	if err != nil {
		fail("second CreateOrder: %v", err)
	}
	if again.GetOrder().GetOrderId() != orderID {
		fail("idempotency broken: %s != %s", again.GetOrder().GetOrderId(), orderID)
	}
	fmt.Printf("[19] CreateOrder(idempotent) ... OK same order %s\n", orderID)
	step++

	// 20. 估价单复用被拒。
	if _, err := client.CreateOrder(ctx, &passengerv1.CreateOrderRequest{
		AccountId: accountID, EstimateNo: estResp.GetEstimateNo(), CarType: 1,
		StartLng: 121.601, StartLat: 31.2054, EndLng: 121.3208, EndLat: 31.1979,
		IdempotencyKey: "key-order-2",
	}); err == nil {
		fail("a reused estimate was accepted, want it rejected")
	}
	fmt.Printf("[20] CreateOrder(reused estimate) ... OK rejected as expected\n")
	step++

	// 21. 当前订单查询。
	currentResp, err := client.CurrentOrder(ctx, &passengerv1.CurrentOrderRequest{AccountId: accountID})
	if err != nil {
		fail("CurrentOrder: %v", err)
	}
	if currentResp.GetOrder().GetOrderId() != orderID {
		fail("CurrentOrder returned %s, want %s", currentResp.GetOrder().GetOrderId(), orderID)
	}
	fmt.Printf("[21] CurrentOrder ... OK orderNo=%s state=%s\n",
		currentResp.GetOrder().GetOrderNo(), currentResp.GetOrder().GetState())
	step++

	// 22. 订单列表。
	listOrdersResp, err := client.ListOrders(ctx, &passengerv1.ListOrdersRequest{
		AccountId: accountID, PageNo: 1, PageSize: 20,
	})
	if err != nil {
		fail("ListOrders: %v", err)
	}
	fmt.Printf("[22] ListOrders ... OK total=%d size=%d\n",
		listOrdersResp.GetTotal(), len(listOrdersResp.GetList()))
	step++

	// 23. 待接单取消免费，一次调用就取消。
	cancelResp, err := client.CancelOrder(ctx, &passengerv1.CancelOrderRequest{
		AccountId: accountID, OrderId: orderID, Reason: "临时有事",
	})
	if err != nil {
		fail("CancelOrder: %v", err)
	}
	if cancelResp.GetNeedConfirm() {
		fail("a free cancel while pending asked for confirmation")
	}
	fmt.Printf("[23] CancelOrder(pending) ... OK state=%s cancelFee=%d\n",
		cancelResp.GetState(), cancelResp.GetCancelFee())
	step++

	// 24. 已取消的订单不能再次取消。
	if _, err := client.CancelOrder(ctx, &passengerv1.CancelOrderRequest{
		AccountId: accountID, OrderId: orderID, Reason: "再取消一次", ConfirmFee: true,
	}); err == nil {
		fail("a cancelled order was cancelled again, want it rejected")
	}
	fmt.Printf("[24] CancelOrder(already cancelled) ... OK rejected as expected\n")
	step++

	// 25. 退出登录撤销刷新令牌。
	if _, err := client.Logout(ctx, &passengerv1.LogoutRequest{AccountId: accountID}); err != nil {
		fail("Logout: %v", err)
	}
	fmt.Printf("[25] Logout ... OK\n")
	step++

	fmt.Println("\n全部链路验证通过")
}

// markRealNameVerified 把测试账号的实名状态置为通过。
//
// 实名认证属于「乘客端-我的」模块，不在本端范围内。
// 联调时直接改库是临时手段，正式流程应由实名域写回。
func markRealNameVerified(ctx context.Context, uri, user, password, database, accountID string) {
	cfg := mongox.DefaultConfig()
	cfg.URI = uri
	cfg.Username = user
	cfg.Password = password
	cfg.Database = database
	cfg.AppName = "passengertest"

	_, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	client, err := mongox.NewClient(cfg)
	if err != nil {
		fail("connect mongodb: %v", err)
	}
	defer func() {
		_ = client.Disconnect(context.Background())
	}()
	db := mongox.Database(client, database)
	result, err := db.Collection("passenger_accounts").UpdateOne(ctx,
		bson.M{"_id": accountID},
		bson.M{"$set": bson.M{"real_name_verified": true}},
	)
	if err != nil {
		fail("mark real name verified: %v", err)
	}
	if result.MatchedCount == 0 {
		fail("account %s not found in mongo", accountID)
	}
	fmt.Printf("[--] real name marked verified for %s\n", accountID)
}

// fail 打印错误并退出。
func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "FAIL: "+format+"\n", args...)
	os.Exit(1)
}
