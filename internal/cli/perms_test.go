package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/spf13/cobra"
)

func TestSetupClaude_restrictsHedgeDirPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits not meaningful on Windows")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)

	var out bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&out)
	if err := runSetupClaude(cmd, nil); err != nil {
		t.Fatalf("runSetupClaude: %v", err)
	}

	info, err := os.Stat(filepath.Join(home, ".hedge"))
	if err != nil {
		t.Fatalf("stat ~/.hedge: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o700 {
		t.Errorf("~/.hedge perm = %o, want 700", perm)
	}
}

func TestOpenSecureAppendLog_restrictsPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits not meaningful on Windows")
	}
	logPath := filepath.Join(t.TempDir(), "daemon.log")
	f, err := openSecureAppendLog(logPath)
	if err != nil {
		t.Fatalf("openSecureAppendLog: %v", err)
	}
	defer f.Close()

	info, err := os.Stat(logPath)
	if err != nil {
		t.Fatalf("stat log: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("daemon.log perm = %o, want 600", perm)
	}
}
