package apichat

import "testing"

func TestStageProgression(t *testing.T) {
	d := &Director{}
	if got := stageFor(d, 1, 6); got != StageRapport {
		t.Fatalf("turn 1 = %s, want rapport", got)
	}
	if got := stageFor(d, 3, 6); got != StageInterest {
		t.Fatalf("turn 3 = %s, want interest", got)
	}
	if got := stageFor(d, 4, 6); got != StageHook {
		t.Fatalf("turn 4 = %s, want hook", got)
	}
	d.AskedSnap = true
	if got := stageFor(d, 5, 6); got != StageAsk {
		t.Fatalf("asked snap = %s, want ask", got)
	}
	d.Shared = true
	if got := stageFor(d, 6, 6); got != StageClose {
		t.Fatalf("shared = %s, want close", got)
	}
}

func TestStageGoalMentionsTheTopic(t *testing.T) {
	p := analyzeMessage("my shift at work was so long today")
	goal := stageGoal(StageInterest, p)
	if goal == "" || !containsTopic(p.Topics, "work") {
		t.Fatalf("goal = %q topics = %v", goal, p.Topics)
	}
	for _, stage := range []Stage{StageRapport, StageHook, StageAsk, StageShare, StageClose} {
		if stageGoal(stage, p) == "" {
			t.Fatalf("stage %s has no goal", stage)
		}
	}
}
