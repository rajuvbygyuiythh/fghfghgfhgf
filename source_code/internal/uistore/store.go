package uistore

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	badger "github.com/dgraph-io/badger/v4"

	"chitchatbot/internal/model"
	"chitchatbot/internal/store"
)

const (
	globalDirName     = "_global"
	keyBrowser        = "settings/browser"
	keyMessage        = "settings/message"
	keyProxy          = "settings/proxy"
	keyGeneral        = "settings/general"
	keyActiveSite     = "settings/active_site"
	logPrefix         = "logs/"
	messageMirrorFile = "message_settings.json"
)

// globalUIRoot is the only shared directory: <dataDir>/_global/ui.
func globalUIRoot(baseDir string) string {
	return filepath.Join(baseDir, globalDirName, "ui")
}

// migrateLegacyUI moves the old <dataDir>/ui tree into <dataDir>/_global/ui
// once. Existing settings are preserved; failures are non-fatal because the new
// location is still created below.
func migrateLegacyUI(baseDir string) {
	legacy := filepath.Join(baseDir, "ui")
	target := globalUIRoot(baseDir)
	if _, err := os.Stat(legacy); err != nil {
		return
	}
	if _, err := os.Stat(target); err == nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return
	}
	_ = os.Rename(legacy, target)
}

type Store struct {
	baseDir        string
	db             *badger.DB
	showModeMu     sync.Mutex
	mirrorMu       sync.Mutex
	lastMirrorSync time.Time
}

func Open(baseDir string) (*Store, error) {
	migrateLegacyUI(baseDir)
	dbDir := filepath.Join(globalUIRoot(baseDir), "badger")
	if err := store.EnsureDir(dbDir); err != nil {
		return nil, err
	}

	opts := badger.DefaultOptions(dbDir)
	opts.Logger = nil
	opts.ValueLogFileSize = 64 << 20

	db, err := badger.Open(opts)
	if err != nil {
		return nil, err
	}

	return &Store{baseDir: baseDir, db: db}, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) LoadBrowserSettings() (model.BrowserSettings, error) {
	settings := model.DefaultBrowserSettings()
	ok, err := s.loadJSON(keyBrowser, &settings)
	if err != nil || !ok {
		return settings, err
	}
	if settings.Profile == "" {
		settings.Profile = "default"
	}
	if settings.Backend != "rod" && settings.Backend != "chromedp" {
		settings.Backend = "rod"
	}
	return settings, nil
}

func (s *Store) SaveBrowserSettings(settings model.BrowserSettings) error {
	if settings.Profile == "" {
		settings.Profile = "default"
	}
	if settings.Backend != "rod" && settings.Backend != "chromedp" {
		settings.Backend = "rod"
	}
	return s.saveJSON(keyBrowser, settings)
}

func (s *Store) LoadActiveSite() string {
	var active string
	ok, err := s.loadJSON(keyActiveSite, &active)
	if err != nil || !ok || strings.TrimSpace(active) == "" {
		return "chitchat"
	}
	return strings.TrimSpace(active)
}

func (s *Store) SaveActiveSite(siteID string) error {
	siteID = strings.TrimSpace(siteID)
	if siteID == "" {
		siteID = "chitchat"
	}
	return s.saveJSON(keyActiveSite, siteID)
}

func (s *Store) mirrorPath() string {
	if s == nil || s.baseDir == "" {
		return ""
	}
	return filepath.Join(globalUIRoot(s.baseDir), messageMirrorFile)
}

func (s *Store) writeMirror(ret model.MessageSettings) {
	path := s.mirrorPath()
	if path == "" {
		return
	}
	s.mirrorMu.Lock()
	defer s.mirrorMu.Unlock()
	_ = store.SaveJSON(path, ret)
	if info, err := os.Stat(path); err == nil {
		s.lastMirrorSync = info.ModTime()
	}
}

func (s *Store) newerMirror() (model.MessageSettings, bool) {
	path := s.mirrorPath()
	if path == "" {
		return model.MessageSettings{}, false
	}
	s.mirrorMu.Lock()
	defer s.mirrorMu.Unlock()
	info, err := os.Stat(path)
	if err != nil {
		return model.MessageSettings{}, false
	}
	if !s.lastMirrorSync.IsZero() && !info.ModTime().After(s.lastMirrorSync) {
		return model.MessageSettings{}, false
	}
	mirrored, err := LoadMessageSettingsMirror(s.baseDir)
	if err != nil {
		return model.MessageSettings{}, false
	}
	s.lastMirrorSync = info.ModTime()
	return mirrored, true
}

func mergeMirroredAPIFields(dst *model.MessageSettings, mirrored model.MessageSettings) {
	if strings.TrimSpace(mirrored.AIAPIKey) != "" {
		dst.AIAPIKey = strings.TrimSpace(mirrored.AIAPIKey)
	}
	if strings.TrimSpace(mirrored.AIBaseURL) != "" {
		dst.AIBaseURL = strings.TrimSpace(mirrored.AIBaseURL)
	}
	if strings.TrimSpace(mirrored.AIModel) != "" {
		dst.AIModel = strings.TrimSpace(mirrored.AIModel)
	}
	if mirrored.APITemperature > 0 {
		dst.APITemperature = mirrored.APITemperature
	}
	if mirrored.APIShareHot > 0 {
		dst.APIShareHot = mirrored.APIShareHot
	}
	if mirrored.APIShareWarm > 0 {
		dst.APIShareWarm = mirrored.APIShareWarm
	}
	if mirrored.APIShareCold > 0 {
		dst.APIShareCold = mirrored.APIShareCold
	}
}

func (s *Store) LoadMessageSettings() (model.MessageSettings, error) {
	if mirrored, ok := s.newerMirror(); ok {
		var prev model.MessageSettings
		if hasPrev, _ := s.loadJSON(keyMessage, &prev); hasPrev {
			mergeMirroredAPIFields(&prev, mirrored)
			if strings.TrimSpace(prev.Usernames) == "" && strings.TrimSpace(mirrored.Usernames) != "" {
				prev.Usernames = mirrored.Usernames
			}
			if strings.TrimSpace(prev.ChatMode) == "" && strings.TrimSpace(mirrored.ChatMode) != "" {
				prev.ChatMode = mirrored.ChatMode
			}
			_ = s.saveJSON(keyMessage, prev)
		} else {
			_ = s.saveJSON(keyMessage, mirrored)
		}
	}

	settings := model.DefaultMessageSettings()
	ok, err := s.loadJSON(keyMessage, &settings)
	if err != nil || !ok {
		if mirrored, mErr := LoadMessageSettingsMirror(s.baseDir); mErr == nil {
			mergeMirroredAPIFields(&settings, mirrored)
			settings = model.NormalizeAPISettings(settings)
		}
		return settings, err
	}

	if model.NormalizeMessageSequence(settings.Sequence) == "" {
		settings.Sequence = model.LegacyMessageSequence(settings.FirstMessage, settings.ReplyMessage)
	}
	if model.NormalizeMessageSequence(settings.Sequence) == "" {
		return model.DefaultMessageSettings(), nil
	}
	ret := model.NewMessageSettings(settings.Sequence, settings.Usernames)
	mode := settings.EffectiveChatMode()
	ret.ChatMode = mode
	ret.LoadedSequences = settings.LoadedSequences
	ret.UseLoadedSequences = (mode == model.ChatModeFolder)
	ret.KeywordRulesContent = settings.KeywordRulesContent
	ret.KeywordFilePath = settings.KeywordFilePath
	ret.AIEnabled = settings.AIEnabled
	ret.AIBaseURL = settings.AIBaseURL
	ret.AIAPIKey = settings.AIAPIKey
	ret.AIModel = settings.AIModel
	ret.APITemperature = settings.APITemperature
	ret.APIShareHot = settings.APIShareHot
	ret.APIShareWarm = settings.APIShareWarm
	ret.APIShareCold = settings.APIShareCold
	if mirrored, mErr := LoadMessageSettingsMirror(s.baseDir); mErr == nil {
		mergeMirroredAPIFields(&ret, mirrored)
	}
	ret = model.NormalizeAPISettings(ret)
	s.writeMirror(ret)
	return ret, nil
}

func (s *Store) SaveMessageSettings(settings model.MessageSettings) error {
	var prev model.MessageSettings
	_, _ = s.loadJSON(keyMessage, &prev)
	if mirrored, mErr := LoadMessageSettingsMirror(s.baseDir); mErr == nil {
		mergeMirroredAPIFields(&prev, mirrored)
	}

	sequence := model.NormalizeMessageSequence(settings.Sequence)
	if sequence == "" {
		sequence = model.LegacyMessageSequence(settings.FirstMessage, settings.ReplyMessage)
	}
	if sequence == "" {
		sequence = model.NormalizeMessageSequence(prev.Sequence)
	}
	if sequence == "" {
		sequence = model.DefaultMessageSettings().Sequence
	}
	ret := model.NewMessageSettings(sequence, settings.Usernames)
	mode := settings.EffectiveChatMode()
	ret.ChatMode = mode
	if settings.LoadedSequences != nil {
		ret.LoadedSequences = settings.LoadedSequences
	} else if mode == model.ChatModeFolder && len(prev.LoadedSequences) > 0 {
		ret.LoadedSequences = prev.LoadedSequences
	}
	ret.UseLoadedSequences = (mode == model.ChatModeFolder)
	ret.KeywordRulesContent = settings.KeywordRulesContent
	ret.KeywordFilePath = settings.KeywordFilePath
	ret.AIEnabled = settings.AIEnabled
	ret.AIBaseURL = strings.TrimSpace(settings.AIBaseURL)
	if ret.AIBaseURL == "" {
		ret.AIBaseURL = strings.TrimSpace(prev.AIBaseURL)
	}
	ret.AIAPIKey = strings.TrimSpace(settings.AIAPIKey)
	if ret.AIAPIKey == "" {
		ret.AIAPIKey = strings.TrimSpace(prev.AIAPIKey)
	}
	ret.AIModel = strings.TrimSpace(settings.AIModel)
	if ret.AIModel == "" {
		ret.AIModel = strings.TrimSpace(prev.AIModel)
	}
	ret.APITemperature = settings.APITemperature
	if ret.APITemperature <= 0 {
		ret.APITemperature = prev.APITemperature
	}
	ret.APIShareHot = settings.APIShareHot
	if ret.APIShareHot <= 0 {
		ret.APIShareHot = prev.APIShareHot
	}
	ret.APIShareWarm = settings.APIShareWarm
	if ret.APIShareWarm <= 0 {
		ret.APIShareWarm = prev.APIShareWarm
	}
	ret.APIShareCold = settings.APIShareCold
	if ret.APIShareCold <= 0 {
		ret.APIShareCold = prev.APIShareCold
	}
	if mirrored, ok := s.newerMirror(); ok {
		mergeMirroredAPIFields(&ret, mirrored)
	}
	ret = model.NormalizeAPISettings(ret)
	if err := s.saveJSON(keyMessage, ret); err != nil {
		return err
	}
	s.writeMirror(ret)
	return nil
}

// LoadSharedMessageSettings reads Message settings from the mirror file first,
// falling back to BadgerDB when unlocked. Safe to call while the main dashboard
// is running.
func LoadSharedMessageSettings(baseDir string) (model.MessageSettings, error) {
	if settings, err := LoadMessageSettingsMirror(baseDir); err == nil {
		return settings, nil
	}
	st, err := Open(baseDir)
	if err != nil {
		return model.NormalizeAPISettings(model.DefaultMessageSettings()), nil
	}
	defer st.Close()
	return st.LoadMessageSettings()
}

// SaveSharedMessageSettings writes Message settings to the shared mirror file
// and also updates BadgerDB when the main dashboard is not holding the lock.
func SaveSharedMessageSettings(baseDir string, settings model.MessageSettings) (model.MessageSettings, error) {
	prev, _ := LoadSharedMessageSettings(baseDir)
	sequence := model.NormalizeMessageSequence(settings.Sequence)
	if sequence == "" {
		sequence = model.NormalizeMessageSequence(prev.Sequence)
	}
	if sequence == "" {
		sequence = model.DefaultMessageSettings().Sequence
	}
	usernames := settings.Usernames
	if strings.TrimSpace(usernames) == "" {
		usernames = prev.Usernames
	}
	ret := model.NewMessageSettings(sequence, usernames)
	mode := settings.EffectiveChatMode()
	if strings.TrimSpace(settings.ChatMode) == "" && strings.TrimSpace(prev.ChatMode) != "" {
		mode = prev.EffectiveChatMode()
	}
	ret.ChatMode = mode
	if len(settings.LoadedSequences) > 0 {
		ret.LoadedSequences = settings.LoadedSequences
	} else {
		ret.LoadedSequences = prev.LoadedSequences
	}
	ret.UseLoadedSequences = (mode == model.ChatModeFolder)
	ret.KeywordRulesContent = settings.KeywordRulesContent
	if ret.KeywordRulesContent == "" {
		ret.KeywordRulesContent = prev.KeywordRulesContent
	}
	ret.KeywordFilePath = settings.KeywordFilePath
	if ret.KeywordFilePath == "" {
		ret.KeywordFilePath = prev.KeywordFilePath
	}
	ret.AIEnabled = settings.AIEnabled
	ret.AIBaseURL = strings.TrimSpace(settings.AIBaseURL)
	if ret.AIBaseURL == "" {
		ret.AIBaseURL = strings.TrimSpace(prev.AIBaseURL)
	}
	ret.AIAPIKey = strings.TrimSpace(settings.AIAPIKey)
	if ret.AIAPIKey == "" {
		ret.AIAPIKey = strings.TrimSpace(prev.AIAPIKey)
	}
	ret.AIModel = strings.TrimSpace(settings.AIModel)
	if ret.AIModel == "" {
		ret.AIModel = strings.TrimSpace(prev.AIModel)
	}
	ret.APITemperature = settings.APITemperature
	if ret.APITemperature <= 0 {
		ret.APITemperature = prev.APITemperature
	}
	ret.APIShareHot = settings.APIShareHot
	if ret.APIShareHot <= 0 {
		ret.APIShareHot = prev.APIShareHot
	}
	ret.APIShareWarm = settings.APIShareWarm
	if ret.APIShareWarm <= 0 {
		ret.APIShareWarm = prev.APIShareWarm
	}
	ret.APIShareCold = settings.APIShareCold
	if ret.APIShareCold <= 0 {
		ret.APIShareCold = prev.APIShareCold
	}
	ret = model.NormalizeAPISettings(ret)

	if err := store.SaveJSON(filepath.Join(globalUIRoot(baseDir), messageMirrorFile), ret); err != nil {
		return ret, err
	}
	if st, err := Open(baseDir); err == nil {
		_ = st.SaveMessageSettings(ret)
		_ = st.Close()
	}
	return ret, nil
}

// LoadMessageSettingsMirror reads the last saved Message settings mirror when
// BadgerDB is locked by the running dashboard process.
func LoadMessageSettingsMirror(baseDir string) (model.MessageSettings, error) {
	var settings model.MessageSettings
	ok, err := store.LoadJSON(filepath.Join(globalUIRoot(baseDir), messageMirrorFile), &settings)
	if err != nil {
		return model.MessageSettings{}, err
	}
	if !ok {
		return model.MessageSettings{}, fmt.Errorf("message settings mirror not found")
	}
	sequence := model.NormalizeMessageSequence(settings.Sequence)
	if sequence == "" {
		sequence = model.DefaultMessageSettings().Sequence
	}
	ret := model.NewMessageSettings(sequence, settings.Usernames)
	mode := settings.EffectiveChatMode()
	ret.ChatMode = mode
	ret.LoadedSequences = settings.LoadedSequences
	ret.UseLoadedSequences = (mode == model.ChatModeFolder)
	ret.KeywordRulesContent = settings.KeywordRulesContent
	ret.KeywordFilePath = settings.KeywordFilePath
	ret.AIEnabled = settings.AIEnabled
	ret.AIBaseURL = settings.AIBaseURL
	ret.AIAPIKey = settings.AIAPIKey
	ret.AIModel = settings.AIModel
	ret.APITemperature = settings.APITemperature
	ret.APIShareHot = settings.APIShareHot
	ret.APIShareWarm = settings.APIShareWarm
	ret.APIShareCold = settings.APIShareCold
	ret = model.NormalizeAPISettings(ret)
	return ret, nil
}

func (s *Store) LoadProxySettings() (model.ProxySettings, error) {
	settings := model.DefaultProxySettings()
	ok, err := s.loadJSON(keyProxy, &settings)
	if err != nil || !ok {
		return settings, err
	}
	if strings.TrimSpace(settings.Entries) == "" && strings.TrimSpace(settings.Address) != "" {
		address := strings.TrimSpace(settings.Address)
		address = strings.TrimPrefix(address, "http://")
		address = strings.TrimPrefix(address, "https://")

		if strings.TrimSpace(settings.Username) != "" || strings.TrimSpace(settings.Password) != "" {
			address = address + ":" + strings.TrimSpace(settings.Username) + ":" + strings.TrimSpace(settings.Password)
		}

		settings.Entries = address
	}
	return settings, nil
}

func (s *Store) SaveProxySettings(settings model.ProxySettings) error {
	settings.Address = ""
	settings.Username = ""
	settings.Password = ""
	return s.saveJSON(keyProxy, settings)
}

func (s *Store) LoadGeneralSettings() (model.GeneralSettings, error) {
	settings := model.DefaultGeneralSettings()
	ok, err := s.loadJSON(keyGeneral, &settings)
	if err != nil || !ok {
		return settings, err
	}
	if settings.PollMinSeconds <= 0 {
		settings.PollMinSeconds = model.DefaultGeneralSettings().PollMinSeconds
	}
	if settings.PollMaxSeconds < settings.PollMinSeconds {
		settings.PollMaxSeconds = settings.PollMinSeconds
	}
	if settings.ActionMinSeconds <= 0 {
		settings.ActionMinSeconds = model.DefaultGeneralSettings().ActionMinSeconds
	}
	if settings.ActionMaxSeconds < settings.ActionMinSeconds {
		settings.ActionMaxSeconds = settings.ActionMinSeconds
	}
	if settings.MatchReplyTimeoutMinSeconds <= 0 {
		settings.MatchReplyTimeoutMinSeconds = model.DefaultGeneralSettings().MatchReplyTimeoutMinSeconds
	}
	if settings.MatchReplyTimeoutMaxSeconds < settings.MatchReplyTimeoutMinSeconds {
		settings.MatchReplyTimeoutMaxSeconds = settings.MatchReplyTimeoutMinSeconds
	}
	if settings.MatchCloseWaitMaxSeconds < settings.MatchCloseWaitMinSeconds {
		settings.MatchCloseWaitMaxSeconds = settings.MatchCloseWaitMinSeconds
	}
	if settings.ReplyDelayMaxSeconds < settings.ReplyDelayMinSeconds {
		settings.ReplyDelayMaxSeconds = settings.ReplyDelayMinSeconds
	}
	return settings, nil
}

func (s *Store) SaveGeneralSettings(settings model.GeneralSettings) error {
	if err := settings.FirstMessage.Validate(); err != nil {
		return err
	}
	if settings.PollMinSeconds <= 0 {
		settings.PollMinSeconds = model.DefaultGeneralSettings().PollMinSeconds
	}
	if settings.PollMaxSeconds < settings.PollMinSeconds {
		settings.PollMaxSeconds = settings.PollMinSeconds
	}
	if settings.ActionMinSeconds <= 0 {
		settings.ActionMinSeconds = model.DefaultGeneralSettings().ActionMinSeconds
	}
	if settings.ActionMaxSeconds < settings.ActionMinSeconds {
		settings.ActionMaxSeconds = settings.ActionMinSeconds
	}
	if settings.MatchReplyTimeoutMinSeconds <= 0 {
		settings.MatchReplyTimeoutMinSeconds = model.DefaultGeneralSettings().MatchReplyTimeoutMinSeconds
	}
	if settings.MatchReplyTimeoutMaxSeconds < settings.MatchReplyTimeoutMinSeconds {
		settings.MatchReplyTimeoutMaxSeconds = settings.MatchReplyTimeoutMinSeconds
	}
	if settings.MatchCloseWaitMaxSeconds < settings.MatchCloseWaitMinSeconds {
		settings.MatchCloseWaitMaxSeconds = settings.MatchCloseWaitMinSeconds
	}
	if settings.ReplyDelayMaxSeconds < settings.ReplyDelayMinSeconds {
		settings.ReplyDelayMaxSeconds = settings.ReplyDelayMinSeconds
	}
	return s.saveJSON(keyGeneral, settings)
}

// logSequence keeps two records written in the same clock tick from sharing a
// Badger key. Windows clocks can be coarse enough for UnixNano to repeat.
var logSequence atomic.Uint64

func (s *Store) AppendLog(level string, message string) (model.LogEntry, error) {
	entry := model.LogEntry{
		Time:    time.Now().UTC().Format(time.RFC3339),
		Level:   level,
		Message: message,
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return model.LogEntry{}, err
	}

	key := []byte(fmt.Sprintf("%s%020d-%08d", logPrefix, time.Now().UTC().UnixNano(), logSequence.Add(1)))
	err = s.db.Update(func(txn *badger.Txn) error {
		return txn.Set(key, data)
	})
	if err != nil {
		return model.LogEntry{}, err
	}

	return entry, nil
}

func (s *Store) ListLogs(limit int) ([]model.LogEntry, error) {
	if limit <= 0 {
		limit = 200
	}

	result := make([]model.LogEntry, 0, limit)
	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = []byte(logPrefix)

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			err := item.Value(func(val []byte) error {
				var entry model.LogEntry
				if err := json.Unmarshal(val, &entry); err != nil {
					return err
				}
				result = append(result, entry)
				return nil
			})
			if err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	if len(result) > limit {
		result = result[len(result)-limit:]
	}

	return result, nil
}

func (s *Store) ClearLogs() error {
	return s.db.Update(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = []byte(logPrefix)

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			if err := txn.Delete(it.Item().KeyCopy(nil)); err != nil {
				return err
			}
		}

		return nil
	})
}

func (s *Store) saveJSON(key string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}

	return s.db.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte(key), data)
	})
}

func (s *Store) loadJSON(key string, dest any) (bool, error) {
	var data []byte
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(key))
		if err != nil {
			return err
		}
		return item.Value(func(val []byte) error {
			data = append([]byte(nil), val...)
			return nil
		})
	})
	if err != nil {
		if err == badger.ErrKeyNotFound {
			return false, nil
		}
		return false, err
	}

	return true, json.Unmarshal(data, dest)
}
