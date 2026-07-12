package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
)

func defaultPIDPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = os.Getenv("HOME")
	}
	return filepath.Join(home, ".hedge", "daemon.pid")
}

func defaultLogPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = os.Getenv("HOME")
	}
	return filepath.Join(home, ".hedge", "daemon.log")
}

func writePIDFile(path string, pid int) error {
	if err := mkdirSecure(filepath.Dir(path)); err != nil {
		return fmt.Errorf("create pid dir: %w", err)
	}
	return os.WriteFile(path, []byte(fmt.Sprintf("%d\n", pid)), 0644)
}

func readPIDFile(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, fmt.Errorf("read pid file: %w", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, fmt.Errorf("parse pid: %w", err)
	}
	return pid, nil
}

func removePIDFile(path string) error {
	err := os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func processAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	if err := proc.Signal(syscall.Signal(0)); err != nil {
		return false
	}
	// A recycled PID can belong to an unrelated process. Where the kernel
	// exposes the command line (Linux), require it to look like an hcli
	// process: either the literal binary name or whatever this executable
	// is called (covers renamed binaries and test harnesses checking
	// their own PID).
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
	if err != nil {
		return true
	}
	cmdline := string(data)
	if strings.Contains(cmdline, "hcli") {
		return true
	}
	self, err := os.Executable()
	if err != nil {
		return false
	}
	return strings.Contains(cmdline, filepath.Base(self))
}

func daemonChildArgs(args []string) []string {
	execArgs := make([]string, 0, len(args))
	for _, arg := range args {
		if arg == "--daemon" || arg == "-d" || strings.HasPrefix(arg, "--daemon=") || strings.HasPrefix(arg, "-d=") {
			continue
		}
		if strings.HasPrefix(arg, "-") && !strings.HasPrefix(arg, "--") && len(arg) > 2 && strings.Contains(arg[1:], "d") {
			kept := strings.ReplaceAll(arg[1:], "d", "")
			if kept == "" {
				continue
			}
			execArgs = append(execArgs, "-"+kept)
			continue
		}
		execArgs = append(execArgs, arg)
	}
	return execArgs
}

func forkDaemon(cmd *cobra.Command, args []string) error {
	pidPath := defaultPIDPath()

	if pid, err := readPIDFile(pidPath); err == nil && processAlive(pid) {
		return fmt.Errorf("daemon already running (PID %d); run 'hcli stop' first", pid)
	}

	logPath := defaultLogPath()
	if err := mkdirSecure(filepath.Dir(logPath)); err != nil {
		return fmt.Errorf("create log dir: %w", err)
	}

	logFile, err := openSecureAppendLog(logPath)
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
	}
	defer logFile.Close()

	execArgs := daemonChildArgs(os.Args[1:])

	daemonCmd := exec.Command(os.Args[0], execArgs...)
	daemonCmd.Env = append(os.Environ(), "HCLI_DAEMON_CHILD=1")
	daemonCmd.Stdout = logFile
	daemonCmd.Stderr = logFile
	daemonCmd.Stdin = nil

	if err := daemonCmd.Start(); err != nil {
		return fmt.Errorf("start daemon: %w", err)
	}

	for i := 0; i < 30; i++ {
		time.Sleep(100 * time.Millisecond)
		if pid, err := readPIDFile(pidPath); err == nil && processAlive(pid) {
			fmt.Fprintf(cmd.OutOrStdout(), "hcli daemon started (PID %d)\n", pid)
			fmt.Fprintf(cmd.OutOrStdout(), "log: %s\n", logPath)
			fmt.Fprintf(cmd.OutOrStdout(), "run 'hcli status' to check, 'hcli stop' to stop\n")
			return nil
		}
	}

	daemonCmd.Process.Kill()
	return fmt.Errorf("daemon did not start within 3 seconds — check %s", logPath)
}
