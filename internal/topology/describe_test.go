package topology

import "testing"

const sample = `
entry: think
nodes:
  - id: think
    type: agent
    prompt: "work"
  - id: gel
    type: tool
    func: freeze
  - id: nested
    type: subgraph
    graph: child.yaml
edges:
  - from: think
    to: [gel, nested]
  - from: gel
    router: pick
`

func TestDescribeReportsNodesAndKinds(t *testing.T) {
	d, err := Describe([]byte(sample))
	if err != nil {
		t.Fatalf("Describe: %v", err)
	}

	if d.Entry != "think" {
		t.Errorf("Entry = %q, want think", d.Entry)
	}
	want := map[string]string{"think": "agent", "gel": "tool", "nested": "subgraph"}
	if len(d.Nodes) != len(want) {
		t.Fatalf("got %d nodes, want %d", len(d.Nodes), len(want))
	}
	for _, n := range d.Nodes {
		if want[n.ID] != n.Kind {
			t.Errorf("node %q kind = %q, want %q", n.ID, n.Kind, want[n.ID])
		}
	}
}

func TestDescribeKeepsStaticTargets(t *testing.T) {
	d, _ := Describe([]byte(sample))

	edge := edgeFrom(t, d, "think")
	if len(edge.To) != 2 || edge.To[0] != "gel" || edge.To[1] != "nested" {
		t.Errorf("To = %v, want [gel nested]", edge.To)
	}
	if edge.Dynamic {
		t.Error("Dynamic = true for a static edge")
	}
}

// A router decides at run time, so its targets cannot be declared. Saying so is the whole
// point: a consumer that draws the graph must know the picture is incomplete rather than
// assume the node is terminal.
func TestDescribeMarksRouterEdgesDynamic(t *testing.T) {
	d, _ := Describe([]byte(sample))

	edge := edgeFrom(t, d, "gel")
	if !edge.Dynamic {
		t.Error("Dynamic = false for a router-driven edge")
	}
	if len(edge.To) != 0 {
		t.Errorf("To = %v, want empty — the targets are unknown before the run", edge.To)
	}
}

func TestDescribeRejectsGarbage(t *testing.T) {
	if _, err := Describe([]byte("entry: [not, a, string]")); err == nil {
		t.Error("Describe accepted malformed yaml")
	}
}

func TestDescribeRequiresAnEntry(t *testing.T) {
	if _, err := Describe([]byte("nodes: []")); err == nil {
		t.Error("Describe accepted a graph with no entry")
	}
}

func edgeFrom(t *testing.T, d Declared, from string) DeclaredEdge {
	t.Helper()
	for _, e := range d.Edges {
		if e.From == from {
			return e
		}
	}
	t.Fatalf("no edge from %q", from)
	return DeclaredEdge{}
}

// The skill a node references is what links a run back to the Grimoire's catalogue. A node
// id is not a skill name — `id: greet` may run `skill: planner` — so without this the two
// can only be matched by guessing.
func TestDescribeCarriesTheSkillReference(t *testing.T) {
	d, err := Describe([]byte(`
entry: greet
nodes:
  - id: greet
    type: agent
    skill: planner
  - id: finish
    type: tool
    func: noop
`))
	if err != nil {
		t.Fatalf("Describe: %v", err)
	}

	byID := map[string]DeclaredNode{}
	for _, n := range d.Nodes {
		byID[n.ID] = n
	}

	if got := byID["greet"].Skill; got != "planner" {
		t.Errorf("greet skill = %q, want planner", got)
	}
	// A tool node names a Go function, not a skill: leaving it blank is the honest answer.
	if got := byID["finish"].Skill; got != "" {
		t.Errorf("finish skill = %q, want empty", got)
	}
}
