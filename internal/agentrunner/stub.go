package agentrunner

import (
	"context"

	"github.com/yoann/kern-orch/internal/graph"
)

// Stub is a deterministic AgentRunner used when no external CLI is configured (and in
// tests). It returns a per-node canned Output, else Default, else echoes the prompt.
type Stub struct {
	Responses map[string]map[string]any
	Default   map[string]any
}

// Run implements graph.AgentRunner.
func (s *Stub) Run(_ context.Context, req graph.AgentRequest) (graph.AgentResult, error) {
	if out, ok := s.Responses[req.NodeID]; ok {
		return graph.AgentResult{Output: out}, nil
	}
	if s.Default != nil {
		return graph.AgentResult{Output: s.Default}, nil
	}
	return graph.AgentResult{Output: map[string]any{"echo": req.Prompt}}, nil
}
