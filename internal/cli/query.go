package cli

import (
	"fmt"
	"strings"

	"github.com/justinmaks/hedge-local/internal/store"
	"github.com/spf13/cobra"
)

var queryCmd = &cobra.Command{
	Use:   "query <sql>",
	Short: "Run a read-only SQL query against the hcli database",
	Long:  "Executes a SELECT or WITH query against the local SQLite database\nand prints results as a table. Only read-only queries are allowed.",
	Args:  cobra.ExactArgs(1),
	RunE:  runQuery,
}

func init() {
	rootCmd.AddCommand(queryCmd)
}

func runQuery(cmd *cobra.Command, args []string) error {
	sqlText := args[0]
	trimmed := strings.TrimSpace(strings.ToUpper(sqlText))
	if !strings.HasPrefix(trimmed, "SELECT") && !strings.HasPrefix(trimmed, "WITH") {
		return fmt.Errorf("only SELECT or WITH queries are allowed")
	}

	_, db, err := loadCLIConfigAndDB()
	if err != nil {
		return err
	}

	// Ensure the schema exists (idempotent) so queries on a fresh machine
	// return empty results rather than "no such table", then run the user's
	// SQL on a read-only connection that refuses writes.
	init, err2 := store.New(db)
	if err2 != nil {
		return fmt.Errorf("open store: %w", err2)
	}
	init.Close()

	s, err := store.NewReadOnly(db)
	if err != nil {
		return fmt.Errorf("open store (read-only): %w", err)
	}
	defer s.Close()

	cols, rows, err := s.QueryRaw(sqlText)
	if err != nil {
		return fmt.Errorf("query: %w", err)
	}

	out := cmd.OutOrStdout()
	fmt.Fprintln(out, strings.Join(cols, "\t"))
	for _, row := range rows {
		fmt.Fprintln(out, strings.Join(row, "\t"))
	}
	return nil
}
