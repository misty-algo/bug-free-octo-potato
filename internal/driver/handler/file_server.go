package handler

import (
	"context"
	"time"

	driverv1 "github.com/BwCloudWeGo/bw-cli/api/gen/driver/v1"
	"github.com/BwCloudWeGo/bw-cli/internal/driver/dto"
	"github.com/BwCloudWeGo/bw-cli/internal/driver/service"
	"github.com/BwCloudWeGo/bw-cli/pkg/authn"
	apperrors "github.com/BwCloudWeGo/bw-cli/pkg/errors"
)

// FileServer 暴露上传会话、授权票据与代理读取。
type FileServer struct {
	driverv1.UnimplementedFileServiceServer
	files *service.FileService
	auth  authn.Config
}

func NewFileServer(f *service.FileService, cfg authn.Config) *FileServer {
	return &FileServer{files: f, auth: cfg}
}

func (s *FileServer) CreateUpload(ctx context.Context, r *driverv1.CreateUploadRequest) (*driverv1.UploadSessionResult, error) {
	principal, err := principalFrom(ctx, s.auth)
	if err != nil {
		return nil, apperrors.ToGRPC(err)
	}
	result, err := s.files.CreateUpload(ctx, dto.CreateUploadCommand{Principal: principal, Purpose: r.GetPurpose()})
	if err != nil {
		return nil, apperrors.ToGRPC(err)
	}
	return &driverv1.UploadSessionResult{
		FileId: result.FileID, MaxBytes: result.MaxBytes,
		AllowedTypes: result.AllowedTypes, MaxPerVersion: int32(result.MaxPerVersion),
	}, nil
}

func (s *FileServer) UploadContent(ctx context.Context, r *driverv1.UploadContentRequest) (*driverv1.FileResult, error) {
	principal, err := principalFrom(ctx, s.auth)
	if err != nil {
		return nil, apperrors.ToGRPC(err)
	}
	result, err := s.files.UploadContent(ctx, dto.UploadContentCommand{
		Principal: principal, FileID: r.GetFileId(), Filename: r.GetFilename(),
		Content: r.GetContent(), ContentType: r.GetContentType(),
	})
	if err != nil {
		return nil, apperrors.ToGRPC(err)
	}
	return &driverv1.FileResult{
		FileId: result.FileID, State: result.State, ContentType: result.ContentType,
		Size: result.Size, Checksum: result.Checksum,
	}, nil
}

func (s *FileServer) GrantDownload(ctx context.Context, r *driverv1.GrantDownloadRequest) (*driverv1.DownloadGrantResult, error) {
	principal, err := principalFrom(ctx, s.auth)
	if err != nil {
		return nil, apperrors.ToGRPC(err)
	}
	result, err := s.files.GrantDownload(ctx, dto.GrantDownloadCommand{
		Principal: principal, FileID: r.GetFileId(), Purpose: r.GetPurpose(),
	})
	if err != nil {
		return nil, apperrors.ToGRPC(err)
	}
	return &driverv1.DownloadGrantResult{
		Ticket: result.Ticket, ExpiresAt: result.ExpiresAt.Unix(),
		ContentType: result.ContentType, Size: result.Size, FileName: result.FileName,
	}, nil
}

func (s *FileServer) Download(ctx context.Context, r *driverv1.DownloadRequest) (*driverv1.DownloadResult, error) {
	principal, err := principalFrom(ctx, s.auth)
	if err != nil {
		return nil, apperrors.ToGRPC(err)
	}
	result, err := s.files.Download(ctx, dto.DownloadCommand{Principal: principal, Ticket: r.GetTicket()})
	if err != nil {
		return nil, apperrors.ToGRPC(err)
	}
	return &driverv1.DownloadResult{Content: result.Body, ContentType: result.ContentType, Size: result.Size}, nil
}

func (s *FileServer) SweepStaging(ctx context.Context, r *driverv1.SweepStagingRequest) (*driverv1.SweepResult, error) {
	principal, err := principalFrom(ctx, s.auth)
	if err != nil {
		return nil, apperrors.ToGRPC(err)
	}
	result, err := s.files.SweepStaging(ctx, dto.SweepStagingCommand{Principal: principal, Limit: r.GetLimit()})
	if err != nil {
		return nil, apperrors.ToGRPC(err)
	}
	return &driverv1.SweepResult{Claimed: result.Claimed, Deleted: result.Deleted, Pending: result.Pending}, nil
}

var _ = time.Now
