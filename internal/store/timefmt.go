package store

import (
	"time"
)

// TimeLayout is the canonical storage format for every timestamp column:
// UTC with a fixed +00:00 offset and fixed millisecond precision, e.g.
// "2026-07-10 06:59:02.123+00:00". This exact shape is load-bearing three
// ways: it is one of the modernc/sqlite driver's native parse layouts (so
// scanning TIMESTAMP columns into time.Time keeps working), SQLite's date
// functions accept it (so strftime(..., 'localtime') grouping works), and
// with a uniform offset and fraction width it sorts lexically, which is what
// BETWEEN range filters rely on.
const TimeLayout = "2006-01-02 15:04:05.000-07:00"

// legacyLayout is how the modernc driver used to serialize time.Time binds
// (Go's time.String() shape). Kept only to convert databases written before
// FormatTime existed.
const legacyLayout = "2006-01-02 15:04:05.999999999 -0700 MST"

// FormatTime renders t for storage or for binding against a timestamp
// column. Every query parameter compared to a timestamp column must go
// through this, or the comparison silently degrades to mixed-format string
// ordering.
func FormatTime(t time.Time) string {
	return t.UTC().Format(TimeLayout)
}

// ParseTime reads a stored timestamp in the canonical layout, falling back
// to the legacy driver format and RFC3339 for not-yet-converted values.
func ParseTime(s string) (time.Time, bool) {
	for _, layout := range []string{TimeLayout, legacyLayout, time.RFC3339Nano} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}
