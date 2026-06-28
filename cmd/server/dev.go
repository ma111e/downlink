package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/ma111e/downlink/cmd/server/internal/config"
	"github.com/ma111e/downlink/cmd/server/internal/notification"
	"github.com/ma111e/downlink/cmd/server/internal/notification/devserver"
	"github.com/ma111e/downlink/cmd/server/internal/store"
	"github.com/ma111e/downlink/pkg/models"
	"github.com/ma111e/downlink/pkg/utils"
)

// newDevCommand builds the `server dev` command group. Its subcommands are local
// development helpers; they deliberately skip the root server's heavy
// PersistentPreRunE (config + DB + feed manager init) via an empty override.
func newDevCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dev",
		Short: "Local development helpers",
		// Override the root PersistentPreRunE so dev commands don't init the DB
		// or feed manager unless they ask for it themselves.
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error { return nil },
	}

	cmd.AddCommand(newDevDigestCommand())
	return cmd
}

func newDevDigestCommand() *cobra.Command {
	var (
		addr         string
		templatesDir string
		assetsDir    string
		noOpen       bool
		exportDir    string
		testDigestID string
		theme        string
		from         string
		to           string
		between      string
		day          string
	)

	cmd := &cobra.Command{
		Use:           "digest",
		Short:         "Serve the digest HTML templates with live reload",
		SilenceUsage:  true,
		SilenceErrors: false,
		Long: `Serve the digest, swipe and archive templates over HTTP and reload the
browser whenever a *.tmpl file changes. Templates are read from disk on every
request, so edits show up with no recompile.

By default a built-in sample digest is rendered. Pass --test-digest-id to render
a single real digest from the local database, or use --from/--to/--between/--day
to serve every stored digest created within a time window (handy for developing
the archive index against real data).`,
		Example: `  # Built-in sample fixture (no DB)
  server dev digest

  # A single stored digest
  server dev digest --test-digest-id <id>

  # Every digest created in the last 30 days
  server dev digest --from 30d

  # Every digest created on a single UTC day
  server dev digest --day 2025-01-15`,
		RunE: func(cmd *cobra.Command, args []string) error {
			windowSet := from != "" || to != "" || between != "" || day != ""

			var digests []models.Digest
			var feeds []models.Feed

			switch {
			case windowSet:
				if testDigestID != "" {
					return fmt.Errorf("--test-digest-id cannot be combined with --from/--to/--between/--day")
				}
				start, end, err := resolveDigestWindow(from, to, between, day)
				if err != nil {
					return err
				}
				digests, err = loadDigestsInWindow(start, end)
				if err != nil {
					return err
				}
			case testDigestID != "":
				digest, err := loadDigestFromDB(testDigestID)
				if err != nil {
					return err
				}
				digests = []models.Digest{digest}
			default:
				digests = []models.Digest{notification.SampleDigest("dev", time.Now())}
			}

			// On the DB-backed paths the store is already initialized; load the feeds
			// so the sources page lists real sources. The sample fixture has none.
			if windowSet || testDigestID != "" {
				loaded, err := store.Db.ListFeeds()
				if err != nil {
					return fmt.Errorf("list feeds: %w", err)
				}
				feeds = loaded
			}

			devOpts := devserver.Options{
				Addr:         addr,
				TemplatesDir: templatesDir,
				AssetsDir:    assetsDir,
				OpenBrowser:  !noOpen,
				Digests:      digests,
				Feeds:        feeds,
				Theme:        theme,
			}

			if exportDir != "" {
				return devserver.Export(devOpts, exportDir)
			}

			return devserver.Run(devOpts)
		},
	}

	cmd.Flags().StringVar(&addr, "addr", ":8099", "Address to listen on")
	cmd.Flags().StringVar(&templatesDir, "templates-dir", "cmd/server/internal/notification/templates", "Directory of *.tmpl files to serve and watch")
	cmd.Flags().StringVar(&assetsDir, "assets-dir", "cmd/server/internal/notification/assets", "Directory of Vite-built CSS/JS to serve and watch (run `npm run watch` in web/ alongside)")
	cmd.Flags().BoolVar(&noOpen, "no-open", false, "Do not open the browser on startup")
	cmd.Flags().StringVar(&exportDir, "export", "", "Render the page set to this directory as static HTML and exit (no server)")
	cmd.Flags().StringVar(&testDigestID, "test-digest-id", "", "Render this digest id from the local DB instead of the sample fixture")
	cmd.Flags().StringVar(&theme, "theme", "", `Template layout to use (default: "default")`)
	cmd.Flags().StringVar(&from, "from", "", "Start of the window selecting stored digests by creation time (e.g., 'now', '2025-01-01', '24h'; default: 24h)")
	cmd.Flags().StringVar(&to, "to", "", "End of the window selecting stored digests by creation time (e.g., 'now', '2025-01-01', '1h')")
	cmd.Flags().StringVar(&between, "between", "", "Select stored digests created between two dates/durations (e.g., '7d,1d', '2025-01-01,2025-01-07')")
	cmd.Flags().StringVar(&day, "day", "", "Select stored digests created on a single day, midnight-to-midnight UTC (YYYY-MM-DD, 'today', or 'yesterday'). Mutually exclusive with --from/--to/--between")

	return cmd
}

// resolveDigestWindow turns the --from/--to/--between/--day flags into a [start,end]
// window. It mirrors the behaviour of the dlk `digest generate` command
// (cmd/dlk/digests.go + cmd/dlk/helpers.go) so the flag semantics are identical.
func resolveDigestWindow(from, to, between, day string) (start, end time.Time, err error) {
	if day != "" {
		if from != "" || to != "" || between != "" {
			return time.Time{}, time.Time{}, fmt.Errorf("--day cannot be combined with --from, --to, or --between")
		}
		return utils.ParseDayUTC(day)
	}

	if between != "" {
		parts := strings.SplitN(between, ",", 2)
		if len(parts) != 2 {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid --between format: must be two values separated by comma (e.g., '-7d,-1d')")
		}
		s, perr := utils.ParseTimeString(strings.TrimSpace(parts[0]))
		if perr != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid start time in --between: %w", perr)
		}
		e, perr := utils.ParseTimeString(strings.TrimSpace(parts[1]))
		if perr != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid end time in --between: %w", perr)
		}
		if s.After(e) {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid --between: start time must be before end time")
		}
		return s, e, nil
	}

	if from != "" {
		start, err = utils.ParseTimeString(from)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("error parsing --from: %w", err)
		}
	} else {
		start = time.Now().Add(-24 * time.Hour)
	}

	if to != "" {
		end, err = utils.ParseTimeString(to)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("error parsing --to: %w", err)
		}
	} else {
		end = time.Now()
	}

	if end.Before(start) {
		return time.Time{}, time.Time{}, fmt.Errorf("error: --to (%v) cannot be before --from (%v)", end, start)
	}
	return start, end, nil
}

// loadDigestsInWindow returns every stored digest created within [start,end],
// fully hydrated for rendering. It reuses the lightweight ListDigests to find the
// matching ids, then GetDigest to load each one's articles and analyses.
func loadDigestsInWindow(start, end time.Time) ([]models.Digest, error) {
	if err := initStore(); err != nil {
		return nil, err
	}

	all, err := store.Db.ListDigests(0, false)
	if err != nil {
		return nil, fmt.Errorf("list digests: %w", err)
	}

	var digests []models.Digest
	for _, d := range all {
		if d.CreatedAt.Before(start) || d.CreatedAt.After(end) {
			continue
		}
		full, err := store.Db.GetDigest(d.Id)
		if err != nil {
			return nil, fmt.Errorf("load digest %q: %w", d.Id, err)
		}
		digests = append(digests, full)
	}

	if len(digests) == 0 {
		return nil, fmt.Errorf("no digests created between %s and %s",
			start.Format("2006-01-02 15:04"), end.Format("2006-01-02 15:04"))
	}
	return digests, nil
}

// loadDigestFromDB loads a single digest by id from the local store.
func loadDigestFromDB(id string) (models.Digest, error) {
	if err := initStore(); err != nil {
		return models.Digest{}, err
	}
	digest, err := store.Db.GetDigest(id)
	if err != nil {
		return models.Digest{}, fmt.Errorf("load digest %q: %w", id, err)
	}
	return digest, nil
}

// initStore lazily initializes config + the database for the DB-backed dev paths.
func initStore() error {
	if err := config.Init(); err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if err := store.Init(); err != nil {
		return fmt.Errorf("init database: %w", err)
	}
	return nil
}
