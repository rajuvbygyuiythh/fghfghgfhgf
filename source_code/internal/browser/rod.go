package browser

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"chitchatbot/internal/model"
	"chitchatbot/internal/proxy"
	"chitchatbot/internal/runtimeassets"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/cdp"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
)

type RodBackend struct {
	browser             *rod.Browser
	page                *rod.Page
	startURL            string
	disablePageRecovery bool
	stopProxyAuth       func()
	proxyRelay          *proxy.Relay
	customDOMJS         string
	composerSelector    string
}

func (b *RodBackend) ConfigureSiteDOM(customJS, composerSelector string) {
	b.customDOMJS = strings.TrimSpace(customJS)
	b.composerSelector = strings.TrimSpace(composerSelector)
}

func (b *RodBackend) Name() string {
	return "rod"
}

// SeedSession restores a saved login into the already-open fresh browser:
// cookies for auth, then localStorage/sessionStorage, then one reload.
func (b *RodBackend) SeedSession(ctx context.Context, login model.LoginInfo) error {
	if b.page == nil || b.browser == nil {
		return fmt.Errorf("rod browser is not ready for session restore")
	}
	if len(login.Cookies) > 0 {
		cookies := make([]*proto.NetworkCookieParam, 0, len(login.Cookies))
		for _, cookie := range login.Cookies {
			if strings.TrimSpace(cookie.Name) == "" || cookie.Value == "" {
				continue
			}
			param := &proto.NetworkCookieParam{
				Name:     cookie.Name,
				Value:    cookie.Value,
				Domain:   cookie.Domain,
				Path:     cookie.Path,
				Secure:   cookie.Secure,
				HTTPOnly: cookie.HTTPOnly,
			}
			if param.Domain == "" {
				param.Domain = ".chitchat.gg"
			}
			if param.Path == "" {
				param.Path = "/"
			}
			if cookie.Expires > 0 {
				param.Expires = proto.TimeSinceEpoch(cookie.Expires)
			}
			cookies = append(cookies, param)
		}
		if len(cookies) > 0 {
			if err := b.page.SetCookies(cookies); err != nil {
				return fmt.Errorf("set saved cookies: %w", err)
			}
		}
	}
	if len(login.LocalStorage) > 0 || len(login.SessionStorage) > 0 {
		_, err := b.page.Eval(`(data) => {
			try {
				for (const [k, v] of Object.entries(data.local || {})) localStorage.setItem(k, v);
				for (const [k, v] of Object.entries(data.session || {})) sessionStorage.setItem(k, v);
			} catch (_) {}
			return true;
		}`, map[string]any{"local": login.LocalStorage, "session": login.SessionStorage})
		if err != nil {
			return fmt.Errorf("restore browser storage: %w", err)
		}
	}
	target := b.startURL
	if target == "" {
		target = "https://app.chitchat.gg/text"
	}
	if parsed, err := url.Parse(target); err == nil {
		// Drop the ?auth=email entry hint; with cookies present the app should
		// land directly in the chat UI, not the email-login screen.
		parsed.RawQuery = ""
		target = parsed.String()
	}
	if err := b.page.Navigate(target); err != nil {
		return fmt.Errorf("navigate after session restore: %w", err)
	}
	if err := b.page.WaitLoad(); err != nil {
		return fmt.Errorf("wait after session restore: %w", err)
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(1200 * time.Millisecond):
	}
	return nil
}

func (b *RodBackend) Start(ctx context.Context, options Options) error {
	var relay *proxy.Relay
	success := false
	if options.Proxy != nil {
		var err error
		relay, err = proxy.StartRelay(ctx, *options.Proxy)
		if err != nil {
			return err
		}
		options.Proxy = &relay.Entry
		defer func() {
			if !success {
				relay.Close()
			}
		}()
	}

	controlURL, err := launchRodBrowser(ctx, options)
	if err != nil {
		return err
	}

	browser := rod.New().
		ControlURL(controlURL).
		NoDefaultDevice()

	if err := browser.Connect(); err != nil {
		return err
	}

	if options.Proxy != nil && options.Proxy.HasAuth() {
		stopProxyAuth := startRodProxyAuth(ctx, browser, options.Proxy.Username, options.Proxy.Password)
		defer func() {
			if b.browser == nil && stopProxyAuth != nil {
				stopProxyAuth()
			}
		}()
		b.stopProxyAuth = stopProxyAuth
	}

	page, err := browser.Page(proto.TargetCreateTarget{URL: options.StartURL})
	if err != nil {
		_ = browser.Close()
		return err
	}

	if err := page.WaitLoad(); err != nil {
		_ = browser.Close()
		return err
	}

	success = true
	b.proxyRelay = relay
	b.browser = browser
	b.page = page
	b.startURL = options.StartURL
	b.disablePageRecovery = options.DisablePageRecovery
	return nil
}

func launchRodBrowser(ctx context.Context, options Options) (string, error) {
	debugPort, err := reserveDebugPort()
	if err != nil {
		return "", err
	}

	l := launcher.New().
		Context(ctx).
		UserDataDir(options.UserDataDir).
		Headless(options.Headless).
		RemoteDebuggingPort(debugPort)

	if runtime.GOOS == "windows" {
		// Windows Defender commonly flags Rod's leakless helper. Disable it and
		// let the app manage browser shutdown directly.
		l.Leakless(false)
	}

	if options.Proxy != nil {
		l.Proxy(options.Proxy.BrowserURL())
		l.Set("proxy-bypass-list", "<-loopback>")
	}

	bin, err := resolveRodBrowserBinary(options.UserDataDir)
	if err != nil {
		return "", err
	}
	l.Bin(bin)

	cmd := exec.Command(bin, l.FormatArgs()...)

	if err := cmd.Start(); err != nil {
		return "", err
	}

	exitCh := make(chan error, 1)
	go func() {
		exitCh <- cmd.Wait()
	}()

	controlURL, err := waitForRodDebugURL(ctx, exitCh, debugPort)
	if err != nil {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		return "", err
	}

	return controlURL, nil
}

func resolveRodBrowserBinary(userDataDir string) (string, error) {
	if bin, reason := runtimeassets.BundledChromiumPathForProfile(userDataDir); bin != "" {
		fmt.Printf("rod: using bundled browser: %s\n", bin)
		return bin, nil
	} else if reason != "" {
		fmt.Printf("rod: skipping bundled browser: %s\n", reason)
	}

	if bin, ok := launcher.LookPath(); ok {
		fmt.Printf("rod: using installed browser: %s\n", bin)
		return bin, nil
	}

	fmt.Println("rod: no installed browser found, falling back to managed Chromium download")
	return launcher.NewBrowser().Get()
}

func reserveDebugPort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()

	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		return 0, fmt.Errorf("unexpected listener address type %T", listener.Addr())
	}

	return addr.Port, nil
}

func waitForRodDebugURL(ctx context.Context, exitCh <-chan error, port int) (string, error) {
	endpoint := fmt.Sprintf("127.0.0.1:%d", port)
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		u, err := launcher.ResolveURL(endpoint)
		if err == nil {
			return u, nil
		}

		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case waitErr := <-exitCh:
			if waitErr == nil {
				return "", fmt.Errorf("rod browser exited before the debugging endpoint became ready")
			}
			return "", fmt.Errorf("rod browser exited before the debugging endpoint became ready: %w", waitErr)
		case <-ticker.C:
		}
	}
}

func (b *RodBackend) Close() error {
	if b.proxyRelay != nil {
		b.proxyRelay.Close()
	}
	if b.stopProxyAuth != nil {
		b.stopProxyAuth()
		b.stopProxyAuth = nil
	}
	if b.browser != nil {
		return b.browser.Close()
	}
	return nil
}

func startRodProxyAuth(parent context.Context, browser *rod.Browser, username string, password string) func() {
	restoreFetch := browser.EnableDomain("", &proto.FetchEnable{
		HandleAuthRequests: true,
	})

	authCtx, cancel := context.WithCancel(parent)
	go browser.Context(authCtx).EachEvent(
		func(e *proto.FetchRequestPaused) {
			if err := (proto.FetchContinueRequest{RequestID: e.RequestID}).Call(browser); err != nil && authCtx.Err() == nil {
				fmt.Printf("rod: failed to continue paused proxy request: %v\n", err)
			}
		},
		func(e *proto.FetchAuthRequired) {
			response := &proto.FetchAuthChallengeResponse{
				Response: proto.FetchAuthChallengeResponseResponseDefault,
			}

			if e.AuthChallenge != nil && e.AuthChallenge.Source == proto.FetchAuthChallengeSourceProxy {
				response = &proto.FetchAuthChallengeResponse{
					Response: proto.FetchAuthChallengeResponseResponseProvideCredentials,
					Username: username,
					Password: password,
				}
			}

			if err := (proto.FetchContinueWithAuth{
				RequestID:             e.RequestID,
				AuthChallengeResponse: response,
			}).Call(browser); err != nil && authCtx.Err() == nil {
				fmt.Printf("rod: failed to continue proxy auth challenge: %v\n", err)
			}
		},
	)()

	return func() {
		cancel()
		restoreFetch()
	}
}

// CheckBan detects the site's /banned page without relying on a fixed layout:
// the URL triggers it, then the page text supplies Ban ID / Reason / Remaining.
func (b *RodBackend) CheckBan(ctx context.Context) (BanReport, error) {
	if b.page == nil {
		return BanReport{}, nil
	}
	info, err := b.page.Info()
	if err != nil {
		return BanReport{}, err
	}
	if !isBannedURL(info.URL) {
		return BanReport{}, nil
	}
	report := BanReport{Banned: true, URL: info.URL}
	result, evalErr := b.page.Context(ctx).Eval(`() => document.body ? document.body.innerText : ''`)
	if evalErr == nil {
		report = ParseBanText(result.Value.Str(), info.URL)
	}
	return report, nil
}

func (b *RodBackend) CaptureSession(ctx context.Context) (model.LoginInfo, error) {
	var login model.LoginInfo

	err := b.withPageRetry(ctx, func(page *rod.Page) error {
		cookies, err := page.Cookies([]string{
			"https://app.chitchat.gg/",
			"https://api.chitchat.gg/",
		})
		if err != nil {
			return err
		}

		storageResult, err := page.Eval(storageFunction)
		if err != nil {
			return err
		}

		fingerprintResult, err := page.Eval(fingerprintFunction)
		if err != nil {
			return err
		}

		var storage struct {
			LocalStorage   map[string]string `json:"local_storage"`
			SessionStorage map[string]string `json:"session_storage"`
		}
		if err := storageResult.Value.Unmarshal(&storage); err != nil {
			return err
		}

		var fingerprint model.TransportFingerprint
		if err := fingerprintResult.Value.Unmarshal(&fingerprint); err != nil {
			return err
		}

		login = model.LoginInfo{
			Backend:              b.Name(),
			Cookies:              make([]model.Cookie, 0, len(cookies)),
			UserAgent:            fingerprint.UserAgent,
			LocalStorage:         storage.LocalStorage,
			SessionStorage:       storage.SessionStorage,
			TransportFingerprint: fingerprint,
		}

		for _, cookie := range cookies {
			login.Cookies = append(login.Cookies, model.Cookie{
				Name:     cookie.Name,
				Value:    cookie.Value,
				Domain:   cookie.Domain,
				Path:     cookie.Path,
				Expires:  float64(cookie.Expires),
				Secure:   cookie.Secure,
				HTTPOnly: cookie.HTTPOnly,
			})
		}

		return nil
	})

	return login, err
}

func (b *RodBackend) Fetch(ctx context.Context, req FetchRequest) (FetchResponse, error) {
	payload := map[string]any{
		"url":     req.URL,
		"method":  req.Method,
		"headers": req.Headers,
	}
	if req.Body != nil {
		payload["body"] = *req.Body
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return FetchResponse{}, err
	}

	var response FetchResponse
	err = b.withPageRetry(ctx, func(page *rod.Page) error {
		result, err := page.Eval(fetchFunction, json.RawMessage(payloadJSON))
		if err != nil {
			return err
		}
		return result.Value.Unmarshal(&response)
	})
	if err != nil {
		return FetchResponse{}, err
	}

	return response, nil
}

func (b *RodBackend) withPageRetry(ctx context.Context, run func(page *rod.Page) error) error {
	page, err := b.currentPage(ctx)
	if err != nil {
		return err
	}

	err = run(page)
	if !isRecoverablePageError(err) {
		return err
	}
	if b.disablePageRecovery {
		return ErrPageUnavailable
	}

	fmt.Println("rod: page session was lost, recovering active page")

	page, recoverErr := b.recoverPage(ctx)
	if recoverErr != nil {
		return recoverErr
	}

	return run(page)
}

func (b *RodBackend) currentPage(ctx context.Context) (*rod.Page, error) {
	if b.page == nil {
		return nil, ErrPageUnavailable
	}
	return b.page.Context(ctx), nil
}

func (b *RodBackend) recoverPage(ctx context.Context) (*rod.Page, error) {
	if b.browser == nil {
		return nil, ErrPageUnavailable
	}

	pages, err := b.browser.Pages()
	if err != nil {
		return nil, err
	}

	for _, candidate := range pages {
		info, err := candidate.Info()
		if err != nil {
			if isRecoverablePageError(err) {
				continue
			}
			continue
		}

		if strings.HasPrefix(info.URL, b.startURL) || strings.HasPrefix(info.URL, "https://app.chitchat.gg/") {
			b.page = candidate
			return candidate.Context(ctx), nil
		}
	}

	page, err := b.browser.Page(proto.TargetCreateTarget{URL: b.startURL})
	if err != nil {
		return nil, err
	}

	if err := page.WaitLoad(); err != nil && !isRecoverablePageError(err) {
		return nil, err
	}

	b.page = page
	return page.Context(ctx), nil
}

func isRecoverablePageError(err error) bool {
	return errors.Is(err, cdp.ErrSessionNotFound) ||
		errors.Is(err, cdp.ErrNotAttachedToActivePage) ||
		errors.Is(err, cdp.ErrCtxNotFound) ||
		errors.Is(err, cdp.ErrCtxDestroyed)
}
