package main

import (
	"testing"
	"time"
)

func TestAppStartSitesStopSitesGetSiteStatusesAndBackwardCompat(t *testing.T) {
	dataDir := t.TempDir()
	app, err := NewApp(dataDir)
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	defer app.shutdown(nil)

	selected := []string{"omezy", "chatib", "y99"}
	if err := app.StartSites(selected); err != nil {
		t.Fatalf("StartSites: %v", err)
	}

	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		statuses := app.GetSiteStatuses()
		allReady := true
		for _, id := range selected {
			st, ok := statuses[id]
			if !ok || !st.Running || st.Stats.CyclesCompleted < 1 {
				allReady = false
				break
			}
		}
		if allReady {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}

	statuses := app.GetSiteStatuses()
	for _, id := range selected {
		st := statuses[id]
		if !st.Running || st.Stats.CyclesCompleted < 1 {
			t.Fatalf("expected %s running with >= 1 cycle, got %+v", id, st)
		}
		hist, err := app.GetSiteChatHistory(id)
		if err != nil || len(hist) == 0 {
			t.Fatalf("expected isolated chat history for %s, got len=%d err=%v", id, len(hist), err)
		}
	}

	// StopSites stops only omezy and chatib while y99 stays running.
	if err := app.StopSites([]string{"omezy", "chatib"}); err != nil {
		t.Fatalf("StopSites: %v", err)
	}
	afterStop := app.GetSiteStatuses()
	if afterStop["omezy"].Running || afterStop["chatib"].Running {
		t.Fatalf("expected omezy and chatib stopped after StopSites")
	}
	if !afterStop["y99"].Running {
		t.Fatalf("expected y99 still running after StopSites([omezy, chatib])")
	}

	// StopBot (legacy chitchat.gg worker control) must remain callable and must not
	// break multi-site Supervisor runners.
	if err := app.StopBot(); err != nil {
		t.Fatalf("StopBot backward compatibility: %v", err)
	}
	if !app.GetSiteStatuses()["y99"].Running {
		t.Fatalf("StopBot must not disturb independent Supervisor runner y99")
	}

	// StopAll stops all remaining Supervisor runners.
	if err := app.StopAll(); err != nil {
		t.Fatalf("StopAll: %v", err)
	}
	for id, st := range app.GetSiteStatuses() {
		if st.Running {
			t.Fatalf("expected %s stopped after StopAll, got %+v", id, st)
		}
	}
}
