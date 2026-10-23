package handler

import (
	"io"
	"strings"

	driverv1 "github.com/BwCloudWeGo/bw-cli/api/gen/driver/v1"
	"github.com/BwCloudWeGo/bw-cli/internal/driver/entity"
	"github.com/BwCloudWeGo/bw-cli/internal/gateway/request"
	"github.com/BwCloudWeGo/bw-cli/pkg/authn"
	apperrors "github.com/BwCloudWeGo/bw-cli/pkg/errors"
	"github.com/BwCloudWeGo/bw-cli/pkg/httpx"
	"github.com/gin-gonic/gin"
)

// FileHandler 暴露上传会话、授权票据与代理下载。
// 上传字节走原始 body，类型与大小由服务端重新判定。
type FileHandler struct {
	client driverv1.FileServiceClient
	auth   authn.Config
}

func NewFileHandler(c driverv1.FileServiceClient, cfg authn.Config) *FileHandler {
	return &FileHandler{client: c, auth: cfg}
}

func (h *FileHandler) DriverGuard() gin.HandlerFunc { return materialGuard(h.auth, "driver") }

// MaterialGuard 覆盖授权票据与代理下载：审核与安全职责也要能取票据，
// 但下载范围仍由服务层按用途与归属二次判定。
func (h *FileHandler) MaterialGuard() gin.HandlerFunc {
	return materialGuard(h.auth, "driver", "auditor", "safety_handler")
}

func (h *FileHandler) StaffGuard() gin.HandlerFunc {
	return materialGuard(h.auth, "auditor", "safety_handler")
}

// CreateUpload: POST /api/drv/file/upload-session
func (h *FileHandler) CreateUpload(c *gin.Context) {
	var r struct {
		Purpose string `json:"purpose"`
	}
	if err := request.DecodeArrivalJSON(c.Request.Body, &r); err != nil {
		httpx.Error(c, apperrors.InvalidArgument("INVALID_REQUEST", err.Error()))
		return
	}
	ctx, cancel := requestContext(c)
	defer cancel()
	out, err := h.client.CreateUpload(ctx, &driverv1.CreateUploadRequest{Purpose: r.Purpose})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	httpx.OK(c, out)
}

// UploadContent: POST /api/drv/file/upload?file_id=&filename=
// body 为原始字节（≤5MB）。Content-Type 仅作参考，不参与判定。
func (h *FileHandler) UploadContent(c *gin.Context) {
	fileID := strings.TrimSpace(c.Query("file_id"))
	filename := strings.TrimSpace(c.Query("filename"))
	if fileID == "" {
		httpx.Error(c, apperrors.InvalidArgument("INVALID_REQUEST", "file_id is required"))
		return
	}
	limited := io.LimitReader(c.Request.Body, entity.MaxFileBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		httpx.Error(c, apperrors.InvalidArgument("INVALID_REQUEST", "cannot read upload body"))
		return
	}
	ctx, cancel := requestContext(c)
	defer cancel()
	out, err := h.client.UploadContent(ctx, &driverv1.UploadContentRequest{
		FileId: fileID, Filename: filename, Content: body, ContentType: c.GetHeader("Content-Type"),
	})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	httpx.OK(c, out)
}

// GrantDownload: POST /api/drv/file/download-grant
func (h *FileHandler) GrantDownload(c *gin.Context) {
	var r struct {
		FileID  string `json:"file_id"`
		Purpose string `json:"purpose"`
	}
	if err := request.DecodeArrivalJSON(c.Request.Body, &r); err != nil {
		httpx.Error(c, apperrors.InvalidArgument("INVALID_REQUEST", err.Error()))
		return
	}
	ctx, cancel := requestContext(c)
	defer cancel()
	out, err := h.client.GrantDownload(ctx, &driverv1.GrantDownloadRequest{FileId: r.FileID, Purpose: r.Purpose})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	httpx.OK(c, out)
}

// Download: GET /api/drv/file/download?ticket=
// 响应头 no-store，禁止浏览器长期缓存证件内容。
func (h *FileHandler) Download(c *gin.Context) {
	ticket := strings.TrimSpace(c.Query("ticket"))
	if ticket == "" {
		httpx.Error(c, apperrors.InvalidArgument("INVALID_REQUEST", "ticket is required"))
		return
	}
	ctx, cancel := requestContext(c)
	defer cancel()
	out, err := h.client.Download(ctx, &driverv1.DownloadRequest{Ticket: ticket})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	attachmentHeaders(c, out.GetContentType(), "material")
	_, _ = c.Writer.Write(out.GetContent())
}

// SweepStaging: POST /api/admin/file/sweep
func (h *FileHandler) SweepStaging(c *gin.Context) {
	var r struct {
		Limit int64 `json:"limit"`
	}
	if err := request.DecodeArrivalJSON(c.Request.Body, &r); err != nil {
		httpx.Error(c, apperrors.InvalidArgument("INVALID_REQUEST", err.Error()))
		return
	}
	ctx, cancel := requestContext(c)
	defer cancel()
	out, err := h.client.SweepStaging(ctx, &driverv1.SweepStagingRequest{Limit: r.Limit})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	httpx.OK(c, out)
}
