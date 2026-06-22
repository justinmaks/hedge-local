package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestStopNoPIDFile(t *testing.T) {
	dir := t.TempDir()
	pidPath := filepath.Join(dir, "daemon.pid")
	var buf bytes.Buffer
	err := stopDaemon(&buf, pidPath)
	if err != nil {
		t.Fatalf("stopDaemon: %v", err)
	}
	if !bytes.Contains(buf.Bytes(), []byte("not running")) {
		t.Errorf("expected 'not running': %s", buf.String())
	}
}

func TestStopStalePIDFile(t *testing.T) {
	dir := t.TempDir()
	pidPath := filepath.Join(dir, "daemon.pid")
	writePIDFile(pidPath, 999999)
	var buf bytes.Buffer
	err := stopDaemon(&buf, pidPath)
	if err != nil {
		t.Fatalf("stopDaemon: %v", err)
	}
	if !bytes.Contains(buf.Bytes(), []byte("stale")) {
		t.Errorf("expected 'stale': %s", buf.String())
	}
	if _, err := os.Stat(pidPath); !os.IsNotExist(err) {
		t.Errorf("stale PID file should be removed")
	}
}

func TestStopRunningDaemon(t *testing.T) {
	if os.Getenv("BE_TEST_CHILD") == "1" {
		time.Sleep(30 * time.Second)
		os.Exit(0)
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestStopRunningDaemon")
	cmd.Env = append(os.Environ(), "BE_TEST_CHILD=1")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start child: %v", err)
	}
	go cmd.Wait()

	dir := t.TempDir()
	pidPath := filepath.Join(dir, "daemon.pid")
	writePIDFile(pidPath, cmd.Process.Pid)

	var buf bytes.Buffer
	err := stopDaemon(&buf, pidPath)
	if err != nil {
		t.Fatalf("stopDaemon: %v", err)
	}
	if !bytes.Contains(buf.Bytes(), []byte("stopped")) {
		t.Errorf("expected 'stopped': %s", buf.String())
	}
	if _, err := os.Stat(pidPath); !os.IsNotExist(err) {
		t.Errorf("PID file should be removed after stop")
	}
}
