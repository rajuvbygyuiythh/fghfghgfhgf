package uistore

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"chitchatbot/internal/model"
)

func TestAppendLogKeepsEntriesFromTheSameClockTick(t *testing.T) {
	baseDir := t.TempDir()
	s, err := Open(baseDir)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer s.Close()

	const count = 500
	for i := 0; i < count; i++ {
		if _, err := s.AppendLog("info", fmt.Sprintf("row %d", i)); err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
	}
	logs, err := s.ListLogs(count + 10)
	if err != nil {
		t.Fatalf("list logs: %v", err)
	}
	if len(logs) != count {
		t.Fatalf("kept %d of %d logs; same-tick keys collided", len(logs), count)
	}
	if last := fmt.Sprintf("row %d", count-1); logs[len(logs)-1].Message != last {
		t.Fatalf("last log = %q, want %q", logs[len(logs)-1].Message, last)
	}
}

func TestSettingsAndLogsRoundTrip(t *testing.T) {
	baseDir := t.TempDir()

	uiStore, err := Open(baseDir)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer uiStore.Close()

	browser := model.BrowserSettings{
		Profile: "account2",
		Backend: "chromedp",
	}
	if err := uiStore.SaveBrowserSettings(browser); err != nil {
		t.Fatalf("save browser settings: %v", err)
	}

	message := model.MessageSettings{
		Sequence:  "hi\nwait",
		Usernames: "snap_one\ntelegram_two",
	}
	if err := uiStore.SaveMessageSettings(message); err != nil {
		t.Fatalf("save message settings: %v", err)
	}

	if _, err := uiStore.AppendLog("info", "hello"); err != nil {
		t.Fatalf("append log: %v", err)
	}

	gotBrowser, err := uiStore.LoadBrowserSettings()
	if err != nil {
		t.Fatalf("load browser settings: %v", err)
	}
	if gotBrowser.Profile != browser.Profile || gotBrowser.Backend != browser.Backend {
		t.Fatalf("unexpected browser settings: %#v", gotBrowser)
	}

	gotMessage, err := uiStore.LoadMessageSettings()
	if err != nil {
		t.Fatalf("load message settings: %v", err)
	}
	if gotMessage.Sequence != model.NormalizeMessageSequence(message.Sequence) || gotMessage.Usernames != model.NormalizeUsernameList(message.Usernames) {
		t.Fatalf("unexpected message settings: %#v", gotMessage)
	}

	logs, err := uiStore.ListLogs(10)
	if err != nil {
		t.Fatalf("list logs: %v", err)
	}
	if len(logs) != 1 || logs[0].Message != "hello" {
		t.Fatalf("unexpected logs: %#v", logs)
	}
}

func TestEVAChatMigrationAndFolderSettingsRoundTrip(t *testing.T) {
	baseDir := t.TempDir()

	uiStore, err := Open(baseDir)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer uiStore.Close()

	// 1. Save folder mode settings
	folderMsg := model.MessageSettings{
		ChatMode:           model.ChatModeFolder,
		Sequence:           "line1\nline2",
		LoadedSequences:    []string{"pattern 1", "pattern 2"},
		UseLoadedSequences: true,
	}
	if err := uiStore.SaveMessageSettings(folderMsg); err != nil {
		t.Fatalf("save folder message: %v", err)
	}
	gotFolder, err := uiStore.LoadMessageSettings()
	if err != nil {
		t.Fatalf("load folder message: %v", err)
	}
	if gotFolder.ChatMode != model.ChatModeFolder || !gotFolder.UseLoadedSequences || len(gotFolder.LoadedSequences) != 2 {
		t.Fatalf("unexpected folder settings: %#v", gotFolder)
	}

	// 2. An old persisted keyword selection migrates to EVAChat Flow.
	keywordMsg := model.MessageSettings{
		ChatMode:            model.ChatModeKeyword,
		Sequence:            "line1\nline2",
		LoadedSequences:     []string{"pattern 1", "pattern 2"},
		UseLoadedSequences:  false,
		KeywordRulesContent: "(he|hello){[hi|hey] [how|hw] [are|r] [you|u]}",
		KeywordFilePath:     "C:\\test\\rules.txt",
	}
	if err := uiStore.SaveMessageSettings(keywordMsg); err != nil {
		t.Fatalf("save keyword message: %v", err)
	}
	gotKeyword, err := uiStore.LoadMessageSettings()
	if err != nil {
		t.Fatalf("load keyword message: %v", err)
	}
	if gotKeyword.ChatMode != model.ChatModeEVAChat {
		t.Fatalf("expected migrated ChatMode to be %q, got %q", model.ChatModeEVAChat, gotKeyword.ChatMode)
	}
	if gotKeyword.UseLoadedSequences {
		t.Fatalf("expected UseLoadedSequences to be false in EVAChat Flow, got true")
	}
	if gotKeyword.KeywordRulesContent != keywordMsg.KeywordRulesContent {
		t.Fatalf("expected KeywordRulesContent %q, got %q", keywordMsg.KeywordRulesContent, gotKeyword.KeywordRulesContent)
	}
	if gotKeyword.KeywordFilePath != keywordMsg.KeywordFilePath {
		t.Fatalf("expected KeywordFilePath %q, got %q", keywordMsg.KeywordFilePath, gotKeyword.KeywordFilePath)
	}
}

func TestSaveMessageSettingsPreservesAPIKeyAndWritesMirror(t *testing.T) {
	baseDir := t.TempDir()
	uiStore, err := Open(baseDir)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer uiStore.Close()

	initial := model.MessageSettings{
		ChatMode:  model.ChatModeAPI,
		Sequence:  "hello",
		Usernames: "@snap1",
		AIBaseURL: "https://api.featherless.ai/v1",
		AIModel:   "deepseek-ai/DeepSeek-V3-0324",
		AIAPIKey:  "sk-secret-persistent-key",
	}
	if err := uiStore.SaveMessageSettings(initial); err != nil {
		t.Fatalf("save initial: %v", err)
	}

	// Saving another mode with an empty AIAPIKey must keep the stored API key.
	update := model.MessageSettings{
		ChatMode:  model.ChatModeSingle,
		Sequence:  "updated line",
		Usernames: "@snap2",
	}
	if err := uiStore.SaveMessageSettings(update); err != nil {
		t.Fatalf("save update: %v", err)
	}

	got, err := uiStore.LoadMessageSettings()
	if err != nil {
		t.Fatalf("load message: %v", err)
	}
	if got.AIAPIKey != "sk-secret-persistent-key" {
		t.Fatalf("expected API key to remain persisted, got %q", got.AIAPIKey)
	}

	// Mirror file is readable even while BadgerDB is still open/locked.
	mirror, err := LoadMessageSettingsMirror(baseDir)
	if err != nil {
		t.Fatalf("load mirror while Badger open: %v", err)
	}
	if mirror.AIAPIKey != "sk-secret-persistent-key" || mirror.Sequence != "updated line" {
		t.Fatalf("unexpected mirror settings: %#v", mirror)
	}
}

func TestGlobalSettingsLiveUnderGlobalDir(t *testing.T) {
	baseDir := t.TempDir()
	s, err := Open(baseDir)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer s.Close()

	if err := s.SaveMessageSettings(model.MessageSettings{ChatMode: model.ChatModeSingle, Sequence: "hello"}); err != nil {
		t.Fatalf("save message: %v", err)
	}
	if _, err := os.Stat(filepath.Join(baseDir, "ui")); !os.IsNotExist(err) {
		t.Fatalf("legacy ui/ must not be created: %v", err)
	}
	if _, err := os.Stat(filepath.Join(baseDir, "_global", "ui", "badger")); err != nil {
		t.Fatalf("badger must live in _global/ui: %v", err)
	}
	if _, err := os.Stat(filepath.Join(baseDir, "_global", "ui", "message_settings.json")); err != nil {
		t.Fatalf("mirror must live in _global/ui: %v", err)
	}
}
