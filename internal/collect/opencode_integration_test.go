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

func TestIntegration_openCodePipeline(t *testing.T) {
	s, err := store.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	pricing := []byte(`[{"provider":"openai","model":"gpt-4.1","input_per_1m":2,"output_per_1m":8,"cache_read_per_1m":0,"cache_write_per_1m":0,"effective_from":"2026-01-01T00:00:00Z"}]`)
	if err := s.ImportPricingJSON(pricing, "test"); err != nil {
		t.Fatalf("ImportPricingJSON: %v", err)
	}

	norm := normalize.NewCompositeNormalizer(&normalize.ClaudeCodeNormalizer{}, &normalize.OpenCodeNormalizer{})
	w := NewWriter(s, true)
	r := NewReceiver(norm, w, 0)
	if err := r.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { r.Stop() })

	baseTime := time.Now()
	llmSpan := &tracepb.Span{
		Name:              "opencode.llm",
		TraceId:           []byte("otrace1"),
		SpanId:            []byte("ospan1"),
		StartTimeUnixNano: uint64(baseTime.UnixNano()),
		EndTimeUnixNano:   uint64(baseTime.Add(time.Second).UnixNano()),
		Attributes: []*commonpb.KeyValue{
			{Key: "session.id", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "opencode-integration"}}},
			{Key: "llm.model_name", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "gpt-4.1"}}},
			{Key: "llm.provider", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "openai"}}},
			{Key: "llm.token_count.prompt", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_IntValue{IntValue: 1000}}},
			{Key: "llm.token_count.completion", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_IntValue{IntValue: 500}}},
		},
	}
	toolSpan := &tracepb.Span{
		Name:              "opencode.tool.bash",
		TraceId:           []byte("otrace1"),
		SpanId:            []byte("ospan2"),
		ParentSpanId:      []byte("ospan1"),
		StartTimeUnixNano: uint64(baseTime.Add(100 * time.Millisecond).UnixNano()),
		EndTimeUnixNano:   uint64(baseTime.Add(400 * time.Millisecond).UnixNano()),
		Attributes: []*commonpb.KeyValue{
			{Key: "session.id", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "opencode-integration"}}},
			{Key: "tool.name", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "bash"}}},
			{Key: "tool.success", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_BoolValue{BoolValue: true}}},
		},
	}
	req := &coltracepb.ExportTraceServiceRequest{
		ResourceSpans: []*tracepb.ResourceSpans{{
			Resource: &resourcepb.Resource{
				Attributes: []*commonpb.KeyValue{
					{Key: "hcli.project_path", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "/tmp/open-project"}}},
				},
			},
			ScopeSpans: []*tracepb.ScopeSpans{{Spans: []*tracepb.Span{llmSpan, toolSpan}}},
		}},
	}
	body, err := proto.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	resp, err := http.Post(fmt.Sprintf("http://localhost:%d/v1/traces", r.Port()), "application/x-protobuf", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status: got %d, want 200", resp.StatusCode)
	}

	time.Sleep(200 * time.Millisecond)

	var llmCount, toolCount int
	_ = s.DB().QueryRow("SELECT count(*) FROM llm_calls WHERE agent = 'opencode' AND model = 'gpt-4.1'").Scan(&llmCount)
	_ = s.DB().QueryRow("SELECT count(*) FROM tool_calls WHERE agent = 'opencode' AND tool_name = 'bash'").Scan(&toolCount)
	if llmCount != 1 || toolCount != 1 {
		t.Fatalf("counts: llm=%d tool=%d", llmCount, toolCount)
	}

	var sessionCount, projectCount int
	_ = s.DB().QueryRow("SELECT count(*) FROM sessions WHERE external_id = 'opencode-integration' AND agent = 'opencode'").Scan(&sessionCount)
	_ = s.DB().QueryRow("SELECT count(*) FROM projects WHERE path = '/tmp/open-project'").Scan(&projectCount)
	if sessionCount != 1 || projectCount != 1 {
		t.Fatalf("session/project: session=%d project=%d", sessionCount, projectCount)
	}

	var cost float64
	_ = s.DB().QueryRow("SELECT total_cost_usd FROM sessions WHERE external_id = 'opencode-integration'").Scan(&cost)
	if cost <= 0 {
		t.Fatalf("session cost should be > 0, got %v", cost)
	}

	var inputTokens int
	_ = s.DB().QueryRow("SELECT total_input_tokens FROM sessions WHERE external_id = 'opencode-integration'").Scan(&inputTokens)
	if inputTokens != 1000 {
		t.Fatalf("session input tokens: got %d, want 1000", inputTokens)
	}

	var toolCallCount int
	_ = s.DB().QueryRow("SELECT tool_call_count FROM sessions WHERE external_id = 'opencode-integration'").Scan(&toolCallCount)
	if toolCallCount != 1 {
		t.Fatalf("session tool_call_count: got %d, want 1", toolCallCount)
	}
}
