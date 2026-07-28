package cmd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/yoann/kern-orch/internal/checkpoint"
	"github.com/yoann/kern-orch/internal/config"
	"github.com/yoann/kern-orch/internal/daemon"
	"github.com/yoann/kern-orch/internal/graph"
	"github.com/yoann/kern-orch/internal/report"
	"github.com/yoann/kern-orch/internal/topology"
)

// preparedRun is a graph built and its reporters wired, ready to execute. Separating this
// from execution is what lets a caller fail fast: loading a bad graph or an absent file
// surfaces synchronously, before anything runs in the background where nobody could see it.
type preparedRun struct {
	graph            *graph.Graph
	graphPath        string
	name             string
	reporter         *report.HTTPReporter
	activity         *activityRelay
	activityReporter *report.ActivityReporter
}

// prepareRun builds the graph and wires every reporter run/resume/the daemon share. It
// touches no store and starts no engine — see preparedRun.
func prepareRun(cfg config.Config, runID, graphPath string) (*preparedRun, error) {
	activity := &activityRelay{}
	reporter := report.NewHTTP(cfg.StepReportURL)
	reporter.Token = cfg.SinkToken

	// Wired before the graph is built, not after: a subgraph node receives its hook at
	// construction time.
	reg := builtinRegistry(newRunner(cfg, activity))
	nestedRuns(reg, reporter, runID)

	g, err := topology.LoadFile(graphPath, reg)
	if err != nil {
		return nil, err
	}

	name := graphName(graphPath)
	activityReporter := report.NewActivityReporter(cfg.ActivityReportURL)
	activityReporter.Token = cfg.SinkToken
	activity.fn = func(nodeID string, generating bool) {
		activityReporter.Report(context.Background(), runID, name, nodeID, generating)
	}

	return &preparedRun{
		graph: g, graphPath: graphPath, name: name,
		reporter: reporter, activity: activity, activityReporter: activityReporter,
	}, nil
}

// run executes a prepared graph to completion — fresh if resume is nil, continued from a
// checkpoint otherwise — and reports a failure if there is one. It is the single
// implementation `run`, `resume` and the daemon all call: none of the three may drift from
// how a report is flushed or a failure is announced.
func (p *preparedRun) run(ctx context.Context, store *checkpoint.SQLiteStore, runID string, resume *checkpoint.Record) error {
	defer p.reporter.Flush()
	defer p.activityReporter.Flush()

	steps := &stepCounter{}
	hook := multiStep(
		checkpointHook(store, runID, p.graphPath),
		steps.count,
		p.reporter.Hook(runID, p.name, describeTopology(p.graphPath)),
	)

	engine := graph.NewEngine(p.graph).OnStep(hook)

	var err error
	if resume != nil {
		steps.last = resume.Step
		err = engine.RunFrom(ctx, resume.State, resume.Frontier)
	} else {
		err = engine.Run(ctx, graph.NewState())
	}
	if err != nil {
		p.reporter.ReportFailure(ctx, runID, p.name, steps.last, steps.frontier, failedNodes(err), err.Error())
	}
	return err
}

// daemonRunner implements daemon.Runner using exactly the engine wiring the CLI's `run` and
// `resume` commands use. It is the only place internal/daemon's abstract Runner meets a real
// graph — the daemon package itself knows nothing of graphs, checkpoints or reporters.
type daemonRunner struct {
	cfg   config.Config
	store *checkpoint.SQLiteStore
}

// StartRun prepares and validates the graph synchronously — a caller learns about a bad
// path or a bad graph immediately — then runs it in the background. A queued checkpoint is
// written before returning, so a status query racing the response never finds nothing.
func (d *daemonRunner) StartRun(ctx context.Context, graphPath string) (string, error) {
	abs, err := filepath.Abs(graphPath)
	if err != nil {
		return "", err
	}
	runID := newRunID()

	prepared, err := prepareRun(d.cfg, runID, abs)
	if err != nil {
		return "", err
	}

	if err := d.store.Save(ctx, checkpoint.Record{
		RunID: runID, Step: checkpoint.QueuedStep, State: graph.NewState(),
		Status: checkpoint.StatusQueued, GraphPath: abs,
	}); err != nil {
		return "", err
	}

	go func() {
		// Detached from the request: the HTTP response returns long before the run does,
		// and its context would already be cancelled.
		if err := prepared.run(context.Background(), d.store, runID, nil); err != nil {
			slog.Error("kern-orch: run failed", "run_id", runID, "error", err)
			return
		}
		slog.Info("kern-orch: run completed", "run_id", runID)
	}()
	return runID, nil
}

// ResumeRun continues a run in the background. A run with an empty frontier is already
// complete — a no-op, not an error, matching what `kern-orch resume` tells a human.
func (d *daemonRunner) ResumeRun(ctx context.Context, runID string) error {
	rec, ok, err := d.store.Latest(ctx, runID)
	if err != nil {
		return err
	}
	if !ok {
		return daemon.ErrUnknownRun
	}
	if len(rec.Frontier) == 0 {
		return nil
	}
	if rec.GraphPath == "" {
		return fmt.Errorf("run %q has no recorded graph path", runID)
	}

	prepared, err := prepareRun(d.cfg, runID, rec.GraphPath)
	if err != nil {
		return err
	}

	go func() {
		if err := prepared.run(context.Background(), d.store, runID, &rec); err != nil {
			slog.Error("kern-orch: resumed run failed", "run_id", runID, "error", err)
			return
		}
		slog.Info("kern-orch: resumed run completed", "run_id", runID)
	}()
	return nil
}

func (d *daemonRunner) ListRuns(ctx context.Context) ([]checkpoint.Summary, error) {
	return d.store.List(ctx)
}

func (d *daemonRunner) GetRun(ctx context.Context, runID string) (checkpoint.Record, bool, error) {
	return d.store.Latest(ctx, runID)
}

const shutdownGrace = 10 * time.Second

func newServeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Run kern-orch as a long-lived service, accepting runs over HTTP",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg := config.FromEnv()

			if err := checkServeExposure(cfg.ServeAddr, cfg.ServeToken); err != nil {
				return err
			}

			store, err := openStore(cfg)
			if err != nil {
				return err
			}
			defer store.Close()

			runner := &daemonRunner{cfg: cfg, store: store}
			srv := &http.Server{
				Addr:              cfg.ServeAddr,
				Handler:           daemon.NewRouter(runner, cfg.ServeToken),
				ReadHeaderTimeout: 5 * time.Second,
			}

			ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()

			errc := make(chan error, 1)
			go func() {
				slog.Info("kern-orch: serving", "addr", cfg.ServeAddr,
					"authenticated", cfg.ServeToken != "")
				if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
					errc <- err
					return
				}
				errc <- nil
			}()

			select {
			case err := <-errc:
				return err
			case <-ctx.Done():
				slog.Info("kern-orch: shutting down")
				shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
				defer cancel()
				// Runs already in flight keep going in their own goroutines and checkpoint
				// as they always do; this only stops the server from taking new requests.
				return srv.Shutdown(shutdownCtx)
			}
		},
	}
}

// checkServeExposure refuses a public address with no credential. A warning would scroll
// past on the first busy day; a process that will not start does not. kern-ui enforces the
// same rule on its own API for the same reason — re-derived here, not shared: neither brick
// depends on the other's internals.
func checkServeExposure(addr, token string) error {
	if !isPublicAddr(addr) || token != "" {
		return nil
	}
	return fmt.Errorf(
		"refusing to listen on %s with no credential: set %s.\n"+
			"Anyone who can reach this address could start, resume or read every run.\n"+
			"To run locally instead, leave %s at 127.0.0.1:7070",
		addr, config.EnvServeToken, config.EnvServeAddr)
}

// isPublicAddr reports whether addr can be reached from another machine. An empty host is
// the trap: `:7070` reads as innocent and binds every interface.
func isPublicAddr(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	host = strings.Trim(host, "[]")

	switch host {
	case "":
		return true
	case "localhost":
		return false
	}
	if ip := net.ParseIP(host); ip != nil {
		return !ip.IsLoopback()
	}
	return true
}
