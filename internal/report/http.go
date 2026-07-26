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

// Topology is the shape of the graph, sent once at the start of a run.
//
// It is declared data, read from the YAML, not read back from the running graph: the
// engine's edges are closures, so a conditional route cannot be enumerated. Such an edge
// arrives with no targets and Dynamic set, which tells a consumer its picture is
// incomplete rather than letting it read the node as terminal.
type Topology struct {
	Entry string         `json:"entry"`
	Nodes []TopologyNode `json:"nodes"`
	Edges []TopologyEdge `json:"edges,omitempty"`
}

// TopologyNode is one unit of work: its id and what kind of work it is.
type TopologyNode struct {
	ID   string `json:"id"`
	Kind string `json:"kind"` // tool | agent | subgraph
}

// TopologyEdge leaves a node towards its declared targets, or is flagged Dynamic when a
// router picks them at run time.
type TopologyEdge struct {
	From    string   `json:"from"`
	To      []string `json:"to,omitempty"`
	Dynamic bool     `json:"dynamic,omitempty"`
}

// Failure ends a run that did not complete, naming what went wrong.
type Failure struct {
	Message string `json:"message"`
}

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

	// Topology rides on the first event of a run only: it never changes, and repeating it
	// on every level would be waste.
	Topology *Topology `json:"topology,omitempty"`

	// Error is set on the terminal event of a run that failed. Without it a failed run is
	// indistinguishable from a finished one.
	Error *Failure `json:"error,omitempty"`
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

// runReporter carries the per-run state the hook needs: the topology still to be announced.
type runReporter struct {
	pending *Topology
	sent    bool
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
func (r *HTTPReporter) Hook(runID, graphName string, topo *Topology) graph.StepFunc {
	if !r.Enabled() {
		return nil
	}
	run := &runReporter{pending: topo}

	return func(ctx context.Context, info graph.StepInfo, s *graph.State) error {
		if err := ctx.Err(); err != nil {
			// The run is already going down; do not add a doomed request to the noise.
			return nil
		}

		ev := StepEvent{
			RunID:    runID,
			Graph:    graphName,
			Step:     info.Step,
			Frontier: nonNil(info.Frontier),
			State:    flatten(s),
			At:       r.now(),
		}
		if !run.sent {
			ev.Topology = run.pending
			run.sent = true
		}

		if err := r.send(ctx, ev); err != nil {
			r.errf("kern-orch: report step %d of run %s: %v\n", info.Step, runID, err)
		}
		// Always nil: see the package comment.
		return nil
	}
}

// ReportFailure announces a run that ended badly.
//
// The step hook cannot do this: it only ever sees levels that succeeded, and the engine
// reports the failure by returning from Run. Without this call a failed run would look
// exactly like a finished one to the sink.
// frontier is the level that was about to run when things broke: without it the sink knows
// the run failed but not where, and can only shrug at the whole graph.
func (r *HTTPReporter) ReportFailure(ctx context.Context, runID, graphName string, step int, frontier []string, message string) {
	if !r.Enabled() {
		return
	}

	// The run context is usually already cancelled by the time we get here, so the report
	// gets a context of its own — otherwise no failure would ever be reported.
	if err := r.send(context.WithoutCancel(ctx), StepEvent{
		RunID:    runID,
		Graph:    graphName,
		Step:     step,
		Frontier: nonNil(frontier),
		At:       r.now(),
		Error:    &Failure{Message: message},
	}); err != nil {
		r.errf("kern-orch: report failure of run %s: %v\n", runID, err)
	}
}

func nonNil(frontier []string) []string {
	if frontier == nil {
		// An absent frontier and an empty one mean the same thing to the sink — the run is
		// over — but only a list survives the round trip unambiguously.
		return []string{}
	}
	return frontier
}

func (r *HTTPReporter) send(ctx context.Context, ev StepEvent) error {
	payload, err := json.Marshal(ev)
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
