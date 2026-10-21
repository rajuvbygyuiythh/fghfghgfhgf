package apichat

import "testing"

func TestAnalyzeImplicitAskWithoutQuestionMark(t *testing.T) {
	p := analyzeMessage("i bet you're a college girl")
	if !p.ImplicitAsk {
		t.Fatalf("implicit ask not detected: %+v", p)
	}
	if p.IsQuestion {
		t.Fatalf("there is no question mark or question word: %+v", p)
	}
	if p.Kind != KindImplicitAsk {
		t.Fatalf("kind = %s, want %s", p.Kind, KindImplicitAsk)
	}
}

func TestAnalyzeCommandAndPicRequest(t *testing.T) {
	p := analyzeMessage("send me a pic")
	if p.Kind != KindCommand {
		t.Fatalf("kind = %s, want command", p.Kind)
	}
	if p.Command != "pic" {
		t.Fatalf("command = %q, want pic", p.Command)
	}
}

func TestAnalyzeMultiPartSharing(t *testing.T) {
	p := analyzeMessage("im tired from work and my boss is annoying")
	if p.Kind != KindSharing {
		t.Fatalf("kind = %s, want sharing", p.Kind)
	}
	if len(p.Parts) != 2 {
		t.Fatalf("expected 2 parts, got %v", p.Parts)
	}
	if !containsTopic(p.Topics, "work") || !containsTopic(p.Topics, "sleep") {
		t.Fatalf("topics = %v", p.Topics)
	}
	if len(p.Keywords) < 2 {
		t.Fatalf("keywords = %v", p.Keywords)
	}
}

func TestAnalyzeLowEffortAndGreeting(t *testing.T) {
	if got := analyzeMessage("k").Kind; got != KindLowEffort {
		t.Fatalf("k -> %s", got)
	}
	if got := analyzeMessage("hey").Kind; got != KindGreeting {
		t.Fatalf("hey -> %s", got)
	}
	if got := analyzeMessage("wyd").Kind; got != KindQuestion {
		t.Fatalf("wyd -> %s", got)
	}
}

func TestHasAnchorUsesKeywordsAndTopics(t *testing.T) {
	p := analyzeMessage("i work at a coffee shop downtown")
	if !hasAnchor([]string{"long shift huh"}, p) {
		t.Fatal("shift should anchor to the work topic")
	}
	if hasAnchor([]string{"lol"}, p) {
		t.Fatal("lol does not anchor")
	}
	if !hasAnchor([]string{"lol"}, analyzeMessage("k")) {
		t.Fatal("low-effort messages must not require an anchor")
	}
}
