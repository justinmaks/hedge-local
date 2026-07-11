package collect

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/justinmaks/hedge-local/internal/normalize"
	"github.com/justinmaks/hedge-local/internal/store"
)

func TestReceiver_tracesEndpoint(t *testing.T) {
	s, err := store.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	if err := s.SeedBundledPricing(); err != nil {
		t.Fatalf("SeedBundledPricing: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	norm := &normalize.ClaudeCodeNormalizer{}
	w := NewWriter(s, false)
	r := NewReceiver(norm, w, 0)

	if err := r.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { r.Stop() })

	span := &tracepb.Span{
		Name:              "claude_code.llm_request",
		SpanId:            []byte("span1"),
		StartTimeUnixNano: uint64(time.Now().UnixNano()),
		EndTimeUnixNano:   uint64(time.Now().Add(time.Second).UnixNano()),
		Attributes: []*commonpb.KeyValue{
			{Key: "session.id", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "recv-sess-1"}}},
			{Key: "gen_ai.response.model", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "claude-sonnet-4"}}},
			{Key: "gen_ai.usage.input_tokens", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_IntValue{IntValue: 1000}}},
			{Key: "gen_ai.usage.output_tokens", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_IntValue{IntValue: 500}}},
		},
	}
	req := &coltracepb.ExportTraceServiceRequest{
		ResourceSpans: []*tracepb.ResourceSpans{{
			ScopeSpans: []*tracepb.ScopeSpans{{Spans: []*tracepb.Span{span}}},
		}},
	}
	body, err := proto.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	resp, err := http.Post(
		fmt.Sprintf("http://localhost:%d/v1/traces", r.Port()),
		"application/x-protobuf",
		bytes.NewReader(body),
	)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("status: got %d, want 200", resp.StatusCode)
	}

	time.Sleep(100 * time.Millisecond)

	var count int
	s.DB().QueryRow("SELECT count(*) FROM llm_calls").Scan(&count)
	if count != 1 {
		t.Errorf("llm_calls after POST: got %d, want 1", count)
	}
}

func newConformanceReceiver(t *testing.T) (*Receiver, *store.Store) {
	t.Helper()
	s, err := store.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	r := NewReceiver(&normalize.ClaudeCodeNormalizer{}, NewWriter(s, false), 0)
	if err := r.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { r.Stop() })
	return r, s
}

func sampleTraceRequest(session string) *coltracepb.ExportTraceServiceRequest {
	span := &tracepb.Span{
		Name:              "claude_code.llm_request",
		SpanId:            []byte(session + "-sp"),
		StartTimeUnixNano: uint64(time.Now().UnixNano()),
		EndTimeUnixNano:   uint64(time.Now().Add(time.Second).UnixNano()),
		Attributes: []*commonpb.KeyValue{
			{Key: "session.id", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: session}}},
			{Key: "gen_ai.response.model", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "claude-sonnet-4"}}},
		},
	}
	return &coltracepb.ExportTraceServiceRequest{
		ResourceSpans: []*tracepb.ResourceSpans{{
			ScopeSpans: []*tracepb.ScopeSpans{{Spans: []*tracepb.Span{span}}},
		}},
	}
}

func TestReceiver_jsonContentType(t *testing.T) {
	r, s := newConformanceReceiver(t)

	body, err := protojson.Marshal(sampleTraceRequest("json-sess"))
	if err != nil {
		t.Fatalf("protojson marshal: %v", err)
	}
	resp, err := http.Post(
		fmt.Sprintf("http://localhost:%d/v1/traces", r.Port()),
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status: got %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("response content type: got %q, want application/json", ct)
	}

	var count int
	s.DB().QueryRow("SELECT count(*) FROM llm_calls").Scan(&count)
	if count != 1 {
		t.Errorf("llm_calls after JSON POST: got %d, want 1", count)
	}
}

func TestReceiver_protoSuccessResponse(t *testing.T) {
	r, _ := newConformanceReceiver(t)

	body, _ := proto.Marshal(sampleTraceRequest("proto-sess"))
	resp, err := http.Post(
		fmt.Sprintf("http://localhost:%d/v1/traces", r.Port()),
		"application/x-protobuf",
		bytes.NewReader(body),
	)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status: got %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/x-protobuf" {
		t.Errorf("response content type: got %q, want application/x-protobuf", ct)
	}
	respBody, _ := io.ReadAll(resp.Body)
	var exportResp coltracepb.ExportTraceServiceResponse
	if err := proto.Unmarshal(respBody, &exportResp); err != nil {
		t.Errorf("response should be a valid ExportTraceServiceResponse: %v", err)
	}
}

func TestReceiver_unsupportedContentType(t *testing.T) {
	r, _ := newConformanceReceiver(t)

	resp, err := http.Post(
		fmt.Sprintf("http://localhost:%d/v1/traces", r.Port()),
		"text/plain",
		bytes.NewReader([]byte("hello")),
	)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnsupportedMediaType {
		t.Errorf("status: got %d, want 415", resp.StatusCode)
	}
	if r.MalformedCount() != 1 {
		t.Errorf("malformed count: got %d, want 1", r.MalformedCount())
	}
	if r.WriteFailureCount() != 0 {
		t.Errorf("write failure count should stay 0, got %d", r.WriteFailureCount())
	}
}

func TestReceiver_garbageProtobufCountsMalformed(t *testing.T) {
	r, _ := newConformanceReceiver(t)

	resp, err := http.Post(
		fmt.Sprintf("http://localhost:%d/v1/traces", r.Port()),
		"application/x-protobuf",
		bytes.NewReader([]byte{0xff, 0xfe, 0xfd}),
	)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status: got %d, want 400", resp.StatusCode)
	}
	if r.MalformedCount() != 1 {
		t.Errorf("malformed count: got %d, want 1", r.MalformedCount())
	}
}
