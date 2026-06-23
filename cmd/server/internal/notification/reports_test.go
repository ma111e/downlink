package notification

import (
	"strings"
	"testing"
	"time"

	"github.com/ma111e/downlink/pkg/models"
)

// sampleReportDigests returns two digests whose articles share one report URL so
// the aggregation can be exercised for dedup, tag union, ref counting, and the
// primary flag.
func sampleReportDigests() []models.Digest {
	older := time.Date(2026, 4, 23, 9, 0, 0, 0, time.UTC)
	newer := time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC)
	shared := models.ReferencedReport{
		Title:     "Threat Group Annual Report",
		URL:       "https://research.example.com/report/",
		Publisher: "Example Labs",
		Category:  "research",
		Context:   "cited as the primary source",
		Primary:   true,
	}
	// Same URL referenced again, non-primary, with a trailing-slash/case variant.
	sharedVariant := shared
	sharedVariant.URL = "https://research.example.com/report"
	sharedVariant.Primary = false
	sharedVariant.Context = "referenced in passing"

	return []models.Digest{
		{
			Id:        "digest-new",
			CreatedAt: newer,
			Articles: []models.Article{
				{Id: "art-1", Title: "Critical CVE disclosed", Link: "https://news.example.com/1", PublishedAt: newer,
					Tags: []models.Tag{{Name: "cve"}, {Name: "ransomware"}}},
			},
			DigestAnalyses: []models.DigestAnalysis{
				{ArticleId: "art-1", Analysis: &models.ArticleAnalysis{
					ArticleId:         "art-1",
					Tldr:              "A serious CVE was disclosed today.",
					ReferencedReports: []models.ReferencedReport{shared},
				}},
			},
		},
		{
			Id:        "digest-old",
			CreatedAt: older,
			Articles: []models.Article{
				{Id: "art-2", Title: "Follow-up analysis", Link: "https://news.example.com/2", PublishedAt: older,
					Tags: []models.Tag{{Name: "ransomware"}, {Name: "apt"}}},
			},
			DigestAnalyses: []models.DigestAnalysis{
				{ArticleId: "art-2", Analysis: &models.ArticleAnalysis{
					ArticleId:         "art-2",
					BriefOverview:     "More detail on the campaign.",
					ReferencedReports: []models.ReferencedReport{sharedVariant},
				}},
			},
		},
	}
}

func TestAggregateReportsDedupAndMerge(t *testing.T) {
	reports := aggregateReports(sampleReportDigests())

	if len(reports) != 1 {
		t.Fatalf("expected 1 deduped report, got %d", len(reports))
	}
	r := reports[0]

	if r.RefCount != 2 {
		t.Errorf("expected RefCount 2 (two distinct source articles), got %d", r.RefCount)
	}
	if !r.Primary {
		t.Error("expected Primary true (OR-ed across sources)")
	}
	if r.Title != "Threat Group Annual Report" || r.Publisher != "Example Labs" || r.Category != "research" {
		t.Errorf("unexpected report metadata: %+v", r)
	}

	wantTags := map[string]bool{"cve": true, "ransomware": true, "apt": true}
	if len(r.Tags) != len(wantTags) {
		t.Errorf("expected unioned tags %v, got %v", wantTags, r.Tags)
	}
	for _, tag := range r.Tags {
		if !wantTags[tag] {
			t.Errorf("unexpected tag %q in %v", tag, r.Tags)
		}
	}

	if len(r.Sources) != 2 {
		t.Fatalf("expected 2 source articles, got %d", len(r.Sources))
	}
	// Description comes from the source article's analysis (Tldr, then BriefOverview).
	var haveTldr, haveBrief bool
	for _, s := range r.Sources {
		switch s.Description {
		case "A serious CVE was disclosed today.":
			haveTldr = true
		case "More detail on the campaign.":
			haveBrief = true
		}
	}
	if !haveTldr || !haveBrief {
		t.Errorf("expected per-article descriptions from Tldr and BriefOverview, got %+v", r.Sources)
	}
}

func TestRenderReportsPageEmbedsJSON(t *testing.T) {
	reports := aggregateReports(sampleReportDigests())
	out, err := RenderReportsPage(reports, "", "")
	if err != nil {
		t.Fatalf("RenderReportsPage: %v", err)
	}
	html := string(out)
	if !strings.Contains(html, "window.__DL_REPORTS") {
		t.Error("expected embedded window.__DL_REPORTS payload")
	}
	if !strings.Contains(html, "Threat Group Annual Report") {
		t.Error("expected report title in rendered page")
	}
}
