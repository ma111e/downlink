package store

import (
	"strings"
	"testing"
	"time"
)

func TestRecordFeedRefreshRoundTrip(t *testing.T) {
	s := newTestStore(t)

	if err := s.StartFeedRefreshRun("refresh-1", "manual-single", time.Now()); err != nil {
		t.Fatalf("StartFeedRefreshRun() error = %v", err)
	}

	errs := []string{"Article A: scrape timeout", "Article B: invalid UTF-8 content"}
	rawBody := strings.Repeat("<item><title>x</title></item>", 500) // compressible
	in := FeedRefreshInput{
		RunID:        "refresh-1",
		FeedId:       "feed-1",
		FeedTitle:    "Hacker News",
		FeedURL:      "https://news.ycombinator.com/rss",
		Success:      true,
		TotalFetched: 40,
		Stored:       5,
		Skipped:      33,
		Errors:       errs,
		RawBody:      []byte(rawBody),
		RawStatus:    200,
		RawType:      "application/rss+xml",
		DurationMs:   1200,
	}
	if err := s.RecordFeedRefresh(in); err != nil {
		t.Fatalf("RecordFeedRefresh() error = %v", err)
	}

	results, err := s.ListFeedRefreshResultsForRun("refresh-1")
	if err != nil {
		t.Fatalf("ListFeedRefreshResultsForRun() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	got := results[0]
	if got.FeedTitle != "Hacker News" || got.FeedURL != "https://news.ycombinator.com/rss" {
		t.Errorf("feed identity not persisted: title=%q url=%q", got.FeedTitle, got.FeedURL)
	}
	if got.TotalFetched != 40 || got.Stored != 5 || got.Skipped != 33 {
		t.Errorf("counts not persisted: fetched=%d stored=%d skipped=%d", got.TotalFetched, got.Stored, got.Skipped)
	}
	if got.ErrorCount != 2 {
		t.Errorf("error count = %d, want 2", got.ErrorCount)
	}
	// The item-level error log round-trips back to its lines.
	if len(got.Errors) != 2 || got.Errors[0] != errs[0] || got.Errors[1] != errs[1] {
		t.Errorf("error log round-trip mismatch: got %v, want %v", got.Errors, errs)
	}
	// The raw body round-trips and its metadata persists.
	if got.RawBody != rawBody {
		t.Errorf("raw body round-trip mismatch: got %d bytes, want %d", len(got.RawBody), len(rawBody))
	}
	if got.RawStatus != 200 || got.RawType != "application/rss+xml" {
		t.Errorf("raw metadata not persisted: status=%d type=%q", got.RawStatus, got.RawType)
	}
	// The stored raw body is compressed.
	if len(got.FeedRefreshResult.RawBody) >= len(rawBody) {
		t.Errorf("expected stored raw body to be compressed: stored %d >= raw %d", len(got.FeedRefreshResult.RawBody), len(rawBody))
	}
}

func TestRecordFeedRefreshFailure(t *testing.T) {
	s := newTestStore(t)
	_ = s.StartFeedRefreshRun("refresh-f", "manual-single", time.Now())

	in := FeedRefreshInput{
		RunID:      "refresh-f",
		FeedId:     "feed-x",
		FeedTitle:  "Broken Feed",
		Success:    false,
		FetchError: "failed to fetch feed: 503 Service Unavailable",
	}
	if err := s.RecordFeedRefresh(in); err != nil {
		t.Fatalf("RecordFeedRefresh() error = %v", err)
	}

	results, _ := s.ListFeedRefreshResultsForRun("refresh-f")
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	got := results[0]
	if got.Success {
		t.Error("expected Success=false")
	}
	if !strings.Contains(got.FetchError, "503") {
		t.Errorf("fetch error not persisted: %q", got.FetchError)
	}
	// No item-level errors → empty slice, not a spurious single empty line.
	if len(got.Errors) != 0 {
		t.Errorf("expected no item errors, got %v", got.Errors)
	}
}

func TestListFeedRefreshRunSummaries(t *testing.T) {
	s := newTestStore(t)

	now := time.Now()
	_ = s.StartFeedRefreshRun("run-old", "startup", now.Add(-2*time.Hour))
	_ = s.StartFeedRefreshRun("run-new", "manual-all", now.Add(-1*time.Hour))

	// run-new: 3 feeds, one a top-level failure, one with item errors.
	_ = s.RecordFeedRefresh(FeedRefreshInput{RunID: "run-new", FeedId: "a", Success: true, TotalFetched: 20, Stored: 4})
	_ = s.RecordFeedRefresh(FeedRefreshInput{RunID: "run-new", FeedId: "b", Success: true, TotalFetched: 10, Stored: 1, Errors: []string{"x: boom"}})
	_ = s.RecordFeedRefresh(FeedRefreshInput{RunID: "run-new", FeedId: "c", Success: false, FetchError: "down"})
	// run-old: 1 feed, success.
	_ = s.RecordFeedRefresh(FeedRefreshInput{RunID: "run-old", FeedId: "a", Success: true, TotalFetched: 5, Stored: 2})

	summaries, err := s.ListFeedRefreshRunSummaries(10)
	if err != nil {
		t.Fatalf("ListFeedRefreshRunSummaries() error = %v", err)
	}
	if len(summaries) != 2 {
		t.Fatalf("expected 2 runs, got %d", len(summaries))
	}
	// Newest first.
	if summaries[0].Id != "run-new" {
		t.Errorf("expected run-new first (newest), got %s", summaries[0].Id)
	}
	rn := summaries[0]
	if rn.FeedCount != 3 {
		t.Errorf("run-new feed count = %d, want 3", rn.FeedCount)
	}
	if rn.OkCount != 2 || rn.FailCount != 1 {
		t.Errorf("run-new ok/fail = %d/%d, want 2/1", rn.OkCount, rn.FailCount)
	}
	if rn.TotalFetched != 30 || rn.TotalStored != 5 {
		t.Errorf("run-new totals: fetched=%d stored=%d, want 30/5", rn.TotalFetched, rn.TotalStored)
	}
	if rn.ErrorCount != 1 {
		t.Errorf("run-new item error count = %d, want 1", rn.ErrorCount)
	}
	if rn.Trigger != "manual-all" {
		t.Errorf("run-new trigger = %q, want manual-all", rn.Trigger)
	}
}

func TestPruneFeedRefreshRuns(t *testing.T) {
	s := newTestStore(t)

	base := time.Now().Add(-10 * time.Hour)
	for i := 0; i < 5; i++ {
		id := string(rune('a' + i))
		_ = s.StartFeedRefreshRun("run-"+id, "startup", base.Add(time.Duration(i)*time.Hour))
		_ = s.RecordFeedRefresh(FeedRefreshInput{RunID: "run-" + id, FeedId: "f", Success: true})
	}

	if err := s.PruneFeedRefreshRuns(2); err != nil {
		t.Fatalf("PruneFeedRefreshRuns() error = %v", err)
	}

	summaries, err := s.ListFeedRefreshRunSummaries(10)
	if err != nil {
		t.Fatalf("ListFeedRefreshRunSummaries() error = %v", err)
	}
	if len(summaries) != 2 {
		t.Fatalf("expected 2 runs after prune, got %d", len(summaries))
	}
	// The two most recent (run-e, run-d) survive.
	if summaries[0].Id != "run-e" || summaries[1].Id != "run-d" {
		t.Errorf("unexpected survivors: %s, %s", summaries[0].Id, summaries[1].Id)
	}
	// Pruned runs' results are gone too.
	results, _ := s.ListFeedRefreshResultsForRun("run-a")
	if len(results) != 0 {
		t.Errorf("expected results of pruned run-a to be deleted, got %d", len(results))
	}
}
