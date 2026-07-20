package agentrunner

import (
	"context"
	"testing"

	"github.com/yoann/kern-orch/internal/graph"
)

// compile-time proof the stub satisfies the port.
var _ graph.AgentRunner = (*Stub)(nil)

func TestStubReturnsPerNodeOutput(t *testing.T) {
	s := &Stub{Responses: map[string]map[string]any{
		"planner": {"plan": "do-x"},
	}}
	res, err := s.Run(context.Background(), graph.AgentRequest{NodeID: "planner", Prompt: "p"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Output["plan"] != "do-x" {
		t.Fatalf("output = %v; want plan=do-x", res.Output)
	}
}

func TestStubFallsBackToEchoingPrompt(t *testing.T) {
	s := &Stub{}
	res, err := s.Run(context.Background(), graph.AgentRequest{NodeID: "x", Prompt: "hello"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Output["echo"] != "hello" {
		t.Fatalf("expected echo fallback, got %v", res.Output)
	}
}
