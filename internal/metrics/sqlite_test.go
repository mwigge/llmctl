package metrics_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/mwigge/llmctl/internal/metrics"
)

func newTestStore(t *testing.T) metrics.Store {
	t.Helper()
	dir := t.TempDir()
	store, err := metrics.NewSQLiteStore(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("store.Close() error = %v", err)
		}
	})
	return store
}

func TestNewSQLiteStore_CreatesDatabase(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	rows, err := store.Query(metrics.Query{})
	if err != nil {
		t.Fatalf("Query() on empty store error = %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("empty store query returned %d rows, want 0", len(rows))
	}
}

func TestRecord_ThreeEntries(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	entries := []metrics.Entry{
		{Model: "llama3", InputTokens: 100, OutputTokens: 50, LatencyMs: 200, CostPerToken: 0.0001, RecordedAt: base},
		{Model: "mistral", InputTokens: 200, OutputTokens: 80, LatencyMs: 350, CostPerToken: 0.0001, RecordedAt: base.Add(time.Minute)},
		{Model: "llama3", InputTokens: 50, OutputTokens: 30, LatencyMs: 100, CostPerToken: 0.0001, RecordedAt: base.Add(2 * time.Minute)},
	}

	for _, e := range entries {
		if err := store.Record(e); err != nil {
			t.Fatalf("Record() error = %v", err)
		}
	}

	all, err := store.Query(metrics.Query{})
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if len(all) != 3 {
		t.Errorf("Query() returned %d rows, want 3", len(all))
	}
}

func TestQuery_ByModel(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	entries := []metrics.Entry{
		{Model: "llama3", InputTokens: 100, OutputTokens: 50, CostPerToken: 0.0001, RecordedAt: base},
		{Model: "mistral", InputTokens: 200, OutputTokens: 80, CostPerToken: 0.0001, RecordedAt: base.Add(time.Minute)},
		{Model: "llama3", InputTokens: 50, OutputTokens: 30, CostPerToken: 0.0001, RecordedAt: base.Add(2 * time.Minute)},
	}
	for _, e := range entries {
		if err := store.Record(e); err != nil {
			t.Fatalf("Record() error = %v", err)
		}
	}

	rows, err := store.Query(metrics.Query{Model: "llama3"})
	if err != nil {
		t.Fatalf("Query(model=llama3) error = %v", err)
	}
	if len(rows) != 2 {
		t.Errorf("Query(model=llama3) returned %d rows, want 2", len(rows))
	}
	for _, r := range rows {
		if r.Model != "llama3" {
			t.Errorf("row model = %q, want llama3", r.Model)
		}
	}
}

func TestQuery_ByTimeRange(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	entries := []metrics.Entry{
		{Model: "llama3", InputTokens: 10, OutputTokens: 5, CostPerToken: 0.0001, RecordedAt: base},
		{Model: "llama3", InputTokens: 20, OutputTokens: 10, CostPerToken: 0.0001, RecordedAt: base.Add(time.Hour)},
		{Model: "llama3", InputTokens: 30, OutputTokens: 15, CostPerToken: 0.0001, RecordedAt: base.Add(2 * time.Hour)},
	}
	for _, e := range entries {
		if err := store.Record(e); err != nil {
			t.Fatalf("Record() error = %v", err)
		}
	}

	// Window: [base+30m, base+90m) — should return only the second entry.
	rows, err := store.Query(metrics.Query{
		Since: base.Add(30 * time.Minute),
		Until: base.Add(90 * time.Minute),
	})
	if err != nil {
		t.Fatalf("Query(time range) error = %v", err)
	}
	if len(rows) != 1 {
		t.Errorf("time-range query returned %d rows, want 1", len(rows))
	}
}

func TestQuery_CostCalculation(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	e := metrics.Entry{
		Model:        "llama3",
		InputTokens:  100,
		OutputTokens: 50,
		CostPerToken: 0.0001,
		RecordedAt:   time.Now().UTC(),
	}
	if err := store.Record(e); err != nil {
		t.Fatalf("Record() error = %v", err)
	}

	rows, err := store.Query(metrics.Query{Model: "llama3"})
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("Query() returned %d rows, want 1", len(rows))
	}

	// cost = (100 + 50) * 0.0001 = 0.015
	// Use epsilon comparison to handle floating-point round-trip through SQLite.
	const eps = 1e-9
	wantCost := 150 * 0.0001
	diff := rows[0].Cost - wantCost
	if diff > eps || diff < -eps {
		t.Errorf("row.Cost = %.15f, want %.15f (diff %.2e)", rows[0].Cost, wantCost, diff)
	}
}

func TestNewSQLiteStore_InvalidPath(t *testing.T) {
	t.Parallel()

	// A path where a directory exists at the file location should fail.
	dir := t.TempDir()
	_, err := metrics.NewSQLiteStore(dir) // passing a directory, not a file
	if err == nil {
		t.Fatal("NewSQLiteStore(directory) expected error, got nil")
	}
}
