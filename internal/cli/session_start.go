package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/justinmaks/hedge-local/internal/store"
	"github.com/spf13/cobra"
)

var sessionStartCmd = &cobra.Command{
	Use:    "session-start",
	Hidden: true,
	Short:  "Claude Code SessionStart hook target (reads hook JSON on stdin)",
	Long: `Reads the Claude Code hook payload from stdin and attributes the
session to the project it was started in. Installed automatically into
~/.claude/settings.json by 'hcli setup claude'; not meant to be run by hand.

Telemetry itself carries no working-directory information, so this hook is
what makes the Projects view group sessions by repo without wrapping the
claude binary.`,
	RunE: runSessionStart,
}

func init() {
	rootCmd.AddCommand(sessionStartCmd)
}

// hookPayload is the subset of the Claude Code hook input we use.
type hookPayload struct {
	SessionID string `json:"session_id"`
	CWD       string `json:"cwd"`
}

func runSessionStart(cmd *cobra.Command, args []string) error {
	data, err := io.ReadAll(io.LimitReader(cmd.InOrStdin(), 1<<20))
	if err != nil {
		return fmt.Errorf("read hook payload: %w", err)
	}
	var payload hookPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return fmt.Errorf("parse hook payload: %w", err)
	}
	if payload.SessionID == "" || payload.CWD == "" {
		// Nothing to attribute; exit quietly so the hook never disturbs
		// the agent session.
		return nil
	}

	_, db, err := loadCLIConfigAndDB()
	if err != nil {
		return err
	}
	s, err := store.New(db)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer s.Close()

	name := filepath.Base(payload.CWD)
	if name == "" || name == "." || name == "/" {
		name = "(ungrouped)"
	}
	projectID, err := s.ProjectUpsert(payload.CWD, name)
	if err != nil {
		return fmt.Errorf("upsert project: %w", err)
	}
	// Create the session row if telemetry has not yet (ON CONFLICT keeps an
	// existing row), then attribute it either way.
	if _, err := s.SessionUpsert(payload.SessionID, "claude_code", projectID, time.Now(), ""); err != nil {
		return fmt.Errorf("upsert session: %w", err)
	}
	if err := s.SessionSetProject(payload.SessionID, projectID); err != nil {
		return fmt.Errorf("set session project: %w", err)
	}
	return nil
}
