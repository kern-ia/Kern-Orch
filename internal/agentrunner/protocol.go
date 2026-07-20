// Package agentrunner provides implementations of the graph.AgentRunner port: a
// deterministic Stub (so the harness runs with no LLM configured) and a Subprocess
// runner that speaks a JSON-lines protocol to the external multi-provider CLI.
//
// PROVISIONAL CONTRACT (spec §6.4) — to be reconciled with the real CLI once
// accessible. The harness writes exactly one Request JSON object to the child's stdin
// and closes it. The child streams Events, one JSON object per line, on stdout:
//
//	{"type":"token","text":"..."}     // incremental output; forwarded to TokenSink
//	{"type":"result","output":{...}}  // final; output map is merged into the state
//	{"type":"error","message":"..."}  // aborts the run
//
// The last "result" event wins. A non-zero exit with no result is an error.
package agentrunner

import "encoding/json"

// Request is the single object sent to the child on stdin.
type Request struct {
	NodeID string          `json:"node_id"`
	Prompt string          `json:"prompt"`
	State  json.RawMessage `json:"state"`
}

// Event is one line of the child's streamed stdout.
type Event struct {
	Type    string         `json:"type"`
	Text    string         `json:"text,omitempty"`
	Output  map[string]any `json:"output,omitempty"`
	Message string         `json:"message,omitempty"`
}

const (
	eventToken  = "token"
	eventResult = "result"
	eventError  = "error"
)
