package bot

import (
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"chitchatbot/internal/apichat"
	"chitchatbot/internal/model"
)

func TestEnsureAPIChatDataSeedsWithoutOverwritingEdits(t *testing.T) {
	root, err := EnsureAPIChatData(t.TempDir())
	if err != nil {
		t.Fatalf("EnsureAPIChatData: %v", err)
	}
	persona := filepath.Join(root, "persona.txt")
	data, err := os.ReadFile(persona)
	if err != nil || len(data) == 0 {
		t.Fatalf("persona not seeded: %v", err)
	}
	custom := "my own persona\n"
	if err := os.WriteFile(persona, []byte(custom), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := EnsureAPIChatData(filepath.Dir(root)); err != nil {
		t.Fatalf("re-seed: %v", err)
	}
	again, _ := os.ReadFile(persona)
	if string(again) != custom {
		t.Fatalf("user edits were overwritten: %q", string(again))
	}
}

func TestLoadAPIChatConfigRequiresAKey(t *testing.T) {
	if _, err := LoadAPIChatConfig(t.TempDir(), model.MessageSettings{}); err == nil {
		t.Fatal("missing key must be a hard error")
	}
	settings := model.MessageSettings{AIAPIKey: "k", AIBaseURL: "https://example.com/v1", AIModel: "m"}
	cfg, err := LoadAPIChatConfig(t.TempDir(), settings)
	if err != nil {
		t.Fatalf("LoadAPIChatConfig: %v", err)
	}
	if cfg.Engine.BaseURL() != "https://example.com/v1" || cfg.Engine.Model() != "m" {
		t.Fatalf("unexpected engine: %s %s", cfg.Engine.BaseURL(), cfg.Engine.Model())
	}
}

func TestAPIChatFinishFillsTheUsernameInGo(t *testing.T) {
	cfg := Config{MessageUsernames: []string{"snapqueen"}}
	state := model.AccountState{}
	userID := "u1"

	session := apichat.Session{}
	result := apichat.Result{
		Lines:       []string{"ok fine lol", "add me on snap %username%"},
		Rule:        "reply 4 · share turn",
		Shared:      true,
		HasUsername: true,
	}
	out := apiChatFinish(cfg, &state, userID, rand.New(rand.NewSource(1)), session, result)
	if !out.HasUsername || !out.SnapShared {
		t.Fatalf("share flags lost: %+v", out)
	}
	if out.Text != "ok fine lol" {
		t.Fatalf("lead-in = %q", out.Text)
	}
	if len(out.Extra) != 1 || !strings.Contains(out.Extra[0], "snapqueen") {
		t.Fatalf("share line not rendered: %v", out.Extra)
	}
	if strings.Contains(out.Extra[0], "%username%") {
		t.Fatal("placeholder must never reach the chat")
	}
	if state.AssignedUsernames[userID] != "snapqueen" {
		t.Fatal("username assignment must be sticky per user")
	}
}

func TestAPIChatFinishDropsShareLineWithoutUsernames(t *testing.T) {
	cfg := Config{}
	state := model.AccountState{}
	out := apiChatFinish(cfg, &state, "u1", rand.New(rand.NewSource(1)), apichat.Session{}, apichat.Result{
		Lines:       []string{"ok fine lol", "add me on snap %username%"},
		HasUsername: true,
	})
	if out.HasUsername {
		t.Fatal("no username means the share line must not be counted")
	}
	if len(out.Extra) != 0 {
		t.Fatalf("dangling share line sent: %v", out.Extra)
	}
	if out.Text != "ok fine lol" {
		t.Fatalf("lead-in lost: %q", out.Text)
	}
}
