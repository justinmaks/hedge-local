package normalize

import (
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	colmetricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
	metricpb "go.opentelemetry.io/proto/otlp/metrics/v1"
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

func (n *ClaudeCodeNormalizer) NormalizeMetrics(req *colmetricspb.ExportMetricsServiceRequest) ([]Event, error) {
	type metricGroup struct {
		event Event
		llm   LLMCallData
	}
	groups := make(map[string]*metricGroup)
	for _, rm := range req.ResourceMetrics {
		projectPath := firstAttrString(rm.Resource.GetAttributes(), "hcli.project_path", "project.path", "process.cwd")
		for _, sm := range rm.ScopeMetrics {
			for _, metric := range sm.Metrics {
				if metric.Name != "claude_code.cost.usage" && metric.Name != "claude_code.token.usage" {
					continue
				}
				for _, dp := range metricDataPoints(metric) {
					sessionID := attrString(dp.Attributes, "session.id")
					model := attrString(dp.Attributes, "model")
					if sessionID == "" || model == "" {
						continue
					}
					ts := time.Unix(0, int64(dp.TimeUnixNano))
					if ts.IsZero() {
						ts = time.Now()
					}
					key := fmt.Sprintf("%s\x00%s\x00%d", sessionID, model, dp.TimeUnixNano)
					group := groups[key]
					if group == nil {
						group = &metricGroup{
							event: Event{Type: EventLLMCall, Timestamp: ts, Agent: "claude_code", SessionID: sessionID, ProjectPath: projectPath},
							llm:   LLMCallData{StartedAt: ts, Model: model, Provider: "anthropic"},
						}
						groups[key] = group
					}
					value := metricPointValue(dp)
					switch metric.Name {
					case "claude_code.cost.usage":
						group.llm.CostUSD += value
					case "claude_code.token.usage":
						switch attrString(dp.Attributes, "type") {
						case "input":
							group.llm.InputTokens += int(value)
						case "output":
							group.llm.OutputTokens += int(value)
						case "cacheRead":
							group.llm.CacheReadTokens += int(value)
						case "cacheCreation":
							group.llm.CacheWriteTokens += int(value)
						}
					}
				}
			}
		}
	}
	events := make([]Event, 0, len(groups))
	for _, group := range groups {
		llm := group.llm
		group.event.LLMCall = &llm
		events = append(events, group.event)
	}
	return events, nil
}

func metricDataPoints(metric *metricpb.Metric) []*metricpb.NumberDataPoint {
	if sum := metric.GetSum(); sum != nil {
		return sum.DataPoints
	}
	if gauge := metric.GetGauge(); gauge != nil {
		return gauge.DataPoints
	}
	return nil
}

func metricPointValue(dp *metricpb.NumberDataPoint) float64 {
	switch v := dp.Value.(type) {
	case *metricpb.NumberDataPoint_AsDouble:
		return v.AsDouble
	case *metricpb.NumberDataPoint_AsInt:
		return float64(v.AsInt)
	default:
		return 0
	}
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
