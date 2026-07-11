package cli

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/justinmaks/hedge-local/internal/store"
	"github.com/spf13/cobra"
)

var pricingFetchURL = "https://raw.githubusercontent.com/justinmaks/hedge-local/main/dist/pricing/pricing.json"

var pricingCmd = &cobra.Command{Use: "pricing", Short: "Manage local model pricing data"}
var pricingListCmd = &cobra.Command{Use: "list", Short: "List local pricing rows", RunE: runPricingList}
var pricingImportCmd = &cobra.Command{Use: "import <pricing.json>", Short: "Import pricing JSON", Args: cobra.ExactArgs(1), RunE: runPricingImport}
var pricingFetchCmd = &cobra.Command{Use: "fetch", Short: "Fetch pricing JSON explicitly", RunE: runPricingFetch}

func init() {
	pricingFetchCmd.Flags().StringVar(&pricingFetchURL, "url", pricingFetchURL, "pricing JSON URL")
	pricingCmd.AddCommand(pricingListCmd, pricingImportCmd, pricingFetchCmd)
	rootCmd.AddCommand(pricingCmd)
}

func openCLIStore() (*store.Store, error) {
	_, db, err := loadCLIConfigAndDB()
	if err != nil {
		return nil, err
	}
	s, err := store.New(db)
	if err != nil {
		return nil, fmt.Errorf("open store: %w", err)
	}
	return s, nil
}

func runPricingList(cmd *cobra.Command, args []string) error {
	s, err := openCLIStore()
	if err != nil {
		return err
	}
	defer s.Close()
	rows, err := s.ListPricing()
	if err != nil {
		return err
	}
	out := cmd.OutOrStdout()
	fmt.Fprintln(out, "provider\tmodel\tinput_per_1m\toutput_per_1m\tcache_read_per_1m\tcache_write_per_1m\teffective_from\tsource")
	for _, row := range rows {
		fmt.Fprintf(out, "%s\t%s\t%.6g\t%.6g\t%.6g\t%.6g\t%s\t%s\n", row.Provider, row.Model, row.InputPer1M, row.OutputPer1M, row.CacheReadPer1M, row.CacheWritePer1M, row.EffectiveFrom.Format(time.RFC3339), row.Source)
	}
	return nil
}

func runPricingImport(cmd *cobra.Command, args []string) error {
	data, err := os.ReadFile(args[0])
	if err != nil {
		return fmt.Errorf("read pricing json: %w", err)
	}
	s, err := openCLIStore()
	if err != nil {
		return err
	}
	defer s.Close()
	if err := s.ImportPricingJSON(data, "imported"); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Imported pricing from %s\n", args[0])
	return nil
}

func runPricingFetch(cmd *cobra.Command, args []string) error {
	if !strings.HasPrefix(pricingFetchURL, "https://") && !strings.HasPrefix(pricingFetchURL, "http://") {
		return fmt.Errorf("pricing url must include http:// or https://")
	}
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(pricingFetchURL)
	if err != nil {
		return fmt.Errorf("fetch pricing: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("fetch pricing: status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return fmt.Errorf("read pricing response: %w", err)
	}
	s, err := openCLIStore()
	if err != nil {
		return err
	}
	defer s.Close()
	if err := s.ImportPricingJSON(data, "fetched"); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Fetched pricing from %s\n", pricingFetchURL)
	return nil
}
