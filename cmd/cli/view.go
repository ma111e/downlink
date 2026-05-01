package main

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"downlink/pkg/downlinkclient"
	"downlink/pkg/models"
)

// ── small formatting helpers ─────────────────────────────────────────────────

const dash = "—"

func newTable(headers ...string) *tabwriter.Writer {
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, strings.Join(headers, "\t"))
	seps := make([]string, len(headers))
	for i, h := range headers {
		seps[i] = strings.Repeat("-", len(h))
	}
	fmt.Fprintln(tw, strings.Join(seps, "\t"))
	return tw
}

func truncate(s string, n int) string {
	if n <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n == 1 {
		return "…"
	}
	return string(r[:n-1]) + "…"
}

func fmtTime(t time.Time) string {
	if t.IsZero() {
		return dash
	}
	return t.Local().Format("2006-01-02 15:04")
}

func fmtDate(t time.Time) string {
	if t.IsZero() {
		return dash
	}
	return t.Local().Format("2006-01-02")
}

func fmtBool(b *bool, t, f, nilv string) string {
	if b == nil {
		return nilv
	}
	if *b {
		return t
	}
	return f
}

func fmtDuration(d time.Duration) string {
	if d == 0 {
		return dash
	}
	if d >= 24*time.Hour && d%(24*time.Hour) == 0 {
		return fmt.Sprintf("%dd", d/(24*time.Hour))
	}
	if d%time.Hour == 0 {
		return fmt.Sprintf("%dh", d/time.Hour)
	}
	return d.String()
}

// shortID returns the first 8 characters of an ID, or the full ID if shorter.
// Safe replacement for ad-hoc id[:N] / id[N:] slicing that panics on short IDs.
func shortID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}

func section(title string) {
	pad := max(0, 60-len(title))
	fmt.Printf("\n── %s %s\n", title, strings.Repeat("─", pad))
}

// kvBlock prints aligned "key: value" pairs.
func kvBlock(pairs [][2]string) {
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	for _, p := range pairs {
		v := p[1]
		if v == "" {
			v = dash
		}
		fmt.Fprintf(tw, "  %s:\t%s\n", p[0], v)
	}
	tw.Flush()
}

func bullets(items []string) {
	for _, it := range items {
		fmt.Printf("  • %s\n", it)
	}
}

func mask(s string) string {
	if s == "" {
		return dash
	}
	if len(s) <= 4 {
		return "***"
	}
	return "***" + s[len(s)-4:]
}

func dashIfEmpty(s string) string {
	if s == "" {
		return dash
	}
	return s
}

func ptrStr(s *string) string {
	if s == nil || *s == "" {
		return dash
	}
	return *s
}

// printBody renders a multi-line text block under a section header.
// Empty input is silently skipped — caller decides whether to call.
func printBody(text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	fmt.Println()
	for line := range strings.SplitSeq(text, "\n") {
		fmt.Println("  " + line)
	}
}

// ── list / table formatters ──────────────────────────────────────────────────

func printFeedTable(feeds []models.Feed) {
	tw := newTable("ID", "TITLE", "TYPE", "ENABLED", "LAST FETCH", "URL")
	for _, f := range feeds {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
			shortID(f.Id),
			truncate(f.Title, 40),
			dashIfEmpty(f.Type),
			fmtBool(f.Enabled, "yes", "no", dash),
			fmtTime(f.LastFetch),
			truncate(f.URL, 60),
		)
	}
	tw.Flush()
	fmt.Printf("\n%d feed(s)\n", len(feeds))
}

func printArticleTable(articles []models.Article) {
	tw := newTable("ID", "PUBLISHED", "READ", "BOOK", "SCORE", "FEED", "TITLE")
	for _, a := range articles {
		score := dash
		if a.LatestImportanceScore != nil {
			score = fmt.Sprintf("%d", *a.LatestImportanceScore)
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			shortID(a.Id),
			fmtDate(a.PublishedAt),
			fmtBool(a.Read, "•", "·", dash),
			fmtBool(a.Bookmarked, "•", "·", dash),
			score,
			shortID(a.FeedId),
			truncate(a.Title, 70),
		)
	}
	tw.Flush()
	fmt.Printf("\n%d article(s)\n", len(articles))
}

func printDigestTable(digests []models.Digest) {
	tw := newTable("ID", "CREATED", "WINDOW", "ARTICLES", "TITLE")
	for _, d := range digests {
		count := dash
		if d.ArticleCount != nil {
			count = fmt.Sprintf("%d", *d.ArticleCount)
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
			shortID(d.Id),
			fmtTime(d.CreatedAt),
			fmtDuration(d.TimeWindow),
			count,
			truncate(d.Title, 70),
		)
	}
	tw.Flush()
	fmt.Printf("\n%d digest(s)\n", len(digests))
}

func printAnalysisList(analyses []models.ArticleAnalysis) {
	tw := newTable("ID", "CREATED", "PROVIDER", "MODEL", "SCORE", "TLDR")
	for _, a := range analyses {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%d\t%s\n",
			shortID(a.Id),
			fmtTime(a.CreatedAt),
			dashIfEmpty(a.ProviderType),
			dashIfEmpty(a.ModelName),
			a.ImportanceScore,
			truncate(strings.TrimSpace(a.Tldr), 60),
		)
	}
	tw.Flush()
	fmt.Printf("\n%d analysis(es)\n", len(analyses))
}

func printModelInfoTable(mods []models.ModelInfo) {
	tw := newTable("PROVIDER", "NAME", "DISPLAY NAME", "DESCRIPTION")
	for _, m := range mods {
		display := m.DisplayName
		if display == m.Name {
			display = ""
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n",
			dashIfEmpty(m.ProviderType),
			dashIfEmpty(m.Name),
			dashIfEmpty(display),
			truncate(m.Description, 60),
		)
	}
	tw.Flush()
	fmt.Printf("\n%d model(s)\n", len(mods))
}

func printProviderTable(providers []models.ProviderConfig) {
	tw := newTable("NAME", "TYPE", "MODEL", "ENABLED", "TEMP")
	for _, p := range providers {
		name := p.Name
		if name == "" {
			name = "(unnamed)"
		}
		enabled := "no"
		if p.Enabled {
			enabled = "yes"
		}
		temp := dash
		if p.Temperature != nil {
			temp = fmt.Sprintf("%.2f", *p.Temperature)
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
			name,
			dashIfEmpty(p.ProviderType),
			dashIfEmpty(p.ModelName),
			enabled,
			temp,
		)
	}
	tw.Flush()
}

// ── detail formatters ────────────────────────────────────────────────────────

func printArticleDetail(a models.Article) {
	section("Article " + shortID(a.Id))

	tags := make([]string, len(a.Tags))
	for i, t := range a.Tags {
		tags[i] = t.Name
	}

	relCount := fmt.Sprintf("%d related", len(a.RelatedArticles))
	score := dash
	if a.LatestImportanceScore != nil {
		score = fmt.Sprintf("%d", *a.LatestImportanceScore)
	}

	kvBlock([][2]string{
		{"Title", a.Title},
		{"Feed", a.FeedId},
		{"Link", a.Link},
		{"Published", fmtTime(a.PublishedAt)},
		{"Fetched", fmtTime(a.FetchedAt)},
		{"Read", fmtBool(a.Read, "yes", "no", dash)},
		{"Bookmarked", fmtBool(a.Bookmarked, "yes", "no", dash)},
		{"Category", ptrStr(a.CategoryName)},
		{"Importance", score},
		{"Tags", dashIfEmpty(strings.Join(tags, ", "))},
		{"Hero image", a.HeroImage},
		{"Related", relCount},
	})

	if strings.TrimSpace(a.Content) != "" {
		section("Content")
		printBody(a.Content)
	}
}

func printDigestDetail(d models.Digest, articles []models.Article) {
	section("Digest " + shortID(d.Id))

	count := dash
	if d.ArticleCount != nil {
		count = fmt.Sprintf("%d", *d.ArticleCount)
	}

	kvBlock([][2]string{
		{"Title", d.Title},
		{"Created", fmtTime(d.CreatedAt)},
		{"Window", fmtDuration(d.TimeWindow)},
		{"Articles", count},
		{"Providers", fmt.Sprintf("%d result(s)", len(d.ProviderResults))},
	})

	if strings.TrimSpace(d.DigestSummary) != "" {
		section("Summary")
		printBody(d.DigestSummary)
	}

	if len(articles) > 0 {
		section("Articles in this digest")
		fmt.Println()
		printArticleTable(articles)
	}
}

func printAnalysisDetail(a models.ArticleAnalysis) {
	section("Analysis " + shortID(a.Id) + "  ·  article " + shortID(a.ArticleId))

	kvBlock([][2]string{
		{"Provider", dashIfEmpty(a.ProviderType)},
		{"Model", dashIfEmpty(a.ModelName)},
		{"Importance", fmt.Sprintf("%d", a.ImportanceScore)},
		{"Created", fmtTime(a.CreatedAt)},
	})

	if s := strings.TrimSpace(a.Tldr); s != "" {
		section("TL;DR")
		printBody(s)
	}
	if s := strings.TrimSpace(a.Justification); s != "" {
		section("Justification")
		printBody(s)
	}
	if s := strings.TrimSpace(a.BriefOverview); s != "" {
		section("Brief overview")
		printBody(s)
	}
	if len(a.KeyPoints) > 0 {
		section("Key points")
		fmt.Println()
		bullets(a.KeyPoints)
	}
	if len(a.Insights) > 0 {
		section("Insights")
		fmt.Println()
		bullets(a.Insights)
	}
	if s := strings.TrimSpace(a.StandardSynthesis); s != "" {
		section("Standard synthesis")
		printBody(s)
	}
	if s := strings.TrimSpace(a.ComprehensiveSynthesis); s != "" {
		section("Comprehensive synthesis")
		printBody(s)
	}
	if len(a.ReferencedReports) > 0 {
		section("Referenced reports")
		fmt.Println()
		for _, r := range a.ReferencedReports {
			head := r.Title
			if r.Publisher != "" {
				head = head + " — " + r.Publisher
			}
			fmt.Printf("  • %s\n", head)
			if r.URL != "" {
				fmt.Printf("    %s\n", r.URL)
			}
			if r.Context != "" {
				fmt.Printf("    %s\n", r.Context)
			}
		}
	}
	if s := strings.TrimSpace(a.ThinkingProcess); s != "" {
		section("Thinking process")
		printBody(s)
	}
}

func printAnalysisConfig(c models.AnalysisConfig) {
	section("Analysis configuration")
	workers := dash
	if c.WorkerPool != nil && c.WorkerPool.MaxWorkers != nil {
		workers = fmt.Sprintf("%d", *c.WorkerPool.MaxWorkers)
	}
	kvBlock([][2]string{
		{"Provider", dashIfEmpty(c.Provider)},
		{"Persona", dashIfEmpty(c.Persona)},
		{"Workers", workers},
	})
}

func printServerConfig(c models.ServerConfig) {
	section("General")
	kvBlock([][2]string{
		{"DB path", dashIfEmpty(c.DbPath)},
		{"Solimen address", dashIfEmpty(c.SolimenAddr)},
		{"Feeds configured", fmt.Sprintf("%d", len(c.Feeds))},
	})

	section("Providers")
	if len(c.Providers) == 0 {
		fmt.Println("  (none configured)")
	} else {
		// Mask API keys before printing.
		masked := make([]models.ProviderConfig, len(c.Providers))
		copy(masked, c.Providers)
		for i := range masked {
			if masked[i].APIKey != "" {
				masked[i].APIKey = mask(masked[i].APIKey)
			}
		}
		printProviderTable(masked)
	}

	section("Analysis")
	workers := dash
	if c.Analysis.WorkerPool != nil && c.Analysis.WorkerPool.MaxWorkers != nil {
		workers = fmt.Sprintf("%d", *c.Analysis.WorkerPool.MaxWorkers)
	}
	kvBlock([][2]string{
		{"Provider", dashIfEmpty(c.Analysis.Provider)},
		{"Persona", dashIfEmpty(c.Analysis.Persona)},
		{"Workers", workers},
	})

	section("Notifications · Discord")
	kvBlock([][2]string{
		{"Enabled", boolStr(c.Notifications.Discord.Enabled)},
		{"Webhook", mask(c.Notifications.Discord.WebhookURL)},
	})

	section("Notifications · GitHub Pages")
	gh := c.Notifications.GitHubPages
	kvBlock([][2]string{
		{"Enabled", boolStr(gh.Enabled)},
		{"Repo URL", dashIfEmpty(gh.RepoURL)},
		{"Branch", dashIfEmpty(gh.Branch)},
		{"Output dir", dashIfEmpty(gh.OutputDir)},
		{"Base URL", dashIfEmpty(gh.BaseURL)},
		{"Configure pages", boolStr(gh.ConfigurePages)},
		{"Token", mask(gh.Token)},
		{"Commit author", dashIfEmpty(gh.CommitAuthor)},
		{"Commit email", dashIfEmpty(gh.CommitEmail)},
		{"Discord webhook", mask(gh.DiscordWebhookURL)},
	})
}

func boolStr(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func printQueueStatus(s downlinkclient.QueueStatus) {
	state := "idle"
	if s.IsProcessing {
		state = "processing"
	}
	section("Queue · " + state)
	kvBlock([][2]string{
		{"Current", dashIfEmpty(s.CurrentTitle)},
		{"Queued", fmt.Sprintf("%d", len(s.Queue))},
	})

	if len(s.Queue) == 0 {
		return
	}

	fmt.Println()
	tw := newTable("#", "TITLE", "PROFILE/PROVIDER", "MODEL")
	for i, j := range s.Queue {
		profile := j.ProviderName
		if profile == "" {
			profile = j.ProviderType
		}
		fmt.Fprintf(tw, "%d\t%s\t%s\t%s\n",
			i+1,
			truncate(j.ArticleTitle, 60),
			dashIfEmpty(profile),
			dashIfEmpty(j.ModelName),
		)
	}
	tw.Flush()
}
