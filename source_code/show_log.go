package main

import (
	"strings"
	"sync"
	"time"

	"chitchatbot/internal/memory"
	"chitchatbot/internal/sites"
)

// Show Mode live log: a bounded, human-readable feed of what the in-browser
// runner actually did. It is separate from logs.txt (technical diagnostics)
// and from the transcript (display-only rows). Entries are memory-only and
// never include credentials.
const showLogCapacity = 500

type ShowLogEntry struct {
	Seq       uint64 `json:"seq"`
	Time      string `json:"time"`
	Kind      string `json:"kind"` // in | out | skip | system | error
	Text      string `json:"text,omitempty"`
	Rule      string `json:"rule,omitempty"`
	Detail    string `json:"detail,omitempty"`
	Level     string `json:"level,omitempty"` // info | warn | error
	SettingOK bool   `json:"setting_ok"`
}

type ShowLogSnapshot struct {
	Version uint64         `json:"version"`
	Entries []ShowLogEntry `json:"entries"`
	Dropped int            `json:"dropped"`
}

type showLogFeed struct {
	mu      sync.Mutex
	seq     uint64
	dropped int
	entries []ShowLogEntry
}

func (f *showLogFeed) add(entry ShowLogEntry) uint64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.seq++
	entry.Seq = f.seq
	if entry.Time == "" {
		entry.Time = time.Now().Format("15:04:05")
	}
	if entry.Level == "" {
		entry.Level = "info"
	}
	f.entries = append(f.entries, entry)
	if len(f.entries) > showLogCapacity {
		drop := len(f.entries) - showLogCapacity
		f.entries = f.entries[drop:]
		f.dropped += drop
	}
	return f.seq
}

func (f *showLogFeed) snapshot(after uint64) ShowLogSnapshot {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]ShowLogEntry, 0, len(f.entries))
	for _, entry := range f.entries {
		if entry.Seq > after {
			out = append(out, entry)
		}
	}
	return ShowLogSnapshot{Version: f.seq, Entries: out, Dropped: f.dropped}
}

func (f *showLogFeed) clear() uint64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.entries = nil
	f.dropped = 0
	return f.seq
}

func (a *App) showLogFeed() *showLogFeed {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.showLog == nil {
		a.showLog = &showLogFeed{}
	}
	return a.showLog
}

func (a *App) addShowLog(entry ShowLogEntry) {
	feed := a.showLogFeed()
	seq := feed.add(entry)
	a.emit("ui:showlog", map[string]uint64{"version": seq})
}

func (a *App) showModeEvent(kind, text, rule, detail string, settingOK bool) {
	level := "info"
	if kind == "error" {
		level = "error"
	} else if kind == "skip" {
		level = "warn"
	}
	if a.dataDir != "" {
		siteID := a.activeSiteID()
		memory.DefaultStore(a.dataDir).RecordEvent("show_mode", siteID, kind, text, rule, detail, settingOK)
		_ = sites.AppendSiteDebugLog(a.dataDir, siteID, "show_"+kind, level, text, strings.TrimSpace(rule+" "+detail))
	}
	a.addShowLog(ShowLogEntry{Kind: kind, Text: text, Rule: rule, Detail: detail, Level: level, SettingOK: settingOK})
}

// showLiveError only records a feed error while a Show Mode run is active, so
// background-bot/login failures do not pollute the Show Mode page.
func (a *App) showLiveError(text string, err error) {
	a.mu.Lock()
	live := a.showModeRunning
	a.mu.Unlock()
	if !live {
		return
	}
	detail := ""
	if err != nil {
		detail = err.Error()
	}
	a.showModeEvent("error", text, "", detail, false)
}

func (a *App) clearShowLog() {
	feed := a.showLogFeed()
	version := feed.clear()
	a.emit("ui:showlog", map[string]uint64{"version": version})
}

func (a *App) setShowModeRunning(running bool) {
	a.mu.Lock()
	changed := a.showModeRunning != running
	a.showModeRunning = running
	a.mu.Unlock()
	if changed {
		a.emit("ui:show_running", running)
	}
}

// GetShowModeLog returns entries after the given sequence number. The
// frontend polls this once per second and also refreshes on ui:showlog.
func (a *App) GetShowModeLog(after uint64) ShowLogSnapshot {
	return a.showLogFeed().snapshot(after)
}

// ClearShowModeLog clears the in-memory feed; logs.txt keeps its history.
func (a *App) ClearShowModeLog() error {
	a.clearShowLog()
	return nil
}
