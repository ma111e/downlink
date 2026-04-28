// Package llmutil holds small helpers shared between LLM-calling code paths.
package llmutil

import "strings"

// CleanLLMResponse strips reasoning/thinking blocks and markdown code fences
// from a raw LLM response so the remaining text is ready for JSON parsing.
//
// Thinking blocks:
//   - <think>...</think>  — both tags present; everything between is removed
//   - ...</think>         — orphan closer; everything before it is removed too
//
// Markdown fences: a leading ```lang\n and a trailing ``` are stripped if
// present. This handles the common "```json\n{...}\n```" wrapping.
func CleanLLMResponse(response string) string {
	cleaned := response

	for {
		endIdx := strings.Index(cleaned, "</think>")
		if endIdx == -1 {
			break
		}
		startIdx := strings.Index(cleaned, "<think>")
		if startIdx == -1 || startIdx > endIdx {
			cleaned = cleaned[endIdx+len("</think>"):]
		} else {
			cleaned = cleaned[:startIdx] + cleaned[endIdx+len("</think>"):]
		}
	}

	cleaned = strings.TrimSpace(cleaned)
	if strings.HasPrefix(cleaned, "```") {
		if idx := strings.Index(cleaned, "\n"); idx != -1 {
			cleaned = cleaned[idx+1:]
		}
		if idx := strings.LastIndex(cleaned, "```"); idx != -1 {
			cleaned = cleaned[:idx]
		}
	}

	return strings.TrimSpace(cleaned)
}

// ExtractJSON is a last-resort fallback used after CleanLLMResponse +
// json.Unmarshal fail: it returns the substring from the first '{' to the
// last '}', or the input unchanged if no braces are found.
func ExtractJSON(response string) string {
	start := strings.Index(response, "{")
	end := strings.LastIndex(response, "}")
	if start != -1 && end != -1 && end > start {
		return response[start : end+1]
	}
	return response
}
