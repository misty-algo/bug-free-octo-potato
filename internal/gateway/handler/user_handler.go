package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	userv1 "github.com/BwCloudWeGo/bw-cli/api/gen/user/v1"
	"github.com/BwCloudWeGo/bw-cli/internal/gateway/request"
	apperrors "github.com/BwCloudWeGo/bw-cli/pkg/errors"
	"github.com/BwCloudWeGo/bw-cli/pkg/httpx"
	"github.com/BwCloudWeGo/bw-cli/pkg/middleware"
)

// UserHandler 将 user HTTP 接口适配到内部 user gRPC client。
type UserHandler struct {
	client userv1.UserServiceClient
	jwtCfg middleware.JWTConfig
	log    *zap.Logger
}

// NewUserHandler 将 user gRPC client 注入 HTTP handler 方法。
func NewUserHandler(client userv1.UserServiceClient, jwtCfg middleware.JWTConfig, log *zap.Logger) *UserHandler {
	return &UserHandler{client: client, jwtCfg: jwtCfg, log: log}
}

// Register 处理 HTTP gateway 发来的用户注册请求。
func (h *UserHandler) Register(c *gin.Context) {
	var req request.RegisterUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, apperrors.InvalidArgument("invalid_request", err.Error()))
		return
	}
	resp, err := h.client.Register(outgoingContext(c), &userv1.RegisterRequest{
		Account:     req.Account,
		DisplayName: req.DisplayName,
		Password:    req.Password,
	})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	h.log.Info("gateway user register proxied", zap.String("request_id", httpx.RequestID(c)), zap.String("user_id", resp.GetId()))
	httpx.Created(c, resp)
}

// Login 处理 HTTP gateway 发来的用户登录请求。
func (h *UserHandler) Login(c *gin.Context) {
	var req request.LoginUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, apperrors.InvalidArgument("invalid_request", err.Error()))
		return
	}
	resp, err := h.client.Login(outgoingContext(c), &userv1.LoginRequest{
		Account:  req.Account,
		Password: req.Password,
	})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	token, err := middleware.GenerateToken(h.jwtCfg, middleware.JWTClaims{UserID: resp.GetId()})
	if err != nil {
		httpx.Error(c, apperrors.Wrap(apperrors.KindInternal, "jwt_token_error", "generate jwt token failed", err))
		return
	}
	h.log.Info("gateway user login proxied", zap.String("request_id", httpx.RequestID(c)), zap.String("user_id", resp.GetId()))
	httpx.OK(c, LoginResponse{User: resp, Token: token})
}

// CurrentUser 代理已认证用户的资料查询。
func (h *UserHandler) CurrentUser(c *gin.Context) {
	claims := middleware.ClaimsFromContext(c)
	if claims.UserID == "" {
		httpx.Error(c, apperrors.Unauthorized("invalid_token", "invalid bearer token"))
		return
	}
	resp, err := h.client.GetUser(outgoingContext(c), &userv1.GetUserRequest{Id: claims.UserID})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	httpx.OK(c, resp)
}

// GetUser 代理按 ID 查询用户资料。
func (h *UserHandler) GetUser(c *gin.Context) {
	resp, err := h.client.GetUser(outgoingContext(c), &userv1.GetUserRequest{Id: c.Param("id")})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	httpx.OK(c, resp)
}

// GetRealName 将 GET /api/pax/realname 代理到 GetRealNameStatus（契约 3.7.1）。
func (h *UserHandler) GetRealName(c *gin.Context) {
	claims := middleware.ClaimsFromContext(c)
	if claims.UserID == "" {
		httpx.PaxError(c, http.StatusUnauthorized, httpx.PaxCodeUnauthorized, "unauthorized")
		return
	}
	resp, err := h.client.GetRealNameStatus(outgoingContext(c), &userv1.GetRealNameStatusRequest{AccountId: claims.UserID})
	if err != nil {
		httpx.PaxAppError(c, apperrors.FromGRPC(err))
		return
	}
	httpx.PaxOK(c, resp)
}

// SubmitRealName 将 POST /api/pax/realname 代理到 SubmitRealName（契约 3.7.1）。
func (h *UserHandler) SubmitRealName(c *gin.Context) {
	var req request.RealNamePaxRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.PaxError(c, http.StatusBadRequest, httpx.PaxCodeInvalidArgument, err.Error())
		return
	}
	claims := middleware.ClaimsFromContext(c)
	if claims.UserID == "" {
		httpx.PaxError(c, http.StatusUnauthorized, httpx.PaxCodeUnauthorized, "unauthorized")
		return
	}
	resp, err := h.client.SubmitRealName(outgoingContext(c), &userv1.SubmitRealNameRequest{
		AccountId:      claims.UserID,
		Name:           req.Name,
		IdNo:           req.IDNo,
		IdempotencyKey: req.IdempotencyKey,
	})
	if err != nil {
		httpx.PaxAppError(c, apperrors.FromGRPC(err))
		return
	}
	httpx.PaxOK(c, resp)
}

// GetAccountSecurity 将 GET /api/pax/account-security 代理到 GetAccountSecurity（契约 3.7.2）。
func (h *UserHandler) GetAccountSecurity(c *gin.Context) {
	claims := middleware.ClaimsFromContext(c)
	if claims.UserID == "" {
		httpx.PaxError(c, http.StatusUnauthorized, httpx.PaxCodeUnauthorized, "unauthorized")
		return
	}
	resp, err := h.client.GetAccountSecurity(outgoingContext(c), &userv1.GetAccountSecurityRequest{AccountId: claims.UserID})
	if err != nil {
		httpx.PaxAppError(c, apperrors.FromGRPC(err))
		return
	}
	httpx.PaxOK(c, resp)
}

// RecordAgreement 将 POST /api/pax/account-security 代理到 RecordAgreement（契约 3.7.2）。
func (h *UserHandler) RecordAgreement(c *gin.Context) {
	var req request.AccountSecurityPaxRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.PaxError(c, http.StatusBadRequest, httpx.PaxCodeInvalidArgument, err.Error())
		return
	}
	claims := middleware.ClaimsFromContext(c)
	if claims.UserID == "" {
		httpx.PaxError(c, http.StatusUnauthorized, httpx.PaxCodeUnauthorized, "unauthorized")
		return
	}
	resp, err := h.client.RecordAgreement(outgoingContext(c), &userv1.RecordAgreementRequest{
		AccountId:   claims.UserID,
		ProtocolVer: req.ProtocolVer,
	})
	if err != nil {
		httpx.PaxAppError(c, apperrors.FromGRPC(err))
		return
	}
	httpx.PaxOK(c, resp)
}

// LoginResponse 是 POST /api/v1/users/login 返回的 gateway HTTP 载荷。
type LoginResponse struct {
	User  *userv1.UserResponse `json:"user"`
	Token string               `json:"token"`
}
