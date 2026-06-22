package normalize

import (
	"encoding/json"
	"time"
)

type EventType int

const (
	EventLLMCall EventType = iota
	EventToolCall
	EventLog
	EventSessionStart
	EventSessionEnd
)

type Event struct {
	Type        EventType
	Timestamp   time.Time
	Agent       string
	SessionID   string
	ProjectPath string

	LLMCall  *LLMCallData
	ToolCall *ToolCallData
	Log      *LogData
}

type LLMCallData struct {
	TraceID          string
	SpanID           string
	ParentSpanID     string
	StartedAt        time.Time
	DurationMs       int
	Model            string
	Provider         string
	InputTokens      int
	OutputTokens     int
	CacheReadTokens  int
	CacheWriteTokens int
	ReasoningTokens  int
	CostUSD          float64
	TTFTMs           int
	StopReason       string
}

type ToolCallData struct {
	TraceID       string
	SpanID        string
	ParentSpanID  string
	StartedAt     time.Time
	DurationMs    int
	ToolName      string
	Success       bool
	ErrorMessage  string
	InputSummary  string
	OutputSummary string
}

type LogData struct {
	TraceID string
	SpanID  string
	Name    string
	Payload json.RawMessage
}
