package llmprovider

import (
	"encoding/json"
	"testing"
)

// TestClaudeCodeResponseParsesUsage guards the messagesResponse JSON tags: the
// Anthropic Messages API returns usage as input_tokens/output_tokens.
func TestClaudeCodeResponseParsesUsage(t *testing.T) {
	raw := `{"content":[{"type":"text","text":"hi"}],"usage":{"input_tokens":1200,"output_tokens":80}}`
	var parsed messagesResponse
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if parsed.Usage.InputTokens != 1200 || parsed.Usage.OutputTokens != 80 {
		t.Errorf("usage not parsed: %+v", parsed.Usage)
	}
}

// TestCodexCompletedEventParsesUsage guards the responsesStreamEvent JSON tags:
// the Codex Responses API reports usage on the response.completed event.
func TestCodexCompletedEventParsesUsage(t *testing.T) {
	raw := `{"type":"response.completed","response":{"usage":{"input_tokens":1500,"output_tokens":120,"total_tokens":1620}}}`
	var ev responsesStreamEvent
	if err := json.Unmarshal([]byte(raw), &ev); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if ev.Type != "response.completed" || ev.Response == nil || ev.Response.Usage == nil {
		t.Fatalf("completed event/usage not parsed: %+v", ev)
	}
	if ev.Response.Usage.InputTokens != 1500 || ev.Response.Usage.OutputTokens != 120 || ev.Response.Usage.TotalTokens != 1620 {
		t.Errorf("usage values wrong: %+v", ev.Response.Usage)
	}
}

// TestExtractUsageNilSafe confirms extractUsage degrades to known=false.
func TestExtractUsageNilSafe(t *testing.T) {
	if _, known := extractUsage(nil); known {
		t.Error("nil message should be unknown usage")
	}
}
