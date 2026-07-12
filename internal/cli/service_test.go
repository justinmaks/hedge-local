package cli

import (
	"bytes"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestSystemdUnit_content(t *testing.T) {
	unit := systemdUnit("/usr/local/bin/hcli")
	for _, want := range []string{
		"ExecStart=/usr/local/bin/hcli collect",
		"Restart=on-failure",
		"WantedBy=default.target",
	} {
		if !strings.Contains(unit, want) {
			t.Errorf("unit missing %q:\n%s", want, unit)
		}
	}
}

func TestLaunchdPlist_content(t *testing.T) {
	plist := launchdPlist("/usr/local/bin/hcli")
	for _, want := range []string{
		"<string>/usr/local/bin/hcli</string>",
		"<string>collect</string>",
		"<key>KeepAlive</key>",
		"<key>RunAtLoad</key>",
		launchdLabel,
	} {
		if !strings.Contains(plist, want) {
			t.Errorf("plist missing %q:\n%s", want, plist)
		}
	}
}

// stubManagementTools puts fake systemctl/loginctl/launchctl on PATH that
// log their argv and succeed, and returns the log file path.
func stubManagementTools(t *testing.T) string {
	t.Helper()
	binDir := t.TempDir()
	logPath := filepath.Join(binDir, "calls.log")
	script := "#!/bin/sh\necho \"$(basename $0) $@\" >> " + logPath + "\nexit 0\n"
	for _, name := range []string{"systemctl", "loginctl", "launchctl"} {
		if err := os.WriteFile(filepath.Join(binDir, name), []byte(script), 0755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return logPath
}

func serviceTestEnv(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	oldCfg, oldDB := cfgFile, dbPath
	t.Cleanup(func() { cfgFile, dbPath = oldCfg, oldDB })
	cfgFile, dbPath = "", ""

	// A live /health endpoint so install's readiness poll succeeds fast.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	port := srv.Listener.Addr().(*net.TCPAddr).Port
	if err := os.MkdirAll(filepath.Join(home, ".hedge"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".hedge", "config.toml"),
		[]byte("otlp_port = "+strconv.Itoa(port)+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	return home
}

func TestServiceInstall_linux(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("linux-only path")
	}
	home := serviceTestEnv(t)
	logPath := stubManagementTools(t)

	var out bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&out)
	if err := runServiceInstall(cmd, nil); err != nil {
		t.Fatalf("install: %v", err)
	}

	unitPath := systemdUnitPath(home)
	data, err := os.ReadFile(unitPath)
	if err != nil {
		t.Fatalf("unit not written: %v", err)
	}
	if !strings.Contains(string(data), "collect") {
		t.Errorf("unit content wrong:\n%s", data)
	}

	calls, _ := os.ReadFile(logPath)
	for _, want := range []string{
		"systemctl --user daemon-reload",
		"systemctl --user enable --now " + systemdUnitName,
		"loginctl enable-linger",
	} {
		if !strings.Contains(string(calls), want) {
			t.Errorf("missing management call %q, got:\n%s", want, calls)
		}
	}
	if !strings.Contains(out.String(), "Collector is listening") {
		t.Errorf("should confirm collector liveness:\n%s", out.String())
	}
}

func TestServiceInstall_refusesWhenDaemonRunning(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("linux-only path")
	}
	serviceTestEnv(t)
	stubManagementTools(t)
	if err := writePIDFile(defaultPIDPath(), os.Getpid()); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&out)
	if err := runServiceInstall(cmd, nil); err == nil {
		t.Fatal("install should refuse while a pidfile daemon is alive")
	}
}

func TestServiceUninstall_linux(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("linux-only path")
	}
	home := serviceTestEnv(t)
	logPath := stubManagementTools(t)

	unitPath := systemdUnitPath(home)
	if err := os.MkdirAll(filepath.Dir(unitPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(unitPath, []byte(systemdUnit("/x/hcli")), 0644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&out)
	if err := runServiceUninstall(cmd, nil); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if _, err := os.Stat(unitPath); !os.IsNotExist(err) {
		t.Error("unit file should be removed")
	}
	calls, _ := os.ReadFile(logPath)
	if !strings.Contains(string(calls), "systemctl --user disable --now "+systemdUnitName) {
		t.Errorf("missing disable call:\n%s", calls)
	}
}

func TestServiceStatus_reportsInstalledAndListening(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("linux-only path")
	}
	home := serviceTestEnv(t)
	unitPath := systemdUnitPath(home)
	if err := os.MkdirAll(filepath.Dir(unitPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(unitPath, []byte(systemdUnit("/x/hcli")), 0644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&out)
	if err := runServiceStatus(cmd, nil); err != nil {
		t.Fatalf("status: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "service: installed") {
		t.Errorf("should report installed:\n%s", got)
	}
	if !strings.Contains(got, "collector: listening") {
		t.Errorf("should report listening (health stub is up):\n%s", got)
	}
}
