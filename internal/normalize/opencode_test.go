package normalize

import (
	"testing"
	"time"

	collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
)

// OpenCode telemetry is canonical via log events (api_request / tool_result),
// which the @devtheops plugin reliably delivers with full token/cost data.
// Trace spans are NOT used to create rows — deriving from both would
// double-count, and in practice the LLM/tool spans are not reliably exported
// (e.g. in `opencode run`). So NormalizeTraces must yield no row events.
func TestOpenCodeNormalizeTraces_yieldsNoRows(t *testing.T) {
	n := &OpenCodeNormalizer{}
	llmSpan := makeSpan("opencode.llm", "ospan1", "", "", tracepb.Span_SPAN_KIND_CLIENT, map[string]any{
		"session.id":             "open-sess",
		"llm.model_name":         "gpt-4.1",
		"llm.token_count.prompt": int64(1000),
		"llm.cost.total":         0.012,
	})
	toolSpan := makeSpan("opencode.tool.bash", "toolspan", "", "", tracepb.Span_SPAN_KIND_INTERNAL, map[string]any{
		"session.id": "open-sess",
		"tool.name":  "bash",
	})
	events, err := n.NormalizeTraces(makeTraceReq(llmSpan, toolSpan))
	if err != nil {
		t.Fatalf("NormalizeTraces: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("expected 0 events (logs are canonical for opencode), got %d: %#v", len(events), events)
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

func TestOpenCodeNormalizeLogs_apiRequestBecomesLLMCall(t *testing.T) {
	n := &OpenCodeNormalizer{}
	req := makeOpenCodeLogReq(&logspb.LogRecord{
		TimeUnixNano: uint64(time.Now().UnixNano()),
		TraceId:      []byte("trace-log"),
		SpanId:       []byte("span-log"),
		Body:         &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "api_request"}},
		Attributes: []*commonpb.KeyValue{
			{Key: "event.name", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "api_request"}}},
			{Key: "session.id", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "open-log-sess"}}},
			{Key: "model", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "google/gemini-2.5-flash-lite"}}},
			{Key: "provider", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "vercel"}}},
			{Key: "input_tokens", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_IntValue{IntValue: 100}}},
			{Key: "output_tokens", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_IntValue{IntValue: 25}}},
			{Key: "cache_read_tokens", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_IntValue{IntValue: 8}}},
			{Key: "cache_creation_tokens", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_IntValue{IntValue: 4}}},
			{Key: "reasoning_tokens", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_IntValue{IntValue: 12}}},
			{Key: "cost_usd", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_DoubleValue{DoubleValue: 0.0042}}},
			{Key: "duration_ms", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_IntValue{IntValue: 700}}},
		},
	})
	events, err := n.NormalizeLogs(req)
	if err != nil {
		t.Fatalf("NormalizeLogs: %v", err)
	}
	if len(events) != 1 || events[0].Type != EventLLMCall || events[0].LLMCall == nil {
		t.Fatalf("expected one llm_call event: %#v", events)
	}
	e := events[0]
	if e.Agent != "opencode" || e.SessionID != "open-log-sess" {
		t.Fatalf("base event: %#v", e)
	}
	llm := e.LLMCall
	if llm.Model != "google/gemini-2.5-flash-lite" || llm.Provider != "vercel" {
		t.Fatalf("model/provider: %#v", llm)
	}
	if llm.InputTokens != 100 || llm.OutputTokens != 25 || llm.CacheReadTokens != 8 || llm.CacheWriteTokens != 4 || llm.ReasoningTokens != 12 {
		t.Fatalf("tokens: %#v", llm)
	}
	if llm.CostUSD != 0.0042 || llm.DurationMs != 700 {
		t.Fatalf("cost/duration: %#v", llm)
	}
}

func TestOpenCodeNormalizeLogs_toolResultBecomesToolCall(t *testing.T) {
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
	if len(events) != 1 || events[0].Type != EventToolCall || events[0].ToolCall == nil {
		t.Fatalf("expected one tool_call event: %#v", events)
	}
	tc := events[0].ToolCall
	if events[0].SessionID != "open-tool-sess" || tc.ToolName != "bash" || !tc.Success || tc.DurationMs != 33 {
		t.Fatalf("tool data: %#v", tc)
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
