package handler

import (
	"context"
	stderrors "errors"
	userv1 "github.com/BwCloudWeGo/bw-cli/api/gen/user/v1"
	"github.com/BwCloudWeGo/bw-cli/internal/user/dto"
	"github.com/BwCloudWeGo/bw-cli/internal/user/entity"
	"github.com/BwCloudWeGo/bw-cli/internal/user/service"
	apperrors "github.com/BwCloudWeGo/bw-cli/pkg/errors"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Server 将 user gRPC 请求适配到 user 服务用例。
type Server struct {
	userv1.UnimplementedUserServiceServer
	svc      *service.Service
	realName *service.RealNameService
	security *service.AccountSecurityService
	config   *service.ConfigService
	log      *zap.Logger
}

// NewServer 创建 user gRPC 服务端适配器。
func NewServer(svc *service.Service, log *zap.Logger) *Server {
	return &Server{svc: svc, log: log}
}

// WithRealName 注入实名认证用例（P20）。
func (s *Server) WithRealName(rn *service.RealNameService) *Server {
	s.realName = rn
	return s
}

// WithAccountSecurity 注入账号安全用例（P21）。
func (s *Server) WithAccountSecurity(sec *service.AccountSecurityService) *Server {
	s.security = sec
	return s
}

// WithConfig 注入客服配置用例（P22）。
func (s *Server) WithConfig(cfg *service.ConfigService) *Server {
	s.config = cfg
	return s
}

// Register 处理用户注册 RPC。
func (s *Server) Register(ctx context.Context, req *userv1.RegisterRequest) (*userv1.UserResponse, error) {
	user, err := s.svc.Register(ctx, dto.RegisterCommand{
		Account:     req.GetAccount(),
		DisplayName: req.GetDisplayName(),
		Password:    req.GetPassword(),
	})
	if err != nil {
		return nil, mapUserError(err)
	}
	s.log.Info("user registered", zap.String("aggregate_id", user.ID), zap.String("use_case", "Register"))
	return toProto(user), nil
}

// Login 处理用户登录 RPC。
func (s *Server) Login(ctx context.Context, req *userv1.LoginRequest) (*userv1.UserResponse, error) {
	user, err := s.svc.Login(ctx, dto.LoginCommand{
		Account:  req.GetAccount(),
		Password: req.GetPassword(),
	})
	if err != nil {
		return nil, mapUserError(err)
	}
	s.log.Info("user logged in", zap.String("aggregate_id", user.ID), zap.String("use_case", "Login"))
	return toProto(user), nil
}

// GetUser 处理按 ID 查询用户资料。
func (s *Server) GetUser(ctx context.Context, req *userv1.GetUserRequest) (*userv1.UserResponse, error) {
	user, err := s.svc.GetUser(ctx, req.GetId())
	if err != nil {
		return nil, mapUserError(err)
	}
	return toProto(user), nil
}

// GetRealNameStatus 处理实名状态查询（GET /api/pax/realname，契约 3.7.1）。
func (s *Server) GetRealNameStatus(ctx context.Context, req *userv1.GetRealNameStatusRequest) (*userv1.RealNameStatusResponse, error) {
	if s.realName == nil {
		return nil, status.Error(codes.Unimplemented, "real name service not configured")
	}
	st, err := s.realName.GetRealNameStatus(ctx, req.GetAccountId())
	if err != nil {
		return nil, mapRealNameError(err)
	}
	return toRealNameProto(st), nil
}

// SubmitRealName 处理实名提交（POST /api/pax/realname，契约 3.7.1）。
func (s *Server) SubmitRealName(ctx context.Context, req *userv1.SubmitRealNameRequest) (*userv1.RealNameStatusResponse, error) {
	if s.realName == nil {
		return nil, status.Error(codes.Unimplemented, "real name service not configured")
	}
	st, err := s.realName.SubmitRealName(ctx, dto.SubmitRealNameCommand{
		AccountID:      req.GetAccountId(),
		Name:           req.GetName(),
		IDNo:           req.GetIdNo(),
		IdempotencyKey: req.GetIdempotencyKey(),
	})
	if err != nil {
		return nil, mapRealNameError(err)
	}
	s.log.Info("real name submitted", zap.String("aggregate_id", req.GetAccountId()), zap.String("use_case", "SubmitRealName"))
	return toRealNameProto(st), nil
}

// toRealNameProto 只输出 state 与脱敏值（不回真值）。
func toRealNameProto(st *dto.RealNameStatus) *userv1.RealNameStatusResponse {
	return &userv1.RealNameStatusResponse{State: st.State, Name: st.NameMask, IdNo: st.IDNoMask}
}

// GetAccountSecurity 处理账号安全查询（GET /api/pax/account-security，契约 3.7.2）。
func (s *Server) GetAccountSecurity(ctx context.Context, req *userv1.GetAccountSecurityRequest) (*userv1.AccountSecurityResponse, error) {
	if s.security == nil {
		return nil, status.Error(codes.Unimplemented, "account security service not configured")
	}
	st, err := s.security.GetAccountSecurity(ctx, req.GetAccountId())
	if err != nil {
		return nil, mapRealNameError(err)
	}
	resp := &userv1.AccountSecurityResponse{ProtocolVer: st.ProtocolVer, RealNameState: st.RealNameState}
	for _, b := range st.Bindings {
		resp.Bindings = append(resp.Bindings, &userv1.ChannelBinding{
			Channel: b.Channel,
			Bound:   b.Bound,
			BoundAt: b.BoundAt,
		})
	}
	return resp, nil
}

// RecordAgreement 处理协议同意记录（POST /api/pax/account-security，契约 3.7.2）。
func (s *Server) RecordAgreement(ctx context.Context, req *userv1.RecordAgreementRequest) (*userv1.RecordAgreementResponse, error) {
	if s.security == nil {
		return nil, status.Error(codes.Unimplemented, "account security service not configured")
	}
	got, err := s.security.RecordAgreement(ctx, req.GetAccountId(), req.GetProtocolVer())
	if err != nil {
		return nil, mapRealNameError(err)
	}
	s.log.Info("protocol agreement recorded", zap.String("aggregate_id", req.GetAccountId()), zap.String("use_case", "RecordAgreement"))
	return &userv1.RecordAgreementResponse{ProtocolVer: got.ProtocolVer}, nil
}

// GetServiceConfig 处理客服配置查询（P22，契约 3.7.3/3.9.6）。
func (s *Server) GetServiceConfig(ctx context.Context, _ *userv1.GetServiceConfigRequest) (*userv1.ServiceConfigResponse, error) {
	if s.config == nil {
		return nil, status.Error(codes.Unimplemented, "config service not configured")
	}
	cfg, err := s.config.GetServiceConfig(ctx)
	if err != nil {
		return nil, mapRealNameError(err)
	}
	return &userv1.ServiceConfigResponse{
		Hotline:     cfg.Hotline,
		ServiceTime: cfg.ServiceTime,
		ServiceSpec: cfg.ServiceSpec,
		FaqJson:     cfg.FAQJSON,
	}, nil
}

// mapRealNameError 透出带数值业务码的应用错误（40901/42201/42901 等）。
func mapRealNameError(err error) error {
	if appErr, ok := apperrors.As(err); ok {
		return appErr
	}
	return apperrors.Wrap(apperrors.KindInternal, "real_name_service_error", "real name service error", err)
}

func toProto(user *dto.UserDTO) *userv1.UserResponse {
	return &userv1.UserResponse{
		Id:          user.ID,
		Account:     user.Account,
		DisplayName: user.DisplayName,
	}
}

func mapUserError(err error) error {
	switch {
	case stderrors.Is(err, entity.ErrInvalidUser):
		return apperrors.InvalidArgument("invalid_user", "invalid user input")
	case stderrors.Is(err, entity.ErrAccountAlreadyExists):
		return apperrors.Conflict("account_already_exists", "account already exists")
	case stderrors.Is(err, entity.ErrUserNotFound):
		return apperrors.NotFound("user_not_found", "user not found")
	case stderrors.Is(err, entity.ErrInvalidCredentials):
		return apperrors.Unauthorized("invalid_credentials", "invalid credentials")
	default:
		return apperrors.Wrap(apperrors.KindInternal, "user_service_error", "user service error", err)
	}
}
