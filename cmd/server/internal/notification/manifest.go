package notification

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/ma111e/downlink/pkg/models"
	"github.com/ma111e/downlink/pkg/scoring"
)

// ManifestFilename is the basename of the manifest checked into the Pages branch.
const ManifestFilename = "manifest.json"

// Headline is one article headline in a manifest entry, paired with its
// importance priority ("must"/"should"/"may"/"opt").
type Headline struct {
	Title    string `json:"title"`
	Priority string `json:"priority,omitempty"`
}

// ManifestEntry describes a single published digest in the manifest.
type ManifestEntry struct {
	Filename string `json:"filename"`
	// PeriodStart and PeriodEnd are the canonical, honestly-named bounds of the
	// digest's article-selection window (ISO "2006-01-02 15:04 UTC", machine-readable).
	PeriodStart string `json:"period_start,omitempty"`
	PeriodEnd   string `json:"period_end,omitempty"`
	// StartedAt is a deprecated legacy alias that holds the window END (not the
	// start). It is still written so entries published before period_end existed
	// keep round-tripping and the archive index can fall back to it. Prefer
	// PeriodEnd; remove once all historical entries have aged out.
	StartedAt    string     `json:"started_at"`
	TimeWindow   string     `json:"time_window"`
	ArticleCount int        `json:"article_count"`
	MustCount    int        `json:"must_count"`
	ShouldCount  int        `json:"should_count"`
	MayCount     int        `json:"may_count"`
	OptCount     int        `json:"opt_count"`
	Provider     string     `json:"provider"`
	Model        string     `json:"model"`
	Models       []string   `json:"models,omitempty"` // all unique model names across summary + article analysis
	Title        string     `json:"title,omitempty"`
	Headlines    []Headline `json:"headlines"`
	Summary      string     `json:"summary"`
}

// Manifest is the JSON document checked into the Pages branch listing every
// published digest. The digest index reads it directly in the browser.
type Manifest struct {
	GeneratedAt string          `json:"generated_at"`
	SourceRepo  string          `json:"source_repo"`
	Digests     []ManifestEntry `json:"digests"`
}

// LoadManifest reads the manifest at path. When the file is missing, it returns
// an empty manifest in the current schema.
func LoadManifest(path string) (Manifest, error) {
	data, err := os.ReadFile(path)
	if err == nil {
		var m Manifest
		if jsonErr := json.Unmarshal(data, &m); jsonErr != nil {
			return Manifest{}, fmt.Errorf("parse manifest: %w", jsonErr)
		}
		if m.SourceRepo == "" && m.GeneratedAt == "" {
			return Manifest{SourceRepo: "downlink"}, nil
		}
		if m.SourceRepo == "" {
			m.SourceRepo = "downlink"
		}
		return m, nil
	}
	if !os.IsNotExist(err) {
		return Manifest{}, fmt.Errorf("read manifest: %w", err)
	}

	return Manifest{SourceRepo: "downlink"}, nil
}

// Upsert inserts entry or replaces an existing entry with the same filename,
// then re-sorts newest-first by filename timestamp.
func (m *Manifest) Upsert(entry ManifestEntry) {
	for i, existing := range m.Digests {
		if existing.Filename == entry.Filename {
			m.Digests[i] = entry
			sortDigestsNewestFirst(m.Digests)
			return
		}
	}
	m.Digests = append(m.Digests, entry)
	sortDigestsNewestFirst(m.Digests)
}

// FindByTitle returns the first entry whose Title matches title
// (case-insensitive). The second return value is false when no match is found.
func (m *Manifest) FindByTitle(title string) (ManifestEntry, bool) {
	lower := strings.ToLower(title)
	for _, e := range m.Digests {
		if strings.ToLower(e.Title) == lower {
			return e, true
		}
	}
	return ManifestEntry{}, false
}

// Prune removes entries whose PeriodStart is before cutoff, returning the count
// removed. Entries with a missing or unparseable PeriodStart are kept.
func (m *Manifest) Prune(cutoff time.Time) int {
	const layout = "2006-01-02 15:04 UTC"
	var filtered []ManifestEntry
	for _, e := range m.Digests {
		t, err := time.Parse(layout, e.PeriodStart)
		if err != nil || !t.Before(cutoff) {
			filtered = append(filtered, e)
		}
	}
	removed := len(m.Digests) - len(filtered)
	m.Digests = filtered
	return removed
}

// Remove removes the entry with the given filename from the manifest.
// Returns true if an entry was found and removed.
func (m *Manifest) Remove(filename string) bool {
	for i, e := range m.Digests {
		if e.Filename == filename {
			m.Digests = append(m.Digests[:i], m.Digests[i+1:]...)
			return true
		}
	}
	return false
}

// Write atomically serializes the manifest to path.
func (m Manifest) Write(path string) error {
	if m.SourceRepo == "" {
		m.SourceRepo = "downlink"
	}
	m.GeneratedAt = time.Now().UTC().Format("2006-01-02 15:04 UTC")
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}
	data = append(data, '\n')

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create manifest dir: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "manifest-*.json.tmp")
	if err != nil {
		return fmt.Errorf("create temp manifest: %w", err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("write temp manifest: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("close temp manifest: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename manifest: %w", err)
	}
	return nil
}

// ManifestEntryFromDigest builds a ManifestEntry for the given digest using
// the archive-index manifest schema.
func ManifestEntryFromDigest(d models.Digest) ManifestEntry {
	provider, model := digestProviderLabel(d)
	must, should, may, opt := digestPriorityCounts(d)
	// CreatedAt is the window start (see GenerateDigest); the window ends one
	// TimeWindow later. period_start/period_end are the canonical bounds;
	// started_at is the deprecated legacy alias for the end.
	const layout = "2006-01-02 15:04 UTC"
	periodEnd := d.CreatedAt.UTC().Add(d.TimeWindow).Format(layout)
	return ManifestEntry{
		Filename:     DigestHTMLFilename(d),
		PeriodStart:  d.CreatedAt.UTC().Format(layout),
		PeriodEnd:    periodEnd,
		StartedAt:    periodEnd,
		TimeWindow:   formatDuration(d.TimeWindow),
		ArticleCount: len(d.Articles),
		MustCount:    must,
		ShouldCount:  should,
		MayCount:     may,
		OptCount:     opt,
		Provider:     provider,
		Model:        model,
		Models:       digestAllModelNames(d),
		Title:        d.Title,
		Headlines:    digestHeadlinePreview(d, 0),
		Summary:      digestSummaryText(d.DigestSummary, 220),
	}
}

// digestProviderLabel picks a provider/model label for a digest. Falls back
// to "unknown" when no provider results have been recorded yet.
func digestProviderLabel(d models.Digest) (providerType, modelName string) {
	for _, r := range d.ProviderResults {
		if r.ProviderType != "" {
			return r.ProviderType, r.ModelName
		}
	}
	return "unknown", "unknown"
}

// digestAllModelNames returns the deduplicated list of every model name involved
// in a digest: the summary provider first, then any article-analysis models that
// differ. Used by both the manifest entry and the HTML digest nav bar.
func digestAllModelNames(d models.Digest) []string {
	seen := make(map[string]bool)
	var names []string
	add := func(name string) {
		if name != "" && !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	for _, r := range d.ProviderResults {
		add(r.ModelName)
	}
	for _, da := range d.DigestAnalyses {
		if da.Analysis != nil {
			add(da.Analysis.ModelName)
		}
	}
	return names
}

func digestPriorityCounts(d models.Digest) (must, should, may, opt int) {
	scoreByArticle := make(map[string]int, len(d.DigestAnalyses))
	for _, da := range d.DigestAnalyses {
		if da.Analysis != nil {
			scoreByArticle[da.ArticleId] = da.Analysis.ImportanceScore
		}
	}
	for _, art := range d.Articles {
		switch priorityKeyForScore(scoreByArticle[art.Id]) {
		case "must":
			must++
		case "should":
			should++
		case "may":
			may++
		default:
			opt++
		}
	}
	return must, should, may, opt
}

func priorityKeyForScore(score int) string {
	return scoring.PriorityKey(score)
}

func digestHeadlinePreview(d models.Digest, limit int) []Headline {
	scoreByArticle := make(map[string]int, len(d.DigestAnalyses))
	for _, da := range d.DigestAnalyses {
		if da.Analysis != nil {
			scoreByArticle[da.ArticleId] = da.Analysis.ImportanceScore
		}
	}
	articles := append([]models.Article(nil), d.Articles...)
	sort.SliceStable(articles, func(i, j int) bool {
		si := scoreByArticle[articles[i].Id]
		sj := scoreByArticle[articles[j].Id]
		if si != sj {
			return si > sj
		}
		return articles[i].PublishedAt.After(articles[j].PublishedAt)
	})
	capacity := len(articles)
	if limit > 0 && limit < capacity {
		capacity = limit
	}
	headlines := make([]Headline, 0, capacity)
	for _, art := range articles {
		title := strings.TrimSpace(articleTitle(art.Title))
		if title == "" {
			continue
		}
		headlines = append(headlines, Headline{
			Title:    title,
			Priority: priorityKeyForScore(scoreByArticle[art.Id]),
		})
		if limit > 0 && len(headlines) == limit {
			break
		}
	}
	return headlines
}

var (
	markdownLinkRE       = regexp.MustCompile(`\[([^\]]+)\]\([^)]+\)`)
	markdownListMarkerRE = regexp.MustCompile(`^([-*+]|\d+[.)])\s+`)
	markdownStyleRE      = regexp.MustCompile("[*_`]+")
	whitespaceRE         = regexp.MustCompile(`\s+`)
)

func digestSummaryText(markdown string, maxLen int) string {
	var parts []string
	var headingFallback []string
	for _, line := range strings.Split(markdown, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") {
			heading := strings.TrimSpace(strings.TrimLeft(line, "#"))
			if heading != "" {
				headingFallback = append(headingFallback, heading)
			}
			continue
		}
		line = markdownListMarkerRE.ReplaceAllString(line, "")
		line = markdownLinkRE.ReplaceAllString(line, "$1")
		line = markdownStyleRE.ReplaceAllString(line, "")
		line = strings.TrimSpace(line)
		if line != "" {
			parts = append(parts, line)
		}
	}
	if len(parts) == 0 {
		parts = headingFallback
	}
	parts = stripSummaryLeadHeader(parts)
	text := strings.TrimSpace(whitespaceRE.ReplaceAllString(strings.Join(parts, " "), " "))
	if maxLen <= 0 || len([]rune(text)) <= maxLen {
		return text
	}
	runes := []rune(text)
	cut := maxLen
	for cut > maxLen-30 && cut > 0 && runes[cut-1] != ' ' {
		cut--
	}
	if cut == 0 {
		cut = maxLen
	}
	return strings.TrimSpace(string(runes[:cut])) + "..."
}

func stripSummaryLeadHeader(parts []string) []string {
	if len(parts) < 2 {
		return parts
	}
	first := strings.TrimSpace(parts[0])
	if first == "" {
		return parts[1:]
	}
	words := strings.Fields(first)
	if len(words) > 12 {
		return parts
	}
	lower := strings.ToLower(first)
	if strings.Contains(lower, "overview") ||
		strings.Contains(lower, "executive") ||
		strings.Contains(lower, "summary") ||
		strings.Contains(lower, "digest") ||
		strings.Contains(lower, "brief") {
		return parts[1:]
	}
	return parts
}

// sortDigestsNewestFirst sorts entries by filename desc. The ISO timestamp prefix
// sorts lexicographically, matching newest-first chronological order.
func sortDigestsNewestFirst(entries []ManifestEntry) {
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].Filename > entries[j].Filename
	})
}
