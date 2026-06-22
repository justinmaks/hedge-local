package normalize

import (
	collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	colmetricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
)

type Normalizer interface {
	Agent() string
	NormalizeTraces(req *coltracepb.ExportTraceServiceRequest) ([]Event, error)
	NormalizeMetrics(req *colmetricspb.ExportMetricsServiceRequest) ([]Event, error)
	NormalizeLogs(req *collogspb.ExportLogsServiceRequest) ([]Event, error)
}
