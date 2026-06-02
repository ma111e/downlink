package models

// GenericLLMRequest represents a generic request to an LLM provider
type GenericLLMRequest struct {
	Model       string    `json:"model,omitempty"`       // Model name
	Messages    []Message `json:"messages,omitempty"`    // For chat-based APIs (OpenAI, Anthropic)
	Prompt      string    `json:"prompt,omitempty"`      // For completion-based APIs (Ollama)
	Temperature float64   `json:"temperature,omitempty"` // Optional
	MaxTokens   int       `json:"max_tokens,omitempty"`  // Optional
}

// Message represents a message in a chat-based LLM request
type Message struct {
	Role    string `json:"role"`    // "system", "user", "assistant"
	Content string `json:"content"` // Message content
}

// OllamaRequest represents the request to the Ollama API
type OllamaRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
}

// OllamaResponse represents the response from the Ollama API
type OllamaResponse struct {
	Model     string `json:"model"`
	Response  string `json:"response"`
	CreatedAt string `json:"created_at"`
}

// OpenAIRequest represents a request to the OpenAI API
type OpenAIRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Temperature float64   `json:"temperature,omitempty"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
}

// OpenAIResponse represents a response from the OpenAI API
type OpenAIResponse struct {
	Id      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index        int     `json:"index"`
		Message      Message `json:"message"`
		FinishReason string  `json:"finish_reason"`
	} `json:"choices"`
}

// AnthropicRequest represents a request to the Anthropic API
type AnthropicRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Temperature float64   `json:"temperature,omitempty"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
}

// AnthropicResponse represents a response from the Anthropic API
type AnthropicResponse struct {
	Id      string `json:"id"`
	Type    string `json:"type"`
	Model   string `json:"model"`
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
}

// ModelInfo represents generic model information
type ModelInfo struct {
	Id           string `json:"id"`
	Name         string `json:"name"`
	DisplayName  string `json:"display_name,omitempty"`
	Description  string `json:"description,omitempty"`
	ProviderType string `json:"provider_type"`
}

// ModelsResponse represents the response structure for GetAvailableModels
type ModelsResponse struct {
	Models []ModelInfo `json:"models"`
	Error  string      `json:"error,omitempty"`
}

// OpenAIModelsResponse represents the response from OpenAI's models endpoint
type OpenAIModelsResponse struct {
	Object string `json:"object"`
	Data   []struct {
		Id      string `json:"id"`
		Object  string `json:"object"`
		Created int64  `json:"created"`
		OwnedBy string `json:"owned_by"`
	} `json:"data"`
}

// AnthropicModelsResponse represents the response from Anthropic's models endpoint
type AnthropicModelsResponse struct {
	Data []struct {
		Type           string `json:"type"`
		Id             string `json:"id"`
		DisplayName    string `json:"display_name"`
		CreatedAt      string `json:"created_at"`
		MaxInputTokens int    `json:"max_input_tokens"`
		MaxTokens      int    `json:"max_tokens"`
	} `json:"data"`
	HasMore bool   `json:"has_more"`
	FirstId string `json:"first_id"`
	LastId  string `json:"last_id"`
}

// OllamaModelsResponse represents the response from Ollama's models endpoint
type OllamaModelsResponse struct {
	Models []struct {
		Name       string             `json:"name"`
		Model      string             `json:"model"`
		ModifiedAt string             `json:"modified_at"`
		Size       int64              `json:"size"`
		Digest     string             `json:"digest"`
		Details    OllamaModelDetails `json:"details"`
	} `json:"models"`
}

// OllamaModelDetails represents the details of an Ollama model
type OllamaModelDetails struct {
	ParentModel       string   `json:"parent_model"`
	Format            string   `json:"format"`
	Family            string   `json:"family"`
	Families          []string `json:"families"`
	ParameterSize     string   `json:"parameter_size"`
	QuantizationLevel string   `json:"quantization_level"`
}

// LlamaCppModelsResponse handles the hybrid response format from llama.cpp /models endpoint,
// which includes both a custom "models" array and an OpenAI-compatible "data" array.
type LlamaCppModelsResponse struct {
	Object string `json:"object"`
	// OpenAI-compatible data array (preferred)
	Data []struct {
		Id      string `json:"id"`
		Object  string `json:"object"`
		Created int64  `json:"created"`
		OwnedBy string `json:"owned_by"`
	} `json:"data"`
	// llama.cpp native models array (fallback)
	Models []struct {
		Name  string `json:"name"`
		Model string `json:"model"`
	} `json:"models"`
}
