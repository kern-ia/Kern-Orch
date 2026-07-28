package cmd

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/yoann/kern-orch/internal/checkpoint"
	"github.com/yoann/kern-orch/internal/config"
	"github.com/yoann/kern-orch/internal/daemon"
)

func openDaemonStore(t *testing.T, dir string) *checkpoint.SQLiteStore {
	t.Helper()
	st, err := checkpoint.OpenSQLite(filepath.Join(dir, "cp.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func waitForRun(t *testing.T, store *checkpoint.SQLiteStore, runID string, want string) checkpoint.Record {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		rec, ok, err := store.Latest(context.Background(), runID)
		if err != nil {
			t.Fatalf("Latest: %v", err)
		}
		if ok && rec.Status == want {
			return rec
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("run %s never reached status %q", runID, want)
	return checkpoint.Record{}
}

// The whole point of the daemon: a run starts in the background and the caller gets its id
// back immediately, without waiting for it to finish.
func TestDaemonRunnerStartsARunInTheBackground(t *testing.T) {
	dir := t.TempDir()
	graphPath := filepath.Join(dir, "g.yaml")
	if err := os.WriteFile(graphPath, []byte(exampleGraph), 0o644); err != nil {
		t.Fatal(err)
	}
	store := openDaemonStore(t, dir)
	d := &daemonRunner{cfg: config.Config{}, store: store}

	runID, err := d.StartRun(context.Background(), graphPath)
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	if runID == "" {
		t.Fatal("StartRun returned no id")
	}

	waitForRun(t, store, runID, checkpoint.StatusDone)
}

// A queued marker must exist the instant StartRun returns, before the engine has produced
// anything: a status query landing between the two calls must not 404.
func TestDaemonRunnerMarksARunQueuedImmediately(t *testing.T) {
	dir := t.TempDir()
	graphPath := filepath.Join(dir, "g.yaml")
	if err := os.WriteFile(graphPath, []byte(exampleGraph), 0o644); err != nil {
		t.Fatal(err)
	}
	store := openDaemonStore(t, dir)
	d := &daemonRunner{cfg: config.Config{}, store: store}

	runID, err := d.StartRun(context.Background(), graphPath)
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}

	rec, ok, err := store.Latest(context.Background(), runID)
	if err != nil || !ok {
		t.Fatalf("Latest immediately after StartRun: ok=%v err=%v", ok, err)
	}
	if rec.Status != checkpoint.StatusQueued && rec.Status != checkpoint.StatusRunning && rec.Status != checkpoint.StatusDone {
		t.Errorf("status = %q right after StartRun, want it to already exist", rec.Status)
	}
}

// A bad graph must fail synchronously: a caller deserves to know immediately rather than
// poll for a run that will never appear.
func TestDaemonRunnerFailsFastOnABadGraph(t *testing.T) {
	dir := t.TempDir()
	graphPath := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(graphPath, []byte("entry: a\nnodes:\n  - id: a\n    type: tool\n    func: ghost\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	store := openDaemonStore(t, dir)
	d := &daemonRunner{cfg: config.Config{}, store: store}

	if _, err := d.StartRun(context.Background(), graphPath); err == nil {
		t.Fatal("StartRun accepted a graph with an unknown tool func")
	}
}

// A missing file is the same story: synchronous, not a goroutine that silently never
// reports anything.
func TestDaemonRunnerFailsFastOnAMissingGraph(t *testing.T) {
	dir := t.TempDir()
	store := openDaemonStore(t, dir)
	d := &daemonRunner{cfg: config.Config{}, store: store}

	if _, err := d.StartRun(context.Background(), filepath.Join(dir, "absent.yaml")); err == nil {
		t.Fatal("StartRun accepted a graph file that does not exist")
	}
}

func TestDaemonRunnerListsAndGetsRuns(t *testing.T) {
	dir := t.TempDir()
	graphPath := filepath.Join(dir, "g.yaml")
	if err := os.WriteFile(graphPath, []byte(exampleGraph), 0o644); err != nil {
		t.Fatal(err)
	}
	store := openDaemonStore(t, dir)
	d := &daemonRunner{cfg: config.Config{}, store: store}

	runID, err := d.StartRun(context.Background(), graphPath)
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	waitForRun(t, store, runID, checkpoint.StatusDone)

	runs, err := d.ListRuns(context.Background())
	if err != nil || len(runs) != 1 || runs[0].RunID != runID {
		t.Fatalf("ListRuns = %v, %v; want the one run", runs, err)
	}

	rec, ok, err := d.GetRun(context.Background(), runID)
	if err != nil || !ok || rec.RunID != runID {
		t.Fatalf("GetRun = %+v, %v, %v; want the run", rec, ok, err)
	}

	if _, ok, err := d.GetRun(context.Background(), "jamais"); err != nil || ok {
		t.Errorf("GetRun on an unknown id = ok=%v err=%v, want ok=false err=nil", ok, err)
	}
}

// Resuming an already-complete run is a no-op: there is nothing left to do, and that is not
// an error condition — the CLI's `resume` says "already complete" for the same case.
func TestDaemonRunnerResumeOnACompleteRunIsANoop(t *testing.T) {
	dir := t.TempDir()
	graphPath := filepath.Join(dir, "g.yaml")
	if err := os.WriteFile(graphPath, []byte(exampleGraph), 0o644); err != nil {
		t.Fatal(err)
	}
	store := openDaemonStore(t, dir)
	d := &daemonRunner{cfg: config.Config{}, store: store}

	runID, err := d.StartRun(context.Background(), graphPath)
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	waitForRun(t, store, runID, checkpoint.StatusDone)

	if err := d.ResumeRun(context.Background(), runID); err != nil {
		t.Errorf("ResumeRun on a complete run = %v, want nil", err)
	}
}

func TestDaemonRunnerResumeOnAnUnknownRunIsErrUnknownRun(t *testing.T) {
	dir := t.TempDir()
	store := openDaemonStore(t, dir)
	d := &daemonRunner{cfg: config.Config{}, store: store}

	err := d.ResumeRun(context.Background(), "jamais")
	if !errors.Is(err, daemon.ErrUnknownRun) {
		t.Errorf("ResumeRun on an unknown id = %v, want daemon.ErrUnknownRun", err)
	}
}
