package cmd

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"

	"github.com/yoann/kern-orch/internal/agentrunner"
	"github.com/yoann/kern-orch/internal/checkpoint"
	"github.com/yoann/kern-orch/internal/config"
	"github.com/yoann/kern-orch/internal/graph"
	"github.com/yoann/kern-orch/internal/report"
	"github.com/yoann/kern-orch/internal/skills"
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
	// freeze: respawns a fresh context — drops ephemeral-zone keys, keeps persistent
	// ones ("gel = respawn contexte frais"). Node type: tool, func: freeze.
	reg.Tool("freeze", func(_ context.Context, s *graph.State) error {
		s.Freeze(nil)
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

// multiStep chains several step hooks into the single one Engine.OnStep accepts. Hooks run
// in the order given and the first error aborts the run, so durability comes first and
// best-effort observers last. Nil hooks are skipped, which lets a caller pass a disabled
// reporter without branching.
func multiStep(hooks ...graph.StepFunc) graph.StepFunc {
	return func(ctx context.Context, info graph.StepInfo, s *graph.State) error {
		for _, hook := range hooks {
			if hook == nil {
				continue
			}
			if err := hook(ctx, info, s); err != nil {
				return err
			}
		}
		return nil
	}
}

// graphName is the label the UI shows for a run: the topology file without its extension.
func graphName(graphPath string) string {
	base := filepath.Base(graphPath)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

// describeTopology reads the graph's declared shape for the reporter. A failure here is
// never fatal: the run matters, drawing it does not.
func describeTopology(graphPath string) *report.Topology {
	d, err := topology.DescribeFile(graphPath)
	if err != nil {
		return nil
	}

	topo := &report.Topology{Entry: d.Entry}
	for _, n := range d.Nodes {
		topo.Nodes = append(topo.Nodes, report.TopologyNode{ID: n.ID, Kind: n.Kind, Skill: n.Skill})
	}
	for _, e := range d.Edges {
		topo.Edges = append(topo.Edges, report.TopologyEdge{From: e.From, To: e.To, Dynamic: e.Dynamic})
	}
	return topo
}

// newRunID returns a short random run identifier.
func newRunID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// stepCounter remembers where the run had got to, so a failure can be reported against it.
// The engine returns its error from Run, long after the last hook fired — by then the level
// and the frontier are gone unless something kept them.
type stepCounter struct {
	last     int
	frontier []string
}

func (c *stepCounter) count(_ context.Context, info graph.StepInfo, _ *graph.State) error {
	c.last = info.Step
	c.frontier = info.Frontier
	return nil
}

// publishRegistry pushes the skills catalogue to the configured sink.
//
// Best-effort by design, exactly like the step reporter: publishing is observability, and
// a sink that is slow, broken or absent must never be able to stop a graph from running.
// The caller gets the error only so it can say something useful; it must not propagate it.
func publishRegistry(ctx context.Context, cfg config.Config, dir string) error {
	pub := report.NewRegistryPublisher(cfg.RegistryReportURL)
	if !pub.Enabled() {
		return nil
	}
	reg, err := skills.Load(dir)
	if err != nil {
		return err
	}
	return pub.Publish(ctx, reg.List())
}
