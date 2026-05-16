package metrics

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3" // CGO SQLite driver
)

const schema = `
CREATE TABLE IF NOT EXISTS inference_metrics (
	id            INTEGER PRIMARY KEY AUTOINCREMENT,
	model         TEXT    NOT NULL,
	input_tokens  INTEGER NOT NULL DEFAULT 0,
	output_tokens INTEGER NOT NULL DEFAULT 0,
	latency_ms    INTEGER NOT NULL DEFAULT 0,
	cost          REAL    NOT NULL DEFAULT 0,
	recorded_at   DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_inference_model ON inference_metrics(model);
CREATE INDEX IF NOT EXISTS idx_inference_recorded_at ON inference_metrics(recorded_at);
CREATE TABLE IF NOT EXISTS observability_events (
	id            INTEGER PRIMARY KEY AUTOINCREMENT,
	kind          TEXT    NOT NULL,
	model         TEXT    NOT NULL DEFAULT '',
	source        TEXT    NOT NULL DEFAULT '',
	value         REAL    NOT NULL DEFAULT 0,
	unit          TEXT    NOT NULL DEFAULT '',
	attributes    TEXT    NOT NULL DEFAULT '{}',
	recorded_at   DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_observability_kind ON observability_events(kind);
CREATE INDEX IF NOT EXISTS idx_observability_model ON observability_events(model);
CREATE INDEX IF NOT EXISTS idx_observability_recorded_at ON observability_events(recorded_at);
`

type sqliteStore struct {
	db *sql.DB
}

// NewSQLiteStore opens (or creates) a SQLite database at path and returns a
// Store backed by it. The caller must call Close when done.
func NewSQLiteStore(path string) (Store, error) {
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite db %s: %w", path, err)
	}

	if _, err := db.Exec(schema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("create schema: %w", err)
	}

	return &sqliteStore{db: db}, nil
}

// Record inserts a single inference entry into the database.
func (s *sqliteStore) Record(e Entry) error {
	at := e.RecordedAt
	if at.IsZero() {
		at = time.Now().UTC()
	}

	cost := float64(e.InputTokens+e.OutputTokens) * e.CostPerToken

	_, err := s.db.Exec(
		`INSERT INTO inference_metrics
			(model, input_tokens, output_tokens, latency_ms, cost, recorded_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		e.Model, e.InputTokens, e.OutputTokens, e.LatencyMs, cost, at.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("insert metric: %w", err)
	}
	return nil
}

// Query retrieves rows from the database matching the supplied filter.
func (s *sqliteStore) Query(q Query) ([]Row, error) {
	query := `SELECT id, model, input_tokens, output_tokens, latency_ms, cost, recorded_at
	          FROM inference_metrics WHERE 1=1`
	args := make([]any, 0, 3)

	if q.Model != "" {
		query += " AND model = ?"
		args = append(args, q.Model)
	}
	if !q.Since.IsZero() {
		query += " AND recorded_at >= ?"
		args = append(args, q.Since.UTC().Format(time.RFC3339Nano))
	}
	if !q.Until.IsZero() {
		query += " AND recorded_at < ?"
		args = append(args, q.Until.UTC().Format(time.RFC3339Nano))
	}
	query += " ORDER BY recorded_at ASC"

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query metrics: %w", err)
	}
	defer rows.Close()

	var results []Row
	for rows.Next() {
		var r Row
		var recordedAt string
		if err := rows.Scan(
			&r.ID, &r.Model, &r.InputTokens, &r.OutputTokens,
			&r.LatencyMs, &r.Cost, &recordedAt,
		); err != nil {
			return nil, fmt.Errorf("scan metric row: %w", err)
		}
		t, err := time.Parse(time.RFC3339Nano, recordedAt)
		if err != nil {
			return nil, fmt.Errorf("parse recorded_at %q: %w", recordedAt, err)
		}
		r.RecordedAt = t
		results = append(results, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate metric rows: %w", err)
	}

	return results, nil
}

// RecordObservation inserts a single model operations/drift observation.
func (s *sqliteStore) RecordObservation(o Observation) error {
	at := o.RecordedAt
	if at.IsZero() {
		at = time.Now().UTC()
	}
	attrs := o.Attributes
	if attrs == nil {
		attrs = map[string]string{}
	}
	attrJSON, err := json.Marshal(attrs)
	if err != nil {
		return fmt.Errorf("marshal observation attributes: %w", err)
	}
	_, err = s.db.Exec(
		`INSERT INTO observability_events
			(kind, model, source, value, unit, attributes, recorded_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		o.Kind, o.Model, o.Source, o.Value, o.Unit, string(attrJSON), at.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("insert observation: %w", err)
	}
	return nil
}

// QueryObservations retrieves observability rows matching the supplied filter.
func (s *sqliteStore) QueryObservations(q ObservationQuery) ([]ObservationRow, error) {
	query := `SELECT id, kind, model, source, value, unit, attributes, recorded_at
	          FROM observability_events WHERE 1=1`
	args := make([]any, 0, 4)
	if q.Kind != "" {
		query += " AND kind = ?"
		args = append(args, q.Kind)
	}
	if q.Model != "" {
		query += " AND model = ?"
		args = append(args, q.Model)
	}
	if !q.Since.IsZero() {
		query += " AND recorded_at >= ?"
		args = append(args, q.Since.UTC().Format(time.RFC3339Nano))
	}
	if !q.Until.IsZero() {
		query += " AND recorded_at < ?"
		args = append(args, q.Until.UTC().Format(time.RFC3339Nano))
	}
	query += " ORDER BY recorded_at ASC"

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query observations: %w", err)
	}
	defer rows.Close()

	var results []ObservationRow
	for rows.Next() {
		var r ObservationRow
		var attrJSON, recordedAt string
		if err := rows.Scan(&r.ID, &r.Kind, &r.Model, &r.Source, &r.Value, &r.Unit, &attrJSON, &recordedAt); err != nil {
			return nil, fmt.Errorf("scan observation row: %w", err)
		}
		if err := json.Unmarshal([]byte(attrJSON), &r.Attributes); err != nil {
			return nil, fmt.Errorf("parse observation attributes: %w", err)
		}
		t, err := time.Parse(time.RFC3339Nano, recordedAt)
		if err != nil {
			return nil, fmt.Errorf("parse recorded_at %q: %w", recordedAt, err)
		}
		r.RecordedAt = t
		results = append(results, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate observation rows: %w", err)
	}
	return results, nil
}

// Close releases the underlying database connection.
func (s *sqliteStore) Close() error {
	return s.db.Close()
}
