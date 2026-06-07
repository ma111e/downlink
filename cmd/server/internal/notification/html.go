package notification

import (
	"bytes"
	"fmt"
	"github.com/ma111e/downlink/pkg/digestthemes"
	"github.com/ma111e/downlink/pkg/models"
	"github.com/ma111e/downlink/pkg/scoring"
	"github.com/ma111e/downlink/pkg/version"
	"html"
	"html/template"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/gomarkdown/markdown"
	mdhtml "github.com/gomarkdown/markdown/html"
	"github.com/gomarkdown/markdown/parser"
)

// TOCEntry holds data for a single article row in the table of contents.
type TOCEntry struct {
	Id                  string
	Title               string
	Category            string // article category (one of the fixed set), empty if unset
	ImportanceScore     int
	ScoreTip            string // rubric dimension breakdown shown on hover, empty if no dimensions
	ReadTag             string
	DuplicateGroup      string
	IsMostComprehensive bool
}

// RenderedAnalysis holds markdown-converted HTML versions of analysis text fields.
// Text fields are template.HTML because matched entity tags are wrapped in
// <mark class="tag-hl"> at build time (see highlight* helpers).
type RenderedAnalysis struct {
	ProviderType           string
	ModelName              string
	Tldr                   template.HTML
	Justification          template.HTML
	BriefOverview          template.HTML
	StandardSynthesis      template.HTML
	ComprehensiveSynthesis template.HTML
	KeyPoints              []template.HTML
	Insights               []template.HTML
	ReferencedReports      []RenderedReport
}

// RenderedReport is a referenced report prepared for the digest template, with its
// context prose tag-highlighted.
type RenderedReport struct {
	Title     string
	URL       string
	Publisher string
	Category  string
	Primary   bool
	Context   template.HTML
}

// reportsHavePrimary reports whether any report in the list is marked primary.
func reportsHavePrimary(reports []RenderedReport) bool {
	for _, r := range reports {
		if r.Primary {
			return true
		}
	}
	return false
}

// reportsNonPrimaryCount counts the reports not marked primary - these are the ones
// collapsed behind the "+N more" button when the list also has a primary report.
func reportsNonPrimaryCount(reports []RenderedReport) int {
	n := 0
	for _, r := range reports {
		if !r.Primary {
			n++
		}
	}
	return n
}

// ArticleEntry holds enriched article data for template rendering
type ArticleEntry struct {
	Id                  string
	Title               string
	Source              string
	Link                string
	PublishedAt         string
	Category            string   // article category (one of the fixed set), empty if unset
	Tags                []string // entity tags (actors/tools/techniques/country/stakeholders), without '#'
	ImportanceScore     int
	ReadTag             string
	DuplicateGroup      string
	IsMostComprehensive bool
	HasAnalysis         bool
	AnalysisError       string // non-empty when HasAnalysis is false and a classified error is available
	Analysis            *RenderedAnalysis
}

// readTag returns a priority label based on a 0-100 importance score, matching the UI thresholds.
func readTag(score int) string {
	return scoring.ReadTier(score)
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
	Theme            string       // resolved data-theme attribute value
	PaletteCSS       template.CSS // per-theme --pN source-color custom properties
	DigestSummary    template.HTML // kept for backwards compat; OverviewSections is used for rendering
	OverviewSections []OverviewSection
	TOCGroups        []TOCGroup
	ArticleEntries   []ArticleEntry
	Categories       []string       // categories present among articles, for the TOC category filter
	CategoryCounts   map[string]int // TOC rows per category, for the category filter badges
	PriorityCounts   map[string]int // TOC rows per priority key (must/should/may), for the priority filter badges
	Tags             []TagCount     // distinct tags present among TOC rows (with match counts), for the tag filter cloud
	Commit           string
}

// TagCount is a tag and the number of TOC rows that carry it (matching the row-level
// tag filter), used to render per-tag match counts in the filter cloud.
type TagCount struct {
	Name  string
	Count int
}

// digestCategoryOrder is the fixed set of article categories surfaced in the digest,
// in display order. Stale/free-form categories outside this set are ignored at render
// time (no label, excluded from the filter). Keep in sync with the LLM categorize
// task's allowed set (services.allowedCategories).
var digestCategoryOrder = []string{"news", "research", "advisory", "opinion", "guide", "commercial", "sponsored", "announcement"}

var digestCategorySet = map[string]bool{}

func init() {
	for _, c := range digestCategoryOrder {
		digestCategorySet[c] = true
	}
}

// RenderDigestHTML generates a self-contained HTML file for the given digest.
// The digest must have Articles, DigestAnalyses (with Analysis preloaded), and ProviderResults populated.
// theme selects the visual style; an empty string or "dark" uses the default dark theme.
//
// The provider/model switcher in the rendered page is populated client-side
// from manifest.json - the page itself only embeds the digest id and a hash
// of its article set used to filter siblings.
func RenderDigestHTML(digest models.Digest, theme string) ([]byte, error) {
	// Build a lookup: articleId → DigestAnalysis (for duplicate metadata and analysis)
	daByArticle := make(map[string]models.DigestAnalysis, len(digest.DigestAnalyses))
	for _, da := range digest.DigestAnalyses {
		daByArticle[da.ArticleId] = da
	}

	var tocEntries []TOCEntry
	var articleEntries []ArticleEntry

	digestModelName := strings.Join(digestAllModelNames(digest), " · ")

	for _, art := range digest.Articles {
		da := daByArticle[art.Id]

		var analysis *models.ArticleAnalysis
		var importanceScore int
		if da.Analysis != nil {
			analysis = da.Analysis
			importanceScore = da.Analysis.ImportanceScore
		}

		tag := readTag(importanceScore)

		var category string
		if art.CategoryName != nil {
			category = strings.ToLower(strings.TrimSpace(*art.CategoryName))
		}
		if !digestCategorySet[category] {
			category = "" // ignore stale/non-conforming categories (LLM enforces the set going forward)
		}

		var scoreTip string
		if analysis != nil {
			scoreTip = scoreTooltip(analysis.ScoreDimensions)
		}

		tocEntries = append(tocEntries, TOCEntry{
			Id:                  art.Id,
			Title:               articleTitle(art.Title),
			Category:            category,
			ImportanceScore:     importanceScore,
			ScoreTip:            scoreTip,
			ReadTag:             tag,
			DuplicateGroup:      da.DuplicateGroup,
			IsMostComprehensive: da.IsMostComprehensive,
		})

		tags := make([]string, 0, len(art.Tags))
		for _, t := range art.Tags {
			tags = append(tags, t.Name)
		}
		tagRe := compileTagRegexp(tags)

		var rendered *RenderedAnalysis
		if analysis != nil {
			rendered = &RenderedAnalysis{
				ProviderType:           analysis.ProviderType,
				ModelName:              analysis.ModelName,
				Tldr:                   highlightPlain(analysis.Tldr, tagRe),
				Justification:          highlightHTMLFragment(markdownToHTML(analysis.Justification), tagRe),
				BriefOverview:          highlightHTMLFragment(markdownToHTML(analysis.BriefOverview), tagRe),
				StandardSynthesis:      highlightHTMLFragment(markdownToHTML(analysis.StandardSynthesis), tagRe),
				ComprehensiveSynthesis: highlightHTMLFragment(markdownToHTML(analysis.ComprehensiveSynthesis), tagRe),
				KeyPoints:              highlightPlainSlice(analysis.KeyPoints, tagRe),
				Insights:               highlightPlainSlice(analysis.Insights, tagRe),
				ReferencedReports:      renderReports(analysis.ReferencedReports, tagRe),
			}
		}

		articleEntries = append(articleEntries, ArticleEntry{
			Id:                  art.Id,
			Title:               articleTitle(art.Title),
			Source:              articleSource(art.Link),
			Link:                art.Link,
			PublishedAt:         art.PublishedAt.Format("2006-01-02 15:04"),
			Category:            category,
			Tags:                tags,
			ImportanceScore:     importanceScore,
			ReadTag:             tag,
			DuplicateGroup:      da.DuplicateGroup,
			IsMostComprehensive: da.IsMostComprehensive,
			HasAnalysis:         rendered != nil,
			AnalysisError:       digest.AnalysisErrors[art.Id],
			Analysis:            rendered,
		})
	}

	articleCount := len(articleEntries)

	// Collect the categories present among articles, in canonical order, for the TOC filter.
	presentCategories := make(map[string]bool, len(articleEntries))
	for _, e := range articleEntries {
		if e.Category != "" {
			presentCategories[e.Category] = true
		}
	}
	var categories []string
	for _, c := range digestCategoryOrder {
		if presentCategories[c] {
			categories = append(categories, c)
		}
	}

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

	// Count rows per priority and per category for the filter badges. This mirrors the
	// client-side selectors that used to run on load (.toc-row-wrap[data-priority] and
	// [data-category]): one count per TOC row, a cluster counted once via its canonical.
	priorityCounts := make(map[string]int)
	categoryCounts := make(map[string]int)
	for gi := range tocGroups {
		for ri := range tocGroups[gi].Rows {
			row := &tocGroups[gi].Rows[ri]
			entry := row.Entry
			if row.IsCluster {
				entry = row.Canonical
			}
			priorityCounts[priorityKey(entry.ReadTag)]++
			if entry.Category != "" {
				categoryCounts[entry.Category]++
			}
		}
	}

	// Count tag occurrences per TOC row - matching the row-level tag filter, where a
	// cluster row carries only its canonical article's tags - so the filter cloud can
	// show how many rows each tag matches. Ordered by count desc, name asc.
	tagCounts := make(map[string]int)
	for gi := range tocGroups {
		label := tocGroups[gi].Label
		if label != "Must Read" && label != "Should Read" {
			continue
		}
		for ri := range tocGroups[gi].Rows {
			row := &tocGroups[gi].Rows[ri]
			detail := row.Detail
			if row.IsCluster {
				detail = row.CanonDetail
			}
			if detail == nil {
				continue
			}
			for _, t := range detail.Tags {
				tagCounts[t]++
			}
		}
	}
	tags := make([]TagCount, 0, len(tagCounts))
	for t, c := range tagCounts {
		tags = append(tags, TagCount{Name: t, Count: c})
	}
	sort.Slice(tags, func(i, j int) bool {
		if tags[i].Count != tags[j].Count {
			return tags[i].Count > tags[j].Count
		}
		return tags[i].Name < tags[j].Name
	})

	// Highlight the digest-level executive summary against the same tag set that builds the
	// header filter cloud, so every highlighted word maps to a real filter pill.
	summaryTagNames := make([]string, 0, len(tags))
	for _, t := range tags {
		summaryTagNames = append(summaryTagNames, t.Name)
	}
	summaryTagRe := compileTagRegexp(summaryTagNames)

	data := digestTemplateData{
		// CreatedAt is the article-selection window start; show it directly so the
		// digest page matches the archive index's period_start (see ManifestEntryFromDigest).
		StartedAt:        digest.CreatedAt.UTC().Format("2006-01-02 15:04 UTC"),
		ArticleCount:     articleCount,
		ModelName:        digestModelName,
		TimeWindow:       formatDuration(digest.TimeWindow),
		SwipeFilename:    SwipeHTMLFilename(digest),
		DigestTitle:      digest.Title,
		DigestSummary:    markdownToHTML(digest.DigestSummary),
		OverviewSections: parseOverviewSections(digest.DigestSummary, summaryTagRe),
		TOCGroups:        tocGroups,
		ArticleEntries:   articleEntries,
		Categories:       categories,
		CategoryCounts:   categoryCounts,
		PriorityCounts:   priorityCounts,
		Tags:             tags,
		Theme:            normalizeTheme(theme),
		PaletteCSS:       paletteCSS(),
		Commit:           version.Commit,
	}

	funcMap := template.FuncMap{
		"add":                func(a, b int) int { return a + b },
		"slugify":            func(s string) string { return strings.ReplaceAll(s, " ", "-") },
		"joinTags":           func(t []string) string { return strings.Join(t, " ") },
		"paletteVar":         paletteVar,
		"dupGroupLetter":     dupGroupLetter,
		"tocBadgeClass":      tocBadgeClass,
		"tocGroupTooltip":    tocGroupTooltip,
		"tocNumClass":        tocNumClass,
		"priorityRowClass":   priorityRowClass,
		"priorityBadgeClass": priorityBadgeClass,
		"priorityShort":      priorityShort,
		"priorityKey":        priorityKey,
		"scoreBar":           scoreBarHTML,
		"scoreBarTip":        scoreBarTipHTML,
		"hasPrimary":         reportsHavePrimary,
		"nonPrimaryCount":    reportsNonPrimaryCount,
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
func parseOverviewSections(md string, re *regexp.Regexp) []OverviewSection {
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
			Body:  highlightHTMLFragment(markdownToHTML(body), re),
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

// htmlTagSplit matches HTML tags so highlighting can skip them and only touch text nodes.
var htmlTagSplit = regexp.MustCompile(`<[^>]*>`)

// compileTagRegexp builds a single case-insensitive, word-bounded alternation that
// matches any of the given kebab-case tags in prose, treating '-' as interchangeable
// with whitespace/hyphens (e.g. "cobalt-strike" matches "Cobalt Strike"). Returns nil
// when there are no usable tags.
func compileTagRegexp(tags []string) *regexp.Regexp {
	terms := make([]string, 0, len(tags))
	for _, t := range tags {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		// Escape regex metachars, then let '-' match runs of spaces/hyphens.
		// (QuoteMeta leaves '-' unescaped, so replace the literal character.)
		quoted := regexp.QuoteMeta(t)
		quoted = strings.ReplaceAll(quoted, "-", `[\s-]+`)
		terms = append(terms, quoted)
	}
	if len(terms) == 0 {
		return nil
	}
	// Longest terms first so multi-word tags win over any shorter overlap.
	sort.Slice(terms, func(i, j int) bool { return len(terms[i]) > len(terms[j]) })
	return regexp.MustCompile(`(?i)\b(?:` + strings.Join(terms, "|") + `)\b`)
}

// highlightSegments wraps tag matches in <mark class="tag-hl"> within the text portions
// of an HTML string only, never inside tags/attributes. The input must already be valid
// HTML (escaped text + real tags).
func highlightSegments(htmlStr string, re *regexp.Regexp) string {
	if re == nil {
		return htmlStr
	}
	tags := htmlTagSplit.FindAllStringIndex(htmlStr, -1)
	var b strings.Builder
	last := 0
	wrap := func(text string) {
		b.WriteString(re.ReplaceAllString(text, `<mark class="tag-hl">$0</mark>`))
	}
	for _, loc := range tags {
		wrap(htmlStr[last:loc[0]])            // text before this tag
		b.WriteString(htmlStr[loc[0]:loc[1]]) // the tag itself, untouched
		last = loc[1]
	}
	wrap(htmlStr[last:]) // trailing text
	return b.String()
}

// highlightHTMLFragment highlights tag matches inside an already-HTML fragment.
func highlightHTMLFragment(h template.HTML, re *regexp.Regexp) template.HTML {
	if re == nil {
		return h
	}
	return template.HTML(highlightSegments(string(h), re)) //nolint:gosec // input is our own rendered HTML
}

// highlightPlain HTML-escapes a plain string (preserving prior escaping behavior) and
// then highlights tag matches, returning render-ready HTML.
func highlightPlain(s string, re *regexp.Regexp) template.HTML {
	escaped := html.EscapeString(s)
	if re == nil {
		return template.HTML(escaped) //nolint:gosec // value is html-escaped above
	}
	return template.HTML(highlightSegments(escaped, re)) //nolint:gosec // value is html-escaped above
}

// highlightPlainSlice applies highlightPlain to each element.
func highlightPlainSlice(items []string, re *regexp.Regexp) []template.HTML {
	if len(items) == 0 {
		return nil
	}
	out := make([]template.HTML, len(items))
	for i, s := range items {
		out[i] = highlightPlain(s, re)
	}
	return out
}

// renderReports prepares referenced reports for the digest template, tag-highlighting the
// context prose of each.
func renderReports(reports []models.ReferencedReport, re *regexp.Regexp) []RenderedReport {
	if len(reports) == 0 {
		return nil
	}
	out := make([]RenderedReport, len(reports))
	for i, r := range reports {
		out[i] = RenderedReport{
			Title:     r.Title,
			URL:       r.URL,
			Publisher: r.Publisher,
			Category:  r.Category,
			Primary:   r.Primary,
			Context:   highlightPlain(r.Context, re),
		}
	}
	return out
}

// paletteSize is the number of source/duplicate-group colors. Every theme palette below has
// exactly this many entries so a hashed index is valid no matter which theme is active.
const paletteSize = 11

// colorPalette is the palette for dark backgrounds (dark / contrast themes).
var colorPalette = []string{
	"#f87171", // red
	"#fb923c", // orange
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

// lightColorPalette is the palette for the cream "light" theme background. Same hues as
// colorPalette but darker and more saturated so they stay legible on a light surface.
var lightColorPalette = []string{
	"#dc2626", // red
	"#ea580c", // orange
	"#16a34a", // green
	"#0d9488", // teal
	"#2563eb", // blue
	"#9333ea", // purple
	"#db2777", // pink
	"#7c3aed", // violet
	"#059669", // emerald
	"#0284c7", // sky
	"#c026d3", // fuchsia
}

// colorblindPalette is the palette for the colorblind-safe "colorblind" theme. Built from the
// AA-adjusted Okabe-Ito colors the theme already uses, so source dots stay distinct under
// protan/deutan/tritan vision on the cream background.
var colorblindPalette = []string{
	"#0072b2", // blue
	"#c44601", // vermillion
	"#1a7a4f", // bluish green
	"#7a4fa0", // purple
	"#8a6d00", // dark gold
	"#2f79a5", // teal-blue
	"#a23b7a", // reddish purple
	"#00786b", // teal
	"#2b6c8f", // dark sky
	"#b35900", // dark orange
	"#5c6e78", // slate
}

// monoPalette is the palette for the grayscale "mono" theme. Eleven evenly-spaced
// lightness steps from #7a (≥4.5:1 on the #0c0c0c bg) to #f0, no chroma.
var monoPalette = []string{
	"#7a7a7a",
	"#868686",
	"#929292",
	"#9e9e9e",
	"#aaaaaa",
	"#b6b6b6",
	"#c2c2c2",
	"#cecece",
	"#dadada",
	"#e6e6e6",
	"#f0f0f0",
}

// normalizeTheme returns theme if it is a known theme, else "dark". Used to fill
// the <html data-theme> attribute so the server-rendered default is always valid.
func normalizeTheme(theme string) string {
	if digestthemes.Valid(theme) {
		return theme
	}
	return "dark"
}

// paletteIndex hashes a string to a stable index into any of the theme palettes.
func paletteIndex(s string) int {
	var h uint32
	for _, c := range s {
		h = h*31 + uint32(c)
	}
	return int(h % uint32(paletteSize))
}

// paletteVar returns a theme-aware CSS variable reference (var(--pN)) for a source string.
// The --pN custom properties are defined per theme by paletteCSS, so the rendered color
// follows whatever data-theme is active.
func paletteVar(s string) template.CSS {
	return template.CSS("var(--p" + strconv.Itoa(paletteIndex(s)) + ")") //nolint:gosec // index is a small integer we control
}

// paletteCSS emits the per-theme --p0..--pN custom properties from the palettes above.
// Dark/contrast inherit the :root (dark) palette; light, colorblind, and mono override it.
func paletteCSS() template.CSS {
	var b strings.Builder
	writeVars := func(selector string, palette []string) {
		b.WriteString(selector)
		b.WriteByte('{')
		for i, c := range palette {
			fmt.Fprintf(&b, "--p%d:%s;", i, c)
		}
		b.WriteByte('}')
	}
	writeVars(":root", colorPalette)
	writeVars(`html[data-theme="light"]`, lightColorPalette)
	writeVars(`html[data-theme="colorblind"]`, colorblindPalette)
	writeVars(`html[data-theme="mono"]`, monoPalette)
	return template.CSS(b.String()) //nolint:gosec // values come from our own hardcoded palettes
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

// tocGroupTooltip returns the scoring criteria description for a TOC group label.
func tocGroupTooltip(label string) string {
	switch label {
	case "Must Read":
		return "Score 91–100: breaking or high-impact event - active exploitation in the wild, major breach, critical patch, or named threat actor operation."
	case "Should Read":
		return "Score 76–90: specific event or finding with broad relevance, even if not immediately urgent."
	case "May Read":
		return "Score 61–75: specific but narrow or low-urgency event, or solid technical analysis grounded in named concrete cases. Tags excluded from filter."
	case "Optional":
		return "Score ≤60: generic concepts, opinion, trend pieces, evergreen educational content, or low-novelty reporting. Tags excluded from filter."
	case "Unscored":
		return "No importance score assigned (article was not analyzed)."
	default:
		return ""
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

// scoreBarTipHTML renders the score bar, wrapping it in a hover tooltip element
// that reveals the rubric dimension breakdown when tip is non-empty.
func scoreBarTipHTML(score int, tip string) template.HTML {
	bar := scoreBarHTML(score)
	if tip == "" {
		return bar
	}
	return template.HTML(fmt.Sprintf( //nolint:gosec
		`<span class="score-tip" data-tip="%s">%s</span>`,
		html.EscapeString(tip), bar,
	))
}

// scoreTooltip builds a one-line rubric breakdown (ordered by scoring weight) from
// the analysis dimensions. Returns "" when no dimensions are available (e.g. legacy
// vibe-score analyses or unanalyzed articles).
func scoreTooltip(d *scoring.Dimensions) string {
	if d == nil {
		return ""
	}
	parts := []string{
		fmt.Sprintf("Severity %d/4", d.Severity),
		fmt.Sprintf("Specificity %d/4", d.Specificity),
		fmt.Sprintf("Breadth %d/4", d.Breadth),
		fmt.Sprintf("Actionability %d/4", d.Actionability),
		fmt.Sprintf("Novelty %d/4", d.Novelty),
		fmt.Sprintf("Credibility %d/4", d.Credibility),
	}
	tip := strings.Join(parts, " · ")
	if d.IsAggregator {
		tip += " · Aggregator (score capped)"
	}
	return tip
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
	Commit        string
	Theme         string // resolved data-theme attribute value
}

// RenderDigestIndex generates the index HTML shell. The digest list is
// populated client-side by fetching manifest.json, so the rendered bytes are
// constant for a given template.
func RenderDigestIndex(theme string) ([]byte, error) {
	return renderDigestIndexWithPaths("manifest.json", "", theme)
}

func renderDigestIndexWithPaths(manifestURL, digestBaseURL, theme string) ([]byte, error) {
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
		Commit:        version.Commit,
		Theme:         normalizeTheme(theme),
	}); err != nil {
		return nil, fmt.Errorf("failed to render digest index: %w", err)
	}
	return buf.Bytes(), nil
}
