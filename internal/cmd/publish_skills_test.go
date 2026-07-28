package cmd

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/yoann/kern-orch/internal/config"
)

// catalogueSink records the last catalogue posted to it.
type catalogueSink struct {
	server *httptest.Server

	mu   sync.Mutex
	body map[string]any
	hits int
}

func newCatalogueSink(t *testing.T) *catalogueSink {
	t.Helper()
	s := &catalogueSink{}
	s.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		s.mu.Lock()
		defer s.mu.Unlock()
		s.hits++
		_ = json.Unmarshal(raw, &s.body)
		w.WriteHeader(http.StatusAccepted)
	}))
	t.Cleanup(s.server.Close)
	return s
}

func (s *catalogueSink) last() map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.body
}

func (s *catalogueSink) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.hits
}

// skillsDir writes a skills tree and returns its path.
func skillsDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Join(dir, name), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name, "SKILL.md"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("analyse", "---\nname: Analyse\ntype: tool\ndescription: Décompose une demande.\n---\nbody\n")
	write("scribe", "---\nname: Scribe\ntype: agent\ndescription: Rédige.\n---\nbody\n")
	return dir
}

func TestPublishSkillsPostsTheCatalogue(t *testing.T) {
	sink := newCatalogueSink(t)
	t.Setenv(config.EnvRegistryReportURL, sink.server.URL)

	out, err := execute(t, "publish-skills", "--skills-dir", skillsDir(t))
	if err != nil {
		t.Fatalf("publish-skills: %v (%s)", err, out)
	}

	body := sink.last()
	if body["source"] != "kern-orch" {
		t.Errorf("source = %v, want kern-orch", body["source"])
	}
	list, ok := body["skills"].([]any)
	if !ok || len(list) != 2 {
		t.Fatalf("skills = %#v, want the two skills on disk", body["skills"])
	}
	first := list[0].(map[string]any)
	if first["name"] != "Analyse" || first["kind"] != "tool" {
		t.Errorf("skills[0] = %v, want the Analyse tool first (sorted by name)", first)
	}
}

// Without a sink configured the command must say so rather than silently succeed: an
// operator running it expects a catalogue to land somewhere, and a no-op that looks like a
// success is how you spend an afternoon debugging the wrong brick.
func TestPublishSkillsSaysWhenNoSinkIsConfigured(t *testing.T) {
	t.Setenv(config.EnvRegistryReportURL, "")

	out, err := execute(t, "publish-skills", "--skills-dir", skillsDir(t))
	if err != nil {
		t.Fatalf("publish-skills: %v", err)
	}

	if !strings.Contains(out, config.EnvRegistryReportURL) {
		t.Errorf("output = %q, want it to name %s", out, config.EnvRegistryReportURL)
	}
}

// A run publishes the catalogue too, so a sink is fed without a separate command.
func TestRunPublishesTheCatalogue(t *testing.T) {
	sink := newCatalogueSink(t)
	dir := t.TempDir()
	graphPath := filepath.Join(dir, "g.yaml")
	if err := os.WriteFile(graphPath, []byte(exampleGraph), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv(config.EnvRegistryReportURL, sink.server.URL)
	t.Setenv(config.EnvSkillsDir, skillsDir(t))
	t.Setenv(config.EnvCheckpointDB, filepath.Join(dir, "k.db"))

	if _, err := execute(t, "run", graphPath); err != nil {
		t.Fatalf("run: %v", err)
	}

	if sink.count() == 0 {
		t.Error("the run never published the catalogue")
	}
}

// Publishing is observability: a sink that is down must not stop a graph from running.
func TestRunSurvivesADeadRegistrySink(t *testing.T) {
	dir := t.TempDir()
	graphPath := filepath.Join(dir, "g.yaml")
	if err := os.WriteFile(graphPath, []byte(exampleGraph), 0o644); err != nil {
		t.Fatal(err)
	}

	// Port 1 refuses connections on every platform we build for.
	t.Setenv(config.EnvRegistryReportURL, "http://127.0.0.1:1/registry")
	t.Setenv(config.EnvSkillsDir, skillsDir(t))
	t.Setenv(config.EnvCheckpointDB, filepath.Join(dir, "k.db"))

	out, err := execute(t, "run", graphPath)
	if err != nil {
		t.Fatalf("a dead registry sink aborted the run: %v (%s)", err, out)
	}
	if !strings.Contains(out, "completed") {
		t.Errorf("output = %q, want the run to have completed anyway", out)
	}
}

// The activity signal has to survive the whole wiring: runner hook → relay → reporter →
// sink. Unit tests cover each link; only this one covers the chain.
func TestRunBracketsAgentActivity(t *testing.T) {
	sink := newCatalogueSink(t)
	dir := t.TempDir()

	agent := filepath.Join(dir, "agent.sh")
	script := "#!/bin/sh\ncat > /dev/null\nprintf '{\"type\":\"result\",\"output\":{}}\\n'\n"
	if err := os.WriteFile(agent, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	graphPath := filepath.Join(dir, "g.yaml")
	if err := os.WriteFile(graphPath, []byte(exampleGraph), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("KERN_AGENT_CLI", agent)
	t.Setenv(config.EnvActivityReportURL, sink.server.URL)
	t.Setenv(config.EnvCheckpointDB, filepath.Join(dir, "k.db"))

	if _, err := execute(t, "run", graphPath); err != nil {
		t.Fatalf("run: %v", err)
	}

	// One agent node, so exactly one start and one stop — and the stop must have arrived,
	// which is what the command's flush is there to guarantee.
	if got := sink.count(); got != 2 {
		t.Fatalf("the sink received %d signals, want a start and a stop", got)
	}
	last := sink.last()
	if last["generating"] != false {
		t.Errorf("the last signal is %v, want the stop", last["generating"])
	}
	if last["node_id"] != "greet" {
		t.Errorf("node_id = %v, want the agent node", last["node_id"])
	}
}

// Delivery moved off the engine's thread, so the guarantee that matters is no longer
// "it was posted" but "the command waited for it before exiting".
func TestRunDeliversEveryLevelDespiteAsyncReporting(t *testing.T) {
	sink := newCatalogueSink(t)
	dir := t.TempDir()
	graphPath := filepath.Join(dir, "g.yaml")
	if err := os.WriteFile(graphPath, []byte(exampleGraph), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv(config.EnvStepReportURL, sink.server.URL)
	t.Setenv(config.EnvCheckpointDB, filepath.Join(dir, "k.db"))

	if _, err := execute(t, "run", graphPath); err != nil {
		t.Fatalf("run: %v", err)
	}

	// greet then finish, then the empty frontier that closes the run.
	if got := sink.count(); got < 2 {
		t.Fatalf("the sink received %d levels, want the whole run — the flush came too early", got)
	}
	if last := sink.last(); last["frontier"] == nil {
		t.Errorf("the last event carries no frontier: %v", last)
	} else if fr, _ := last["frontier"].([]any); len(fr) != 0 {
		t.Errorf("the last frontier is %v, want the empty one that closes the run", fr)
	}
}

// A sink nobody can reach must cost the run nothing at all, now that nothing waits on it.
func TestRunIsNotSlowedByAnUnreachableSink(t *testing.T) {
	dir := t.TempDir()
	graphPath := filepath.Join(dir, "g.yaml")
	if err := os.WriteFile(graphPath, []byte(exampleGraph), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv(config.EnvStepReportURL, "http://127.0.0.1:1/steps")
	t.Setenv(config.EnvCheckpointDB, filepath.Join(dir, "k.db"))

	out, err := execute(t, "run", graphPath)
	if err != nil {
		t.Fatalf("an unreachable sink aborted the run: %v (%s)", err, out)
	}
	if !strings.Contains(out, "completed") {
		t.Errorf("output = %q, want the run to have completed", out)
	}
}
