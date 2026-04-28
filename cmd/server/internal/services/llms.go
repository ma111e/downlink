package services

import (
	"context"
	"downlink/cmd/server/internal/config"
	"downlink/cmd/server/internal/store"
	"downlink/pkg/llmgateway"
	"downlink/pkg/llmutil"
	"downlink/pkg/mappers"
	"downlink/pkg/models"
	"downlink/pkg/protos"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/eino/schema"

	htmltomarkdown "github.com/JohannesKaufmann/html-to-markdown/v2"
	log "github.com/sirupsen/logrus"
	"google.golang.org/protobuf/types/known/emptypb"
)

// LLMsServer implements the LLMsService gRPC service
type LLMsServer struct {
	protos.UnimplementedLLMsServiceServer

	gw *llmgateway.Gateway
}

// NewLLMsServer creates a new LLMs server instance. The gateway is the single
// chokepoint for every LLM call the server makes; see pkg/llmgateway.
func NewLLMsServer(gw *llmgateway.Gateway) *LLMsServer {
	return &LLMsServer{gw: gw}
}

// GetLLMProviders returns the current LLM provider configurations
func (s *LLMsServer) GetLLMProviders(_ context.Context, _ *protos.GetLLMProvidersRequest) (*protos.GetLLMProvidersResponse, error) {
	config.ReloadConfig()
	protoConfig, err := mappers.ServerConfigToProto(config.Config)
	if err != nil {
		return nil, err
	}

	return &protos.GetLLMProvidersResponse{
		Providers: protoConfig.Providers,
	}, nil
}

// SaveLLMProviders updates the LLM provider configurations
func (s *LLMsServer) SaveLLMProviders(_ context.Context, req *protos.SaveLLMProvidersRequest) (*emptypb.Empty, error) {
	config.Config.Providers = mappers.AllProviderConfigsToModels(req.Providers)
	if err := config.Config.Save(config.ConfigPath); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

// GetAvailableModels fetches available models from all configured providers concurrently.
func (s *LLMsServer) GetAvailableModels(_ context.Context, _ *protos.GetAvailableModelsRequest) (*protos.ModelsResponse, error) {
	// Deduplicate by provider type + base URL
	type work struct {
		provider models.ProviderConfig
	}
	seen := make(map[string]bool)
	var jobs []work
	for _, provider := range config.Config.Providers {
		if !provider.Enabled {
			continue
		}
		key := provider.ProviderType + ":" + provider.BaseURL
		if seen[key] {
			continue
		}
		seen[key] = true
		jobs = append(jobs, work{provider})
	}

	type result struct {
		models []*protos.ModelInfo
		errMsg string
	}
	results := make([]result, len(jobs))

	var wg sync.WaitGroup
	for i, job := range jobs {
		wg.Add(1)
		go func(i int, provider models.ProviderConfig) {
			defer wg.Done()
			providerModels, err := fetchProviderModels(provider.ProviderType, provider)
			if err != nil {
				if !strings.Contains(err.Error(), "api key not found") {
					results[i].errMsg = fmt.Sprintf("\n| Warning: %s API issues: %v", provider.ProviderType, err)
				}
				return
			}
			results[i].models = mappers.AllModelInfoToProto(providerModels)
		}(i, job.provider)
	}
	wg.Wait()

	res := &protos.ModelsResponse{Models: []*protos.ModelInfo{}}
	for _, r := range results {
		res.Models = append(res.Models, r.models...)
		res.Error += r.errMsg
	}
	return res, nil
}

// analysisTask defines a single step in the sequential analysis pipeline.
type analysisTask struct {
	name        string
	instruction string
	outputKey   string // top-level key in the assembled result (empty = merge root)
	schema      string // expected JSON output format hint
}

// articleContext holds the prepared article data shared across all tasks.
type articleContext struct {
	articleId   string
	articleJSON string
	contentLen  int
}

// dataImageRe matches HTML data URI image tags (src="data:image/...;base64,...")
var dataImageRe = regexp.MustCompile(`(?i)\s*src\s*=\s*["']data:image/[^"']*["']`)

// prepareArticleContext fetches and converts article content for the analysis pipeline.
func (s *LLMsServer) prepareArticleContext(articleId string) (*articleContext, error) {
	article, err := store.Db.GetArticle(articleId)
	if err != nil {
		return nil, fmt.Errorf("failed to get article: %w", err)
	}

	// Strip base64-encoded inline images before conversion to avoid sending large payloads to the LLM
	stripped := dataImageRe.ReplaceAllString(article.Content, "")

	var content string
	markdown, err := htmltomarkdown.ConvertString(stripped)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Warnf("Error converting HTML to Markdown. Using HTML content instead")
		content = stripped
	} else {
		content = markdown
	}

	articleJSON := fmt.Sprintf("{\n\t\"id\":%q,\n\t\"title\":%q,\n\t\"content\":%q\n}", article.Id, article.Title, content)

	return &articleContext{
		articleId:   articleId,
		articleJSON: articleJSON,
		contentLen:  len(content),
	}, nil
}

// // getAnalysisTasks returns the ordered sequence of analysis tasks for the pipeline.
// func getAnalysisTasks(contentLen int) []analysisTask {
// 	tasks := []analysisTask{
// 		{
// 			name: "categorize",
// 			instruction: `Assign a single category and extract 3-10 relevant tags for this article (e.g., #zeroday, #ransomware, #APT, #financialsector, etc).
// It is very important that you set tags for all the malware, attackers, and victim names mentioned.
// Reply with ONLY a JSON object, no extra text.`,
// 			schema: `{"category": "<your category name>", "tags": ["<tag1>", "<tag2>", "<tag...>"]}`,
// 		},
// 		{
// 			name: "key_points",
// 			instruction: `Extract the key points from this article as bullet points (3-5 points). Do not skip details.
// Reply with ONLY a JSON object, no extra text.`,
// 			schema: `{"key_points": ["<key point1>", "<key point2>", "<key point...>"]}`,
// 		},
// 		{
// 			name: "insights",
// 			instruction: `Extract notable insights from this article — things that are surprising, non-obvious, or particularly valuable.
// Reply with ONLY a JSON object, no extra text.`,
// 			schema: `{"insights": ["<insight1>", "<insight2>", "<insight...>"]}`,
// 		},
// 	}

// 	if contentLen > 1000 {
// 		tasks = append(tasks, analysisTask{
// 			name: "summaries",
// 			instruction: `Create accurate summaries with ONLY information and sentences from the article by strictly following these rules:
// 1) use markdown and bulletpoints to make it easily readable
// 2) brief overview: a concise summary of the most important points. It should depict a rough but detailed idea of the article (5-10 sentences)
// 3) standard synthesis: this is a recap of the article. It must not miss any critical information (10-20 sentences)
// 4) comprehensive synthesis: this is a thorough analysis of the article with detailed insights (unlimited sentences)
// Reply with ONLY a JSON object, no extra text.`,
// 			schema: `{"summaries": {"brief_overview": "<brief overview>", "standard_synthesis": "<standard synthesis>", "comprehensive_synthesis": "<comprehensive synthesis>"}}`,
// 		})
// 	}

// 	tasks = append(tasks, analysisTask{
// 		name: "importance",
// 		instruction: `Score the importance of this article from 1-100 using this scale:
// - Above 90: I must read it
// - Above 75: I should read it
// - Above 60: I may read it
// - Below 50: reading it does not matter
// Also provide a brief justification (1-5 sentences).
// Reply with ONLY a JSON object, no extra text.`,
// 		schema: `{"importance_score": <score>, "justification": "<brief justification>"}`,
// 	})

// 	return tasks
// }

// // buildTaskPrompt constructs the prompt for a single analysis task.
// func buildTaskPrompt(actx *articleContext, task analysisTask) string {
// 	return fmt.Sprintf(`<start_of_article>:
// %s
// <end_of_article>

// <start_of_task>:
// %s
// <end_of_task>

// <start_of_output_format>:
// %s
// <end_of_output_format>`, actx.articleJSON, task.instruction, task.schema)
// }
func getAnalysisTasks(contentLen int, skipCategorize bool, fastMode bool) []analysisTask {
	if fastMode {
		return []analysisTask{
			{
				name: "key_points",
				instruction: `You are a cybersecurity analyst. Extract 3 to 5 key points from the article.
Each point must be a complete, self-contained sentence grounded strictly in the article's content.
Do not infer, speculate, or add context not present in the article.
Return ONLY the JSON object below.`,
				schema: `{"key_points": ["<point 1>", "<point 2>"]}`,
			},
		}
	}

	var tasks []analysisTask
	if !skipCategorize {
		tasks = append(tasks, analysisTask{
			name: "categorize",
			instruction: `You are a cybersecurity analyst. Assign exactly one category and between 3 and 15 tags to the article.

Tag priority order (high to low):
1. Attack techniques (sandbox escape, exploitation methods, etc.)
2. Named malware families
3. Named threat actors
4. CVEs
5. Named victim organizations
6. Named tools or software with specific vulnerabilities mentioned

If covering all entities would exceed 15 tags, drop the lowest-priority ones first.
Tags must be lowercase kebab-case prefixed with # (e.g. #ransomware, #apt29, #cve-2024-1234).
Return ONLY the JSON object. Make your decision quickly: one pass through the article is sufficient. Be very careful about looping, don't loop.`,
			schema: `{"category": "<single category>", "tags": ["#tag1", "#tag2"]}`,
		})
	}

	tasks = append(tasks,
		analysisTask{
			name: "tldr",
			instruction: `You are a cybersecurity analyst. Write a TL;DR for this article in 1–2 plain sentences. Capture the single most important takeaway. Do not use bullet points or markdown.
Return ONLY the JSON object below.`,
			schema: `{"tldr": "<1-2 sentence summary>"}`,
		},
		analysisTask{
			name: "key_points",
			instruction: `You are a cybersecurity analyst. Extract 3 to 5 key points from the article.
Each point must be a complete, self-contained sentence grounded strictly in the article's content.
Do not infer, speculate, or add context not present in the article.
Return ONLY the JSON object below.`,
			schema: `{"key_points": ["<point 1>", "<point 2>"]}`,
		},
		analysisTask{
			name: "insights",
			instruction: `You are a cybersecurity analyst. Identify 2 to 5 notable insights from the article.
An insight must be non-obvious, surprising, or particularly actionable — not a restatement of a key point.
Ground every insight strictly in the article's content; do not speculate.
Return ONLY the JSON object below.`,
			schema: `{"insights": ["<insight 1>", "<insight 2>"]}`,
		},
		analysisTask{
			name: "referenced_reports",
			instruction: `You are a cybersecurity analyst. Extract explicit third-party reports, research, studies, advisories, whitepapers, or technical analyses linked from this article.

Only include a link when the article clearly frames it as a report/research/advisory/whitepaper/study/technical analysis from another entity.
Do not include ordinary source links, product pages, news articles, social media posts, generic homepages, or links that are not explicitly described as research/report material.
For each item, capture the report title, URL, publisher/entity, and a short context sentence explaining how the article references it.
If none are present, return an empty referenced_reports array.
Return ONLY the JSON object below.`,
			schema: `{"referenced_reports": [{"title": "<report title>", "url": "<absolute URL>", "publisher": "<publisher/entity>", "context": "<short context>"}]}`,
		},
	)

	if contentLen > 1000 {
		tasks = append(tasks, analysisTask{
			name: "summaries",
			instruction: `You are a cybersecurity analyst. Write three summaries of the article using ONLY information present in the article — do not infer or add external context.

brief_overview: 5–10 sentences. A concise recap of the most important points, written in plain prose.
standard_synthesis: 10–20 sentences. A complete recap that omits no critical detail.
comprehensive_synthesis: Unlimited length. A thorough, structured analysis using markdown and bullet points where helpful.

For readability, add paragraph breaks where natural, roughly every 2-3 sentences.

Return ONLY the JSON object below.`,
			schema: `{"summaries": {"brief_overview": "<text>", "standard_synthesis": "<text>", "comprehensive_synthesis": "<text>"}}`,
		})
	}

	tasks = append(tasks, analysisTask{
		name: "importance",
		instruction: `You are a cybersecurity analyst. Score how important this article is for a security professional to read, using this scale:
91–100: Must read — high-impact, time-sensitive, or industry-altering
76–90:  Should read — significant findings or broad relevance
61–75:  May read — useful but not urgent
≤60:    Low priority — low novelty or narrow relevance

Provide a score (integer 1–100) and a concise justification of 1–3 sentences.
Return ONLY the JSON object below.`,
		schema: `{"importance_score": <integer 1-100>, "justification": "<1-3 sentences>"}`,
	})

	return tasks
}

// buildTaskPrompt constructs the prompt for a single analysis task.
func buildTaskPrompt(actx *articleContext, task analysisTask) string {
	return fmt.Sprintf(`<article>
%s
</article>

<instruction>
%s
</instruction>

<output_format>
%s
</output_format>`, actx.articleJSON, task.instruction, task.schema)
}

func (s *LLMsServer) buildAnalysisPromptForRequest(req *protos.AnalyzeArticleWithProviderModelRequest) (string, error) {
	actx, err := s.prepareArticleContext(req.ArticleId)
	if err != nil {
		return "", err
	}

	tasks := getAnalysisTasks(actx.contentLen, req.SkipCategorize, req.FastMode)
	var allInstructions, allSchemas []string
	for i, t := range tasks {
		allInstructions = append(allInstructions, fmt.Sprintf("%d. %s", i+1, t.instruction))
		allSchemas = append(allSchemas, t.schema)
	}

	return fmt.Sprintf(`<start_of_article>:
%s
<end_of_article>

<start_of_context>:
%s
<end_of_context>

<start_of_tasks>:
%s
<end_of_tasks>

<start_of_output_format>:
Return ONLY a single JSON object combining these output shapes:
%s
<end_of_output_format>`, actx.articleJSON, config.Config.Analysis.Persona, strings.Join(allInstructions, "\n\n"), strings.Join(allSchemas, "\n")), nil
}

// buildAnalysisPrompt constructs the full monolithic analysis prompt for an article.
// Kept for PreviewAnalysisPrompt and the direct-generate fallback path.
func (s *LLMsServer) buildAnalysisPrompt(articleId string) (string, error) {
	return s.buildAnalysisPromptForRequest(&protos.AnalyzeArticleWithProviderModelRequest{
		ArticleId: articleId,
	})
}

// PreviewAnalysisPrompt builds the prompt for an article without sending it to an LLM
func (s *LLMsServer) PreviewAnalysisPrompt(_ context.Context, req *protos.PreviewAnalysisPromptRequest) (*protos.PreviewAnalysisPromptResponse, error) {
	prompt, err := s.buildAnalysisPrompt(req.ArticleId)
	if err != nil {
		return nil, err
	}
	return &protos.PreviewAnalysisPromptResponse{Prompt: prompt}, nil
}

// progressCallback is called after each analysis task starts and completes.
type progressCallback func(taskName, status string, taskIndex, totalTasks int, taskResultJSON string, err error)

// runAnalysisPipeline executes the sequential task pipeline and returns the assembled result.
// The optional onProgress callback is invoked for each task start/complete.
func (s *LLMsServer) runAnalysisPipeline(ctx context.Context, req *protos.AnalyzeArticleWithProviderModelRequest, onProgress progressCallback) (*protos.AnalyzeArticleWithProviderModelResponse, error) {
	// Ensure the pipeline has at least 60 minutes total (tasks run sequentially;
	// each individual Stream call gets its own 10-minute sub-deadline below).
	const pipelineTimeout = 60 * time.Minute
	deadline, ok := ctx.Deadline()
	if ok {
		remaining := time.Until(deadline)
		log.Infof("Context deadline: %v (remaining: %v)", deadline, remaining)
		if remaining < pipelineTimeout {
			log.Warnf("Context deadline too short (%v), extending to %v", remaining, pipelineTimeout)
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(context.Background(), pipelineTimeout)
			defer cancel()
		}
	} else {
		log.Warnf("No context deadline set, adding %v timeout", pipelineTimeout)
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), pipelineTimeout)
		defer cancel()
	}

	resolved, err := ResolveLLM(LLMRequest{
		ProviderName: req.ProviderName,
		ProviderType: req.ProviderType,
		ModelName:    req.ModelName,
		MaxTokens:    defaultMaxTokensLarge,
	})
	if err != nil {
		return nil, err
	}
	req.ProviderType = resolved.ProviderType
	req.ModelName = resolved.ModelName

	actx, err := s.prepareArticleContext(req.ArticleId)
	if err != nil {
		return nil, err
	}

	// Run analysis tasks sequentially using a single ChatModel conversation.
	// The article is sent once in the first message; subsequent tasks only send the instruction.
	tasks := getAnalysisTasks(actx.contentLen, req.SkipCategorize, req.FastMode)
	totalTasks := len(tasks)
	assembled := make(map[string]interface{})
	assembled["id"] = actx.articleId
	var allRawResponses []string
	var allThinking []string
	succeededTasks := 0

	// Build conversation history — persona as system message, then article + tasks
	var conversationHistory []*schema.Message
	if persona := config.Config.Analysis.Persona; persona != "" {
		conversationHistory = append(conversationHistory, &schema.Message{
			Role:    schema.System,
			Content: persona,
		})
	}

	for i, task := range tasks {
		taskIdx := i + 1

		// First task includes the article; subsequent tasks only send the instruction
		var userMessage string
		if i == 0 {
			userMessage = buildTaskPrompt(actx, task)
		} else {
			userMessage = fmt.Sprintf(`<start_of_task>:
%s
<end_of_task>

<start_of_output_format>:
%s
<end_of_output_format>`, task.instruction, task.schema)
		}

		log.WithField("task", task.name).Info("Running analysis task")
		log.WithField("task", task.name).WithField("content_length", len(userMessage)).Debug("Prompt:")
		log.Debug(userMessage)
		if onProgress != nil {
			onProgress(task.name, "started", taskIdx, totalTasks, userMessage, nil)
		}

		// Append the new user message to conversation history
		conversationHistory = append(conversationHistory, &schema.Message{
			Role:    schema.User,
			Content: userMessage,
		})

		// Route every Stream call through the gateway so --max-concurrent-llm-requests applies.
		if deadline, ok := ctx.Deadline(); ok {
			log.WithField("task", task.name).Debugf("Before Stream: context deadline in %v", time.Until(deadline))
		} else {
			log.WithField("task", task.name).Debug("Before Stream: no context deadline")
		}

		onChunk := func(chunk *schema.Message) error {
			if chunk.Content != "" && onProgress != nil {
				onProgress(task.name, "token", taskIdx, totalTasks, chunk.Content, nil)
			}
			return nil
		}

		taskCtx, taskCancel := context.WithTimeout(ctx, 10*time.Minute)
		response, err := s.gw.Stream(
			taskCtx,
			resolved.Provider,
			conversationHistory,
			onChunk,
			llmgateway.WithLabel(fmt.Sprintf("analyze:task=%s", task.name)),
		)
		taskCancel()
		if err != nil {
			if onProgress != nil {
				onProgress(task.name, "error", taskIdx, totalTasks, "", err)
			}
			return nil, fmt.Errorf("model error during task %s (%s): %w", task.name, resolved.ProviderType, err)
		}

		log.WithField("task", task.name).WithField("provider", resolved.ProviderType).Debugf("Full response:\n%s", response)

		if response == "" {
			taskErr := fmt.Errorf("model returned empty response for task %s (%s)", task.name, resolved.ProviderType)
			log.WithField("task", task.name).Error(taskErr.Error())
			if onProgress != nil {
				onProgress(task.name, "error", taskIdx, totalTasks, "", taskErr)
			}
			return nil, taskErr
		}

		// Append assistant response to conversation history
		conversationHistory = append(conversationHistory, &schema.Message{
			Role:    schema.Assistant,
			Content: response,
		})

		log.WithField("task", task.name).WithField("content_length", len(response)).Debug("Response:")
		log.Debug(response)

		allRawResponses = append(allRawResponses, fmt.Sprintf("--- %s ---\n%s", task.name, response))

		// Extract thinking if present (handles both <think>...</think> and bare ...</think>)
		if endIdx := strings.Index(response, "</think>"); endIdx != -1 {
			startIdx := strings.Index(response, "<think>")
			var thinkingText string
			if startIdx != -1 {
				thinkingText = response[startIdx+len("<think>") : endIdx]
			} else {
				thinkingText = response[:endIdx]
			}
			allThinking = append(allThinking, fmt.Sprintf("[%s] %s", task.name, strings.TrimSpace(thinkingText)))
		}

		// Parse this task's JSON output and merge into assembled result
		cleaned := llmutil.CleanLLMResponse(response)

		var taskResult map[string]any
		if err := json.Unmarshal([]byte(cleaned), &taskResult); err != nil {
			// Fall back to trimming to the outermost {...} span.
			if err2 := json.Unmarshal([]byte(llmutil.ExtractJSON(cleaned)), &taskResult); err2 != nil {
				parseErr := fmt.Errorf("failed to parse response as JSON for task %s: %w", task.name, err)
				log.WithField("task", task.name).Error(parseErr.Error())
				if onProgress != nil {
					onProgress(task.name, "error", taskIdx, totalTasks, "", parseErr)
				}
				return nil, parseErr
			}
		}

		maps.Copy(assembled, taskResult)

		// Send completed progress with the parsed JSON result
		taskResultJSON, _ := json.Marshal(taskResult)
		if onProgress != nil {
			onProgress(task.name, "completed", taskIdx, totalTasks, string(taskResultJSON), nil)
		}

		succeededTasks++
		log.WithField("task", task.name).Info("Analysis task completed")
	}

	if succeededTasks == 0 {
		return nil, fmt.Errorf("all analysis tasks failed for article %s", actx.articleId)
	}

	combinedRaw := strings.Join(allRawResponses, "\n\n")
	combinedThinking := strings.Join(allThinking, "\n\n")

	return s.storeAnalysisFromResult(req, assembled, combinedRaw, combinedThinking)
}

// AnalyzeArticleWithProviderModel analyzes an article using sequential Eino ChatModelAgent calls.
func (s *LLMsServer) AnalyzeArticleWithProviderModel(ctx context.Context, req *protos.AnalyzeArticleWithProviderModelRequest) (*protos.AnalyzeArticleWithProviderModelResponse, error) {
	return s.runAnalysisPipeline(ctx, req, nil)
}

// AnalyzeArticleWithProgress is like AnalyzeArticleWithProviderModel but invokes the provided
// callback for each pipeline task start/completion so callers can forward progress events.
func (s *LLMsServer) AnalyzeArticleWithProgress(ctx context.Context, req *protos.AnalyzeArticleWithProviderModelRequest, onTask func(taskName, status string, taskIndex, totalTasks int, err error)) (*protos.AnalyzeArticleWithProviderModelResponse, error) {
	var cb progressCallback
	if onTask != nil {
		cb = func(taskName, status string, taskIndex, totalTasks int, _ string, err error) {
			// Filter out the per-token firehose; only surface lifecycle events.
			if status == "token" {
				return
			}
			onTask(taskName, status, taskIndex, totalTasks, err)
		}
	}
	return s.runAnalysisPipeline(ctx, req, cb)
}

func (s *LLMsServer) AnalyzeArticleOneShot(ctx context.Context, req *protos.AnalyzeArticleWithProviderModelRequest, onTask func(taskName, status string, taskIndex, totalTasks int, err error)) (*protos.AnalyzeArticleWithProviderModelResponse, error) {
	const oneShotTimeout = 60 * time.Minute
	deadline, ok := ctx.Deadline()
	if ok {
		remaining := time.Until(deadline)
		log.Infof("Context deadline: %v (remaining: %v)", deadline, remaining)
		if remaining < oneShotTimeout {
			log.Warnf("Context deadline too short (%v), extending to %v", remaining, oneShotTimeout)
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(context.Background(), oneShotTimeout)
			defer cancel()
		}
	} else {
		log.Warnf("No context deadline set, adding %v timeout", oneShotTimeout)
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), oneShotTimeout)
		defer cancel()
	}

	resolved, err := ResolveLLM(LLMRequest{
		ProviderName: req.ProviderName,
		ProviderType: req.ProviderType,
		ModelName:    req.ModelName,
		MaxTokens:    defaultMaxTokensLarge,
	})
	if err != nil {
		return nil, err
	}
	req.ProviderType = resolved.ProviderType
	req.ModelName = resolved.ModelName

	prompt, err := s.buildAnalysisPromptForRequest(req)
	if err != nil {
		return nil, err
	}

	if onTask != nil {
		onTask("one_shot_analysis", "started", 1, 1, nil)
	}

	response, err := s.gw.Generate(
		ctx,
		resolved.Provider,
		prompt,
		llmgateway.WithLabel("analyze:one_shot_analysis"),
	)
	if err != nil {
		if onTask != nil {
			onTask("one_shot_analysis", "error", 1, 1, err)
		}
		return nil, fmt.Errorf("model error during one-shot analysis (%s): %w", resolved.ProviderType, err)
	}

	res, err := s.buildAndStoreAnalysis(req, response)
	if err != nil {
		if onTask != nil {
			onTask("one_shot_analysis", "error", 1, 1, err)
		}
		return nil, err
	}

	if onTask != nil {
		onTask("one_shot_analysis", "completed", 1, 1, nil)
	}

	return res, nil
}

// StreamAnalyzeArticle is the streaming version that sends progress events per task.
func (s *LLMsServer) StreamAnalyzeArticle(req *protos.AnalyzeArticleWithProviderModelRequest, stream protos.LLMsService_StreamAnalyzeArticleServer) error {
	ctx := stream.Context()

	onProgress := func(taskName, status string, taskIndex, totalTasks int, data string, taskErr error) {
		event := &protos.AnalysisProgressEvent{
			TaskName:   taskName,
			Status:     status,
			TaskIndex:  int32(taskIndex),
			TotalTasks: int32(totalTasks),
		}
		switch status {
		case "token":
			event.TokenChunk = data
		case "completed":
			event.TaskResult = data
		case "error":
			if taskErr != nil {
				event.Error = taskErr.Error()
			}
		}
		if sendErr := stream.Send(event); sendErr != nil {
			log.WithError(sendErr).Warn("Failed to send analysis progress event")
		}
	}

	res, err := s.runAnalysisPipeline(ctx, req, onProgress)
	if err != nil {
		_ = stream.Send(&protos.AnalysisProgressEvent{
			Status: "error",
			Error:  err.Error(),
		})
		return err
	}

	_ = stream.Send(&protos.AnalysisProgressEvent{
		Status:   "done",
		Analysis: res.Analysis,
	})

	return nil
}

// buildAndStoreAnalysis parses a monolithic LLM response and stores the analysis.
// Used by the direct-generate fallback path (llamacpp).
func (s *LLMsServer) buildAndStoreAnalysis(req *protos.AnalyzeArticleWithProviderModelRequest, response string) (*protos.AnalyzeArticleWithProviderModelResponse, error) {
	thinkingProcess := ""
	if strings.Contains(response, "<think>") && strings.Contains(response, "</think>") {
		start := strings.Index(response, "<think>") + len("<think>")
		end := strings.Index(response, "</think>")
		thinkingProcess = strings.TrimSpace(response[start:end])
	}

	cleaned := llmutil.CleanLLMResponse(response)
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(cleaned), &result); err != nil {
		if err2 := json.Unmarshal([]byte(llmutil.ExtractJSON(cleaned)), &result); err2 != nil {
			return nil, fmt.Errorf("failed to parse response as JSON: %w, response: %s", err, cleaned)
		}
	}

	return s.storeAnalysisFromResult(req, result, response, thinkingProcess)
}

func referencedReportsFromResult(value any) []models.ReferencedReport {
	items, ok := value.([]interface{})
	if !ok {
		return nil
	}

	var reports []models.ReferencedReport
	for _, item := range items {
		obj, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		report := models.ReferencedReport{
			Title:     stringFromObject(obj, "title"),
			URL:       stringFromObject(obj, "url"),
			Publisher: stringFromObject(obj, "publisher"),
			Context:   stringFromObject(obj, "context"),
		}
		if report.URL == "" {
			report.URL = stringFromObject(obj, "URL")
		}
		if report.URL == "" {
			continue
		}
		reports = append(reports, report)
	}

	return reports
}

func stringFromObject(obj map[string]interface{}, key string) string {
	value, ok := obj[key].(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(value)
}

// storeAnalysisFromResult takes an assembled result map and persists it as an ArticleAnalysis.
func (s *LLMsServer) storeAnalysisFromResult(req *protos.AnalyzeArticleWithProviderModelRequest, result map[string]interface{}, rawResponse string, thinkingProcess string) (*protos.AnalyzeArticleWithProviderModelResponse, error) {
	analysis := &models.ArticleAnalysis{
		ArticleId:       req.ArticleId,
		ProviderType:    req.ProviderType,
		ModelName:       req.ModelName,
		ThinkingProcess: thinkingProcess,
		RawResponse:     rawResponse,
		CreatedAt:       time.Now(),
	}

	if score, ok := result["importance_score"].(float64); ok {
		analysis.ImportanceScore = int(score)
	} else if scoreStr, ok := result["importance_score"].(string); ok {
		if score, err := strconv.Atoi(scoreStr); err == nil {
			analysis.ImportanceScore = score
		}
	}

	if tldr, ok := result["tldr"].(string); ok {
		analysis.Tldr = tldr
	}

	if justification, ok := result["justification"].(string); ok {
		analysis.Justification = justification
	}

	if keyPoints, ok := result["key_points"].([]interface{}); ok {
		for _, point := range keyPoints {
			if pointStr, ok := point.(string); ok {
				analysis.KeyPoints = append(analysis.KeyPoints, pointStr)
			}
		}
	}

	if insights, ok := result["insights"].([]interface{}); ok {
		for _, insight := range insights {
			if insightStr, ok := insight.(string); ok {
				analysis.Insights = append(analysis.Insights, insightStr)
			}
		}
	}

	if reports := referencedReportsFromResult(result["referenced_reports"]); len(reports) > 0 {
		analysis.ReferencedReports = reports
	}

	if summaries, ok := result["summaries"].(map[string]interface{}); ok {
		if briefOverview, ok := summaries["brief_overview"].(string); ok {
			analysis.BriefOverview = briefOverview
		}
		if standardSynthesis, ok := summaries["standard_synthesis"].(string); ok {
			analysis.StandardSynthesis = standardSynthesis
		}
		if comprehensiveSynthesis, ok := summaries["comprehensive_synthesis"].(string); ok {
			analysis.ComprehensiveSynthesis = comprehensiveSynthesis
		}
	}

	if err := store.Db.SaveArticleAnalysis(analysis); err != nil {
		log.WithError(err).Warn("Failed to save article analysis")
	}

	// Update the article with category and tags
	var tagIds []string
	if tags, ok := result["tags"].([]interface{}); ok {
		for _, tag := range tags {
			if tagStr, ok := tag.(string); ok {
				tagStr = strings.TrimPrefix(tagStr, "#")
				tagIds = append(tagIds, tagStr)
			}
		}
	}

	var categoryName *string
	if category, ok := result["category"].(string); ok && category != "" {
		cat, err := store.Db.GetOrCreateCategory(category)
		if err == nil {
			categoryName = &cat.Name
		}
	}

	if len(tagIds) > 0 || categoryName != nil {
		update := models.ArticleUpdate{}
		if len(tagIds) > 0 {
			update.TagIds = &tagIds
		}
		if categoryName != nil {
			update.CategoryName = categoryName
		}
		if err := store.Db.UpdateArticle(req.ArticleId, update); err != nil {
			log.WithError(err).Warn("Failed to update article with analysis results")
		}
	}

	return &protos.AnalyzeArticleWithProviderModelResponse{
		Analysis: mappers.ArticleAnalysisToProto(analysis),
	}, nil
}

// AnalyzeArticle analyzes an article using the default model
func (s *LLMsServer) AnalyzeArticle(ctx context.Context, req *protos.AnalyzeArticleRequest) (*protos.AnalyzeArticleResponse, error) {
	if config.Config.Analysis.Provider == "" {
		return nil, fmt.Errorf("analysis provider not configured")
	}

	// Call the unified implementation with no explicit provider/model so
	// runAnalysisPipeline resolves it from the analysis config's provider name.
	res, err := s.AnalyzeArticleWithProviderModel(ctx, &protos.AnalyzeArticleWithProviderModelRequest{
		ArticleId: req.ArticleId,
		FastMode:  req.FastMode,
	})
	if err != nil {
		return nil, err
	}

	return &protos.AnalyzeArticleResponse{
		Analysis: res.Analysis,
	}, nil
}

// GetAnalysisConfig returns the current analysis configuration
func (s *LLMsServer) GetAnalysisConfig(_ context.Context, _ *protos.GetAnalysisConfigRequest) (*protos.GetAnalysisConfigResponse, error) {
	config.ReloadConfig()
	protoConfig, err := mappers.ServerConfigToProto(config.Config)
	if err != nil {
		return nil, err
	}

	return &protos.GetAnalysisConfigResponse{
		AnalysisConfig: protoConfig.Analysis,
	}, nil
}

// fetchProviderModels is a unified function to fetch models from any provider
func fetchProviderModels(providerType string, provider models.ProviderConfig) ([]models.ModelInfo, error) {
	var modelInfos []models.ModelInfo
	var err error
	var apiEndpoint string

	// Get the API key from provider config
	apiKey := provider.APIKey
	// Ollama and llama.cpp don't require an API key
	if apiKey == "" && providerType != "ollama" && providerType != "llamacpp" {
		return nil, fmt.Errorf("api key not found for %s", providerType)
	}

	// Setup HTTP client
	client := &http.Client{Timeout: 5 * time.Second}
	var req *http.Request

	// Provider-specific configuration
	switch providerType {
	case "mistral":
		// Set API endpoint
		apiEndpoint = "https://api.mistral.ai/v1/models"
		if provider.BaseURL != "" {
			apiEndpoint = provider.BaseURL + "/models"
		}

		// Create request
		req, err = http.NewRequest("GET", apiEndpoint, nil)
		if err != nil {
			// Fallback to default model list
			return getFallbackMistralModels(), fmt.Errorf("failed to create request: %w", err)
		}

		// Set headers
		req.Header.Set("Authorization", "Bearer "+apiKey)

		// Make the request
		resp, err := client.Do(req)
		if err != nil {
			// Fallback to default model list
			return getFallbackMistralModels(), fmt.Errorf("failed to make API request: %w", err)
		}
		defer resp.Body.Close()

		// Handle non-200 responses
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			// Fallback to default model list
			return getFallbackMistralModels(), fmt.Errorf("Mistral API returned status %d: %s", resp.StatusCode, string(body))
		}

		// Parse the response with the updated format
		var result struct {
			Object string `json:"object"`
			Data   []struct {
				Id           string `json:"id"`
				Object       string `json:"object"`
				Created      int64  `json:"created"`
				OwnedBy      string `json:"owned_by"`
				Capabilities struct {
					CompletionChat  bool `json:"completion_chat"`
					CompletionFIM   bool `json:"completion_fim"`
					FunctionCalling bool `json:"function_calling"`
					FineTuning      bool `json:"fine_tuning"`
					Vision          bool `json:"vision"`
				} `json:"capabilities"`
				Name                    string   `json:"name"`
				Description             string   `json:"description"`
				MaxContextLength        int      `json:"max_context_length"`
				Aliases                 []string `json:"aliases"`
				Deprecation             string   `json:"deprecation"`
				DefaultModelTemperature float64  `json:"default_model_temperature"`
				Type                    string   `json:"type"`
			} `json:"data"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			// Fallback to default model list
			return getFallbackMistralModels(), fmt.Errorf("failed to decode response: %w", err)
		}

		// Convert to our format
		for _, model := range result.Data {
			modelInfo := models.ModelInfo{
				Id:           model.Id,
				Name:         model.Id,
				ProviderType: "mistral",
			}
			// Use Name as DisplayName if available
			if model.Name != "" && model.Name != model.Id {
				modelInfo.DisplayName = model.Name
			}
			// Use Description if available
			if model.Description != "" {
				modelInfo.Description = model.Description
			}
			modelInfos = append(modelInfos, modelInfo)
		}

		// If no models found, use fallback
		if len(modelInfos) == 0 {
			return getFallbackMistralModels(), nil
		}

	case "openai":
		// Set API endpoint - BaseURL may be a host (https://host) or include /v1 (https://host/v1).
		// Normalise by stripping a trailing /v1 so we can always append /v1/models.
		apiEndpoint = "https://api.openai.com/v1/models"
		if provider.BaseURL != "" {
			base := strings.TrimRight(provider.BaseURL, "/")
			base = strings.TrimSuffix(base, "/v1")
			apiEndpoint = base + "/v1/models"
		}

		// Create request
		req, err = http.NewRequest("GET", apiEndpoint, nil)
		if err != nil {
			return nil, err
		}

		// Set headers
		req.Header.Add("Authorization", "Bearer "+apiKey)

		// Make the request
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		// Handle non-200 responses
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return nil, fmt.Errorf("OpenAI API returned status %d: %s", resp.StatusCode, string(body))
		}

		// Parse the response
		var openAIResp models.OpenAIModelsResponse
		if err := json.NewDecoder(resp.Body).Decode(&openAIResp); err != nil {
			return nil, err
		}

		// Convert to our format
		for _, model := range openAIResp.Data {
			modelInfo := models.ModelInfo{
				Id:           model.Id,
				Name:         model.Id,
				ProviderType: "openai",
			}
			modelInfos = append(modelInfos, modelInfo)
		}

	case "anthropic":
		// Set API base endpoint
		apiBase := "https://api.anthropic.com/v1/models"
		if provider.BaseURL != "" {
			apiBase = provider.BaseURL + "/models"
		}

		// Paginate through all available models
		afterId := ""
		for {
			// Build URL with query parameters
			apiEndpoint = apiBase + "?limit=1000"
			if afterId != "" {
				apiEndpoint += "&after_id=" + afterId
			}

			// Create request
			req, err = http.NewRequest("GET", apiEndpoint, nil)
			if err != nil {
				return getFallbackAnthropicModels(), fmt.Errorf("failed to create request: %w", err)
			}

			// Set headers
			req.Header.Add("x-api-key", apiKey)
			req.Header.Add("anthropic-version", "2023-06-01")

			// Make the request
			resp, err := client.Do(req)
			if err != nil {
				return getFallbackAnthropicModels(), fmt.Errorf("failed to make API request: %w", err)
			}

			// Handle non-200 responses — close body explicitly (no defer inside loop)
			if resp.StatusCode != http.StatusOK {
				body, _ := io.ReadAll(resp.Body)
				resp.Body.Close()
				return getFallbackAnthropicModels(), fmt.Errorf("Anthropic API returned status %d: %s", resp.StatusCode, string(body))
			}

			// Parse the response, then close the body before the next iteration
			var anthropicResp models.AnthropicModelsResponse
			decodeErr := json.NewDecoder(resp.Body).Decode(&anthropicResp)
			resp.Body.Close()
			if decodeErr != nil {
				return getFallbackAnthropicModels(), fmt.Errorf("failed to decode response: %w", decodeErr)
			}

			// Convert to our format
			for _, model := range anthropicResp.Data {
				modelInfo := models.ModelInfo{
					Id:           model.Id,
					Name:         model.Id,
					DisplayName:  model.DisplayName,
					ProviderType: "anthropic",
				}
				modelInfos = append(modelInfos, modelInfo)
			}

			// Check if there are more pages
			if !anthropicResp.HasMore || anthropicResp.LastId == "" {
				break
			}
			afterId = anthropicResp.LastId
		}

		// If no models found, use fallback
		if len(modelInfos) == 0 {
			return getFallbackAnthropicModels(), nil
		}

	case "ollama":
		// Strip trailing slash to avoid double-slash in constructed URL
		baseURL := strings.TrimRight(provider.BaseURL, "/")
		if baseURL == "" {
			baseURL = "http://localhost:11434"
		}

		apiEndpoint = fmt.Sprintf("%s/api/tags", baseURL)

		// Create request
		req, err = http.NewRequest("GET", apiEndpoint, nil)
		if err != nil {
			return nil, err
		}

		// Add API key authentication if provided
		if apiKey != "" {
			req.Header.Add("Authorization", "Bearer "+apiKey)
		}

		// Make the request
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		// Handle non-200 responses
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return nil, fmt.Errorf("Ollama API returned status %d: %s", resp.StatusCode, string(body))
		}

		// Parse the response
		var ollamaResp models.OllamaModelsResponse
		if err := json.NewDecoder(resp.Body).Decode(&ollamaResp); err != nil {
			return nil, err
		}

		// Convert to our format
		for _, model := range ollamaResp.Models {
			modelInfo := models.ModelInfo{
				Id:           model.Name,
				Name:         model.Name,
				ProviderType: "ollama",
			}
			modelInfos = append(modelInfos, modelInfo)
		}

	case "llamacpp":
		// llama.cpp servers may expose models at /v1/models (OpenAI-compatible) or /models
		baseURL := strings.TrimRight(provider.BaseURL, "/")
		if baseURL == "" {
			return nil, fmt.Errorf("llamacpp provider requires a base_url (e.g. http://localhost:8080)")
		}

		// Try /v1/models first, fall back to /models.
		// Some llama.cpp builds return HTTP 200 on /v1/models with an empty data array,
		// so we also continue to the next endpoint when no models are found.
		endpoints := []string{
			fmt.Sprintf("%s/v1/models", baseURL),
			fmt.Sprintf("%s/models", baseURL),
		}

		var llamaResp models.LlamaCppModelsResponse
		var lastErr error
		for _, endpoint := range endpoints {
			req, err = http.NewRequest("GET", endpoint, nil)
			if err != nil {
				lastErr = err
				continue
			}

			// API key is optional for llama.cpp
			if apiKey != "" {
				req.Header.Add("Authorization", "Bearer "+apiKey)
			}

			resp, err := client.Do(req)
			if err != nil {
				lastErr = err
				continue
			}

			if resp.StatusCode != http.StatusOK {
				body, _ := io.ReadAll(resp.Body)
				resp.Body.Close()
				lastErr = fmt.Errorf("llama.cpp API returned status %d: %s", resp.StatusCode, string(body))
				continue
			}

			var candidate models.LlamaCppModelsResponse
			decodeErr := json.NewDecoder(resp.Body).Decode(&candidate)
			resp.Body.Close()
			if decodeErr != nil {
				lastErr = decodeErr
				continue
			}

			// Accept this response only if it contains at least one model
			if len(candidate.Data) == 0 && len(candidate.Models) == 0 {
				lastErr = fmt.Errorf("llama.cpp endpoint %s returned no models", endpoint)
				continue
			}

			llamaResp = candidate
			lastErr = nil
			break
		}

		if lastErr != nil {
			return nil, lastErr
		}

		// Prefer the OpenAI-compatible data[] array; fall back to native models[] array
		if len(llamaResp.Data) > 0 {
			for _, model := range llamaResp.Data {
				modelInfos = append(modelInfos, models.ModelInfo{
					Id:           model.Id,
					Name:         model.Id,
					ProviderType: "llamacpp",
				})
			}
		} else {
			for _, model := range llamaResp.Models {
				id := model.Model
				if id == "" {
					id = model.Name
				}
				modelInfos = append(modelInfos, models.ModelInfo{
					Id:           id,
					Name:         id,
					ProviderType: "llamacpp",
				})
			}
		}

	default:
		return nil, fmt.Errorf("unsupported provider type: %s", providerType)
	}

	return modelInfos, nil
}

// getFallbackMistralModels returns a default list of Mistral models to use when API calls fail -- 03/20/2025
func getFallbackMistralModels() []models.ModelInfo {
	return []models.ModelInfo{
		{
			Id:           "mistral-small-latest",
			Name:         "mistral-small-latest",
			DisplayName:  "Mistral Small (Latest)",
			Description:  "Mistral's small model optimized for efficiency and performance",
			ProviderType: "mistral",
		},
	}
}

// getFallbackOpenAIModels returns a default list of OpenAI models to use when API calls fail -- 03/20/2025
func getFallbackOpenAIModels() []models.ModelInfo {
	return []models.ModelInfo{
		{
			Id:           "gpt-4o",
			Name:         "gpt-4o",
			DisplayName:  "GPT-4o",
			Description:  "OpenAI's most advanced multimodal model with high capability across text, vision, and audio tasks",
			ProviderType: "openai",
		},
		{
			Id:           "gpt-4o-mini",
			Name:         "gpt-4o-mini",
			DisplayName:  "GPT-4o Mini",
			Description:  "Smaller, faster, and more cost-effective version of GPT-4o",
			ProviderType: "openai",
		},
		{
			Id:           "o3-mini",
			Name:         "o3-mini",
			DisplayName:  "O3 Mini",
			Description:  "OpenAI's efficient small-scale model optimized for speed and efficiency",
			ProviderType: "openai",
		},
	}
}

// getFallbackAnthropicModels returns a default list of Anthropic models to use when API calls fail -- 03/20/2025
func getFallbackAnthropicModels() []models.ModelInfo {
	return []models.ModelInfo{
		{
			Id:           "claude-3-7-sonnet-latest",
			Name:         "claude-3-7-sonnet-latest",
			DisplayName:  "Claude 3.7 Sonnet",
			Description:  "Anthropic's latest Claude 3.7 Sonnet model offering a balance of intelligence and speed",
			ProviderType: "anthropic",
		},
		{
			Id:           "claude-3-5-haiku-latest",
			Name:         "claude-3-5-haiku-latest",
			DisplayName:  "Claude 3.5 Haiku",
			Description:  "Anthropic's quick and efficient model optimized for fast responses",
			ProviderType: "anthropic",
		},
		{
			Id:           "claude-3-5-sonnet-latest",
			Name:         "claude-3-5-sonnet-latest",
			DisplayName:  "Claude 3.5 Sonnet",
			Description:  "Balanced model for both complex reasoning and speed",
			ProviderType: "anthropic",
		},
		{
			Id:           "claude-3-opus-latest",
			Name:         "claude-3-opus-latest",
			DisplayName:  "Claude 3 Opus",
			Description:  "Anthropic's most capable model for complex tasks requiring deep analysis",
			ProviderType: "anthropic",
		},
	}
}
