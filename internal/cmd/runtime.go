package cmd

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"

	"github.com/yoann/kern-orch/internal/agentrunner"
	"github.com/yoann/kern-orch/internal/checkpoint"
	"github.com/yoann/kern-orch/internal/config"
	"github.com/yoann/kern-orch/internal/graph"
	"github.com/yoann/kern-orch/internal/topology"
)

// newRunner returns the real subprocess runner when KERN_AGENT_CLI is set, otherwise the
// deterministic stub so the harness runs with no LLM configured.
func newRunner(cfg config.Config) graph.AgentRunner {
	if r, ok := agentrunner.NewSubprocessFromEnv(); ok {
		r.Stderr = os.Stderr
		r.TokenSink = os.Stderr
		return r
	}
	return &agentrunner.Stub{}
}

// builtinRegistry wires the built-in tool/router functions available to every graph.
// Projects extend this set in Go; the YAML topology references entries by name.
func builtinRegistry(runner graph.AgentRunner) *topology.Registry {
	reg := topology.NewRegistry(runner)
	reg.Tool("noop", func(context.Context, *graph.State) error { return nil })
	// double: demo tool for the subgraph example — multiplies state key "n" by 2.
	reg.Tool("double", func(_ context.Context, s *graph.State) error {
		if v, ok := s.Get("n"); ok {
			if iv, ok := v.(int); ok {
				s.Set("n", iv*2)
			}
		}
		return nil
	})
	// seed: demo tool that initializes state key "n" to 3.
	reg.Tool("seed", func(_ context.Context, s *graph.State) error {
		s.Set("n", 3)
		return nil
	})
	return reg
}

// openStore opens the checkpoint store, creating the parent directory if needed.
func openStore(cfg config.Config) (*checkpoint.SQLiteStore, error) {
	if dir := filepath.Dir(cfg.CheckpointDB); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}
	return checkpoint.OpenSQLite(cfg.CheckpointDB)
}

// checkpointHook persists the state after each level under runID, recording graphPath
// so `resume` can reload the graph without the caller re-supplying it.
func checkpointHook(store *checkpoint.SQLiteStore, runID, graphPath string) graph.StepFunc {
	return func(ctx context.Context, info graph.StepInfo, s *graph.State) error {
		status := checkpoint.StatusRunning
		if len(info.Frontier) == 0 {
			status = checkpoint.StatusDone
		}
		return store.Save(ctx, checkpoint.Record{
			RunID: runID, Step: info.Step, Frontier: info.Frontier, State: s,
			Status: status, GraphPath: graphPath,
		})
	}
}

// newRunID returns a short random run identifier.
func newRunID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
