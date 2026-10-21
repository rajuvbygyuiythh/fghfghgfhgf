package main

// Show Mode live chat, DOM-only edition. After the user logs in manually in
// the visible Rod browser, chatting continues inside that same browser using
// only what a human sees: message rows are read from the DOM, replies are
// typed into the message box, and Skip/START are real button clicks. No
// WebSocket frames and no API calls are used (explicit user requirement).
// Normal Rod/ChromeDP login capture and the Start-Bot worker are untouched;
// this runs only when BrowserSettings.LiveChat is set (Show Mode).

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"path/filepath"
	"strings"
	"time"

	"chitchatbot/internal/apichat"
	"chitchatbot/internal/bot"
	"chitchatbot/internal/browser"
	"chitchatbot/internal/model"
)

// ---------------------------------------------------------------------------
// Reply responders (unchanged semantics). EVA mode reuses the exported
// EVAChat engine; Folder mode picks ONE sequence per conversation, like the
// worker bot; Single mode walks its lines.
// ---------------------------------------------------------------------------

// liveReply carries the text plus the rule that produced it, so the Show Mode
// live log can explain every outgoing line without guessing.
type liveReply struct {
	Lines  []string
	End    bool
	Rule   string
	Detail string
}

type liveResponder interface {
	Start() liveReply
	Reply(incoming string) liveReply
}

type detectOnlyResponder struct{}

func (detectOnlyResponder) Start() liveReply       { return liveReply{} }
func (detectOnlyResponder) Reply(string) liveReply { return liveReply{} }

type sequenceLiveResponder struct {
	sequences [][]string
	lines     []string
	next      int
	pick      int
	total     int
	rng       *rand.Rand
}

func (r *sequenceLiveResponder) Start() liveReply {
	usable := r.sequences[:0]
	for _, seq := range r.sequences {
		if len(seq) > 0 {
			usable = append(usable, seq)
		}
	}
	if len(usable) == 0 {
		return liveReply{}
	}
	pick := 0
	if len(usable) > 1 && r.rng != nil {
		pick = r.rng.Intn(len(usable))
	}
	r.lines = usable[pick]
	r.pick = pick
	r.total = len(usable)
	r.next = 1
	return liveReply{
		Lines:  []string{r.lines[0]},
		Rule:   fmt.Sprintf("sequence %d/%d · line 1/%d", pick+1, len(usable), len(r.lines)),
		Detail: "opener",
	}
}

func (r *sequenceLiveResponder) Reply(string) liveReply {
	if r.lines == nil {
		return r.Start()
	}
	if r.next < len(r.lines) {
		line := r.lines[r.next]
		r.next++
		return liveReply{
			Lines: []string{line},
			Rule:  fmt.Sprintf("sequence %d/%d · line %d/%d", r.pick+1, r.total, r.next, len(r.lines)),
		}
	}
	return liveReply{End: true, Rule: "sequence exhausted"}
}

// apiLiveResponder drives API Chat inside Show Mode. It uses the exact same
// internal/apichat engine as the background workers, so both surfaces share
// one persona, one director and one funnel.
type apiLiveResponder struct {
	engine   *apichat.Engine
	state    *apichat.Session
	rng      *rand.Rand
	username string
}

func (r *apiLiveResponder) Start() liveReply {
	return r.replyOf(r.engine.Start(context.Background(), r.state), "opener")
}

func (r *apiLiveResponder) Reply(incoming string) liveReply {
	return r.replyOf(r.engine.Reply(context.Background(), r.state, incoming), fmt.Sprintf("reply #%d", r.state.Turn))
}

func (r *apiLiveResponder) replyOf(result apichat.Result, phase string) liveReply {
	lines := make([]string, 0, len(result.Lines))
	for _, line := range result.Lines {
		if strings.Contains(line, "%username%") || strings.Contains(line, "{snap}") {
			if r.username == "" {
				continue
			}
			line = strings.NewReplacer("%username%", r.username, "{snap}", r.username).Replace(line)
		}
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
		}
	}
	detail := phase
	if result.Detail != "" {
		detail += " · " + result.Detail
	}
	if result.Shared {
		detail += " · snap shared"
	}
	return liveReply{
		Lines:  lines,
		End:    result.Ended,
		Rule:   "API · " + result.Rule,
		Detail: detail,
	}
}

type evaLiveResponder struct {
	engine   *bot.EVAChatEngine
	state    *model.EVAChatSession
	rng      *rand.Rand
	username string
}

func evaLines(result bot.EVAReplyResult) []string {
	lines := []string{}
	if strings.TrimSpace(result.Text) != "" {
		lines = append(lines, result.Text)
	}
	for _, extra := range result.Extra {
		if strings.TrimSpace(extra) != "" {
			lines = append(lines, extra)
		}
	}
	return lines
}

func (r *evaLiveResponder) Start() liveReply {
	return r.replyOf(r.engine.Start(r.state, r.rng), "opener")
}

func (r *evaLiveResponder) Reply(incoming string) liveReply {
	result := r.engine.ReplyWithOptions(incoming, r.state, r.rng, bot.EVAReplyOptions{HasUsernames: r.username != ""})
	return r.replyOf(result, fmt.Sprintf("reply #%d", r.state.BotReplyCount))
}

func (r *evaLiveResponder) replyOf(result bot.EVAReplyResult, phase string) liveReply {
	pool := result.SourcePool
	if pool == "" {
		pool = "none"
	}
	detail := phase + " · stage=" + r.state.Stage
	if result.Reason != "" {
		detail += " · " + result.Reason
	}
	if result.SnapShared {
		detail += " · snap shared"
	}
	return liveReply{
		Lines:  r.render(result),
		End:    result.Ended,
		Rule:   "EVA · " + pool,
		Detail: detail,
	}
}

func (r *evaLiveResponder) render(result bot.EVAReplyResult) []string {
	lines := evaLines(result)
	for i, line := range lines {
		lines[i] = strings.NewReplacer("%username%", r.username, "{snap}", r.username).Replace(line)
	}
	return lines
}

func liveSequences(settings model.MessageSettings) [][]string {
	if settings.EffectiveChatMode() == model.ChatModeFolder && len(settings.LoadedSequences) > 0 {
		sequences := make([][]string, 0, len(settings.LoadedSequences))
		for _, seq := range settings.LoadedSequences {
			sequences = append(sequences, model.MessageSequenceLines(seq))
		}
		return sequences
	}
	return [][]string{settings.SequenceLines()}
}

func (a *App) newLiveResponder() (liveResponder, string, error) {
	settings, err := a.effectiveMessageSettings(a.activeSiteID())
	if err != nil {
		return nil, "", err
	}
	mode := settings.EffectiveChatMode()
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	username := ""
	if handles := settings.UsernameLines(); len(handles) > 0 {
		username = handles[rng.Intn(len(handles))]
	}
	if mode == model.ChatModeEVAChat {
		dataRoot, err := bot.EnsureEVAChatData(a.activeSiteDataDir())
		if err != nil {
			return nil, "", err
		}
		engine, err := bot.LoadEVAChatEngine(dataRoot)
		if err != nil {
			return nil, "", err
		}
		session := bot.NewEVAChatSession()
		return &evaLiveResponder{
			engine:   engine,
			state:    &session,
			rng:      rng,
			username: username,
		}, "EVAChat Flow", nil
	}
	if mode == model.ChatModeAPI {
		config, err := bot.LoadAPIChatConfig(a.activeSiteDataDir(), settings)
		if err != nil {
			return nil, "", err
		}
		session := apichat.NewSession(rng)
		return &apiLiveResponder{
			engine:   config.Engine,
			state:    &session,
			rng:      rng,
			username: username,
		}, "API Chat", nil
	}
	sequences := liveSequences(settings)
	for i, seq := range sequences {
		for j, line := range seq {
			if username == "" && (strings.Contains(line, "%username%") || strings.Contains(line, "{snap}")) {
				return nil, "", fmt.Errorf("Show Mode sequence needs a Message → Usernames entry")
			}
			sequences[i][j] = strings.NewReplacer("%username%", username, "{snap}", username).Replace(line)
		}
	}
	return &sequenceLiveResponder{sequences: sequences, rng: rng}, mode, nil
}

// DOM runner: single-consumer queue over the observer's retained rows. Unknown
// authors are not guessed; uncertain sends pause until a different conversation.
type liveRunner struct {
	firstMessage             model.FirstMessageSettings
	firstWaiting             bool
	firstDeadline            time.Time
	dom                      browser.DOMChatBackend
	prefix                   string
	logf                     func(string, string)
	eventf                   func(kind, text, rule, detail string, settingOK bool)
	newResponder             func() (liveResponder, string, error)
	replyDelayMin            time.Duration
	replyDelayMax            time.Duration
	replyDelayRNG            *rand.Rand
	pollInterval             time.Duration
	conversation             string
	controls                 browser.DOMControls
	responder                liveResponder
	seenIDs                  map[string]bool
	queuedIDs                map[string]bool
	queue                    []browser.DOMMessage
	paused                   bool
	rotating                 bool
	rotateReason             string
	lastStartAt              time.Time
	lastRotateAt             time.Time
	lastStepError            string
	timeoutRotating          bool
	autoSkip                 bool
	noReplyMin, noReplyMax   time.Duration
	nextChatMin, nextChatMax time.Duration
	replyDeadline            time.Time
	lastNoReplyWait          time.Duration
	firstWaitDuration        time.Duration
	nextChatAt               time.Time
	previousPhase            string
	lastProtection           string
	probeLogged              bool
	ownNameWarned            bool
	banCheck                 func(context.Context) (browser.BanReport, bool)
	banned                   *browser.BanReport
	lastBanCheck             time.Time
}

func (a *App) runLiveChat(ctx context.Context, backend browser.Backend, _ model.LoginInfo, profileName string, index, total int, proxyLabel string) {
	dom, ok := backend.(browser.DOMChatBackend)
	if !ok {
		a.log("activity", "SHOW MODE ERROR | backend does not support DOM chat")
		a.showModeEvent("error", "backend does not support DOM chat", "", "", false)
		return
	}
	prefix := fmt.Sprintf("[%s show-live %d/%d]", profileName, index, total)
	showSettings, err := a.effectiveShowModeSettings(a.activeSiteID())
	if err != nil {
		a.log("activity", prefix+" SETTINGS FAILED | could not load Reply delay settings: "+err.Error())
		a.showModeEvent("error", "could not load Show Mode settings", "", err.Error(), false)
		return
	}
	replyDelayMin, replyDelayMax := showModeReplyDelayRange(showSettings)
	noReplyMin := time.Duration(showSettings.NoReplyMinSeconds) * time.Second
	noReplyMax := time.Duration(showSettings.NoReplyMaxSeconds) * time.Second
	nextChatMin := time.Duration(showSettings.NextChatMinSeconds) * time.Second
	nextChatMax := time.Duration(showSettings.NextChatMaxSeconds) * time.Second

	a.showModeEvent("system", "settings applied", "", fmt.Sprintf(
		"profile=%s · proxy=%s · reply delay=%s · auto-skip=%t (%s) · next chat=%s · first=%s",
		profileName,
		proxyLabel,
		showModeDurationRange(replyDelayMin, replyDelayMax),
		showSettings.AutoSkip,
		showModeDurationRange(noReplyMin, noReplyMax),
		showModeDurationRange(nextChatMin, nextChatMax),
		showModeFirstLabel(showSettings.FirstMessage.Effective()),
	), false)

	observed, stopTranscript := a.startHomeChatObserver(ctx, dom, profileName, index, total)
	defer stopTranscript()
	r := &liveRunner{
		firstMessage:  showSettings.FirstMessage.Effective(),
		dom:           observed,
		prefix:        prefix,
		logf:          a.log,
		eventf:        a.showModeEvent,
		newResponder:  a.newLiveResponder,
		replyDelayMin: replyDelayMin,
		replyDelayMax: replyDelayMax,
		replyDelayRNG: rand.New(rand.NewSource(time.Now().UnixNano())),
		autoSkip:      showSettings.AutoSkip,
		noReplyMin:    noReplyMin,
		noReplyMax:    noReplyMax,
		nextChatMin:   nextChatMin,
		nextChatMax:   nextChatMax,
		pollInterval:  350 * time.Millisecond,
	}
	if checker, ok := backend.(browser.BanChecker); ok {
		r.banCheck = func(checkCtx context.Context) (browser.BanReport, bool) {
			report, err := checker.CheckBan(checkCtx)
			return report, err == nil && report.Banned
		}
	}
	a.log("activity", fmt.Sprintf("%s DOM READY | browser-only chat watcher started; reply delay setting=%s-%s; close its browser window to stop", r.prefix, r.replyDelayMin, r.replyDelayMax))
	a.showModeEvent("system", "chat watcher started", "", "waiting for a conversation", false)
	r.run(ctx)
	if r.banned != nil {
		a.handleBanned(filepath.Join(a.activeSiteDataDir(), "profiles", profileName), profileName, *r.banned)
	}
	a.log("activity", r.prefix+" LIVE CHAT stopped")
	a.showModeEvent("system", "live chat stopped", "", "", false)
}

func showModeDurationRange(min, max time.Duration) string {
	secs := func(d time.Duration) string {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	if min == max {
		return secs(min)
	}
	return secs(min) + "–" + secs(max)
}

func showModeFirstLabel(s model.FirstMessageSettings) string {
	switch s.Mode {
	case "user":
		return "user"
	case "random":
		return fmt.Sprintf("random %d%%", s.BotFirstPercent)
	default:
		return "bot"
	}
}

func showModeReplyDelayRange(settings model.ShowModeSettings) (time.Duration, time.Duration) {
	minSeconds := settings.ReplyDelayMinSeconds
	maxSeconds := settings.ReplyDelayMaxSeconds
	if minSeconds < 0 {
		minSeconds = 0
	}
	if maxSeconds < 0 {
		maxSeconds = 0
	}
	if maxSeconds < minSeconds {
		maxSeconds = minSeconds
	}
	return time.Duration(minSeconds) * time.Second, time.Duration(maxSeconds) * time.Second
}

func chooseShowModeReplyDelay(rng *rand.Rand, minDelay, maxDelay time.Duration) time.Duration {
	if minDelay < 0 {
		minDelay = 0
	}
	if maxDelay < minDelay {
		maxDelay = minDelay
	}
	if rng == nil || maxDelay == minDelay {
		return minDelay
	}
	return minDelay + time.Duration(rng.Int63n(int64(maxDelay-minDelay)))
}

func (r *liveRunner) run(ctx context.Context) {
	ticker := time.NewTicker(r.pollInterval)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil || r.step(ctx) {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (r *liveRunner) reset(c browser.DOMControls) {
	r.firstWaiting = false
	r.firstDeadline = time.Time{}
	r.timeoutRotating = false
	r.replyDeadline = time.Time{}
	r.conversation = c.Conversation
	r.controls = c
	r.responder = nil
	r.seenIDs = make(map[string]bool)
	r.queuedIDs = make(map[string]bool)
	r.queue = nil
	r.paused = false
	r.rotating = false
	r.probeLogged = false
	r.lastStepError = ""
	r.lastRotateAt = time.Time{}
	r.lastStartAt = time.Time{}
}

func (r *liveRunner) step(ctx context.Context) bool {
	if r.banCheck != nil && time.Since(r.lastBanCheck) >= 2*time.Second {
		r.lastBanCheck = time.Now()
		if report, banned := r.banCheck(ctx); banned {
			r.banned = &report
			detail := report.Summary()
			r.logf("error", r.prefix+" BANNED | "+detail)
			r.emit("error", "BANNED — account suspended", "", detail, false)
			return true
		}
	}
	snapshot, err := r.dom.ReadDOMSnapshot(ctx)
	if err != nil {
		if r.pageGone(err) {
			return true
		}
		r.logStepError("DOM READ FAILED | " + err.Error())
		return false
	}
	c := snapshot.Controls
	if c.Protection != "" && c.Protection != "clean" && c.Protection != r.lastProtection {
		r.lastProtection = c.Protection
		r.logf("warn", r.prefix+" PROTECTION DETECTED | "+c.Protection)
		r.emit("system", "anti-bot protection detected: "+c.Protection, "protection_detector", "verify or solve in visible browser", false)
	} else if c.Protection == "clean" {
		r.lastProtection = "clean"
	}
	// Remember the chat-to-lobby transition even when the DOM generation changes.
	if c.Phase != "chatting" && r.previousPhase == "chatting" {
		r.nextChatAt = time.Now().Add(chooseShowModeReplyDelay(r.replyDelayRNG, r.nextChatMin, r.nextChatMax))
	}
	r.previousPhase = c.Phase
	if c.Conversation == "" {
		r.logStepError("DOM NOT READY | missing conversation identity")
		return false
	}
	if c.Conversation != r.conversation {
		r.reset(c)
	}
	r.controls = c
	if c.Overflow {
		r.paused = true
		r.logStepError("QUEUE FULL | paused safely; start a new chat")
		return false
	}
	if c.Phase != "chatting" {
		r.responder = nil
		r.queue = nil
		if !time.Now().Before(r.nextChatAt) && hasDOMButton(c.Buttons, "START") && time.Since(r.lastStartAt) > 6*time.Second {
			r.lastStartAt = time.Now()
			clicked, err := r.dom.ClickDOMButton(ctx, []string{"START"})
			if err != nil {
				if r.pageGone(err) {
					return true
				}
				r.logStepError("START FAILED | " + err.Error())
			} else if clicked != "" {
				r.logf("activity", r.prefix+" START clicked | waiting for a new conversation")
			}
		} else if !r.probeLogged {
			r.probeLogged = true
			if probe, err := r.dom.DOMProbe(ctx); err == nil {
				r.logf("activity", r.prefix+" PAGE PROBE | "+probe)
			}
		}
		return false
	}
	if r.rotating {
		return r.rotate(ctx)
	}
	if r.paused {
		return false
	}
	if !c.CanSend || c.Partner == "" || (c.OwnName != "" && strings.EqualFold(c.Partner, c.OwnName)) {
		if c.OwnName == "" && !r.ownNameWarned {
			r.ownNameWarned = true
			r.logf("activity", r.prefix+" IDENTITY | own profile control missing; matching partner rows only")
			r.emit("system", "identity fallback", "", "own profile control missing; matching partner rows only", false)
		}
		r.logStepError("DOM WAIT | composer or own/partner identity not ready; no guessed replies")
		return false
	}
	if time.Now().Before(r.nextChatAt) {
		return false
	}
	if r.responder == nil {
		// Seed own/unknown history only. An already-present partner message
		// wins over an unsolicited opener and is processed through Reply.
		for _, row := range snapshot.Messages {
			if row.ID != "" && !strings.EqualFold(row.Author, c.Partner) {
				r.seenIDs[row.ID] = true
			}
		}
		responder, mode, err := r.newResponder()
		if err != nil {
			r.paused = true
			r.logStepError("REPLY ENGINE FAILED | " + err.Error())
			return false
		}
		r.responder = responder
		r.logf("activity", fmt.Sprintf("%s CHAT STARTED | mode=%s history=%d observer=%t", r.prefix, mode, len(r.seenIDs), c.ObserverReady))
		r.emit("system", "chat started", "", fmt.Sprintf("mode=%s · partner=%s", mode, c.Partner), false)
		hasIncoming := false
		for _, row := range snapshot.Messages {
			if row.ID != "" && strings.EqualFold(row.Author, c.Partner) && strings.TrimSpace(row.Text) != "" {
				hasIncoming = true
				break
			}
		}
		botFirst := r.firstMessage.ChooseBotFirst(r.replyDelayRNG)
		firstBranch := "wait for the user"
		if botFirst {
			firstBranch = "bot opener"
		}
		r.emit("system", "first turn", "", fmt.Sprintf("policy=%s → %s", showModeFirstLabel(r.firstMessage), firstBranch), false)
		if botFirst && !hasIncoming {
			// Recheck just before opening: incoming may have arrived while loading
			// the responder. There is no atomic browser/server ordering guarantee.
			fresh, err := r.dom.ReadDOMSnapshot(ctx)
			if err != nil {
				r.paused = true
				r.logStepError("FIRST TURN RECHECK FAILED | " + err.Error())
				return r.pageGone(err)
			}
			if fresh.Controls.Conversation != r.conversation {
				return false
			}
			snapshot.Messages = fresh.Messages
			for _, row := range snapshot.Messages {
				if row.ID != "" && strings.EqualFold(row.Author, c.Partner) && strings.TrimSpace(row.Text) != "" {
					hasIncoming = true
					break
				}
			}
			if !hasIncoming {
				return r.sendLines(ctx, responder.Start(), 0, 0, 0, "opener")
			}
		}
		r.firstWaiting = true
		policy := r.firstMessage.Effective()
		if policy.WaitTimeout {
			r.firstWaitDuration = policy.WaitDuration(r.replyDelayRNG)
			r.firstDeadline = time.Now().Add(r.firstWaitDuration)
		}
		// Fall through: a partner already present must receive a reply, not a second opener.
	}
	for _, row := range snapshot.Messages {
		if row.ID == "" || r.seenIDs[row.ID] || r.queuedIDs[row.ID] || strings.TrimSpace(row.Text) == "" {
			continue
		}
		if strings.EqualFold(row.Author, c.OwnName) {
			r.seenIDs[row.ID] = true
			continue
		}
		if !strings.EqualFold(row.Author, c.Partner) {
			r.logStepError("AUTHOR UNKNOWN | unread row retained; no text-only echo guessing")
			continue
		}
		r.firstWaiting = false
		r.firstDeadline = time.Time{}
		r.replyDeadline = time.Time{}
		r.queue = append(r.queue, row)
		r.queuedIDs[row.ID] = true
		r.emit("in", row.Text, "", fmt.Sprintf("partner · pending=%d", len(r.queue)), false)
	}
	if len(r.queue) == 0 {
		if r.firstWaiting && !r.firstDeadline.IsZero() && !time.Now().Before(r.firstDeadline) {
			r.timeoutRotating = true
			r.rotating = true
			r.rotateReason = "first-message wait timeout"
			return r.rotate(ctx)
		}
		if r.autoSkip && !r.replyDeadline.IsZero() && !time.Now().Before(r.replyDeadline) {
			r.replyDeadline = time.Time{}
			r.timeoutRotating = true
			r.rotating = true
			r.rotateReason = "no-reply auto-skip"
			return r.rotate(ctx)
		}
		return false
	}
	m := r.queue[0]
	r.logf("activity", fmt.Sprintf("%s INCOMING | partner row accepted; pending=%d", r.prefix, len(r.queue)))
	replyDelay := chooseShowModeReplyDelay(r.replyDelayRNG, r.replyDelayMin, r.replyDelayMax)
	r.logf("activity", fmt.Sprintf("%s REPLY DELAY | selected=%s setting=%s-%s", r.prefix, replyDelay, r.replyDelayMin, r.replyDelayMax))
	if sleepLive(ctx, replyDelay) {
		return true
	}
	// Do not let a slow reply or an old batch leak into the next partner's chat.
	if !r.stillCurrent(ctx) {
		return ctx.Err() != nil
	}
	generation := r.conversation
	reply := r.responder.Reply(m.Text)
	r.seenIDs[m.ID] = true
	delete(r.queuedIDs, m.ID)
	r.queue = r.queue[1:]
	if r.sendLines(ctx, reply, replyDelay, r.replyDelayMin, r.replyDelayMax, "reply") {
		return true
	}
	if r.conversation != generation || r.responder == nil {
		return false
	}
	if reply.End && !r.paused {
		r.queue = nil
		r.rotating = true
		r.rotateReason = "engine ended"
		return r.rotate(ctx)
	}
	return false
}

func (r *liveRunner) stillCurrent(ctx context.Context) bool {
	s, err := r.dom.ReadDOMSnapshot(ctx)
	if err != nil {
		r.paused = true
		r.logStepError("DOM RECHECK FAILED | " + err.Error())
		return false
	}
	if s.Controls.Conversation != r.conversation || s.Controls.Phase != "chatting" {
		r.reset(s.Controls)
		return false
	}
	if !s.Controls.CanSend || s.Controls.Overflow {
		r.paused = true
		r.logStepError("SEND PAUSED | composer is not ready; inspect the page")
		return false
	}
	return true
}

func (r *liveRunner) sendLines(ctx context.Context, reply liveReply, delay, min, max time.Duration, phase string) bool {
	lines := reply.Lines
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if i > 0 && sleepLive(ctx, 400*time.Millisecond) {
			return true
		}
		if !r.stillCurrent(ctx) {
			return ctx.Err() != nil
		}
		sent, err := r.dom.SendDOMMessage(ctx, line)
		if err != nil && r.pageGone(err) {
			return true
		}
		if err != nil || !sent {
			r.paused = true
			detail := "automation paused; inspect the page before starting a new chat (no automatic resend)"
			if err != nil {
				detail = err.Error() + "; " + detail
			}
			r.logStepError("SEND UNCONFIRMED | " + detail)
			return false
		}
		ok := true
		detail := reply.Detail
		if phase == "opener" {
			if detail == "" {
				detail = "opener"
			}
			detail += " · no delay"
		} else {
			ok = delay >= min && delay <= max
			if detail != "" {
				detail += " · "
			}
			detail += fmt.Sprintf("delay %.1fs (setting %s)", delay.Seconds(), showModeDurationRange(min, max))
			if ok {
				detail += " ✓"
			} else {
				detail += " ✗"
			}
		}
		if len(lines) > 1 {
			detail += fmt.Sprintf(" · line %d/%d", i+1, len(lines))
		}
		r.emit("out", line, reply.Rule, detail, ok)
		if r.autoSkip {
			r.lastNoReplyWait = chooseShowModeReplyDelay(r.replyDelayRNG, r.noReplyMin, r.noReplyMax)
			r.replyDeadline = time.Now().Add(r.lastNoReplyWait)
		}
		r.logf("activity", r.prefix+" SEND CONFIRMED | new own message row observed")
	}
	return false
}

func (r *liveRunner) rotate(ctx context.Context) bool {
	if time.Since(r.lastRotateAt) < 2*time.Second {
		return false
	}
	r.lastRotateAt = time.Now()
	s, err := r.dom.ReadDOMSnapshot(ctx)
	if err != nil {
		if r.pageGone(err) {
			return true
		}
		r.logStepError("SKIP CHECK FAILED | " + err.Error())
		return false
	}
	if s.Controls.Conversation != r.conversation {
		r.reset(s.Controls)
		return false
	}
	// Recheck fresh rows before a timeout skip; a late incoming message wins.
	if r.timeoutRotating {
		for _, row := range s.Messages {
			if row.ID != "" && !r.seenIDs[row.ID] && strings.EqualFold(row.Author, s.Controls.Partner) && strings.TrimSpace(row.Text) != "" {
				r.firstWaiting = false
				r.firstDeadline = time.Time{}
				r.rotating = false
				r.timeoutRotating = false
				r.rotateReason = ""
				r.emit("system", "timeout cancelled", "", "a new partner message arrived first", false)
				return false
			}
		}
	}
	if hasDOMButton(s.Controls.Buttons, "START") && time.Now().Before(r.nextChatAt) {
		return false
	}
	// Exact active aria-label only; hidden START/STOP labels never participate.
	clicked, err := r.dom.ClickDOMButton(ctx, []string{"SKIP", "CONFIRM?", "START"})
	if err != nil {
		if r.pageGone(err) {
			return true
		}
		r.logStepError("SKIP FAILED | " + err.Error())
	} else if clicked == "" {
		r.logStepError("SKIP WAIT | active cycle button unavailable; use the page manually")
	} else {
		nextWait := chooseShowModeReplyDelay(r.replyDelayRNG, r.nextChatMin, r.nextChatMax)
		if r.nextChatAt.IsZero() || time.Now().After(r.nextChatAt) {
			r.nextChatAt = time.Now().Add(nextWait)
		}
		reason := r.rotateReason
		if reason == "" {
			reason = "skip"
		}
		detail := reason
		switch reason {
		case "no-reply auto-skip":
			detail = fmt.Sprintf("%s · waited %.1fs (setting %s) · next chat %s",
				reason, r.lastNoReplyWait.Seconds(),
				showModeDurationRange(r.noReplyMin, r.noReplyMax),
				showModeDurationRange(r.nextChatMin, r.nextChatMax))
		case "first-message wait timeout":
			detail = fmt.Sprintf("%s · waited %.1fs (setting %s) · next chat %s",
				reason, r.firstWaitDuration.Seconds(),
				showModeDurationRange(time.Duration(r.firstMessage.WaitMinSeconds)*time.Second, time.Duration(r.firstMessage.WaitMaxSeconds)*time.Second),
				showModeDurationRange(r.nextChatMin, r.nextChatMax))
		default:
			detail = fmt.Sprintf("%s · next chat %s", reason, showModeDurationRange(r.nextChatMin, r.nextChatMax))
		}
		r.emit("skip", "skipped conversation", "", detail, false)
		r.rotateReason = ""
		r.logf("activity", r.prefix+" SKIP | clicked active "+clicked+"; awaiting conversation change")
		r.lastStartAt = time.Time{}
	}
	// Do not greet the old page again just because click() returned successfully.
	return false
}

func (r *liveRunner) emit(kind, text, rule, detail string, settingOK bool) {
	if r.eventf != nil {
		r.eventf(kind, text, rule, detail, settingOK)
	}
}

func (r *liveRunner) pageGone(err error) bool {
	return errors.Is(err, browser.ErrPageUnavailable) || errors.Is(err, context.Canceled)
}
func (r *liveRunner) logStepError(message string) {
	if message == r.lastStepError {
		return
	}
	r.lastStepError = message
	r.logf("activity", r.prefix+" "+message)
	r.emit("error", message, "", "", false)
}
func hasDOMButton(buttons []string, label string) bool {
	for _, b := range buttons {
		if strings.EqualFold(strings.TrimSpace(b), label) {
			return true
		}
	}
	return false
}
func sleepLive(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return true
	case <-timer.C:
		return false
	}
}
