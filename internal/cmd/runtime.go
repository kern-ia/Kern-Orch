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

// checkpointHook persists the state after each level under runID.
func checkpointHook(store *checkpoint.SQLiteStore, runID string) graph.StepFunc {
	return func(ctx context.Context, info graph.StepInfo, s *graph.State) error {
		status := checkpoint.StatusRunning
		if len(info.Frontier) == 0 {
			status = checkpoint.StatusDone
		}
		return store.Save(ctx, checkpoint.Record{
			RunID: runID, Step: info.Step, Frontier: info.Frontier, State: s, Status: status,
		})
	}
}

// newRunID returns a short random run identifier.
func newRunID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
