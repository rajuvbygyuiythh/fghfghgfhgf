package sites

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const (
	DefaultSiteID           = "chitchat"
	TemplateSiteID          = "template_new_site"
	AccountModeAutoRegister = "auto_register"
	AccountModeBrowserSetup = "browser_setup"
)

// Selectors defines the CSS selectors used by the universal per-site DOM driver
// when automating any web chat site in Show Mode.
type Selectors struct {
	ComposerInput          string `json:"composer_input"`
	SendButton             string `json:"send_button,omitempty"`
	SkipButton             string `json:"skip_button"`
	StartButton            string `json:"start_button"`
	MessageHistory         string `json:"message_history"`
	MessageRow             string `json:"message_row"`
	MessageText            string `json:"message_text"`
	PartnerMessageSelector string `json:"partner_message_selector,omitempty"`
	OwnMessageSelector     string `json:"own_message_selector,omitempty"`
	PartnerName            string `json:"partner_name,omitempty"`
	ChatEndedText          string `json:"chat_ended_text,omitempty"`
}

// Config is the per-site configuration stored in sites/<site_id>/site.json.
// Each site lives in its own isolated folder so broken or changed sites never
// affect any other site or the shared reply engine.
type Config struct {
	ID                string    `json:"id"`
	Name              string    `json:"name"`
	Family            string    `json:"family,omitempty"`
	AccountMode       string    `json:"account_mode"`
	AccountCount      int       `json:"account_count,omitempty"`
	HasSavedSession   bool      `json:"has_saved_session,omitempty"`
	SessionSavedAt    string    `json:"session_saved_at,omitempty"`
	SessionPath       string    `json:"session_path,omitempty"`
	Enabled           bool      `json:"enabled"`
	StartURL          string    `json:"start_url"`
	AppURL            string    `json:"app_url"`
	APIBaseURL        string    `json:"api_base_url,omitempty"`
	AuthMode          string    `json:"auth_mode"`
	EntryFlow         string    `json:"entry_flow,omitempty"`
	ProtectionSummary string    `json:"protection_summary,omitempty"`
	Selectors         Selectors `json:"selectors"`
	FolderPath        string    `json:"folder_path,omitempty"`
}

var nonSlugChars = regexp.MustCompile(`[^a-z0-9_-]+`)

// SanitizeID converts a human site name or slug into a safe directory name.
func SanitizeID(raw string) string {
	s := strings.ToLower(strings.TrimSpace(raw))
	s = strings.ReplaceAll(s, " ", "_")
	s = nonSlugChars.ReplaceAllString(s, "")
	s = strings.Trim(s, "_-")
	if s == "" {
		return ""
	}
	return s
}

// NormalizeAccountMode normalizes any user or config string into either
// "browser_setup" or "auto_register".
func NormalizeAccountMode(mode, siteID, authMode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case AccountModeBrowserSetup, "browser", "manual", "first_browser":
		return AccountModeBrowserSetup
	case AccountModeAutoRegister, "auto", "guest", "browserless":
		return AccountModeAutoRegister
	default:
		return DefaultAccountMode(siteID, authMode)
	}
}

// DefaultChitChatConfig returns the isolated site config for ChitChat.gg.
func DefaultChitChatConfig() Config {
	return Config{
		ID:                DefaultSiteID,
		Name:              "ChitChat.gg",
		Family:            "chitchat_core",
		AccountMode:       AccountModeBrowserSetup,
		Enabled:           true,
		StartURL:          "https://app.chitchat.gg/text?auth=email",
		AppURL:            "https://app.chitchat.gg/",
		APIBaseURL:        "https://api.chitchat.gg",
		AuthMode:          "chitchat_jwt",
		EntryFlow:         "First browser run email/JWT login or anonymous guest text chat -> Start -> Skip (Esc)",
		ProtectionSummary: "AI moderation (Hive) + rate limiting on rapid links; keep natural reply delay (2s-5s).",
		Selectors: Selectors{
			ComposerInput:  `[data-chat-composer="true"] textarea[aria-label="Send a message"]`,
			SkipButton:     `[data-chat-composer="true"] button[aria-keyshortcuts="Escape"]`,
			StartButton:    `button`,
			MessageHistory: `ol[aria-label="Message history"]`,
			MessageRow:     `li[data-message-id]`,
			MessageText:    `p[data-from]`,
			PartnerName:    `[data-conversation-header] button[aria-label^="View @"]`,
		},
	}
}

// DefaultTemplateConfig returns a starter template config for adding any new site.
func DefaultTemplateConfig() Config {
	return Config{
		ID:                TemplateSiteID,
		Name:              "Template New Site (Example)",
		Family:            "custom",
		AccountMode:       AccountModeAutoRegister,
		Enabled:           true,
		StartURL:          "https://example.com/chat",
		AppURL:            "https://example.com/",
		APIBaseURL:        "",
		AuthMode:          "dom_only",
		EntryFlow:         "Customize site.json selectors or run capture/run.bat with skills/dom_auto_scout.js",
		ProtectionSummary: "Template site folder. Use skills/protection_detector.js to inspect target anti-bot rules.",
		Selectors:         ArchetypeSelectors("family_a"),
	}
}

// EnsureSites initializes the single canonical sites root for this data
// directory. Each site gets exactly one folder; there is no second copy under
// data/sites when a workspace sites/ folder is already present.
//
// Layout:
//
//	<dataDir>/_global/                 shared settings only
//	<sites-root>/<site_id>/            one folder per site
//	    site.json, dom_driver.js, README.txt, <id>_protection.txt
//	    accounts/, profiles/, sessions/, logs/
func EnsureSites(dataDir string) (string, error) {
	if strings.TrimSpace(dataDir) == "" {
		return "", fmt.Errorf("sites: empty data directory")
	}
	root := SitesRoot(filepath.Clean(dataDir))
	if err := os.MkdirAll(root, 0o700); err != nil {
		return "", err
	}
	if err := EnsureGlobalDirs(dataDir); err != nil {
		return "", err
	}
	if err := ensureSiteFolder(root, DefaultChitChatConfig(), true); err != nil {
		return "", err
	}
	for _, catCfg := range CatalogConfigs() {
		if err := ensureSiteFolder(root, catCfg, false); err != nil {
			return "", err
		}
	}
	if err := ensureSiteFolder(root, DefaultTemplateConfig(), false); err != nil {
		return "", err
	}
	if err := migrateLegacyChitChatData(dataDir); err != nil {
		return "", err
	}
	return root, nil
}

// ensureSiteFolder creates/repairs one site folder. It never writes outside
// the context root; every operation goes through SiteContext.
func ensureSiteFolder(root string, cfg Config, isChitChat bool) error {
	cfg.AccountMode = NormalizeAccountMode(cfg.AccountMode, cfg.ID, cfg.AuthMode)
	ctx, err := newSiteContextAtRoot(root, cfg.ID)
	if err != nil {
		return err
	}
	if err := ctx.Ensure(); err != nil {
		return err
	}
	cfg.FolderPath = ctx.FolderPath

	for _, rel := range []string{
		filepath.Join("profiles", "default", ".gitkeep"),
		filepath.Join("accounts", ".gitkeep"),
		filepath.Join("sessions", ".gitkeep"),
		filepath.Join("logs", ".gitkeep"),
	} {
		if _, statErr := ctx.Stat(rel); errors.Is(statErr, os.ErrNotExist) {
			_ = ctx.WriteFile(rel, nil, 0o644)
		}
	}

	cfgPath := "site.json"
	if _, statErr := ctx.Stat(cfgPath); errors.Is(statErr, os.ErrNotExist) {
		if err := ctx.SaveJSON(cfgPath, cfg); err != nil {
			return err
		}
	} else {
		var existing Config
		if ok, loadErr := ctx.LoadJSON(cfgPath, &existing); loadErr == nil && ok {
			normalized := NormalizeAccountMode(existing.AccountMode, cfg.ID, existing.AuthMode)
			if existing.AccountMode != normalized {
				existing.AccountMode = normalized
				if err := ctx.SaveJSON(cfgPath, existing); err != nil {
					return err
				}
			}
		}
	}

	if _, statErr := ctx.Stat("README.txt"); errors.Is(statErr, os.ErrNotExist) {
		if err := ctx.WriteFile("README.txt", []byte(siteReadmeText(cfg)), 0o644); err != nil {
			return err
		}
	}
	protectionName := cfg.ID + "_protection.txt"
	if _, statErr := ctx.Stat(protectionName); errors.Is(statErr, os.ErrNotExist) {
		if err := ctx.WriteFile(protectionName, []byte(SiteProtectionText(cfg)), 0o644); err != nil {
			return err
		}
	}
	if _, statErr := ctx.Stat("dom_driver.js"); errors.Is(statErr, os.ErrNotExist) {
		js := UniversalDOMDriverJS(cfg)
		if isChitChat {
			js = "// ChitChat.gg uses the built-in verified DOM observer in internal/browser/showmode_dom.js\n// Override here only if ChitChat changes its page structure.\n"
		}
		if err := ctx.WriteFile("dom_driver.js", []byte(js), 0o644); err != nil {
			return err
		}
	}
	return nil
}

// migrateLegacyChitChatData performs a one-time move of pre-isolation global
// chitchat runtime data into sites/<id>/. It never copies into another site
// folder and never overwrites an existing destination.
func migrateLegacyChitChatData(dataDir string) error {
	ctx, err := NewSiteContext(dataDir, DefaultSiteID)
	if err != nil {
		return err
	}
	if err := ctx.Ensure(); err != nil {
		return err
	}
	if err := mergeLegacyDir(filepath.Join(dataDir, "profiles"), ctx.ProfilesPath); err != nil {
		return err
	}
	// Older builds kept login_info_*.json directly in data/.
	legacyLogins, _ := filepath.Glob(filepath.Join(dataDir, "login_info_*.json"))
	for _, path := range legacyLogins {
		target := filepath.Join(ctx.ProfilesPath, "default", filepath.Base(path))
		if _, statErr := os.Lstat(target); statErr == nil {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return err
		}
		if err := os.Rename(path, target); err != nil {
			if copyErr := copyLegacyFile(path, target); copyErr != nil {
				return copyErr
			}
			_ = os.Remove(path)
		}
	}
	for _, name := range []string{"tmp", "fresh-runs"} {
		if err := mergeLegacyDir(filepath.Join(dataDir, name), filepath.Join(ctx.FolderPath, name)); err != nil {
			return err
		}
	}
	legacyRegistry := filepath.Join(dataDir, "bot_registry.json")
	if _, statErr := os.Lstat(legacyRegistry); statErr == nil {
		target := filepath.Join(ctx.FolderPath, "bot_registry.json")
		if _, destErr := os.Lstat(target); errors.Is(destErr, os.ErrNotExist) {
			if err := os.Rename(legacyRegistry, target); err != nil {
				if copyErr := copyLegacyFile(legacyRegistry, target); copyErr != nil {
					return copyErr
				}
				_ = os.Remove(legacyRegistry)
			}
		}
	}
	return nil
}

func mergeLegacyDir(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if err := os.MkdirAll(dst, 0o700); err != nil {
		return err
	}
	for _, entry := range entries {
		from := filepath.Join(src, entry.Name())
		to := filepath.Join(dst, entry.Name())
		if info, statErr := os.Lstat(to); statErr == nil {
			if entry.IsDir() && info.IsDir() {
				if err := mergeLegacyDir(from, to); err != nil {
					return err
				}
				continue
			}
			continue // never overwrite an existing site-local file
		}
		if err := os.Rename(from, to); err != nil {
			if entry.IsDir() {
				if copyErr := CopyTree(from, to); copyErr != nil {
					return copyErr
				}
				_ = os.RemoveAll(from)
			} else {
				if copyErr := copyLegacyFile(from, to); copyErr != nil {
					return copyErr
				}
				_ = os.Remove(from)
			}
		}
	}
	_ = os.Remove(src) // succeeds only when the legacy directory is empty
	return nil
}

func copyLegacyFile(src, dst string) error {
	raw, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return err
	}
	return os.WriteFile(dst, raw, 0o600)
}

// List returns all isolated site configurations sorted with chitchat first.
func List(dataDir string) ([]Config, error) {
	root, err := EnsureSites(dataDir)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	out := make([]Config, 0, len(entries))
	seen := map[string]bool{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		id, idErr := validateSiteID(entry.Name())
		if idErr != nil {
			continue
		}
		cfg, loadErr := loadSiteAtRoot(root, dataDir, id)
		if loadErr != nil {
			continue
		}
		seen[cfg.ID] = true
		out = append(out, cfg)
	}
	if !seen[DefaultSiteID] {
		def := DefaultChitChatConfig()
		if ctx, ctxErr := newSiteContextAtRoot(root, DefaultSiteID); ctxErr == nil {
			def.FolderPath = ctx.FolderPath
		}
		out = append(out, def)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ID == DefaultSiteID {
			return true
		}
		if out[j].ID == DefaultSiteID {
			return false
		}
		if out[i].ID == TemplateSiteID {
			return false
		}
		if out[j].ID == TemplateSiteID {
			return true
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}

// SiteDir returns the one canonical directory for a site. Invalid IDs return
// an empty string; callers should use NewSiteContext when they need an error.
func SiteDir(dataDir, siteID string) string {
	id, err := validateSiteID(siteID)
	if err != nil {
		return ""
	}
	return filepath.Join(SitesRoot(dataDir), id)
}

// DataDirForSite returns the isolated data root for a site. Since Phase 1 every
// site (including chitchat) has its own folder; the legacy shared dataDir is
// never returned.
func DataDirForSite(dataDir, siteID string) string {
	return SiteDir(dataDir, siteID)
}

// LoadSite reads a single site configuration by ID. It is deliberately
// read-only: a dashboard refresh or an active-site lookup must never create or
// repair another site's folder. EnsureSites performs that repair once, at
// startup.
func LoadSite(dataDir, siteID string) (Config, error) {
	cleanData := filepath.Clean(strings.TrimSpace(dataDir))
	if cleanData == "" || cleanData == "." {
		return Config{}, fmt.Errorf("%w: empty data directory", ErrUnsafePath)
	}
	id, err := validateSiteID(siteID)
	if err != nil {
		return Config{}, fmt.Errorf("%w: %q", err, siteID)
	}
	return loadSiteAtRoot(SitesRoot(cleanData), cleanData, id)
}

func loadSiteAtRoot(root, dataDir, id string) (Config, error) {
	ctx, err := newSiteContextAtRoot(root, id)
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	ok, err := ctx.LoadJSON("site.json", &cfg)
	if err != nil {
		return Config{}, err
	}
	if !ok {
		if id == DefaultSiteID {
			cfg = DefaultChitChatConfig()
		} else if id == TemplateSiteID {
			cfg = DefaultTemplateConfig()
		} else if catCfg, found := CatalogByID(id); found {
			cfg = catCfg
		} else {
			return Config{}, fmt.Errorf("site %q not found in sites/%s", id, id)
		}
	}
	cfg.ID = id
	if strings.TrimSpace(cfg.Name) == "" {
		cfg.Name = id
	}
	if strings.TrimSpace(cfg.StartURL) == "" {
		cfg.StartURL = "https://app.chitchat.gg/text?auth=email"
	}
	if strings.TrimSpace(cfg.AppURL) == "" {
		cfg.AppURL = cfg.StartURL
	}
	if strings.TrimSpace(cfg.AuthMode) == "" {
		if id == DefaultSiteID {
			cfg.AuthMode = "chitchat_jwt"
		} else {
			cfg.AuthMode = "dom_only"
		}
	}
	cfg.AccountMode = NormalizeAccountMode(cfg.AccountMode, cfg.ID, cfg.AuthMode)
	cfg.AccountCount = CountSiteAccounts(dataDir, id)
	cfg.HasSavedSession, cfg.SessionSavedAt, cfg.SessionPath = SiteSessionStatus(dataDir, id)
	cfg.FolderPath = ctx.FolderPath
	return cfg, nil
}

// CreateSite creates a brand-new isolated site folder under sites/<site_id>/
// with its own site.json, dom_driver.js, <site_id>_protection.txt, README.txt,
// accounts/ directory, and profiles/default directory.
func CreateSite(dataDir, name, startURL string) (Config, error) {
	return CreateSiteWithMode(dataDir, name, startURL, AccountModeAutoRegister)
}

// CreateSiteWithMode creates a brand-new isolated site folder under sites/<site_id>/
// with an explicit AccountMode ("auto_register" or "browser_setup").
func CreateSiteWithMode(dataDir, name, startURL, accountMode string) (Config, error) {
	name = strings.TrimSpace(name)
	startURL = strings.TrimSpace(startURL)
	if name == "" {
		return Config{}, errors.New("site name is required")
	}
	id := SanitizeID(name)
	if id == "" {
		return Config{}, errors.New("site name must contain letters or numbers")
	}
	if startURL == "" {
		return Config{}, errors.New("site start URL is required (e.g. https://example.com/chat)")
	}
	if !strings.HasPrefix(strings.ToLower(startURL), "http://") && !strings.HasPrefix(strings.ToLower(startURL), "https://") {
		startURL = "https://" + startURL
	}

	tmpl := DefaultTemplateConfig()
	accMode := NormalizeAccountMode(accountMode, id, "dom_only")
	cfg := Config{
		ID:                id,
		Name:              name,
		Family:            "custom",
		AccountMode:       accMode,
		Enabled:           true,
		StartURL:          startURL,
		AppURL:            startURL,
		APIBaseURL:        "",
		AuthMode:          "dom_only",
		EntryFlow:         "Auto-onboard (18+ / Female / Guest Nickname) -> Start Chat",
		ProtectionSummary: "Custom site created via dashboard. Inspect with skills/protection_detector.js if needed.",
		Selectors:         tmpl.Selectors,
	}

	root, err := EnsureSites(dataDir)
	if err != nil {
		return Config{}, err
	}
	if err := writeSiteFiles(root, cfg); err != nil {
		return Config{}, err
	}
	ctx, err := newSiteContextAtRoot(root, id)
	if err != nil {
		return Config{}, err
	}
	cfg.FolderPath = ctx.FolderPath
	return cfg, nil
}

func writeSiteFiles(root string, cfg Config) error {
	cfg.AccountMode = NormalizeAccountMode(cfg.AccountMode, cfg.ID, cfg.AuthMode)
	ctx, err := newSiteContextAtRoot(root, cfg.ID)
	if err != nil {
		return err
	}
	if err := ctx.Ensure(); err != nil {
		return err
	}
	cfg.FolderPath = ctx.FolderPath
	for _, rel := range []string{
		filepath.Join("profiles", "default", ".gitkeep"),
		filepath.Join("accounts", ".gitkeep"),
		filepath.Join("sessions", ".gitkeep"),
		filepath.Join("logs", ".gitkeep"),
	} {
		if err := ctx.WriteFile(rel, nil, 0o644); err != nil {
			return err
		}
	}
	if err := ctx.SaveJSON("site.json", cfg); err != nil {
		return err
	}
	if err := ctx.WriteFile("README.txt", []byte(siteReadmeText(cfg)), 0o644); err != nil {
		return err
	}
	if err := ctx.WriteFile(cfg.ID+"_protection.txt", []byte(SiteProtectionText(cfg)), 0o644); err != nil {
		return err
	}
	if _, statErr := ctx.Stat("dom_driver.js"); errors.Is(statErr, os.ErrNotExist) {
		if err := ctx.WriteFile("dom_driver.js", []byte(UniversalDOMDriverJS(cfg)), 0o644); err != nil {
			return err
		}
	}
	return nil
}

// CustomDOMDriverJS returns the custom JS driver for a site if it is non-ChitChat
// or if the user customized sites/<site_id>/dom_driver.js. Returns "" to use the
// built-in verified ChitChat DOM observer.
func CustomDOMDriverJS(dataDir, siteID string) string {
	id, err := validateSiteID(siteID)
	if err != nil || id == DefaultSiteID {
		return ""
	}
	cfg, err := LoadSite(dataDir, id)
	if err != nil {
		return ""
	}
	ctx, err := NewSiteContext(dataDir, id)
	if err != nil {
		return ""
	}
	raw, err := ctx.ReadFile("dom_driver.js")
	if err == nil {
		trimmed := strings.TrimSpace(string(raw))
		if trimmed != "" && strings.Contains(trimmed, "snapshot") {
			return string(raw)
		}
	}
	return UniversalDOMDriverJS(cfg)
}

// UniversalDOMDriverJS generates a self-contained Show Mode DOM driver + Auto-Onboard
// Agent for any site based on its site.json Selectors and Family archetype.
func UniversalDOMDriverJS(cfg Config) string {
	selJSON, _ := json.Marshal(cfg.Selectors)
	family := cfg.Family
	if family == "" {
		family = "family_a"
	}
	return fmt.Sprintf(`// Isolated DOM Driver + Auto-Onboard Agent for site: %s (%s | %s)
// Edit selectors in site.json or customize this script directly.
// One site breaking or changing layout will never affect any other site.
const __sm = window.__chitchatUniversalSiteDOM || (() => {
  const CFG = %s;
  const FAMILY = %q;
  const ALIASES = ['Sophia', 'Emma', 'Olivia', 'Mia', 'Chloe', 'Ava', 'Lily', 'Zoe', 'Hannah', 'Maya'];
  const clean = s => (s || '').replace(/\s+/g, ' ').trim();
  const visible = el => {
    if (!el || el.nodeType !== 1 || el.closest('[hidden], [aria-hidden="true"]')) return false;
    const st = getComputedStyle(el), r = el.getBoundingClientRect();
    return st.display !== 'none' && st.visibility !== 'hidden' && r.width > 0 && r.height > 0;
  };
  const usable = el => visible(el) && !el.disabled && el.getAttribute('aria-disabled') !== 'true' && !el.readOnly;
  const findFirst = (sel, pred = visible) => {
    if (!sel) return null;
    try { return [...document.querySelectorAll(sel)].find(pred) || null; } catch (_) { return null; }
  };
  const setNativeValue = (inp, val) => {
    try {
      const desc = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, 'value');
      if (desc && desc.set) desc.set.call(inp, val);
      else inp.value = val;
      inp.dispatchEvent(new Event('input', { bubbles: true }));
      inp.dispatchEvent(new Event('change', { bubbles: true }));
    } catch (_) {}
  };
  const state = {
    document: String(Date.now()),
    generation: 1,
    phase: 'matching',
    rows: new Map(),
    nextOrder: 0,
    revision: 0,
    lastOnboardAt: 0
  };
  function autoOnboardIfNeeded(hasComposer) {
    const now = Date.now();
    if (hasComposer || now - state.lastOnboardAt < 1800) return;
    state.lastOnboardAt = now;
    try {
      // 1. Tick 18+ / terms / age checkboxes
      [...document.querySelectorAll('input[type="checkbox"]')].filter(visible).forEach(cb => {
        if (!cb.checked) cb.click();
      });
      // 2. Fill empty guest nickname / handle input (never touch composer)
      let savedAlias = '';
      try { savedAlias = localStorage.getItem('chitchat_guest_nickname') || localStorage.getItem('guest_nick') || ''; } catch (_) {}
      [...document.querySelectorAll('input[type="text"], input:not([type])')].filter(usable).forEach(inp => {
        if (inp.getAttribute('data-sm-composer') === 'true' || (inp.value && inp.value.trim() !== '')) return;
        const hint = ((inp.placeholder || '') + ' ' + (inp.name || '') + ' ' + (inp.id || '') + ' ' + (inp.getAttribute('aria-label') || '')).toLowerCase();
        if (/(nick|name|handle|alias|username|your name)/i.test(hint) && !/(message|chat|type a|write|send|search)/i.test(hint)) {
          const chosen = savedAlias || (ALIASES[Math.floor(Math.random() * ALIASES.length)] + String(19 + Math.floor(Math.random() * 6)));
          setNativeValue(inp, chosen);
        }
      });
      // 3. Fill age input/select if visible
      let savedAge = '21';
      try { savedAge = localStorage.getItem('chitchat_guest_age') || '21'; } catch (_) {}
      [...document.querySelectorAll('input[type="number"], select')].filter(usable).forEach(el => {
        const hint = ((el.name || '') + ' ' + (el.id || '') + ' ' + (el.placeholder || '') + ' ' + (el.getAttribute('aria-label') || '')).toLowerCase();
        if (/\bage\b/i.test(hint)) {
          if (el.tagName.toLowerCase() === 'select') {
            const opt = [...(el.options || [])].find(o => clean(o.value || o.text) === savedAge || /^(20|21|22)$/.test(clean(o.value || o.text)));
            if (opt && el.value !== opt.value) { el.value = opt.value; el.dispatchEvent(new Event('change', { bubbles: true })); }
          } else if (!el.value) {
            setNativeValue(el, savedAge);
          }
        }
      });
      // 4. Select Female / Woman / Girl option
      const clickables = [...document.querySelectorAll('button, label, a, [role="button"], [role="radio"]')].filter(visible);
      clickables.forEach(el => {
        const txt = clean(el.innerText || el.textContent);
        if (/^(♀\s*female|female|woman|i'm a girl|i am a girl|girl)$/i.test(txt)) {
          if (!el.classList.contains('active') && !el.classList.contains('selected') && el.getAttribute('aria-pressed') !== 'true' && el.getAttribute('aria-checked') !== 'true') {
            el.click();
          }
        }
      });
      // 5. Click 18+ / Cookie / Guest / Text-mode modal buttons
      const modalBtn = clickables.find(el => {
        const txt = clean(el.innerText || el.textContent);
        return /^(yes,?\s*i'?m 18( or older)?|i am 18( or older)?|agree|i agree|accept all|chat anonymously|continue as guest|guest login|text chat|text only|yes,?\s*skip)$/i.test(txt);
      });
      if (modalBtn) modalBtn.click();
    } catch (_) {}
  }
  function tagActiveComposer(composer) {
    try {
      [...document.querySelectorAll('[data-sm-composer="true"]')].forEach(el => {
        if (el !== composer) el.removeAttribute('data-sm-composer');
      });
      if (composer) composer.setAttribute('data-sm-composer', 'true');
    } catch (_) {}
  }
  function classifyAuthor(el, rawText) {
    if (CFG.own_message_selector) {
      try { if (el.matches(CFG.own_message_selector) || el.querySelector(CFG.own_message_selector)) return 'self'; } catch (_) {}
    }
    if (CFG.partner_message_selector) {
      try { if (el.matches(CFG.partner_message_selector) || el.querySelector(CFG.partner_message_selector)) return 'partner'; } catch (_) {}
    }
    const cls = ((el.className || '') + ' ' + (el.id || '') + ' ' + (el.getAttribute('data-author') || '') + ' ' + (el.getAttribute('data-from') || '')).toLowerCase();
    if (/\b(youmsg|self-msg|outgoing|self|own|mine|sent|from-me|by-me|right)\b/.test(cls)) return 'self';
    if (/\b(strangermsg|partner-msg|incoming|partner|stranger|them|other|received|from-them|left)\b/.test(cls)) return 'partner';
    if (/^(you|me)\s*:/i.test(rawText)) return 'self';
    return 'partner';
  }
  function stripSpeakerPrefix(text) {
    return clean(text.replace(/^(stranger|partner|you|me|guest[0-9]*)\s*:\s*/i, ''));
  }
  function detectProtectionState(bodyText) {
    try {
      if (document.querySelector('iframe[src*="challenges.cloudflare.com"], .cf-turnstile, [data-sitekey][class*="turnstile"]')) return 'cloudflare_turnstile';
      if (document.querySelector('iframe[src*="hcaptcha.com"], .h-captcha')) return 'hcaptcha';
      if (document.querySelector('iframe[src*="google.com/recaptcha"], .g-recaptcha')) return 'recaptcha';
      if (/\b(vpn detected|proxy detected|disable your vpn)\b/i.test(bodyText)) return 'vpn_blocked';
      if (/\b(another tab opened|multiple tabs|already open in another tab)\b/i.test(bodyText)) return 'single_tab_lock';
      if (/\b(too many requests|rate limit|daily limit|slow down)\b/i.test(bodyText)) return 'rate_limited';
    } catch (_) {}
    return 'clean';
  }
  function scan() {
    let composer = findFirst(CFG.composer_input, usable);
    autoOnboardIfNeeded(!!composer);
    if (!composer) composer = findFirst(CFG.composer_input, usable);
    tagActiveComposer(composer);
    const skipBtn = findFirst(CFG.skip_button, visible);
    const startBtn = findFirst(CFG.start_button, usable);
    const endedRe = CFG.chat_ended_text ? new RegExp(CFG.chat_ended_text, 'i') : null;
    const bodyText = document.body ? document.body.innerText : '';
    const protection = detectProtectionState(bodyText);
    const chatEnded = (endedRe && endedRe.test(bodyText)) || (!composer && !!startBtn);
    const phase = chatEnded ? 'lobby' : (composer ? 'chatting' : 'matching');
    if (phase !== state.phase && phase === 'chatting') {
      state.generation++;
      state.rows.clear();
      state.nextOrder = 0;
    }
    state.phase = phase;
    const history = findFirst(CFG.message_history, () => true) || document.body;
    let elements = [];
    try {
      elements = history && CFG.message_row ? [...history.querySelectorAll(CFG.message_row)] : [];
    } catch (_) {}
    elements.forEach((el, idx) => {
      const id = el.getAttribute('data-message-id') || el.id || ('row-' + idx);
      let textEl = el;
      if (CFG.message_text) {
        try { textEl = el.querySelector(CFG.message_text) || el; } catch (_) {}
      }
      const rawText = clean(textEl.textContent);
      if (!rawText) return;
      if (/^(stranger is typing|looking for a|connecting to|connected! say hi|chat ended|stranger has disconnected)/i.test(rawText)) return;
      const author = classifyAuthor(el, rawText);
      const text = stripSpeakerPrefix(rawText);
      if (!text) return;
      const old = state.rows.get(id);
      if (!old || old.text !== text) {
        state.rows.set(id, { id, order: old ? old.order : ++state.nextOrder, author, text });
        state.revision++;
      }
    });
    const partnerEl = findFirst(CFG.partner_name, () => true);
    const partner = clean(partnerEl ? partnerEl.textContent : 'stranger') || 'stranger';
    const buttons = [];
    if (chatEnded && startBtn) buttons.push('START');
    else if (skipBtn) buttons.push('SKIP');
    return {
      controls: {
        hasInput: !!composer,
        inputDesc: composer ? 'composer' : '',
        buttons,
        chatEnded: phase !== 'chatting',
        partner,
        ownName: 'self',
        conversation: state.document + ':' + state.generation,
        phase,
        protection,
        canSend: phase === 'chatting' && !!composer,
        observerReady: true,
        overflow: false
      },
      messages: [...state.rows.values()],
      revision: state.revision
    };
  }
  return {
    snapshot: () => scan(),
    probe: () => {
      const s = scan();
      return 'family=' + FAMILY + ' phase=' + s.controls.phase + ' input=' + s.controls.hasInput + ' msgs=' + s.messages.length + ' prot=' + (s.controls.protection || 'clean');
    },
    click: (labels) => {
      const want = (labels || []).map(l => String(l).toUpperCase());
      if (want.includes('SEND')) {
        const sb = findFirst(CFG.send_button, usable);
        if (sb) { sb.click(); return 'SEND'; }
      }
      if (want.includes('START')) {
        const b = findFirst(CFG.start_button, usable);
        if (b) { b.click(); return 'START'; }
      }
      if (want.includes('SKIP') || want.includes('CONFIRM?')) {
        const b = findFirst(CFG.skip_button, usable);
        if (b) { b.click(); return 'SKIP'; }
      }
      return '';
    },
    focusInput: () => {
      const c = findFirst(CFG.composer_input, usable);
      if (!c) return false;
      tagActiveComposer(c);
      c.focus();
      return document.activeElement === c;
    },
    readInput: () => {
      const c = findFirst(CFG.composer_input, usable);
      return c ? String(c.value || '') : '';
    },
    clearInput: () => {
      const c = findFirst(CFG.composer_input, usable);
      if (!c) return false;
      c.value = '';
      c.dispatchEvent(new Event('input', { bubbles: true }));
      return true;
    }
  };
})();
`, cfg.Name, cfg.ID, family, string(selJSON), family)
}

// SiteProtectionText returns the detailed <site_id>_protection.txt intelligence
// note stored inside sites/<site_id>/<site_id>_protection.txt for future reference.
func SiteProtectionText(cfg Config) string {
	family := cfg.Family
	if family == "" {
		family = "family_a"
	}
	accMode := NormalizeAccountMode(cfg.AccountMode, cfg.ID, cfg.AuthMode)
	entryFlow := cfg.EntryFlow
	if entryFlow == "" {
		entryFlow = "Anonymous Guest -> Auto 18+ / Female / Nickname -> Start Chat"
	}
	prot := cfg.ProtectionSummary
	if prot == "" {
		prot = "Standard WebSocket/DOM rate-limiting. Keep reply delay >= 2.0s and avoid raw URL spam on opener."
	}
	return fmt.Sprintf(`====================================================================
SITE PROTECTION & INTELLIGENCE NOTE: %s (%s)
====================================================================
Site ID            : %s
Site Name          : %s
Engine Family      : %s
Account Setup Mode : %s
Target Start URL   : %s
Auth Mode          : %s

1. ONBOARDING & GUEST ENTRY FLOW
--------------------------------------------------------------------
%s
- Auto-Onboard Agent in dom_driver.js automatically ticks 18+ checkboxes,
  selects Female/Woman/Girl, fills a natural female guest alias + age 21
  if prompted, and clicks Start / New Chat.

2. ANTI-BOT & PROTECTION DETAILS
--------------------------------------------------------------------
%s

3. SAFE BOT OPERATION RULES (FOR FUTURE WORK)
--------------------------------------------------------------------
- Recommended Reply Delay : 2.0s – 5.0s (configured in Dashboard Show Mode).
- Username / Snap Sharing : Share only on later turns or when asked (EVAChat /
                            API Chat funnel handles this automatically).
- Selector Auto-Recovery  : If this site updates its HTML layout, run
                            source_code/capture/run.bat or skills/dom_auto_scout.js
                            to refresh selectors in sites/%s/site.json.
- Isolated Accounts Path  : sites/%s/accounts/
- Isolated Profile Path   : sites/%s/profiles/default/
====================================================================
`, cfg.Name, cfg.ID, cfg.ID, cfg.Name, family, accMode, cfg.StartURL, cfg.AuthMode, entryFlow, prot, cfg.ID, cfg.ID, cfg.ID)
}

func siteReadmeText(cfg Config) string {
	accMode := NormalizeAccountMode(cfg.AccountMode, cfg.ID, cfg.AuthMode)
	return fmt.Sprintf(`====================================================================
ISOLATED SITE FOLDER: %s (%s)
====================================================================

This folder is 100%% isolated from other sites and from the core bot engine.
- Account Setup Mode: %s
- Bot reply rules (Single Sequence, Folder Sequences, EVAChat Flow, API Chat),
  apichat.exe Persona & API Key, Timing, Proxy, and Usernames are shared
  automatically from the main engine.
- Only this site's URL, DOM selectors, protection notes, accounts, and browser sessions live here.

FILES IN THIS FOLDER:
1. site.json              -> Site Name, Start URL, Family Archetype, Account Mode,
                             and CSS selectors for chat input, skip/start buttons, and messages.
2. dom_driver.js          -> Isolated browser DOM script + Auto-Onboard Agent for this site.
3. %s_protection.txt -> Anti-bot protection notes, entry flow, and safe pacing rules.
4. accounts/              -> Auto-registered or browser-captured account JSON files for this site.
5. profiles/              -> Saved browser sessions and login_info_*.json states for this site only.
`, cfg.Name, cfg.ID, accMode, cfg.ID)
}
