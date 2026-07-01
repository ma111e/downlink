package mappers

import (
	"testing"
	"time"

	"github.com/ma111e/downlink/pkg/models"
)

func TestArticleRoundTripPreservesFieldsAndPointers(t *testing.T) {
	read := true
	bookmarked := false
	catName := "security"
	var score int32 = 87
	pub := time.Date(2026, 6, 1, 8, 0, 0, 0, time.UTC)
	fetched := time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)

	in := &models.Article{
		Id:                    "a1",
		FeedId:                "f1",
		Title:                 "Title",
		Content:               "Body",
		Link:                  "https://x/1",
		FetchedAt:             fetched,
		PublishedAt:           pub,
		HeroImage:             "hero.png",
		Read:                  &read,
		Bookmarked:            &bookmarked,
		CategoryName:          &catName,
		LatestImportanceScore: &score,
		Category:              &models.Category{Name: "security", Color: "#111", Icon: "shield"},
		Tags:                  []models.Tag{{Id: "t1", Name: "cve"}},
	}

	out := ArticleToModel(ArticleToProto(in))

	if out.Id != "a1" || out.FeedId != "f1" || out.Title != "Title" || out.Content != "Body" ||
		out.Link != in.Link || out.HeroImage != "hero.png" {
		t.Errorf("scalar fields lost: %+v", out)
	}
	if !out.FetchedAt.Equal(fetched) || !out.PublishedAt.Equal(pub) {
		t.Errorf("timestamps lost: fetched=%v published=%v", out.FetchedAt, out.PublishedAt)
	}
	if out.Read == nil || !*out.Read {
		t.Errorf("Read = %v, want true pointer", out.Read)
	}
	if out.Bookmarked == nil || *out.Bookmarked {
		t.Errorf("Bookmarked = %v, want false pointer (not nil)", out.Bookmarked)
	}
	if out.CategoryName == nil || *out.CategoryName != "security" {
		t.Errorf("CategoryName = %v, want security", out.CategoryName)
	}
	if out.LatestImportanceScore == nil || *out.LatestImportanceScore != 87 {
		t.Errorf("LatestImportanceScore = %v, want 87", out.LatestImportanceScore)
	}
	if out.Category == nil || out.Category.Color != "#111" || out.Category.Icon != "shield" {
		t.Errorf("Category lost: %+v", out.Category)
	}
	if len(out.Tags) != 1 || out.Tags[0].Id != "t1" || out.Tags[0].Name != "cve" {
		t.Errorf("Tags lost: %+v", out.Tags)
	}
}

func TestArticleToModelNilIsNil(t *testing.T) {
	if ArticleToModel(nil) != nil {
		t.Fatal("ArticleToModel(nil) != nil")
	}
}

func TestArticleNilPointersStayNil(t *testing.T) {
	in := &models.Article{Id: "a1"} // Read/Bookmarked/CategoryName/score all nil
	out := ArticleToModel(ArticleToProto(in))
	if out.Read != nil || out.Bookmarked != nil || out.CategoryName != nil || out.LatestImportanceScore != nil {
		t.Fatalf("nil pointers became non-nil: read=%v bm=%v cat=%v score=%v",
			out.Read, out.Bookmarked, out.CategoryName, out.LatestImportanceScore)
	}
	if out.Category != nil {
		t.Fatalf("Category = %+v, want nil", out.Category)
	}
}

func TestArticleFilterRoundTrip(t *testing.T) {
	start := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)
	in := &models.ArticleFilter{
		UnreadOnly:      true,
		CategoryName:    "sec",
		TagId:           "t1",
		BookmarkedOnly:  true,
		FeedId:          "f1",
		Offset:          20,
		Limit:           50,
		Query:           "ransomware",
		ExcludeDigested: true,
		Unbounded:       false,
		StartDate:       &start,
		EndDate:         &end,
	}

	out := ArticleFilterToModel(ArticleFilterToProto(in))

	if out.UnreadOnly != true || out.CategoryName != "sec" || out.TagId != "t1" ||
		!out.BookmarkedOnly || out.FeedId != "f1" || out.Query != "ransomware" || !out.ExcludeDigested {
		t.Errorf("filter scalar fields lost: %+v", out)
	}
	if out.Offset != 20 || out.Limit != 50 {
		t.Errorf("offset/limit = %d/%d, want 20/50 (int32<->int)", out.Offset, out.Limit)
	}
	if out.StartDate == nil || !out.StartDate.Equal(start) {
		t.Errorf("StartDate = %v, want %v", out.StartDate, start)
	}
	if out.EndDate == nil || !out.EndDate.Equal(end) {
		t.Errorf("EndDate = %v, want %v", out.EndDate, end)
	}
}

func TestArticleFilterNilDatesStayNil(t *testing.T) {
	out := ArticleFilterToModel(ArticleFilterToProto(&models.ArticleFilter{Limit: 10}))
	if out.StartDate != nil || out.EndDate != nil {
		t.Fatalf("nil dates became non-nil: start=%v end=%v", out.StartDate, out.EndDate)
	}
}

func TestTagRoundTrip(t *testing.T) {
	in := []models.Tag{{Id: "t1", Name: "cve"}, {Id: "t2", Name: "apt"}}
	out := AllTagsToModels(AllTagsToProto(in))
	if len(out) != 2 || out[0].Id != "t1" || out[0].Name != "cve" || out[1].Name != "apt" {
		t.Fatalf("tag round-trip lost data: %+v", out)
	}
}
