package cmd

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/yoann/kern-orch/internal/checkpoint"
	"github.com/yoann/kern-orch/internal/config"
	"github.com/yoann/kern-orch/internal/daemon"
	"github.com/yoann/kern-orch/internal/skills"
)

// writeDispatchSkills writes one tool skill ("echo", a real Python subprocess) and one
// agent skill ("planner", backed by the deterministic stub since KERN_AGENT_CLI is unset
// in tests) — enough to exercise both branches Dispatch can take.
func writeDispatchSkills(t *testing.T, dir string) {
	t.Helper()
	toolDir := filepath.Join(dir, "echo")
	if err := os.MkdirAll(toolDir, 0o755); err != nil {
		t.Fatal(err)
	}
	skillMD := "---\nname: echo\ntype: tool\ncommand: [\"python3\", \"" + filepath.Join(toolDir, "tool.py") + "\"]\n---\n"
	if err := os.WriteFile(filepath.Join(toolDir, "SKILL.md"), []byte(skillMD), 0o644); err != nil {
		t.Fatal(err)
	}
	toolPy := "#!/usr/bin/env python3\nimport json, sys\njson.load(sys.stdin)\nprint(json.dumps({\"label\": \"Echo\", \"value\": \"ok\"}))\n"
	if err := os.WriteFile(filepath.Join(toolDir, "tool.py"), []byte(toolPy), 0o755); err != nil {
		t.Fatal(err)
	}

	agentDir := filepath.Join(dir, "planner")
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(agentDir, "SKILL.md"), []byte("---\nname: planner\ntype: agent\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDaemonRunnerDispatchInvokesATool(t *testing.T) {
	dir := t.TempDir()
	writeDispatchSkills(t, dir)
	store := openDaemonStore(t, dir)
	d := &daemonRunner{cfg: config.Config{SkillsDir: dir}, store: store}

	result, err := d.Dispatch(context.Background(), "echo", "", "")
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if result.Kind != "tool" || result.Result == nil || result.Result.Value != "ok" {
		t.Errorf("got %+v", result)
	}
}

// The real proof dispatch's agent path works: a run genuinely launched, genuinely
// completed, carrying the requester and the chat text as its whole prompt.
func TestDaemonRunnerDispatchLaunchesAnAgentRun(t *testing.T) {
	dir := t.TempDir()
	writeDispatchSkills(t, dir)
	store := openDaemonStore(t, dir)
	d := &daemonRunner{cfg: config.Config{SkillsDir: dir}, store: store}

	result, err := d.Dispatch(context.Background(), "planner", "analyse ceci", "yoann")
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if result.Kind != "run" || result.RunID == "" {
		t.Fatalf("got %+v", result)
	}
	waitForRun(t, store, result.RunID, checkpoint.StatusDone)

	rec, ok, err := d.GetRun(context.Background(), result.RunID)
	if err != nil || !ok {
		t.Fatalf("GetRun: ok=%v err=%v", ok, err)
	}
	if rec.Requester != "yoann" {
		t.Errorf("Requester = %q, want yoann", rec.Requester)
	}
	if v, _ := rec.State.Get("echo"); v != "analyse ceci" {
		t.Errorf("stub did not receive the dispatched text as its prompt: %v", v)
	}
}

func TestDaemonRunnerDispatchOnAnUnknownSkillListsKnownNames(t *testing.T) {
	dir := t.TempDir()
	writeDispatchSkills(t, dir)
	store := openDaemonStore(t, dir)
	d := &daemonRunner{cfg: config.Config{SkillsDir: dir}, store: store}

	_, err := d.Dispatch(context.Background(), "jamais", "", "")
	var unknown *daemon.UnknownSkillError
	if !errors.As(err, &unknown) {
		t.Fatalf("err = %v, want *daemon.UnknownSkillError", err)
	}
	if len(unknown.Known) != 2 {
		t.Errorf("Known = %v, want [echo planner]", unknown.Known)
	}
}

func TestDispatchInputWithNoRequiredParamIgnoresText(t *testing.T) {
	input, err := dispatchInput(skills.Skill{}, "hello")
	if err != nil || input != nil {
		t.Errorf("got %v, %v; want nil, nil", input, err)
	}
}

func TestDispatchInputWithOneRequiredParamUsesTheWholeText(t *testing.T) {
	sk := skills.Skill{Params: []skills.Param{{Name: "name", Required: true}}}
	input, err := dispatchInput(sk, "Yoann")
	if err != nil {
		t.Fatalf("dispatchInput: %v", err)
	}
	if input["name"] != "Yoann" {
		t.Errorf("input = %v, want name=Yoann", input)
	}
}

func TestDispatchInputWithSeveralRequiredParamsRefuses(t *testing.T) {
	sk := skills.Skill{
		Name: "multi",
		Params: []skills.Param{
			{Name: "a", Required: true},
			{Name: "b", Required: true},
		},
	}
	if _, err := dispatchInput(sk, "x"); err == nil {
		t.Error("dispatchInput accepted a skill needing two values from one chat message")
	}
}
