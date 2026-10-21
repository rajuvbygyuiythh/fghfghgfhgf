package runner

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"chitchatbot/internal/browser"
	"chitchatbot/internal/model"
	"chitchatbot/internal/settings"
	"chitchatbot/internal/sites"
)

func waitUntil(t *testing.T, timeout time.Duration, cond func() bool, desc string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", desc)
}

func TestSupervisorStartStopSelectedStopAllRestartAndStatus(t *testing.T) {
	dataDir := t.TempDir()
	if _, err := sites.EnsureSites(dataDir); err != nil {
		t.Fatalf("EnsureSites: %v", err)
	}

	sup := NewSupervisor(Options{
		DataDir:       dataDir,
		CycleInterval: 15 * time.Millisecond,
	})
	defer func() { _ = sup.StopAll() }()

	// 1. StartSite
	if err := sup.StartSite("omezy"); err != nil {
		t.Fatalf("StartSite(omezy): %v", err)
	}
	waitUntil(t, 2*time.Second, func() bool {
		st, err := sup.GetStatus("omezy")
		return err == nil && st.Running && st.Stats.CyclesCompleted >= 2
	}, "omezy to complete >= 2 cycles")

	st, err := sup.GetStatus("omezy")
	if err != nil {
		t.Fatal(err)
	}
	if st.Status != StatusRunning || st.Stats.MessagesSent == 0 {
		t.Fatalf("unexpected omezy status: %+v", st)
	}

	// 2. PauseSite
	if err := sup.PauseSite("omezy", "manual maintenance pause"); err != nil {
		t.Fatalf("PauseSite: %v", err)
	}
	st, _ = sup.GetStatus("omezy")
	if st.Status != StatusPaused || !st.Paused || st.Running || st.PauseReason != "manual maintenance pause" {
		t.Fatalf("expected paused status, got %+v", st)
	}

	// 3. RestartSite
	if err := sup.RestartSite("omezy"); err != nil {
		t.Fatalf("RestartSite: %v", err)
	}
	waitUntil(t, 2*time.Second, func() bool {
		s, _ := sup.GetStatus("omezy")
		return s.Running && s.RestartCount == 1 && s.Stats.CyclesCompleted >= 3
	}, "omezy to restart and run more cycles")

	// 4. StartSelected
	selected := []string{"chatib", "y99", "chatblink"}
	if err := sup.StartSelected(selected); err != nil {
		t.Fatalf("StartSelected: %v", err)
	}
	waitUntil(t, 2*time.Second, func() bool {
		all := sup.GetAllStatuses()
		for _, id := range selected {
			if !all[id].Running || all[id].Stats.CyclesCompleted < 1 {
				return false
			}
		}
		return true
	}, "all selected sites to run")

	// 5. StopSite on single site
	if err := sup.StopSite("chatib"); err != nil {
		t.Fatalf("StopSite(chatib): %v", err)
	}
	stChatib, _ := sup.GetStatus("chatib")
	if stChatib.Running || stChatib.Status != StatusStopped {
		t.Fatalf("expected chatib stopped, got %+v", stChatib)
	}
	// Other sites must still be running.
	stY99, _ := sup.GetStatus("y99")
	if !stY99.Running {
		t.Fatalf("expected y99 still running after stopping chatib")
	}

	// 6. StopAll
	if err := sup.StopAll(); err != nil {
		t.Fatalf("StopAll: %v", err)
	}
	for id, item := range sup.GetAllStatuses() {
		if item.Running {
			t.Fatalf("site %s still running after StopAll: %+v", id, item)
		}
	}
}

func TestOneSitePanicMarksOnlyThatSiteFailedWhileOtherSitesKeepRunning(t *testing.T) {
	dataDir := t.TempDir()
	if _, err := sites.EnsureSites(dataDir); err != nil {
		t.Fatalf("EnsureSites: %v", err)
	}

	var notifMu sync.Mutex
	var notifications []Notification

	sup := NewSupervisor(Options{
		DataDir:       dataDir,
		CycleInterval: 15 * time.Millisecond,
		NotifyFunc: func(n Notification) {
			notifMu.Lock()
			notifications = append(notifications, n)
			notifMu.Unlock()
		},
		CycleFunc: func(ctx context.Context, cc *CycleContext) (CycleResult, error) {
			if cc.SiteID == "chatib" && cc.CycleNumber >= 2 {
				panic("simulated catastrophic site failure in chatib")
			}
			turn := cc.ChatSession.Start(ctx)
			return CycleResult{
				MessagesSent:     len(turn.Lines),
				MessagesReceived: 1,
				Conversations:    1,
				LastReplyRule:    turn.Reason,
			}, nil
		},
	})
	defer func() { _ = sup.StopAll() }()

	if err := sup.StartSelected([]string{"omezy", "chatib", "chitchat", "y99"}); err != nil {
		t.Fatalf("StartSelected: %v", err)
	}

	// Wait until chatib crashes and the other 3 sites have completed >= 5 cycles.
	waitUntil(t, 3*time.Second, func() bool {
		c, _ := sup.GetStatus("chatib")
		o, _ := sup.GetStatus("omezy")
		cc, _ := sup.GetStatus("chitchat")
		y, _ := sup.GetStatus("y99")
		return c.Status == StatusFailed &&
			o.Running && o.Stats.CyclesCompleted >= 5 &&
			cc.Running && cc.Stats.CyclesCompleted >= 5 &&
			y.Running && y.Stats.CyclesCompleted >= 5
	}, "chatib to fail on panic while omezy, chitchat, and y99 keep running")

	cStatus, _ := sup.GetStatus("chatib")
	if !cStatus.Failed || cStatus.Running || cStatus.LastErrorType != ErrorTypeCrash {
		t.Fatalf("expected chatib failed with crash error type, got %+v", cStatus)
	}
	if !strings.Contains(cStatus.LastError, "simulated catastrophic site failure") {
		t.Fatalf("unexpected LastError: %q", cStatus.LastError)
	}

	// Verify crash log was written strictly to sites/chatib/logs/errors.jsonl.
	errLogs, err := ReadSiteErrorLogs(dataDir, "chatib")
	if err != nil {
		t.Fatal(err)
	}
	if len(errLogs) != 1 || errLogs[0]["error_type"] != ErrorTypeCrash {
		t.Fatalf("expected 1 crash log in sites/chatib/logs/errors.jsonl, got %+v", errLogs)
	}
	for _, healthy := range []string{"omezy", "chitchat", "y99"} {
		otherErrs, _ := ReadSiteErrorLogs(dataDir, healthy)
		if len(otherErrs) != 0 {
			t.Fatalf("healthy site %s should have 0 error logs, got %d", healthy, len(otherErrs))
		}
	}
}

func TestFiveConsecutiveErrorsTriggerAutoPauseAndUINotification(t *testing.T) {
	dataDir := t.TempDir()
	if _, err := sites.EnsureSites(dataDir); err != nil {
		t.Fatalf("EnsureSites: %v", err)
	}

	var notifMu sync.Mutex
	var autoPauseNotif *Notification

	scriptedErrors := []error{
		errors.New("websocket dial connection refused"),
		errors.New("HTTP 401 unauthorized: auth expired"),
		&browser.BannedError{Report: browser.BanReport{Banned: true, Reason: "ip blocked"}},
		errors.New("cloudflare turnstile captcha challenge detected"),
		context.DeadlineExceeded,
	}

	sup := NewSupervisor(Options{
		DataDir:              dataDir,
		MaxConsecutiveErrors: 5,
		CycleInterval:        10 * time.Millisecond,
		NotifyFunc: func(n Notification) {
			notifMu.Lock()
			if n.Event == "auto_paused" {
				cp := n
				autoPauseNotif = &cp
			}
			notifMu.Unlock()
		},
		CycleFunc: func(ctx context.Context, cc *CycleContext) (CycleResult, error) {
			idx := cc.CycleNumber - 1
			if idx < len(scriptedErrors) {
				return CycleResult{}, scriptedErrors[idx]
			}
			return CycleResult{MessagesSent: 1}, nil
		},
	})
	defer func() { _ = sup.StopAll() }()

	if err := sup.StartSite("omezy"); err != nil {
		t.Fatalf("StartSite(omezy): %v", err)
	}

	waitUntil(t, 3*time.Second, func() bool {
		st, _ := sup.GetStatus("omezy")
		return st.Status == StatusPaused && st.AutoPaused
	}, "omezy to auto-pause after 5 consecutive errors")

	st, err := sup.GetStatus("omezy")
	if err != nil {
		t.Fatal(err)
	}
	if st.Running || !st.Paused || !st.AutoPaused || st.ConsecutiveErrors != 5 || st.TotalErrors != 5 {
		t.Fatalf("unexpected auto-paused status: %+v", st)
	}
	for _, wantType := range []string{
		ErrorTypeConnection,
		ErrorTypeAuthExpired,
		ErrorTypeBanned,
		ErrorTypeCaptcha,
		ErrorTypeTimeout,
	} {
		if st.ErrorCounts[wantType] != 1 {
			t.Fatalf("expected ErrorCounts[%q] == 1, got %d (%+v)", wantType, st.ErrorCounts[wantType], st.ErrorCounts)
		}
	}

	notifMu.Lock()
	gotNotif := autoPauseNotif
	notifMu.Unlock()
	if gotNotif == nil || gotNotif.SiteID != "omezy" || gotNotif.ConsecutiveErrors != 5 {
		t.Fatalf("expected UI auto_paused notification for omezy with 5 consecutive errors, got %+v", gotNotif)
	}

	// Verify all 5 errors are recorded in sites/omezy/logs/errors.jsonl.
	errLogs, err := ReadSiteErrorLogs(dataDir, "omezy")
	if err != nil {
		t.Fatal(err)
	}
	if len(errLogs) != 5 {
		t.Fatalf("expected 5 entries in sites/omezy/logs/errors.jsonl, got %d", len(errLogs))
	}
}

type fakeDOMBackend struct {
	mu               sync.Mutex
	startedOpts      browser.Options
	seededLogin      model.LoginInfo
	customJS         string
	composerSelector string
	sentMessages     []string
	closed           bool
}

func (f *fakeDOMBackend) Name() string { return "rod" }
func (f *fakeDOMBackend) Start(_ context.Context, opts browser.Options) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.startedOpts = opts
	return nil
}
func (f *fakeDOMBackend) CaptureSession(_ context.Context) (model.LoginInfo, error) {
	return model.LoginInfo{UserID: "u1"}, nil
}
func (f *fakeDOMBackend) Fetch(_ context.Context, _ browser.FetchRequest) (browser.FetchResponse, error) {
	return browser.FetchResponse{OK: true, Status: 200}, nil
}
func (f *fakeDOMBackend) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	return nil
}
func (f *fakeDOMBackend) SeedSession(_ context.Context, login model.LoginInfo) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.seededLogin = login
	return nil
}
func (f *fakeDOMBackend) ConfigureSiteDOM(customJS, composerSelector string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.customJS = customJS
	f.composerSelector = composerSelector
}
func (f *fakeDOMBackend) ReadDOMSnapshot(_ context.Context) (browser.DOMSnapshot, error) {
	return browser.DOMSnapshot{
		Controls: browser.DOMControls{
			HasInput:  true,
			CanSend:   true,
			ChatEnded: false,
			Phase:     "chatting",
			Partner:   "stranger_42",
		},
		Messages: []browser.DOMMessage{
			{ID: "m1", Order: 1, Author: "stranger", Text: "asl? where are you from?"},
		},
		Revision: 1,
	}, nil
}
func (f *fakeDOMBackend) SendDOMMessage(_ context.Context, text string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sentMessages = append(f.sentMessages, text)
	return true, nil
}
func (f *fakeDOMBackend) ClickDOMButton(_ context.Context, _ []string) (string, error) {
	return "START", nil
}
func (f *fakeDOMBackend) DOMProbe(_ context.Context) (string, error) {
	return "phase=chatting", nil
}

func TestRunnerExecutesBotChatModesAndBrowserConnectionInIsolatedSiteFolder(t *testing.T) {
	dataDir := t.TempDir()
	if _, err := sites.EnsureSites(dataDir); err != nil {
		t.Fatalf("EnsureSites: %v", err)
	}
	st, err := settings.Open(dataDir)
	if err != nil {
		t.Fatalf("settings.Open: %v", err)
	}

	// Configure chatblink to use EVAChat mode via per-site override.
	evaMode := model.ChatModeEVAChat
	if _, err := st.SaveOverride("chatblink", settings.Override{MessageMode: &evaMode}); err != nil {
		t.Fatalf("SaveOverride: %v", err)
	}

	fakeBrowser := &fakeDOMBackend{}
	sup := NewSupervisor(Options{
		DataDir:        dataDir,
		SettingsStore:  st,
		CycleInterval:  15 * time.Millisecond,
		UseLiveBrowser: true,
		BrowserFactory: func(backendName string) (browser.Backend, error) {
			return fakeBrowser, nil
		},
	})

	if err := sup.StartSite("chatblink"); err != nil {
		t.Fatalf("StartSite(chatblink): %v", err)
	}

	waitUntil(t, 3*time.Second, func() bool {
		status, _ := sup.GetStatus("chatblink")
		return status.Stats.MessagesSent >= 1
	}, "chatblink DOM runner to send at least 1 EVAChat reply")

	status, _ := sup.GetStatus("chatblink")
	if status.ChatMode != model.ChatModeEVAChat {
		t.Fatalf("chatblink ChatMode = %q, want %q", status.ChatMode, model.ChatModeEVAChat)
	}

	if err := sup.StopSite("chatblink"); err != nil {
		t.Fatalf("StopSite(chatblink): %v", err)
	}

	fakeBrowser.mu.Lock()
	userDataDir := fakeBrowser.startedOpts.UserDataDir
	sentCount := len(fakeBrowser.sentMessages)
	wasClosed := fakeBrowser.closed
	fakeBrowser.mu.Unlock()

	siteCtx, _ := sites.NewSiteContext(dataDir, "chatblink")
	if !sites.PathWithin(siteCtx.FolderPath, userDataDir) {
		t.Fatalf("browser UserDataDir %q is outside isolated site folder %q", userDataDir, siteCtx.FolderPath)
	}
	wantSuffix := filepath.Join("sites", "chatblink", "profiles", "default", "browser")
	if !strings.HasSuffix(userDataDir, wantSuffix) {
		t.Fatalf("browser UserDataDir = %q, want suffix %q", userDataDir, wantSuffix)
	}
	if sentCount == 0 {
		t.Fatal("expected fakeBrowser to have sent at least 1 message")
	}
	if !wasClosed {
		t.Fatal("expected fakeBrowser.Close() to be called when StopSite completed")
	}
}

func TestSupervisorWiresWebSocketSiteToRealtimeGoAndIsolatedFolder(t *testing.T) {
	dataDir := t.TempDir()
	if _, err := sites.EnsureSites(dataDir); err != nil {
		t.Fatalf("EnsureSites: %v", err)
	}
	st, err := settings.Open(dataDir)
	if err != nil {
		t.Fatalf("settings.Open: %v", err)
	}

	evaMode := model.ChatModeEVAChat
	if _, err := st.SaveOverride(sites.DefaultSiteID, settings.Override{MessageMode: &evaMode}); err != nil {
		t.Fatalf("SaveOverride: %v", err)
	}

	sup := NewSupervisor(Options{
		DataDir:       dataDir,
		SettingsStore: st,
		CycleInterval: 15 * time.Millisecond,
	})
	defer func() { _ = sup.StopAll() }()

	packet := `42["chatMessage",{"message":{"id":"ws-msg-1","conversationId":"ws-conv-99","author":{"id":"traffic-user-77","username":"WsStranger"},"content":"19 m usa"}}]`
	if err := sup.PushRealtimePacket(sites.DefaultSiteID, packet); err != nil {
		t.Fatalf("PushRealtimePacket: %v", err)
	}

	if err := sup.StartSite(sites.DefaultSiteID); err != nil {
		t.Fatalf("StartSite(chitchat): %v", err)
	}

	waitUntil(t, 3*time.Second, func() bool {
		status, _ := sup.GetStatus(sites.DefaultSiteID)
		return status.Stats.CyclesCompleted >= 2 && status.Stats.MessagesSent >= 2
	}, "chitchat WebSocket runner to process realtime.go packet and send EVA replies")

	status, err := sup.GetStatus(sites.DefaultSiteID)
	if err != nil {
		t.Fatal(err)
	}
	if status.Transport != TransportWebSocket {
		t.Fatalf("expected transport %q, got %q", TransportWebSocket, status.Transport)
	}
	if !strings.Contains(status.Stats.LastReplyRule, "EVA(websocket)") {
		t.Fatalf("expected EVA(websocket) reply rule, got %q", status.Stats.LastReplyRule)
	}

	history, err := ReadSiteChatHistory(dataDir, sites.DefaultSiteID)
	if err != nil {
		t.Fatalf("ReadSiteChatHistory: %v", err)
	}
	if len(history) < 2 {
		t.Fatalf("expected chat_history.jsonl in sites/chitchat/logs/ to contain >= 2 entries, got %d", len(history))
	}
	foundIncoming := false
	for _, entry := range history {
		if entry.Direction == "in" && entry.Message == "19 m usa" && entry.PartnerID == "traffic-user-77" {
			foundIncoming = true
		}
		if entry.SiteID != sites.DefaultSiteID {
			t.Fatalf("unexpected site_id %q in sites/chitchat/logs/chat_history.jsonl", entry.SiteID)
		}
	}
	if !foundIncoming {
		t.Fatalf("expected incoming realtime.go message in sites/chitchat/logs/chat_history.jsonl, got %+v", history)
	}
}

func TestSupervisorWiresAllEnginesAndStopSitesAndIsolatedFolders(t *testing.T) {
	dataDir := t.TempDir()
	if _, err := sites.EnsureSites(dataDir); err != nil {
		t.Fatalf("EnsureSites: %v", err)
	}
	st, err := settings.Open(dataDir)
	if err != nil {
		t.Fatalf("settings.Open: %v", err)
	}

	// Mock OpenAI-compatible HTTP server for API Chat mode (api_chat.go).
	aiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"choices":[{"message":{"content":"hey im 19 f usa hbu"}}],"usage":{"total_tokens":12}}`)
	}))
	defer aiServer.Close()

	// Configure:
	// - omezy: Single Sequence mode (message_sequence.go)
	// - chatib: EVAChat mode (eva_chat.go)
	// - y99: API Chat mode (api_chat.go)
	seqMode := model.ChatModeSingle
	if _, err := st.SaveOverride("omezy", settings.Override{MessageMode: &seqMode}); err != nil {
		t.Fatal(err)
	}
	evaMode := model.ChatModeEVAChat
	if _, err := st.SaveOverride("chatib", settings.Override{MessageMode: &evaMode}); err != nil {
		t.Fatal(err)
	}
	apiMode := model.ChatModeAPI
	if _, err := st.SaveOverride("y99", settings.Override{
		MessageMode: &apiMode,
		API: &settings.APISettings{
			Enabled:     true,
			BaseURL:     aiServer.URL + "/v1",
			APIKey:      "test-api-key",
			Model:       "test-model",
			Temperature: 0.7,
		},
	}); err != nil {
		t.Fatal(err)
	}

	sup := NewSupervisor(Options{
		DataDir:       dataDir,
		SettingsStore: st,
		CycleInterval: 15 * time.Millisecond,
	})
	defer func() { _ = sup.StopAll() }()

	targetSites := []string{"omezy", "chatib", "y99", "chatblink"}
	if err := sup.StartSites(targetSites); err != nil {
		t.Fatalf("StartSites: %v", err)
	}

	waitUntil(t, 4*time.Second, func() bool {
		statuses := sup.GetSiteStatuses()
		for _, id := range targetSites {
			s := statuses[id]
			if !s.Running || s.Stats.CyclesCompleted < 2 || s.Stats.MessagesSent < 1 {
				return false
			}
		}
		return true
	}, "all 4 sites (Sequence, EVA, API, DOM) to complete >= 2 cycles")

	statuses := sup.GetSiteStatuses()
	if !strings.Contains(statuses["omezy"].Stats.LastReplyRule, "SEQ(") {
		t.Fatalf("expected omezy to run message_sequence.go, got rule %q", statuses["omezy"].Stats.LastReplyRule)
	}
	if !strings.Contains(statuses["chatib"].Stats.LastReplyRule, "EVA(") {
		t.Fatalf("expected chatib to run eva_chat.go, got rule %q", statuses["chatib"].Stats.LastReplyRule)
	}
	if !strings.Contains(statuses["y99"].Stats.LastReplyRule, "API(") {
		t.Fatalf("expected y99 to run api_chat.go, got rule %q", statuses["y99"].Stats.LastReplyRule)
	}
	if statuses["chatblink"].Transport != TransportDOM {
		t.Fatalf("expected chatblink transport %q, got %q", TransportDOM, statuses["chatblink"].Transport)
	}

	// Test StopSites stops only omezy and chatib while y99 and chatblink keep running.
	if err := sup.StopSites([]string{"omezy", "chatib"}); err != nil {
		t.Fatalf("StopSites: %v", err)
	}
	afterStop := sup.GetSiteStatuses()
	if afterStop["omezy"].Running || afterStop["chatib"].Running {
		t.Fatalf("expected omezy and chatib stopped, got omezy=%v chatib=%v", afterStop["omezy"].Running, afterStop["chatib"].Running)
	}
	if !afterStop["y99"].Running || !afterStop["chatblink"].Running {
		t.Fatalf("expected y99 and chatblink still running, got y99=%v chatblink=%v", afterStop["y99"].Running, afterStop["chatblink"].Running)
	}

	// Verify session load, bot_state.json, runner.jsonl, and chat_history.jsonl live strictly inside each site's folder.
	for _, id := range targetSites {
		stItem := afterStop[id]
		siteCtx, err := sites.NewSiteContext(dataDir, id)
		if err != nil {
			t.Fatal(err)
		}
		for label, p := range map[string]string{
			"SessionPath":     stItem.SessionPath,
			"BotStatePath":    stItem.BotStatePath,
			"ChatHistoryPath": stItem.ChatHistoryPath,
			"LogPath":         stItem.LogPath,
		} {
			if !sites.PathWithin(siteCtx.FolderPath, p) {
				t.Fatalf("%s for %s (%q) is outside site folder %q", label, id, p, siteCtx.FolderPath)
			}
			if _, statErr := os.Stat(p); statErr != nil {
				t.Fatalf("expected %s file %q to exist for %s: %v", label, p, id, statErr)
			}
		}
		hist, err := ReadSiteChatHistory(dataDir, id)
		if err != nil || len(hist) == 0 {
			t.Fatalf("expected non-empty chat_history.jsonl for %s, got len=%d err=%v", id, len(hist), err)
		}
		for _, entry := range hist {
			if entry.SiteID != id {
				t.Fatalf("cross-site contamination in %s chat_history.jsonl: found site_id=%q", id, entry.SiteID)
			}
		}
	}
}
