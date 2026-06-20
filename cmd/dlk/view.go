package main

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/ma111e/downlink/pkg/downlinkclient"
	"github.com/ma111e/downlink/pkg/models"
)

// ── small formatting helpers ─────────────────────────────────────────────────

const dash = "—"

func newTable(headers ...string) *tabwriter.Writer {
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	styled := make([]string, len(headers))
	seps := make([]string, len(headers))
	for i, h := range headers {
		styled[i] = styleColHdr.Render(h)
		// Use lipgloss.Width to get visible width (accounts for ANSI codes)
		width := lipgloss.Width(h)
		seps[i] = styleDim.Render(strings.Repeat("─", width))
	}
	fmt.Fprintln(tw, strings.Join(styled, "\t"))
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

func shortID(id string) string {
	return id
}

func section(title string) {
	pad := max(0, 60-len(title))
	sep := styleDim.Render(strings.Repeat("─", pad))
	fmt.Printf("\n%s %s\n", styleSection.Render("── "+title), sep)
}

// kvBlock prints aligned "key: value" pairs.
func kvBlock(pairs [][2]string) {
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	for _, p := range pairs {
		v := p[1]
		if v == "" {
			v = dash
		}
		fmt.Fprintf(tw, "  %s:\t%s\n", styleKey.Render(p[0]), v)
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
// Empty input is silently skipped; the caller decides whether to call.
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

// htmlToMarkdown converts basic HTML to markdown.
func htmlToMarkdown(html string) string {
	// Remove script and style tags
	html = strings.ReplaceAll(html, "<script", "<SKIP_SCRIPT")
	html = strings.ReplaceAll(html, "</script>", "</SKIP_SCRIPT>")
	for strings.Contains(html, "<SKIP_SCRIPT") {
		start := strings.Index(html, "<SKIP_SCRIPT")
		end := strings.Index(html[start:], "</SKIP_SCRIPT>")
		if end == -1 {
			break
		}
		html = html[:start] + html[start+end+14:]
	}

	// Handle block elements with newlines
	blockElements := []string{"</p>", "</div>", "</h1>", "</h2>", "</h3>", "</h4>", "</h5>", "</h6>", "</li>", "</blockquote>", "<br>", "<br/>", "<br />"}
	for _, elem := range blockElements {
		html = strings.ReplaceAll(html, elem, elem+"\n")
	}

	// Headers: <h1> → #, <h2> → ##, etc.
	for i := 1; i <= 6; i++ {
		hashmarks := strings.Repeat("#", i)
		openTag := fmt.Sprintf("<h%d>", i)
		closeTag := fmt.Sprintf("</h%d>", i)
		html = strings.ReplaceAll(html, openTag, "\n"+hashmarks+" ")
		html = strings.ReplaceAll(html, closeTag, "\n")
	}

	// Paragraph tags
	html = strings.ReplaceAll(html, "<p>", "")
	html = strings.ReplaceAll(html, "</p>", "\n")

	// Bold and strong
	html = strings.ReplaceAll(html, "<strong>", "**")
	html = strings.ReplaceAll(html, "</strong>", "**")
	html = strings.ReplaceAll(html, "<b>", "**")
	html = strings.ReplaceAll(html, "</b>", "**")

	// Italic and emphasis
	html = strings.ReplaceAll(html, "<em>", "*")
	html = strings.ReplaceAll(html, "</em>", "*")
	html = strings.ReplaceAll(html, "<i>", "*")
	html = strings.ReplaceAll(html, "</i>", "*")

	// Links: <a href="url">text</a> → [text](url)
	linkRegex := `<a\s+[^>]*href="([^"]*)"[^>]*>([^<]*)</a>`
	html = replaceLinks(html, linkRegex)

	// List items
	html = strings.ReplaceAll(html, "<li>", "- ")
	html = strings.ReplaceAll(html, "</li>", "\n")

	// List containers (remove them)
	html = strings.ReplaceAll(html, "<ul>", "")
	html = strings.ReplaceAll(html, "</ul>", "\n")
	html = strings.ReplaceAll(html, "<ol>", "")
	html = strings.ReplaceAll(html, "</ol>", "\n")

	// Code blocks
	html = strings.ReplaceAll(html, "<code>", "`")
	html = strings.ReplaceAll(html, "</code>", "`")
	html = strings.ReplaceAll(html, "<pre>", "```\n")
	html = strings.ReplaceAll(html, "</pre>", "\n```")

	// Remove remaining HTML tags
	for strings.Contains(html, "<") {
		start := strings.Index(html, "<")
		end := strings.Index(html[start:], ">")
		if end == -1 {
			break
		}
		html = html[:start] + html[start+end+1:]
	}

	// HTML entities
	html = strings.ReplaceAll(html, "&nbsp;", " ")
	html = strings.ReplaceAll(html, "&lt;", "<")
	html = strings.ReplaceAll(html, "&gt;", ">")
	html = strings.ReplaceAll(html, "&amp;", "&")
	html = strings.ReplaceAll(html, "&quot;", "\"")
	html = strings.ReplaceAll(html, "&#39;", "'")

	// Clean up multiple newlines
	for strings.Contains(html, "\n\n\n") {
		html = strings.ReplaceAll(html, "\n\n\n", "\n\n")
	}

	return strings.TrimSpace(html)
}

// replaceLinks finds and replaces HTML links with markdown format.
func replaceLinks(text, pattern string) string {
	// Simple regex-free approach: find <a> tags and convert manually
	for strings.Contains(text, "<a ") || strings.Contains(text, "<a>") {
		start := strings.Index(text, "<a")
		if start == -1 {
			break
		}
		end := strings.Index(text[start:], "</a>")
		if end == -1 {
			break
		}

		tagEnd := strings.Index(text[start:], ">")
		if tagEnd == -1 || tagEnd > end {
			break
		}

		tag := text[start : start+tagEnd+1]
		linkText := text[start+tagEnd+1 : start+end]

		// Extract href
		hrefStart := strings.Index(tag, `href="`)
		if hrefStart == -1 {
			hrefStart = strings.Index(tag, `href='`)
			if hrefStart == -1 {
				text = text[:start] + linkText + text[start+end+4:]
				continue
			}
			hrefStart += 6
			hrefEnd := strings.Index(tag[hrefStart:], "'")
			if hrefEnd == -1 {
				text = text[:start] + linkText + text[start+end+4:]
				continue
			}
			href := tag[hrefStart : hrefStart+hrefEnd]
			text = text[:start] + "[" + linkText + "](" + href + ")" + text[start+end+4:]
		} else {
			hrefStart += 6
			hrefEnd := strings.Index(tag[hrefStart:], "\"")
			if hrefEnd == -1 {
				text = text[:start] + linkText + text[start+end+4:]
				continue
			}
			href := tag[hrefStart : hrefStart+hrefEnd]
			text = text[:start] + "[" + linkText + "](" + href + ")" + text[start+end+4:]
		}
	}
	return text
}

// renderMarkdownWithGlow renders markdown with terminal styling using glamour.
func renderMarkdownWithGlow(markdown string) string {
	if markdown == "" {
		return markdown
	}

	// Use glamour to render markdown to ANSI
	r, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
	)
	if err != nil {
		return markdown
	}

	rendered, err := r.Render(markdown)
	if err != nil {
		return markdown
	}

	// Indent each non-empty line
	var result strings.Builder
	lines := strings.Split(rendered, "\n")
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			result.WriteString("  " + line + "\n")
		} else {
			result.WriteString("\n")
		}
	}

	return result.String()
}

// formatMarkdown converts HTML to markdown and renders it with styling.
func formatMarkdown(text string) string {
	if text == "" {
		return text
	}

	// If it looks like HTML, convert to markdown first
	if strings.Contains(text, "</") || strings.Contains(text, "/>") || strings.Contains(text, "<p>") {
		text = htmlToMarkdown(text)
	}

	return renderMarkdownWithGlow(text)
}

// min returns the minimum of two integers.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// printMarkdownSection prints a section with markdown-formatted text.
func printMarkdownSection(title, text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	section(title)
	fmt.Print(formatMarkdown(text))
}

// ── list / table formatters ──────────────────────────────────────────────────

func printFeedTable(feeds []models.Feed) {
	tw := newTable("ID", "TITLE", "TYPE", "ENABLED", "LAST FETCH", "URL")
	for _, f := range feeds {
		enabled := fmtBool(f.Enabled, styleOK.Render("yes"), styleErr.Render("no"), dash)
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
			shortID(f.Id),
			truncate(f.Title, 40),
			dashIfEmpty(f.Type),
			enabled,
			fmtTime(f.LastFetch),
			styleDim.Render(truncate(f.URL, 60)),
		)
	}
	tw.Flush()
	fmt.Printf("\n%s\n", styleDim.Render(fmt.Sprintf("%d feed(s)", len(feeds))))
}

func printArticleTable(articles []models.Article) {
	tw := newTable("ID", "PUBLISHED", "READ", "BOOK", "SCORE", "FEED", "TITLE")
	for _, a := range articles {
		score := dash
		if a.LatestImportanceScore != nil {
			score = fmt.Sprintf("%d", *a.LatestImportanceScore)
		}
		read := fmtBool(a.Read, styleOK.Render("•"), styleDim.Render("·"), dash)
		book := fmtBool(a.Bookmarked, styleOK.Render("•"), styleDim.Render("·"), dash)
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			styleDim.Render(shortID(a.Id)),
			fmtDate(a.PublishedAt),
			read,
			book,
			score,
			styleDim.Render(shortID(a.FeedId)),
			truncate(a.Title, 70),
		)
	}
	tw.Flush()
	fmt.Printf("\n%s\n", styleDim.Render(fmt.Sprintf("%d article(s)", len(articles))))
}

func printDigestTable(digests []models.Digest) {
	tw := newTable("ID", "CREATED", "WINDOW", "ARTICLES", "TITLE")
	for _, d := range digests {
		count := dash
		if d.ArticleCount != nil {
			count = fmt.Sprintf("%d", *d.ArticleCount)
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
			styleDim.Render(shortID(d.Id)),
			fmtTime(d.CreatedAt),
			styleDim.Render(fmtDuration(d.TimeWindow)),
			count,
			truncate(d.Title, 70),
		)
	}
	tw.Flush()
	fmt.Printf("\n%s\n", styleDim.Render(fmt.Sprintf("%d digest(s)", len(digests))))
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

func printProviderTable(providers []models.ProviderConfig) {
	tw := newTable("NAME", "TYPE", "MODEL", "ENABLED")
	for _, p := range providers {
		name := p.Name
		if name == "" {
			name = styleDim.Render("(unnamed)")
		}
		var enabled string
		if p.Enabled {
			enabled = styleOK.Render("yes")
		} else {
			enabled = styleErr.Render("no")
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n",
			name,
			dashIfEmpty(p.ProviderType),
			dashIfEmpty(p.ModelName),
			enabled,
		)
	}
	tw.Flush()
}

// ── detail formatters ────────────────────────────────────────────────────────

func printArticleDetail(client *downlinkclient.DownlinkClient, a models.Article) {
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

	feedName := getFeedName(client, a.FeedId)
	kvBlock([][2]string{
		{"Title", a.Title},
		{"Feed", feedName},
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

func printArticleDetailMarkdown(client *downlinkclient.DownlinkClient, a models.Article) {
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

	feedName := getFeedName(client, a.FeedId)
	kvBlock([][2]string{
		{"Title", a.Title},
		{"Feed", feedName},
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

	printMarkdownSection("Content", a.Content)
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

func printDigestDetailMarkdown(d models.Digest, articles []models.Article) {
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

	printMarkdownSection("Summary", d.DigestSummary)

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

func printAnalysisDetailMarkdown(a models.ArticleAnalysis) {
	section("Analysis " + shortID(a.Id) + "  ·  article " + shortID(a.ArticleId))

	kvBlock([][2]string{
		{"Provider", dashIfEmpty(a.ProviderType)},
		{"Model", dashIfEmpty(a.ModelName)},
		{"Importance", fmt.Sprintf("%d", a.ImportanceScore)},
		{"Created", fmtTime(a.CreatedAt)},
	})

	printMarkdownSection("TL;DR", a.Tldr)
	printMarkdownSection("Justification", a.Justification)
	printMarkdownSection("Brief overview", a.BriefOverview)

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

	printMarkdownSection("Standard synthesis", a.StandardSynthesis)
	printMarkdownSection("Comprehensive synthesis", a.ComprehensiveSynthesis)

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

	printMarkdownSection("Thinking process", a.ThinkingProcess)
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
