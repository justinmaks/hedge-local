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
)

type OpenCodeNormalizer struct{}

func (n *OpenCodeNormalizer) Agent() string { return "opencode" }

// NormalizeTraces yields no events. OpenCode telemetry is canonical via the
// @devtheops plugin's log events (api_request / tool_result), which reliably
// carry token counts and explicit cost. Deriving rows from trace spans as well
// would double-count, and the LLM/tool spans are not reliably exported (e.g. in
// `opencode run`). See NormalizeLogs.
func (n *OpenCodeNormalizer) NormalizeTraces(*coltracepb.ExportTraceServiceRequest) ([]Event, error) {
	return nil, nil
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

	ts := time.Unix(0, int64(lr.TimeUnixNano))
	base := Event{Timestamp: ts, Agent: "opencode", SessionID: sessionID, ProjectPath: projectPath}

	switch eventName {
	case "api_request":
		// The plugin reports a completed LLM call here with full usage + cost.
		base.Type = EventLLMCall
		base.LLMCall = &LLMCallData{
			TraceID:          hex.EncodeToString(lr.TraceId),
			SpanID:           hex.EncodeToString(lr.SpanId),
			StartedAt:        ts,
			DurationMs:       firstSafeAttrInt(lr.Attributes, "duration_ms"),
			Model:            firstSafeAttrString(lr.Attributes, "model"),
			Provider:         firstSafeAttrString(lr.Attributes, "provider"),
			InputTokens:      firstSafeAttrInt(lr.Attributes, "input_tokens"),
			OutputTokens:     firstSafeAttrInt(lr.Attributes, "output_tokens"),
			CacheReadTokens:  firstSafeAttrInt(lr.Attributes, "cache_read_tokens"),
			CacheWriteTokens: firstSafeAttrInt(lr.Attributes, "cache_creation_tokens", "cache_write_tokens"),
			ReasoningTokens:  firstSafeAttrInt(lr.Attributes, "reasoning_tokens"),
			CostUSD:          firstSafeAttrFloat(lr.Attributes, "cost_usd"),
		}
		return base, base.LLMCall.Model != ""

	case "tool_result":
		base.Type = EventToolCall
		base.ToolCall = &ToolCallData{
			TraceID:      hex.EncodeToString(lr.TraceId),
			SpanID:       hex.EncodeToString(lr.SpanId),
			StartedAt:    ts,
			DurationMs:   firstSafeAttrInt(lr.Attributes, "duration_ms"),
			ToolName:     firstSafeAttrString(lr.Attributes, "tool_name"),
			Success:      firstSafeAttrBool(lr.Attributes, "success"),
			ErrorMessage: firstSafeAttrString(lr.Attributes, "error"),
		}
		return base, base.ToolCall.ToolName != ""

	default:
		// session.created, user_prompt, api_error, commit, etc. — stored raw
		// (only persisted when --with-logs is enabled).
		base.Type = EventLog
		base.Log = &LogData{
			TraceID: hex.EncodeToString(lr.TraceId),
			SpanID:  hex.EncodeToString(lr.SpanId),
			Name:    eventName,
			Payload: opencodeLogPayload(lr.Body, lr.Attributes),
		}
		return base, true
	}
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

func firstSafeAttrInt(attrs []*commonpb.KeyValue, keys ...string) int {
	for _, key := range keys {
		for _, kv := range attrs {
			if kv != nil && kv.Value != nil && kv.Key == key {
				return int(kv.Value.GetIntValue())
			}
		}
	}
	return 0
}

func firstSafeAttrFloat(attrs []*commonpb.KeyValue, keys ...string) float64 {
	for _, key := range keys {
		for _, kv := range attrs {
			if kv != nil && kv.Value != nil && kv.Key == key {
				return kv.Value.GetDoubleValue()
			}
		}
	}
	return 0
}

func firstSafeAttrBool(attrs []*commonpb.KeyValue, keys ...string) bool {
	for _, key := range keys {
		for _, kv := range attrs {
			if kv != nil && kv.Value != nil && kv.Key == key {
				return kv.Value.GetBoolValue()
			}
		}
	}
	return false
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
