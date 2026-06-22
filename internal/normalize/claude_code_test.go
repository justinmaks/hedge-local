package normalize

import (
	"testing"
	"time"

	collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	colmetricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
)

func makeSpan(name, spanID, parentID, sessionID string, kind tracepb.Span_SpanKind, attrs map[string]any) *tracepb.Span {
	span := &tracepb.Span{
		Name:              name,
		SpanId:            []byte(spanID),
		Kind:              kind,
		StartTimeUnixNano: uint64(time.Now().UnixNano()),
		EndTimeUnixNano:   uint64(time.Now().Add(1 * time.Second).UnixNano()),
	}
	if parentID != "" {
		span.ParentSpanId = []byte(parentID)
	}
	for k, v := range attrs {
		attr := &commonpb.KeyValue{Key: k}
		switch val := v.(type) {
		case string:
			attr.Value = &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: val}}
		case int64:
			attr.Value = &commonpb.AnyValue{Value: &commonpb.AnyValue_IntValue{IntValue: val}}
		case int:
			attr.Value = &commonpb.AnyValue{Value: &commonpb.AnyValue_IntValue{IntValue: int64(val)}}
		case float64:
			attr.Value = &commonpb.AnyValue{Value: &commonpb.AnyValue_DoubleValue{DoubleValue: val}}
		case bool:
			attr.Value = &commonpb.AnyValue{Value: &commonpb.AnyValue_BoolValue{BoolValue: val}}
		}
		span.Attributes = append(span.Attributes, attr)
	}
	if sessionID != "" {
		span.Attributes = append(span.Attributes, &commonpb.KeyValue{
			Key:   "session.id",
			Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: sessionID}},
		})
	}
	return span
}

func makeTraceReq(spans ...*tracepb.Span) *coltracepb.ExportTraceServiceRequest {
	var scopeSpans []*tracepb.ScopeSpans
	scopeSpans = append(scopeSpans, &tracepb.ScopeSpans{Spans: spans})
	return &coltracepb.ExportTraceServiceRequest{
		ResourceSpans: []*tracepb.ResourceSpans{{ScopeSpans: scopeSpans}},
	}
}

func TestNormalizeTraces_llmRequest(t *testing.T) {
	n := &ClaudeCodeNormalizer{}
	span := makeSpan("claude_code.llm_request", "span1", "", "sess-123", tracepb.Span_SPAN_KIND_INTERNAL, map[string]any{
		"gen_ai.response.model":                    "claude-sonnet-4-20250514",
		"gen_ai.usage.input_tokens":                int64(1200),
		"gen_ai.usage.output_tokens":               int64(340),
		"gen_ai.usage.cache_read_input_tokens":     int64(500),
		"gen_ai.usage.cache_creation_input_tokens": int64(100),
		"gen_ai.response.finish_reasons":           "end_turn",
	})
	req := makeTraceReq(span)
	events, err := n.NormalizeTraces(req)
	if err != nil {
		t.Fatalf("NormalizeTraces: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != EventLLMCall {
		t.Errorf("expected EventLLMCall, got %v", events[0].Type)
	}
	if events[0].Agent != "claude_code" {
		t.Errorf("expected agent claude_code, got %q", events[0].Agent)
	}
	if events[0].SessionID != "sess-123" {
		t.Errorf("expected session sess-123, got %q", events[0].SessionID)
	}
	llm := events[0].LLMCall
	if llm == nil {
		t.Fatal("expected LLMCall data")
	}
	if llm.Model != "claude-sonnet-4-20250514" {
		t.Errorf("model: got %q", llm.Model)
	}
	if llm.InputTokens != 1200 {
		t.Errorf("input tokens: got %d, want 1200", llm.InputTokens)
	}
	if llm.OutputTokens != 340 {
		t.Errorf("output tokens: got %d, want 340", llm.OutputTokens)
	}
	if llm.CacheReadTokens != 500 {
		t.Errorf("cache read: got %d, want 500", llm.CacheReadTokens)
	}
	if llm.CacheWriteTokens != 100 {
		t.Errorf("cache write: got %d, want 100", llm.CacheWriteTokens)
	}
	if llm.StopReason != "end_turn" {
		t.Errorf("stop reason: got %q", llm.StopReason)
	}
}

func TestNormalizeTraces_toolExecution(t *testing.T) {
	n := &ClaudeCodeNormalizer{}
	span := makeSpan("claude_code.tool.execution", "span2", "span1", "sess-123", tracepb.Span_SPAN_KIND_INTERNAL, map[string]any{
		"gen_ai.tool.name": "bash",
		"success":          true,
	})
	req := makeTraceReq(span)
	events, err := n.NormalizeTraces(req)
	if err != nil {
		t.Fatalf("NormalizeTraces: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != EventToolCall {
		t.Fatalf("expected EventToolCall, got %v", events[0].Type)
	}
	tc := events[0].ToolCall
	if tc == nil {
		t.Fatal("expected ToolCall data")
	}
	if tc.ToolName != "bash" {
		t.Errorf("tool name: got %q, want bash", tc.ToolName)
	}
	if !tc.Success {
		t.Error("expected success=true")
	}
}

func TestNormalizeTraces_currentClaudeLLMAttributes(t *testing.T) {
	n := &ClaudeCodeNormalizer{}
	span := makeSpan("claude_code.llm_request", "span1", "", "sess-current", tracepb.Span_SPAN_KIND_INTERNAL, map[string]any{
		"model":                 "claude-sonnet-4-5",
		"input_tokens":          int64(1200),
		"output_tokens":         int64(340),
		"cache_read_tokens":     int64(500),
		"cache_creation_tokens": int64(100),
		"ttft_ms":               int64(250),
		"stop_reason":           "tool_use",
	})

	events, err := n.NormalizeTraces(makeTraceReq(span))
	if err != nil {
		t.Fatalf("NormalizeTraces: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	llm := events[0].LLMCall
	if llm.Model != "claude-sonnet-4-5" {
		t.Fatalf("model: got %q", llm.Model)
	}
	if llm.InputTokens != 1200 || llm.OutputTokens != 340 || llm.CacheReadTokens != 500 || llm.CacheWriteTokens != 100 {
		t.Fatalf("tokens: got input=%d output=%d cacheRead=%d cacheWrite=%d", llm.InputTokens, llm.OutputTokens, llm.CacheReadTokens, llm.CacheWriteTokens)
	}
	if llm.TTFTMs != 250 || llm.StopReason != "tool_use" {
		t.Fatalf("ttft/stop: got %d/%q", llm.TTFTMs, llm.StopReason)
	}
}

func TestNormalizeTraces_currentClaudeToolSpan(t *testing.T) {
	n := &ClaudeCodeNormalizer{}
	span := makeSpan("claude_code.tool", "span2", "span1", "sess-current", tracepb.Span_SPAN_KIND_INTERNAL, map[string]any{
		"tool_name":   "mcp__supabase__execute_sql",
		"duration_ms": int64(42),
	})

	events, err := n.NormalizeTraces(makeTraceReq(span))
	if err != nil {
		t.Fatalf("NormalizeTraces: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	tool := events[0].ToolCall
	if tool.ToolName != "mcp__supabase__execute_sql" {
		t.Fatalf("tool name: got %q", tool.ToolName)
	}
	if tool.DurationMs != 42 {
		t.Fatalf("duration: got %d, want 42", tool.DurationMs)
	}
}

// Claude Code emits the same LLM call via both a trace span and the
// claude_code.cost.usage / token.usage metrics. Those two streams cannot be
// reliably joined (metrics carry no span_id and their timestamps don't match
// span start times), so traces are the single source of truth for llm_calls and
// metrics must NOT create rows — otherwise every call is double-counted.
func TestNormalizeMetrics_doesNotCreateLLMCalls(t *testing.T) {
	n := &ClaudeCodeNormalizer{}
	now := uint64(time.Now().UnixNano())
	attrs := []*commonpb.KeyValue{
		{Key: "session.id", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "sess-metrics"}}},
		{Key: "model", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "claude-sonnet-4-5"}}},
	}
	req := &colmetricspb.ExportMetricsServiceRequest{ResourceMetrics: []*metricspb.ResourceMetrics{{
		Resource: &resourcepb.Resource{Attributes: []*commonpb.KeyValue{
			{Key: "hcli.project_path", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "/tmp/project"}}},
		}},
		ScopeMetrics: []*metricspb.ScopeMetrics{{Metrics: []*metricspb.Metric{
			{
				Name: "claude_code.cost.usage",
				Data: &metricspb.Metric_Sum{Sum: &metricspb.Sum{DataPoints: []*metricspb.NumberDataPoint{{
					TimeUnixNano: now,
					Attributes:   attrs,
					Value:        &metricspb.NumberDataPoint_AsDouble{AsDouble: 0.0123},
				}}}},
			},
			claudeTokenMetric(now, attrs, "input", 1000),
			claudeTokenMetric(now, attrs, "output", 500),
		}}},
	}}}

	events, err := n.NormalizeMetrics(req)
	if err != nil {
		t.Fatalf("NormalizeMetrics: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("expected 0 events (traces are canonical for llm_calls), got %d", len(events))
	}
}

func claudeTokenMetric(ts uint64, baseAttrs []*commonpb.KeyValue, typ string, value int64) *metricspb.Metric {
	attrs := append([]*commonpb.KeyValue{}, baseAttrs...)
	attrs = append(attrs, &commonpb.KeyValue{Key: "type", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: typ}}})
	return &metricspb.Metric{
		Name: "claude_code.token.usage",
		Data: &metricspb.Metric_Sum{Sum: &metricspb.Sum{DataPoints: []*metricspb.NumberDataPoint{{
			TimeUnixNano: ts,
			Attributes:   attrs,
			Value:        &metricspb.NumberDataPoint_AsInt{AsInt: value},
		}}}},
	}
}

func TestNormalizeLogs_toolResult(t *testing.T) {
	n := &ClaudeCodeNormalizer{}
	logReq := &collogspb.ExportLogsServiceRequest{
		ResourceLogs: []*logspb.ResourceLogs{{
			ScopeLogs: []*logspb.ScopeLogs{{
				LogRecords: []*logspb.LogRecord{{
					TimeUnixNano: uint64(time.Now().UnixNano()),
					Body: &commonpb.AnyValue{
						Value: &commonpb.AnyValue_StringValue{StringValue: `{"tool":"bash","success":true}`},
					},
					Attributes: []*commonpb.KeyValue{
						{Key: "event.name", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "claude_code.tool_result"}}},
						{Key: "session.id", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "sess-456"}}},
					},
				}},
			}},
		}},
	}
	events, err := n.NormalizeLogs(logReq)
	if err != nil {
		t.Fatalf("NormalizeLogs: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != EventLog {
		t.Errorf("expected EventLog, got %v", events[0].Type)
	}
	if events[0].SessionID != "sess-456" {
		t.Errorf("session: got %q, want sess-456", events[0].SessionID)
	}
	if events[0].Log == nil {
		t.Fatal("expected Log data")
	}
	if events[0].Log.Name != "claude_code.tool_result" {
		t.Errorf("event name: got %q", events[0].Log.Name)
	}
}

func TestNormalizeLogs_ignoresOpenCodeStyleLogs(t *testing.T) {
	n := &ClaudeCodeNormalizer{}
	logReq := &collogspb.ExportLogsServiceRequest{
		ResourceLogs: []*logspb.ResourceLogs{{
			ScopeLogs: []*logspb.ScopeLogs{{
				LogRecords: []*logspb.LogRecord{{
					TimeUnixNano: uint64(time.Now().UnixNano()),
					Body: &commonpb.AnyValue{
						Value: &commonpb.AnyValue_StringValue{StringValue: "api_request"},
					},
					Attributes: []*commonpb.KeyValue{
						{Key: "event.name", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "api_request"}}},
						{Key: "session.id", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "sess-o"}}},
					},
				}},
			}},
		}},
	}
	events, err := n.NormalizeLogs(logReq)
	if err != nil {
		t.Fatalf("NormalizeLogs: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("expected no events for non-claude_code log, got %#v", events)
	}
}

func TestNormalizeLogs_ignoresLogsWithoutSessionID(t *testing.T) {
	n := &ClaudeCodeNormalizer{}
	logReq := &collogspb.ExportLogsServiceRequest{
		ResourceLogs: []*logspb.ResourceLogs{{
			ScopeLogs: []*logspb.ScopeLogs{{
				LogRecords: []*logspb.LogRecord{{
					TimeUnixNano: uint64(time.Now().UnixNano()),
					Attributes: []*commonpb.KeyValue{
						{Key: "event.name", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "claude_code.tool_result"}}},
					},
				}},
			}},
		}},
	}
	events, err := n.NormalizeLogs(logReq)
	if err != nil {
		t.Fatalf("NormalizeLogs: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("expected no events without session id, got %#v", events)
	}
}
