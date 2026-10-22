package sites

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Site classification groups (Group A through Group E).
const (
	GroupA = "A" // Browserless — signup লাগে না, direct chat (no captcha, no login)
	GroupB = "B" // Auto-Signup — API দিয়ে account বানানো যায়, captcha নেই
	GroupC = "C" // Browser-Once — captcha/login লাগে একবার, পরে browserless
	GroupD = "D" // DOM-Only — API/WS নেই, headless browser দিয়ে DOM chat
	GroupE = "E" // Manual — heavy bot detection, manual only
)

// Classification confidence levels.
const (
	ConfidenceLow    = "low"
	ConfidenceMedium = "medium"
	ConfidenceHigh   = "high"
)

// Classification sources.
const (
	ClassificationSourceInitialGuess = "initial_guess"
	ClassificationSourceLiveProbe    = "live_probe"
	ClassificationSourceManual       = "manual"
)

// GroupDefinition describes one of the 5 canonical site classification groups.
type GroupDefinition struct {
	Group       string `json:"group"`
	Name        string `json:"name"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Browserless bool   `json:"browserless"`
}

// AllGroups returns the 5 canonical site classification groups (A–E).
func AllGroups() []GroupDefinition {
	return []GroupDefinition{
		{
			Group:       GroupA,
			Name:        "Browserless",
			Label:       "Group A: Browserless",
			Description: "signup লাগে না, direct chat (no captcha, no login)",
			Browserless: true,
		},
		{
			Group:       GroupB,
			Name:        "Auto-Signup",
			Label:       "Group B: Auto-Signup",
			Description: "API দিয়ে account বানানো যায়, captcha নেই",
			Browserless: true,
		},
		{
			Group:       GroupC,
			Name:        "Browser-Once",
			Label:       "Group C: Browser-Once",
			Description: "captcha/login লাগে একবার, পরে browserless",
			Browserless: true,
		},
		{
			Group:       GroupD,
			Name:        "DOM-Only",
			Label:       "Group D: DOM-Only",
			Description: "API/WS নেই, headless browser দিয়ে DOM chat",
			Browserless: false,
		},
		{
			Group:       GroupE,
			Name:        "Manual",
			Label:       "Group E: Manual",
			Description: "heavy bot detection, manual only",
			Browserless: false,
		},
	}
}

// GroupName returns the canonical short name for a group code ("A"–"E").
func GroupName(group string) string {
	switch NormalizeGroup(group) {
	case GroupA:
		return "Browserless"
	case GroupB:
		return "Auto-Signup"
	case GroupC:
		return "Browser-Once"
	case GroupD:
		return "DOM-Only"
	case GroupE:
		return "Manual"
	default:
		return "DOM-Only"
	}
}

// NormalizeGroup normalises any group code or label into "A", "B", "C", "D", or "E".
func NormalizeGroup(raw string) string {
	cleaned := strings.ToUpper(strings.TrimSpace(raw))
	cleaned = strings.TrimPrefix(cleaned, "GROUP ")
	cleaned = strings.TrimPrefix(cleaned, "GROUP_")
	if idx := strings.IndexAny(cleaned, ":- "); idx > 0 {
		cleaned = strings.TrimSpace(cleaned[:idx])
	}
	switch cleaned {
	case GroupA, "BROWSERLESS":
		return GroupA
	case GroupB, "AUTO-SIGNUP", "AUTOSIGNUP", "AUTO_SIGNUP":
		return GroupB
	case GroupC, "BROWSER-ONCE", "BROWSERONCE", "BROWSER_ONCE":
		return GroupC
	case GroupD, "DOM-ONLY", "DOMONLY", "DOM_ONLY":
		return GroupD
	case GroupE, "MANUAL":
		return GroupE
	default:
		return ""
	}
}

// NormalizeConfidence normalises a confidence string into "low", "medium", or "high".
func NormalizeConfidence(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case ConfidenceHigh:
		return ConfidenceHigh
	case ConfidenceMedium:
		return ConfidenceMedium
	case ConfidenceLow:
		return ConfidenceLow
	default:
		return ConfidenceLow
	}
}

// ProbeResult captures what the site prober detected on the target URL.
type ProbeResult struct {
	URL                string   `json:"url"`
	FinalURL           string   `json:"final_url,omitempty"`
	StatusCode         int      `json:"status_code,omitempty"`
	Reachable          bool     `json:"reachable"`
	Error              string   `json:"error,omitempty"`
	HasCaptcha         bool     `json:"has_captcha"`
	CaptchaKeywords    []string `json:"captcha_keywords,omitempty"`
	HasSignupForm      bool     `json:"has_signup_form"`
	SignupIndicators   []string `json:"signup_indicators,omitempty"`
	HasWebSocket       bool     `json:"has_websocket"`
	WebSocketEndpoints []string `json:"websocket_endpoints,omitempty"`
	HasAPIEndpoint     bool     `json:"has_api_endpoint"`
	APIEndpoints       []string `json:"api_endpoints,omitempty"`
	LoginRequired      bool     `json:"login_required"`
	LoginIndicators    []string `json:"login_indicators,omitempty"`
	HeavyBotDetection  bool     `json:"heavy_bot_detection"`
	BotCheckKeywords   []string `json:"bot_check_keywords,omitempty"`
	ProbedAt           string   `json:"probed_at,omitempty"`
}

// Classification is stored in sites/<site_id>/site.json under "classification".
type Classification struct {
	Group        string       `json:"group"`
	GroupName    string       `json:"group_name"`
	Confidence   string       `json:"confidence"`
	ChatMethod   string       `json:"chat_method,omitempty"`
	Source       string       `json:"source,omitempty"`
	Reason       string       `json:"reason,omitempty"`
	ProbeCount   int          `json:"probe_count,omitempty"`
	ClassifiedAt string       `json:"classified_at,omitempty"`
	Probe        *ProbeResult `json:"probe,omitempty"`
}

// Normalize fills in canonical group names and default confidence values.
func (c Classification) Normalize(fallback Config) Classification {
	group := NormalizeGroup(c.Group)
	if group == "" {
		guess := InitialClassification(fallback)
		group = guess.Group
		if strings.TrimSpace(c.Reason) == "" {
			c.Reason = guess.Reason
		}
		if strings.TrimSpace(c.Source) == "" {
			c.Source = guess.Source
		}
	}
	c.Group = group
	c.GroupName = GroupName(group)
	c.Confidence = NormalizeConfidence(c.Confidence)
	if strings.TrimSpace(c.Source) == "" {
		c.Source = ClassificationSourceInitialGuess
	}
	return c
}

var (
	wsEndpointPattern  = regexp.MustCompile(`(?i)\b(?:wss?://[^\s"'<>\\]+|/socket\.io/?[^\s"'<>\\]*|/ws(?:/[^\s"'<>\\]*)?|/cable\b|/sockjs[^\s"'<>\\]*)`)
	apiEndpointPattern = regexp.MustCompile(`(?i)(?:https?://[^\s"'<>\\]+)?(?:/api/[a-z0-9_/\-.]+|/v[123]/[a-z0-9_/\-.]+|/graphql\b|/auth/[a-z0-9_/\-.]+)`)
	formTagPattern     = regexp.MustCompile(`(?is)<form\b[^>]*>.*?</form>`)
)

// ProbeURL visits rawURL (using a safe default HTTP client if client is nil),
// inspects the HTTP response headers, redirects, and HTML/JS body, and returns
// the detected signals.
func ProbeURL(ctx context.Context, rawURL string, client *http.Client) (ProbeResult, error) {
	target := strings.TrimSpace(rawURL)
	now := time.Now().UTC().Format(time.RFC3339)
	if target == "" {
		return ProbeResult{URL: rawURL, ProbedAt: now, Error: "empty probe URL"}, fmt.Errorf("sites: empty probe URL")
	}
	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
	}
	if client == nil {
		client = &http.Client{
			Timeout: 15 * time.Second,
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return ProbeResult{URL: target, ProbedAt: now, Error: err.Error()}, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/json;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	resp, err := client.Do(req)
	if err != nil {
		return ProbeResult{
			URL:      target,
			ProbedAt: now,
			Error:    err.Error(),
		}, err
	}
	defer resp.Body.Close()

	limited, readErr := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	body := string(limited)
	finalURL := target
	if resp.Request != nil && resp.Request.URL != nil {
		finalURL = resp.Request.URL.String()
	}
	result := AnalyzeHTML(target, finalURL, resp.StatusCode, resp.Header, body)
	result.ProbedAt = now
	if readErr != nil {
		result.Error = readErr.Error()
	}
	return result, nil
}

// AnalyzeHTML inspects an HTTP response's URL, status code, headers, and HTML/JS
// body to detect captcha, signup forms, WebSocket endpoints, API endpoints,
// login requirements, and heavy bot detection.
func AnalyzeHTML(rawURL, finalURL string, statusCode int, headers http.Header, body string) ProbeResult {
	lower := strings.ToLower(body)
	finalLower := strings.ToLower(finalURL)

	result := ProbeResult{
		URL:        rawURL,
		FinalURL:   finalURL,
		StatusCode: statusCode,
		Reachable:  statusCode > 0,
	}

	// 1. Captcha detection (recaptcha, hcaptcha, cloudflare turnstile, funcaptcha, etc.)
	captchaSignals := map[string]string{
		"recaptcha":             "recaptcha",
		"g-recaptcha":           "recaptcha",
		"grecaptcha":            "recaptcha",
		"hcaptcha":              "hcaptcha",
		"h-captcha":             "hcaptcha",
		"cf-turnstile":          "cloudflare",
		"challenges.cloudflare": "cloudflare",
		"turnstile/v0/api.js":   "cloudflare",
		"cf-chl-widget":         "cloudflare",
		"arkoselabs":            "arkose",
		"funcaptcha":            "funcaptcha",
		"datadome":              "datadome",
		"px-captcha":            "perimeterx",
		"perimeterx":            "perimeterx",
	}
	captchaSet := map[string]bool{}
	for needle, label := range captchaSignals {
		if strings.Contains(lower, needle) {
			captchaSet[label] = true
		}
	}
	if headers != nil {
		if strings.Contains(strings.ToLower(headers.Get("Cf-Mitigated")), "challenge") {
			captchaSet["cloudflare"] = true
		}
	}
	if len(captchaSet) > 0 {
		result.HasCaptcha = true
		result.CaptchaKeywords = sortedKeys(captchaSet)
	}

	// 2. Heavy bot detection (interstitial block pages, WAF challenge walls, multi-layer blocks)
	botSet := map[string]bool{}
	botPatterns := map[string]string{
		"just a moment...":                          "cloudflare_challenge",
		"attention required! | cloudflare":          "cloudflare_block",
		"cf-browser-verification":                   "cloudflare_challenge",
		"_cf_chl_opt":                               "cloudflare_challenge",
		"Access denied | ":                          "waf_access_denied",
		"pardon our interruption":                   "bot_interstitial",
		"verify you are human":                      "human_verification_wall",
		"enable javascript and cookies to continue": "cloudflare_challenge",
		"datadome":                                  "datadome",
		"perimeterx":                                "perimeterx",
		"kasada":                                    "kasada",
		"incapsula":                                 "imperva_incapsula",
		"akamai":                                    "akamai_bot_manager",
	}
	for needle, label := range botPatterns {
		if strings.Contains(lower, strings.ToLower(needle)) {
			botSet[label] = true
		}
	}
	if headers != nil {
		if strings.Contains(strings.ToLower(headers.Get("Cf-Mitigated")), "challenge") {
			botSet["cloudflare_challenge"] = true
		}
	}
	if (statusCode == http.StatusForbidden || statusCode == http.StatusServiceUnavailable || statusCode == http.StatusTooManyRequests) &&
		(len(botSet) > 0 || result.HasCaptcha || strings.Contains(lower, "cloudflare")) {
		botSet["http_challenge_block"] = true
	}
	if len(botSet) > 0 {
		result.HeavyBotDetection = true
		result.BotCheckKeywords = sortedKeys(botSet)
	}

	// 3. WebSocket detection
	wsSet := map[string]bool{}
	for _, match := range wsEndpointPattern.FindAllString(body, 25) {
		wsSet[strings.TrimRight(match, `"' );,`)] = true
	}
	for _, token := range []string{"new websocket(", "window.websocket", "socket.io", "io.connect(", "sockjs"} {
		if strings.Contains(lower, token) {
			wsSet[token] = true
		}
	}
	if len(wsSet) > 0 {
		result.HasWebSocket = true
		result.WebSocketEndpoints = sortedKeys(wsSet)
	}

	// 4. API endpoint detection
	apiSet := map[string]bool{}
	for _, match := range apiEndpointPattern.FindAllString(body, 25) {
		apiSet[strings.TrimRight(match, `"' );,`)] = true
	}
	for _, token := range []string{"fetch(", "axios.", "xmlhttprequest", "application/json", "graphql"} {
		if strings.Contains(lower, token) {
			apiSet[token] = true
		}
	}
	if headers != nil && strings.Contains(strings.ToLower(headers.Get("Content-Type")), "application/json") {
		apiSet["content-type:application/json"] = true
	}
	if len(apiSet) > 0 {
		result.HasAPIEndpoint = true
		result.APIEndpoints = sortedKeys(apiSet)
	}

	// 5. Signup form & Login requirement detection
	signupSet := map[string]bool{}
	loginSet := map[string]bool{}

	if statusCode == http.StatusUnauthorized {
		loginSet["http_401_unauthorized"] = true
	}
	if strings.Contains(finalLower, "/login") || strings.Contains(finalLower, "/signin") || strings.Contains(finalLower, "auth=email") {
		loginSet["login_url_gate"] = true
	}
	if strings.Contains(finalLower, "/signup") || strings.Contains(finalLower, "/register") {
		signupSet["signup_url"] = true
	}

	forms := formTagPattern.FindAllString(body, 20)
	for _, form := range forms {
		fl := strings.ToLower(form)
		hasPassword := strings.Contains(fl, `type="password"`) || strings.Contains(fl, `type='password'`)
		isSignup := strings.Contains(fl, "signup") || strings.Contains(fl, "sign-up") ||
			strings.Contains(fl, "register") || strings.Contains(fl, "create account") ||
			strings.Contains(fl, "guest") || strings.Contains(fl, "nickname") ||
			(strings.Contains(fl, "gender") && strings.Contains(fl, "age"))
		isLogin := strings.Contains(fl, "login") || strings.Contains(fl, "log-in") ||
			strings.Contains(fl, "signin") || strings.Contains(fl, "sign-in") ||
			strings.Contains(fl, `name="password"`)

		if isSignup {
			signupSet["html_signup_form"] = true
		}
		if isLogin && hasPassword && !isSignup {
			loginSet["html_login_form"] = true
		}
	}

	for _, indicator := range []string{
		"/api/register", "/api/signup", "/api/guest", `name="nickname"`, `name="username"`,
		"create an account", "sign up", "guest login", "enter nickname",
	} {
		if strings.Contains(lower, indicator) {
			signupSet[indicator] = true
		}
	}
	for _, indicator := range []string{
		"login required", "please log in", "sign in to continue", "must be logged in",
		`href="/login"`, `action="/login"`,
	} {
		if strings.Contains(lower, indicator) {
			loginSet[indicator] = true
		}
	}

	if len(signupSet) > 0 {
		result.HasSignupForm = true
		result.SignupIndicators = sortedKeys(signupSet)
	}
	if len(loginSet) > 0 {
		result.LoginRequired = true
		result.LoginIndicators = sortedKeys(loginSet)
	}

	return result
}

// ClassifyProbe maps a ProbeResult into one of the 5 groups (Group A–E).
//
// Decision rules:
//   - Group E (Manual): heavy bot detection (Cloudflare/DataDome/PerimeterX challenge block
//     or multi-wall captcha + mandatory login without reusable API/WS).
//   - Group C (Browser-Once): captcha or login is required once at entry, after which session/chat
//     can proceed via WebSocket/API or saved session.
//   - Group B (Auto-Signup): no captcha and no login wall, has a signup/guest registration form
//     plus an API or WebSocket endpoint so accounts can be created automatically.
//   - Group A (Browserless): no captcha, no login, no signup form, and has direct WebSocket/API
//     chat transport.
//   - Group D (DOM-Only): no captcha/heavy bot wall, and no direct API/WebSocket transport
//     (or DOM-driven signup/chat without exposed API/WS) — requires headless browser DOM chat.
func ClassifyProbe(probe ProbeResult) Classification {
	now := probe.ProbedAt
	if strings.TrimSpace(now) == "" {
		now = time.Now().UTC().Format(time.RFC3339)
	}
	confidence := ConfidenceHigh
	if !probe.Reachable || probe.Error != "" {
		confidence = ConfidenceLow
	}
	hasTransport := probe.HasWebSocket || probe.HasAPIEndpoint

	// 1. Group E: Manual — heavy bot detection, manual only.
	if probe.HeavyBotDetection || (probe.HasCaptcha && probe.LoginRequired && !hasTransport) {
		reason := "Heavy bot detection / anti-bot challenge wall detected"
		if len(probe.BotCheckKeywords) > 0 {
			reason = fmt.Sprintf("Heavy bot detection (%s) — manual browser handling required", strings.Join(probe.BotCheckKeywords, ", "))
		} else if len(probe.CaptchaKeywords) > 0 {
			reason = fmt.Sprintf("Captcha (%s) + mandatory login without direct API/WS transport", strings.Join(probe.CaptchaKeywords, ", "))
		}
		return Classification{
			Group:        GroupE,
			GroupName:    GroupName(GroupE),
			Confidence:   confidence,
			Source:       ClassificationSourceLiveProbe,
			Reason:       reason,
			ClassifiedAt: now,
			Probe:        &probe,
		}
	}

	// 2. Group C: Browser-Once — captcha/login লাগে একবার, পরে browserless.
	if probe.HasCaptcha || probe.LoginRequired {
		var parts []string
		if probe.HasCaptcha {
			parts = append(parts, fmt.Sprintf("captcha (%s)", strings.Join(probe.CaptchaKeywords, ", ")))
		}
		if probe.LoginRequired {
			parts = append(parts, "login gate")
		}
		reason := fmt.Sprintf("Requires one-time %s in browser; session can be reused afterwards", strings.Join(parts, " + "))
		return Classification{
			Group:        GroupC,
			GroupName:    GroupName(GroupC),
			Confidence:   confidence,
			Source:       ClassificationSourceLiveProbe,
			Reason:       reason,
			ClassifiedAt: now,
			Probe:        &probe,
		}
	}

	// 3. Group B: Auto-Signup — API দিয়ে account বানানো যায়, captcha নেই.
	if probe.HasSignupForm && hasTransport {
		return Classification{
			Group:        GroupB,
			GroupName:    GroupName(GroupB),
			Confidence:   confidence,
			Source:       ClassificationSourceLiveProbe,
			Reason:       "Signup/guest form + API/WebSocket endpoint detected with no captcha",
			ClassifiedAt: now,
			Probe:        &probe,
		}
	}

	// 4. Group A: Browserless — signup লাগে না, direct chat (no captcha, no login).
	if !probe.HasSignupForm && hasTransport {
		return Classification{
			Group:        GroupA,
			GroupName:    GroupName(GroupA),
			Confidence:   confidence,
			Source:       ClassificationSourceLiveProbe,
			Reason:       "Direct WebSocket/API chat with no signup, no login, and no captcha",
			ClassifiedAt: now,
			Probe:        &probe,
		}
	}

	// 5. Group D: DOM-Only — API/WS নেই, headless browser দিয়ে DOM chat.
	return Classification{
		Group:        GroupD,
		GroupName:    GroupName(GroupD),
		Confidence:   confidence,
		Source:       ClassificationSourceLiveProbe,
		Reason:       "No direct API/WebSocket transport exposed; automate via headless browser DOM",
		ClassifiedAt: now,
		Probe:        &probe,
	}
}

// InitialClassification returns the initial low-confidence classification guess
// for a site based on its known metadata before a live probe is run.
func InitialClassification(cfg Config) Classification {
	id := SanitizeID(cfg.ID)
	family := strings.ToLower(strings.TrimSpace(cfg.Family))
	accMode := NormalizeAccountMode(cfg.AccountMode, cfg.ID, cfg.AuthMode)
	authMode := strings.ToLower(strings.TrimSpace(cfg.AuthMode))
	combined := strings.ToLower(cfg.EntryFlow + " " + cfg.ProtectionSummary + " " + cfg.APIBaseURL)

	// Strip negated captcha phrases so "no captcha" / "zero captcha" is not
	// misread as requiring a CAPTCHA.
	sanitizedForCaptcha := combined
	for _, neg := range []string{"no initial captcha", "zero captcha", "no captcha", "without captcha"} {
		sanitizedForCaptcha = strings.ReplaceAll(sanitizedForCaptcha, neg, " ")
	}
	hasPositiveCaptcha := strings.Contains(sanitizedForCaptcha, "captcha") || strings.Contains(sanitizedForCaptcha, "turnstile")

	group := GroupD
	reason := "Initial guess from site catalog metadata (DOM chat archetype); re-probe to verify"

	switch {
	case strings.Contains(combined, "heavy bot") ||
		strings.Contains(combined, "high anti-bot") ||
		strings.Contains(combined, "periodic cloudflare turnstile") ||
		strings.Contains(combined, "blocks datacenter/vpn") ||
		strings.Contains(combined, "captcha loop") ||
		strings.Contains(combined, "strict fingerprinting") ||
		strings.Contains(combined, "manual only"):
		group = GroupE
		reason = "Initial guess: heavy anti-bot / recurring challenge protection described in site metadata"

	case id == DefaultSiteID || authMode == "chitchat_jwt" || accMode == AccountModeBrowserSetup ||
		hasPositiveCaptcha || strings.Contains(combined, "jwt login") || strings.Contains(combined, "solve once"):
		group = GroupC
		reason = "Initial guess: one-time browser login or CAPTCHA/Turnstile setup before session reuse"

	case family == "family_c" ||
		strings.Contains(combined, "nickname") ||
		strings.Contains(combined, "guest form") ||
		strings.Contains(combined, "alias setup") ||
		(strings.Contains(combined, "auto_register") && strings.Contains(combined, "guest")):
		group = GroupB
		reason = "Initial guess: guest/nickname auto-signup flow with no CAPTCHA"

	case strings.Contains(combined, "websocket") ||
		strings.Contains(combined, "zero captcha") ||
		strings.Contains(combined, "no captcha") ||
		strings.Contains(combined, "no initial captcha") ||
		strings.Contains(combined, "no sign-up") ||
		strings.Contains(combined, "no signup") ||
		strings.Contains(combined, "no registration") ||
		strings.Contains(combined, "instant anonymous"):
		group = GroupA
		reason = "Initial guess: direct anonymous chat with no signup and no CAPTCHA"

	default:
		group = GroupD
		reason = "Initial guess: DOM-driven stranger chat; run live probe to detect API/WebSocket endpoints"
	}

	return Classification{
		Group:      group,
		GroupName:  GroupName(group),
		Confidence: ConfidenceLow,
		Source:     ClassificationSourceInitialGuess,
		Reason:     reason,
	}
}

// ProbeAndClassifySite probes the site's StartURL (or AppURL), classifies it into
// Group A–E, persists the updated classification into sites/<site_id>/site.json,
// and returns the updated site Config.
//
// It is safe to call repeatedly at any time (re-runnable). If the target URL is
// unreachable in an offline environment, the probe error is recorded in
// Classification.Probe.Error, ProbeCount is incremented, and the config is still
// saved with low confidence so callers always get a consistent record.
func ProbeAndClassifySite(ctx context.Context, dataDir, siteID string, client *http.Client) (Config, error) {
	cfg, err := LoadSite(dataDir, siteID)
	if err != nil {
		return Config{}, err
	}
	siteCtx, err := NewSiteContext(dataDir, cfg.ID)
	if err != nil {
		return Config{}, err
	}
	if err := siteCtx.Ensure(); err != nil {
		return Config{}, err
	}

	targetURL := strings.TrimSpace(cfg.StartURL)
	if targetURL == "" {
		targetURL = strings.TrimSpace(cfg.AppURL)
	}
	prevCount := cfg.Classification.ProbeCount

	probe, probeErr := ProbeURL(ctx, targetURL, client)
	var next Classification
	if probeErr != nil && !probe.Reachable {
		// Offline / unreachable fallback: keep or compute heuristic group, mark
		// confidence low, and record the probe attempt + error.
		next = InitialClassification(cfg)
		if prevGroup := NormalizeGroup(cfg.Classification.Group); prevGroup != "" {
			next.Group = prevGroup
			next.GroupName = GroupName(prevGroup)
		}
		next.Confidence = ConfidenceLow
		next.Source = ClassificationSourceLiveProbe
		next.Reason = fmt.Sprintf("Probe unreachable (%s); retained %s classification", probeErr.Error(), next.GroupName)
		next.ClassifiedAt = probe.ProbedAt
		next.Probe = &probe
	} else {
		next = ClassifyProbe(probe)
	}
	next.ProbeCount = prevCount + 1
	cfg.Classification = next

	// Persist to sites/<site_id>/site.json without transient runtime-only fields.
	toSave := cfg
	toSave.AccountCount = 0
	toSave.HasSavedSession = false
	toSave.SessionSavedAt = ""
	toSave.SessionPath = ""
	toSave.FolderPath = ""
	if err := siteCtx.SaveJSON("site.json", toSave); err != nil {
		return Config{}, err
	}

	AppendSiteDebugLog(
		dataDir,
		cfg.ID,
		"info",
		"classify_probe",
		fmt.Sprintf("Group %s (%s)", cfg.Classification.Group, cfg.Classification.GroupName),
		fmt.Sprintf("confidence=%s reason=%s", cfg.Classification.Confidence, cfg.Classification.Reason),
	)
	return cfg, nil
}

// ProbeAndClassifyAllSites re-probes and updates the classification of every
// site in the workspace.
func ProbeAndClassifyAllSites(ctx context.Context, dataDir string, client *http.Client) ([]Config, error) {
	list, err := List(dataDir)
	if err != nil {
		return nil, err
	}
	out := make([]Config, 0, len(list))
	for _, item := range list {
		updated, probeErr := ProbeAndClassifySite(ctx, dataDir, item.ID, client)
		if probeErr != nil {
			return out, probeErr
		}
		out = append(out, updated)
	}
	return out, nil
}

// SaveSiteClassification persists an explicit or re-computed classification into
// sites/<site_id>/site.json.
func SaveSiteClassification(dataDir, siteID string, cls Classification) (Config, error) {
	cfg, err := LoadSite(dataDir, siteID)
	if err != nil {
		return Config{}, err
	}
	siteCtx, err := NewSiteContext(dataDir, cfg.ID)
	if err != nil {
		return Config{}, err
	}
	if err := siteCtx.Ensure(); err != nil {
		return Config{}, err
	}
	cls = cls.Normalize(cfg)
	if strings.TrimSpace(cls.ClassifiedAt) == "" {
		cls.ClassifiedAt = time.Now().UTC().Format(time.RFC3339)
	}
	cfg.Classification = cls

	toSave := cfg
	toSave.AccountCount = 0
	toSave.HasSavedSession = false
	toSave.SessionSavedAt = ""
	toSave.SessionPath = ""
	toSave.FolderPath = ""
	if err := siteCtx.SaveJSON("site.json", toSave); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func sortedKeys(m map[string]bool) []string {
	if len(m) == 0 {
		return nil
	}
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
