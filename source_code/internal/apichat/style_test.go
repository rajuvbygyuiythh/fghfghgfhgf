package apichat

import "testing"

func TestPickStyleUsesTheRoll(t *testing.T) {
	d := NewDirector(7)
	seen := map[Style]bool{}
	for i := 0; i < 40; i++ {
		seen[pickStyle(&d)] = true
	}
	if len(seen) < 3 {
		t.Fatalf("style rotation is too narrow: %v", seen)
	}
}

func TestMirrorStyleFollowsTheStranger(t *testing.T) {
	cases := []struct {
		text string
		want Style
	}{
		{"lol u funny", StylePlayful},
		{"ur so cute", StyleFlirty},
		{"i cant sleep, feeling lonely", StyleSoft},
		{"k", StyleChill},
	}
	for _, c := range cases {
		if got := mirrorStyle(StyleConfident, analyzeMessage(c.text)); got != c.want {
			t.Fatalf("%q -> %s, want %s", c.text, got, c.want)
		}
	}
}

func TestInterestScoresEngagement(t *testing.T) {
	d := NewDirector(3)
	d.Profile = analyzeMessage("i love music, tell me what u listen to")
	d.LastTopic = "music"
	d.observeInterest()
	if d.Interest["music"] != 1 {
		t.Fatalf("engaged topic should score +1, got %v", d.Interest)
	}

	d.Profile = analyzeMessage("k")
	d.LastTopic = "work"
	d.observeInterest()
	if d.Interest["work"] != -1 {
		t.Fatalf("ignored topic should score -1, got %v", d.Interest)
	}
}

func TestPickTopicPrefersInterest(t *testing.T) {
	d := NewDirector(5)
	d.Interest = Interest{"music": 4}
	d.Profile = analyzeMessage("im bored")
	if got := d.pickTopic(); got != "music" {
		t.Fatalf("pickTopic = %s, want music", got)
	}
	if got := d.pickTopic(); got != "music" {
		t.Fatalf("top topic should stay stable, got %s", got)
	}
}

func TestStyleHintsExistForEveryArchetype(t *testing.T) {
	for _, s := range allStyles {
		if styleHint(s) == "" || styleMaxWords(s) <= 0 {
			t.Fatalf("style %s is not fully described", s)
		}
	}
}
