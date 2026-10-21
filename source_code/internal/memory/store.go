package memory

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"chitchatbot/internal/store"
)

// ChatRecord represents one real-time event or SMS message persisted to
// memory/chat_history.jsonl with immediate disk sync so power loss never
// loses conversation history.
type ChatRecord struct {
	Timestamp string `json:"timestamp"`
	Source    string `json:"source"`           // "show_mode" | "bot_worker" | "system"
	SiteID    string `json:"site_id"`          // e.g. "omezy", "funchatx", "chitchat"
	Kind      string `json:"kind"`             // "in" | "out" | "skip" | "system" | "error"
	Text      string `json:"text,omitempty"`   // incoming/outgoing SMS text or event summary
	Rule      string `json:"rule,omitempty"`   // reply rule / worker profile
	Detail    string `json:"detail,omitempty"` // timing / partner / metadata
	SettingOK bool   `json:"setting_ok,omitempty"`
}

// SiteMemoryStats tracks per-site real-time counters in operation_memory.json.
type SiteMemoryStats struct {
	SiteID         string `json:"site_id"`
	IncomingCount  uint64 `json:"incoming_count"`
	OutgoingCount  uint64 `json:"outgoing_count"`
	SkipCount      uint64 `json:"skip_count"`
	ErrorCount     uint64 `json:"error_count"`
	LastEventAt    string `json:"last_event_at,omitempty"`
	LastSMSPreview string `json:"last_sms_preview,omitempty"`
}

// OperationState is persisted atomically in memory/operation_memory.json.
type OperationState struct {
	UpdatedAt     string                     `json:"updated_at"`
	ActiveSiteID  string                     `json:"active_site_id"`
	TotalIncoming uint64                     `json:"total_incoming"`
	TotalOutgoing uint64                     `json:"total_outgoing"`
	TotalSkips    uint64                     `json:"total_skips"`
	TotalErrors   uint64                     `json:"total_errors"`
	LastEvent     *ChatRecord                `json:"last_event,omitempty"`
	SiteStats     map[string]SiteMemoryStats `json:"site_stats"`
}

// Store manages power-safe real-time persistence in <repo>/memory/ and <dataDir>/memory/.
type Store struct {
	mu          sync.Mutex
	primaryDir  string
	externalDir string
	state       OperationState
}

var (
	registryMu sync.Mutex
	stores     = map[string]*Store{}
)

// DefaultStore returns a singleton power-safe Memory Store for the given dataDir.
func DefaultStore(dataDir string) *Store {
	clean := filepath.Clean(dataDir)
	registryMu.Lock()
	defer registryMu.Unlock()
	if s, ok := stores[clean]; ok {
		return s
	}
	s := NewStore(clean)
	stores[clean] = s
	return s
}

// NewStore creates and initializes a Store backed by <dataDir>/_global/memory.
// Memory is application-level state; site-specific state stays in the site
// folder (bot_state.json, sessions, logs).
func NewStore(dataDir string) *Store {
	primary := filepath.Join(dataDir, "_global", "memory")
	// One-time migration from the pre-isolation data/memory directory.
	legacy := filepath.Join(dataDir, "memory")
	if _, err := os.Stat(primary); os.IsNotExist(err) {
		if _, legacyErr := os.Stat(legacy); legacyErr == nil {
			if renameErr := os.Rename(legacy, primary); renameErr != nil {
				_ = store.EnsureDir(primary)
			}
		}
	}
	_ = store.EnsureDir(primary)
	s := &Store{
		primaryDir:  primary,
		externalDir: "",
		state: OperationState{
			UpdatedAt:    time.Now().UTC().Format(time.RFC3339),
			ActiveSiteID: "chitchat",
			SiteStats:    map[string]SiteMemoryStats{},
		},
	}
	s.loadExistingState()
	return s
}

func (s *Store) loadExistingState() {
	for _, dir := range []string{s.externalDir, s.primaryDir} {
		if dir == "" {
			continue
		}
		var loaded OperationState
		if ok, err := store.LoadJSON(filepath.Join(dir, "operation_memory.json"), &loaded); err == nil && ok {
			if loaded.SiteStats == nil {
				loaded.SiteStats = map[string]SiteMemoryStats{}
			}
			if strings.TrimSpace(loaded.ActiveSiteID) == "" {
				loaded.ActiveSiteID = "chitchat"
			}
			s.state = loaded
			return
		}
	}
	s.flushStateLocked()
}

// SetActiveSite updates the active site ID and flushes operation_memory.json immediately.
func (s *Store) SetActiveSite(siteID string) {
	siteID = strings.TrimSpace(strings.ToLower(siteID))
	if siteID == "" {
		siteID = "chitchat"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.ActiveSiteID = siteID
	s.state.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	s.flushStateLocked()
}

// RecordEvent appends a chat or lifecycle event to chat_history.jsonl with
// immediate fsync and updates operation_memory.json atomically.
func (s *Store) RecordEvent(source, siteID, kind, text, rule, detail string, settingOK bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC().Format(time.RFC3339)
	if strings.TrimSpace(siteID) == "" {
		siteID = s.state.ActiveSiteID
	}
	if siteID == "" {
		siteID = "chitchat"
	}
	rec := ChatRecord{
		Timestamp: now,
		Source:    source,
		SiteID:    siteID,
		Kind:      kind,
		Text:      strings.TrimSpace(text),
		Rule:      strings.TrimSpace(rule),
		Detail:    strings.TrimSpace(detail),
		SettingOK: settingOK,
	}

	line, err := json.Marshal(rec)
	if err == nil {
		line = append(line, '\n')
		appendSyncFile(filepath.Join(s.primaryDir, "chat_history.jsonl"), line)
		if s.externalDir != "" && s.externalDir != s.primaryDir {
			appendSyncFile(filepath.Join(s.externalDir, "chat_history.jsonl"), line)
		}
	}

	st := s.state.SiteStats[siteID]
	st.SiteID = siteID
	st.LastEventAt = now
	switch kind {
	case "in":
		s.state.TotalIncoming++
		st.IncomingCount++
		st.LastSMSPreview = "IN: " + truncate(rec.Text, 120)
	case "out":
		s.state.TotalOutgoing++
		st.OutgoingCount++
		st.LastSMSPreview = "OUT: " + truncate(rec.Text, 120)
	case "skip":
		s.state.TotalSkips++
		st.SkipCount++
	case "error":
		s.state.TotalErrors++
		st.ErrorCount++
	}
	s.state.SiteStats[siteID] = st
	s.state.UpdatedAt = now
	s.state.LastEvent = &rec
	s.flushStateLocked()
}

// Snapshot returns a copy of the current OperationState.
func (s *Store) Snapshot() OperationState {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := s.state
	cp.SiteStats = make(map[string]SiteMemoryStats, len(s.state.SiteStats))
	for k, v := range s.state.SiteStats {
		cp.SiteStats[k] = v
	}
	return cp
}

func (s *Store) flushStateLocked() {
	_ = store.SaveJSON(filepath.Join(s.primaryDir, "operation_memory.json"), s.state)
	if s.externalDir != "" && s.externalDir != s.primaryDir {
		_ = store.SaveJSON(filepath.Join(s.externalDir, "operation_memory.json"), s.state)
	}
}

func appendSyncFile(path string, data []byte) {
	if err := store.EnsureDir(filepath.Dir(path)); err != nil {
		return
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	_, _ = f.Write(data)
	_ = f.Sync()
	_ = f.Close()
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
