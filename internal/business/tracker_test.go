package business_test

import (
	"bytes"
	"context"
	"encoding/csv"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/mwigge/llmctl/internal/business"
	"github.com/mwigge/llmctl/internal/metrics"
)

func newTestStore(t *testing.T) metrics.Store {
	t.Helper()
	store, err := metrics.NewSQLiteStore(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

// seedRows inserts entries into store across two calendar days.
// day1: 3 rows for "model-a", day2: 2 rows for "model-b".
// Dates are relative to today so they fall within WeeklySummary's 7-day window.
func seedRows(t *testing.T, store metrics.Store) (day1, day2 time.Time) {
	t.Helper()
	today := time.Now().UTC().Truncate(24 * time.Hour)
	day1 = today.Add(-2 * 24 * time.Hour).Add(10 * time.Hour) // 2 days ago at 10:00
	day2 = today.Add(-1 * 24 * time.Hour).Add(10 * time.Hour) // yesterday at 10:00

	rows := []metrics.Entry{
		{Model: "model-a", InputTokens: 100, OutputTokens: 50, LatencyMs: 200, CostPerToken: 0.0001, RecordedAt: day1},
		{Model: "model-a", InputTokens: 200, OutputTokens: 80, LatencyMs: 400, CostPerToken: 0.0001, RecordedAt: day1.Add(time.Hour)},
		{Model: "model-a", InputTokens: 50, OutputTokens: 20, LatencyMs: 100, CostPerToken: 0.0001, RecordedAt: day1.Add(2 * time.Hour)},
		{Model: "model-b", InputTokens: 300, OutputTokens: 90, LatencyMs: 600, CostPerToken: 0.0001, RecordedAt: day2},
		{Model: "model-b", InputTokens: 120, OutputTokens: 60, LatencyMs: 300, CostPerToken: 0.0001, RecordedAt: day2.Add(time.Hour)},
	}
	for _, e := range rows {
		if err := store.Record(e); err != nil {
			t.Fatalf("Record: %v", err)
		}
	}
	return day1, day2
}

func TestTracker_DailySummary_Day1(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	day1, _ := seedRows(t, store)

	tracker := business.NewTracker(store, 0.0001)
	summary, err := tracker.DailySummary(context.Background(), day1)
	if err != nil {
		t.Fatalf("DailySummary: %v", err)
	}

	if summary.TotalRequests != 3 {
		t.Errorf("TotalRequests = %d, want 3", summary.TotalRequests)
	}
	if summary.TokensIn != 350 {
		t.Errorf("TokensIn = %d, want 350", summary.TokensIn)
	}
	if summary.TokensOut != 150 {
		t.Errorf("TokensOut = %d, want 150", summary.TokensOut)
	}
}

func TestTracker_DailySummary_Day2(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	_, day2 := seedRows(t, store)

	tracker := business.NewTracker(store, 0.0001)
	summary, err := tracker.DailySummary(context.Background(), day2)
	if err != nil {
		t.Fatalf("DailySummary: %v", err)
	}

	if summary.TotalRequests != 2 {
		t.Errorf("TotalRequests = %d, want 2", summary.TotalRequests)
	}
}

func TestTracker_WeeklySummary(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	seedRows(t, store)

	tracker := business.NewTracker(store, 0.0001)
	summaries, err := tracker.WeeklySummary(context.Background())
	if err != nil {
		t.Fatalf("WeeklySummary: %v", err)
	}
	// We seeded 2 distinct days.
	if len(summaries) < 2 {
		t.Errorf("WeeklySummary returned %d entries, want at least 2", len(summaries))
	}
}

func TestTracker_ExportCSV(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	day1, _ := seedRows(t, store)

	tracker := business.NewTracker(store, 0.0001)
	var buf bytes.Buffer
	err := tracker.ExportCSV(context.Background(), &buf, day1.Add(-time.Minute))
	if err != nil {
		t.Fatalf("ExportCSV: %v", err)
	}

	r := csv.NewReader(&buf)
	records, err := r.ReadAll()
	if err != nil {
		t.Fatalf("CSV parse error: %v", err)
	}
	// Header row + 5 data rows.
	if len(records) != 6 {
		t.Errorf("CSV rows = %d, want 6 (1 header + 5 data)", len(records))
	}

	// Verify header fields.
	header := records[0]
	wantFields := []string{"date", "model", "tokens_in", "tokens_out", "cost", "latency_ms"}
	for i, f := range wantFields {
		if i >= len(header) || header[i] != f {
			t.Errorf("header[%d] = %q, want %q", i, header[i], f)
		}
	}

	// Spot check first data row tokens_in parses as integer.
	if _, err := strconv.Atoi(records[1][2]); err != nil {
		t.Errorf("tokens_in field not parseable as int: %q", records[1][2])
	}
}

func TestTracker_DailySummary_Empty(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	tracker := business.NewTracker(store, 0.0001)

	summary, err := tracker.DailySummary(context.Background(), time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("DailySummary on empty store: %v", err)
	}
	if summary.TotalRequests != 0 {
		t.Errorf("TotalRequests = %d, want 0 on empty store", summary.TotalRequests)
	}
}
