package cli

import (
	"bytes"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/justinmaks/hedge-local/internal/store"
	appui "github.com/justinmaks/hedge-local/internal/tui"
	"github.com/justinmaks/hedge-local/internal/tui/queries"
	"github.com/spf13/cobra"
)

func TestRunTUIUsesConfiguredDBAndSeedsPricing(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", filepath.Join(dir, "home"))
	db := filepath.Join(dir, "custom.db")
	cfgPath := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(cfgPath, []byte("db_path = \""+db+"\"\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	oldDB := dbPath
	oldCfg := cfgFile
	oldRun := runTUIApp
	dbPath = ""
	cfgFile = cfgPath
	t.Cleanup(func() {
		dbPath = oldDB
		cfgFile = oldCfg
		runTUIApp = oldRun
	})

	called := false
	runTUIApp = func(svc *queries.Service, collecting bool, probe func() bool) error {
		called = true
		if collecting {
			t.Fatalf("collecting = true, want false")
		}
		app := appui.NewApp(svc, collecting)
		model, _ := app.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
		rendered := model.(*appui.App).View()
		if strings.Contains(rendered, "not implemented yet") {
			t.Fatalf("expected concrete views to be registered, got: %s", rendered)
		}
		rows, err := svc.Store().ListPricing()
		if err != nil {
			t.Fatalf("ListPricing: %v", err)
		}
		if len(rows) == 0 {
			t.Fatal("expected bundled pricing to be seeded")
		}
		return nil
	}

	if err := runTUI(&cobra.Command{}, nil); err != nil {
		t.Fatalf("runTUI: %v", err)
	}
	if !called {
		t.Fatal("expected TUI runner to be called")
	}
	if _, err := os.Stat(db); err != nil {
		t.Fatalf("configured db was not created: %v", err)
	}

	s, err := store.New(db)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer s.Close()
	rows, err := s.ListPricing()
	if err != nil {
		t.Fatalf("ListPricing after runTUI: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("expected pricing rows in configured db")
	}
}

func TestRunTUIShowsCollectingWhenCollectorAnswers(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", filepath.Join(dir, "home"))
	db := filepath.Join(dir, "custom.db")

	// A live collector is detected via its /health endpoint, regardless of
	// whether it is the daemon, a service, or another hcli process.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	port := srv.Listener.Addr().(*net.TCPAddr).Port

	cfgPath := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(cfgPath, []byte("db_path = \""+db+"\"\notlp_port = "+strconv.Itoa(port)+"\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	oldDB := dbPath
	oldCfg := cfgFile
	oldRun := runTUIApp
	dbPath = ""
	cfgFile = cfgPath
	t.Cleanup(func() {
		dbPath = oldDB
		cfgFile = oldCfg
		runTUIApp = oldRun
	})

	called := false
	runTUIApp = func(svc *queries.Service, collecting bool, probe func() bool) error {
		called = true
		if !collecting {
			t.Fatalf("collecting = false, want true when a collector answers /health")
		}
		if probe == nil || !probe() {
			t.Fatal("expected a live probe to be provided")
		}
		return nil
	}

	if err := runTUI(&cobra.Command{}, nil); err != nil {
		t.Fatalf("runTUI: %v", err)
	}
	if !called {
		t.Fatal("expected TUI runner to be called")
	}
}

func TestRunRootStartsEmbeddedReceiverAndRunsCollectingTUI(t *testing.T) {
	dir := t.TempDir()
	db := filepath.Join(dir, "embedded.db")
	cfgPath := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(cfgPath, []byte("db_path = \""+db+"\"\notlp_port = 0\nwith_logs = true\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	oldDB := dbPath
	oldCfg := cfgFile
	oldRun := runTUIApp
	dbPath = ""
	cfgFile = cfgPath
	t.Cleanup(func() {
		dbPath = oldDB
		cfgFile = oldCfg
		runTUIApp = oldRun
	})

	called := false
	runTUIApp = func(svc *queries.Service, collecting bool, probe func() bool) error {
		called = true
		if !collecting {
			t.Fatalf("collecting = false, want true")
		}
		rows, err := svc.Store().ListPricing()
		if err != nil {
			t.Fatalf("ListPricing: %v", err)
		}
		if len(rows) == 0 {
			t.Fatal("expected bundled pricing to be seeded")
		}
		return nil
	}

	if err := runRoot(&cobra.Command{}, nil); err != nil {
		t.Fatalf("runRoot: %v", err)
	}
	if !called {
		t.Fatal("expected TUI runner to be called")
	}
	if _, err := os.Stat(db); err != nil {
		t.Fatalf("configured db was not created: %v", err)
	}
	s, err := store.New(db)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer s.Close()
}

func TestRunRootFallsBackToNonCollectingTUIWhenReceiverStartFails(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port

	dir := t.TempDir()
	db := filepath.Join(dir, "fallback.db")
	cfgPath := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(cfgPath, []byte("db_path = \""+db+"\"\notlp_port = "+strconv.Itoa(port)+"\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	oldDB := dbPath
	oldCfg := cfgFile
	oldRun := runTUIApp
	dbPath = ""
	cfgFile = cfgPath
	t.Cleanup(func() {
		dbPath = oldDB
		cfgFile = oldCfg
		runTUIApp = oldRun
	})

	var errBuf bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetErr(&errBuf)

	called := false
	runTUIApp = func(svc *queries.Service, collecting bool, probe func() bool) error {
		called = true
		if collecting {
			t.Fatalf("collecting = true, want false")
		}
		rows, err := svc.Store().ListPricing()
		if err != nil {
			t.Fatalf("ListPricing: %v", err)
		}
		if len(rows) == 0 {
			t.Fatal("expected bundled pricing to be seeded")
		}
		return nil
	}

	if err := runRoot(cmd, nil); err != nil {
		t.Fatalf("runRoot: %v", err)
	}
	if !called {
		t.Fatal("expected TUI runner to be called")
	}
	if !strings.Contains(errBuf.String(), "Warning: could not start receiver") {
		t.Fatalf("expected warning on stderr, got: %s", errBuf.String())
	}
	if _, err := os.Stat(db); err != nil {
		t.Fatalf("configured db was not created: %v", err)
	}
}
