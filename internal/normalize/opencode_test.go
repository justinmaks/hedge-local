package normalize

import (
	"encoding/json"
	"testing"
	"time"

	collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
)

func TestOpenCodeNormalizeTraces_llmSpan(t *testing.T) {
	n := &OpenCodeNormalizer{}
	span := makeSpan("opencode.llm", "ospan1", "", "", tracepb.Span_SPAN_KIND_CLIENT, map[string]any{
		"session.id":                                   "open-sess",
		"llm.model_name":                               "gpt-4.1",
		"llm.provider":                                 "openai",
		"llm.token_count.prompt":                       int64(1000),
		"llm.token_count.completion":                   int64(300),
		"llm.token_count.prompt_details.cache_read":    int64(50),
		"llm.token_count.prompt_details.cache_write":   int64(25),
		"llm.token_count.completion_details.reasoning": int64(10),
		"llm.cost.total":                               0.012,
		"llm.finish_reason":                            "stop",
	})
	req := makeTraceReq(span)
	req.ResourceSpans[0].Resource = &resourcepb.Resource{Attributes: []*commonpb.KeyValue{
		{Key: "hcli.project_path", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "/tmp/open"}}},
	}}
	events, err := n.NormalizeTraces(req)
	if err != nil {
		t.Fatalf("NormalizeTraces: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events: got %d, want 1", len(events))
	}
	e := events[0]
	if e.Agent != "opencode" || e.SessionID != "open-sess" || e.ProjectPath != "/tmp/open" {
		t.Fatalf("unexpected base event: %#v", e)
	}
	if e.Type != EventLLMCall || e.LLMCall == nil {
		t.Fatalf("expected llm event: %#v", e)
	}
	if e.LLMCall.Model != "gpt-4.1" || e.LLMCall.Provider != "openai" {
		t.Fatalf("model/provider: %#v", e.LLMCall)
	}
	if e.LLMCall.InputTokens != 1000 || e.LLMCall.OutputTokens != 300 || e.LLMCall.CacheReadTokens != 50 || e.LLMCall.CacheWriteTokens != 25 || e.LLMCall.ReasoningTokens != 10 {
		t.Fatalf("tokens: %#v", e.LLMCall)
	}
	if e.LLMCall.CostUSD != 0.012 {
		t.Fatalf("cost: got %v, want 0.012", e.LLMCall.CostUSD)
	}
}

func TestOpenCodeNormalizeTraces_toolSpan(t *testing.T) {
	n := &OpenCodeNormalizer{}
	span := makeSpan("opencode.tool.bash", "toolspan", "ospan1", "", tracepb.Span_SPAN_KIND_INTERNAL, map[string]any{
		"session.id":   "open-sess",
		"tool.name":    "bash",
		"tool.success": true,
	})
	events, err := n.NormalizeTraces(makeTraceReq(span))
	if err != nil {
		t.Fatalf("NormalizeTraces: %v", err)
	}
	if len(events) != 1 || events[0].Type != EventToolCall || events[0].ToolCall == nil {
		t.Fatalf("expected one tool event: %#v", events)
	}
	if events[0].ToolCall.ToolName != "bash" || !events[0].ToolCall.Success {
		t.Fatalf("tool data: %#v", events[0].ToolCall)
	}
}

func TestOpenCodeNormalizeTraces_ignoresIncompleteLLMSpan(t *testing.T) {
	n := &OpenCodeNormalizer{}
	span := makeSpan("opencode.llm.metadata", "meta", "", "", tracepb.Span_SPAN_KIND_INTERNAL, map[string]any{
		"session.id": "open-sess",
	})
	events, err := n.NormalizeTraces(makeTraceReq(span))
	if err != nil {
		t.Fatalf("NormalizeTraces: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("expected no events, got %#v", events)
	}
}

func TestOpenCodeNormalizeTraces_ignoresClaudeSpan(t *testing.T) {
	n := &OpenCodeNormalizer{}
	span := makeSpan("claude_code.llm_request", "span1", "", "sess", tracepb.Span_SPAN_KIND_INTERNAL, map[string]any{})
	events, err := n.NormalizeTraces(makeTraceReq(span))
	if err != nil {
		t.Fatalf("NormalizeTraces: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("expected no events, got %#v", events)
	}
}

func makeOpenCodeLogReq(records ...*logspb.LogRecord) *collogspb.ExportLogsServiceRequest {
	return &collogspb.ExportLogsServiceRequest{
		ResourceLogs: []*logspb.ResourceLogs{{
			ScopeLogs: []*logspb.ScopeLogs{{
				LogRecords: records,
			}},
		}},
	}
}

func openCodeLogPayload(t *testing.T, log *LogData) map[string]any {
	t.Helper()
	if log == nil {
		t.Fatal("expected log data")
	}
	var payload map[string]any
	if err := json.Unmarshal(log.Payload, &payload); err != nil {
		t.Fatalf("payload JSON: %v", err)
	}
	return payload
}

func payloadAttrs(t *testing.T, payload map[string]any) map[string]any {
	t.Helper()
	attrs, ok := payload["attributes"].(map[string]any)
	if !ok {
		t.Fatalf("payload attributes: %#v", payload)
	}
	return attrs
}

func TestOpenCodeNormalizeLogs_apiRequestPersistsLogOnly(t *testing.T) {
	n := &OpenCodeNormalizer{}
	req := makeOpenCodeLogReq(&logspb.LogRecord{
		TimeUnixNano: uint64(time.Now().UnixNano()),
		TraceId:      []byte("trace-log"),
		SpanId:       []byte("span-log"),
		Body:         &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "api_request"}},
		Attributes: []*commonpb.KeyValue{
			{Key: "event.name", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "api_request"}}},
			{Key: "session.id", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "open-log-sess"}}},
			{Key: "model", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "gpt-4.1"}}},
			{Key: "provider", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "openai"}}},
			{Key: "input_tokens", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_IntValue{IntValue: 100}}},
			{Key: "output_tokens", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_IntValue{IntValue: 25}}},
			{Key: "cost_usd", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_DoubleValue{DoubleValue: 0.0042}}},
			{Key: "duration_ms", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_IntValue{IntValue: 700}}},
		},
	})
	events, err := n.NormalizeLogs(req)
	if err != nil {
		t.Fatalf("NormalizeLogs: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events: got %d, want 1", len(events))
	}
	if events[0].Type != EventLog {
		t.Fatalf("unexpected event order/types: %#v", events)
	}
	if events[0].SessionID != "open-log-sess" || events[0].Log == nil || events[0].Log.Name != "api_request" {
		t.Fatalf("log data: %#v", events[0])
	}
	payload := openCodeLogPayload(t, events[0].Log)
	if payload["body"] != "api_request" {
		t.Fatalf("payload body: %#v", payload)
	}
	attrs := payloadAttrs(t, payload)
	if attrs["model"] != "gpt-4.1" || attrs["cost_usd"] != 0.0042 || attrs["input_tokens"] != float64(100) || attrs["output_tokens"] != float64(25) {
		t.Fatalf("payload attrs: %#v", attrs)
	}
}

func TestOpenCodeNormalizeLogs_toolResultPersistsLogOnly(t *testing.T) {
	n := &OpenCodeNormalizer{}
	req := makeOpenCodeLogReq(&logspb.LogRecord{
		TimeUnixNano: uint64(time.Now().UnixNano()),
		Body:         &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "tool_result"}},
		Attributes: []*commonpb.KeyValue{
			{Key: "event.name", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "tool_result"}}},
			{Key: "session.id", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "open-tool-sess"}}},
			{Key: "tool_name", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "bash"}}},
			{Key: "success", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_BoolValue{BoolValue: true}}},
			{Key: "duration_ms", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_IntValue{IntValue: 33}}},
		},
	})
	events, err := n.NormalizeLogs(req)
	if err != nil {
		t.Fatalf("NormalizeLogs: %v", err)
	}
	if len(events) != 1 || events[0].Type != EventLog {
		t.Fatalf("unexpected events: %#v", events)
	}
	if events[0].SessionID != "open-tool-sess" || events[0].Log == nil || events[0].Log.Name != "tool_result" {
		t.Fatalf("log data: %#v", events[0])
	}
	attrs := payloadAttrs(t, openCodeLogPayload(t, events[0].Log))
	if attrs["tool_name"] != "bash" || attrs["success"] != true || attrs["duration_ms"] != float64(33) {
		t.Fatalf("payload attrs: %#v", attrs)
	}
}

func TestOpenCodeNormalizeLogs_sessionCreatedPersistsLog(t *testing.T) {
	n := &OpenCodeNormalizer{}
	req := makeOpenCodeLogReq(&logspb.LogRecord{
		TimeUnixNano: uint64(time.Now().UnixNano()),
		Body:         &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "session.created"}},
		Attributes: []*commonpb.KeyValue{
			{Key: "event.name", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "session.created"}}},
			{Key: "session.id", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "open-session"}}},
		},
	})
	events, err := n.NormalizeLogs(req)
	if err != nil {
		t.Fatalf("NormalizeLogs: %v", err)
	}
	if len(events) != 1 || events[0].Type != EventLog || events[0].Log == nil || events[0].Log.Name != "session.created" {
		t.Fatalf("unexpected events: %#v", events)
	}
}

func TestOpenCodeNormalizeLogs_ignoresEventWithoutSessionID(t *testing.T) {
	n := &OpenCodeNormalizer{}
	req := makeOpenCodeLogReq(&logspb.LogRecord{
		TimeUnixNano: uint64(time.Now().UnixNano()),
		Attributes: []*commonpb.KeyValue{
			{Key: "event.name", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "api_request"}}},
		},
	})
	events, err := n.NormalizeLogs(req)
	if err != nil {
		t.Fatalf("NormalizeLogs: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("expected no events, got %#v", events)
	}
}

func TestOpenCodeNormalizeLogs_ignoresClaudeCodeLogs(t *testing.T) {
	n := &OpenCodeNormalizer{}
	req := makeOpenCodeLogReq(&logspb.LogRecord{
		TimeUnixNano: uint64(time.Now().UnixNano()),
		Body:         &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: `{"tool":"bash"}`}},
		Attributes: []*commonpb.KeyValue{
			{Key: "event.name", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "claude_code.tool_result"}}},
			{Key: "session.id", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "sess-c"}}},
		},
	})
	events, err := n.NormalizeLogs(req)
	if err != nil {
		t.Fatalf("NormalizeLogs: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("expected no events for claude_code log, got %#v", events)
	}
}

func TestOpenCodeNormalizeLogs_handlesNilEntries(t *testing.T) {
	n := &OpenCodeNormalizer{}
	req := &collogspb.ExportLogsServiceRequest{ResourceLogs: []*logspb.ResourceLogs{
		nil,
		{ScopeLogs: []*logspb.ScopeLogs{
			nil,
			{LogRecords: []*logspb.LogRecord{
				nil,
				{
					TimeUnixNano: uint64(time.Now().UnixNano()),
					Attributes: []*commonpb.KeyValue{
						nil,
						{Key: "event.name", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "user_prompt"}}},
						{Key: "session.id", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "nil-safe-session"}}},
						{Key: "ignored", Value: nil},
					},
				},
			}},
		}},
	}}
	events, err := n.NormalizeLogs(req)
	if err != nil {
		t.Fatalf("NormalizeLogs: %v", err)
	}
	if len(events) != 1 || events[0].Type != EventLog || events[0].Log == nil || events[0].Log.Name != "user_prompt" {
		t.Fatalf("unexpected events: %#v", events)
	}
}
