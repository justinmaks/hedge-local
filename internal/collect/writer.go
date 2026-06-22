package collect

import (
	"fmt"
	"path/filepath"

	"github.com/justinmaks/hedge-local/internal/normalize"
	"github.com/justinmaks/hedge-local/internal/store"
)

type Writer struct {
	store    *store.Store
	withLogs bool
}

func NewWriter(s *store.Store, withLogs bool) *Writer {
	return &Writer{store: s, withLogs: withLogs}
}

func (w *Writer) Write(events []normalize.Event) error {
	for _, e := range events {
		if err := w.writeOne(e); err != nil {
			return err
		}
	}
	return nil
}

func (w *Writer) writeOne(e normalize.Event) error {
	projectID, err := w.upsertProject(e.ProjectPath)
	if err != nil {
		return err
	}
	sessionID, err := w.upsertSession(e, projectID)
	if err != nil {
		return err
	}

	switch e.Type {
	case normalize.EventLLMCall:
		return w.writeLLMCall(e, sessionID)
	case normalize.EventToolCall:
		return w.writeToolCall(e, sessionID)
	case normalize.EventLog:
		if !w.withLogs {
			return nil
		}
		return w.writeLog(e, sessionID)
	}
	return nil
}

func (w *Writer) upsertProject(path string) (int64, error) {
	if path == "" {
		path = "(ungrouped)"
	}
	name := filepath.Base(path)
	if name == "" || name == "." || name == "/" {
		name = "(ungrouped)"
	}
	return w.store.ProjectUpsert(path, name)
}

func (w *Writer) upsertSession(e normalize.Event, projectID int64) (int64, error) {
	if e.SessionID == "" {
		return 0, fmt.Errorf("event has no session ID")
	}
	return w.store.SessionUpsert(e.SessionID, e.Agent, projectID, e.Timestamp, "")
}

func (w *Writer) writeLLMCall(e normalize.Event, sessionID int64) error {
	llm := e.LLMCall
	if llm == nil {
		return nil
	}
	cost := llm.CostUSD
	if cost == 0 && llm.Model != "" && llm.Provider != "" {
		pricing, err := w.store.PricingFor(llm.Provider, llm.Model, llm.StartedAt)
		if err == nil {
			cost = store.ComputeCost(pricing, llm.InputTokens, llm.OutputTokens, llm.CacheReadTokens, llm.CacheWriteTokens)
		}
	}
	params := store.LLMCallParams{
		SessionID:        sessionID,
		TraceID:          llm.TraceID,
		SpanID:           llm.SpanID,
		ParentSpanID:     llm.ParentSpanID,
		StartedAt:        llm.StartedAt,
		DurationMs:       llm.DurationMs,
		Agent:            e.Agent,
		Model:            llm.Model,
		Provider:         llm.Provider,
		InputTokens:      llm.InputTokens,
		OutputTokens:     llm.OutputTokens,
		CacheReadTokens:  llm.CacheReadTokens,
		CacheWriteTokens: llm.CacheWriteTokens,
		ReasoningTokens:  llm.ReasoningTokens,
		CostUSD:          cost,
		TTFTMs:           llm.TTFTMs,
		StopReason:       llm.StopReason,
	}
	_, err := w.store.LLMCallInsert(params)
	return err
}

func (w *Writer) writeToolCall(e normalize.Event, sessionID int64) error {
	tc := e.ToolCall
	if tc == nil {
		return nil
	}
	params := store.ToolCallParams{
		SessionID:     sessionID,
		TraceID:       tc.TraceID,
		SpanID:        tc.SpanID,
		StartedAt:     tc.StartedAt,
		DurationMs:    tc.DurationMs,
		Agent:         e.Agent,
		ToolName:      tc.ToolName,
		Success:       tc.Success,
		ErrorMessage:  tc.ErrorMessage,
		InputSummary:  tc.InputSummary,
		OutputSummary: tc.OutputSummary,
	}
	_, err := w.store.ToolCallInsert(params)
	return err
}

func (w *Writer) writeLog(e normalize.Event, sessionID int64) error {
	log := e.Log
	if log == nil {
		return nil
	}
	params := store.EventParams{
		SessionID: sessionID,
		Timestamp: e.Timestamp,
		Agent:     e.Agent,
		EventName: log.Name,
		Payload:   string(log.Payload),
		TraceID:   log.TraceID,
		SpanID:    log.SpanID,
	}
	_, err := w.store.EventInsert(params)
	return err
}
