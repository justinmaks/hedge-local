package collect

import (
	"bytes"
	"fmt"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/proto"

	"github.com/justinmaks/hedge-local/internal/normalize"
	"github.com/justinmaks/hedge-local/internal/store"
)

func TestIntegration_claudeCodePipeline(t *testing.T) {
	s, err := store.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	if err := s.SeedBundledPricing(); err != nil {
		t.Fatalf("SeedBundledPricing: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	norm := &normalize.ClaudeCodeNormalizer{}
	w := NewWriter(s, true)
	r := NewReceiver(norm, w, 0)
	if err := r.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { r.Stop() })

	baseTime := time.Now()
	var spans []*tracepb.Span

	llmSpan := &tracepb.Span{
		Name:              "claude_code.llm_request",
		TraceId:           []byte("trace1"),
		SpanId:            []byte("span1"),
		StartTimeUnixNano: uint64(baseTime.UnixNano()),
		EndTimeUnixNano:   uint64(baseTime.Add(1500 * time.Millisecond).UnixNano()),
		Attributes: []*commonpb.KeyValue{
			{Key: "session.id", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "integration-sess"}}},
			{Key: "gen_ai.response.model", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "claude-sonnet-4"}}},
			{Key: "gen_ai.usage.input_tokens", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_IntValue{IntValue: 2000}}},
			{Key: "gen_ai.usage.output_tokens", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_IntValue{IntValue: 800}}},
			{Key: "gen_ai.usage.cache_read_input_tokens", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_IntValue{IntValue: 400}}},
			{Key: "gen_ai.usage.cache_creation_input_tokens", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_IntValue{IntValue: 200}}},
			{Key: "gen_ai.response.finish_reasons", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "end_turn"}}},
		},
	}
	spans = append(spans, llmSpan)

	toolSpan := &tracepb.Span{
		Name:              "claude_code.tool.execution",
		TraceId:           []byte("trace1"),
		SpanId:            []byte("span2"),
		ParentSpanId:      []byte("span1"),
		StartTimeUnixNano: uint64(baseTime.Add(100 * time.Millisecond).UnixNano()),
		EndTimeUnixNano:   uint64(baseTime.Add(400 * time.Millisecond).UnixNano()),
		Attributes: []*commonpb.KeyValue{
			{Key: "session.id", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "integration-sess"}}},
			{Key: "gen_ai.tool.name", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "bash"}}},
			{Key: "success", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_BoolValue{BoolValue: true}}},
		},
	}
	spans = append(spans, toolSpan)

	req := &coltracepb.ExportTraceServiceRequest{
		ResourceSpans: []*tracepb.ResourceSpans{{
			Resource: &resourcepb.Resource{
				Attributes: []*commonpb.KeyValue{
					{Key: "hcli.project_path", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "/home/user/myproject"}}},
				},
			},
			ScopeSpans: []*tracepb.ScopeSpans{{Spans: spans}},
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
		t.Fatalf("status: got %d, want 200", resp.StatusCode)
	}

	time.Sleep(200 * time.Millisecond)

	var sessionCount int
	s.DB().QueryRow("SELECT count(*) FROM sessions WHERE external_id = 'integration-sess'").Scan(&sessionCount)
	if sessionCount != 1 {
		t.Errorf("sessions: got %d, want 1", sessionCount)
	}

	var llmCount int
	s.DB().QueryRow("SELECT count(*) FROM llm_calls WHERE model = 'claude-sonnet-4'").Scan(&llmCount)
	if llmCount != 1 {
		t.Errorf("llm_calls: got %d, want 1", llmCount)
	}

	var toolCount int
	s.DB().QueryRow("SELECT count(*) FROM tool_calls WHERE tool_name = 'bash'").Scan(&toolCount)
	if toolCount != 1 {
		t.Errorf("tool_calls: got %d, want 1", toolCount)
	}

	var projectCount int
	s.DB().QueryRow("SELECT count(*) FROM projects WHERE path = '/home/user/myproject'").Scan(&projectCount)
	if projectCount != 1 {
		t.Errorf("projects: got %d, want 1", projectCount)
	}

	var sessionCost float64
	s.DB().QueryRow("SELECT total_cost_usd FROM sessions WHERE external_id = 'integration-sess'").Scan(&sessionCost)
	if sessionCost <= 0 {
		t.Errorf("session cost should be > 0, got %v", sessionCost)
	}

	var sessionTokens int
	s.DB().QueryRow("SELECT total_input_tokens FROM sessions WHERE external_id = 'integration-sess'").Scan(&sessionTokens)
	if sessionTokens != 2000 {
		t.Errorf("session input tokens: got %d, want 2000", sessionTokens)
	}

	var sessionToolCount int
	s.DB().QueryRow("SELECT tool_call_count FROM sessions WHERE external_id = 'integration-sess'").Scan(&sessionToolCount)
	if sessionToolCount != 1 {
		t.Errorf("session tool_call_count: got %d, want 1", sessionToolCount)
	}
}
