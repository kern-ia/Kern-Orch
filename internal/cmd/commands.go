package cmd

import (
	"fmt"
	"path/filepath"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/yoann/kern-orch/internal/config"
	"github.com/yoann/kern-orch/internal/graph"
	"github.com/yoann/kern-orch/internal/report"
	"github.com/yoann/kern-orch/internal/skills"
	"github.com/yoann/kern-orch/internal/topology"
)

func newRunCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "run <graph.yaml>",
		Short: "Run a graph from its YAML topology",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := config.FromEnv()
			activity := &activityRelay{}
			runner := newRunner(cfg, activity)
			graphPath, err := filepath.Abs(args[0])
			if err != nil {
				return err
			}
			runID := newRunID()
			name := graphName(graphPath)
			reporter := report.NewHTTP(cfg.StepReportURL)
			reporter.Token = cfg.SinkToken
			// Levels are delivered off the engine's thread, so the last one — and the
			// failure that may follow it — would die with this process without a flush.
			defer reporter.Flush()

			// Wired before the graph is built, not after: a subgraph node receives its
			// hook at construction time.
			reg := builtinRegistry(runner)
			nestedRuns(reg, reporter, runID)

			g, err := topology.LoadFile(graphPath, reg)
			if err != nil {
				return err
			}
			store, err := openStore(cfg)
			if err != nil {
				return err
			}
			defer store.Close()

			// The catalogue rides along with the run so a sink is fed without needing a
			// separate command. A failure here is worth a line on stderr and nothing more.
			if err := publishRegistry(cmd.Context(), cfg, cfg.SkillsDir); err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "kern-orch: publish skills catalogue: %v\n", err)
			}

			steps := &stepCounter{}

			// The runner exists before the run has an id, so the hook is wired here.
			// Flushing before returning matters: the signal that says an agent stopped is
			// the last one a run emits, and it is exactly the one a process exiting would
			// drop — leaving a beacon lit over a run that is long over.
			activityReporter := report.NewActivityReporter(cfg.ActivityReportURL)
			activityReporter.Token = cfg.SinkToken
			activity.fn = func(nodeID string, generating bool) {
				activityReporter.Report(cmd.Context(), runID, name, nodeID, generating)
			}
			defer activityReporter.Flush()

			err = graph.NewEngine(g).
				OnStep(multiStep(
					checkpointHook(store, runID, graphPath),
					steps.count,
					reporter.Hook(runID, name, describeTopology(graphPath)),
				)).
				Run(cmd.Context(), graph.NewState())
			if err != nil {
				reporter.ReportFailure(cmd.Context(), runID, name, steps.last, steps.frontier, failedNodes(err), err.Error())
				fmt.Fprintf(cmd.OutOrStdout(), "run %s failed at last checkpoint: %v\n", runID, err)
				fmt.Fprintf(cmd.OutOrStdout(), "resume with: kern-orch resume %s\n", runID)
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "run %s completed\n", runID)
			return nil
		},
	}
}

func newResumeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "resume <run-id> [graph.yaml]",
		Short: "Resume a run from its last checkpoint (graph path is optional — it is read from the checkpoint)",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			runID := args[0]
			cfg := config.FromEnv()
			store, err := openStore(cfg)
			if err != nil {
				return err
			}
			defer store.Close()

			rec, ok, err := store.Latest(cmd.Context(), runID)
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("no checkpoint for run %q", runID)
			}
			if len(rec.Frontier) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "run %s already complete\n", runID)
				return nil
			}
			// Graph path: explicit arg overrides, else the one recorded at run time.
			graphPath := rec.GraphPath
			if len(args) == 2 {
				graphPath = args[1]
			}
			if graphPath == "" {
				return fmt.Errorf("run %q has no recorded graph path; pass it explicitly: resume %s <graph.yaml>", runID, runID)
			}
			activity := &activityRelay{}
			name := graphName(graphPath)
			reporter := report.NewHTTP(cfg.StepReportURL)
			reporter.Token = cfg.SinkToken
			defer reporter.Flush()

			reg := builtinRegistry(newRunner(cfg, activity))
			nestedRuns(reg, reporter, runID)

			g, err := topology.LoadFile(graphPath, reg)
			if err != nil {
				return err
			}
			steps := &stepCounter{last: rec.Step}

			activityReporter := report.NewActivityReporter(cfg.ActivityReportURL)
			activityReporter.Token = cfg.SinkToken
			activity.fn = func(nodeID string, generating bool) {
				activityReporter.Report(cmd.Context(), runID, name, nodeID, generating)
			}
			defer activityReporter.Flush()

			if err := graph.NewEngine(g).
				OnStep(multiStep(
					checkpointHook(store, runID, graphPath),
					steps.count,
					reporter.Hook(runID, name, describeTopology(graphPath)),
				)).
				RunFrom(cmd.Context(), rec.State, rec.Frontier); err != nil {
				reporter.ReportFailure(cmd.Context(), runID, name, steps.last, steps.frontier, failedNodes(err), err.Error())
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "run %s resumed and completed\n", runID)
			return nil
		},
	}
}

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show status of runs",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg := config.FromEnv()
			store, err := openStore(cfg)
			if err != nil {
				return err
			}
			defer store.Close()

			runs, err := store.List(cmd.Context())
			if err != nil {
				return err
			}
			if len(runs) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "no runs recorded")
				return nil
			}
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "RUN\tSTEP\tSTATUS\tUPDATED")
			for _, r := range runs {
				fmt.Fprintf(w, "%s\t%d\t%s\t%s\n", r.RunID, r.LastStep, r.Status, r.UpdatedAt.Format("2006-01-02 15:04:05"))
			}
			return w.Flush()
		},
	}
}

func newListSkillsCmd() *cobra.Command {
	var dir string
	c := &cobra.Command{
		Use:   "list-skills",
		Short: "List registered skills and their type (tool|agent)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			reg, err := skills.Load(dir)
			if err != nil {
				return err
			}
			list := reg.List()
			if len(list) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "no skills found in %s\n", dir)
				return nil
			}
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "NAME\tTYPE\tDESCRIPTION")
			for _, s := range list {
				fmt.Fprintf(w, "%s\t%s\t%s\n", s.Name, s.Type, s.Description)
			}
			return w.Flush()
		},
	}
	c.Flags().StringVar(&dir, "skills-dir", "skills", "directory containing skill subdirectories")
	return c
}

// newPublishSkillsCmd pushes the catalogue on demand, so a sink can be fed without launching
// a graph first.
func newPublishSkillsCmd() *cobra.Command {
	var dir string
	c := &cobra.Command{
		Use:   "publish-skills",
		Short: "Publish the skills catalogue to the configured sink (kern.registry/v1)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg := config.FromEnv()
			if cfg.RegistryReportURL == "" {
				fmt.Fprintf(cmd.OutOrStdout(),
					"no sink configured: set %s to publish the catalogue\n",
					config.EnvRegistryReportURL)
				return nil
			}
			if err := publishRegistry(cmd.Context(), cfg, dir); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "catalogue published to %s\n", cfg.RegistryReportURL)
			return nil
		},
	}
	c.Flags().StringVar(&dir, "skills-dir", config.FromEnv().SkillsDir,
		"directory containing skill subdirectories")
	return c
}
