package config

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestRemoteManager_Start(t *testing.T) {
	// Setup a fast mock server that counts fetches
	var fetchCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fetchCount.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"strategy": "remote"}`))
	}))
	defer srv.Close()

	// Initialise with a very short poll interval (10ms)
	rm := NewRemoteManager(srv.URL, 10*time.Millisecond, nil)

	// Run Start (which executes fetch immediately, then loops in background)
	go rm.Start()

	// Wait for at least one fetch via ticker
	time.Sleep(50 * time.Millisecond)

	if fetchCount.Load() == 0 {
		t.Errorf("expected remote manager to fetch at least once during Start loop")
	}

}
