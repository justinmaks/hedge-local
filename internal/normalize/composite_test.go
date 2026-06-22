package normalize

import (
	"errors"
	"testing"
	"time"

	collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	colmetricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	tracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
)

type stubNormalizer struct {
	agent  string
	events []Event
	err    error
}

func (s stubNormalizer) Agent() string { return s.agent }
func (s stubNormalizer) NormalizeTraces(*tracepb.ExportTraceServiceRequest) ([]Event, error) {
	return s.events, s.err
}
func (s stubNormalizer) NormalizeMetrics(*colmetricspb.ExportMetricsServiceRequest) ([]Event, error) {
	return s.events, s.err
}
func (s stubNormalizer) NormalizeLogs(*collogspb.ExportLogsServiceRequest) ([]Event, error) {
	return s.events, s.err
}

func TestCompositeNormalizer_concatenatesEvents(t *testing.T) {
	n := NewCompositeNormalizer(
		stubNormalizer{agent: "a", events: []Event{{Agent: "a", SessionID: "one"}}},
		stubNormalizer{agent: "b", events: []Event{{Agent: "b", SessionID: "two"}}},
	)
	events, err := n.NormalizeTraces(&tracepb.ExportTraceServiceRequest{})
	if err != nil {
		t.Fatalf("NormalizeTraces: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("events: got %d, want 2", len(events))
	}
	if events[0].Agent != "a" || events[1].Agent != "b" {
		t.Fatalf("events not concatenated in order: %#v", events)
	}
}

func TestCompositeNormalizer_returnsChildError(t *testing.T) {
	n := NewCompositeNormalizer(stubNormalizer{agent: "bad", err: errors.New("boom")})
	_, err := n.NormalizeTraces(&tracepb.ExportTraceServiceRequest{})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestCompositeNormalizer_logsRouteToCorrectAgent(t *testing.T) {
	n := NewCompositeNormalizer(&ClaudeCodeNormalizer{}, &OpenCodeNormalizer{})
	req := &collogspb.ExportLogsServiceRequest{
		ResourceLogs: []*logspb.ResourceLogs{{
			ScopeLogs: []*logspb.ScopeLogs{{
				LogRecords: []*logspb.LogRecord{
					{
						TimeUnixNano: uint64(time.Now().UnixNano()),
						Body:         &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: `{"tool":"bash"}`}},
						Attributes: []*commonpb.KeyValue{
							{Key: "event.name", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "claude_code.tool_result"}}},
							{Key: "session.id", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "sess-c"}}},
						},
					},
					{
						TimeUnixNano: uint64(time.Now().UnixNano()),
						Body:         &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "api_request"}},
						Attributes: []*commonpb.KeyValue{
							{Key: "event.name", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "api_request"}}},
							{Key: "session.id", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "sess-o"}}},
							{Key: "model", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "google/gemini-2.5-flash-lite"}}},
						},
					},
				},
			}},
		}},
	}
	events, err := n.NormalizeLogs(req)
	if err != nil {
		t.Fatalf("NormalizeLogs: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("events: got %d, want 2", len(events))
	}
	agents := map[string]int{}
	for _, e := range events {
		agents[e.Agent]++
	}
	if agents["claude_code"] != 1 || agents["opencode"] != 1 {
		t.Fatalf("expected one claude_code and one opencode event, got %#v", agents)
	}
}
