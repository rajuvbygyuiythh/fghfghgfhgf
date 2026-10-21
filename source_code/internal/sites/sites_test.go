package sites

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"chitchatbot/internal/model"
)

func TestEnsureSitesAndCreateIsolatedSite(t *testing.T) {
	tmp := t.TempDir()
	dataDir := filepath.Join(tmp, "data")

	catalog := CatalogConfigs()
	if len(catalog) != 100 {
		t.Fatalf("expected exactly 100 built-in non-ChitChat catalog sites, got %d", len(catalog))
	}

	seen := map[string]bool{}
	autoRegCount := 0
	browserSetupCount := 0
	for idx, c := range catalog {
		if c.ID == "" || c.ID == DefaultSiteID {
			t.Fatalf("invalid catalog ID at index %d: %q", idx, c.ID)
		}
		if seen[c.ID] {
			t.Fatalf("duplicate catalog site ID %q at index %d", c.ID, idx)
		}
		seen[c.ID] = true
		if !strings.HasPrefix(c.StartURL, "https://") {
			t.Fatalf("site %s StartURL must start with https://, got %q", c.ID, c.StartURL)
		}
		if strings.TrimSpace(c.EntryFlow) == "" || strings.TrimSpace(c.ProtectionSummary) == "" {
			t.Fatalf("site %s missing EntryFlow or ProtectionSummary", c.ID)
		}
		switch c.AccountMode {
		case AccountModeAutoRegister:
			autoRegCount++
		case AccountModeBrowserSetup:
			browserSetupCount++
		default:
			t.Fatalf("site %s has invalid AccountMode %q", c.ID, c.AccountMode)
		}
	}
	if autoRegCount != 85 || browserSetupCount != 15 {
		t.Fatalf("expected 85 auto_register and 15 browser_setup catalog sites, got auto=%d browser=%d", autoRegCount, browserSetupCount)
	}

	list, err := List(dataDir)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	// 100 catalog sites + chitchat + template_new_site = 102
	if len(list) != 102 {
		t.Fatalf("expected 102 sites (chitchat + 100 catalog + template_new_site), got %d", len(list))
	}
	if list[0].ID != DefaultSiteID {
		t.Fatalf("expected first site to be %q, got %q", DefaultSiteID, list[0].ID)
	}
	if list[0].AccountMode != AccountModeBrowserSetup {
		t.Fatalf("expected chitchat AccountMode=%q, got %q", AccountModeBrowserSetup, list[0].AccountMode)
	}

	// Verify every catalog site folder has site.json, dom_driver.js, <site_id>_protection.txt, README.txt, accounts/.gitkeep, profiles/default/.gitkeep
	for _, c := range catalog {
		sDir := DataDirForSite(dataDir, c.ID)
		for _, rel := range []string{
			"site.json",
			"dom_driver.js",
			c.ID + "_protection.txt",
			"README.txt",
			filepath.Join("accounts", ".gitkeep"),
			filepath.Join("profiles", "default", ".gitkeep"),
		} {
			if _, err := os.Stat(filepath.Join(sDir, rel)); err != nil {
				t.Fatalf("site %s missing %s: %v", c.ID, rel, err)
			}
		}
	}

	// Phase 1: every site, including chitchat, has its own folder. The shared
	// dataDir is never used as a site data root.
	if got := DataDirForSite(dataDir, DefaultSiteID); got == dataDir || filepath.Base(got) != DefaultSiteID {
		t.Fatalf("expected default site to be isolated under sites/%s, got %q", DefaultSiteID, got)
	}
	if _, err := os.Stat(filepath.Join(DataDirForSite(dataDir, DefaultSiteID), "site.json")); err != nil {
		t.Fatalf("expected chitchat site.json in its own folder: %v", err)
	}

	// Create a new site and verify complete folder isolation.
	created, err := CreateSiteWithMode(dataDir, "Custom Test Chat", "customchat.example/text", AccountModeBrowserSetup)
	if err != nil {
		t.Fatalf("CreateSiteWithMode failed: %v", err)
	}
	if created.ID != "custom_test_chat" {
		t.Fatalf("expected sanitized ID 'custom_test_chat', got %q", created.ID)
	}
	if created.AccountMode != AccountModeBrowserSetup {
		t.Fatalf("expected AccountMode=%q, got %q", AccountModeBrowserSetup, created.AccountMode)
	}
	if created.StartURL != "https://customchat.example/text" {
		t.Fatalf("expected normalized https URL, got %q", created.StartURL)
	}
	if created.AuthMode != "dom_only" {
		t.Fatalf("expected dom_only auth mode for new site, got %q", created.AuthMode)
	}

	siteDataDir := DataDirForSite(dataDir, created.ID)
	if siteDataDir == dataDir {
		t.Fatalf("new site must have isolated dataDir, got shared %q", siteDataDir)
	}
	for _, rel := range []string{
		"site.json",
		"dom_driver.js",
		"custom_test_chat_protection.txt",
		"README.txt",
		filepath.Join("accounts", ".gitkeep"),
		filepath.Join("profiles", "default", ".gitkeep"),
	} {
		if _, err := os.Stat(filepath.Join(siteDataDir, rel)); err != nil {
			t.Fatalf("missing isolated site file %s: %v", rel, err)
		}
	}

	js := CustomDOMDriverJS(dataDir, created.ID)
	if !strings.Contains(js, "snapshot") || !strings.Contains(js, "custom_test_chat") || !strings.Contains(js, "autoOnboardIfNeeded") {
		t.Fatalf("expected custom DOM driver JS with autoOnboardIfNeeded for isolated site, got len=%d", len(js))
	}
	if chitchatJS := CustomDOMDriverJS(dataDir, DefaultSiteID); chitchatJS != "" {
		t.Fatalf("expected default chitchat site to use built-in verified DOM driver, got override")
	}
}

func TestAutoRegisterAndBrowserAccountIsolation(t *testing.T) {
	tmp := t.TempDir()
	dataDir := filepath.Join(tmp, "data")
	if _, err := EnsureSites(dataDir); err != nil {
		t.Fatalf("EnsureSites failed: %v", err)
	}

	// 1. Auto-register 3 accounts for "omezy" without browser
	accs, err := AutoRegisterSiteAccounts(context.Background(), dataDir, "omezy", "default", 3, false)
	if err != nil {
		t.Fatalf("AutoRegisterSiteAccounts failed: %v", err)
	}
	if len(accs) != 3 {
		t.Fatalf("expected 3 auto-registered accounts, got %d", len(accs))
	}
	for _, a := range accs {
		if a.SiteID != "omezy" || a.AccountMode != AccountModeAutoRegister || a.Gender != "female" {
			t.Fatalf("unexpected account metadata: %+v", a)
		}
		if _, err := os.Stat(a.FilePath); err != nil {
			t.Fatalf("expected account file on disk at %s: %v", a.FilePath, err)
		}
		if _, err := os.Stat(a.LoginPath); err != nil {
			t.Fatalf("expected profile login_info file on disk at %s: %v", a.LoginPath, err)
		}
	}

	// Verify omezy has 3 accounts while funchatx still has 0 (100% separate folders)
	if got := CountSiteAccounts(dataDir, "omezy"); got != 3 {
		t.Fatalf("expected omezy account count=3, got %d", got)
	}
	if got := CountSiteAccounts(dataDir, "funchatx"); got != 0 {
		t.Fatalf("expected funchatx account count=0, got %d", got)
	}

	// 2. Record a browser-captured account for "emeraldchat" (browser_setup site)
	rec, err := RecordBrowserSetupAccount(dataDir, "emeraldchat", "vip", model.LoginInfo{
		UserID:  "emerald_user_01",
		Profile: "vip",
		Backend: "rod",
		Cookies: []model.Cookie{{Name: "token", Value: "jwt_emerald_123"}},
	}, "")
	if err != nil {
		t.Fatalf("RecordBrowserSetupAccount failed: %v", err)
	}
	if rec.SiteID != "emeraldchat" || rec.AccountMode != AccountModeBrowserSetup {
		t.Fatalf("unexpected recorded browser account: %+v", rec)
	}
	if got := CountSiteAccounts(dataDir, "emeraldchat"); got != 1 {
		t.Fatalf("expected emeraldchat account count=1, got %d", got)
	}

	// 3. Run deep diagnostics on "omezy" (3 accounts, single sequence) -> should be PASS
	msgSettings := model.MessageSettings{
		ChatMode:  model.ChatModeSingle,
		Sequence:  "hey how are you\nadd my snap %username%",
		Usernames: "snap_sophia_21",
	}
	report, err := RunSiteDiagnostics(dataDir, "omezy", msgSettings, model.ProxySettings{Enabled: false})
	if err != nil {
		t.Fatalf("RunSiteDiagnostics failed: %v", err)
	}
	if report.OverallStatus != "PASS" || len(report.Checks) != 6 {
		t.Fatalf("expected PASS with 6 checks for omezy, got status=%s checks=%d (%+v)", report.OverallStatus, len(report.Checks), report.Checks)
	}

	// 4. Verify AppendSiteDebugLog redacts secrets and stays isolated per site
	if err := AppendSiteDebugLog(dataDir, "omezy", "dom", "INFO", "Authorization: Bearer secret_jwt_token_123", "phase=chatting"); err != nil {
		t.Fatalf("AppendSiteDebugLog failed: %v", err)
	}
	tail, err := ReadSiteDebugTail(dataDir, "omezy", 10)
	if err != nil || len(tail) < 2 {
		t.Fatalf("expected >=2 debug log entries in sites/omezy/logs/debug.jsonl, got %d (err=%v)", len(tail), err)
	}
	last := tail[len(tail)-1]
	if strings.Contains(last.Message, "secret_jwt_token_123") || !strings.Contains(last.Message, "[REDACTED]") {
		t.Fatalf("expected secret token redaction in debug log, got %q", last.Message)
	}
	if otherTail, _ := ReadSiteDebugTail(dataDir, "funchatx", 10); len(otherTail) != 0 {
		t.Fatalf("expected 0 debug entries in untouched site funchatx, got %d", len(otherTail))
	}

	// 5. Verify SaveSiteSession and LoadSiteSession strictly isolate sessions per site
	sessPath, err := SaveSiteSession(dataDir, "chatib", "default", model.LoginInfo{
		UserID:       "chatib_girl_21",
		LocalStorage: map[string]string{"guest_nick": "Sophia21"},
		Cookies:      []model.Cookie{{Name: "chatib_sid", Value: "sid_998877"}},
	})
	if err != nil {
		t.Fatalf("SaveSiteSession failed: %v", err)
	}
	if _, err := os.Stat(sessPath); err != nil {
		t.Fatalf("expected saved session file at %s: %v", sessPath, err)
	}
	loadedLogin, _, ok := LoadSiteSession(dataDir, "chatib", "default")
	if !ok || loadedLogin.LocalStorage["guest_nick"] != "Sophia21" {
		t.Fatalf("expected LoadSiteSession to return chatib session, got ok=%v login=%+v", ok, loadedLogin)
	}
	if _, _, okOther := LoadSiteSession(dataDir, "funchatx", "default"); okOther {
		t.Fatalf("expected untouched site funchatx to have no saved session")
	}
	chatibCfg, err := LoadSite(dataDir, "chatib")
	if err != nil || !chatibCfg.HasSavedSession {
		t.Fatalf("expected LoadSite(chatib).HasSavedSession=true, got %+v (err=%v)", chatibCfg, err)
	}
}

func TestMaterializeWorkspaceSitesAndMemoryIndex(t *testing.T) {
	// If running inside the repo (../../../run.bat exists), materialize
	// all 102 site folders into <repo>/sites/ and write <repo>/memory/sites_status.md.
	repoRoot := filepath.Clean(filepath.Join("..", "..", ".."))
	if _, err := os.Stat(filepath.Join(repoRoot, "run.bat")); err != nil {
		return
	}
	sitesRoot := filepath.Join(repoRoot, "sites")
	all := append([]Config{DefaultChitChatConfig()}, CatalogConfigs()...)
	all = append(all, DefaultTemplateConfig())
	for _, cfg := range all {
		isCC := cfg.ID == DefaultSiteID
		if err := ensureSiteFolder(sitesRoot, cfg, isCC); err != nil {
			t.Fatalf("failed seeding %s in repo sites/: %v", cfg.ID, err)
		}
		// Refresh README.txt, <site_id>_protection.txt, and non-ChitChat dom_driver.js
		siteDir := filepath.Join(sitesRoot, cfg.ID)
		_ = os.WriteFile(filepath.Join(siteDir, "README.txt"), []byte(siteReadmeText(cfg)), 0o644)
		_ = os.WriteFile(filepath.Join(siteDir, cfg.ID+"_protection.txt"), []byte(SiteProtectionText(cfg)), 0o644)
		if !isCC {
			_ = os.WriteFile(filepath.Join(siteDir, "dom_driver.js"), []byte(UniversalDOMDriverJS(cfg)), 0o644)
		}
	}

	memDir := filepath.Join(repoRoot, "memory")
	_ = os.MkdirAll(memDir, 0o755)
	var md strings.Builder
	md.WriteString("# Multi-Site Stranger Text-Chat Intelligence & Protection Registry (100 Non-ChitChat Sites + ChitChat)\n\n")
	md.WriteString("Every site listed below has its own isolated directory in `sites/<site_id>/` containing:\n")
	md.WriteString("- `site.json` (StartURL, Engine Family, Account Mode: `auto_register` or `browser_setup`, Selectors)\n")
	md.WriteString("- `dom_driver.js` (Isolated DOM Observer + Auto-Onboard Agent)\n")
	md.WriteString("- `<site_id>_protection.txt` (Site anti-bot protection & safe operation rules)\n")
	md.WriteString("- `accounts/` (Isolated auto-registered or browser-captured accounts for this site)\n")
	md.WriteString("- `profiles/default/` (Isolated browser session storage)\n\n")
	md.WriteString("| # | Site ID | Name | Family | Account Mode | Start URL | Onboarding Flow | Protection File |\n")
	md.WriteString("|---|---|---|---|---|---|---|---|\n")
	for i, c := range CatalogConfigs() {
		md.WriteString(fmt.Sprintf("| %d | `%s` | %s | `%s` | `%s` | `%s` | %s | `sites/%s/%s_protection.txt` |\n",
			i+1, c.ID, c.Name, c.Family, c.AccountMode, c.StartURL, c.EntryFlow, c.ID, c.ID))
	}
	if err := os.WriteFile(filepath.Join(memDir, "sites_status.md"), []byte(md.String()), 0o644); err != nil {
		t.Fatalf("failed writing memory/sites_status.md: %v", err)
	}
}
