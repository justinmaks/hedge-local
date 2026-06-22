package collect

import (
	"bytes"
	"fmt"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/justinmaks/hedge-local/internal/normalize"
	"github.com/justinmaks/hedge-local/internal/store"
)

func newTestReceiver(t *testing.T) *Receiver {
	t.Helper()
	s, err := store.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return NewReceiver(&normalize.ClaudeCodeNormalizer{}, NewWriter(s, false), 0)
}

func TestReceiver_rejectsOversizedBody(t *testing.T) {
	r := newTestReceiver(t)
	r.maxBodyBytes = 1024
	if err := r.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { r.Stop() })

	oversized := bytes.Repeat([]byte("A"), int(r.maxBodyBytes)+1)
	resp, err := http.Post(
		fmt.Sprintf("http://localhost:%d/v1/traces", r.Port()),
		"application/x-protobuf",
		bytes.NewReader(oversized),
	)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode < 400 || resp.StatusCode >= 500 {
		t.Errorf("oversized body status = %d, want 4xx", resp.StatusCode)
	}
	if r.MalformedCount() == 0 {
		t.Errorf("expected malformed count to increment on oversized body")
	}
}

func TestReceiver_setsReadHeaderTimeout(t *testing.T) {
	r := newTestReceiver(t)
	if err := r.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { r.Stop() })

	if r.server.ReadHeaderTimeout <= 0 {
		t.Errorf("ReadHeaderTimeout = %v, want > 0", r.server.ReadHeaderTimeout)
	}
}
