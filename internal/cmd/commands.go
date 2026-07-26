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
			runner := newRunner(cfg)
			graphPath, err := filepath.Abs(args[0])
			if err != nil {
				return err
			}
			g, err := topology.LoadFile(graphPath, builtinRegistry(runner))
			if err != nil {
				return err
			}
			store, err := openStore(cfg)
			if err != nil {
				return err
			}
			defer store.Close()

			runID := newRunID()
			err = graph.NewEngine(g).
				OnStep(multiStep(
					checkpointHook(store, runID, graphPath),
					report.NewHTTP(cfg.StepReportURL).Hook(runID, graphName(graphPath)),
				)).
				Run(cmd.Context(), graph.NewState())
			if err != nil {
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
			g, err := topology.LoadFile(graphPath, builtinRegistry(newRunner(cfg)))
			if err != nil {
				return err
			}
			if err := graph.NewEngine(g).
				OnStep(multiStep(
					checkpointHook(store, runID, graphPath),
					report.NewHTTP(cfg.StepReportURL).Hook(runID, graphName(graphPath)),
				)).
				RunFrom(cmd.Context(), rec.State, rec.Frontier); err != nil {
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
