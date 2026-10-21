package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"time"

	"chitchatbot/internal/bot"
	"chitchatbot/internal/browser"
	"chitchatbot/internal/model"
	"chitchatbot/internal/proxy"
	"chitchatbot/internal/settings"
	"chitchatbot/internal/sites"
)

// Runner lifecycle states.
const (
	StatusIdle     = "idle"
	StatusStarting = "starting"
	StatusRunning  = "running"
	StatusPaused   = "paused"
	StatusStopping = "stopping"
	StatusStopped  = "stopped"
	StatusFailed   = "failed"
)

// Canonical runner error types (Prompt 5).
const (
	ErrorTypeConnection  = "connection"
	ErrorTypeAuthExpired = "auth expired"
	ErrorTypeBanned      = "banned"
	ErrorTypeCaptcha     = "captcha"
	ErrorTypeTimeout     = "timeout"
	ErrorTypeCrash       = "crash"
)

const (
	// DefaultMaxConsecutiveErrors is the threshold after which a site auto-pauses.
	DefaultMaxConsecutiveErrors = 5
	// DefaultCycleInterval is the default delay between runner cycles.
	DefaultCycleInterval = 2 * time.Second

	RunnerLogFileName      = "runner.jsonl"
	RunnerErrorLogFileName = "errors.jsonl"
)

// AllErrorTypes returns the 6 canonical runner error classifications.
func AllErrorTypes() []string {
	return []string{
		ErrorTypeConnection,
		ErrorTypeAuthExpired,
		ErrorTypeBanned,
		ErrorTypeCaptcha,
		ErrorTypeTimeout,
		ErrorTypeCrash,
	}
}

// RunnerError wraps a typed error from a site runner cycle.
type RunnerError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
	Cause   error  `json:"-"`
}

func (e *RunnerError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return fmt.Sprintf("%s: %s", e.Type, e.Message)
	}
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Type, e.Cause)
	}
	return e.Type
}

func (e *RunnerError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// NewRunnerError creates a RunnerError with an explicit error type.
func NewRunnerError(errType, message string, cause error) *RunnerError {
	return &RunnerError{
		Type:    NormalizeErrorType(errType),
		Message: strings.TrimSpace(message),
		Cause:   cause,
	}
}

// NormalizeErrorType normalizes an error type string into one of the 6 canonical types.
func NormalizeErrorType(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case ErrorTypeConnection, "conn", "network", "websocket", "ws":
		return ErrorTypeConnection
	case ErrorTypeAuthExpired, "auth_expired", "authexpired", "login_expired", "login expired", "unauthorized":
		return ErrorTypeAuthExpired
	case ErrorTypeBanned, "ban", "blocked":
		return ErrorTypeBanned
	case ErrorTypeCaptcha, "recaptcha", "hcaptcha", "turnstile":
		return ErrorTypeCaptcha
	case ErrorTypeTimeout, "timed_out", "deadline_exceeded":
		return ErrorTypeTimeout
	case ErrorTypeCrash, "panic", "fatal":
		return ErrorTypeCrash
	default:
		return ErrorTypeConnection
	}
}

// ClassifyRunnerError inspects an error and classifies it into one of the 6 canonical
// error types: connection, auth expired, banned, captcha, timeout, or crash.
func ClassifyRunnerError(err error) string {
	if err == nil {
		return ""
	}
	var re *RunnerError
	if errors.As(err, &re) && re.Type != "" {
		return NormalizeErrorType(re.Type)
	}
	var bannedErr *browser.BannedError
	if errors.As(err, &bannedErr) {
		return ErrorTypeBanned
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return ErrorTypeTimeout
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return ErrorTypeTimeout
	}

	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "panic") || strings.Contains(msg, "crash") || strings.Contains(msg, "runtime error:"):
		return ErrorTypeCrash
	case strings.Contains(msg, "banned") || strings.Contains(msg, "account suspended") || strings.Contains(msg, "access denied") || strings.Contains(msg, "ip blocked"):
		return ErrorTypeBanned
	case strings.Contains(msg, "captcha") || strings.Contains(msg, "recaptcha") || strings.Contains(msg, "hcaptcha") || strings.Contains(msg, "turnstile") || strings.Contains(msg, "cloudflare challenge"):
		return ErrorTypeCaptcha
	case strings.Contains(msg, "auth expired") || strings.Contains(msg, "auth_expired") || strings.Contains(msg, "login expired") || strings.Contains(msg, "token expired") || strings.Contains(msg, "unauthorized") || strings.Contains(msg, "http 401") || strings.Contains(msg, "status 401"):
		return ErrorTypeAuthExpired
	case strings.Contains(msg, "timeout") || strings.Contains(msg, "timed out") || strings.Contains(msg, "deadline exceeded"):
		return ErrorTypeTimeout
	default:
		return ErrorTypeConnection
	}
}

// Notification represents a UI notification event emitted by the Supervisor
// (e.g. when a site auto-pauses after 5 consecutive errors or crashes).
type Notification struct {
	SiteID            string `json:"site_id"`
	SiteName          string `json:"site_name"`
	Level             string `json:"level"` // "info", "warn", "error"
	Event             string `json:"event"` // "started", "stopped", "paused", "auto_paused", "failed", "error", "restarted"
	Reason            string `json:"reason"`
	ErrorType         string `json:"error_type,omitempty"`
	ConsecutiveErrors int    `json:"consecutive_errors,omitempty"`
	Timestamp         string `json:"timestamp"`
}

// SiteStats holds per-site runtime counters.
type SiteStats struct {
	CyclesCompleted  int    `json:"cycles_completed"`
	MessagesSent     int    `json:"messages_sent"`
	MessagesReceived int    `json:"messages_received"`
	Conversations    int    `json:"conversations"`
	LastReplyRule    string `json:"last_reply_rule,omitempty"`
	LastMessageSent  string `json:"last_message_sent,omitempty"`
	LastActivityAt   string `json:"last_activity_at,omitempty"`
}

// SiteStatus reports the current status, error state, and stats of one isolated site runner.
type SiteStatus struct {
	SiteID            string         `json:"site_id"`
	SiteName          string         `json:"site_name"`
	Group             string         `json:"group"`
	GroupName         string         `json:"group_name"`
	Status            string         `json:"status"` // "idle", "starting", "running", "paused", "stopping", "stopped", "failed"
	Running           bool           `json:"running"`
	Paused            bool           `json:"paused"`
	AutoPaused        bool           `json:"auto_paused"`
	Failed            bool           `json:"failed"`
	PauseReason       string         `json:"pause_reason,omitempty"`
	ChatMode          string         `json:"chat_mode"`
	Profile           string         `json:"profile"`
	FolderPath        string         `json:"folder_path"`
	SessionPath       string         `json:"session_path,omitempty"`
	LogPath           string         `json:"log_path"`
	ErrorLogPath      string         `json:"error_log_path"`
	ConsecutiveErrors int            `json:"consecutive_errors"`
	TotalErrors       int            `json:"total_errors"`
	ErrorCounts       map[string]int `json:"error_counts"`
	LastError         string         `json:"last_error,omitempty"`
	LastErrorType     string         `json:"last_error_type,omitempty"`
	LastErrorAt       string         `json:"last_error_at,omitempty"`
	StartedAt         string         `json:"started_at,omitempty"`
	StoppedAt         string         `json:"stopped_at,omitempty"`
	RestartCount      int            `json:"restart_count"`
	Stats             SiteStats      `json:"stats"`
}

// CycleContext is passed to CycleFunc on each iteration of a site's runner goroutine.
type CycleContext struct {
	SiteID       string
	Profile      string
	CycleNumber  int
	SiteCtx      sites.SiteContext
	SiteCfg      sites.Config
	EffectiveCfg settings.Effective
	BotCfg       bot.Config
	ChatSession  *bot.LocalChatSession
	Browser      browser.Backend
	Login        model.LoginInfo
}

// CycleResult reports the work performed in a single cycle.
type CycleResult struct {
	MessagesSent     int
	MessagesReceived int
	Conversations    int
	LastReplyRule    string
	LastMessageSent  string
}

// CycleFunc executes one cycle of work for a site inside its isolated goroutine.
// Returning an error increments ConsecutiveErrors; panicking is caught by the
// supervisor recovery barrier and marks ONLY that site as "failed".
type CycleFunc func(ctx context.Context, cc *CycleContext) (CycleResult, error)

// BrowserFactory creates a browser.Backend when a site uses DOM/browser execution.
type BrowserFactory func(backendName string) (browser.Backend, error)

// Options configures a Supervisor instance.
type Options struct {
	DataDir              string
	SettingsStore        *settings.Store
	MaxConsecutiveErrors int
	CycleInterval        time.Duration
	DefaultProfile       string
	DefaultBackend       string
	UseLiveBrowser       bool
	BrowserFactory       BrowserFactory
	CycleFunc            CycleFunc
	NotifyFunc           func(Notification)
}

type siteRunner struct {
	mu          sync.Mutex
	siteID      string
	siteCtx     sites.SiteContext
	siteCfg     sites.Config
	status      SiteStatus
	cancel      context.CancelFunc
	done        chan struct{}
	browserConn browser.Backend
	chatSession *bot.LocalChatSession
}

func (sr *siteRunner) snapshot() SiteStatus {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	cp := sr.status
	if sr.status.ErrorCounts != nil {
		cp.ErrorCounts = make(map[string]int, len(sr.status.ErrorCounts))
		for k, v := range sr.status.ErrorCounts {
			cp.ErrorCounts[k] = v
		}
	}
	return cp
}

// Supervisor manages isolated per-site runner goroutines.
//
// Absolute guarantee: each site runs in its own goroutine with its own context
// and its own recover() barrier. A crash or panic in one site only marks that
// site as "failed" and logs to that site's logs/ directory — all other running
// sites continue unaffected.
type Supervisor struct {
	mu                   sync.RWMutex
	dataDir              string
	settingsStore        *settings.Store
	maxConsecutiveErrors int
	cycleInterval        time.Duration
	defaultProfile       string
	defaultBackend       string
	useLiveBrowser       bool
	browserFactory       BrowserFactory
	cycleFunc            CycleFunc
	notifyFunc           func(Notification)
	runners              map[string]*siteRunner
}

// NewSupervisor creates a new isolated multi-site runner Supervisor.
func NewSupervisor(opts Options) *Supervisor {
	maxErr := opts.MaxConsecutiveErrors
	if maxErr <= 0 {
		maxErr = DefaultMaxConsecutiveErrors
	}
	interval := opts.CycleInterval
	if interval <= 0 {
		interval = DefaultCycleInterval
	}
	profile := strings.TrimSpace(opts.DefaultProfile)
	if profile == "" {
		profile = "default"
	}
	backend := strings.TrimSpace(opts.DefaultBackend)
	if backend == "" {
		backend = "rod"
	}
	bf := opts.BrowserFactory
	if bf == nil {
		bf = browser.New
	}
	store := opts.SettingsStore
	if store == nil && strings.TrimSpace(opts.DataDir) != "" {
		store, _ = settings.Open(opts.DataDir)
	}
	return &Supervisor{
		dataDir:              filepath.Clean(strings.TrimSpace(opts.DataDir)),
		settingsStore:        store,
		maxConsecutiveErrors: maxErr,
		cycleInterval:        interval,
		defaultProfile:       profile,
		defaultBackend:       backend,
		useLiveBrowser:       opts.UseLiveBrowser,
		browserFactory:       bf,
		cycleFunc:            opts.CycleFunc,
		notifyFunc:           opts.NotifyFunc,
		runners:              make(map[string]*siteRunner),
	}
}

func (s *Supervisor) getOrCreateRunner(siteID string) (*siteRunner, error) {
	id := strings.TrimSpace(siteID)
	s.mu.RLock()
	if existing, ok := s.runners[id]; ok {
		s.mu.RUnlock()
		return existing, nil
	}
	s.mu.RUnlock()

	siteCtx, err := sites.NewSiteContext(s.dataDir, siteID)
	if err != nil {
		return nil, err
	}
	if err := siteCtx.Ensure(); err != nil {
		return nil, err
	}
	siteCfg, err := sites.LoadSite(s.dataDir, siteCtx.SiteID)
	if err != nil {
		siteCfg = sites.Config{
			ID:          siteCtx.SiteID,
			Name:        siteCtx.SiteID,
			Family:      "family_a",
			AccountMode: sites.AccountModeAutoRegister,
			Enabled:     true,
			StartURL:    "https://" + strings.ReplaceAll(siteCtx.SiteID, "_", ".") + "/",
			AppURL:      "https://" + strings.ReplaceAll(siteCtx.SiteID, "_", ".") + "/",
			AuthMode:    "dom_only",
			Selectors:   sites.ArchetypeSelectors("family_a"),
		}
		siteCfg.Classification = sites.InitialClassification(siteCfg)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if existing, ok := s.runners[siteCtx.SiteID]; ok {
		return existing, nil
	}

	logPath, _ := siteCtx.Resolve(filepath.Join("logs", RunnerLogFileName))
	errLogPath, _ := siteCtx.Resolve(filepath.Join("logs", RunnerErrorLogFileName))

	sr := &siteRunner{
		siteID:  siteCtx.SiteID,
		siteCtx: siteCtx,
		siteCfg: siteCfg,
		status: SiteStatus{
			SiteID:       siteCtx.SiteID,
			SiteName:     siteCfg.Name,
			Group:        siteCfg.Classification.Group,
			GroupName:    siteCfg.Classification.GroupName,
			Status:       StatusIdle,
			Profile:      s.defaultProfile,
			FolderPath:   siteCtx.FolderPath,
			LogPath:      logPath,
			ErrorLogPath: errLogPath,
			ErrorCounts:  make(map[string]int),
		},
	}
	s.runners[siteCtx.SiteID] = sr
	return sr, nil
}

// StartSite launches an isolated goroutine with its own context for siteID.
func (s *Supervisor) StartSite(siteID string) error {
	sr, err := s.getOrCreateRunner(siteID)
	if err != nil {
		return err
	}

	sr.mu.Lock()
	if sr.status.Running {
		sr.mu.Unlock()
		return nil
	}
	oldCancel := sr.cancel
	oldDone := sr.done
	sr.mu.Unlock()

	if oldCancel != nil {
		oldCancel()
	}
	if oldDone != nil {
		<-oldDone
	}

	sr.mu.Lock()
	if sr.status.Running {
		sr.mu.Unlock()
		return nil
	}

	eff, chatSession, botCfg, login, sessionPath, prepErr := s.prepareSiteExecution(sr.siteCtx, sr.siteCfg, sr.status.Profile)
	if prepErr != nil {
		sr.mu.Unlock()
		return prepErr
	}

	runCtx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	now := time.Now().UTC().Format(time.RFC3339Nano)

	sr.cancel = cancel
	sr.done = done
	sr.chatSession = chatSession
	sr.status.Status = StatusRunning
	sr.status.Running = true
	sr.status.Paused = false
	sr.status.AutoPaused = false
	sr.status.Failed = false
	sr.status.PauseReason = ""
	sr.status.ChatMode = botCfg.ChatMode
	sr.status.SessionPath = sessionPath
	sr.status.ConsecutiveErrors = 0
	sr.status.StartedAt = now
	sr.status.StoppedAt = ""
	siteName := sr.status.SiteName
	sr.mu.Unlock()

	appendSiteRunnerLog(sr.siteCtx, "runner_started", map[string]any{
		"site_id":      sr.siteID,
		"chat_mode":    botCfg.ChatMode,
		"profile":      botCfg.Profile,
		"session_path": sessionPath,
		"folder_path":  sr.siteCtx.FolderPath,
	})
	s.notify(Notification{
		SiteID:    sr.siteID,
		SiteName:  siteName,
		Level:     "info",
		Event:     "started",
		Reason:    fmt.Sprintf("Started isolated runner for %s (mode=%s)", sr.siteID, botCfg.ChatMode),
		Timestamp: now,
	})

	go s.runSiteLoop(runCtx, done, sr, eff, botCfg, login)
	return nil
}

// StopSite cleanly stops the isolated runner goroutine for siteID.
func (s *Supervisor) StopSite(siteID string) error {
	id := strings.TrimSpace(siteID)
	s.mu.RLock()
	sr, ok := s.runners[id]
	s.mu.RUnlock()
	if !ok {
		// Validate ID so callers get a clear error for invalid IDs.
		if _, err := sites.NewSiteContext(s.dataDir, id); err != nil {
			return err
		}
		return nil
	}

	sr.mu.Lock()
	cancel := sr.cancel
	done := sr.done
	wasRunning := sr.status.Running
	if wasRunning {
		sr.status.Status = StatusStopping
	}
	sr.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}

	sr.mu.Lock()
	// Preserve "paused" or "failed" if stop was triggered by PauseSite or panic recovery.
	if sr.status.Status != StatusPaused && sr.status.Status != StatusFailed {
		sr.status.Status = StatusStopped
	}
	sr.status.Running = false
	sr.status.StoppedAt = time.Now().UTC().Format(time.RFC3339Nano)
	siteName := sr.status.SiteName
	finalStatus := sr.status.Status
	sr.mu.Unlock()

	if wasRunning && finalStatus == StatusStopped {
		appendSiteRunnerLog(sr.siteCtx, "runner_stopped", map[string]any{
			"site_id": sr.siteID,
			"status":  finalStatus,
		})
		s.notify(Notification{
			SiteID:    sr.siteID,
			SiteName:  siteName,
			Level:     "info",
			Event:     "stopped",
			Reason:    fmt.Sprintf("Stopped runner for %s", sr.siteID),
			Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		})
	}
	return nil
}

// StartSelected starts multiple isolated site runners in parallel.
// Any error starting one site does not prevent the other selected sites from starting.
func (s *Supervisor) StartSelected(siteIDs []string) error {
	errs := s.StartSelectedResults(siteIDs)
	if len(errs) == 0 {
		return nil
	}
	msgs := make([]string, 0, len(errs))
	for id, err := range errs {
		msgs = append(msgs, fmt.Sprintf("%s: %v", id, err))
	}
	sort.Strings(msgs)
	return errors.New(strings.Join(msgs, "; "))
}

// StartSelectedResults starts each site in siteIDs and returns a map of any startup errors.
func (s *Supervisor) StartSelectedResults(siteIDs []string) map[string]error {
	errs := make(map[string]error)
	for _, rawID := range siteIDs {
		id := strings.TrimSpace(rawID)
		if id == "" {
			continue
		}
		if err := s.StartSite(id); err != nil {
			errs[id] = err
		}
	}
	return errs
}

// StopAll cleanly stops all currently running site goroutines.
func (s *Supervisor) StopAll() error {
	s.mu.RLock()
	ids := make([]string, 0, len(s.runners))
	for id := range s.runners {
		ids = append(ids, id)
	}
	s.mu.RUnlock()

	var wg sync.WaitGroup
	for _, id := range ids {
		siteID := id
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = s.StopSite(siteID)
		}()
	}
	wg.Wait()
	return nil
}

// PauseSite pauses a site runner with a reason (callable directly or by auto-pause
// when consecutive errors reach the threshold).
func (s *Supervisor) PauseSite(siteID, reason string) error {
	return s.pauseSiteInternal(siteID, reason, false)
}

func (s *Supervisor) pauseSiteInternal(siteID, reason string, autoPaused bool) error {
	sr, err := s.getOrCreateRunner(siteID)
	if err != nil {
		return err
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "Paused by supervisor"
	}

	sr.mu.Lock()
	cancel := sr.cancel
	siteName := sr.status.SiteName
	consec := sr.status.ConsecutiveErrors
	lastErrType := sr.status.LastErrorType
	sr.mu.Unlock()

	event := "runner_paused"
	notifyEvent := "paused"
	level := "warn"
	if autoPaused {
		event = "runner_auto_paused"
		notifyEvent = "auto_paused"
		level = "error"
	}
	appendSiteRunnerLog(sr.siteCtx, event, map[string]any{
		"site_id":            sr.siteID,
		"reason":             reason,
		"auto_paused":        autoPaused,
		"consecutive_errors": consec,
		"last_error_type":    lastErrType,
	})
	s.notify(Notification{
		SiteID:            sr.siteID,
		SiteName:          siteName,
		Level:             level,
		Event:             notifyEvent,
		Reason:            reason,
		ErrorType:         lastErrType,
		ConsecutiveErrors: consec,
		Timestamp:         time.Now().UTC().Format(time.RFC3339Nano),
	})

	sr.mu.Lock()
	sr.status.Status = StatusPaused
	sr.status.Running = false
	sr.status.Paused = true
	sr.status.AutoPaused = autoPaused
	sr.status.PauseReason = reason
	sr.status.StoppedAt = time.Now().UTC().Format(time.RFC3339Nano)
	sr.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	return nil
}

// RestartSite cleanly stops the site runner (if active) and starts a fresh
// isolated goroutine for it.
func (s *Supervisor) RestartSite(siteID string) error {
	if err := s.StopSite(siteID); err != nil {
		return err
	}
	sr, err := s.getOrCreateRunner(siteID)
	if err != nil {
		return err
	}
	sr.mu.Lock()
	sr.status.RestartCount++
	restarts := sr.status.RestartCount
	siteName := sr.status.SiteName
	sr.mu.Unlock()

	appendSiteRunnerLog(sr.siteCtx, "runner_restarting", map[string]any{
		"site_id":       sr.siteID,
		"restart_count": restarts,
	})
	if err := s.StartSite(siteID); err != nil {
		return err
	}
	s.notify(Notification{
		SiteID:    sr.siteID,
		SiteName:  siteName,
		Level:     "info",
		Event:     "restarted",
		Reason:    fmt.Sprintf("Restarted runner for %s (restart #%d)", sr.siteID, restarts),
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
	})
	return nil
}

// GetStatus returns the current status and statistics for one site.
func (s *Supervisor) GetStatus(siteID string) (SiteStatus, error) {
	sr, err := s.getOrCreateRunner(siteID)
	if err != nil {
		return SiteStatus{}, err
	}
	return sr.snapshot(), nil
}

// GetAllStatuses returns a map of siteID -> SiteStatus for all tracked sites.
func (s *Supervisor) GetAllStatuses() map[string]SiteStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]SiteStatus, len(s.runners))
	for id, sr := range s.runners {
		out[id] = sr.snapshot()
	}
	return out
}

func (s *Supervisor) runSiteLoop(
	ctx context.Context,
	done chan struct{},
	sr *siteRunner,
	eff settings.Effective,
	botCfg bot.Config,
	login model.LoginInfo,
) {
	defer close(done)
	defer func() {
		// Clean up any per-site browser connection on exit.
		sr.mu.Lock()
		b := sr.browserConn
		sr.browserConn = nil
		sr.mu.Unlock()
		if b != nil {
			_ = b.Close()
		}
	}()

	// Absolute panic/crash isolation barrier:
	// If this site panics anywhere inside its runner loop, ONLY this site is marked
	// "failed" and logged to its own logs/ folder. No other site goroutine is affected.
	defer func() {
		if r := recover(); r != nil {
			stack := string(debug.Stack())
			crashMsg := fmt.Sprintf("panic recovered in site %s runner: %v", sr.siteID, r)
			now := time.Now().UTC().Format(time.RFC3339Nano)

			sr.mu.Lock()
			consec := sr.status.ConsecutiveErrors + 1
			siteName := sr.status.SiteName
			sr.mu.Unlock()

			appendSiteErrorLog(sr.siteCtx, ErrorTypeCrash, crashMsg, stack, consec)
			appendSiteRunnerLog(sr.siteCtx, "runner_crashed", map[string]any{
				"site_id":            sr.siteID,
				"error_type":         ErrorTypeCrash,
				"error":              crashMsg,
				"consecutive_errors": consec,
			})
			s.notify(Notification{
				SiteID:            sr.siteID,
				SiteName:          siteName,
				Level:             "error",
				Event:             "failed",
				Reason:            crashMsg,
				ErrorType:         ErrorTypeCrash,
				ConsecutiveErrors: consec,
				Timestamp:         now,
			})

			sr.mu.Lock()
			sr.status.ConsecutiveErrors = consec
			sr.status.TotalErrors++
			if sr.status.ErrorCounts == nil {
				sr.status.ErrorCounts = make(map[string]int)
			}
			sr.status.ErrorCounts[ErrorTypeCrash]++
			sr.status.LastError = crashMsg
			sr.status.LastErrorType = ErrorTypeCrash
			sr.status.LastErrorAt = now
			sr.status.Status = StatusFailed
			sr.status.Running = false
			sr.status.Failed = true
			sr.status.StoppedAt = now
			sr.mu.Unlock()
		}
	}()

	cycleNum := 0
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		cycleNum++
		sr.mu.Lock()
		bConn := sr.browserConn
		sr.mu.Unlock()
		cc := &CycleContext{
			SiteID:       sr.siteID,
			Profile:      botCfg.Profile,
			CycleNumber:  cycleNum,
			SiteCtx:      sr.siteCtx,
			SiteCfg:      sr.siteCfg,
			EffectiveCfg: eff,
			BotCfg:       botCfg,
			ChatSession:  sr.chatSession,
			Browser:      bConn,
			Login:        login,
		}

		cycleRes, cycleErr := s.executeOneCycle(ctx, sr, cc)
		if cycleErr != nil {
			if errors.Is(cycleErr, context.Canceled) && ctx.Err() != nil {
				return
			}
			s.handleCycleError(sr, cycleErr)
			sr.mu.Lock()
			stillRunning := sr.status.Running
			sr.mu.Unlock()
			if !stillRunning {
				return
			}
		} else {
			now := time.Now().UTC().Format(time.RFC3339Nano)
			sr.mu.Lock()
			sr.status.ConsecutiveErrors = 0
			sr.status.Stats.CyclesCompleted++
			sr.status.Stats.MessagesSent += cycleRes.MessagesSent
			sr.status.Stats.MessagesReceived += cycleRes.MessagesReceived
			sr.status.Stats.Conversations += cycleRes.Conversations
			if cycleRes.LastReplyRule != "" {
				sr.status.Stats.LastReplyRule = cycleRes.LastReplyRule
			}
			if cycleRes.LastMessageSent != "" {
				sr.status.Stats.LastMessageSent = cycleRes.LastMessageSent
			}
			sr.status.Stats.LastActivityAt = now
			sr.mu.Unlock()

			appendSiteRunnerLog(sr.siteCtx, "runner_cycle_ok", map[string]any{
				"cycle":             cycleNum,
				"messages_sent":     cycleRes.MessagesSent,
				"messages_received": cycleRes.MessagesReceived,
				"last_reply_rule":   cycleRes.LastReplyRule,
			})
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(s.cycleInterval):
		}
	}
}

func (s *Supervisor) executeOneCycle(ctx context.Context, sr *siteRunner, cc *CycleContext) (CycleResult, error) {
	if s.cycleFunc != nil {
		return s.cycleFunc(ctx, cc)
	}

	// If live browser execution is enabled for this supervisor, connect via
	// internal/browser using this site's isolated profile directory.
	if s.useLiveBrowser {
		return s.executeBrowserDOMCycle(ctx, sr, cc)
	}

	// Default isolated cycle using internal/bot's chat logic (Sequence, Folder, EVAChat, or API Chat)
	// and the site's isolated session in sites/<site_id>/.
	if cc.ChatSession == nil {
		return CycleResult{}, NewRunnerError(ErrorTypeConnection, "chat session not initialized", nil)
	}

	var turn bot.LocalChatTurn
	if cc.CycleNumber == 1 {
		turn = cc.ChatSession.Start(ctx)
	} else {
		turn = cc.ChatSession.Reply(ctx, "hey how are you doing?")
	}

	sentCount := len(turn.Lines)
	lastSent := ""
	if sentCount > 0 {
		lastSent = turn.Lines[sentCount-1]
	}
	convs := 0
	if cc.CycleNumber == 1 {
		convs = 1
	}
	return CycleResult{
		MessagesSent:     sentCount,
		MessagesReceived: 1,
		Conversations:    convs,
		LastReplyRule:    turn.Reason,
		LastMessageSent:  lastSent,
	}, nil
}

func (s *Supervisor) executeBrowserDOMCycle(ctx context.Context, sr *siteRunner, cc *CycleContext) (CycleResult, error) {
	sr.mu.Lock()
	b := sr.browserConn
	sr.mu.Unlock()

	if b == nil {
		created, err := s.browserFactory(cc.BotCfg.Backend)
		if err != nil {
			return CycleResult{}, NewRunnerError(ErrorTypeConnection, "create browser backend", err)
		}
		userDataDir, err := cc.SiteCtx.BrowserProfileDir(cc.Profile)
		if err != nil {
			_ = created.Close()
			return CycleResult{}, NewRunnerError(ErrorTypeConnection, "resolve isolated browser profile dir", err)
		}
		if err := os.MkdirAll(userDataDir, 0o700); err != nil {
			_ = created.Close()
			return CycleResult{}, NewRunnerError(ErrorTypeConnection, "create isolated browser profile dir", err)
		}
		if domCfg, ok := created.(browser.SiteDOMConfigurer); ok {
			domCfg.ConfigureSiteDOM(
				sites.CustomDOMDriverJS(s.dataDir, cc.SiteID),
				cc.SiteCfg.Selectors.ComposerInput,
			)
		}
		var selectedProxy *proxy.Entry
		if cc.BotCfg.ProxyRotator != nil {
			if nextProxy, ok := cc.BotCfg.ProxyRotator.Next(); ok {
				selectedProxy = &nextProxy
			}
		}
		if seeder, ok := created.(browser.SessionSeeder); ok && cc.Login.Token() != "" {
			_ = seeder.SeedSession(ctx, cc.Login)
		}
		if err := created.Start(ctx, browser.Options{
			StartURL:    cc.SiteCfg.StartURL,
			UserDataDir: userDataDir,
			Headless:    cc.BotCfg.Headless,
			Proxy:       selectedProxy,
		}); err != nil {
			_ = created.Close()
			return CycleResult{}, NewRunnerError(ClassifyRunnerError(err), err.Error(), err)
		}
		sr.mu.Lock()
		sr.browserConn = created
		b = created
		sr.mu.Unlock()
	}

	if checker, ok := b.(browser.BanChecker); ok {
		if report, banErr := checker.CheckBan(ctx); banErr == nil && report.Banned {
			return CycleResult{}, &browser.BannedError{Report: report}
		}
	}

	domBackend, ok := b.(browser.DOMChatBackend)
	if !ok {
		return CycleResult{}, nil
	}
	snap, err := domBackend.ReadDOMSnapshot(ctx)
	if err != nil {
		return CycleResult{}, NewRunnerError(ClassifyRunnerError(err), err.Error(), err)
	}
	if strings.Contains(strings.ToLower(snap.Controls.Protection), "captcha") {
		return CycleResult{}, NewRunnerError(ErrorTypeCaptcha, "CAPTCHA detected in DOM: "+snap.Controls.Protection, nil)
	}
	if strings.Contains(strings.ToLower(snap.Controls.Protection), "ban") {
		return CycleResult{}, NewRunnerError(ErrorTypeBanned, "ban detected in DOM: "+snap.Controls.Protection, nil)
	}

	if snap.Controls.ChatEnded && len(snap.Controls.Buttons) > 0 {
		_, _ = domBackend.ClickDOMButton(ctx, []string{"START", "SKIP"})
		return CycleResult{Conversations: 1}, nil
	}

	if snap.Controls.CanSend && cc.ChatSession != nil && len(snap.Messages) > 0 {
		lastMsg := snap.Messages[len(snap.Messages)-1]
		if lastMsg.Author != "self" && strings.TrimSpace(lastMsg.Text) != "" {
			turn := cc.ChatSession.Reply(ctx, lastMsg.Text)
			sent := 0
			lastText := ""
			for _, line := range turn.Lines {
				if okSend, sendErr := domBackend.SendDOMMessage(ctx, line); sendErr == nil && okSend {
					sent++
					lastText = line
				}
			}
			return CycleResult{
				MessagesSent:     sent,
				MessagesReceived: 1,
				LastReplyRule:    turn.Reason,
				LastMessageSent:  lastText,
			}, nil
		}
	}
	return CycleResult{}, nil
}

func (s *Supervisor) handleCycleError(sr *siteRunner, err error) {
	errType := ClassifyRunnerError(err)
	errMsg := err.Error()
	now := time.Now().UTC().Format(time.RFC3339Nano)

	sr.mu.Lock()
	sr.status.ConsecutiveErrors++
	sr.status.TotalErrors++
	if sr.status.ErrorCounts == nil {
		sr.status.ErrorCounts = make(map[string]int)
	}
	sr.status.ErrorCounts[errType]++
	sr.status.LastError = errMsg
	sr.status.LastErrorType = errType
	sr.status.LastErrorAt = now
	consec := sr.status.ConsecutiveErrors
	siteName := sr.status.SiteName
	sr.mu.Unlock()

	appendSiteErrorLog(sr.siteCtx, errType, errMsg, "", consec)
	appendSiteRunnerLog(sr.siteCtx, "runner_error", map[string]any{
		"site_id":            sr.siteID,
		"error_type":         errType,
		"error":              errMsg,
		"consecutive_errors": consec,
	})

	s.notify(Notification{
		SiteID:            sr.siteID,
		SiteName:          siteName,
		Level:             "warn",
		Event:             "error",
		Reason:            errMsg,
		ErrorType:         errType,
		ConsecutiveErrors: consec,
		Timestamp:         now,
	})

	if consec >= s.maxConsecutiveErrors {
		pauseReason := fmt.Sprintf(
			"Auto-paused after %d consecutive errors (last [%s]: %s)",
			consec,
			errType,
			errMsg,
		)
		_ = s.pauseSiteInternal(sr.siteID, pauseReason, true)
	}
}

// prepareSiteExecution resolves the site's isolated session, effective global+override
// settings, and initializes the internal/bot chat session (Sequence, Folder, EVAChat, or API Chat)
// rooted strictly inside sites/<site_id>/.
func (s *Supervisor) prepareSiteExecution(
	siteCtx sites.SiteContext,
	siteCfg sites.Config,
	profile string,
) (settings.Effective, *bot.LocalChatSession, bot.Config, model.LoginInfo, string, error) {
	if strings.TrimSpace(profile) == "" {
		profile = s.defaultProfile
	}

	var eff settings.Effective
	if s.settingsStore != nil {
		if loadedEff, err := s.settingsStore.Effective(siteCtx.SiteID); err == nil {
			eff = loadedEff
		} else {
			eff = settings.Merge(settings.Default(), settings.Override{})
		}
	} else {
		eff = settings.Merge(settings.Default(), settings.Override{})
	}

	// Load or initialize the site's isolated session inside sites/<site_id>/sessions/.
	login, sessionPath, hasSession := sites.LoadSiteSession(s.dataDir, siteCtx.SiteID, profile)
	if !hasSession {
		if sites.NormalizeAccountMode(siteCfg.AccountMode, siteCfg.ID, siteCfg.AuthMode) == sites.AccountModeAutoRegister {
			_, _ = sites.AutoRegisterSiteAccounts(context.Background(), s.dataDir, siteCtx.SiteID, profile, 1, false)
			login, sessionPath, _ = sites.LoadSiteSession(s.dataDir, siteCtx.SiteID, profile)
		} else {
			// Create an isolated placeholder session inside sites/<site_id>/sessions/ so
			// the runner never looks outside sites/<site_id>/.
			placeholder := model.LoginInfo{
				UserID:  siteCtx.SiteID + "_" + profile,
				Profile: profile,
				Backend: s.defaultBackend,
			}
			sessionPath, _ = sites.SaveSiteSession(s.dataDir, siteCtx.SiteID, profile, placeholder)
			login = placeholder
		}
	}

	msgSettings := eff.ApplyMessageSettings(model.DefaultMessageSettings())
	genSettings := eff.ApplyGeneralSettings(model.DefaultGeneralSettings())
	proxySettings := eff.ApplyProxySettings(model.DefaultProxySettings())

	botCfg := bot.DefaultConfig(siteCtx.FolderPath, profile, s.defaultBackend, true)
	if strings.TrimSpace(siteCfg.AppURL) != "" {
		botCfg.AppURL = siteCfg.AppURL
	}
	if strings.TrimSpace(siteCfg.APIBaseURL) != "" {
		botCfg.APIBaseURL = siteCfg.APIBaseURL
	}
	botCfg.LoginInfoPath = sessionPath
	botCfg.ChatMode = msgSettings.EffectiveChatMode()
	botCfg.MessageSequence = msgSettings.SequenceLines()
	botCfg.AvailableSequences = [][]string{msgSettings.SequenceLines()}
	botCfg.MessageUsernames = msgSettings.UsernameLines()
	botCfg.FirstMessage = genSettings.FirstMessage.Effective()
	botCfg.ReplyDelayMin = time.Duration(genSettings.ReplyDelayMinSeconds) * time.Second
	botCfg.ReplyDelayMax = time.Duration(genSettings.ReplyDelayMaxSeconds) * time.Second

	if proxySettings.Enabled && strings.TrimSpace(proxySettings.Entries) != "" {
		if entries, err := proxy.ParseList(proxySettings.Entries); err == nil && len(entries) > 0 {
			botCfg.ProxyRotator = proxy.NewRotator(entries)
		}
	}

	// Build the internal/bot chat session (Sequence, Folder Sequences, EVAChat, or API Chat)
	// rooted inside this site's isolated FolderPath (sites/<site_id>/).
	chatSession, err := bot.NewLocalChatSession(siteCtx.FolderPath, msgSettings, botCfg.ChatMode)
	if err != nil {
		// Fall back to single sequence mode if Folder/API mode lacks external assets in a fresh test dir.
		fallbackMsg := model.DefaultMessageSettings()
		chatSession, err = bot.NewLocalChatSession(siteCtx.FolderPath, fallbackMsg, model.ChatModeSingle)
		if err != nil {
			return eff, nil, botCfg, login, sessionPath, err
		}
	}

	return eff, chatSession, botCfg, login, sessionPath, nil
}

func (s *Supervisor) notify(n Notification) {
	if s.notifyFunc != nil {
		s.notifyFunc(n)
	}
}

func appendSiteRunnerLog(siteCtx sites.SiteContext, event string, fields map[string]any) {
	if fields == nil {
		fields = map[string]any{}
	}
	entry := make(map[string]any, len(fields)+3)
	for k, v := range fields {
		entry[k] = v
	}
	entry["ts"] = time.Now().UTC().Format(time.RFC3339Nano)
	entry["site_id"] = siteCtx.SiteID
	entry["event"] = event

	_ = siteCtx.EnsureDir("logs")
	if raw, err := json.Marshal(entry); err == nil {
		raw = append(raw, '\n')
		if f, openErr := siteCtx.OpenFile(filepath.Join("logs", RunnerLogFileName), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600); openErr == nil {
			_, _ = f.Write(raw)
			_ = f.Close()
		}
	}
	detail, _ := json.Marshal(fields)
	_ = sites.AppendSiteDebugLog(siteCtx.DataDir, siteCtx.SiteID, event, "info", event, string(detail))
}

func appendSiteErrorLog(siteCtx sites.SiteContext, errType, errMsg, stack string, consecutive int) {
	entry := map[string]any{
		"ts":                 time.Now().UTC().Format(time.RFC3339Nano),
		"site_id":            siteCtx.SiteID,
		"error_type":         errType,
		"error":              errMsg,
		"consecutive_errors": consecutive,
	}
	if strings.TrimSpace(stack) != "" {
		entry["stack"] = stack
	}

	_ = siteCtx.EnsureDir("logs")
	if raw, err := json.Marshal(entry); err == nil {
		raw = append(raw, '\n')
		if f, openErr := siteCtx.OpenFile(filepath.Join("logs", RunnerErrorLogFileName), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600); openErr == nil {
			_, _ = f.Write(raw)
			_ = f.Close()
		}
	}
	_ = sites.AppendSiteDebugLog(siteCtx.DataDir, siteCtx.SiteID, "runner_error_"+errType, "error", errMsg, stack)
}

// ReadSiteErrorLogs reads all JSONL entries from sites/<site_id>/logs/errors.jsonl.
func ReadSiteErrorLogs(dataDir, siteID string) ([]map[string]any, error) {
	siteCtx, err := sites.NewSiteContext(dataDir, siteID)
	if err != nil {
		return nil, err
	}
	raw, err := siteCtx.ReadFile(filepath.Join("logs", RunnerErrorLogFileName))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	out := make([]map[string]any, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var item map[string]any
		if json.Unmarshal([]byte(line), &item) == nil {
			out = append(out, item)
		}
	}
	return out, nil
}
