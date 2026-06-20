package notification

import (
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

	"github.com/gorilla/feeds"
	gogit "gopkg.in/src-d/go-git.v4"
)

// RSSFilename and AtomFilename are the basenames of the subscription feeds
// written at the root of the Pages branch on every push.
const (
	RSSFilename  = "rss.xml"
	AtomFilename = "atom.xml"
)

// BuildDigestFeeds renders RSS and Atom feeds for the given digests (expected
// newest-first). Each digest becomes one feed entry whose HTML body lists, for
// every article in the digest, its TLDR and key points. Links point at the
// published digest HTML page under baseURL/outputDir; when baseURL is empty the
// links are relative to the site root.
func BuildDigestFeeds(digests []models.Digest, outputDir, baseURL string) (rss, atom []byte, err error) {
	now := time.Now()
	updated := now
	if len(digests) > 0 {
		updated = digests[0].CreatedAt
	}

	feed := &feeds.Feed{
		Title:       "Downlink Digests",
		Link:        &feeds.Link{Href: joinURL(baseURL, outputDir, "")},
		Description: "Latest intelligence digests from Downlink",
		Created:     updated,
		Updated:     updated,
	}

	for _, d := range digests {
		link := joinURL(baseURL, outputDir, DigestHTMLFilename(d))
		created := d.CreatedAt
		itemUpdated := d.CreatedAt.Add(d.TimeWindow)

		title := strings.TrimSpace(d.Title)
		if title == "" {
			title = "Digest " + d.CreatedAt.UTC().Format("2006-01-02 15:04 UTC")
		}

		feed.Items = append(feed.Items, &feeds.Item{
			Title:       title,
			Link:        &feeds.Link{Href: link},
			Id:          link,
			Created:     created,
			Updated:     itemUpdated,
			Description: digestSummaryText(d.DigestSummary, 300),
			Content:     digestFeedContent(d),
		})
	}

	rssStr, err := feed.ToRss()
	if err != nil {
		return nil, nil, fmt.Errorf("render rss feed: %w", err)
	}
	atomStr, err := feed.ToAtom()
	if err != nil {
		return nil, nil, fmt.Errorf("render atom feed: %w", err)
	}
	return []byte(rssStr), []byte(atomStr), nil
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

// writeAndStageFeeds builds the RSS and Atom feeds from digests and writes them
// at the root of the Pages clone, staging both in the worktree.
func (p *GitHubPagesPublisher) writeAndStageFeeds(wt *gogit.Worktree, outputDir string, digests []models.Digest) error {
	rss, atom, err := BuildDigestFeeds(digests, outputDir, p.cfg.BaseURL)
	if err != nil {
		return fmt.Errorf("github pages: build feeds: %w", err)
	}

	for name, data := range map[string][]byte{RSSFilename: rss, AtomFilename: atom} {
		absPath := filepath.Join(p.cfg.CloneDir, name)
		if err := os.WriteFile(absPath, data, 0644); err != nil {
			return fmt.Errorf("github pages: write %s: %w", name, err)
		}
		if _, err := wt.Add(name); err != nil {
			return fmt.Errorf("github pages: stage %s: %w", name, err)
		}
	}
	return nil
}
