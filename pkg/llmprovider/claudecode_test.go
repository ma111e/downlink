package llmprovider

import "testing"

func TestClaudeCodeProvider_MaxTokensClamp(t *testing.T) {
	cases := []struct {
		name string
		in   int
		want int
	}{
		{"zero uses default", 0, defaultClaudeMaxTokens},
		{"negative uses default", -5, defaultClaudeMaxTokens},
		{"within ceiling preserved", 16000, 16000},
		{"large value clamped", 200000, maxClaudeOutputTokens},
		{"exact ceiling preserved", maxClaudeOutputTokens, maxClaudeOutputTokens},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := newClaudeCodeProviderFromPool("claude-sonnet-4-6", "", nil, tc.in, 0)
			if p.maxTokens != tc.want {
				t.Fatalf("maxTokens = %d, want %d", p.maxTokens, tc.want)
			}
		})
	}
}
