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

	driverv1 "github.com/BwCloudWeGo/bw-cli/api/gen/driver/v1"
	driverhandler "github.com/BwCloudWeGo/bw-cli/internal/driver/handler"
	driverrepo "github.com/BwCloudWeGo/bw-cli/internal/driver/repo"
	driverservice "github.com/BwCloudWeGo/bw-cli/internal/driver/service"
	"github.com/BwCloudWeGo/bw-cli/pkg/config"
	"github.com/BwCloudWeGo/bw-cli/pkg/grpcx"
	"github.com/BwCloudWeGo/bw-cli/pkg/logger"
	"github.com/BwCloudWeGo/bw-cli/pkg/mongox"
	"github.com/BwCloudWeGo/bw-cli/pkg/rabbitmqx"
)

const serviceName = "driver-service"
const defaultGRPCPort = 9101

func main() {
	if err := config.InitGlobal("configs/config.yaml"); err != nil {
		panic(err)
	}
	cfg := config.MustGlobal()
	cfg.Log.Service = cfg.ServiceName("driver")
	cfg.Log = logger.WithDailyFileName(cfg.Log, time.Now())

	log, err := logger.New(cfg.Log)
	if err != nil {
		panic(err)
	}
	defer log.Sync()
	config.PrintSourceNotice(cfg, os.Stdout)

	// ── 1. 连接本地 MongoDB（day1 库）──
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	mongoClient, err := mongox.NewClient(cfg.MongoDB.MongoxConfig())
	if err != nil {
		log.Fatal("connect mongodb failed", zap.Error(err))
	}
	defer mongoClient.Disconnect(context.Background())
	if err := mongox.Ping(ctx, mongoClient); err != nil {
		log.Fatal("ping mongodb failed", zap.String("database", cfg.MongoDB.Database), zap.Error(err))
	}
	mongoDB := mongox.Database(mongoClient, cfg.MongoDB.Database)
	log.Info("mongodb connected", zap.String("database", cfg.MongoDB.Database))

	// ── 2. 连接本地 RabbitMQ，订阅 order 服务发布的下单事件 ──
	mqCfg := cfg.RabbitMQ.RabbitmqxConfig()
	mqClient, err := rabbitmqx.New(mqCfg)
	if err != nil {
		log.Fatal("connect rabbitmq failed", zap.Error(err))
	}
	defer mqClient.Close()

	// ── 3. 注入司机端仓储与用例服务 ──
	repos := driverrepo.NewMongoRepositories(mongoDB, log)
	// 建立跨模块一致性所需的唯一约束（幂等键、事件去重、一司机一个活动任务）。
	if err := repos.Support().EnsureIndexes(ctx); err != nil {
		log.Warn("ensure indexes failed", zap.Error(err))
	}
	bizCfg := driverservice.BusinessConfig{
		CommissionRate:        cfg.Driver.CommissionRate,
		MinWithdrawFen:        cfg.Driver.MinWithdrawFen,
		MaxWithdrawFen:        cfg.Driver.MaxWithdrawFen,
		DailyWithdrawLimitFen: cfg.Driver.DailyWithdrawLimitFen,
		DailyWithdrawCount:    cfg.Driver.DailyWithdrawCount,
		DualApprovalFen:       cfg.Driver.DualApprovalFen,
		DailyReleaseLimit:     cfg.Driver.DailyReleaseLimit,
		AcceptTimeoutSec:      cfg.Driver.AcceptTimeoutSeconds,
		ListenWindowSec:       cfg.Driver.ListenWindowSeconds,
		DispatchWindowSec:     cfg.Driver.DispatchWindowSeconds,
		HeartbeatTimeoutSec:   cfg.Driver.HeartbeatTimeoutSeconds,
		HallListLimit:         cfg.Driver.HallListLimit,
		Weights: driverservice.DispatchWeights{
			Distance: cfg.Driver.Weights.Distance,
			Rating:   cfg.Driver.Weights.Rating,
			Online:   cfg.Driver.Weights.Online,
			Success:  cfg.Driver.Weights.Success,
		},
	}
	// 司机端令牌的密钥与签发者必须与网关 JWT 中间件一致，否则网关会拒收。
	// 密钥同样走环境变量注入，签发者沿用同一份配置文件的 middleware.jwt.issuer。
	if value := strings.TrimSpace(os.Getenv("PASSENGER_TOKEN_SECRET")); value != "" {
		cfg.Middleware.JWT.Secret = value
	}
	svc := driverservice.NewService(repos.Drivers(), repos.Orders(), repos.Incomes(), repos.Support(), bizCfg, log).
		WithAuth(repos.Codes(), driverservice.AuthConfig{
			TokenSecret: cfg.Middleware.JWT.Secret,
			TokenIssuer: cfg.Middleware.JWT.Issuer,
		})
	svc.SetEventPublisher(mqClient.Publish)
	// 校准司机编号序列，避免重启后从 D000001 重新开始导致编号重复。
	svc.SyncDriverNoSequence(ctx)

	// ── 4. 消费订单事件：order.created → 任务进入接单大厅 ──
	go consumeOrderEvents(svc, mqClient, mqCfg.Queue, mqCfg.RoutingKeys, log)

	// ── 5. 后台循环：派单调度（1 秒）+ 可靠事件投递（2 秒）──
	go runDispatchLoop(svc, log)
	go runEventPublishLoop(svc, log)

	server := grpc.NewServer(grpc.UnaryInterceptor(grpcx.UnaryServerInterceptor(log)))
	driverv1.RegisterDriverServiceServer(server, driverhandler.NewServer(svc, log))

	port := cfg.ServicePort("driver", defaultGRPCPort)
	addr := fmt.Sprintf("%s:%d", cfg.GRPC.Host, port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\n[Service Start Failed]\n  service: %s\n  listen: %s\n  error: %v\n\n", serviceName, addr, err)
		log.Fatal("listen failed", zap.String("addr", addr), zap.Error(err))
	}

	printStartupSummary(cfg, addr, port)
	go shutdownOnSignal(server, log)
	if err := server.Serve(listener); err != nil {
		log.Fatal("service stopped unexpectedly", zap.Error(err))
	}
}

func printStartupSummary(cfg *config.Config, addr string, port int) {
	host := strings.Split(addr, ":")[0]
	if host == "" || host == "0.0.0.0" {
		host = "127.0.0.1"
	}
	fmt.Fprintf(os.Stdout, "\n[Service Started]\n")
	fmt.Fprintf(os.Stdout, "  service: %s\n", serviceName)
	fmt.Fprintf(os.Stdout, "  env: %s\n", cfg.App.Env)
	fmt.Fprintf(os.Stdout, "  listen: %s\n", addr)
	fmt.Fprintf(os.Stdout, "  grpc: %s:%d\n", host, port)
	fmt.Fprintf(os.Stdout, "  mongodb: %s (database: %s)\n", cfg.MongoDB.URI, cfg.MongoDB.Database)
	fmt.Fprintf(os.Stdout, "  rabbitmq: %s (queue: %s)\n\n", cfg.RabbitMQ.URL, cfg.RabbitMQ.Queue)
}

func shutdownOnSignal(server *grpc.Server, log *zap.Logger) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	<-ch
	log.Info("service shutting down", zap.String("service", serviceName))
	server.GracefulStop()
}

// runDispatchLoop 后台派单调度：邀请超时轮换、听单窗口转抢单池、超时取消、心跳收车。
func runDispatchLoop(svc *driverservice.Service, log *zap.Logger) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		res := svc.DispatchTick(context.Background())
		if res.InviteExpired+res.SwitchedToPool+res.CancelExpired+res.HeartbeatOffline > 0 {
			log.Info("dispatch tick",
				zap.Int("invite_expired", res.InviteExpired),
				zap.Int("switched_to_pool", res.SwitchedToPool),
				zap.Int("cancel_expired", res.CancelExpired),
				zap.Int("heartbeat_offline", res.HeartbeatOffline))
		}
	}
}

// runEventPublishLoop 可靠事件投递：把事件表中的待投递事件发往消息队列。
func runEventPublishLoop(svc *driverservice.Service, log *zap.Logger) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		published, err := svc.PublishPendingEvents(context.Background(), 50)
		if err != nil {
			log.Warn("publish pending events failed", zap.Error(err))
			continue
		}
		if published > 0 {
			log.Info("outbox events published", zap.Int("count", published))
		}
	}
}

// consumeOrderEvents 消费下单事件；连接断开后 rabbitmqx 会自动重连并恢复订阅。
func consumeOrderEvents(svc *driverservice.Service, client *rabbitmqx.Client, queue string, routingKeys []string, log *zap.Logger) {
	handler := func(ctx context.Context, body []byte) error {
		return svc.HandleOrderCreated(ctx, body)
	}
	log.Info("driver consuming order events", zap.String("queue", queue), zap.Strings("routing_keys", routingKeys))
	if err := client.Consume(context.Background(), queue, routingKeys, handler); err != nil {
		log.Error("order event consumer stopped", zap.Error(err))
	}
}
