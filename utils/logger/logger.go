package logger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const LevelCritical = slog.LevelError + 4

var levelMap = map[string]slog.Level{
	"DEBUG":    slog.LevelDebug,
	"INFO":     slog.LevelInfo,
	"WARNING":  slog.LevelWarn,
	"WARN":     slog.LevelWarn,
	"ERROR":    slog.LevelError,
	"CRITICAL": LevelCritical,
}

var globalLogLevel slog.LevelVar

var (
	globalFileWriter   io.Writer
	globalFileWriterMu sync.RWMutex
	globalWriteMu      sync.Mutex
	commandFileWriter  io.Writer
	commandWriterMu    sync.RWMutex
)

type Logger struct {
	inner *slog.Logger
}

var DefaultLogger *Logger

func init() {
	globalLogLevel.Set(slog.LevelInfo)
	DefaultLogger = NewLoggerFromGlobal("Haruki")
}

func SetDefaultLogger(l *Logger) {
	if l != nil {
		DefaultLogger = l
	}
}

func SetGlobalLogLevel(level string) {
	globalLogLevel.Set(parseLevel(level))
}

func GetGlobalLogLevel() string {
	return levelName(globalLogLevel.Level())
}

func SetGlobalFileWriter(w io.Writer) {
	globalFileWriterMu.Lock()
	globalFileWriter = w
	globalFileWriterMu.Unlock()
}

func getGlobalFileWriter() io.Writer {
	globalFileWriterMu.RLock()
	writer := globalFileWriter
	globalFileWriterMu.RUnlock()
	if writer == nil {
		return os.Stdout
	}
	return writer
}

// GlobalWriter returns a writer that follows the latest global log output.
func GlobalWriter() io.Writer {
	return dynamicWriter{}
}

// SetCommandWriter configures the dedicated sink for command summary records.
// Passing nil restores the default behavior of following the global writer.
func SetCommandWriter(w io.Writer) {
	commandWriterMu.Lock()
	commandFileWriter = w
	commandWriterMu.Unlock()
}

// InstallStandardHandlers routes log/slog and the standard log package through
// the same global formatter and destination as the project logger.
func InstallStandardHandlers() {
	slog.SetDefault(NewLoggerFromGlobal("Haruki").Slog())
}

// OpenLogFile opens a log file for writing, creating directories if needed.
func OpenLogFile(path string) (*os.File, error) {
	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("failed to create log directory: %w", err)
		}
	}
	return os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
}

type multiWriter struct {
	writers []io.Writer
}

func (mw *multiWriter) Write(p []byte) (int, error) {
	var firstErr error
	for _, writer := range mw.writers {
		if writer == nil {
			continue
		}
		written, err := writer.Write(p)
		if err == nil && written != len(p) {
			err = io.ErrShortWrite
		}
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return len(p), firstErr
}

func NewMultiWriter(writers ...io.Writer) io.Writer {
	filtered := make([]io.Writer, 0, len(writers))
	for _, writer := range writers {
		if writer != nil {
			filtered = append(filtered, writer)
		}
	}
	switch len(filtered) {
	case 0:
		return os.Stdout
	case 1:
		return filtered[0]
	default:
		return &multiWriter{writers: filtered}
	}
}

func NewLogger(name, level string, writer io.Writer) *Logger {
	if writer == nil {
		writer = os.Stdout
	}
	return newLogger(name, writer, parseLevel(level))
}

// NewLoggerFromGlobal follows both the current global level and output writer.
func NewLoggerFromGlobal(name string) *Logger {
	return newLogger(name, dynamicWriter{}, &globalLogLevel)
}

// NewLoggerWithGlobalWriter follows the global output writer while retaining a
// fixed level. It is intended for access and command telemetry that must remain
// available even when ordinary application logs use a stricter level.
func NewLoggerWithGlobalWriter(name, level string) *Logger {
	return newLogger(name, dynamicWriter{}, parseLevel(level))
}

// NewLoggerWithCommandWriter follows the dedicated command summary sink while
// retaining a fixed level. Before a command sink is configured it follows the
// global writer, which keeps standalone use and tests predictable.
func NewLoggerWithCommandWriter(name, level string) *Logger {
	return newLogger(name, commandDynamicWriter{}, parseLevel(level))
}

func newLogger(name string, writer io.Writer, level slog.Leveler) *Logger {
	handler := slog.NewTextHandler(writer, &slog.HandlerOptions{
		Level:       level,
		ReplaceAttr: replaceBuiltinAttr,
	})
	contextual := &contextHandler{next: handler}
	return &Logger{inner: slog.New(contextual).With("component", strings.TrimSpace(name))}
}

func replaceBuiltinAttr(_ []string, attr slog.Attr) slog.Attr {
	switch attr.Key {
	case slog.TimeKey:
		attr.Value = slog.TimeValue(attr.Value.Time().UTC())
	case slog.LevelKey:
		if level, ok := attr.Value.Any().(slog.Level); ok && level >= LevelCritical {
			attr.Value = slog.StringValue("CRITICAL")
		}
	}
	return attr
}

func parseLevel(level string) slog.Level {
	if parsed, ok := levelMap[strings.ToUpper(strings.TrimSpace(level))]; ok {
		return parsed
	}
	return slog.LevelInfo
}

func levelName(level slog.Level) string {
	switch {
	case level <= slog.LevelDebug:
		return "DEBUG"
	case level < slog.LevelWarn:
		return "INFO"
	case level < slog.LevelError:
		return "WARN"
	case level < LevelCritical:
		return "ERROR"
	default:
		return "CRITICAL"
	}
}

type dynamicWriter struct{}

func (dynamicWriter) Write(p []byte) (int, error) {
	globalWriteMu.Lock()
	defer globalWriteMu.Unlock()
	return getGlobalFileWriter().Write(p)
}

type commandDynamicWriter struct{}

func (commandDynamicWriter) Write(p []byte) (int, error) {
	commandWriterMu.RLock()
	writer := commandFileWriter
	if writer != nil {
		written, err := writer.Write(p)
		commandWriterMu.RUnlock()
		return written, err
	}
	commandWriterMu.RUnlock()
	return dynamicWriter{}.Write(p)
}

type serializedWriter struct {
	mu          sync.Mutex
	destination io.Writer
}

func (w *serializedWriter) Write(payload []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.destination.Write(payload)
}

// NewSerializedWriter prevents independent asynchronous sinks from
// interleaving records written to the same terminal or file.
func NewSerializedWriter(destination io.Writer) io.Writer {
	if destination == nil {
		destination = io.Discard
	}
	return &serializedWriter{destination: destination}
}

// WriteEmergency writes one canonical record directly to the supplied writer.
// It deliberately bypasses every global or asynchronous sink so fatal-path
// reporting cannot recurse through, or deadlock behind, a blocked log queue.
func WriteEmergency(writer io.Writer, component, msg string, args ...any) error {
	if writer == nil {
		writer = os.Stderr
	}
	record := slog.NewRecord(time.Now(), LevelCritical, msg, 0)
	record.Add("component", strings.TrimSpace(component))
	record.Add(args...)
	handler := slog.NewTextHandler(writer, &slog.HandlerOptions{ReplaceAttr: replaceBuiltinAttr})
	return handler.Handle(context.Background(), record)
}

type contextAttrsKey struct{}

// WithContextAttrs adds attributes that every project/slog record emitted with
// the returned context will inherit.
func WithContextAttrs(ctx context.Context, attrs ...slog.Attr) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if len(attrs) == 0 {
		return ctx
	}
	existing, _ := ctx.Value(contextAttrsKey{}).([]slog.Attr)
	combined := make([]slog.Attr, 0, len(existing)+len(attrs))
	combined = append(combined, existing...)
	combined = append(combined, attrs...)
	return context.WithValue(ctx, contextAttrsKey{}, combined)
}

// DetachedContext keeps only structured log attributes from ctx. It is safe to
// retain for queued background work without holding the request context, trace,
// body, or cancellation tree alive.
func DetachedContext(ctx context.Context) context.Context {
	base := context.Background()
	if ctx == nil {
		return base
	}
	attrs, _ := ctx.Value(contextAttrsKey{}).([]slog.Attr)
	if len(attrs) == 0 {
		return base
	}
	cloned := append([]slog.Attr(nil), attrs...)
	return context.WithValue(base, contextAttrsKey{}, cloned)
}

type contextHandler struct {
	next slog.Handler
}

func (h *contextHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(nonNilContext(ctx), level)
}

func (h *contextHandler) Handle(ctx context.Context, record slog.Record) error {
	ctx = nonNilContext(ctx)
	if attrs, ok := ctx.Value(contextAttrsKey{}).([]slog.Attr); ok {
		record.AddAttrs(attrs...)
	}
	return h.next.Handle(ctx, record)
}

func (h *contextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &contextHandler{next: h.next.WithAttrs(attrs)}
}

func (h *contextHandler) WithGroup(name string) slog.Handler {
	return &contextHandler{next: h.next.WithGroup(name)}
}

func (l *Logger) Slog() *slog.Logger {
	if l == nil || l.inner == nil {
		return slog.Default()
	}
	return l.inner
}

func (l *Logger) Enabled(ctx context.Context, level slog.Level) bool {
	return l != nil && l.inner != nil && l.inner.Enabled(nonNilContext(ctx), level)
}

func (l *Logger) logf(ctx context.Context, level slog.Level, format string, args ...any) {
	if !l.Enabled(ctx, level) {
		return
	}
	l.inner.Log(ctx, level, fmt.Sprintf(format, args...))
}

func (l *Logger) Debug(msg string, args ...any) { l.DebugContext(context.Background(), msg, args...) }
func (l *Logger) Info(msg string, args ...any)  { l.InfoContext(context.Background(), msg, args...) }
func (l *Logger) Warn(msg string, args ...any)  { l.WarnContext(context.Background(), msg, args...) }
func (l *Logger) Error(msg string, args ...any) { l.ErrorContext(context.Background(), msg, args...) }

func (l *Logger) DebugContext(ctx context.Context, msg string, args ...any) {
	ctx = nonNilContext(ctx)
	if l.Enabled(ctx, slog.LevelDebug) {
		l.inner.DebugContext(ctx, msg, args...)
	}
}

func (l *Logger) InfoContext(ctx context.Context, msg string, args ...any) {
	ctx = nonNilContext(ctx)
	if l.Enabled(ctx, slog.LevelInfo) {
		l.inner.InfoContext(ctx, msg, args...)
	}
}

func (l *Logger) WarnContext(ctx context.Context, msg string, args ...any) {
	ctx = nonNilContext(ctx)
	if l.Enabled(ctx, slog.LevelWarn) {
		l.inner.WarnContext(ctx, msg, args...)
	}
}

func (l *Logger) ErrorContext(ctx context.Context, msg string, args ...any) {
	ctx = nonNilContext(ctx)
	if l.Enabled(ctx, slog.LevelError) {
		l.inner.ErrorContext(ctx, msg, args...)
	}
}

func nonNilContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func (l *Logger) Debugf(format string, args ...any) {
	l.logf(context.Background(), slog.LevelDebug, format, args...)
}
func (l *Logger) Infof(format string, args ...any) {
	l.logf(context.Background(), slog.LevelInfo, format, args...)
}
func (l *Logger) Warnf(format string, args ...any) {
	l.logf(context.Background(), slog.LevelWarn, format, args...)
}
func (l *Logger) Errorf(format string, args ...any) {
	l.logf(context.Background(), slog.LevelError, format, args...)
}
func (l *Logger) Criticalf(format string, args ...any) {
	l.logf(context.Background(), LevelCritical, format, args...)
}
func (l *Logger) Exceptionf(format string, args ...any) {
	l.logf(context.Background(), slog.LevelError, format, args...)
}

func Debugf(format string, args ...any)    { DefaultLogger.Debugf(format, args...) }
func Infof(format string, args ...any)     { DefaultLogger.Infof(format, args...) }
func Warnf(format string, args ...any)     { DefaultLogger.Warnf(format, args...) }
func Errorf(format string, args ...any)    { DefaultLogger.Errorf(format, args...) }
func Criticalf(format string, args ...any) { DefaultLogger.Criticalf(format, args...) }

func Debug(msg string, args ...any) { DefaultLogger.Debug(msg, args...) }
func Info(msg string, args ...any)  { DefaultLogger.Info(msg, args...) }
func Warn(msg string, args ...any)  { DefaultLogger.Warn(msg, args...) }
func Error(msg string, args ...any) { DefaultLogger.Error(msg, args...) }

func DebugContext(ctx context.Context, msg string, args ...any) {
	DefaultLogger.DebugContext(ctx, msg, args...)
}
func InfoContext(ctx context.Context, msg string, args ...any) {
	DefaultLogger.InfoContext(ctx, msg, args...)
}
func WarnContext(ctx context.Context, msg string, args ...any) {
	DefaultLogger.WarnContext(ctx, msg, args...)
}
func ErrorContext(ctx context.Context, msg string, args ...any) {
	DefaultLogger.ErrorContext(ctx, msg, args...)
}
