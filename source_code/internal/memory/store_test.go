package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMemoryStorePersistsChatHistoryAndOperationState(t *testing.T) {
	tmp := t.TempDir()
	s := NewStore(tmp)

	s.SetActiveSite("omezy")
	s.RecordEvent("show_mode", "", "in", "hey where are you from?", "", "partner · pending=1", false)
	s.RecordEvent("show_mode", "omezy", "out", "i am from usa wbu?", "API · reply", "delay 2.1s ✓", true)
	s.RecordEvent("show_mode", "omezy", "skip", "skipped conversation", "", "engine ended", false)

	snap := s.Snapshot()
	if snap.ActiveSiteID != "omezy" {
		t.Fatalf("expected active site omezy, got %q", snap.ActiveSiteID)
	}
	if snap.TotalIncoming != 1 || snap.TotalOutgoing != 1 || snap.TotalSkips != 1 {
		t.Fatalf("unexpected totals: %+v", snap)
	}
	st, ok := snap.SiteStats["omezy"]
	if !ok || st.IncomingCount != 1 || st.OutgoingCount != 1 || st.SkipCount != 1 {
		t.Fatalf("unexpected site stats for omezy: %+v", st)
	}

	jsonlPath := filepath.Join(tmp, "sites", "_global", "memory", "chat_history.jsonl")
	raw, err := os.ReadFile(jsonlPath)
	if err != nil {
		t.Fatalf("failed reading chat_history.jsonl: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 jsonl lines, got %d", len(lines))
	}
	if !strings.Contains(lines[0], "hey where are you from?") || !strings.Contains(lines[1], "i am from usa wbu?") {
		t.Fatalf("unexpected jsonl content: %s", string(raw))
	}

	// Re-open store from disk to simulate recovery after power loss.
	recovered := NewStore(tmp)
	recSnap := recovered.Snapshot()
	if recSnap.ActiveSiteID != "omezy" || recSnap.TotalIncoming != 1 || recSnap.TotalOutgoing != 1 {
		t.Fatalf("power-loss recovery failed: %+v", recSnap)
	}
}
