package filex

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// localBackend 是课程可控的私有目录适配器：对象落在服务端私有目录，
// 从不产出公开 URL，读取必须走 AuthorizedReader 的授权代理。
type localBackend struct {
	provider string
	bucket   string
	root     string
}

func newLocalBackend(cfg Config) (*localBackend, error) {
	root := strings.TrimSpace(cfg.Local.RootDir)
	if root == "" {
		return nil, errors.New("file storage local root_dir is required")
	}
	return &localBackend{provider: ProviderLocal, bucket: cfg.Local.RootDir, root: root}, nil
}

func (b localBackend) Provider() string { return b.provider }
func (b localBackend) Bucket() string   { return b.bucket }

// Put 写入对象，并在写入过程中统计真实字节数：
// 超过限制则删除半包并返回错误，不轻信调用方声明的 Size。
func (b localBackend) Put(_ context.Context, req preparedUpload) (string, error) {
	if err := os.MkdirAll(filepath.Dir(b.path(req.Key)), 0o700); err != nil {
		return "", fmt.Errorf("prepare local object directory: %w", err)
	}
	file, err := os.OpenFile(b.path(req.Key), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return "", fmt.Errorf("create local object: %w", err)
	}
	limit := req.MaxSizeByte
	if limit <= 0 {
		limit = maxSizeBytes(Config{MaxSizeMB: DefaultConfig().MaxSizeMB})
	}
	var header bytes.Buffer
	tee := io.TeeReader(req.Reader, &header)
	written, err := io.Copy(file, io.LimitReader(tee, limit+1))
	if err != nil {
		file.Close()
		os.Remove(b.path(req.Key))
		return "", fmt.Errorf("write local object: %w", err)
	}
	if written > limit {
		file.Close()
		os.Remove(b.path(req.Key))
		return "", fmt.Errorf("file size exceeds limit: max %d bytes", limit)
	}
	if err := file.Close(); err != nil {
		return "", fmt.Errorf("close local object: %w", err)
	}
	sum := sha256.Sum256(header.Bytes())
	return hex.EncodeToString(sum[:16]), nil
}

// Open 只被授权读取代理调用（NewPrivateReader 内部），不对外暴露。
func (b localBackend) Open(_ context.Context, key string) (ObjectStream, error) {
	path := b.path(key)
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return ObjectStream{}, errors.New("object not found")
		}
		return ObjectStream{}, fmt.Errorf("stat local object: %w", err)
	}
	file, err := os.Open(path)
	if err != nil {
		return ObjectStream{}, fmt.Errorf("open local object: %w", err)
	}
	var header [512]byte
	n, _ := io.ReadFull(file, header[:])
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		file.Close()
		return ObjectStream{}, fmt.Errorf("rewind local object: %w", err)
	}
	contentType := SniffImageKind(header[:n])
	if contentType == "" {
		contentType = DetectContentType(key)
	}
	return ObjectStream{Body: file, ContentType: contentType, Size: info.Size()}, nil
}

func (b localBackend) Delete(_ context.Context, key string) error {
	if err := os.Remove(b.path(key)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("delete local object: %w", err)
	}
	return nil
}

func (b localBackend) path(key string) string {
	clean := filepath.Clean(filepath.FromSlash(strings.TrimLeft(key, "/")))
	return filepath.Join(b.root, clean)
}
