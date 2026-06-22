package cli

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestWriteAndReadPIDFile(t *testing.T) {
	dir := t.TempDir()
	pidPath := filepath.Join(dir, "daemon.pid")

	if err := writePIDFile(pidPath, 12345); err != nil {
		t.Fatalf("writePIDFile: %v", err)
	}

	pid, err := readPIDFile(pidPath)
	if err != nil {
		t.Fatalf("readPIDFile: %v", err)
	}
	if pid != 12345 {
		t.Errorf("pid: got %d, want 12345", pid)
	}
}

func TestReadPIDFileMissing(t *testing.T) {
	dir := t.TempDir()
	pidPath := filepath.Join(dir, "nonexistent.pid")
	_, err := readPIDFile(pidPath)
	if err == nil {
		t.Fatal("expected error for missing PID file")
	}
}

func TestRemovePIDFile(t *testing.T) {
	dir := t.TempDir()
	pidPath := filepath.Join(dir, "daemon.pid")
	writePIDFile(pidPath, 999)
	if err := removePIDFile(pidPath); err != nil {
		t.Fatalf("removePIDFile: %v", err)
	}
	if _, err := os.Stat(pidPath); !os.IsNotExist(err) {
		t.Errorf("PID file should be removed")
	}
}

func TestRemovePIDFileMissing(t *testing.T) {
	dir := t.TempDir()
	pidPath := filepath.Join(dir, "nonexistent.pid")
	if err := removePIDFile(pidPath); err != nil {
		t.Errorf("removePIDFile on missing file should not error: %v", err)
	}
}

func TestProcessAlive(t *testing.T) {
	if !processAlive(os.Getpid()) {
		t.Error("current process should be alive")
	}
	if processAlive(999999) {
		t.Error("nonexistent PID should not be alive")
	}
}

func TestDaemonFlagParsing(t *testing.T) {
	cmd := collectCmd
	daemonFlag := cmd.Flag("daemon")
	if daemonFlag == nil {
		t.Fatal("collect command should have --daemon flag")
	}
	if daemonFlag.Shorthand != "d" {
		t.Errorf("daemon flag shorthand: got %q, want 'd'", daemonFlag.Shorthand)
	}
}

func TestDaemonChildArgsPreserveRootFlagsAndStripDaemonFlags(t *testing.T) {
	args := []string{"--config", "/tmp/config.toml", "collect", "-d", "--daemon", "--daemon=true", "-d=true", "-dv", "-qd", "--db", "/tmp/hedge.db"}
	want := []string{"--config", "/tmp/config.toml", "collect", "-v", "-q", "--db", "/tmp/hedge.db"}

	got := daemonChildArgs(args)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("daemonChildArgs() = %#v, want %#v", got, want)
	}
}
