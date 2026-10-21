package sites

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestFiveClassificationGroupsAreDefined(t *testing.T) {
	groups := AllGroups()
	if len(groups) != 5 {
		t.Fatalf("expected 5 groups (A-E), got %d", len(groups))
	}
	want := map[string]string{
		GroupA: "Browserless",
		GroupB: "Auto-Signup",
		GroupC: "Browser-Once",
		GroupD: "DOM-Only",
		GroupE: "Manual",
	}
	for _, g := range groups {
		if want[g.Group] != g.Name {
			t.Fatalf("group %s name = %q, want %q", g.Group, g.Name, want[g.Group])
		}
		if g.Description == "" {
			t.Fatalf("group %s has empty description", g.Group)
		}
	}
}

func TestProbeAndAutoClassifyAllFiveGroups(t *testing.T) {
	cases := []struct {
		name      string
		status    int
		headers   map[string]string
		body      string
		wantGroup string
		check     func(t *testing.T, p ProbeResult)
	}{
		{
			name:   "Group A Browserless (direct WS chat, no signup, no captcha, no login)",
			status: http.StatusOK,
			body: `<!doctype html><html><body>
				<div id="chat">Connected! Say hi</div>
				<script>const ws = new WebSocket("wss://chat.example.org/ws/match");</script>
			</body></html>`,
			wantGroup: GroupA,
			check: func(t *testing.T, p ProbeResult) {
				if !p.HasWebSocket || p.HasCaptcha || p.HasSignupForm || p.LoginRequired {
					t.Fatalf("unexpected probe signals for Group A: %+v", p)
				}
			},
		},
		{
			name:   "Group B Auto-Signup (signup form + API endpoint, no captcha)",
			status: http.StatusOK,
			body: `<!doctype html><html><body>
				<form action="/register" method="POST">
					<input name="nickname" placeholder="Pick a nickname" />
					<select name="gender"><option>Female</option></select>
					<input name="age" value="22" />
				</form>
				<script>fetch("/api/v1/guest/signup", {method:"POST"});</script>
			</body></html>`,
			wantGroup: GroupB,
			check: func(t *testing.T, p ProbeResult) {
				if !p.HasSignupForm || !p.HasAPIEndpoint || p.HasCaptcha {
					t.Fatalf("unexpected probe signals for Group B: %+v", p)
				}
			},
		},
		{
			name:   "Group C Browser-Once (hCaptcha/reCAPTCHA + WebSocket/API session)",
			status: http.StatusOK,
			body: `<!doctype html><html><body>
				<div class="h-captcha" data-sitekey="1234"></div>
				<script src="https://js.hcaptcha.com/1/api.js"></script>
				<script>const sock = io.connect("wss://realtime.example.org/socket.io/");</script>
			</body></html>`,
			wantGroup: GroupC,
			check: func(t *testing.T, p ProbeResult) {
				if !p.HasCaptcha || !p.HasWebSocket || p.HeavyBotDetection {
					t.Fatalf("unexpected probe signals for Group C: %+v", p)
				}
			},
		},
		{
			name:   "Group D DOM-Only (no API/WS exposed, no captcha, pure DOM chat)",
			status: http.StatusOK,
			body: `<!doctype html><html><body>
				<div class="chat-messages"><div class="message-row">Hello stranger</div></div>
				<textarea placeholder="Type a message..."></textarea>
				<button class="send-btn">Send</button>
			</body></html>`,
			wantGroup: GroupD,
			check: func(t *testing.T, p ProbeResult) {
				if p.HasWebSocket || p.HasAPIEndpoint || p.HasCaptcha || p.HeavyBotDetection {
					t.Fatalf("unexpected probe signals for Group D: %+v", p)
				}
			},
		},
		{
			name:   "Group E Manual (Cloudflare challenge / heavy bot detection)",
			status: http.StatusForbidden,
			headers: map[string]string{
				"Cf-Mitigated": "challenge",
			},
			body: `<!doctype html><html><head><title>Just a moment...</title></head><body>
				<div class="cf-turnstile"></div>
				<script src="https://challenges.cloudflare.com/turnstile/v0/api.js"></script>
				Enable JavaScript and cookies to continue
			</body></html>`,
			wantGroup: GroupE,
			check: func(t *testing.T, p ProbeResult) {
				if !p.HeavyBotDetection || !p.HasCaptcha {
					t.Fatalf("unexpected probe signals for Group E: %+v", p)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				for k, v := range tc.headers {
					w.Header().Set(k, v)
				}
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			probe, err := ProbeURL(context.Background(), srv.URL, srv.Client())
			if err != nil {
				t.Fatalf("ProbeURL: %v", err)
			}
			tc.check(t, probe)

			cls := ClassifyProbe(probe)
			if cls.Group != tc.wantGroup {
				t.Fatalf("ClassifyProbe group = %q (%s), want %q; reason=%s", cls.Group, cls.GroupName, tc.wantGroup, cls.Reason)
			}
			if cls.Confidence != ConfidenceHigh {
				t.Fatalf("ClassifyProbe confidence = %q, want %q", cls.Confidence, ConfidenceHigh)
			}
		})
	}
}

func TestAll102SitesHaveInitialLowConfidenceClassificationInSiteJSON(t *testing.T) {
	dataDir := t.TempDir()
	root, err := EnsureSites(dataDir)
	if err != nil {
		t.Fatalf("EnsureSites: %v", err)
	}

	list, err := List(dataDir)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 102 {
		t.Fatalf("expected 102 sites, got %d", len(list))
	}

	validGroups := map[string]bool{
		GroupA: true,
		GroupB: true,
		GroupC: true,
		GroupD: true,
		GroupE: true,
	}
	for _, cfg := range list {
		if !validGroups[cfg.Classification.Group] {
			t.Fatalf("site %s has invalid classification group %q", cfg.ID, cfg.Classification.Group)
		}
		if cfg.Classification.Confidence != ConfidenceLow {
			t.Fatalf("site %s initial confidence = %q, want %q", cfg.ID, cfg.Classification.Confidence, ConfidenceLow)
		}
		// Verify it is actually persisted inside sites/<site_id>/site.json on disk.
		raw, err := os.ReadFile(filepath.Join(root, cfg.ID, "site.json"))
		if err != nil {
			t.Fatalf("read %s/site.json: %v", cfg.ID, err)
		}
		var onDisk map[string]any
		if err := json.Unmarshal(raw, &onDisk); err != nil {
			t.Fatalf("parse %s/site.json: %v", cfg.ID, err)
		}
		clsMap, ok := onDisk["classification"].(map[string]any)
		if !ok {
			t.Fatalf("site %s site.json missing classification object", cfg.ID)
		}
		if clsMap["group"] != cfg.Classification.Group || clsMap["confidence"] != ConfidenceLow {
			t.Fatalf("site %s on-disk classification = %+v", cfg.ID, clsMap)
		}
	}
}

func TestClassificationIsRerunnableAndPersistsToSiteJSON(t *testing.T) {
	dataDir := t.TempDir()
	if _, err := EnsureSites(dataDir); err != nil {
		t.Fatalf("EnsureSites: %v", err)
	}

	// Start with a Group A (direct WebSocket, no signup, no captcha) response,
	// then switch the server to Group C (recaptcha + login gate) and re-probe.
	mode := "group_a"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if mode == "group_a" {
			_, _ = w.Write([]byte(`<html><script>new WebSocket("wss://omezy.test/ws")</script></html>`))
			return
		}
		_, _ = w.Write([]byte(`<html>
			<form action="/login"><input type="password" name="password"/></form>
			<div class="g-recaptcha" data-sitekey="abc"></div>
			<script>fetch("/api/v1/session")</script>
		</html>`))
	}))
	defer srv.Close()

	// Point omezy's start_url at our test server.
	omezyCtx, err := NewSiteContext(dataDir, "omezy")
	if err != nil {
		t.Fatal(err)
	}
	var omezyCfg Config
	if _, err := omezyCtx.LoadJSON("site.json", &omezyCfg); err != nil {
		t.Fatal(err)
	}
	omezyCfg.StartURL = srv.URL
	omezyCfg.AppURL = srv.URL
	if err := omezyCtx.SaveJSON("site.json", omezyCfg); err != nil {
		t.Fatal(err)
	}

	// 1st probe -> Group A, confidence high, probe_count = 1
	first, err := ProbeAndClassifySite(context.Background(), dataDir, "omezy", srv.Client())
	if err != nil {
		t.Fatalf("1st ProbeAndClassifySite: %v", err)
	}
	if first.Classification.Group != GroupA || first.Classification.Confidence != ConfidenceHigh {
		t.Fatalf("1st probe classification = %+v, want Group A / high", first.Classification)
	}
	if first.Classification.ProbeCount != 1 {
		t.Fatalf("1st probe count = %d, want 1", first.Classification.ProbeCount)
	}

	// 2nd probe after site adds reCAPTCHA + login -> Group C, confidence high, probe_count = 2
	mode = "group_c"
	second, err := ProbeAndClassifySite(context.Background(), dataDir, "omezy", srv.Client())
	if err != nil {
		t.Fatalf("2nd ProbeAndClassifySite: %v", err)
	}
	if second.Classification.Group != GroupC || second.Classification.Confidence != ConfidenceHigh {
		t.Fatalf("2nd probe classification = %+v, want Group C / high", second.Classification)
	}
	if second.Classification.ProbeCount != 2 {
		t.Fatalf("2nd probe count = %d, want 2", second.Classification.ProbeCount)
	}
	if second.Classification.Probe == nil || !second.Classification.Probe.HasCaptcha || !second.Classification.Probe.LoginRequired {
		t.Fatalf("2nd probe details not saved: %+v", second.Classification.Probe)
	}

	// Verify LoadSite reads the updated classification from sites/omezy/site.json,
	// while other sites (e.g. funchatx) remain untouched at probe_count = 0 / low confidence.
	reloaded, err := LoadSite(dataDir, "omezy")
	if err != nil {
		t.Fatalf("LoadSite(omezy): %v", err)
	}
	if reloaded.Classification.Group != GroupC || reloaded.Classification.ProbeCount != 2 {
		t.Fatalf("reloaded classification = %+v", reloaded.Classification)
	}
	other, err := LoadSite(dataDir, "funchatx")
	if err != nil {
		t.Fatalf("LoadSite(funchatx): %v", err)
	}
	if other.Classification.ProbeCount != 0 || other.Classification.Confidence != ConfidenceLow {
		t.Fatalf("untouched site funchatx was modified: %+v", other.Classification)
	}
}
