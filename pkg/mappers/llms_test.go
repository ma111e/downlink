package mappers

import (
	"testing"

	"github.com/ma111e/downlink/pkg/models"
)

func TestMessageRoundTrip(t *testing.T) {
	in := models.Message{Role: "user", Content: "hello"}
	out := MessageToModel(MessageToProto(in))
	if out.Role != "user" || out.Content != "hello" {
		t.Errorf("round-trip lost: %+v", out)
	}
}

func TestMessageToModelNilReturnsZero(t *testing.T) {
	out := MessageToModel(nil)
	if out.Role != "" || out.Content != "" {
		t.Errorf("nil proto gave non-zero: %+v", out)
	}
}

func TestMessagesRoundTrip(t *testing.T) {
	in := []models.Message{
		{Role: "system", Content: "you are helpful"},
		{Role: "user", Content: "summarise this"},
	}
	out := MessagesToModel(MessagesToProto(in))
	if len(out) != 2 || out[0].Role != "system" || out[1].Content != "summarise this" {
		t.Errorf("slice round-trip lost: %+v", out)
	}
}

// GenericLLMRequest verifies the float64→float32→float64 and int→int32→int casts.
func TestGenericLLMRequestRoundTrip(t *testing.T) {
	in := models.GenericLLMRequest{
		Model:       "claude-opus",
		Temperature: 0.7,
		MaxTokens:   2048,
		Prompt:      "do something",
		Messages:    []models.Message{{Role: "user", Content: "hi"}},
	}
	out := GenericLLMRequestToModel(GenericLLMRequestToProto(in))
	if out.Model != "claude-opus" || out.Prompt != "do something" {
		t.Errorf("string fields lost: %+v", out)
	}
	// float32 round-trip loses precision; verify within tolerance.
	if out.Temperature < 0.69 || out.Temperature > 0.71 {
		t.Errorf("Temperature = %v, want ~0.7", out.Temperature)
	}
	if out.MaxTokens != 2048 {
		t.Errorf("MaxTokens = %d, want 2048", out.MaxTokens)
	}
	if len(out.Messages) != 1 || out.Messages[0].Role != "user" {
		t.Errorf("Messages lost: %+v", out.Messages)
	}
}

func TestGenericLLMRequestToModelNilReturnsZero(t *testing.T) {
	out := GenericLLMRequestToModel(nil)
	if out.Model != "" || out.MaxTokens != 0 {
		t.Errorf("nil gave non-zero: %+v", out)
	}
}

func TestOpenAIResponseRoundTripWithChoices(t *testing.T) {
	in := models.OpenAIResponse{
		Id:    "cmpl-1",
		Model: "gpt-4",
		Choices: []struct {
			Index        int            `json:"index"`
			Message      models.Message `json:"message"`
			FinishReason string         `json:"finish_reason"`
		}{
			{Index: 0, Message: models.Message{Role: "assistant", Content: "answer"}, FinishReason: "stop"},
		},
	}
	out := OpenAIResponseToModel(OpenAIResponseToProto(in))
	if out.Id != "cmpl-1" || out.Model != "gpt-4" {
		t.Errorf("id/model lost: %+v", out)
	}
	if len(out.Choices) != 1 || out.Choices[0].Index != 0 || out.Choices[0].FinishReason != "stop" {
		t.Errorf("choice lost: %+v", out.Choices)
	}
	if out.Choices[0].Message.Content != "answer" {
		t.Errorf("choice message lost: %+v", out.Choices[0].Message)
	}
}

func TestOpenAIResponseToModelNilReturnsZero(t *testing.T) {
	out := OpenAIResponseToModel(nil)
	if out.Id != "" || len(out.Choices) != 0 {
		t.Errorf("nil gave non-zero: %+v", out)
	}
}

func TestAnthropicResponseRoundTripWithContent(t *testing.T) {
	in := models.AnthropicResponse{
		Id:    "msg-1",
		Type:  "message",
		Model: "claude-3",
		Content: []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}{
			{Type: "text", Text: "the answer"},
		},
	}
	out := AnthropicResponseToModel(AnthropicResponseToProto(in))
	if out.Id != "msg-1" || out.Type != "message" || out.Model != "claude-3" {
		t.Errorf("fields lost: %+v", out)
	}
	if len(out.Content) != 1 || out.Content[0].Type != "text" || out.Content[0].Text != "the answer" {
		t.Errorf("content lost: %+v", out.Content)
	}
}

func TestModelInfoRoundTrip(t *testing.T) {
	in := models.ModelInfo{
		Id:           "claude-opus",
		Name:         "claude-opus",
		DisplayName:  "Claude Opus",
		Description:  "Most capable",
		ProviderType: "claude",
	}
	out := ModelInfoToModel(ModelInfoToProto(in))
	if out.Id != "claude-opus" || out.DisplayName != "Claude Opus" || out.ProviderType != "claude" {
		t.Errorf("round-trip lost: %+v", out)
	}
}
