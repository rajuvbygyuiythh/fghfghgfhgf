package apichat

// API Chat engine: persona + director + OpenAI-compatible model. One call per
// bot reply, with one guard retry and an editable fallback deck so the bot
// never goes silent when the API is down.

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"strings"
	"time"
)

const (
	// DefaultBaseURL is the provider used by the apitest tuning tool.
	DefaultBaseURL = "https://api.featherless.ai/v1"
	// DefaultModel is a non-thinking, casual-English model.
	DefaultModel   = "deepseek-ai/DeepSeek-V3-0324"
	DefaultTimeout = 25 * time.Second
	DefaultTokens  = 160
	DefaultHistory = 24
	DefaultAgeMin  = 20
	DefaultAgeMax  = 25
	// UsernamePlaceholder is replaced by the bot with the assigned handle.
	UsernamePlaceholder = "%username%"
)

// Config is the runtime configuration for API Chat.
type Config struct {
	BaseURL       string
	APIKey        string
	Model         string
	Temperature   float64
	Timeout       time.Duration
	MaxTokens     int
	History       int
	AgeMin        int
	AgeMax        int
	Countries     []string
	ShareHot      int
	ShareWarm     int
	ShareCold     int
	Persona       string
	ShareLines    []string
	FallbackLines []string
	HTTPClient    *http.Client
	// DisableNoThink keeps the automatic thinking-off request fields out.
	DisableNoThink bool
}

// Engine holds the validated configuration; it is safe for concurrent use.
type Engine struct {
	cfg Config
}

// Result is one bot turn.
type Result struct {
	Lines          []string
	Rule           string
	Detail         string
	Ended          bool
	Shared         bool
	HasUsername    bool
	PersonaAge     int
	PersonaCountry string
	Stage          string
	Style          string
	RuleWhy        string
	Mood           string
	Instruction    string
	UserKind       string
	RawOutput      string
	Diagnostic     string
}

// New validates and normalises the configuration.
func New(cfg Config) (*Engine, error) {
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, errors.New("API Chat needs an API key")
	}
	base := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if base == "" {
		base = DefaultBaseURL
	}
	if !strings.HasPrefix(base, "http://") && !strings.HasPrefix(base, "https://") {
		return nil, fmt.Errorf("API base URL must start with http:// or https:// (got %q)", base)
	}
	if strings.TrimSpace(cfg.Model) == "" {
		cfg.Model = DefaultModel
	}
	if cfg.Temperature <= 0 {
		cfg.Temperature = 0.9
	}
	cfg.Timeout = clampTimeout(cfg.Timeout)
	if cfg.MaxTokens <= 0 {
		cfg.MaxTokens = DefaultTokens
	}
	if cfg.History <= 0 {
		cfg.History = DefaultHistory
	}
	if cfg.AgeMin < 18 {
		cfg.AgeMin = DefaultAgeMin
	}
	if cfg.AgeMax < cfg.AgeMin || cfg.AgeMax > 60 {
		cfg.AgeMax = DefaultAgeMax
	}
	if cfg.ShareHot <= 0 {
		cfg.ShareHot = 4
	}
	if cfg.ShareWarm <= 0 {
		cfg.ShareWarm = 6
	}
	if cfg.ShareCold <= 0 {
		cfg.ShareCold = 8
	}
	if len(cfg.ShareLines) == 0 {
		cfg.ShareLines = append([]string(nil), defaultShareLines...)
	}
	if len(cfg.FallbackLines) == 0 {
		cfg.FallbackLines = append([]string(nil), defaultFallbackLines...)
	}
	cfg.BaseURL = base
	return &Engine{cfg: cfg}, nil
}

// BaseURL is the normalised endpoint (no key, safe to log).
func (e *Engine) BaseURL() string { return e.cfg.BaseURL }

// Model is the configured model name (safe to log).
func (e *Engine) Model() string { return e.cfg.Model }

// Describe is a key-free one-liner for the worker log.
func (e *Engine) Describe() string {
	return fmt.Sprintf("API Chat: %s (%s), share at %d/%d/%d, %d countries", e.cfg.BaseURL, e.cfg.Model, e.cfg.ShareHot, e.cfg.ShareWarm, e.cfg.ShareCold, len(e.cfg.Countries))
}

// RenderSystemPrompt returns the rendered persona prompt for the session.
func (e *Engine) RenderSystemPrompt(s *Session) string {
	e.ensure(s)
	return systemPrompt(e.cfg, s)
}

// DefaultPersonaText returns the built-in default persona template.
func DefaultPersonaText() string {
	return defaultPersona
}

// Start produces the opening message of a new conversation.
func (e *Engine) Start(ctx context.Context, s *Session) Result {
	e.ensure(s)
	s.Started = true
	s.Turn++
	s.Director.Turn = s.Turn
	p := Plan{
		Mood:        "friendly",
		Instruction: "you are opening the chat (they have not written yet): send a short 2-3 word casual opener like 'heyy wyd' or 'hey how r u'. one short line.",
		Why:         "opener",
	}
	return e.produce(ctx, s, p, "")
}

// Reply answers one stranger message.
func (e *Engine) Reply(ctx context.Context, s *Session, incoming string) Result {
	e.ensure(s)
	incoming = strings.TrimSpace(incoming)
	s.Started = true
	s.Turn++
	s.Director.Turn = s.Turn
	s.Director.Observe(incoming)
	s.History = append(s.History, Message{Role: "user", Content: incoming})
	p := s.Director.Plan(s.Turn, incoming, e.cfg.ShareHot, e.cfg.ShareWarm, e.cfg.ShareCold)
	return e.produce(ctx, s, p, incoming)
}

// Test sends one tiny request so the dashboard can verify the key.
func (e *Engine) Test(ctx context.Context) (string, error) {
	s := NewSession(rand.New(rand.NewSource(time.Now().UnixNano())))
	e.ensure(&s)
	msgs := []Message{
		{Role: "system", Content: systemPrompt(e.cfg, &s)},
		{Role: "user", Content: "hey wyd"},
	}
	raw, err := e.chat(ctx, msgs)
	if err != nil {
		return "", err
	}
	lines, _ := sanitizeLines(raw, false)
	if len(lines) > 0 {
		return lines[0], nil
	}
	return strings.TrimSpace(raw), nil
}

func (e *Engine) ensure(s *Session) {
	s.ensurePersona(e.cfg.Countries)
	if s.Director.Tier == "" {
		s.Director.Tier = tierCold
	}
	if s.Director.Moods == nil {
		s.Director.Moods = append([]string(nil), allMoods...)
	}
	if s.Director.Style == "" {
		s.Director.Style = pickStyle(&s.Director)
	}
	if s.Director.Interest == nil {
		s.Director.Interest = Interest{}
	}
	s.Director.Ledger.init()
	if s.Director.Turn == 0 {
		s.Director.Turn = s.Turn
	}
}

func (e *Engine) produce(ctx context.Context, s *Session, p Plan, userText string) Result {
	res := Result{
		PersonaAge:     s.PersonaAge,
		PersonaCountry: s.PersonaCountry,
		RuleWhy:        p.Why,
		Mood:           p.Mood,
		Instruction:    p.Instruction,
		UserKind:       string(s.Director.Profile.Kind),
	}

	var lines []string
	detail := ""
	rawOut := ""
	if p.Fixed != "" {
		lines = []string{p.Fixed}
		detail = "fixed: " + p.Why
		rawOut = p.Fixed
	} else {
		lines, detail, rawOut = e.generate(ctx, s, p, userText)
	}
	res.RawOutput = rawOut
	res.Diagnostic = detail

	if p.Why == "greeting" && !s.Director.userQuestion && len(lines) > 0 {
		lines = []string{trimShortGreeting(lines[0])}
	}

	if p.ShareNow {
		if share := e.pickShareLine(s); share != "" {
			lines = append(lines, share)
			res.Shared = true
			res.HasUsername = strings.Contains(share, UsernamePlaceholder)
		}
	}

	for _, line := range lines {
		s.History = append(s.History, Message{Role: "assistant", Content: line})
	}
	s.trimHistory(e.cfg.History)

	res.Lines = lines
	res.Ended = p.EndAfter
	res.Rule = fmt.Sprintf("reply %d", s.Turn)
	if p.Why != "" {
		res.Rule += " · " + p.Why
	}
	res.Detail = s.Director.Status(e.cfg.ShareHot, e.cfg.ShareWarm, e.cfg.ShareCold)
	if detail != "" {
		res.Detail += " · " + detail
	}
	res.Stage = string(stageFor(&s.Director, s.Turn, s.Director.ShareTurn(e.cfg.ShareHot, e.cfg.ShareWarm, e.cfg.ShareCold)))
	res.Style = string(s.Director.styleFor())
	return res
}

func (e *Engine) generate(ctx context.Context, s *Session, p Plan, userText string) ([]string, string, string) {
	var lastLines []string
	var lastWarn string
	var lastProblem string
	var lastRaw string

	for attempt := 0; attempt < 2; attempt++ {
		msgs := buildMessages(e.cfg, s, p, s.Turn, lastProblem)
		raw, err := e.chat(ctx, msgs)
		if err != nil {
			return e.fallback(s), "api error: " + truncate(err.Error(), 160), ""
		}
		lastRaw = strings.TrimSpace(raw)
		lines, warn := sanitizeLines(raw, p.Double)
		if len(lines) == 0 {
			lastWarn = addNote(warn, "empty output")
			lastProblem = "empty reply"
			continue
		}
		lastLines, lastWarn = lines, warn
		problem := validateReply(lines, &s.Director, s.Director.Profile, userText, s.History)
		if problem == "" {
			return polish(lines, s.History), warn, lastRaw
		}
		lastProblem = problem
		lastWarn = addNote(warn, "retry: "+problem)
	}
	if len(lastLines) > 0 && softGuardProblem(lastProblem) {
		return polish(lastLines, s.History), addNote(lastWarn, "accepted after retry"), lastRaw
	}
	return safeReply(s.Director.Profile), addNote(lastWarn, "safe reply"), lastRaw
}

func (e *Engine) fallback(s *Session) []string {
	if len(e.cfg.FallbackLines) == 0 {
		return []string{"lol my phone froze, say that again"}
	}
	// Even the offline deck must not repeat a line the stranger just saw.
	candidates := make([]string, 0, len(e.cfg.FallbackLines))
	for _, line := range e.cfg.FallbackLines {
		if score, _ := mostSimilar(line, s.History, 8); score < repeatThreshold {
			candidates = append(candidates, line)
		}
	}
	if len(candidates) == 0 {
		candidates = e.cfg.FallbackLines
	}
	return []string{candidates[s.Director.roll(len(candidates))]}
}

// pickShareLine draws a random line from sharesnap.txt, never repeating a
// line inside the same conversation until the deck is exhausted.
func (e *Engine) pickShareLine(s *Session) string {
	if len(e.cfg.ShareLines) == 0 {
		return ""
	}
	last := ""
	if n := len(s.UsedShareLines); n > 0 {
		last = s.UsedShareLines[n-1]
	}
	used := make(map[string]bool, len(s.UsedShareLines))
	for _, line := range s.UsedShareLines {
		used[line] = true
	}
	candidates := make([]string, 0, len(e.cfg.ShareLines))
	for _, line := range e.cfg.ShareLines {
		if !used[line] {
			candidates = append(candidates, line)
		}
	}
	if len(candidates) == 0 {
		s.UsedShareLines = nil
		candidates = append(candidates, e.cfg.ShareLines...)
	}
	if len(candidates) > 1 && last != "" {
		filtered := make([]string, 0, len(candidates))
		for _, line := range candidates {
			if line != last {
				filtered = append(filtered, line)
			}
		}
		if len(filtered) > 0 {
			candidates = filtered
		}
	}
	pick := candidates[s.Director.roll(len(candidates))]
	s.UsedShareLines = append(s.UsedShareLines, pick)
	return pick
}

func addNote(warn, note string) string {
	warn = strings.TrimSpace(warn)
	if warn == "" {
		return note
	}
	return warn + "; " + note
}

func truncate(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// trimShortGreeting keeps only the short greeting token ("heyy", "hii", "hey lol",
// "hey there") when the stranger sent a pure greeting ("hi", "hey", "hello"),
// removing any unsolicited trailing clause like "how's ur night going".
func trimShortGreeting(line string) string {
	words := strings.Fields(strings.TrimSpace(line))
	if len(words) == 0 {
		return "heyy"
	}
	first := strings.Trim(words[0], ",.!?")
	if first == "" {
		return "heyy"
	}
	if len(words) >= 2 {
		second := strings.ToLower(strings.Trim(words[1], ",.!?"))
		if second == "lol" || second == "there" || second == "u" {
			return first + " " + second
		}
	}
	return first
}
