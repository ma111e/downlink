package scrapers

import (
	"strings"
	"testing"
)

func TestParseWebSocketDebuggerURL(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "present",
			body: `{"Browser":"Lightpanda","webSocketDebuggerUrl":"ws://127.0.0.1:9222/devtools/browser/abc"}`,
			want: "ws://127.0.0.1:9222/devtools/browser/abc",
		},
		{"absent", `{"Browser":"Lightpanda"}`, ""},
		{"empty body", ``, ""},
		{"garbage", `not json`, ""},
		{"trims whitespace", `{"webSocketDebuggerUrl":"  ws://x/y  "}`, "ws://x/y"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseWebSocketDebuggerURL(strings.NewReader(tt.body))
			if got != tt.want {
				t.Errorf("parseWebSocketDebuggerURL(%q) = %q, want %q", tt.body, got, tt.want)
			}
		})
	}
}
