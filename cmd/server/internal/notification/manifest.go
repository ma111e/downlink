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

	"downlink/pkg/models"
)

// ManifestFilename is the basename of the manifest checked into the Pages branch.
const ManifestFilename = "manifest.json"

// ManifestEntry describes a single published digest in the manifest.
type ManifestEntry struct {
	Filename           string   `json:"filename"`
	StartedAt          string   `json:"started_at"`
	TimeWindow         string   `json:"time_window"`
	ArticleCount       int      `json:"article_count"`
	MustCount          int      `json:"must_count"`
	ShouldCount        int      `json:"should_count"`
	MayCount           int      `json:"may_count"`
	OptCount           int      `json:"opt_count"`
	Provider           string   `json:"provider"`
	Model              string   `json:"model"`
	Title              string   `json:"title,omitempty"`
	Headlines          []string `json:"headlines"`
	HeadlinePriorities []string `json:"headline_priorities,omitempty"`
	Summary            string   `json:"summary"`
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
	headlines, headlinePriorities := digestHeadlinePreview(d, 0)
	return ManifestEntry{
		Filename:           DigestHTMLFilename(d),
		StartedAt:          d.CreatedAt.UTC().Format("2006-01-02 15:04 UTC"),
		TimeWindow:         formatDuration(d.TimeWindow),
		ArticleCount:       len(d.Articles),
		MustCount:          must,
		ShouldCount:        should,
		MayCount:           may,
		OptCount:           opt,
		Provider:           provider,
		Model:              model,
		Title:              d.Title,
		Headlines:          headlines,
		HeadlinePriorities: headlinePriorities,
		Summary:            digestSummaryText(d.DigestSummary, 220),
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
	switch {
	case score >= 90:
		return "must"
	case score >= 75:
		return "should"
	case score >= 60:
		return "may"
	default:
		return "opt"
	}
}

func digestHeadlines(d models.Digest, limit int) []string {
	headlines, _ := digestHeadlinePreview(d, limit)
	return headlines
}

func digestHeadlinePreview(d models.Digest, limit int) ([]string, []string) {
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
	headlines := make([]string, 0, capacity)
	priorities := make([]string, 0, capacity)
	for _, art := range articles {
		title := strings.TrimSpace(articleTitle(art.Title))
		if title == "" {
			continue
		}
		headlines = append(headlines, title)
		priorities = append(priorities, priorityKeyForScore(scoreByArticle[art.Id]))
		if limit > 0 && len(headlines) == limit {
			break
		}
	}
	return headlines, priorities
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

// sortDigestsNewestFirst sorts entries by filename desc — ISO timestamp prefix
// sorts lexicographically, matching newest-first chronological order.
func sortDigestsNewestFirst(entries []ManifestEntry) {
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].Filename > entries[j].Filename
	})
}
