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

	settlementv1 "github.com/BwCloudWeGo/bw-cli/api/gen/settlement/v1"
	settlementhandler "github.com/BwCloudWeGo/bw-cli/internal/settlement/handler"
	settlementrepo "github.com/BwCloudWeGo/bw-cli/internal/settlement/repo"
	settlementservice "github.com/BwCloudWeGo/bw-cli/internal/settlement/service"
	"github.com/BwCloudWeGo/bw-cli/pkg/config"
	"github.com/BwCloudWeGo/bw-cli/pkg/grpcx"
	"github.com/BwCloudWeGo/bw-cli/pkg/logger"
	"github.com/BwCloudWeGo/bw-cli/pkg/mongox"
)

const serviceName = "settlement-service"
const defaultGRPCPort = 9103

func main() {
	if err := config.InitGlobal("configs/config.yaml"); err != nil {
		panic(err)
	}
	cfg := config.MustGlobal()
	cfg.Log.Service = cfg.ServiceName("settlement")
	cfg.Log = logger.WithDailyFileName(cfg.Log, time.Now())

	log, err := logger.New(cfg.Log)
	if err != nil {
		panic(err)
	}
	defer log.Sync()
	config.PrintSourceNotice(cfg, os.Stdout)

	// settlement 使用 MongoDB：账单/流水/钱包均为文档模型，无需 migrate。
	mongoClient, err := mongox.NewClient(cfg.MongoDB.MongoxConfig())
	if err != nil {
		log.Fatal("connect mongodb failed", zap.Error(err))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := mongoClient.Ping(ctx, nil); err != nil {
		log.Warn("mongodb ping failed, service will retry on demand", zap.Error(err))
	}
	mongoDB := mongox.Database(mongoClient, cfg.MongoDB.Database)

	repo := settlementrepo.NewMongoRepository(mongoDB, log)
	svc := settlementservice.NewService(repo, log)
	// 注入 MongoDB session 事务执行器：SplitBill/ApplyRefund 多表写入走同一事务。
	svc.SetTxRunner(repo.WithTransaction)
	server := grpc.NewServer(grpc.UnaryInterceptor(grpcx.UnaryServerInterceptor(log)))
	settlementv1.RegisterSettlementServiceServer(server, settlementhandler.NewServer(svc, log))

	port := cfg.ServicePort("settlement", defaultGRPCPort)
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
	fmt.Fprintf(os.Stdout, "  config: services.settlement.port\n\n")
}

func shutdownOnSignal(server *grpc.Server, log *zap.Logger) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	<-ch
	log.Info("service shutting down", zap.String("service", serviceName))
	server.GracefulStop()
}
