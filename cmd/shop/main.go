package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	settlementv1 "github.com/BwCloudWeGo/bw-cli/api/gen/settlement/v1"
	shopv1 "github.com/BwCloudWeGo/bw-cli/api/gen/shop/v1"
	shophandler "github.com/BwCloudWeGo/bw-cli/internal/shop/handler"
	shoprepo "github.com/BwCloudWeGo/bw-cli/internal/shop/repo"
	shopservice "github.com/BwCloudWeGo/bw-cli/internal/shop/service"
	"github.com/BwCloudWeGo/bw-cli/pkg/config"
	"github.com/BwCloudWeGo/bw-cli/pkg/database"
	"github.com/BwCloudWeGo/bw-cli/pkg/gopay"
	"github.com/BwCloudWeGo/bw-cli/pkg/grpcx"
	"github.com/BwCloudWeGo/bw-cli/pkg/logger"
	"github.com/BwCloudWeGo/bw-cli/pkg/mongox"
	"github.com/BwCloudWeGo/bw-cli/pkg/wechatpayx"
)

const serviceName = "shop-service"
const defaultGRPCPort = 9101

// 掉单补偿参数：扫描间隔、pending 超时阈值、单次补偿超时。
const (
	paymentCompensateInterval = 60 * time.Second
	paymentPendingTimeout     = 5 * time.Minute
	paymentCompensateTimeout  = 30 * time.Second
)

func main() {
	if err := config.InitGlobal("configs/config.yaml"); err != nil {
		panic(err)
	}
	cfg := config.MustGlobal()
	cfg.Log.Service = cfg.ServiceName("shop")
	cfg.Log = logger.WithDailyFileName(cfg.Log, time.Now())

	log, err := logger.New(cfg.Log)
	if err != nil {
		panic(err)
	}
	defer log.Sync()
	config.PrintSourceNotice(cfg, os.Stdout)

	db, err := database.Open(cfg.Database, cfg.MySQL, cfg.PostgreSQL, log)
	if err != nil {
		log.Fatal("open database failed", zap.Error(err))
	}

	if err := shoprepo.AutoMigrate(db); err != nil {
		log.Fatal("migrate shop database failed", zap.Error(err))
	}

	repo := shoprepo.NewGormRepository(db, log)
	svc := shopservice.NewService(repo, log)
	svc.SetPaymentRepository(shoprepo.NewGormPaymentRepository(db, log))
	initSettlementClient(svc, cfg, log)
	initMongoRepositories(svc, cfg, log)
	initAlipayClient(svc, cfg, log)
	initWechatClient(svc, cfg, log)
	go startPaymentCompensation(svc, log)
	server := grpc.NewServer(grpc.UnaryInterceptor(grpcx.UnaryServerInterceptor(log)))
	shopv1.RegisterShopServiceServer(server, shophandler.NewServer(svc, log))

	port := cfg.ServicePort("shop", defaultGRPCPort)
	addr := fmt.Sprintf("%s:%d", cfg.GRPC.Host, port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\n[Service Start Failed]\n  service: %s\n  listen: %s\n  error: %v\n\n", serviceName, addr, err)
		log.Fatal("listen failed", zap.String("addr", addr), zap.Error(err))
	}

	printStartupSummary(cfg.App.Env, addr, port)
	go shutdownOnSignal(server, log)
	if err := server.Serve(listener); err != nil {
		log.Fatal("service stopped unexpectedly", zap.Error(err))
	}
}

// initMongoRepositories 初始化 P08–P12 共享 MongoDB 仓储并注入 service。
// 连接失败时以致命错误退出，保证依赖 Mongo 的用例不会在 nil 仓储上运行。
func initMongoRepositories(svc *shopservice.Service, cfg *config.Config, log *zap.Logger) {
	client, err := mongox.NewClient(cfg.MongoDB.MongoxConfig())
	if err != nil {
		log.Fatal("init mongodb client failed", zap.Error(err))
	}
	if err := mongox.Ping(context.Background(), client); err != nil {
		log.Fatal("ping mongodb failed", zap.Error(err))
	}
	mongoDB := mongox.Database(client, cfg.MongoDB.Database)
	svc.SetTrajectoryRepository(shoprepo.NewMongoTrajectoryRepository(mongoDB, log))
	svc.SetVirtualNumberRepository(shoprepo.NewMongoVirtualNumberRepository(mongoDB, log))
	svc.SetDestinationChangeRepository(shoprepo.NewMongoDestinationChangeRepository(mongoDB, log))
	svc.SetTripShareRepository(shoprepo.NewMongoTripShareRepository(mongoDB, log))
	svc.SetCancelQuoteRepository(shoprepo.NewMongoCancelQuoteRepository(mongoDB, log))
	log.Info("mongodb repositories initialized", zap.String("service", serviceName),
		zap.String("database", cfg.MongoDB.Database))
}

// initSettlementClient 注入结算服务客户端：退款成功后异步触发司机入账冲正。
// settlement 未配置 target 时跳过注入（退款仍正常完成，冲正由事件消费端兜底）。
func initSettlementClient(svc *shopservice.Service, cfg *config.Config, log *zap.Logger) {
	target := cfg.ServiceTarget("settlement")
	if target == "" {
		log.Warn("settlement target not configured, skip settlement client injection")
		return
	}
	conn, err := grpc.NewClient(target,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(grpcx.UnaryClientInterceptor(fmt.Sprintf("shop-%d", time.Now().Unix()))),
	)
	if err != nil {
		log.Fatal("dial settlement service failed", zap.String("target", target), zap.Error(err))
	}
	svc.SetSettlementClient(settlementv1.NewSettlementServiceClient(conn))
	log.Info("settlement client initialized", zap.String("target", target))
}

// initAlipayClient 读取 alipay 配置并初始化 gopay client 注入 service。
// 配置为空时以 nil client 启动并打警告日志，保证本地未配密钥时 CRUD 不受影响。
func initAlipayClient(svc *shopservice.Service, cfg *config.Config, log *zap.Logger) {
	if cfg.Alipay.AppID == "" || cfg.Alipay.PrivateKey == "" {
		log.Warn("alipay not configured, payment APIs will return alipay not configured",
			zap.String("service", serviceName))
		return
	}
	client, err := gopay.InitFromAlipay(&cfg.Alipay)
	if err != nil {
		log.Warn("init alipay client failed, payment APIs will return alipay not configured",
			zap.String("service", serviceName), zap.Error(err))
		return
	}
	svc.SetAlipayClient(client)
	log.Info("alipay client initialized", zap.String("service", serviceName))
}

// initWechatClient 读取 wechat_pay 配置并初始化 wechatpayx client 注入 service。
// 配置为空时以 nil client 启动并打警告日志，保证本地未配密钥时 CRUD 不受影响。
func initWechatClient(svc *shopservice.Service, cfg *config.Config, log *zap.Logger) {
	if cfg.WechatPay.MchID == "" || cfg.WechatPay.APIv3Key == "" || cfg.WechatPay.PrivateKey == "" {
		log.Warn("wechat pay not configured, wechat channel APIs will return wechat not configured",
			zap.String("service", serviceName))
		return
	}
	client, err := wechatpayx.Init(cfg.WechatPay)
	if err != nil {
		log.Warn("init wechat client failed, wechat channel APIs will return wechat not configured",
			zap.String("service", serviceName), zap.Error(err))
		return
	}
	svc.SetWechatClient(client)
	log.Info("wechat client initialized", zap.String("service", serviceName))
}

// startPaymentCompensation 后台补偿 goroutine：周期扫描超过阈值的 pending 支付单，
// 逐个调用 service 查单方法刷新状态（成功→success，失败保持 pending），实现掉单补偿闭环。
func startPaymentCompensation(svc *shopservice.Service, log *zap.Logger) {
	ticker := time.NewTicker(paymentCompensateInterval)
	defer ticker.Stop()
	for range ticker.C {
		ctx, cancel := context.WithTimeout(context.Background(), paymentCompensateTimeout)
		checked, err := svc.CompensatePending(ctx, paymentPendingTimeout)
		cancel()
		if err != nil {
			log.Warn("payment compensation round failed", zap.String("service", serviceName), zap.Error(err))
			continue
		}
		if checked > 0 {
			log.Info("payment compensation round done", zap.String("service", serviceName), zap.Int("checked", checked))
		}
	}
}

func printStartupSummary(env string, addr string, port int) {
	host := strings.Split(addr, ":")[0]
	if host == "" || host == "0.0.0.0" {
		host = "127.0.0.1"
	}
	fmt.Fprintf(os.Stdout, "\n[Service Started]\n")
	fmt.Fprintf(os.Stdout, "  service: %s\n", serviceName)
	fmt.Fprintf(os.Stdout, "  env: %s\n", env)
	fmt.Fprintf(os.Stdout, "  listen: %s\n", addr)
	fmt.Fprintf(os.Stdout, "  grpc: %s:%d\n", host, port)
	fmt.Fprintf(os.Stdout, "  config: services.shop.port\n\n")
}

func shutdownOnSignal(server *grpc.Server, log *zap.Logger) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	<-ch
	log.Info("service shutting down", zap.String("service", serviceName))
	server.GracefulStop()
}
