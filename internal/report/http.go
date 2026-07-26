// Package report pushes run transitions to an external HTTP sink.
//
// It is observability, never a dependency of the run: the hook it produces reports errors
// to stderr and always returns nil, so a sink that is slow, broken or absent can never
// abort a graph. The direction of dependency matches the rest of the infra — report
// depends on graph, never the reverse.
package report

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/yoann/kern-orch/internal/graph"
)

// DefaultTimeout caps how long a single report may hold up a run.
const DefaultTimeout = 2 * time.Second

// StepEvent is the payload accepted by the sink: one completed graph level.
//
// State is the flat business data, not graph.State's wire form. Marshalling the State
// itself would ship kern-orch's envelope (zones, frozen counter, internal step) across the
// contract, which is nobody else's business.
type StepEvent struct {
	RunID    string         `json:"run_id"`
	Graph    string         `json:"graph"`
	Step     int            `json:"step"`
	Frontier []string       `json:"frontier"`
	State    map[string]any `json:"state,omitempty"`
	At       time.Time      `json:"at"`
}

// flatten extracts the business data of a state, leaving its internals behind.
func flatten(s *graph.State) map[string]any {
	if s == nil {
		return nil
	}
	keys := s.Keys()
	if len(keys) == 0 {
		return nil
	}

	data := make(map[string]any, len(keys))
	for _, k := range keys {
		if v, ok := s.Get(k); ok {
			data[k] = v
		}
	}
	return data
}

// HTTPReporter posts step events to a single configured URL. The URL is the whole contract:
// the reporter knows nothing of the sink's route shape.
type HTTPReporter struct {
	URL     string
	Timeout time.Duration
	Client  *http.Client

	// Errf receives delivery failures. Defaults to stderr.
	Errf func(format string, args ...any)

	now func() time.Time
}

// NewHTTP returns a reporter posting to url. An empty url yields a disabled reporter whose
// Hook is nil.
func NewHTTP(url string) *HTTPReporter {
	return &HTTPReporter{
		URL:     url,
		Timeout: DefaultTimeout,
		now:     func() time.Time { return time.Now().UTC() },
	}
}

// Enabled reports whether a sink is configured.
func (r *HTTPReporter) Enabled() bool { return r.URL != "" }

// Hook returns the StepFunc to register on the engine, or nil when no sink is configured
// so the caller can leave it out of the chain entirely.
func (r *HTTPReporter) Hook(runID, graphName string) graph.StepFunc {
	if !r.Enabled() {
		return nil
	}

	return func(ctx context.Context, info graph.StepInfo, s *graph.State) error {
		if err := ctx.Err(); err != nil {
			// The run is already going down; do not add a doomed request to the noise.
			return nil
		}
		if err := r.post(ctx, runID, graphName, info, s); err != nil {
			r.errf("kern-orch: report step %d of run %s: %v\n", info.Step, runID, err)
		}
		// Always nil: see the package comment.
		return nil
	}
}

func (r *HTTPReporter) post(ctx context.Context, runID, graphName string, info graph.StepInfo, s *graph.State) error {
	frontier := info.Frontier
	if frontier == nil {
		// An absent frontier and an empty one mean the same thing to the sink — the run is
		// over — but only a list survives the round trip unambiguously.
		frontier = []string{}
	}

	payload, err := json.Marshal(StepEvent{
		RunID:    runID,
		Graph:    graphName,
		Step:     info.Step,
		Frontier: frontier,
		State:    flatten(s),
		At:       r.now(),
	})
	if err != nil {
		return fmt.Errorf("encode: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, r.timeout())
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.URL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := r.client().Do(req)
	if err != nil {
		return fmt.Errorf("post: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("sink answered %s", resp.Status)
	}
	return nil
}

func (r *HTTPReporter) client() *http.Client {
	if r.Client != nil {
		return r.Client
	}
	return http.DefaultClient
}

func (r *HTTPReporter) timeout() time.Duration {
	if r.Timeout > 0 {
		return r.Timeout
	}
	return DefaultTimeout
}

func (r *HTTPReporter) errf(format string, args ...any) {
	if r.Errf != nil {
		r.Errf(format, args...)
		return
	}
	fmt.Fprintf(os.Stderr, format, args...)
}
