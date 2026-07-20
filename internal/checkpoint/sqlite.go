package checkpoint

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/yoann/kern-orch/internal/graph"
	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS checkpoints (
	run_id     TEXT    NOT NULL,
	step       INTEGER NOT NULL,
	frontier   TEXT    NOT NULL,
	state      TEXT    NOT NULL,
	status     TEXT    NOT NULL,
	created_at TEXT    NOT NULL,
	PRIMARY KEY (run_id, step)
);`

// SQLiteStore is a Store backed by modernc.org/sqlite (pure Go, no cgo).
type SQLiteStore struct {
	db *sql.DB
}

// OpenSQLite opens (creating if needed) the checkpoint database at path and ensures
// the schema exists.
func OpenSQLite(path string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("checkpoint: open %q: %w", path, err)
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("checkpoint: schema: %w", err)
	}
	return &SQLiteStore{db: db}, nil
}

// Save upserts the checkpoint for (RunID, Step).
func (s *SQLiteStore) Save(ctx context.Context, r Record) error {
	if r.RunID == "" {
		return ErrEmptyRunID
	}
	frontier, err := json.Marshal(r.Frontier)
	if err != nil {
		return fmt.Errorf("checkpoint: marshal frontier: %w", err)
	}
	state, err := json.Marshal(r.State)
	if err != nil {
		return fmt.Errorf("checkpoint: marshal state: %w", err)
	}
	created := r.CreatedAt
	if created.IsZero() {
		created = time.Now().UTC()
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO checkpoints (run_id, step, frontier, state, status, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(run_id, step) DO UPDATE SET
		   frontier=excluded.frontier, state=excluded.state,
		   status=excluded.status, created_at=excluded.created_at`,
		r.RunID, r.Step, string(frontier), string(state), r.Status, created.Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("checkpoint: save: %w", err)
	}
	return nil
}

// Latest returns the highest-step checkpoint for runID; ok is false if none exists.
func (s *SQLiteStore) Latest(ctx context.Context, runID string) (Record, bool, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT step, frontier, state, status, created_at
		 FROM checkpoints WHERE run_id = ? ORDER BY step DESC LIMIT 1`, runID)
	var (
		step               int
		frontier, state    string
		status, createdStr string
	)
	switch err := row.Scan(&step, &frontier, &state, &status, &createdStr); err {
	case sql.ErrNoRows:
		return Record{}, false, nil
	case nil:
		// fallthrough below
	default:
		return Record{}, false, fmt.Errorf("checkpoint: latest: %w", err)
	}

	rec := Record{RunID: runID, Step: step, Status: status, State: graph.NewState()}
	if err := json.Unmarshal([]byte(frontier), &rec.Frontier); err != nil {
		return Record{}, false, fmt.Errorf("checkpoint: decode frontier: %w", err)
	}
	if err := json.Unmarshal([]byte(state), rec.State); err != nil {
		return Record{}, false, fmt.Errorf("checkpoint: decode state: %w", err)
	}
	rec.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdStr)
	return rec, true, nil
}

// List returns one Summary per run, taking each run's highest-step row.
func (s *SQLiteStore) List(ctx context.Context) ([]Summary, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT c.run_id, c.step, c.status, c.created_at
		 FROM checkpoints c
		 JOIN (SELECT run_id, MAX(step) AS step FROM checkpoints GROUP BY run_id) m
		   ON c.run_id = m.run_id AND c.step = m.step
		 ORDER BY c.created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("checkpoint: list: %w", err)
	}
	defer rows.Close()

	var out []Summary
	for rows.Next() {
		var (
			sum        Summary
			createdStr string
		)
		if err := rows.Scan(&sum.RunID, &sum.LastStep, &sum.Status, &createdStr); err != nil {
			return nil, fmt.Errorf("checkpoint: scan summary: %w", err)
		}
		sum.UpdatedAt, _ = time.Parse(time.RFC3339Nano, createdStr)
		out = append(out, sum)
	}
	return out, rows.Err()
}

// Close releases the database handle.
func (s *SQLiteStore) Close() error { return s.db.Close() }
