package normalize

import (
	"encoding/hex"
	"strings"
	"time"

	collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	colmetricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
)

type ClaudeCodeNormalizer struct{}

func (n *ClaudeCodeNormalizer) Agent() string { return "claude_code" }

func (n *ClaudeCodeNormalizer) NormalizeTraces(req *coltracepb.ExportTraceServiceRequest) ([]Event, error) {
	var events []Event
	for _, rs := range req.ResourceSpans {
		projectPath := attrString(rs.Resource.GetAttributes(), "hcli.project_path")
		for _, ss := range rs.ScopeSpans {
			for _, span := range ss.Spans {
				e, ok := n.traceSpanToEvent(span, projectPath)
				if ok {
					events = append(events, e)
				}
			}
		}
	}
	return events, nil
}

func (n *ClaudeCodeNormalizer) traceSpanToEvent(span *tracepb.Span, projectPath string) (Event, bool) {
	sessionID := attrString(span.Attributes, "session.id")
	base := Event{
		Timestamp:   time.Unix(0, int64(span.StartTimeUnixNano)),
		Agent:       "claude_code",
		SessionID:   sessionID,
		ProjectPath: projectPath,
	}

	switch span.Name {
	case "claude_code.llm_request":
		llm := &LLMCallData{
			TraceID:          hex.EncodeToString(span.TraceId),
			SpanID:           hex.EncodeToString(span.SpanId),
			ParentSpanID:     hex.EncodeToString(span.ParentSpanId),
			StartedAt:        time.Unix(0, int64(span.StartTimeUnixNano)),
			DurationMs:       int((span.EndTimeUnixNano - span.StartTimeUnixNano) / 1e6),
			Model:            firstAttrString(span.Attributes, "model", "gen_ai.request.model", "gen_ai.response.model"),
			Provider:         firstAttrString(span.Attributes, "gen_ai.system", "provider"),
			InputTokens:      firstAttrInt(span.Attributes, "input_tokens", "gen_ai.usage.input_tokens"),
			OutputTokens:     firstAttrInt(span.Attributes, "output_tokens", "gen_ai.usage.output_tokens"),
			CacheReadTokens:  firstAttrInt(span.Attributes, "cache_read_tokens", "gen_ai.usage.cache_read_input_tokens"),
			CacheWriteTokens: firstAttrInt(span.Attributes, "cache_creation_tokens", "gen_ai.usage.cache_creation_input_tokens"),
			TTFTMs:           firstAttrInt(span.Attributes, "ttft_ms"),
			StopReason:       firstAttrString(span.Attributes, "stop_reason", "gen_ai.response.finish_reasons"),
		}
		if llm.Provider == "" {
			llm.Provider = "anthropic"
		}
		base.Type = EventLLMCall
		base.LLMCall = llm
		return base, true

	case "claude_code.tool", "claude_code.tool.execution":
		tc := &ToolCallData{
			TraceID:      hex.EncodeToString(span.TraceId),
			SpanID:       hex.EncodeToString(span.SpanId),
			ParentSpanID: hex.EncodeToString(span.ParentSpanId),
			StartedAt:    time.Unix(0, int64(span.StartTimeUnixNano)),
			DurationMs:   firstAttrInt(span.Attributes, "duration_ms"),
			ToolName:     firstAttrString(span.Attributes, "tool_name", "gen_ai.tool.name"),
			Success:      attrBool(span.Attributes, "success"),
			ErrorMessage: firstAttrString(span.Attributes, "error"),
		}
		if tc.DurationMs == 0 {
			tc.DurationMs = int((span.EndTimeUnixNano - span.StartTimeUnixNano) / 1e6)
		}
		base.Type = EventToolCall
		base.ToolCall = tc
		return base, tc.ToolName != ""

	default:
		return base, false
	}
}

// NormalizeMetrics intentionally produces no events. Claude Code reports the
// same LLM usage through both trace spans and the claude_code.cost.usage /
// token.usage metrics. The two streams share no per-call key (metrics have no
// span_id and their timestamps don't line up with span start times), so traces
// are the single source of truth for llm_calls. Deriving rows from metrics as
// well would double-count every call. Per-call cost comes from the pricing
// table applied to the trace-derived tokens in the writer.
func (n *ClaudeCodeNormalizer) NormalizeMetrics(req *colmetricspb.ExportMetricsServiceRequest) ([]Event, error) {
	return nil, nil
}

func (n *ClaudeCodeNormalizer) NormalizeLogs(req *collogspb.ExportLogsServiceRequest) ([]Event, error) {
	var events []Event
	for _, rl := range req.ResourceLogs {
		projectPath := attrString(rl.Resource.GetAttributes(), "hcli.project_path")
		for _, sl := range rl.ScopeLogs {
			for _, lr := range sl.LogRecords {
				e, ok := n.logRecordToEvent(lr, projectPath)
				if ok {
					events = append(events, e)
				}
			}
		}
	}
	return events, nil
}

func (n *ClaudeCodeNormalizer) logRecordToEvent(lr *logspb.LogRecord, projectPath string) (Event, bool) {
	eventName := attrString(lr.Attributes, "event.name")
	if eventName == "" {
		return Event{}, false
	}
	if !strings.HasPrefix(eventName, "claude_code.") {
		return Event{}, false
	}
	sessionID := attrString(lr.Attributes, "session.id")
	if sessionID == "" {
		return Event{}, false
	}
	payload := []byte(lr.Body.GetStringValue())
	if len(payload) == 0 {
		payload = []byte("{}")
	}
	return Event{
		Type:        EventLog,
		Timestamp:   time.Unix(0, int64(lr.TimeUnixNano)),
		Agent:       "claude_code",
		SessionID:   sessionID,
		ProjectPath: projectPath,
		Log: &LogData{
			TraceID: hex.EncodeToString(lr.TraceId),
			SpanID:  hex.EncodeToString(lr.SpanId),
			Name:    eventName,
			Payload: payload,
		},
	}, true
}

func attrString(attrs []*commonpb.KeyValue, key string) string {
	for _, kv := range attrs {
		if kv.Key == key {
			return kv.Value.GetStringValue()
		}
	}
	return ""
}

func attrInt(attrs []*commonpb.KeyValue, key string) int {
	for _, kv := range attrs {
		if kv.Key == key {
			return int(kv.Value.GetIntValue())
		}
	}
	return 0
}

func attrBool(attrs []*commonpb.KeyValue, key string) bool {
	for _, kv := range attrs {
		if kv.Key == key {
			return kv.Value.GetBoolValue()
		}
	}
	return false
}
