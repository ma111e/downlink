package main

import (
	"fmt"
	"os"
	"strings"
	"syscall"
	"time"

	"charm.land/huh/v2"
	"downlink/pkg/downlinkclient"
	"downlink/pkg/models"
	"downlink/pkg/utils"
)

func parseTimeWindow(from, to, between string, defaultFrom *time.Time) (*time.Time, *time.Time, error) {
	var fromTime, toTime *time.Time

	if between != "" {
		parts := strings.SplitN(between, ",", 2)
		if len(parts) != 2 {
			return nil, nil, fmt.Errorf("invalid --between format: must be two values separated by comma (e.g., '-7d,-1d')")
		}

		start, err := utils.ParseTimeString(strings.TrimSpace(parts[0]))
		if err != nil {
			return nil, nil, fmt.Errorf("invalid start time in --between: %w", err)
		}

		end, err := utils.ParseTimeString(strings.TrimSpace(parts[1]))
		if err != nil {
			return nil, nil, fmt.Errorf("invalid end time in --between: %w", err)
		}

		if start.After(end) {
			return nil, nil, fmt.Errorf("invalid --between: start time must be before end time")
		}

		return &start, &end, nil
	}

	if from != "" {
		parsed, err := utils.ParseTimeString(from)
		if err != nil {
			return nil, nil, fmt.Errorf("error parsing --from: %w", err)
		}
		fromTime = &parsed
	} else if defaultFrom != nil {
		value := *defaultFrom
		fromTime = &value
	}

	if to != "" {
		parsed, err := utils.ParseTimeString(to)
		if err != nil {
			return nil, nil, fmt.Errorf("error parsing --to: %w", err)
		}
		toTime = &parsed
	}

	if fromTime != nil && toTime != nil && toTime.Before(*fromTime) {
		return nil, nil, fmt.Errorf("error: --to (%v) cannot be before --from (%v)", toTime, fromTime)
	}

	return fromTime, toTime, nil
}

func findFeedByIDOrNormalizedName(client *downlinkclient.DownlinkClient, input string) (models.Feed, []models.Feed, error) {
	feeds, err := client.ListFeeds()
	if err != nil {
		return models.Feed{}, nil, err
	}

	for _, feed := range feeds {
		if feed.Id == input {
			return feed, feeds, nil
		}
	}

	normalizedInput := utils.NormalizeFeedName(input)
	for _, feed := range feeds {
		if utils.NormalizeFeedName(feed.Title) == normalizedInput {
			return feed, feeds, nil
		}
	}

	return models.Feed{}, feeds, fmt.Errorf("feed not found: %s", input)
}

func printAvailableFeeds(feeds []models.Feed) {
	fmt.Println("\nAvailable feeds:")
	printFeedTable(feeds)
}

// printArticleContentPreview prints the first and last 10 non-empty lines of
// content, indented by prefix. Used by --dry-run --debug to spot-check article
// bodies without printing the full text.
func printArticleContentPreview(content, prefix string) {
	if content == "" {
		fmt.Printf("%s(no content)\n", prefix)
		return
	}
	var lines []string
	for _, l := range strings.Split(content, "\n") {
		if strings.TrimSpace(l) != "" {
			lines = append(lines, l)
		}
	}
	const n = 10
	if len(lines) <= n*2 {
		for _, l := range lines {
			fmt.Printf("%s%s\n", prefix, l)
		}
		return
	}
	for _, l := range lines[:n] {
		fmt.Printf("%s%s\n", prefix, l)
	}
	fmt.Printf("%s... (%d lines omitted) ...\n", prefix, len(lines)-n*2)
	for _, l := range lines[len(lines)-n:] {
		fmt.Printf("%s%s\n", prefix, l)
	}
}

// flushStdin discards any bytes buffered in the tty input queue.
// Call this before starting a huh form to prevent a leftover Enter keypress
// from a previous form (or from keys typed during a network call) from being
// immediately consumed by the new form.
func flushStdin() {
	const tcflsh = 0x540B // TCFLSH ioctl, Linux
	syscall.Syscall(syscall.SYS_IOCTL, os.Stdin.Fd(), tcflsh, 0)
}

// ── interactive pickers ───────────────────────────────────────────────────────

// selectFeed shows a huh picker over all registered feeds.
// Returns zero value + nil on cancel, error on fetch failure.
func selectFeed(client *downlinkclient.DownlinkClient) (models.Feed, error) {
	feeds, err := client.ListFeeds()
	if err != nil {
		return models.Feed{}, fmt.Errorf("failed to list feeds: %w", err)
	}
	if len(feeds) == 0 {
		return models.Feed{}, fmt.Errorf("no feeds registered")
	}
	if len(feeds) == 1 {
		return feeds[0], nil
	}
	opts := make([]huh.Option[string], len(feeds))
	for i, f := range feeds {
		label := fmt.Sprintf("%-40s  %s", truncate(f.Title, 40), truncate(f.URL, 50))
		opts[i] = huh.NewOption(label, f.Id)
	}
	var id string
	flushStdin()
	if err := huh.NewSelect[string]().
		Title("Select feed").
		Options(opts...).
		Value(&id).
		Run(); err != nil {
		return models.Feed{}, nil
	}
	for _, f := range feeds {
		if f.Id == id {
			return f, nil
		}
	}
	return models.Feed{}, nil
}

// selectArticle shows a huh picker over a filtered article list.
// Returns zero value + nil on cancel, error on fetch failure.
func selectArticle(client *downlinkclient.DownlinkClient, filter models.ArticleFilter) (models.Article, error) {
	articles, err := client.ListArticles(filter)
	if err != nil {
		return models.Article{}, fmt.Errorf("failed to list articles: %w", err)
	}
	if len(articles) == 0 {
		return models.Article{}, fmt.Errorf("no articles found")
	}
	if len(articles) == 1 {
		return articles[0], nil
	}
	opts := make([]huh.Option[string], len(articles))
	for i, a := range articles {
		date := fmtDate(a.PublishedAt)
		label := fmt.Sprintf("%-12s  %s", date, truncate(a.Title, 70))
		opts[i] = huh.NewOption(label, a.Id)
	}
	var id string
	flushStdin()
	if err := huh.NewSelect[string]().
		Title("Select article").
		Options(opts...).
		Value(&id).
		Run(); err != nil {
		return models.Article{}, nil
	}
	for _, a := range articles {
		if a.Id == id {
			return a, nil
		}
	}
	return models.Article{}, nil
}

// selectDigest shows a huh picker over available digests.
// Returns zero value + nil on cancel, error on fetch failure.
func selectDigest(client *downlinkclient.DownlinkClient) (models.Digest, error) {
	digests, err := client.ListDigests(0)
	if err != nil {
		return models.Digest{}, fmt.Errorf("failed to list digests: %w", err)
	}
	if len(digests) == 0 {
		return models.Digest{}, fmt.Errorf("no digests found")
	}
	if len(digests) == 1 {
		return digests[0], nil
	}
	opts := make([]huh.Option[string], len(digests))
	for i, d := range digests {
		title := d.Title
		if title == "" {
			title = "(untitled)"
		}
		count := 0
		if d.ArticleCount != nil {
			count = *d.ArticleCount
		}
		label := fmt.Sprintf("%-16s  %-60s  (%d articles)",
			fmtTime(d.CreatedAt), truncate(title, 60), count)
		opts[i] = huh.NewOption(label, d.Id)
	}
	var id string
	flushStdin()
	if err := huh.NewSelect[string]().
		Title("Select digest").
		Options(opts...).
		Value(&id).
		Run(); err != nil {
		return models.Digest{}, nil
	}
	for _, d := range digests {
		if d.Id == id {
			return d, nil
		}
	}
	return models.Digest{}, nil
}

// getFeedName returns the feed title by ID, or the ID if lookup fails.
func getFeedName(client *downlinkclient.DownlinkClient, feedId string) string {
	feeds, err := client.ListFeeds()
	if err != nil {
		return feedId
	}
	for _, f := range feeds {
		if f.Id == feedId {
			return f.Title
		}
	}
	return feedId
}
