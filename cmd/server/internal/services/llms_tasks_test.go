package services

import "testing"

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

	if got := lastTaskName(getAnalysisTasks(contentLen, false, false, false)); got != "rubric" {
		t.Errorf("rubric mode: last task = %q, want \"rubric\"", got)
	}

	if got := lastTaskName(getAnalysisTasks(contentLen, false, true, false)); got != "importance" {
		t.Errorf("vibe mode: last task = %q, want \"importance\"", got)
	}
}

func TestGetAnalysisTasksBeginnerMode(t *testing.T) {
	const contentLen = 2000

	names := func(tasks []analysisTask) map[string]bool {
		m := make(map[string]bool, len(tasks))
		for _, t := range tasks {
			m[t.name] = true
		}
		return m
	}

	if names(getAnalysisTasks(contentLen, false, false, false))["beginner"] {
		t.Error("beginner disabled: beginner task should not be present")
	}
	if !names(getAnalysisTasks(contentLen, false, false, true))["beginner"] {
		t.Error("beginner enabled: beginner task should be present")
	}
	// The scoring task must still come last regardless of beginner mode.
	if got := lastTaskName(getAnalysisTasks(contentLen, false, false, true)); got != "rubric" {
		t.Errorf("beginner enabled: last task = %q, want \"rubric\"", got)
	}
}

func TestGetAnalysisTasksFastModeUnaffectedByVibe(t *testing.T) {
	// Fast mode only extracts key_points and has no scoring task regardless of vibe.
	for _, vibe := range []bool{false, true} {
		tasks := getAnalysisTasks(2000, true, vibe, true)
		if len(tasks) != 1 || tasks[0].name != "key_points" {
			t.Errorf("fast mode (vibe=%v): got %d tasks, want single key_points task", vibe, len(tasks))
		}
	}
}
