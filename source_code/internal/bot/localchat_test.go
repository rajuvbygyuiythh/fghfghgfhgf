package bot

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"chitchatbot/internal/model"
)

func TestNormalizeLocalModeAcceptsDashboardValues(t *testing.T) {
	cases := map[string]string{
		"1":        model.ChatModeSingle,
		"folder":   model.ChatModeFolder,
		"3":        model.ChatModeEVAChat,
		"api_chat": model.ChatModeAPI,
		"API":      model.ChatModeAPI,
		"":         "",
		"nope":     "",
	}
	for raw, want := range cases {
		if got := normalizeLocalMode(raw); got != want {
			t.Fatalf("%q -> %q, want %q", raw, got, want)
		}
	}
}

func TestRunLocalChatSingleSequenceUsesDashboardSettings(t *testing.T) {
	dataDir := t.TempDir()
	settings := model.NewMessageSettings("hello %username%\nok wait a bit", "snapqueen")
	settings.ChatMode = model.ChatModeSingle

	var out bytes.Buffer
	err := RunLocalChat(context.Background(), LocalChatOptions{
		DataDir:  dataDir,
		Settings: settings,
		Mode:     "1",
		In:       strings.NewReader("hi there\nhow are you\n/state\n/quit\n"),
		Out:      &out,
	})
	if err != nil {
		t.Fatalf("RunLocalChat: %v", err)
	}

	text := out.String()
	for _, want := range []string{"Single Sequence", "hello snapqueen", "ok wait a bit", "step 1/2", "step 2/2"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in output:\n%s", want, text)
		}
	}
	if strings.Contains(text, "%username%") {
		t.Fatalf("username placeholder leaked:\n%s", text)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "bot_state.json")); !os.IsNotExist(err) {
		t.Fatalf("the local test must not write bot state: %v", err)
	}
}

func TestRunLocalChatMenuListsAllFourDashboardModes(t *testing.T) {
	settings := model.NewMessageSettings("hey there", "")
	settings.ChatMode = model.ChatModeSingle

	var out bytes.Buffer
	err := RunLocalChat(context.Background(), LocalChatOptions{
		DataDir:  t.TempDir(),
		Settings: settings,
		In:       strings.NewReader("1\nhi\n/quit\n"),
		Out:      &out,
	})
	if err != nil {
		t.Fatalf("RunLocalChat: %v", err)
	}
	text := out.String()
	for _, want := range []string{
		"Choose the dashboard Message option",
		"Single Sequence",
		"Folder Sequences",
		"EVAChat Flow",
		"API Chat",
		"bot> hey there",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in menu output:\n%s", want, text)
		}
	}
	if !strings.Contains(text, "none in the dashboard") {
		t.Fatalf("missing username warning:\n%s", text)
	}
}

func TestRunLocalChatAPIWithoutKeyExplainsSetup(t *testing.T) {
	var out bytes.Buffer
	err := RunLocalChat(context.Background(), LocalChatOptions{
		DataDir:  t.TempDir(),
		Settings: model.DefaultMessageSettings(),
		Mode:     "4",
		In:       strings.NewReader(""),
		Out:      &out,
	})
	if err == nil {
		t.Fatal("API Chat without a key must fail with a setup hint")
	}
	if !strings.Contains(err.Error(), "API Chat") || !strings.Contains(err.Error(), "dashboard") {
		t.Fatalf("unhelpful error: %v", err)
	}
}

func TestLocalChatSessionInteractiveStartAndReply(t *testing.T) {
	dataDir := t.TempDir()
	settings := model.NewMessageSettings("hey %username%\nwait a sec", "snap_star")
	settings.ChatMode = model.ChatModeSingle

	session, err := NewLocalChatSession(dataDir, settings, model.ChatModeSingle)
	if err != nil {
		t.Fatalf("NewLocalChatSession: %v", err)
	}
	turn1 := session.Start(context.Background())
	if len(turn1.Lines) != 1 || turn1.Lines[0] != "hey snap_star" {
		t.Fatalf("unexpected start turn: %+v", turn1)
	}
	if turn1.State.AssignedUsername != "snap_star" || turn1.State.Step != 1 || turn1.State.TotalSteps != 2 {
		t.Fatalf("unexpected state after start: %+v", turn1.State)
	}

	turn2 := session.Reply(context.Background(), "hello who is this")
	if len(turn2.Lines) != 1 || turn2.Lines[0] != "wait a sec" {
		t.Fatalf("unexpected reply turn: %+v", turn2)
	}
	if turn2.State.Step != 2 || turn2.State.TotalSteps != 2 {
		t.Fatalf("unexpected state after reply: %+v", turn2.State)
	}
}
