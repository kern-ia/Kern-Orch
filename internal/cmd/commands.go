package cmd

import (
	"errors"
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/yoann/kern-orch/internal/skills"
)

// errNotImplemented marks a bootstrap stub to be filled by a later feature branch.
var errNotImplemented = errors.New("not implemented yet")

func newRunCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "run <graph.yaml>",
		Short: "Run a graph from its YAML topology",
		Args:  cobra.ExactArgs(1),
		RunE:  func(*cobra.Command, []string) error { return errNotImplemented },
	}
}

func newResumeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "resume <run-id>",
		Short: "Resume a run from its last checkpoint",
		Args:  cobra.ExactArgs(1),
		RunE:  func(*cobra.Command, []string) error { return errNotImplemented },
	}
}

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status [run-id]",
		Short: "Show status of runs",
		Args:  cobra.MaximumNArgs(1),
		RunE:  func(*cobra.Command, []string) error { return errNotImplemented },
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
