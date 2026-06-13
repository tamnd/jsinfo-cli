// Package cli builds the jsinfo command tree on top of the jsinfo library.
package cli

import (
	"fmt"
	"os"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
	"github.com/tamnd/jsinfo-cli/jsinfo"
)

// Build metadata, set via -ldflags at release time.
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

// exit codes.
const (
	exitError  = 1
	exitUsage  = 2
	exitNoData = 3
)

// App holds shared state threaded through every command.
type App struct {
	client *jsinfo.Client
	cfg    jsinfo.Config

	format   string
	fields   []string
	noHeader bool
	template string
	limit    int
	quiet    bool
}

// Root builds the root command and its subtree.
func Root() *cobra.Command {
	app := &App{cfg: jsinfo.DefaultConfig()}

	root := &cobra.Command{
		Use:   "jsinfo",
		Short: "Browse JavaScript.info modern JavaScript tutorial",
		Long: `jsinfo browses the javascript.info Modern JavaScript Tutorial.

It fetches the table of contents from javascript.info and lets you list or
search tutorial articles. No API key required — the site is public.

jsinfo is an independent tool and is not affiliated with javascript.info.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			return app.setup()
		},
	}

	pf := root.PersistentFlags()
	pf.StringVar(&app.format, "format", "auto", "output format: table|json|jsonl|csv|tsv (auto=table on TTY, jsonl piped)")
	pf.StringSliceVar(&app.fields, "fields", nil, "comma-separated columns to include")
	pf.BoolVar(&app.noHeader, "no-header", false, "omit the header row in table/csv/tsv")
	pf.StringVar(&app.template, "template", "", "Go text/template applied per record")
	pf.IntVarP(&app.limit, "limit", "n", 0, "max number of records (0 = command default)")
	pf.BoolVarP(&app.quiet, "quiet", "q", false, "suppress progress messages on stderr")

	pf.StringVar(&app.cfg.BaseURL, "base-url", app.cfg.BaseURL, "javascript.info base URL")
	pf.DurationVar(&app.cfg.Rate, "delay", app.cfg.Rate, "minimum gap between requests")
	pf.IntVar(&app.cfg.Retries, "retries", app.cfg.Retries, "retry attempts on 429/5xx")
	pf.StringVar(&app.cfg.UserAgent, "user-agent", app.cfg.UserAgent, "User-Agent header")

	root.AddCommand(
		app.contentsCmd(),
		app.searchCmd(),
		newVersionCmd(),
	)
	return root
}

func (a *App) setup() error {
	if a.format == "" || a.format == "auto" {
		if isatty.IsTerminal(os.Stdout.Fd()) {
			a.format = "table"
		} else {
			a.format = "jsonl"
		}
	}
	if !validFormat(a.format) {
		return &ExitError{Code: exitUsage, Err: fmt.Errorf("unknown format %q", a.format)}
	}
	a.client = jsinfo.NewClient(a.cfg)
	return nil
}

func (a *App) render(records any) error {
	r := NewRenderer(os.Stdout, a.format, a.fields, a.noHeader, a.template)
	return r.Render(records)
}

func (a *App) renderOrEmpty(records any, n int) error {
	if err := a.render(records); err != nil {
		return err
	}
	if n == 0 {
		return &ExitError{Code: exitNoData}
	}
	return nil
}

func (a *App) progressf(format string, args ...any) {
	if a.quiet {
		return
	}
	fmt.Fprintf(os.Stderr, format+"\n", args...)
}

func (a *App) effectiveLimit(def int) int {
	if a.limit > 0 {
		return a.limit
	}
	return def
}
