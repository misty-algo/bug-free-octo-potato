package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/BwCloudWeGo/bw-cli/internal/driver/dto"
	"github.com/BwCloudWeGo/bw-cli/internal/driver/entity"
	"github.com/BwCloudWeGo/bw-cli/pkg/authn"
	"github.com/BwCloudWeGo/bw-cli/pkg/clock"
	apperrors "github.com/BwCloudWeGo/bw-cli/pkg/errors"
	"github.com/BwCloudWeGo/bw-cli/pkg/filex"
)

// stagingTTL 是暂存期限。到期且未被提交引用的文件才可被回收。
const stagingTTL = 24 * time.Hour

// FileService 负责上传会话、完整性校验、授权下载与孤儿清理。
// 对象存储与数据库不共事务：外部删除在数据库事务之外执行，
// 删除响应未知时保持 DELETING 并幂等重试，绝不改回可提交状态。
type FileService struct {
	files    entity.FileRepository
	uploader filex.Uploader
	reader   filex.PrivateReader
	cfg      filex.Config
	clock    clock.Clock
	events   entity.EventWriter
}

func NewFileService(files entity.FileRepository, uploader filex.Uploader, reader filex.PrivateReader, cfg filex.Config, events entity.EventWriter, clk clock.Clock) *FileService {
	return &FileService{files: files, uploader: uploader, reader: reader, cfg: cfg, events: events, clock: clk}
}

// CreateUpload 创建上传会话。真正的字节在 UploadContent 中由服务端实测。
func (s *FileService) CreateUpload(ctx context.Context, cmd dto.CreateUploadCommand) (*dto.UploadSessionResult, error) {
	if err := requireRole(cmd.Principal, RoleDriver); err != nil {
		return nil, err
	}
	now := s.clock.Now().UTC()
	expires := now.Add(stagingTTL)
	fileID := "F:" + uuid.NewString()
	rec := &entity.FileRecord{
		FileID: fileID, OwnerID: cmd.Principal.UserID, Purpose: cmd.Purpose,
		State: entity.FileStateUploading, Key: filex.NewObjectKey(s.cfg.ObjectPrefix, fileID+".bin"),
		CreatedAt: now, StagingExpiresAt: &expires,
	}
	if err := s.files.Insert(ctx, rec); err != nil {
		return nil, err
	}
	types := make([]string, 0, len(entity.AllowedImageTypes))
	for t := range entity.AllowedImageTypes {
		types = append(types, t)
	}
	return &dto.UploadSessionResult{
		FileID: fileID, MaxBytes: entity.MaxFileBytes, AllowedTypes: types,
		MaxPerVersion: entity.MaxFilesPerRevision,
	}, nil
}

// UploadContent 接收字节。真实类型靠 magic bytes 判定，
// 实际字节数由服务端统计，不信扩展名、Content-Type 或前端声明。
func (s *FileService) UploadContent(ctx context.Context, cmd dto.UploadContentCommand) (*dto.FileResult, error) {
	if err := requireRole(cmd.Principal, RoleDriver); err != nil {
		return nil, err
	}
	rec, err := s.files.ByID(ctx, cmd.FileID)
	if err != nil {
		return nil, err
	}
	if rec == nil {
		return nil, apperrors.NotFound("FILE_NOT_FOUND", "upload session not found")
	}
	if rec.OwnerID != cmd.Principal.UserID {
		return nil, apperrors.Forbidden("FORBIDDEN", "upload session belongs to another account")
	}
	if rec.State != entity.FileStateUploading {
		return nil, apperrors.Conflict("FILE_NOT_UPLOADING", "upload session is not accepting content")
	}
	content := cmd.Content
	if int64(len(content)) > entity.MaxFileBytes {
		if failErr := s.files.Fail(ctx, cmd.FileID, "file exceeds 5MB"); failErr != nil {
			return nil, failErr
		}
		return nil, apperrors.InvalidArgument("FILE_TOO_LARGE", fmt.Sprintf("each file must not exceed %d bytes", entity.MaxFileBytes))
	}
	kind := filex.SniffImageKind(content)
	if !entity.AllowedImageTypes[kind] {
		if failErr := s.files.Fail(ctx, cmd.FileID, "unsupported image type"); failErr != nil {
			return nil, failErr
		}
		return nil, apperrors.InvalidArgument("UNSUPPORTED_IMAGE_TYPE", "only JPG and PNG are accepted")
	}
	result, err := s.uploader.Upload(ctx, filex.UploadRequest{
		Reader:      bytes.NewReader(content),
		Filename:    cmd.Filename,
		ContentType: kind,
		Size:        int64(len(content)),
		ObjectKey:   rec.Key,
		Metadata:    map[string]string{"ownerId": rec.OwnerID, "fileId": rec.FileID},
	})
	if err != nil {
		if failErr := s.files.Fail(ctx, cmd.FileID, "upload rejected"); failErr != nil {
			return nil, failErr
		}
		return nil, apperrors.InvalidArgument("UPLOAD_REJECTED", err.Error())
	}
	// MarkUploaded 只在 UPLOADING 行上成功，写入服务端实测值。
	ok, err := s.files.MarkUploaded(ctx, cmd.FileID, result.Provider, result.Bucket, kind, result.ETag, int64(len(content)))
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, apperrors.Conflict("FILE_NOT_UPLOADING", "upload session was already consumed")
	}
	return &dto.FileResult{
		FileID: cmd.FileID, State: entity.FileStateReady,
		ContentType: kind, Size: int64(len(content)), Checksum: result.ETag,
	}, nil
}

// GrantDownload 签发短期一次用途票据。这里返回票据而非对象直签 URL：
// key 不可预测不等于已授权。
func (s *FileService) GrantDownload(ctx context.Context, cmd dto.GrantDownloadCommand) (dto.DownloadGrantResult, error) {
	if err := requireRole(cmd.Principal, RoleDriver, RoleAuditor, RoleSafetyHandler); err != nil {
		return dto.DownloadGrantResult{}, err
	}
	rec, err := s.files.ByID(ctx, cmd.FileID)
	if err != nil {
		return dto.DownloadGrantResult{}, err
	}
	if rec == nil {
		return dto.DownloadGrantResult{}, apperrors.NotFound("FILE_NOT_FOUND", "file not found")
	}
	purpose := strings.TrimSpace(cmd.Purpose)
	if purpose == "" {
		return dto.DownloadGrantResult{}, apperrors.InvalidArgument("INVALID_PURPOSE", "download purpose is required")
	}
	// 票据用途必须与文件落库用途一致：拿到 safety 票据下载不了证件。
	if purpose != rec.Purpose {
		return dto.DownloadGrantResult{}, apperrors.Forbidden("PURPOSE_MISMATCH", "download purpose does not match file usage")
	}
	if !authorizedToRead(cmd.Principal, rec, purpose) {
		return dto.DownloadGrantResult{}, apperrors.Forbidden("FORBIDDEN", "not authorized to read this file")
	}
	grant, err := s.reader.GrantDownload(ctx, filex.GrantRequest{
		Key: rec.Key, Purpose: purpose, SubjectID: cmd.Principal.UserID,
		TTL: filex.DefaultTicketTTL, MaxBytes: entity.MaxFileBytes,
	})
	if err != nil {
		return dto.DownloadGrantResult{}, apperrors.Internal("DOWNLOAD_GRANT_FAILED", err.Error())
	}
	// 授权动作本身写审计：谁、哪个文件、什么用途，都留痕。
	if s.events != nil {
		now := s.clock.Now().UTC()
		payload := fmt.Sprintf(`{"file_id":%q,"purpose":%q,"actor":%q}`, cmd.FileID, purpose, cmd.Principal.UserID)
		if err := s.events.AppendEvent(ctx, fmt.Sprintf("FILE:grant:%s:%s:%d", cmd.FileID, cmd.Principal.UserID, now.UnixNano()), "FileDownloadGrant", "safety", payload, now); err != nil {
			return dto.DownloadGrantResult{}, apperrors.Internal("AUDIT_FAILED", err.Error())
		}
	}
	return dto.DownloadGrantResult{
		Ticket: grant.Ticket, ExpiresAt: grant.ExpiresAt,
		ContentType: rec.ContentType, Size: rec.Size, FileName: cmd.FileID,
	}, nil
}

// Download 代理读取。票据只是凭证，这里再次校验身份与用途后才取对象。
func (s *FileService) Download(ctx context.Context, cmd dto.DownloadCommand) (*dto.DownloadedFile, error) {
	if strings.TrimSpace(cmd.Principal.UserID) == "" {
		return nil, apperrors.Unauthorized("UNAUTHENTICATED", "a valid principal is required")
	}
	claims, err := s.reader.Inspect(ctx, cmd.Ticket)
	if err != nil {
		return nil, apperrors.Forbidden("DOWNLOAD_DENIED", err.Error())
	}
	rec, err := s.files.ByKey(ctx, claims.Key)
	if err != nil {
		return nil, err
	}
	if rec == nil {
		return nil, apperrors.NotFound("FILE_NOT_FOUND", "file not found")
	}
	// 双重校验：票据用途必须与文件落库用途一致，堵住"换 purpose 重签"的绕过。
	if claims.Purpose != rec.Purpose {
		return nil, apperrors.Forbidden("PURPOSE_MISMATCH", "ticket purpose does not match file usage")
	}
	// 再次判定：票据本身不携带「已授权」这一结论。
	if !authorizedToRead(cmd.Principal, rec, claims.Purpose) {
		return nil, apperrors.Forbidden("FORBIDDEN", "not authorized to read this file")
	}
	if claims.Subject != cmd.Principal.UserID {
		return nil, apperrors.Forbidden("FORBIDDEN", "ticket was issued to another subject")
	}
	stream, err := s.reader.Open(ctx, cmd.Ticket)
	if err != nil {
		return nil, apperrors.Forbidden("DOWNLOAD_DENIED", err.Error())
	}
	defer stream.Body.Close()
	body, err := io.ReadAll(io.LimitReader(stream.Body, entity.MaxFileBytes+1))
	if err != nil {
		return nil, apperrors.Internal("DOWNLOAD_FAILED", err.Error())
	}
	if int64(len(body)) > entity.MaxFileBytes {
		return nil, apperrors.Forbidden("DOWNLOAD_DENIED", "object exceeds authorized size")
	}
	return &dto.DownloadedFile{Body: body, ContentType: stream.ContentType, Size: int64(len(body))}, nil
}

// SweepStaging 回收暂存孤儿，并在数据库之外幂等执行外部删除。
func (s *FileService) SweepStaging(ctx context.Context, cmd dto.SweepStagingCommand) (*dto.SweepResult, error) {
	if err := requireRole(cmd.Principal, RoleAuditor, RoleSafetyHandler); err != nil {
		return nil, err
	}
	now := s.clock.Now().UTC()
	limit := cmd.Limit
	if limit <= 0 {
		limit = 20
	}
	claimed, err := s.files.ClaimStale(ctx, now, limit)
	if err != nil {
		return nil, err
	}
	tasks, err := s.files.PendingDeleteTasks(ctx, limit)
	if err != nil {
		return nil, err
	}
	result := &dto.SweepResult{Claimed: int64(len(claimed)), Pending: int64(len(tasks))}
	for _, task := range tasks {
		if err := s.files.BumpDeleteAttempt(ctx, task.TaskID); err != nil {
			return result, err
		}
		// 外部对象删除在数据库事务之外；响应未知不改回可提交状态，下次继续重试。
		if err := s.reader.Delete(ctx, task.Key); err != nil {
			continue
		}
		if err := s.files.MarkFileDeleted(ctx, task.FileID, now); err != nil {
			return result, err
		}
		if err := s.files.MarkDeleteTaskDone(ctx, task.TaskID, now); err != nil {
			return result, err
		}
		result.Deleted++
	}
	return result, nil
}

// authorizedToRead 判定读取范围：
// driver_self 只能读自己的材料，audit/safety 走职责范围。
func authorizedToRead(principal authn.Principal, rec *entity.FileRecord, purpose string) bool {
	switch purpose {
	case filex.PurposeDriverSelf:
		return rec.OwnerID == principal.UserID
	case filex.PurposeAudit:
		return hasRole(principal, RoleAuditor)
	case filex.PurposeSafety:
		return hasRole(principal, RoleAuditor) || hasRole(principal, RoleSafetyHandler)
	default:
		return false
	}
}
