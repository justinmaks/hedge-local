package tui

import (
	"strings"
	"testing"
)

func TestSparkline_fillsRequestedWidth(t *testing.T) {
	values := []float64{1, 2, 3, 2, 1, 3, 2, 1, 2, 3}
	result := Sparkline(values, 3, 20)
	runes := []rune(result)
	if len(runes) != 20 {
		t.Errorf("expected width 20, got %d: %q", len(runes), result)
	}
}

func TestSparkline_singleValue(t *testing.T) {
	values := []float64{5}
	result := Sparkline(values, 5, 10)
	runes := []rune(result)
	if len(runes) != 10 {
		t.Errorf("expected width 10, got %d: %q", len(runes), result)
	}
}

func TestSparkline_emptyValues(t *testing.T) {
	result := Sparkline(nil, 0, 15)
	runes := []rune(result)
	if len(runes) != 15 {
		t.Errorf("expected width 15, got %d: %q", len(runes), result)
	}
}

func TestSparkline_moreValuesThanWidth(t *testing.T) {
	values := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	result := Sparkline(values, 10, 5)
	runes := []rune(result)
	if len(runes) != 5 {
		t.Errorf("expected width 5, got %d: %q", len(runes), result)
	}
}

func TestBar_responsiveWidth(t *testing.T) {
	result := Bar(50, 60)
	full := strings.Count(result, "░") + strings.Count(result, "█")
	if full != 50 {
		t.Errorf("expected 50 chars, got %d: %q", full, result)
	}
}
