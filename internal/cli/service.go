package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/justinmaks/hedge-local/internal/collect"
	"github.com/spf13/cobra"
)

var serviceCmd = &cobra.Command{
	Use:   "service",
	Short: "Manage hcli as an always-on background service",
	Long: `Installs the collector as an OS-managed user service (systemd --user on
Linux, launchd on macOS) so telemetry is captured continuously, survives
crashes and reboots, and never depends on a terminal being open.

OTLP exporters do not buffer: whenever no collector is listening, that
telemetry is lost forever. The service is the paved path; 'hcli collect -d'
remains as a portable fallback.`,
}

var serviceInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Install and start the collector service",
	RunE:  runServiceInstall,
}

var serviceUninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Stop and remove the collector service",
	RunE:  runServiceUninstall,
}

var serviceStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show collector service state",
	RunE:  runServiceStatus,
}

func init() {
	serviceCmd.AddCommand(serviceInstallCmd, serviceUninstallCmd, serviceStatusCmd)
	rootCmd.AddCommand(serviceCmd)
}

const systemdUnitName = "hcli-collector.service"
const launchdLabel = "io.github.justinmaks.hcli-collector"

// systemdUnit renders the user unit file. Restart=on-failure plus
// WantedBy=default.target gives crash recovery and start-at-login.
func systemdUnit(hcliPath string) string {
	return `[Unit]
Description=hcli telemetry collector (OTLP receiver)
Documentation=https://github.com/justinmaks/hedge-local

[Service]
ExecStart=` + hcliPath + ` collect
Restart=on-failure
RestartSec=2

[Install]
WantedBy=default.target
`
}

// launchdPlist renders the LaunchAgent. KeepAlive restarts on crash,
// RunAtLoad starts it at login.
func launchdPlist(hcliPath string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>` + launchdLabel + `</string>
	<key>ProgramArguments</key>
	<array>
		<string>` + hcliPath + `</string>
		<string>collect</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<dict>
		<key>SuccessfulExit</key>
		<false/>
	</dict>
</dict>
</plist>
`
}

func systemdUnitPath(home string) string {
	return filepath.Join(home, ".config", "systemd", "user", systemdUnitName)
}

func launchdPlistPath(home string) string {
	return filepath.Join(home, "Library", "LaunchAgents", launchdLabel+".plist")
}

func runServiceInstall(cmd *cobra.Command, args []string) error {
	out := cmd.OutOrStdout()
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home dir: %w", err)
	}
	hcliPath, err := os.Executable()
	if err != nil || hcliPath == "" {
		return fmt.Errorf("resolve hcli binary path: %w", err)
	}

	// A pidfile daemon would fight the service for the port.
	if pid, err := readPIDFile(defaultPIDPath()); err == nil && processAlive(pid) {
		return fmt.Errorf("a daemon is already running (PID %d); run 'hcli stop' first", pid)
	}

	switch runtime.GOOS {
	case "linux":
		unitPath := systemdUnitPath(home)
		if err := os.MkdirAll(filepath.Dir(unitPath), 0755); err != nil {
			return err
		}
		if err := os.WriteFile(unitPath, []byte(systemdUnit(hcliPath)), 0644); err != nil {
			return fmt.Errorf("write unit: %w", err)
		}
		fmt.Fprintf(out, "Wrote %s\n", unitPath)
		steps := [][]string{
			{"systemctl", "--user", "daemon-reload"},
			{"systemctl", "--user", "enable", "--now", systemdUnitName},
		}
		for _, step := range steps {
			if err := runCommand(step); err != nil {
				return fmt.Errorf("%v: %w (unit file is in place; run it manually)", step, err)
			}
		}
		fmt.Fprintln(out, "Service enabled and started.")
		// Lingering keeps the user manager (and the collector) running
		// after logout and starts it at boot before login. Best effort:
		// polkit may prompt or deny in some environments.
		if err := runCommand([]string{"loginctl", "enable-linger"}); err != nil {
			fmt.Fprintln(out, "Note: 'loginctl enable-linger' failed; the collector runs only while you are logged in.")
		}
	case "darwin":
		plistPath := launchdPlistPath(home)
		if err := os.MkdirAll(filepath.Dir(plistPath), 0755); err != nil {
			return err
		}
		if err := os.WriteFile(plistPath, []byte(launchdPlist(hcliPath)), 0644); err != nil {
			return fmt.Errorf("write plist: %w", err)
		}
		fmt.Fprintf(out, "Wrote %s\n", plistPath)
		if err := runCommand([]string{"launchctl", "load", "-w", plistPath}); err != nil {
			return fmt.Errorf("launchctl load: %w (plist is in place; load it manually)", err)
		}
		fmt.Fprintln(out, "Service loaded and started.")
	default:
		return fmt.Errorf("service install is not supported on %s; use 'hcli collect -d'", runtime.GOOS)
	}

	// Give it a moment, then confirm it actually answers.
	cfg, _, err := loadCLIConfigAndDB()
	if err != nil {
		return err
	}
	for i := 0; i < 10; i++ {
		if collect.HealthCheck(cfg.OTLPPort, 300*time.Millisecond) {
			fmt.Fprintf(out, "Collector is listening on 127.0.0.1:%d.\n", cfg.OTLPPort)
			return nil
		}
		time.Sleep(300 * time.Millisecond)
	}
	fmt.Fprintf(out, "Warning: collector not answering on port %d yet; check 'hcli service status'.\n", cfg.OTLPPort)
	return nil
}

func runServiceUninstall(cmd *cobra.Command, args []string) error {
	out := cmd.OutOrStdout()
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home dir: %w", err)
	}

	switch runtime.GOOS {
	case "linux":
		_ = runCommand([]string{"systemctl", "--user", "disable", "--now", systemdUnitName})
		if err := os.Remove(systemdUnitPath(home)); err != nil && !os.IsNotExist(err) {
			return err
		}
		_ = runCommand([]string{"systemctl", "--user", "daemon-reload"})
	case "darwin":
		plistPath := launchdPlistPath(home)
		_ = runCommand([]string{"launchctl", "unload", "-w", plistPath})
		if err := os.Remove(plistPath); err != nil && !os.IsNotExist(err) {
			return err
		}
	default:
		return fmt.Errorf("service install is not supported on %s", runtime.GOOS)
	}
	fmt.Fprintln(out, "Service stopped and removed.")
	return nil
}

func runServiceStatus(cmd *cobra.Command, args []string) error {
	out := cmd.OutOrStdout()
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home dir: %w", err)
	}

	installed := false
	switch runtime.GOOS {
	case "linux":
		if _, err := os.Stat(systemdUnitPath(home)); err == nil {
			installed = true
		}
	case "darwin":
		if _, err := os.Stat(launchdPlistPath(home)); err == nil {
			installed = true
		}
	}
	if installed {
		fmt.Fprintln(out, "service: installed")
	} else {
		fmt.Fprintln(out, "service: not installed (run 'hcli service install')")
	}

	cfg, _, err := loadCLIConfigAndDB()
	if err != nil {
		return err
	}
	if collect.HealthCheck(cfg.OTLPPort, 300*time.Millisecond) {
		fmt.Fprintf(out, "collector: listening on 127.0.0.1:%d\n", cfg.OTLPPort)
	} else {
		fmt.Fprintf(out, "collector: not answering on 127.0.0.1:%d\n", cfg.OTLPPort)
	}
	return nil
}

// runCommand executes a small management command, discarding output on
// success. Kept as a seam so tests can stub the binaries via PATH.
func runCommand(argv []string) error {
	c := exec.Command(argv[0], argv[1:]...)
	c.Stdout = nil
	c.Stderr = nil
	return c.Run()
}
