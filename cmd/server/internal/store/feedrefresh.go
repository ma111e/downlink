package store

import (
	"fmt"
	"strings"
	"time"

	"github.com/ma111e/downlink/pkg/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// FeedRefreshInput is the raw (uncompressed) outcome of refreshing one feed.
// RecordFeedRefresh joins and gzip-compresses Errors before persisting.
type FeedRefreshInput struct {
	RunID        string
	FeedId       string
	FeedTitle    string
	FeedURL      string
	Success      bool
	TotalFetched int
	Stored       int
	Skipped      int
	FetchError   string
	Errors       []string // item-level errors; joined with "\n" into ErrorLog
	Warnings     []string // non-fatal notices; joined with "\n" into WarningLog
	RawBody      []byte   // raw fetched feed body; gzip-compressed before storage
	RawStatus    int
	RawType      string
	DurationMs   int64
}

// FeedRefreshRunSummary is a refresh-run row plus aggregate counters over its
// per-feed results, for the refresh list page.
type FeedRefreshRunSummary struct {
	models.FeedRefreshRun
	FeedCount    int
	OkCount      int
	FailCount    int
	TotalStored  int
	TotalFetched int
	ErrorCount   int
	WarningCount int
}

// FeedRefreshResultView is a decompressed per-feed result for display; Errors
// and Warnings are the item-level logs split back into lines and RawBody is the
// raw fetched feed body as plain text.
type FeedRefreshResultView struct {
	models.FeedRefreshResult
	Errors   []string
	Warnings []string
	RawBody  string
}

// StartFeedRefreshRun inserts a new refresh-run row at the start of a cycle.
func (s *GormStore) StartFeedRefreshRun(id, trigger string, startedAt time.Time) error {
	run := models.FeedRefreshRun{Id: id, Trigger: trigger, StartedAt: startedAt}
	return s.db.Create(&run).Error
}

// RecordFeedRefresh persists one feed's refresh outcome, gzip-compressing the
// item-level error log.
func (s *GormStore) RecordFeedRefresh(in FeedRefreshInput) error {
	res := models.FeedRefreshResult{
		Id:           uuid.New().String(),
		RunId:        in.RunID,
		FeedId:       in.FeedId,
		FeedTitle:    in.FeedTitle,
		FeedURL:      in.FeedURL,
		Success:      in.Success,
		TotalFetched: in.TotalFetched,
		Stored:       in.Stored,
		Skipped:      in.Skipped,
		ErrorCount:   len(in.Errors),
		WarningCount: len(in.Warnings),
		FetchError:   in.FetchError,
		ErrorLog:     gzipString(strings.Join(in.Errors, "\n")),
		WarningLog:   gzipString(strings.Join(in.Warnings, "\n")),
		RawBody:      gzipString(string(in.RawBody)),
		RawStatus:    in.RawStatus,
		RawType:      in.RawType,
		DurationMs:   in.DurationMs,
		CreatedAt:    time.Now(),
	}
	return s.db.Create(&res).Error
}

// FinishFeedRefreshRun stamps a refresh run's completion time.
func (s *GormStore) FinishFeedRefreshRun(id string, finishedAt time.Time) error {
	return s.db.Model(&models.FeedRefreshRun{}).Where("id = ?", id).
		Update("finished_at", finishedAt).Error
}

// ListFeedRefreshRunSummaries returns the most recent refresh runs with per-run
// result counts and totals, newest first. limit <= 0 means no limit.
func (s *GormStore) ListFeedRefreshRunSummaries(limit int) ([]FeedRefreshRunSummary, error) {
	q := s.db.Table("feed_refresh_runs").
		Select("feed_refresh_runs.*, " +
			"COALESCE(COUNT(feed_refresh_results.id), 0) AS feed_count, " +
			"COALESCE(SUM(CASE WHEN feed_refresh_results.success THEN 1 ELSE 0 END), 0) AS ok_count, " +
			"COALESCE(SUM(CASE WHEN feed_refresh_results.success THEN 0 ELSE 1 END), 0) AS fail_count, " +
			"COALESCE(SUM(feed_refresh_results.stored), 0) AS total_stored, " +
			"COALESCE(SUM(feed_refresh_results.total_fetched), 0) AS total_fetched, " +
			"COALESCE(SUM(feed_refresh_results.error_count), 0) AS error_count, " +
			"COALESCE(SUM(feed_refresh_results.warning_count), 0) AS warning_count").
		Joins("LEFT JOIN feed_refresh_results ON feed_refresh_results.run_id = feed_refresh_runs.id").
		Group("feed_refresh_runs.id").
		Order("feed_refresh_runs.started_at DESC")
	if limit > 0 {
		q = q.Limit(limit)
	}

	var summaries []FeedRefreshRunSummary
	if err := q.Scan(&summaries).Error; err != nil {
		return nil, fmt.Errorf("list feed refresh run summaries: %w", err)
	}
	return summaries, nil
}

// GetFeedRefreshRun returns a single refresh run by id.
func (s *GormStore) GetFeedRefreshRun(id string) (models.FeedRefreshRun, error) {
	var run models.FeedRefreshRun
	if err := s.db.Where("id = ?", id).First(&run).Error; err != nil {
		return models.FeedRefreshRun{}, err
	}
	return run, nil
}

// ListFeedRefreshResultsForRun returns a run's per-feed results in chronological
// order, with the item-level error log decompressed into lines.
func (s *GormStore) ListFeedRefreshResultsForRun(runID string) ([]FeedRefreshResultView, error) {
	var results []models.FeedRefreshResult
	if err := s.db.Where("run_id = ?", runID).Order("created_at ASC").Find(&results).Error; err != nil {
		return nil, fmt.Errorf("list feed refresh results: %w", err)
	}

	views := make([]FeedRefreshResultView, 0, len(results))
	for _, r := range results {
		log, err := gunzipBytes(r.ErrorLog)
		if err != nil {
			return nil, fmt.Errorf("decompress error log for result %s: %w", r.Id, err)
		}
		var lines []string
		if log != "" {
			lines = strings.Split(log, "\n")
		}
		warnLog, err := gunzipBytes(r.WarningLog)
		if err != nil {
			return nil, fmt.Errorf("decompress warning log for result %s: %w", r.Id, err)
		}
		var warnLines []string
		if warnLog != "" {
			warnLines = strings.Split(warnLog, "\n")
		}
		body, err := gunzipBytes(r.RawBody)
		if err != nil {
			return nil, fmt.Errorf("decompress raw body for result %s: %w", r.Id, err)
		}
		views = append(views, FeedRefreshResultView{FeedRefreshResult: r, Errors: lines, Warnings: warnLines, RawBody: body})
	}
	return views, nil
}

// PruneFeedRefreshRuns deletes all but the most recent keep runs (and their
// per-feed results). keep <= 0 is a no-op.
func (s *GormStore) PruneFeedRefreshRuns(keep int) error {
	if keep <= 0 {
		return nil
	}

	var staleIDs []string
	err := s.db.Model(&models.FeedRefreshRun{}).
		Order("started_at DESC").
		Offset(keep).
		Pluck("id", &staleIDs).Error
	if err != nil {
		return fmt.Errorf("find stale refresh runs: %w", err)
	}
	if len(staleIDs) == 0 {
		return nil
	}

	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("run_id IN ?", staleIDs).Delete(&models.FeedRefreshResult{}).Error; err != nil {
			return fmt.Errorf("delete results for stale refresh runs: %w", err)
		}
		if err := tx.Where("id IN ?", staleIDs).Delete(&models.FeedRefreshRun{}).Error; err != nil {
			return fmt.Errorf("delete stale refresh runs: %w", err)
		}
		return nil
	})
}
