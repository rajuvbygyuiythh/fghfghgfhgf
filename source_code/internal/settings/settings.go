// Package settings owns the application-wide settings file and the per-site
// overrides that may replace individual parts of it.
//
// Layout (Phase 2):
//
//	<sites-root>/_global/settings.json            global defaults, shared by all sites
//	<sites-root>/<site_id>/settings_override.json per-site override, isolated in the site folder
//
// The effective settings for a site are always:
//
//	effective = merge(global, siteOverride)
//
// Every field in an override is optional. A field that is absent (nil) is
// inherited from the global file; a field that is present replaces the global
// value for that site only. The 102 sites therefore share one set of defaults
// without sharing a single runtime file.
package settings

import (
	"fmt"
	"strings"

	"chitchatbot/internal/model"
)

// DurationRange is a min/max window in seconds. It is used for reply delay and
// for the skip timeout so both are validated by the same code.
type DurationRange struct {
	MinSeconds int `json:"min_seconds"`
	MaxSeconds int `json:"max_seconds"`
}

// Normalize makes the range sane: negative values become 0 and max is raised to
// min so callers never have to defend against min > max.
func (r DurationRange) Normalize() DurationRange {
	if r.MinSeconds < 0 {
		r.MinSeconds = 0
	}
	if r.MaxSeconds < r.MinSeconds {
		r.MaxSeconds = r.MinSeconds
	}
	if r.MaxSeconds > MaxDurationSeconds {
		r.MaxSeconds = MaxDurationSeconds
	}
	if r.MinSeconds > MaxDurationSeconds {
		r.MinSeconds = MaxDurationSeconds
	}
	return r
}

// Validate rejects ranges a user could have typed but the app must not persist.
func (r DurationRange) Validate(name string) error {
	if r.MinSeconds < 0 || r.MaxSeconds < 0 {
		return fmt.Errorf("%s must not be negative", name)
	}
	if r.MinSeconds > MaxDurationSeconds || r.MaxSeconds > MaxDurationSeconds {
		return fmt.Errorf("%s must not exceed %d seconds", name, MaxDurationSeconds)
	}
	if r.MaxSeconds < r.MinSeconds {
		return fmt.Errorf("%s min must not exceed max", name)
	}
	return nil
}

// Range returns the values as a usec-free time window the workers can consume.
func (r DurationRange) Range() (min, max int) { return r.MinSeconds, r.MaxSeconds }

// APISettings holds the OpenAI-compatible credentials used by API Chat. The key
// is written only to the local settings file and is never logged.
type APISettings struct {
	Enabled     bool    `json:"enabled"`
	BaseURL     string  `json:"base_url,omitempty"`
	APIKey      string  `json:"api_key,omitempty"`
	Model       string  `json:"model,omitempty"`
	Temperature float64 `json:"temperature,omitempty"`
}

// Normalize trims whitespace and clamps the temperature to a safe range.
func (a APISettings) Normalize() APISettings {
	a.BaseURL = strings.TrimSpace(a.BaseURL)
	a.APIKey = strings.TrimSpace(a.APIKey)
	a.Model = strings.TrimSpace(a.Model)
	if a.Temperature < 0 {
		a.Temperature = 0
	}
	if a.Temperature > 2 {
		a.Temperature = 2
	}
	if a.BaseURL == "" {
		a.BaseURL = DefaultAPIBaseURL
	}
	return a
}

// Configured reports whether API Chat has enough to run.
func (a APISettings) Configured() bool { return strings.TrimSpace(a.APIKey) != "" }

// Redacted returns a copy safe for logs and diagnostics.
func (a APISettings) Redacted() APISettings {
	if a.APIKey != "" {
		a.APIKey = "[REDACTED]"
	}
	return a
}

// Settings is one complete set of application settings: the global file, an
// override, or the merged result. Every value here has a defined default, so a
// missing settings.json still yields a runnable configuration.
type Settings struct {
	// MessageMode is the reply engine for the site: single, folder, eva_chat_flow
	// or api_chat (model.ChatMode*).
	MessageMode string `json:"message_mode"`
	// Proxy is the rotation list used by every browser backend.
	Proxy model.ProxySettings `json:"proxy"`
	// ReplyDelay is how long a worker waits before answering an incoming message.
	ReplyDelay DurationRange `json:"reply_delay"`
	// SkipTimeout is how long a chat may stay silent before the worker walks away.
	SkipTimeout DurationRange `json:"skip_timeout"`
	// FirstMessage is the opening-message policy (bot/user/random, wait window).
	FirstMessage model.FirstMessageSettings `json:"first_message"`
	// API holds the API Chat credentials and model.
	API APISettings `json:"api"`
	// UpdatedAt is set by the store; it is informational only.
	UpdatedAt string `json:"updated_at,omitempty"`
}

// Default returns the settings used when no settings.json exists yet. The values
// mirror the long-standing defaults of the dashboard so upgrading users see no
// behaviour change.
func Default() Settings {
	general := model.DefaultGeneralSettings()
	return Settings{
		MessageMode: model.ChatModeSingle,
		Proxy:       model.DefaultProxySettings(),
		ReplyDelay: DurationRange{
			MinSeconds: general.ReplyDelayMinSeconds,
			MaxSeconds: general.ReplyDelayMaxSeconds,
		},
		SkipTimeout: DurationRange{
			MinSeconds: general.MatchReplyTimeoutMinSeconds,
			MaxSeconds: general.MatchReplyTimeoutMaxSeconds,
		},
		FirstMessage: model.DefaultFirstMessageSettings(),
		API:          APISettings{BaseURL: DefaultAPIBaseURL}.Normalize(),
	}
}

// Normalize fills gaps left by an older or hand-edited settings.json and returns
// a value that Validate accepts.
func (s Settings) Normalize() Settings {
	defaults := Default()

	s.MessageMode = NormalizeMessageMode(s.MessageMode)
	if s.MessageMode == "" {
		s.MessageMode = defaults.MessageMode
	}
	s.Proxy = normalizeProxy(s.Proxy)
	s.ReplyDelay = s.ReplyDelay.Normalize()
	s.SkipTimeout = s.SkipTimeout.Normalize()

	if strings.TrimSpace(s.FirstMessage.Mode) == "" {
		s.FirstMessage = defaults.FirstMessage
	} else {
		s.FirstMessage = s.FirstMessage.Effective()
	}

	s.API = s.API.Normalize()
	if s.API.BaseURL == "" {
		s.API.BaseURL = defaults.API.BaseURL
	}
	return s
}

// Validate is the gate the dashboard save path uses. It runs on the normalized
// value so a caller may submit a partially filled form.
func (s Settings) Validate() error {
	if NormalizeMessageMode(s.MessageMode) == "" {
		return fmt.Errorf("unknown message mode %q", s.MessageMode)
	}
	if err := s.ReplyDelay.Validate("reply delay"); err != nil {
		return err
	}
	if err := s.SkipTimeout.Validate("skip timeout"); err != nil {
		return err
	}
	if err := s.FirstMessage.Validate(); err != nil {
		return err
	}
	if s.API.Enabled && !s.API.Configured() {
		return fmt.Errorf("API Chat is enabled but no API key is set")
	}
	// Validate the proxy list only when it is switched on, exactly like the
	// dashboard form, so a disabled legacy entry cannot block a save.
	if s.Proxy.Enabled {
		if _, err := parseProxyEntries(s.Proxy.Entries); err != nil {
			return err
		}
	}
	return nil
}

// Override is the per-site delta. Each field is a pointer: nil means "inherit
// the global value", non-nil means "use this for this site only".
type Override struct {
	MessageMode  *string                     `json:"message_mode,omitempty"`
	Proxy        *model.ProxySettings        `json:"proxy,omitempty"`
	ReplyDelay   *DurationRange              `json:"reply_delay,omitempty"`
	SkipTimeout  *DurationRange              `json:"skip_timeout,omitempty"`
	FirstMessage *model.FirstMessageSettings `json:"first_message,omitempty"`
	API          *APISettings                `json:"api,omitempty"`
	UpdatedAt    string                      `json:"updated_at,omitempty"`
}

// IsZero reports whether the override changes nothing at all. An all-nil
// override is deleted instead of being stored as an empty file.
func (o Override) IsZero() bool {
	return o.MessageMode == nil && o.Proxy == nil && o.ReplyDelay == nil &&
		o.SkipTimeout == nil && o.FirstMessage == nil && o.API == nil
}

// Keys lists the overridden sections in a stable order (used by the dashboard
// and by the diagnostics report).
func (o Override) Keys() []string {
	keys := make([]string, 0, 6)
	if o.MessageMode != nil {
		keys = append(keys, "message_mode")
	}
	if o.Proxy != nil {
		keys = append(keys, "proxy")
	}
	if o.ReplyDelay != nil {
		keys = append(keys, "reply_delay")
	}
	if o.SkipTimeout != nil {
		keys = append(keys, "skip_timeout")
	}
	if o.FirstMessage != nil {
		keys = append(keys, "first_message")
	}
	if o.API != nil {
		keys = append(keys, "api")
	}
	return keys
}

// Normalize repairs an override read from disk without inventing fields that
// were not set (a nil pointer stays nil).
func (o Override) Normalize() Override {
	if o.MessageMode != nil {
		mode := NormalizeMessageMode(*o.MessageMode)
		if mode == "" {
			o.MessageMode = nil
		} else {
			o.MessageMode = &mode
		}
	}
	if o.Proxy != nil {
		proxy := normalizeProxy(*o.Proxy)
		o.Proxy = &proxy
	}
	if o.ReplyDelay != nil {
		r := o.ReplyDelay.Normalize()
		o.ReplyDelay = &r
	}
	if o.SkipTimeout != nil {
		r := o.SkipTimeout.Normalize()
		o.SkipTimeout = &r
	}
	if o.FirstMessage != nil {
		f := o.FirstMessage.Effective()
		o.FirstMessage = &f
	}
	if o.API != nil {
		a := o.API.Normalize()
		o.API = &a
	}
	return o
}

// Validate checks the parts of the override that are present.
func (o Override) Validate() error {
	if o.MessageMode != nil && NormalizeMessageMode(*o.MessageMode) == "" {
		return fmt.Errorf("unknown message mode %q", *o.MessageMode)
	}
	if o.ReplyDelay != nil {
		if err := o.ReplyDelay.Validate("reply delay"); err != nil {
			return err
		}
	}
	if o.SkipTimeout != nil {
		if err := o.SkipTimeout.Validate("skip timeout"); err != nil {
			return err
		}
	}
	if o.FirstMessage != nil {
		if err := o.FirstMessage.Validate(); err != nil {
			return err
		}
	}
	if o.Proxy != nil && o.Proxy.Enabled {
		if _, err := parseProxyEntries(o.Proxy.Entries); err != nil {
			return err
		}
	}
	if o.API != nil && o.API.Enabled && !o.API.Configured() {
		return fmt.Errorf("API Chat is enabled but no API key is set")
	}
	return nil
}

// Source names describe where one effective value came from.
const (
	SourceGlobal = "global"
	SourceSite   = "site"
)

// Effective is the merged result plus the provenance of every section. The
// dashboard shows "global" or "site (override)" next to each field, and the
// diagnostics report uses the same map.
type Effective struct {
	Settings Settings          `json:"settings"`
	SiteID   string            `json:"site_id"`
	Sources  map[string]string `json:"sources"`
}

// Merge applies the override on top of the global settings. It never mutates
// either input, and it normalises the result so callers can trust the output.
//
// Rule: an override field replaces the global value for that site only; every
// field the override does not mention keeps the global value.
func Merge(global Settings, override Override) Effective {
	global = global.Normalize()
	override = override.Normalize()

	merged := global
	sources := map[string]string{
		"message_mode":  SourceGlobal,
		"proxy":         SourceGlobal,
		"reply_delay":   SourceGlobal,
		"skip_timeout":  SourceGlobal,
		"first_message": SourceGlobal,
		"api":           SourceGlobal,
	}
	if override.MessageMode != nil {
		merged.MessageMode = *override.MessageMode
		sources["message_mode"] = SourceSite
	}
	if override.Proxy != nil {
		merged.Proxy = *override.Proxy
		sources["proxy"] = SourceSite
	}
	if override.ReplyDelay != nil {
		merged.ReplyDelay = *override.ReplyDelay
		sources["reply_delay"] = SourceSite
	}
	if override.SkipTimeout != nil {
		merged.SkipTimeout = *override.SkipTimeout
		sources["skip_timeout"] = SourceSite
	}
	if override.FirstMessage != nil {
		merged.FirstMessage = *override.FirstMessage
		sources["first_message"] = SourceSite
	}
	if override.API != nil {
		// An API override replaces the whole section: a site that points at its
		// own endpoint must not inherit the global key or model.
		merged.API = *override.API
		sources["api"] = SourceSite
	}
	merged.UpdatedAt = global.UpdatedAt
	return Effective{Settings: merged.Normalize(), Sources: sources}
}

// NormalizeMessageMode maps aliases onto the canonical model.ChatMode* values.
// An unknown value returns "" so callers keep the previous setting instead of
// silently switching engines.
func NormalizeMessageMode(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case model.ChatModeSingle, "1", "single_message":
		return model.ChatModeSingle
	case model.ChatModeFolder, "2", "sequences", "folder_mode":
		return model.ChatModeFolder
	case model.ChatModeEVAChat, model.ChatModeKeyword, "3", "eva", "eva_chat":
		return model.ChatModeEVAChat
	case model.ChatModeAPI, "4", "api":
		return model.ChatModeAPI
	default:
		return ""
	}
}

// normalizeProxy keeps the legacy address/username/password form readable by
// converting it into the entries list the workers consume.
func normalizeProxy(p model.ProxySettings) model.ProxySettings {
	p.Entries = strings.TrimSpace(p.Entries)
	if p.Entries == "" && strings.TrimSpace(p.Address) != "" {
		address := strings.TrimSpace(p.Address)
		address = strings.TrimPrefix(address, "http://")
		address = strings.TrimPrefix(address, "https://")
		if user := strings.TrimSpace(p.Username); user != "" || strings.TrimSpace(p.Password) != "" {
			address = address + ":" + user + ":" + strings.TrimSpace(p.Password)
		}
		p.Entries = address
	}
	// The split fields are legacy input only; never persist credentials twice.
	p.Address = ""
	p.Username = ""
	p.Password = ""
	return p
}
