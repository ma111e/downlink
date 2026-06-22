package store

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"time"

	"github.com/ma111e/downlink/pkg/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// LLMCallInput is the raw (uncompressed) data for one recorded LLM call.
// RecordLLMCall compresses Prompt/Response before persisting.
type LLMCallInput struct {
	RunID            string
	Label            string
	ProviderType     string
	ModelName        string
	Prompt           string
	Response         string
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	TokensKnown      bool
	DurationMs       int64
	Err              string
}

// LLMRunSummary is a run row plus aggregate counters for the runs list page.
type LLMRunSummary struct {
	models.LLMRun
	CallCount             int
	TotalTokens           int
	TotalPromptTokens     int // sent
	TotalCompletionTokens int // received
	ArticleCount          int // articles in the run's digest (0 if no digest yet)
}

// AvgTokensPerArticle is total tokens spread across the run's articles, or 0
// when the article count is unknown (no linked digest).
func (r LLMRunSummary) AvgTokensPerArticle() int {
	if r.ArticleCount <= 0 {
		return 0
	}
	return r.TotalTokens / r.ArticleCount
}

// LLMCallView is a decompressed LLM call for display; Prompt/Response are plain text.
type LLMCallView struct {
	models.LLMCall
	Prompt   string
	Response string
}

// StartLLMRun inserts a new run row at the start of digest generation, tagged
// with the profile the digest is being generated for.
func (s *GormStore) StartLLMRun(id, profileId string, startedAt time.Time) error {
	run := models.LLMRun{Id: id, ProfileId: profileId, StartedAt: startedAt}
	return s.db.Create(&run).Error
}

// LinkLLMRunToDigest attaches the produced digest id and title to a run once known.
func (s *GormStore) LinkLLMRunToDigest(runID, digestID, title string) error {
	return s.db.Model(&models.LLMRun{}).Where("id = ?", runID).
		Updates(map[string]interface{}{"digest_id": digestID, "title": title}).Error
}

// FinishLLMRun stamps a run's completion time.
func (s *GormStore) FinishLLMRun(id string, finishedAt time.Time) error {
	return s.db.Model(&models.LLMRun{}).Where("id = ?", id).
		Update("finished_at", finishedAt).Error
}

// RecordLLMCall persists one LLM call, gzip-compressing the prompt and response.
func (s *GormStore) RecordLLMCall(in LLMCallInput) error {
	call := models.LLMCall{
		Id:               uuid.New().String(),
		RunId:            in.RunID,
		Label:            in.Label,
		ProviderType:     in.ProviderType,
		ModelName:        in.ModelName,
		Prompt:           gzipString(in.Prompt),
		Response:         gzipString(in.Response),
		PromptTokens:     in.PromptTokens,
		CompletionTokens: in.CompletionTokens,
		TotalTokens:      in.TotalTokens,
		TokensKnown:      in.TokensKnown,
		DurationMs:       in.DurationMs,
		Err:              in.Err,
		CreatedAt:        time.Now(),
	}
	return s.db.Create(&call).Error
}

// ListLLMRunSummaries returns the most recent runs with per-run call counts and
// token totals, newest first. limit <= 0 means no limit. A non-empty profileID
// restricts results to that profile's runs.
func (s *GormStore) ListLLMRunSummaries(limit int, profileID string) ([]LLMRunSummary, error) {
	q := s.db.Table("llm_runs").
		Select("llm_runs.*, "+
			"COALESCE(COUNT(llm_calls.id), 0) AS call_count, "+
			"COALESCE(SUM(llm_calls.total_tokens), 0) AS total_tokens, "+
			"COALESCE(SUM(llm_calls.prompt_tokens), 0) AS total_prompt_tokens, "+
			"COALESCE(SUM(llm_calls.completion_tokens), 0) AS total_completion_tokens, "+
			"COALESCE(MAX(digests.article_count), 0) AS article_count").
		Joins("LEFT JOIN llm_calls ON llm_calls.run_id = llm_runs.id").
		Joins("LEFT JOIN digests ON digests.id = llm_runs.digest_id").
		Group("llm_runs.id").
		Order("llm_runs.started_at DESC")
	if profileID != "" {
		q = q.Where("llm_runs.profile_id = ?", profileID)
	}
	if limit > 0 {
		q = q.Limit(limit)
	}

	var summaries []LLMRunSummary
	if err := q.Scan(&summaries).Error; err != nil {
		return nil, fmt.Errorf("list llm run summaries: %w", err)
	}
	return summaries, nil
}

// GetLLMRun returns a single run by id.
func (s *GormStore) GetLLMRun(id string) (models.LLMRun, error) {
	var run models.LLMRun
	if err := s.db.Where("id = ?", id).First(&run).Error; err != nil {
		return models.LLMRun{}, err
	}
	return run, nil
}

// ListLLMCallsForRun returns a run's calls in chronological order, with prompt
// and response decompressed to plain text for display.
func (s *GormStore) ListLLMCallsForRun(runID string) ([]LLMCallView, error) {
	var calls []models.LLMCall
	if err := s.db.Where("run_id = ?", runID).Order("created_at ASC").Find(&calls).Error; err != nil {
		return nil, fmt.Errorf("list llm calls: %w", err)
	}

	views := make([]LLMCallView, 0, len(calls))
	for _, c := range calls {
		prompt, err := gunzipBytes(c.Prompt)
		if err != nil {
			return nil, fmt.Errorf("decompress prompt for call %s: %w", c.Id, err)
		}
		response, err := gunzipBytes(c.Response)
		if err != nil {
			return nil, fmt.Errorf("decompress response for call %s: %w", c.Id, err)
		}
		views = append(views, LLMCallView{LLMCall: c, Prompt: prompt, Response: response})
	}
	return views, nil
}

// PruneLLMRuns deletes all but the most recent keep runs (and their calls).
// keep <= 0 is a no-op.
func (s *GormStore) PruneLLMRuns(keep int) error {
	if keep <= 0 {
		return nil
	}

	var staleIDs []string
	err := s.db.Model(&models.LLMRun{}).
		Order("started_at DESC").
		Offset(keep).
		Pluck("id", &staleIDs).Error
	if err != nil {
		return fmt.Errorf("find stale runs: %w", err)
	}
	if len(staleIDs) == 0 {
		return nil
	}

	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("run_id IN ?", staleIDs).Delete(&models.LLMCall{}).Error; err != nil {
			return fmt.Errorf("delete calls for stale runs: %w", err)
		}
		if err := tx.Where("id IN ?", staleIDs).Delete(&models.LLMRun{}).Error; err != nil {
			return fmt.Errorf("delete stale runs: %w", err)
		}
		return nil
	})
}

// gzipString compresses s with gzip. A nil/empty input yields a valid empty-gzip stream.
func gzipString(s string) []byte {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	_, _ = w.Write([]byte(s))
	_ = w.Close()
	return buf.Bytes()
}

// gunzipBytes reverses gzipString. Empty input decodes to an empty string.
func gunzipBytes(b []byte) (string, error) {
	if len(b) == 0 {
		return "", nil
	}
	r, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		return "", err
	}
	defer r.Close()
	out, err := io.ReadAll(r)
	if err != nil {
		return "", err
	}
	return string(out), nil
}
