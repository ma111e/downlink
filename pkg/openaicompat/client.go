// Package openaicompat provides a lightweight client for any OpenAI-compatible
// chat completions API (llama.cpp, vLLM, LM Studio, Ollama /v1, etc.).
package openaicompat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client calls a single OpenAI-compatible /v1/chat/completions endpoint.
type Client struct {
	baseURL    string
	model      string
	apiKey     string
	httpClient *http.Client
}

// NewClient creates a Client for the given base URL (e.g. "http://localhost:8080").
// apiKey may be empty for unauthenticated local servers.
func NewClient(baseURL, model, apiKey string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		model:   model,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

// chatMessage mirrors the OpenAI message object.
type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// chatRequest is the POST body for /v1/chat/completions.
type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature,omitempty"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
}

// chatChoice is one element of the choices array in the response.
type chatChoice struct {
	Message      chatMessage `json:"message"`
	FinishReason string      `json:"finish_reason"`
}

// chatResponse is the top-level response object.
type chatResponse struct {
	ID      string       `json:"id"`
	Object  string       `json:"object"`
	Model   string       `json:"model"`
	Choices []chatChoice `json:"choices"`
}

// Complete sends a single user message to the model and returns the assistant reply.
// systemPrompt may be empty.
func (c *Client) Complete(ctx context.Context, systemPrompt, userPrompt string, temperature float64, maxTokens int) (string, error) {
	messages := make([]chatMessage, 0, 2)
	if systemPrompt != "" {
		messages = append(messages, chatMessage{Role: "system", Content: systemPrompt})
	}
	messages = append(messages, chatMessage{Role: "user", Content: userPrompt})

	body, err := json.Marshal(chatRequest{
		Model:       c.model,
		Messages:    messages,
		Temperature: temperature,
		MaxTokens:   maxTokens,
	})
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("failed to build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("http request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var chatResp chatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}
	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("API returned no choices")
	}

	return chatResp.Choices[0].Message.Content, nil
}
