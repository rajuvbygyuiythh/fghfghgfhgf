package apichat

import (
	"math/rand"
	"strings"
	"testing"
)

func testSession(t *testing.T) Session {
	t.Helper()
	return NewSession(rand.New(rand.NewSource(7)))
}

func TestObserveReadsShortASLAndRemembersIt(t *testing.T) {
	s := testSession(t)
	s.Director.Observe("f 21 from texas, bored lol")
	if s.Director.UserGender != "f" || s.Director.UserAge != 21 {
		t.Fatalf("gender/age not parsed: %+v", s.Director)
	}
	if s.Director.UserPlace != "texas" {
		t.Fatalf("place not parsed: %q", s.Director.UserPlace)
	}
	facts, dontAsk := s.Director.Known()
	if !strings.Contains(facts, "21") || !strings.Contains(facts, "texas") {
		t.Fatalf("known facts missing: %q", facts)
	}
	if len(dontAsk) < 2 {
		t.Fatalf("dontAsk should protect age and place: %v", dontAsk)
	}
}

func TestUnderageGateEndsImmediately(t *testing.T) {
	s := testSession(t)
	s.Director.Observe("m 16")
	p := s.Director.Plan(2, "m 16", 4, 6, 8)
	if !p.EndAfter || p.Fixed == "" {
		t.Fatalf("underage must end with a fixed line: %+v", p)
	}
	if !s.Director.Ended || s.Director.EndReason != "underage" {
		t.Fatalf("director not ended: %+v", s.Director)
	}
}

func TestPlanAnswersASLWithoutAskingBackWhenKnown(t *testing.T) {
	s := testSession(t)
	s.Director.Observe("m 24")
	p := s.Director.Plan(2, "m 24", 4, 6, 8)
	if strings.Contains(strings.ToLower(p.Instruction), "u?") {
		t.Fatalf("must not ask u? when the age is known: %q", p.Instruction)
	}
	if !strings.Contains(p.Instruction, "f ") {
		t.Fatalf("must answer with the short asl form: %q", p.Instruction)
	}
}

func TestHotTierSharesEarlier(t *testing.T) {
	s := testSession(t)
	for i := 0; i < 4; i++ {
		s.Director.Observe("ur so hot, send pics")
	}
	if s.Director.Tier != tierHot {
		t.Fatalf("tier = %s, want HOT (score %d)", s.Director.Tier, s.Director.Score)
	}
	if got := s.Director.ShareTurn(4, 6, 8); got != 4 {
		t.Fatalf("hot share turn = %d, want 4", got)
	}
}

func TestShareHappensOnceAndEndsAfterTwoTurns(t *testing.T) {
	s := testSession(t)
	d := &s.Director
	d.Tier = tierHot
	p := d.Plan(4, "u got snap?", 4, 6, 8)
	if !p.ShareNow {
		t.Fatalf("plan should share now: %+v", p)
	}
	if !d.Shared || d.SharedTurn != 4 {
		t.Fatalf("director did not record the share: %+v", d)
	}
	d.Observe("added u")
	p = d.Plan(5, "added u", 4, 6, 8)
	if !p.EndAfter {
		t.Fatalf("adding the snap should close the chat: %+v", p)
	}
}

func TestRepeatMessageIsTeasedNotAnswered(t *testing.T) {
	s := testSession(t)
	s.Director.Observe("hey wyd")
	p := s.Director.Plan(2, "hey wyd", 4, 6, 8)
	if p.Why == "repeat" {
		t.Fatalf("first turn should not be a repeat: %+v", p)
	}
	s.Director.Observe("hey wyd")
	p = s.Director.Plan(3, "hey wyd", 4, 6, 8)
	if p.Why != "repeat" {
		t.Fatalf("identical message should be treated as repeat, got %q", p.Why)
	}
}

func TestNonEnglishEndsAfterTwoHits(t *testing.T) {
	s := testSession(t)
	s.Director.Observe("kemon acho")
	p := s.Director.Plan(2, "kemon acho", 4, 6, 8)
	if p.EndAfter || p.Fixed == "" {
		t.Fatalf("first non-english hit should warn and keep going: %+v", p)
	}
	s.Director.Observe("kemon acho bhai")
	p = s.Director.Plan(3, "kemon acho bhai", 4, 6, 8)
	if !p.EndAfter || !strings.Contains(p.Fixed, "english only") {
		t.Fatalf("second hit should end in english only, got %+v", p)
	}
}

func TestStatusIsHumanReadable(t *testing.T) {
	s := testSession(t)
	status := s.Director.Status(4, 6, 8)
	if !strings.Contains(status, "tier=COLD") || !strings.Contains(status, "snap at reply 8") {
		t.Fatalf("unexpected status: %q", status)
	}
}

func TestGreetingOnTurnTwoRepliesWithGreetingNotASL(t *testing.T) {
	s := testSession(t)
	s.Director.Observe("hi")
	if s.Director.ColdStreak != 0 {
		t.Fatalf("greeting must not count as cold streak, got %d", s.Director.ColdStreak)
	}
	p := s.Director.Plan(2, "hi", 4, 6, 8)
	if p.Why != "greeting" {
		t.Fatalf("expected Why=greeting on 'hi', got %q (%q)", p.Why, p.Instruction)
	}
	if strings.Contains(strings.ToLower(p.Instruction), "give yours the short way") {
		t.Fatalf("greeting on turn 2 must not force unsolicited ASL dump: %q", p.Instruction)
	}
	if got := trimShortGreeting("heyy how's ur night going"); got != "heyy" {
		t.Fatalf("trimShortGreeting should strip extra greeting clause, got %q", got)
	}
	if got := trimShortGreeting("hey! just chilling here, you?"); got != "hey" {
		t.Fatalf("trimShortGreeting should strip extra chilling clause, got %q", got)
	}
}

func TestFlowProgressionNeverMentionsSnapBeforeDirectShare(t *testing.T) {
	s := testSession(t)
	d := &s.Director

	// Turn 2: Greeting
	d.Observe("hi")
	p2 := d.Plan(2, "hi", 4, 6, 8)
	if p2.Why != "greeting" {
		t.Fatalf("turn 2 Why = %q, want greeting", p2.Why)
	}

	// Turn 3: ASL
	d.Observe("m 22")
	p3 := d.Plan(3, "m 22", 4, 6, 8)
	if p3.Why != "asl back (no u?)" {
		t.Fatalf("turn 3 Why = %q, want asl back (no u?)", p3.Why)
	}

	// Turn 4: Country (since ASL is known and country is still empty)
	d.Observe("chilling")
	p4 := d.Plan(4, "chilling", 4, 6, 8)
	if p4.Why != "country" {
		t.Fatalf("turn 4 Why = %q, want country", p4.Why)
	}

	// Turn 5: Single horny question gets a flirty tease, NOT an early snap share
	d.Observe("horny ?")
	p5 := d.Plan(5, "horny ?", 4, 6, 8)
	if p5.ShareNow || p5.Why != "horny" {
		t.Fatalf("single horny question should tease (Why=horny, ShareNow=false), got Why=%q ShareNow=%v", p5.Why, p5.ShareNow)
	}
	if !strings.Contains(p5.Instruction, "do NOT mention snap") {
		t.Fatalf("horny instruction must forbid mentioning snap: %q", p5.Instruction)
	}
}
