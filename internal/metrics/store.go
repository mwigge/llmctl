package metrics

import "time"

// Entry is a single inference event to be persisted.
type Entry struct {
	Model       string
	InputTokens int
	OutputTokens int
	LatencyMs   int64
	CostPerToken float64
	RecordedAt  time.Time
}

// Query filters rows returned by Store.Query.
type Query struct {
	// Model filters by exact model name. Empty means all models.
	Model string
	// Since filters to rows recorded at or after this time. Zero means no lower bound.
	Since time.Time
	// Until filters to rows recorded before this time. Zero means no upper bound.
	Until time.Time
}

// Row is a single result row returned by Store.Query.
type Row struct {
	ID           int64
	Model        string
	InputTokens  int
	OutputTokens int
	LatencyMs    int64
	Cost         float64
	RecordedAt   time.Time
}

// Store persists and retrieves inference metrics.
type Store interface {
	// Record persists a single inference entry.
	Record(e Entry) error
	// Query retrieves rows matching the filter.
	Query(q Query) ([]Row, error)
	// Close releases any underlying resources.
	Close() error
}
