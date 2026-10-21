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
	"chitchatbot/internal/sites"
	"chitchatbot/internal/store"
	"chitchatbot/internal/uistore"
	"chitchatbot/internal/workerplan"
)

type App struct {
	ctx             context.Context
	dataDir         string
	uiStore         *uistore.Store
	mu              sync.Mutex
	localTestMu     sync.Mutex
	localTest       *bot.LocalChatSession
	chatFeed        *homeChatFeed
	showLog         *showLogFeed
	botLog          *botLogFeed
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
	_, _ = sites.EnsureSites(dataDir)
	_, _ = uiStore.LoadMessageSettings()

	return app, nil
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
	return model.SiteOption{
		ID:              c.ID,
		Name:            c.Name,
		Family:          c.Family,
		AccountMode:     c.AccountMode,
		AccountCount:    c.AccountCount,
		HasSavedSession: c.HasSavedSession,
		SessionSavedAt:  c.SessionSavedAt,
		SessionPath:     c.SessionPath,
		StartURL:        c.StartURL,
		AuthMode:        c.AuthMode,
		EntryFlow:       c.EntryFlow,
		FolderPath:      c.FolderPath,
	}
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
	msgSettings, _ := a.uiStore.LoadMessageSettings()
	proxySettings, _ := a.uiStore.LoadProxySettings()
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

	showSettings, err := a.uiStore.LoadShowModeSettings()
	if err != nil {
		return model.UISnapshot{}, err
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
	a.log("info", fmt.Sprintf("Message settings saved. Mode=%s", settings.EffectiveChatMode()))
	return nil
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

	if settings.Enabled {
		entries, _ := proxy.ParseList(settings.Entries)
		a.log("info", fmt.Sprintf("Proxy settings saved. %d proxy line(s) enabled for Rod, ChromeDP, and Go request rotation.", len(entries)))
	} else {
		a.log("info", "Proxy settings saved. Proxy use is disabled.")
	}
	return nil
}

func (a *App) SaveShowModeSettings(settings model.ShowModeSettings) error {
	return a.uiStore.SaveShowModeSettings(settings)
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
	a.log("info", fmt.Sprintf(
		"Timing settings saved. Poll=%ds-%ds Action=%ds-%ds",
		settings.PollMinSeconds,
		settings.PollMaxSeconds,
		settings.ActionMinSeconds,
		settings.ActionMaxSeconds,
	))
	return nil
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

	messageSettings, err := a.uiStore.LoadMessageSettings()
	if err != nil {
		return nil, err
	}

	generalSettings, err := a.uiStore.LoadGeneralSettings()
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
	proxySettings, err := a.uiStore.LoadProxySettings()
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
		a.emit("ui:log", entry) // Browser status still consumes this event.
	} else {
		disk.Error(fmt.Sprintf("UI log persistence failed: %v", err))
	}
}

// Separate read-only display API; settings/stats/logs contracts are unchanged.
func (a *App) GetLiveChatSnapshot(sessionID string) homeChatSnapshot {
	return a.homeChatFeed().snapshot(sessionID)
}
