package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrintStatusNotRunning(t *testing.T) {
	dir := t.TempDir()
	pidPath := filepath.Join(dir, "daemon.pid")
	var buf bytes.Buffer
	printStatus(&buf, pidPath, "/tmp/nonexistent.db")
	output := buf.String()
	if !strings.Contains(output, "not running") {
		t.Errorf("expected 'not running' in output: %s", output)
	}
}

func TestPrintStatusStalePID(t *testing.T) {
	dir := t.TempDir()
	pidPath := filepath.Join(dir, "daemon.pid")
	writePIDFile(pidPath, 999999)
	var buf bytes.Buffer
	printStatus(&buf, pidPath, "/tmp/nonexistent.db")
	output := buf.String()
	if !strings.Contains(output, "stopped") && !strings.Contains(output, "stale") {
		t.Errorf("expected 'stopped' or 'stale' in output: %s", output)
	}
}
