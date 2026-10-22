package runner

import (
	"errors"
	"strings"
	"testing"
	"time"

	"chitchatbot/internal/sites"
)

func TestPrompt12SelfRecoveryScenariosAtoEAndNeedsHumanLimit(t *testing.T) {
	dataDir := t.TempDir()
	if _, err := sites.EnsureSites(dataDir); err != nil {
		t.Fatalf("EnsureSites: %v", err)
	}

	var notifications []Notification
	sup := NewSupervisor(Options{
		DataDir:       dataDir,
		CycleInterval: 20 * time.Millisecond,
		NotifyFunc: func(n Notification) {
			notifications = append(notifications, n)
		},
	})
	defer func() { _ = sup.StopAll() }()

	// Scenario A (Session expired + CAPTCHA notification)
	repA, err := sup.RecoverSite("chitchat", "auth_expired captcha")
	if err != nil {
		t.Fatalf("RecoverSite Scenario A: %v", err)
	}
	if repA.Scenario != ScenarioASessionExpired || !repA.SessionCaptured || !repA.CaptchaDetected || repA.Attempt != 1 {
		t.Fatalf("unexpected Scenario A report: %+v", repA)
	}

	// Scenario B (Auto-signup fail)
	repB, err := sup.RecoverSite("omezy", "signup_failed")
	if err != nil {
		t.Fatalf("RecoverSite Scenario B: %v", err)
	}
	if repB.Scenario != ScenarioBAutoSignupFail || !repB.SessionCaptured || repB.Attempt != 1 {
		t.Fatalf("unexpected Scenario B report: %+v", repB)
	}

	// Scenario C (WS/API broken -> DOM fallback -> "switched to DOM mode")
	repC, err := sup.RecoverSite("chatib", "ws_broken")
	if err != nil {
		t.Fatalf("RecoverSite Scenario C: %v", err)
	}
	if repC.Scenario != ScenarioCWSAPIBrokenDOMFallback || !repC.SwitchedToDOM || !strings.Contains(repC.Message, "switched to DOM mode") {
		t.Fatalf("unexpected Scenario C report: %+v", repC)
	}
	stChatib, _ := sup.GetStatus("chatib")
	if stChatib.Transport != TransportDOM {
		t.Fatalf("expected chatib switched to DOM transport, got %s", stChatib.Transport)
	}

	// Scenario D (Site structure changed -> dom_auto_scout.js updates dom_driver.js)
	repD, err := sup.RecoverSite("y99", "dom_changed")
	if err != nil {
		t.Fatalf("RecoverSite Scenario D: %v", err)
	}
	if repD.Scenario != ScenarioDSiteStructureChanged || !repD.SelectorsUpdated {
		t.Fatalf("unexpected Scenario D report: %+v", repD)
	}

	// Scenario E (Complete failure -> visible browser "I'm stuck, please login manually")
	repE, err := sup.RecoverSite("emeraldchat", "complete_failure")
	if err != nil {
		t.Fatalf("RecoverSite Scenario E: %v", err)
	}
	if repE.Scenario != ScenarioECompleteFailureManual || !strings.Contains(repE.Message, "I'm stuck, please login manually") {
		t.Fatalf("unexpected Scenario E report: %+v", repE)
	}

	// Max 3 attempts limit -> marks site "needs_human" on 3rd failure
	for i := 1; i <= 2; i++ {
		r, fErr := sup.RecoverSite("e_chat", "auth_expired force_fail")
		if fErr == nil {
			t.Fatalf("expected attempt %d to fail", i)
		}
		if r.NeedsHuman || r.Attempt != i {
			t.Fatalf("unexpected state on attempt %d: %+v", i, r)
		}
	}

	// 3rd failed attempt must mark e_chat as "needs_human"
	rep3, err3 := sup.RecoverSite("e_chat", "auth_expired force_fail")
	if !errors.Is(err3, ErrNeedsHuman) {
		t.Fatalf("expected ErrNeedsHuman on 3rd failure, got %v", err3)
	}
	if !rep3.NeedsHuman || rep3.Status != RecoveryStatusNeedsHuman || rep3.Attempt != 3 {
		t.Fatalf("expected NeedsHuman=true and Attempt=3, got %+v", rep3)
	}
	stEChat, _ := sup.GetStatus("e_chat")
	if stEChat.Status != StatusNeedsHuman || !stEChat.NeedsHuman || stEChat.RecoveryAttempts != 3 {
		t.Fatalf("expected e_chat status=needs_human, got %+v", stEChat)
	}

	// ResetSiteRecovery resets attempt counter back to 0
	sup.ResetSiteRecovery("e_chat")
	stReset, _ := sup.GetStatus("e_chat")
	if stReset.NeedsHuman || stReset.RecoveryAttempts != 0 {
		t.Fatalf("expected e_chat reset to 0 attempts, got %+v", stReset)
	}
}
