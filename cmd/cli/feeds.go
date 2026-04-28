package main

import (
	"downlink/pkg/downlinkclient"
	"downlink/pkg/models"
	"downlink/pkg/protos"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/spf13/cobra"
)

// ── live refresh display ──────────────────────────────────────────────────────

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

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
func refreshAllFeedsWithWindow(client *downlinkclient.DownlinkClient, fromTime, toTime *time.Time, overwrite, restore bool) error {
	feeds, err := client.ListFeeds()
	if err != nil {
		return fmt.Errorf("failed to list feeds: %w", err)
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
			resp, err := client.RefreshFeedWithTimeWindow(feed.Id, fromTime, toTime, overwrite, restore)
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
				fmt.Printf("Found %d feeds:\n", len(feeds))
				for _, feed := range feeds {
					spew.Dump(feed)
				}
			}
		},
	}

	// Add feed command
	addCmd := &cobra.Command{
		Use:   "add",
		Short: "Register a new feed",
		Long:  `Add a new feed to be monitored.`,
		Run: func(cmd *cobra.Command, args []string) {
			client := getNewDownlinkClient()

			if feedURL == "" {
				fmt.Println("Error: Feed URL is required")
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

			err := client.RegisterFeed(feedConfig)
			if err != nil {
				fmt.Printf("Failed to register feed: %v\n", err)
				return
			}

			fmt.Println("Feed registered successfully")
		},
	}

	// Add flags for add command
	addCmd.Flags().StringVar(&feedURL, "url", "", "Feed URL (required)")
	addCmd.Flags().StringVar(&feedName, "name", "", "Feed name (optional, will be auto-detected if empty)")
	addCmd.Flags().StringVar(&feedCategory, "type", "rss", "Feed type (e.g. rss)")
	addCmd.Flags().StringVar(&feedScraping, "scraping", "", `Scraping mode: "dynamic" or "full_browser" (default: static)`)
	addCmd.Flags().StringVar(&feedArticleSelector, "selector-article", "", "CSS selector for article content")
	addCmd.Flags().StringVar(&feedCutoffSelector, "selector-cutoff", "", "CSS selector where extraction stops")
	addCmd.Flags().StringVar(&feedBlacklistSelector, "selector-blacklist", "", "CSS selector for elements to exclude")
	addCmd.MarkFlagRequired("url")

	// Refresh feeds command
	var fromStr, toStr, betweenStr string
	var overwrite, restore, refreshDryRun bool
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
  downlink-cli feeds refresh                          # Refresh all feeds
  downlink-cli feeds refresh all                      # Refresh all feeds explicitly
  downlink-cli feeds refresh tech-news --from -7d     # Articles from last 7 days
  downlink-cli feeds refresh "My Feed" --from 2025-01-01  # Articles from Jan 1, 2025
  downlink-cli feeds refresh feed-123 --from -1d --to now # Articles from last 24 hours`,
		Args: cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			fromTime, toTime, err := parseTimeWindow(fromStr, toStr, betweenStr, nil)
			if err != nil {
				fmt.Println(err)
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
						}
					}

					fmt.Println("  (no actual refresh performed)")
					return
				}

				resp, err := client.RefreshFeedWithTimeWindow(feedId, fromTime, toTime, overwrite, restore)
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
			} else {
				// Refresh all feeds
				// If time window filtering is requested, refresh each feed individually with the filter
				if fromTime != nil || toTime != nil {
					// Dry-run mode: just list feeds that would be refreshed
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

					if err := refreshAllFeedsWithWindow(client, fromTime, toTime, overwrite, restore); err != nil {
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
	refreshCmd.Flags().StringVar(&fromStr, "from", "", "Start of time window (e.g., 'now', '2025-01-01', '-7d')")
	refreshCmd.Flags().StringVar(&toStr, "to", "", "End of time window (e.g., 'now', '2025-01-01', '-1h')")
	refreshCmd.Flags().StringVar(&betweenStr, "between", "", "Filter articles between two dates/durations (e.g., '-7d,-1d', '2025-01-01,2025-01-07')")
	refreshCmd.Flags().BoolVar(&overwrite, "overwrite", false, "Overwrite existing articles instead of skipping them")
	refreshCmd.Flags().BoolVar(&restore, "restore", false, "Overwrite existing articles that have no content")
	refreshCmd.Flags().BoolVar(&refreshDryRun, "dry-run", false, "Preview matching articles without refreshing")

	// Delete feed command
	deleteCmd := &cobra.Command{
		Use:     "delete [id]",
		Aliases: []string{"del", "rm"},
		Short:   "Delete a feed",
		Long:    `Remove a feed from being monitored.`,
		Args:    cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			client := getNewDownlinkClient()

			feedId := args[0]
			err := client.DeleteFeed(feedId)
			if err != nil {
				fmt.Printf("Failed to delete feed: %v\n", err)
				return
			}

			fmt.Printf("Feed %s deleted successfully\n", feedId)
		},
	}

	cmd.AddCommand(listCmd, addCmd, refreshCmd, deleteCmd)
	return cmd
}
