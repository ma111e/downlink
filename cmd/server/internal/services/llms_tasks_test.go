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

	if got := lastTaskName(getAnalysisTasks(contentLen, false, false)); got != "rubric" {
		t.Errorf("rubric mode: last task = %q, want \"rubric\"", got)
	}

	if got := lastTaskName(getAnalysisTasks(contentLen, false, true)); got != "importance" {
		t.Errorf("vibe mode: last task = %q, want \"importance\"", got)
	}
}

func TestGetAnalysisTasksFastModeUnaffectedByVibe(t *testing.T) {
	// Fast mode only extracts key_points and has no scoring task regardless of vibe.
	for _, vibe := range []bool{false, true} {
		tasks := getAnalysisTasks(2000, true, vibe)
		if len(tasks) != 1 || tasks[0].name != "key_points" {
			t.Errorf("fast mode (vibe=%v): got %d tasks, want single key_points task", vibe, len(tasks))
		}
	}
}
