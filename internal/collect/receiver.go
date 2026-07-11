package collect

import (
	"context"
	"fmt"
	"io"
	"log"
	"mime"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	colmetricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/justinmaks/hedge-local/internal/normalize"
)

// defaultMaxBodyBytes caps a single OTLP request body. The receiver is
// localhost-only, but a malformed/oversized body should not exhaust memory.
const defaultMaxBodyBytes = 16 << 20 // 16 MiB

// readHeaderTimeout bounds how long a client may take to send request headers,
// guarding against slow-client connection exhaustion.
const readHeaderTimeout = 10 * time.Second

const (
	contentTypeProto = "application/x-protobuf"
	contentTypeJSON  = "application/json"
)

type Receiver struct {
	normalizer   normalize.Normalizer
	writer       *Writer
	port         int
	server       *http.Server
	started      atomic.Bool
	malformed    atomic.Int64
	writeFailed  atomic.Int64
	maxBodyBytes int64
}

func NewReceiver(n normalize.Normalizer, w *Writer, port int) *Receiver {
	return &Receiver{normalizer: n, writer: w, port: port, maxBodyBytes: defaultMaxBodyBytes}
}

func (r *Receiver) Port() int {
	return r.port
}

func (r *Receiver) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/traces", r.handleTraces)
	mux.HandleFunc("/v1/metrics", r.handleMetrics)
	mux.HandleFunc("/v1/logs", r.handleLogs)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	})

	r.server = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: readHeaderTimeout,
	}

	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", r.port))
	if err != nil {
		return fmt.Errorf("listen on port %d: %w", r.port, err)
	}
	r.port = ln.Addr().(*net.TCPAddr).Port

	go func() {
		if err := r.server.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Printf("receiver server error: %v", err)
		}
	}()

	r.started.Store(true)
	return nil
}

func (r *Receiver) Stop() error {
	if !r.started.Load() {
		return nil
	}
	r.started.Store(false)
	return r.server.Shutdown(context.Background())
}

// MalformedCount counts client-side problems: unreadable bodies, unknown
// content types, and payloads that fail to decode.
func (r *Receiver) MalformedCount() int64 {
	return r.malformed.Load()
}

// WriteFailureCount counts our own failures (normalize/storage errors), so a
// local disk problem is not blamed on the agent.
func (r *Receiver) WriteFailureCount() int64 {
	return r.writeFailed.Load()
}

// decodeRequest reads and decodes an OTLP/HTTP request into msg, honoring
// the Content-Type (protobuf or JSON per the OTLP spec). On failure it
// records a malformed request, writes the HTTP error, and returns false.
func (r *Receiver) decodeRequest(w http.ResponseWriter, req *http.Request, msg proto.Message) bool {
	req.Body = http.MaxBytesReader(w, req.Body, r.maxBodyBytes)
	body, err := io.ReadAll(req.Body)
	if err != nil {
		r.malformed.Add(1)
		http.Error(w, "read body", http.StatusBadRequest)
		return false
	}

	ct := req.Header.Get("Content-Type")
	mediaType := contentTypeProto // OTLP/HTTP default when unset
	if ct != "" {
		if mt, _, err := mime.ParseMediaType(ct); err == nil {
			mediaType = mt
		} else {
			mediaType = strings.TrimSpace(strings.ToLower(ct))
		}
	}

	switch mediaType {
	case contentTypeProto:
		if err := proto.Unmarshal(body, msg); err != nil {
			r.malformed.Add(1)
			http.Error(w, "unmarshal protobuf", http.StatusBadRequest)
			return false
		}
	case contentTypeJSON:
		if err := protojson.Unmarshal(body, msg); err != nil {
			r.malformed.Add(1)
			http.Error(w, "unmarshal json", http.StatusBadRequest)
			return false
		}
	default:
		r.malformed.Add(1)
		http.Error(w, "unsupported content type (use application/x-protobuf or application/json)", http.StatusUnsupportedMediaType)
		return false
	}
	return true
}

// respondSuccess writes the OTLP success response encoded to match the
// request's content type.
func respondSuccess(w http.ResponseWriter, req *http.Request, resp proto.Message) {
	if strings.Contains(req.Header.Get("Content-Type"), "json") {
		data, err := protojson.Marshal(resp)
		if err == nil {
			w.Header().Set("Content-Type", contentTypeJSON)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(data)
			return
		}
	}
	data, err := proto.Marshal(resp)
	if err != nil {
		w.WriteHeader(http.StatusOK)
		return
	}
	w.Header().Set("Content-Type", contentTypeProto)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// storeEvents normalizes and persists, recording internal failures.
func (r *Receiver) storeEvents(w http.ResponseWriter, events []normalize.Event, normErr error) bool {
	if normErr != nil {
		r.writeFailed.Add(1)
		http.Error(w, "normalize", http.StatusInternalServerError)
		return false
	}
	if err := r.writer.Write(events); err != nil {
		r.writeFailed.Add(1)
		http.Error(w, "write", http.StatusInternalServerError)
		return false
	}
	return true
}

func (r *Receiver) handleTraces(w http.ResponseWriter, req *http.Request) {
	exportReq := &coltracepb.ExportTraceServiceRequest{}
	if !r.decodeRequest(w, req, exportReq) {
		return
	}
	events, err := r.normalizer.NormalizeTraces(exportReq)
	if !r.storeEvents(w, events, err) {
		return
	}
	respondSuccess(w, req, &coltracepb.ExportTraceServiceResponse{})
}

func (r *Receiver) handleMetrics(w http.ResponseWriter, req *http.Request) {
	exportReq := &colmetricspb.ExportMetricsServiceRequest{}
	if !r.decodeRequest(w, req, exportReq) {
		return
	}
	events, err := r.normalizer.NormalizeMetrics(exportReq)
	if !r.storeEvents(w, events, err) {
		return
	}
	respondSuccess(w, req, &colmetricspb.ExportMetricsServiceResponse{})
}

func (r *Receiver) handleLogs(w http.ResponseWriter, req *http.Request) {
	exportReq := &collogspb.ExportLogsServiceRequest{}
	if !r.decodeRequest(w, req, exportReq) {
		return
	}
	events, err := r.normalizer.NormalizeLogs(exportReq)
	if !r.storeEvents(w, events, err) {
		return
	}
	respondSuccess(w, req, &collogspb.ExportLogsServiceResponse{})
}
