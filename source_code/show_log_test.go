package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"chitchatbot/internal/browser"
	"chitchatbot/internal/model"
)

func TestShowLogFeedIsBoundedAndIncremental(t *testing.T) {
	a := browserTestApp(t)
	for i := 0; i < showLogCapacity+7; i++ {
		a.addShowLog(ShowLogEntry{Kind: "system", Text: "row", Detail: "seq"})
	}
	snapshot := a.GetShowModeLog(0)
	if len(snapshot.Entries) != showLogCapacity {
		t.Fatalf("kept %d entries, want %d", len(snapshot.Entries), showLogCapacity)
	}
	if snapshot.Dropped != 7 {
		t.Fatalf("dropped=%d, want 7", snapshot.Dropped)
	}
	last := snapshot.Version
	partial := a.GetShowModeLog(last - 3)
	if len(partial.Entries) != 3 {
		t.Fatalf("incremental entries=%d, want 3", len(partial.Entries))
	}

	if err := a.ClearShowModeLog(); err != nil {
		t.Fatal(err)
	}
	after := a.GetShowModeLog(0)
	if len(after.Entries) != 0 {
		t.Fatalf("clear kept %d entries", len(after.Entries))
	}
}

func TestShowLogRecordsRuleDelayAndSkipReason(t *testing.T) {
	a := browserTestApp(t)
	if err := a.SaveMessageSettings(model.NewMessageSettings("hello first\nreply line", "")); err != nil {
		t.Fatal(err)
	}
	fake := &fakeDOMBackend{controls: browser.DOMControls{HasInput: true, Buttons: []string{"SKIP"}}}
	r := testLiveRunner(a, fake)
	r.replyDelayMin = 0
	r.replyDelayMax = 0
	var events []ShowLogEntry
	r.eventf = func(kind, text, rule, detail string, settingOK bool) {
		events = append(events, ShowLogEntry{Kind: kind, Text: text, Rule: rule, Detail: detail, SettingOK: settingOK})
	}

	ctx := context.Background()
	r.step(ctx) // opener
	fake.addRow("partner", "hi")
	r.step(ctx) // reply

	var sawIn, sawOpener, sawReply bool
	for _, e := range events {
		if e.Kind == "in" && e.Text == "hi" {
			sawIn = true
		}
		if e.Kind == "out" && e.Text == "hello first" {
			if !strings.Contains(e.Rule, "sequence 1/1 · line 1/2") || !strings.Contains(e.Detail, "opener") {
				t.Fatalf("opener metadata: %+v", e)
			}
			sawOpener = true
		}
		if e.Kind == "out" && e.Text == "reply line" {
			if !strings.Contains(e.Rule, "line 2/2") || !strings.Contains(e.Detail, "delay") || !e.SettingOK {
				t.Fatalf("reply metadata: %+v", e)
			}
			sawReply = true
		}
	}
	if !sawIn || !sawOpener || !sawReply {
		t.Fatalf("missing events in=%t opener=%t reply=%t: %+v", sawIn, sawOpener, sawReply, events)
	}
}

func TestShowLogSkipEventRecordsTimeout(t *testing.T) {
	a := browserTestApp(t)
	if err := a.SaveMessageSettings(model.NewMessageSettings("hello", "")); err != nil {
		t.Fatal(err)
	}
	fake := &fakeDOMBackend{controls: browser.DOMControls{HasInput: true, Buttons: []string{"SKIP"}}}
	r := testLiveRunner(a, fake)
	var skip *ShowLogEntry
	r.eventf = func(kind, text, rule, detail string, ok bool) {
		if kind == "skip" {
			skip = &ShowLogEntry{Kind: kind, Text: text, Rule: rule, Detail: detail, SettingOK: ok}
		}
	}
	r.autoSkip = true
	r.noReplyMin, r.noReplyMax = time.Second, time.Second
	r.nextChatMin, r.nextChatMax = 5*time.Second, 5*time.Second

	ctx := context.Background()
	r.step(ctx) // sets the conversation and sends the opener
	r.lastNoReplyWait = 12 * time.Second
	r.rotating = true
	r.rotateReason = "no-reply auto-skip"
	r.lastRotateAt = time.Time{}
	r.rotate(ctx)
	if skip == nil || !strings.Contains(skip.Detail, "no-reply auto-skip") || !strings.Contains(skip.Detail, "12.0s") || !strings.Contains(skip.Detail, "next chat") {
		t.Fatalf("skip event missing settings detail: %+v", skip)
	}
}

func TestShowModeDurationAndFirstLabels(t *testing.T) {
	if got := showModeDurationRange(3*time.Second, 6*time.Second); got != "3.0s–6.0s" {
		t.Fatalf("range=%q", got)
	}
	if got := showModeDurationRange(0, 0); got != "0.0s" {
		t.Fatalf("range=%q", got)
	}
	cases := map[string]string{
		"":       "bot",
		"bot":    "bot",
		"user":   "user",
		"random": "random 70%",
	}
	for mode, want := range cases {
		if got := showModeFirstLabel(model.FirstMessageSettings{Mode: mode, BotFirstPercent: 70}); got != want {
			t.Fatalf("mode %q label=%q, want %q", mode, got, want)
		}
	}
}

func TestSequenceAndEVARespondersExposeRules(t *testing.T) {
	a := browserTestApp(t)
	if err := a.SaveMessageSettings(model.NewMessageSettings("hello\nreply", "")); err != nil {
		t.Fatal(err)
	}
	responder, _, err := a.newLiveResponder()
	if err != nil {
		t.Fatal(err)
	}
	opener := responder.Start()
	if len(opener.Lines) != 1 || !strings.Contains(opener.Rule, "sequence 1/1") {
		t.Fatalf("sequence opener=%+v", opener)
	}
	reply := responder.Reply("hi")
	if len(reply.Lines) != 1 || !strings.Contains(reply.Rule, "line 2/2") {
		t.Fatalf("sequence reply=%+v", reply)
	}

	settings := model.NewMessageSettings("", "")
	settings.ChatMode = model.ChatModeEVAChat
	if err := a.SaveMessageSettings(settings); err != nil {
		t.Fatal(err)
	}
	eva, _, err := a.newLiveResponder()
	if err != nil {
		t.Fatal(err)
	}
	start := eva.Start()
	if len(start.Lines) == 0 || !strings.Contains(start.Rule, "EVA ·") || !strings.Contains(start.Detail, "stage=") {
		t.Fatalf("EVA start=%+v", start)
	}
}
