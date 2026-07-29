package cmd

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/yoann/kern-orch/internal/agentrunner"
	"github.com/yoann/kern-orch/internal/checkpoint"
	"github.com/yoann/kern-orch/internal/config"
	"github.com/yoann/kern-orch/internal/graph"
	"github.com/yoann/kern-orch/internal/notify"
	"github.com/yoann/kern-orch/internal/report"
	"github.com/yoann/kern-orch/internal/skills"
	"github.com/yoann/kern-orch/internal/topology"
)

// newRunner returns the real subprocess runner when KERN_AGENT_CLI is set, otherwise the
// deterministic stub so the harness runs with no LLM configured.
// activityRelay is the seam between the runner and the reporter. The runner is built before
// the run has an id, so the hook cannot be written at construction time; the relay is handed
// over empty and filled once the id exists. A nil target is a no-op, which is what an
// unconfigured sink amounts to.
type activityRelay struct {
	fn func(nodeID string, generating bool)
}

func (a *activityRelay) call(nodeID string, generating bool) {
	if a.fn != nil {
		a.fn(nodeID, generating)
	}
}

func newRunner(cfg config.Config, activity *activityRelay) graph.AgentRunner {
	if r, ok := agentrunner.NewSubprocessFromEnv(); ok {
		r.Stderr = os.Stderr
		r.TokenSink = os.Stderr
		if activity != nil {
			r.OnActivity = activity.call
		}
		return r
	}
	return &agentrunner.Stub{}
}

// builtinRegistry wires the built-in tool/router functions available to every graph.
// Projects extend this set in Go; the YAML topology references entries by name.
func builtinRegistry(runner graph.AgentRunner, cfg config.Config) *topology.Registry {
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
	// announce: demo tool for the notify example — sets state key "message" to a fixed
	// demo string, standing in for what an agent's own output would set.
	reg.Tool("announce", func(_ context.Context, s *graph.State) error {
		s.Set("message", "Test kern-orch : le nœud notify a bien envoyé ce message.")
		return nil
	})
	// notify: an agent's own outbound channel to a human — sends state key "message" to
	// Telegram. Unconfigured (no KERN_TELEGRAM_BOT_TOKEN/KERN_TELEGRAM_CHAT_ID) fails
	// the node rather than dropping the message silently.
	var notifyClient *notify.Client
	if cfg.TelegramBotToken != "" && cfg.TelegramChatID != "" {
		notifyClient = notify.New(cfg.TelegramBotToken, cfg.TelegramChatID)
	}
	reg.Tool("notify", notify.Tool(notifyClient))
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
	pub.Token = cfg.SinkToken
	if !pub.Enabled() {
		return nil
	}
	reg, err := skills.Load(dir)
	if err != nil {
		return err
	}
	return pub.Publish(ctx, reg.List())
}

// failedNodes pulls the nodes a level error names. An error of any other shape names none,
// and the caller then reports the frontier alone — which is what happened before the engine
// carried this.
func failedNodes(err error) []string {
	var lvl *graph.LevelError
	if errors.As(err, &lvl) {
		return lvl.Nodes
	}
	return nil
}

// nestedRuns wires every subgraph node so its nested graph reports as a run of its own,
// pointing back at the node it belongs to.
//
// Each execution gets a fresh run id: the same node running twice — a retry, a loop — is
// two nested runs, not one run reported twice. The shape is read from the file the loader
// resolved; a graph built in Go carries no file and travels without a topology, exactly as
// an undeclared parent would.
func nestedRuns(reg *topology.Registry, reporter *report.HTTPReporter, parentRun string) {
	if !reporter.Enabled() {
		return
	}
	reg.OnChildStep(func(nodeID, graphRef string) graph.StepFunc {
		return reporter.NestedHook(newRunID(), graphName(graphRef), describeTopology(graphRef),
			&report.Parent{RunID: parentRun, NodeID: nodeID})
	})
}
