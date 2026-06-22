package collect

import (
	"bytes"
	"fmt"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	colmetricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/proto"

	"github.com/justinmaks/hedge-local/internal/normalize"
	"github.com/justinmaks/hedge-local/internal/store"
)

// Claude Code reports each LLM call via both a trace span and the
// cost.usage/token.usage metrics. Posting both must yield exactly one llm_call
// row (traces are canonical), not two — guarding against the double-counting
// regression.
func TestIntegration_claudeTraceAndMetricsNoDoubleCount(t *testing.T) {
	s, err := store.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	if err := s.SeedBundledPricing(); err != nil {
		t.Fatalf("SeedBundledPricing: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	r := NewReceiver(&normalize.ClaudeCodeNormalizer{}, NewWriter(s, false), 0)
	if err := r.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { r.Stop() })

	base := time.Now()
	sessAttr := func() []*commonpb.KeyValue {
		return []*commonpb.KeyValue{
			{Key: "session.id", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "dup-sess"}}},
			{Key: "model", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "claude-sonnet-4"}}},
		}
	}

	// Trace span for the call.
	traceReq := &coltracepb.ExportTraceServiceRequest{ResourceSpans: []*tracepb.ResourceSpans{{
		ScopeSpans: []*tracepb.ScopeSpans{{Spans: []*tracepb.Span{{
			Name:              "claude_code.llm_request",
			TraceId:           []byte("tracedup"),
			SpanId:            []byte("spandup1"),
			StartTimeUnixNano: uint64(base.UnixNano()),
			EndTimeUnixNano:   uint64(base.Add(time.Second).UnixNano()),
			Attributes: append(sessAttr(),
				&commonpb.KeyValue{Key: "input_tokens", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_IntValue{IntValue: 1000}}},
				&commonpb.KeyValue{Key: "output_tokens", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_IntValue{IntValue: 500}}},
			),
		}}}},
	}}}
	postProto(t, r, "/v1/traces", traceReq)

	// Matching metrics for the same call (different timestamp, no span_id).
	mAttr := sessAttr()
	tokAttr := append(append([]*commonpb.KeyValue{}, mAttr...),
		&commonpb.KeyValue{Key: "type", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "input"}}})
	metricsReq := &colmetricspb.ExportMetricsServiceRequest{ResourceMetrics: []*metricspb.ResourceMetrics{{
		Resource: &resourcepb.Resource{},
		ScopeMetrics: []*metricspb.ScopeMetrics{{Metrics: []*metricspb.Metric{
			{Name: "claude_code.cost.usage", Data: &metricspb.Metric_Sum{Sum: &metricspb.Sum{DataPoints: []*metricspb.NumberDataPoint{{
				TimeUnixNano: uint64(base.Add(3 * time.Second).UnixNano()),
				Attributes:   mAttr,
				Value:        &metricspb.NumberDataPoint_AsDouble{AsDouble: 0.05},
			}}}}},
			{Name: "claude_code.token.usage", Data: &metricspb.Metric_Sum{Sum: &metricspb.Sum{DataPoints: []*metricspb.NumberDataPoint{{
				TimeUnixNano: uint64(base.Add(3 * time.Second).UnixNano()),
				Attributes:   tokAttr,
				Value:        &metricspb.NumberDataPoint_AsInt{AsInt: 1000},
			}}}}},
		}}},
	}}}
	postProto(t, r, "/v1/metrics", metricsReq)

	time.Sleep(150 * time.Millisecond)

	var count int
	if err := s.DB().QueryRow("SELECT COUNT(*) FROM llm_calls").Scan(&count); err != nil {
		t.Fatalf("count llm_calls: %v", err)
	}
	if count != 1 {
		t.Fatalf("llm_calls = %d, want 1 (no double-counting between traces and metrics)", count)
	}

	var inTok int
	var cost float64
	if err := s.DB().QueryRow("SELECT input_tokens, cost_usd FROM llm_calls").Scan(&inTok, &cost); err != nil {
		t.Fatalf("read llm_call: %v", err)
	}
	if inTok != 1000 {
		t.Errorf("input_tokens = %d, want 1000 (from trace, not summed with metric)", inTok)
	}
	if cost <= 0 {
		t.Errorf("cost_usd = %v, want > 0 (computed from pricing for claude-sonnet-4)", cost)
	}
}

func postProto(t *testing.T, r *Receiver, path string, msg proto.Message) {
	t.Helper()
	body, err := proto.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	resp, err := http.Post(fmt.Sprintf("http://localhost:%d%s", r.Port(), path), "application/x-protobuf", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("POST %s status = %d", path, resp.StatusCode)
	}
}
