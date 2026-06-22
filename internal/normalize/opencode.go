package normalize

import (
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	colmetricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
)

type OpenCodeNormalizer struct{}

func (n *OpenCodeNormalizer) Agent() string { return "opencode" }

func (n *OpenCodeNormalizer) NormalizeTraces(req *coltracepb.ExportTraceServiceRequest) ([]Event, error) {
	var events []Event
	for _, rs := range req.ResourceSpans {
		projectPath := firstAttrString(rs.Resource.GetAttributes(), "hcli.project_path", "project.path", "opencode.project.path", "process.cwd")
		for _, ss := range rs.ScopeSpans {
			for _, span := range ss.Spans {
				if e, ok := n.traceSpanToEvent(span, projectPath); ok {
					events = append(events, e)
				}
			}
		}
	}
	return events, nil
}

func (n *OpenCodeNormalizer) traceSpanToEvent(span *tracepb.Span, projectPath string) (Event, bool) {
	if !strings.HasPrefix(span.Name, "opencode.") {
		return Event{}, false
	}
	sessionID := firstAttrString(span.Attributes, "session.id", "opencode.session.id")
	base := Event{Timestamp: time.Unix(0, int64(span.StartTimeUnixNano)), Agent: "opencode", SessionID: sessionID, ProjectPath: projectPath}

	spanKind := firstAttrString(span.Attributes, "openinference.span.kind", "openinference_span_kind")
	if strings.Contains(span.Name, "llm") || strings.EqualFold(spanKind, "LLM") {
		base.Type = EventLLMCall
		base.LLMCall = &LLMCallData{
			TraceID:          hex.EncodeToString(span.TraceId),
			SpanID:           hex.EncodeToString(span.SpanId),
			ParentSpanID:     hex.EncodeToString(span.ParentSpanId),
			StartedAt:        time.Unix(0, int64(span.StartTimeUnixNano)),
			DurationMs:       int((span.EndTimeUnixNano - span.StartTimeUnixNano) / 1e6),
			Model:            firstAttrString(span.Attributes, "llm.model_name", "gen_ai.response.model", "model", "opencode.model"),
			Provider:         firstAttrString(span.Attributes, "llm.provider", "llm.system", "gen_ai.system", "provider", "opencode.provider"),
			InputTokens:      firstAttrInt(span.Attributes, "llm.token_count.prompt", "gen_ai.usage.input_tokens", "input_tokens"),
			OutputTokens:     firstAttrInt(span.Attributes, "llm.token_count.completion", "gen_ai.usage.output_tokens", "output_tokens"),
			CacheReadTokens:  firstAttrInt(span.Attributes, "llm.token_count.prompt_details.cache_read", "gen_ai.usage.cache_read_input_tokens", "cache_read_tokens"),
			CacheWriteTokens: firstAttrInt(span.Attributes, "llm.token_count.prompt_details.cache_write", "gen_ai.usage.cache_creation_input_tokens", "cache_creation_tokens", "cache_write_tokens"),
			ReasoningTokens:  firstAttrInt(span.Attributes, "llm.token_count.completion_details.reasoning", "reasoning_tokens"),
			CostUSD:          firstAttrFloat(span.Attributes, "llm.cost.total", "cost_usd"),
			StopReason:       firstAttrString(span.Attributes, "llm.finish_reason", "gen_ai.response.finish_reasons"),
		}
		llm := base.LLMCall
		hasUsage := llm.InputTokens != 0 || llm.OutputTokens != 0 || llm.CacheReadTokens != 0 || llm.CacheWriteTokens != 0 || llm.ReasoningTokens != 0 || llm.CostUSD != 0
		return base, base.SessionID != "" && llm.Model != "" && hasUsage
	}

	if strings.Contains(span.Name, "tool") || strings.EqualFold(spanKind, "TOOL") {
		base.Type = EventToolCall
		base.ToolCall = &ToolCallData{
			TraceID:      hex.EncodeToString(span.TraceId),
			SpanID:       hex.EncodeToString(span.SpanId),
			ParentSpanID: hex.EncodeToString(span.ParentSpanId),
			StartedAt:    time.Unix(0, int64(span.StartTimeUnixNano)),
			DurationMs:   int((span.EndTimeUnixNano - span.StartTimeUnixNano) / 1e6),
			ToolName:     firstAttrString(span.Attributes, "tool.name", "gen_ai.tool.name", "opencode.tool.name"),
			Success:      firstAttrBool(span.Attributes, "tool.success", "success"),
			ErrorMessage: firstAttrString(span.Attributes, "tool.error", "error"),
		}
		return base, base.SessionID != "" && base.ToolCall.ToolName != ""
	}

	return Event{}, false
}

func (n *OpenCodeNormalizer) NormalizeMetrics(*colmetricspb.ExportMetricsServiceRequest) ([]Event, error) {
	return nil, nil
}

func (n *OpenCodeNormalizer) NormalizeLogs(req *collogspb.ExportLogsServiceRequest) ([]Event, error) {
	var events []Event
	if req == nil {
		return events, nil
	}
	for _, rl := range req.ResourceLogs {
		if rl == nil {
			continue
		}
		projectPath := firstSafeAttrString(rl.Resource.GetAttributes(), "hcli.project_path", "project.path", "opencode.project.path", "process.cwd")
		for _, sl := range rl.ScopeLogs {
			if sl == nil {
				continue
			}
			for _, lr := range sl.LogRecords {
				if lr == nil {
					continue
				}
				base, ok := n.logRecordToEvent(lr, projectPath)
				if !ok {
					continue
				}
				events = append(events, base)
			}
		}
	}
	return events, nil
}

func (n *OpenCodeNormalizer) logRecordToEvent(lr *logspb.LogRecord, projectPath string) (Event, bool) {
	eventName := firstSafeAttrString(lr.Attributes, "event.name")
	sessionID := firstSafeAttrString(lr.Attributes, "session.id", "opencode.session.id")
	if eventName == "" || sessionID == "" {
		return Event{}, false
	}
	if strings.HasPrefix(eventName, "claude_code.") {
		return Event{}, false
	}

	return Event{
		Type:        EventLog,
		Timestamp:   time.Unix(0, int64(lr.TimeUnixNano)),
		Agent:       "opencode",
		SessionID:   sessionID,
		ProjectPath: projectPath,
		Log: &LogData{
			TraceID: hex.EncodeToString(lr.TraceId),
			SpanID:  hex.EncodeToString(lr.SpanId),
			Name:    eventName,
			Payload: opencodeLogPayload(lr.Body, lr.Attributes),
		},
	}, true
}

func opencodeLogPayload(body *commonpb.AnyValue, attrs []*commonpb.KeyValue) []byte {
	payload := map[string]any{
		"body":       opencodeBodyValue(body),
		"attributes": opencodeAttrs(attrs),
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return []byte("{}")
	}
	return data
}

func opencodeAttrs(attrs []*commonpb.KeyValue) map[string]any {
	values := make(map[string]any)
	for _, kv := range attrs {
		if kv == nil || kv.Value == nil {
			continue
		}
		v := opencodeAnyValue(kv.Value)
		if v == nil {
			continue
		}
		values[kv.Key] = v
	}
	return values
}

func opencodeBodyValue(value *commonpb.AnyValue) any {
	if value == nil || value.Value == nil {
		return ""
	}
	return opencodeAnyValue(value)
}

func opencodeAnyValue(value *commonpb.AnyValue) any {
	if value == nil {
		return nil
	}
	switch v := value.Value.(type) {
	case *commonpb.AnyValue_StringValue:
		return v.StringValue
	case *commonpb.AnyValue_IntValue:
		return v.IntValue
	case *commonpb.AnyValue_DoubleValue:
		return v.DoubleValue
	case *commonpb.AnyValue_BoolValue:
		return v.BoolValue
	default:
		return nil
	}
}

func firstSafeAttrString(attrs []*commonpb.KeyValue, keys ...string) string {
	for _, key := range keys {
		for _, kv := range attrs {
			if kv != nil && kv.Value != nil && kv.Key == key {
				return kv.Value.GetStringValue()
			}
		}
	}
	return ""
}

func firstAttrString(attrs []*commonpb.KeyValue, keys ...string) string {
	for _, key := range keys {
		if v := attrString(attrs, key); v != "" {
			return v
		}
	}
	return ""
}

func firstAttrInt(attrs []*commonpb.KeyValue, keys ...string) int {
	for _, key := range keys {
		if v := attrInt(attrs, key); v != 0 {
			return v
		}
	}
	return 0
}

func firstAttrBool(attrs []*commonpb.KeyValue, keys ...string) bool {
	for _, key := range keys {
		if attrBool(attrs, key) {
			return true
		}
	}
	return false
}

func firstAttrFloat(attrs []*commonpb.KeyValue, keys ...string) float64 {
	for _, key := range keys {
		for _, kv := range attrs {
			if kv.Key == key {
				return kv.Value.GetDoubleValue()
			}
		}
	}
	return 0
}
