package mappers

import (
	"downlink/pkg/models"
	"downlink/pkg/protos"
)

// Message mappers
func MessageToProto(msg models.Message) *protos.Message {
	return &protos.Message{
		Role:    msg.Role,
		Content: msg.Content,
	}
}

func MessageToModel(msg *protos.Message) models.Message {
	if msg == nil {
		return models.Message{}
	}
	return models.Message{
		Role:    msg.Role,
		Content: msg.Content,
	}
}

func MessagesToProto(messages []models.Message) []*protos.Message {
	var protoMessages []*protos.Message
	for _, msg := range messages {
		protoMessages = append(protoMessages, MessageToProto(msg))
	}
	return protoMessages
}

func MessagesToModel(messages []*protos.Message) []models.Message {
	var modelMessages []models.Message
	for _, msg := range messages {
		modelMessages = append(modelMessages, MessageToModel(msg))
	}
	return modelMessages
}

// GenericLLMRequest mappers
func GenericLLMRequestToProto(req models.GenericLLMRequest) *protos.GenericLLMRequest {
	return &protos.GenericLLMRequest{
		Model:       req.Model,
		Messages:    MessagesToProto(req.Messages),
		Prompt:      req.Prompt,
		Temperature: float32(req.Temperature),
		MaxTokens:   int32(req.MaxTokens),
	}
}

func GenericLLMRequestToModel(req *protos.GenericLLMRequest) models.GenericLLMRequest {
	if req == nil {
		return models.GenericLLMRequest{}
	}
	return models.GenericLLMRequest{
		Model:       req.Model,
		Messages:    MessagesToModel(req.Messages),
		Prompt:      req.Prompt,
		Temperature: float64(req.Temperature),
		MaxTokens:   int(req.MaxTokens),
	}
}

// OllamaRequest mappers
func OllamaRequestToProto(req models.OllamaRequest) *protos.OllamaRequest {
	return &protos.OllamaRequest{
		Model:  req.Model,
		Prompt: req.Prompt,
		Stream: req.Stream,
	}
}

func OllamaRequestToModel(req *protos.OllamaRequest) models.OllamaRequest {
	if req == nil {
		return models.OllamaRequest{}
	}
	return models.OllamaRequest{
		Model:  req.Model,
		Prompt: req.Prompt,
		Stream: req.Stream,
	}
}

// OllamaResponse mappers
func OllamaResponseToProto(res models.OllamaResponse) *protos.OllamaResponse {
	return &protos.OllamaResponse{
		Model:     res.Model,
		Response:  res.Response,
		CreatedAt: res.CreatedAt,
	}
}

func OllamaResponseToModel(res *protos.OllamaResponse) models.OllamaResponse {
	if res == nil {
		return models.OllamaResponse{}
	}
	return models.OllamaResponse{
		Model:     res.Model,
		Response:  res.Response,
		CreatedAt: res.CreatedAt,
	}
}

// OpenAIRequest mappers
func OpenAIRequestToProto(req models.OpenAIRequest) *protos.OpenAIRequest {
	return &protos.OpenAIRequest{
		Model:       req.Model,
		Messages:    MessagesToProto(req.Messages),
		Temperature: float32(req.Temperature),
		MaxTokens:   int32(req.MaxTokens),
	}
}

func OpenAIRequestToModel(req *protos.OpenAIRequest) models.OpenAIRequest {
	if req == nil {
		return models.OpenAIRequest{}
	}
	return models.OpenAIRequest{
		Model:       req.Model,
		Messages:    MessagesToModel(req.Messages),
		Temperature: float64(req.Temperature),
		MaxTokens:   int(req.MaxTokens),
	}
}

// OpenAIChoice mappers
func OpenAIChoiceToProto(choice struct {
	Index        int            `json:"index"`
	Message      models.Message `json:"message"`
	FinishReason string         `json:"finish_reason"`
}) *protos.OpenAIChoice {
	return &protos.OpenAIChoice{
		Index:        int32(choice.Index),
		Message:      MessageToProto(choice.Message),
		FinishReason: choice.FinishReason,
	}
}

// OpenAIResponse mappers
func OpenAIResponseToProto(res models.OpenAIResponse) *protos.OpenAIResponse {
	var choices []*protos.OpenAIChoice
	for _, choice := range res.Choices {
		choices = append(choices, OpenAIChoiceToProto(choice))
	}

	return &protos.OpenAIResponse{
		Id:      res.Id,
		Object:  res.Object,
		Created: res.Created,
		Model:   res.Model,
		Choices: choices,
	}
}

func OpenAIResponseToModel(res *protos.OpenAIResponse) models.OpenAIResponse {
	if res == nil {
		return models.OpenAIResponse{}
	}

	modelResponse := models.OpenAIResponse{
		Id:      res.Id,
		Object:  res.Object,
		Created: res.Created,
		Model:   res.Model,
		Choices: []struct {
			Index        int            `json:"index"`
			Message      models.Message `json:"message"`
			FinishReason string         `json:"finish_reason"`
		}{},
	}

	for _, choice := range res.Choices {
		modelResponse.Choices = append(modelResponse.Choices, struct {
			Index        int            `json:"index"`
			Message      models.Message `json:"message"`
			FinishReason string         `json:"finish_reason"`
		}{
			Index:        int(choice.Index),
			Message:      MessageToModel(choice.Message),
			FinishReason: choice.FinishReason,
		})
	}

	return modelResponse
}

// AnthropicRequest mappers
func AnthropicRequestToProto(req models.AnthropicRequest) *protos.AnthropicRequest {
	return &protos.AnthropicRequest{
		Model:       req.Model,
		Messages:    MessagesToProto(req.Messages),
		Temperature: float32(req.Temperature),
		MaxTokens:   int32(req.MaxTokens),
	}
}

func AnthropicRequestToModel(req *protos.AnthropicRequest) models.AnthropicRequest {
	if req == nil {
		return models.AnthropicRequest{}
	}
	return models.AnthropicRequest{
		Model:       req.Model,
		Messages:    MessagesToModel(req.Messages),
		Temperature: float64(req.Temperature),
		MaxTokens:   int(req.MaxTokens),
	}
}

// AnthropicContentItem mappers
func AnthropicContentItemToProto(content struct {
	Type string `json:"type"`
	Text string `json:"text"`
}) *protos.AnthropicContentItem {
	return &protos.AnthropicContentItem{
		Type: content.Type,
		Text: content.Text,
	}
}

// AnthropicResponse mappers
func AnthropicResponseToProto(res models.AnthropicResponse) *protos.AnthropicResponse {
	var content []*protos.AnthropicContentItem
	for _, item := range res.Content {
		content = append(content, AnthropicContentItemToProto(item))
	}

	return &protos.AnthropicResponse{
		Id:      res.Id,
		Type:    res.Type,
		Model:   res.Model,
		Content: content,
	}
}

func AnthropicResponseToModel(res *protos.AnthropicResponse) models.AnthropicResponse {
	if res == nil {
		return models.AnthropicResponse{}
	}

	modelResponse := models.AnthropicResponse{
		Id:    res.Id,
		Type:  res.Type,
		Model: res.Model,
		Content: []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}{},
	}

	for _, item := range res.Content {
		modelResponse.Content = append(modelResponse.Content, struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}{
			Type: item.Type,
			Text: item.Text,
		})
	}

	return modelResponse
}

// ModelInfo mappers
func ModelInfoToProto(info models.ModelInfo) *protos.ModelInfo {
	return &protos.ModelInfo{
		Id:           info.Id,
		Name:         info.Name,
		DisplayName:  info.DisplayName,
		Description:  info.Description,
		ProviderType: info.ProviderType,
	}
}

func ModelInfoToModel(info *protos.ModelInfo) models.ModelInfo {
	if info == nil {
		return models.ModelInfo{}
	}
	return models.ModelInfo{
		Id:           info.Id,
		Name:         info.Name,
		DisplayName:  info.DisplayName,
		Description:  info.Description,
		ProviderType: info.ProviderType,
	}
}

func AllModelInfoToProto(models []models.ModelInfo) []*protos.ModelInfo {
	var protoInfos []*protos.ModelInfo
	for _, info := range models {
		protoInfos = append(protoInfos, ModelInfoToProto(info))
	}
	return protoInfos
}

func AllModelInfoToModel(protos []*protos.ModelInfo) []models.ModelInfo {
	var modelInfos []models.ModelInfo
	for _, info := range protos {
		modelInfos = append(modelInfos, ModelInfoToModel(info))
	}
	return modelInfos
}

// ModelsResponse mappers
func ModelsResponseToProto(res models.ModelsResponse) *protos.ModelsResponse {
	return &protos.ModelsResponse{
		Models: AllModelInfoToProto(res.Models),
		Error:  res.Error,
	}
}

func ModelsResponseToModel(res *protos.ModelsResponse) models.ModelsResponse {
	if res == nil {
		return models.ModelsResponse{}
	}
	return models.ModelsResponse{
		Models: AllModelInfoToModel(res.Models),
		Error:  res.Error,
	}
}
