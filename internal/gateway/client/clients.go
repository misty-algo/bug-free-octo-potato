package client

import (
	"fmt"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	driverv1 "github.com/BwCloudWeGo/bw-cli/api/gen/driver/v1"
	notev1 "github.com/BwCloudWeGo/bw-cli/api/gen/note/v1"
	orderv1 "github.com/BwCloudWeGo/bw-cli/api/gen/order/v1"
	userv1 "github.com/BwCloudWeGo/bw-cli/api/gen/user/v1"
	"github.com/BwCloudWeGo/bw-cli/pkg/config"
)

// 资料上传单文件可达 5MB，gRPC 默认 4MB 收发上限必须放宽。
const maxMaterialMessageBytes = 8 * 1024 * 1024

// Clients 聚合 HTTP gateway 使用的所有 gRPC client。
type Clients struct {
	User    userv1.UserServiceClient
	Note    notev1.NoteServiceClient
	Order   orderv1.OrderServiceClient
	Arrival orderv1.ArrivalServiceClient
	Contact orderv1.ContactServiceClient
	Spec    orderv1.ServiceSpecServiceClient
	Vehicle driverv1.VehicleServiceClient
	File    driverv1.FileServiceClient
	Safety  driverv1.SafetyServiceClient
	Config  *config.Config

	conns []*grpc.ClientConn
}

// New 连接配置的 gRPC 目标地址并创建强类型服务 client。
func New(cfg *config.Config, log *zap.Logger) (*Clients, error) {
	userTarget := cfg.ServiceTarget("user")
	noteTarget := cfg.ServiceTarget("note")
	orderTarget := cfg.ServiceTarget("order")
	userConn, err := grpc.Dial(userTarget, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("dial user service: %w", err)
	}
	noteConn, err := grpc.Dial(noteTarget, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		userConn.Close()
		return nil, fmt.Errorf("dial note service: %w", err)
	}
	orderConn, err := grpc.Dial(orderTarget,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(
			grpc.MaxCallSendMsgSize(maxMaterialMessageBytes),
			grpc.MaxCallRecvMsgSize(maxMaterialMessageBytes),
		))
	if err != nil {
		userConn.Close()
		noteConn.Close()
		return nil, fmt.Errorf("dial order service: %w", err)
	}

	log.Info("grpc clients initialized",
		zap.String("user_target", userTarget),
		zap.String("note_target", noteTarget),
		zap.String("order_target", orderTarget),
	)
	return &Clients{
		User:    userv1.NewUserServiceClient(userConn),
		Note:    notev1.NewNoteServiceClient(noteConn),
		Order:   orderv1.NewOrderServiceClient(orderConn),
		Arrival: orderv1.NewArrivalServiceClient(orderConn),
		Contact: orderv1.NewContactServiceClient(orderConn),
		Spec:    orderv1.NewServiceSpecServiceClient(orderConn),
		Vehicle: driverv1.NewVehicleServiceClient(orderConn),
		File:    driverv1.NewFileServiceClient(orderConn),
		Safety:  driverv1.NewSafetyServiceClient(orderConn),
		Config:  cfg,
		conns:   []*grpc.ClientConn{userConn, noteConn, orderConn},
	}, nil
}

// Close 释放 gateway 的所有 gRPC client 连接。
func (c *Clients) Close() {
	for _, conn := range c.conns {
		_ = conn.Close()
	}
}
