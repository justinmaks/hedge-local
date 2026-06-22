package normalize

import (
	"fmt"

	logspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	metricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	tracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
)

type CompositeNormalizer struct {
	normalizers []Normalizer
}

func NewCompositeNormalizer(normalizers ...Normalizer) *CompositeNormalizer {
	return &CompositeNormalizer{normalizers: normalizers}
}

func (n *CompositeNormalizer) Agent() string { return "composite" }

func (n *CompositeNormalizer) NormalizeTraces(req *tracepb.ExportTraceServiceRequest) ([]Event, error) {
	var all []Event
	for _, child := range n.normalizers {
		events, err := child.NormalizeTraces(req)
		if err != nil {
			return nil, fmt.Errorf("%s traces: %w", child.Agent(), err)
		}
		all = append(all, events...)
	}
	return all, nil
}

func (n *CompositeNormalizer) NormalizeMetrics(req *metricspb.ExportMetricsServiceRequest) ([]Event, error) {
	var all []Event
	for _, child := range n.normalizers {
		events, err := child.NormalizeMetrics(req)
		if err != nil {
			return nil, fmt.Errorf("%s metrics: %w", child.Agent(), err)
		}
		all = append(all, events...)
	}
	return all, nil
}

func (n *CompositeNormalizer) NormalizeLogs(req *logspb.ExportLogsServiceRequest) ([]Event, error) {
	var all []Event
	for _, child := range n.normalizers {
		events, err := child.NormalizeLogs(req)
		if err != nil {
			return nil, fmt.Errorf("%s logs: %w", child.Agent(), err)
		}
		all = append(all, events...)
	}
	return all, nil
}
