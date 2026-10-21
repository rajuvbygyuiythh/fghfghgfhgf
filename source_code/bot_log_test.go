package main

import (
	"strings"
	"testing"
)

func TestBotLogFeedIsBoundedAndIncremental(t *testing.T) {
	a := browserTestApp(t)
	for i := 0; i < botLogCapacity+5; i++ {
		a.addBotLog(BotLogEntry{Kind: "system", Text: "row"})
	}
	snapshot := a.GetBotLiveLog(0)
	if len(snapshot.Entries) != botLogCapacity {
		t.Fatalf("kept %d entries, want %d", len(snapshot.Entries), botLogCapacity)
	}
	if snapshot.Dropped != 5 {
		t.Fatalf("dropped=%d, want 5", snapshot.Dropped)
	}
	last := snapshot.Version
	partial := a.GetBotLiveLog(last - 4)
	if len(partial.Entries) != 4 {
		t.Fatalf("incremental entries=%d, want 4", len(partial.Entries))
	}

	if err := a.ClearBotLiveLog(); err != nil {
		t.Fatal(err)
	}
	if after := a.GetBotLiveLog(0); len(after.Entries) != 0 {
		t.Fatalf("clear kept %d entries", len(after.Entries))
	}
}

func TestBotLogParsesWorkerActivity(t *testing.T) {
	a := browserTestApp(t)
	a.botWorkerActivity("default", "SEND | bot default (1) -> traffic bob (2) | message: hello there")
	a.botWorkerActivity("default", "RECEIVE | traffic bob (2) -> bot default (1) | message: hi")

	snapshot := a.GetBotLiveLog(0)
	if len(snapshot.Entries) != 2 {
		t.Fatalf("entries=%d, want 2", len(snapshot.Entries))
	}
	out := snapshot.Entries[0]
	if out.Kind != "out" || out.Text != "hello there" || out.Rule != "default" {
		t.Fatalf("out entry=%+v", out)
	}
	if !strings.Contains(out.Detail, "traffic bob (2)") {
		t.Fatalf("out detail=%q", out.Detail)
	}
	in := snapshot.Entries[1]
	if in.Kind != "in" || in.Text != "hi" || in.Rule != "default" {
		t.Fatalf("in entry=%+v", in)
	}
	if !strings.Contains(in.Detail, "traffic bob (2)") {
		t.Fatalf("in detail=%q", in.Detail)
	}
}

func TestBotWorkerLogClassifiesErrorsAndSkips(t *testing.T) {
	a := browserTestApp(t)
	a.botWorkerLog("default", "GET dm conversations failed: 500 boom")
	a.botWorkerLog("default", "  -> Sequence line 2 rendered empty for bob. Skipped reply to 9.")
	a.botWorkerLog("default", "No DM conversations right now.")

	snapshot := a.GetBotLiveLog(0)
	want := []string{"error", "skip", "system"}
	if len(snapshot.Entries) != len(want) {
		t.Fatalf("entries=%d, want %d", len(snapshot.Entries), len(want))
	}
	for i, kind := range want {
		if snapshot.Entries[i].Kind != kind {
			t.Fatalf("entry %d kind=%q, want %q", i, snapshot.Entries[i].Kind, kind)
		}
		if snapshot.Entries[i].Rule != "default" {
			t.Fatalf("entry %d rule=%q", i, snapshot.Entries[i].Rule)
		}
	}
}
