package main

import (
	"context"
	"fmt"
	"math"
	"runtime"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"go.opentelemetry.io/otel/attribute"

	"github.com/mwigge/llmctl/internal/config"
	"github.com/mwigge/llmctl/internal/metrics"
	llmotel "github.com/mwigge/llmctl/internal/otel"
)

func newObserveCmd(cfgPath *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "observe",
		Short: "record model-serving operations and AI observability events",
	}
	cmd.AddCommand(
		newObserveSnapshotCmd(cfgPath),
		newObserveDriftCmd(cfgPath),
		newObserveUsageCmd(cfgPath),
		newObserveShowCmd(cfgPath),
	)
	return cmd
}

func newObserveSnapshotCmd(cfgPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "snapshot",
		Short: "record OS/runtime resource telemetry for the model server",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, store, shutdown, err := openObservability(cmd.Context(), *cfgPath)
			if err != nil {
				return err
			}
			defer func() { _ = store.Close() }()
			defer func() {
				if shutdown != nil {
					_ = shutdown(context.Background())
				}
			}()
			observations := resourceSnapshot(cfg)
			for _, obs := range observations {
				if err := store.RecordObservation(obs); err != nil {
					return err
				}
				llmotel.RecordOperation(cmd.Context(), obs.Source, obs.Value, obs.Unit,
					attribute.String("kind", obs.Kind),
					attribute.String("model", obs.Model),
				)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "[ok] recorded %d operations observations\n", len(observations))
			return nil
		},
	}
}

func newObserveDriftCmd(cfgPath *string) *cobra.Command {
	var modelName, baseline, sample, source string
	cmd := &cobra.Command{
		Use:   "drift",
		Short: "record a drift score for a model output sample",
		RunE: func(cmd *cobra.Command, args []string) error {
			if modelName == "" {
				modelName = defaultModelAlias(*cfgPath)
			}
			score := textDriftScore(baseline, sample)
			_, store, shutdown, err := openObservability(cmd.Context(), *cfgPath)
			if err != nil {
				return err
			}
			defer func() { _ = store.Close() }()
			defer func() {
				if shutdown != nil {
					_ = shutdown(context.Background())
				}
			}()
			obs := metrics.Observation{
				Kind:   "ai.drift",
				Model:  modelName,
				Source: source,
				Value:  score,
				Unit:   "score",
				Attributes: map[string]string{
					"baseline_chars": strconv.Itoa(len(baseline)),
					"sample_chars":   strconv.Itoa(len(sample)),
				},
			}
			if err := store.RecordObservation(obs); err != nil {
				return err
			}
			llmotel.RecordDrift(cmd.Context(), modelName, score, source)
			fmt.Fprintf(cmd.OutOrStdout(), "drift_score\t%.4f\n", score)
			return nil
		},
	}
	cmd.Flags().StringVar(&modelName, "model", "", "model alias")
	cmd.Flags().StringVar(&baseline, "baseline", "", "baseline text")
	cmd.Flags().StringVar(&sample, "sample", "", "sample text")
	cmd.Flags().StringVar(&source, "source", "manual", "source label for this drift event")
	return cmd
}

func newObserveUsageCmd(cfgPath *string) *cobra.Command {
	var modelName string
	var inTok, outTok int
	var latency int64
	cmd := &cobra.Command{
		Use:   "usage",
		Short: "record usage/tokens/latency for an external client request",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, store, shutdown, err := openObservability(cmd.Context(), *cfgPath)
			if err != nil {
				return err
			}
			defer func() { _ = store.Close() }()
			defer func() {
				if shutdown != nil {
					_ = shutdown(context.Background())
				}
			}()
			if modelName == "" {
				modelName = defaultModelAlias(*cfgPath)
			}
			entry := metrics.Entry{
				Model:        modelName,
				InputTokens:  inTok,
				OutputTokens: outTok,
				LatencyMs:    latency,
				CostPerToken: cfg.Business.CostPerToken,
			}
			if err := store.Record(entry); err != nil {
				return err
			}
			llmotel.RecordRequest(cmd.Context(), modelName, inTok, outTok, latency)
			fmt.Fprintln(cmd.OutOrStdout(), "[ok] usage recorded")
			return nil
		},
	}
	cmd.Flags().StringVar(&modelName, "model", "", "model alias")
	cmd.Flags().IntVar(&inTok, "input-tokens", 0, "prompt/input token count")
	cmd.Flags().IntVar(&outTok, "output-tokens", 0, "completion/output token count")
	cmd.Flags().Int64Var(&latency, "latency-ms", 0, "request latency in milliseconds")
	return cmd
}

func newObserveShowCmd(cfgPath *string) *cobra.Command {
	var kind, modelName, since string
	cmd := &cobra.Command{
		Use:   "show",
		Short: "show recorded operations/drift observations",
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
			q := metrics.ObservationQuery{Kind: kind, Model: modelName}
			if since != "" {
				d, err := time.ParseDuration(since)
				if err != nil {
					return err
				}
				q.Since = time.Now().UTC().Add(-d)
			}
			rows, err := store.QueryObservations(q)
			if err != nil {
				return err
			}
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "time\tkind\tmodel\tsource\tvalue\tunit")
			for _, r := range rows {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%.4f\t%s\n",
					r.RecordedAt.Format(time.RFC3339), r.Kind, r.Model, r.Source, r.Value, r.Unit)
			}
			return w.Flush()
		},
	}
	cmd.Flags().StringVar(&kind, "kind", "", "filter by kind, e.g. ops.memory or ai.drift")
	cmd.Flags().StringVar(&modelName, "model", "", "filter by model alias")
	cmd.Flags().StringVar(&since, "since", "", "duration window e.g. 24h")
	return cmd
}

func openObservability(ctx context.Context, cfgPath string) (*config.Config, metrics.Store, func(context.Context) error, error) {
	cfg, err := loadConfig(cfgPath)
	if err != nil {
		return nil, nil, nil, err
	}
	store, err := openMetricsStore(cfg)
	if err != nil {
		return nil, nil, nil, err
	}
	var shutdown func(context.Context) error
	if cfg.OTel.Endpoint != "" {
		shutdown, err = llmotel.Setup(llmotel.Config{
			Endpoint:    cfg.OTel.Endpoint,
			ServiceName: defaultString(cfg.OTel.ServiceName, "llmctl"),
		})
		if err != nil {
			_ = store.Close()
			return nil, nil, nil, err
		}
	}
	return cfg, store, shutdown, nil
}

func resourceSnapshot(cfg *config.Config) []metrics.Observation {
	modelName := ""
	if len(cfg.Models) > 0 {
		modelName = cfg.Models[0].Alias
	}
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	now := time.Now().UTC()
	out := []metrics.Observation{
		{Kind: "ops.cpu", Model: modelName, Source: "runtime.num_cpu", Value: float64(runtime.NumCPU()), Unit: "cores", RecordedAt: now},
		{Kind: "ops.cpu", Model: modelName, Source: "server.threads", Value: float64(cfg.Server.Threads), Unit: "threads", RecordedAt: now},
		{Kind: "ops.memory", Model: modelName, Source: "runtime.alloc", Value: float64(mem.Alloc), Unit: "bytes", RecordedAt: now},
		{Kind: "ops.memory", Model: modelName, Source: "system.total", Value: systemRAMGB() * 1024 * 1024 * 1024, Unit: "bytes", RecordedAt: now},
	}
	if gpu, err := detectBestLocalGPU(); err == nil {
		out = append(out, metrics.Observation{
			Kind: "ops.gpu", Model: modelName, Source: gpu.Vendor + ".vram", Value: gpu.VRAMGB, Unit: "gb", RecordedAt: now,
			Attributes: map[string]string{"name": gpu.Name},
		})
	}
	return out
}

func textDriftScore(baseline, sample string) float64 {
	base := tokenSet(baseline)
	candidate := tokenSet(sample)
	if len(base) == 0 && len(candidate) == 0 {
		return 0
	}
	var intersection int
	for tok := range base {
		if candidate[tok] {
			intersection++
		}
	}
	union := len(base) + len(candidate) - intersection
	if union == 0 {
		return 0
	}
	return math.Max(0, 1-(float64(intersection)/float64(union)))
}

func tokenSet(s string) map[string]bool {
	out := map[string]bool{}
	for _, tok := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9') && r != '_'
	}) {
		if tok != "" {
			out[tok] = true
		}
	}
	return out
}

func defaultModelAlias(cfgPath string) string {
	cfg, err := config.Load(cfgPath)
	if err == nil && len(cfg.Models) > 0 && cfg.Models[0].Alias != "" {
		return cfg.Models[0].Alias
	}
	return "local"
}
