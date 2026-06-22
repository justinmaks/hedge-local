package cli

import (
	"fmt"
	"io"
	"os"
	"syscall"
	"time"

	"github.com/spf13/cobra"
)

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the running hcli daemon",
	RunE:  runStop,
}

func init() {
	rootCmd.AddCommand(stopCmd)
}

func runStop(cmd *cobra.Command, args []string) error {
	pidPath := defaultPIDPath()
	return stopDaemon(cmd.OutOrStdout(), pidPath)
}

func stopDaemon(w io.Writer, pidPath string) error {
	pid, err := readPIDFile(pidPath)
	if err != nil {
		fmt.Fprintf(w, "hcli daemon: not running\n")
		return nil
	}

	if !processAlive(pid) {
		fmt.Fprintf(w, "hcli daemon: was not running (cleaned up stale PID file)\n")
		_ = removePIDFile(pidPath)
		return nil
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		fmt.Fprintf(w, "hcli daemon: could not find process (cleaned up PID file)\n")
		_ = removePIDFile(pidPath)
		return nil
	}

	if err := proc.Signal(syscall.SIGTERM); err != nil {
		fmt.Fprintf(w, "hcli daemon: error sending SIGTERM: %v\n", err)
		_ = removePIDFile(pidPath)
		return nil
	}

	for i := 0; i < 50; i++ {
		time.Sleep(100 * time.Millisecond)
		if !processAlive(pid) {
			fmt.Fprintf(w, "hcli daemon: stopped (PID %d)\n", pid)
			_ = removePIDFile(pidPath)
			return nil
		}
	}

	fmt.Fprintf(w, "hcli daemon: did not stop within 5s, sending SIGKILL\n")
	_ = proc.Signal(syscall.SIGKILL)
	_ = removePIDFile(pidPath)
	return nil
}
