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

func TestSparkline_stretchesSparseData(t *testing.T) {
	// 7 points across 70 cells: every cell should be a block char, not
	// trailing space padding.
	values := []float64{1, 2, 3, 4, 3, 2, 1}
	result := Sparkline(values, 4, 70)
	if strings.Contains(result, " ") {
		t.Errorf("stretched sparkline should have no padding spaces: %q", result)
	}
	if len([]rune(result)) != 70 {
		t.Errorf("width: got %d, want 70", len([]rune(result)))
	}
}

func TestWaveform_twoRowSplit(t *testing.T) {
	// Low values stay in the bottom row; high values spill into the top.
	values := []float64{1, 8, 16}
	top, bottom := Waveform(values, 16, 3)
	topR, bottomR := []rune(top), []rune(bottom)
	if len(topR) != 3 || len(bottomR) != 3 {
		t.Fatalf("rows must be exactly width: top=%d bottom=%d", len(topR), len(bottomR))
	}
	if topR[0] != ' ' || topR[1] != ' ' {
		t.Errorf("low values should not reach the top row: %q", top)
	}
	if topR[2] == ' ' {
		t.Errorf("max value should spill into the top row: %q", top)
	}
	if bottomR[2] != '█' {
		t.Errorf("spilled value should saturate the bottom row: %q", bottom)
	}
	if bottomR[0] == ' ' {
		t.Errorf("nonzero value must be visible in the bottom row: %q", bottom)
	}
}

func TestWaveform_padsAndEmpty(t *testing.T) {
	top, bottom := Waveform([]float64{5}, 10, 6)
	if len([]rune(top)) != 6 || len([]rune(bottom)) != 6 {
		t.Fatalf("short input must pad to width: %q / %q", top, bottom)
	}
	top, bottom = Waveform(nil, 0, 4)
	if top != "    " || bottom != "    " {
		t.Fatalf("empty input should render blank rows: %q / %q", top, bottom)
	}
}
