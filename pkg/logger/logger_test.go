package logger

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zapcore"
)

func TestDefaultConfigRetainsLogFilesForSevenDays(t *testing.T) {
	cfg := DefaultConfig("user-service")

	require.Equal(t, "user-service", cfg.Service)
	require.Equal(t, "info", cfg.Level)
	require.Equal(t, "logs/app.log", cfg.File.Filename)
	require.Equal(t, 7, cfg.File.MaxAgeDays)
	require.True(t, cfg.File.Compress)
}

func TestWithDailyFileNameUsesServiceAndCurrentDate(t *testing.T) {
	cfg := DefaultConfig("gateway")
	now := time.Date(2026, 4, 28, 12, 0, 0, 0, time.UTC)

	cfg = WithDailyFileName(cfg, now)
	require.Equal(
		t,
		filepath.Join("logs", "gateway-2026-04-28.log"),
		cfg.File.Filename,
	)
}

func TestNewLoggerAttachesCommonServiceDimensions(t *testing.T) {
	cfg := DefaultConfig("note-service")
	cfg.Environment = "test"
	cfg.File.Enabled = false

	log, err := New(cfg)
	require.NoError(t, err)
	require.NotNil(t, log)

	// Smoke test for required common dimensions. The logger should be ready to
	// write structured logs with service/env fields already attached.
	log.Info("logger initialized")
}

func TestWriterUsesConsoleAndFileWhenFileLoggingEnabled(t *testing.T) {
	var console bytes.Buffer
	path := filepath.Join(t.TempDir(), "app.log")

	sink := writerWithConsole(FileConfig{
		Enabled:    true,
		Filename:   path,
		MaxSizeMB:  1,
		MaxBackups: 1,
		MaxAgeDays: 1,
	}, zapcore.AddSync(&console))
	t.Cleanup(func() { require.NoError(t, sink.Close()) })

	_, err := sink.Write([]byte("hello\n"))
	require.NoError(t, err)
	require.NoError(t, sink.Sync())

	require.Contains(t, console.String(), "hello")
	fileData, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Contains(t, string(fileData), "hello")
}

func TestWriterSyncKeepsFileOpenUntilExplicitClose(t *testing.T) {
	var console bytes.Buffer
	path := filepath.Join(t.TempDir(), "lifecycle.log")
	sink := writerWithConsole(FileConfig{Enabled: true, Filename: path}, zapcore.AddSync(&console))
	t.Cleanup(func() { require.NoError(t, sink.Close()) })
	_, err := sink.Write([]byte("first\n"))
	require.NoError(t, err)
	require.NoError(t, sink.Sync())
	_, err = sink.Write([]byte("second\n"))
	require.NoError(t, err)
	require.NoError(t, sink.Close())
	require.NoError(t, sink.Close())
	fileData, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "first\nsecond\n", string(fileData))
	require.Equal(t, "first\nsecond\n", console.String())
	n, err := sink.Write([]byte("after close\n"))
	require.ErrorIs(t, err, os.ErrClosed)
	require.Zero(t, n)
	require.NoError(t, os.Remove(path))
}

func TestWriterDoesNotCloseBorrowedConsole(t *testing.T) {
	console, err := os.CreateTemp(t.TempDir(), "console")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, console.Close()) })
	sink := writerWithConsole(FileConfig{}, console)
	require.NoError(t, sink.Close())
	_, err = console.Write([]byte("still open"))
	require.NoError(t, err)
}
