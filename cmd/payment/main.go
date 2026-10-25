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

	paymentv1 "github.com/BwCloudWeGo/bw-cli/api/gen/payment/v1"
	paymenthandler "github.com/BwCloudWeGo/bw-cli/internal/payment/handler"
	paymentrepo "github.com/BwCloudWeGo/bw-cli/internal/payment/repo"
	paymentservice "github.com/BwCloudWeGo/bw-cli/internal/payment/service"
	"github.com/BwCloudWeGo/bw-cli/pkg/config"
	"github.com/BwCloudWeGo/bw-cli/pkg/database"
	"github.com/BwCloudWeGo/bw-cli/pkg/grpcx"
	"github.com/BwCloudWeGo/bw-cli/pkg/logger"
	"github.com/BwCloudWeGo/bw-cli/pkg/redisx"
	"github.com/BwCloudWeGo/bw-cli/pkg/rocketmqx"
)

const serviceName = "payment-service"
const defaultGRPCPort = 9102

func main() {
	if err := config.InitGlobal("configs/config.yaml"); err != nil {
		panic(err)
	}
	cfg := config.MustGlobal()
	cfg.Log.Service = cfg.ServiceName("payment")
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

	if err := paymentrepo.AutoMigrate(db); err != nil {
		log.Fatal("migrate payment database failed", zap.Error(err))
	}

	repo := paymentrepo.NewGormRepository(db, log)
	svc := paymentservice.NewService(repo, log)
	initEventPublisher(svc, cfg, log)
	initLocker(svc, cfg, log)
	go startEventCompensator(svc, log)
	server := grpc.NewServer(grpc.UnaryInterceptor(grpcx.UnaryServerInterceptor(log)))
	paymentv1.RegisterPaymentServiceServer(server, paymenthandler.NewServer(svc, log))

	port := cfg.ServicePort("payment", defaultGRPCPort)
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

// initEventPublisher 初始化 RocketMQ 生产者并注入 service。
// RocketMQ 不可用时不阻塞启动：支付成功事件回退到 payment_events 待处理表。
func initEventPublisher(svc *paymentservice.Service, cfg *config.Config, log *zap.Logger) {
	producer, err := rocketmqx.NewSimpleProducer(cfg.RocketMQ)
	if err != nil {
		log.Warn("rocketmq producer init failed, pay success events will fallback to payment_events table",
			zap.String("service", serviceName), zap.Error(err))
		return
	}
	svc.SetEventPublisher(producer)
	log.Info("rocketmq producer initialized", zap.String("service", serviceName))
}

// initLocker 初始化 Redis 分布式锁（NotifyPayment 并发防重）。
func initLocker(svc *paymentservice.Service, cfg *config.Config, log *zap.Logger) {
	if cfg.Redis.Addr == "" {
		log.Warn("redis not configured, notify lock disabled", zap.String("service", serviceName))
		return
	}
	client := redisx.NewClient(cfg.Redis)
	locker := redisx.NewLocker(client, redisx.LockConfig{})
	svc.SetLocker(locker)
	log.Info("redis locker initialized", zap.String("service", serviceName))
}

// startEventCompensator 定时扫描 payment_events 待处理事件并重发（P1：MQ 失败兜底补偿）。
func startEventCompensator(svc *paymentservice.Service, log *zap.Logger) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		processed, err := svc.ProcessPendingEvents(ctx, 100)
		cancel()
		if err != nil {
			log.Warn("payment event compensator scan failed", zap.Error(err))
		} else if processed > 0 {
			log.Info("payment event compensator processed", zap.Int("count", processed))
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
	fmt.Fprintf(os.Stdout, "  config: services.payment.port\n\n")
}

func shutdownOnSignal(server *grpc.Server, log *zap.Logger) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	<-ch
	log.Info("service shutting down", zap.String("service", serviceName))
	server.GracefulStop()
}
