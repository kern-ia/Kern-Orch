package graph

import (
	"context"
	"fmt"
)

// SubgraphNode runs a nested graph as a single node — the sub-agent of spec §3. The
// child runs with its own State (seeded from the parent) and its result is merged back
// into the parent. From the parent's checkpoint view the whole sub-run is one atomic
// step (spec §6.3: checkpoint at sub-graph boundaries).
type SubgraphNode struct {
	id       string
	sub      *Graph
	graphRef string
	input    func(parent *State) *State
	output   func(parent, child *State)

	// childStep builds the hook the nested engine runs, or is nil when nobody is watching.
	// It is a factory rather than a hook because each execution is a distinct nested run and
	// the builder needs to know which node it belongs to.
	childStep func(nodeID, graphRef string) StepFunc
}

// WithGraphRef records the file the nested graph came from, so a caller can describe its
// shape later. Purely informational: the engine never reads it.
func WithGraphRef(ref string) SubgraphOption {
	return func(n *SubgraphNode) { n.graphRef = ref }
}

// SubgraphOption customizes how state flows in and out of the nested graph.
type SubgraphOption func(*SubgraphNode)

// WithInput overrides how the child's initial state is derived from the parent.
// Default: a Clone of the parent (the child sees the parent's context).
func WithInput(fn func(parent *State) *State) SubgraphOption {
	return func(n *SubgraphNode) { n.input = fn }
}

// WithOutput overrides how the finished child state is folded back into the parent.
// Default: Merge every child key into the parent.
func WithOutput(fn func(parent, child *State)) SubgraphOption {
	return func(n *SubgraphNode) { n.output = fn }
}

// WithChildStep makes the nested run report its own levels.
//
// Without it the child is invisible: the parent sees one atomic step, so a consumer can
// only ever draw the sub-agent as a single dot with no idea what happens inside. Opt-in,
// so a graph built in Go without observability keeps running exactly as before.
func WithChildStep(build func(nodeID, graphRef string) StepFunc) SubgraphOption {
	return func(n *SubgraphNode) { n.childStep = build }
}

// GraphRef returns the file the nested graph was loaded from, or "" when it was built in
// Go. A caller describing the child's shape needs it; the engine never does.
func (n *SubgraphNode) GraphRef() string { return n.graphRef }

// NewSubgraphNode builds a subgraph node wrapping sub.
func NewSubgraphNode(id string, sub *Graph, opts ...SubgraphOption) *SubgraphNode {
	n := &SubgraphNode{
		id:     id,
		sub:    sub,
		input:  func(parent *State) *State { return parent.Clone() },
		output: func(parent, child *State) { parent.Merge(child) },
	}
	for _, o := range opts {
		o(n)
	}
	return n
}

func (n *SubgraphNode) ID() string { return n.id }
func (n *SubgraphNode) Kind() Kind { return KindSubgraph }

// Execute seeds the child state, runs the nested graph to completion, then bubbles the
// result back into the parent.
func (n *SubgraphNode) Execute(ctx context.Context, s *State) error {
	child := n.input(s)

	engine := NewEngine(n.sub)
	if n.childStep != nil {
		if hook := n.childStep(n.id, n.graphRef); hook != nil {
			engine.OnStep(hook)
		}
	}

	if err := engine.Run(ctx, child); err != nil {
		return fmt.Errorf("subgraph %q: %w", n.id, err)
	}
	n.output(s, child)
	return nil
}
