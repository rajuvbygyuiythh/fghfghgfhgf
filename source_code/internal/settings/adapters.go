package settings

import (
	"strings"

	"chitchatbot/internal/model"
)

// This file is the bridge between the settings store and the long-standing
// model.* structs the workers already consume. Keeping the conversions here
// means the rest of the application never has to know both shapes.

// ApplyMessageSettings overlays the effective settings onto a message-settings
// value loaded from uistore. Everything the global settings owns (mode, API
// credentials, model) is replaced; everything else (sequence, usernames, loaded
// sequences, keyword files) is preserved.
func (e Effective) ApplyMessageSettings(base model.MessageSettings) model.MessageSettings {
	base.ChatMode = e.MessageMode()
	base.UseLoadedSequences = base.ChatMode == model.ChatModeFolder

	api := e.Settings.API
	if api.BaseURL != "" {
		base.AIBaseURL = api.BaseURL
	}
	if api.APIKey != "" {
		base.AIAPIKey = api.APIKey
	}
	if api.Model != "" {
		base.AIModel = api.Model
	}
	if api.Temperature > 0 {
		base.APITemperature = api.Temperature
	}
	if base.ChatMode == model.ChatModeAPI {
		base.AIEnabled = true
	}
	return model.NormalizeAPISettings(base)
}

// ApplyProxySettings returns the proxy the site must use: the effective proxy
// when it has content, otherwise the caller's stored fallback.
func (e Effective) ApplyProxySettings(fallback model.ProxySettings) model.ProxySettings {
	effective := e.Settings.Proxy
	if !effective.Enabled || strings.TrimSpace(effective.Entries) == "" {
		if e.Overridden("proxy") {
			// A site that deliberately disables the proxy must not fall back to
			// whatever the dashboard form still holds.
			return model.ProxySettings{Enabled: false}
		}
		return fallback
	}
	return effective
}

// ApplyGeneralSettings overlays reply delay and skip timeout onto the general
// (worker timing) settings. Poll/action windows stay untouched: they are runtime
// tuning, not global policy.
func (e Effective) ApplyGeneralSettings(base model.GeneralSettings) model.GeneralSettings {
	base.ReplyDelayMinSeconds = e.Settings.ReplyDelay.MinSeconds
	base.ReplyDelayMaxSeconds = e.Settings.ReplyDelay.MaxSeconds
	// Skip timeout is the reply-wait window: how long a worker keeps a chat open
	// without an answer before it closes it.
	base.MatchReplyTimeoutMinSeconds = e.Settings.SkipTimeout.MinSeconds
	base.MatchReplyTimeoutMaxSeconds = e.Settings.SkipTimeout.MaxSeconds
	if base.FirstMessage.Mode == "" || e.Overridden("first_message") {
		base.FirstMessage = e.Settings.FirstMessage
	}
	return base
}

// ApplyShowModeSettings mirrors ApplyGeneralSettings for Show Mode, which keeps
// its own timing struct.
func (e Effective) ApplyShowModeSettings(base model.ShowModeSettings) model.ShowModeSettings {
	base.ReplyDelayMinSeconds = e.Settings.ReplyDelay.MinSeconds
	base.ReplyDelayMaxSeconds = e.Settings.ReplyDelay.MaxSeconds
	base.NoReplyMinSeconds = e.Settings.SkipTimeout.MinSeconds
	base.NoReplyMaxSeconds = e.Settings.SkipTimeout.MaxSeconds
	base.FirstMessage = e.Settings.FirstMessage
	return base
}

// MessageSettings returns a full message-settings value for a site, built on top
// of the app defaults and then the stored dashboard value.
func (e Effective) MessageSettings(base model.MessageSettings) model.MessageSettings {
	return e.ApplyMessageSettings(base)
}

// RedactedSettings returns a copy of a settings value with credentials masked.
// Use it for logs, diagnostics and every dashboard payload.
func RedactedSettings(s Settings) Settings {
	if s.API.APIKey != "" {
		s.API = s.API.Redacted()
	}
	s.Proxy.Entries = maskProxyEntries(s.Proxy.Entries)
	return s
}

// Redacted returns a copy of the effective settings with the API key and proxy
// credentials masked. Use this for logs, diagnostics and any dashboard payload
// that is written to disk.
func (e Effective) Redacted() Effective {
	out := e
	out.Settings = RedactedSettings(e.Settings)
	return out
}

// Redacted returns a copy of the override with credentials masked.
func (o Override) Redacted() Override {
	out := o
	if o.API != nil {
		redactedAPI := o.API.Redacted()
		out.API = &redactedAPI
	}
	if o.Proxy != nil {
		redactedProxy := *o.Proxy
		redactedProxy.Entries = maskProxyEntries(redactedProxy.Entries)
		out.Proxy = &redactedProxy
	}
	return out
}

// IsEmpty is an alias for IsZero used by dashboard views.
func (o Override) IsEmpty() bool {
	return o.IsZero()
}

// maskProxyEntries hides credentials in a proxy list while keeping host:port
// visible, which is what makes a support screenshot useful.
func maskProxyEntries(entries string) string {
	lines := strings.Split(entries, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		lines[i] = maskProxyLine(trimmed)
	}
	return strings.Join(lines, "\n")
}

func maskProxyLine(line string) string {
	scheme := ""
	rest := line
	if i := strings.Index(line, "://"); i > 0 {
		scheme = line[:i+3]
		rest = line[i+3:]
	}
	// user:pass@host:port
	if at := strings.LastIndex(rest, "@"); at >= 0 {
		return scheme + "***@" + rest[at+1:]
	}
	// host:port:user:pass
	parts := strings.Split(rest, ":")
	if len(parts) >= 4 {
		return scheme + strings.Join(parts[:2], ":") + ":***"
	}
	return line
}
