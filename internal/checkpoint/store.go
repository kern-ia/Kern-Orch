// Package checkpoint persists run state per step to SQLite so a run can be resumed
// after failure and inspected. It depends on graph only for the State type; the graph
// engine reaches this package through its StepFunc hook, not the other way around.
package checkpoint

import (
	"context"
	"errors"
	"time"

	"github.com/yoann/kern-orch/internal/graph"
)

// Run status values persisted with each checkpoint.
const (
	StatusRunning = "running"
	StatusDone    = "done"
	StatusFailed  = "failed"
)

// ErrEmptyRunID is returned when a Record has no run id.
var ErrEmptyRunID = errors.New("checkpoint: empty run id")

// Record is one persisted checkpoint: the state after a level, plus the frontier still
// to execute (empty when finished), keyed by (RunID, Step).
type Record struct {
	RunID     string
	Step      int
	Frontier  []string
	State     *graph.State
	Status    string
	CreatedAt time.Time
}

// Summary is a per-run rollup for the `status` command.
type Summary struct {
	RunID     string
	LastStep  int
	Status    string
	UpdatedAt time.Time
}

// Store persists and retrieves checkpoints. Implementations must be safe for use from
// a single run's goroutine; the engine calls Save sequentially between levels.
type Store interface {
	Save(ctx context.Context, r Record) error
	Latest(ctx context.Context, runID string) (Record, bool, error)
	List(ctx context.Context) ([]Summary, error)
	Close() error
}
