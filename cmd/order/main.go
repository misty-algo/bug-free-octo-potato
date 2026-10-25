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
	orderv1 "github.com/BwCloudWeGo/bw-cli/api/gen/order/v1"
	driverhandler "github.com/BwCloudWeGo/bw-cli/internal/driver/handler"
	driverrepo "github.com/BwCloudWeGo/bw-cli/internal/driver/repo"
	driverservice "github.com/BwCloudWeGo/bw-cli/internal/driver/service"
	orderentity "github.com/BwCloudWeGo/bw-cli/internal/order/entity"
	orderhandler "github.com/BwCloudWeGo/bw-cli/internal/order/handler"
	orderrepo "github.com/BwCloudWeGo/bw-cli/internal/order/repo"
	orderservice "github.com/BwCloudWeGo/bw-cli/internal/order/service"
	"github.com/BwCloudWeGo/bw-cli/pkg/authn"
	"github.com/BwCloudWeGo/bw-cli/pkg/clock"
	"github.com/BwCloudWeGo/bw-cli/pkg/config"
	"github.com/BwCloudWeGo/bw-cli/pkg/database"
	"github.com/BwCloudWeGo/bw-cli/pkg/filex"
	"github.com/BwCloudWeGo/bw-cli/pkg/grpcx"
	"github.com/BwCloudWeGo/bw-cli/pkg/logger"
	"github.com/BwCloudWeGo/bw-cli/pkg/mongox"
	"github.com/BwCloudWeGo/bw-cli/pkg/rabbitmqx"
)

const serviceName = "order-service"
const defaultGRPCPort = 9100

func main() {
	if err := config.InitGlobal("configs/config.yaml"); err != nil {
		panic(err)
	}
	cfg := config.MustGlobal()
	arrivalWrites := strings.EqualFold(os.Getenv("ARRIVAL_ENABLED"), "true")
	arrivalEnabled := arrivalWrites || strings.EqualFold(os.Getenv("ARRIVAL_READ_ENABLED"), "true")
	if arrivalEnabled {
		if secret, exists := os.LookupEnv("ARRIVAL_JWT_SECRET"); exists {
			cfg.Middleware.JWT.Secret = secret
		}
		if issuer, exists := os.LookupEnv("ARRIVAL_JWT_ISSUER"); exists {
			cfg.Middleware.JWT.Issuer = issuer
		}
		if strings.TrimSpace(cfg.Middleware.JWT.Secret) == "" || strings.TrimSpace(cfg.Middleware.JWT.Issuer) == "" {
			panic("arrival requires JWT secret and issuer")
		}
		if uri, exists := os.LookupEnv("ARRIVAL_MONGO_URI"); exists {
			cfg.MongoDB.URI = uri
			cfg.MongoDB.Username = ""
			cfg.MongoDB.Password = ""
		}
		if name, exists := os.LookupEnv("ARRIVAL_MONGO_DATABASE"); exists {
			cfg.MongoDB.Database = name
		}
	}
	cfg.Log.Service = cfg.ServiceName("order")
	cfg.Log = logger.WithDailyFileName(cfg.Log, time.Now())

	log, err := logger.New(cfg.Log)
	if err != nil {
		panic(err)
	}
	defer log.Sync()
	config.PrintSourceNotice(cfg, os.Stdout)

	var legacyRepo orderentity.Repository
	// 材料上传单文件可达 5MB，需放宽默认 4MB 接收上限。
	server := grpc.NewServer(
		grpc.UnaryInterceptor(grpcx.UnaryServerInterceptor(log)),
		grpc.MaxRecvMsgSize(8*1024*1024),
	)
	if arrivalEnabled {
		mongoClient, err := mongox.NewClient(cfg.MongoDB.MongoxConfig())
		if err != nil {
			// Driver connection errors can embed the configured URI.
			log.Fatal("open arrival MongoDB failed; check connection configuration")
		}
		defer func() {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if err := mongoClient.Disconnect(ctx); err != nil {
				log.Error("close arrival MongoDB failed", zap.Error(err))
			}
		}()
		db := mongox.Database(mongoClient, cfg.MongoDB.Database)
		// 启动时只校验；建集合/建索引走独立的 cmd/arrival-schema。
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		err = orderrepo.ValidateMongoArrivalSchema(ctx, db)
		cancel()
		if err != nil {
			log.Fatal("arrival schema validation failed", zap.Error(err))
		}
		legacyRepo = orderrepo.NewMongoRepository(db, log)
		arrivalRepo := orderrepo.NewMongoArrivalRepository(db)
		clk := clock.Real{}
		arrivals := orderservice.NewArrivalService(arrivalRepo, clk)
		// D06 行程用例复用同一个密钥做核验摘要，避免另配一份秘密。
		trips := orderservice.NewTripService(orderrepo.NewMongoTripRepository(db), clk, cfg.Middleware.JWT.Secret)
		orderv1.RegisterArrivalServiceServer(server, orderhandler.NewArrivalServer(
			arrivals, arrivals, trips,
			authn.Config{Secret: cfg.Middleware.JWT.Secret, Issuer: cfg.Middleware.JWT.Issuer}, arrivalWrites,
		))
		// D07 乘客联系：虚拟号使用课程可控适配器，明确标记为模拟通道。
		contacts := orderservice.NewContactService(orderrepo.NewMongoContactRepository(db), orderservice.SimulatedVirtualCallProvider{Clock: clk}, clk)
		orderv1.RegisterContactServiceServer(server, orderhandler.NewContactServer(
			contacts,
			authn.Config{Secret: cfg.Middleware.JWT.Secret, Issuer: cfg.Middleware.JWT.Issuer}, arrivalWrites,
		))
		// D08 服务规范：只读消费 A18 的 config_dictionary，本服务没有任何写入路径。
		specs := orderservice.NewSpecService(orderrepo.NewMongoSpecRepository(db))
		orderv1.RegisterServiceSpecServiceServer(server, orderhandler.NewSpecServer(
			specs, authn.Config{Secret: cfg.Middleware.JWT.Secret, Issuer: cfg.Middleware.JWT.Issuer},
		))
		// D14 车辆资料 + D15 审核域：提交与裁决共用同一套并发控制。
		vehicleRepo := driverrepo.NewMongoVehicleRepository(db)
		fileRepo := driverrepo.NewMongoFileRepository(db)
		admissionRepo := driverrepo.NewMongoAdmissionRepository(db)
		taskReader := driverrepo.NewDispatchTaskReader(db)
		eventWriter := driverrepo.NewMongoEventWriter(db)
		reminderRepo := driverrepo.NewMongoReminderRepository(db)
		dispatchRepo := driverrepo.NewMongoDispatchRepository(db)
		reminders := driverservice.NewReminderService(reminderRepo, vehicleRepo, clk, log)
		eligibility := driverservice.NewEligibilityService(
			vehicleRepo, vehicleRepo, admissionRepo, dispatchRepo, clk, log,
		)
		// 末单结束时由行程域按最新准入决定收车；途中到期不打断行程。
		trips.SetEligibilityGate(eligibility, log)
		// D17/D18 安全事件：受理、处置、证据读取、说明内容。
		safetyRepo := driverrepo.NewMongoSafetyRepository(db)
		evidenceReader := driverrepo.NewMongoSafetyEvidenceReader(db)
		noticeReader := driverrepo.NewMongoSafetyNoticeReader(db)
		safety := driverservice.NewSafetyService(safetyRepo, taskReader, safetyRepo, fileRepo, eventWriter, evidenceReader, clk, log)
		notice := driverservice.NewNoticeService(noticeReader)
		vehicles := driverservice.NewVehicleService(vehicleRepo, fileRepo, vehicleRepo, clk, cfg.Middleware.JWT.Secret).
			WithReminders(log, reminders)
		reviews := driverservice.NewReviewService(
			vehicleRepo, vehicleRepo, admissionRepo, taskReader,
			vehicleRepo, fileRepo, eventWriter, clk, log,
		)
		reader, err := filex.NewPrivateReader(cfg.FileStorage)
		if err != nil {
			log.Fatal("init private reader failed", zap.Error(err))
		}
		uploader, err := filex.NewUploader(cfg.FileStorage)
		if err != nil {
			log.Fatal("init file uploader failed", zap.Error(err))
		}
		files := driverservice.NewFileService(fileRepo, uploader, reader, cfg.FileStorage, eventWriter, clk)
		driverAuth := authn.Config{Secret: cfg.Middleware.JWT.Secret, Issuer: cfg.Middleware.JWT.Issuer}
		driverv1.RegisterVehicleServiceServer(server, driverhandler.NewVehicleServer(vehicles, reviews, eligibility, reminders, driverAuth))
		driverv1.RegisterFileServiceServer(server, driverhandler.NewFileServer(files, driverAuth))
		driverv1.RegisterSafetyServiceServer(server, driverhandler.NewSafetyServer(safety, notice, driverAuth))
	} else {
		db, err := database.Open(cfg.Database, cfg.MySQL, cfg.PostgreSQL, log)
		if err != nil {
			log.Fatal("open database failed", zap.Error(err))
		}
		if err := orderrepo.AutoMigrate(db); err != nil {
			log.Fatal("migrate order database failed", zap.Error(err))
		}
		legacyRepo = orderrepo.NewGormRepository(db, log)
	}
	// 订单事件发布器：RabbitMQ 不可用时降级为 nil publisher（NewService 支持），
	// 保证未部署消息队列的环境同样能启动本服务，下单主流程不受影响。
	var orderPublisher orderservice.EventPublisher
	if mqClient, mqErr := rabbitmqx.New(cfg.RabbitMQ.RabbitmqxConfig()); mqErr != nil {
		log.Error("connect rabbitmq failed, order events disabled", zap.Error(mqErr))
	} else {
		defer mqClient.Close()
		orderPublisher = mqClient
	}
	orderv1.RegisterOrderServiceServer(server, orderhandler.NewServer(orderservice.NewService(legacyRepo, orderPublisher, log), log))

	port := cfg.ServicePort("order", defaultGRPCPort)
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
	fmt.Fprintf(os.Stdout, "  config: services.order.port\n\n")
}

func shutdownOnSignal(server *grpc.Server, log *zap.Logger) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	<-ch
	log.Info("service shutting down", zap.String("service", serviceName))
	server.GracefulStop()
}
