package graph

import (
	"context"
	"errors"
	"testing"
)

// buildChild returns a 2-node child graph that reads "seed" and writes "child_out".
func buildChild() *Graph {
	g := NewGraph()
	g.AddNode(NewToolNode("c1", func(_ context.Context, s *State) error {
		v, _ := s.Get("seed")
		n, _ := v.(int)
		s.Set("child_out", n*10)
		return nil
	}))
	g.SetEntry("c1")
	return g
}

func TestSubgraphNodeRunsNestedGraphAndBubblesResult(t *testing.T) {
	n := NewSubgraphNode("nested", buildChild())
	if n.Kind() != KindSubgraph {
		t.Fatalf("Kind() = %v; want KindSubgraph", n.Kind())
	}
	parent := NewState()
	parent.Set("seed", 4)
	if err := n.Execute(context.Background(), parent); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	// default input clones parent (child sees seed), default output merges child back.
	if v, _ := parent.Get("child_out"); v != 40 {
		t.Fatalf("child_out = %v; want 40 (result did not bubble up)", v)
	}
}

func TestSubgraphNodeIsolatesChildStateByDefault(t *testing.T) {
	child := NewGraph()
	child.AddNode(NewToolNode("only", func(_ context.Context, s *State) error {
		s.Set("seed", 999) // mutate a key that also exists in parent
		return nil
	}))
	child.SetEntry("only")

	// With a custom output that drops child keys, the parent must stay untouched.
	n := NewSubgraphNode("iso", child,
		WithOutput(func(parent, _ *State) { /* discard child result */ }))
	parent := NewState()
	parent.Set("seed", 1)
	if err := n.Execute(context.Background(), parent); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if v, _ := parent.Get("seed"); v != 1 {
		t.Fatalf("parent seed mutated to %v; child state leaked", v)
	}
}

func TestSubgraphCustomInputMapper(t *testing.T) {
	n := NewSubgraphNode("mapped", buildChild(),
		WithInput(func(parent *State) *State {
			c := NewState()
			pv, _ := parent.Get("outer")
			c.Set("seed", pv) // rename outer -> seed for the child
			return c
		}))
	parent := NewState()
	parent.Set("outer", 5)
	if err := n.Execute(context.Background(), parent); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if v, _ := parent.Get("child_out"); v != 50 {
		t.Fatalf("child_out = %v; want 50", v)
	}
}

func TestSubgraphPropagatesChildError(t *testing.T) {
	boom := errors.New("child failed")
	child := NewGraph()
	child.AddNode(NewToolNode("x", func(context.Context, *State) error { return boom }))
	child.SetEntry("x")
	n := NewSubgraphNode("bad", child)
	if err := n.Execute(context.Background(), NewState()); !errors.Is(err, boom) {
		t.Fatalf("Execute error = %v; want boom", err)
	}
}

func TestSubgraphInsideEngine(t *testing.T) {
	parentG := NewGraph()
	parentG.AddNode(NewToolNode("prep", func(_ context.Context, s *State) error {
		s.Set("seed", 3)
		return nil
	}))
	parentG.AddNode(NewSubgraphNode("sub", buildChild()))
	parentG.SetEntry("prep")
	parentG.AddEdge("prep", Static("sub"))

	s := NewState()
	if err := NewEngine(parentG).Run(context.Background(), s); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if v, _ := s.Get("child_out"); v != 30 {
		t.Fatalf("child_out = %v; want 30", v)
	}
}
