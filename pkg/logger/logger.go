package logger

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"time"
)

var defaultLogger *slog.Logger

func init() {
	opts := &slog.HandlerOptions{
		AddSource: true,
		Level:     slog.LevelInfo,
	}
	handler := slog.NewTextHandler(os.Stdout, opts)
	defaultLogger = slog.New(handler)
	slog.SetDefault(defaultLogger)
}

// SetLogger allows customized global loggers
func SetLogger(l *slog.Logger) {
	defaultLogger = l
	slog.SetDefault(l)
}

// log is a helper that adds the correct source code position skipping wrapper functions.
func log(level slog.Level, msg string, args ...any) {
	if !defaultLogger.Enabled(context.Background(), level) {
		return
	}
	var pcs [1]uintptr
	// Skip runtime.Callers, this func, and the exported wrapper func
	runtime.Callers(3, pcs[:])

	r := slog.NewRecord(time.Now(), level, msg, pcs[0])
	r.Add(args...)
	_ = defaultLogger.Handler().Handle(context.Background(), r)
}

func logf(level slog.Level, format string, args ...any) {
	if !defaultLogger.Enabled(context.Background(), level) {
		return
	}
	var pcs [1]uintptr
	runtime.Callers(3, pcs[:])

	msg := fmt.Sprintf(format, args...)
	r := slog.NewRecord(time.Now(), level, msg, pcs[0])
	_ = defaultLogger.Handler().Handle(context.Background(), r)
}

func Info(msg string, args ...any)  { log(slog.LevelInfo, msg, args...) }
func Warn(msg string, args ...any)  { log(slog.LevelWarn, msg, args...) }
func Error(msg string, args ...any) { log(slog.LevelError, msg, args...) }
func Debug(msg string, args ...any) { log(slog.LevelDebug, msg, args...) }

// Compatible methods for existing log.Printf and log.Fatalf usages
func Printf(format string, args ...any) { logf(slog.LevelInfo, format, args...) }
func Fatalf(format string, args ...any) {
	logf(slog.LevelError, format, args...)
	os.Exit(1)
}
func Fatal(args ...any) {
	log(slog.LevelError, fmt.Sprint(args...))
	os.Exit(1)
}
