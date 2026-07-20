package checkpoint

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/yoann/kern-orch/internal/graph"
)

// TestCrashThenResume runs a 3-node chain a->b->c where b fails on the first attempt.
// After the crash we reopen the store, load the latest checkpoint, and RunFrom its
// frontier to completion — proving checkpoint + resume works end to end.
func TestCrashThenResume(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "run.db")
	const runID = "run-42"
	boom := errors.New("b crashed")
	bShouldFail := true

	build := func() *graph.Graph {
		g := graph.NewGraph()
		g.AddNode(graph.NewToolNode("a", func(_ context.Context, s *graph.State) error {
			s.Set("a_done", true)
			return nil
		}))
		g.AddNode(graph.NewToolNode("b", func(_ context.Context, s *graph.State) error {
			if bShouldFail {
				return boom
			}
			s.Set("b_done", true)
			return nil
		}))
		g.AddNode(graph.NewToolNode("c", func(_ context.Context, s *graph.State) error {
			s.Set("c_done", true)
			return nil
		}))
		g.SetEntry("a")
		g.AddEdge("a", graph.Static("b"))
		g.AddEdge("b", graph.Static("c"))
		return g
	}

	hook := func(store *SQLiteStore) graph.StepFunc {
		return func(ctx context.Context, info graph.StepInfo, s *graph.State) error {
			status := StatusRunning
			if len(info.Frontier) == 0 {
				status = StatusDone
			}
			return store.Save(ctx, Record{RunID: runID, Step: info.Step, Frontier: info.Frontier, State: s, Status: status})
		}
	}

	// --- First run: crashes at b. Checkpoint after a recorded frontier [b]. ---
	st1, err := OpenSQLite(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	err = graph.NewEngine(build()).OnStep(hook(st1)).Run(context.Background(), graph.NewState())
	if !errors.Is(err, boom) {
		t.Fatalf("first run error = %v; want boom", err)
	}
	rec, ok, err := st1.Latest(context.Background(), runID)
	if err != nil || !ok {
		t.Fatalf("Latest after crash: ok=%v err=%v", ok, err)
	}
	if len(rec.Frontier) != 1 || rec.Frontier[0] != "b" {
		t.Fatalf("checkpoint frontier = %v; want [b]", rec.Frontier)
	}
	st1.Close()

	// --- Resume: reopen, load checkpoint, fix b, continue from saved frontier. ---
	bShouldFail = false
	st2, err := OpenSQLite(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer st2.Close()
	rec, _, _ = st2.Latest(context.Background(), runID)
	if v, _ := rec.State.Get("a_done"); v != true {
		t.Fatalf("restored state lost a_done: %v", rec.State.Keys())
	}
	if err := graph.NewEngine(build()).OnStep(hook(st2)).RunFrom(context.Background(), rec.State, rec.Frontier); err != nil {
		t.Fatalf("resume: %v", err)
	}
	final, _, _ := st2.Latest(context.Background(), runID)
	if final.Status != StatusDone {
		t.Fatalf("final status = %q; want done", final.Status)
	}
	for _, k := range []string{"a_done", "b_done", "c_done"} {
		if v, _ := final.State.Get(k); v != true {
			t.Fatalf("final state missing %s: %v", k, final.State.Keys())
		}
	}
}
