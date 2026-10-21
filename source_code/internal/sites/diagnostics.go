package sites

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"chitchatbot/internal/applog"
	"chitchatbot/internal/model"
	"chitchatbot/internal/store"
)

var siteDebugMu sync.Mutex

// SiteDebugEntry is a single structured line written to sites/<site_id>/logs/debug.jsonl.
type SiteDebugEntry struct {
	Time     string `json:"time"`
	SiteID   string `json:"site_id"`
	Category string `json:"category"` // dom | account | reply | protection | diagnostics | error
	Level    string `json:"level"`    // INFO | WARN | ERROR
	Message  string `json:"message"`
	Detail   string `json:"detail,omitempty"`
}

// DiagnosticCheck is one step in a deep site diagnostic check.
type DiagnosticCheck struct {
	Name   string `json:"name"`
	Status string `json:"status"` // PASS | WARN | FAIL
	Detail string `json:"detail"`
}

// SiteDiagnosticReport summarizes the complete health of an isolated site folder
// (sites/<site_id>/) and its active reply/account/proxy pipeline.
type SiteDiagnosticReport struct {
	SiteID        string            `json:"site_id"`
	SiteName      string            `json:"site_name"`
	Family        string            `json:"family"`
	AccountMode   string            `json:"account_mode"`
	StartURL      string            `json:"start_url"`
	FolderPath    string            `json:"folder_path"`
	OverallStatus string            `json:"overall_status"` // PASS | WARN | FAIL
	CheckedAt     string            `json:"checked_at"`
	AccountCount  int               `json:"account_count"`
	Checks        []DiagnosticCheck `json:"checks"`
	Summary       string            `json:"summary"`
}

// AppendSiteDebugLog appends a power-safe (fsync) redacted JSONL debug entry
// into sites/<site_id>/logs/debug.jsonl so every site's debug trace stays
// strictly inside its own folder.
func AppendSiteDebugLog(dataDir, siteID, category, level, message, detail string) error {
	siteDebugMu.Lock()
	defer siteDebugMu.Unlock()

	ctx, err := NewSiteContext(dataDir, siteID)
	if err != nil {
		return err
	}
	category = strings.TrimSpace(category)
	if category == "" {
		category = "system"
	}
	level = strings.ToUpper(strings.TrimSpace(level))
	if level == "" {
		level = "INFO"
	}

	entry := SiteDebugEntry{
		Time:     time.Now().UTC().Format(time.RFC3339),
		SiteID:   ctx.SiteID,
		Category: category,
		Level:    level,
		Message:  applog.Redact(strings.TrimSpace(message)),
		Detail:   applog.Redact(strings.TrimSpace(detail)),
	}
	line, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	line = append(line, '\n')

	if err := ctx.EnsureDir("logs"); err != nil {
		return err
	}
	f, err := ctx.OpenFile(filepath.Join("logs", "debug.jsonl"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	_, writeErr := f.Write(line)
	syncErr := f.Sync()
	closeErr := f.Close()
	if writeErr != nil {
		return writeErr
	}
	if syncErr != nil {
		return syncErr
	}
	return closeErr
}

// ReadSiteDebugTail returns the newest N entries from this site's logs/debug.jsonl.
func ReadSiteDebugTail(dataDir, siteID string, limit int) ([]SiteDebugEntry, error) {
	if limit <= 0 {
		limit = 50
	}
	ctx, err := NewSiteContext(dataDir, siteID)
	if err != nil {
		return nil, err
	}
	raw, err := ctx.ReadFile(filepath.Join("logs", "debug.jsonl"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) > limit {
		lines = lines[len(lines)-limit:]
	}
	out := make([]SiteDebugEntry, 0, len(lines))
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		var e SiteDebugEntry
		if err := json.Unmarshal([]byte(l), &e); err == nil {
			out = append(out, e)
		}
	}
	return out, nil
}

// RunSiteDiagnostics performs a 6-step deep health check on sites/<site_id>/
// and persists the report to sites/<site_id>/logs/last_diagnostics.json.
func RunSiteDiagnostics(
	dataDir string,
	siteID string,
	msgSettings model.MessageSettings,
	proxySettings model.ProxySettings,
) (SiteDiagnosticReport, error) {
	cfg, err := LoadSite(dataDir, siteID)
	if err != nil {
		return SiteDiagnosticReport{}, err
	}
	ctx, err := NewSiteContext(dataDir, cfg.ID)
	if err != nil {
		return SiteDiagnosticReport{}, err
	}
	sDir := ctx.FolderPath
	checks := make([]DiagnosticCheck, 0, 6)

	// Step 1: Isolated Site Folder Integrity
	missingFiles := []string{}
	for _, rel := range []string{
		"site.json",
		"dom_driver.js",
		cfg.ID + "_protection.txt",
		"README.txt",
		"accounts",
		filepath.Join("profiles", "default"),
		"logs",
	} {
		if _, statErr := os.Stat(filepath.Join(sDir, rel)); statErr != nil {
			missingFiles = append(missingFiles, rel)
		}
	}
	if len(missingFiles) == 0 {
		checks = append(checks, DiagnosticCheck{
			Name:   "1. Isolated Folder Structure",
			Status: "PASS",
			Detail: fmt.Sprintf("All 7 required files/folders verified in sites/%s/", cfg.ID),
		})
	} else {
		checks = append(checks, DiagnosticCheck{
			Name:   "1. Isolated Folder Structure",
			Status: "FAIL",
			Detail: fmt.Sprintf("Missing in sites/%s/: %s", cfg.ID, strings.Join(missingFiles, ", ")),
		})
	}

	// Step 2: URL, Selectors & DOM Driver Health
	u, urlErr := url.Parse(cfg.StartURL)
	sel := cfg.Selectors
	hasCoreSelectors := strings.TrimSpace(sel.ComposerInput) != "" &&
		strings.TrimSpace(sel.SkipButton) != "" &&
		strings.TrimSpace(sel.StartButton) != "" &&
		strings.TrimSpace(sel.MessageHistory) != "" &&
		strings.TrimSpace(sel.MessageRow) != ""
	driverOK := true
	if cfg.ID != DefaultSiteID {
		js := CustomDOMDriverJS(dataDir, cfg.ID)
		driverOK = strings.Contains(js, "snapshot") &&
			strings.Contains(js, "autoOnboardIfNeeded") &&
			strings.Contains(js, "detectProtectionState")
	}
	if urlErr == nil && u != nil && (u.Scheme == "https" || u.Scheme == "http") && hasCoreSelectors && driverOK {
		checks = append(checks, DiagnosticCheck{
			Name:   "2. DOM Selectors & Auto-Onboard Driver",
			Status: "PASS",
			Detail: fmt.Sprintf("URL (%s), family=%s selectors, and dom_driver.js verified", cfg.StartURL, cfg.Family),
		})
	} else {
		checks = append(checks, DiagnosticCheck{
			Name:   "2. DOM Selectors & Auto-Onboard Driver",
			Status: "FAIL",
			Detail: fmt.Sprintf("Invalid StartURL or incomplete selectors/driver in sites/%s/", cfg.ID),
		})
	}

	// Step 3: Isolated Accounts & Profile Sessions
	accCount := CountSiteAccounts(dataDir, cfg.ID)
	if accCount > 0 {
		checks = append(checks, DiagnosticCheck{
			Name:   "3. Site Accounts & Sessions",
			Status: "PASS",
			Detail: fmt.Sprintf("%d saved account(s) ready in sites/%s/accounts/ (mode=%s)", accCount, cfg.ID, cfg.AccountMode),
		})
	} else if cfg.AccountMode == AccountModeAutoRegister {
		checks = append(checks, DiagnosticCheck{
			Name:   "3. Site Accounts & Sessions",
			Status: "WARN",
			Detail: fmt.Sprintf("0 pre-registered accounts in sites/%s/accounts/ — click '⚡ Auto-Register Without Browser' or run directly as guest", cfg.ID),
		})
	} else {
		checks = append(checks, DiagnosticCheck{
			Name:   "3. Site Accounts & Sessions",
			Status: "WARN",
			Detail: fmt.Sprintf("0 saved browser accounts in sites/%s/accounts/ — click '🌐 Launch Browser for Account Setup' for first login", cfg.ID),
		})
	}

	// Step 4: Active Reply Engine Readiness
	chatMode := msgSettings.EffectiveChatMode()
	switch chatMode {
	case model.ChatModeAPI:
		hasKey := strings.TrimSpace(msgSettings.AIAPIKey) != ""
		if !hasKey {
			var apiMirror struct {
				APIKey string `json:"api_key"`
			}
			// API Chat config is per-site now; the studio/legacy locations stay
			// readable so an existing installation keeps working after upgrade.
			for _, mirrorPath := range []string{
				filepath.Join(ctx.FolderPath, "api_chat", "config.json"),
				filepath.Join(GlobalDir(dataDir), "api_chat", "config.json"),
				filepath.Join(dataDir, "api_chat", "config.json"),
			} {
				if ok, _ := store.LoadJSON(mirrorPath, &apiMirror); ok && strings.TrimSpace(apiMirror.APIKey) != "" {
					hasKey = true
					break
				}
			}
		}
		if hasKey {
			checks = append(checks, DiagnosticCheck{
				Name:   "4. Reply Engine (API Chat)",
				Status: "PASS",
				Detail: "API Chat key & persona config ready via apichat.exe",
			})
		} else {
			checks = append(checks, DiagnosticCheck{
				Name:   "4. Reply Engine (API Chat)",
				Status: "WARN",
				Detail: "API Chat selected, but no API key saved yet in apichat.exe (fallback deck will be used)",
			})
		}
	case model.ChatModeEVAChat:
		checks = append(checks, DiagnosticCheck{
			Name:   "4. Reply Engine (EVAChat Flow)",
			Status: "PASS",
			Detail: "Pure TXT EVAChat Flow stateful reply pools enabled",
		})
	case model.ChatModeFolder:
		if len(msgSettings.LoadedSequences) > 0 {
			checks = append(checks, DiagnosticCheck{
				Name:   "4. Reply Engine (Folder Sequences)",
				Status: "PASS",
				Detail: fmt.Sprintf("%d TXT sequence file(s) loaded", len(msgSettings.LoadedSequences)),
			})
		} else {
			checks = append(checks, DiagnosticCheck{
				Name:   "4. Reply Engine (Folder Sequences)",
				Status: "FAIL",
				Detail: "Folder mode selected, but 0 sequences loaded",
			})
		}
	default:
		lines := msgSettings.SequenceLines()
		if len(lines) > 0 {
			checks = append(checks, DiagnosticCheck{
				Name:   "4. Reply Engine (Single Sequence)",
				Status: "PASS",
				Detail: fmt.Sprintf("Single Sequence ready (%d step(s))", len(lines)),
			})
		} else {
			checks = append(checks, DiagnosticCheck{
				Name:   "4. Reply Engine (Single Sequence)",
				Status: "WARN",
				Detail: "Single Sequence is empty — enter message lines in 3 · Message",
			})
		}
	}

	// Step 5: Proxy & Network Configuration
	if proxySettings.Enabled {
		lines := 0
		for _, l := range strings.Split(proxySettings.Entries, "\n") {
			if strings.TrimSpace(l) != "" {
				lines++
			}
		}
		if lines > 0 {
			checks = append(checks, DiagnosticCheck{
				Name:   "5. Proxy Rotation",
				Status: "PASS",
				Detail: fmt.Sprintf("Proxy enabled with %d line(s)", lines),
			})
		} else {
			checks = append(checks, DiagnosticCheck{
				Name:   "5. Proxy Rotation",
				Status: "FAIL",
				Detail: "Proxy is enabled, but proxy list is empty",
			})
		}
	} else {
		checks = append(checks, DiagnosticCheck{
			Name:   "5. Proxy Rotation",
			Status: "PASS",
			Detail: "Direct connection (Proxy disabled)",
		})
	}

	// Step 6: Power-Safe Isolated Debug Log
	logErr := AppendSiteDebugLog(dataDir, cfg.ID, "diagnostics", "INFO", "Deep diagnostics executed", fmt.Sprintf("mode=%s accounts=%d", cfg.AccountMode, accCount))
	if logErr == nil {
		checks = append(checks, DiagnosticCheck{
			Name:   "6. Isolated Site Debug Log (fsync)",
			Status: "PASS",
			Detail: fmt.Sprintf("Power-safe debug log writable at sites/%s/logs/debug.jsonl", cfg.ID),
		})
	} else {
		checks = append(checks, DiagnosticCheck{
			Name:   "6. Isolated Site Debug Log (fsync)",
			Status: "FAIL",
			Detail: fmt.Sprintf("Failed writing sites/%s/logs/debug.jsonl: %v", cfg.ID, logErr),
		})
	}

	overall := "PASS"
	passCount, warnCount, failCount := 0, 0, 0
	for _, c := range checks {
		switch c.Status {
		case "FAIL":
			failCount++
			overall = "FAIL"
		case "WARN":
			warnCount++
			if overall == "PASS" {
				overall = "WARN"
			}
		default:
			passCount++
		}
	}

	summary := fmt.Sprintf("[%s] %s (sites/%s/) — %d PASS, %d WARN, %d FAIL · Mode=%s · Saved Accounts=%d",
		overall, cfg.Name, cfg.ID, passCount, warnCount, failCount, cfg.AccountMode, accCount)

	report := SiteDiagnosticReport{
		SiteID:        cfg.ID,
		SiteName:      cfg.Name,
		Family:        cfg.Family,
		AccountMode:   cfg.AccountMode,
		StartURL:      cfg.StartURL,
		FolderPath:    sDir,
		OverallStatus: overall,
		CheckedAt:     time.Now().UTC().Format(time.RFC3339),
		AccountCount:  accCount,
		Checks:        checks,
		Summary:       summary,
	}

	if err := ctx.SaveJSON(filepath.Join("logs", "last_diagnostics.json"), report); err != nil {
		return report, err
	}

	return report, nil
}
