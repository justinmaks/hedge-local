package collect

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"sync/atomic"
	"time"

	collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	colmetricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/protobuf/proto"

	"github.com/justinmaks/hedge-local/internal/normalize"
)

// defaultMaxBodyBytes caps a single OTLP request body. The receiver is
// localhost-only, but a malformed/oversized body should not exhaust memory.
const defaultMaxBodyBytes = 16 << 20 // 16 MiB

// readHeaderTimeout bounds how long a client may take to send request headers,
// guarding against slow-client connection exhaustion.
const readHeaderTimeout = 10 * time.Second

type Receiver struct {
	normalizer   normalize.Normalizer
	writer       *Writer
	port         int
	server       *http.Server
	started      atomic.Bool
	malformed    atomic.Int64
	maxBodyBytes int64
}

func NewReceiver(n normalize.Normalizer, w *Writer, port int) *Receiver {
	return &Receiver{normalizer: n, writer: w, port: port, maxBodyBytes: defaultMaxBodyBytes}
}

// readBody reads the request body up to maxBodyBytes. On any read error
// (including an oversized body) it records a malformed request, writes a 400,
// and returns ok=false.
func (r *Receiver) readBody(w http.ResponseWriter, req *http.Request) ([]byte, bool) {
	req.Body = http.MaxBytesReader(w, req.Body, r.maxBodyBytes)
	body, err := io.ReadAll(req.Body)
	if err != nil {
		r.malformed.Add(1)
		http.Error(w, "read body", http.StatusBadRequest)
		return nil, false
	}
	return body, true
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

func (r *Receiver) MalformedCount() int64 {
	return r.malformed.Load()
}

func (r *Receiver) handleTraces(w http.ResponseWriter, req *http.Request) {
	body, ok := r.readBody(w, req)
	if !ok {
		return
	}

	exportReq := &coltracepb.ExportTraceServiceRequest{}
	if err := proto.Unmarshal(body, exportReq); err != nil {
		r.malformed.Add(1)
		http.Error(w, "unmarshal", http.StatusBadRequest)
		return
	}

	events, err := r.normalizer.NormalizeTraces(exportReq)
	if err != nil {
		r.malformed.Add(1)
		http.Error(w, "normalize", http.StatusInternalServerError)
		return
	}

	if err := r.writer.Write(events); err != nil {
		r.malformed.Add(1)
		http.Error(w, "write", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(200)
}

func (r *Receiver) handleMetrics(w http.ResponseWriter, req *http.Request) {
	body, ok := r.readBody(w, req)
	if !ok {
		return
	}

	exportReq := &colmetricspb.ExportMetricsServiceRequest{}
	if err := proto.Unmarshal(body, exportReq); err != nil {
		r.malformed.Add(1)
		http.Error(w, "unmarshal", http.StatusBadRequest)
		return
	}

	events, err := r.normalizer.NormalizeMetrics(exportReq)
	if err != nil {
		r.malformed.Add(1)
		http.Error(w, "normalize", http.StatusInternalServerError)
		return
	}

	if err := r.writer.Write(events); err != nil {
		r.malformed.Add(1)
		http.Error(w, "write", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(200)
}

func (r *Receiver) handleLogs(w http.ResponseWriter, req *http.Request) {
	body, ok := r.readBody(w, req)
	if !ok {
		return
	}

	exportReq := &collogspb.ExportLogsServiceRequest{}
	if err := proto.Unmarshal(body, exportReq); err != nil {
		r.malformed.Add(1)
		http.Error(w, "unmarshal", http.StatusBadRequest)
		return
	}

	events, err := r.normalizer.NormalizeLogs(exportReq)
	if err != nil {
		r.malformed.Add(1)
		http.Error(w, "normalize", http.StatusInternalServerError)
		return
	}

	if err := r.writer.Write(events); err != nil {
		r.malformed.Add(1)
		http.Error(w, "write", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(200)
}
