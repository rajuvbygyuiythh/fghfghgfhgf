package runner

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"chitchatbot/internal/bot"
	"chitchatbot/internal/browser"
	"chitchatbot/internal/model"
	"chitchat-wails/internal/sites"
)

var (
	// ErrNeedsSession is returned when a site's session is expired or missing
	// and requires session capture before browserless execution can proceed (Prompt 11).
	ErrNeedsSession = errors.New("needs session")

	// ErrManualModeOnly is returned when attempting automated browserless chat
	// on a Group E manual-only site (Prompt 11).
	ErrManualModeOnly = errors.New("manual mode site requires manual control")
)

const (
	BrowserlessMethodWebSocket   = "websocket"
	BrowserlessMethodAPI         = "api"
	BrowserlessMethodHeadlessDOM = "headless_dom"
	BrowserlessMethodManual      = "manual"
)

// BrowserlessConnection encapsulates a site's isolated session, transport
// client (WebSocket headers, HTTP CookieJar client, or Headless Rod DOM
// connection), and same-session reconnection state (Prompt 11).
type BrowserlessConnection struct {
	SiteID         string
	SiteName       string
	Group          string
	Method         string
	Headless       bool
	VisibleBrowser bool
	SessionStatus  string
	CapturedAt     string
	ExpiresAt      string
	WSURL          string
	WSEndpoints    []string
	AuthToken      string
	AuthHeaders    map[string]string
	Cookies        []model.Cookie
	LocalStorage   map[string]string
	SessionStorage map[string]string
	Login          model.LoginInfo
	Bundle         sites.CapturedSessionBundle
	HTTPClient     *http.Client
	DOMConn        browser.DOMSessionConnection
	ReconnectCount int
}

type authHeaderTransport struct {
	base    http.RoundTripper
	headers map[string]string
}

func (t *authHeaderTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	clone := req.Clone(req.Context())
	for k, v := range t.headers {
		if strings.TrimSpace(k) != "" && strings.TrimSpace(v) != "" && clone.Header.Get(k) == "" {
			clone.Header.Set(k, v)
		}
	}
	return base.RoundTrip(clone)
}

func buildSessionHTTPClient(startURL string, cookies []model.Cookie, authHeaders map[string]string) *http.Client {
	jar, _ := cookiejar.New(nil)
	if jar != nil && strings.TrimSpace(startURL) != "" {
		if u, err := url.Parse(strings.TrimSpace(startURL)); err == nil && u.Scheme != "" && u.Host != "" {
			httpCookies := make([]*http.Cookie, 0, len(cookies))
			for _, c := range cookies {
				if strings.TrimSpace(c.Name) == "" {
					continue
				}
				httpCookies = append(httpCookies, &http.Cookie{
					Name:   c.Name,
					Value:  c.Value,
					Path:   c.Path,
					Domain: c.Domain,
				})
			}
			if len(httpCookies) > 0 {
				jar.SetCookies(u, httpCookies)
			}
		}
	}
	hdrCopy := make(map[string]string, len(authHeaders))
	for k, v := range authHeaders {
		hdrCopy[k] = v
	}
	return &http.Client{
		Timeout:   20 * time.Second,
		Jar:       jar,
		Transport: &authHeaderTransport{base: http.DefaultTransport, headers: hdrCopy},
	}
}

func resolveGroupMethod(cfg sites.Config) (method string, headless bool, visible bool) {
	grp := strings.ToUpper(strings.TrimSpace(cfg.Classification.Group))
	chatMethod := strings.ToLower(strings.TrimSpace(cfg.Classification.ChatMethod))
	switch grp {
	case sites.GroupA:
		// Group A (Guest Instant): browserless WebSocket or API (no visible browser)
		if chatMethod == "api" {
			return BrowserlessMethodAPI, true, false
		}
		return BrowserlessMethodWebSocket, true, false
	case sites.GroupB, sites.GroupC:
		// Group B (Auto-Signup) & Group C (Manual Login Once): browserless WS or API
		if chatMethod == "websocket" || strings.EqualFold(cfg.AuthMode, "chitchat_jwt") {
			return BrowserlessMethodWebSocket, true, false
		}
		return BrowserlessMethodAPI, true, false
	case sites.GroupD:
		// Group D (Anti-Bot): headless Rod browser in background (no visible UI)
		return BrowserlessMethodHeadlessDOM, true, false
	case sites.GroupE:
		// Group E (Complex): manual mode
		return BrowserlessMethodManual, false, true
	default:
		return BrowserlessMethodWebSocket, true, false
	}
}

func isBundleExpired(bundle sites.CapturedSessionBundle) bool {
	expStr := strings.TrimSpace(bundle.Meta.ExpiresAt)
	if expStr == "" {
		return false
	}
	for _, layout := range []string{time.RFC3339, time.RFC3339Nano, "2006-01-02 15:04 UTC"} {
		if exp, err := time.Parse(layout, expStr); err == nil {
			return !exp.After(time.Now().UTC())
		}
	}
	return false
}

// ConnectBrowserlessSite loads the isolated session from sites/<site_id>/sessions/default/,
// validates expiry, and builds the appropriate BrowserlessConnection (WebSocket,
// HTTP API client with cookies, or Headless Rod DOM injector) for Group A–E (Prompt 11).
func ConnectBrowserlessSite(dataDir, siteID string) (*BrowserlessConnection, error) {
	cfg, err := sites.LoadSite(dataDir, siteID)
	if err != nil {
		return nil, err
	}
	siteCtx, err := sites.NewSiteContext(dataDir, cfg.ID)
	if err != nil {
		return nil, err
	}
	grp := strings.ToUpper(strings.TrimSpace(cfg.Classification.Group))
	if grp == "" {
		grp = sites.GroupA
	}
	method, headless, visible := resolveGroupMethod(cfg)

	// Load captured session bundle strictly from sites/<site_id>/sessions/default/
	bundle, hasBundle, err := sites.LoadCapturedSessionBundle(dataDir, cfg.ID)
	if err != nil {
		return nil, err
	}

	// Check if an existing session has expired or was explicitly marked missing/expired
	if hasBundle && (isBundleExpired(bundle) || strings.EqualFold(bundle.Meta.Status, sites.SessionStateMissing) || strings.EqualFold(bundle.Meta.Status, "expired")) {
		bundle.Meta.Status = sites.SessionStateMissing
		_ = siteCtx.SaveJSON(filepath.Join("sessions", "default", "meta.json"), bundle.Meta)
		return nil, fmt.Errorf("%w: session for %s expired at %s", ErrNeedsSession, cfg.ID, bundle.Meta.ExpiresAt)
	}

	// Group A, B, C, D can auto-initialize a session in their isolated folder if none exists yet
	if !hasBundle && grp != sites.GroupE {
		if _, capErr := sites.CaptureSiteSession(dataDir, cfg.ID, nil); capErr == nil {
			bundle, hasBundle, _ = sites.LoadCapturedSessionBundle(dataDir, cfg.ID)
		}
	}

	sessSummary, _ := sites.ValidateSiteSession(dataDir, cfg.ID)
	if hasBundle && sessSummary.Status == sites.SessionStateMissing {
		return nil, fmt.Errorf("%w: session for %s is invalid or expired", ErrNeedsSession, cfg.ID)
	}

	wsURL := strings.TrimSpace(bundle.WSEndpoints.Primary)
	if wsURL == "" && len(bundle.WSEndpoints.Endpoints) > 0 {
		wsURL = strings.TrimSpace(bundle.WSEndpoints.Endpoints[0])
	}
	if wsURL == "" {
		wsList := cfg.Classification.DiscoveredEndpoints.WebSocketURLs
		if len(wsList) > 0 {
			wsURL = wsList[0]
		} else {
			u := strings.TrimSpace(cfg.StartURL)
			wsURL = strings.TrimRight(strings.Replace(strings.Replace(u, "https://", "wss://", 1), "http://", "ws://", 1), "/") + "/ws"
		}
	}

	authHeaders := map[string]string{}
	for k, v := range bundle.AuthTokens.Headers {
		authHeaders[k] = v
	}
	if bundle.AuthTokens.Token != "" && authHeaders["Authorization"] == "" {
		authHeaders["Authorization"] = "Bearer " + bundle.AuthTokens.Token
	}

	mergedLocal := map[string]string{}
	for k, v := range bundle.LocalStorage.LocalStorage {
		mergedLocal[k] = v
	}
	for k, v := range bundle.LocalStorage.SessionStorage {
		mergedLocal["sessionStorage."+k] = v
	}

	login := model.LoginInfo{
		Profile:      "default",
		Backend:      "rod",
		UserID:       bundle.AuthTokens.UserID,
		Cookies:      bundle.Cookies,
		LocalStorage: mergedLocal,
	}
	if capTime, parseErr := time.Parse(time.RFC3339, bundle.Meta.CapturedAt); parseErr == nil {
		login.SavedAt = capTime
	}

	profileDir, _ := siteCtx.ProfilePath("default")
	domConn := browser.DOMSessionConnection{
		SiteID:           cfg.ID,
		StartURL:         cfg.StartURL,
		UserDataDir:      profileDir,
		Headless:         true, // Group D always runs headless in background
		CustomDOMJS:      sites.CustomDOMDriverJS(dataDir, cfg.ID),
		ComposerSelector: cfg.Selectors.ComposerInput,
		Login:            login,
	}

	conn := &BrowserlessConnection{
		SiteID:         cfg.ID,
		SiteName:       cfg.Name,
		Group:          grp,
		Method:         method,
		Headless:       headless,
		VisibleBrowser: visible,
		SessionStatus:  sessSummary.Status,
		CapturedAt:     bundle.Meta.CapturedAt,
		ExpiresAt:      bundle.Meta.ExpiresAt,
		WSURL:          wsURL,
		WSEndpoints:    bundle.WSEndpoints.Endpoints,
		AuthToken:      bundle.AuthTokens.Token,
		AuthHeaders:    authHeaders,
		Cookies:        bundle.Cookies,
		LocalStorage:   bundle.LocalStorage.LocalStorage,
		SessionStorage: bundle.LocalStorage.SessionStorage,
		Login:          login,
		Bundle:         bundle,
		HTTPClient:     buildSessionHTTPClient(cfg.StartURL, bundle.Cookies, authHeaders),
		DOMConn:        domConn,
	}
	return conn, nil
}

// Reconnect re-establishes the transport client using the SAME saved session
// (without re-logging in). If the session has expired in the meantime, it
// returns ErrNeedsSession so the Supervisor auto-pauses the site (Prompt 11).
func (b *BrowserlessConnection) Reconnect(ctx context.Context) error {
	if b == nil {
		return ErrNeedsSession
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	if isBundleExpired(b.Bundle) {
		b.SessionStatus = sites.SessionStateMissing
		return fmt.Errorf("%w: session expired for %s during reconnect", ErrNeedsSession, b.SiteID)
	}
	b.ReconnectCount++
	b.HTTPClient = buildSessionHTTPClient(b.DOMConn.StartURL, b.Cookies, b.AuthHeaders)
	return nil
}

// ToBotSessionConnection converts BrowserlessConnection into a *bot.SessionConnection
// accepted by bot/realtime.go and bot/api_chat.go (Prompt 11).
func (b *BrowserlessConnection) ToBotSessionConnection() *bot.SessionConnection {
	if b == nil {
		return nil
	}
	return &bot.SessionConnection{
		SiteID:         b.SiteID,
		Method:         b.Method,
		WSURL:          b.WSURL,
		WSEndpoints:    b.WSEndpoints,
		AuthToken:      b.AuthToken,
		AuthHeaders:    b.AuthHeaders,
		Cookies:        b.Cookies,
		LocalStorage:   b.LocalStorage,
		SessionStorage: b.SessionStorage,
		HTTPClient:     b.HTTPClient,
		Headless:       b.Headless,
		ReconnectCount: b.ReconnectCount,
	}
}
