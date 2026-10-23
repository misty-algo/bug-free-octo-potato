// Command passenger 启动乘客端业务域的 gRPC 服务。
package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc"

	passengerv1 "github.com/BwCloudWeGo/bw-cli/api/gen/passenger/v1"
	passengerhandler "github.com/BwCloudWeGo/bw-cli/internal/passenger/handler"
	passengerrepo "github.com/BwCloudWeGo/bw-cli/internal/passenger/repo"
	passengerservice "github.com/BwCloudWeGo/bw-cli/internal/passenger/service"
	passengertoken "github.com/BwCloudWeGo/bw-cli/internal/passenger/service/token"
	"github.com/BwCloudWeGo/bw-cli/pkg/config"
	"github.com/BwCloudWeGo/bw-cli/pkg/grpcx"
	"github.com/BwCloudWeGo/bw-cli/pkg/logger"
	"github.com/BwCloudWeGo/bw-cli/pkg/mongox"
)

func main() {
	// 加载服务身份、数据库和日志配置。
	configPath := flag.String("config", "configs/config.yaml", "配置文件路径")
	flag.Parse()
	if err := config.InitGlobal(*configPath); err != nil {
		panic(err)
	}
	cfg := config.MustGlobal()
	cfg.Log.Service = cfg.ServiceName("passenger")
	cfg.Log = logger.WithDailyFileName(cfg.Log, time.Now())

	// MongoDB 的连接串、用户名与密码都是环境相关的真值，MUST NOT 写进配置文件。
	// 三者都支持环境变量覆盖，配置里的值只作为本地默认。
	mongoCfg := cfg.MongoDB.MongoxConfig()
	if value := strings.TrimSpace(os.Getenv("MONGODB_URI")); value != "" {
		mongoCfg.URI = value
	}
	if value := strings.TrimSpace(os.Getenv("MONGODB_USERNAME")); value != "" {
		mongoCfg.Username = value
	}
	if value := strings.TrimSpace(os.Getenv("MONGODB_PASSWORD")); value != "" {
		mongoCfg.Password = value
	}
	if value := strings.TrimSpace(os.Getenv("MONGODB_DATABASE")); value != "" {
		mongoCfg.Database = value
	}

	// 手机号加密密钥从环境变量读。MUST NOT 提交真值到仓库，
	// 生产环境应从配置中心或密钥管理服务注入。
	// 缺失时直接退出而不是用兜底值：兜底密钥等于没有加密。
	// AES-256-GCM 要求密钥长度为 16、24 或 32 字节。
	phoneKey := strings.TrimSpace(os.Getenv("PASSENGER_PHONE_KEY"))
	if phoneKey == "" {
		panic("environment variable PASSENGER_PHONE_KEY is required (16, 24 or 32 bytes)")
	}
	if err := passengerrepo.SetPhoneKey(phoneKey); err != nil {
		panic(fmt.Errorf("configure phone encryption key: %w", err))
	}

	log, err := logger.New(cfg.Log)
	if err != nil {
		panic(err)
	}
	defer log.Sync()
	config.PrintSourceNotice(cfg, os.Stdout)

	// 乘客端数据落在 MongoDB。
	client, err := mongox.NewClient(mongoCfg)
	if err != nil {
		log.Fatal("open mongodb client failed", zap.Error(err))
	}
	defer func() {
		_ = client.Disconnect(context.Background())
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := mongox.Ping(ctx, client); err != nil {
		log.Fatal("ping mongodb failed", zap.Error(err))
	}
	db := mongox.Database(client, cfg.MongoDB.Database)

	store := passengerrepo.NewStore(db)
	svc := passengerservice.NewService(
		store.Accounts(),
		store.Channels(),
		store.Sessions(),
		store.Codes(),
		store.Estimates(),
		store.Addresses(),
		passengerrepo.NewLocalPOIRepository(),
		store.Orders(),
		store.QuickEntries(),
		passengertoken.NewIssuer(passengerConfig().TokenSecret),
		passengerrepo.NewBcryptHasher(),
		passengerrepo.NewLocalCodeSender(),
		passengerrepo.NewLocalChannelAuthenticator(),
		passengerrepo.NewIDGenerator(),
	)
	svc.WithCouponRepository(passengerrepo.NewCouponRepository(db))
	svc.WithDriverRepository(passengerrepo.NewDriverRepository(db))
	svc.WithCancelFee(passengerConfig().CancelFeeAfterAccepted, passengerConfig().CancelFeeAfterArrived)

	server := grpc.NewServer(grpc.UnaryInterceptor(grpcx.UnaryServerInterceptor(log)))
	passengerv1.RegisterPassengerServiceServer(server, passengerhandler.NewServer(svc, log))

	addr := fmt.Sprintf("%s:%d", cfg.GRPC.Host, cfg.ServicePort("passenger", 9005))
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatal("listen failed", zap.String("addr", addr), zap.Error(err))
	}

	go shutdownOnSignal(server, log)
	log.Info("passenger service listening", zap.String("addr", addr))
	if err := server.Serve(listener); err != nil {
		log.Fatal("passenger service stopped unexpectedly", zap.Error(err))
	}
}

// shutdownOnSignal 收到中断信号时优雅停机。
func shutdownOnSignal(server *grpc.Server, log *zap.Logger) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	<-ch
	log.Info("passenger service shutting down")
	server.GracefulStop()
}

// PassengerConfig 是乘客端专属配置。
type PassengerConfig struct {
	// TokenSecret 用于签发乘客端令牌，生产必须换成配置中心或环境变量里的真值。
	TokenSecret string
	// CancelFeeAfterAccepted 为已接单取消的取消费，单位分。
	CancelFeeAfterAccepted int64
	// CancelFeeAfterArrived 为已到达取消的取消费，单位分。
	CancelFeeAfterArrived int64
}

// passengerConfig 从环境变量读取乘客端配置。
//
// 密钥类配置 MUST NOT 提交真值到仓库，只提交 .env.example 占位。
func passengerConfig() PassengerConfig {
	return PassengerConfig{
		TokenSecret:           envOr("PASSENGER_TOKEN_SECRET", "local-dev-secret-change-me"),
		CancelFeeAfterAccepted: envInt64("PASSENGER_CANCEL_FEE_ACCEPTED", 500),
		CancelFeeAfterArrived:  envInt64("PASSENGER_CANCEL_FEE_ARRIVED", 1000),
	}
}

// envOr 读取环境变量，未配置时返回兜底值。
func envOr(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

// envInt64 读取整数型环境变量，未配置或非法时返回兜底值。
func envInt64(key string, fallback int64) int64 {
	value, err := strconv.ParseInt(strings.TrimSpace(os.Getenv(key)), 10, 64)
	if err != nil {
		return fallback
	}
	return value
}
