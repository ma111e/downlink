package notification

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRenderRootLanding(t *testing.T) {
	profiles := []LandingProfile{
		{Slug: "beginner", Name: "Security 101", Description: "Approachable news", Icon: "🧑‍🎓", Subdir: "beginner"},
		{Slug: "red-team", Name: "Offensive Brief", Subdir: "red-team"},
	}
	html, err := renderRootLanding(profiles)
	if err != nil {
		t.Fatalf("renderRootLanding: %v", err)
	}
	s := string(html)
	for _, want := range []string{
		`href="beginner/"`,
		`Security 101`,
		`Approachable news`,
		`🧑‍🎓`,
		`href="red-team/"`,
		`Offensive Brief`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("landing HTML missing %q", want)
		}
	}
}

func TestProfilesJSON(t *testing.T) {
	profiles := []LandingProfile{{Slug: "a", Name: "A", Subdir: "a"}}
	data, err := ProfilesJSON(profiles)
	if err != nil {
		t.Fatalf("ProfilesJSON: %v", err)
	}
	var out struct {
		Profiles []LandingProfile `json:"profiles"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out.Profiles) != 1 || out.Profiles[0].Slug != "a" || out.Profiles[0].Subdir != "a" {
		t.Errorf("round-trip mismatch: %+v", out.Profiles)
	}
}
