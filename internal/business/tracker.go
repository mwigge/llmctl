// Package business provides cost and usage aggregation over inference metrics.
package business

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"sort"
	"strconv"
	"time"

	"github.com/mwigge/llmctl/internal/metrics"
)

// Summary holds aggregated metrics for a single calendar day.
type Summary struct {
	Date          time.Time
	TotalRequests int
	TokensIn      int
	TokensOut     int
	CostUSD       float64
	AvgLatencyMs  float64
	P95LatencyMs  float64
	TopModels     []string
}

// Tracker aggregates inference metrics into business-level summaries.
type Tracker struct {
	store        metrics.Store
	costPerToken float64
}

// NewTracker returns a Tracker backed by store. costPerToken is used to
// compute USD cost when the per-row cost is not already stored.
func NewTracker(store metrics.Store, costPerToken float64) *Tracker {
	return &Tracker{store: store, costPerToken: costPerToken}
}

// DailySummary returns an aggregated Summary for all rows whose recorded_at
// timestamp falls within the calendar day containing date (UTC).
func (t *Tracker) DailySummary(ctx context.Context, date time.Time) (Summary, error) {
	since, until := dayBounds(date)
	rows, err := t.store.Query(metrics.Query{Since: since, Until: until})
	if err != nil {
		return Summary{}, fmt.Errorf("query daily metrics: %w", err)
	}
	return aggregate(date.Truncate(24*time.Hour), rows), nil
}

// WeeklySummary returns one Summary per calendar day for the last 7 days
// (UTC), ordered chronologically.
func (t *Tracker) WeeklySummary(ctx context.Context) ([]Summary, error) {
	today := time.Now().UTC().Truncate(24 * time.Hour)
	since := today.Add(-6 * 24 * time.Hour)

	rows, err := t.store.Query(metrics.Query{Since: since})
	if err != nil {
		return nil, fmt.Errorf("query weekly metrics: %w", err)
	}

	// Group rows by calendar day.
	byDay := make(map[time.Time][]metrics.Row)
	for _, r := range rows {
		day := r.RecordedAt.UTC().Truncate(24 * time.Hour)
		byDay[day] = append(byDay[day], r)
	}

	days := make([]time.Time, 0, len(byDay))
	for d := range byDay {
		days = append(days, d)
	}
	sort.Slice(days, func(i, j int) bool { return days[i].Before(days[j]) })

	summaries := make([]Summary, 0, len(days))
	for _, d := range days {
		summaries = append(summaries, aggregate(d, byDay[d]))
	}
	return summaries, nil
}

// ExportCSV writes all rows recorded at or after since to w in CSV format.
// The CSV includes a header row and one data row per inference entry.
func (t *Tracker) ExportCSV(ctx context.Context, w io.Writer, since time.Time) error {
	rows, err := t.store.Query(metrics.Query{Since: since})
	if err != nil {
		return fmt.Errorf("query for export: %w", err)
	}

	cw := csv.NewWriter(w)
	if err := cw.Write([]string{"date", "model", "tokens_in", "tokens_out", "cost", "latency_ms"}); err != nil {
		return fmt.Errorf("write csv header: %w", err)
	}

	for _, r := range rows {
		record := []string{
			r.RecordedAt.UTC().Format(time.RFC3339),
			r.Model,
			strconv.Itoa(r.InputTokens),
			strconv.Itoa(r.OutputTokens),
			strconv.FormatFloat(r.Cost, 'f', 6, 64),
			strconv.FormatInt(r.LatencyMs, 10),
		}
		if err := cw.Write(record); err != nil {
			return fmt.Errorf("write csv record: %w", err)
		}
	}

	cw.Flush()
	return cw.Error()
}

// dayBounds returns the UTC [start, end) of the calendar day containing t.
func dayBounds(t time.Time) (time.Time, time.Time) {
	start := t.UTC().Truncate(24 * time.Hour)
	return start, start.Add(24 * time.Hour)
}

// aggregate builds a Summary from a pre-filtered slice of rows.
func aggregate(day time.Time, rows []metrics.Row) Summary {
	if len(rows) == 0 {
		return Summary{Date: day}
	}

	var totalIn, totalOut int
	var totalCost, totalLatency float64
	latencies := make([]float64, 0, len(rows))
	modelCount := make(map[string]int)

	for _, r := range rows {
		totalIn += r.InputTokens
		totalOut += r.OutputTokens
		totalCost += r.Cost
		totalLatency += float64(r.LatencyMs)
		latencies = append(latencies, float64(r.LatencyMs))
		modelCount[r.Model]++
	}

	sort.Float64s(latencies)

	return Summary{
		Date:          day,
		TotalRequests: len(rows),
		TokensIn:      totalIn,
		TokensOut:     totalOut,
		CostUSD:       totalCost,
		AvgLatencyMs:  totalLatency / float64(len(rows)),
		P95LatencyMs:  percentile(latencies, 95),
		TopModels:     topModels(modelCount, 3),
	}
}

// percentile returns the p-th percentile of a sorted slice.
func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(float64(len(sorted)-1) * p / 100.0)
	return sorted[idx]
}

// topModels returns model names ordered by count descending, capped at n.
func topModels(counts map[string]int, n int) []string {
	type pair struct {
		name  string
		count int
	}
	pairs := make([]pair, 0, len(counts))
	for name, count := range counts {
		pairs = append(pairs, pair{name, count})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].count != pairs[j].count {
			return pairs[i].count > pairs[j].count
		}
		return pairs[i].name < pairs[j].name
	})

	if n > len(pairs) {
		n = len(pairs)
	}
	names := make([]string, n)
	for i := range n {
		names[i] = pairs[i].name
	}
	return names
}
