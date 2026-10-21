package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"chitchatbot/internal/appdirs"
	"chitchatbot/internal/model"
	"chitchatbot/internal/settings"
)

func strPtr(v string) *string { return &v }

func TestDashboardGlobalSettingsApplyToAllSitesWithoutOverride(t *testing.T) {
	a, err := NewApp(t.TempDir())
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { _ = a.uiStore.Close() })

	wantGlobalPath := filepath.Join(appdirs.SitesRoot(a.dataDir), "_global", "settings.json")
	if _, err := os.Stat(wantGlobalPath); err != nil {
		t.Fatalf("expected seeded %s: %v", wantGlobalPath, err)
	}

	// 1. Save global settings via dashboard actions (Message, Proxy, General).
	msg := model.NewMessageSettings("hello\nreply", "user1")
	msg.ChatMode = model.ChatModeEVAChat
	msg.AIEnabled = true
	msg.AIBaseURL = "https://api.example.test/v1"
	msg.AIAPIKey = "sk-global-key"
	msg.AIModel = "global-model-v1"
	msg.APITemperature = 0.65
	if err := a.SaveMessageSettings(msg); err != nil {
		t.Fatalf("SaveMessageSettings: %v", err)
	}

	if err := a.SaveProxySettings(model.ProxySettings{
		Enabled: true,
		Entries: "http://10.0.0.1:8080\nsocks5://10.0.0.2:1080",
	}); err != nil {
		t.Fatalf("SaveProxySettings: %v", err)
	}

	gen := model.DefaultGeneralSettings()
	gen.ReplyDelayMinSeconds = 4
	gen.ReplyDelayMaxSeconds = 9
	gen.MatchReplyTimeoutMinSeconds = 45
	gen.MatchReplyTimeoutMaxSeconds = 75
	gen.FirstMessage = model.FirstMessageSettings{
		Mode:            "random",
		BotFirstPercent: 55,
		WaitTimeout:     true,
		WaitMinSeconds:  15,
		WaitMaxSeconds:  30,
	}
	if err := a.SaveGeneralSettings(gen); err != nil {
		t.Fatalf("SaveGeneralSettings: %v", err)
	}

	// Verify all 102 sites receive these global settings by default.
	summaries, err := a.ListSiteSettings()
	if err != nil {
		t.Fatalf("ListSiteSettings: %v", err)
	}
	if len(summaries) != 102 {
		t.Fatalf("expected 102 site summaries, got %d", len(summaries))
	}
	for _, s := range summaries {
		if len(s.Overrides) != 0 || s.HasFile {
			t.Fatalf("site %s unexpectedly has override: %+v", s.SiteID, s)
		}
		eff := s.Effective.Settings
		if eff.MessageMode != model.ChatModeEVAChat {
			t.Fatalf("site %s message_mode = %q, want %q", s.SiteID, eff.MessageMode, model.ChatModeEVAChat)
		}
		if !eff.Proxy.Enabled || !strings.Contains(eff.Proxy.Entries, "10.0.0.1:8080") {
			t.Fatalf("site %s proxy = %+v", s.SiteID, eff.Proxy)
		}
		if eff.ReplyDelay != (settings.DurationRange{MinSeconds: 4, MaxSeconds: 9}) {
			t.Fatalf("site %s reply_delay = %+v", s.SiteID, eff.ReplyDelay)
		}
		if eff.SkipTimeout != (settings.DurationRange{MinSeconds: 45, MaxSeconds: 75}) {
			t.Fatalf("site %s skip_timeout = %+v", s.SiteID, eff.SkipTimeout)
		}
		if eff.FirstMessage.Mode != "random" || eff.FirstMessage.BotFirstPercent != 55 {
			t.Fatalf("site %s first_message = %+v", s.SiteID, eff.FirstMessage)
		}
		if eff.API.Model != "global-model-v1" || eff.API.BaseURL != "https://api.example.test/v1" {
			t.Fatalf("site %s api = %+v", s.SiteID, eff.API)
		}
	}

	// 2. Set a per-site override on "omezy" (custom proxy + custom message mode).
	customProxy := model.ProxySettings{
		Enabled: true,
		Entries: "socks5://192.168.99.99:9050",
	}
	omezyEff, err := a.SaveSiteSettingsOverride("omezy", settings.Override{
		MessageMode: strPtr(model.ChatModeSingle),
		Proxy:       &customProxy,
	})
	if err != nil {
		t.Fatalf("SaveSiteSettingsOverride(omezy): %v", err)
	}
	if omezyEff.Settings.MessageMode != model.ChatModeSingle {
		t.Fatalf("omezy message_mode = %q, want single", omezyEff.Settings.MessageMode)
	}
	if omezyEff.Settings.Proxy.Entries != "socks5://192.168.99.99:9050" {
		t.Fatalf("omezy proxy = %+v", omezyEff.Settings.Proxy)
	}
	// Non-overridden fields on omezy still come from global.
	if omezyEff.Settings.ReplyDelay != (settings.DurationRange{MinSeconds: 4, MaxSeconds: 9}) {
		t.Fatalf("omezy reply_delay = %+v, want global 4..9", omezyEff.Settings.ReplyDelay)
	}
	if omezyEff.Settings.SkipTimeout != (settings.DurationRange{MinSeconds: 45, MaxSeconds: 75}) {
		t.Fatalf("omezy skip_timeout = %+v, want global 45..75", omezyEff.Settings.SkipTimeout)
	}

	// Selecting omezy in the dashboard reflects omezy's effective settings.
	if _, err := a.SelectSite("omezy"); err != nil {
		t.Fatalf("SelectSite(omezy): %v", err)
	}
	snapOmezy, err := a.GetInitialState()
	if err != nil {
		t.Fatalf("GetInitialState(omezy): %v", err)
	}
	if snapOmezy.Message.ChatMode != model.ChatModeSingle {
		t.Fatalf("snapshot for omezy chat_mode = %q, want single", snapOmezy.Message.ChatMode)
	}
	if snapOmezy.Proxy.Entries != "socks5://192.168.99.99:9050" {
		t.Fatalf("snapshot for omezy proxy = %+v", snapOmezy.Proxy)
	}

	// Selecting another site ("funchatx") still gets the global settings.
	if _, err := a.SelectSite("funchatx"); err != nil {
		t.Fatalf("SelectSite(funchatx): %v", err)
	}
	snapFunchatx, err := a.GetInitialState()
	if err != nil {
		t.Fatalf("GetInitialState(funchatx): %v", err)
	}
	if snapFunchatx.Message.ChatMode != model.ChatModeEVAChat {
		t.Fatalf("snapshot for funchatx chat_mode = %q, want eva_chat_flow", snapFunchatx.Message.ChatMode)
	}
	if !strings.Contains(snapFunchatx.Proxy.Entries, "10.0.0.1:8080") {
		t.Fatalf("snapshot for funchatx proxy = %+v", snapFunchatx.Proxy)
	}

	// 3. Updating global settings from the dashboard updates all non-overridden
	// fields across all sites, while omezy keeps its proxy + message_mode override.
	gen.ReplyDelayMinSeconds = 12
	gen.ReplyDelayMaxSeconds = 18
	if err := a.SaveGeneralSettings(gen); err != nil {
		t.Fatalf("SaveGeneralSettings update: %v", err)
	}
	omezyAfter, err := a.GetSiteSettings("omezy")
	if err != nil {
		t.Fatalf("GetSiteSettings(omezy): %v", err)
	}
	if omezyAfter.Settings.ReplyDelay != (settings.DurationRange{MinSeconds: 12, MaxSeconds: 18}) {
		t.Fatalf("omezy did not inherit updated global reply_delay: %+v", omezyAfter.Settings.ReplyDelay)
	}
	if omezyAfter.Settings.MessageMode != model.ChatModeSingle || omezyAfter.Settings.Proxy.Entries != "socks5://192.168.99.99:9050" {
		t.Fatalf("omezy override was lost after global update: %+v", omezyAfter.Settings)
	}

	// 4. Clearing omezy's override restores global inheritance.
	cleared, err := a.ClearSiteSettingsOverride("omezy")
	if err != nil {
		t.Fatalf("ClearSiteSettingsOverride(omezy): %v", err)
	}
	if cleared.Settings.MessageMode != model.ChatModeEVAChat || !strings.Contains(cleared.Settings.Proxy.Entries, "10.0.0.1:8080") {
		t.Fatalf("omezy did not return to global settings after clear: %+v", cleared.Settings)
	}

	// 5. SaveGlobalSettings directly updates sites/_global/settings.json and
	// applies to all sites while redacting API keys in UI views.
	view, err := a.SaveGlobalSettings(settings.Settings{
		MessageMode: model.ChatModeAPI,
		Proxy: model.ProxySettings{
			Enabled: true,
			Entries: "http://user:secretpass@10.0.0.9:8080",
		},
		ReplyDelay:  settings.DurationRange{MinSeconds: 2, MaxSeconds: 5},
		SkipTimeout: settings.DurationRange{MinSeconds: 30, MaxSeconds: 45},
		FirstMessage: model.FirstMessageSettings{
			Mode:            "bot",
			BotFirstPercent: 80,
			WaitMinSeconds:  10,
			WaitMaxSeconds:  20,
		},
		API: settings.APISettings{
			Enabled:     true,
			BaseURL:     "https://api.example.test/v1",
			APIKey:      "sk-top-secret",
			Model:       "gpt-test",
			Temperature: 0.5,
		},
	})
	if err != nil {
		t.Fatalf("SaveGlobalSettings: %v", err)
	}
	if !view.APIKeySet || view.Settings.API.APIKey != "[REDACTED]" {
		t.Fatalf("expected redacted API key in view, got %+v", view)
	}
	if strings.Contains(view.Settings.Proxy.Entries, "secretpass") {
		t.Fatalf("expected redacted proxy password in view, got %q", view.Settings.Proxy.Entries)
	}
	// Worker-facing effective helpers receive unredacted credentials.
	workerMsg, err := a.effectiveMessageSettings("omezy")
	if err != nil {
		t.Fatalf("effectiveMessageSettings: %v", err)
	}
	if workerMsg.ChatMode != model.ChatModeAPI || workerMsg.AIAPIKey != "sk-top-secret" || workerMsg.AIModel != "gpt-test" {
		t.Fatalf("worker effective message settings = %+v", workerMsg)
	}
	workerProxy, err := a.effectiveProxySettings("omezy")
	if err != nil {
		t.Fatalf("effectiveProxySettings: %v", err)
	}
	if !strings.Contains(workerProxy.Entries, "secretpass") {
		t.Fatalf("worker effective proxy lost credentials: %+v", workerProxy)
	}
}
