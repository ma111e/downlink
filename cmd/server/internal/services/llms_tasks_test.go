package services

import (
	"strings"
	"testing"
)

// lastTaskName returns the name of the final task in the pipeline, which is the
// scoring task (importance for legacy vibe mode, rubric otherwise).
func lastTaskName(tasks []analysisTask) string {
	if len(tasks) == 0 {
		return ""
	}
	return tasks[len(tasks)-1].name
}

func TestGetAnalysisTasksScoringMode(t *testing.T) {
	const contentLen = 2000 // > 1000 so summaries task is included too

	if got := lastTaskName(getAnalysisTasks(contentLen, false, false, false, false, false)); got != "rubric" {
		t.Errorf("rubric mode: last task = %q, want \"rubric\"", got)
	}

	if got := lastTaskName(getAnalysisTasks(contentLen, false, true, false, false, false)); got != "importance" {
		t.Errorf("vibe mode: last task = %q, want \"importance\"", got)
	}
}

func TestGetAnalysisTasksGlossaryMode(t *testing.T) {
	const contentLen = 2000

	names := func(tasks []analysisTask) map[string]bool {
		m := make(map[string]bool, len(tasks))
		for _, t := range tasks {
			m[t.name] = true
		}
		return m
	}

	if names(getAnalysisTasks(contentLen, false, false, false, false, false))["glossary"] {
		t.Error("glossary disabled: glossary task should not be present")
	}
	if !names(getAnalysisTasks(contentLen, false, false, true, false, false))["glossary"] {
		t.Error("glossary enabled: glossary task should be present")
	}
	// The scoring task must still come last regardless of glossary mode.
	if got := lastTaskName(getAnalysisTasks(contentLen, false, false, true, false, false)); got != "rubric" {
		t.Errorf("glossary enabled: last task = %q, want \"rubric\"", got)
	}
}

func TestGetAnalysisTasksAlwaysIncludesWhyItMatters(t *testing.T) {
	const contentLen = 2000

	has := func(tasks []analysisTask, name string) bool {
		for _, task := range tasks {
			if task.name == name {
				return true
			}
		}
		return false
	}

	// why_it_matters is a core, always-on task regardless of mode flags.
	if !has(getAnalysisTasks(contentLen, false, false, false, false, false), "why_it_matters") {
		t.Error("why_it_matters task should always be present (rubric mode)")
	}
	if !has(getAnalysisTasks(contentLen, false, true, false, false, false), "why_it_matters") {
		t.Error("why_it_matters task should always be present (vibe mode)")
	}
}

func TestGetAnalysisTasksSummaryLevels(t *testing.T) {
	const contentLen = 2000 // > 1000 so the summaries task is included

	summariesSchema := func(standard, comprehensive bool) string {
		for _, task := range getAnalysisTasks(contentLen, false, false, false, standard, comprehensive) {
			if task.name == "summaries" {
				return task.schema
			}
		}
		t.Fatal("summaries task not present")
		return ""
	}

	// brief_overview is always requested; standard/comprehensive only when enabled.
	cases := []struct {
		standard, comprehensive bool
	}{
		{false, false},
		{true, false},
		{false, true},
		{true, true},
	}
	for _, c := range cases {
		schema := summariesSchema(c.standard, c.comprehensive)
		if !strings.Contains(schema, "brief_overview") {
			t.Errorf("standard=%v comprehensive=%v: schema missing brief_overview: %s", c.standard, c.comprehensive, schema)
		}
		if got := strings.Contains(schema, "standard_synthesis"); got != c.standard {
			t.Errorf("standard=%v: schema standard_synthesis present=%v, want %v", c.standard, got, c.standard)
		}
		if got := strings.Contains(schema, "comprehensive_synthesis"); got != c.comprehensive {
			t.Errorf("comprehensive=%v: schema comprehensive_synthesis present=%v, want %v", c.comprehensive, got, c.comprehensive)
		}
	}
}

func TestGetAnalysisTasksFastModeUnaffectedByVibe(t *testing.T) {
	// Fast mode only extracts key_points and has no scoring task regardless of vibe.
	for _, vibe := range []bool{false, true} {
		tasks := getAnalysisTasks(2000, true, vibe, true, false, false)
		if len(tasks) != 1 || tasks[0].name != "key_points" {
			t.Errorf("fast mode (vibe=%v): got %d tasks, want single key_points task", vibe, len(tasks))
		}
	}
}
