package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// After a run, `resume <run-id>` with NO graph argument must resolve the run and use the
// graph path recorded in the checkpoint.
func TestResumeWithoutGraphArgUsesRecordedPath(t *testing.T) {
	dir := t.TempDir()
	graphPath := filepath.Join(dir, "g.yaml")
	if err := os.WriteFile(graphPath, []byte(exampleGraph), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KERN_AGENT_CLI", "")
	t.Setenv("KERN_CHECKPOINT_DB", filepath.Join(dir, "cp.db"))

	out, err := execute(t, "run", graphPath)
	if err != nil {
		t.Fatalf("run: %v\n%s", err, out)
	}
	runID := extractRunID(t, out)

	// No graph.yaml arg — the path must come from the checkpoint.
	resumeOut, err := execute(t, "resume", runID)
	if err != nil {
		t.Fatalf("resume without graph arg: %v\n%s", err, resumeOut)
	}
	if !strings.Contains(resumeOut, "already complete") {
		t.Fatalf("resume output = %q; want 'already complete'", resumeOut)
	}
}

func TestResumeUnknownRunErrors(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("KERN_CHECKPOINT_DB", filepath.Join(dir, "cp.db"))
	if _, err := execute(t, "resume", "does-not-exist"); err == nil {
		t.Fatal("expected error for unknown run id")
	}
}

// extractRunID pulls the run id out of "run <id> completed".
func extractRunID(t *testing.T, out string) string {
	t.Helper()
	for _, f := range strings.Fields(out) {
		if len(f) == 16 { // hex run id from newRunID()
			return f
		}
	}
	t.Fatalf("no run id found in output %q", out)
	return ""
}
