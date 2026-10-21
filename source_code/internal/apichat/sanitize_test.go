package apichat

import (
	"strings"
	"testing"
)

func TestSanitizeLinesCleansModelOutput(t *testing.T) {
	lines, warn := sanitizeLines("<think>she is 22</think>\n\"Hey There!\"\n- I'm Good.", true)
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %v (%s)", lines, warn)
	}
	if lines[0] != "hey there" {
		t.Fatalf("quotes/!/case not cleaned: %q", lines[0])
	}
	if lines[1] != "i'm good" {
		t.Fatalf("list marker/trailing dot not cleaned: %q", lines[1])
	}
	if !strings.Contains(warn, "think") {
		t.Fatalf("expected a think warning, got %q", warn)
	}
}

func TestSanitizeLinesKeepsOneLineWhenNotDouble(t *testing.T) {
	lines, warn := sanitizeLines("first line\nsecond line", false)
	if len(lines) != 1 || lines[0] != "first line" {
		t.Fatalf("expected one line, got %v", lines)
	}
	if !strings.Contains(warn, "kept 1") {
		t.Fatalf("expected a truncation warning, got %q", warn)
	}
}

func TestSanitizeLinesStripsModelEndTokens(t *testing.T) {
	lines, _ := sanitizeLines("hey what's up?<|end|>\nhow are u<|end|>", false)
	if len(lines) != 1 || lines[0] != "hey what's up?" {
		t.Fatalf("expected <|end|> stripped and trailing turns cut, got %v", lines)
	}
}

func TestOnlyQuestion(t *testing.T) {
	for _, line := range []string{"wyd?", "u got snap?", "what do u do?"} {
		if !onlyQuestion(line) {
			t.Fatalf("%q should be a bare question", line)
		}
	}
	for _, line := range []string{"lol idk", "f 20, wyd?", "haha true"} {
		if onlyQuestion(line) {
			t.Fatalf("%q is not a bare question", line)
		}
	}
}

func TestParrotGramFindsFourWordEcho(t *testing.T) {
	user := "i work at a coffee shop downtown"
	reply := "oh nice i work at a coffee shop too"
	if got := parrotGram(user, reply); got != "i work at a" {
		t.Fatalf("parrot gram = %q", got)
	}
	if got := parrotGram("hey", "hey there"); got != "" {
		t.Fatalf("short messages must not trigger: %q", got)
	}
}

func TestPolishDropsRepeatedEmojiAndLolOpener(t *testing.T) {
	history := []Message{
		{Role: "assistant", Content: "lol that's wild"},
		{Role: "assistant", Content: "so cute 😏"},
	}
	out := polish([]string{"lol same 😏😏"}, history)
	if len(out) != 1 {
		t.Fatalf("unexpected output: %v", out)
	}
	if strings.Contains(out[0], "😏") {
		t.Fatalf("emoji already used recently should be removed: %q", out[0])
	}
	if strings.HasPrefix(out[0], "lol ") {
		t.Fatalf("repeated lol opener should be removed: %q", out[0])
	}
}

func TestAnswerProblemCatchesAITalkAndRepeats(t *testing.T) {
	d := &Director{}
	problem := answerProblem([]string{"as an ai language model i cannot"}, d, "hey", nil)
	if !strings.Contains(problem, "ai") {
		t.Fatalf("expected ai-talk problem, got %q", problem)
	}
	history := []Message{{Role: "assistant", Content: "lol same"}}
	problem = answerProblem([]string{"lol same"}, d, "wyd", history)
	if problem == "" {
		t.Fatal("expected a repeated-line problem")
	}
}
