package bot

// API Chat worker glue: the fourth chat mode. Every line is written by the
// configured model (internal/apichat), the persona/share/fallback decks live in
// data/api_chat, and the snap username is always added here in Go.

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"

	"chitchatbot/internal/apichat"
	"chitchatbot/internal/model"
)

// APIChatConfig is the loaded, shared API Chat runtime for all workers.
type APIChatConfig struct {
	DataRoot string
	Engine   *apichat.Engine
}

// LoadAPIChatConfig seeds/reads data/api_chat and validates the dashboard
// settings. A missing key is a hard error: API Chat cannot fall back to TXT.
func LoadAPIChatConfig(dataDir string, settings model.MessageSettings) (*APIChatConfig, error) {
	if strings.TrimSpace(settings.AIAPIKey) == "" {
		return nil, fmt.Errorf("API Chat needs an API key (Message → API)")
	}
	dataRoot, err := EnsureAPIChatData(dataDir)
	if err != nil {
		return nil, err
	}
	settings = model.NormalizeAPISettings(settings)
	persona, err := readAPIText(filepath.Join(dataRoot, "persona.txt"))
	if err != nil {
		return nil, err
	}
	countries, err := readAPILines(filepath.Join(dataRoot, "persona_countries.txt"))
	if err != nil {
		return nil, err
	}
	// sharesnap.txt is the real deck. share.txt stays readable so installs
	// created before the split keep their own lines.
	share, err := readAPILines(filepath.Join(dataRoot, "sharesnap.txt"))
	if err != nil {
		return nil, err
	}
	if len(share) == 0 {
		share, err = readAPILines(filepath.Join(dataRoot, "share.txt"))
		if err != nil {
			return nil, err
		}
	}
	fallback, err := readAPILines(filepath.Join(dataRoot, "fallback.txt"))
	if err != nil {
		return nil, err
	}
	if len(countries) == 0 {
		countries = []string{"usa", "uk", "canada", "australia"}
	}

	engine, err := apichat.New(apichat.Config{
		BaseURL:       strings.TrimSpace(settings.AIBaseURL),
		APIKey:        strings.TrimSpace(settings.AIAPIKey),
		Model:         strings.TrimSpace(settings.AIModel),
		Temperature:   settings.APITemperature,
		Countries:     countries,
		ShareHot:      settings.APIShareHot,
		ShareWarm:     settings.APIShareWarm,
		ShareCold:     settings.APIShareCold,
		Persona:       persona,
		ShareLines:    share,
		FallbackLines: fallback,
	})
	if err != nil {
		return nil, err
	}
	return &APIChatConfig{DataRoot: dataRoot, Engine: engine}, nil
}

// readAPIText reads a whole TXT file; a missing file returns "" so the engine
// falls back to its built-in default persona.
func readAPIText(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func apiChatEnabled(cfg Config) bool {
	return cfg.ChatMode == model.ChatModeAPI && cfg.APIChat != nil && cfg.APIChat.Engine != nil
}

func apiChatSession(accountState *model.AccountState, userID string, rng *rand.Rand) apichat.Session {
	if accountState.APIChatSessions == nil {
		accountState.APIChatSessions = make(map[string]apichat.Session)
	}
	if session, ok := accountState.APIChatSessions[userID]; ok && session.Started {
		return session
	}
	session := apichat.NewSession(rng)
	accountState.APIChatSessions[userID] = session
	return session
}

// WithSessionConnection attaches a browserless SessionConnection to a bot Config (Prompt 11).
func (c Config) WithSessionConnection(conn *SessionConnection) Config {
	c.SessionConn = conn
	if conn != nil && strings.TrimSpace(conn.WSURL) != "" && strings.TrimSpace(c.WebSocketURL) == "" {
		c.WebSocketURL = conn.WSURL
	}
	return c
}

// APIChatStartTurnWithConnection starts an API Chat conversation bound to a
// browserless SessionConnection (Prompt 11).
func APIChatStartTurnWithConnection(ctx context.Context, cfg Config, conn *SessionConnection, accountState *model.AccountState, userID string, rng *rand.Rand) EVAReplyResult {
	return APIChatStartTurn(ctx, cfg.WithSessionConnection(conn), accountState, userID, rng)
}

// APIChatReplyTurnWithConnection processes one incoming user message bound to a
// browserless SessionConnection (Prompt 11).
func APIChatReplyTurnWithConnection(ctx context.Context, cfg Config, conn *SessionConnection, accountState *model.AccountState, userID, incoming string, rng *rand.Rand) EVAReplyResult {
	return APIChatReplyTurn(ctx, cfg.WithSessionConnection(conn), accountState, userID, incoming, rng)
}

// APIChatStartTurn starts a new API Chat conversation for userID using
// api_chat.go and updates the site's isolated AccountState.
func APIChatStartTurn(ctx context.Context, cfg Config, accountState *model.AccountState, userID string, rng *rand.Rand) EVAReplyResult {
	normalizeBotAccountState(accountState)
	if rng == nil {
		rng = rand.New(rand.NewSource(1))
	}
	res := apiChatStart(ctx, cfg, accountState, userID, rng)
	if strings.TrimSpace(res.Text) != "" || len(res.Extra) > 0 {
		accountState.SentMessageCounts[userID]++
		accountState.BaselineDone[userID] = true
		if res.HasUsername {
			accountState.SnapTgSentCount++
			if dir := cfg.EffectiveDataDir(); dir != "" {
				_ = markUserFlagged(dir, userID)
			}
		}
	}
	return res
}

// APIChatReplyTurn processes one incoming user message through API Chat using
// api_chat.go and updates the site's isolated AccountState.
func APIChatReplyTurn(ctx context.Context, cfg Config, accountState *model.AccountState, userID, incoming string, rng *rand.Rand) EVAReplyResult {
	normalizeBotAccountState(accountState)
	if rng == nil {
		rng = rand.New(rand.NewSource(1))
	}
	res := apiChatReply(ctx, cfg, accountState, userID, incoming, rng)
	if strings.TrimSpace(res.Text) != "" || len(res.Extra) > 0 {
		accountState.SentMessageCounts[userID]++
		accountState.BaselineDone[userID] = true
		if res.HasUsername {
			accountState.SnapTgSentCount++
			if dir := cfg.EffectiveDataDir(); dir != "" {
				_ = markUserFlagged(dir, userID)
			}
		}
	}
	return res
}

// FlowMessageLines returns all non-empty lines from an EVAChat or API Chat turn.
func FlowMessageLines(result EVAReplyResult) []string {
	lines := make([]string, 0, 1+len(result.Extra))
	if t := strings.TrimSpace(result.Text); t != "" {
		lines = append(lines, t)
	}
	for _, extra := range result.Extra {
		if t := strings.TrimSpace(extra); t != "" {
			lines = append(lines, t)
		}
	}
	return lines
}

func apiChatStart(ctx context.Context, cfg Config, accountState *model.AccountState, userID string, rng *rand.Rand) EVAReplyResult {
	session := apiChatSession(accountState, userID, rng)
	result := cfg.APIChat.Engine.Start(ctx, &session)
	return apiChatFinish(cfg, accountState, userID, rng, session, result)
}

func apiChatReply(ctx context.Context, cfg Config, accountState *model.AccountState, userID, incoming string, rng *rand.Rand) EVAReplyResult {
	session := apiChatSession(accountState, userID, rng)
	result := cfg.APIChat.Engine.Reply(ctx, &session, incoming)
	return apiChatFinish(cfg, accountState, userID, rng, session, result)
}

func apiChatFinish(cfg Config, accountState *model.AccountState, userID string, rng *rand.Rand, session apichat.Session, result apichat.Result) EVAReplyResult {
	if accountState.APIChatSessions == nil {
		accountState.APIChatSessions = make(map[string]apichat.Session)
	}
	accountState.APIChatSessions[userID] = session

	out := EVAReplyResult{
		Ended:       result.Ended,
		SourcePool:  "api",
		Reason:      result.Rule,
		SnapShared:  result.Shared,
		RuleWhy:     result.RuleWhy,
		Mood:        result.Mood,
		Instruction: result.Instruction,
		UserKind:    result.UserKind,
		RawOutput:   result.RawOutput,
		Diagnostic:  result.Diagnostic,
	}
	if len(result.Lines) == 0 {
		return out
	}

	var username string
	if result.HasUsername {
		username = getOrAssignUsername(cfg, accountState, userID, rng)
	}
	lines := make([]string, 0, len(result.Lines))
	for _, line := range result.Lines {
		if strings.Contains(line, "%username%") || strings.Contains(line, "{snap}") {
			if username == "" {
				username = getOrAssignUsername(cfg, accountState, userID, rng)
			}
			if username == "" {
				// No username configured: never send a dangling "add me on
				// snap" line. Keep the lead-in and log the misconfiguration.
				logf(cfg, "  -> [apichat] share line skipped: no dashboard username configured.\n")
				continue
			}
			line = strings.NewReplacer("%username%", username, "{snap}", username).Replace(line)
			out.HasUsername = true
			out.SnapShared = true
		}
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
		}
	}
	if len(lines) == 0 {
		return out
	}
	out.Text = lines[0]
	out.Extra = lines[1:]
	if result.Detail != "" {
		out.Reason = strings.TrimSpace(result.Rule + " | " + result.Detail)
	}
	if result.Ended {
		out.Reason = strings.TrimSpace(out.Reason + " | flow ended")
	}
	return out
}

// statefulChatStart dispatches the two stateful chat modes (TXT EVAChat Flow
// and model-driven API Chat) for a conversation opener.
func statefulChatStart(ctx context.Context, cfg Config, accountState *model.AccountState, userID string, rng *rand.Rand) (EVAReplyResult, string, bool) {
	switch {
	case evaChatEnabled(cfg):
		return evaChatStart(cfg, accountState, userID, rng), "evachat", true
	case apiChatEnabled(cfg):
		return apiChatStart(ctx, cfg, accountState, userID, rng), "apichat", true
	}
	return EVAReplyResult{}, "", false
}

// statefulChatReply dispatches one incoming message to the active mode.
func statefulChatReply(ctx context.Context, cfg Config, accountState *model.AccountState, userID, incoming string, rng *rand.Rand) (EVAReplyResult, string, bool) {
	switch {
	case evaChatEnabled(cfg):
		return evaChatReply(cfg, accountState, userID, incoming, rng), "evachat", true
	case apiChatEnabled(cfg):
		return apiChatReply(ctx, cfg, accountState, userID, incoming, rng), "apichat", true
	}
	return EVAReplyResult{}, "", false
}

// flowMessageText merges follow-up lines for call sites that can only send a
// single message (first message / realtime). Without it a double-text share
// would lose the username line while still counting as shared.
func flowMessageText(result EVAReplyResult) string {
	text := strings.TrimSpace(result.Text)
	for _, extra := range result.Extra {
		if extra = strings.TrimSpace(extra); extra != "" {
			text += "\n" + extra
		}
	}
	return text
}

// APIChatDescribe is a key-free status line for the dashboard/log.
func APIChatDescribe(cfg *APIChatConfig) string {
	if cfg == nil || cfg.Engine == nil {
		return "API Chat: not configured"
	}
	return cfg.Engine.Describe()
}
