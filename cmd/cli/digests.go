package main

import (
	"context"
	"downlink/pkg/digestthemes"
	"downlink/pkg/downlinkclient"
	"downlink/pkg/models"
	"downlink/pkg/protos"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/spf13/cobra"
)

var (

	// Digest flags
	digestLimit int
)

// Digest commands
func createDigestCommands() *cobra.Command {
	var listThemesFlag bool
	cmd := &cobra.Command{
		Use:   "digest",
		Short: "Manage article digests",
		Long:  `Create and view article digests.`,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			if listThemesFlag {
				fmt.Println("Available themes:")
				for _, t := range digestthemes.All() {
					fmt.Printf("  %-12s %s\n", t.Name, t.Description)
				}
				os.Exit(0)
			}
		},
	}
	cmd.PersistentFlags().BoolVar(&listThemesFlag, "list-themes", false, "List available HTML themes and exit")

	// List digests command
	listCmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List digests",
		Long:    `View all available digests.`,
		Run: func(cmd *cobra.Command, args []string) {
			client := getNewDownlinkClient()

			digests, err := client.ListDigests(digestLimit)
			if err != nil {
				fmt.Printf("Failed to list digests: %v\n", err)
				return
			}

			if len(digests) == 0 {
				fmt.Println("No digests found")
				return
			}

			if jsonOutput {
				out, err := json.MarshalIndent(digests, "", "  ")
				if err != nil {
					fmt.Printf("Error marshalling to JSON: %v\n", err)
					return
				}
				fmt.Println(string(out))
			} else {
				fmt.Printf("Found %d digests:\n", len(digests))
				for _, digest := range digests {
					spew.Dump(digest)
				}
			}
		},
	}

	// Add flags for list command
	listCmd.Flags().IntVar(&digestLimit, "limit", 10, "Maximum number of digests to return")

	// Get digest command
	getCmd := &cobra.Command{
		Use:   "get [id]",
		Short: "Get digest details",
		Long:  `View details of a specific digest.`,
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			client := getNewDownlinkClient()

			digestId := args[0]
			digest, err := client.GetDigest(digestId)
			if err != nil {
				fmt.Printf("Failed to get digest: %v\n", err)
				return
			}

			if jsonOutput {
				out, err := json.MarshalIndent(digest, "", "  ")
				if err != nil {
					fmt.Printf("Error marshalling to JSON: %v\n", err)
					return
				}
				fmt.Println(string(out))
			} else {
				fmt.Println("Digest Details:")
				spew.Dump(digest)

				// Get articles for this digest
				articles, err := client.GetDigestArticles(digestId)
				if err != nil {
					fmt.Printf("Failed to get digest articles: %v\n", err)
					return
				}

				if len(articles) > 0 {
					fmt.Println("\nArticles in this digest:")
					for i, article := range articles {
						fmt.Printf("%d. %s\n", i+1, article.Title)
					}
				} else {
					fmt.Println("\nNo articles in this digest.")
				}
			}
		},
	}

	// Generate digest command
	var digestFrom, digestTo, digestBetween, digestTheme, digestTestID string
	var digestDryRun, digestRefreshFeeds, digestTest bool
	generateCmd := &cobra.Command{
		Use:   "generate",
		Short: "Generate a new digest",
		Long: `Create a new digest from recent articles.

Examples:
  downlink-cli digest generate                      # Last 24 hours (default)
  downlink-cli digest generate --from -7d           # Last 7 days
  downlink-cli digest generate --from -2h           # Last 2 hours
  downlink-cli digest generate --from 2025-01-01    # From specific date
  downlink-cli digest generate --from -7d --to -1d  # Between 7 days and 1 day ago`,
		Run: func(cmd *cobra.Command, args []string) {
			client := getNewDownlinkClient()

			if digestTest {
				if !digestthemes.Valid(digestTheme) {
					fmt.Printf("Unknown theme %q. Run 'digest --list-themes' to see available themes.\n", digestTheme)
					return
				}

				prog := newBatchProgress()
				prog.addHiddenRow("notify", "sending test digest")
				prog.startSpinner()

				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				sigCh := make(chan os.Signal, 2)
				signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
				go func() {
					select {
					case <-ctx.Done():
						return
					case <-sigCh:
						prog.updateRow("cancel", "cancelling — waiting for server confirmation...")
						cancel()
						select {
						case <-sigCh:
							os.Exit(130)
						case <-ctx.Done():
						}
					}
				}()
				defer signal.Stop(sigCh)

				handler := newDigestProgressHandler(prog)
				digest, err := client.GenerateDigestWithOptions(ctx, downlinkclient.GenerateDigestOptions{
					StartTime:    time.Now(),
					EndTime:      time.Now(),
					Theme:        digestTheme,
					Test:         true,
					TestDigestID: digestTestID,
					OnEvent:      handler,
				})

				prog.stop()

				if err != nil {
					if ctx.Err() != nil {
						fmt.Println("\nDigest notification test cancelled (confirmed by server).")
						return
					}
					fmt.Printf("Failed to send test digest: %v\n", err)
					return
				}

				fmt.Printf("\nTest digest sent: %s\n", digest.Id)
				articleCount := 0
				if digest.ArticleCount != nil {
					articleCount = *digest.ArticleCount
				}
				fmt.Printf("Contains %d articles\n", articleCount)
				return
			}

			defaultFrom := time.Now().Add(-24 * time.Hour)
			fromTime, toTime, err := parseTimeWindow(digestFrom, digestTo, digestBetween, &defaultFrom)
			if err != nil {
				fmt.Println(err)
				return
			}

			// Calculate hours from time window
			var toTimeVal time.Time
			if toTime != nil {
				toTimeVal = *toTime
			} else {
				toTimeVal = time.Now()
			}

			hours := int(toTimeVal.Sub(*fromTime).Hours())
			if hours < 1 {
				hours = 1 // Minimum 1 hour
			}

			// Refresh feeds over the same window before generating the digest, when
			// requested. Skipped in dry-run to keep that mode side-effect-free.
			if digestRefreshFeeds {
				if digestDryRun {
					fmt.Println("(skipping feed refresh in --dry-run mode)")
				} else {
					fmt.Printf("Refreshing feeds from %s to %s...\n",
						fromTime.Format("2006-01-02 15:04:05"), toTimeVal.Format("2006-01-02 15:04:05"))
					if err := refreshAllFeedsWithWindow(client, fromTime, &toTimeVal, false, false); err != nil {
						fmt.Println(err)
						return
					}
					fmt.Println()
				}
			}

			// Dry-run mode: just list articles that would be included
			if digestDryRun {
				excludeDigested, _ := cmd.Flags().GetBool("exclude-digested")
				filter := models.ArticleFilter{
					StartDate:       fromTime,
					EndDate:         &toTimeVal,
					ExcludeDigested: excludeDigested,
				}
				articles, err := client.ListArticles(filter)
				if err != nil {
					fmt.Printf("Failed to list articles: %v\n", err)
					return
				}

				fmt.Printf("Would generate digest with %d articles from %s to %s:\n\n",
					len(articles), fromTime.Format("2006-01-02 15:04:05"), toTimeVal.Format("2006-01-02 15:04:05"))
				if len(articles) == 0 {
					fmt.Println("No articles found in this time window.")
				} else {
					for i, article := range articles {
						title := article.Title
						published := ""
						if !article.PublishedAt.IsZero() {
							published = article.PublishedAt.Format("2006-01-02")
						}
						fmt.Printf("%d. %-12s %-10s %s\n", i+1, article.Id[34:], published, title)
					}
					if len(articles) >= 100 {
						fmt.Println("\nNote: Showing first 100 article")
					}
				}
				fmt.Println("\n(no digest generated)")
				return
			}

			if !digestthemes.Valid(digestTheme) {
				fmt.Printf("Unknown theme %q. Run 'digest --list-themes' to see available themes.\n", digestTheme)
				return
			}

			skipAnalysis, _ := cmd.Flags().GetBool("skip-analysis")
			skipDuplicates, _ := cmd.Flags().GetBool("skip-duplicates")
			skipSummary, _ := cmd.Flags().GetBool("skip-summary")
			excludeDigested, _ := cmd.Flags().GetBool("exclude-digested")
			oneShotAnalysis, _ := cmd.Flags().GetBool("one-shot")

			if skipLLM, _ := cmd.Flags().GetBool("skip-llm"); skipLLM {
				skipAnalysis = true
				skipDuplicates = true
				skipSummary = true
			}

			prog := newBatchProgress()
			prog.addRow("fetch", "fetching articles")
			if !skipAnalysis {
				prog.addRow("analyze", "analyzing articles")
			}
			if !skipDuplicates {
				prog.addHiddenRow("dedupe", "deduplicating")
			}
			if !skipSummary {
				prog.addHiddenRow("summarize", "generating summary")
			}
			prog.addHiddenRow("store", "storing digest")
			prog.addHiddenRow("notify", "sending notification")
			prog.startSpinner()

			// Allow the user to cancel digest generation with Ctrl-C. The first
			// SIGINT signals the server to stop at the next stage boundary and
			// waits for its "cancelled" confirmation before exiting. A second
			// SIGINT forces an immediate exit.
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			sigCh := make(chan os.Signal, 2)
			signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
			go func() {
				select {
				case <-ctx.Done():
					return
				case <-sigCh:
					prog.updateRow("cancel", "cancelling — waiting for server confirmation...")
					cancel()
					select {
					case <-sigCh:
						os.Exit(130)
					case <-ctx.Done():
					}
				}
			}()
			defer signal.Stop(sigCh)

			handler := newDigestProgressHandler(prog)
			digest, err := client.GenerateDigestWithOptions(ctx, downlinkclient.GenerateDigestOptions{
				StartTime:       *fromTime,
				EndTime:         toTimeVal,
				SkipAnalysis:    skipAnalysis,
				SkipDuplicates:  skipDuplicates,
				ExcludeDigested: excludeDigested,
				SkipSummary:     skipSummary,
				Theme:           digestTheme,
				OneShotAnalysis: oneShotAnalysis,
				OnEvent:         handler,
			})

			prog.stop()

			if err != nil {
				if ctx.Err() != nil {
					fmt.Println("\nDigest generation cancelled (confirmed by server).")
					return
				}
				fmt.Printf("Failed to generate digest: %v\n", err)
				return
			}

			fmt.Printf("\nNew digest generated: %s\n", digest.Id)
			articleCount := 0
			if digest.ArticleCount != nil {
				articleCount = *digest.ArticleCount
			}
			fmt.Printf("Contains %d articles from %s to %s\n",
				articleCount, fromTime.Format("2006-01-02 15:04:05"), toTimeVal.Format("2006-01-02 15:04:05"))
		},
	}

	// Add flags for generate command
	generateCmd.Flags().StringVar(&digestFrom, "from", "", "Start of time window (e.g., 'now', '2025-01-01', '-24h' — default: -24h)")
	generateCmd.Flags().StringVar(&digestTo, "to", "", "End of time window (e.g., 'now', '2025-01-01', '-1h')")
	generateCmd.Flags().StringVar(&digestBetween, "between", "", "Filter articles between two dates/durations (e.g., '-7d,-1d', '2025-01-01,2025-01-07')")
	generateCmd.Flags().BoolVar(&digestDryRun, "dry-run", false, "List matching articles without generating digest")
	generateCmd.Flags().BoolVar(&digestRefreshFeeds, "refresh-feeds", false, "Refresh all feeds over the same time window before generating the digest")
	generateCmd.Flags().Bool("skip-llm", false, "Skip all LLM usage (analysis, duplicate detection, and summary)")
	generateCmd.Flags().Bool("skip-analysis", false, "Skip LLM-based article analysis")
	generateCmd.Flags().Bool("skip-duplicates", false, "Skip LLM-based duplicate detection")
	generateCmd.Flags().Bool("skip-summary", false, "Skip LLM-based digest summary generation")
	generateCmd.Flags().Bool("one-shot", false, "Analyze missing articles with one full LLM prompt instead of the multi-step chain")
	generateCmd.Flags().Bool("exclude-digested", false, "Exclude articles already included in a previous digest")
	generateCmd.Flags().StringVar(&digestTheme, "theme", "dark", "HTML theme for the digest (see: digest --list-themes)")
	generateCmd.Flags().BoolVar(&digestTest, "test", false, "Send a stored test digest to configured notification channels without generating a new digest")
	generateCmd.Flags().StringVar(&digestTestID, "test-digest-id", "", "Digest ID to use with --test (default: server-selected rich test digest)")

	// Get digest articles command
	articlesCmd := &cobra.Command{
		Use:   "articles [digest-id]",
		Short: "List articles in digest",
		Long:  `View all articles included in a specific digest.`,
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			client := getNewDownlinkClient()

			digestId := args[0]
			articles, err := client.GetDigestArticles(digestId)
			if err != nil {
				fmt.Printf("Failed to get digest articles: %v\n", err)
				return
			}

			if len(articles) == 0 {
				fmt.Println("No articles in this digest.")
				return
			}

			if jsonOutput {
				out, err := json.MarshalIndent(articles, "", "  ")
				if err != nil {
					fmt.Printf("Error marshalling to JSON: %v\n", err)
					return
				}
				fmt.Println(string(out))
			} else {
				fmt.Printf("Found %d articles in digest %s:\n", len(articles), digestId)

				for _, article := range articles {
					spew.Dump(article)
				}
			}
		},
	}

	cmd.AddCommand(listCmd, getCmd, generateCmd, articlesCmd)
	return cmd
}

// newDigestProgressHandler returns a stateful event handler that maps server-sent
// DigestProgressEvents to batchProgress rows. With parallel article analysis,
// each article gets its own row keyed by article_id; task events update that
// row with the currently-running task. The "analyze" header row summarizes
// total progress across all in-flight articles.
func newDigestProgressHandler(prog *batchProgress) func(*protos.DigestProgressEvent) {
	var (
		total     uint32
		started   uint32
		completed uint32
		// Ordered list of article IDs as they begin, so new article rows
		// are inserted in deterministic order beneath the "analyze" header.
		articleOrder []string
		articleSeen  = map[string]bool{}
	)

	updateAnalyzeHeader := func() {
		var label string
		if total > 0 {
			inFlight := started - completed
			label = fmt.Sprintf("analyzing [%d/%d, %d in parallel]", completed, total, inFlight)
		} else {
			label = "analyzing articles"
		}
		prog.updateRow("analyze", label)
	}

	articleRowId := func(articleId string) string { return "analyze:art:" + articleId }
	// Row format: "      └ <title> · <task> (N/M)" (~8 chars of indent/glyph).
	// Renderer caps labels at 100 chars; reserve ~22 for the task suffix.
	shortTitle := func(t string) string {
		t = strings.TrimSpace(t)
		if t == "" {
			return "(untitled)"
		}
		const maxTitle = 70
		if len(t) > maxTitle {
			t = t[:maxTitle-1] + "…"
		}
		return t
	}

	return func(ev *protos.DigestProgressEvent) {
		switch ev.Stage {
		case "fetch":
			if ev.Total > 0 {
				prog.completeRow("fetch", true, fmt.Sprintf("%d articles", ev.Total))
			} else {
				prog.updateRow("fetch", "fetching articles...")
			}
		case "analyze":
			if ev.Total > 0 {
				total = ev.Total
			}
			if ev.ArticleId != "" && !articleSeen[ev.ArticleId] {
				articleSeen[ev.ArticleId] = true
				articleOrder = append(articleOrder, ev.ArticleId)
				started++
				// Insert the article's row right after "analyze" (or after the
				// last article row we inserted, to preserve start-order).
				afterKey := "analyze"
				if len(articleOrder) > 1 {
					afterKey = articleRowId(articleOrder[len(articleOrder)-2])
				}
				prog.insertRowAfter(afterKey, articleRowId(ev.ArticleId),
					fmt.Sprintf("      └ %s", shortTitle(ev.ArticleTitle)))
			}
			updateAnalyzeHeader()
		case "analyze_task":
			if ev.ArticleId == "" {
				return
			}
			rowId := articleRowId(ev.ArticleId)
			title := shortTitle(ev.ArticleTitle)
			switch ev.TaskStatus {
			case "started":
				prog.updateRow(rowId,
					fmt.Sprintf("      └ %s · %s (%d/%d)", title, ev.TaskName, ev.TaskIndex, ev.TaskTotal))
			case "completed":
				if ev.TaskIndex == ev.TaskTotal {
					// Last task for this article: mark the row done.
					prog.updateRow(rowId, fmt.Sprintf("      └ %s", title))
					prog.completeRow(rowId, true, "")
					completed++
					updateAnalyzeHeader()
					if completed == total && total > 0 {
						prog.completeRow("analyze", true, fmt.Sprintf("%d articles", total))
					}
				} else {
					prog.updateRow(rowId,
						fmt.Sprintf("      └ %s · %s done", title, ev.TaskName))
				}
			case "error":
				prog.completeRow(rowId, false, fmt.Sprintf("%s: %s", title, ev.Error))
				completed++
				updateAnalyzeHeader()
			}
		case "dedupe":
			prog.showRow("dedupe")
			if ev.Message != "" && ev.Message != "identifying duplicate articles..." {
				prog.completeRow("dedupe", true, ev.Message)
			}
		case "summarize":
			prog.showRow("summarize")
			if ev.Message == "digest summary generated" {
				prog.completeRow("summarize", true, "done")
			}
		case "store":
			prog.showRow("store")
		case "notify":
			prog.showRow("notify")
		case "done":
			for _, id := range []string{"fetch", "analyze", "dedupe", "summarize", "store", "notify"} {
				prog.completeRow(id, true, "")
			}
		case "error":
			prog.completeRow("fetch", false, ev.Error)
			prog.completeRow("analyze", false, ev.Error)
			prog.completeRow("dedupe", false, ev.Error)
			prog.completeRow("summarize", false, ev.Error)
			prog.completeRow("store", false, ev.Error)
			prog.completeRow("notify", false, ev.Error)
		}
	}
}
