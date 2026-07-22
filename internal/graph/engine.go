package graph

import (
	"context"
	"fmt"
	"sort"
	"sync"
)

// Graph is the in-memory topology: nodes keyed by ID, an entry node, and one RouteFunc
// per node. A node with no registered route is terminal.
type Graph struct {
	nodes  map[string]Node
	routes map[string]RouteFunc
	entry  string
}

// NewGraph returns an empty graph.
func NewGraph() *Graph {
	return &Graph{nodes: make(map[string]Node), routes: make(map[string]RouteFunc)}
}

// AddNode registers a node; it returns the graph for chaining.
func (g *Graph) AddNode(n Node) *Graph {
	g.nodes[n.ID()] = n
	return g
}

// SetEntry sets the entry node ID.
func (g *Graph) SetEntry(id string) *Graph {
	g.entry = id
	return g
}

// AddEdge registers the route leaving the given node.
func (g *Graph) AddEdge(from string, route RouteFunc) *Graph {
	g.routes[from] = route
	return g
}

// Validate checks the entry exists. Static edges are checked against known nodes;
// conditional edges are dynamic and validated at run time when they yield a target.
func (g *Graph) Validate() error {
	if g.entry == "" {
		return fmt.Errorf("graph: no entry node set")
	}
	if _, ok := g.nodes[g.entry]; !ok {
		return fmt.Errorf("graph: entry node %q not registered", g.entry)
	}
	for from, route := range g.routes {
		if _, ok := g.nodes[from]; !ok {
			return fmt.Errorf("graph: edge from unknown node %q", from)
		}
		// Best-effort static check: Static routes are pure and safe to probe with nil state.
		for _, to := range route(&State{data: map[string]any{}}) {
			if _, ok := g.nodes[to]; !ok {
				return fmt.Errorf("graph: edge %q -> unknown node %q", from, to)
			}
		}
	}
	return nil
}

const defaultMaxSteps = 10_000

// Engine executes a Graph over a shared State using level-synchronous scheduling: each
// step runs the current frontier of nodes in parallel (each on a cloned state), merges
// their outputs back in a stable order, then computes the next frontier from their routes.
type Engine struct {
	g        *Graph
	maxSteps int
	onStep   StepFunc
}

// StepInfo describes the graph's position after a level completes: the next frontier
// to execute (empty when the run is finished) and the running step count.
type StepInfo struct {
	Step     int
	Frontier []string
}

// StepFunc is called after every executed level with the merged state and the next
// frontier. It is the seam the checkpoint store hooks into; the graph package stays
// unaware of persistence. Returning an error aborts the run.
type StepFunc func(ctx context.Context, info StepInfo, s *State) error

// NewEngine builds an engine for the graph.
func NewEngine(g *Graph) *Engine {
	return &Engine{g: g, maxSteps: defaultMaxSteps}
}

// WithMaxSteps overrides the cycle guard (number of levels executed).
func (e *Engine) WithMaxSteps(n int) *Engine {
	e.maxSteps = n
	return e
}

// OnStep registers a hook invoked after each level (see StepFunc).
func (e *Engine) OnStep(f StepFunc) *Engine {
	e.onStep = f
	return e
}

// Run executes the graph from its entry node, mutating s in place.
func (e *Engine) Run(ctx context.Context, s *State) error {
	return e.RunFrom(ctx, s, []string{e.g.entry})
}

// RunFrom executes the graph starting from an arbitrary frontier — used by resume to
// continue from a checkpoint. It stops when the frontier empties or ctx is cancelled,
// and errors if the step budget is exhausted (cycle guard).
func (e *Engine) RunFrom(ctx context.Context, s *State, frontier []string) error {
	if err := e.g.Validate(); err != nil {
		return err
	}
	for level := 0; len(frontier) > 0; level++ {
		if level >= e.maxSteps {
			return fmt.Errorf("graph: step budget %d exhausted (cycle?)", e.maxSteps)
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		next, err := e.runLevel(ctx, s, frontier)
		if err != nil {
			return err
		}
		if e.onStep != nil {
			if err := e.onStep(ctx, StepInfo{Step: s.Step, Frontier: next}, s); err != nil {
				return err
			}
		}
		frontier = next
	}
	return nil
}

// runLevel executes every node in the frontier in parallel and merges results.
func (e *Engine) runLevel(ctx context.Context, s *State, frontier []string) ([]string, error) {
	type outcome struct {
		id     string
		branch *State
		route  []string
		err    error
	}
	results := make([]outcome, len(frontier))
	var wg sync.WaitGroup
	for i, id := range frontier {
		node, ok := e.g.nodes[id]
		if !ok {
			return nil, fmt.Errorf("graph: node %q not registered", id)
		}
		wg.Add(1)
		go func(i int, id string, node Node) {
			defer wg.Done()
			branch := s.Clone()
			if err := node.Execute(ctx, branch); err != nil {
				results[i] = outcome{id: id, err: fmt.Errorf("node %q: %w", id, err)}
				return
			}
			var route []string
			if r, ok := e.g.routes[id]; ok {
				route = r(branch)
			}
			results[i] = outcome{id: id, branch: branch, route: route}
		}(i, id, node)
	}
	wg.Wait()

	// Combine branches deterministically (frontier order) and collect the next frontier.
	// A single-node frontier REPLACES the state with its branch, so context-replacing
	// operations (Freeze / key deletion) and the Frozen counter propagate. A fan-out
	// (>1 node) uses additive Merge, since each branch only contributes its own keys.
	single := len(results) == 1
	seen := make(map[string]bool)
	var next []string
	for _, o := range results {
		if o.err != nil {
			return nil, o.err
		}
		if single {
			s.replaceWith(o.branch)
		} else {
			s.Merge(o.branch)
		}
		s.Step++
		for _, to := range o.route {
			if !seen[to] {
				seen[to] = true
				next = append(next, to)
			}
		}
	}
	sort.Strings(next)
	return next, nil
}
