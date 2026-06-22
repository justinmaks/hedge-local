package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var (
	logsLines  int
	logsFollow bool
)

var logsCmd = &cobra.Command{
	Use:   "logs",
	Short: "Tail daemon logs",
	RunE:  runLogs,
}

func init() {
	logsCmd.Flags().IntVarP(&logsLines, "lines", "n", 50, "number of lines to show")
	logsCmd.Flags().BoolVarP(&logsFollow, "follow", "f", false, "follow log output")
	rootCmd.AddCommand(logsCmd)
}

func runLogs(cmd *cobra.Command, args []string) error {
	logPath := defaultLogPath()

	if logsFollow {
		return followLog(cmd.OutOrStdout(), logPath)
	}

	printLogTail(cmd.OutOrStdout(), logPath, logsLines)
	return nil
}

func printLogTail(w io.Writer, logPath string, n int) {
	data, err := os.ReadFile(logPath)
	if err != nil {
		fmt.Fprintf(w, "no daemon logs found at %s\n", logPath)
		return
	}

	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	start := len(lines) - n
	if start < 0 {
		start = 0
	}
	for _, line := range lines[start:] {
		fmt.Fprintln(w, line)
	}
}

func followLog(w io.Writer, logPath string) error {
	f, err := os.Open(logPath)
	if err != nil {
		fmt.Fprintf(w, "no daemon logs found at %s\n", logPath)
		return nil
	}
	defer f.Close()

	f.Seek(0, io.SeekEnd)

	reader := bufio.NewReader(f)
	for {
		line, err := reader.ReadString('\n')
		if len(line) > 0 {
			fmt.Fprint(w, line)
		}
		if err != nil {
			time.Sleep(200 * time.Millisecond)
		}
	}
}
