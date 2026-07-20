package agentrunner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/yoann/kern-orch/internal/graph"
)

var _ graph.AgentRunner = (*Subprocess)(nil)

// helperRunner builds a Subprocess that re-invokes this test binary as the fake CLI.
func helperRunner(t *testing.T, mode string, sink *bytes.Buffer) *Subprocess {
	t.Helper()
	r := &Subprocess{
		Path: os.Args[0],
		Args: []string{"-test.run=TestHelperProcess"},
		Env:  append(os.Environ(), "GO_WANT_HELPER_PROCESS=1", "HELPER_MODE="+mode),
	}
	if sink != nil { // avoid a typed-nil *bytes.Buffer becoming a non-nil io.Writer
		r.TokenSink = sink
	}
	return r
}

func TestSubprocessParsesResultAndStreamsTokens(t *testing.T) {
	var sink bytes.Buffer
	r := helperRunner(t, "ok", &sink)
	st := graph.NewState()
	st.Set("topic", "routing")
	res, err := r.Run(context.Background(), graph.AgentRequest{NodeID: "n1", Prompt: "hi", State: st})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Output["answer"] != "42" {
		t.Fatalf("output = %v; want answer=42", res.Output)
	}
	if res.Output["echo_node"] != "n1" {
		t.Fatalf("child did not receive request: %v", res.Output)
	}
	if sink.String() != "think..done" {
		t.Fatalf("token sink = %q; want think..done", sink.String())
	}
}

func TestSubprocessReturnsErrorEvent(t *testing.T) {
	r := helperRunner(t, "error", nil)
	_, err := r.Run(context.Background(), graph.AgentRequest{NodeID: "n", Prompt: "p", State: graph.NewState()})
	if err == nil {
		t.Fatal("expected error from error event")
	}
}

func TestSubprocessErrorsWhenNoResult(t *testing.T) {
	r := helperRunner(t, "noresult", nil)
	_, err := r.Run(context.Background(), graph.AgentRequest{NodeID: "n", Prompt: "p", State: graph.NewState()})
	if err == nil {
		t.Fatal("expected error when child emits no result")
	}
}

// TestHelperProcess is not a real test: when GO_WANT_HELPER_PROCESS=1 it impersonates
// the external CLI, reading the Request from stdin and emitting Events per HELPER_MODE.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	var req Request
	_ = json.NewDecoder(os.Stdin).Decode(&req)
	enc := json.NewEncoder(os.Stdout)
	switch os.Getenv("HELPER_MODE") {
	case "ok":
		enc.Encode(Event{Type: eventToken, Text: "think.."})
		enc.Encode(Event{Type: eventToken, Text: "done"})
		enc.Encode(Event{Type: eventResult, Output: map[string]any{"answer": "42", "echo_node": req.NodeID}})
	case "error":
		enc.Encode(Event{Type: eventError, Message: "provider unavailable"})
	case "noresult":
		enc.Encode(Event{Type: eventToken, Text: "..."})
	}
	fmt.Fprint(os.Stderr, "") // keep stderr wiring exercised
	os.Exit(0)
}
