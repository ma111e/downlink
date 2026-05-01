package notification

import (
	"bytes"
	"downlink/pkg/digestthemes"
	"downlink/pkg/models"
	"fmt"
	"html/template"
	"net/url"
	"sort"
	"strings"

	"github.com/gomarkdown/markdown"
	mdhtml "github.com/gomarkdown/markdown/html"
	"github.com/gomarkdown/markdown/parser"
)

// TOCEntry holds data for a single article row in the table of contents.
type TOCEntry struct {
	Id                  string
	Title               string
	ImportanceScore     int
	ReadTag             string
	DuplicateGroup      string
	IsMostComprehensive bool
}

// RenderedAnalysis holds markdown-converted HTML versions of analysis text fields.
type RenderedAnalysis struct {
	ProviderType           string
	ModelName              string
	Tldr                   string
	Justification          template.HTML
	BriefOverview          template.HTML
	StandardSynthesis      template.HTML
	ComprehensiveSynthesis template.HTML
	KeyPoints              []string
	Insights               []string
	ReferencedReports      []models.ReferencedReport
}

// ArticleEntry holds enriched article data for template rendering
type ArticleEntry struct {
	Id                  string
	Title               string
	Source              string
	Link                string
	PublishedAt         string
	ImportanceScore     int
	ReadTag             string
	DuplicateGroup      string
	IsMostComprehensive bool
	HasAnalysis         bool
	Analysis            *RenderedAnalysis
}

// readTag returns a priority label based on a 1-100 importance score, matching the UI thresholds.
func readTag(score int) string {
	switch {
	case score >= 90:
		return "Must Read"
	case score >= 75:
		return "Should Read"
	case score >= 60:
		return "May Read"
	case score > 0:
		return "Optional"
	default:
		return "Unscored"
	}
}

// tagOrder defines the display order of read-tag groups in the TOC.
var tagOrder = []string{"Must Read", "Should Read", "May Read", "Optional", "Unscored"}

// TOCRow is a single rendered row in a TOC group: either a plain article entry or
// a duplicate cluster (one canonical article + its alternates in a <details> block).
type TOCRow struct {
	IsCluster    bool
	Entry        TOCEntry        // used when IsCluster == false
	Canonical    TOCEntry        // used when IsCluster == true: the most-comprehensive article
	Others       []TOCEntry      // used when IsCluster == true: remaining members
	Group        string          // duplicate group key, used for colour
	Detail       *ArticleEntry   // full detail for non-cluster row
	CanonDetail  *ArticleEntry   // full detail for cluster canonical
	OtherDetails []*ArticleEntry // full detail for each cluster member
}

// TOCGroup is a labelled section in the table of contents.
type TOCGroup struct {
	Label string
	Rows  []TOCRow
}

// buildTOCGroups partitions already-sorted TOC entries into labelled groups, collapsing
// duplicate clusters into a single <details> row placed in the highest-priority group
// any cluster member appears in.
func buildTOCGroups(entries []TOCEntry) []TOCGroup {
	// Phase 1: collect per-group members and track highest-priority tag for each cluster.
	tagPriority := make(map[string]int, len(tagOrder))
	for i, t := range tagOrder {
		tagPriority[t] = i
	}

	type clusterInfo struct {
		canonical TOCEntry
		others    []TOCEntry
		bestTag   string
		bestPrio  int
	}
	clusters := make(map[string]*clusterInfo)
	var clusterOrder []string // insertion order for stable output

	var plain []TOCEntry // non-duplicate entries

	for _, e := range entries {
		if e.DuplicateGroup == "" {
			plain = append(plain, e)
			continue
		}
		ci, exists := clusters[e.DuplicateGroup]
		if !exists {
			ci = &clusterInfo{bestPrio: len(tagOrder)}
			clusters[e.DuplicateGroup] = ci
			clusterOrder = append(clusterOrder, e.DuplicateGroup)
		}
		prio := tagPriority[e.ReadTag]
		if prio < ci.bestPrio {
			ci.bestPrio = prio
			ci.bestTag = e.ReadTag
		}
		if e.IsMostComprehensive {
			ci.canonical = e
		} else {
			ci.others = append(ci.others, e)
		}
	}

	// Phase 2: build priority buckets of TOCRows.
	type bucket struct {
		rows []TOCRow
	}
	buckets := make(map[string]*bucket, len(tagOrder))
	for _, t := range tagOrder {
		buckets[t] = &bucket{}
	}

	// Place each cluster row into the bucket of its highest-priority tag.
	for _, g := range clusterOrder {
		ci := clusters[g]
		// If no member was marked most-comprehensive, promote the first other.
		if ci.canonical.Id == "" && len(ci.others) > 0 {
			ci.canonical = ci.others[0]
			ci.others = ci.others[1:]
		}
		buckets[ci.bestTag].rows = append(buckets[ci.bestTag].rows, TOCRow{
			IsCluster: true,
			Canonical: ci.canonical,
			Others:    ci.others,
			Group:     g,
		})
	}

	// Place plain entries into their respective buckets.
	for _, e := range plain {
		b := buckets[e.ReadTag]
		b.rows = append(b.rows, TOCRow{Entry: e})
	}

	// Phase 3: sort each bucket: clusters by canonical score, plain by score (already sorted
	// globally, but plain entries need stable interleaving with cluster rows).
	// Simple approach: re-sort each bucket by lead score descending.
	for _, b := range buckets {
		sort.SliceStable(b.rows, func(i, j int) bool {
			si := b.rows[i].leadScore()
			sj := b.rows[j].leadScore()
			return si > sj
		})
	}

	var groups []TOCGroup
	for _, label := range tagOrder {
		if b := buckets[label]; len(b.rows) > 0 {
			groups = append(groups, TOCGroup{Label: label, Rows: b.rows})
		}
	}
	return groups
}

func (r TOCRow) leadScore() int {
	if r.IsCluster {
		return r.Canonical.ImportanceScore
	}
	return r.Entry.ImportanceScore
}

// OverviewSection is a single cell in the Intelligence Brief 2-column grid.
type OverviewSection struct {
	Tag   string        // e.g. "EXEC", "01", "02"
	Title string        // section heading
	Body  template.HTML // rendered markdown body
}

// digestTemplateData is the root data passed to the HTML template
type digestTemplateData struct {
	StartedAt        string
	ArticleCount     int
	ModelName        string
	TimeWindow       string
	SwipeFilename    string
	DigestTitle      string
	ThemeOverride    template.CSS
	DigestSummary    template.HTML // kept for backwards compat; OverviewSections is used for rendering
	OverviewSections []OverviewSection
	TOCGroups        []TOCGroup
	ArticleEntries   []ArticleEntry
}

// RenderDigestHTML generates a self-contained HTML file for the given digest.
// The digest must have Articles, DigestAnalyses (with Analysis preloaded), and ProviderResults populated.
// theme selects the visual style; an empty string or "dark" uses the default dark theme.
//
// The provider/model switcher in the rendered page is populated client-side
// from manifest.json — the page itself only embeds the digest id and a hash
// of its article set used to filter siblings.
func RenderDigestHTML(digest models.Digest, theme string) ([]byte, error) {
	// Build a lookup: articleId → DigestAnalysis (for duplicate metadata and analysis)
	daByArticle := make(map[string]models.DigestAnalysis, len(digest.DigestAnalyses))
	for _, da := range digest.DigestAnalyses {
		daByArticle[da.ArticleId] = da
	}

	var tocEntries []TOCEntry
	var articleEntries []ArticleEntry
	var digestModelName string

	for _, art := range digest.Articles {
		da := daByArticle[art.Id]

		var analysis *models.ArticleAnalysis
		var importanceScore int
		if da.Analysis != nil {
			analysis = da.Analysis
			importanceScore = da.Analysis.ImportanceScore
		}

		tag := readTag(importanceScore)

		tocEntries = append(tocEntries, TOCEntry{
			Id:                  art.Id,
			Title:               articleTitle(art.Title),
			ImportanceScore:     importanceScore,
			ReadTag:             tag,
			DuplicateGroup:      da.DuplicateGroup,
			IsMostComprehensive: da.IsMostComprehensive,
		})

		var rendered *RenderedAnalysis
		if analysis != nil {
			if digestModelName == "" {
				digestModelName = analysis.ModelName
			}
			rendered = &RenderedAnalysis{
				ProviderType:           analysis.ProviderType,
				ModelName:              analysis.ModelName,
				Tldr:                   analysis.Tldr,
				Justification:          markdownToHTML(analysis.Justification),
				BriefOverview:          markdownToHTML(analysis.BriefOverview),
				StandardSynthesis:      markdownToHTML(analysis.StandardSynthesis),
				ComprehensiveSynthesis: markdownToHTML(analysis.ComprehensiveSynthesis),
				KeyPoints:              analysis.KeyPoints,
				Insights:               analysis.Insights,
				ReferencedReports:      analysis.ReferencedReports,
			}
		}

		articleEntries = append(articleEntries, ArticleEntry{
			Id:                  art.Id,
			Title:               articleTitle(art.Title),
			Source:              articleSource(art.Link),
			Link:                art.Link,
			PublishedAt:         art.PublishedAt.Format("2006-01-02 15:04"),
			ImportanceScore:     importanceScore,
			ReadTag:             tag,
			DuplicateGroup:      da.DuplicateGroup,
			IsMostComprehensive: da.IsMostComprehensive,
			HasAnalysis:         rendered != nil,
			Analysis:            rendered,
		})
	}

	articleCount := len(articleEntries)

	// Sort TOC by importance score descending before grouping.
	sort.Slice(tocEntries, func(i, j int) bool {
		return tocEntries[i].ImportanceScore > tocEntries[j].ImportanceScore
	})

	tocGroups := buildTOCGroups(tocEntries)

	// Build id→detail map and attach full article data to each TOC row.
	detailByID := make(map[string]*ArticleEntry, len(articleEntries))
	for i := range articleEntries {
		detailByID[articleEntries[i].Id] = &articleEntries[i]
	}
	for gi := range tocGroups {
		for ri := range tocGroups[gi].Rows {
			row := &tocGroups[gi].Rows[ri]
			if row.IsCluster {
				row.CanonDetail = detailByID[row.Canonical.Id]
				row.OtherDetails = make([]*ArticleEntry, len(row.Others))
				for oi, o := range row.Others {
					row.OtherDetails[oi] = detailByID[o.Id]
				}
			} else {
				row.Detail = detailByID[row.Entry.Id]
			}
		}
	}

	var themeOverride template.CSS
	if t, ok := digestthemes.Get(theme); ok && t.Vars != nil {
		var sb strings.Builder
		for k, v := range t.Vars {
			sb.WriteString(k)
			sb.WriteString(": ")
			sb.WriteString(v)
			sb.WriteString("; ")
		}
		themeOverride = template.CSS(sb.String()) //nolint:gosec // values come from our own hardcoded theme map
	}

	data := digestTemplateData{
		StartedAt:        digest.CreatedAt.Add(-digest.TimeWindow).Format("2006-01-02 15:04 UTC"),
		ArticleCount:     articleCount,
		ModelName:        digestModelName,
		TimeWindow:       formatDuration(digest.TimeWindow),
		SwipeFilename:    SwipeHTMLFilename(digest),
		DigestTitle:      digest.Title,
		DigestSummary:    markdownToHTML(digest.DigestSummary),
		OverviewSections: parseOverviewSections(digest.DigestSummary),
		TOCGroups:        tocGroups,
		ArticleEntries:   articleEntries,
		ThemeOverride:    themeOverride,
	}

	funcMap := template.FuncMap{
		"add":                func(a, b int) int { return a + b },
		"slugify":            func(s string) string { return strings.ReplaceAll(s, " ", "-") },
		"dupColor":           dupGroupColor,
		"sourceColor":        sourceColor,
		"sourceColorVal":     sourceColorVal,
		"dupBadgeStyle":      dupBadgeStyle,
		"dupGroupLetter":     dupGroupLetter,
		"tocBadgeClass":      tocBadgeClass,
		"tocNumClass":        tocNumClass,
		"priorityRowClass":   priorityRowClass,
		"priorityBadgeClass": priorityBadgeClass,
		"priorityShort":      priorityShort,
		"priorityKey":        priorityKey,
		"scoreBar":           scoreBarHTML,
		// overview grid helpers
		// evenIndex: i=0 is EXEC (full-width); i=2,4,6… are left-column cells → get border-right
		"evenIndex": func(i int) bool { return i%2 == 0 },
		// sectionBorderB: border-bottom on all cells except the last two (the last pair)
		"sectionBorderB": func(i, total int) bool { return i < total-2 },
	}

	templateText, err := loadNotificationTemplate("digest.html.tmpl")
	if err != nil {
		return nil, fmt.Errorf("failed to load digest HTML template: %w", err)
	}

	tmpl, err := template.New("digest").Funcs(funcMap).Parse(templateText)
	if err != nil {
		return nil, fmt.Errorf("failed to parse digest HTML template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("failed to render digest HTML: %w", err)
	}

	return buf.Bytes(), nil
}

// DigestHTMLFilename returns a filesystem-safe filename for the digest HTML file.
func DigestHTMLFilename(digest models.Digest) string {
	ts := digest.CreatedAt.UTC().Format("2006-01-02_1504")
	return fmt.Sprintf("downlink-digest-%s.html", ts)
}

// parseOverviewSections splits a markdown digest summary into OverviewSection blocks.
// It splits on level-2 headings (## Heading). The content before the first heading
// (if any) becomes the EXEC section. Each subsequent ## heading produces a numbered
// section (01, 02, …). If there are no ## headings the entire text becomes one EXEC cell.
func parseOverviewSections(md string) []OverviewSection {
	if strings.TrimSpace(md) == "" {
		return nil
	}

	lines := strings.Split(md, "\n")
	type rawSection struct {
		title string
		lines []string
	}

	var sections []rawSection
	var cur *rawSection

	for _, line := range lines {
		if strings.HasPrefix(line, "## ") {
			if cur != nil {
				sections = append(sections, *cur)
			}
			cur = &rawSection{title: strings.TrimPrefix(line, "## ")}
		} else {
			if cur == nil {
				cur = &rawSection{title: "Executive Overview"}
			}
			cur.lines = append(cur.lines, line)
		}
	}
	if cur != nil {
		sections = append(sections, *cur)
	}

	if len(sections) == 0 {
		return nil
	}

	result := make([]OverviewSection, 0, len(sections))
	numbered := 0
	for i, s := range sections {
		body := strings.TrimSpace(strings.Join(s.lines, "\n"))
		var tag string
		if i == 0 && (s.title == "Executive Overview" || len(sections) == 1) {
			tag = "EXEC"
		} else {
			numbered++
			tag = fmt.Sprintf("%02d", numbered)
		}
		result = append(result, OverviewSection{
			Tag:   tag,
			Title: s.title,
			Body:  markdownToHTML(body),
		})
	}
	return result
}

// markdownToHTML converts a markdown string to sanitized HTML using gomarkdown.
func markdownToHTML(md string) template.HTML {
	if md == "" {
		return ""
	}
	extensions := parser.CommonExtensions | parser.AutoHeadingIDs | parser.NoEmptyLineBeforeBlock
	p := parser.NewWithExtensions(extensions)
	opts := mdhtml.RendererOptions{Flags: mdhtml.CommonFlags | mdhtml.HrefTargetBlank}
	renderer := mdhtml.NewRenderer(opts)
	output := markdown.ToHTML([]byte(md), p, renderer)
	return template.HTML(output) //nolint:gosec // markdown is LLM-generated content stored in our own DB
}

// colorPalette is a set of visually distinct colors used for source and duplicate group dots.
var colorPalette = []string{
	"#f87171", // red
	"#fb923c", // orange
	"#ca9a04", // yellow
	"#4ade80", // green
	"#2dd4bf", // teal
	"#60a5fa", // blue
	"#c084fc", // purple
	"#f472b6", // pink
	"#a78bfa", // violet
	"#34d399", // emerald
	"#38bdf8", // sky
	"#e879f9", // fuchsia
}

// paletteColor hashes a string to a consistent color from colorPalette.
func paletteColor(s string) string {
	var h uint32
	for _, c := range s {
		h = h*31 + uint32(c)
	}
	return colorPalette[h%uint32(len(colorPalette))]
}

// dupGroupColor returns an inline CSS background style for a duplicate group dot.
func dupGroupColor(group string) template.CSS {
	return template.CSS(fmt.Sprintf("background:%s", paletteColor(group)))
}

// sourceColor returns an inline CSS background style for a source dot.
func sourceColor(source string) template.CSS {
	return template.CSS(fmt.Sprintf("background:%s", paletteColor(source)))
}

// sourceColorVal returns just the color value string (no "background:" prefix).
func sourceColorVal(source string) string {
	return paletteColor(source)
}

// dupGroupLetter returns a short letter label for a duplicate group key.
func dupGroupLetter(group string) string {
	if group == "" {
		return ""
	}
	// Use the first character of the hashed index to produce A, B, C… labels.
	var h uint32
	for _, c := range group {
		h = h*31 + uint32(c)
	}
	return string(rune('A' + h%26))
}

// dupBadgeStyle returns inline CSS for a group badge (color + border + background).
func dupBadgeStyle(group string) template.CSS {
	c := paletteColor(group)
	return template.CSS(fmt.Sprintf("color:%s;border:1px solid %s40;background:%s1a", c, c, c))
}

// tocBadgeClass returns the CSS class for a TOC group header priority badge.
func tocBadgeClass(label string) string {
	switch label {
	case "Must Read":
		return "priority-badge badge-must"
	case "Should Read":
		return "priority-badge badge-should"
	case "May Read":
		return "priority-badge badge-may"
	default:
		return "priority-badge badge-opt"
	}
}

// tocNumClass returns the CSS class for a TOC row number based on read tag.
func tocNumClass(tag string) string {
	switch tag {
	case "Must Read":
		return "toc-num-must"
	case "Should Read":
		return "toc-num-should"
	case "May Read":
		return "toc-num-may"
	default:
		return "toc-num-opt"
	}
}

// priorityRowClass returns the CSS class for an article row's priority rail.
func priorityRowClass(tag string) string {
	switch tag {
	case "Must Read":
		return "must-row"
	case "Should Read":
		return "should-row"
	case "May Read":
		return "may-row"
	default:
		return ""
	}
}

// priorityBadgeClass returns the CSS class for an article priority badge.
func priorityBadgeClass(tag string) string {
	switch tag {
	case "Must Read":
		return "badge-must"
	case "Should Read":
		return "badge-should"
	case "May Read":
		return "badge-may"
	default:
		return "badge-opt"
	}
}

// priorityShort returns the short label used in the priority badge.
func priorityShort(tag string) string {
	switch tag {
	case "Must Read":
		return "MUST"
	case "Should Read":
		return "SHOULD"
	case "May Read":
		return "MAY"
	default:
		return tag
	}
}

// priorityKey returns the filter key used in data-priority attributes.
func priorityKey(tag string) string {
	switch tag {
	case "Must Read":
		return "must"
	case "Should Read":
		return "should"
	case "May Read":
		return "may"
	default:
		return "opt"
	}
}

// scoreBarHTML renders an inline score bar for the articles list.
func scoreBarHTML(score int) template.HTML {
	var fillClass, numClass string
	switch {
	case score >= 90:
		fillClass, numClass = "score-fill score-fill-high", "score-num score-num-high"
	case score >= 75:
		fillClass, numClass = "score-fill score-fill-mid", "score-num score-num-mid"
	default:
		fillClass, numClass = "score-fill score-fill-low", "score-num score-num-low"
	}
	return template.HTML(fmt.Sprintf( //nolint:gosec
		`<div class="score-bar"><div class="score-track"><div class="%s" style="width:%d%%"></div></div><span class="%s">%d</span></div>`,
		fillClass, score, numClass, score,
	))
}

func articleTitle(t string) string {
	if t == "" {
		return "Untitled"
	}
	return t
}

func articleSource(link string) string {
	u, err := url.Parse(link)
	if err != nil || u.Hostname() == "" {
		return link
	}
	return strings.TrimPrefix(u.Hostname(), "www.")
}

type digestIndexTemplateData struct {
	ManifestURL   string
	DigestBaseURL string
}

// RenderDigestIndex generates the index HTML shell. The digest list is
// populated client-side by fetching manifest.json, so the rendered bytes are
// constant for a given template.
func RenderDigestIndex() ([]byte, error) {
	return renderDigestIndexWithPaths("manifest.json", "")
}

func renderDigestIndexWithPaths(manifestURL, digestBaseURL string) ([]byte, error) {
	templateText, err := loadNotificationTemplate("archive-index.html.tmpl")
	if err != nil {
		return nil, fmt.Errorf("failed to load index template: %w", err)
	}

	tmpl, err := template.New("index").Parse(templateText)
	if err != nil {
		return nil, fmt.Errorf("failed to parse index template: %w", err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, digestIndexTemplateData{
		ManifestURL:   manifestURL,
		DigestBaseURL: digestBaseURL,
	}); err != nil {
		return nil, fmt.Errorf("failed to render digest index: %w", err)
	}
	return buf.Bytes(), nil
}
