package config

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// --- ModelSetter mock ---

type mockModelSetter struct {
	calls []string
}

func (m *mockModelSetter) SetDefaultModel(model string) {
	m.calls = append(m.calls, model)
}

// --- RemoteStrategy ---

func TestRemoteStrategy_FallbackOn404Enabled_DefaultTrue(t *testing.T) {
	rs := &RemoteStrategy{}
	if !rs.FallbackOn404Enabled() {
		t.Error("expected FallbackOn404Enabled to default to true when field is absent")
	}
}

func TestRemoteStrategy_FallbackOn404Enabled_ExplicitFalse(t *testing.T) {
	f := false
	rs := &RemoteStrategy{FallbackOn404: &f}
	if rs.FallbackOn404Enabled() {
		t.Error("expected FallbackOn404Enabled to return false when explicitly set to false")
	}
}

func TestRemoteStrategy_FallbackOn404Enabled_ExplicitTrue(t *testing.T) {
	tr := true
	rs := &RemoteStrategy{FallbackOn404: &tr}
	if !rs.FallbackOn404Enabled() {
		t.Error("expected FallbackOn404Enabled to return true when explicitly set to true")
	}
}

func TestRemoteStrategy_NilReceiver(t *testing.T) {
	var rs *RemoteStrategy
	if !rs.FallbackOn404Enabled() {
		t.Error("expected FallbackOn404Enabled to return true for nil receiver")
	}
}

// --- RemoteStrategy JSON parsing ---

func TestRemoteStrategy_ProviderModels_Parsed(t *testing.T) {
	raw := `{
		"strategy": "remote",
		"remote_provider": "google",
		"remote_model": "gemini-2.0-flash",
		"provider_models": {
			"openai":  "gpt-5",
			"google":  "gemini-2.0-flash",
			"anthropic": "claude-3-5-haiku-20241022"
		},
		"updated_at": "2026-03-01"
	}`
	var rs RemoteStrategy
	if err := json.Unmarshal([]byte(raw), &rs); err != nil {
		t.Fatalf("unexpected unmarshal error: %v", err)
	}
	if len(rs.ProviderModels) != 3 {
		t.Fatalf("expected 3 provider_models entries, got %d", len(rs.ProviderModels))
	}
	if rs.ProviderModels["openai"] != "gpt-5" {
		t.Errorf("unexpected openai model: %q", rs.ProviderModels["openai"])
	}
}

func TestRemoteStrategy_ProviderModels_AbsentField(t *testing.T) {
	raw := `{"strategy":"local","local_model":"llama-3"}`
	var rs RemoteStrategy
	if err := json.Unmarshal([]byte(raw), &rs); err != nil {
		t.Fatalf("unexpected unmarshal error: %v", err)
	}
	if rs.ProviderModels != nil {
		t.Errorf("expected nil ProviderModels, got %v", rs.ProviderModels)
	}
}

// --- applyProviderModels ---

func TestApplyProviderModels_NonEmptyValuesApplied(t *testing.T) {
	setter := &mockModelSetter{}
	rm := &RemoteManager{
		providers: map[string]ModelSetter{"openai": setter},
	}
	rm.applyProviderModels(map[string]string{"openai": "gpt-5", "google": "gemini-2.0-flash"})

	// Only openai is registered; google has no setter.
	if len(setter.calls) != 1 || setter.calls[0] != "gpt-5" {
		t.Errorf("expected one SetDefaultModel(gpt-5) call, got %v", setter.calls)
	}
}

func TestApplyProviderModels_EmptyValueSkipped(t *testing.T) {
	setter := &mockModelSetter{}
	rm := &RemoteManager{
		providers: map[string]ModelSetter{"openai": setter},
	}
	rm.applyProviderModels(map[string]string{"openai": ""}) // empty → skip

	if len(setter.calls) != 0 {
		t.Errorf("expected no SetDefaultModel calls for empty value, got %v", setter.calls)
	}
}

func TestApplyProviderModels_NilMap(t *testing.T) {
	setter := &mockModelSetter{}
	rm := &RemoteManager{
		providers: map[string]ModelSetter{"openai": setter},
	}
	rm.applyProviderModels(nil) // nil map → no-op, no panic

	if len(setter.calls) != 0 {
		t.Errorf("expected no calls for nil map, got %v", setter.calls)
	}
}

// --- RemoteManager.fetch via mock HTTP server ---

func TestRemoteManager_Fetch_UpdatesProviderModel(t *testing.T) {
	setter := &mockModelSetter{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(RemoteStrategy{
			Strategy:       "remote",
			RemoteProvider: "openai",
			RemoteModel:    "gpt-5",
			ProviderModels: map[string]string{"openai": "gpt-5"},
			UpdatedAt:      "2026-03-01",
		})
	}))
	defer srv.Close()

	rm := NewRemoteManager(srv.URL, time.Minute, map[string]ModelSetter{"openai": setter})
	if err := rm.fetch(); err != nil {
		t.Fatalf("fetch error: %v", err)
	}

	if len(setter.calls) != 1 || setter.calls[0] != "gpt-5" {
		t.Errorf("expected SetDefaultModel(gpt-5) after fetch, got %v", setter.calls)
	}

	got := rm.GetStrategy()
	if got.RemoteModel != "gpt-5" {
		t.Errorf("expected RemoteModel gpt-5, got %q", got.RemoteModel)
	}
}

func TestRemoteManager_Fetch_EmptyURL(t *testing.T) {
	rm := NewRemoteManager("", time.Minute, nil)
	if err := rm.fetch(); err == nil {
		t.Error("expected error for empty URL, got nil")
	}
}

func TestRemoteManager_Fetch_NonOKStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	rm := NewRemoteManager(srv.URL, time.Minute, nil)
	if err := rm.fetch(); err == nil {
		t.Error("expected error for non-200 status, got nil")
	}
}
