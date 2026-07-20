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
	id     string
	sub    *Graph
	input  func(parent *State) *State
	output func(parent, child *State)
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
	if err := NewEngine(n.sub).Run(ctx, child); err != nil {
		return fmt.Errorf("subgraph %q: %w", n.id, err)
	}
	n.output(s, child)
	return nil
}
