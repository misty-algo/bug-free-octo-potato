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
	"github.com/BwCloudWeGo/bw-cli/pkg/database"
	"github.com/BwCloudWeGo/bw-cli/pkg/grpcx"
	"github.com/BwCloudWeGo/bw-cli/pkg/logger"
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

	db, err := database.Open(cfg.Database, cfg.MySQL, cfg.PostgreSQL, log)
	if err != nil {
		log.Fatal("open database failed", zap.Error(err))
	}

	if err := driverrepo.AutoMigrate(db); err != nil {
		log.Fatal("migrate driver database failed", zap.Error(err))
	}

	// 连接本地 RabbitMQ，订阅 order 服务发布的下单事件。
	mqCfg := cfg.RabbitMQ.RabbitmqxConfig()
	mqClient, err := rabbitmqx.New(mqCfg)
	if err != nil {
		log.Fatal("connect rabbitmq failed", zap.Error(err))
	}
	defer mqClient.Close()

	go consumeOrderEvents(mqClient, mqCfg.Queue, mqCfg.RoutingKeys, log)

	repo := driverrepo.NewGormRepository(db, log)
	svc := driverservice.NewService(repo, log)
	server := grpc.NewServer(grpc.UnaryInterceptor(grpcx.UnaryServerInterceptor(log)))
	driverv1.RegisterDriverServiceServer(server, driverhandler.NewServer(svc, log))

	port := cfg.ServicePort("driver", defaultGRPCPort)
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
	fmt.Fprintf(os.Stdout, "  config: services.driver.port\n\n")
}

func shutdownOnSignal(server *grpc.Server, log *zap.Logger) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	<-ch
	log.Info("service shutting down", zap.String("service", serviceName))
	server.GracefulStop()
}

// consumeOrderEvents 消费下单事件；连接断开后 rabbitmqx 会自动重连并恢复订阅。
func consumeOrderEvents(client *rabbitmqx.Client, queue string, routingKeys []string, log *zap.Logger) {
	handler := func(_ context.Context, body []byte) error {
		log.Info("order created event received", zap.String("queue", queue), zap.ByteString("payload", body))
		return nil
	}
	log.Info("driver consuming order events", zap.String("queue", queue))
	if err := client.Consume(context.Background(), queue, routingKeys, handler); err != nil {
		log.Error("order event consumer stopped", zap.Error(err))
	}
}
