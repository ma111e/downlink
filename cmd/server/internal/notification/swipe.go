package notification

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"text/template"

	"downlink/pkg/models"
)

// swipeArticle is the JSON representation of an article for the swipe triage view.
type swipeArticle struct {
	N                      int                       `json:"n"`
	Title                  string                    `json:"title"`
	Source                 string                    `json:"source"`
	SourceColor            string                    `json:"sourceColor"`
	Link                   string                    `json:"link"`
	Time                   string                    `json:"time"`
	Priority               string                    `json:"priority"`
	Score                  int                       `json:"score"`
	Group                  *string                   `json:"group"`
	GroupColor             *string                   `json:"groupColor"`
	Tldr                   string                    `json:"tldr"`
	BriefOverview          string                    `json:"briefOverview"`
	StandardSynthesis      string                    `json:"standardSynthesis"`
	ComprehensiveSynthesis string                    `json:"comprehensiveSynthesis"`
	KeyPoints              []string                  `json:"keyPoints"`
	Insights               []string                  `json:"insights"`
	ReferencedReports      []models.ReferencedReport `json:"referencedReports"`
	Body                   string                    `json:"body"`
}

type swipeTemplateData struct {
	DigestFilename string
	TimeWindow     string
	ArticlesJSON   string
}

var swipePriorityRank = map[string]int{
	"MUST READ":   0,
	"SHOULD READ": 1,
	"MAY READ":    2,
}

func swipePriorityLabel(tag string) string {
	switch tag {
	case "Must Read":
		return "MUST READ"
	case "Should Read":
		return "SHOULD READ"
	default:
		return "MAY READ"
	}
}

// RenderSwipeHTML generates the self-contained Tinder-style triage page for a digest.
// digestFilename is the filename of the companion list-view page (used for the back link).
func RenderSwipeHTML(digest models.Digest, digestFilename string) ([]byte, error) {
	daByArticle := make(map[string]models.DigestAnalysis, len(digest.DigestAnalyses))
	for _, da := range digest.DigestAnalyses {
		daByArticle[da.ArticleId] = da
	}

	articles := make([]swipeArticle, 0, len(digest.Articles))
	for i, art := range digest.Articles {
		da := daByArticle[art.Id]

		var score int
		var tldr string
		var briefOverview string
		var standardSynthesis string
		var comprehensiveSynthesis string
		var keyPoints []string
		var insights []string
		var referencedReports []models.ReferencedReport
		var body string
		if da.Analysis != nil {
			score = da.Analysis.ImportanceScore
			tldr = da.Analysis.Tldr
			keyPoints = da.Analysis.KeyPoints
			insights = da.Analysis.Insights
			referencedReports = da.Analysis.ReferencedReports
			briefOverview = string(markdownToHTML(da.Analysis.BriefOverview))
			standardSynthesis = string(markdownToHTML(da.Analysis.StandardSynthesis))
			comprehensiveSynthesis = string(markdownToHTML(da.Analysis.ComprehensiveSynthesis))
			src := da.Analysis.BriefOverview
			if src == "" {
				src = da.Analysis.StandardSynthesis
			}
			body = string(markdownToHTML(src))
		}
		if keyPoints == nil {
			keyPoints = []string{}
		}
		if insights == nil {
			insights = []string{}
		}
		if referencedReports == nil {
			referencedReports = []models.ReferencedReport{}
		}

		tag := readTag(score)
		srcDomain := articleSource(art.Link)

		var group, groupColor *string
		if da.DuplicateGroup != "" {
			g := dupGroupLetter(da.DuplicateGroup)
			c := paletteColor(da.DuplicateGroup)
			group = &g
			groupColor = &c
		}

		articles = append(articles, swipeArticle{
			N:                      i + 1,
			Title:                  articleTitle(art.Title),
			Source:                 srcDomain,
			SourceColor:            paletteColor(srcDomain),
			Link:                   art.Link,
			Time:                   art.PublishedAt.Format("15:04"),
			Priority:               swipePriorityLabel(tag),
			Score:                  score,
			Group:                  group,
			GroupColor:             groupColor,
			Tldr:                   tldr,
			BriefOverview:          briefOverview,
			StandardSynthesis:      standardSynthesis,
			ComprehensiveSynthesis: comprehensiveSynthesis,
			KeyPoints:              keyPoints,
			Insights:               insights,
			ReferencedReports:      referencedReports,
			Body:                   body,
		})
	}

	sort.SliceStable(articles, func(i, j int) bool {
		pi := swipePriorityRank[articles[i].Priority]
		pj := swipePriorityRank[articles[j].Priority]
		if pi != pj {
			return pi < pj
		}
		return articles[i].Score > articles[j].Score
	})

	articlesJSON, err := json.Marshal(articles)
	if err != nil {
		return nil, fmt.Errorf("swipe: marshal articles: %w", err)
	}

	data := swipeTemplateData{
		DigestFilename: digestFilename,
		TimeWindow:     formatDuration(digest.TimeWindow),
		ArticlesJSON:   string(articlesJSON),
	}

	templateText, err := loadNotificationTemplate("swipe.html.tmpl")
	if err != nil {
		return nil, fmt.Errorf("swipe: load template: %w", err)
	}

	// Use <% %> delimiters so JSX {{ }} syntax in the template is left untouched.
	tmpl, err := template.New("swipe").Delims("<%", "%>").Parse(templateText)
	if err != nil {
		return nil, fmt.Errorf("swipe: parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("swipe: render: %w", err)
	}
	return buf.Bytes(), nil
}

// SwipeHTMLFilename returns the filename for the swipe triage view of a digest.
func SwipeHTMLFilename(digest models.Digest) string {
	ts := digest.CreatedAt.UTC().Format("2006-01-02_1504")
	return fmt.Sprintf("downlink-swipe-%s.html", ts)
}
