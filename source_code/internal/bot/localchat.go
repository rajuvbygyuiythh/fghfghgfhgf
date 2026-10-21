package bot

// Local test chat: a console harness that lets the owner talk to the bot like a
// stranger would, using the dashboard's saved Message settings and the exact
// same engines the workers use. Nothing is sent to the real app, no browser is
// opened, and no bot state is written to disk. It exists so all four dashboard
// Message options (Single Sequence, Folder Sequences, EVAChat Flow, API Chat)
// can be tried locally before a real run.

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"math/rand"
	"strings"
	"time"

	"chitchatbot/internal/model"
)

// LocalChatOptions configures one interactive test session.
type LocalChatOptions struct {
	DataDir  string
	Settings model.MessageSettings
	// Mode is optional: "", "1".."4" or a chat mode name. Empty opens the menu.
	Mode string
	In   io.Reader
	Out  io.Writer
}

// localModes is the order of the four options on the dashboard Message page.
var localModes = []string{
	model.ChatModeSingle,
	model.ChatModeFolder,
	model.ChatModeEVAChat,
	model.ChatModeAPI,
}

const localTyping = "bot is typing..."

func localModeLabel(mode string) string {
	switch mode {
	case model.ChatModeSingle:
		return "Single Sequence"
	case model.ChatModeFolder:
		return "Folder Sequences"
	case model.ChatModeEVAChat:
		return "EVAChat Flow"
	case model.ChatModeAPI:
		return "API Chat"
	}
	return mode
}

func localModeStatus(s model.MessageSettings, mode string) string {
	switch mode {
	case model.ChatModeSingle:
		if n := len(s.SequenceLines()); n > 0 {
			return fmt.Sprintf("%d message(s) from the dashboard", n)
		}
		return "no sequence configured"
	case model.ChatModeFolder:
		if n := len(s.LoadedSequences); n > 0 {
			return fmt.Sprintf("%d loaded sequence(s)", n)
		}
		return "no folder loaded (falls back to Single)"
	case model.ChatModeEVAChat:
		return "TXT reply pools (data/eva_chat)"
	case model.ChatModeAPI:
		if strings.TrimSpace(s.AIAPIKey) == "" {
			return "NO API KEY - set it in the dashboard"
		}
		label := strings.TrimSpace(s.AIModel)
		if label == "" {
			label = "default model"
		}
		return "model: " + label
	}
	return ""
}

func normalizeLocalMode(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "single", "single sequence":
		return model.ChatModeSingle
	case "2", "folder", "folder sequences":
		return model.ChatModeFolder
	case "3", "eva", "evachat", "eva_chat_flow", "eva chat flow":
		return model.ChatModeEVAChat
	case "4", "api", "api chat", "api_chat":
		return model.ChatModeAPI
	}
	return ""
}

// RunLocalChat starts the console chat. An empty Mode opens the same four
// options that the dashboard Message page shows.
func RunLocalChat(ctx context.Context, opts LocalChatOptions) error {
	out := opts.Out
	if out == nil {
		out = io.Discard
	}
	in := opts.In
	if in == nil {
		in = strings.NewReader("")
	}
	if strings.TrimSpace(opts.DataDir) == "" {
		return fmt.Errorf("data folder is not set")
	}

	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)

	fmt.Fprintln(out)
	fmt.Fprintln(out, "==============================================================")
	fmt.Fprintln(out, "  ChitChat Bot - local message test (you are the stranger)")
	fmt.Fprintln(out, "==============================================================")
	fmt.Fprintf(out, "  data: %s\n", opts.DataDir)
	fmt.Fprintln(out, "  Nothing is sent to the real app. No browser is opened.")
	fmt.Fprintln(out, "  Commands: /new  /state  /help  /quit")
	fmt.Fprintln(out, "==============================================================")

	raw := strings.TrimSpace(opts.Mode)
	mode := normalizeLocalMode(raw)
	if mode == "" && raw != "" {
		return fmt.Errorf("unknown mode %q (use 1-4, single, folder, eva_chat_flow or api_chat)", raw)
	}

	var cfg Config
	var note string
	var err error
	if mode != "" {
		cfg, note, err = localChatConfig(opts.DataDir, opts.Settings, mode)
		if err != nil {
			return err
		}
	} else {
		for {
			mode, err = chooseLocalMode(scanner, out, opts.Settings)
			if err != nil {
				return err
			}
			if mode == "" {
				return nil
			}
			cfg, note, err = localChatConfig(opts.DataDir, opts.Settings, mode)
			if err == nil {
				break
			}
			fmt.Fprintf(out, "\n  [cannot start] %v\n", err)
		}
	}

	runner := &localChatRunner{
		cfg:     cfg,
		mode:    mode,
		rng:     rand.New(rand.NewSource(time.Now().UnixNano())),
		userID:  "local-test-user",
		scanner: scanner,
		out:     out,
	}
	if note != "" {
		fmt.Fprintf(out, "\n  note: %s\n", note)
	}
	return runner.run(ctx)
}

func chooseLocalMode(scanner *bufio.Scanner, out io.Writer, s model.MessageSettings) (string, error) {
	for {
		fmt.Fprintln(out)
		fmt.Fprintln(out, "  Choose the dashboard Message option to test:")
		for i, mode := range localModes {
			fmt.Fprintf(out, "   %d) %-17s %s\n", i+1, localModeLabel(mode), localModeStatus(s, mode))
		}
		fmt.Fprintln(out, "   q) Quit")
		fmt.Fprint(out, "  Select 1-4: ")
		if !scanner.Scan() {
			fmt.Fprintln(out)
			return "", scanner.Err()
		}
		choice := strings.TrimSpace(scanner.Text())
		switch strings.ToLower(choice) {
		case "q", "quit", "exit", "0":
			return "", nil
		case "":
			continue
		}
		if mode := normalizeLocalMode(choice); mode != "" {
			return mode, nil
		}
		fmt.Fprintln(out, "  Please type 1, 2, 3, 4 or q.")
	}
}

func localChatConfig(dataDir string, settings model.MessageSettings, mode string) (Config, string, error) {
	cfg := DefaultConfig(dataDir, "local-test", "rod", true)
	cfg.ChatMode = mode
	cfg.MessageUsernames = settings.UsernameLines()
	note := ""

	switch mode {
	case model.ChatModeAPI:
		loaded, err := LoadAPIChatConfig(dataDir, settings)
		if err != nil {
			return cfg, "", fmt.Errorf("API Chat is not ready: %w\n     open the dashboard -> 3 - Message -> API Chat, fill the base URL / key, press Save, then run test.bat again", err)
		}
		cfg.APIChat = loaded
		// API Chat is model-driven end to end: TXT sequences must not drive it.
		cfg.MessageSequence = nil
		cfg.AvailableSequences = nil
	case model.ChatModeEVAChat:
		root, err := EnsureEVAChatData(dataDir)
		if err != nil {
			return cfg, "", err
		}
		engine, err := LoadEVAChatEngine(root)
		if err != nil {
			return cfg, "", fmt.Errorf("load EVAChat Flow data: %w", err)
		}
		cfg.EVAChat = &EVAChatConfig{DataRoot: root, Engine: engine}
		cfg.MessageSequence = nil
		cfg.AvailableSequences = nil
	case model.ChatModeFolder:
		for _, raw := range settings.LoadedSequences {
			if lines := model.MessageSequenceLines(raw); len(lines) > 0 {
				cfg.AvailableSequences = append(cfg.AvailableSequences, lines)
			}
		}
		if len(cfg.AvailableSequences) == 0 {
			cfg.AvailableSequences = [][]string{settings.SequenceLines()}
			note = "no folder is loaded; using the Single Sequence, exactly like the real bot's fallback"
		}
	default:
		cfg.AvailableSequences = [][]string{settings.SequenceLines()}
	}

	if mode == model.ChatModeSingle || mode == model.ChatModeFolder {
		if len(cfg.AvailableSequences) == 0 || len(cfg.AvailableSequences[0]) == 0 {
			return cfg, "", fmt.Errorf("no message sequence is configured: open the dashboard -> 3 - Message, write the sequence (one line = one message), press Save")
		}
	}
	return cfg, note, nil
}

type localChatRunner struct {
	cfg     Config
	mode    string
	rng     *rand.Rand
	state   model.AccountState
	userID  string
	seq     []string
	sent    int
	ended   bool
	scanner *bufio.Scanner
	out     io.Writer
}

func (r *localChatRunner) run(ctx context.Context) error {
	r.printWelcome()
	r.startConversation(ctx)

	for {
		fmt.Fprint(r.out, "\nyou> ")
		if !r.scanner.Scan() {
			fmt.Fprintln(r.out)
			return r.scanner.Err()
		}
		line := strings.TrimSpace(r.scanner.Text())
		if line == "" {
			continue
		}
		switch strings.ToLower(line) {
		case "/quit", "/exit", "/q", "quit", "exit":
			fmt.Fprintln(r.out, "bye.")
			return nil
		case "/help", "/h", "help", "?":
			r.printHelp()
			continue
		case "/state", "/status":
			r.printState()
			continue
		case "/new", "/restart":
			fmt.Fprintln(r.out, "\n--- new conversation ---")
			r.startConversation(ctx)
			continue
		}
		r.reply(ctx, line)
	}
}

func (r *localChatRunner) printWelcome() {
	fmt.Fprintf(r.out, "\n  Mode: %s\n", localModeLabel(r.mode))
	if len(r.cfg.MessageUsernames) == 0 {
		fmt.Fprintln(r.out, "  Snap usernames: none in the dashboard, so share lines are skipped (same as the real bot).")
	} else {
		fmt.Fprintf(r.out, "  Snap usernames: %d configured, one is assigned per chat.\n", len(r.cfg.MessageUsernames))
	}
	if apiChatEnabled(r.cfg) {
		fmt.Fprintf(r.out, "  Model: %s\n", r.cfg.APIChat.Engine.Model())
	}
	fmt.Fprintln(r.out, "  Type like a stranger and press Enter. /help for commands.")
}

func (r *localChatRunner) printHelp() {
	fmt.Fprintln(r.out, "\n  /new    start a fresh conversation (new persona for API Chat)")
	fmt.Fprintln(r.out, "  /state  show what the bot remembers about you")
	fmt.Fprintln(r.out, "  /help   this list")
	fmt.Fprintln(r.out, "  /quit   exit the local test")
}

func (r *localChatRunner) startConversation(ctx context.Context) {
	r.state = model.AccountState{}
	r.seq = nil
	r.sent = 0
	r.ended = false

	if evaChatEnabled(r.cfg) || apiChatEnabled(r.cfg) {
		r.typing()
		result, tag, ok := statefulChatStart(ctx, r.cfg, &r.state, r.userID, r.rng)
		r.clearTyping()
		if !ok {
			fmt.Fprintln(r.out, "  (this mode is not ready)")
			return
		}
		r.printResult(tag, result)
		return
	}

	r.seq = getActiveSequence(r.cfg, &r.state, r.userID, 0, r.rng)
	if len(r.seq) == 0 {
		fmt.Fprintln(r.out, "  (no sequence configured)")
		return
	}
	r.sendSequenceLine()
}

func (r *localChatRunner) reply(ctx context.Context, text string) {
	if r.ended {
		fmt.Fprintln(r.out, "  (this chat has ended - type /new for a fresh one)")
		return
	}

	if evaChatEnabled(r.cfg) || apiChatEnabled(r.cfg) {
		r.typing()
		result, tag, ok := statefulChatReply(ctx, r.cfg, &r.state, r.userID, text, r.rng)
		r.clearTyping()
		if !ok {
			fmt.Fprintln(r.out, "  (this mode is not ready)")
			return
		}
		if strings.TrimSpace(result.Text) == "" && len(result.Extra) == 0 {
			reason := strings.TrimSpace(result.Reason)
			if reason == "" {
				reason = "no reply"
			}
			fmt.Fprintf(r.out, "  (bot had no line: %s)\n", reason)
			if result.Ended {
				r.ended = true
			}
			return
		}
		r.printResult(tag, result)
		return
	}

	r.sendSequenceLine()
}

func (r *localChatRunner) sendSequenceLine() {
	if r.sent >= len(r.seq) {
		fmt.Fprintln(r.out, "  (message sequence finished - /new to restart the chat)")
		r.ended = true
		return
	}
	text, _ := renderMsg(r.cfg, &r.state, r.userID, r.sent, r.rng, r.seq[r.sent])
	r.sent++
	text = strings.TrimSpace(text)
	if text == "" {
		fmt.Fprintln(r.out, "  (template rendered empty - the real bot ends the chat here)")
		r.ended = true
		return
	}
	fmt.Fprintf(r.out, "bot> %s\n", text)
	fmt.Fprintf(r.out, "     [%s] step %d/%d\n", localModeLabel(r.mode), r.sent, len(r.seq))
}

func (r *localChatRunner) printResult(tag string, result EVAReplyResult) {
	lines := make([]string, 0, 1+len(result.Extra))
	if text := strings.TrimSpace(result.Text); text != "" {
		lines = append(lines, text)
	}
	for _, extra := range result.Extra {
		if text := strings.TrimSpace(extra); text != "" {
			lines = append(lines, text)
		}
	}
	if len(lines) == 0 {
		reason := strings.TrimSpace(result.Reason)
		if reason == "" {
			reason = "no line"
		}
		fmt.Fprintf(r.out, "  (bot had no line: %s)\n", reason)
	} else {
		for i, line := range lines {
			if i == 0 {
				fmt.Fprintf(r.out, "bot> %s\n", line)
			} else {
				fmt.Fprintf(r.out, "     %s\n", line)
			}
		}
	}
	if reason := strings.TrimSpace(result.Reason); reason != "" {
		fmt.Fprintf(r.out, "     [%s] %s\n", tag, compactLocalText(reason, 170))
	}
	if result.Ended {
		r.ended = true
		fmt.Fprintln(r.out, "     (flow ended - type /new for a fresh chat)")
	}
}

func (r *localChatRunner) printState() {
	fmt.Fprintf(r.out, "\n  mode: %s\n", localModeLabel(r.mode))
	if assigned := r.state.AssignedUsernames[r.userID]; assigned != "" {
		fmt.Fprintf(r.out, "  snap username assigned to you: %s\n", assigned)
	} else {
		fmt.Fprintln(r.out, "  snap username: not assigned yet")
	}

	switch {
	case apiChatEnabled(r.cfg):
		session, ok := r.state.APIChatSessions[r.userID]
		if !ok {
			fmt.Fprintln(r.out, "  api chat: not started")
			return
		}
		d := session.Director
		facts, dontAsk := d.Known()
		fmt.Fprintf(r.out, "  api chat: reply=%d tier=%s score=%d style=%s\n", session.Turn, d.Tier, d.Score, d.Style)
		fmt.Fprintf(r.out, "  asked snap=%v shared=%v snap dodges=%d ended=%v\n", d.AskedSnap, d.Shared, d.SnapDodges, d.Ended)
		if facts == "" {
			fmt.Fprintln(r.out, "  known facts: (none yet)")
		} else {
			fmt.Fprintf(r.out, "  known facts: %s\n", facts)
		}
		if len(dontAsk) > 0 {
			fmt.Fprintf(r.out, "  never ask again: %s\n", strings.Join(dontAsk, ", "))
		}
	case evaChatEnabled(r.cfg):
		session, ok := r.state.EVAChatSessions[r.userID]
		if !ok {
			fmt.Fprintln(r.out, "  evachat: not started")
			return
		}
		fmt.Fprintf(r.out, "  evachat: stage=%s bot replies=%d asked snap=%v ended=%v\n", session.Stage, session.BotReplyCount, session.AskedSnap, session.Ended)
		fmt.Fprintf(r.out, "  user: age=%d gender=%s country=%s\n", session.UserAge, session.UserGender, session.UserCountry)
	default:
		fmt.Fprintf(r.out, "  sequence: step %d/%d\n", r.sent, len(r.seq))
	}
}

func (r *localChatRunner) typing() {
	fmt.Fprint(r.out, localTyping)
}

func (r *localChatRunner) clearTyping() {
	fmt.Fprint(r.out, "\r"+strings.Repeat(" ", len(localTyping)+2)+"\r")
}

// compactLocalText keeps console meta lines readable.
func compactLocalText(text string, limit int) string {
	text = strings.Join(strings.Fields(text), " ")
	if len(text) <= limit {
		return text
	}
	return text[:limit] + "..."
}

// LocalChatTurn represents one bot turn in an in-dashboard local test session.
type LocalChatTurn struct {
	Lines       []string      `json:"lines"`
	Tag         string        `json:"tag"`
	Reason      string        `json:"reason"`
	Ended       bool          `json:"ended"`
	Note        string        `json:"note,omitempty"`
	RuleWhy     string        `json:"rule_why,omitempty"`
	Mood        string        `json:"mood,omitempty"`
	Instruction string        `json:"instruction,omitempty"`
	UserKind    string        `json:"user_kind,omitempty"`
	RawOutput   string        `json:"raw_output,omitempty"`
	Diagnostic  string        `json:"diagnostic,omitempty"`
	State       LocalChatInfo `json:"state"`
}

// LocalChatInfo describes the bot's current memory and roleplay state.
type LocalChatInfo struct {
	Mode             string   `json:"mode"`
	ModeLabel        string   `json:"mode_label"`
	Model            string   `json:"model,omitempty"`
	AssignedUsername string   `json:"assigned_username,omitempty"`
	Step             int      `json:"step,omitempty"`
	TotalSteps       int      `json:"total_steps,omitempty"`
	Turn             int      `json:"turn,omitempty"`
	Stage            string   `json:"stage,omitempty"`
	Tier             string   `json:"tier,omitempty"`
	Score            int      `json:"score,omitempty"`
	Style            string   `json:"style,omitempty"`
	AskedSnap        bool     `json:"asked_snap"`
	Shared           bool     `json:"shared"`
	SnapDodges       int      `json:"snap_dodges,omitempty"`
	Ended            bool     `json:"ended"`
	KnownFacts       string   `json:"known_facts,omitempty"`
	DontAsk          []string `json:"dont_ask,omitempty"`
	PersonaName      string   `json:"persona_name,omitempty"`
	PersonaAge       int      `json:"persona_age,omitempty"`
	PersonaCountry   string   `json:"persona_country,omitempty"`
	RenderedPersona  string   `json:"rendered_persona,omitempty"`
}

// LocalChatSession runs an interactive local test conversation in memory for
// the dashboard without opening a browser or touching any account files.
type LocalChatSession struct {
	runner *localChatRunner
	note   string
}

// NewLocalChatSession prepares a local test session for one of the four
// dashboard Message modes.
func NewLocalChatSession(dataDir string, settings model.MessageSettings, rawMode string) (*LocalChatSession, error) {
	if strings.TrimSpace(dataDir) == "" {
		return nil, fmt.Errorf("data folder is not set")
	}
	mode := normalizeLocalMode(rawMode)
	if mode == "" {
		mode = normalizeLocalMode(settings.EffectiveChatMode())
	}
	if mode == "" {
		mode = model.ChatModeSingle
	}
	cfg, note, err := localChatConfig(dataDir, settings, mode)
	if err != nil {
		return nil, err
	}
	return &LocalChatSession{
		runner: &localChatRunner{
			cfg:    cfg,
			mode:   mode,
			rng:    rand.New(rand.NewSource(time.Now().UnixNano())),
			userID: "local-test-user",
		},
		note: note,
	}, nil
}

// Start resets the conversation and produces the bot's opening turn.
func (s *LocalChatSession) Start(ctx context.Context) LocalChatTurn {
	r := s.runner
	r.state = model.AccountState{}
	r.seq = nil
	r.sent = 0
	r.ended = false

	if evaChatEnabled(r.cfg) || apiChatEnabled(r.cfg) {
		result, tag, ok := statefulChatStart(ctx, r.cfg, &r.state, r.userID, r.rng)
		if !ok {
			return LocalChatTurn{
				Tag:    localModeLabel(r.mode),
				Reason: "this mode is not ready",
				Ended:  true,
				Note:   s.note,
				State:  s.Snapshot(),
			}
		}
		return s.turnFromResult(tag, result, s.note)
	}

	r.seq = getActiveSequence(r.cfg, &r.state, r.userID, 0, r.rng)
	if len(r.seq) == 0 {
		r.ended = true
		return LocalChatTurn{
			Tag:    localModeLabel(r.mode),
			Reason: "no sequence configured",
			Ended:  true,
			Note:   s.note,
			State:  s.Snapshot(),
		}
	}
	return s.nextSequenceTurn(s.note)
}

// Reply sends one stranger message to the bot and returns its reply turn.
func (s *LocalChatSession) Reply(ctx context.Context, userText string) LocalChatTurn {
	r := s.runner
	if r.ended {
		return LocalChatTurn{
			Tag:    localModeLabel(r.mode),
			Reason: "this chat has ended - press New Chat for a fresh one",
			Ended:  true,
			State:  s.Snapshot(),
		}
	}

	if evaChatEnabled(r.cfg) || apiChatEnabled(r.cfg) {
		result, tag, ok := statefulChatReply(ctx, r.cfg, &r.state, r.userID, userText, r.rng)
		if !ok {
			return LocalChatTurn{
				Tag:    localModeLabel(r.mode),
				Reason: "this mode is not ready",
				Ended:  r.ended,
				State:  s.Snapshot(),
			}
		}
		return s.turnFromResult(tag, result, "")
	}

	return s.nextSequenceTurn("")
}

func (s *LocalChatSession) nextSequenceTurn(note string) LocalChatTurn {
	r := s.runner
	if r.sent >= len(r.seq) {
		r.ended = true
		return LocalChatTurn{
			Tag:    localModeLabel(r.mode),
			Reason: "message sequence finished - press New Chat to restart",
			Ended:  true,
			Note:   note,
			State:  s.Snapshot(),
		}
	}
	text, _ := renderMsg(r.cfg, &r.state, r.userID, r.sent, r.rng, r.seq[r.sent])
	r.sent++
	text = strings.TrimSpace(text)
	if text == "" {
		r.ended = true
		return LocalChatTurn{
			Tag:    localModeLabel(r.mode),
			Reason: "template rendered empty - chat ended",
			Ended:  true,
			Note:   note,
			State:  s.Snapshot(),
		}
	}
	return LocalChatTurn{
		Lines:  []string{text},
		Tag:    localModeLabel(r.mode),
		Reason: fmt.Sprintf("step %d/%d", r.sent, len(r.seq)),
		Ended:  false,
		Note:   note,
		State:  s.Snapshot(),
	}
}

func (s *LocalChatSession) turnFromResult(tag string, result EVAReplyResult, note string) LocalChatTurn {
	r := s.runner
	lines := make([]string, 0, 1+len(result.Extra))
	if text := strings.TrimSpace(result.Text); text != "" {
		lines = append(lines, text)
	}
	for _, extra := range result.Extra {
		if text := strings.TrimSpace(extra); text != "" {
			lines = append(lines, text)
		}
	}
	if result.Ended {
		r.ended = true
	}
	return LocalChatTurn{
		Lines:       lines,
		Tag:         tag,
		Reason:      compactLocalText(strings.TrimSpace(result.Reason), 170),
		Ended:       r.ended,
		Note:        note,
		RuleWhy:     result.RuleWhy,
		Mood:        result.Mood,
		Instruction: result.Instruction,
		UserKind:    result.UserKind,
		RawOutput:   result.RawOutput,
		Diagnostic:  result.Diagnostic,
		State:       s.Snapshot(),
	}
}

// Snapshot returns the bot's current memory and roleplay state.
func (s *LocalChatSession) Snapshot() LocalChatInfo {
	r := s.runner
	info := LocalChatInfo{
		Mode:      r.mode,
		ModeLabel: localModeLabel(r.mode),
		Ended:     r.ended,
	}
	if r.state.AssignedUsernames != nil {
		info.AssignedUsername = r.state.AssignedUsernames[r.userID]
	}
	switch {
	case apiChatEnabled(r.cfg):
		info.Model = r.cfg.APIChat.Engine.Model()
		if session, ok := r.state.APIChatSessions[r.userID]; ok {
			d := session.Director
			facts, dontAsk := d.Known()
			info.Turn = session.Turn
			info.Tier = string(d.Tier)
			info.Score = d.Score
			info.Style = string(d.Style)
			info.AskedSnap = d.AskedSnap
			info.Shared = d.Shared
			info.SnapDodges = d.SnapDodges
			info.Ended = r.ended || d.Ended
			info.KnownFacts = facts
			info.DontAsk = dontAsk
			info.PersonaName = session.PersonaName
			info.PersonaAge = session.PersonaAge
			info.PersonaCountry = session.PersonaCountry
			info.RenderedPersona = r.cfg.APIChat.Engine.RenderSystemPrompt(&session)
		}
	case evaChatEnabled(r.cfg):
		if session, ok := r.state.EVAChatSessions[r.userID]; ok {
			info.Stage = session.Stage
			info.Turn = session.BotReplyCount
			info.AskedSnap = session.AskedSnap
			info.Ended = r.ended || session.Ended
			var parts []string
			if session.UserAge > 0 {
				parts = append(parts, fmt.Sprintf("age=%d", session.UserAge))
			}
			if session.UserGender != "" {
				parts = append(parts, "gender="+session.UserGender)
			}
			if session.UserCountry != "" {
				parts = append(parts, "country="+session.UserCountry)
			}
			info.KnownFacts = strings.Join(parts, ", ")
		}
	default:
		info.Step = r.sent
		info.TotalSteps = len(r.seq)
	}
	return info
}
