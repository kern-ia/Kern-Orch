package graph

import "context"

// Kind distinguishes a direct-execution node from one that invokes an LLM agent.
type Kind int

const (
	// KindTool is a node that runs Go code directly — no LLM.
	KindTool Kind = iota
	// KindAgent is a node that delegates to an AgentRunner (the external LLM CLI).
	KindAgent
)

// Node is a single unit of work in the graph. Execute mutates the shared state in
// place and reports an error. A Node never decides the next node — routing is the
// engine's responsibility (via Edges), keeping execution and routing independent.
type Node interface {
	ID() string
	Kind() Kind
	Execute(ctx context.Context, s *State) error
}

// ToolFunc is the body of a ToolNode: pure/direct logic over the state.
type ToolFunc func(ctx context.Context, s *State) error

// ToolNode wraps a ToolFunc as a graph node.
type ToolNode struct {
	id string
	fn ToolFunc
}

// NewToolNode builds a tool node with the given id and body.
func NewToolNode(id string, fn ToolFunc) *ToolNode {
	return &ToolNode{id: id, fn: fn}
}

func (n *ToolNode) ID() string { return n.id }
func (n *ToolNode) Kind() Kind { return KindTool }
func (n *ToolNode) Execute(ctx context.Context, s *State) error {
	return n.fn(ctx, s)
}

// AgentRequest is what an AgentNode hands to the runner. The prompt template and the
// full state are provided; how the runner renders/serializes them is its concern.
type AgentRequest struct {
	NodeID string
	Prompt string
	State  *State
}

// AgentResult is what the runner returns; Output is merged into the state.
type AgentResult struct {
	Output map[string]any
}

// AgentRunner is the port through which agent nodes reach the external multi-provider
// LLM CLI. The graph depends on this abstraction only (dependency inversion); the
// concrete stub and subprocess implementations live in the agentrunner package.
type AgentRunner interface {
	Run(ctx context.Context, req AgentRequest) (AgentResult, error)
}

// AgentNode invokes an AgentRunner and merges its output into the state.
type AgentNode struct {
	id     string
	prompt string
	runner AgentRunner
}

// NewAgentNode builds an agent node bound to a prompt template and a runner.
func NewAgentNode(id, prompt string, runner AgentRunner) *AgentNode {
	return &AgentNode{id: id, prompt: prompt, runner: runner}
}

func (n *AgentNode) ID() string { return n.id }
func (n *AgentNode) Kind() Kind { return KindAgent }

func (n *AgentNode) Execute(ctx context.Context, s *State) error {
	res, err := n.runner.Run(ctx, AgentRequest{NodeID: n.id, Prompt: n.prompt, State: s})
	if err != nil {
		return err
	}
	for k, v := range res.Output {
		s.Set(k, v)
	}
	return nil
}
