package notification

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"net/url"
	"sort"
	"strings"

	"github.com/ma111e/downlink/pkg/models"
	"github.com/ma111e/downlink/pkg/utils"
	"github.com/ma111e/downlink/pkg/version"
)

// reportSource is one article that referenced a report, shown under the report
// on the reports page.
type reportSource struct {
	Title       string `json:"title"`
	Link        string `json:"link"`
	PublishedAt string `json:"publishedAt"`
	Context     string `json:"context"`     // how this article framed the report
	Description string `json:"description"` // the article's own summary (TLDR)
}

// aggregatedReport is a referenced report deduplicated by URL across every
// article that links to it. Tags are the union of the source articles' tags.
type aggregatedReport struct {
	URL       string         `json:"url"`
	Title     string         `json:"title"`
	Publisher string         `json:"publisher"`
	Category  string         `json:"category"`
	Primary   bool           `json:"primary"`  // true if any source marked it primary
	RefCount  int            `json:"refCount"` // distinct source articles
	Tags      []string       `json:"tags"`
	Sources   []reportSource `json:"sources"`
}

// normalizeReportURL produces a dedup key for a report URL: lowercased scheme and
// host, no fragment, no trailing slash. Unparseable inputs fall back to a trimmed,
// lowercased, slash-stripped string so they still collapse consistently.
func normalizeReportURL(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	u, err := url.Parse(s)
	if err != nil || u.Host == "" {
		return strings.ToLower(strings.TrimRight(s, "/"))
	}
	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.ToLower(u.Host)
	u.Fragment = ""
	return strings.TrimRight(u.String(), "/")
}

// aggregateReports collapses every referenced report across the given digests
// into one entry per URL, merging the tags and source articles it was referenced
// from. The result is sorted most-referenced first (then by title) as a sensible
// default; the page lets the reader re-sort client-side.
func aggregateReports(digests []models.Digest) []aggregatedReport {
	type acc struct {
		rep        *aggregatedReport
		tagSet     map[string]bool
		articleSet map[string]bool // dedup source articles across digests
	}
	byURL := make(map[string]*acc)
	order := make([]string, 0)

	for _, d := range digests {
		artByID := make(map[string]models.Article, len(d.Articles))
		for _, a := range d.Articles {
			artByID[a.Id] = a
		}
		for _, da := range d.DigestAnalyses {
			if da.Analysis == nil {
				continue
			}
			art, haveArt := artByID[da.ArticleId]
			for _, r := range da.Analysis.ReferencedReports {
				key := normalizeReportURL(r.URL)
				if key == "" {
					continue
				}
				a := byURL[key]
				if a == nil {
					a = &acc{
						rep:        &aggregatedReport{URL: r.URL, Title: r.Title, Publisher: r.Publisher, Category: r.Category},
						tagSet:     make(map[string]bool),
						articleSet: make(map[string]bool),
					}
					byURL[key] = a
					order = append(order, key)
				}
				rep := a.rep
				if rep.Title == "" {
					rep.Title = r.Title
				}
				if rep.Publisher == "" {
					rep.Publisher = r.Publisher
				}
				if rep.Category == "" {
					rep.Category = r.Category
				}
				if r.Primary {
					rep.Primary = true
				}

				// Count and record each distinct source article once. A report
				// referenced by the same article across two digests is one source.
				srcKey := da.ArticleId
				if haveArt && art.Link != "" {
					srcKey = art.Link
				}
				if a.articleSet[srcKey] {
					continue
				}
				a.articleSet[srcKey] = true
				rep.RefCount++

				if haveArt {
					for _, t := range art.Tags {
						name := strings.TrimSpace(t.Name)
						if name != "" && !a.tagSet[name] {
							a.tagSet[name] = true
							rep.Tags = append(rep.Tags, name)
						}
					}
				}

				desc := strings.TrimSpace(da.Analysis.Tldr)
				if desc == "" {
					desc = strings.TrimSpace(da.Analysis.BriefOverview)
				}
				src := reportSource{Context: strings.TrimSpace(r.Context), Description: desc}
				if haveArt {
					src.Title = articleTitle(art.Title)
					src.Link = art.Link
					if !art.PublishedAt.IsZero() {
						src.PublishedAt = art.PublishedAt.UTC().Format("2006-01-02")
					}
				}
				rep.Sources = append(rep.Sources, src)
			}
		}
	}

	out := make([]aggregatedReport, 0, len(order))
	for _, key := range order {
		rep := byURL[key].rep
		sort.Strings(rep.Tags)
		out = append(out, *rep)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].RefCount != out[j].RefCount {
			return out[i].RefCount > out[j].RefCount
		}
		return strings.ToLower(out[i].Title) < strings.ToLower(out[j].Title)
	})
	return out
}

type reportsTemplateData struct {
	Theme       string        // resolved data-theme attribute value
	Themes      []themeOption // all known themes, for the picker + pre-paint allowlist
	Commit      string
	ReportCount int
	ReportsJSON template.JS   // marshaled []aggregatedReport for client-side search
	StyleCSS    template.CSS  // static page stylesheet (inline mode); empty when external
	StyleLink   template.HTML // <link> to the external stylesheet (external mode); empty when inline
}

// RenderReportsPageForDigests aggregates the referenced reports across the given
// digests and renders the standalone reports page. It is the exported entry point
// for callers outside this package (the publisher and the dev preview server),
// since the aggregated report type is internal.
func RenderReportsPageForDigests(digests []models.Digest, layout, theme string, opts ...RenderOption) ([]byte, error) {
	return RenderReportsPage(aggregateReports(digests), layout, theme, opts...)
}

// RenderReportsPage generates the standalone "reports" page. Every referenced
// report is embedded as JSON so search, tag/category filtering, and sorting all
// run client-side with no fetch.
func RenderReportsPage(reports []aggregatedReport, layout, theme string, opts ...RenderOption) ([]byte, error) {
	rc := applyRenderOptions(opts)
	layout, err := resolveLayout(layout)
	if err != nil {
		return nil, err
	}
	templateText, err := loadNotificationTemplate(layout, "reports.html.tmpl")
	if err != nil {
		return nil, fmt.Errorf("failed to load reports template: %w", err)
	}
	styleCSS, err := loadNotificationTemplate(layout, "reports.css")
	if err != nil {
		return nil, fmt.Errorf("failed to load reports CSS: %w", err)
	}
	tmpl, err := template.New("reports").Parse(templateText)
	if err != nil {
		return nil, fmt.Errorf("failed to parse reports template: %w", err)
	}

	if reports == nil {
		reports = []aggregatedReport{}
	}
	payload, err := json.Marshal(reports)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal reports: %w", err)
	}

	body, link := rc.styleFields(utils.StripCSSComments(styleCSS), "reports.css")
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, reportsTemplateData{
		Theme:       resolveTheme(theme),
		Themes:      themeOptions(),
		Commit:      version.Commit,
		ReportCount: len(reports),
		ReportsJSON: template.JS(payload),
		StyleCSS:    template.CSS(body),
		StyleLink:   template.HTML(link),
	}); err != nil {
		return nil, fmt.Errorf("failed to render reports page: %w", err)
	}
	return buf.Bytes(), nil
}
