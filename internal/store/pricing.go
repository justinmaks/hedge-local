package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	pricingfiles "github.com/justinmaks/hedge-local/dist/pricing"
)

var pricingFS = pricingfiles.FS

type PricingRow struct {
	ID              int64
	Provider        string
	Model           string
	InputPer1M      float64
	OutputPer1M     float64
	CacheReadPer1M  float64
	CacheWritePer1M float64
	EffectiveFrom   time.Time
	EffectiveTo     *time.Time
	Source          string
}

type bundledPricingEntry struct {
	Provider        string   `json:"provider"`
	Model           string   `json:"model"`
	InputPer1M      *float64 `json:"input_per_1m"`
	OutputPer1M     *float64 `json:"output_per_1m"`
	CacheReadPer1M  float64  `json:"cache_read_per_1m"`
	CacheWritePer1M float64  `json:"cache_write_per_1m"`
	EffectiveFrom   string   `json:"effective_from"`
}

type parsedPricingEntry struct {
	Provider        string
	Model           string
	InputPer1M      float64
	OutputPer1M     float64
	CacheReadPer1M  float64
	CacheWritePer1M float64
	EffectiveFrom   time.Time
}

func (s *Store) SeedBundledPricing() error {
	data, err := pricingFS.ReadFile("pricing.json")
	if err != nil {
		return fmt.Errorf("read bundled pricing: %w", err)
	}
	return s.ImportPricingJSON(data, "bundled")
}

func (s *Store) ImportPricingJSON(data []byte, source string) error {
	if source == "" {
		source = "imported"
	}
	var entries []bundledPricingEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return fmt.Errorf("parse pricing json: %w", err)
	}
	parsed := make([]parsedPricingEntry, 0, len(entries))
	for i, e := range entries {
		context := pricingEntryContext(i, e.Provider, e.Model)
		if e.Provider == "" || e.Model == "" || e.EffectiveFrom == "" {
			return fmt.Errorf("%s: pricing entry requires provider, model, and effective_from", context)
		}
		if e.InputPer1M == nil || *e.InputPer1M <= 0 {
			return fmt.Errorf("%s: input_per_1m must be greater than 0", context)
		}
		if e.OutputPer1M == nil || *e.OutputPer1M <= 0 {
			return fmt.Errorf("%s: output_per_1m must be greater than 0", context)
		}
		from, err := time.Parse(time.RFC3339, e.EffectiveFrom)
		if err != nil {
			return fmt.Errorf("%s: parse effective_from %q: %w", context, e.EffectiveFrom, err)
		}
		parsed = append(parsed, parsedPricingEntry{
			Provider:        e.Provider,
			Model:           e.Model,
			InputPer1M:      *e.InputPer1M,
			OutputPer1M:     *e.OutputPer1M,
			CacheReadPer1M:  e.CacheReadPer1M,
			CacheWritePer1M: e.CacheWritePer1M,
			EffectiveFrom:   from,
		})
	}

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin pricing import: %w", err)
	}
	defer tx.Rollback()

	for i, e := range parsed {
		_, err = tx.Exec(
			`INSERT OR IGNORE INTO pricing (provider, model, input_per_1m, output_per_1m, cache_read_per_1m, cache_write_per_1m, effective_from, source)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			e.Provider, e.Model, e.InputPer1M, e.OutputPer1M, e.CacheReadPer1M, e.CacheWritePer1M, e.EffectiveFrom, source,
		)
		if err != nil {
			return fmt.Errorf("insert pricing %s: %w", pricingEntryContext(i, e.Provider, e.Model), err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit pricing import: %w", err)
	}
	return nil
}

func (s *Store) ListPricing() ([]PricingRow, error) {
	rows, err := s.db.Query(
		`SELECT id, provider, model, input_per_1m, output_per_1m, cache_read_per_1m, cache_write_per_1m, effective_from, effective_to, source
		 FROM pricing
		 ORDER BY provider, model, effective_from`,
	)
	if err != nil {
		return nil, fmt.Errorf("list pricing: %w", err)
	}
	defer rows.Close()

	var result []PricingRow
	for rows.Next() {
		var row PricingRow
		var inputPer1M, outputPer1M, cacheReadPer1M, cacheWritePer1M sql.NullFloat64
		var source sql.NullString
		if err := rows.Scan(&row.ID, &row.Provider, &row.Model, &inputPer1M, &outputPer1M, &cacheReadPer1M, &cacheWritePer1M, &row.EffectiveFrom, &row.EffectiveTo, &source); err != nil {
			return nil, fmt.Errorf("scan pricing: %w", err)
		}
		row.InputPer1M = inputPer1M.Float64
		row.OutputPer1M = outputPer1M.Float64
		row.CacheReadPer1M = cacheReadPer1M.Float64
		row.CacheWritePer1M = cacheWritePer1M.Float64
		row.Source = source.String
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate pricing: %w", err)
	}
	return result, nil
}

func pricingEntryContext(index int, provider, model string) string {
	if provider != "" || model != "" {
		return fmt.Sprintf("pricing row %d (%s/%s)", index, provider, model)
	}
	return fmt.Sprintf("pricing row %d", index)
}

func (s *Store) PricingFor(provider, model string, at time.Time) (PricingRow, error) {
	row := PricingRow{}
	err := s.db.QueryRow(
		`SELECT id, provider, model, input_per_1m, output_per_1m, cache_read_per_1m, cache_write_per_1m, effective_from, effective_to, source
		 FROM pricing
		 WHERE provider = ? AND model = ? AND effective_from <= ?
		 ORDER BY effective_from DESC
		 LIMIT 1`,
		provider, model, at,
	).Scan(
		&row.ID, &row.Provider, &row.Model, &row.InputPer1M, &row.OutputPer1M,
		&row.CacheReadPer1M, &row.CacheWritePer1M, &row.EffectiveFrom, &row.EffectiveTo, &row.Source,
	)
	if err != nil {
		return row, fmt.Errorf("pricing for %s/%s at %s: %w", provider, model, at, err)
	}
	return row, nil
}

func ComputeCost(p PricingRow, inputTokens, outputTokens, cacheReadTokens, cacheWriteTokens int) float64 {
	cost := 0.0
	cost += float64(inputTokens) / 1_000_000.0 * p.InputPer1M
	cost += float64(outputTokens) / 1_000_000.0 * p.OutputPer1M
	cost += float64(cacheReadTokens) / 1_000_000.0 * p.CacheReadPer1M
	cost += float64(cacheWriteTokens) / 1_000_000.0 * p.CacheWritePer1M
	return cost
}
