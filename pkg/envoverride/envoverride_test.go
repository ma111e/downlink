package envoverride

import (
	"os"
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name      string
		entry     string
		wantKey   string
		wantValue string
		wantErr   bool
	}{
		{name: "basic", entry: "DOWNLINK_LOG_LEVEL=debug", wantKey: "DOWNLINK_LOG_LEVEL", wantValue: "debug"},
		{name: "empty value", entry: "DOWNLINK_GH_PAGES_TOKEN=", wantKey: "DOWNLINK_GH_PAGES_TOKEN", wantValue: ""},
		{name: "value contains equals", entry: "TOKEN=a=b=c", wantKey: "TOKEN", wantValue: "a=b=c"},
		{name: "missing equals", entry: "DOWNLINK_LOG_LEVEL", wantErr: true},
		{name: "empty key", entry: "=debug", wantErr: true},
		{name: "starts with digit", entry: "1DOWNLINK=value", wantErr: true},
		{name: "contains dash", entry: "DOWNLINK-LOG=value", wantErr: true},
		{name: "contains space", entry: "DOWNLINK LOG=value", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotKey, gotValue, err := Parse(tt.entry)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Parse(%q) error = nil, want error", tt.entry)
				}
				return
			}
			if err != nil {
				t.Fatalf("Parse(%q) error = %v", tt.entry, err)
			}
			if gotKey != tt.wantKey || gotValue != tt.wantValue {
				t.Fatalf("Parse(%q) = (%q, %q), want (%q, %q)", tt.entry, gotKey, gotValue, tt.wantKey, tt.wantValue)
			}
		})
	}
}

func TestApply(t *testing.T) {
	t.Setenv("DOWNLINK_ENV_OVERRIDE_TEST", "old")

	if err := Apply([]string{"DOWNLINK_ENV_OVERRIDE_TEST=new"}); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	if got := os.Getenv("DOWNLINK_ENV_OVERRIDE_TEST"); got != "new" {
		t.Fatalf("DOWNLINK_ENV_OVERRIDE_TEST = %q, want %q", got, "new")
	}
}
