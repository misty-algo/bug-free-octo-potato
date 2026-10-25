package dto

import (
	"time"

	"github.com/BwCloudWeGo/bw-cli/pkg/authn"
)

// CreateUploadCommand 创建上传会话，返回 server 侧 file_id。
// 真正的字节在 UploadContent 中传输，限制在服务端执行。
type CreateUploadCommand struct {
	Principal authn.Principal `json:"-"`
	Purpose   string          `json:"purpose"`
}

type UploadSessionResult struct {
	FileID        string   `json:"file_id"`
	MaxBytes      int64    `json:"max_bytes"`
	AllowedTypes  []string `json:"allowed_types"`
	MaxPerVersion int      `json:"max_per_version"`
}

// UploadContentCommand 携带真实字节。服务端重新嗅探类型并统计实际大小。
type UploadContentCommand struct {
	Principal   authn.Principal `json:"-"`
	FileID      string          `json:"file_id"`
	Filename    string          `json:"filename"`
	Content     []byte          `json:"content"`
	ContentType string          `json:"content_type"`
}

type FileResult struct {
	FileID      string `json:"file_id"`
	State       string `json:"state"`
	ContentType string `json:"content_type"`
	Size        int64  `json:"size"`
	Checksum    string `json:"checksum"`
}

type GrantDownloadCommand struct {
	Principal authn.Principal `json:"-"`
	FileID    string          `json:"file_id"`
	Purpose   string          `json:"purpose"`
}

type DownloadGrantResult struct {
	Ticket      string    `json:"ticket"`
	ExpiresAt   time.Time `json:"expires_at"`
	ContentType string    `json:"content_type"`
	Size        int64     `json:"size"`
	FileName    string    `json:"file_name"`
}

type DownloadCommand struct {
	Principal authn.Principal `json:"-"`
	Ticket    string          `json:"ticket"`
}

type DownloadedFile struct {
	Body        []byte `json:"-"`
	ContentType string `json:"content_type"`
	Size        int64  `json:"size"`
}

type SweepStagingCommand struct {
	Principal authn.Principal `json:"-"`
	Limit     int64           `json:"limit"`
}

type SweepResult struct {
	Claimed int64 `json:"claimed"`
	Deleted int64 `json:"deleted"`
	Pending int64 `json:"pending"`
}
