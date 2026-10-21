package apichat

import "testing"

func TestSimilarityCatchesNearDuplicates(t *testing.T) {
	if got := similarity("ok fine lol", "ok fine lol"); got != 1 {
		t.Fatalf("identical similarity = %v", got)
	}
	if got := similarity("ok fine lol", "ok fine lmao"); got < 0.5 {
		t.Fatalf("near duplicate scored too low: %v", got)
	}
	if got := similarity("u got snap?", "how old r u"); got > 0.3 {
		t.Fatalf("unrelated lines scored too high: %v", got)
	}
}

func TestRepeatedOpenerAndQuestion(t *testing.T) {
	history := []Message{
		{Role: "assistant", Content: "ok fine lol"},
		{Role: "user", Content: "hey"},
	}
	if !repeatedOpener("ok fine yeah", history) {
		t.Fatal("same opener not detected")
	}
	if repeatedOpener("lol same", history) {
		t.Fatal("different opener flagged")
	}

	questions := []Message{{Role: "assistant", Content: "whats ur age"}}
	if !repeatedQuestion("how old r u?", questions) {
		t.Fatal("same age question not detected")
	}
	if repeatedQuestion("u got snap?", questions) {
		t.Fatal("a different question was flagged")
	}
}

func TestMostSimilarReturnsTheClosestLine(t *testing.T) {
	history := []Message{
		{Role: "assistant", Content: "lol same"},
		{Role: "assistant", Content: "ok fine lol"},
	}
	score, match := mostSimilar("ok fine lmao", history, 8)
	if score < 0.5 || match != "ok fine lol" {
		t.Fatalf("score=%v match=%q", score, match)
	}
}

func TestCountQuestions(t *testing.T) {
	if got := countQuestions([]string{"hey?", "wyd?"}); got != 2 {
		t.Fatalf("count = %d", got)
	}
	if got := countQuestions([]string{"lol"}); got != 0 {
		t.Fatalf("count = %d", got)
	}
}
