package main

import (
	"context"
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/mwigge/llmctl/internal/business"
	"github.com/mwigge/llmctl/internal/config"
	"github.com/mwigge/llmctl/internal/metrics"
)

func newMetricsCmd(cfgPath *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "metrics",
		Short: "view inference metrics",
	}
	cmd.AddCommand(
		newMetricsShowCmd(cfgPath),
		newMetricsSummaryCmd(cfgPath),
		newMetricsExportCmd(cfgPath),
		newMetricsResetCmd(cfgPath),
	)
	return cmd
}

func openMetricsStore(cfg *config.Config) (metrics.Store, error) {
	store, err := metrics.NewSQLiteStore(cfg.Metrics.DBPath)
	if err != nil {
		return nil, fmt.Errorf("open metrics db %s: %w", cfg.Metrics.DBPath, err)
	}
	return store, nil
}

func newMetricsShowCmd(cfgPath *string) *cobra.Command {
	var (
		since     string
		modelFlag string
	)
	cmd := &cobra.Command{
		Use:   "show",
		Short: "show a table of token/cost/latency metrics",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig(*cfgPath)
			if err != nil {
				return err
			}
			store, err := openMetricsStore(cfg)
			if err != nil {
				return err
			}
			defer func() { _ = store.Close() }()

			q := metrics.Query{Model: modelFlag}
			if since != "" {
				d, parseErr := time.ParseDuration(since)
				if parseErr != nil {
					return fmt.Errorf("invalid --since duration %q: %w", since, parseErr)
				}
				q.Since = time.Now().UTC().Add(-d)
			}

			rows, err := store.Query(q)
			if err != nil {
				return fmt.Errorf("query metrics: %w", err)
			}

			// Aggregate per-model summary for display.
			type modelStats struct {
				tokensIn  int
				tokensOut int
				cost      float64
				latencies []int64
			}
			byModel := make(map[string]*modelStats)
			// Preserve insertion order for deterministic output.
			order := make([]string, 0)
			for _, r := range rows {
				if _, ok := byModel[r.Model]; !ok {
					byModel[r.Model] = &modelStats{}
					order = append(order, r.Model)
				}
				s := byModel[r.Model]
				s.tokensIn += r.InputTokens
				s.tokensOut += r.OutputTokens
				s.cost += r.Cost
				s.latencies = append(s.latencies, r.LatencyMs)
			}

			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "model\ttokens_in\ttokens_out\tcost\tp50_ms\tp95_ms")
			for _, modelName := range order {
				s := byModel[modelName]
				p50 := latencyPercentile(s.latencies, 50)
				p95 := latencyPercentile(s.latencies, 95)
				fmt.Fprintf(w, "%s\t%s\t%s\t$%.2f\t%d\t%d\n",
					modelName,
					formatInt(s.tokensIn),
					formatInt(s.tokensOut),
					s.cost,
					p50,
					p95,
				)
			}
			return w.Flush()
		},
	}
	cmd.Flags().StringVar(&since, "since", "", "duration window e.g. 24h")
	cmd.Flags().StringVar(&modelFlag, "model", "", "filter by model alias")
	return cmd
}

func newMetricsSummaryCmd(cfgPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "summary",
		Short: "show daily summary: tokens in/out, cost, requests",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig(*cfgPath)
			if err != nil {
				return err
			}
			store, err := openMetricsStore(cfg)
			if err != nil {
				return err
			}
			defer func() { _ = store.Close() }()

			tracker := business.NewTracker(store, cfg.Business.CostPerToken)
			summaries, err := tracker.WeeklySummary(context.Background())
			if err != nil {
				return fmt.Errorf("weekly summary: %w", err)
			}

			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "date\trequests\ttokens_in\ttokens_out\tcost\tavg_ms")
			for _, s := range summaries {
				fmt.Fprintf(w, "%s\t%d\t%s\t%s\t$%.4f\t%.0f\n",
					s.Date.Format("2006-01-02"),
					s.TotalRequests,
					formatInt(s.TokensIn),
					formatInt(s.TokensOut),
					s.CostUSD,
					s.AvgLatencyMs,
				)
			}
			return w.Flush()
		},
	}
}

func newMetricsExportCmd(cfgPath *string) *cobra.Command {
	var csvFlag bool
	cmd := &cobra.Command{
		Use:   "export",
		Short: "export all metrics to CSV",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig(*cfgPath)
			if err != nil {
				return err
			}
			store, err := openMetricsStore(cfg)
			if err != nil {
				return err
			}
			defer func() { _ = store.Close() }()

			tracker := business.NewTracker(store, cfg.Business.CostPerToken)
			// Export everything by using zero time.
			if err := tracker.ExportCSV(context.Background(), cmd.OutOrStdout(), time.Time{}); err != nil {
				return fmt.Errorf("export csv: %w", err)
			}
			_ = csvFlag // output is always CSV
			return nil
		},
	}
	cmd.Flags().BoolVar(&csvFlag, "csv", false, "output in CSV format (default)")
	return cmd
}

func newMetricsResetCmd(cfgPath *string) *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "reset",
		Short: "clear the metrics database",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !yes {
				return fmt.Errorf("pass --yes to confirm metrics reset")
			}
			fmt.Fprintln(cmd.OutOrStdout(), "metrics reset — not yet implemented")
			return nil
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "confirm reset")
	return cmd
}

// latencyPercentile returns the p-th percentile of a slice of latencies (sorted in place).
func latencyPercentile(latencies []int64, p int) int64 {
	if len(latencies) == 0 {
		return 0
	}
	// Insertion sort — CLI usage keeps this small.
	for i := 1; i < len(latencies); i++ {
		key := latencies[i]
		j := i - 1
		for j >= 0 && latencies[j] > key {
			latencies[j+1] = latencies[j]
			j--
		}
		latencies[j+1] = key
	}
	idx := int(float64(len(latencies)-1) * float64(p) / 100.0)
	return latencies[idx]
}

// formatInt formats an integer with comma separators.
func formatInt(n int) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	result := make([]byte, 0, len(s)+len(s)/3)
	mod := len(s) % 3
	if mod == 0 {
		mod = 3
	}
	result = append(result, s[:mod]...)
	for i := mod; i < len(s); i += 3 {
		result = append(result, ',')
		result = append(result, s[i:i+3]...)
	}
	return string(result)
}
