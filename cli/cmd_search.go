package cli

import (
	"github.com/spf13/cobra"
)

func (a *App) searchCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Search tutorial articles by title or part",
		Long: `search fetches the full table of contents and filters articles whose
title or part name contains the query string (case-insensitive).

Each result has three fields: title, part, and url.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			n := a.effectiveLimit(0)
			query := args[0]
			a.progressf("searching for %q...", query)
			results, err := a.client.Search(cmd.Context(), query, n)
			if err != nil {
				return &ExitError{Code: exitError, Err: err}
			}
			return a.renderOrEmpty(results, len(results))
		},
	}
	return cmd
}
