package notification

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/ma111e/downlink/pkg/digestlayouts"
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
	"time"

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
	PlainWords             template.HTML
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
	WindowStart         string // digest window start, human style (e.g. "26 Jun 2026 14:00 UTC")
	WindowEnd           string // digest window end, human style
	WindowRange         string // compact "start → end" range for the nav
	ArticleCount        int
	ModelName           string
	TimeWindow          string
	SwipeFilename       string
	DigestTitle         string
	Theme               string        // resolved data-theme attribute value
	Themes              []themeOption // all known themes, for the picker + pre-paint allowlist
	PaletteCSS          template.CSS  // per-theme --pN source-color custom properties
	StyleCSS            template.CSS  // static page stylesheet (inline mode); empty when external
	StyleLink           template.HTML // <link> to the external stylesheet (external mode); empty when inline
	ScriptJS            template.JS   // page bundle (inline mode); empty when external
	ScriptSrc           template.HTML // <script src> to the external bundle (external mode); empty when inline
	DigestSummary       template.HTML // kept for backwards compat; OverviewSections is used for rendering
	OverviewSections    []OverviewSection
	TOCGroups           []TOCGroup
	ArticleEntries      []ArticleEntry
	Categories          []string             // categories present among articles, for the TOC category filter
	CategoryCounts      map[string]int       // TOC rows per category, for the category filter badges
	PriorityCounts      map[string]int       // TOC rows per priority key (must/should/may), for the priority filter badges
	Tags                []TagCount           // distinct tags present among TOC rows (with match counts), for the tag filter cloud
	HasLearning         bool                 // true when the digest has beginner aids ("In plain words" or glossary terms), gating the Learning switch + popup
	GlossaryJSON        template.JS          // normalized-key → {term, def, type} map baked in for the definition popup
	GlossaryContextJSON template.JS          // articleId → {normalized-key → context} for per-article popup context
	GlossaryPanel       []GlossaryPanelEntry // deduped term list for the right-side glossary drawer
	Commit              string
}

// glossaryJSEntry is one entry in the page's baked-in term→definition lookup.
type glossaryJSEntry struct {
	Term string `json:"term"`
	Def  string `json:"def"`
	Type string `json:"type,omitempty"`
	Tag  string `json:"tag,omitempty"`
	Lvl  int    `json:"lvl,omitempty"` // help tier: 1=advanced, 2=intermediate, 3=beginner
}

// GlossaryPanelEntry is one deduplicated term shown in the right-side glossary drawer
// (term + semantic type + general definition; no per-article context).
type GlossaryPanelEntry struct {
	Term       string
	Type       string
	Definition string
	Aliases    []string // similar surface forms grouped under this term, shown as "also: …"
	Lvl        int      // help tier: 1=advanced, 2=intermediate, 3=beginner
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
// layout selects the template set (a templates/<layout>/ directory); an empty string uses the default.
//
// The provider/model switcher in the rendered page is populated client-side
// from manifest.json - the page itself only embeds the digest id and a hash
// of its article set used to filter siblings.
func RenderDigestHTML(digest models.Digest, layout, theme string, opts ...RenderOption) ([]byte, error) {
	rc := applyRenderOptions(opts)
	layout, err := resolveLayout(layout)
	if err != nil {
		return nil, err
	}
	// Build a lookup: articleId → DigestAnalysis (for duplicate metadata and analysis)
	daByArticle := make(map[string]models.DigestAnalysis, len(digest.DigestAnalyses))
	for _, da := range digest.DigestAnalyses {
		daByArticle[da.ArticleId] = da
	}

	var tocEntries []TOCEntry
	var articleEntries []ArticleEntry

	// Build the digest-wide glossary lookup from the entries this digest references. In-prose
	// highlighting is driven by this set (defined jargon + defined entities), so every highlight
	// resolves to a definition in the popup. When the digest has no glossary, highlighting is off.
	glossaryByKey := make(map[string]glossaryJSEntry, len(digest.DigestGlossary))
	glossaryTerms := make([]string, 0, len(digest.DigestGlossary))
	glossaryPanel := make([]GlossaryPanelEntry, 0, len(digest.DigestGlossary))
	panelIdxByKey := make(map[string]int, len(digest.DigestGlossary))
	for _, dg := range digest.DigestGlossary {
		if dg.Entry == nil {
			continue
		}
		def := dg.Entry.EffectiveDefinition()
		if def == "" {
			continue
		}
		key := dg.Entry.NormalizedKey
		if _, seen := glossaryByKey[key]; seen {
			continue
		}
		tier := models.GlossaryDifficultyTier(dg.Entry.Difficulty)
		glossaryByKey[key] = glossaryJSEntry{Term: dg.Entry.Term, Def: def, Type: dg.Entry.Category, Tag: dg.Entry.TagId, Lvl: tier}
		glossaryTerms = append(glossaryTerms, dg.Entry.Term)
		panelIdxByKey[key] = len(glossaryPanel)
		glossaryPanel = append(glossaryPanel, GlossaryPanelEntry{Term: dg.Entry.Term, Type: dg.Entry.Category, Definition: def, Lvl: tier})
	}
	// Layer in alias surface forms: other phrasings the articles use for a defined term
	// (e.g. "QNAP NAS boxes" / "QNAP NAS devices" for "QNAP NAS"). Each alias highlights in
	// prose and resolves, via the popup map, to its canonical term's definition, and is
	// listed under that term in the drawer. Aliases never shadow a real canonical term.
	for _, da := range digest.DigestAnalyses {
		if da.Analysis == nil {
			continue
		}
		for _, gt := range da.Analysis.GlossaryTerms {
			if len(gt.Aliases) == 0 {
				continue
			}
			canonKey := models.NormalizeGlossaryKey(gt.Term)
			canon, ok := glossaryByKey[canonKey]
			if !ok {
				continue
			}
			for _, alias := range gt.Aliases {
				aliasKey := models.NormalizeGlossaryKey(alias)
				if aliasKey == "" || aliasKey == canonKey {
					continue
				}
				if _, seen := glossaryByKey[aliasKey]; seen {
					continue
				}
				display := strings.TrimSpace(alias)
				glossaryByKey[aliasKey] = canon
				glossaryTerms = append(glossaryTerms, display)
				// Group the similar term under its canonical row in the drawer.
				if idx, ok := panelIdxByKey[canonKey]; ok {
					glossaryPanel[idx].Aliases = append(glossaryPanel[idx].Aliases, display)
				}
			}
		}
	}
	glossaryActive := len(glossaryByKey) > 0
	glossaryRe := compileTagRegexp(glossaryTerms) // nil when no glossary → highlighting is a no-op
	// Consolidated, deduplicated glossary for the right-side panel, sorted by term.
	sort.SliceStable(glossaryPanel, func(i, j int) bool {
		return strings.ToLower(glossaryPanel[i].Term) < strings.ToLower(glossaryPanel[j].Term)
	})

	// Per-article context: key → "why this term matters in this article". Only kept for terms
	// that are part of the digest glossary (so it lines up with what gets highlighted). The popup
	// shows the global definition plus, when present, the context for the article the mark sits in.
	glossaryContext := make(map[string]map[string]string)

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

		// Collect this article's per-term context for terms that are in the digest glossary.
		if glossaryActive && analysis != nil {
			for _, gt := range analysis.GlossaryTerms {
				ctx := strings.TrimSpace(gt.Context)
				if ctx == "" {
					continue
				}
				key := models.NormalizeGlossaryKey(gt.Term)
				if _, ok := glossaryByKey[key]; !ok {
					continue
				}
				if glossaryContext[art.Id] == nil {
					glossaryContext[art.Id] = make(map[string]string)
				}
				glossaryContext[art.Id][key] = ctx
			}
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
		// Highlight defined glossary terms (jargon + entities), not raw tags. When the digest has
		// no glossary, glossaryRe is nil and highlighting is a no-op.
		tagRe := glossaryRe

		var rendered *RenderedAnalysis
		if analysis != nil {
			rendered = &RenderedAnalysis{
				ProviderType:           analysis.ProviderType,
				ModelName:              analysis.ModelName,
				Tldr:                   highlightPlain(analysis.Tldr, tagRe),
				PlainWords:             highlightHTMLFragment(markdownToHTML(analysis.PlainWords), tagRe),
				Justification:          highlightHTMLFragment(markdownToHTML(analysis.Justification), tagRe),
				BriefOverview:          highlightHTMLFragment(markdownToHTML(analysis.BriefOverview), tagRe),
				StandardSynthesis:      mdProseOrEmpty(analysis.StandardSynthesis, tagRe),
				ComprehensiveSynthesis: mdProseOrEmpty(analysis.ComprehensiveSynthesis, tagRe),
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
			PublishedAt:         formatTimestamp(art.PublishedAt),
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

	// In glossary mode, highlight the executive summary against the glossary term set (so its
	// highlights open the definition popup); otherwise leave the summary unhighlighted.
	summaryTagRe := glossaryRe

	// hasLearning means "has beginner aids" — it gates the reader's Learning switch and the
	// data-learning consent gate that reveals the "In plain words" blocks and the glossary drawer.
	hasLearning := glossaryActive
	if !hasLearning {
		for _, e := range articleEntries {
			if e.Analysis != nil && e.Analysis.PlainWords != "" {
				hasLearning = true
				break
			}
		}
	}

	// CreatedAt is the article-selection window start (see GenerateDigest); the
	// window ends one TimeWindow later. Present both ends explicitly so the page
	// matches the archive index's period_start/period_end (see ManifestEntryFromDigest).
	windowStart := digest.CreatedAt
	windowEnd := digest.CreatedAt.Add(digest.TimeWindow)
	data := digestTemplateData{
		WindowStart:         formatTimestamp(windowStart),
		WindowEnd:           formatTimestamp(windowEnd),
		WindowRange:         formatWindowRange(windowStart, windowEnd),
		ArticleCount:        articleCount,
		ModelName:           digestModelName,
		TimeWindow:          formatDuration(digest.TimeWindow),
		SwipeFilename:       SwipeHTMLFilename(digest),
		DigestTitle:         digest.Title,
		DigestSummary:       markdownToHTML(digest.DigestSummary),
		OverviewSections:    parseOverviewSections(digest.DigestSummary, summaryTagRe),
		TOCGroups:           tocGroups,
		ArticleEntries:      articleEntries,
		Categories:          categories,
		CategoryCounts:      categoryCounts,
		PriorityCounts:      priorityCounts,
		Tags:                tags,
		Theme:               resolveTheme(theme),
		Themes:              themeOptions(),
		PaletteCSS:          paletteCSS(),
		HasLearning:         hasLearning,
		GlossaryJSON:        marshalGlossaryJS(glossaryByKey),
		GlossaryContextJSON: marshalGlossaryContextJS(glossaryContext),
		GlossaryPanel:       glossaryPanel,
		Commit:              version.Commit,
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

	styleCSS, err := loadStyleCSS(layout, "digest.css")
	if err != nil {
		return nil, fmt.Errorf("failed to load digest CSS: %w", err)
	}
	body, link := rc.styleFields(styleCSS, "digest.css")
	data.StyleCSS = template.CSS(body)
	data.StyleLink = template.HTML(link)

	scriptJS, err := loadBuiltAsset("digest.js")
	if err != nil {
		return nil, fmt.Errorf("failed to load digest JS: %w", err)
	}
	scriptBody, scriptSrc := rc.scriptFields(scriptJS, "digest.js")
	data.ScriptJS = template.JS(scriptBody)
	data.ScriptSrc = template.HTML(scriptSrc)

	templateText, err := loadNotificationTemplate(layout, "digest.html.tmpl")
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

// digestFileTimestampLayout is the timestamp format embedded in published digest
// and swipe filenames (see DigestHTMLFilename / SwipeHTMLFilename).
const digestFileTimestampLayout = "2006-01-02_1504"

// DigestHTMLFilename returns a filesystem-safe filename for the digest HTML file.
func DigestHTMLFilename(digest models.Digest) string {
	ts := digest.CreatedAt.UTC().Format(digestFileTimestampLayout)
	return fmt.Sprintf("downlink-digest-%s.html", ts)
}

// parseDigestFileTimestamp extracts the UTC timestamp from a published digest or
// swipe filename (downlink-digest-<ts>.html / downlink-swipe-<ts>.html). The
// second return value is false when name is not such a file or the timestamp
// segment does not parse.
func parseDigestFileTimestamp(name string) (time.Time, bool) {
	core := strings.TrimSuffix(name, ".html")
	if core == name {
		return time.Time{}, false
	}
	switch {
	case strings.HasPrefix(core, "downlink-digest-"):
		core = strings.TrimPrefix(core, "downlink-digest-")
	case strings.HasPrefix(core, "downlink-swipe-"):
		core = strings.TrimPrefix(core, "downlink-swipe-")
	default:
		return time.Time{}, false
	}
	ts, err := time.ParseInLocation(digestFileTimestampLayout, core, time.UTC)
	if err != nil {
		return time.Time{}, false
	}
	return ts, true
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
// mdProseOrEmpty renders markdown prose to tag-highlighted HTML, returning an empty
// value when the source is blank or whitespace-only. This keeps optional summary
// fields truly empty so the digest template can hide their tabs (markdownToHTML
// only short-circuits on exactly "").
func mdProseOrEmpty(md string, tagRe *regexp.Regexp) template.HTML {
	if strings.TrimSpace(md) == "" {
		return ""
	}
	return highlightHTMLFragment(markdownToHTML(md), tagRe)
}

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

// marshalGlossaryJS serializes the term→definition lookup into a JS object literal for the
// definition popup. encoding/json escapes '<', '>' and '&' (e.g. "</script>" → "</script>"),
// so the result is safe to inject inside a <script> block via template.JS. Returns "{}" when empty.
func marshalGlossaryJS(m map[string]glossaryJSEntry) template.JS {
	if len(m) == 0 {
		return template.JS("{}")
	}
	b, err := json.Marshal(m)
	if err != nil {
		return template.JS("{}")
	}
	return template.JS(b) //nolint:gosec // values are JSON-encoded; encoding/json escapes HTML-significant runes
}

// marshalGlossaryContextJS serializes the per-article context lookup (articleId → key → context)
// into a JS object literal, with the same escaping guarantees as marshalGlossaryJS.
func marshalGlossaryContextJS(m map[string]map[string]string) template.JS {
	if len(m) == 0 {
		return template.JS("{}")
	}
	b, err := json.Marshal(m)
	if err != nil {
		return template.JS("{}")
	}
	return template.JS(b) //nolint:gosec // values are JSON-encoded; encoding/json escapes HTML-significant runes
}

// nonAlphanumRun matches a maximal run of non-alphanumeric characters.
var nonAlphanumRun = regexp.MustCompile(`[^a-zA-Z0-9]+`)

// compileTagRegexp builds a single case-insensitive, word-bounded alternation that matches any
// of the given terms in prose, treating any internal run of non-alphanumeric characters as
// interchangeable with any other separator/punctuation (e.g. "cobalt-strike" matches
// "Cobalt Strike", "wscript.exe" matches "wscript-exe", "HTTP/3" matches "HTTP 3"). This
// separator equivalence MUST agree with NormalizeGlossaryKey (pkg/models/glossary.go) and the JS
// glossaryKey() normalizer in the digest template. Returns nil when there are no usable terms.
func compileTagRegexp(tags []string) *regexp.Regexp {
	terms := make([]string, 0, len(tags))
	for _, t := range tags {
		t = strings.TrimSpace(t)
		if p := termPattern(t); p != "" {
			terms = append(terms, p)
		}
	}
	if len(terms) == 0 {
		return nil
	}
	// Longest terms first so multi-word tags win over any shorter overlap.
	sort.Slice(terms, func(i, j int) bool { return len(terms[i]) > len(terms[j]) })
	return regexp.MustCompile(`(?i)\b(?:` + strings.Join(terms, "|") + `)\b`)
}

// termPattern turns a single term into a word-bounded regex fragment whose alphanumeric chunks
// are matched literally and whose internal punctuation/whitespace runs match any separator run.
// Leading/trailing non-alphanumeric runs are dropped so the \b anchors land on word characters.
// Returns "" for a term with no alphanumeric content.
func termPattern(t string) string {
	chunks := nonAlphanumRun.Split(t, -1)
	parts := make([]string, 0, len(chunks))
	for _, c := range chunks {
		if c != "" {
			parts = append(parts, regexp.QuoteMeta(c))
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, `[^a-zA-Z0-9]+`)
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
// colorPalette but lighter and more saturated for vivid source dots on a light surface.
var lightColorPalette = []string{
	"#ef4444", // red
	"#f97316", // orange
	"#22c55e", // green
	"#14b8a6", // teal
	"#3b82f6", // blue
	"#a855f7", // purple
	"#ec4899", // pink
	"#8b5cf6", // violet
	"#10b981", // emerald
	"#0ea5e9", // sky
	"#d946ef", // fuchsia
}

// colorblindPalette is the palette for the colorblind-safe "colorblind" theme. Built from the
// Okabe-Ito colors the theme uses, tuned more saturated but keeping their hues so source dots
// stay distinct under protan/deutan/tritan vision and AA-legible (>=4.5:1) on the cream bg.
var colorblindPalette = []string{
	"#0071b0", // blue
	"#be4401", // vermillion
	"#0d7b4a", // bluish green
	"#8f4acc", // purple
	"#856900", // dark gold
	"#1972a7", // teal-blue
	"#c42787", // reddish purple
	"#007a6d", // teal
	"#1a72a1", // dark sky
	"#aa5400", // dark orange
	"#517082", // slate
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

// pastelPalette is the palette for the soft "pastel" theme's cream background. Medium-depth,
// slightly dusty tones (same hue order as colorPalette) so source dots stay legible on #faf6e6.
var pastelPalette = []string{
	"#e0655c", // coral
	"#e08a2e", // orange
	"#2f9e63", // green
	"#1f9d8f", // teal
	"#4a90d9", // blue
	"#8c6dd0", // purple
	"#de6b94", // pink
	"#a05fc0", // violet
	"#2faa8e", // emerald
	"#3f8fd0", // sky
	"#c267c0", // fuchsia
}

// themeOption is a registry theme shaped for the in-page picker and the pre-paint
// allowlist. Both are rendered from themeOptions() so a theme added to digestthemes
// shows up in every page without editing the templates' option/allowlist lists.
type themeOption struct {
	Value string // data-theme value
	Label string // uppercased display label
}

// themeOptions returns every known theme in display order, for the theme picker.
func themeOptions() []themeOption {
	all := digestthemes.All()
	opts := make([]themeOption, len(all))
	for i, t := range all {
		opts[i] = themeOption{Value: t.Name, Label: strings.ToUpper(t.Name)}
	}
	return opts
}

// firstPaintTheme is the color theme baked into the <html data-theme> attribute for
// the server-rendered first paint. Color selection is otherwise entirely client-side
// (the in-page dropdown + localStorage override this immediately).
const firstPaintTheme = "dark"

// resolveTheme returns a profile's first-paint theme: the requested theme when it
// is a known palette, otherwise the default. An empty/invalid theme keeps today's
// behavior (firstPaintTheme).
func resolveTheme(theme string) string {
	if digestthemes.Valid(theme) {
		return theme
	}
	return firstPaintTheme
}

// resolveLayout returns the layout to render: the default when layout is empty, or an
// error when a non-empty layout is not a known one (so typos surface instead of
// silently falling back).
func resolveLayout(layout string) (string, error) {
	if layout == "" {
		return digestlayouts.Default(), nil
	}
	// A layout is valid if it's compiled in OR supplied on disk under the
	// configured layouts directory (an operator/profile template pack).
	if !digestlayouts.Valid(layout) && !OnDiskLayoutExists(layout) {
		return "", fmt.Errorf("unknown layout %q", layout)
	}
	return layout, nil
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
// Dark/contrast inherit the :root (dark) palette; light, colorblind, mono, and pastel override it.
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
	writeVars(`html[data-theme="pastel"]`, pastelPalette)
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
	if d.IsPromotional {
		tip += " · Promotional (score capped)"
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

// siteHomepage derives the blog/site root (scheme + host) from a feed URL, so a
// source links to the site itself rather than its raw RSS/Atom endpoint. Falls
// back to the original link when it can't be parsed.
func siteHomepage(feedURL string) string {
	u, err := url.Parse(feedURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return feedURL
	}
	return u.Scheme + "://" + u.Host + "/"
}

type digestIndexTemplateData struct {
	ManifestURL   string
	DigestBaseURL string
	Commit        string
	Theme         string        // resolved data-theme attribute value
	Themes        []themeOption // all known themes, for the picker + pre-paint allowlist
	StyleCSS      template.CSS  // static page stylesheet (inline mode); empty when external
	StyleLink     template.HTML // <link> to the external stylesheet (external mode); empty when inline
	ScriptJS      template.JS   // page bundle (inline mode); empty when external
	ScriptSrc     template.HTML // <script src> to the external bundle (external mode); empty when inline
}

// RenderDigestIndex generates the index HTML shell. The digest list is
// populated client-side by fetching manifest.json, so the rendered bytes are
// constant for a given template.
func RenderDigestIndex(layout, theme string, opts ...RenderOption) ([]byte, error) {
	return renderDigestIndexWithPaths("manifest.json", "", layout, theme, opts...)
}

func renderDigestIndexWithPaths(manifestURL, digestBaseURL, layout, theme string, opts ...RenderOption) ([]byte, error) {
	rc := applyRenderOptions(opts)
	layout, err := resolveLayout(layout)
	if err != nil {
		return nil, err
	}
	templateText, err := loadNotificationTemplate(layout, "archive-index.html.tmpl")
	if err != nil {
		return nil, fmt.Errorf("failed to load index template: %w", err)
	}
	styleCSS, err := loadStyleCSS(layout, "archive-index.css")
	if err != nil {
		return nil, fmt.Errorf("failed to load index CSS: %w", err)
	}
	scriptJS, err := loadBuiltAsset("archive-index.js")
	if err != nil {
		return nil, fmt.Errorf("failed to load index JS: %w", err)
	}

	tmpl, err := template.New("index").Parse(templateText)
	if err != nil {
		return nil, fmt.Errorf("failed to parse index template: %w", err)
	}
	body, link := rc.styleFields(styleCSS, "archive-index.css")
	scriptBody, scriptSrc := rc.scriptFields(scriptJS, "archive-index.js")
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, digestIndexTemplateData{
		ManifestURL:   manifestURL,
		DigestBaseURL: digestBaseURL,
		Commit:        version.Commit,
		Theme:         resolveTheme(theme),
		Themes:        themeOptions(),
		StyleCSS:      template.CSS(body),
		StyleLink:     template.HTML(link),
		ScriptJS:      template.JS(scriptBody),
		ScriptSrc:     template.HTML(scriptSrc),
	}); err != nil {
		return nil, fmt.Errorf("failed to render digest index: %w", err)
	}
	return buf.Bytes(), nil
}

// sourceEntry is one feed row on the sources page.
type sourceEntry struct {
	Title string
	URL   string
	Host  string
}

type sourcesTemplateData struct {
	Theme     string        // resolved data-theme attribute value
	Themes    []themeOption // all known themes, for the picker + pre-paint allowlist
	Commit    string
	Sources   []sourceEntry
	StyleCSS  template.CSS  // static page stylesheet (inline mode); empty when external
	StyleLink template.HTML // <link> to the external stylesheet (external mode); empty when inline
	ScriptJS  template.JS   // page bundle (inline mode); empty when external
	ScriptSrc template.HTML // <script src> to the external bundle (external mode); empty when inline
}

// RenderSourcesPage generates the standalone "sources" page listing every
// enabled feed. The feeds are embedded server-side, so the rendered bytes are
// self-contained and need no client-side fetch. Disabled feeds are omitted.
func RenderSourcesPage(feeds []models.Feed, layout, theme string, opts ...RenderOption) ([]byte, error) {
	rc := applyRenderOptions(opts)
	layout, err := resolveLayout(layout)
	if err != nil {
		return nil, err
	}
	templateText, err := loadNotificationTemplate(layout, "sources.html.tmpl")
	if err != nil {
		return nil, fmt.Errorf("failed to load sources template: %w", err)
	}
	styleCSS, err := loadStyleCSS(layout, "sources.css")
	if err != nil {
		return nil, fmt.Errorf("failed to load sources CSS: %w", err)
	}
	scriptJS, err := loadBuiltAsset("sources.js")
	if err != nil {
		return nil, fmt.Errorf("failed to load sources JS: %w", err)
	}

	tmpl, err := template.New("sources").Parse(templateText)
	if err != nil {
		return nil, fmt.Errorf("failed to parse sources template: %w", err)
	}

	var entries []sourceEntry
	for _, f := range feeds {
		if f.Enabled != nil && !*f.Enabled {
			continue
		}
		title := f.Title
		if title == "" {
			title = articleSource(f.URL)
		}
		entries = append(entries, sourceEntry{
			Title: title,
			URL:   siteHomepage(f.URL),
			Host:  articleSource(f.URL),
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		return strings.ToLower(entries[i].Title) < strings.ToLower(entries[j].Title)
	})

	body, link := rc.styleFields(styleCSS, "sources.css")
	scriptBody, scriptSrc := rc.scriptFields(scriptJS, "sources.js")
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, sourcesTemplateData{
		Theme:     resolveTheme(theme),
		Themes:    themeOptions(),
		Commit:    version.Commit,
		Sources:   entries,
		StyleCSS:  template.CSS(body),
		StyleLink: template.HTML(link),
		ScriptJS:  template.JS(scriptBody),
		ScriptSrc: template.HTML(scriptSrc),
	}); err != nil {
		return nil, fmt.Errorf("failed to render sources page: %w", err)
	}
	return buf.Bytes(), nil
}
