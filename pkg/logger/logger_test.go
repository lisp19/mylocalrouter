package logger

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

// captureLogs redirects the global logger to an in-memory buffer for the
// duration of fn and returns the captured output.
func captureLogs(t *testing.T, fn func()) string {
	t.Helper()
	var buf bytes.Buffer
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	prev := defaultLogger
	SetLogger(slog.New(handler))
	t.Cleanup(func() { SetLogger(prev) })
	fn()
	return buf.String()
}

func TestInfo(t *testing.T) {
	out := captureLogs(t, func() { Info("hello", "key", "val") })
	if !strings.Contains(out, "hello") {
		t.Errorf("expected 'hello' in output, got: %s", out)
	}
}

func TestWarn(t *testing.T) {
	out := captureLogs(t, func() { Warn("warn-msg") })
	if !strings.Contains(out, "warn-msg") {
		t.Errorf("expected warn-msg in output: %s", out)
	}
}

func TestError(t *testing.T) {
	out := captureLogs(t, func() { Error("err-msg", "err", "oops") })
	if !strings.Contains(out, "err-msg") {
		t.Errorf("expected err-msg in output: %s", out)
	}
}

func TestDebug(t *testing.T) {
	out := captureLogs(t, func() { Debug("dbg-msg") })
	if !strings.Contains(out, "dbg-msg") {
		t.Errorf("expected dbg-msg in output: %s", out)
	}
}

func TestInfof(t *testing.T) {
	out := captureLogs(t, func() { Infof("formatted %s %d", "val", 42) })
	if !strings.Contains(out, "formatted val 42") {
		t.Errorf("expected formatted output: %s", out)
	}
}

func TestWarnf(t *testing.T) {
	out := captureLogs(t, func() { Warnf("warnf-%d", 7) })
	if !strings.Contains(out, "warnf-7") {
		t.Errorf("expected warnf-7 in output: %s", out)
	}
}

func TestErrorf(t *testing.T) {
	out := captureLogs(t, func() { Errorf("errorf-%s", "x") })
	if !strings.Contains(out, "errorf-x") {
		t.Errorf("expected errorf-x in output: %s", out)
	}
}

func TestDebugf(t *testing.T) {
	out := captureLogs(t, func() { Debugf("debugf-%s", "y") })
	if !strings.Contains(out, "debugf-y") {
		t.Errorf("expected debugf-y in output: %s", out)
	}
}

func TestPrintf(t *testing.T) {
	out := captureLogs(t, func() { Printf("printf-%s", "z") })
	if !strings.Contains(out, "printf-z") {
		t.Errorf("expected printf-z in output: %s", out)
	}
}

func TestSetLogger(t *testing.T) {
	var buf bytes.Buffer
	l := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	prev := defaultLogger
	SetLogger(l)
	defer SetLogger(prev)
	Info("set-logger-test")
	if !strings.Contains(buf.String(), "set-logger-test") {
		t.Errorf("custom logger should capture output: %s", buf.String())
	}
}
