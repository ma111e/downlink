package services

import (
	"context"
	"testing"

	"github.com/ma111e/downlink/pkg/protos"
)

// Invalid YAML must fail during parsing, before the handler reaches the global
// manager (which is nil in unit tests — reaching it would panic, not error).
func TestApplyProfilesRejectsInvalidYAML(t *testing.T) {
	s := NewProfilesServer()
	_, err := s.ApplyProfiles(context.Background(), &protos.ApplyProfilesRequest{
		ProfilesYaml: "profiles:\n  - slug: [unclosed",
	})
	if err == nil {
		t.Fatal("expected a parse error for invalid YAML")
	}
}
