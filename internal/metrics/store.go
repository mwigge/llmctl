package metrics

import "time"

// Entry is a single inference event to be persisted.
type Entry struct {
	Model        string
	InputTokens  int
	OutputTokens int
	LatencyMs    int64
	CostPerToken float64
	RecordedAt   time.Time
}

// Observation is an operational or AI-observability event.
type Observation struct {
	Kind       string
	Model      string
	Source     string
	Value      float64
	Unit       string
	Attributes map[string]string
	RecordedAt time.Time
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

// ObservationQuery filters observability rows.
type ObservationQuery struct {
	Kind  string
	Model string
	Since time.Time
	Until time.Time
}

// ObservationRow is a persisted observability event.
type ObservationRow struct {
	ID         int64
	Kind       string
	Model      string
	Source     string
	Value      float64
	Unit       string
	Attributes map[string]string
	RecordedAt time.Time
}

// Store persists and retrieves inference metrics.
type Store interface {
	// Record persists a single inference entry.
	Record(e Entry) error
	// Query retrieves rows matching the filter.
	Query(q Query) ([]Row, error)
	// RecordObservation persists a model operations/drift observation.
	RecordObservation(o Observation) error
	// QueryObservations retrieves observability rows matching the filter.
	QueryObservations(q ObservationQuery) ([]ObservationRow, error)
	// Close releases any underlying resources.
	Close() error
}
