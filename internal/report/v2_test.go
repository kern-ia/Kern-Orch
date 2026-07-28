package report

import (
	"context"
	"net/http"
	"testing"

	"github.com/yoann/kern-orch/internal/graph"
)

func topo() *Topology {
	return &Topology{
		Entry: "think",
		Nodes: []TopologyNode{{ID: "think", Kind: "agent"}, {ID: "done", Kind: "tool"}},
		Edges: []TopologyEdge{{From: "think", To: []string{"done"}}},
	}
}

// The topology never changes during a run, so sending it on every level would be waste.
// Sending it once means a consumer that connects late gets it from the sink, not from us.
func TestTopologyIsSentOnceAtTheStartOfTheRun(t *testing.T) {
	s := newSink(t, http.StatusAccepted)
	r := NewHTTP(s.server.URL)
	r.now = fixedNow

	hook := r.Hook("run-1", "review", topo())
	for step := 1; step <= 3; step++ {
		if err := hook(context.Background(), graph.StepInfo{Step: step, Frontier: []string{"done"}}, graph.NewState()); err != nil {
			t.Fatalf("hook: %v", err)
		}
	}

	r.Flush()

	bodies := s.all()
	if len(bodies) != 3 {
		t.Fatalf("the sink received %d events, want 3", len(bodies))
	}
	if _, ok := bodies[0]["topology"]; !ok {
		t.Error("the first event carries no topology")
	}
	for i, body := range bodies[1:] {
		if _, ok := body["topology"]; ok {
			t.Errorf("event %d repeats the topology", i+2)
		}
	}
}

func TestTopologyCarriesNodesAndEdges(t *testing.T) {
	s := newSink(t, http.StatusAccepted)
	r := NewHTTP(s.server.URL)
	r.now = fixedNow

	_ = r.Hook("run-1", "review", topo())(context.Background(),
		graph.StepInfo{Step: 1, Frontier: []string{"done"}}, graph.NewState())

	r.Flush()

	sent, ok := s.last()["topology"].(map[string]any)
	if !ok {
		t.Fatal("no topology in the payload")
	}
	if sent["entry"] != "think" {
		t.Errorf("entry = %v, want think", sent["entry"])
	}
	if nodes, _ := sent["nodes"].([]any); len(nodes) != 2 {
		t.Errorf("got %d nodes, want 2", len(nodes))
	}
	if edges, _ := sent["edges"].([]any); len(edges) != 1 {
		t.Errorf("got %d edges, want 1", len(edges))
	}
}

func TestRunsWithoutATopologySendNone(t *testing.T) {
	s := newSink(t, http.StatusAccepted)
	r := NewHTTP(s.server.URL)
	r.now = fixedNow

	_ = r.Hook("run-1", "review", nil)(context.Background(),
		graph.StepInfo{Step: 1, Frontier: []string{"a"}}, graph.NewState())

	r.Flush()

	if _, ok := s.last()["topology"]; ok {
		t.Error("a topology appeared out of nowhere")
	}
}

// A failure is invisible to the step hook: the engine reports it by returning from Run,
// after the last successful level. Without this call a failed run would look finished.
func TestFailureIsReportedAsATerminalEvent(t *testing.T) {
	s := newSink(t, http.StatusAccepted)
	r := NewHTTP(s.server.URL)
	r.now = fixedNow

	r.ReportFailure(context.Background(), "run-1", "review", 4, []string{"think"}, []string{"think"}, "agent think: exit status 1")

	r.Flush()

	body := s.last()
	if body == nil {
		t.Fatal("nothing was reported")
	}
	failure, ok := body["error"].(map[string]any)
	if !ok {
		t.Fatalf("no error in the payload: %v", body)
	}
	if failure["message"] != "agent think: exit status 1" {
		t.Errorf("message = %v", failure["message"])
	}
	if body["step"] != float64(4) {
		t.Errorf("step = %v, want 4", body["step"])
	}
	// The frontier that was running is the whole point: it says *where* the run broke.
	frontier, _ := body["frontier"].([]any)
	if len(frontier) != 1 || frontier[0] != "think" {
		t.Errorf("frontier = %v, want [think] — the level that was running when it broke", frontier)
	}
}

func TestFailureNeverPanicsWithoutASink(t *testing.T) {
	NewHTTP("").ReportFailure(context.Background(), "run-1", "review", 1, nil, nil, "boom")
}
