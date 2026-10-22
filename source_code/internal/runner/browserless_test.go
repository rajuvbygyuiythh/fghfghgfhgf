package runner

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"chitchat-wails/internal/sites"
)

func TestPrompt11BrowserlessConnectionsGroupsAtoEAndSameSessionReconnect(t *testing.T) {
	dataDir := t.TempDir()
	if _, err := sites.EnsureSites(dataDir); err != nil {
		t.Fatalf("EnsureSites: %v", err)
	}

	// Group A (Guest Instant): browserless WebSocket / API (no visible browser)
	connA, err := ConnectBrowserlessSite(dataDir, "chatib")
	if err != nil {
		t.Fatalf("ConnectBrowserlessSite(chatib): %v", err)
	}
	if connA.Group != sites.GroupA || connA.VisibleBrowser || !connA.Headless {
		t.Fatalf("expected Group A browserless (Headless=true, VisibleBrowser=false), got %+v", connA)
	}
	if connA.WSURL == "" || len(connA.AuthHeaders) == 0 || connA.HTTPClient == nil {
		t.Fatalf("expected WSURL, AuthHeaders, and HTTPClient on Group A connection, got %+v", connA)
	}

	// Group B (Auto-Signup): browserless API / WebSocket
	connB, err := ConnectBrowserlessSite(dataDir, "omezy")
	if err != nil {
		t.Fatalf("ConnectBrowserlessSite(omezy): %v", err)
	}
	if connB.Group != sites.GroupB || connB.VisibleBrowser || !connB.Headless {
		t.Fatalf("expected Group B browserless, got %+v", connB)
	}

	// Group C (Manual Login Once): browserless with captured session
	if _, err := sites.CaptureSiteSession(dataDir, "chitchat", nil); err != nil {
		t.Fatalf("CaptureSiteSession(chitchat): %v", err)
	}
	connC, err := ConnectBrowserlessSite(dataDir, "chitchat")
	if err != nil {
		t.Fatalf("ConnectBrowserlessSite(chitchat): %v", err)
	}
	if connC.Group != sites.GroupC || connC.VisibleBrowser || !connC.Headless {
		t.Fatalf("expected Group C browserless, got %+v", connC)
	}

	// Group D (Anti-Bot): headless Rod in background with cookies injected
	connD, err := ConnectBrowserlessSite(dataDir, "e_chat")
	if err != nil {
		t.Fatalf("ConnectBrowserlessSite(e_chat): %v", err)
	}
	if connD.Group != sites.GroupD || connD.Method != BrowserlessMethodHeadlessDOM || !connD.Headless || connD.VisibleBrowser {
		t.Fatalf("expected Group D headless_dom in background, got %+v", connD)
	}
	if !connD.DOMConn.Headless || len(connD.DOMConn.Login.Cookies) == 0 {
		t.Fatalf("expected Group D DOMConn Headless=true with injected cookies, got %+v", connD.DOMConn)
	}

	// Group E (Complex): manual mode
	connE, err := ConnectBrowserlessSite(dataDir, "emeraldchat")
	if err != nil {
		t.Fatalf("ConnectBrowserlessSite(emeraldchat): %v", err)
	}
	if connE.Group != sites.GroupE || connE.Method != BrowserlessMethodManual || !connE.VisibleBrowser {
		t.Fatalf("expected Group E manual mode, got %+v", connE)
	}

	// Same-session Reconnect: reuses the same token & cookies without re-login
	origToken := connB.AuthToken
	if err := connB.Reconnect(context.Background()); err != nil {
		t.Fatalf("Reconnect(omezy): %v", err)
	}
	if connB.ReconnectCount != 1 || connB.AuthToken != origToken {
		t.Fatalf("expected ReconnectCount=1 with same AuthToken, got count=%d token=%s", connB.ReconnectCount, connB.AuthToken)
	}
}

func TestPrompt11ExpiredSessionAutoPausesWithNeedsSession(t *testing.T) {
	dataDir := t.TempDir()
	if _, err := sites.EnsureSites(dataDir); err != nil {
		t.Fatalf("EnsureSites: %v", err)
	}

	if _, err := sites.CaptureSiteSession(dataDir, "omezy", nil); err != nil {
		t.Fatalf("CaptureSiteSession(omezy): %v", err)
	}

	// Expire omezy's session in sites/omezy/sessions/default/meta.json
	ctxOmezy, err := sites.NewSiteContext(dataDir, "omezy")
	if err != nil {
		t.Fatalf("NewSiteContext(omezy): %v", err)
	}
	bundle, ok, err := sites.LoadCapturedSessionBundle(dataDir, "omezy")
	if err != nil || !ok {
		t.Fatalf("LoadCapturedSessionBundle: ok=%v err=%v", ok, err)
	}
	bundle.Meta.ExpiresAt = time.Now().UTC().Add(-1 * time.Hour).Format(time.RFC3339)
	if err := ctxOmezy.SaveJSON(filepath.Join("sessions", "default", "meta.json"), bundle.Meta); err != nil {
		t.Fatalf("SaveJSON(meta.json): %v", err)
	}

	_, err = ConnectBrowserlessSite(dataDir, "omezy")
	if !errors.Is(err, ErrNeedsSession) {
		t.Fatalf("expected ErrNeedsSession for expired session, got %v", err)
	}

	// Supervisor StartSite on expired session must auto-pause with PauseReason == "needs session"
	sup := NewSupervisor(Options{
		DataDir:       dataDir,
		CycleInterval: 20 * time.Millisecond,
	})
	defer func() { _ = sup.StopAll() }()

	startErr := sup.StartSite("omezy")
	if !errors.Is(startErr, ErrNeedsSession) {
		t.Fatalf("expected StartSite to return ErrNeedsSession, got %v", startErr)
	}
	st, err := sup.GetStatus("omezy")
	if err != nil {
		t.Fatalf("GetStatus(omezy): %v", err)
	}
	if st.Status != StatusPaused || !st.Paused || !st.AutoPaused || st.PauseReason != "needs session" {
		t.Fatalf("expected site auto-paused with PauseReason='needs session', got %+v", st)
	}
}
