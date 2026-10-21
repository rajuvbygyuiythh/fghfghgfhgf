package apichat

import (
	"strings"
	"testing"
)

func TestGuardRejectsKnownFactQuestion(t *testing.T) {
	d := &Director{UserAge: 21}
	d.syncLedger()
	problem := validateReply([]string{"how old r u?"}, d, analyzeMessage("im tired from work"), "im tired from work", nil)
	if !strings.Contains(problem, "already told") {
		t.Fatalf("known-fact question accepted: %q", problem)
	}
}

func TestGuardRequiresAnAnchorForSharing(t *testing.T) {
	d := NewDirector(9)
	user := "i work at a coffee shop downtown"
	problem := validateReply([]string{"lol"}, &d, analyzeMessage(user), user, nil)
	if !strings.Contains(problem, "refer to anything") {
		t.Fatalf("anchorless reply accepted: %q", problem)
	}
	problem = validateReply([]string{"long shift huh"}, &d, analyzeMessage(user), user, nil)
	if problem != "" {
		t.Fatalf("anchored reply rejected: %q", problem)
	}
}

func TestGuardAnswersQuestions(t *testing.T) {
	d := NewDirector(2)
	user := "how old r u"
	problem := validateReply([]string{"wyd?"}, &d, analyzeMessage(user), user, nil)
	if !strings.Contains(problem, "did not answer") {
		t.Fatalf("question-only reply accepted: %q", problem)
	}
	if problem := validateReply([]string{"f 22, u?"}, &d, analyzeMessage(user), user, nil); problem != "" {
		t.Fatalf("answer rejected: %q", problem)
	}
}

func TestGuardBlocksRepeatsAndQuestionSpam(t *testing.T) {
	d := NewDirector(4)
	history := []Message{
		{Role: "assistant", Content: "ok fine lol"},
		{Role: "user", Content: "hey"},
	}
	problem := validateReply([]string{"ok fine lmao"}, &d, analyzeMessage("hey"), "hey", history)
	if !strings.Contains(problem, "repeated") {
		t.Fatalf("near duplicate accepted: %q", problem)
	}

	spam := []string{"ok so whats ur fav food?", "and whats ur fav show?"}
	problem = validateReply(spam, &d, analyzeMessage("i love music and food"), "i love music and food", nil)
	if !strings.Contains(problem, "more than one question") {
		t.Fatalf("question spam accepted: %q", problem)
	}
}

func TestSafeReplyMatchesTheMessageType(t *testing.T) {
	if got := safeReply(analyzeMessage("how old r u")); got[0] != "idk tbh" {
		t.Fatalf("question fallback = %v", got)
	}
	if got := safeReply(analyzeMessage("ur cute")); !strings.Contains(got[0], "blush") {
		t.Fatalf("flirt fallback = %v", got)
	}
	if got := safeReply(analyzeMessage("k")); got[0] != "lol fair" {
		t.Fatalf("low effort fallback = %v", got)
	}
}
