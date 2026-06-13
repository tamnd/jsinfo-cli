package cli

import (
	"github.com/spf13/cobra"
)

func (a *App) contentsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "contents",
		Short: "List tutorial articles from the javascript.info table of contents",
		Long: `contents fetches the javascript.info homepage and prints all tutorial
articles in table-of-contents order, grouped by part.

Each record has three fields: title, part, and url.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			n := a.effectiveLimit(0)
			a.progressf("fetching javascript.info table of contents...")
			articles, err := a.client.Contents(cmd.Context(), n)
			if err != nil {
				return &ExitError{Code: exitError, Err: err}
			}
			return a.renderOrEmpty(articles, len(articles))
		},
	}
	return cmd
}
