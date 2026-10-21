package apichat

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// newTestEngine wires the engine to a local OpenAI-compatible fake.
func newTestEngine(t *testing.T, handler http.Handler) *Engine {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	engine, err := New(Config{
		BaseURL:    srv.URL,
		APIKey:     "test-key",
		Model:      "mock-model",
		Countries:  []string{"uk"},
		HTTPClient: srv.Client(),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return engine
}

func replyJSON(content string) string {
	payload := map[string]any{
		"choices": []map[string]any{{"message": map[string]any{"content": content}}},
	}
	data, _ := json.Marshal(payload)
	return string(data)
}

func TestEngineStartAndReplyUseThePersona(t *testing.T) {
	var sawPersona atomic.Bool
	engine := newTestEngine(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Model    string `json:"model"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.Model != "mock-model" {
			t.Errorf("model = %q", body.Model)
		}
		for _, m := range body.Messages {
			if m.Role == "system" && strings.Contains(m.Content, "anonymous") {
				sawPersona.Store(true)
			}
		}
		fmt.Fprint(w, replyJSON("hey lol"))
	}))

	session := NewSession(nil)
	ctx := context.Background()

	start := engine.Start(ctx, &session)
	if len(start.Lines) != 1 || start.Lines[0] != "hey lol" {
		t.Fatalf("start lines = %v", start.Lines)
	}
	if start.PersonaAge < 18 || start.PersonaCountry != "uk" {
		t.Fatalf("persona not applied: %+v", start)
	}

	reply := engine.Reply(ctx, &session, "hey wyd")
	if len(reply.Lines) == 0 {
		t.Fatal("empty reply")
	}
	if !sawPersona.Load() {
		t.Fatal("persona system prompt was never sent")
	}
	if len(session.History) < 3 {
		t.Fatalf("history should contain both sides: %v", session.History)
	}
}

func TestEngineRetriesOnceWhenTheReplySoundsLikeAnAI(t *testing.T) {
	var calls atomic.Int32
	engine := newTestEngine(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			fmt.Fprint(w, replyJSON("as an ai language model i cannot do that"))
			return
		}
		fmt.Fprint(w, replyJSON("lol idk what u mean"))
	}))

	session := NewSession(nil)
	result := engine.Reply(context.Background(), &session, "u a bot?")
	if len(result.Lines) != 1 || result.Lines[0] != "lol idk what u mean" {
		t.Fatalf("expected the retried line, got %v (%s)", result.Lines, result.Detail)
	}
	if calls.Load() != 2 {
		t.Fatalf("expected exactly one retry, got %d calls", calls.Load())
	}
}

func TestEngineFallsBackWhenTheAPIErrors(t *testing.T) {
	engine := newTestEngine(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":{"message":"boom"}}`, http.StatusInternalServerError)
	}))

	session := NewSession(nil)
	result := engine.Reply(context.Background(), &session, "hey")
	if len(result.Lines) != 1 || result.Lines[0] == "" {
		t.Fatalf("fallback line missing: %+v", result)
	}
	if !strings.Contains(result.Detail, "api error") {
		t.Fatalf("detail should explain the failure: %q", result.Detail)
	}
	if len(session.History) == 0 || session.History[len(session.History)-1].Content != result.Lines[0] {
		t.Fatal("fallback line must be appended to the history so the model sees it next time")
	}
}

func TestEngineSharesUsernamePlaceholderAndNeverWritesItItself(t *testing.T) {
	engine := newTestEngine(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, replyJSON("ok fine lol"))
	}))

	session := NewSession(nil)
	session.Director.Tier = tierHot
	ctx := context.Background()
	_ = engine.Start(ctx, &session)
	result := engine.Reply(ctx, &session, "u got snap?")
	if !result.Shared || !result.HasUsername {
		t.Fatalf("hot reply to a snap ask must share: %+v", result)
	}
	if len(result.Lines) < 2 {
		t.Fatalf("expected lead-in plus share line, got %v", result.Lines)
	}
	if !strings.Contains(result.Lines[len(result.Lines)-1], UsernamePlaceholder) {
		t.Fatalf("share line must carry the placeholder for Go to fill: %v", result.Lines)
	}
}

func TestEngineWithoutKeyIsRejected(t *testing.T) {
	if _, err := New(Config{}); err == nil {
		t.Fatal("an API key must be required")
	}
}

func TestNoThinkFieldsSwitchesOffKnownReasoningModels(t *testing.T) {
	if got := noThinkFields("Qwen/Qwen3-32B"); got == nil || got["chat_template_kwargs"] == nil {
		t.Fatalf("qwen3 should disable thinking: %v", got)
	}
	if got := noThinkFields("gpt-oss-120b"); got["reasoning_effort"] != "low" {
		t.Fatalf("gpt-oss should use low reasoning: %v", got)
	}
	if got := noThinkFields("deepseek-ai/DeepSeek-V3-0324"); got != nil {
		t.Fatalf("non-thinking models must not be modified: %v", got)
	}
}

func TestEngineGuardRetryCarriesTheReason(t *testing.T) {
	var calls atomic.Int32
	var retrySawReason atomic.Bool
	engine := newTestEngine(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if calls.Add(1) == 1 {
			fmt.Fprint(w, replyJSON("hey"))
			return
		}
		if strings.Contains(string(body), "rejected") && strings.Contains(string(body), "refer to") {
			retrySawReason.Store(true)
		}
		fmt.Fprint(w, replyJSON("long shift huh"))
	}))

	session := NewSession(nil)
	result := engine.Reply(context.Background(), &session, "i work at a coffee shop downtown")
	if len(result.Lines) != 1 || result.Lines[0] != "long shift huh" {
		t.Fatalf("anchored retry not used: %v (%s)", result.Lines, result.Detail)
	}
	if calls.Load() != 2 {
		t.Fatalf("expected one retry, got %d calls", calls.Load())
	}
	if !retrySawReason.Load() {
		t.Fatal("retry prompt did not explain the rejection")
	}
}

func TestShareDeckNeverRepeatsInsideOneChat(t *testing.T) {
	engine := newTestEngine(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, replyJSON("lol"))
	}))
	engine.cfg.ShareLines = []string{"a %username%", "b %username%", "c %username%"}
	session := NewSession(nil)
	seen := map[string]bool{}
	for i := 0; i < len(engine.cfg.ShareLines); i++ {
		line := engine.pickShareLine(&session)
		if seen[line] {
			t.Fatalf("share line repeated before the deck ran out: %v", session.UsedShareLines)
		}
		seen[line] = true
	}
	if len(session.UsedShareLines) != 3 {
		t.Fatalf("used lines = %v", session.UsedShareLines)
	}
	if line := engine.pickShareLine(&session); line == "" || len(session.UsedShareLines) != 1 {
		t.Fatalf("deck did not reset: %v", session.UsedShareLines)
	}
}

func TestSessionKeepsQualityStateThroughJSON(t *testing.T) {
	session := NewSession(rand.New(rand.NewSource(11)))
	session.Director.Style = pickStyle(&session.Director)
	session.Director.Observe("im 22 from texas, i work at a cafe")
	session.UsedShareLines = []string{"ok add me %username%"}

	data, err := json.Marshal(session)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back Session
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.Director.Style != session.Director.Style || back.Director.Style == "" {
		t.Fatalf("style lost: %q vs %q", back.Director.Style, session.Director.Style)
	}
	if !back.Director.Ledger.Known(FactAge) || !back.Director.Ledger.Known(FactPlace) {
		t.Fatalf("ledger lost: %+v", back.Director.Ledger)
	}
	if len(back.UsedShareLines) != 1 {
		t.Fatalf("share rotation lost: %v", back.UsedShareLines)
	}
}
