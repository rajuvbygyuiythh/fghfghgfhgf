package settings

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"chitchatbot/internal/appdirs"
	"chitchatbot/internal/model"
	"chitchatbot/internal/sites"
)

func intPtr(v int) *int { return &v }

func strPtr(v string) *string { return &v }

func openStore(t *testing.T) (*Store, string) {
	t.Helper()
	dataDir := t.TempDir()
	store, err := Open(dataDir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return store, dataDir
}

func TestGlobalSettingsLiveInSharedFolder(t *testing.T) {
	store, dataDir := openStore(t)
	want := filepath.Join(appdirs.SitesRoot(dataDir), "_global", "settings.json")
	if got := store.GlobalPath(); got != want {
		t.Fatalf("global path = %q, want %q", got, want)
	}
	if _, err := store.Save(Default()); err != nil {
		t.Fatalf("Save: %v", err)
	}
	info, err := os.Stat(want)
	if err != nil {
		t.Fatalf("settings.json not created: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("settings.json permissions = %v, want 0600", info.Mode().Perm())
	}
	// The shared folder must sit next to the site folders, never inside one.
	if !strings.HasSuffix(filepath.Dir(want), "_global") {
		t.Fatalf("settings.json must live inside _global, got %q", want)
	}
}

func TestSaveLoadRoundTripKeepsEveryRequiredField(t *testing.T) {
	store, _ := openStore(t)
	input := Settings{
		MessageMode: model.ChatModeEVAChat,
		Proxy: model.ProxySettings{
			Enabled: true,
			Entries: "http://user:pass@1.2.3.4:8080\nsocks5://5.6.7.8:1080",
		},
		ReplyDelay:   DurationRange{MinSeconds: 5, MaxSeconds: 15},
		SkipTimeout:  DurationRange{MinSeconds: 90, MaxSeconds: 120},
		FirstMessage: model.FirstMessageSettings{Mode: "random", BotFirstPercent: 40, WaitMinSeconds: 10, WaitMaxSeconds: 20},
		API: APISettings{
			Enabled:     true,
			BaseURL:     "https://api.example.test/v1",
			APIKey:      "sk-test-key",
			Model:       "test-model",
			Temperature: 0.7,
		},
	}
	if _, err := store.Save(input); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.MessageMode != model.ChatModeEVAChat {
		t.Fatalf("message mode = %q", got.MessageMode)
	}
	if !got.Proxy.Enabled || !strings.Contains(got.Proxy.Entries, "1.2.3.4:8080") {
		t.Fatalf("proxy = %+v", got.Proxy)
	}
	if got.ReplyDelay != (DurationRange{5, 15}) {
		t.Fatalf("reply delay = %+v", got.ReplyDelay)
	}
	if got.SkipTimeout != (DurationRange{90, 120}) {
		t.Fatalf("skip timeout = %+v", got.SkipTimeout)
	}
	if got.FirstMessage.Mode != "random" || got.FirstMessage.BotFirstPercent != 40 {
		t.Fatalf("first message = %+v", got.FirstMessage)
	}
	if got.API.APIKey != "sk-test-key" || got.API.Model != "test-model" || got.API.Temperature != 0.7 {
		t.Fatalf("api = %+v", got.API)
	}
	if got.UpdatedAt == "" {
		t.Fatal("Save must stamp updated_at")
	}
}

func TestBlankAPIKeyOnSaveKeepsStoredKey(t *testing.T) {
	store, _ := openStore(t)
	base := Default()
	base.API = APISettings{Enabled: true, BaseURL: DefaultAPIBaseURL, APIKey: "sk-stored", Model: "m1"}
	if _, err := store.Save(base); err != nil {
		t.Fatalf("Save: %v", err)
	}
	// The dashboard never echoes the key back, so an empty field must not wipe it.
	next := base
	next.API.APIKey = ""
	if _, err := store.Save(next); err != nil {
		t.Fatalf("Save blank key: %v", err)
	}
	got, _ := store.Load()
	if got.API.APIKey != "sk-stored" {
		t.Fatalf("stored key was lost: %q", got.API.APIKey)
	}
}

func TestLoadMissingFileReturnsDefaults(t *testing.T) {
	store, _ := openStore(t)
	got, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := Default()
	if got.MessageMode != want.MessageMode || got.API.BaseURL != want.API.BaseURL {
		t.Fatalf("defaults not applied: %+v", got)
	}
	if got.ReplyDelay.MaxSeconds != want.ReplyDelay.MaxSeconds {
		t.Fatalf("reply delay default = %+v", got.ReplyDelay)
	}
}

func TestCorruptFileFallsBackAndReportsError(t *testing.T) {
	store, _ := openStore(t)
	if err := os.WriteFile(store.GlobalPath(), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := store.Load()
	if err == nil {
		t.Fatal("expected an error for a corrupt file")
	}
	if got.MessageMode != Default().MessageMode {
		t.Fatalf("corrupt file must fall back to defaults, got %+v", got)
	}
}

func TestMergeUsesSiteOverrideOnlyForOverriddenSections(t *testing.T) {
	global := Default()
	global.MessageMode = model.ChatModeAPI
	global.Proxy = model.ProxySettings{Enabled: true, Entries: "http://global:1"}
	global.ReplyDelay = DurationRange{MinSeconds: 3, MaxSeconds: 6}
	global.SkipTimeout = DurationRange{MinSeconds: 30, MaxSeconds: 60}
	global.FirstMessage = model.FirstMessageSettings{Mode: "bot", BotFirstPercent: 80, WaitMinSeconds: 1, WaitMaxSeconds: 2}
	global.API = APISettings{Enabled: true, BaseURL: "https://global.test/v1", APIKey: "global-key", Model: "global-model"}

	override := Override{
		MessageMode: strPtr(model.ChatModeFolder),
		Proxy:       &model.ProxySettings{Enabled: true, Entries: "socks5://site-only:9"},
	}

	effective := Merge(global, override)
	if effective.Settings.MessageMode != model.ChatModeFolder {
		t.Fatalf("message mode = %q, want folder", effective.Settings.MessageMode)
	}
	if effective.Settings.Proxy.Entries != "socks5://site-only:9" {
		t.Fatalf("proxy = %+v, want the site entry", effective.Settings.Proxy)
	}
	// Everything the override does not touch stays global.
	if effective.Settings.ReplyDelay != global.ReplyDelay {
		t.Fatalf("reply delay leaked: %+v", effective.Settings.ReplyDelay)
	}
	if effective.Settings.SkipTimeout != global.SkipTimeout {
		t.Fatalf("skip timeout leaked: %+v", effective.Settings.SkipTimeout)
	}
	if effective.Settings.FirstMessage != global.FirstMessage {
		t.Fatalf("first message leaked: %+v", effective.Settings.FirstMessage)
	}
	if effective.Settings.API != global.API {
		t.Fatalf("api leaked: %+v", effective.Settings.API)
	}
	if !effective.Overridden("message_mode") || !effective.Overridden("proxy") {
		t.Fatalf("sources = %+v", effective.Sources)
	}
	if effective.Overridden("reply_delay") || effective.Overridden("api") {
		t.Fatalf("unexpected override sources: %+v", effective.Sources)
	}
}

func TestMergeNeverMutatesInputs(t *testing.T) {
	global := Default()
	global.MessageMode = model.ChatModeAPI
	override := Override{MessageMode: strPtr(model.ChatModeSingle)}

	_ = Merge(global, override)
	if global.MessageMode != model.ChatModeAPI {
		t.Fatalf("Merge mutated the global value: %q", global.MessageMode)
	}
	if override.MessageMode == nil || *override.MessageMode != model.ChatModeSingle {
		t.Fatal("Merge mutated the override")
	}
}

func TestAPIOverrideReplacesWholeSection(t *testing.T) {
	global := Default()
	global.API = APISettings{Enabled: true, BaseURL: "https://global.test", APIKey: "global-key", Model: "global-model"}
	override := Override{API: &APISettings{Enabled: true, APIKey: "site-key", Model: "site-model"}}

	effective := Merge(global, override)
	if effective.Settings.API.APIKey != "site-key" || effective.Settings.API.Model != "site-model" {
		t.Fatalf("api override ignored: %+v", effective.Settings.API)
	}
	if effective.Settings.API.BaseURL != DefaultAPIBaseURL {
		t.Fatalf("api base url = %q, want the default, not the global one", effective.Settings.API.BaseURL)
	}
}

func TestOverrideIsStoredInsideTheSiteFolder(t *testing.T) {
	store, dataDir := openStore(t)
	if _, err := store.SaveOverride("omezy", Override{ReplyDelay: &DurationRange{MinSeconds: 7, MaxSeconds: 9}}); err != nil {
		t.Fatalf("SaveOverride: %v", err)
	}

	siteDir := filepath.Join(appdirs.SitesRoot(dataDir), "omezy")
	path := filepath.Join(siteDir, OverrideFileName)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("override file missing in the site folder: %v", err)
	}
	// No other site may receive a copy.
	other := filepath.Join(appdirs.SitesRoot(dataDir), "funchatx", OverrideFileName)
	if _, err := os.Stat(other); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("another site gained an override file (err=%v)", err)
	}
	// And nothing global changed.
	if _, err := os.Stat(store.GlobalPath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("saving an override must not create the global file")
	}
}

func TestOverrideRejectsUnsafeSiteID(t *testing.T) {
	store, _ := openStore(t)
	for _, id := range []string{"", ".", "..", "../evil", "a/b", `a\b`, sites.GlobalDirName} {
		if _, err := store.OverridePath(id); err == nil {
			t.Fatalf("OverridePath(%q) accepted an unsafe id", id)
		}
		if _, err := store.SaveOverride(id, Override{MessageMode: strPtr(model.ChatModeAPI)}); err == nil {
			t.Fatalf("SaveOverride(%q) accepted an unsafe id", id)
		}
	}
}

func TestEmptyOverrideDeletesTheFile(t *testing.T) {
	store, _ := openStore(t)
	if _, err := store.SaveOverride("omezy", Override{MessageMode: strPtr(model.ChatModeAPI)}); err != nil {
		t.Fatalf("SaveOverride: %v", err)
	}
	if _, err := store.SaveOverride("omezy", Override{}); err != nil {
		t.Fatalf("SaveOverride empty: %v", err)
	}
	path, _ := store.OverridePath("omezy")
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("empty override must delete the file (err=%v)", err)
	}
}

func TestEffectiveFollowsGlobalUntilSiteOverridesIt(t *testing.T) {
	store, dataDir := openStore(t)
	if _, err := sites.EnsureSites(dataDir); err != nil {
		t.Fatalf("EnsureSites: %v", err)
	}

	global := Default()
	global.MessageMode = model.ChatModeAPI
	global.Proxy = model.ProxySettings{Enabled: true, Entries: "http://global:1"}
	if _, err := store.Save(global); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// A site without an override sees the global values immediately.
	effective, err := store.Effective("omezy")
	if err != nil {
		t.Fatalf("Effective: %v", err)
	}
	if effective.Settings.MessageMode != model.ChatModeAPI || effective.Settings.Proxy.Entries != "http://global:1" {
		t.Fatalf("site inherited the wrong settings: %+v", effective.Settings)
	}

	// Give one site a custom proxy; everyone else must keep the global one.
	if _, err := store.SaveOverride("omezy", Override{Proxy: &model.ProxySettings{Enabled: true, Entries: "http://omezy-only:2"}}); err != nil {
		t.Fatalf("SaveOverride: %v", err)
	}
	if _, err := store.SaveOverride("funchatx", Override{MessageMode: strPtr(model.ChatModeSingle)}); err != nil {
		t.Fatalf("SaveOverride: %v", err)
	}

	omezy, _ := store.Effective("omezy")
	if omezy.Settings.Proxy.Entries != "http://omezy-only:2" {
		t.Fatalf("omezy proxy = %+v", omezy.Settings.Proxy)
	}
	if omezy.Settings.MessageMode != model.ChatModeAPI {
		t.Fatalf("omezy must keep the global mode, got %q", omezy.Settings.MessageMode)
	}

	funchatx, _ := store.Effective("funchatx")
	if funchatx.Settings.MessageMode != model.ChatModeSingle {
		t.Fatalf("funchatx mode = %q", funchatx.Settings.MessageMode)
	}
	if funchatx.Settings.Proxy.Entries != "http://global:1" {
		t.Fatalf("funchatx must keep the global proxy, got %+v", funchatx.Settings.Proxy)
	}

	// A later global save still reaches every site that kept inheriting.
	global.ReplyDelay = DurationRange{MinSeconds: 11, MaxSeconds: 22}
	if _, err := store.Save(global); err != nil {
		t.Fatalf("Save: %v", err)
	}
	for _, id := range []string{"omezy", "funchatx"} {
		got, err := store.Effective(id)
		if err != nil {
			t.Fatalf("Effective(%s): %v", id, err)
		}
		if got.Settings.ReplyDelay != (DurationRange{11, 22}) {
			t.Fatalf("%s reply delay = %+v, want the new global value", id, got.Settings.ReplyDelay)
		}
	}
}

func TestClearOverrideRestoresGlobalBehaviour(t *testing.T) {
	store, _ := openStore(t)
	global := Default()
	global.MessageMode = model.ChatModeAPI
	if _, err := store.Save(global); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := store.SaveOverride("omezy", Override{MessageMode: strPtr(model.ChatModeSingle)}); err != nil {
		t.Fatalf("SaveOverride: %v", err)
	}
	if err := store.ClearOverride("omezy"); err != nil {
		t.Fatalf("ClearOverride: %v", err)
	}
	got, err := store.Effective("omezy")
	if err != nil {
		t.Fatalf("Effective: %v", err)
	}
	if got.Settings.MessageMode != model.ChatModeAPI || got.Overridden("message_mode") {
		t.Fatalf("override survived ClearOverride: %+v", got.Settings)
	}
	// Clearing twice is not an error.
	if err := store.ClearOverride("omezy"); err != nil {
		t.Fatalf("second ClearOverride: %v", err)
	}
}

func TestSummariesListEverySite(t *testing.T) {
	store, dataDir := openStore(t)
	if _, err := sites.EnsureSites(dataDir); err != nil {
		t.Fatalf("EnsureSites: %v", err)
	}
	if _, err := store.SaveOverride("omezy", Override{Proxy: &model.ProxySettings{Enabled: true, Entries: "http://x:1"}}); err != nil {
		t.Fatalf("SaveOverride: %v", err)
	}
	summaries, err := store.Summaries()
	if err != nil {
		t.Fatalf("Summaries: %v", err)
	}
	if len(summaries) != 102 {
		t.Fatalf("expected 102 sites, got %d", len(summaries))
	}
	seen := map[string]SiteSummary{}
	for _, summary := range summaries {
		seen[summary.SiteID] = summary
	}
	omezy := seen["omezy"]
	if len(omezy.Overrides) != 1 || omezy.Overrides[0] != "proxy" || !omezy.HasFile {
		t.Fatalf("omezy summary = %+v", omezy)
	}
	if len(seen["funchatx"].Overrides) != 0 || seen["funchatx"].HasFile {
		t.Fatalf("funchatx must have no override: %+v", seen["funchatx"])
	}
}

func TestValidationRejectsBadInput(t *testing.T) {
	store, _ := openStore(t)
	cases := []Settings{
		func() Settings { s := Default(); s.MessageMode = "bogus"; return s }(),
		func() Settings { s := Default(); s.ReplyDelay = DurationRange{MinSeconds: 10, MaxSeconds: 5}; return s }(),
		func() Settings { s := Default(); s.SkipTimeout = DurationRange{MinSeconds: -1}; return s }(),
		func() Settings { s := Default(); s.FirstMessage.Mode = "nobody"; return s }(),
		func() Settings {
			s := Default()
			s.Proxy = model.ProxySettings{Enabled: true, Entries: ""}
			return s
		}(),
		func() Settings {
			s := Default()
			s.API = APISettings{Enabled: true}
			return s
		}(),
	}
	for i, input := range cases {
		if _, err := store.Save(input); err == nil {
			t.Fatalf("case %d: invalid settings were accepted", i)
		}
	}
}

func TestNormalizeRepairsRangesAndLegacyProxy(t *testing.T) {
	s := Settings{
		MessageMode: " API ",
		Proxy:       model.ProxySettings{Enabled: true, Address: "http://1.2.3.4:8080", Username: "u", Password: "p"},
		ReplyDelay:  DurationRange{MinSeconds: 9, MaxSeconds: 4},
		SkipTimeout: DurationRange{MinSeconds: -3, MaxSeconds: MaxDurationSeconds * 2},
	}
	got := s.Normalize()
	if got.MessageMode != model.ChatModeAPI {
		t.Fatalf("mode = %q", got.MessageMode)
	}
	if got.Proxy.Entries != "1.2.3.4:8080:u:p" {
		t.Fatalf("legacy proxy not converted: %q", got.Proxy.Entries)
	}
	if got.Proxy.Address != "" || got.Proxy.Username != "" || got.Proxy.Password != "" {
		t.Fatalf("credentials duplicated: %+v", got.Proxy)
	}
	if got.ReplyDelay != (DurationRange{9, 9}) {
		t.Fatalf("reply delay = %+v, want max raised to min", got.ReplyDelay)
	}
	if got.SkipTimeout != (DurationRange{0, MaxDurationSeconds}) {
		t.Fatalf("skip timeout = %+v", got.SkipTimeout)
	}
	if got.FirstMessage.Mode == "" {
		t.Fatal("first message default was not applied")
	}
}

func TestRedactedHidesSecrets(t *testing.T) {
	effective := Merge(Default(), Override{})
	effective.Settings.API = APISettings{Enabled: true, APIKey: "sk-secret", BaseURL: "https://x.test", Model: "m"}
	effective.Settings.Proxy = model.ProxySettings{Enabled: true, Entries: "http://user:pass@1.2.3.4:8080\nhost:9:user2:pass2"}

	redacted := effective.Redacted()
	if redacted.Settings.API.APIKey == "sk-secret" {
		t.Fatal("API key was not redacted")
	}
	if strings.Contains(redacted.Settings.Proxy.Entries, "pass") {
		t.Fatalf("proxy credentials leaked: %q", redacted.Settings.Proxy.Entries)
	}
	if !strings.Contains(redacted.Settings.Proxy.Entries, "1.2.3.4:8080") {
		t.Fatalf("proxy host was lost: %q", redacted.Settings.Proxy.Entries)
	}
}

func TestApplyMessageSettingsOverridesModeAndAPIOnly(t *testing.T) {
	effective := Merge(Default(), Override{MessageMode: strPtr(model.ChatModeAPI)})
	effective.Settings.API = APISettings{Enabled: true, BaseURL: "https://site.test/v1", APIKey: "k", Model: "m", Temperature: 0.3}

	base := model.NewMessageSettings("hello\nhi", "alice, bob")
	base.LoadedSequences = []string{"seq-a.txt"}
	got := effective.ApplyMessageSettings(base)

	if got.ChatMode != model.ChatModeAPI {
		t.Fatalf("mode = %q", got.ChatMode)
	}
	if got.AIModel != "m" || got.AIAPIKey != "k" || got.AIBaseURL != "https://site.test/v1" {
		t.Fatalf("api fields not applied: %+v", got)
	}
	if got.Sequence != base.Sequence || got.Usernames != base.Usernames {
		t.Fatalf("sequence/usernames were changed: %+v", got)
	}
	if len(got.LoadedSequences) != 1 {
		t.Fatalf("loaded sequences were lost: %+v", got.LoadedSequences)
	}
}

func TestApplyProxySettingsHonoursExplicitSiteDisable(t *testing.T) {
	global := Default()
	global.Proxy = model.ProxySettings{Enabled: true, Entries: "http://global:1"}
	fallback := model.ProxySettings{Enabled: true, Entries: "http://legacy-form:2"}

	inherited := Merge(global, Override{})
	if got := inherited.ApplyProxySettings(fallback); got.Entries != "http://global:1" {
		t.Fatalf("inherited proxy = %+v", got)
	}

	disabled := Merge(global, Override{Proxy: &model.ProxySettings{Enabled: false}})
	if got := disabled.ApplyProxySettings(fallback); got.Enabled || got.Entries != "" {
		t.Fatalf("site-level disable was ignored: %+v", got)
	}
}

func TestDurationRangeJSONShape(t *testing.T) {
	data, err := json.Marshal(DurationRange{MinSeconds: 1, MaxSeconds: 2})
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `{"min_seconds":1,"max_seconds":2}` {
		t.Fatalf("json = %s", data)
	}
}
