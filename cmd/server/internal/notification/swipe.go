package notification

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"sort"

	"github.com/ma111e/downlink/pkg/models"
)

// swipeArticle is the JSON representation of an article for the swipe triage view.
type swipeArticle struct {
	N                      int                       `json:"n"`
	Title                  string                    `json:"title"`
	Source                 string                    `json:"source"`
	SourceColorIdx         int                       `json:"sourceColorIdx"`
	Link                   string                    `json:"link"`
	Time                   string                    `json:"time"`
	Priority               string                    `json:"priority"`
	Score                  int                       `json:"score"`
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
	DigestTitle    string
	TimeWindow     string
	ArticlesJSON   template.JS   // marshaled []swipeArticle for the #dl-articles island
	MetaJSON       template.JS   // {digest, window, themes} for the #dl-meta island
	PaletteCSS     template.CSS  // per-theme --pN source-color custom properties
	StyleCSS       template.CSS  // static page stylesheet (inline mode); empty when external
	StyleLink      template.HTML // <link> to the external stylesheet (external mode); empty when inline
	ScriptJS       template.JS   // page bundle (inline mode); empty when external
	ScriptSrc      template.HTML // <script src> to the external bundle (external mode); empty when inline
	Theme          string        // resolved data-theme attribute value
	Themes         []themeOption // all known themes, for the pre-paint allowlist
}

// swipeMeta is the #dl-meta island payload the swipe bundle reads. Field names
// match what main.tsx expects (lowercase JSON keys).
type swipeMeta struct {
	Digest string           `json:"digest"`
	Window string           `json:"window"`
	Themes []swipeThemeMeta `json:"themes"`
}

type swipeThemeMeta struct {
	Value string `json:"value"`
	Label string `json:"label"`
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
func RenderSwipeHTML(digest models.Digest, digestFilename string, layout, theme string, opts ...RenderOption) ([]byte, error) {
	rc := applyRenderOptions(opts)
	layout, err := resolveLayout(layout)
	if err != nil {
		return nil, err
	}
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
			standardSynthesis = string(mdProseOrEmpty(da.Analysis.StandardSynthesis, nil))
			comprehensiveSynthesis = string(mdProseOrEmpty(da.Analysis.ComprehensiveSynthesis, nil))
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

		articles = append(articles, swipeArticle{
			N:                      i + 1,
			Title:                  articleTitle(art.Title),
			Source:                 srcDomain,
			SourceColorIdx:         paletteIndex(srcDomain),
			Link:                   art.Link,
			Time:                   art.PublishedAt.Format("15:04"),
			Priority:               swipePriorityLabel(tag),
			Score:                  score,
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

	themeOpts := themeOptions()
	metaThemes := make([]swipeThemeMeta, len(themeOpts))
	for i, t := range themeOpts {
		metaThemes[i] = swipeThemeMeta{Value: t.Value, Label: t.Label}
	}
	metaJSON, err := json.Marshal(swipeMeta{
		Digest: digestFilename,
		Window: formatDuration(digest.TimeWindow),
		Themes: metaThemes,
	})
	if err != nil {
		return nil, fmt.Errorf("swipe: marshal meta: %w", err)
	}

	data := swipeTemplateData{
		DigestFilename: digestFilename,
		DigestTitle:    digest.Title,
		TimeWindow:     formatDuration(digest.TimeWindow),
		ArticlesJSON:   template.JS(articlesJSON),
		MetaJSON:       template.JS(metaJSON),
		PaletteCSS:     paletteCSS(),
		Theme:          resolveTheme(theme),
		Themes:         themeOpts,
	}

	templateText, err := loadNotificationTemplate(layout, "swipe.html.tmpl")
	if err != nil {
		return nil, fmt.Errorf("swipe: load template: %w", err)
	}
	styleCSS, err := loadStyleCSS(layout, "swipe.css")
	if err != nil {
		return nil, fmt.Errorf("swipe: load CSS: %w", err)
	}
	styleBody, styleLink := rc.styleFields(styleCSS, "swipe.css")
	data.StyleCSS = template.CSS(styleBody)
	data.StyleLink = template.HTML(styleLink)

	scriptJS, err := loadBuiltAsset("swipe.js")
	if err != nil {
		return nil, fmt.Errorf("swipe: load JS: %w", err)
	}
	scriptBody, scriptSrc := rc.scriptFields(scriptJS, "swipe.js")
	data.ScriptJS = template.JS(scriptBody)
	data.ScriptSrc = template.HTML(scriptSrc)

	tmpl, err := template.New("swipe").Parse(templateText)
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
