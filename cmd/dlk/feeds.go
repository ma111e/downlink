package main

import (
	"encoding/json"
	"fmt"
	"github.com/ma111e/downlink/pkg/downlinkclient"
	"github.com/ma111e/downlink/pkg/models"
	"github.com/ma111e/downlink/pkg/protos"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"charm.land/huh/v2"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// ── live refresh display ──────────────────────────────────────────────────────

type feedRow struct {
	title    string
	done     bool
	stored   int32
	skipped  int32
	errCount int
	errors   []string
	fatal    bool // feed itself failed to fetch
}

type progressDisplay struct {
	rows       []*feedRow
	index      map[string]int // feed_id → row index
	frame      int
	drawnLines int
	mu         sync.Mutex
	stopCh     chan struct{}
	wg         sync.WaitGroup
}

func newProgressDisplay() *progressDisplay {
	return &progressDisplay{
		index:  make(map[string]int),
		stopCh: make(chan struct{}),
	}
}

func (d *progressDisplay) addFeed(id, title string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.index[id] = len(d.rows)
	d.rows = append(d.rows, &feedRow{title: title})
	d.redraw()
}

func (d *progressDisplay) completeFeed(result *protos.RefreshFeedResponse) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if idx, ok := d.index[result.FeedId]; ok {
		r := d.rows[idx]
		r.done = true
		r.stored = result.Stored
		r.skipped = result.Skipped
		r.errCount = len(result.Errors)
		r.errors = result.Errors
		r.fatal = result.TotalFetched == 0 && len(result.Errors) > 0
	}
	d.redraw()
}

func (d *progressDisplay) startSpinner() {
	ticker := time.NewTicker(100 * time.Millisecond)
	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		defer ticker.Stop()
		for {
			select {
			case <-d.stopCh:
				return
			case <-ticker.C:
				d.mu.Lock()
				d.frame = (d.frame + 1) % len(spinnerFrames)
				d.redraw()
				d.mu.Unlock()
			}
		}
	}()
}

// redraw redraws all rows in-place. Caller must hold d.mu.
func (d *progressDisplay) redraw() {
	if len(d.rows) == 0 {
		return
	}
	if d.drawnLines > 0 {
		fmt.Printf("\033[%dA", d.drawnLines) // cursor up N lines
	}
	for _, r := range d.rows {
		fmt.Print("\r\033[K") // clear line
		if r.done {
			switch {
			case r.fatal:
				fmt.Printf("  ✗ %-45s failed to fetch\n", r.title)
			case r.errCount > 0:
				fmt.Printf("  ⚠ %-45s +%d stored, %d skipped  (%d errors)\n", r.title, r.stored, r.skipped, r.errCount)
			default:
				fmt.Printf("  ✓ %-45s +%d stored, %d skipped\n", r.title, r.stored, r.skipped)
			}
		} else {
			fmt.Printf("  %s %-45s\n", spinnerFrames[d.frame], r.title)
		}
	}
	d.drawnLines = len(d.rows)
}

func (d *progressDisplay) stop() {
	close(d.stopCh)
	d.wg.Wait()
	// Final draw with spinner stopped
	d.mu.Lock()
	d.redraw()
	d.mu.Unlock()
}

func (d *progressDisplay) printErrors() {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, r := range d.rows {
		if len(r.errors) == 0 {
			continue
		}
		fmt.Printf("\n  Errors for %s:\n", r.title)
		for _, e := range r.errors {
			fmt.Printf("    • %s\n", e)
		}
	}
}

// refreshAllFeedsWithWindow refreshes every feed in parallel constrained to the
// given time window, rendering live progress and printing a summary line. It
// returns an error only when the feed list itself cannot be fetched; per-feed
// scrape failures are surfaced through the progress display.
func refreshAllFeedsWithWindow(client *downlinkclient.DownlinkClient, fromTime, toTime *time.Time, overwrite, restore bool, lastN int) error {
	all, err := client.ListFeeds()
	if err != nil {
		return fmt.Errorf("failed to list feeds: %w", err)
	}

	var feeds []models.Feed
	for _, f := range all {
		if f.Enabled != nil && *f.Enabled {
			feeds = append(feeds, f)
		}
	}

	if len(feeds) == 0 {
		fmt.Println("No feeds to refresh.")
		return nil
	}

	display := newProgressDisplay()
	display.startSpinner()

	for _, feed := range feeds {
		display.addFeed(feed.Id, feed.Title)
	}

	var totalStored, totalSkipped, totalErrors atomic.Int32
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup

	for _, feed := range feeds {
		wg.Add(1)
		feed := feed
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			resp, err := client.RefreshFeedWithTimeWindow(feed.Id, fromTime, toTime, overwrite, restore, lastN)
			if err != nil {
				display.completeFeed(&protos.RefreshFeedResponse{
					FeedId:    feed.Id,
					FeedTitle: feed.Title,
					Errors:    []string{err.Error()},
				})
			} else {
				display.completeFeed(resp)
				totalStored.Add(resp.Stored)
				totalSkipped.Add(resp.Skipped)
				totalErrors.Add(int32(len(resp.Errors)))
			}
		}()
	}
	wg.Wait()
	display.stop()
	display.printErrors()

	fmt.Printf("\nDone. %d stored, %d skipped", totalStored.Load(), totalSkipped.Load())
	if totalErrors.Load() > 0 {
		fmt.Printf(", %d scrape errors", totalErrors.Load())
	}
	fmt.Println()
	return nil
}

// ─────────────────────────────────────────────────────────────────────────────

var (
	// Feed registration flags
	feedURL               string
	feedName              string
	feedCategory          string
	feedScraping          string
	feedArticleSelector   string
	feedCutoffSelector    string
	feedBlacklistSelector string

	// apply / delete flags
	applyFile    string
	applyDryRun  bool
	deleteFile   string
	deleteTitle  string
	deleteId     string
	deleteDryRun bool

	// export flags
	exportOutput      string
	exportEnabledOnly bool

	// diagnose flags
	diagnoseRaw bool
)

// Feed commands
func createFeedCommands() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "feeds",
		Short: "Manage feeds",
		Long:  `List, add, refresh, and delete RSS/Atom feeds.`,
	}

	// List feeds command
	listCmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"l", "ls"},
		Short:   "List all feeds",
		Long:    `Display all registered feeds with their details.`,
		Run: func(cmd *cobra.Command, args []string) {
			client := getNewDownlinkClient()

			feeds, err := client.ListFeeds()
			if err != nil {
				fmt.Printf("Failed to list feeds: %v\n", err)
				return
			}

			if len(feeds) == 0 {
				fmt.Println("No feeds registered.")
				return
			}

			if jsonOutput {
				out, err := json.MarshalIndent(feeds, "", "  ")
				if err != nil {
					fmt.Printf("Error marshalling to JSON: %v\n", err)
					return
				}
				fmt.Println(string(out))
			} else {
				printFeedTable(feeds)
			}
		},
	}

	// Add feed command
	addCmd := &cobra.Command{
		Use:   "add",
		Short: "Register a new feed",
		Long:  `Add a new feed to be monitored. Run without --url to use the interactive wizard.`,
		Run: func(cmd *cobra.Command, args []string) {
			client := getNewDownlinkClient()

			if !cmd.Flags().Changed("url") {
				runAddFeedInteractive(client)
				return
			}

			feedConfig := models.FeedConfig{
				URL:      feedURL,
				Type:     feedCategory,
				Title:    feedName,
				Enabled:  true,
				Scraping: feedScraping,
			}

			if feedArticleSelector != "" || feedCutoffSelector != "" || feedBlacklistSelector != "" {
				feedConfig.Selectors = &models.Selectors{
					Article:   feedArticleSelector,
					Cutoff:    feedCutoffSelector,
					Blacklist: feedBlacklistSelector,
				}
			}

			if err := client.RegisterFeed(feedConfig); err != nil {
				fmt.Printf("Failed to register feed: %v\n", err)
				return
			}
			fmt.Println(styleOK.Render("✓") + " Feed registered successfully")
		},
	}

	// Add flags for add command
	addCmd.Flags().StringVar(&feedURL, "url", "", "Feed URL (required)")
	addCmd.Flags().StringVar(&feedName, "name", "", "Feed name (optional, will be auto-detected if empty)")
	addCmd.Flags().StringVar(&feedCategory, "type", "rss", "Feed type (e.g. rss)")
	addCmd.Flags().StringVar(&feedScraping, "scraping", "", `Scraping mode: "dynamic", "full_browser", or "none" (use feed content, no fetch) (default: static)`)
	addCmd.Flags().StringVar(&feedArticleSelector, "selector-article", "", "CSS selector for article content")
	addCmd.Flags().StringVar(&feedCutoffSelector, "selector-cutoff", "", "CSS selector where extraction stops")
	addCmd.Flags().StringVar(&feedBlacklistSelector, "selector-blacklist", "", "CSS selector for elements to exclude")

	// Refresh feeds command
	var fromStr, toStr, betweenStr string
	var overwrite, restore, refreshDryRun, refreshDebug bool
	var lastN int
	refreshCmd := &cobra.Command{
		Use:   "refresh [feed-id-or-name|all]",
		Short: "Refresh feeds",
		Long: `Trigger a refresh of all feeds or a specific feed.

Feed Selection:
  You can specify a feed by:
    - Feed ID (exact match)
    - Feed name (normalized: lowercase, spaces->hyphens, special chars->underscores)
    - "all" (special keyword to refresh all feeds)

  Examples:
    downlink-cli feeds refresh my-feed-id              # By ID
    downlink-cli feeds refresh "Tech News"             # By name (will be normalized to "tech-news")
    downlink-cli feeds refresh tech-news               # By normalized name
    downlink-cli feeds refresh all                     # Refresh all feeds explicitly

Time window filtering:
  Use --from and --to to filter articles by publication date.
  Supported formats:
    - "now" (current time)
    - RFC3339: "2006-01-02T15:04:05Z07:00"
    - Date: "2006-01-02"
    - Relative: "-7d" (7 days ago), "-2h" (2 hours ago), "-30m" (30 minutes ago)

Examples:
  downlink-cli feeds refresh all                      # Refresh all feeds
  downlink-cli feeds refresh tech-news --from 7d       # Articles from last 7 days
  downlink-cli feeds refresh "My Feed" --from 2025-01-01  # Articles from Jan 1, 2025
  downlink-cli feeds refresh feed-123 --from 1d --to now  # Articles from last 24 hours
  downlink-cli feeds refresh my-feed --last-n 10      # 10 most recent articles
  downlink-cli feeds refresh all --last-n 5           # 5 most recent articles per feed`,
		Args: cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			fromTime, toTime, err := parseTimeWindow(fromStr, toStr, betweenStr, nil)
			if err != nil {
				fmt.Println(err)
				return
			}

			if lastN > 0 && (fromTime != nil || toTime != nil) {
				fmt.Println("Error: --last-n cannot be combined with --from, --to, or --between")
				return
			}

			client := getNewDownlinkClient()

			// Check if specific feed ID or name was provided (but not "all")
			if len(args) > 0 && args[0] != "all" {
				feedIdentifier := args[0]

				feed, feeds, err := findFeedByIDOrNormalizedName(client, feedIdentifier)
				if err != nil && feeds == nil {
					fmt.Printf("Failed to list feeds: %v\n", err)
					return
				}
				if err != nil {
					fmt.Println(err)
					printAvailableFeeds(feeds)
					return
				}
				feedId := feed.Id
				feedTitle := feed.Title

				// Dry-run mode: just show what would be refreshed
				if refreshDryRun {
					fmt.Printf("Would refresh feed: %s\n", feedTitle)
					if fromTime != nil {
						fmt.Printf("  From: %s\n", fromTime.Format("2006-01-02 15:04:05"))
					}
					if toTime != nil {
						fmt.Printf("  To: %s\n", toTime.Format("2006-01-02 15:04:05"))
					}

					// Fetch articles for this feed to show what would be refreshed
					filter := models.ArticleFilter{
						FeedId:    feedId,
						StartDate: fromTime,
						EndDate:   toTime,
					}
					articles, err := client.ListArticles(filter)
					if err == nil && len(articles) > 0 {
						fmt.Printf("  Articles (%d):\n", len(articles))
						for _, article := range articles {
							fmt.Printf("    - %s\n", article.Title)
							if refreshDebug {
								printArticleContentPreview(article.Content, "      ")
							}
						}
					}

					fmt.Println("  (no actual refresh performed)")
					return
				}

				resp, err := client.RefreshFeedWithTimeWindow(feedId, fromTime, toTime, overwrite, restore, lastN)
				if err != nil {
					fmt.Printf("Failed to refresh feed %s: %v\n", feedTitle, err)
					return
				}

				fmt.Printf("Feed '%s' refreshed\n", feedTitle)
				fmt.Printf("  Fetched: %-4d  Stored: %-4d  Skipped: %d\n",
					resp.TotalFetched, resp.Stored, resp.Skipped)
				if len(resp.Errors) > 0 {
					fmt.Printf("  Errors (%d):\n", len(resp.Errors))
					for _, e := range resp.Errors {
						fmt.Printf("    - %s\n", e)
					}
				}
			} else if len(args) == 0 {
				feed, err := selectFeed(client)
				if err != nil {
					fmt.Printf("Error: %v\n", err)
					return
				}
				if feed.Id == "" {
					fmt.Println("Cancelled.")
					return
				}
				resp, err := client.RefreshFeedWithTimeWindow(feed.Id, fromTime, toTime, overwrite, restore, lastN)
				if err != nil {
					fmt.Printf("Failed to refresh feed %s: %v\n", feed.Title, err)
					return
				}
				fmt.Printf("Feed %q refreshed\n", feed.Title)
				fmt.Printf("  Fetched: %-4d  Stored: %-4d  Skipped: %d\n",
					resp.TotalFetched, resp.Stored, resp.Skipped)
				if len(resp.Errors) > 0 {
					fmt.Printf("  Errors (%d):\n", len(resp.Errors))
					for _, e := range resp.Errors {
						fmt.Printf("    - %s\n", e)
					}
				}
				return
			} else {
				// Refresh all feeds (args[0] == "all")
				// If time window or last-n filtering is requested, refresh each feed individually with the filter
				if fromTime != nil || toTime != nil || lastN > 0 {
					// Dry-run mode: just list feeds that would be refreshed
					if refreshDryRun {
						allFeeds, err := client.ListFeeds()
						if err != nil {
							fmt.Printf("Failed to list feeds: %v\n", err)
							return
						}

						var feeds []models.Feed
						for _, f := range allFeeds {
							if f.Enabled != nil && *f.Enabled {
								feeds = append(feeds, f)
							}
						}

						if len(feeds) == 0 {
							fmt.Println("No feeds to refresh.")
							return
						}

						fmt.Printf("Would refresh %d feeds with time window:\n", len(feeds))
						if fromTime != nil {
							fmt.Printf("  From: %s\n", fromTime.Format("2006-01-02 15:04:05"))
						}
						if toTime != nil {
							fmt.Printf("  To: %s\n", toTime.Format("2006-01-02 15:04:05"))
						}
						fmt.Println("\nFeeds to refresh:")
						for _, feed := range feeds {
							fmt.Printf("  - %s\n", feed.Title)
							// Show articles for this feed that would be refreshed
							filter := models.ArticleFilter{
								FeedId:    feed.Id,
								StartDate: fromTime,
								EndDate:   toTime,
							}
							articles, err := client.ListArticles(filter)
							if err == nil && len(articles) > 0 {
								for i, article := range articles {
									if i < 5 { // Show first 5 articles
										fmt.Printf("      • %s\n", article.Title)
									}
								}
								if len(articles) > 5 {
									fmt.Printf("      ... and %d more\n", len(articles)-5)
								}
							}
						}
						fmt.Println("  (no actual refresh performed)")
						return
					}

					if err := refreshAllFeedsWithWindow(client, fromTime, toTime, overwrite, restore, lastN); err != nil {
						fmt.Println(err)
						return
					}
				} else {
					// No time window filtering, use the efficient stream-based refresh
					// Dry-run mode: just list all feeds that would be refreshed
					if refreshDryRun {
						feeds, err := client.ListFeeds()
						if err != nil {
							fmt.Printf("Failed to list feeds: %v\n", err)
							return
						}

						if len(feeds) == 0 {
							fmt.Println("No feeds to refresh.")
							return
						}

						fmt.Printf("Would refresh %d feeds:\n", len(feeds))
						for _, feed := range feeds {
							fmt.Printf("  - %s\n", feed.Title)
							// Show recent articles for this feed
							filter := models.ArticleFilter{
								FeedId: feed.Id,
							}
							articles, err := client.ListArticles(filter)
							if err == nil && len(articles) > 0 {
								for i, article := range articles {
									if i < 5 { // Show first 5 articles
										fmt.Printf("      • %s\n", article.Title)
									}
								}
								if len(articles) > 5 {
									fmt.Printf("      ... and %d more\n", len(articles)-5)
								}
							}
						}
						fmt.Println("  (no actual refresh performed)")
						return
					}

					display := newProgressDisplay()
					display.startSpinner()

					var totalStored, totalSkipped, totalErrors int32
					err := client.RefreshAllFeeds(func(ev *protos.RefreshAllFeedsEvent) {
						switch ev.EventType {
						case protos.RefreshEventType_STARTED:
							display.addFeed(ev.Result.FeedId, ev.Result.FeedTitle)
						case protos.RefreshEventType_COMPLETED:
							display.completeFeed(ev.Result)
							totalStored += ev.Result.Stored
							totalSkipped += ev.Result.Skipped
							totalErrors += int32(len(ev.Result.Errors))
						}
					})
					display.stop()
					display.printErrors()

					if err != nil {
						fmt.Printf("Failed to refresh feeds: %v\n", err)
						return
					}

					fmt.Printf("\nDone. %d stored, %d skipped", totalStored, totalSkipped)
					if totalErrors > 0 {
						fmt.Printf(", %d scrape errors", totalErrors)
					}
					fmt.Println()
				}
			}
		},
	}
	refreshCmd.Flags().StringVar(&fromStr, "from", "", "Start of time window (e.g., 'now', '2025-01-01', '7d')")
	refreshCmd.Flags().StringVar(&toStr, "to", "", "End of time window (e.g., 'now', '2025-01-01', '1h')")
	refreshCmd.Flags().StringVar(&betweenStr, "between", "", "Filter articles between two dates/durations (e.g., '7d,1d', '2025-01-01,2025-01-07')")
	refreshCmd.Flags().BoolVar(&overwrite, "overwrite", false, "Overwrite existing articles instead of skipping them")
	refreshCmd.Flags().BoolVar(&restore, "restore", false, "Overwrite existing articles that have no content")
	refreshCmd.Flags().BoolVar(&refreshDryRun, "dry-run", false, "Preview matching articles without refreshing")
	refreshCmd.Flags().BoolVar(&refreshDebug, "debug", false, "With --dry-run: show first and last 10 lines of each article's content")
	refreshCmd.Flags().IntVar(&lastN, "last-n", 0, "Keep only the N most-recently-published articles (mutually exclusive with --from/--to/--between)")

	// Apply feeds command
	applyCmd := &cobra.Command{
		Use:   "apply",
		Short: "Reconcile feeds from a file",
		Long: `Reconcile the database to match a feeds YAML file: feeds in the file are
created or updated, and feeds no longer present are disabled (their articles are kept).`,
		Run: func(cmd *cobra.Command, args []string) {
			ff, err := loadFeedsFile(applyFile)
			if err != nil {
				fmt.Printf("%s %v\n", styleErr.Render("✗"), err)
				return
			}

			client := getNewDownlinkClient()
			res, err := client.ApplyFeeds(ff.Feeds, ff.DefaultSelectors, applyDryRun)
			if err != nil {
				fmt.Printf("Failed to apply feeds: %v\n", err)
				return
			}

			if jsonOutput {
				out, _ := json.MarshalIndent(map[string][]string{
					"created": res.Created, "updated": res.Updated, "disabled": res.Disabled,
				}, "", "  ")
				fmt.Println(string(out))
				return
			}

			if applyDryRun {
				fmt.Println(styleWarn.Render("DRY RUN: no changes applied"))
			}
			printFeedActionList("Created", res.Created)
			printFeedActionList("Updated", res.Updated)
			printFeedActionList("Disabled", res.Disabled)
			if len(res.Created)+len(res.Updated)+len(res.Disabled) == 0 {
				fmt.Println(styleDim.Render("Nothing to do; feeds already in sync."))
			}
		},
	}
	applyCmd.Flags().StringVarP(&applyFile, "file", "f", "", "Path to feeds YAML file (required)")
	applyCmd.Flags().BoolVar(&applyDryRun, "dry-run", false, "Show what would change without applying")
	_ = applyCmd.MarkFlagRequired("file")

	// Delete feed command
	deleteCmd := &cobra.Command{
		Use:     "delete",
		Aliases: []string{"del", "rm"},
		Short:   "Delete feeds",
		Long: `Delete feeds by file (-f), title (-t), or id (-i). The selectors are
mutually exclusive; with none, pick a feed interactively. -t deletes every feed
matching the title. Deleting a feed also removes its articles.`,
		Run: func(cmd *cobra.Command, args []string) {
			selectors := 0
			for _, s := range []string{deleteFile, deleteTitle, deleteId} {
				if s != "" {
					selectors++
				}
			}
			if selectors > 1 {
				fmt.Println("Specify only one of --file, --title, or --id.")
				return
			}

			client := getNewDownlinkClient()

			ids, labels, err := resolveDeleteTargets(client)
			if err != nil {
				fmt.Printf("%s %v\n", styleErr.Render("✗"), err)
				return
			}
			if len(ids) == 0 {
				fmt.Println("No matching feeds to delete.")
				return
			}

			if deleteDryRun {
				fmt.Println(styleWarn.Render("DRY RUN: would delete:"))
				for _, l := range labels {
					fmt.Printf("  - %s\n", l)
				}
				return
			}

			fmt.Println("About to delete:")
			for _, l := range labels {
				fmt.Printf("  - %s\n", l)
			}
			confirm := false
			flushStdin()
			if err := huh.NewConfirm().
				Title(fmt.Sprintf("Delete %d feed(s) and their articles?", len(ids))).
				Affirmative("Yes, delete").
				Negative("No, keep them").
				Value(&confirm).
				Run(); err != nil || !confirm {
				fmt.Println("Cancelled.")
				return
			}

			res, err := client.DeleteFeeds(ids, false)
			if err != nil {
				fmt.Printf("Failed to delete feeds: %v\n", err)
				return
			}
			fmt.Printf("%s Deleted %d feed(s)\n", styleOK.Render("✓"), len(res.Deleted))
			for _, nf := range res.NotFound {
				fmt.Printf("  %s\n", styleDim.Render("not found: "+nf))
			}
		},
	}
	deleteCmd.Flags().StringVarP(&deleteFile, "file", "f", "", "Delete the feeds defined in this YAML file")
	deleteCmd.Flags().StringVarP(&deleteTitle, "title", "t", "", "Delete all feeds with this title")
	deleteCmd.Flags().StringVarP(&deleteId, "id", "i", "", "Delete the feed with this id")
	deleteCmd.Flags().BoolVar(&deleteDryRun, "dry-run", false, "Show what would be deleted without deleting")

	exportCmd := &cobra.Command{
		Use:   "export",
		Short: "Export feeds from DB to YAML",
		Long:  `Generate a feeds.yml from the feeds currently stored in the database.`,
		Run: func(cmd *cobra.Command, args []string) {
			client := getNewDownlinkClient()
			feeds, err := client.ListFeeds()
			if err != nil {
				fmt.Printf("Failed to list feeds: %v\n", err)
				return
			}

			var configs []models.FeedConfig
			for _, f := range feeds {
				if exportEnabledOnly && (f.Enabled == nil || !*f.Enabled) {
					continue
				}
				enabled := f.Enabled != nil && *f.Enabled
				configs = append(configs, models.FeedConfig{
					URL:     f.URL,
					Title:   f.Title,
					Type:    f.Type,
					Enabled: enabled,
					Scraper: map[string]any(f.Scraper),
				})
			}

			ff := models.FeedsFile{Feeds: configs}
			out, err := yaml.Marshal(&ff)
			if err != nil {
				fmt.Printf("Failed to marshal YAML: %v\n", err)
				return
			}

			if exportOutput == "" || exportOutput == "-" {
				fmt.Print(string(out))
				return
			}
			if err := os.WriteFile(exportOutput, out, 0644); err != nil {
				fmt.Printf("Failed to write %s: %v\n", exportOutput, err)
				return
			}
			fmt.Printf("%s Written to %s\n", styleOK.Render("✓"), exportOutput)
		},
	}
	exportCmd.Flags().StringVarP(&exportOutput, "output", "o", "", "Write to file instead of stdout (\"-\" for stdout)")
	exportCmd.Flags().BoolVar(&exportEnabledOnly, "enabled-only", false, "Only export enabled feeds")

	// Diagnose feed command
	diagnoseCmd := &cobra.Command{
		Use:   "diagnose [feed-id-or-name]",
		Short: "Diagnose why a feed fails to fetch or parse",
		Long: `Fetch a single feed and report what actually came back over the wire.

This is a read-only probe: it stores no articles and does not update the feed's
last-fetch time. Use it to debug feeds that error during "feeds refresh all" with
messages like "Failed to detect feed type" or "invalid utf-8 syntax".

It reports the HTTP status, content type, a guess at what the body is (RSS / Atom /
JSON feed / HTML / empty), any parse error, the exact byte offset of invalid UTF-8
(with a hex dump), and the on-disk path to the saved raw body.

Examples:
  downlink-cli feeds diagnose "HYAS Blog"
  downlink-cli feeds diagnose hyas-blog --raw   # also print the saved raw body path/bytes`,
		Args: cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			client := getNewDownlinkClient()

			feed, feeds, err := findFeedByIDOrNormalizedName(client, args[0])
			if err != nil && feeds == nil {
				fmt.Printf("Failed to list feeds: %v\n", err)
				return
			}
			if err != nil {
				fmt.Println(err)
				printAvailableFeeds(feeds)
				return
			}

			diag, err := client.DiagnoseFeed(feed.Id)
			if err != nil {
				fmt.Printf("%s Diagnose failed: %v\n", styleErr.Render("✗"), err)
				return
			}
			if diag == nil {
				fmt.Println("No diagnosis returned by server.")
				return
			}

			if jsonOutput {
				out, err := json.MarshalIndent(diag, "", "  ")
				if err != nil {
					fmt.Printf("Error marshalling to JSON: %v\n", err)
					return
				}
				fmt.Println(string(out))
				return
			}

			printDiagnosis(feed.Title, diag, diagnoseRaw)
		},
	}
	diagnoseCmd.Flags().BoolVar(&diagnoseRaw, "raw", false, "Also print the saved raw body path and a preview")

	cmd.AddCommand(listCmd, addCmd, refreshCmd, applyCmd, deleteCmd, exportCmd, diagnoseCmd)
	cmd.AddCommand(createFeedBuildCommands()...)
	return cmd
}

// printDiagnosis renders a FeedDiagnosis as a human-readable verdict block.
func printDiagnosis(title string, d *protos.FeedDiagnosis, showRaw bool) {
	// The verdict is the headline: green when the feed is valid, red/amber otherwise.
	verdictStyle := styleErr
	switch {
	case d.ParseError == "" && d.HttpStatus < 400 && d.FeedTypeGuess != "html" && d.FeedTypeGuess != "empty":
		verdictStyle = styleOK
	case d.FeedTypeGuess == "html", d.FeedTypeGuess == "empty":
		verdictStyle = styleWarn
	}

	fmt.Printf("\n%s %s\n", styleBold.Render("Diagnosis:"), title)
	fmt.Printf("  %s %s\n", styleKey.Render("Verdict "), verdictStyle.Render(d.Verdict))

	row := func(k, v string) {
		if v == "" {
			return
		}
		fmt.Printf("  %s %s\n", styleKey.Render(k), v)
	}
	row("URL     ", d.Url)
	if d.FinalUrl != "" && d.FinalUrl != d.Url {
		row("Final   ", d.FinalUrl+styleDim.Render("  (redirected)"))
	}
	status := fmt.Sprintf("%d", d.HttpStatus)
	if d.HttpStatus == 0 {
		status = styleDim.Render("(no response)")
	}
	row("HTTP    ", status)
	row("Type    ", fmt.Sprintf("%s  %s", d.ContentType, styleDim.Render(fmt.Sprintf("(looks like: %s)", d.FeedTypeGuess))))
	row("Size    ", fmt.Sprintf("%d bytes  %s", d.ContentLength, styleDim.Render(fmt.Sprintf("in %dms", d.FetchDurationMs))))
	if d.DeclaredCharset != "" {
		row("Charset ", d.DeclaredCharset)
	}
	if d.ItemCount > 0 {
		row("Items   ", fmt.Sprintf("%d", d.ItemCount))
	}
	if d.ParseError != "" {
		row("Parse   ", styleErr.Render(d.ParseError))
	}
	if d.InvalidUtf8At >= 0 {
		row("UTF-8   ", styleErr.Render(fmt.Sprintf("first invalid byte at offset %d", d.InvalidUtf8At)))
		if d.HexDump != "" {
			fmt.Printf("%s\n", styleDim.Render(indent(d.HexDump, "    ")))
		}
	}
	if d.BodySnippet != "" {
		row("Body    ", styleDim.Render(truncate(d.BodySnippet, 120)))
	}
	if d.RawBodyPath != "" {
		row("Raw body", d.RawBodyPath)
		if showRaw {
			if data, err := os.ReadFile(d.RawBodyPath); err == nil {
				fmt.Printf("\n%s\n", styleSection.Render("── raw body preview ──"))
				fmt.Println(string(truncateBytes(data, 2000)))
			}
		}
	}
	fmt.Println()
}

// indent prefixes every line of s with pad.
func indent(s, pad string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = pad + l
	}
	return strings.Join(lines, "\n")
}

// truncateBytes returns the first n bytes of b, appending a notice when cut.
func truncateBytes(b []byte, n int) []byte {
	if len(b) <= n {
		return b
	}
	return append(b[:n:n], []byte("\n… (truncated)")...)
}

// loadFeedsFile reads and parses a feeds YAML file (same format as feeds.yml).
func loadFeedsFile(path string) (*models.FeedsFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read feeds file: %w", err)
	}
	var ff models.FeedsFile
	if err := yaml.Unmarshal(data, &ff); err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", path, err)
	}
	return &ff, nil
}

// resolveDeleteTargets turns the active delete selector (--id/--title/--file, or
// the interactive picker) into the feed ids to delete plus display labels.
func resolveDeleteTargets(client *downlinkclient.DownlinkClient) (ids []string, labels []string, err error) {
	switch {
	case deleteId != "":
		return []string{deleteId}, []string{deleteId}, nil

	case deleteTitle != "":
		feeds, err := client.ListFeeds()
		if err != nil {
			return nil, nil, err
		}
		for _, f := range feeds {
			if f.Title == deleteTitle {
				ids = append(ids, f.Id)
				labels = append(labels, feedDisplay(f))
			}
		}
		if len(ids) == 0 {
			return nil, nil, fmt.Errorf("no feeds found with title %q", deleteTitle)
		}
		return ids, labels, nil

	case deleteFile != "":
		ff, err := loadFeedsFile(deleteFile)
		if err != nil {
			return nil, nil, err
		}
		feeds, err := client.ListFeeds()
		if err != nil {
			return nil, nil, err
		}
		byURL := make(map[string]models.Feed, len(feeds))
		for _, f := range feeds {
			byURL[f.URL] = f
		}
		for _, fc := range ff.Feeds {
			if f, ok := byURL[fc.URL]; ok {
				ids = append(ids, f.Id)
				labels = append(labels, feedDisplay(f))
			} else {
				fmt.Printf("  %s\n", styleDim.Render("not in DB, skipping: "+fc.URL))
			}
		}
		return ids, labels, nil

	default:
		feed, err := selectFeed(client)
		if err != nil {
			return nil, nil, err
		}
		if feed.Id == "" {
			return nil, nil, nil // cancelled
		}
		return []string{feed.Id}, []string{feedDisplay(feed)}, nil
	}
}

// feedDisplay renders a feed for human-readable CLI output.
func feedDisplay(f models.Feed) string {
	if f.Title == "" {
		return f.URL
	}
	return fmt.Sprintf("%s (%s)", f.Title, f.URL)
}

// printFeedActionList prints a labelled group of feed actions, if non-empty.
func printFeedActionList(label string, items []string) {
	if len(items) == 0 {
		return
	}
	fmt.Println(styleSection.Render(fmt.Sprintf("%s (%d)", label, len(items))))
	for _, it := range items {
		fmt.Printf("  - %s\n", it)
	}
}

func runAddFeedInteractive(client *downlinkclient.DownlinkClient) {
	var url, name, feedType, scraping string
	feedType = "rss"

	flushStdin()
	if err := huh.NewInput().
		Title("Feed URL").
		Placeholder("https://example.com/feed.xml").
		Value(&url).
		Validate(func(s string) error {
			if strings.TrimSpace(s) == "" {
				return fmt.Errorf("URL is required")
			}
			return nil
		}).
		Run(); err != nil {
		fmt.Println("Cancelled.")
		return
	}

	flushStdin()
	if err := huh.NewInput().
		Title("Feed name").
		Description("Leave empty to auto-detect from the feed").
		Value(&name).
		Run(); err != nil {
		fmt.Println("Cancelled.")
		return
	}

	flushStdin()
	if err := huh.NewSelect[string]().
		Title("Feed type").
		Options(
			huh.NewOption("RSS", "rss"),
			huh.NewOption("Atom", "atom"),
		).
		Value(&feedType).
		Run(); err != nil {
		fmt.Println("Cancelled.")
		return
	}

	flushStdin()
	if err := huh.NewSelect[string]().
		Title("Scraping mode").
		Options(
			huh.NewOption("Static (default)", ""),
			huh.NewOption("Dynamic (headless JS)", "dynamic"),
			huh.NewOption("Full browser", "full_browser"),
			huh.NewOption("None (use feed content, no fetch)", "none"),
		).
		Value(&scraping).
		Run(); err != nil {
		fmt.Println("Cancelled.")
		return
	}

	cfg := models.FeedConfig{
		URL:      strings.TrimSpace(url),
		Title:    strings.TrimSpace(name),
		Type:     feedType,
		Enabled:  true,
		Scraping: scraping,
	}

	if err := client.RegisterFeed(cfg); err != nil {
		fmt.Printf("Failed to register feed: %v\n", err)
		return
	}
	fmt.Println(styleOK.Render("✓") + " Feed registered successfully")
}
