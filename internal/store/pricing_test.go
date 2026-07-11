package store

import (
	"strings"
	"testing"
	"time"
)

func TestPricingFor_knownModel(t *testing.T) {
	s := tempDB(t)
	if err := s.SeedBundledPricing(); err != nil {
		t.Fatalf("SeedBundledPricing: %v", err)
	}
	ts := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	p, err := s.PricingFor("anthropic", "claude-sonnet-4", ts)
	if err != nil {
		t.Fatalf("PricingFor: %v", err)
	}
	if p.InputPer1M != 3.00 {
		t.Errorf("input price: got %v, want 3.00", p.InputPer1M)
	}
	if p.OutputPer1M != 15.00 {
		t.Errorf("output price: got %v, want 15.00", p.OutputPer1M)
	}
}

func TestPricingFor_currentModels(t *testing.T) {
	s := tempDB(t)
	if err := s.SeedBundledPricing(); err != nil {
		t.Fatalf("SeedBundledPricing: %v", err)
	}
	now := time.Now()
	cases := []struct {
		model          string
		input, output  float64
		cacheR, cacheW float64
	}{
		{"claude-opus-4-8", 5, 25, 0.50, 6.25},
		{"claude-opus-4-5", 5, 25, 0.50, 6.25},
		{"claude-sonnet-4-6", 3, 15, 0.30, 3.75},
		{"claude-sonnet-4-5", 3, 15, 0.30, 3.75},
		{"claude-haiku-4-5", 1, 5, 0.10, 1.25},
	}
	for _, c := range cases {
		p, err := s.PricingFor("anthropic", c.model, now)
		if err != nil {
			t.Errorf("%s: PricingFor: %v", c.model, err)
			continue
		}
		if p.InputPer1M != c.input || p.OutputPer1M != c.output || p.CacheReadPer1M != c.cacheR || p.CacheWritePer1M != c.cacheW {
			t.Errorf("%s: got in=%v out=%v cr=%v cw=%v, want in=%v out=%v cr=%v cw=%v",
				c.model, p.InputPer1M, p.OutputPer1M, p.CacheReadPer1M, p.CacheWritePer1M,
				c.input, c.output, c.cacheR, c.cacheW)
		}
	}
}

func TestPricingFor_unknownModel(t *testing.T) {
	s := tempDB(t)
	if err := s.SeedBundledPricing(); err != nil {
		t.Fatalf("SeedBundledPricing: %v", err)
	}
	ts := time.Now()
	_, err := s.PricingFor("anthropic", "nonexistent-model", ts)
	if err == nil {
		t.Error("expected error for unknown model, got nil")
	}
}

func TestPricingFor_timeWindow(t *testing.T) {
	s := tempDB(t)
	if err := s.SeedBundledPricing(); err != nil {
		t.Fatalf("SeedBundledPricing: %v", err)
	}
	old := time.Date(2024, 11, 1, 0, 0, 0, 0, time.UTC)
	p, err := s.PricingFor("anthropic", "claude-3-5-sonnet", old)
	if err != nil {
		t.Fatalf("PricingFor old date: %v", err)
	}
	if p.InputPer1M != 3.00 {
		t.Errorf("expected 3.00, got %v", p.InputPer1M)
	}
}

func TestComputeCost(t *testing.T) {
	p := PricingRow{
		InputPer1M:      3.00,
		OutputPer1M:     15.00,
		CacheReadPer1M:  0.30,
		CacheWritePer1M: 3.75,
	}
	cost := ComputeCost(p, 100000, 50000, 20000, 5000)
	want := 1.07475
	if abs(cost-want) > 0.0001 {
		t.Errorf("ComputeCost: got %v, want %v", cost, want)
	}
}

func TestPricingImportJSONAndList(t *testing.T) {
	s := tempDB(t)
	data := []byte(`[
	  {"provider":"openai","model":"gpt-4.1","input_per_1m":2.0,"output_per_1m":8.0,"cache_read_per_1m":0.5,"cache_write_per_1m":1.0,"effective_from":"2026-01-01T00:00:00Z"}
	]`)
	if err := s.ImportPricingJSON(data, "imported"); err != nil {
		t.Fatalf("ImportPricingJSON: %v", err)
	}
	rows, err := s.ListPricing()
	if err != nil {
		t.Fatalf("ListPricing: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows: got %d, want 1", len(rows))
	}
	if rows[0].Provider != "openai" || rows[0].Model != "gpt-4.1" || rows[0].Source != "imported" {
		t.Fatalf("unexpected row: %#v", rows[0])
	}
	if rows[0].InputPer1M != 2.0 || rows[0].OutputPer1M != 8.0 {
		t.Fatalf("unexpected prices: %#v", rows[0])
	}
}

func TestPricingImportJSONRejectsInvalidEffectiveFrom(t *testing.T) {
	s := tempDB(t)
	data := []byte(`[{"provider":"openai","model":"bad","input_per_1m":1,"output_per_1m":1,"effective_from":"not-a-date"}]`)
	if err := s.ImportPricingJSON(data, "imported"); err == nil {
		t.Fatal("expected invalid effective_from error")
	}
}

func TestPricingImportJSONIsAtomicOnValidationError(t *testing.T) {
	s := tempDB(t)
	data := []byte(`[
	  {"provider":"openai","model":"good","input_per_1m":1,"output_per_1m":2,"effective_from":"2026-01-01T00:00:00Z"},
	  {"provider":"openai","model":"bad","input_per_1m":1,"output_per_1m":2,"effective_from":"not-a-date"}
	]`)
	if err := s.ImportPricingJSON(data, "imported"); err == nil {
		t.Fatal("expected import error")
	}
	rows, err := s.ListPricing()
	if err != nil {
		t.Fatalf("ListPricing: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("expected no rows after failed import, got %#v", rows)
	}
}

func TestPricingImportJSONRejectsMissingRequiredPrices(t *testing.T) {
	s := tempDB(t)
	data := []byte(`[{"provider":"openai","model":"bad","output_per_1m":2,"effective_from":"2026-01-01T00:00:00Z"}]`)
	if err := s.ImportPricingJSON(data, "imported"); err == nil {
		t.Fatal("expected missing input price error")
	}
}

func TestPricingImportJSONValidationErrorsIncludeRowContext(t *testing.T) {
	s := tempDB(t)
	data := []byte(`[
	  {"provider":"openai","model":"good","input_per_1m":1,"output_per_1m":2,"effective_from":"2026-01-01T00:00:00Z"},
	  {"provider":"openai","model":"bad","input_per_1m":1,"output_per_1m":2,"effective_from":"not-a-date"}
	]`)
	err := s.ImportPricingJSON(data, "imported")
	if err == nil {
		t.Fatal("expected import error")
	}
	if !strings.Contains(err.Error(), "pricing row 1 (openai/bad)") {
		t.Fatalf("error %q does not include row context", err.Error())
	}
}

func TestListPricingAllowsNullSource(t *testing.T) {
	s := tempDB(t)
	_, err := s.DB().Exec(`INSERT INTO pricing (provider, model, input_per_1m, output_per_1m, effective_from) VALUES (?, ?, ?, ?, ?)`, "openai", "manual", 1.0, 2.0, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("insert pricing: %v", err)
	}
	rows, err := s.ListPricing()
	if err != nil {
		t.Fatalf("ListPricing: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows: got %d, want 1", len(rows))
	}
	if rows[0].Source != "" {
		t.Fatalf("source: got %q, want empty string", rows[0].Source)
	}
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
