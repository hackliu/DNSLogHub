package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/hackliu/DNSLogHub/internal/config"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	AuditLogger *zap.Logger
	auditMu     sync.Mutex
	auditSeq    uint64
)

// InitAuditLogger 初始化结构化审计日志：滚动写入文件，保留策略由 config.Log 控制。
// 需在 readConfig + Normalize 之后调用。
func InitAuditLogger() {
	cfg := config.Config.Log
	writer := newRotatingWriter(cfg.Dir, "audit.log", cfg.MaxSize, cfg.MaxBackups, cfg.MaxAge)
	enc := zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig())
	AuditLogger = zap.New(zapcore.NewCore(enc, zapcore.AddSync(writer), parseLevel(cfg.Level)), zap.AddCaller())
}

func parseLevel(s string) zapcore.Level {
	switch strings.ToLower(s) {
	case "debug":
		return zapcore.DebugLevel
	case "warn":
		return zapcore.WarnLevel
	case "error":
		return zapcore.ErrorLevel
	default:
		return zapcore.InfoLevel
	}
}

// Audit 记录一条结构化审计日志。operation 为操作类型（如 user.register / dnslog.access）。
func Audit(operation string, fields ...zap.Field) {
	if AuditLogger == nil {
		return
	}
	auditMu.Lock()
	auditSeq++
	seq := auditSeq
	auditMu.Unlock()

	all := append([]zap.Field{
		zap.Uint64("seq", seq),
		zap.String("operation", operation),
		zap.Time("ts", time.Now()),
	}, fields...)
	AuditLogger.Info("audit", all...)
}

// MaskToken 脱敏 token：保留前后各 4 位，中间打码，短 token 全打码。
func MaskToken(t string) string {
	if len(t) <= 8 {
		return "****"
	}
	return t[:4] + "****" + t[len(t)-4:]
}

// rotatingWriter 简单的按大小滚动 + 按数量/天数保留的文件写入器。
type rotatingWriter struct {
	mu         sync.Mutex
	dir        string
	name       string
	maxSize    int // MB
	maxBackups int
	maxAge     int // days
	file       *os.File
	size       int64
}

func newRotatingWriter(dir, name string, maxSize, maxBackups, maxAge int) *rotatingWriter {
	return &rotatingWriter{dir: dir, name: name, maxSize: maxSize, maxBackups: maxBackups, maxAge: maxAge}
}

func (w *rotatingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		if err := w.open(); err != nil {
			return 0, err
		}
	}
	n, err := w.file.Write(p)
	w.size += int64(n)
	if w.maxSize > 0 && w.size >= int64(w.maxSize)*1024*1024 {
		w.rotate()
	}
	return n, err
}

func (w *rotatingWriter) open() error {
	if err := os.MkdirAll(w.dir, 0o750); err != nil {
		return err
	}
	path := filepath.Join(w.dir, w.name)
	// #nosec G304 -- 日志目录来自配置文件（管理员可控），非用户输入
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	st, _ := f.Stat()
	w.file = f
	w.size = st.Size()
	return nil
}

func (w *rotatingWriter) rotate() {
	if w.file != nil {
		_ = w.file.Close()
		w.file = nil
	}
	path := filepath.Join(w.dir, w.name)
	rotated := filepath.Join(w.dir, fmt.Sprintf("%s.%s", w.name, time.Now().Format("20060102-150405")))
	_ = os.Rename(path, rotated)
	w.size = 0
	w.prune()
	_ = w.open()
}

func (w *rotatingWriter) prune() {
	entries, _ := filepath.Glob(filepath.Join(w.dir, w.name+".*"))
	type dated struct {
		path string
		t    time.Time
	}
	var files []dated
	for _, e := range entries {
		if info, err := os.Stat(e); err == nil {
			files = append(files, dated{e, info.ModTime()})
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].t.After(files[j].t) })
	for i, f := range files {
		remove := false
		if w.maxBackups > 0 && i >= w.maxBackups {
			remove = true
		}
		if w.maxAge > 0 && time.Since(f.t) > time.Duration(w.maxAge)*24*time.Hour {
			remove = true
		}
		if remove {
			_ = os.Remove(f.path)
		}
	}
}
