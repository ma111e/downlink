package manager

import (
	"crypto/md5"
	"fmt"
	"time"

	"github.com/ma111e/downlink/cmd/server/internal/store"
	"github.com/ma111e/downlink/pkg/models"

	log "github.com/sirupsen/logrus"
)

// FeedRefreshRetention bounds how many recent refresh runs (and their per-feed
// results) the monitor keeps. 0 disables pruning. Mirrors
// services.LLMMonitorRetention.
var FeedRefreshRetention = 100

// generateRefreshRunId returns a unique id for a feed-refresh monitoring run.
// Nanosecond precision keeps back-to-back runs from colliding.
func generateRefreshRunId(t time.Time) string {
	hash := md5.Sum([]byte(t.Format(time.RFC3339Nano)))
	return fmt.Sprintf("refresh-%s", fmt.Sprintf("%x", hash)[:12])
}

// StartRefreshRun opens a refresh-monitoring run and returns its id. Recording
// is best-effort: a store failure is logged and an empty id returned, which the
// subsequent Record/Finish calls treat as a no-op.
func (m *FeedManager) StartRefreshRun(trigger string) string {
	runID := generateRefreshRunId(time.Now())
	if err := m.store.StartFeedRefreshRun(runID, trigger, time.Now()); err != nil {
		log.WithError(err).Warn("failed to start feed refresh monitor run")
		return ""
	}
	return runID
}

// RecordRefresh persists one feed's refresh outcome against a run. A blank runID
// (failed StartRefreshRun) is a no-op. Best-effort: store errors are logged.
func (m *FeedManager) RecordRefresh(runID string, feed models.Feed, fr models.FetchResult, ferr error, dur time.Duration) {
	if runID == "" {
		return
	}
	in := store.FeedRefreshInput{
		RunID:        runID,
		FeedId:       feed.Id,
		FeedTitle:    feed.Title,
		FeedURL:      feed.URL,
		Success:      ferr == nil,
		TotalFetched: fr.TotalFetched,
		Stored:       fr.Stored,
		Skipped:      fr.Skipped,
		Errors:       fr.Errors,
		RawBody:      fr.RawBody,
		RawStatus:    fr.RawStatus,
		RawType:      fr.RawContentType,
		DurationMs:   dur.Milliseconds(),
	}
	if ferr != nil {
		in.FetchError = ferr.Error()
	}
	if err := m.store.RecordFeedRefresh(in); err != nil {
		log.WithError(err).WithField("feed", feed.Id).Warn("failed to record feed refresh")
	}
}

// FinishRefreshRun stamps a run's completion and prunes old runs. Best-effort.
func (m *FeedManager) FinishRefreshRun(runID string) {
	if runID == "" {
		return
	}
	if err := m.store.FinishFeedRefreshRun(runID, time.Now()); err != nil {
		log.WithError(err).WithField("run_id", runID).Warn("failed to finish feed refresh monitor run")
	}
	if err := m.store.PruneFeedRefreshRuns(FeedRefreshRetention); err != nil {
		log.WithError(err).Warn("failed to prune old feed refresh monitor runs")
	}
}
