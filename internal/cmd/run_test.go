package cmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const exampleGraph = `
entry: greet
nodes:
  - id: greet
    type: agent
    prompt: "hi there"
  - id: finish
    type: tool
    func: noop
edges:
  - from: greet
    to: [finish]
`

// execute runs the root command with args and returns combined stdout.
func execute(t *testing.T, args ...string) (string, error) {
	t.Helper()
	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)
	err := root.ExecuteContext(context.Background())
	return out.String(), err
}

func TestRunThenStatus(t *testing.T) {
	dir := t.TempDir()
	graphPath := filepath.Join(dir, "g.yaml")
	if err := os.WriteFile(graphPath, []byte(exampleGraph), 0o644); err != nil {
		t.Fatal(err)
	}
	// Isolate config: stub runner (no KERN_AGENT_CLI), temp checkpoint db.
	t.Setenv("KERN_AGENT_CLI", "")
	t.Setenv("KERN_CHECKPOINT_DB", filepath.Join(dir, "cp.db"))

	out, err := execute(t, "run", graphPath)
	if err != nil {
		t.Fatalf("run: %v\n%s", err, out)
	}
	if !strings.Contains(out, "completed") {
		t.Fatalf("run output = %q; want completed", out)
	}

	statusOut, err := execute(t, "status")
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if !strings.Contains(statusOut, "done") {
		t.Fatalf("status output = %q; want a done run", statusOut)
	}
}

func TestRunUnknownFuncFails(t *testing.T) {
	dir := t.TempDir()
	graphPath := filepath.Join(dir, "bad.yaml")
	os.WriteFile(graphPath, []byte("entry: a\nnodes:\n  - id: a\n    type: tool\n    func: ghost\n"), 0o644)
	t.Setenv("KERN_CHECKPOINT_DB", filepath.Join(dir, "cp.db"))
	if _, err := execute(t, "run", graphPath); err == nil {
		t.Fatal("expected run to fail on unknown func")
	}
}
