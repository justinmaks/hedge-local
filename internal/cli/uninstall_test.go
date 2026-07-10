package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestUninstall_removesHedgeDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	oldYes := uninstallYes
	oldDryRun := uninstallDryRun
	uninstallYes = true
	uninstallDryRun = false
	t.Cleanup(func() {
		uninstallYes = oldYes
		uninstallDryRun = oldDryRun
	})

	hedgeDir := filepath.Join(home, ".hedge")
	if err := os.MkdirAll(hedgeDir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(hedgeDir, "hedge.db"), []byte("fake"), 0600); err != nil {
		t.Fatalf("write db: %v", err)
	}
	if err := os.WriteFile(filepath.Join(hedgeDir, "config.toml"), []byte("db_path = 'x'"), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	var out bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&out)
	if err := runUninstall(cmd, nil); err != nil {
		t.Fatalf("uninstall: %v", err)
	}

	if _, err := os.Stat(hedgeDir); !os.IsNotExist(err) {
		t.Fatalf("~/.hedge should be removed, got: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "Removed") {
		t.Errorf("should confirm removal:\n%s", output)
	}
	if !strings.Contains(output, "source ~/.hedge/env.sh") {
		t.Errorf("should mention manual cleanup:\n%s", output)
	}
}

func TestUninstall_promptDeclinedKeepsDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	oldYes := uninstallYes
	oldDryRun := uninstallDryRun
	uninstallYes = false
	uninstallDryRun = false
	t.Cleanup(func() {
		uninstallYes = oldYes
		uninstallDryRun = oldDryRun
	})

	hedgeDir := filepath.Join(home, ".hedge")
	if err := os.MkdirAll(hedgeDir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(hedgeDir, "hedge.db"), []byte("fake"), 0600); err != nil {
		t.Fatalf("write db: %v", err)
	}

	var out bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&out)
	cmd.SetIn(strings.NewReader("n\n"))
	if err := runUninstall(cmd, nil); err != nil {
		t.Fatalf("uninstall: %v", err)
	}

	if _, err := os.Stat(hedgeDir); err != nil {
		t.Fatalf("~/.hedge should still exist after declined prompt: %v", err)
	}
	if !strings.Contains(out.String(), "Aborted") {
		t.Errorf("should print Aborted:\n%s", out.String())
	}
}

func TestUninstall_promptAcceptedRemovesDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	oldYes := uninstallYes
	oldDryRun := uninstallDryRun
	uninstallYes = false
	uninstallDryRun = false
	t.Cleanup(func() {
		uninstallYes = oldYes
		uninstallDryRun = oldDryRun
	})

	hedgeDir := filepath.Join(home, ".hedge")
	if err := os.MkdirAll(hedgeDir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	var out bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&out)
	cmd.SetIn(strings.NewReader("y\n"))
	if err := runUninstall(cmd, nil); err != nil {
		t.Fatalf("uninstall: %v", err)
	}

	if _, err := os.Stat(hedgeDir); !os.IsNotExist(err) {
		t.Fatalf("~/.hedge should be removed after accepted prompt, got: %v", err)
	}
}

func TestUninstall_noopWhenNoDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	oldYes := uninstallYes
	oldDryRun := uninstallDryRun
	uninstallYes = true
	uninstallDryRun = false
	t.Cleanup(func() {
		uninstallYes = oldYes
		uninstallDryRun = oldDryRun
	})

	var out bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&out)
	if err := runUninstall(cmd, nil); err != nil {
		t.Fatalf("uninstall: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "Nothing to remove") {
		t.Errorf("should say nothing to remove:\n%s", output)
	}
}

func TestUninstall_dryRunDoesNotRemove(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	oldYes := uninstallYes
	oldDryRun := uninstallDryRun
	uninstallYes = false
	uninstallDryRun = true
	t.Cleanup(func() {
		uninstallYes = oldYes
		uninstallDryRun = oldDryRun
	})

	hedgeDir := filepath.Join(home, ".hedge")
	if err := os.MkdirAll(hedgeDir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(hedgeDir, "hedge.db"), []byte("fake"), 0600); err != nil {
		t.Fatalf("write db: %v", err)
	}

	var out bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&out)
	if err := runUninstall(cmd, nil); err != nil {
		t.Fatalf("uninstall dry-run: %v", err)
	}

	if _, err := os.Stat(hedgeDir); err != nil {
		t.Fatalf("~/.hedge should still exist after dry-run")
	}

	output := out.String()
	if !strings.Contains(output, "dry run") && !strings.Contains(output, "would remove") {
		t.Errorf("should indicate dry run:\n%s", output)
	}
}
