package agentrunner

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/yoann/kern-orch/internal/graph"
)

// TestEndToEndGraphWithRealSubprocess drives the full chain: Engine -> AgentNode ->
// Subprocess -> a real external process (a shell script standing in for the LLM CLI)
// -> JSON-lines back -> merged into state -> a downstream ToolNode consumes it.
func TestEndToEndGraphWithRealSubprocess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-script fake CLI is POSIX-only")
	}
	dir := t.TempDir()
	cli := filepath.Join(dir, "fake-cli.sh")
	script := "#!/bin/sh\n" +
		"cat >/dev/null\n" + // drain the request on stdin
		`printf '%s\n' '{"type":"token","text":"working"}'` + "\n" +
		`printf '%s\n' '{"type":"result","output":{"answer":"provider-says-hi"}}'` + "\n"
	if err := os.WriteFile(cli, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	runner := &Subprocess{Path: cli}
	g := graph.NewGraph()
	g.AddNode(graph.NewAgentNode("ask", "what is up?", runner))
	g.AddNode(graph.NewToolNode("record", func(_ context.Context, s *State) error {
		if v, _ := s.Get("answer"); v == "provider-says-hi" {
			s.Set("recorded", true)
		}
		return nil
	}))
	g.SetEntry("ask")
	g.AddEdge("ask", graph.Static("record"))

	s := graph.NewState()
	if err := graph.NewEngine(g).Run(context.Background(), s); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if v, _ := s.Get("recorded"); v != true {
		t.Fatalf("end-to-end failed; state=%v", s.Keys())
	}
}

// State is aliased so the ToolNode closure above reads naturally.
type State = graph.State
