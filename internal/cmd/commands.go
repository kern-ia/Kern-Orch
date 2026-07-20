package cmd

import (
	"errors"

	"github.com/spf13/cobra"
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
	return &cobra.Command{
		Use:   "list-skills",
		Short: "List registered skills and their type (tool|agent)",
		Args:  cobra.NoArgs,
		RunE:  func(*cobra.Command, []string) error { return errNotImplemented },
	}
}
