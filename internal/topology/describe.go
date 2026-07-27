package topology

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Declared is the graph as its YAML declares it, without building anything.
//
// It exists because the runtime graph.Graph cannot answer the question: its edges are
// RouteFunc closures, so a conditional route is impossible to enumerate. The YAML, on the
// other hand, states every edge — and says which ones are decided at run time.
type Declared struct {
	Entry string         `json:"entry"`
	Nodes []DeclaredNode `json:"nodes"`
	Edges []DeclaredEdge `json:"edges,omitempty"`
}

// DeclaredNode is one unit of work: its id, what kind of work it is, and — for an agent —
// the skill backing it.
//
// Skill is not the id: a node `greet` may run the skill `planner`. It is what lets a
// consumer holding the skills catalogue tell which of its entries a run is exercising,
// without matching names that were never meant to match.
type DeclaredNode struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"` // tool | agent | subgraph
	Skill string `json:"skill,omitempty"`
}

// DeclaredEdge leaves a node. Static edges carry their targets; a router-driven edge
// carries none and is flagged Dynamic, so a consumer drawing the graph knows the picture is
// incomplete rather than reading the node as terminal.
type DeclaredEdge struct {
	From    string   `json:"from"`
	To      []string `json:"to,omitempty"`
	Dynamic bool     `json:"dynamic,omitempty"`
}

// Describe reads a topology from YAML. It validates nothing beyond the shape: a graph that
// describes cleanly may still fail to build, and that is Load's business.
func Describe(data []byte) (Declared, error) {
	var sp spec
	if err := yaml.Unmarshal(data, &sp); err != nil {
		return Declared{}, fmt.Errorf("topology: parse yaml: %w", err)
	}
	if sp.Entry == "" {
		return Declared{}, fmt.Errorf("topology: no entry declared")
	}

	d := Declared{Entry: sp.Entry}
	for _, n := range sp.Nodes {
		d.Nodes = append(d.Nodes, DeclaredNode{ID: n.ID, Kind: n.Type, Skill: n.Skill})
	}
	for _, e := range sp.Edges {
		d.Edges = append(d.Edges, DeclaredEdge{
			From:    e.From,
			To:      e.To,
			Dynamic: e.Router != "",
		})
	}
	return d, nil
}

// DescribeFile reads a topology from a YAML file.
func DescribeFile(path string) (Declared, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Declared{}, fmt.Errorf("topology: read %s: %w", path, err)
	}
	return Describe(data)
}
