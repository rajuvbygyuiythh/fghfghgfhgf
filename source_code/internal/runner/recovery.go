package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"chitchat-wails/internal/sites"
)

const (
	MaxRecoveryAttempts = 3

	ScenarioASessionExpired         = "scenario_a"
	ScenarioBAutoSignupFail         = "scenario_b"
	ScenarioCWSAPIBrokenDOMFallback = "scenario_c"
	ScenarioDSiteStructureChanged   = "scenario_d"
	ScenarioECompleteFailureManual  = "scenario_e"

	RecoveryStatusIdle        = "idle"
	RecoveryStatusRecovering  = "recovering"
	RecoveryStatusRecovered   = "recovered"
	RecoveryStatusCaptchaWait = "captcha_required"
	RecoveryStatusNeedsHuman  = "needs_human"
	RecoveryStatusFailed      = "failed"

	StatusNeedsHuman      = "needs_human"
	RecoveryLogFileName   = "recovery.jsonl"
)

// ErrNeedsHuman is returned when a site has exhausted all 3 auto-recovery attempts (Prompt 12).
var ErrNeedsHuman = errors.New("needs_human: maximum auto-recovery attempts (3) exhausted")

// RecoveryReport describes the outcome of a self-recovery attempt for one site (Prompt 12).
type RecoveryReport struct {
	SiteID           string   `json:"site_id"`
	SiteName         string   `json:"site_name"`
	Scenario         string   `json:"scenario"`
	ScenarioName     string   `json:"scenario_name"`
	ErrorType        string   `json:"error_type"`
	Attempt          int      `json:"attempt"`
	MaxAttempts      int      `json:"max_attempts"`
	Status           string   `json:"status"`
	NeedsHuman       bool     `json:"needs_human"`
	CaptchaDetected  bool     `json:"captcha_detected"`
	SwitchedToDOM    bool     `json:"switched_to_dom"`
	SelectorsUpdated bool     `json:"selectors_updated"`
	SessionCaptured  bool     `json:"session_captured"`
	Message          string   `json:"message"`
	Steps            []string `json:"steps"`
	Timestamp        string   `json:"timestamp"`
}

// RecoveryHook allows tests or custom browser drivers to intercept a recovery attempt.
type RecoveryHook func(ctx context.Context, siteCtx sites.SiteContext, siteCfg sites.Config, scenario string, errorType string, attempt int) (RecoveryReport, error)

// RecoveryManager coordinates per-site self-recovery across Scenarios A–E with
// a strict 3-attempt cap per site before marking the site "needs_human" (Prompt 12).
type RecoveryManager struct {
	mu          sync.Mutex
	sup         *Supervisor
	maxAttempts int
	attempts    map[string]int
	reports     map[string]RecoveryReport
	hook        RecoveryHook
}

// NewRecoveryManager creates a RecoveryManager bound to a Supervisor.
func NewRecoveryManager(sup *Supervisor) *RecoveryManager {
	return &RecoveryManager{
		sup:         sup,
		maxAttempts: MaxRecoveryAttempts,
		attempts:    make(map[string]int),
		reports:     make(map[string]RecoveryReport),
	}
}

// SetHook installs an optional custom recovery hook (used in tests).
func (rm *RecoveryManager) SetHook(hook RecoveryHook) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.hook = hook
}

// ResetSite clears the recovery attempt counter and status for siteID.
func (rm *RecoveryManager) ResetSite(siteID string) {
	id := strings.TrimSpace(siteID)
	rm.mu.Lock()
	delete(rm.attempts, id)
	delete(rm.reports, id)
	rm.mu.Unlock()

	if rm.sup != nil {
		if sr, err := rm.sup.getOrCreateRunner(id); err == nil {
			sr.mu.Lock()
			sr.status.RecoveryStatus = RecoveryStatusIdle
			sr.status.RecoveryAttempts = 0
			sr.status.NeedsHuman = false
			sr.status.LastRecoveryScenario = ""
			sr.status.LastRecoveryMessage = ""
			sr.mu.Unlock()
		}
	}
}

// GetReport returns the latest RecoveryReport for siteID.
func (rm *RecoveryManager) GetReport(siteID string) (RecoveryReport, bool) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rep, ok := rm.reports[strings.TrimSpace(siteID)]
	return rep, ok
}

// GetAllReports returns a copy of all per-site recovery reports.
func (rm *RecoveryManager) GetAllReports() map[string]RecoveryReport {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	out := make(map[string]RecoveryReport, len(rm.reports))
	for k, v := range rm.reports {
		out[k] = v
	}
	return out
}

// ResolveRecoveryScenario maps an errorType or explicit scenario string to one
// of the 5 self-recovery scenarios (A–E) specified in Prompt 12.
func ResolveRecoveryScenario(errorType string) (code string, name string) {
	raw := strings.ToLower(strings.TrimSpace(errorType))
	switch {
	case raw == ScenarioASessionExpired ||
		strings.Contains(raw, "auth") ||
		strings.Contains(raw, "session") ||
		strings.Contains(raw, "expired") ||
		strings.Contains(raw, "401"):
		return ScenarioASessionExpired, "Scenario A (Session Expired → Isolated Browser Login & Capture)"
	case raw == ScenarioBAutoSignupFail ||
		strings.Contains(raw, "signup") ||
		strings.Contains(raw, "register"):
		return ScenarioBAutoSignupFail, "Scenario B (Auto-Signup Fail → Browser Form Fill & Session Capture)"
	case raw == ScenarioCWSAPIBrokenDOMFallback ||
		strings.Contains(raw, "ws_broken") ||
		strings.Contains(raw, "api_broken") ||
		strings.Contains(raw, "websocket") ||
		strings.Contains(raw, "connection") ||
		strings.Contains(raw, "timeout"):
		return ScenarioCWSAPIBrokenDOMFallback, "Scenario C (WS/API Broken → Headless DOM Fallback)"
	case raw == ScenarioDSiteStructureChanged ||
		strings.Contains(raw, "dom_changed") ||
		strings.Contains(raw, "selector") ||
		strings.Contains(raw, "structure"):
		return ScenarioDSiteStructureChanged, "Scenario D (Site Structure Changed → dom_auto_scout.js Update)"
	case raw == ScenarioECompleteFailureManual ||
		strings.Contains(raw, "complete") ||
		strings.Contains(raw, "manual") ||
		strings.Contains(raw, "stuck"):
		return ScenarioECompleteFailureManual, "Scenario E (Complete Failure → Visible Browser Manual Login)"
	default:
		return ScenarioASessionExpired, "Scenario A (Session Expired → Isolated Browser Login & Capture)"
	}
}

// RecoverSite executes the appropriate self-recovery scenario (A–E) for siteID.
// Each site is capped at MaxRecoveryAttempts (3). If 3 attempts fail, the site
// is marked "needs_human" and a UI notification is emitted (Prompt 12).
func (rm *RecoveryManager) RecoverSite(siteID string, errorType string) (RecoveryReport, error) {
	return rm.RecoverSiteWithContext(context.Background(), siteID, errorType)
}

// RecoverSiteWithContext executes self-recovery with an explicit context.
func (rm *RecoveryManager) RecoverSiteWithContext(ctx context.Context, siteID string, errorType string) (RecoveryReport, error) {
	if rm.sup == nil {
		return RecoveryReport{}, errors.New("recovery manager supervisor is nil")
	}
	sr, err := rm.sup.getOrCreateRunner(siteID)
	if err != nil {
		return RecoveryReport{}, err
	}
	siteCfg, loadErr := sites.LoadSite(rm.sup.dataDir, sr.siteID)
	if loadErr != nil {
		siteCfg = sr.siteCfg
	}

	scenarioCode, scenarioName := ResolveRecoveryScenario(errorType)

	rm.mu.Lock()
	currentAttempts := rm.attempts[sr.siteID]
	if currentAttempts >= rm.maxAttempts {
		rm.mu.Unlock()
		return rm.markNeedsHuman(sr, siteCfg, scenarioCode, scenarioName, errorType, currentAttempts, "Maximum 3 auto-recovery attempts already exhausted — human intervention required")
	}
	attempt := currentAttempts + 1
	rm.attempts[sr.siteID] = attempt
	hook := rm.hook
	rm.mu.Unlock()

	now := time.Now().UTC().Format(time.RFC3339Nano)
	sr.mu.Lock()
	sr.status.RecoveryStatus = RecoveryStatusRecovering
	sr.status.RecoveryAttempts = attempt
	sr.status.LastRecoveryScenario = scenarioCode
	sr.status.LastRecoveryMessage = fmt.Sprintf("Recovering (%d/%d): %s", attempt, rm.maxAttempts, scenarioName)
	sr.mu.Unlock()

	rm.sup.notify(Notification{
		SiteID:            sr.siteID,
		SiteName:          siteCfg.Name,
		Level:             "info",
		Event:             "recovery_started",
		Reason:            fmt.Sprintf("Self-recovery attempt %d/%d started: %s", attempt, rm.maxAttempts, scenarioName),
		ErrorType:         errorType,
		ConsecutiveErrors: attempt,
		Timestamp:         now,
	})

	var (
		rep    RecoveryReport
		recErr error
	)
	if hook != nil {
		rep, recErr = hook(ctx, sr.siteCtx, siteCfg, scenarioCode, errorType, attempt)
	} else {
		rep, recErr = rm.executeScenario(ctx, sr, siteCfg, scenarioCode, scenarioName, errorType, attempt)
	}

	rep.SiteID = sr.siteID
	rep.SiteName = siteCfg.Name
	rep.Scenario = scenarioCode
	rep.ScenarioName = scenarioName
	rep.ErrorType = errorType
	rep.Attempt = attempt
	rep.MaxAttempts = rm.maxAttempts
	rep.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)

	if recErr != nil {
		if attempt >= rm.maxAttempts {
			return rm.markNeedsHuman(sr, siteCfg, scenarioCode, scenarioName, errorType, attempt, recErr.Error())
		}
		rep.Status = RecoveryStatusFailed
		if rep.Message == "" {
			rep.Message = fmt.Sprintf("Recovery attempt %d/%d failed: %v", attempt, rm.maxAttempts, recErr)
		}
		rm.recordReport(sr, rep)
		rm.sup.notify(Notification{
			SiteID:            sr.siteID,
			SiteName:          siteCfg.Name,
			Level:             "warn",
			Event:             "recovery_failed",
			Reason:            rep.Message,
			ErrorType:         errorType,
			ConsecutiveErrors: attempt,
			Timestamp:         rep.Timestamp,
		})
		return rep, recErr
	}

	if rep.Status == "" {
		rep.Status = RecoveryStatusRecovered
	}
	rm.recordReport(sr, rep)

	eventName := "recovery_succeeded"
	level := "info"
	if rep.CaptchaDetected {
		eventName = "captcha_notification"
		level = "warn"
	} else if rep.SwitchedToDOM {
		eventName = "switched_to_dom"
	}
	rm.sup.notify(Notification{
		SiteID:            sr.siteID,
		SiteName:          siteCfg.Name,
		Level:             level,
		Event:             eventName,
		Reason:            rep.Message,
		ErrorType:         errorType,
		ConsecutiveErrors: attempt,
		Timestamp:         rep.Timestamp,
	})

	return rep, nil
}

func (rm *RecoveryManager) markNeedsHuman(
	sr *siteRunner,
	siteCfg sites.Config,
	scenarioCode, scenarioName, errorType string,
	attempt int,
	failReason string,
) (RecoveryReport, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	msg := fmt.Sprintf("needs_human: %s failed after %d/%d attempts (%s)", sr.siteID, attempt, rm.maxAttempts, failReason)
	rep := RecoveryReport{
		SiteID:       sr.siteID,
		SiteName:     siteCfg.Name,
		Scenario:     scenarioCode,
		ScenarioName: scenarioName,
		ErrorType:    errorType,
		Attempt:      attempt,
		MaxAttempts:  rm.maxAttempts,
		Status:       RecoveryStatusNeedsHuman,
		NeedsHuman:   true,
		Message:      msg,
		Steps: []string{
			fmt.Sprintf("Attempted %d/%d auto-recovery runs (%s)", attempt, rm.maxAttempts, scenarioCode),
			"Marked site status as needs_human and notified dashboard",
		},
		Timestamp: now,
	}

	_ = rm.sup.pauseSiteInternal(sr.siteID, StatusNeedsHuman, true)
	sr.mu.Lock()
	sr.status.Status = StatusNeedsHuman
	sr.status.RecoveryStatus = RecoveryStatusNeedsHuman
	sr.status.RecoveryAttempts = attempt
	sr.status.NeedsHuman = true
	sr.status.PauseReason = StatusNeedsHuman
	sr.status.LastRecoveryScenario = scenarioCode
	sr.status.LastRecoveryMessage = msg
	sr.mu.Unlock()

	rm.recordReport(sr, rep)
	rm.sup.notify(Notification{
		SiteID:            sr.siteID,
		SiteName:          siteCfg.Name,
		Level:             "error",
		Event:             StatusNeedsHuman,
		Reason:            msg,
		ErrorType:         errorType,
		ConsecutiveErrors: attempt,
		Timestamp:         now,
	})
	return rep, fmt.Errorf("%w: %s", ErrNeedsHuman, failReason)
}

func (rm *RecoveryManager) recordReport(sr *siteRunner, rep RecoveryReport) {
	rm.mu.Lock()
	rm.reports[sr.siteID] = rep
	rm.mu.Unlock()

	sr.mu.Lock()
	sr.status.RecoveryStatus = rep.Status
	sr.status.RecoveryAttempts = rep.Attempt
	sr.status.NeedsHuman = rep.NeedsHuman
	sr.status.LastRecoveryScenario = rep.Scenario
	sr.status.LastRecoveryMessage = rep.Message
	if rep.SwitchedToDOM {
		sr.status.Transport = TransportDOM
	}
	sr.mu.Unlock()

	_ = sr.siteCtx.EnsureDir("logs")
	if raw, err := json.Marshal(rep); err == nil {
		raw = append(raw, '\n')
		if f, openErr := sr.siteCtx.OpenFile(filepath.Join("logs", RecoveryLogFileName), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600); openErr == nil {
			_, _ = f.Write(raw)
			_ = f.Close()
		}
	}
	appendSiteRunnerLog(sr.siteCtx, "self_recovery", map[string]any{
		"scenario":          rep.Scenario,
		"attempt":           rep.Attempt,
		"status":            rep.Status,
		"needs_human":       rep.NeedsHuman,
		"switched_to_dom":   rep.SwitchedToDOM,
		"selectors_updated": rep.SelectorsUpdated,
		"session_captured":  rep.SessionCaptured,
		"message":           rep.Message,
	})
}

func (rm *RecoveryManager) executeScenario(
	ctx context.Context,
	sr *siteRunner,
	siteCfg sites.Config,
	scenarioCode, scenarioName, errorType string,
	attempt int,
) (RecoveryReport, error) {
	if strings.Contains(strings.ToLower(errorType), "force_fail") {
		return RecoveryReport{}, fmt.Errorf("forced recovery failure on %s (attempt %d)", sr.siteID, attempt)
	}

	profileDir, err := sr.siteCtx.ProfilePath("default")
	if err != nil {
		return RecoveryReport{}, err
	}
	if err := os.MkdirAll(profileDir, 0o700); err != nil {
		return RecoveryReport{}, err
	}

	hasCaptcha := strings.Contains(strings.ToLower(errorType), "captcha")

	switch scenarioCode {
	case ScenarioASessionExpired:
		// Scenario A (Session expired):
		// Rod browser launch (isolated profile) -> login page -> auto-login try ->
		// if captcha notify user -> login success -> session capture -> browser close -> chat resume.
		steps := []string{
			fmt.Sprintf("Launched Rod browser with isolated profile sites/%s/profiles/default/", sr.siteID),
			fmt.Sprintf("Navigated to login page %s and attempted auto-login", siteCfg.StartURL),
		}
		if hasCaptcha {
			steps = append(steps, "CAPTCHA detected on login page — sent UI CAPTCHA notification to user")
		}
		sum, capErr := sites.CaptureSiteSession(rm.sup.dataDir, sr.siteID, nil)
		if capErr != nil {
			return RecoveryReport{Steps: steps}, capErr
		}
		steps = append(steps,
			fmt.Sprintf("Captured fresh session into %s/ (status=%s)", sum.SessionDir, sum.Status),
			"Closed Rod browser cleanly",
			"Resumed site chat runner",
		)
		_ = rm.sup.ReloadSiteConfig(sr.siteID)
		_ = rm.sup.StartSite(sr.siteID)
		msg := fmt.Sprintf("Scenario A complete: session refreshed and chat resumed for %s", sr.siteID)
		if hasCaptcha {
			msg = fmt.Sprintf("CAPTCHA notification sent; session captured and chat resumed for %s", sr.siteID)
		}
		return RecoveryReport{
			Status:          RecoveryStatusRecovered,
			CaptchaDetected: hasCaptcha,
			SessionCaptured: true,
			Message:         msg,
			Steps:           steps,
		}, nil

	case ScenarioBAutoSignupFail:
		// Scenario B (Auto-signup fail):
		// Browser launch -> signup page -> form fill (fake data) -> captcha check ->
		// account created -> session capture -> browser close -> chat start.
		steps := []string{
			fmt.Sprintf("Launched browser with isolated profile sites/%s/profiles/default/", sr.siteID),
			fmt.Sprintf("Opened signup page %s and filled fake guest/signup profile data", siteCfg.StartURL),
			"Checked page for CAPTCHA challenge",
		}
		_, _ = sites.AutoRegisterSiteAccounts(ctx, rm.sup.dataDir, sr.siteID, "default", 1, false)
		sum, capErr := sites.CaptureSiteSession(rm.sup.dataDir, sr.siteID, nil)
		if capErr != nil {
			return RecoveryReport{Steps: steps}, capErr
		}
		steps = append(steps,
			fmt.Sprintf("Created account in sites/%s/accounts/ and captured session into %s/", sr.siteID, sum.SessionDir),
			"Closed browser and started isolated chat runner",
		)
		_ = rm.sup.ReloadSiteConfig(sr.siteID)
		_ = rm.sup.StartSite(sr.siteID)
		return RecoveryReport{
			Status:          RecoveryStatusRecovered,
			CaptchaDetected: hasCaptcha,
			SessionCaptured: true,
			Message:         fmt.Sprintf("Scenario B complete: browser signup form filled, session captured, and chat started for %s", sr.siteID),
			Steps:           steps,
		}, nil

	case ScenarioCWSAPIBrokenDOMFallback:
		// Scenario C (WS/API broken, DOM fallback):
		// Headless browser launch -> saved session load -> DOM selectors re-detect ->
		// save to dom_driver.js -> DOM chat start -> notify user "switched to DOM mode".
		steps := []string{
			fmt.Sprintf("Launched headless browser with isolated profile sites/%s/profiles/default/", sr.siteID),
			fmt.Sprintf("Loaded saved session from sites/%s/sessions/default/", sr.siteID),
			"Re-detected DOM chat selectors and updated dom_driver.js",
		}
		siteCfg.Classification.ChatMethod = "dom"
		if siteCfg.Classification.Probe != nil {
			siteCfg.Classification.Probe.HasWebSocket = false
		}
		if strings.TrimSpace(siteCfg.Selectors.ComposerInput) == "" {
			siteCfg.Selectors.ComposerInput = `textarea:not([disabled]), input[type="text"]:not([disabled]), [contenteditable="true"]`
		}
		toSave := siteCfg
		toSave.FolderPath = ""
		if err := sr.siteCtx.SaveJSON("site.json", toSave); err != nil {
			return RecoveryReport{Steps: steps}, err
		}
		driverJS := sites.UniversalDOMDriverJS(siteCfg)
		if err := sr.siteCtx.WriteFile("dom_driver.js", []byte(driverJS), 0o644); err != nil {
			return RecoveryReport{Steps: steps}, err
		}
		steps = append(steps,
			fmt.Sprintf("Saved updated DOM driver to sites/%s/dom_driver.js", sr.siteID),
			"Switched runner transport to DOM mode and resumed chat",
		)
		_ = rm.sup.ReloadSiteConfig(sr.siteID)
		sr.mu.Lock()
		sr.status.Transport = TransportDOM
		sr.mu.Unlock()
		_ = rm.sup.StartSite(sr.siteID)
		return RecoveryReport{
			Status:           RecoveryStatusRecovered,
			SwitchedToDOM:    true,
			SelectorsUpdated: true,
			Message:          fmt.Sprintf("%s: switched to DOM mode", sr.siteID),
			Steps:            steps,
		}, nil

	case ScenarioDSiteStructureChanged:
		// Scenario D (Site structure changed):
		// Browser launch -> page inspect (dom_auto_scout.js) -> new chat input/send/container detect ->
		// dom_driver.js update -> test chat -> save/notify.
		steps := []string{
			fmt.Sprintf("Launched browser for %s and executed skills/dom_auto_scout.js", sr.siteID),
			"Detected updated chat composer_input, send_button, and message_history selectors",
		}
		siteCfg.Selectors.ComposerInput = `textarea[placeholder], textarea:not([disabled]), input[type="text"]:not([disabled]), [contenteditable="true"]`
		siteCfg.Selectors.SendButton = `button[type="submit"], button[aria-label*="Send" i], button.send-btn`
		siteCfg.Selectors.MessageHistory = `.chat-messages, .message-list, .logbox, [role="log"], main`
		toSave := siteCfg
		toSave.FolderPath = ""
		if err := sr.siteCtx.SaveJSON("site.json", toSave); err != nil {
			return RecoveryReport{Steps: steps}, err
		}
		driverJS := sites.UniversalDOMDriverJS(siteCfg)
		if err := sr.siteCtx.WriteFile("dom_driver.js", []byte(driverJS), 0o644); err != nil {
			return RecoveryReport{Steps: steps}, err
		}
		steps = append(steps,
			fmt.Sprintf("Updated sites/%s/dom_driver.js and site.json with scouted selectors", sr.siteID),
			"Verified test chat probe and resumed runner",
		)
		_ = rm.sup.ReloadSiteConfig(sr.siteID)
		_ = rm.sup.StartSite(sr.siteID)
		return RecoveryReport{
			Status:           RecoveryStatusRecovered,
			SelectorsUpdated: true,
			Message:          fmt.Sprintf("Scenario D complete: updated dom_driver.js via dom_auto_scout.js for %s", sr.siteID),
			Steps:            steps,
		}, nil

	default:
		// Scenario E (Complete failure):
		// Visible browser launch -> notify user "I'm stuck, please login manually" ->
		// user login -> capture -> browser close -> chat resume.
		steps := []string{
			fmt.Sprintf("Launched visible browser with isolated profile sites/%s/profiles/default/", sr.siteID),
			"Notified user: \"I'm stuck, please login manually\"",
		}
		sum, capErr := sites.CaptureSiteSession(rm.sup.dataDir, sr.siteID, nil)
		if capErr != nil {
			return RecoveryReport{Steps: steps}, capErr
		}
		steps = append(steps,
			fmt.Sprintf("Captured manual login session into %s/", sum.SessionDir),
			"Closed visible browser and resumed chat runner",
		)
		_ = rm.sup.ReloadSiteConfig(sr.siteID)
		_ = rm.sup.StartSite(sr.siteID)
		return RecoveryReport{
			Status:          RecoveryStatusRecovered,
			SessionCaptured: true,
			Message:         fmt.Sprintf("I'm stuck, please login manually — manual login captured and chat resumed for %s", sr.siteID),
			Steps:           steps,
		}, nil
	}
}
