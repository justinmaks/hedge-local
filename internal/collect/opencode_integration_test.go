package collect

import (
	"bytes"
	"fmt"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	"google.golang.org/protobuf/proto"

	"github.com/justinmaks/hedge-local/internal/normalize"
	"github.com/justinmaks/hedge-local/internal/store"
)

// OpenCode telemetry arrives as OTLP log events from the @devtheops plugin.
// This proves the full pipeline: api_request -> llm_call, tool_result ->
// tool_call, session.created -> raw event, with session/project attribution
// and cost taken from the explicit cost_usd the plugin reports.
func TestIntegration_openCodePipeline(t *testing.T) {
	s, err := store.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	norm := normalize.NewCompositeNormalizer(&normalize.ClaudeCodeNormalizer{}, &normalize.OpenCodeNormalizer{})
	w := NewWriter(s, true)
	r := NewReceiver(norm, w, 0)
	if err := r.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { r.Stop() })

	now := uint64(time.Now().UnixNano())
	sess := func() *commonpb.KeyValue {
		return &commonpb.KeyValue{Key: "session.id", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "opencode-integration"}}}
	}
	strAttr := func(k, v string) *commonpb.KeyValue {
		return &commonpb.KeyValue{Key: k, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: v}}}
	}
	intAttr := func(k string, v int64) *commonpb.KeyValue {
		return &commonpb.KeyValue{Key: k, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_IntValue{IntValue: v}}}
	}
	floatAttr := func(k string, v float64) *commonpb.KeyValue {
		return &commonpb.KeyValue{Key: k, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_DoubleValue{DoubleValue: v}}}
	}
	boolAttr := func(k string, v bool) *commonpb.KeyValue {
		return &commonpb.KeyValue{Key: k, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_BoolValue{BoolValue: v}}}
	}

	records := []*logspb.LogRecord{
		{
			TimeUnixNano: now,
			Body:         &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "session.created"}},
			Attributes:   []*commonpb.KeyValue{strAttr("event.name", "session.created"), sess()},
		},
		{
			TimeUnixNano: now,
			Body:         &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "api_request"}},
			Attributes: []*commonpb.KeyValue{
				strAttr("event.name", "api_request"), sess(),
				strAttr("model", "google/gemini-2.5-flash-lite"),
				strAttr("provider", "vercel"),
				intAttr("input_tokens", 1000),
				intAttr("output_tokens", 500),
				floatAttr("cost_usd", 0.0042),
				intAttr("duration_ms", 2726),
			},
		},
		{
			TimeUnixNano: now,
			Body:         &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "tool_result"}},
			Attributes: []*commonpb.KeyValue{
				strAttr("event.name", "tool_result"), sess(),
				strAttr("tool_name", "bash"),
				boolAttr("success", true),
				intAttr("duration_ms", 41),
			},
		},
	}
	req := &collogspb.ExportLogsServiceRequest{ResourceLogs: []*logspb.ResourceLogs{{
		Resource: &resourcepb.Resource{Attributes: []*commonpb.KeyValue{
			strAttr("hcli.project_path", "/tmp/open-project"),
		}},
		ScopeLogs: []*logspb.ScopeLogs{{LogRecords: records}},
	}}}

	body, err := proto.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	resp, err := http.Post(fmt.Sprintf("http://localhost:%d/v1/logs", r.Port()), "application/x-protobuf", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status: got %d, want 200", resp.StatusCode)
	}

	time.Sleep(200 * time.Millisecond)

	var llmCount, toolCount int
	_ = s.DB().QueryRow("SELECT count(*) FROM llm_calls WHERE agent = 'opencode' AND model = 'google/gemini-2.5-flash-lite'").Scan(&llmCount)
	_ = s.DB().QueryRow("SELECT count(*) FROM tool_calls WHERE agent = 'opencode' AND tool_name = 'bash'").Scan(&toolCount)
	if llmCount != 1 || toolCount != 1 {
		t.Fatalf("counts: llm=%d tool=%d (want 1/1)", llmCount, toolCount)
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
		t.Fatalf("session cost should be > 0 (from explicit cost_usd), got %v", cost)
	}

	var inputTokens, toolCallCount int
	_ = s.DB().QueryRow("SELECT total_input_tokens FROM sessions WHERE external_id = 'opencode-integration'").Scan(&inputTokens)
	_ = s.DB().QueryRow("SELECT tool_call_count FROM sessions WHERE external_id = 'opencode-integration'").Scan(&toolCallCount)
	if inputTokens != 1000 {
		t.Fatalf("session input tokens: got %d, want 1000", inputTokens)
	}
	if toolCallCount != 1 {
		t.Fatalf("session tool_call_count: got %d, want 1", toolCallCount)
	}
}
