package cli

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestIntegration_daemonLifecycle(t *testing.T) {
	// Build a fresh binary
	binary := filepath.Join(t.TempDir(), "hcli")
	repoRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	buildCmd := exec.Command("go", "build", "-o", binary, "./cmd/hcli")
	buildCmd.Dir = repoRoot
	buildCmd.Env = append(os.Environ(), "GO111MODULE=on")
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}

	// Use temp home dir to avoid clashing with real daemon
	homeDir := t.TempDir()
	pidPath := filepath.Join(homeDir, ".hedge", "daemon.pid")
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	// Start daemon
	daemonCmd := exec.Command(binary, "collect", "--db", filepath.Join(homeDir, "test.db"), "--port", fmt.Sprint(port))
	daemonCmd.Env = append([]string{"HOME=" + homeDir}, "HCLI_DAEMON_CHILD=1")
	if err := daemonCmd.Start(); err != nil {
		t.Fatalf("start daemon: %v", err)
	}
	// Reap the child when it exits so it doesn't linger as a zombie
	// (which would otherwise cause processAlive to keep returning true).
	go daemonCmd.Wait()

	// Wait for daemon to be alive
	time.Sleep(500 * time.Millisecond)

	// Write PID file manually (simulating what forkDaemon would do)
	if err := writePIDFile(pidPath, daemonCmd.Process.Pid); err != nil {
		t.Fatalf("writePIDFile: %v", err)
	}

	// Check status shows running
	var statusBuf bytes.Buffer
	printStatus(&statusBuf, pidPath, filepath.Join(homeDir, "test.db"))
	if !bytes.Contains(statusBuf.Bytes(), []byte("running")) {
		t.Fatalf("status should show running: %s", statusBuf.String())
	}

	// Stop daemon
	var stopBuf bytes.Buffer
	if err := stopDaemon(&stopBuf, pidPath); err != nil {
		t.Fatalf("stopDaemon: %v", err)
	}
	if !bytes.Contains(stopBuf.Bytes(), []byte("stopped")) {
		t.Fatalf("stop should show stopped: %s", stopBuf.String())
	}

	// Verify process is dead
	if processAlive(daemonCmd.Process.Pid) {
		daemonCmd.Process.Kill()
		t.Fatal("daemon process should be dead after stop")
	}

	// Verify PID file is removed
	if _, err := readPIDFile(pidPath); err == nil {
		t.Fatal("PID file should be removed after stop")
	}
}

func TestIntegration_daemonForkPreservesConfigFlag(t *testing.T) {
	// Build a fresh binary
	binary := filepath.Join(t.TempDir(), "hcli")
	repoRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	buildCmd := exec.Command("go", "build", "-o", binary, "./cmd/hcli")
	buildCmd.Dir = repoRoot
	buildCmd.Env = append(os.Environ(), "GO111MODULE=on")
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}

	homeDir := t.TempDir()
	configuredDB := filepath.Join(t.TempDir(), "configured.db")
	cfgPath := filepath.Join(t.TempDir(), "config.toml")
	cfg := fmt.Sprintf("db_path = %q\notlp_port = 0\n", configuredDB)
	if err := os.WriteFile(cfgPath, []byte(cfg), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	startCmd := exec.Command(binary, "--config", cfgPath, "collect", "-d")
	startCmd.Env = append(os.Environ(), "HOME="+homeDir)
	if out, err := startCmd.CombinedOutput(); err != nil {
		t.Fatalf("start daemon: %v\n%s", err, out)
	}

	t.Cleanup(func() {
		stopCmd := exec.Command(binary, "stop")
		stopCmd.Env = append(os.Environ(), "HOME="+homeDir)
		_ = stopCmd.Run()
	})

	var dbErr error
	for i := 0; i < 30; i++ {
		_, dbErr = os.Stat(configuredDB)
		if dbErr == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if dbErr != nil {
		t.Fatalf("configured db should exist: %v", dbErr)
	}

	defaultDB := filepath.Join(homeDir, ".hedge", "hedge.db")
	if _, err := os.Stat(defaultDB); err == nil {
		t.Fatalf("daemon created default db %s instead of configured db", defaultDB)
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat default db: %v", err)
	}
}

func TestIntegration_daemonForkFailsWhenReceiverCannotStart(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "hcli")
	repoRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	buildCmd := exec.Command("go", "build", "-o", binary, "./cmd/hcli")
	buildCmd.Dir = repoRoot
	buildCmd.Env = append(os.Environ(), "GO111MODULE=on")
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port

	homeDir := t.TempDir()
	t.Cleanup(func() {
		stopCmd := exec.Command(binary, "stop")
		stopCmd.Env = append(os.Environ(), "HOME="+homeDir)
		_ = stopCmd.Run()
	})

	startCmd := exec.Command(binary, "collect", "-d", "--port", fmt.Sprint(port), "--db", filepath.Join(homeDir, "test.db"))
	startCmd.Env = append(os.Environ(), "HOME="+homeDir)
	out, err := startCmd.CombinedOutput()
	if err == nil {
		t.Fatalf("start daemon should fail when port is unavailable, output:\n%s", out)
	}

	if _, err := os.Stat(filepath.Join(homeDir, ".hedge", "daemon.pid")); !os.IsNotExist(err) {
		t.Fatalf("pid file should not remain after failed start, stat err: %v", err)
	}
}
