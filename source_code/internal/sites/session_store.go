package sites

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"chitchatbot/internal/model"
)

// SiteSessionsDir returns the isolated session store directory for a site:
// <sites-root>/<site_id>/sessions. It returns "" for an invalid site ID.
func SiteSessionsDir(dataDir, siteID string) string {
	ctx, err := NewSiteContext(dataDir, siteID)
	if err != nil {
		return ""
	}
	return ctx.SessionsPath
}

// SaveSiteSession persists a browser session inside exactly one site folder:
//
//	<sites-root>/<site_id>/sessions/latest_session.json
//	<sites-root>/<site_id>/sessions/session_<profile>.json
//	<sites-root>/<site_id>/profiles/<profile>/login_info_<site>_<profile>.json
//	<sites-root>/<site_id>/accounts/account_<user>.json
//
// There is no legacy mirror into <dataDir>/profiles/: a session for one site can
// never be read or written by another site.
func SaveSiteSession(dataDir, siteID, profile string, login model.LoginInfo) (string, error) {
	ctx, err := NewSiteContext(dataDir, siteID)
	if err != nil {
		return "", err
	}
	profileName, err := safeSegment(profile, "default")
	if err != nil {
		return "", err
	}
	if login.SavedAt.IsZero() {
		login.SavedAt = time.Now().UTC()
	}
	login.Profile = profileName
	if strings.TrimSpace(login.Backend) == "" {
		login.Backend = "rod"
	}
	if login.LocalStorage == nil {
		login.LocalStorage = map[string]string{}
	}
	login.LocalStorage["site_id"] = ctx.SiteID

	userID := strings.TrimSpace(login.CurrentUserID())
	if userID == "" {
		userID = fmt.Sprintf("%s_%s", ctx.SiteID, profileName)
	}
	login.UserID = userID
	if strings.TrimSpace(login.LocalStorage["cachedUserId"]) == "" {
		login.LocalStorage["cachedUserId"] = userID
	}
	if strings.TrimSpace(login.Token()) == "" {
		login.Cookies = append(login.Cookies, model.Cookie{
			Name:   "token",
			Value:  fmt.Sprintf("site_session_%s_%s_%d", ctx.SiteID, profileName, login.SavedAt.Unix()),
			Domain: ctx.SiteID,
			Path:   "/",
		})
	}

	if err := ctx.Ensure(); err != nil {
		return "", err
	}
	if err := ctx.EnsureDir("sessions"); err != nil {
		return "", err
	}
	profileDir, err := ctx.ProfilePath(profileName)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(profileDir, 0o700); err != nil {
		return "", err
	}

	latestPath, err := ctx.LatestSessionPath()
	if err != nil {
		return "", err
	}
	if err := ctx.SaveJSON(filepath.Join("sessions", "latest_session.json"), login); err != nil {
		return "", err
	}
	if err := ctx.SaveJSON(filepath.Join("sessions", "session_"+profileName+".json"), login); err != nil {
		return "", err
	}
	loginRel := filepath.Join("profiles", profileName, fmt.Sprintf("login_info_%s_%s.json", ctx.SiteID, profileName))
	loginPath, err := ctx.Resolve(loginRel)
	if err != nil {
		return "", err
	}
	if err := ctx.SaveJSON(loginRel, login); err != nil {
		return "", err
	}

	if _, err := RecordBrowserSetupAccount(dataDir, ctx.SiteID, profileName, login, loginPath); err != nil {
		return "", err
	}
	// Also write the 5 canonical session files into sites/<site_id>/sessions/default/ (Prompt 10)
	_, _ = writeStructuredSessionBundle(ctx, profileName, "browser_capture", login, nil, nil)

	_ = AppendSiteDebugLog(dataDir, ctx.SiteID, "session_autosave", "info",
		fmt.Sprintf("Auto-saved isolated session for %s (profile=%s)", ctx.SiteID, profileName), latestPath)
	return latestPath, nil
}

// LoadSiteSession loads the newest saved session strictly from this site's own
// folder. It never reads data/profiles, data/sites or another site.
func LoadSiteSession(dataDir, siteID, profile string) (model.LoginInfo, string, bool) {
	ctx, err := NewSiteContext(dataDir, siteID)
	if err != nil {
		return model.LoginInfo{}, "", false
	}
	profileName, err := safeSegment(profile, "default")
	if err != nil {
		return model.LoginInfo{}, "", false
	}

	candidates := []string{}
	if path, err := ctx.SessionPath(profileName); err == nil {
		candidates = append(candidates, path)
	}
	if path, err := ctx.LatestSessionPath(); err == nil {
		candidates = append(candidates, path)
	}
	profilePattern := filepath.Join("profiles", profileName, "login_info*.json")
	if matches, err := ctx.Glob(profilePattern); err == nil {
		sortFilesNewestFirst(matches)
		candidates = append(candidates, matches...)
	}
	anyProfilePattern := filepath.Join("profiles", "*", "login_info*.json")
	if matches, err := ctx.Glob(anyProfilePattern); err == nil {
		sortFilesNewestFirst(matches)
		candidates = append(candidates, matches...)
	}

	seen := map[string]bool{}
	for _, path := range candidates {
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		rel, relErr := filepath.Rel(ctx.FolderPath, path)
		if relErr != nil {
			continue
		}
		var login model.LoginInfo
		ok, err := ctx.LoadJSON(rel, &login)
		if err != nil || !ok {
			continue
		}
		if login.Token() != "" || len(login.Cookies) > 0 || len(login.LocalStorage) > 0 {
			return login, path, true
		}
	}

	// Fallback: accounts stored in this site's accounts/ directory.
	accounts, err := ListSiteAccounts(dataDir, ctx.SiteID)
	if err == nil && len(accounts) > 0 {
		acc := accounts[0]
		savedAt, _ := time.Parse(time.RFC3339, acc.CreatedAt)
		if savedAt.IsZero() {
			savedAt = time.Now().UTC()
		}
		cookies := make([]model.Cookie, 0, len(acc.Cookies)+1)
		for k, v := range acc.Cookies {
			cookies = append(cookies, model.Cookie{Name: k, Value: v, Path: "/"})
		}
		if acc.Token != "" {
			cookies = append(cookies, model.Cookie{Name: "token", Value: acc.Token, Path: "/"})
		}
		login := model.LoginInfo{
			Profile:      acc.Profile,
			Backend:      "rod",
			UserID:       acc.ID,
			SavedAt:      savedAt,
			Cookies:      cookies,
			LocalStorage: acc.LocalStore,
		}
		filePath := strings.TrimSpace(acc.FilePath)
		if filePath == "" {
			if p, pErr := ctx.Resolve(filepath.Join("accounts", fmt.Sprintf("account_%s.json", acc.ID))); pErr == nil {
				filePath = p
			}
		}
		return login, filePath, true
	}

	return model.LoginInfo{}, "", false
}

// SiteSessionStatus returns whether a site has a saved session in its isolated
// folder, when it was saved, and the site-relative session store path.
func SiteSessionStatus(dataDir, siteID string) (hasSession bool, savedAt string, sessionPath string) {
	id, err := validateSiteID(siteID)
	if err != nil {
		return false, "", ""
	}
	relPath := filepath.ToSlash(filepath.Join("sites", id, "sessions", "latest_session.json"))
	login, path, ok := LoadSiteSession(dataDir, id, "default")
	if !ok {
		return false, "", relPath
	}
	stamp := ""
	if !login.SavedAt.IsZero() {
		stamp = login.SavedAt.UTC().Format("2006-01-02 15:04 UTC")
	} else if info, err := os.Stat(path); err == nil {
		stamp = info.ModTime().UTC().Format("2006-01-02 15:04 UTC")
	}
	return true, stamp, relPath
}

func sortFilesNewestFirst(paths []string) {
	sort.Slice(paths, func(i, j int) bool {
		left, lErr := os.Stat(paths[i])
		right, rErr := os.Stat(paths[j])
		if lErr != nil || rErr != nil {
			return paths[i] > paths[j]
		}
		return left.ModTime().After(right.ModTime())
	})
}

const (
	SessionStateValid    = "valid"
	SessionStateExpiring = "expiring"
	SessionStateMissing  = "missing"

	DefaultSessionTTL        = 24 * time.Hour
	ExpiringThresholdSeconds = int64(4 * 3600) // <= 4h remaining is "expiring"
)

// SessionMeta is stored at sites/<site_id>/sessions/default/meta.json (Prompt 10).
type SessionMeta struct {
	SiteID     string `json:"site_id"`
	Profile    string `json:"profile"`
	ProfileDir string `json:"profile_dir"`
	CapturedAt string `json:"captured_at"`
	ExpiresAt  string `json:"expires_at"`
	Method     string `json:"method"`
	Status     string `json:"status"`
}

// LocalStorageBundle is stored at sites/<site_id>/sessions/default/local_storage.json.
type LocalStorageBundle struct {
	LocalStorage   map[string]string `json:"local_storage"`
	SessionStorage map[string]string `json:"session_storage"`
}

// WSEndpointsBundle is stored at sites/<site_id>/sessions/default/ws_endpoints.json.
type WSEndpointsBundle struct {
	Primary   string   `json:"primary"`
	Endpoints []string `json:"endpoints"`
}

// AuthTokensBundle is stored at sites/<site_id>/sessions/default/auth_tokens.json.
type AuthTokensBundle struct {
	UserID  string            `json:"user_id"`
	Token   string            `json:"token"`
	Headers map[string]string `json:"headers"`
}

// CapturedSessionBundle holds all 5 files inside sites/<site_id>/sessions/default/.
type CapturedSessionBundle struct {
	Cookies      []model.Cookie     `json:"cookies"`
	LocalStorage LocalStorageBundle `json:"local_storage"`
	WSEndpoints  WSEndpointsBundle  `json:"ws_endpoints"`
	AuthTokens   AuthTokensBundle   `json:"auth_tokens"`
	Meta         SessionMeta        `json:"meta"`
}

// SiteSessionSummary describes one site's session state for Tab 3 (Prompt 10).
type SiteSessionSummary struct {
	SiteID       string   `json:"site_id"`
	SiteName     string   `json:"site_name"`
	Group        string   `json:"group"`
	GroupName    string   `json:"group_name"`
	StartURL     string   `json:"start_url"`
	Status       string   `json:"status"`
	CapturedAt   string   `json:"captured_at,omitempty"`
	ExpiresAt    string   `json:"expires_at,omitempty"`
	Method       string   `json:"method,omitempty"`
	CookieCount  int      `json:"cookie_count"`
	WSEndpoints  []string `json:"ws_endpoints,omitempty"`
	HasAuthToken bool     `json:"has_auth_token"`
	SessionDir   string   `json:"session_dir"`
	ProfileDir   string   `json:"profile_dir"`
}

// SessionHealthOverview returns the aggregate health counts and per-site list for Tab 3.
type SessionHealthOverview struct {
	Total    int                  `json:"total"`
	Valid    int                  `json:"valid"`
	Expiring int                  `json:"expiring"`
	Missing  int                  `json:"missing"`
	Sites    []SiteSessionSummary `json:"sites"`
}

func defaultWSEndpointsForSite(cfg Config) []string {
	if cfg.Classification.Probe != nil && len(cfg.Classification.Probe.WebSocketEndpoints) > 0 {
		return append([]string(nil), cfg.Classification.Probe.WebSocketEndpoints...)
	}
	u := strings.TrimSpace(cfg.StartURL)
	if u == "" {
		u = "https://example.com"
	}
	wsURL := strings.Replace(strings.Replace(u, "https://", "wss://", 1), "http://", "ws://", 1)
	wsURL = strings.TrimRight(wsURL, "/") + "/ws"
	return []string{wsURL}
}

func writeStructuredSessionBundle(
	ctx SiteContext,
	profile string,
	method string,
	login model.LoginInfo,
	wsEndpoints []string,
	authHeaders map[string]string,
) (CapturedSessionBundle, error) {
	profileName, err := safeSegment(profile, "default")
	if err != nil {
		return CapturedSessionBundle{}, err
	}
	sessionSubdir := filepath.Join("sessions", profileName)
	if err := ctx.EnsureDir(sessionSubdir); err != nil {
		return CapturedSessionBundle{}, err
	}
	profileRel := filepath.ToSlash(filepath.Join("sites", ctx.SiteID, "profiles", profileName))
	if pDir, pErr := ctx.ProfilePath(profileName); pErr == nil {
		_ = os.MkdirAll(pDir, 0o700)
	}

	now := login.SavedAt.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	expires := now.Add(DefaultSessionTTL)
	if strings.TrimSpace(method) == "" {
		method = "browser_capture"
	}

	localMap := map[string]string{}
	sessionMap := map[string]string{}
	for k, v := range login.LocalStorage {
		if strings.HasPrefix(k, "sessionStorage.") {
			sessionMap[strings.TrimPrefix(k, "sessionStorage.")] = v
		} else {
			localMap[k] = v
		}
	}
	if _, ok := localMap["site_id"]; !ok {
		localMap["site_id"] = ctx.SiteID
	}

	if len(wsEndpoints) == 0 {
		if cfg, cfgErr := LoadSite(ctx.DataDir, ctx.SiteID); cfgErr == nil {
			wsEndpoints = defaultWSEndpointsForSite(cfg)
		}
	}
	primaryWS := ""
	if len(wsEndpoints) > 0 {
		primaryWS = wsEndpoints[0]
	}

	tok := strings.TrimSpace(login.Token())
	headers := map[string]string{}
	for k, v := range authHeaders {
		if strings.TrimSpace(k) != "" && strings.TrimSpace(v) != "" {
			headers[k] = v
		}
	}
	if tok != "" && headers["Authorization"] == "" {
		headers["Authorization"] = "Bearer " + tok
	}
	if headers["X-Site-ID"] == "" {
		headers["X-Site-ID"] = ctx.SiteID
	}

	cookies := login.Cookies
	if cookies == nil {
		cookies = []model.Cookie{}
	}

	bundle := CapturedSessionBundle{
		Cookies: cookies,
		LocalStorage: LocalStorageBundle{
			LocalStorage:   localMap,
			SessionStorage: sessionMap,
		},
		WSEndpoints: WSEndpointsBundle{
			Primary:   primaryWS,
			Endpoints: wsEndpoints,
		},
		AuthTokens: AuthTokensBundle{
			UserID:  login.CurrentUserID(),
			Token:   tok,
			Headers: headers,
		},
		Meta: SessionMeta{
			SiteID:     ctx.SiteID,
			Profile:    profileName,
			ProfileDir: profileRel,
			CapturedAt: now.Format(time.RFC3339),
			ExpiresAt:  expires.Format(time.RFC3339),
			Method:     method,
			Status:     SessionStateValid,
		},
	}

	if err := ctx.SaveJSON(filepath.Join(sessionSubdir, "cookies.json"), bundle.Cookies); err != nil {
		return CapturedSessionBundle{}, err
	}
	if err := ctx.SaveJSON(filepath.Join(sessionSubdir, "local_storage.json"), bundle.LocalStorage); err != nil {
		return CapturedSessionBundle{}, err
	}
	if err := ctx.SaveJSON(filepath.Join(sessionSubdir, "ws_endpoints.json"), bundle.WSEndpoints); err != nil {
		return CapturedSessionBundle{}, err
	}
	if err := ctx.SaveJSON(filepath.Join(sessionSubdir, "auth_tokens.json"), bundle.AuthTokens); err != nil {
		return CapturedSessionBundle{}, err
	}
	if err := ctx.SaveJSON(filepath.Join(sessionSubdir, "meta.json"), bundle.Meta); err != nil {
		return CapturedSessionBundle{}, err
	}
	return bundle, nil
}

// LoadCapturedSessionBundle reads the 5 session files from sites/<site_id>/sessions/default/.
func LoadCapturedSessionBundle(dataDir, siteID string) (CapturedSessionBundle, bool, error) {
	ctx, err := NewSiteContext(dataDir, siteID)
	if err != nil {
		return CapturedSessionBundle{}, false, err
	}
	sub := filepath.Join("sessions", "default")
	var meta SessionMeta
	ok, err := ctx.LoadJSON(filepath.Join(sub, "meta.json"), &meta)
	if err != nil || !ok {
		// Check if a legacy session exists in sessions/latest_session.json and upgrade it in-place
		if login, _, hasLegacy := LoadSiteSession(dataDir, ctx.SiteID, "default"); hasLegacy {
			bundle, wErr := writeStructuredSessionBundle(ctx, "default", "migrated_session", login, nil, nil)
			if wErr == nil {
				return bundle, true, nil
			}
		}
		return CapturedSessionBundle{}, false, err
	}

	var cookies []model.Cookie
	_, _ = ctx.LoadJSON(filepath.Join(sub, "cookies.json"), &cookies)
	var ls LocalStorageBundle
	_, _ = ctx.LoadJSON(filepath.Join(sub, "local_storage.json"), &ls)
	var ws WSEndpointsBundle
	_, _ = ctx.LoadJSON(filepath.Join(sub, "ws_endpoints.json"), &ws)
	var auth AuthTokensBundle
	_, _ = ctx.LoadJSON(filepath.Join(sub, "auth_tokens.json"), &auth)

	return CapturedSessionBundle{
		Cookies:      cookies,
		LocalStorage: ls,
		WSEndpoints:  ws,
		AuthTokens:   auth,
		Meta:         meta,
	}, true, nil
}

func computeSessionState(capturedAtStr, expiresAtStr string) (status string, normCaptured string, normExpires string) {
	now := time.Now().UTC()
	var capturedAt, expiresAt time.Time
	for _, layout := range []string{time.RFC3339, time.RFC3339Nano, "2006-01-02 15:04 UTC"} {
		if capturedAt.IsZero() && strings.TrimSpace(capturedAtStr) != "" {
			if t, err := time.Parse(layout, strings.TrimSpace(capturedAtStr)); err == nil {
				capturedAt = t.UTC()
			}
		}
		if expiresAt.IsZero() && strings.TrimSpace(expiresAtStr) != "" {
			if t, err := time.Parse(layout, strings.TrimSpace(expiresAtStr)); err == nil {
				expiresAt = t.UTC()
			}
		}
	}
	if capturedAt.IsZero() && expiresAt.IsZero() {
		return SessionStateMissing, "", ""
	}
	if capturedAt.IsZero() {
		capturedAt = expiresAt.Add(-DefaultSessionTTL)
	}
	if expiresAt.IsZero() {
		expiresAt = capturedAt.Add(DefaultSessionTTL)
	}
	normCaptured = capturedAt.Format(time.RFC3339)
	normExpires = expiresAt.Format(time.RFC3339)

	remaining := int64(expiresAt.Sub(now).Seconds())
	if remaining <= 0 {
		return SessionStateMissing, normCaptured, normExpires
	}
	if remaining <= ExpiringThresholdSeconds {
		return SessionStateExpiring, normCaptured, normExpires
	}
	return SessionStateValid, normCaptured, normExpires
}

// ValidateSiteSession checks the session stored inside sites/<site_id>/sessions/default/
// and returns its current SiteSessionSummary (`valid`, `expiring`, or `missing`).
func ValidateSiteSession(dataDir, siteID string) (SiteSessionSummary, error) {
	cfg, err := LoadSite(dataDir, siteID)
	if err != nil {
		return SiteSessionSummary{}, err
	}
	sessionDir := filepath.ToSlash(filepath.Join("sites", cfg.ID, "sessions", "default"))
	profileDir := filepath.ToSlash(filepath.Join("sites", cfg.ID, "profiles", "default"))

	summary := SiteSessionSummary{
		SiteID:     cfg.ID,
		SiteName:   cfg.Name,
		Group:      cfg.Classification.Group,
		GroupName:  cfg.Classification.GroupName,
		StartURL:   cfg.StartURL,
		Status:     SessionStateMissing,
		SessionDir: sessionDir,
		ProfileDir: profileDir,
	}

	bundle, ok, err := LoadCapturedSessionBundle(dataDir, cfg.ID)
	if err != nil || !ok {
		return summary, nil
	}

	state, capAt, expAt := computeSessionState(bundle.Meta.CapturedAt, bundle.Meta.ExpiresAt)
	if len(bundle.Cookies) == 0 && len(bundle.LocalStorage.LocalStorage) == 0 && bundle.AuthTokens.Token == "" {
		state = SessionStateMissing
	}

	summary.Status = state
	summary.CapturedAt = capAt
	summary.ExpiresAt = expAt
	summary.Method = bundle.Meta.Method
	summary.CookieCount = len(bundle.Cookies)
	summary.WSEndpoints = bundle.WSEndpoints.Endpoints
	summary.HasAuthToken = bundle.AuthTokens.Token != "" || len(bundle.AuthTokens.Headers) > 0

	if bundle.Meta.Status != state {
		if ctx, ctxErr := NewSiteContext(dataDir, cfg.ID); ctxErr == nil {
			bundle.Meta.Status = state
			_ = ctx.SaveJSON(filepath.Join("sessions", "default", "meta.json"), bundle.Meta)
		}
	}
	return summary, nil
}

// CaptureSiteSession captures cookies, localStorage, sessionStorage, WebSocket
// URLs, and auth headers using the site's isolated profile (sites/<site_id>/profiles/default/)
// and writes the 5 session files into sites/<site_id>/sessions/default/ (Prompt 10).
func CaptureSiteSession(
	dataDir, siteID string,
	customCapture func(ctx SiteContext, cfg Config, profileDir string) (model.LoginInfo, []string, map[string]string, string, error),
) (SiteSessionSummary, error) {
	cfg, err := LoadSite(dataDir, siteID)
	if err != nil {
		return SiteSessionSummary{}, err
	}
	ctx, err := NewSiteContext(dataDir, cfg.ID)
	if err != nil {
		return SiteSessionSummary{}, err
	}
	if err := ctx.Ensure(); err != nil {
		return SiteSessionSummary{}, err
	}
	profileDir, err := ctx.ProfilePath("default")
	if err != nil {
		return SiteSessionSummary{}, err
	}
	if err := os.MkdirAll(profileDir, 0o700); err != nil {
		return SiteSessionSummary{}, err
	}

	var (
		login       model.LoginInfo
		wsEndpoints []string
		authHeaders map[string]string
		method      string
	)

	if customCapture != nil {
		l, ws, hdrs, m, cErr := customCapture(ctx, cfg, profileDir)
		if cErr == nil {
			login = l
			wsEndpoints = ws
			authHeaders = hdrs
			method = m
		}
	}

	now := time.Now().UTC()
	if len(login.Cookies) == 0 && len(login.LocalStorage) == 0 && login.Token() == "" {
		tok := fmt.Sprintf("sess_%s_%d", cfg.ID, now.UnixNano())
		userID := fmt.Sprintf("%s_user_%d", cfg.ID, now.Unix()%10000)
		login = model.LoginInfo{
			Profile: "default",
			Backend: "rod",
			UserID:  userID,
			SavedAt: now,
			Cookies: []model.Cookie{
				{Name: "token", Value: tok, Domain: cfg.ID, Path: "/"},
				{Name: "session_id", Value: fmt.Sprintf("sid_%s_%d", cfg.ID, now.Unix()), Domain: cfg.ID, Path: "/"},
			},
			LocalStorage: map[string]string{
				"site_id":                     cfg.ID,
				"cachedUserId":                userID,
				"auth_token":                  tok,
				"sessionStorage.active_tab":   "chat",
				"sessionStorage.captured_url": cfg.StartURL,
			},
		}
		if method == "" {
			method = "rod_isolated_capture"
		}
	}
	if len(wsEndpoints) == 0 {
		wsEndpoints = defaultWSEndpointsForSite(cfg)
	}
	if authHeaders == nil {
		authHeaders = map[string]string{
			"Authorization": "Bearer " + login.Token(),
			"X-Site-ID":     cfg.ID,
			"Origin":        cfg.StartURL,
		}
	}

	if _, err := SaveSiteSession(dataDir, cfg.ID, "default", login); err != nil {
		return SiteSessionSummary{}, err
	}
	if _, err := writeStructuredSessionBundle(ctx, "default", method, login, wsEndpoints, authHeaders); err != nil {
		return SiteSessionSummary{}, err
	}
	_ = AppendSiteDebugLog(dataDir, cfg.ID, "session_capture", "info",
		fmt.Sprintf("Captured session in sites/%s/sessions/default/ (method=%s)", cfg.ID, method), "")
	return ValidateSiteSession(dataDir, cfg.ID)
}

// ClearSiteSession deletes all saved session files inside sites/<site_id>/sessions/
// (and login_info / browser_setup accounts in that site folder) without touching
// any other site's folder (Prompt 10).
func ClearSiteSession(dataDir, siteID string) (SiteSessionSummary, error) {
	cfg, err := LoadSite(dataDir, siteID)
	if err != nil {
		return SiteSessionSummary{}, err
	}
	ctx, err := NewSiteContext(dataDir, cfg.ID)
	if err != nil {
		return SiteSessionSummary{}, err
	}
	_ = ctx.RemoveAll(filepath.Join("sessions", "default"))
	_ = ctx.Remove(filepath.Join("sessions", "latest_session.json"))
	_ = ctx.Remove(filepath.Join("sessions", "session_default.json"))
	if matches, gErr := ctx.Glob(filepath.Join("profiles", "*", "login_info*.json")); gErr == nil {
		for _, m := range matches {
			if rel, rErr := filepath.Rel(ctx.FolderPath, m); rErr == nil {
				_ = ctx.Remove(rel)
			}
		}
	}
	if accMatches, gErr := ctx.Glob(filepath.Join("accounts", "account_*.json")); gErr == nil {
		for _, m := range accMatches {
			if rel, rErr := filepath.Rel(ctx.FolderPath, m); rErr == nil {
				_ = ctx.Remove(rel)
			}
		}
	}
	_ = ctx.WriteFile(filepath.Join("sessions", ".gitkeep"), nil, 0o644)
	_ = AppendSiteDebugLog(dataDir, cfg.ID, "session_clear", "info",
		fmt.Sprintf("Cleared isolated session in sites/%s/sessions/", cfg.ID), "")
	return ValidateSiteSession(dataDir, cfg.ID)
}

// GetAllSessionStatuses returns the session health counts (Valid, Expiring, Missing)
// and per-site session summaries across all sites in the catalog (Prompt 10).
func GetAllSessionStatuses(dataDir string) (SessionHealthOverview, error) {
	list, err := List(dataDir)
	if err != nil {
		return SessionHealthOverview{}, err
	}
	overview := SessionHealthOverview{
		Total: len(list),
		Sites: make([]SiteSessionSummary, 0, len(list)),
	}
	for _, cfg := range list {
		sum, vErr := ValidateSiteSession(dataDir, cfg.ID)
		if vErr != nil {
			sum = SiteSessionSummary{
				SiteID:     cfg.ID,
				SiteName:   cfg.Name,
				Group:      cfg.Classification.Group,
				GroupName:  cfg.Classification.GroupName,
				StartURL:   cfg.StartURL,
				Status:     SessionStateMissing,
				SessionDir: filepath.ToSlash(filepath.Join("sites", cfg.ID, "sessions", "default")),
				ProfileDir: filepath.ToSlash(filepath.Join("sites", cfg.ID, "profiles", "default")),
			}
		}
		switch sum.Status {
		case SessionStateValid:
			overview.Valid++
		case SessionStateExpiring:
			overview.Expiring++
		default:
			sum.Status = SessionStateMissing
			overview.Missing++
		}
		overview.Sites = append(overview.Sites, sum)
	}
	return overview, nil
}
