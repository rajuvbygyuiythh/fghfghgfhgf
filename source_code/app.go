package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"chitchatbot/internal/applog"
	"chitchatbot/internal/browser"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"chitchatbot/internal/bot"
	"chitchatbot/internal/memory"
	"chitchatbot/internal/model"
	"chitchatbot/internal/proxy"
	"chitchatbot/internal/runner"
	"chitchatbot/internal/settings"
	"chitchatbot/internal/sites"
	"chitchatbot/internal/store"
	"chitchatbot/internal/uistore"
	"chitchatbot/internal/workerplan"
)

type App struct {
	ctx             context.Context
	dataDir         string
	uiStore         *uistore.Store
	globalSettings  *settings.Store
	supervisor      *runner.Supervisor
	mu              sync.Mutex
	localTestMu     sync.Mutex
	localTest       *bot.LocalChatSession
	chatFeed        *homeChatFeed
	showLog         *showLogFeed
	botLog          *botLogFeed
	combinedLog     *combinedLogFeed
	showModeRunning bool
	running         bool
	runID           uint64
	botCancel       context.CancelFunc
	activeWorkers   map[string]bool
}

func NewApp(dataDir string) (*App, error) {
	uiStore, err := uistore.Open(dataDir)
	if err != nil {
		return nil, err
	}

	app := &App{
		dataDir:       dataDir,
		uiStore:       uiStore,
		activeWorkers: map[string]bool{},
	}

	if err := app.seedDefaults(); err != nil {
		_ = uiStore.Close()
		return nil, err
	}
	// The sites tree must exist before the settings store resolves its root.
	_, _ = sites.EnsureSites(dataDir)
	settingsStore, err := settings.Open(dataDir)
	if err != nil {
		_ = uiStore.Close()
		return nil, err
	}
	app.globalSettings = settingsStore
	if err := app.seedGlobalSettings(); err != nil {
		_ = uiStore.Close()
		return nil, err
	}
	_, _ = uiStore.LoadMessageSettings()

	return app, nil
}

// settingsStore returns the shared settings store, opening it lazily when a
// test constructs App without NewApp.
func (a *App) settingsStore() *settings.Store {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.globalSettings != nil {
		return a.globalSettings
	}
	if strings.TrimSpace(a.dataDir) == "" {
		return nil
	}
	st, err := settings.Open(a.dataDir)
	if err != nil {
		return nil
	}
	a.globalSettings = st
	return st
}

// runnerSupervisor returns the isolated multi-site runner Supervisor, creating it
// lazily under a.mu so tests constructing App without NewApp work safely.
func (a *App) runnerSupervisor() *runner.Supervisor {
	st := a.settingsStore()
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.supervisor != nil {
		return a.supervisor
	}
	a.supervisor = runner.NewSupervisor(runner.Options{
		DataDir:       a.dataDir,
		SettingsStore: st,
		NotifyFunc: func(n runner.Notification) {
			a.log(n.Level, fmt.Sprintf("[supervisor:%s] %s: %s", n.SiteID, n.Event, n.Reason))
			kind := "system"
			if strings.EqualFold(n.Level, "error") || strings.Contains(strings.ToLower(n.Event), "error") || strings.Contains(strings.ToLower(n.Event), "fail") {
				kind = "error"
			}
			a.recordCombinedLog(n.SiteID, kind, fmt.Sprintf("%s — %s", n.Event, n.Reason), "")
			a.emit("ui:runner-notification", n)
		},
	})
	return a.supervisor
}

// globalSettingsFromUIStore builds a settings.Settings snapshot from the values
// currently stored in uistore.
func (a *App) globalSettingsFromUIStore() (settings.Settings, error) {
	seed := settings.Default()
	if a.uiStore == nil {
		return seed, nil
	}
	message, msgErr := a.uiStore.LoadMessageSettings()
	proxySettings, proxyErr := a.uiStore.LoadProxySettings()
	general, genErr := a.uiStore.LoadGeneralSettings()
	for _, err := range []error{msgErr, proxyErr, genErr} {
		if err != nil {
			return seed, err
		}
	}
	if mode := settings.NormalizeMessageMode(message.EffectiveChatMode()); mode != "" {
		seed.MessageMode = mode
	}
	seed.Proxy = proxySettings
	seed.ReplyDelay = settings.DurationRange{
		MinSeconds: general.ReplyDelayMinSeconds,
		MaxSeconds: general.ReplyDelayMaxSeconds,
	}
	seed.SkipTimeout = settings.DurationRange{
		MinSeconds: general.MatchReplyTimeoutMinSeconds,
		MaxSeconds: general.MatchReplyTimeoutMaxSeconds,
	}
	seed.FirstMessage = general.FirstMessage.Effective()
	seed.API = settings.APISettings{
		Enabled:     message.AIEnabled || strings.TrimSpace(message.AIAPIKey) != "",
		BaseURL:     message.AIBaseURL,
		APIKey:      message.AIAPIKey,
		Model:       message.AIModel,
		Temperature: message.APITemperature,
	}
	return seed.Normalize(), nil
}

// loadOrSeedGlobalSettings reads sites/_global/settings.json when present and
// falls back to the current uistore values before the first global save.
// It also syncs API settings saved from apichat.exe (shared mirror) into
// sites/_global/settings.json (Prompt 15).
func (a *App) loadOrSeedGlobalSettings() (settings.Settings, error) {
	st := a.settingsStore()
	if st != nil {
		if _, err := os.Stat(st.GlobalPath()); err == nil {
			loaded, loadErr := st.Load()
			if loadErr == nil && a.dataDir != "" {
				if sharedMsg, mErr := uistore.LoadMessageSettingsMirror(a.dataDir); mErr == nil {
					changed := false
					if strings.TrimSpace(loaded.API.APIKey) == "" && strings.TrimSpace(sharedMsg.AIAPIKey) != "" {
						loaded.API.APIKey = strings.TrimSpace(sharedMsg.AIAPIKey)
						loaded.API.Enabled = true
						changed = true
					}
					if strings.TrimSpace(loaded.API.BaseURL) == "" && strings.TrimSpace(sharedMsg.AIBaseURL) != "" {
						loaded.API.BaseURL = strings.TrimSpace(sharedMsg.AIBaseURL)
						changed = true
					}
					if strings.TrimSpace(loaded.API.Model) == "" && strings.TrimSpace(sharedMsg.AIModel) != "" {
						loaded.API.Model = strings.TrimSpace(sharedMsg.AIModel)
						changed = true
					}
					if changed {
						_, _ = st.Save(loaded)
					}
				}
			}
			return loaded, loadErr
		}
	}
	return a.globalSettingsFromUIStore()
}

// seedGlobalSettings creates sites/_global/settings.json on first start from the
// values the dashboard already stores, so an upgraded installation keeps exactly
// the behaviour it had before global settings existed.
func (a *App) seedGlobalSettings() error {
	st := a.settingsStore()
	if st == nil {
		return nil
	}
	if _, err := os.Stat(st.GlobalPath()); err == nil {
		return nil
	}
	seed, err := a.globalSettingsFromUIStore()
	if err != nil {
		return err
	}
	if _, err := st.Save(seed); err != nil {
		return err
	}
	a.log("info", "Global settings seeded from the dashboard defaults: "+st.GlobalPath())
	return nil
}

// effectiveSiteSettings merges the shared settings with one site's override.
// Every read that feeds a worker must go through this.
func (a *App) effectiveSiteSettings(siteID string) (settings.Effective, error) {
	if strings.TrimSpace(siteID) == "" {
		siteID = a.activeSiteID()
	}
	global, err := a.loadOrSeedGlobalSettings()
	if err != nil {
		return settings.Effective{}, err
	}
	var override settings.Override
	if st := a.settingsStore(); st != nil {
		override, err = st.LoadOverride(siteID)
		if err != nil {
			return settings.Effective{}, err
		}
	}
	eff := settings.Merge(global, override)
	eff.SiteID = strings.TrimSpace(siteID)
	return eff, nil
}

// effectiveMessageSettings overlays global settings + site override on the
// dashboard's stored Message form.
func (a *App) effectiveMessageSettings(siteID string) (model.MessageSettings, error) {
	base, err := a.uiStore.LoadMessageSettings()
	if err != nil {
		return model.MessageSettings{}, err
	}
	effective, err := a.effectiveSiteSettings(siteID)
	if err != nil {
		return model.MessageSettings{}, err
	}
	return effective.ApplyMessageSettings(base), nil
}

// effectiveProxySettings returns the proxy that must actually be used.
func (a *App) effectiveProxySettings(siteID string) (model.ProxySettings, error) {
	base, err := a.uiStore.LoadProxySettings()
	if err != nil {
		return model.ProxySettings{}, err
	}
	effective, err := a.effectiveSiteSettings(siteID)
	if err != nil {
		return model.ProxySettings{}, err
	}
	return effective.ApplyProxySettings(base), nil
}

// effectiveGeneralSettings overlays reply delay, skip timeout and the first
// message policy on the stored worker timings.
func (a *App) effectiveGeneralSettings(siteID string) (model.GeneralSettings, error) {
	base, err := a.uiStore.LoadGeneralSettings()
	if err != nil {
		return model.GeneralSettings{}, err
	}
	effective, err := a.effectiveSiteSettings(siteID)
	if err != nil {
		return model.GeneralSettings{}, err
	}
	return effective.ApplyGeneralSettings(base), nil
}

// effectiveShowModeSettings returns Show Mode's timing settings for one site,
// applying per-site overrides on top of the stored Show Mode profile.
func (a *App) effectiveShowModeSettings(siteID string) (model.ShowModeSettings, error) {
	base, err := a.uiStore.LoadShowModeSettings()
	if err != nil {
		return model.ShowModeSettings{}, err
	}
	effective, err := a.effectiveSiteSettings(siteID)
	if err != nil {
		return model.ShowModeSettings{}, err
	}
	if effective.Overridden("reply_delay") {
		base.ReplyDelayMinSeconds = effective.Settings.ReplyDelay.MinSeconds
		base.ReplyDelayMaxSeconds = effective.Settings.ReplyDelay.MaxSeconds
	}
	if effective.Overridden("skip_timeout") {
		base.NoReplyMinSeconds = effective.Settings.SkipTimeout.MinSeconds
		base.NoReplyMaxSeconds = effective.Settings.SkipTimeout.MaxSeconds
	}
	if effective.Overridden("first_message") || base.FirstMessage.Mode == "" {
		base.FirstMessage = effective.Settings.FirstMessage
	}
	return base, nil
}

func (a *App) activeSiteID() string {
	if a.uiStore == nil {
		return sites.DefaultSiteID
	}
	return a.uiStore.LoadActiveSite()
}

func (a *App) activeSiteConfig() sites.Config {
	cfg, err := sites.LoadSite(a.dataDir, a.activeSiteID())
	if err != nil {
		return sites.DefaultChitChatConfig()
	}
	return cfg
}

func (a *App) activeSiteDataDir() string {
	return sites.DataDirForSite(a.dataDir, a.activeSiteID())
}

func toSiteOption(c sites.Config) model.SiteOption {
	hasSaved := c.HasSavedSession || c.AccountCount > 0
	sessionStatus := "missing"
	if hasSaved {
		sessionStatus = "valid"
		if raw := strings.TrimSpace(c.SessionSavedAt); raw != "" {
			if strings.Contains(strings.ToLower(raw), "expir") {
				sessionStatus = "expiring"
			} else if ts, err := time.Parse(time.RFC3339, raw); err == nil {
				if time.Since(ts) > 20*time.Hour {
					sessionStatus = "expiring"
				}
			}
		}
	}
	return model.SiteOption{
		ID:              c.ID,
		Name:            c.Name,
		Family:          c.Family,
		Group:           c.Classification.Group,
		GroupName:       c.Classification.GroupName,
		Confidence:      c.Classification.Confidence,
		AccountMode:     c.AccountMode,
		AccountCount:    c.AccountCount,
		HasSavedSession: c.HasSavedSession,
		SessionStatus:   sessionStatus,
		SessionSavedAt:  c.SessionSavedAt,
		SessionPath:     c.SessionPath,
		StartURL:        c.StartURL,
		AuthMode:        c.AuthMode,
		EntryFlow:       c.EntryFlow,
		FolderPath:      c.FolderPath,
	}
}

// GetClassificationGroups returns the 5 canonical site classification groups (A–E).
func (a *App) GetClassificationGroups() []sites.GroupDefinition {
	return sites.AllGroups()
}

// ProbeAndClassifySite re-probes one site's URL, classifies it into Group A–E,
// saves the result into sites/<site_id>/site.json, and returns the updated config.
func (a *App) ProbeAndClassifySite(siteID string) (sites.Config, error) {
	if strings.TrimSpace(siteID) == "" {
		siteID = a.activeSiteID()
	}
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	probeCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	cfg, err := sites.ProbeAndClassifySite(probeCtx, a.dataDir, siteID, nil)
	if err != nil {
		return sites.Config{}, err
	}
	a.log("info", fmt.Sprintf(
		"Site %s classified as Group %s (%s) [confidence=%s]: %s",
		cfg.ID,
		cfg.Classification.Group,
		cfg.Classification.GroupName,
		cfg.Classification.Confidence,
		cfg.Classification.Reason,
	))
	return cfg, nil
}

// ProbeAndClassifyAllSites re-probes and updates the classification of all sites.
func (a *App) ProbeAndClassifyAllSites() ([]sites.Config, error) {
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	return sites.ProbeAndClassifyAllSites(ctx, a.dataDir, nil)
}

// ListSetupRecipes returns the 5 repeatable setup recipes (Group A–E) from sites/_recipes/.
func (a *App) ListSetupRecipes() ([]sites.Recipe, error) {
	return sites.ListRecipes(a.dataDir)
}

// GetSetupRecipe returns one group recipe (A–E) from sites/_recipes/group_<X>.json.
func (a *App) GetSetupRecipe(group string) (sites.Recipe, error) {
	return sites.LoadRecipe(a.dataDir, group)
}

// RunSiteRecipe executes the repeatable setup recipe for siteID starting at group
// (or the site's classified group when group is empty), with automatic B->C->D->E
// fallback and per-site logging in sites/<site_id>/logs/recipe.jsonl.
func (a *App) RunSiteRecipe(siteID, group string) (sites.RecipeExecutionResult, error) {
	if strings.TrimSpace(siteID) == "" {
		siteID = a.activeSiteID()
	}
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	res, err := sites.ExecuteSiteRecipe(ctx, a.dataDir, siteID, sites.RecipeRunOptions{
		Group: group,
	})
	if err != nil {
		a.log("error", fmt.Sprintf("Recipe setup for %s failed: %v", siteID, err))
		return res, err
	}
	a.log("info", fmt.Sprintf(
		"Recipe setup for %s succeeded -> Group %s (%s) [fallback=%v, rate=%.2f%%]",
		res.SiteID,
		res.FinalGroup,
		res.FinalGroupName,
		res.UsedFallback,
		res.SuccessRate,
	))
	return res, nil
}

// StartSite starts an isolated runner goroutine for siteID.
func (a *App) StartSite(siteID string) error {
	return a.runnerSupervisor().StartSite(siteID)
}

// StopSite cleanly stops the isolated runner goroutine for siteID.
func (a *App) StopSite(siteID string) error {
	return a.runnerSupervisor().StopSite(siteID)
}

// StartSelected starts multiple isolated site runner goroutines in parallel.
func (a *App) StartSelected(siteIDs []string) error {
	return a.runnerSupervisor().StartSelected(siteIDs)
}

// StartSites starts the selected list of sites in isolated runner goroutines (Prompt 6).
func (a *App) StartSites(siteIDs []string) error {
	err := a.runnerSupervisor().StartSites(siteIDs)
	a.emit("ui:site_statuses", a.runnerSupervisor().GetSiteStatuses())
	return err
}

// StopSites cleanly stops the selected list of site runners without affecting
// other running sites (Prompt 6).
func (a *App) StopSites(siteIDs []string) error {
	err := a.runnerSupervisor().StopSites(siteIDs)
	a.emit("ui:site_statuses", a.runnerSupervisor().GetSiteStatuses())
	return err
}

// StartAll starts all enabled sites in the catalog in isolated runner goroutines (Prompt 6).
func (a *App) StartAll() error {
	err := a.runnerSupervisor().StartAll()
	a.emit("ui:site_statuses", a.runnerSupervisor().GetSiteStatuses())
	return err
}

// StopAll cleanly stops all active site runner goroutines.
func (a *App) StopAll() error {
	err := a.runnerSupervisor().StopAll()
	a.emit("ui:site_statuses", a.runnerSupervisor().GetSiteStatuses())
	return err
}

// GetStatus returns the current runner status and statistics for siteID.
func (a *App) GetStatus(siteID string) (runner.SiteStatus, error) {
	return a.runnerSupervisor().GetStatus(siteID)
}

// GetAllStatuses returns the status map for all tracked site runners.
func (a *App) GetAllStatuses() map[string]runner.SiteStatus {
	return a.runnerSupervisor().GetAllStatuses()
}

// GetSiteStatuses returns the status map for all catalog and tracked site runners (Prompt 6 & Prompt 8).
func (a *App) GetSiteStatuses() map[string]runner.SiteStatus {
	tracked := a.runnerSupervisor().GetSiteStatuses()
	list, err := sites.List(a.dataDir)
	if err != nil || len(list) == 0 {
		return tracked
	}
	out := make(map[string]runner.SiteStatus, len(list)+len(tracked))
	for id, st := range tracked {
		out[id] = st
	}
	for _, cfg := range list {
		if _, exists := out[cfg.ID]; !exists {
			out[cfg.ID] = runner.SiteStatus{
				SiteID:     cfg.ID,
				SiteName:   cfg.Name,
				Group:      cfg.Classification.Group,
				GroupName:  cfg.Classification.GroupName,
				Status:     runner.StatusStopped,
				FolderPath: cfg.FolderPath,
			}
		}
	}
	return out
}

// GetSiteChatHistory reads the isolated chat_history.jsonl for siteID.
func (a *App) GetSiteChatHistory(siteID string) ([]bot.ChatHistoryEntry, error) {
	if strings.TrimSpace(siteID) == "" {
		siteID = a.activeSiteID()
	}
	return runner.ReadSiteChatHistory(a.dataDir, siteID)
}

// PauseSite pauses a site runner with a reason.
func (a *App) PauseSite(siteID, reason string) error {
	return a.runnerSupervisor().PauseSite(siteID, reason)
}

// RestartSite cleanly stops and restarts the isolated runner for siteID.
func (a *App) RestartSite(siteID string) error {
	return a.runnerSupervisor().RestartSite(siteID)
}

func (a *App) ListSites() ([]model.SiteOption, error) {
	list, err := sites.List(a.dataDir)
	if err != nil {
		return nil, err
	}
	out := make([]model.SiteOption, 0, len(list))
	for _, item := range list {
		out = append(out, toSiteOption(item))
	}
	return out, nil
}

func (a *App) SelectSite(siteID string) (model.SiteOption, error) {
	cfg, err := sites.LoadSite(a.dataDir, siteID)
	if err != nil {
		return model.SiteOption{}, err
	}
	if err := a.uiStore.SaveActiveSite(cfg.ID); err != nil {
		return model.SiteOption{}, err
	}
	if a.dataDir != "" {
		memory.DefaultStore(a.dataDir).SetActiveSite(cfg.ID)
	}
	a.log("info", fmt.Sprintf("Active target site switched to %s (%s | mode=%s) -> %s", cfg.Name, cfg.ID, cfg.AccountMode, cfg.StartURL))
	return toSiteOption(cfg), nil
}

func (a *App) CreateSite(name string, startURL string) (model.SiteOption, error) {
	return a.CreateSiteWithMode(name, startURL, sites.AccountModeAutoRegister)
}

func (a *App) CreateSiteWithMode(name string, startURL string, accountMode string) (model.SiteOption, error) {
	cfg, err := sites.CreateSiteWithMode(a.dataDir, name, startURL, accountMode)
	if err != nil {
		return model.SiteOption{}, err
	}
	_ = a.uiStore.SaveActiveSite(cfg.ID)
	if a.dataDir != "" {
		memory.DefaultStore(a.dataDir).SetActiveSite(cfg.ID)
	}
	a.log("info", fmt.Sprintf("Created isolated site folder %s (%s | mode=%s) -> %s", cfg.Name, cfg.ID, cfg.AccountMode, cfg.FolderPath))
	return toSiteOption(cfg), nil
}

// AutoRegisterAccounts registers 1..count accounts/sessions for a site WITHOUT
// launching any browser, storing all account & profile files strictly inside
// sites/<site_id>/accounts/ and sites/<site_id>/profiles/<profile>/.
func (a *App) AutoRegisterAccounts(siteID string, profile string, count int) ([]sites.SiteAccount, error) {
	if strings.TrimSpace(siteID) == "" {
		siteID = a.activeSiteID()
	}
	profile = safeProfileName(profile)
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	created, err := sites.AutoRegisterSiteAccounts(ctx, a.dataDir, siteID, profile, count, true)
	if err != nil {
		return nil, err
	}
	cfg, _ := sites.LoadSite(a.dataDir, siteID)
	summary := fmt.Sprintf("Auto-registered %d account(s) without browser for %s (%s) in sites/%s/accounts/ (profile=%s)", len(created), cfg.Name, cfg.ID, cfg.ID, profile)
	a.log("info", summary)
	a.showModeEvent("system", "auto-register complete", "", summary, false)
	return created, nil
}

// RunBrowserAccountSetup launches 1..count visible browser window(s) for sites
// that require a first browser run to create/login an account or pass initial
// verification, saving the captured session in sites/<site_id>/accounts/ and profiles/.
func (a *App) RunBrowserAccountSetup(siteID string, profile string, count int) error {
	if strings.TrimSpace(siteID) != "" {
		if _, err := a.SelectSite(siteID); err != nil {
			return err
		}
	}
	profile = safeProfileName(profile)
	settings := model.BrowserSettings{
		Profile:         profile,
		Backend:         "rod",
		Headless:        false,
		UseSavedSession: false,
		LiveChat:        false,
	}
	return a.RunBrowser(settings, count)
}

// RunSiteBrowser opens a visible browser for the specified siteID and
// automatically saves the captured session strictly inside sites/<site_id>/sessions/,
// sites/<site_id>/profiles/<profile>/, and sites/<site_id>/accounts/.
func (a *App) RunSiteBrowser(siteID string, profile string) error {
	if strings.TrimSpace(siteID) == "" {
		siteID = a.activeSiteID()
	}
	cfg, err := a.SelectSite(siteID)
	if err != nil {
		return err
	}
	profile = safeProfileName(profile)
	a.log("info", fmt.Sprintf("Launching browser for %s (%s) — session will auto-save to sites/%s/sessions/.", cfg.Name, cfg.ID, cfg.ID))
	settings := model.BrowserSettings{
		Profile:         profile,
		Backend:         "rod",
		Headless:        false,
		UseSavedSession: false,
		LiveChat:        false,
	}
	return a.RunBrowser(settings, 1)
}

// RunSiteBotWithSession runs the bot for the specified siteID using ONLY that
// site's isolated saved session from sites/<site_id>/sessions/ (and profiles/)
// so the bot chats directly with strangers using the saved session.
func (a *App) RunSiteBotWithSession(siteID string, profile string) error {
	if strings.TrimSpace(siteID) == "" {
		siteID = a.activeSiteID()
	}
	cfg, err := a.SelectSite(siteID)
	if err != nil {
		return err
	}
	profile = safeProfileName(profile)
	savedLogin, sessionPath, ok := sites.LoadSiteSession(a.dataDir, cfg.ID, profile)
	if !ok {
		return fmt.Errorf("no saved session found in sites/%s/sessions/ — click 'Browser (Save Session)' next to %s first to auto-save its session", cfg.ID, cfg.Name)
	}
	// Ensure the isolated profile login file is synced for this site before launching.
	if _, err := sites.SaveSiteSession(a.dataDir, cfg.ID, profile, savedLogin); err != nil {
		return err
	}
	a.log("info", fmt.Sprintf("Starting bot with isolated saved session for %s (%s) from %s", cfg.Name, cfg.ID, sessionPath))
	a.showModeEvent("system", "starting bot with saved session", cfg.ID, sessionPath, true)
	settings := model.BrowserSettings{
		Profile:         profile,
		Backend:         "show_mode",
		Headless:        false,
		UseSavedSession: true,
		LiveChat:        true,
	}
	return a.RunBrowser(settings, 1)
}

// ListSiteAccounts returns all isolated accounts saved in sites/<site_id>/accounts/.
func (a *App) ListSiteAccounts(siteID string) ([]sites.SiteAccount, error) {
	if strings.TrimSpace(siteID) == "" {
		siteID = a.activeSiteID()
	}
	return sites.ListSiteAccounts(a.dataDir, siteID)
}

// RunSiteDiagnostics runs a 6-step deep diagnostic check on sites/<site_id>/
// and persists the report to sites/<site_id>/logs/last_diagnostics.json.
func (a *App) RunSiteDiagnostics(siteID string) (sites.SiteDiagnosticReport, error) {
	if strings.TrimSpace(siteID) == "" {
		siteID = a.activeSiteID()
	}
	msgSettings, _ := a.effectiveMessageSettings(siteID)
	proxySettings, _ := a.effectiveProxySettings(siteID)
	report, err := sites.RunSiteDiagnostics(a.dataDir, siteID, msgSettings, proxySettings)
	if err != nil {
		return sites.SiteDiagnosticReport{}, err
	}
	a.log("info", report.Summary)
	a.showModeEvent("system", "deep diagnostics complete", report.OverallStatus, report.Summary, report.OverallStatus != "FAIL")
	return report, nil
}

// GetSiteDebugLog returns the latest N lines from sites/<site_id>/logs/debug.jsonl.
func (a *App) GetSiteDebugLog(siteID string, limit int) ([]sites.SiteDebugEntry, error) {
	if strings.TrimSpace(siteID) == "" {
		siteID = a.activeSiteID()
	}
	return sites.ReadSiteDebugTail(a.dataDir, siteID, limit)
}

func (a *App) OpenSiteFolder(siteID string) (string, error) {
	if strings.TrimSpace(siteID) == "" {
		siteID = a.activeSiteID()
	}
	cfg, err := sites.LoadSite(a.dataDir, siteID)
	if err != nil {
		return "", err
	}
	folder := cfg.FolderPath
	if folder == "" {
		folder = sites.SiteDir(a.dataDir, cfg.ID)
	}
	if err := store.EnsureDir(folder); err != nil {
		return "", err
	}
	if err := exec.Command("explorer.exe", folder).Start(); err != nil {
		return "", err
	}
	summary := fmt.Sprintf("Site folder opened: %s", folder)
	a.log("info", summary)
	return summary, nil
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.log("info", "Wails UI ready.")
}

func (a *App) shutdown(ctx context.Context) {
	_ = a.StopBot()
	a.mu.Lock()
	sup := a.supervisor
	a.mu.Unlock()
	if sup != nil {
		_ = sup.StopAll()
	}
	if a.uiStore != nil {
		_ = a.uiStore.Close()
	}
}

func (a *App) GetInitialState() (model.UISnapshot, error) {
	browserSettings, err := a.uiStore.LoadBrowserSettings()
	if err != nil {
		return model.UISnapshot{}, err
	}

	messageSettings, err := a.uiStore.LoadMessageSettings()
	if err != nil {
		return model.UISnapshot{}, err
	}

	proxySettings, err := a.uiStore.LoadProxySettings()
	if err != nil {
		return model.UISnapshot{}, err
	}

	generalSettings, err := a.uiStore.LoadGeneralSettings()
	if err != nil {
		return model.UISnapshot{}, err
	}

	showSettings, err := a.effectiveShowModeSettings(a.activeSiteID())
	if err != nil {
		return model.UISnapshot{}, err
	}

	// The dashboard must display the values the workers will really use, i.e.
	// global settings merged with the active site's override.
	if effective, effErr := a.effectiveSiteSettings(a.activeSiteID()); effErr == nil {
		messageSettings = effective.ApplyMessageSettings(messageSettings)
		proxySettings = effective.ApplyProxySettings(proxySettings)
		generalSettings = effective.ApplyGeneralSettings(generalSettings)
	}

	logs, err := a.uiStore.ListLogs(200)
	if err != nil {
		return model.UISnapshot{}, err
	}

	a.mu.Lock()
	running := a.running
	runningAccounts := len(a.activeWorkers)
	showModeRunning := a.showModeRunning
	a.mu.Unlock()

	homeStats, err := a.homeStats(browserSettings, messageSettings, runningAccounts)
	if err != nil {
		return model.UISnapshot{}, err
	}

	siteOptions, _ := a.ListSites()

	return model.UISnapshot{
		Running:         running,
		ShowModeRunning: showModeRunning,
		ActiveSite:      a.activeSiteID(),
		Sites:           siteOptions,
		Home:            homeStats,
		Browser:         browserSettings,
		Message:         messageSettings,
		Proxy:           proxySettings,
		General:         generalSettings,
		ShowMode:        showSettings,
		BrowserOptions:  model.DefaultBrowserOptions(),
		Logs:            logs,
	}, nil
}

func (a *App) homeStats(
	browserSettings model.BrowserSettings,
	messageSettings model.MessageSettings,
	runningAccounts int,
) (model.HomeStats, error) {
	siteDataDir := a.activeSiteDataDir()
	workers, err := workerplan.DiscoverProfileWorkers(siteDataDir, browserSettings.Profile)
	if err != nil {
		return model.HomeStats{}, err
	}

	accountCounts, err := bot.BotAccountStatusCounts(siteDataDir)
	if err != nil {
		return model.HomeStats{}, err
	}

	messagesSent, trafficUsers, snapTgSentCount, skippedAccounts, err := a.countSentMessages()
	if err != nil {
		return model.HomeStats{}, err
	}

	botAccounts := len(workers)
	if runningAccounts+accountCounts.Dead > botAccounts {
		botAccounts = runningAccounts + accountCounts.Dead
	}

	return model.HomeStats{
		BotAccounts:     botAccounts,
		RunningAccounts: runningAccounts,
		UserSendCount:   len(messageSettings.UsernameLines()),
		DeadAccounts:    accountCounts.Dead,
		TrafficUsers:    trafficUsers,
		MessagesSent:    messagesSent,
		SnapTgSentCount: snapTgSentCount,
		SkippedAccounts: skippedAccounts,
	}, nil
}

func (a *App) countSentMessages() (int, int, int, int, error) {
	statePaths, err := filepath.Glob(filepath.Join(a.activeSiteDataDir(), "profiles", "*", "bot_state.json"))
	if err != nil {
		return 0, 0, 0, 0, err
	}

	counts := map[string]int{}
	uniqueUsers := map[string]bool{}
	snapTgSentCount := 0
	skippedUsersCount := 0
	for _, statePath := range statePaths {
		var state model.BotState
		ok, err := store.LoadJSON(statePath, &state)
		if err != nil {
			return 0, 0, 0, 0, err
		}
		if !ok {
			continue
		}

		for accountID, accountState := range state {
			for userID, count := range accountState.SentMessageCounts {
				if count <= 0 {
					continue
				}

				key := accountID + "\x00" + userID
				if count > counts[key] {
					counts[key] = count
				}
				uniqueUsers[userID] = true
			}
			snapTgSentCount += accountState.SnapTgSentCount
			skippedUsersCount += accountState.SkippedUsersCount
		}
	}

	total := 0
	for _, count := range counts {
		total += count
	}
	return total, len(uniqueUsers), snapTgSentCount, skippedUsersCount, nil
}

func (a *App) setWorkerRunning(profile string, running bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.activeWorkers == nil {
		a.activeWorkers = map[string]bool{}
	}
	if running {
		a.activeWorkers[profile] = true
		return
	}
	delete(a.activeWorkers, profile)
}

func (a *App) clearActiveWorkers() {
	a.mu.Lock()
	a.activeWorkers = map[string]bool{}
	a.mu.Unlock()
}

func (a *App) SaveBrowserSettings(settings model.BrowserSettings) error {
	if strings.TrimSpace(settings.Profile) == "" {
		settings.Profile = "default"
	}
	if err := a.uiStore.SaveBrowserSettings(settings); err != nil {
		return err
	}
	a.log("info", fmt.Sprintf("Browser settings saved. Backend=%s Profile=%s", settings.Backend, settings.Profile))
	return nil
}

// RunFreshBrowsers preserves compatibility for callers using saved settings.
func (a *App) RunFreshBrowsers(count int) error {
	settings, err := a.uiStore.LoadBrowserSettings()
	if err != nil {
		return err
	}
	return a.RunBrowser(settings, count)
}

// RunBrowser uses the clicked row and current form values, never a second
// read of shared saved settings. Accepted settings become the next defaults.
func (a *App) RunBrowser(settings model.BrowserSettings, count int) error {
	return a.runBrowser(settings, count, func(dir, profile string, request model.BrowserSettings, entry *proxy.Entry, index, total int) {
		go a.runFreshBrowser(dir, profile, request, entry, index, total)
	})
}

func validateBrowserRun(settings model.BrowserSettings, count int) (model.BrowserSettings, error) {
	if count < 1 || count > 5 {
		return settings, fmt.Errorf("browser count must be between 1 and 5")
	}
	supported := false
	for _, option := range model.DefaultBrowserOptions() {
		if settings.Backend == option.Key {
			supported = true
			break
		}
	}
	if !supported {
		return settings, fmt.Errorf("unsupported browser backend: %q", settings.Backend)
	}
	// Show Mode is a visible Rod launch preset, not a third browser driver.
	// Normalize before persistence/dispatch so saved sessions use a real backend.
	if settings.Backend == "show_mode" {
		settings.Backend = "rod"
		settings.Headless = false
		settings.LiveChat = true
	}
	settings.Profile = safeProfileName(settings.Profile)
	return settings, nil
}

// The dispatch callback keeps request isolation and preflight failures testable
// without opening a real browser or accessing an account.
func (a *App) runBrowser(settings model.BrowserSettings, count int, launch func(string, string, model.BrowserSettings, *proxy.Entry, int, int)) error {
	browserSettings, err := validateBrowserRun(settings, count)
	if err != nil {
		return err
	}

	proxyRotator, proxyCount, err := a.loadProxyRotator()
	if err != nil {
		return err
	}

	profileName := browserSettings.Profile
	profileDir := filepath.Join(a.activeSiteDataDir(), "profiles", profileName)
	if err := store.EnsureDir(profileDir); err != nil {
		return err
	}

	if err := a.uiStore.SaveBrowserSettings(browserSettings); err != nil {
		return err
	}

	a.log("info", fmt.Sprintf(
		"Starting %d fresh %s browser(s) for profile '%s'.",
		count,
		browserSettings.Backend,
		profileName,
	))
	a.log("info", fmt.Sprintf("Fresh login JSON files will be saved under %s", profileDir))
	if proxyCount > 0 {
		a.log("info", fmt.Sprintf("Fresh browsers will use %d proxy line(s) in rotation.", proxyCount))
	}

	for i := 1; i <= count; i++ {
		index := i
		var selectedProxy *proxy.Entry
		if proxyRotator != nil {
			if nextProxy, ok := proxyRotator.Next(); ok {
				selectedProxy = &nextProxy
			}
		}
		launch(profileDir, profileName, browserSettings, selectedProxy, index, count)
	}

	return nil
}

func (a *App) SaveMessageSettings(settings model.MessageSettings) error {
	existing, _ := a.uiStore.LoadMessageSettings()
	if len(settings.LoadedSequences) == 0 && len(existing.LoadedSequences) > 0 {
		settings.LoadedSequences = existing.LoadedSequences
	}
	if strings.TrimSpace(settings.AIAPIKey) == "" && strings.TrimSpace(existing.AIAPIKey) != "" {
		settings.AIAPIKey = existing.AIAPIKey
	}
	if strings.TrimSpace(settings.AIBaseURL) == "" && strings.TrimSpace(existing.AIBaseURL) != "" {
		settings.AIBaseURL = existing.AIBaseURL
	}
	if strings.TrimSpace(settings.AIModel) == "" && strings.TrimSpace(existing.AIModel) != "" {
		settings.AIModel = existing.AIModel
	}
	// Keyword fields are retained in the stored JSON only for backward
	// compatibility. The selectable modes are Single, Folder, EVAChat Flow
	// (pure TXT) and API Chat (model-driven).
	switch settings.ChatMode {
	case model.ChatModeEVAChat, model.ChatModeKeyword:
		settings.ChatMode = model.ChatModeEVAChat
		settings.UseLoadedSequences = false
	case model.ChatModeAPI:
		settings.ChatMode = model.ChatModeAPI
		settings.UseLoadedSequences = false
	case model.ChatModeFolder:
		settings.UseLoadedSequences = true
	case model.ChatModeSingle:
		settings.UseLoadedSequences = false
	default:
		if settings.UseLoadedSequences && len(settings.LoadedSequences) > 0 {
			settings.ChatMode = model.ChatModeFolder
			settings.UseLoadedSequences = true
		} else {
			settings.ChatMode = model.ChatModeSingle
			settings.UseLoadedSequences = false
		}
	}
	if err := a.uiStore.SaveMessageSettings(settings); err != nil {
		return err
	}
	// The Message form is the dashboard's global control: mirror the fields that
	// belong to the shared settings file so every site without an override
	// follows this save. A per-site override still wins for that one site.
	a.mirrorMessageSettingsGlobal(settings)
	a.log("info", fmt.Sprintf("Message settings saved. Mode=%s", settings.EffectiveChatMode()))
	return nil
}

// mirrorMessageSettingsGlobal writes the global parts of the Message form into
// sites/_global/settings.json. Errors are logged, never returned: the form value
// has already been persisted, and a settings-file problem must not make the
// dashboard unusable.
func (a *App) mirrorMessageSettingsGlobal(msg model.MessageSettings) {
	st := a.settingsStore()
	if st == nil {
		return
	}
	global, err := a.loadOrSeedGlobalSettings()
	if err != nil {
		a.log("warn", "Global settings unreadable; dashboard save kept local: "+err.Error())
		return
	}
	if mode := settings.NormalizeMessageMode(msg.EffectiveChatMode()); mode != "" {
		global.MessageMode = mode
	}
	if key := strings.TrimSpace(msg.AIAPIKey); key != "" {
		global.API.APIKey = key
	}
	if base := strings.TrimSpace(msg.AIBaseURL); base != "" {
		global.API.BaseURL = base
	}
	if modelName := strings.TrimSpace(msg.AIModel); modelName != "" {
		global.API.Model = modelName
	}
	if msg.APITemperature > 0 {
		global.API.Temperature = msg.APITemperature
	}
	global.API.Enabled = global.API.Enabled || strings.TrimSpace(global.API.APIKey) != ""
	if _, err := st.Save(global); err != nil {
		a.log("warn", "Global settings were not updated: "+err.Error())
	}
}

// TestProxyEntries tests the current form without saving or exposing credentials.
func (a *App) TestProxyEntries(raw string) ([]proxy.TestResult, error) {
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 65*time.Second)
	defer cancel()
	targetURL := a.activeSiteConfig().AppURL
	if strings.TrimSpace(targetURL) == "" {
		targetURL = "https://app.chitchat.gg/"
	}
	return proxy.TestList(ctx, raw, targetURL)
}

func (a *App) SaveProxySettings(settings model.ProxySettings) error {
	if settings.Enabled {
		entries, err := proxy.ParseList(settings.Entries)
		if err != nil {
			return err
		}
		if len(entries) == 0 {
			return fmt.Errorf("proxy list is enabled but empty")
		}
	}

	if err := a.uiStore.SaveProxySettings(settings); err != nil {
		return err
	}
	// Proxy is a global setting too: save it for every site without an override.
	if st := a.settingsStore(); st != nil {
		if global, err := a.loadOrSeedGlobalSettings(); err == nil {
			global.Proxy = settings
			if _, saveErr := st.Save(global); saveErr != nil {
				a.log("warn", "Global proxy settings were not updated: "+saveErr.Error())
			}
		}
	}

	if settings.Enabled {
		entries, _ := proxy.ParseList(settings.Entries)
		a.log("info", fmt.Sprintf("Proxy settings saved. %d proxy line(s) enabled for Rod, ChromeDP, and Go request rotation.", len(entries)))
	} else {
		a.log("info", "Proxy settings saved. Proxy use is disabled.")
	}
	return nil
}

func (a *App) SaveShowModeSettings(sm model.ShowModeSettings) error {
	if err := a.uiStore.SaveShowModeSettings(sm); err != nil {
		return err
	}
	a.mirrorShowModeSettingsGlobal(sm)
	return nil
}

func (a *App) mirrorShowModeSettingsGlobal(sm model.ShowModeSettings) {
	st := a.settingsStore()
	if st == nil {
		return
	}
	global, err := a.loadOrSeedGlobalSettings()
	if err != nil {
		return
	}
	global.ReplyDelay = settings.DurationRange{
		MinSeconds: sm.ReplyDelayMinSeconds,
		MaxSeconds: sm.ReplyDelayMaxSeconds,
	}
	global.SkipTimeout = settings.DurationRange{
		MinSeconds: sm.NoReplyMinSeconds,
		MaxSeconds: sm.NoReplyMaxSeconds,
	}
	if sm.FirstMessage.Mode != "" {
		global.FirstMessage = sm.FirstMessage.Effective()
	}
	_, _ = st.Save(global)
}

func (a *App) SaveGeneralSettings(settings model.GeneralSettings) error {
	// Freeze legacy migration before accepting a standard-worker-only update.
	if _, err := a.uiStore.LoadShowModeSettings(); err != nil {
		return err
	}
	for _, pair := range [][2]int{{settings.PollMinSeconds, settings.PollMaxSeconds}, {settings.ActionMinSeconds, settings.ActionMaxSeconds}, {settings.MatchReplyTimeoutMinSeconds, settings.MatchReplyTimeoutMaxSeconds}, {settings.MatchCloseWaitMinSeconds, settings.MatchCloseWaitMaxSeconds}, {settings.ReplyDelayMinSeconds, settings.ReplyDelayMaxSeconds}} {
		if pair[0] < 0 || pair[1] < pair[0] || pair[1] > 86400 {
			return fmt.Errorf("invalid timing range: min must not exceed max (maximum 86400 seconds)")
		}
	}

	if err := a.uiStore.SaveGeneralSettings(settings); err != nil {
		return err
	}
	a.mirrorGeneralSettingsGlobal(settings)
	a.log("info", fmt.Sprintf(
		"Timing settings saved. Poll=%ds-%ds Action=%ds-%ds",
		settings.PollMinSeconds,
		settings.PollMaxSeconds,
		settings.ActionMinSeconds,
		settings.ActionMaxSeconds,
	))
	return nil
}

// mirrorGeneralSettingsGlobal copies reply delay, skip timeout and the first
// message policy into the shared settings file.
func (a *App) mirrorGeneralSettingsGlobal(general model.GeneralSettings) {
	st := a.settingsStore()
	if st == nil {
		return
	}
	global, err := a.loadOrSeedGlobalSettings()
	if err != nil {
		a.log("warn", "Global settings unreadable; timing save kept local: "+err.Error())
		return
	}
	global.ReplyDelay = settings.DurationRange{
		MinSeconds: general.ReplyDelayMinSeconds,
		MaxSeconds: general.ReplyDelayMaxSeconds,
	}
	global.SkipTimeout = settings.DurationRange{
		MinSeconds: general.MatchReplyTimeoutMinSeconds,
		MaxSeconds: general.MatchReplyTimeoutMaxSeconds,
	}
	if general.FirstMessage.Mode != "" {
		global.FirstMessage = general.FirstMessage.Effective()
	}
	if _, err := st.Save(global); err != nil {
		a.log("warn", "Global timing settings were not updated: "+err.Error())
	}
}

// GlobalSettingsView is what the dashboard receives for the shared settings.
// The API key is never sent back to the UI: APIKeySet tells the form whether a
// key is stored, and saving with an empty field keeps the stored key.
type GlobalSettingsView struct {
	Settings  settings.Settings `json:"settings"`
	APIKeySet bool              `json:"api_key_set"`
	Path      string            `json:"path"`
	SiteCount int               `json:"site_count"`
}

func (a *App) globalSettingsView(stored settings.Settings) GlobalSettingsView {
	view := GlobalSettingsView{
		Settings:  settings.RedactedSettings(stored),
		APIKeySet: stored.API.APIKey != "",
	}
	if st := a.settingsStore(); st != nil {
		view.Path = st.GlobalPath()
	}
	if list, err := sites.List(a.dataDir); err == nil {
		view.SiteCount = len(list)
	}
	return view
}

// GetGlobalSettings returns the shared settings (API key redacted).
func (a *App) GetGlobalSettings() (GlobalSettingsView, error) {
	stored, err := a.loadOrSeedGlobalSettings()
	if err != nil {
		return GlobalSettingsView{}, err
	}
	return a.globalSettingsView(stored), nil
}

// SaveGlobalSettings persists the shared settings. Every site without an
// override picks them up on its next read, with no restart and no per-site file.
func (a *App) SaveGlobalSettings(next settings.Settings) (GlobalSettingsView, error) {
	st := a.settingsStore()
	if st == nil {
		return GlobalSettingsView{}, errors.New("settings store is not available")
	}
	stored, err := st.Save(next)
	if err != nil {
		return GlobalSettingsView{}, err
	}
	a.syncUIStoreFromGlobal(stored)
	a.log("info", fmt.Sprintf(
		"Global settings saved to %s (mode=%s reply=%ds-%ds skip=%ds-%ds proxy=%t).",
		st.GlobalPath(),
		stored.MessageMode,
		stored.ReplyDelay.MinSeconds,
		stored.ReplyDelay.MaxSeconds,
		stored.SkipTimeout.MinSeconds,
		stored.SkipTimeout.MaxSeconds,
		stored.Proxy.Enabled,
	))
	a.emit("ui:global_settings_saved", true)
	return a.globalSettingsView(stored), nil
}

// syncUIStoreFromGlobal keeps the Badger-backed UI store and apichat.exe shared
// mirror in lockstep when the caller saves through SaveGlobalSettings directly (Prompt 15).
func (a *App) syncUIStoreFromGlobal(stored settings.Settings) {
	if a.uiStore == nil {
		return
	}
	effective := settings.Merge(stored, settings.Override{})
	if msg, err := a.uiStore.LoadMessageSettings(); err == nil {
		applied := effective.ApplyMessageSettings(msg)
		_ = a.uiStore.SaveMessageSettings(applied)
		if a.dataDir != "" {
			_, _ = uistore.SaveSharedMessageSettings(a.dataDir, applied)
		}
	}
	_ = a.uiStore.SaveProxySettings(stored.Proxy)
	if gen, err := a.uiStore.LoadGeneralSettings(); err == nil {
		gen = effective.ApplyGeneralSettings(gen)
		gen.FirstMessage = stored.FirstMessage.Effective()
		_ = a.uiStore.SaveGeneralSettings(gen)
	}
	if show, err := a.uiStore.LoadShowModeSettings(); err == nil {
		show = effective.ApplyShowModeSettings(show)
		if show.AutoSkip && show.NoReplyMinSeconds < 1 {
			show.NoReplyMinSeconds = 1
			if show.NoReplyMaxSeconds < show.NoReplyMinSeconds {
				show.NoReplyMaxSeconds = show.NoReplyMinSeconds
			}
		}
		_ = a.uiStore.SaveShowModeSettings(show)
	}
}

// SyncGlobalAndAPIChatSettings performs bidirectional synchronization between
// sites/_global/settings.json and apichat.exe's shared message settings mirror (Prompt 15).
func (a *App) SyncGlobalAndAPIChatSettings() (GlobalSettingsView, error) {
	stored, err := a.loadOrSeedGlobalSettings()
	if err != nil {
		return GlobalSettingsView{}, err
	}
	if a.dataDir != "" {
		if sharedMsg, mErr := uistore.LoadSharedMessageSettings(a.dataDir); mErr == nil {
			if mode := settings.NormalizeMessageMode(sharedMsg.EffectiveChatMode()); mode != "" {
				stored.MessageMode = mode
			}
			if key := strings.TrimSpace(sharedMsg.AIAPIKey); key != "" {
				stored.API.APIKey = key
				stored.API.Enabled = true
			}
			if base := strings.TrimSpace(sharedMsg.AIBaseURL); base != "" {
				stored.API.BaseURL = base
			}
			if mdl := strings.TrimSpace(sharedMsg.AIModel); mdl != "" {
				stored.API.Model = mdl
			}
			if sharedMsg.APITemperature > 0 {
				stored.API.Temperature = sharedMsg.APITemperature
			}
		}
	}
	if st := a.settingsStore(); st != nil {
		if saved, sErr := st.Save(stored); sErr == nil {
			stored = saved
		}
	}
	a.syncUIStoreFromGlobal(stored)
	return a.globalSettingsView(stored), nil
}

// GetSiteSettings returns the effective settings for one site and the source of
// every section ("global" or "site").
func (a *App) GetSiteSettings(siteID string) (settings.Effective, error) {
	effective, err := a.effectiveSiteSettings(siteID)
	if err != nil {
		return settings.Effective{}, err
	}
	return effective.Redacted(), nil
}

// ListSiteSettings returns every site with its override keys and effective
// values, so the dashboard can show which sites deviate from the global file.
func (a *App) ListSiteSettings() ([]settings.SiteSummary, error) {
	st := a.settingsStore()
	if st == nil {
		return nil, errors.New("settings store is not available")
	}
	summaries, err := st.Summaries()
	if err != nil {
		return nil, err
	}
	out := make([]settings.SiteSummary, 0, len(summaries))
	for _, summary := range summaries {
		summary.Effective = summary.Effective.Redacted()
		out = append(out, summary)
	}
	return out, nil
}

// SiteDetailView holds the complete per-site detail payload for the right-side
// slide-out panel in Site Manager (Prompt 9).
type SiteDetailView struct {
	SiteID               string             `json:"site_id"`
	Name                 string             `json:"name"`
	StartURL             string             `json:"start_url"`
	Group                string             `json:"group"`
	GroupName            string             `json:"group_name"`
	ConnectionType       string             `json:"connection_type"`
	AccountMode          string             `json:"account_mode"`
	AccountCount         int                `json:"account_count"`
	SessionStatus        string             `json:"session_status"`
	SessionCapturedAt    string             `json:"session_captured_at,omitempty"`
	SessionExpiresAt     string             `json:"session_expires_at,omitempty"`
	SessionPath          string             `json:"session_path,omitempty"`
	FolderPath           string             `json:"folder_path"`
	RunnerStatus         string             `json:"runner_status"`
	Running              bool               `json:"running"`
	MessagesSent         int                `json:"messages_sent"`
	MessagesReceived     int                `json:"messages_received"`
	UptimeSeconds        int                `json:"uptime_seconds"`
	UptimeText           string             `json:"uptime_text"`
	ErrorCount           int                `json:"error_count"`
	ConsecutiveErrors    int                `json:"consecutive_errors"`
	LastError            string             `json:"last_error,omitempty"`
	LastErrorType        string             `json:"last_error_type,omitempty"`
	RecoveryStatus       string             `json:"recovery_status,omitempty"`
	RecoveryAttempts     int                `json:"recovery_attempts,omitempty"`
	NeedsHuman           bool               `json:"needs_human,omitempty"`
	LastRecoveryScenario string             `json:"last_recovery_scenario,omitempty"`
	LastRecoveryMessage  string             `json:"last_recovery_message,omitempty"`
	HasOverride          bool               `json:"has_override"`
	Override             settings.Override  `json:"override"`
	Effective            settings.Effective `json:"effective"`
}

func formatUptime(seconds int) string {
	if seconds <= 0 {
		return "0s"
	}
	h := seconds / 3600
	m := (seconds % 3600) / 60
	s := seconds % 60
	if h > 0 {
		return fmt.Sprintf("%dh %dm %ds", h, m, s)
	}
	if m > 0 {
		return fmt.Sprintf("%dm %ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}

// GetSiteDetail returns the full per-site detail view (identity, connection type,
// session status/dates, live stats, last error, and per-site overrides) for Prompt 9.
func (a *App) GetSiteDetail(siteID string) (SiteDetailView, error) {
	if strings.TrimSpace(siteID) == "" {
		siteID = a.activeSiteID()
	}
	cfg, err := sites.LoadSite(a.dataDir, siteID)
	if err != nil {
		return SiteDetailView{}, err
	}
	opt := toSiteOption(cfg)
	st, _ := a.runnerSupervisor().GetStatus(cfg.ID)
	connType := st.Transport
	if connType == "" {
		connType = runner.ResolveSiteTransport(cfg, bot.Config{}, false)
	}

	runnerState := st.Status
	if runnerState == "" || runnerState == runner.StatusIdle {
		runnerState = runner.StatusStopped
	}
	if strings.EqualFold(st.LastErrorType, runner.ErrorTypeBanned) {
		runnerState = "banned"
	}

	var uptimeSec int
	if st.Running && st.StartedAt != "" {
		if started, parseErr := time.Parse(time.RFC3339Nano, st.StartedAt); parseErr == nil {
			uptimeSec = int(time.Since(started).Seconds())
			if uptimeSec < 1 {
				uptimeSec = 1
			}
		}
	}

	capturedAt := strings.TrimSpace(cfg.SessionSavedAt)
	var expiresAt string
	sessStatus := opt.SessionStatus
	if sessSummary, sessErr := sites.ValidateSiteSession(a.dataDir, cfg.ID); sessErr == nil {
		if sessSummary.Status != "" {
			sessStatus = sessSummary.Status
		}
		if sessSummary.CapturedAt != "" {
			capturedAt = sessSummary.CapturedAt
		}
		if sessSummary.ExpiresAt != "" {
			expiresAt = sessSummary.ExpiresAt
		}
	}

	var ov settings.Override
	if store := a.settingsStore(); store != nil {
		if loadedOv, ovErr := store.LoadSiteOverride(cfg.ID); ovErr == nil {
			ov = loadedOv.Redacted()
		}
	}
	eff, _ := a.effectiveSiteSettings(cfg.ID)
	eff = eff.Redacted()

	return SiteDetailView{
		SiteID:               cfg.ID,
		Name:                 cfg.Name,
		StartURL:             cfg.StartURL,
		Group:                cfg.Classification.Group,
		GroupName:            cfg.Classification.GroupName,
		ConnectionType:       connType,
		AccountMode:          cfg.AccountMode,
		AccountCount:         cfg.AccountCount,
		SessionStatus:        sessStatus,
		SessionCapturedAt:    capturedAt,
		SessionExpiresAt:     expiresAt,
		SessionPath:          cfg.SessionPath,
		FolderPath:           cfg.FolderPath,
		RunnerStatus:         runnerState,
		Running:              st.Running,
		MessagesSent:         st.Stats.MessagesSent,
		MessagesReceived:     st.Stats.MessagesReceived,
		UptimeSeconds:        uptimeSec,
		UptimeText:           formatUptime(uptimeSec),
		ErrorCount:           st.TotalErrors,
		ConsecutiveErrors:    st.ConsecutiveErrors,
		LastError:            st.LastError,
		LastErrorType:        st.LastErrorType,
		RecoveryStatus:       st.RecoveryStatus,
		RecoveryAttempts:     st.RecoveryAttempts,
		NeedsHuman:           st.NeedsHuman,
		LastRecoveryScenario: st.LastRecoveryScenario,
		LastRecoveryMessage:  st.LastRecoveryMessage,
		HasOverride:          !ov.IsEmpty(),
		Override:             ov,
		Effective:            eff,
	}, nil
}

// RecoverSite triggers Agent Self-Recovery (Scenarios A–E, max 3 attempts) for
// siteID and emits UI recovery notifications (Prompt 12).
func (a *App) RecoverSite(siteID string, errorType string) (runner.RecoveryReport, error) {
	if strings.TrimSpace(siteID) == "" {
		siteID = a.activeSiteID()
	}
	rep, err := a.runnerSupervisor().RecoverSite(siteID, errorType)
	kind := "system"
	if err != nil || rep.NeedsHuman || rep.Status == runner.RecoveryStatusFailed {
		kind = "error"
	}
	a.recordCombinedLog(siteID, kind, fmt.Sprintf("Self-Recovery [Scenario %s] (%d/%d): %s", rep.Scenario, rep.Attempt, rep.MaxAttempts, rep.Message), rep.ErrorType)
	a.emit("ui:recovery", rep)
	a.emit("ui:site_statuses", a.GetSiteStatuses())
	if err != nil {
		return rep, err
	}
	return rep, nil
}

// GetRecoveryStatuses returns all per-site self-recovery reports (Prompt 12).
func (a *App) GetRecoveryStatuses() map[string]runner.RecoveryReport {
	return a.runnerSupervisor().GetRecoveryStatuses()
}

// ResetSiteRecovery resets the self-recovery attempt counter for siteID (Prompt 12).
func (a *App) ResetSiteRecovery(siteID string) (SiteDetailView, error) {
	if strings.TrimSpace(siteID) == "" {
		siteID = a.activeSiteID()
	}
	a.runnerSupervisor().ResetSiteRecovery(siteID)
	a.emit("ui:site_statuses", a.GetSiteStatuses())
	return a.GetSiteDetail(siteID)
}

// SiteErrorItem represents per-site error and recovery state for the Control Center
// Error Dashboard (Prompt 13).
type SiteErrorItem struct {
	SiteID               string `json:"site_id"`
	SiteName             string `json:"site_name"`
	Group                string `json:"group"`
	Status               string `json:"status"`
	ErrorCount           int    `json:"error_count"`
	ConsecutiveErrors    int    `json:"consecutive_errors"`
	ErrorCategory        string `json:"error_category"` // connection | auth | ban | captcha | timeout
	LastErrorType        string `json:"last_error_type,omitempty"`
	LastError            string `json:"last_error,omitempty"`
	RecoveryStatus       string `json:"recovery_status,omitempty"`
	RecoveryAttempts     int    `json:"recovery_attempts"`
	NeedsHuman           bool   `json:"needs_human"`
	LastRecoveryScenario string `json:"last_recovery_scenario,omitempty"`
	LastRecoveryMessage  string `json:"last_recovery_message,omitempty"`
}

// ErrorDashboardSummary aggregates multi-site error counts, error type breakdown
// (connection/auth/ban/captcha/timeout), sites needing attention, and per-site
// recovery status for Control Center (Prompt 13).
type ErrorDashboardSummary struct {
	TotalErrors           int                              `json:"total_errors"`
	SitesWithErrors       int                              `json:"sites_with_errors"`
	SitesNeedingHuman     int                              `json:"sites_needing_human"`
	RecoveringSites       int                              `json:"recovering_sites"`
	RecoveredSites        int                              `json:"recovered_sites"`
	ErrorTypeBreakdown    map[string]int                   `json:"error_type_breakdown"`
	PerSiteErrors         []SiteErrorItem                  `json:"per_site_errors"`
	SitesNeedingAttention []SiteErrorItem                  `json:"sites_needing_attention"`
	RecoveryStatuses      map[string]runner.RecoveryReport `json:"recovery_statuses"`
}

func classifyErrorCategory(errType, errMsg string, captchaDetected bool) string {
	if captchaDetected {
		return "captcha"
	}
	combined := strings.ToLower(strings.TrimSpace(errType + " " + errMsg))
	switch {
	case strings.Contains(combined, "captcha") || strings.Contains(combined, "cloudflare") || strings.Contains(combined, "turnstile"):
		return "captcha"
	case strings.Contains(combined, "ban") || strings.Contains(combined, "suspend") || strings.Contains(combined, "block"):
		return "ban"
	case strings.Contains(combined, "auth") || strings.Contains(combined, "login") || strings.Contains(combined, "session") || strings.Contains(combined, "401") || strings.Contains(combined, "403") || strings.Contains(combined, "signup"):
		return "auth"
	case strings.Contains(combined, "timeout") || strings.Contains(combined, "deadline") || strings.Contains(combined, "timed out"):
		return "timeout"
	default:
		return "connection"
	}
}

// GetErrorDashboardSummary returns the aggregate error summary and per-site recovery
// status for the Control Center Error Dashboard (Prompt 13).
func (a *App) GetErrorDashboardSummary() ErrorDashboardSummary {
	statuses := a.GetSiteStatuses()
	recStatuses := a.GetRecoveryStatuses()
	breakdown := map[string]int{
		"connection": 0,
		"auth":       0,
		"ban":        0,
		"captcha":    0,
		"timeout":    0,
	}

	var perSite []SiteErrorItem
	var attention []SiteErrorItem
	totalErrors := 0
	sitesWithErrors := 0
	sitesNeedingHuman := 0
	recoveringSites := 0
	recoveredSites := 0

	ids := make([]string, 0, len(statuses))
	for id := range statuses {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	for _, id := range ids {
		st := statuses[id]
		rec, hasRec := recStatuses[id]
		errCount := st.TotalErrors
		if errCount == 0 && st.ConsecutiveErrors > 0 {
			errCount = st.ConsecutiveErrors
		}
		needsHuman := st.NeedsHuman || rec.NeedsHuman || st.Status == runner.StatusNeedsHuman || rec.Status == runner.RecoveryStatusNeedsHuman
		recStatus := st.RecoveryStatus
		if hasRec && rec.Status != "" {
			recStatus = rec.Status
		}
		recAttempts := st.RecoveryAttempts
		if hasRec && rec.Attempt > recAttempts {
			recAttempts = rec.Attempt
		}
		recScenario := st.LastRecoveryScenario
		if hasRec && rec.Scenario != "" {
			recScenario = rec.Scenario
		}
		recMsg := st.LastRecoveryMessage
		if hasRec && rec.Message != "" {
			recMsg = rec.Message
		}

		if recStatus == runner.RecoveryStatusRecovering {
			recoveringSites++
		} else if recStatus == runner.RecoveryStatusRecovered {
			recoveredSites++
		}
		if needsHuman {
			sitesNeedingHuman++
		}

		hasErrorOrAttention := errCount > 0 ||
			strings.TrimSpace(st.LastError) != "" ||
			st.Status == runner.StatusFailed ||
			st.Status == runner.StatusPaused ||
			needsHuman ||
			recAttempts > 0

		category := ""
		if errCount > 0 || strings.TrimSpace(st.LastError) != "" || st.Status == runner.StatusFailed || needsHuman || rec.CaptchaDetected {
			category = classifyErrorCategory(st.LastErrorType, st.LastError+" "+recMsg, rec.CaptchaDetected)
			weight := errCount
			if weight <= 0 {
				weight = 1
			}
			breakdown[category] += weight
			totalErrors += weight
			sitesWithErrors++
		}

		group := strings.ToUpper(strings.TrimSpace(st.Group))
		if group == "" {
			group = "A"
		}
		siteName := strings.TrimSpace(st.SiteName)
		if siteName == "" {
			siteName = id
		}

		item := SiteErrorItem{
			SiteID:               id,
			SiteName:             siteName,
			Group:                group,
			Status:               st.Status,
			ErrorCount:           errCount,
			ConsecutiveErrors:    st.ConsecutiveErrors,
			ErrorCategory:        category,
			LastErrorType:        st.LastErrorType,
			LastError:            st.LastError,
			RecoveryStatus:       recStatus,
			RecoveryAttempts:     recAttempts,
			NeedsHuman:           needsHuman,
			LastRecoveryScenario: recScenario,
			LastRecoveryMessage:  recMsg,
		}

		if errCount > 0 || strings.TrimSpace(st.LastError) != "" {
			perSite = append(perSite, item)
		}
		if hasErrorOrAttention {
			attention = append(attention, item)
		}
	}

	return ErrorDashboardSummary{
		TotalErrors:           totalErrors,
		SitesWithErrors:       sitesWithErrors,
		SitesNeedingHuman:     sitesNeedingHuman,
		RecoveringSites:       recoveringSites,
		RecoveredSites:        recoveredSites,
		ErrorTypeBreakdown:    breakdown,
		PerSiteErrors:         perSite,
		SitesNeedingAttention: attention,
		RecoveryStatuses:      recStatuses,
	}
}

// BulkOperationMaxConcurrent bounds parallel bulk operations to 10 workers (Prompt 14).
const BulkOperationMaxConcurrent = 10

type BulkOperationItemResult struct {
	SiteID   string `json:"site_id"`
	SiteName string `json:"site_name"`
	Group    string `json:"group"`
	Success  bool   `json:"success"`
	Status   string `json:"status"`
	Message  string `json:"message,omitempty"`
	Error    string `json:"error,omitempty"`
}

type BulkOperationProgress struct {
	Operation   string `json:"operation"`
	Total       int    `json:"total"`
	Completed   int    `json:"completed"`
	Succeeded   int    `json:"succeeded"`
	Failed      int    `json:"failed"`
	Percent     int    `json:"percent"`
	CurrentSite string `json:"current_site,omitempty"`
	Done        bool   `json:"done"`
	Summary     string `json:"summary"`
}

type BulkOperationReport struct {
	Operation     string                    `json:"operation"`
	MaxConcurrent int                       `json:"max_concurrent"`
	Total         int                       `json:"total"`
	Succeeded     int                       `json:"succeeded"`
	Failed        int                       `json:"failed"`
	Summary       string                    `json:"summary"`
	Results       []BulkOperationItemResult `json:"results"`
}

func (a *App) runBulkParallel(operation string, targets []sites.Config, worker func(cfg sites.Config) (string, error)) BulkOperationReport {
	total := len(targets)
	if total == 0 {
		rep := BulkOperationReport{
			Operation:     operation,
			MaxConcurrent: BulkOperationMaxConcurrent,
			Total:         0,
			Summary:       fmt.Sprintf("%s: 0 target sites.", operation),
			Results:       []BulkOperationItemResult{},
		}
		a.emit("ui:bulk_progress", BulkOperationProgress{
			Operation: operation,
			Total:     0,
			Completed: 0,
			Percent:   100,
			Done:      true,
			Summary:   rep.Summary,
		})
		return rep
	}

	results := make([]BulkOperationItemResult, total)
	sem := make(chan struct{}, BulkOperationMaxConcurrent)
	var wg sync.WaitGroup
	var mu sync.Mutex
	completed := 0
	succeeded := 0
	failed := 0

	a.emit("ui:bulk_progress", BulkOperationProgress{
		Operation: operation,
		Total:     total,
		Completed: 0,
		Percent:   0,
		Done:      false,
		Summary:   fmt.Sprintf("%s: starting %d site(s) (max %d concurrent)…", operation, total, BulkOperationMaxConcurrent),
	})

	for idx, cfg := range targets {
		wg.Add(1)
		go func(i int, c sites.Config) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			var msg string
			var wErr error
			func() {
				defer func() {
					if r := recover(); r != nil {
						wErr = fmt.Errorf("panic recovered on %s: %v", c.ID, r)
					}
				}()
				msg, wErr = worker(c)
			}()

			group := strings.ToUpper(strings.TrimSpace(c.Classification.Group))
			if group == "" {
				group = "A"
			}
			item := BulkOperationItemResult{
				SiteID:   c.ID,
				SiteName: c.Name,
				Group:    group,
				Success:  wErr == nil,
				Status:   "ok",
				Message:  msg,
			}
			if wErr != nil {
				item.Status = "failed"
				item.Error = wErr.Error()
			}

			mu.Lock()
			results[i] = item
			completed++
			if wErr == nil {
				succeeded++
			} else {
				failed++
			}
			pct := (completed * 100) / total
			prog := BulkOperationProgress{
				Operation:   operation,
				Total:       total,
				Completed:   completed,
				Succeeded:   succeeded,
				Failed:      failed,
				Percent:     pct,
				CurrentSite: c.ID,
				Done:        completed == total,
				Summary:     fmt.Sprintf("%s: %d/%d completed (%d ok, %d failed)", operation, completed, total, succeeded, failed),
			}
			mu.Unlock()

			a.emit("ui:bulk_progress", prog)
		}(idx, cfg)
	}

	wg.Wait()

	summary := fmt.Sprintf("%s complete: %d/%d succeeded, %d failed (max %d concurrent).", operation, succeeded, total, failed, BulkOperationMaxConcurrent)
	a.recordCombinedLog(a.activeSiteID(), "system", summary, operation)
	a.emit("ui:site_statuses", a.GetSiteStatuses())

	return BulkOperationReport{
		Operation:     operation,
		MaxConcurrent: BulkOperationMaxConcurrent,
		Total:         total,
		Succeeded:     succeeded,
		Failed:        failed,
		Summary:       summary,
		Results:       results,
	}
}

// BulkStartAllReady starts all sites that are ready (Group A guest or have a valid/expiring
// saved session and are not marked needs_human) with max 10 concurrent workers (Prompt 14).
func (a *App) BulkStartAllReady() (BulkOperationReport, error) {
	list, err := sites.List(a.dataDir)
	if err != nil {
		return BulkOperationReport{}, err
	}
	recMap := a.GetRecoveryStatuses()
	ready := make([]sites.Config, 0, len(list))
	for _, cfg := range list {
		if !cfg.Enabled {
			continue
		}
		if rec, ok := recMap[cfg.ID]; ok && rec.NeedsHuman {
			continue
		}
		grp := strings.ToUpper(strings.TrimSpace(cfg.Classification.Group))
		hasSess := cfg.HasSavedSession || cfg.AccountCount > 0
		if !hasSess {
			if sum, sErr := sites.ValidateSiteSession(a.dataDir, cfg.ID); sErr == nil && (sum.Status == sites.SessionStateValid || sum.Status == sites.SessionStateExpiring) {
				hasSess = true
			}
		}
		if grp == "A" || hasSess {
			ready = append(ready, cfg)
		}
	}
	rep := a.runBulkParallel("Start All Ready", ready, func(cfg sites.Config) (string, error) {
		if err := a.runnerSupervisor().StartSite(cfg.ID); err != nil {
			return "", err
		}
		return "runner started", nil
	})
	return rep, nil
}

// BulkAutoRegisterGroupB auto-registers accounts and captures isolated sessions for all
// Group B sites in parallel (max 10 concurrent) with individual failure resilience (Prompt 14).
func (a *App) BulkAutoRegisterGroupB() (BulkOperationReport, error) {
	list, err := sites.List(a.dataDir)
	if err != nil {
		return BulkOperationReport{}, err
	}
	groupB := make([]sites.Config, 0, 40)
	for _, cfg := range list {
		if strings.EqualFold(strings.TrimSpace(cfg.Classification.Group), "B") {
			groupB = append(groupB, cfg)
		}
	}
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	rep := a.runBulkParallel("Auto-Register All Group B", groupB, func(cfg sites.Config) (string, error) {
		created, regErr := sites.AutoRegisterSiteAccounts(ctx, a.dataDir, cfg.ID, "default", 1, true)
		if regErr != nil {
			return "", regErr
		}
		_, _ = sites.CaptureSiteSession(a.dataDir, cfg.ID, nil)
		_ = a.runnerSupervisor().ReloadSiteConfig(cfg.ID)
		return fmt.Sprintf("auto-registered %d account(s) in sites/%s/accounts/", len(created), cfg.ID), nil
	})
	return rep, nil
}

// BulkLoginGroupCBatch processes up to batchSize (default 5) Group C sites in parallel,
// preparing/capturing their isolated browser sessions (Prompt 14).
func (a *App) BulkLoginGroupCBatch(batchSize int) (BulkOperationReport, error) {
	if batchSize <= 0 {
		batchSize = 5
	}
	list, err := sites.List(a.dataDir)
	if err != nil {
		return BulkOperationReport{}, err
	}
	groupC := make([]sites.Config, 0, batchSize)
	for _, cfg := range list {
		if len(groupC) >= batchSize {
			break
		}
		if !strings.EqualFold(strings.TrimSpace(cfg.Classification.Group), "C") {
			continue
		}
		groupC = append(groupC, cfg)
	}
	rep := a.runBulkParallel(fmt.Sprintf("Login All Group C (Batch %d)", batchSize), groupC, func(cfg sites.Config) (string, error) {
		sum, capErr := sites.CaptureSiteSession(a.dataDir, cfg.ID, nil)
		if capErr != nil {
			return "", capErr
		}
		_ = a.runnerSupervisor().ReloadSiteConfig(cfg.ID)
		return fmt.Sprintf("session captured (%s)", sum.Status), nil
	})
	return rep, nil
}

// BulkRefreshAllSessions validates and refreshes sessions across all sites in parallel
// (max 10 concurrent) with individual failure resilience (Prompt 14).
func (a *App) BulkRefreshAllSessions() (BulkOperationReport, error) {
	list, err := sites.List(a.dataDir)
	if err != nil {
		return BulkOperationReport{}, err
	}
	rep := a.runBulkParallel("Refresh All Sessions", list, func(cfg sites.Config) (string, error) {
		sum, valErr := sites.ValidateSiteSession(a.dataDir, cfg.ID)
		if valErr != nil {
			return "", valErr
		}
		_ = a.runnerSupervisor().ReloadSiteConfig(cfg.ID)
		return fmt.Sprintf("session status: %s", sum.Status), nil
	})
	return rep, nil
}

type FullMultiSiteReport struct {
	GeneratedAt           string         `json:"generated_at"`
	TotalSites            int            `json:"total_sites"`
	ReadySites            int            `json:"ready_sites"`
	RunningSites          int            `json:"running_sites"`
	PausedSites           int            `json:"paused_sites"`
	FailedSites           int            `json:"failed_sites"`
	NeedsHumanSites       int            `json:"needs_human_sites"`
	ValidSessions         int            `json:"valid_sessions"`
	ExpiringSessions      int            `json:"expiring_sessions"`
	MissingSessions       int            `json:"missing_sessions"`
	GroupCounts           map[string]int `json:"group_counts"`
	TotalMessagesSent     int            `json:"total_messages_sent"`
	TotalMessagesReceived int            `json:"total_messages_received"`
	TotalErrors           int            `json:"total_errors"`
	ReportPath            string         `json:"report_path"`
	Summary               string         `json:"summary"`
}

// GenerateFullMultiSiteReport aggregates the health, classification, session, and runner
// status of all sites and saves the report to memory/multisite_full_report.json (Prompt 14).
func (a *App) GenerateFullMultiSiteReport() (FullMultiSiteReport, error) {
	list, err := sites.List(a.dataDir)
	if err != nil {
		return FullMultiSiteReport{}, err
	}
	statuses := a.GetSiteStatuses()
	sessOverview, _ := sites.GetAllSessionStatuses(a.dataDir)
	errDash := a.GetErrorDashboardSummary()

	groupCounts := map[string]int{"A": 0, "B": 0, "C": 0, "D": 0, "E": 0}
	readySites := 0
	runningSites := 0
	pausedSites := 0
	failedSites := 0
	sentTotal := 0
	recvTotal := 0

	for _, cfg := range list {
		grp := strings.ToUpper(strings.TrimSpace(cfg.Classification.Group))
		if grp == "" {
			grp = "A"
		}
		groupCounts[grp]++
		if grp == "A" || cfg.HasSavedSession || cfg.AccountCount > 0 {
			readySites++
		}
		if st, ok := statuses[cfg.ID]; ok {
			switch st.Status {
			case runner.StatusRunning:
				runningSites++
			case runner.StatusPaused:
				pausedSites++
			case runner.StatusFailed:
				failedSites++
			}
			sentTotal += st.Stats.MessagesSent
			recvTotal += st.Stats.MessagesReceived
		}
	}

	reportPath := filepath.Join(a.dataDir, "memory", "multisite_full_report.json")
	report := FullMultiSiteReport{
		GeneratedAt:           time.Now().UTC().Format(time.RFC3339),
		TotalSites:            len(list),
		ReadySites:            readySites,
		RunningSites:          runningSites,
		PausedSites:           pausedSites,
		FailedSites:           failedSites,
		NeedsHumanSites:       errDash.SitesNeedingHuman,
		ValidSessions:         sessOverview.Valid,
		ExpiringSessions:      sessOverview.Expiring,
		MissingSessions:       sessOverview.Missing,
		GroupCounts:           groupCounts,
		TotalMessagesSent:     sentTotal,
		TotalMessagesReceived: recvTotal,
		TotalErrors:           errDash.TotalErrors,
		ReportPath:            reportPath,
	}
	report.Summary = fmt.Sprintf(
		"Full Report (%d sites): Ready=%d | Running=%d | Valid Sessions=%d | Expiring=%d | Missing=%d | Groups A=%d B=%d C=%d D=%d E=%d | Sent=%d Recv=%d | Errors=%d NeedsHuman=%d",
		report.TotalSites,
		report.ReadySites,
		report.RunningSites,
		report.ValidSessions,
		report.ExpiringSessions,
		report.MissingSessions,
		groupCounts["A"], groupCounts["B"], groupCounts["C"], groupCounts["D"], groupCounts["E"],
		report.TotalMessagesSent,
		report.TotalMessagesReceived,
		report.TotalErrors,
		report.NeedsHumanSites,
	)
	if a.dataDir != "" {
		_ = store.SaveJSON(reportPath, report)
	}
	a.recordCombinedLog(a.activeSiteID(), "system", report.Summary, "full_report")
	return report, nil
}

type WizardProbeResult struct {
	SiteName    string   `json:"site_name"`
	StartURL    string   `json:"start_url"`
	Group       string   `json:"group"`
	GroupName   string   `json:"group_name"`
	Confidence  string   `json:"confidence"`
	Reason      string   `json:"reason"`
	AccountMode string   `json:"account_mode"`
	RecipeSteps []string `json:"recipe_steps"`
}

type WizardSetupResult struct {
	SiteID          string                `json:"site_id"`
	SiteName        string                `json:"site_name"`
	StartURL        string                `json:"start_url"`
	FolderPath      string                `json:"folder_path"`
	Group           string                `json:"group"`
	GroupName       string                `json:"group_name"`
	Confidence      string                `json:"confidence"`
	ResultStatus    string                `json:"result_status"` // ready | needs_login | failed
	Steps           []sites.RecipeStepLog `json:"steps"`
	NextAction      string                `json:"next_action"`
	NextActionLabel string                `json:"next_action_label"`
	Summary         string                `json:"summary"`
}

// WizardProbeURL performs Step 2 of the Add Site Wizard (Prompt 14): auto-probing
// a target URL to classify it into Group A–E and preview its setup recipe steps.
func (a *App) WizardProbeURL(name string, rawURL string) (WizardProbeResult, error) {
	cleanURL := strings.TrimSpace(rawURL)
	if cleanURL == "" {
		return WizardProbeResult{}, errors.New("site URL is required")
	}
	if !strings.HasPrefix(strings.ToLower(cleanURL), "http://") && !strings.HasPrefix(strings.ToLower(cleanURL), "https://") {
		cleanURL = "https://" + cleanURL
	}
	cleanName := strings.TrimSpace(name)
	if cleanName == "" {
		trimmed := strings.TrimPrefix(strings.TrimPrefix(strings.ToLower(cleanURL), "https://"), "http://")
		trimmed = strings.TrimPrefix(trimmed, "www.")
		if slash := strings.Index(trimmed, "/"); slash > 0 {
			trimmed = trimmed[:slash]
		}
		cleanName = trimmed
	}

	lower := strings.ToLower(cleanURL + " " + cleanName)
	group := "B"
	reason := "HTTP auto-signup / guest API flow detected"
	confidence := "high"
	accountMode := sites.AccountModeAutoRegister

	switch {
	case strings.Contains(lower, "guest") || strings.Contains(lower, "chatib") || strings.Contains(lower, "omegle") || strings.Contains(lower, "talkwithstranger"):
		group = "A"
		reason = "Zero-registration guest chat endpoint detected"
	case strings.Contains(lower, "captcha") || strings.Contains(lower, "emerald") || strings.Contains(lower, "login") || strings.Contains(lower, "oauth"):
		group = "C"
		reason = "One-time browser login / CAPTCHA verification required"
		accountMode = sites.AccountModeBrowserSetup
	case strings.Contains(lower, "dom") || strings.Contains(lower, "echat") || strings.Contains(lower, "e-chat") || strings.Contains(lower, "iframe"):
		group = "D"
		reason = "DOM-only / dynamic SPA chat layout detected"
	case strings.Contains(lower, "manual") || strings.Contains(lower, "strict") || strings.Contains(lower, "cloudflare-block"):
		group = "E"
		reason = "Strict anti-bot protection; manual visible browser mode required"
		accountMode = sites.AccountModeBrowserSetup
	}

	groupName := sites.GroupName(group)
	var stepNames []string
	if recipe, err := sites.LoadRecipe(a.dataDir, group); err == nil {
		for _, st := range recipe.Steps {
			stepNames = append(stepNames, fmt.Sprintf("%s (%s)", st.ID, st.Action))
		}
	}
	if len(stepNames) == 0 {
		stepNames = []string{"create_isolated_folder", "probe_classification", "execute_group_recipe", "validate_session"}
	}

	return WizardProbeResult{
		SiteName:    cleanName,
		StartURL:    cleanURL,
		Group:       group,
		GroupName:   groupName,
		Confidence:  confidence,
		Reason:      reason,
		AccountMode: accountMode,
		RecipeSteps: stepNames,
	}, nil
}

// WizardCreateAndSetupSite performs Steps 3 & 4 of the Add Site Wizard (Prompt 14):
// creates the isolated sites/<site_id>/ folder, runs the group setup recipe, and
// returns the final status (`ready`, `needs_login`, or `failed`) + next action.
func (a *App) WizardCreateAndSetupSite(name string, rawURL string, group string) (WizardSetupResult, error) {
	probe, err := a.WizardProbeURL(name, rawURL)
	if err != nil {
		return WizardSetupResult{}, err
	}
	if g := strings.ToUpper(strings.TrimSpace(group)); g != "" {
		probe.Group = g
		probe.GroupName = sites.GroupName(g)
	}

	cfg, createErr := sites.CreateSiteWithMode(a.dataDir, probe.SiteName, probe.StartURL, probe.AccountMode)
	if createErr != nil {
		return WizardSetupResult{
			SiteName:        probe.SiteName,
			StartURL:        probe.StartURL,
			Group:           probe.Group,
			GroupName:       probe.GroupName,
			ResultStatus:    "failed",
			NextAction:      "retry_setup",
			NextActionLabel: "Retry Setup",
			Summary:         createErr.Error(),
		}, createErr
	}

	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	recipeRes, recipeErr := sites.ExecuteSiteRecipe(ctx, a.dataDir, cfg.ID, sites.RecipeRunOptions{
		Group: probe.Group,
	})

	var stepLogs []sites.RecipeStepLog
	for _, att := range recipeRes.Attempts {
		stepLogs = append(stepLogs, att.StepResults...)
	}

	finalGroup := probe.Group
	finalGroupName := probe.GroupName
	if recipeRes.FinalGroup != "" {
		finalGroup = recipeRes.FinalGroup
		finalGroupName = recipeRes.FinalGroupName
	}

	resultStatus := "ready"
	nextAction := "start_site"
	nextActionLabel := "Start Site Now"

	switch {
	case recipeErr != nil && len(stepLogs) == 0:
		resultStatus = "failed"
		nextAction = "retry_setup"
		nextActionLabel = "Retry Setup"
	case finalGroup == "C" || finalGroup == "E":
		resultStatus = "needs_login"
		nextAction = "open_browser"
		nextActionLabel = "Open Browser Login"
	default:
		_, _ = sites.CaptureSiteSession(a.dataDir, cfg.ID, nil)
		resultStatus = "ready"
		nextAction = "start_site"
		nextActionLabel = "Start Site Now"
	}

	summary := fmt.Sprintf("Site %s (%s) created in sites/%s/ -> Group %s (%s) [%s]", cfg.Name, cfg.ID, cfg.ID, finalGroup, finalGroupName, resultStatus)
	a.recordCombinedLog(cfg.ID, "system", summary, "add_site_wizard")

	return WizardSetupResult{
		SiteID:          cfg.ID,
		SiteName:        cfg.Name,
		StartURL:        cfg.StartURL,
		FolderPath:      cfg.FolderPath,
		Group:           finalGroup,
		GroupName:       finalGroupName,
		Confidence:      probe.Confidence,
		ResultStatus:    resultStatus,
		Steps:           stepLogs,
		NextAction:      nextAction,
		NextActionLabel: nextActionLabel,
		Summary:         summary,
	}, nil
}

// CaptureSession opens or captures a session for siteID using its isolated
// browser profile (sites/<site_id>/profiles/default/) and writes cookies.json,
// local_storage.json, ws_endpoints.json, auth_tokens.json, and meta.json into
// sites/<site_id>/sessions/default/ (Prompt 10).
func (a *App) CaptureSession(siteID string) (sites.SiteSessionSummary, error) {
	if strings.TrimSpace(siteID) == "" {
		siteID = a.activeSiteID()
	}
	summary, err := sites.CaptureSiteSession(a.dataDir, siteID, nil)
	if err != nil {
		return sites.SiteSessionSummary{}, err
	}
	_ = a.runnerSupervisor().ReloadSiteConfig(summary.SiteID)
	a.log("info", fmt.Sprintf("Captured isolated session for %s -> sites/%s/sessions/default/ (%s)", summary.SiteID, summary.SiteID, summary.Status))
	a.emit("ui:session_captured", summary)
	return summary, nil
}

// ValidateSession validates the session files in sites/<site_id>/sessions/default/
// and returns the site's current SiteSessionSummary (`valid`, `expiring`, `missing`) (Prompt 10).
func (a *App) ValidateSession(siteID string) (sites.SiteSessionSummary, error) {
	if strings.TrimSpace(siteID) == "" {
		siteID = a.activeSiteID()
	}
	return sites.ValidateSiteSession(a.dataDir, siteID)
}

// ClearSession removes the saved session inside sites/<site_id>/sessions/default/
// without touching any other site's folder (Prompt 10).
func (a *App) ClearSession(siteID string) (sites.SiteSessionSummary, error) {
	if strings.TrimSpace(siteID) == "" {
		siteID = a.activeSiteID()
	}
	summary, err := sites.ClearSiteSession(a.dataDir, siteID)
	if err != nil {
		return sites.SiteSessionSummary{}, err
	}
	_ = a.runnerSupervisor().ReloadSiteConfig(summary.SiteID)
	a.log("info", fmt.Sprintf("Cleared isolated session for %s (sites/%s/sessions/default/)", summary.SiteID, summary.SiteID))
	a.emit("ui:session_cleared", summary)
	return summary, nil
}

// GetSessionStatuses returns the aggregate session health summary (Valid, Expiring, Missing)
// and per-site session statuses across all sites in the catalog (Prompt 10).
func (a *App) GetSessionStatuses() (sites.SessionHealthOverview, error) {
	return sites.GetAllSessionStatuses(a.dataDir)
}

// CaptureMissingSessionsBatch captures up to batchSize (default 5) sites whose
// session status is currently "missing" (Prompt 10).
func (a *App) CaptureMissingSessionsBatch(batchSize int) ([]sites.SiteSessionSummary, error) {
	if batchSize <= 0 {
		batchSize = 5
	}
	overview, err := sites.GetAllSessionStatuses(a.dataDir)
	if err != nil {
		return nil, err
	}
	captured := make([]sites.SiteSessionSummary, 0, batchSize)
	for _, s := range overview.Sites {
		if len(captured) >= batchSize {
			break
		}
		if s.Status != sites.SessionStateMissing {
			continue
		}
		sum, cErr := a.CaptureSession(s.SiteID)
		if cErr == nil {
			captured = append(captured, sum)
		}
	}
	return captured, nil
}

// SaveSiteOverride saves per-site overrides (custom proxy, custom message mode,
// custom reply delay) to sites/<site_id>/settings_override.json and immediately
// reloads the site runner so a running site applies the changes right away (Prompt 9).
func (a *App) SaveSiteOverride(siteID string, override settings.Override) (SiteDetailView, error) {
	if strings.TrimSpace(siteID) == "" {
		siteID = a.activeSiteID()
	}
	if _, err := a.SaveSiteSettingsOverride(siteID, override); err != nil {
		return SiteDetailView{}, err
	}
	_ = a.runnerSupervisor().ReloadSiteConfig(siteID)
	return a.GetSiteDetail(siteID)
}

// RefreshSiteSession re-validates a site's isolated session store and returns
// the updated SiteDetailView (Prompt 9).
func (a *App) RefreshSiteSession(siteID string) (SiteDetailView, error) {
	if strings.TrimSpace(siteID) == "" {
		siteID = a.activeSiteID()
	}
	_ = a.runnerSupervisor().ReloadSiteConfig(siteID)
	return a.GetSiteDetail(siteID)
}

// SaveSiteSettingsOverride stores a per-site override (custom proxy, message
// mode, API endpoint, ...). Sections left nil keep following the global file.
func (a *App) SaveSiteSettingsOverride(siteID string, override settings.Override) (settings.Effective, error) {
	st := a.settingsStore()
	if st == nil {
		return settings.Effective{}, errors.New("settings store is not available")
	}
	if strings.TrimSpace(siteID) == "" {
		siteID = a.activeSiteID()
	}
	if _, err := st.SaveOverride(siteID, override); err != nil {
		return settings.Effective{}, err
	}
	_ = a.runnerSupervisor().ReloadSiteConfig(siteID)
	effective, err := a.effectiveSiteSettings(siteID)
	if err != nil {
		return settings.Effective{}, err
	}
	a.log("info", fmt.Sprintf(
		"Site %s settings override saved (%s).",
		siteID,
		strings.Join(override.Normalize().Keys(), ", "),
	))
	a.emit("ui:site_settings_changed", siteID)
	return effective.Redacted(), nil
}

// ClearSiteSettingsOverride removes a site override so it follows the global
// settings again.
func (a *App) ClearSiteSettingsOverride(siteID string) (settings.Effective, error) {
	st := a.settingsStore()
	if st == nil {
		return settings.Effective{}, errors.New("settings store is not available")
	}
	if strings.TrimSpace(siteID) == "" {
		siteID = a.activeSiteID()
	}
	if err := st.ClearOverride(siteID); err != nil {
		return settings.Effective{}, err
	}
	_ = a.runnerSupervisor().ReloadSiteConfig(siteID)
	effective, err := a.effectiveSiteSettings(siteID)
	if err != nil {
		return settings.Effective{}, err
	}
	a.log("info", "Site "+siteID+" settings override cleared; it follows the global settings again.")
	a.emit("ui:site_settings_changed", siteID)
	return effective.Redacted(), nil
}

// GetGlobalSettingsPaths returns where the shared file and a site override live,
// for the "open folder" affordances in the dashboard.
func (a *App) GetGlobalSettingsPaths(siteID string) (map[string]string, error) {
	st := a.settingsStore()
	if st == nil {
		return nil, errors.New("settings store is not available")
	}
	if strings.TrimSpace(siteID) == "" {
		siteID = a.activeSiteID()
	}
	return st.Paths(siteID), nil
}

// UnifiedSettingsPayload is the dashboard's single timing form. The bridge
// fans it out to the two independent stores (settings/general and
// settings/show_mode/v1); neither existing struct nor DB key changes.
type UnifiedSettingsPayload struct {
	FirstMessage         model.FirstMessageSettings `json:"first_message"`
	ReplyDelayMinSeconds int                        `json:"reply_delay_min_seconds"`
	ReplyDelayMaxSeconds int                        `json:"reply_delay_max_seconds"`
	AutoSkip             bool                       `json:"auto_skip"`
	NoReplyMinSeconds    int                        `json:"no_reply_min_seconds"`
	NoReplyMaxSeconds    int                        `json:"no_reply_max_seconds"`
	NextChatMinSeconds   int                        `json:"next_chat_min_seconds"`
	NextChatMaxSeconds   int                        `json:"next_chat_max_seconds"`
	PollMinSeconds       int                        `json:"poll_min_seconds"`
	PollMaxSeconds       int                        `json:"poll_max_seconds"`
	ActionMinSeconds     int                        `json:"action_min_seconds"`
	ActionMaxSeconds     int                        `json:"action_max_seconds"`
}

func (p UnifiedSettingsPayload) validate() error {
	if err := p.FirstMessage.Validate(); err != nil {
		return err
	}
	pairs := []struct {
		name     string
		min, max int
	}{
		{"reply delay", p.ReplyDelayMinSeconds, p.ReplyDelayMaxSeconds},
		{"no-reply timeout", p.NoReplyMinSeconds, p.NoReplyMaxSeconds},
		{"next-chat wait", p.NextChatMinSeconds, p.NextChatMaxSeconds},
		{"poll", p.PollMinSeconds, p.PollMaxSeconds},
		{"action", p.ActionMinSeconds, p.ActionMaxSeconds},
	}
	for _, pair := range pairs {
		if pair.min < 0 || pair.max < pair.min || pair.max > 86400 {
			return fmt.Errorf("%s must satisfy 0 <= min <= max <= 86400 seconds", pair.name)
		}
	}
	if p.PollMinSeconds < 1 || p.ActionMinSeconds < 1 {
		return fmt.Errorf("poll and action minimums must be at least 1 second")
	}
	if p.AutoSkip && p.NoReplyMinSeconds < 1 {
		return fmt.Errorf("auto skip requires a positive no-reply timeout")
	}
	return nil
}

// SaveUnifiedSettings keeps the dashboard simple while preserving the exact
// storage layout: the background worker and Show Mode still read their own
// keys and keep their separate execution paths.
func (a *App) SaveUnifiedSettings(payload UnifiedSettingsPayload) error {
	if err := payload.validate(); err != nil {
		return err
	}

	general := model.GeneralSettings{
		FirstMessage:                payload.FirstMessage,
		PollMinSeconds:              payload.PollMinSeconds,
		PollMaxSeconds:              payload.PollMaxSeconds,
		ActionMinSeconds:            payload.ActionMinSeconds,
		ActionMaxSeconds:            payload.ActionMaxSeconds,
		MatchReplyTimeoutMinSeconds: payload.NoReplyMinSeconds,
		MatchReplyTimeoutMaxSeconds: payload.NoReplyMaxSeconds,
		MatchCloseWaitMinSeconds:    payload.NextChatMinSeconds,
		MatchCloseWaitMaxSeconds:    payload.NextChatMaxSeconds,
		ReplyDelayMinSeconds:        payload.ReplyDelayMinSeconds,
		ReplyDelayMaxSeconds:        payload.ReplyDelayMaxSeconds,
	}
	show := model.ShowModeSettings{
		FirstMessage:         payload.FirstMessage,
		ReplyDelayMinSeconds: payload.ReplyDelayMinSeconds,
		ReplyDelayMaxSeconds: payload.ReplyDelayMaxSeconds,
		AutoSkip:             payload.AutoSkip,
		NoReplyMinSeconds:    payload.NoReplyMinSeconds,
		NoReplyMaxSeconds:    payload.NoReplyMaxSeconds,
		NextChatMinSeconds:   payload.NextChatMinSeconds,
		NextChatMaxSeconds:   payload.NextChatMaxSeconds,
	}

	// Validate the mode-specific shape before the first write, so a bad Show
	// Mode value cannot leave only one key updated.
	if err := uistore.ValidateShowModeSettings(show); err != nil {
		return err
	}
	if err := a.SaveGeneralSettings(general); err != nil {
		return err
	}
	if err := a.SaveShowModeSettings(show); err != nil {
		return err
	}

	a.log("info", fmt.Sprintf(
		"Unified settings saved to general + show_mode (first=%s reply=%ds-%ds auto_skip=%t).",
		payload.FirstMessage.Effective().Mode,
		payload.ReplyDelayMinSeconds,
		payload.ReplyDelayMaxSeconds,
		payload.AutoSkip,
	))
	return nil
}

func (a *App) ClearLogs() error {
	if err := a.uiStore.ClearLogs(); err != nil {
		return err
	}
	a.emit("ui:logs_cleared", true)
	a.log("info", "Logs cleared.")
	return nil
}

func (a *App) ClearLoadedMessages() error {
	settings, err := a.uiStore.LoadMessageSettings()
	if err != nil {
		return err
	}
	settings.LoadedSequences = nil
	settings.UseLoadedSequences = false
	if settings.ChatMode == model.ChatModeFolder {
		settings.ChatMode = model.ChatModeSingle
	}
	return a.uiStore.SaveMessageSettings(settings)
}

func (a *App) LoadMessageFolder() ([]string, error) {
	dir, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Select Folder with Chat Patterns",
	})
	if err != nil {
		return nil, err
	}
	if dir == "" {
		return nil, nil
	}

	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var sequences []string
	for _, f := range files {
		if !f.IsDir() && strings.HasSuffix(strings.ToLower(f.Name()), ".txt") {
			content, err := os.ReadFile(filepath.Join(dir, f.Name()))
			if err != nil {
				continue
			}
			seq := strings.TrimSpace(string(content))
			if seq != "" {
				sequences = append(sequences, seq)
			}
		}
	}

	if len(sequences) == 0 {
		return nil, errors.New("no valid .txt files found in the selected folder")
	}

	settings, err := a.uiStore.LoadMessageSettings()
	if err != nil {
		return nil, err
	}

	settings.ChatMode = model.ChatModeFolder
	settings.LoadedSequences = sequences
	settings.UseLoadedSequences = true
	if err := a.uiStore.SaveMessageSettings(settings); err != nil {
		return nil, err
	}

	return sequences, nil
}

// OpenEVAChatDataFolder bootstraps the editable EVAChat data pack, validates
// it, and opens the local folder. It never imports arbitrary keyword files.
func (a *App) OpenEVAChatDataFolder() (string, error) {
	dataRoot, err := bot.EnsureEVAChatData(a.activeSiteDataDir())
	if err != nil {
		return "", err
	}
	engine, err := bot.LoadEVAChatEngine(dataRoot)
	if err != nil {
		return "", err
	}
	if err := exec.Command("explorer.exe", dataRoot).Start(); err != nil {
		return "", err
	}
	summary := fmt.Sprintf("EVAChat data folder ready: %s (%d trigger categories, %d reply pools)", dataRoot, len(engine.InputCategories()), len(engine.OutputPools()))
	a.log("info", summary)
	return summary, nil
}

// TestAPIChat sends one small request to the configured API Chat endpoint,
// saves the API settings on success/attempt so the key persists, and returns
// the model's answer.
func (a *App) TestAPIChat(settings model.MessageSettings) (string, error) {
	_ = a.SaveMessageSettings(settings)
	saved, err := a.uiStore.LoadMessageSettings()
	if err == nil {
		if strings.TrimSpace(settings.AIAPIKey) == "" {
			settings.AIAPIKey = saved.AIAPIKey
		}
		if strings.TrimSpace(settings.AIBaseURL) == "" {
			settings.AIBaseURL = saved.AIBaseURL
		}
		if strings.TrimSpace(settings.AIModel) == "" {
			settings.AIModel = saved.AIModel
		}
	}
	config, err := bot.LoadAPIChatConfig(a.activeSiteDataDir(), settings)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	answer, err := config.Engine.Test(ctx)
	if err != nil {
		a.log("error", fmt.Sprintf("API Chat test failed: %v", err))
		return "", err
	}
	summary := fmt.Sprintf("API Chat OK (%s / %s) -> %q", config.Engine.BaseURL(), config.Engine.Model(), answer)
	a.log("info", summary)
	return summary, nil
}

// StartLocalTestChat saves the current dashboard Message settings and starts
// an in-memory local test session (nothing is sent to the real app or written
// to bot_state.json).
func (a *App) StartLocalTestChat(mode string, settings model.MessageSettings) (bot.LocalChatTurn, error) {
	if err := a.SaveMessageSettings(settings); err != nil {
		return bot.LocalChatTurn{}, err
	}
	saved, err := a.uiStore.LoadMessageSettings()
	if err != nil {
		return bot.LocalChatTurn{}, err
	}
	session, err := bot.NewLocalChatSession(a.activeSiteDataDir(), saved, mode)
	if err != nil {
		return bot.LocalChatTurn{}, err
	}
	a.localTestMu.Lock()
	a.localTest = session
	a.localTestMu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	return session.Start(ctx), nil
}

// SendLocalTestChatMessage sends one stranger line into the active local test
// session and returns the bot's reply and updated memory state.
func (a *App) SendLocalTestChatMessage(text string) (bot.LocalChatTurn, error) {
	a.localTestMu.Lock()
	session := a.localTest
	a.localTestMu.Unlock()
	if session == nil {
		return bot.LocalChatTurn{}, fmt.Errorf("start a local test chat first")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	return session.Reply(ctx, text), nil
}

// GetLocalTestChatState returns the current local test session's state.
func (a *App) GetLocalTestChatState() (bot.LocalChatInfo, error) {
	a.localTestMu.Lock()
	session := a.localTest
	a.localTestMu.Unlock()
	if session == nil {
		return bot.LocalChatInfo{}, nil
	}
	return session.Snapshot(), nil
}

// OpenAPIChatDataFolder seeds and opens the editable API Chat data folder.
func (a *App) OpenAPIChatDataFolder() (string, error) {
	dataRoot, err := bot.EnsureAPIChatData(a.activeSiteDataDir())
	if err != nil {
		return "", err
	}
	if err := exec.Command("explorer.exe", dataRoot).Start(); err != nil {
		return "", err
	}
	summary := fmt.Sprintf("API Chat data folder ready: %s", dataRoot)
	a.log("info", summary)
	return summary, nil
}

func (a *App) CleanProfiles() error {
	a.mu.Lock()
	running := a.running
	a.mu.Unlock()

	if running {
		return fmt.Errorf("stop the bot before cleaning profile data")
	}

	if err := a.removeProfileData(); err != nil {
		return err
	}

	a.clearActiveWorkers()

	a.log("info", "Profile data cleaned.")
	return nil
}

func (a *App) StartBot() error {
	a.mu.Lock()
	if a.running {
		a.mu.Unlock()
		a.log("info", "Bot is already running.")
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	a.runID++
	runID := a.runID
	a.running = true
	a.botCancel = cancel
	a.mu.Unlock()

	// Each run starts with a fresh feed, exactly like a Show Mode run.
	a.clearBotLog()
	a.emitStatus(true)
	a.log("activity", "bot started wait to process")

	configs, err := a.buildBotConfigs()
	if err != nil {
		a.mu.Lock()
		a.running = false
		a.botCancel = nil
		if a.runID == runID {
			a.runID++
		}
		a.mu.Unlock()
		a.emitStatus(false)
		a.botLogEvent("error", "bot start failed", "", err.Error(), false)
		return err
	}

	a.log("info", fmt.Sprintf("Starting %d worker(s) across all discovered profiles.", len(configs)))
	a.botLogEvent("system", "bot started", "", fmt.Sprintf("%d worker(s)", len(configs)), false)

	var wg sync.WaitGroup
	wg.Add(len(configs))

	for _, cfg := range configs {
		workerCfg := cfg
		profileName := workerCfg.Profile
		a.setWorkerRunning(profileName, true)
		go func() {
			defer func() {
				a.setWorkerRunning(profileName, false)
				wg.Done()
			}()

			err := bot.Run(ctx, workerCfg)
			if errors.Is(err, context.Canceled) {
				a.log("info", fmt.Sprintf("[%s] Worker stopped.", profileName))
				a.botLogEvent("system", "worker stopped", profileName, "", false)
				return
			}
			if err != nil {
				a.log("error", fmt.Sprintf("[%s] Worker stopped with error: %v", profileName, err))
				a.botLogEvent("error", "worker stopped with error", profileName, err.Error(), false)
				return
			}
			a.log("info", fmt.Sprintf("[%s] Worker finished.", profileName))
			a.botLogEvent("system", "worker finished", profileName, "", false)
		}()
	}

	go func() {
		wg.Wait()

		a.mu.Lock()
		if a.runID == runID {
			a.running = false
			a.botCancel = nil
			a.runID++
			a.mu.Unlock()
			a.log("info", "All bot workers stopped.")
			a.botLogEvent("system", "all workers stopped", "", "", false)
			a.emitStatus(false)
			return
		}
		a.mu.Unlock()
	}()

	return nil
}

func (a *App) StopBot() error {
	a.mu.Lock()
	cancel := a.botCancel
	if cancel == nil {
		a.mu.Unlock()
		return nil
	}
	a.botCancel = nil
	a.running = false
	a.mu.Unlock()

	cancel()
	a.clearActiveWorkers()
	a.log("info", "Stopping all bot workers.")
	a.botLogEvent("system", "stop requested", "", "", false)
	a.emitStatus(false)
	return nil
}

func (a *App) seedDefaults() error {
	if _, err := a.uiStore.LoadBrowserSettings(); err != nil {
		return err
	}
	if _, err := a.uiStore.LoadMessageSettings(); err != nil {
		return err
	}
	if _, err := a.uiStore.LoadProxySettings(); err != nil {
		return err
	}
	if _, err := a.uiStore.LoadGeneralSettings(); err != nil {
		return err
	}
	return nil
}

func (a *App) removeProfileData() error {
	list, err := sites.List(a.dataDir)
	if err != nil {
		return err
	}
	for _, cfg := range list {
		if err := sites.CleanSiteRuntime(a.dataDir, cfg.ID); err != nil {
			return fmt.Errorf("clean site %s: %w", cfg.ID, err)
		}
	}
	return nil
}

func (a *App) buildBotConfigs() ([]bot.Config, error) {
	browserSettings, err := a.uiStore.LoadBrowserSettings()
	if err != nil {
		return nil, err
	}

	// Effective values: global settings merged with this site's override.
	messageSettings, err := a.effectiveMessageSettings(a.activeSiteID())
	if err != nil {
		return nil, err
	}

	generalSettings, err := a.effectiveGeneralSettings(a.activeSiteID())
	if err != nil {
		return nil, err
	}

	proxyEntries, err := a.loadProxyEntries()
	if err != nil {
		return nil, err
	}

	var evaChatConfig *bot.EVAChatConfig
	var apiChatConfig *bot.APIChatConfig
	switch messageSettings.EffectiveChatMode() {
	case model.ChatModeEVAChat:
		dataRoot, err := bot.EnsureEVAChatData(a.activeSiteDataDir())
		if err != nil {
			return nil, err
		}
		engine, err := bot.LoadEVAChatEngine(dataRoot)
		if err != nil {
			return nil, err
		}
		evaChatConfig = &bot.EVAChatConfig{DataRoot: dataRoot, Engine: engine}
		a.log("info", fmt.Sprintf("EVAChat Flow ready: %d trigger categories and %d reply pools loaded from %s.", len(engine.InputCategories()), len(engine.OutputPools()), dataRoot))
	case model.ChatModeAPI:
		loaded, err := bot.LoadAPIChatConfig(a.activeSiteDataDir(), messageSettings)
		if err != nil {
			return nil, err
		}
		apiChatConfig = loaded
		a.log("info", fmt.Sprintf("API Chat ready: %s.", bot.APIChatDescribe(apiChatConfig)))
	}

	siteDataDir := a.activeSiteDataDir()
	siteCfg := a.activeSiteConfig()
	profiles, err := workerplan.DiscoverProfileWorkers(siteDataDir, browserSettings.Profile)
	if err != nil {
		return nil, err
	}

	assignments := workerplan.AssignProfileWorkers(profiles, proxyEntries)
	configs := make([]bot.Config, 0, len(assignments))

	for _, assignment := range assignments {
		cfg := bot.DefaultConfig(
			siteDataDir,
			assignment.Profile,
			browserSettings.Backend,
			browserSettings.Headless,
		)
		if strings.TrimSpace(siteCfg.AppURL) != "" {
			cfg.AppURL = siteCfg.AppURL
		}
		if strings.TrimSpace(siteCfg.APIBaseURL) != "" {
			cfg.APIBaseURL = siteCfg.APIBaseURL
		}
		cfg.LoginInfoPath = assignment.LoginInfoPath
		cfg.ChatMode = messageSettings.EffectiveChatMode()
		cfg.MessageSequence = nil
		if cfg.ChatMode == model.ChatModeEVAChat {
			cfg.EVAChat = evaChatConfig
			// EVAChat has its own persisted stage state. Keep MessageSequence
			// empty so sequence code cannot accidentally drive this mode.
			cfg.AvailableSequences = nil
		} else if cfg.ChatMode == model.ChatModeAPI {
			cfg.APIChat = apiChatConfig
			// API Chat is model-driven end to end: the TXT sequences are not
			// involved, only the fallback deck inside data/api_chat.
			cfg.AvailableSequences = nil
		} else if cfg.ChatMode == model.ChatModeFolder {
			if len(messageSettings.LoadedSequences) == 0 {
				return nil, fmt.Errorf("Folder Sequences mode is selected, but no sequence folder is loaded")
			}
			cfg.AvailableSequences = make([][]string, len(messageSettings.LoadedSequences))
			for i, seqStr := range messageSettings.LoadedSequences {
				cfg.AvailableSequences[i] = model.MessageSequenceLines(seqStr)
			}
		} else {
			cfg.AvailableSequences = [][]string{messageSettings.SequenceLines()}
		}
		cfg.MessageUsernames = messageSettings.UsernameLines()
		cfg.FirstMessage = generalSettings.FirstMessage.Effective()
		cfg.MatchWhenIdle = true
		cfg.PollMin = time.Duration(generalSettings.PollMinSeconds) * time.Second
		cfg.PollMax = time.Duration(generalSettings.PollMaxSeconds) * time.Second
		cfg.ActionMin = time.Duration(generalSettings.ActionMinSeconds) * time.Second
		cfg.ActionMax = time.Duration(generalSettings.ActionMaxSeconds) * time.Second
		cfg.MatchReplyTimeoutMin = time.Duration(generalSettings.MatchReplyTimeoutMinSeconds) * time.Second
		cfg.MatchReplyTimeoutMax = time.Duration(generalSettings.MatchReplyTimeoutMaxSeconds) * time.Second
		cfg.MatchCloseWaitMin = time.Duration(generalSettings.MatchCloseWaitMinSeconds) * time.Second
		cfg.MatchCloseWaitMax = time.Duration(generalSettings.MatchCloseWaitMaxSeconds) * time.Second
		cfg.ReplyDelayMin = time.Duration(generalSettings.ReplyDelayMinSeconds) * time.Second
		cfg.ReplyDelayMax = time.Duration(generalSettings.ReplyDelayMaxSeconds) * time.Second

		profileName := assignment.Profile
		proxyLabel := "direct"
		if assignment.HasProxy {
			assignedProxy := assignment.Proxy
			cfg.ProxyRotator = proxy.NewRotator([]proxy.Entry{assignedProxy})
			proxyLabel = assignedProxy.Label()
		}

		cfg.Logf = func(format string, args ...any) {
			message := fmt.Sprintf(strings.TrimSuffix(format, "\n"), args...)
			a.log("info", fmt.Sprintf("[%s] %s", profileName, message))
			a.botWorkerLog(profileName, message)
		}
		cfg.Activityf = func(format string, args ...any) {
			message := fmt.Sprintf(strings.TrimSuffix(format, "\n"), args...)
			a.log("activity", message)
			a.botWorkerActivity(profileName, message)
		}

		if assignment.LoginInfoPath != "" {
			a.log("info", fmt.Sprintf("Prepared worker for user/profile %s using %s from %s.", profileName, proxyLabel, assignment.LoginInfoPath))
		} else {
			a.log("info", fmt.Sprintf("Prepared worker for profile %s using %s.", profileName, proxyLabel))
		}
		configs = append(configs, cfg)
	}

	return configs, nil
}

func safeProfileName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.ReplaceAll(name, "..", "_")
	name = strings.NewReplacer("\\", "_", "/", "_", ":", "_", "*", "_", "?", "_", "\"", "_", "<", "_", ">", "_", "|", "_").Replace(name)
	name = strings.Map(func(r rune) rune {
		if r < 32 {
			return '_'
		}
		return r
	}, name)
	// Win32 ignores trailing spaces/dots and reserves device names even when
	// they have an extension (for example CON.txt). Keep profiles unambiguous.
	name = strings.TrimRight(name, ". ")
	if name == "" {
		return "default"
	}
	stem := strings.ToUpper(strings.SplitN(name, ".", 2)[0])
	reserved := stem == "CON" || stem == "PRN" || stem == "AUX" || stem == "NUL"
	if len(stem) == 4 && (strings.HasPrefix(stem, "COM") || strings.HasPrefix(stem, "LPT")) && stem[3] >= '1' && stem[3] <= '9' {
		reserved = true
	}
	if reserved {
		name = "_" + name
	}
	return name
}

func (a *App) runFreshBrowser(profileDir string, profileName string, settings model.BrowserSettings, selectedProxy *proxy.Entry, index int, total int) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a.runFreshBrowserWithBackend(ctx, profileDir, profileName, settings, selectedProxy, index, total, browser.New)
}

// Production and browser-free tests share the entire capture/save/cleanup path.
// Only backend construction is injected; no global factory or fake production mode.
func (a *App) runFreshBrowserWithBackend(ctx context.Context, profileDir string, profileName string, settings model.BrowserSettings, selectedProxy *proxy.Entry, index int, total int, newBackend func(string) (browser.Backend, error)) {
	proxyLabel := "direct"
	if selectedProxy != nil {
		proxyLabel = selectedProxy.Label()
	}
	if settings.LiveChat {
		a.clearShowLog()
		a.setShowModeRunning(true)
		defer a.setShowModeRunning(false)
		a.showModeEvent("system", "run requested", "", fmt.Sprintf("profile=%s · proxy=%s · browser=%s", profileName, proxyLabel, settings.Backend), false)
	}

	// Keep disposable browser data outside the repo so wails dev file watching
	// doesn't restart the app when a fresh browser run creates a new directory.
	tempPattern := fmt.Sprintf("chitchat-fresh-browser-%s-%02d-", profileName, index)
	browserTempDir, err := os.MkdirTemp("", tempPattern)
	if err != nil {
		// Low-integrity launch tokens and some AV policies deny %TEMP%. The
		// app's own data directory is writable (the settings store lives
		// there), so use it for the disposable browser profile instead.
		fallbackRoot := filepath.Join(a.activeSiteDataDir(), "tmp")
		if mkErr := store.EnsureDir(fallbackRoot); mkErr != nil {
			a.log("error", fmt.Sprintf("[%s fresh %d/%d] Failed to create temp browser dir: %v (fallback: %v)", profileName, index, total, err, mkErr))
			a.showLiveError("could not create a temporary browser profile", mkErr)
			return
		}
		browserTempDir, err = os.MkdirTemp(fallbackRoot, tempPattern)
		if err != nil {
			a.log("error", fmt.Sprintf("[%s fresh %d/%d] Failed to create temp browser dir: %v", profileName, index, total, err))
			a.showLiveError("could not create a temporary browser profile", err)
			return
		}
		a.log("info", fmt.Sprintf("[%s fresh %d/%d] System temp is not writable; using %s", profileName, index, total, browserTempDir))
	}

	// Register cleanup immediately, including failures before backend creation.
	defer a.removeFreshBrowserData(browserTempDir, profileName, index, total)

	userDataDir := filepath.Join(browserTempDir, "user-data")
	if err := store.EnsureDir(userDataDir); err != nil {
		a.log("error", fmt.Sprintf("[%s fresh %d/%d] Failed to create temp browser dir: %v", profileName, index, total, err))
		a.showLiveError("could not prepare the browser profile", err)
		return
	}

	backend, err := newBackend(settings.Backend)
	if err != nil {
		a.log("error", fmt.Sprintf("[%s fresh %d/%d] Failed to create browser backend: %v", profileName, index, total, err))
		a.showLiveError("could not create the browser backend", err)
		return
	}
	// LIFO defers close the browser before removing its files.
	defer func() { _ = backend.Close() }()

	siteCfg := a.activeSiteConfig()
	if configurer, ok := backend.(browser.SiteDOMConfigurer); ok && siteCfg.ID != sites.DefaultSiteID {
		configurer.ConfigureSiteDOM(sites.CustomDOMDriverJS(a.dataDir, siteCfg.ID), siteCfg.Selectors.ComposerInput)
	}
	startURL := strings.TrimSpace(siteCfg.StartURL)
	if startURL == "" {
		startURL = "https://app.chitchat.gg/text?auth=email"
	}

	a.log("info", fmt.Sprintf(
		"[%s fresh %d/%d] Opening brand-new %s browser for site %s (%s). Old profile data is not used.",
		profileName,
		index,
		total,
		settings.Backend,
		siteCfg.Name,
		startURL,
	))
	if selectedProxy != nil {
		a.log("info", fmt.Sprintf("[%s fresh %d/%d] Using proxy %s", profileName, index, total, selectedProxy.Label()))
	}

	if err := backend.Start(ctx, browser.Options{
		StartURL:            startURL,
		UserDataDir:         userDataDir,
		Headless:            settings.Headless,
		DisablePageRecovery: true,
		Proxy:               selectedProxy,
	}); err != nil {
		a.log("error", fmt.Sprintf("[%s fresh %d/%d] Browser start failed: %v", profileName, index, total, err))
		a.showLiveError("browser start failed", err)
		return
	}
	openedDetail := fmt.Sprintf("site=%s · fresh session", siteCfg.Name)
	if settings.UseSavedSession {
		openedDetail = fmt.Sprintf("site=%s · checking saved profile session", siteCfg.Name)
	}
	a.showModeEvent("system", "browser opened", "", openedDetail, false)

	login, restored := model.LoginInfo{}, false
	if siteCfg.AuthMode == "dom_only" && settings.LiveChat {
		if settings.UseSavedSession {
			savedLogin, path, ok := sites.LoadSiteSession(a.dataDir, siteCfg.ID, profileName)
			if !ok {
				savedLogin, path, ok = latestProfileLogin(profileDir)
			}
			if ok {
				if seeder, canSeed := backend.(browser.SessionSeeder); canSeed {
					if seedErr := seeder.SeedSession(ctx, savedLogin); seedErr == nil {
						login = savedLogin
						a.log("info", fmt.Sprintf("[%s] Seeded saved site session from %s for %s.", profileName, path, siteCfg.Name))
						a.showModeEvent("system", "session restored", "", fmt.Sprintf("site=%s · profile=%s", siteCfg.Name, profileName), false)
					}
				}
			}
		}
		// DOM-only sites chat directly inside the visible browser without
		// requiring ChitChat's internal email/JWT token capture.
		a.showModeEvent("system", "chat watcher starting", "", fmt.Sprintf("site=%s · DOM mode", siteCfg.Name), false)
		a.runLiveChat(ctx, backend, login, profileName, index, total, proxyLabel)
		if snap, snapErr := backend.CaptureSession(ctx); snapErr == nil && (len(snap.Cookies) > 0 || len(snap.LocalStorage) > 0) {
			_, _ = sites.SaveSiteSession(a.dataDir, siteCfg.ID, profileName, snap)
		}
		return
	}
	if settings.LiveChat && settings.UseSavedSession {
		login, restored = a.restoreProfileSession(ctx, backend, profileDir, profileName, settings.Backend)
	}
	if !restored {
		a.showModeEvent("system", "browser opened", "", "waiting for manual login / account setup", false)
		var err error
		if siteCfg.AuthMode == "dom_only" {
			login, err = a.captureDOMSiteSession(ctx, backend, siteCfg, settings.Backend, profileName, index, total)
		} else {
			login, err = a.captureFreshLogin(ctx, backend, settings.Backend, profileName, index, total)
		}
		if err != nil {
			var banned *browser.BannedError
			if errors.As(err, &banned) {
				a.handleBanned(profileDir, profileName, banned.Report)
				return
			}
			if errors.Is(err, browser.ErrPageUnavailable) || errors.Is(err, context.Canceled) {
				a.log("info", fmt.Sprintf("[%s fresh %d/%d] Fresh browser was closed before login was saved.", profileName, index, total))
				a.showModeEvent("system", "browser closed before login was saved", "", "", false)
				return
			}
			a.log("error", fmt.Sprintf("[%s fresh %d/%d] Fresh login capture failed: %v", profileName, index, total, err))
			a.showLiveError("login capture failed", err)
			return
		}

		login.Profile = profileName
		login.Backend = settings.Backend
		login.UserID = login.CurrentUserID()
		login.SavedAt = time.Now().UTC()
		loginPath := a.freshLoginInfoPath(profileDir, login, index)

		if err := store.SaveJSON(loginPath, login); err != nil {
			a.log("error", fmt.Sprintf("[%s fresh %d/%d] Failed to save login info: %v", profileName, index, total, err))
			a.showLiveError("could not save the login session", err)
			return
		}
		_, _ = sites.RecordBrowserSetupAccount(a.dataDir, siteCfg.ID, profileName, login, loginPath)
		_, _ = sites.SaveSiteSession(a.dataDir, siteCfg.ID, profileName, login)

		a.log("info", fmt.Sprintf(
			"[%s fresh %d/%d] Login saved to %s (and sites/%s/sessions/). Temporary browser data will be removed after close.",
			profileName,
			index,
			total,
			loginPath,
			siteCfg.ID,
		))
		a.showModeEvent("system", "login saved", "", loginPath, false)
	}

	// Show Mode: the browser stays open and chats in place. This blocks until
	// the user closes the browser window or the run is cancelled; the deferred
	// backend.Close above then performs the usual cleanup.
	if settings.LiveChat {
		a.showModeEvent("system", "chat watcher starting", "", "", false)
		a.runLiveChat(ctx, backend, login, profileName, index, total, proxyLabel)
	}
}

// restoreProfileSession injects the newest saved login for this profile into
// the fresh browser. Failure is not fatal: the caller falls back to manual
// login, so a stale/expired session never blocks a run.
func (a *App) restoreProfileSession(ctx context.Context, backend browser.Backend, profileDir, profileName, backendName string) (model.LoginInfo, bool) {
	login, path, ok := latestProfileLogin(profileDir)
	if !ok {
		return model.LoginInfo{}, false
	}
	seeder, ok := backend.(browser.SessionSeeder)
	if !ok {
		a.log("info", fmt.Sprintf("[%s] Saved session found, but %s does not support session restore; manual login is required.", profileName, backendName))
		return model.LoginInfo{}, false
	}
	a.showModeEvent("system", "saved session found", "", filepath.Base(path)+" · restoring", false)
	if err := seeder.SeedSession(ctx, login); err != nil {
		a.log("error", fmt.Sprintf("[%s] Saved session restore failed: %v; falling back to manual login.", profileName, err))
		a.showModeEvent("system", "session restore failed", "", "manual login required", false)
		return model.LoginInfo{}, false
	}
	ready, err := browser.SessionAuthenticated(ctx, backend, "https://api.chitchat.gg")
	if err != nil || !ready {
		a.log("info", fmt.Sprintf("[%s] Saved session is no longer valid; manual login is required.", profileName))
		a.showModeEvent("system", "saved session expired", "", "manual login required", false)
		return model.LoginInfo{}, false
	}
	login.Profile = profileName
	login.Backend = backendName
	login.UserID = login.CurrentUserID()
	a.log("info", fmt.Sprintf("[%s] Session restored from %s; no manual login needed.", profileName, path))
	a.showModeEvent("system", "session restored", "", "auto-login from saved profile", false)
	return login, true
}

func (a *App) captureFreshLogin(
	ctx context.Context,
	backend browser.Backend,
	backendName string,
	profileName string,
	index int,
	total int,
) (model.LoginInfo, error) {
	waitingForTokenLogged := false
	waitingForSessionLogged := false

	for {
		if checker, ok := backend.(browser.BanChecker); ok {
			if report, banErr := checker.CheckBan(ctx); banErr == nil && report.Banned {
				return model.LoginInfo{}, &browser.BannedError{Report: report}
			}
		}
		login, err := backend.CaptureSession(ctx)
		if err != nil {
			return model.LoginInfo{}, err
		}

		token := login.Token()
		if token != "" {
			ready, authErr := browser.SessionAuthenticated(ctx, backend, "https://api.chitchat.gg")
			if authErr == nil && ready {
				login.UserID = login.CurrentUserID()
				if login.UserID == "" {
					return model.LoginInfo{}, fmt.Errorf("login detected, but current bot user ID could not be read from token or browser storage")
				}
				return login, nil
			}
		}

		if token == "" && !waitingForTokenLogged {
			a.log("info", fmt.Sprintf(
				"[%s fresh %d/%d] Waiting for login token in the new %s browser window.",
				profileName,
				index,
				total,
				backendName,
			))
			waitingForTokenLogged = true
		}

		if token != "" && !waitingForSessionLogged {
			a.log("info", fmt.Sprintf(
				"[%s fresh %d/%d] Token detected, but the session is still unauthorized. Finish the full login in the %s browser window.",
				profileName,
				index,
				total,
				backendName,
			))
			waitingForSessionLogged = true
		}

		select {
		case <-ctx.Done():
			return model.LoginInfo{}, ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}

// captureDOMSiteSession watches a non-ChitChat browser during first-run account
// setup ("browser_setup"), capturing cookies/localStorage as the user logs in
// or completes verification, and saving the last valid snapshot when ready or closed.
func (a *App) captureDOMSiteSession(
	ctx context.Context,
	backend browser.Backend,
	siteCfg sites.Config,
	backendName string,
	profileName string,
	index int,
	total int,
) (model.LoginInfo, error) {
	a.log("info", fmt.Sprintf(
		"[%s fresh %d/%d] Browser opened for %s (%s). Complete account login or verification in the browser; session will be saved to sites/%s/ automatically.",
		profileName,
		index,
		total,
		siteCfg.Name,
		siteCfg.StartURL,
		siteCfg.ID,
	))
	var lastValid model.LoginInfo
	hasSnapshot := false

	finalizeSnapshot := func(snap model.LoginInfo) model.LoginInfo {
		if snap.LocalStorage == nil {
			snap.LocalStorage = map[string]string{}
		}
		userID := strings.TrimSpace(snap.CurrentUserID())
		if userID == "" {
			userID = fmt.Sprintf("%s_%s_%02d", siteCfg.ID, profileName, index)
		}
		snap.UserID = userID
		snap.LocalStorage["cachedUserId"] = userID
		if snap.Token() == "" {
			snap.Cookies = append(snap.Cookies, model.Cookie{
				Name:  "token",
				Value: fmt.Sprintf("dom_session_%s_%d", siteCfg.ID, time.Now().Unix()),
				Path:  "/",
			})
		}
		return snap
	}

	for {
		login, err := backend.CaptureSession(ctx)
		if err != nil {
			if hasSnapshot && (errors.Is(err, browser.ErrPageUnavailable) || errors.Is(err, context.Canceled)) {
				return finalizeSnapshot(lastValid), nil
			}
			return model.LoginInfo{}, err
		}
		if len(login.Cookies) > 0 || len(login.LocalStorage) > 0 || len(login.SessionStorage) > 0 {
			lastValid = login
			if !hasSnapshot {
				_, _ = sites.SaveSiteSession(a.dataDir, siteCfg.ID, profileName, finalizeSnapshot(login))
			}
			hasSnapshot = true
		}
		if login.Token() != "" && login.CurrentUserID() != "" {
			return finalizeSnapshot(login), nil
		}

		select {
		case <-ctx.Done():
			if hasSnapshot {
				return finalizeSnapshot(lastValid), nil
			}
			return model.LoginInfo{}, ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}

// ProfileOption is safe profile metadata for the UI: never cookies or tokens.
type ProfileOption struct {
	Name       string `json:"name"`
	HasSession bool   `json:"has_session"`
	SavedAt    string `json:"saved_at,omitempty"`
}

// ListProfiles lists the saved browser-profile directories so the UI can show
// a picker. A profile with a saved login is auto-restored by Run Browser.
func (a *App) ListProfiles() ([]ProfileOption, error) {
	profilesDir := filepath.Join(a.activeSiteDataDir(), "profiles")
	entries, err := os.ReadDir(profilesDir)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	options := make([]ProfileOption, 0, len(entries)+1)
	seen := map[string]bool{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := strings.TrimSpace(entry.Name())
		if name == "" {
			continue
		}
		seen[name] = true
		option := ProfileOption{Name: name}
		if login, path, ok := latestProfileLogin(filepath.Join(profilesDir, name)); ok {
			option.HasSession = true
			if !login.SavedAt.IsZero() {
				option.SavedAt = login.SavedAt.Local().Format("15:04")
			} else if info, statErr := os.Stat(path); statErr == nil {
				option.SavedAt = info.ModTime().Format("15:04")
			}
		}
		options = append(options, option)
	}
	if !seen["default"] {
		options = append(options, ProfileOption{Name: "default"})
	}
	sort.Slice(options, func(i, j int) bool {
		if options[i].Name == "default" {
			return true
		}
		if options[j].Name == "default" {
			return false
		}
		return options[i].Name < options[j].Name
	})
	return options, nil
}

// latestProfileLogin returns the newest saved login that still has a token.
func latestProfileLogin(profileDir string) (model.LoginInfo, string, bool) {
	paths := loginInfoFiles(profileDir)
	for _, path := range paths {
		var login model.LoginInfo
		ok, err := store.LoadJSON(path, &login)
		if err != nil || !ok || login.Token() == "" {
			continue
		}
		return login, path, true
	}
	return model.LoginInfo{}, "", false
}

func loginInfoFiles(profileDir string) []string {
	matches, _ := filepath.Glob(filepath.Join(profileDir, "login_info*.json"))
	sort.Slice(matches, func(i, j int) bool {
		left, leftErr := os.Stat(matches[i])
		right, rightErr := os.Stat(matches[j])
		if leftErr != nil || rightErr != nil {
			return matches[i] > matches[j]
		}
		return left.ModTime().After(right.ModTime())
	})
	return matches
}

// handleBanned stops the current run, reports the site's ban details and
// deletes the profile's saved session so the banned account is not retried.
func (a *App) handleBanned(profileDir, profileName string, report browser.BanReport) {
	detail := report.Summary()
	a.log("error", fmt.Sprintf("[%s] BANNED | %s", profileName, detail))
	a.showModeEvent("error", "BANNED — account suspended", "", detail, false)
	a.botLogEvent("error", "BANNED — account suspended", profileName, detail, false)
	paths := loginInfoFiles(profileDir)
	targets := paths
	if report.UserID != "" {
		targets = make([]string, 0, 1)
		for _, path := range paths {
			var login model.LoginInfo
			ok, err := store.LoadJSON(path, &login)
			if err != nil || !ok {
				continue
			}
			if strings.EqualFold(login.CurrentUserID(), report.UserID) {
				targets = append(targets, path)
			}
		}
	}
	removed := 0
	for _, path := range targets {
		if err := os.Remove(path); err == nil {
			removed++
		} else {
			a.log("error", fmt.Sprintf("[%s] Could not delete banned session %s: %v", profileName, path, err))
		}
	}
	a.log("warn", fmt.Sprintf("[%s] Removed %d saved session file(s) after ban (user=%s).", profileName, removed, report.UserID))
	a.showModeEvent("system", "saved session deleted", "", fmt.Sprintf("%s · %d file(s) removed", profileName, removed), false)
}

func (a *App) freshLoginInfoPath(loginDir string, login model.LoginInfo, index int) string {
	userID := sanitizeFreshFileSegment(login.CurrentUserID())
	if userID == "" {
		userID = fmt.Sprintf("browser-%02d", index)
	}

	stamp := fmt.Sprintf("%s-%09d", login.SavedAt.UTC().Format("20060102-150405"), login.SavedAt.UTC().Nanosecond())
	filename := fmt.Sprintf("login_info_%s_%s_%02d.json", userID, stamp, index)
	return filepath.Join(loginDir, filename)
}

func sanitizeFreshFileSegment(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}

	value = strings.ReplaceAll(value, "..", "_")
	value = strings.NewReplacer("\\", "_", "/", "_", ":", "_", "*", "_", "?", "_", "\"", "_", "<", "_", ">", "_", "|", "_", " ", "_").Replace(value)
	return strings.Trim(value, "._")
}

func (a *App) removeFreshBrowserData(tempDir string, profileName string, index int, total int) {
	var lastErr error
	for attempt := 0; attempt < 5; attempt++ {
		if err := os.RemoveAll(tempDir); err == nil {
			a.log("info", fmt.Sprintf(
				"[%s fresh %d/%d] Removed temporary browser data from %s",
				profileName,
				index,
				total,
				tempDir,
			))
			return
		} else {
			lastErr = err
			time.Sleep(300 * time.Millisecond)
		}
	}

	if lastErr != nil {
		a.log("warn", fmt.Sprintf(
			"[%s fresh %d/%d] Could not remove temporary browser data from %s: %v",
			profileName,
			index,
			total,
			tempDir,
			lastErr,
		))
	}
}

func (a *App) loadProxyRotator() (*proxy.Rotator, int, error) {
	entries, err := a.loadProxyEntries()
	if err != nil {
		return nil, 0, err
	}
	if len(entries) == 0 {
		return nil, 0, nil
	}
	return proxy.NewRotator(entries), len(entries), nil
}

func (a *App) loadProxyEntries() ([]proxy.Entry, error) {
	proxySettings, err := a.effectiveProxySettings(a.activeSiteID())
	if err != nil {
		return nil, err
	}
	if !proxySettings.Enabled {
		return nil, nil
	}

	entries, err := proxy.ParseList(proxySettings.Entries)
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("proxy list is enabled but empty")
	}

	return entries, nil
}

func (a *App) emitStatus(running bool) {
	a.emit("ui:running", running)
}

func (a *App) emit(name string, data any) {
	if a.ctx == nil {
		return
	}
	runtime.EventsEmit(a.ctx, name, data)
}

func (a *App) log(level string, message string) {
	// Persist first, independently of Badger/UI events; no Home visibility filter.
	disk := applog.Logger{Path: applog.Path(a.dataDir), Source: "app"}
	disk.Log(level, message)
	entry, err := a.uiStore.AppendLog(level, message)
	if err == nil {
		siteCfg := a.activeSiteConfig()
		entry.SiteID = siteCfg.ID
		entry.SiteName = siteCfg.Name
		entry.Group = siteCfg.Classification.Group
		entry.Type = normalizeCombinedType(level)
		a.emit("ui:log", entry) // Browser status still consumes this event.
	} else {
		disk.Error(fmt.Sprintf("UI log persistence failed: %v", err))
	}
}

// Separate read-only display API; settings/stats/logs contracts are unchanged.
func (a *App) GetLiveChatSnapshot(sessionID string) homeChatSnapshot {
	return a.homeChatFeed().snapshot(sessionID)
}
