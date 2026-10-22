package browser

// Show Mode only: visible page DOM + native browser input. The normal Backend,
// Rod/ChromeDP login capture, proxy setup and worker transports are unchanged.
import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/cdp"
	"github.com/go-rod/rod/lib/input"

	"chitchatbot/internal/model"
)

// DOMSessionConnection holds isolated session cookies, localStorage, and
// headless configuration for launching a Group D site in headless Rod (Prompt 11).
type DOMSessionConnection struct {
	SiteID           string
	StartURL         string
	UserDataDir      string
	Headless         bool
	CustomDOMJS      string
	ComposerSelector string
	Login            model.LoginInfo
}

// ConnectHeadlessDOMSession launches a headless Rod browser using the site's
// isolated UserDataDir, configures its site DOM driver, and injects saved
// cookies and localStorage before navigating (Prompt 11).
func ConnectHeadlessDOMSession(ctx context.Context, backend Backend, conn DOMSessionConnection) error {
	if backend == nil {
		return errors.New("browser backend is nil")
	}
	if configurer, ok := backend.(SiteDOMConfigurer); ok && strings.TrimSpace(conn.CustomDOMJS) != "" {
		configurer.ConfigureSiteDOM(conn.CustomDOMJS, conn.ComposerSelector)
	}
	if err := backend.Start(ctx, Options{
		StartURL:    conn.StartURL,
		UserDataDir: conn.UserDataDir,
		Headless:    conn.Headless,
	}); err != nil {
		return err
	}
	if len(conn.Login.Cookies) > 0 || len(conn.Login.LocalStorage) > 0 {
		_ = backend.RestoreSession(ctx, conn.Login)
	}
	return nil
}

type DOMMessage struct {
	ID     string `json:"id"`
	Order  int    `json:"order"`
	Author string `json:"author"`
	Text   string `json:"text"`
}

type DOMControls struct {
	HasInput      bool     `json:"hasInput"`
	InputDesc     string   `json:"inputDesc"`
	Buttons       []string `json:"buttons"`
	ChatEnded     bool     `json:"chatEnded"`
	Partner       string   `json:"partner"`
	OwnName       string   `json:"ownName"`
	Conversation  string   `json:"conversation"`
	Phase         string   `json:"phase"`
	Protection    string   `json:"protection,omitempty"`
	CanSend       bool     `json:"canSend"`
	ObserverReady bool     `json:"observerReady"`
	Overflow      bool     `json:"overflow"`
}

type DOMSnapshot struct {
	Controls DOMControls  `json:"controls"`
	Messages []DOMMessage `json:"messages"`
	Revision int          `json:"revision"`
}

// Optional Show Mode surface only; the shared login Backend is not modified.
type DOMChatBackend interface {
	ReadDOMSnapshot(context.Context) (DOMSnapshot, error)
	SendDOMMessage(context.Context, string) (bool, error)
	ClickDOMButton(context.Context, []string) (string, error)
	DOMProbe(context.Context) (string, error)
}

// SiteDOMConfigurer allows an isolated site folder (sites/<site_id>/) to supply
// its own DOM driver JS and composer input selector without touching the default
// ChitChat DOM observer.
type SiteDOMConfigurer interface {
	ConfigureSiteDOM(customJS, composerSelector string)
}

// Embedded verbatim so browser-free tests execute the actual production JS.
//
//go:embed showmode_dom.js
var domLibJS string

func (b *RodBackend) activeDOMJS() string {
	if strings.TrimSpace(b.customDOMJS) != "" {
		return b.customDOMJS
	}
	return domLibJS
}

func (b *RodBackend) activeComposerSelector() string {
	if strings.TrimSpace(b.composerSelector) != "" {
		return b.composerSelector
	}
	return `[data-chat-composer="true"] textarea[aria-label="Send a message"]`
}

func (b *RodBackend) evalDOM(ctx context.Context, expression string, args []any, out any) error {
	return b.withPageRetry(ctx, func(page *rod.Page) error {
		res, err := page.Eval(expression, args...)
		// A reload can destroy only the JS context while the same page is alive.
		// Retry this read-only evaluation on that page, never relaunch a browser.
		for attempt := 0; attempt < 2 && (errors.Is(err, cdp.ErrCtxDestroyed) || errors.Is(err, cdp.ErrCtxNotFound)); attempt++ {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(150 * time.Millisecond):
			}
			res, err = page.Eval(expression, args...)
		}
		if err != nil {
			return err
		}
		if out == nil {
			return nil
		}
		return res.Value.Unmarshal(out)
	})
}

func (b *RodBackend) ReadDOMSnapshot(ctx context.Context) (DOMSnapshot, error) {
	var snapshot DOMSnapshot
	err := b.evalDOM(ctx, "() => { "+b.activeDOMJS()+" return __sm.snapshot(); }", nil, &snapshot)
	return snapshot, err
}

func (b *RodBackend) DOMProbe(ctx context.Context) (string, error) {
	var summary string
	err := b.evalDOM(ctx, "() => { "+b.activeDOMJS()+" return __sm.probe(); }", nil, &summary)
	return summary, err
}

func (b *RodBackend) ClickDOMButton(ctx context.Context, labels []string) (string, error) {
	// Do not retry side effects on a recovered page: a click may already have run.
	page, err := b.currentPage(ctx)
	if err != nil {
		return "", err
	}
	res, err := page.Eval("(labels) => { "+b.activeDOMJS()+" return __sm.click(labels); }", labels)
	if err != nil {
		return "", err
	}
	var clicked string
	err = res.Value.Unmarshal(&clicked)
	return clicked, err
}

// SendDOMMessage uses CDP browser input/Enter, not page fetch, WebSocket,
// form.submit or synthetic KeyboardEvents. Success requires a NEW own row ID.
// An uncertain delivery is never automatically retried (avoids duplicate sends).
func (b *RodBackend) SendDOMMessage(ctx context.Context, text string) (bool, error) {
	return b.SendDOMMessageObserved(ctx, text, nil)
}

// Optional Show Mode display hook. Every actual read is exposed, including the
// final confirmed row before an immediate Skip. No proposed/draft text is emitted.
func (b *RodBackend) SendDOMMessageObserved(ctx context.Context, text string, observe func(DOMSnapshot)) (bool, error) {
	before, err := b.ReadDOMSnapshot(ctx)
	if err != nil {
		return false, err
	}
	if observe != nil {
		observe(before)
	}
	if !before.Controls.CanSend || before.Controls.Overflow {
		return false, fmt.Errorf("Show Mode composer unavailable; no send attempted")
	}
	page, err := b.currentPage(ctx)
	if err != nil {
		return false, err
	}
	// Bound element waits; user edits are never overwritten.
	actionCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	page = page.Context(actionCtx)
	var el *rod.Element
	if strings.TrimSpace(b.customDOMJS) != "" {
		if tagged, tagErr := page.Timeout(600 * time.Millisecond).Element(`[data-sm-composer="true"]`); tagErr == nil && tagged != nil {
			el = tagged
		}
	}
	if el == nil {
		el, err = page.Element(b.activeComposerSelector())
		if err != nil {
			return false, err
		}
	}
	value, err := el.Property("value")
	if err != nil {
		return false, err
	}
	if value.Str() != "" {
		return false, fmt.Errorf("Show Mode composer has a draft; refusing to overwrite it")
	}
	if err = el.Input(strings.TrimSpace(text)); err != nil {
		return false, err
	}
	check, err := b.ReadDOMSnapshot(actionCtx)
	if err != nil {
		return false, err
	}
	if observe != nil {
		observe(check)
	}
	if !sameDOMConversation(before.Controls, check.Controls) || !check.Controls.CanSend {
		// A navigation/rematch occurred while typing. Never press Enter here.
		return false, fmt.Errorf("Show Mode conversation changed while typing; inspect the draft")
	}
	if err = el.Type(input.Enter); err != nil {
		return false, err
	}
	if strings.TrimSpace(b.customDOMJS) != "" {
		time.Sleep(220 * time.Millisecond)
		if rem, remErr := el.Property("value"); remErr == nil && strings.TrimSpace(rem.Str()) != "" {
			_, _ = b.ClickDOMButton(ctx, []string{"SEND"})
		}
	}
	deadline := time.NewTimer(8 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(150 * time.Millisecond)
	defer ticker.Stop()
	last := before
	for {
		after, err := b.ReadDOMSnapshot(ctx)
		if err != nil {
			return false, err
		}
		last = after
		if observe != nil {
			observe(after)
		}
		if !sameDOMConversation(before.Controls, after.Controls) {
			return false, fmt.Errorf("Show Mode conversation changed before send confirmation")
		}
		if confirmedDOMSend(before, after, strings.TrimSpace(text)) {
			return true, nil
		}
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-deadline.C:
			return false, fmt.Errorf("send not confirmed within 8s; rows before=%d after=%d", len(before.Messages), len(last.Messages))
		case <-ticker.C:
		}
	}
}

func sameDOMConversation(before, after DOMControls) bool {
	return before.Conversation != "" && before.Conversation == after.Conversation &&
		after.Phase == "chatting" && strings.EqualFold(before.Partner, after.Partner)
}

func confirmedDOMSend(before, after DOMSnapshot, text string) bool {
	if !sameDOMConversation(before.Controls, after.Controls) {
		return false
	}
	own := strings.TrimSpace(before.Controls.OwnName)
	partner := strings.TrimSpace(before.Controls.Partner)
	ids := make(map[string]bool, len(before.Messages))
	for _, m := range before.Messages {
		ids[m.ID] = true
	}
	for _, m := range after.Messages {
		if m.ID == "" || ids[m.ID] || m.Text != text {
			continue
		}
		if own != "" {
			if strings.EqualFold(m.Author, own) {
				return true
			}
			continue
		}
		// The current site no longer exposes our own profile control. Own rows
		// have no author label (and may later gain our own name), while partner
		// rows always carry the partner name, so a new row with the exact text
		// that is not attributed to the partner is our confirmed send.
		if partner != "" && !strings.EqualFold(m.Author, partner) {
			return true
		}
	}
	return false
}
