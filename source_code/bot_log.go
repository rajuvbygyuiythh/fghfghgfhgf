package main

import (
	"strings"
	"sync"
	"time"

	"chitchatbot/internal/memory"
	"chitchatbot/internal/sites"
)

// Background Bot live log: a bounded, human-readable feed of what the API
// workers actually did — IN/OUT messages, skips, lifecycle and errors.
//
// It mirrors the Show Mode feed (show_log.go) but is completely separate: it
// is fed by the worker Logf/Activityf callbacks and bot lifecycle events, never
// by the visible-browser runner. Entries are memory-only and never include
// credentials.
const botLogCapacity = 500

// botActivityMark separates the direction block from the message text in the
// worker activity lines ("SEND | bot x -> traffic y | message: hello").
const botActivityMark = " | message: "

type BotLogEntry struct {
	Seq       uint64 `json:"seq"`
	Time      string `json:"time"`
	Kind      string `json:"kind"` // in | out | skip | system | error
	Text      string `json:"text,omitempty"`
	Rule      string `json:"rule,omitempty"` // worker profile
	Detail    string `json:"detail,omitempty"`
	Level     string `json:"level,omitempty"` // info | warn | error
	SettingOK bool   `json:"setting_ok"`
}

type BotLogSnapshot struct {
	Version uint64        `json:"version"`
	Entries []BotLogEntry `json:"entries"`
	Dropped int           `json:"dropped"`
}

type botLogFeed struct {
	mu      sync.Mutex
	seq     uint64
	dropped int
	entries []BotLogEntry
}

func (f *botLogFeed) add(entry BotLogEntry) uint64 {
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
	if len(f.entries) > botLogCapacity {
		drop := len(f.entries) - botLogCapacity
		f.entries = f.entries[drop:]
		f.dropped += drop
	}
	return f.seq
}

func (f *botLogFeed) snapshot(after uint64) BotLogSnapshot {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]BotLogEntry, 0, len(f.entries))
	for _, entry := range f.entries {
		if entry.Seq > after {
			out = append(out, entry)
		}
	}
	return BotLogSnapshot{Version: f.seq, Entries: out, Dropped: f.dropped}
}

func (f *botLogFeed) clear() uint64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.entries = nil
	f.dropped = 0
	return f.seq
}

func (a *App) botLogFeed() *botLogFeed {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.botLog == nil {
		a.botLog = &botLogFeed{}
	}
	return a.botLog
}

func (a *App) addBotLog(entry BotLogEntry) {
	feed := a.botLogFeed()
	seq := feed.add(entry)
	a.emit("ui:botlog", map[string]uint64{"version": seq})
}

// botLogEvent stores one human-readable entry and emits the UI event.
func (a *App) botLogEvent(kind, text, rule, detail string, settingOK bool) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	level := "info"
	switch kind {
	case "error":
		level = "error"
	case "skip":
		level = "warn"
	}
	if a.dataDir != "" {
		siteID := a.activeSiteID()
		memory.DefaultStore(a.dataDir).RecordEvent("bot_worker", siteID, kind, text, rule, detail, settingOK)
		_ = sites.AppendSiteDebugLog(a.dataDir, siteID, "bot_"+kind, level, text, strings.TrimSpace(rule+" "+detail))
	}
	a.addBotLog(BotLogEntry{Kind: kind, Text: text, Rule: rule, Detail: detail, Level: level, SettingOK: settingOK})
}

func (a *App) clearBotLog() {
	feed := a.botLogFeed()
	version := feed.clear()
	a.emit("ui:botlog", map[string]uint64{"version": version})
}

// botWorkerActivity converts one worker activity line into a feed entry:
//
//	SEND | bot default (1) -> traffic bob (2) | message: hello   -> out
//	RECEIVE | traffic bob (2) -> bot default (1) | message: hi    -> in
func (a *App) botWorkerActivity(profile, line string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}

	head := line
	text := ""
	if idx := strings.Index(line, botActivityMark); idx >= 0 {
		head = strings.TrimSpace(line[:idx])
		text = strings.TrimSpace(line[idx+len(botActivityMark):])
	}

	from, to := "", ""
	if parts := strings.SplitN(head, "->", 2); len(parts) == 2 {
		from = botActivityPeer(parts[0])
		to = botActivityPeer(parts[1])
	}

	kind := "system"
	detail := ""
	switch {
	case strings.HasPrefix(head, "SEND"):
		kind = "out"
		if to != "" {
			detail = "→ " + to
		}
	case strings.HasPrefix(head, "RECEIVE"):
		kind = "in"
		if from != "" {
			detail = "← " + from
		}
	default:
		text = head
	}
	if text == "" {
		text = head
	}
	a.botLogEvent(kind, text, profile, detail, true)
}

// botActivityPeer strips the leading direction keyword and separator from one
// side of an activity line.
func botActivityPeer(value string) string {
	value = strings.TrimSpace(value)
	for _, prefix := range []string{"SEND", "RECEIVE", "|"} {
		value = strings.TrimSpace(strings.TrimPrefix(value, prefix))
	}
	return strings.TrimSpace(value)
}

// botWorkerLog converts one technical worker log line into a feed entry.
func (a *App) botWorkerLog(profile, message string) {
	message = strings.TrimSpace(message)
	if message == "" {
		return
	}

	lower := strings.ToLower(message)
	kind := "system"
	switch {
	case strings.Contains(lower, "worker stopped with error"),
		strings.Contains(lower, "failed to send message"),
		strings.Contains(lower, "server error"),
		strings.HasPrefix(lower, "get relationships failed"),
		strings.HasPrefix(lower, "get dm conversations failed"),
		strings.HasPrefix(lower, "get messages failed"),
		strings.HasPrefix(lower, "post typing failed"):
		kind = "error"
	case strings.Contains(lower, "skip"):
		kind = "skip"
	}
	a.botLogEvent(kind, message, profile, "", false)
}

// GetBotLiveLog returns entries after the given sequence number. The frontend
// polls this once per second and also refreshes on ui:botlog.
func (a *App) GetBotLiveLog(after uint64) BotLogSnapshot {
	return a.botLogFeed().snapshot(after)
}

// ClearBotLiveLog clears the in-memory feed; logs.txt keeps its history.
func (a *App) ClearBotLiveLog() error {
	a.clearBotLog()
	return nil
}
