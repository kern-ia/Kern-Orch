package graph

import (
	"context"
	"sort"
	"testing"
)

// appendNode records its id into a "trace" slice and sets a per-node key.
func appendNode(id string) Node {
	return NewToolNode(id, func(_ context.Context, s *State) error {
		tr, _ := s.Get("trace")
		list, _ := tr.([]string)
		s.Set("trace", append(list, id))
		s.Set("visited_"+id, true)
		return nil
	})
}

func TestEngineLinearPath(t *testing.T) {
	g := NewGraph()
	g.AddNode(appendNode("a")).AddNode(appendNode("b")).AddNode(appendNode("c"))
	g.SetEntry("a")
	g.AddEdge("a", Static("b"))
	g.AddEdge("b", Static("c"))
	// c has no edge => terminal
	if err := g.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	s := NewState()
	if err := NewEngine(g).Run(context.Background(), s); err != nil {
		t.Fatalf("Run: %v", err)
	}
	tr, _ := s.Get("trace")
	got, _ := tr.([]string)
	want := []string{"a", "b", "c"}
	if len(got) != 3 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("trace = %v; want %v", got, want)
	}
	if s.Step != 3 {
		t.Fatalf("Step = %d; want 3", s.Step)
	}
}

func TestEngineConditionalRouting(t *testing.T) {
	g := NewGraph()
	g.AddNode(appendNode("start")).AddNode(appendNode("left")).AddNode(appendNode("right"))
	g.SetEntry("start")
	g.AddEdge("start", Conditional(func(s *State) []string {
		if v, _ := s.Get("go"); v == "L" {
			return []string{"left"}
		}
		return []string{"right"}
	}))
	s := NewState()
	s.Set("go", "L")
	if err := NewEngine(g).Run(context.Background(), s); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !s.Has("visited_left") || s.Has("visited_right") {
		t.Fatalf("wrong branch taken: %v", s.Keys())
	}
}

func TestEngineFanOutRunsBranchesAndMerges(t *testing.T) {
	g := NewGraph()
	g.AddNode(appendNode("root")).AddNode(appendNode("x")).AddNode(appendNode("y"))
	g.SetEntry("root")
	g.AddEdge("root", Static("x", "y")) // fan-out
	s := NewState()
	if err := NewEngine(g).Run(context.Background(), s); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !s.Has("visited_x") || !s.Has("visited_y") {
		t.Fatalf("both branches should have run: %v", s.Keys())
	}
}

func TestValidateRejectsUnknownEntryAndEdges(t *testing.T) {
	g := NewGraph()
	g.AddNode(appendNode("a"))
	g.SetEntry("missing")
	if err := g.Validate(); err == nil {
		t.Fatal("expected error for unknown entry")
	}
	g.SetEntry("a")
	g.AddEdge("a", Static("ghost"))
	if err := g.Validate(); err == nil {
		t.Fatal("expected error for edge to unknown node")
	}
}

func TestEngineCycleGuard(t *testing.T) {
	g := NewGraph()
	g.AddNode(appendNode("loop"))
	g.SetEntry("loop")
	g.AddEdge("loop", Static("loop"))
	err := NewEngine(g).WithMaxSteps(5).Run(context.Background(), NewState())
	if err == nil {
		t.Fatal("expected cycle-guard error")
	}
}

func TestStaticRouteIsStable(t *testing.T) {
	r := Static("b", "a", "c")
	got := r(NewState())
	cp := append([]string(nil), got...)
	sort.Strings(cp)
	if got[0] != "b" || got[1] != "a" || got[2] != "c" {
		t.Fatalf("Static should preserve given order, got %v", got)
	}
}
