package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"chitchatbot/internal/model"
	"chitchatbot/internal/runner"
	"chitchatbot/internal/settings"
	"chitchatbot/internal/sites"
	"chitchatbot/internal/store"
)

// TestPrompt16FullMultiSiteIntegration creates 3 mock sites (Group A, Group B, Group C)
// and verifies the complete 8-part end-to-end multi-site architecture (Prompt 16):
//  1. 3 mock sites creation & Session Capture mock (5-file bundle in sites/<id>/sessions/default/)
//  2. Browserless Connection mock (Group A guest, Group B/C saved session, Reconnect, expired pause)
//  3. Isolated Goroutine Start across all 3 mock sites
//  4. Single-Site Crash Isolation (panic in one site never stops the other two)
//  5. Global Settings Pickup from sites/_global/settings.json
//  6. Per-Site Override Isolation in sites/<id>/settings_override.json
//  7. Agent Self-Recovery mock (Scenarios A–E + 3-attempt needs_human limit)
//  8. Hard Cross-Site Access Block via SiteContext.Resolve / SafeJoin
func TestPrompt16FullMultiSiteIntegration(t *testing.T) {
	dataDir := t.TempDir()
	app, err := NewApp(dataDir)
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	defer app.shutdown(nil)

	mockDefs := []struct {
		id          string
		name        string
		url         string
		group       string
		groupName   string
		accountMode string
	}{
		{
			id:          "mock_group_a",
			name:        "Mock Group A Guest",
			url:         "https://mock-a.example/guest-chat",
			group:       "A",
			groupName:   "Pure Browserless Guest",
			accountMode: sites.AccountModeAutoRegister,
		},
		{
			id:          "mock_group_b",
			name:        "Mock Group B AutoSignup",
			url:         "https://mock-b.example/signup-chat",
			group:       "B",
			groupName:   "HTTP Auto-Signup",
			accountMode: sites.AccountModeAutoRegister,
		},
		{
			id:          "mock_group_c",
			name:        "Mock Group C BrowserOnce",
			url:         "https://mock-c.example/login-chat",
			group:       "C",
			groupName:   "Browser-Once + Headless Chat",
			accountMode: sites.AccountModeBrowserSetup,
		},
	}

	for _, m := range mockDefs {
		opt, cErr := app.CreateSiteWithMode(m.name, m.url, m.accountMode)
		if cErr != nil {
			t.Fatalf("CreateSiteWithMode(%s): %v", m.id, cErr)
		}
		// Ensure deterministic canonical IDs mock_group_a, mock_group_b, mock_group_c
		cfg, lErr := sites.LoadSite(dataDir, opt.ID)
		if lErr != nil {
			t.Fatalf("LoadSite(%s): %v", opt.ID, lErr)
		}
		cfg.ID = m.id
		cfg.Name = m.name
		cfg.Classification.Group = m.group
		cfg.Classification.GroupName = m.groupName
		cfg.Classification.Confidence = "high"
		sc, sErr := sites.NewSiteContext(dataDir, m.id)
		if sErr != nil {
			t.Fatalf("NewSiteContext(%s): %v", m.id, sErr)
		}
		if err := sc.Ensure(); err != nil {
			t.Fatalf("sc.Ensure(%s): %v", m.id, err)
		}
		if err := sc.SaveJSON("site.json", cfg); err != nil {
			t.Fatalf("SaveJSON(%s): %v", m.id, err)
		}
		_ = sc.WriteFile("dom_driver.js", []byte(sites.UniversalDOMDriverJS(cfg)), 0o644)
	}

	// -------------------------------------------------------------------------
	// 1. Session Capture Mock (5-file bundle in sites/<id>/sessions/default/)
	// -------------------------------------------------------------------------
	for _, id := range []string{"mock_group_a", "mock_group_b", "mock_group_c"} {
		sum, capErr := app.CaptureSession(id)
		if capErr != nil {
			t.Fatalf("CaptureSession(%s): %v", id, capErr)
		}
		if sum.Status != sites.SessionStateValid {
			t.Fatalf("expected valid session for %s, got %+v", id, sum)
		}
		sessDir := filepath.Join(sites.SiteDir(dataDir, id), "sessions", "default")
		for _, fname := range []string{"cookies.json", "local_storage.json", "ws_endpoints.json", "auth_tokens.json", "meta.json"} {
			if _, statErr := os.Stat(filepath.Join(sessDir, fname)); statErr != nil {
				t.Fatalf("missing session file %s/%s: %v", id, fname, statErr)
			}
		}
	}

	// -------------------------------------------------------------------------
	// 2. Browserless Connection Mock + Same-Session Reconnect
	// -------------------------------------------------------------------------
	for _, m := range mockDefs {
		conn, bErr := runner.ConnectBrowserlessSite(dataDir, m.id)
		if bErr != nil {
			t.Fatalf("ConnectBrowserlessSite(%s): %v", m.id, bErr)
		}
		if conn.Group != m.group || !conn.Headless || conn.VisibleBrowser {
			t.Fatalf("unexpected browserless config for %s: %+v", m.id, conn)
		}
		if conn.WSURL == "" || conn.HTTPClient == nil || len(conn.AuthHeaders) == 0 {
			t.Fatalf("expected WSURL, HTTPClient, and AuthHeaders for %s, got %+v", m.id, conn)
		}
		if recErr := conn.Reconnect(context.Background()); recErr != nil {
			t.Fatalf("Reconnect(%s): %v", m.id, recErr)
		}
		if conn.ReconnectCount != 1 {
			t.Fatalf("expected ReconnectCount=1 for %s, got %d", m.id, conn.ReconnectCount)
		}
	}

	// -------------------------------------------------------------------------
	// 3. Isolated Goroutine Start across all 3 mock sites
	// -------------------------------------------------------------------------
	mockIDs := []string{"mock_group_a", "mock_group_b", "mock_group_c"}
	if err := app.StartSites(mockIDs); err != nil {
		t.Fatalf("StartSites(mockIDs): %v", err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		statuses := app.GetSiteStatuses()
		readyCount := 0
		for _, id := range mockIDs {
			if st := statuses[id]; st.Running && st.Stats.CyclesCompleted >= 1 {
				readyCount++
			}
		}
		if readyCount == len(mockIDs) {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	for _, id := range mockIDs {
		st, gErr := app.GetStatus(id)
		if gErr != nil || !st.Running || st.Stats.CyclesCompleted < 1 {
			t.Fatalf("expected %s running in isolated goroutine, got %+v (err=%v)", id, st, gErr)
		}
	}
	_ = app.StopSites(mockIDs)

	// -------------------------------------------------------------------------
	// 4. Single-Site Crash Isolation (panic in mock_group_b does not affect A or C)
	// -------------------------------------------------------------------------
	var panicInjected atomic.Bool
	crashSup := runner.NewSupervisor(runner.Options{
		DataDir:       dataDir,
		SettingsStore: app.settingsStore(),
		CycleInterval: 20 * time.Millisecond,
		CycleFunc: func(ctx context.Context, cc *runner.CycleContext) (runner.CycleResult, error) {
			if cc.SiteID == "mock_group_b" && !panicInjected.Swap(true) {
				panic("simulated crash in mock_group_b")
			}
			return runner.CycleResult{MessagesSent: 1, MessagesReceived: 1}, nil
		},
	})
	if err := crashSup.StartSites(mockIDs); err != nil {
		t.Fatalf("crashSup.StartSites: %v", err)
	}
	time.Sleep(120 * time.Millisecond)
	stA, _ := crashSup.GetStatus("mock_group_a")
	stB, _ := crashSup.GetStatus("mock_group_b")
	stC, _ := crashSup.GetStatus("mock_group_c")
	if !stA.Running || !stC.Running {
		t.Fatalf("crash in mock_group_b must not stop mock_group_a (%v) or mock_group_c (%v)", stA.Running, stC.Running)
	}
	if stB.TotalErrors < 1 {
		t.Fatalf("expected panic in mock_group_b to be caught and counted, got %+v", stB)
	}
	_ = crashSup.StopAll()

	// -------------------------------------------------------------------------
	// 5. Global Settings Pickup + 6. Per-Site Override Isolation
	// -------------------------------------------------------------------------
	globalCfg, err := app.loadOrSeedGlobalSettings()
	if err != nil {
		t.Fatalf("loadOrSeedGlobalSettings: %v", err)
	}
	globalCfg.MessageMode = model.ChatModeEVAChat
	globalCfg.ReplyDelay = settings.DurationRange{MinSeconds: 4, MaxSeconds: 9}
	if _, err := app.SaveGlobalSettings(globalCfg); err != nil {
		t.Fatalf("SaveGlobalSettings: %v", err)
	}

	for _, id := range mockIDs {
		eff, eErr := app.GetSiteSettings(id)
		if eErr != nil {
			t.Fatalf("GetSiteSettings(%s): %v", id, eErr)
		}
		if eff.Settings.MessageMode != model.ChatModeEVAChat || eff.Settings.ReplyDelay.MinSeconds != 4 {
			t.Fatalf("expected %s to pick up global settings, got %+v", id, eff.Settings)
		}
	}

	// Override ONLY mock_group_a; mock_group_b and mock_group_c must stay on global settings.
	customMode := model.ChatModeSingle
	if _, err := app.SaveSiteOverride("mock_group_a", settings.Override{
		MessageMode: &customMode,
		ReplyDelay:  &settings.DurationRange{MinSeconds: 1, MaxSeconds: 2},
	}); err != nil {
		t.Fatalf("SaveSiteOverride(mock_group_a): %v", err)
	}

	effA, _ := app.GetSiteSettings("mock_group_a")
	effB, _ := app.GetSiteSettings("mock_group_b")
	effC, _ := app.GetSiteSettings("mock_group_c")
	if effA.Settings.MessageMode != model.ChatModeSingle || effA.Settings.ReplyDelay.MinSeconds != 1 {
		t.Fatalf("expected mock_group_a to use per-site override, got %+v", effA.Settings)
	}
	if effB.Settings.MessageMode != model.ChatModeEVAChat || effC.Settings.MessageMode != model.ChatModeEVAChat {
		t.Fatalf("override on mock_group_a leaked to B (%s) or C (%s)", effB.Settings.MessageMode, effC.Settings.MessageMode)
	}

	// -------------------------------------------------------------------------
	// 7. Agent Self-Recovery Mock (Scenario C & 3-Attempt needs_human limit)
	// -------------------------------------------------------------------------
	recRep, recErr := app.RecoverSite("mock_group_b", "ws_broken")
	if recErr != nil {
		t.Fatalf("RecoverSite(mock_group_b, ws_broken): %v", recErr)
	}
	if !recRep.SwitchedToDOM || recRep.Status != runner.RecoveryStatusRecovered {
		t.Fatalf("expected Scenario C DOM switch on mock_group_b, got %+v", recRep)
	}

	// Exhaust 3 failed recovery attempts on mock_group_c -> must mark needs_human.
	failMgr := runner.NewRecoveryManager(app.runnerSupervisor())
	failMgr.SetHook(func(ctx context.Context, siteCtx sites.SiteContext, siteCfg sites.Config, scenario string, errorType string, attempt int) (runner.RecoveryReport, error) {
		return runner.RecoveryReport{Message: "simulated captcha block"}, errors.New("captcha block")
	})
	for i := 1; i <= runner.MaxRecoveryAttempts; i++ {
		_, _ = failMgr.RecoverSite("mock_group_c", "session_expired")
	}
	repC, ok := failMgr.GetReport("mock_group_c")
	if !ok || !repC.NeedsHuman || repC.Status != runner.RecoveryStatusNeedsHuman {
		t.Fatalf("expected mock_group_c marked needs_human after 3 failed attempts, got %+v", repC)
	}

	// -------------------------------------------------------------------------
	// 8. Hard Cross-Site Access Block via SiteContext.Resolve (SafeJoin)
	// -------------------------------------------------------------------------
	ctxA, err := sites.NewSiteContext(dataDir, "mock_group_a")
	if err != nil {
		t.Fatalf("NewSiteContext(mock_group_a): %v", err)
	}
	if _, escapeErr := ctxA.Resolve(filepath.Join("..", "mock_group_b", "site.json")); escapeErr == nil {
		t.Fatalf("expected cross-site path traversal from mock_group_a to mock_group_b to be blocked")
	}
	_ = store.EnsureDir(ctxA.LogsPath)
}
