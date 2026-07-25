package notification

import (
	"encoding/xml"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ma111e/downlink/pkg/models"
	"github.com/ma111e/downlink/pkg/scoring"
	"github.com/ma111e/downlink/pkg/utils"

	log "github.com/sirupsen/logrus"
	gogit "gopkg.in/src-d/go-git.v4"
)

// RSSFilename is the basename of the subscription feed written at the root of
// the Pages branch on every push.
const RSSFilename = "rss.xml"

// downlinkFeedNS is the XML namespace URI for downlink's custom RSS extension
// elements (the per-item technology/product/vendor fields). It is declared as
// xmlns:downlink on <rss>; the value only needs to be a stable unique URI.
const downlinkFeedNS = "https://ma111e.github.io/downlink/ns"

const rssContentNS = "http://purl.org/rss/1.0/modules/content/"

// rssDocument and friends are the minimal RSS 2.0 shape we marshal ourselves.
// gorilla/feeds cannot emit custom namespaced item fields, so we build the XML
// directly, reusing its proven content:encoded CDATA + xmlns attr pattern. The
// downlink:* element names carry a colon that encoding/xml emits verbatim; they
// are valid because xmlns:downlink is declared on the root.
type rssDocument struct {
	XMLName    xml.Name `xml:"rss"`
	Version    string   `xml:"version,attr"`
	ContentNS  string   `xml:"xmlns:content,attr"`
	DownlinkNS string   `xml:"xmlns:downlink,attr"`
	Channel    rssChannel
}

type rssChannel struct {
	XMLName       xml.Name  `xml:"channel"`
	Title         string    `xml:"title"`
	Link          string    `xml:"link"`
	Description   string    `xml:"description"`
	LastBuildDate string    `xml:"lastBuildDate,omitempty"`
	Items         []rssItem `xml:"item"`
}

type rssItem struct {
	XMLName      xml.Name      `xml:"item"`
	Title        string        `xml:"title"`
	Link         string        `xml:"link"`
	Guid         *rssGuid      `xml:"guid,omitempty"`
	PubDate      string        `xml:"pubDate,omitempty"`
	Description  string        `xml:"description"`
	Content      *rssContent   `xml:"content:encoded,omitempty"`
	Technologies *techGroup    `xml:"downlink:technologies,omitempty"`
	Products     *productGroup `xml:"downlink:products,omitempty"`
	Vendors      *vendorGroup  `xml:"downlink:vendors,omitempty"`
}

type rssContent struct {
	XMLName xml.Name `xml:"content:encoded"`
	Content string   `xml:",cdata"`
}

type rssGuid struct {
	XMLName     xml.Name `xml:"guid"`
	Id          string   `xml:",chardata"`
	IsPermaLink string   `xml:"isPermaLink,attr,omitempty"`
}

// techGroup/productGroup/vendorGroup are the three namespaced parent elements
// (e.g. <downlink:technologies>), each holding one namespaced child per term. A
// distinct struct per axis lets encoding/xml emit the correct child element name.
type techGroup struct {
	Terms []string `xml:"downlink:technology"`
}
type productGroup struct {
	Terms []string `xml:"downlink:product"`
}
type vendorGroup struct {
	Terms []string `xml:"downlink:vendor"`
}

// BuildDigestFeeds renders an RSS feed for the given digests (expected
// newest-first). Each digest becomes one feed entry whose HTML body lists, for
// every article in the digest, its TLDR and key points. Links point at the
// published digest HTML page under baseURL/outputDir; when baseURL is empty the
// links are relative to the site root.
func BuildDigestFeeds(digests []models.Digest, outputDir, baseURL string) (rss []byte, err error) {
	updated := time.Now()
	if len(digests) > 0 {
		updated = digests[0].CreatedAt
	}

	channel := rssChannel{
		Title:         "Downlink Digests",
		Link:          joinURL(baseURL, outputDir, ""),
		Description:   "Latest intelligence digests from Downlink",
		LastBuildDate: updated.UTC().Format(time.RFC1123Z),
	}

	for _, d := range digests {
		link := joinURL(baseURL, outputDir, DigestHTMLFilename(d))

		title := strings.TrimSpace(d.Title)
		if title == "" {
			title = "Digest " + d.CreatedAt.UTC().Format("2006-01-02 15:04 UTC")
		}

		item := rssItem{
			Title:       title,
			Link:        link,
			Guid:        &rssGuid{Id: link},
			PubDate:     d.CreatedAt.UTC().Format(time.RFC1123Z),
			Description: digestSummaryText(d.DigestSummary, 300),
			Content:     &rssContent{Content: digestFeedContent(d)},
		}

		tech, products, vendors := aggregateDigestTerms(d)
		if len(tech) > 0 {
			item.Technologies = &techGroup{Terms: tech}
		}
		if len(products) > 0 {
			item.Products = &productGroup{Terms: products}
		}
		if len(vendors) > 0 {
			item.Vendors = &vendorGroup{Terms: vendors}
		}

		channel.Items = append(channel.Items, item)
	}

	doc := rssDocument{
		Version:    "2.0",
		ContentNS:  rssContentNS,
		DownlinkNS: downlinkFeedNS,
		Channel:    channel,
	}

	body, err := xml.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("render rss feed: %w", err)
	}
	return append([]byte(xml.Header), body...), nil
}

// aggregateDigestTerms returns the distinct technologies, products, and vendors
// across the digest's canonical articles (first-seen order). Duplicate
// non-canonical articles are skipped, matching digestFeedContent, so the feed's
// structured fields mirror what the content shows.
func aggregateDigestTerms(d models.Digest) (tech, products, vendors []string) {
	seenT := map[string]bool{}
	seenP := map[string]bool{}
	seenV := map[string]bool{}
	addDistinct := func(dst *[]string, seen map[string]bool, items []string) {
		for _, it := range items {
			it = strings.TrimSpace(it)
			if it == "" || seen[it] {
				continue
			}
			seen[it] = true
			*dst = append(*dst, it)
		}
	}
	for _, da := range d.DigestAnalyses {
		if da.Analysis == nil {
			continue
		}
		if da.DuplicateGroup != "" && !da.IsMostComprehensive {
			continue
		}
		addDistinct(&tech, seenT, da.Analysis.Technologies)
		addDistinct(&products, seenP, da.Analysis.Products)
		addDistinct(&vendors, seenV, da.Analysis.Vendors)
	}
	return tech, products, vendors
}

// digestFeedContent builds the HTML body for a digest feed entry: one section per
// article (skipping duplicate non-canonical articles), highest importance first,
// each showing the article's TLDR and key points.
func digestFeedContent(d models.Digest) string {
	daByArticle := make(map[string]models.DigestAnalysis, len(d.DigestAnalyses))
	scoreByArticle := make(map[string]int, len(d.DigestAnalyses))
	for _, da := range d.DigestAnalyses {
		daByArticle[da.ArticleId] = da
		if da.Analysis != nil {
			scoreByArticle[da.ArticleId] = da.Analysis.ImportanceScore
		}
	}

	articles := append([]models.Article(nil), d.Articles...)
	sort.SliceStable(articles, func(i, j int) bool {
		si, sj := scoreByArticle[articles[i].Id], scoreByArticle[articles[j].Id]
		if si != sj {
			return si > sj
		}
		return articles[i].PublishedAt.After(articles[j].PublishedAt)
	})

	var b strings.Builder
	writeDigestWindow(&b, d)
	if summary := digestSummaryText(d.DigestSummary, 0); summary != "" {
		fmt.Fprintf(&b, "<p>%s</p>\n", html.EscapeString(summary))
	}

	for _, art := range articles {
		da, ok := daByArticle[art.Id]
		if !ok || da.Analysis == nil {
			continue
		}
		// Skip duplicate articles that are not the canonical (most comprehensive) one.
		if da.DuplicateGroup != "" && !da.IsMostComprehensive {
			continue
		}

		title := strings.TrimSpace(articleTitle(art.Title))
		if title == "" {
			continue
		}
		tier := scoring.ReadTier(da.Analysis.ImportanceScore)
		fmt.Fprintf(&b, "<h3>%s — %s</h3>\n", html.EscapeString(title), html.EscapeString(tier))

		if tldr := strings.TrimSpace(da.Analysis.Tldr); tldr != "" {
			fmt.Fprintf(&b, "<p>%s</p>\n", html.EscapeString(tldr))
		}

		if len(da.Analysis.KeyPoints) > 0 {
			b.WriteString("<ul>\n")
			for _, kp := range da.Analysis.KeyPoints {
				kp = strings.TrimSpace(kp)
				if kp == "" {
					continue
				}
				fmt.Fprintf(&b, "<li>%s</li>\n", html.EscapeString(kp))
			}
			b.WriteString("</ul>\n")
		}

		writeTermList(&b, "technologies", "Technologies", da.Analysis.Technologies)
		writeTermList(&b, "products", "Products", da.Analysis.Products)
		writeTermList(&b, "vendors", "Vendors", da.Analysis.Vendors)
	}

	return b.String()
}

// writeDigestWindow writes the digest's coverage window as a paragraph that is
// both human-readable and machine-parsable: the start/end are wrapped in <time>
// elements carrying RFC3339 datetimes, and the duration in a <data> element
// carrying an ISO-8601 duration. These attributes survive in the feed's
// content payload regardless of how a reader sanitizes the display HTML.
func writeDigestWindow(b *strings.Builder, d models.Digest) {
	start := d.CreatedAt
	end := d.CreatedAt.Add(d.TimeWindow)
	s, e := start.UTC(), end.UTC()
	fmt.Fprintf(b,
		"<p class=\"digest-window\">Window: <time datetime=\"%s\">%s</time> → <time datetime=\"%s\">%s</time> (<data value=\"%s\">%s</data>)</p>\n",
		s.Format(time.RFC3339), html.EscapeString(s.Format("02 Jan 15:04")),
		e.Format(time.RFC3339), html.EscapeString(e.Format("02 Jan 15:04")+" UTC"),
		iso8601Duration(d.TimeWindow), html.EscapeString(formatDuration(d.TimeWindow)),
	)
}

// writeTermList writes one axis of an article's classification (technologies,
// products, or vendors) as a paragraph tagged with data-axis, each term wrapped
// in a <data value> element so a consumer can extract individual terms per
// category. Empty or all-blank lists write nothing.
func writeTermList(b *strings.Builder, axis, label string, items []string) {
	var terms []string
	for _, it := range items {
		if it = strings.TrimSpace(it); it != "" {
			terms = append(terms, it)
		}
	}
	if len(terms) == 0 {
		return
	}
	fmt.Fprintf(b, "<p class=\"digest-terms\" data-axis=\"%s\"><strong>%s:</strong> ", axis, label)
	for i, t := range terms {
		if i > 0 {
			b.WriteString(", ")
		}
		esc := html.EscapeString(t)
		fmt.Fprintf(b, "<data value=\"%s\">%s</data>", esc, esc)
	}
	b.WriteString("</p>\n")
}

// iso8601Duration renders a duration as an ISO-8601 time duration (PnHnMnS form,
// e.g. "PT24H", "PT1H30M", "PT45M"). Components that are zero are omitted; a
// zero duration renders "PT0S". Kept in hours/minutes/seconds (no calendar
// days/months) so the value is unambiguous.
func iso8601Duration(d time.Duration) string {
	if d <= 0 {
		return "PT0S"
	}
	total := int64(d / time.Second)
	h := total / 3600
	m := (total % 3600) / 60
	s := total % 60
	var b strings.Builder
	b.WriteString("PT")
	if h > 0 {
		fmt.Fprintf(&b, "%dH", h)
	}
	if m > 0 {
		fmt.Fprintf(&b, "%dM", m)
	}
	if s > 0 {
		fmt.Fprintf(&b, "%dS", s)
	}
	return b.String()
}

// recentFeedDigests fetches the newest digests via the lister, ensures the
// just-pushed digest is included, dedupes by Id, sorts newest-first, and caps
// the result at limit. A nil lister yields just the pushed digest.
func (p *GitHubPagesPublisher) recentFeedDigests(pushed models.Digest, limit int) ([]models.Digest, error) {
	var recent []models.Digest
	if p.listDigests != nil {
		var err error
		recent, err = p.listDigests(limit)
		if err != nil {
			return nil, err
		}
	}
	return mergeDigestsNewestFirst(append([]models.Digest{pushed}, recent...), limit), nil
}

// mergeDigestsNewestFirst dedupes digests by Id, sorts them newest-first by
// CreatedAt, and truncates to limit (0 = no cap).
func mergeDigestsNewestFirst(digests []models.Digest, limit int) []models.Digest {
	seen := make(map[string]bool, len(digests))
	out := make([]models.Digest, 0, len(digests))
	for _, d := range digests {
		if seen[d.Id] {
			continue
		}
		seen[d.Id] = true
		out = append(out, d)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// filterDigestsNewerThan returns digests whose CreatedAt is at or after cutoff.
func filterDigestsNewerThan(digests []models.Digest, cutoff time.Time) []models.Digest {
	out := make([]models.Digest, 0, len(digests))
	for _, d := range digests {
		if !d.CreatedAt.Before(cutoff) {
			out = append(out, d)
		}
	}
	return out
}

// joinURL joins a base URL with path segments, trimming slashes and skipping
// empty segments. A trailing empty segment yields the directory URL. When base
// is empty the result is a root-relative path.
func joinURL(base string, segments ...string) string {
	return utils.JoinURL(base, segments...)
}

// pagesBaseURL returns the public base URL of the published site. An explicit
// cfg.BaseURL always wins; otherwise the canonical GitHub Pages URL is derived
// from repo_url: https://<owner>.github.io/<repo> for a project site, or
// https://<owner>.github.io for a user/org site (repo == "<owner>.github.io").
// A repo_url that can't be parsed yields "" (feed links stay relative, as
// before) so a misconfigured URL never breaks a publish.
func (p *GitHubPagesPublisher) pagesBaseURL() string {
	if p.cfg.BaseURL != "" {
		return p.cfg.BaseURL
	}
	owner, repo, err := parseGitHubRepoURL(p.cfg.RepoURL)
	if err != nil {
		log.WithError(err).Warn("github pages: cannot derive feed base URL from repo_url; feed links will be relative")
		return ""
	}
	base := "https://" + owner + ".github.io"
	if !strings.EqualFold(repo, owner+".github.io") {
		base += "/" + repo
	}
	return base
}

// feedURL returns the absolute URL of the published RSS feed, or "" when no base
// URL is available. Used for the head autodiscovery link and the footer link.
func (p *GitHubPagesPublisher) feedURL() string {
	base := p.pagesBaseURL()
	if base == "" {
		return ""
	}
	return joinURL(base, RSSFilename)
}

// writeAndStageFeeds builds the RSS feed from digests and writes it at the root
// of the Pages clone, staging it in the worktree.
func (p *GitHubPagesPublisher) writeAndStageFeeds(wt *gogit.Worktree, outputDir string, digests []models.Digest) error {
	rss, err := BuildDigestFeeds(digests, outputDir, p.pagesBaseURL())
	if err != nil {
		return fmt.Errorf("github pages: build feeds: %w", err)
	}

	absPath := filepath.Join(p.cfg.CloneDir, RSSFilename)
	if err := os.WriteFile(absPath, rss, 0644); err != nil {
		return fmt.Errorf("github pages: write %s: %w", RSSFilename, err)
	}
	if _, err := wt.Add(RSSFilename); err != nil {
		return fmt.Errorf("github pages: stage %s: %w", RSSFilename, err)
	}
	return nil
}
