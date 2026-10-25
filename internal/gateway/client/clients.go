package client

import (
	"fmt"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	notev1 "github.com/BwCloudWeGo/bw-cli/api/gen/note/v1"
	orderv1 "github.com/BwCloudWeGo/bw-cli/api/gen/order/v1"
	paymentv1 "github.com/BwCloudWeGo/bw-cli/api/gen/payment/v1"
	settlementv1 "github.com/BwCloudWeGo/bw-cli/api/gen/settlement/v1"
	shopv1 "github.com/BwCloudWeGo/bw-cli/api/gen/shop/v1"
	userv1 "github.com/BwCloudWeGo/bw-cli/api/gen/user/v1"
	"github.com/BwCloudWeGo/bw-cli/pkg/config"
)

// Clients 聚合 HTTP gateway 使用的所有 gRPC client。
type Clients struct {
	User       userv1.UserServiceClient
	Note       notev1.NoteServiceClient
	Order      orderv1.OrderServiceClient
	Shop       shopv1.ShopServiceClient
	Payment    paymentv1.PaymentServiceClient
	Settlement settlementv1.SettlementServiceClient
	Config     *config.Config

	conns []*grpc.ClientConn
}

// New 连接配置的 gRPC 目标地址并创建强类型服务 client。
func New(cfg *config.Config, log *zap.Logger) (*Clients, error) {
	userTarget := cfg.ServiceTarget("user")
	noteTarget := cfg.ServiceTarget("note")
	orderTarget := cfg.ServiceTarget("order")
	shopTarget := cfg.ServiceTarget("shop")
	paymentTarget := cfg.ServiceTarget("payment")
	settlementTarget := cfg.ServiceTarget("settlement")

	userConn, err := grpc.Dial(userTarget, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("dial user service: %w", err)
	}
	noteConn, err := grpc.Dial(noteTarget, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		userConn.Close()
		return nil, fmt.Errorf("dial note service: %w", err)
	}
	orderConn, err := grpc.Dial(orderTarget, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		userConn.Close()
		noteConn.Close()
		return nil, fmt.Errorf("dial order service: %w", err)
	}
	shopConn, err := grpc.Dial(shopTarget, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		userConn.Close()
		noteConn.Close()
		orderConn.Close()
		return nil, fmt.Errorf("dial shop service: %w", err)
	}
	paymentConn, err := grpc.Dial(paymentTarget, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		userConn.Close()
		noteConn.Close()
		orderConn.Close()
		shopConn.Close()
		return nil, fmt.Errorf("dial payment service: %w", err)
	}
	settlementConn, err := grpc.Dial(settlementTarget, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		userConn.Close()
		noteConn.Close()
		orderConn.Close()
		shopConn.Close()
		paymentConn.Close()
		return nil, fmt.Errorf("dial settlement service: %w", err)
	}

	log.Info("grpc clients initialized",
		zap.String("user_target", userTarget),
		zap.String("note_target", noteTarget),
		zap.String("order_target", orderTarget),
		zap.String("shop_target", shopTarget),
		zap.String("payment_target", paymentTarget),
		zap.String("settlement_target", settlementTarget),
	)
	return &Clients{
		User:       userv1.NewUserServiceClient(userConn),
		Note:       notev1.NewNoteServiceClient(noteConn),
		Order:      orderv1.NewOrderServiceClient(orderConn),
		Shop:       shopv1.NewShopServiceClient(shopConn),
		Payment:    paymentv1.NewPaymentServiceClient(paymentConn),
		Settlement: settlementv1.NewSettlementServiceClient(settlementConn),
		Config:     cfg,
		conns:      []*grpc.ClientConn{userConn, noteConn, orderConn, shopConn, paymentConn, settlementConn},
	}, nil
}

// Close 释放 gateway 的所有 gRPC client 连接。
func (c *Clients) Close() {
	for _, conn := range c.conns {
		_ = conn.Close()
	}
}
