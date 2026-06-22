package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestPrintLogTail(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "daemon.log")
	lines := "line1\nline2\nline3\nline4\nline5\n"
	os.WriteFile(logPath, []byte(lines), 0644)

	var buf bytes.Buffer
	printLogTail(&buf, logPath, 3)
	output := buf.String()
	if !bytes.Contains(buf.Bytes(), []byte("line3")) {
		t.Errorf("expected 'line3' in tail: %s", output)
	}
	if !bytes.Contains(buf.Bytes(), []byte("line5")) {
		t.Errorf("expected 'line5' in tail: %s", output)
	}
	if bytes.Contains(buf.Bytes(), []byte("line1")) {
		t.Errorf("should not contain 'line1' in last-3 tail: %s", output)
	}
}

func TestPrintLogTailNoFile(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "nonexistent.log")
	var buf bytes.Buffer
	printLogTail(&buf, logPath, 50)
	if !bytes.Contains(buf.Bytes(), []byte("no daemon logs")) {
		t.Errorf("expected 'no daemon logs': %s", buf.String())
	}
}
