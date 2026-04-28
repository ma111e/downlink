package downlinkclient

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"downlink/pkg/mappers"
	"downlink/pkg/models"
	"downlink/pkg/protos"

	log "github.com/sirupsen/logrus"
)

// ConnectionTestResult is returned by TestProviderConnection.
type ConnectionTestResult struct {
	Success   bool   `json:"success"`
	Message   string `json:"message"`
	LatencyMs int64  `json:"latency_ms"`
}

// GetLLMProviders returns the current LLM provider configurations
func (pc *DownlinkClient) GetLLMProviders() ([]models.ProviderConfig, error) {
	res, err := pc.llmsClient.GetLLMProviders(pc.ctx, &protos.GetLLMProvidersRequest{})
	if err != nil {
		return nil, err
	}
	return mappers.AllProviderConfigsToModels(res.Providers), nil
}

// SaveLLMProviders updates the LLM provider configurations
func (pc *DownlinkClient) SaveLLMProviders(providers []models.ProviderConfig) error {
	_, err := pc.llmsClient.SaveLLMProviders(pc.ctx, &protos.SaveLLMProvidersRequest{
		Providers: mappers.AllProviderConfigsToProtos(providers),
	})
	if err != nil {
		log.WithFields(log.Fields{
			"error": err,
		}).Error("Failed to save LLM providers")
		return err
	}
	return nil
}

func (pc *DownlinkClient) GetAvailableModels() (*models.ModelsResponse, error) {
	res, err := pc.llmsClient.GetAvailableModels(pc.ctx, &protos.GetAvailableModelsRequest{})
	if err != nil {
		return nil, err
	}

	modelsResponse := mappers.ModelsResponseToModel(res)
	return &modelsResponse, nil
}

// GetAvailableModelsForProvider fetches models for a single provider (by type + base URL).
// It calls the shared gRPC endpoint and filters the result, so no proto changes are needed.
func (pc *DownlinkClient) GetAvailableModelsForProvider(providerType, baseURL string) (*models.ModelsResponse, error) {
	res, err := pc.llmsClient.GetAvailableModels(pc.ctx, &protos.GetAvailableModelsRequest{})
	if err != nil {
		return nil, err
	}

	full := mappers.ModelsResponseToModel(res)
	filtered := make([]models.ModelInfo, 0, len(full.Models))
	for _, m := range full.Models {
		if m.ProviderType == providerType {
			filtered = append(filtered, m)
		}
	}
	return &models.ModelsResponse{Models: filtered, Error: full.Error}, nil
}

// GetAnalysisConfig returns the current enrichment configuration
func (pc *DownlinkClient) GetAnalysisConfig() (models.AnalysisConfig, error) {
	res, err := pc.llmsClient.GetAnalysisConfig(pc.ctx, &protos.GetAnalysisConfigRequest{})
	if err != nil {
		return models.AnalysisConfig{}, err
	}
	a := mappers.AnalysisConfigToModel(res.AnalysisConfig)
	if a == nil {
		return models.AnalysisConfig{}, nil
	}
	return *a, nil
}

func (pc *DownlinkClient) AnalyzeArticleWithProviderModel(articleId string, providerType string, modelName string) (models.ArticleAnalysis, error) {
	return pc.AnalyzeArticleWithProviderModelFast(articleId, providerType, modelName, false)
}

func (pc *DownlinkClient) AnalyzeArticleWithProviderModelFast(articleId string, providerType string, modelName string, fastMode bool) (models.ArticleAnalysis, error) {
	res, err := pc.llmsClient.AnalyzeArticleWithProviderModel(pc.ctx, &protos.AnalyzeArticleWithProviderModelRequest{
		ArticleId:    articleId,
		ProviderType: providerType,
		ModelName:    modelName,
		FastMode:     fastMode,
	})
	if err != nil {
		log.WithFields(log.Fields{
			"error": err,
		}).Error("Failed to analyze article with provider model")
		return models.ArticleAnalysis{}, err
	}

	if a := mappers.ArticleAnalysisToModel(res.Analysis); a != nil {
		return *a, nil
	}
	return models.ArticleAnalysis{}, nil
}

// AnalysisProgressEvent mirrors the server-side progress event for use by CLI callers.
type AnalysisProgressEvent struct {
	TaskName   string
	Status     string // "started", "token", "completed", "error", "done"
	TaskIndex  int
	TotalTasks int
	TaskResult string
	Error      string
	Analysis   *models.ArticleAnalysis
}

type analysisProgressStream interface {
	Recv() (*protos.AnalysisProgressEvent, error)
}

// StreamAnalyzeArticleWithProviderModel streams analysis progress events for a single article.
// The provided callback is called for each event. The final "done" event carries the completed analysis.
func (pc *DownlinkClient) StreamAnalyzeArticleWithProviderModel(articleId, providerType, modelName string, fastMode bool, onEvent func(AnalysisProgressEvent)) (models.ArticleAnalysis, error) {
	stream, err := pc.llmsClient.StreamAnalyzeArticle(pc.ctx, &protos.AnalyzeArticleWithProviderModelRequest{
		ArticleId:    articleId,
		ProviderType: providerType,
		ModelName:    modelName,
		FastMode:     fastMode,
	})
	if err != nil {
		return models.ArticleAnalysis{}, err
	}

	return streamAnalysisEvents(stream, onEvent)
}

// StreamAnalyzeArticleWithProfile streams analysis progress using a named provider profile.
func (pc *DownlinkClient) StreamAnalyzeArticleWithProfile(articleId, profileName string, fastMode bool, onEvent func(AnalysisProgressEvent)) (models.ArticleAnalysis, error) {
	stream, err := pc.llmsClient.StreamAnalyzeArticle(pc.ctx, &protos.AnalyzeArticleWithProviderModelRequest{
		ArticleId:    articleId,
		ProviderName: profileName,
		FastMode:     fastMode,
	})
	if err != nil {
		return models.ArticleAnalysis{}, err
	}

	return streamAnalysisEvents(stream, onEvent)
}

func streamAnalysisEvents(stream analysisProgressStream, onEvent func(AnalysisProgressEvent)) (models.ArticleAnalysis, error) {
	for {
		event, recvErr := stream.Recv()
		if recvErr != nil {
			return models.ArticleAnalysis{}, recvErr
		}

		ev := AnalysisProgressEvent{
			TaskName:   event.GetTaskName(),
			Status:     event.GetStatus(),
			TaskIndex:  int(event.GetTaskIndex()),
			TotalTasks: int(event.GetTotalTasks()),
			TaskResult: event.GetTaskResult(),
			Error:      event.GetError(),
		}

		if event.GetStatus() == "done" {
			if a := mappers.ArticleAnalysisToModel(event.GetAnalysis()); a != nil {
				ev.Analysis = a
			}
			if onEvent != nil {
				onEvent(ev)
			}
			if ev.Analysis != nil {
				return *ev.Analysis, nil
			}
			return models.ArticleAnalysis{}, nil
		}

		if event.GetStatus() == "error" && event.GetError() != "" && event.GetTaskName() == "" {
			return models.ArticleAnalysis{}, fmt.Errorf("%s", event.GetError())
		}

		if onEvent != nil {
			onEvent(ev)
		}
	}
}

// StreamAnalyzeArticle streams analysis progress using the default configured provider.
func (pc *DownlinkClient) StreamAnalyzeArticle(articleId string, fastMode bool, onEvent func(AnalysisProgressEvent)) (models.ArticleAnalysis, error) {
	return pc.StreamAnalyzeArticleWithProviderModel(articleId, "", "", fastMode, onEvent)
}

func (pc *DownlinkClient) AnalyzeArticle(articleId string) (models.ArticleAnalysis, error) {
	return pc.AnalyzeArticleFast(articleId, false)
}

func (pc *DownlinkClient) AnalyzeArticleFast(articleId string, fastMode bool) (models.ArticleAnalysis, error) {
	res, err := pc.llmsClient.AnalyzeArticle(pc.ctx, &protos.AnalyzeArticleRequest{
		ArticleId: articleId,
		FastMode:  fastMode,
	})
	if err != nil {
		log.WithFields(log.Fields{
			"error": err,
		}).Error("Failed to analyze article")
		return models.ArticleAnalysis{}, err
	}

	if a := mappers.ArticleAnalysisToModel(res.Analysis); a != nil {
		return *a, nil
	}
	return models.ArticleAnalysis{}, nil
}

// PreviewAnalysisPrompt returns the full prompt that would be sent for an article analysis
func (pc *DownlinkClient) PreviewAnalysisPrompt(articleId string) (string, error) {
	res, err := pc.llmsClient.PreviewAnalysisPrompt(pc.ctx, &protos.PreviewAnalysisPromptRequest{
		ArticleId: articleId,
	})
	if err != nil {
		return "", err
	}
	return res.Prompt, nil
}

// TestProviderConnection checks whether a provider's endpoint is reachable without consuming tokens.
// It tries lightweight read-only endpoints (health check or model listing) and returns latency.
func (pc *DownlinkClient) TestProviderConnection(providerType, baseURL, apiKey string) ConnectionTestResult {
	candidates, err := resolveProbeEndpoints(providerType, baseURL)
	if err != nil {
		return ConnectionTestResult{Success: false, Message: err.Error()}
	}

	client := &http.Client{Timeout: 10 * time.Second}
	var failures []string
	for _, endpoint := range candidates {
		result, err := probeEndpoint(client, endpoint, apiKey)
		if err == nil {
			return result
		}
		failures = append(failures, err.Error())
	}

	return ConnectionTestResult{
		Success: false,
		Message: fmt.Sprintf("All endpoints failed: %s", strings.Join(failures, "; ")),
	}
}

func resolveProbeEndpoints(providerType, baseURL string) ([]string, error) {
	baseURL = strings.TrimRight(baseURL, "/")
	// Normalise: strip trailing /v1 so we can always append /v1/<path> ourselves.
	baseURL = strings.TrimSuffix(baseURL, "/v1")

	// Apply provider defaults when no custom base URL is set
	if baseURL == "" {
		switch providerType {
		case "openai":
			baseURL = "https://api.openai.com"
		case "mistral":
			baseURL = "https://api.mistral.ai"
		default:
			return nil, fmt.Errorf("no endpoint URL configured")
		}
	}

	// Ordered list of candidate endpoints to probe (cheapest first)
	var candidates []string
	switch providerType {
	case "llamacpp":
		candidates = []string{
			baseURL + "/health",
			baseURL + "/v1/models",
			baseURL + "/models",
		}
	case "ollama":
		candidates = []string{
			baseURL + "/api/tags",
		}
	case "openai", "mistral":
		candidates = []string{
			baseURL + "/v1/models",
		}
	default:
		return nil, fmt.Errorf("connectivity test not supported for provider type %q", providerType)
	}

	return candidates, nil
}

func probeEndpoint(client *http.Client, endpoint, apiKey string) (ConnectionTestResult, error) {
	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return ConnectionTestResult{}, fmt.Errorf("%s: build request: %w", endpoint, err)
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	start := time.Now()
	resp, err := client.Do(req)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		return ConnectionTestResult{}, fmt.Errorf("%s: request failed: %w", endpoint, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return ConnectionTestResult{
			Success:   false,
			Message:   fmt.Sprintf("Reachable but unauthorized - check your API key (%d ms)", latency),
			LatencyMs: latency,
		}, nil
	}
	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNoContent {
		return ConnectionTestResult{
			Success:   true,
			Message:   fmt.Sprintf("Connected (%d ms)", latency),
			LatencyMs: latency,
		}, nil
	}
	return ConnectionTestResult{}, fmt.Errorf("%s: returned HTTP %d", endpoint, resp.StatusCode)
}
