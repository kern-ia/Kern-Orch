package topology

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yoann/kern-orch/internal/agentrunner"
	"github.com/yoann/kern-orch/internal/graph"
)

func write(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadFileResolvesSubgraph(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "child.yaml", `
entry: c1
nodes:
  - id: c1
    type: tool
    func: double
`)
	parent := write(t, dir, "parent.yaml", `
entry: prep
nodes:
  - id: prep
    type: tool
    func: seed3
  - id: sub
    type: subgraph
    graph: child.yaml
edges:
  - from: prep
    to: [sub]
`)
	reg := NewRegistry(&agentrunner.Stub{})
	reg.Tool("seed3", func(_ context.Context, s *graph.State) error { s.Set("n", 3); return nil })
	reg.Tool("double", func(_ context.Context, s *graph.State) error {
		v, _ := s.Get("n")
		iv, _ := v.(int)
		s.Set("n", iv*2)
		return nil
	})

	g, err := LoadFile(parent, reg)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	s := graph.NewState()
	if err := graph.NewEngine(g).Run(context.Background(), s); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if v, _ := s.Get("n"); v != 6 {
		t.Fatalf("n = %v; want 6 (3 seeded, doubled in subgraph)", v)
	}
}

func TestLoadFileDetectsRecursiveSubgraph(t *testing.T) {
	dir := t.TempDir()
	// a.yaml references b.yaml which references a.yaml
	write(t, dir, "a.yaml", "entry: s\nnodes:\n  - id: s\n    type: subgraph\n    graph: b.yaml\n")
	write(t, dir, "b.yaml", "entry: s\nnodes:\n  - id: s\n    type: subgraph\n    graph: a.yaml\n")
	_, err := LoadFile(filepath.Join(dir, "a.yaml"), NewRegistry(nil))
	if err == nil || !strings.Contains(err.Error(), "recursive") {
		t.Fatalf("err = %v; want recursive-subgraph error", err)
	}
}

func TestLoadFileMissingSubgraphFile(t *testing.T) {
	dir := t.TempDir()
	p := write(t, dir, "p.yaml", "entry: s\nnodes:\n  - id: s\n    type: subgraph\n    graph: nope.yaml\n")
	if _, err := LoadFile(p, NewRegistry(nil)); err == nil {
		t.Fatal("expected error for missing subgraph file")
	}
}
