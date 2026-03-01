package httputil

import (
	"errors"
	"strings"
	"testing"
)

func TestProcessSSEStream_ParsesDataLines(t *testing.T) {
	input := "data: hello\ndata: world\n"
	var payloads []string
	err := ProcessSSEStream(strings.NewReader(input), func(data []byte) error {
		payloads = append(payloads, string(data))
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(payloads) != 2 || payloads[0] != "hello" || payloads[1] != "world" {
		t.Errorf("unexpected payloads: %v", payloads)
	}
}

func TestProcessSSEStream_StopsOnDONE(t *testing.T) {
	input := "data: first\ndata: [DONE]\ndata: second\n"
	var payloads []string
	ProcessSSEStream(strings.NewReader(input), func(data []byte) error {
		payloads = append(payloads, string(data))
		return nil
	})
	if len(payloads) != 1 || payloads[0] != "first" {
		t.Errorf("expected only 'first', got %v", payloads)
	}
}

func TestProcessSSEStream_SkipsEmptyLines(t *testing.T) {
	input := "\ndata: ok\n\n"
	var payloads []string
	ProcessSSEStream(strings.NewReader(input), func(data []byte) error {
		payloads = append(payloads, string(data))
		return nil
	})
	if len(payloads) != 1 || payloads[0] != "ok" {
		t.Errorf("unexpected payloads: %v", payloads)
	}
}

func TestProcessSSEStream_SkipsNonDataLines(t *testing.T) {
	input := "event: ping\ndata: real\n"
	var payloads []string
	ProcessSSEStream(strings.NewReader(input), func(data []byte) error {
		payloads = append(payloads, string(data))
		return nil
	})
	if len(payloads) != 1 || payloads[0] != "real" {
		t.Errorf("unexpected payloads: %v", payloads)
	}
}

func TestProcessSSEStream_CallbackError(t *testing.T) {
	input := "data: boom\n"
	want := errors.New("cb error")
	err := ProcessSSEStream(strings.NewReader(input), func(data []byte) error {
		return want
	})
	if err != want {
		t.Errorf("expected callback error, got %v", err)
	}
}

func TestProcessSSEStream_EmptyInput(t *testing.T) {
	err := ProcessSSEStream(strings.NewReader(""), func(data []byte) error {
		t.Error("callback should not be called on empty input")
		return nil
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
