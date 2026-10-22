package main

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"chitchatbot/internal/applog"
	"chitchatbot/internal/bot"
	"chitchatbot/internal/model"
	"chitchatbot/internal/runtimeassets"
	"chitchatbot/internal/settings"
	"chitchatbot/internal/sites"
	"chitchatbot/internal/uistore"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/logger"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:ui
var embeddedUI embed.FS

// StudioState is the initial and refreshed state for the Persona Studio & Test Chat dashboard.
type StudioState struct {
	DataDir  string                `json:"data_dir"`
	Settings model.MessageSettings `json:"settings"`
	Deck     bot.APIChatDeck       `json:"deck"`
	Chat     bot.LocalChatInfo     `json:"chat"`
}

// StudioSaveRequest carries both the API settings and the editable TXT decks.
type StudioSaveRequest struct {
	Settings model.MessageSettings `json:"settings"`
	Deck     bot.APIChatDeck       `json:"deck"`
}

// StudioChatStartRequest starts a new local test conversation using the latest editor state.
type StudioChatStartRequest struct {
	Mode     string                `json:"mode"`
	Settings model.MessageSettings `json:"settings"`
	Deck     bot.APIChatDeck       `json:"deck"`
}

// StudioApp powers the standalone apichat.exe Persona Studio & Live Test Chat window.
type StudioApp struct {
	ctx     context.Context
	dataDir string
	mu      sync.Mutex
	session *bot.LocalChatSession
}

func NewStudioApp(dataDir string) *StudioApp {
	return &StudioApp{dataDir: dataDir}
}

func (a *StudioApp) startup(ctx context.Context) {
	a.ctx = ctx
}

func (a *StudioApp) shutdown(_ context.Context) {}

func syncGlobalToStudioSettings(dataDir string, msg model.MessageSettings) model.MessageSettings {
	if strings.TrimSpace(dataDir) == "" {
		return msg
	}
	st, err := settings.Open(dataDir)
	if err != nil {
		return msg
	}
	global, err := st.Load()
	if err != nil {
		return msg
	}
	if strings.TrimSpace(global.API.BaseURL) != "" {
		msg.AIBaseURL = strings.TrimSpace(global.API.BaseURL)
	}
	if strings.TrimSpace(global.API.APIKey) != "" {
		msg.AIAPIKey = strings.TrimSpace(global.API.APIKey)
		msg.AIEnabled = true
	}
	if strings.TrimSpace(global.API.Model) != "" {
		msg.AIModel = strings.TrimSpace(global.API.Model)
	}
	if global.API.Temperature > 0 {
		msg.APITemperature = global.API.Temperature
	}
	return model.NormalizeAPISettings(msg)
}

func syncStudioSettingsToGlobal(dataDir string, msg model.MessageSettings) {
	if strings.TrimSpace(dataDir) == "" {
		return
	}
	st, err := settings.Open(dataDir)
	if err != nil {
		return
	}
	global, err := st.Load()
	if err != nil {
		global = settings.Default()
	}
	if mode := settings.NormalizeMessageMode(msg.EffectiveChatMode()); mode != "" {
		global.MessageMode = mode
	}
	if key := strings.TrimSpace(msg.AIAPIKey); key != "" {
		global.API.APIKey = key
	}
	if base := strings.TrimSpace(msg.AIBaseURL); base != "" {
		global.API.BaseURL = base
	}
	if modelName := strings.TrimSpace(msg.AIModel); modelName != "" {
		global.API.Model = modelName
	}
	if msg.APITemperature > 0 {
		global.API.Temperature = msg.APITemperature
	}
	global.API.Enabled = msg.AIEnabled || strings.TrimSpace(global.API.APIKey) != ""
	_, _ = st.Save(global)
}

// GetStudioState loads the shared Message settings (synced with sites/_global/settings.json)
// and data/api_chat TXT files.
func (a *StudioApp) GetStudioState() (StudioState, error) {
	msgSettings, err := uistore.LoadSharedMessageSettings(a.dataDir)
	if err != nil {
		msgSettings = model.NormalizeAPISettings(model.DefaultMessageSettings())
	}
	msgSettings = syncGlobalToStudioSettings(a.dataDir, msgSettings)
	deck, err := bot.LoadAPIChatDeck(a.dataDir)
	if err != nil {
		return StudioState{}, err
	}
	var chatInfo bot.LocalChatInfo
	a.mu.Lock()
	if a.session != nil {
		chatInfo = a.session.Snapshot()
	}
	a.mu.Unlock()
	return StudioState{
		DataDir:  a.dataDir,
		Settings: msgSettings,
		Deck:     deck,
		Chat:     chatInfo,
	}, nil
}

// SaveStudioState persists the persona/share/fallback/countries TXT files and
// syncs the API settings with both the main dashboard and sites/_global/settings.json (Prompt 15).
func (a *StudioApp) SaveStudioState(req StudioSaveRequest) (StudioState, error) {
	deck, err := bot.SaveAPIChatDeck(a.dataDir, req.Deck)
	if err != nil {
		return StudioState{}, err
	}
	if strings.TrimSpace(req.Settings.ChatMode) == "" {
		req.Settings.ChatMode = model.ChatModeAPI
	}
	savedSettings, err := uistore.SaveSharedMessageSettings(a.dataDir, req.Settings)
	if err != nil {
		return StudioState{}, err
	}
	syncStudioSettingsToGlobal(a.dataDir, savedSettings)
	var chatInfo bot.LocalChatInfo
	a.mu.Lock()
	if a.session != nil {
		chatInfo = a.session.Snapshot()
	}
	a.mu.Unlock()
	return StudioState{
		DataDir:  a.dataDir,
		Settings: savedSettings,
		Deck:     deck,
		Chat:     chatInfo,
	}, nil
}

// ResetPersonaDeck restores the starter persona.txt, sharesnap.txt, fallback.txt, and persona_countries.txt.
func (a *StudioApp) ResetPersonaDeck() (bot.APIChatDeck, error) {
	return bot.ResetAPIChatDeck(a.dataDir)
}

// OpenAPIChatFolder opens data/api_chat in Windows Explorer.
func (a *StudioApp) OpenAPIChatFolder() (string, error) {
	root, err := bot.EnsureAPIChatData(a.dataDir)
	if err != nil {
		return "", err
	}
	if err := exec.Command("explorer.exe", root).Start(); err != nil {
		return "", err
	}
	return fmt.Sprintf("Opened %s", root), nil
}

// TestAPIConnection saves the current studio settings/persona and sends one live test prompt.
func (a *StudioApp) TestAPIConnection(req StudioSaveRequest) (string, error) {
	state, err := a.SaveStudioState(req)
	if err != nil {
		return "", err
	}
	cfg, err := bot.LoadAPIChatConfig(a.dataDir, state.Settings)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	answer, err := cfg.Engine.Test(ctx)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("API Chat OK (%s / %s) -> %q", cfg.Engine.BaseURL(), cfg.Engine.Model(), answer), nil
}

// StartTestChat saves the current Persona + API settings and starts a fresh in-memory conversation.
func (a *StudioApp) StartTestChat(req StudioChatStartRequest) (bot.LocalChatTurn, error) {
	state, err := a.SaveStudioState(StudioSaveRequest{
		Settings: req.Settings,
		Deck:     req.Deck,
	})
	if err != nil {
		return bot.LocalChatTurn{}, err
	}
	mode := strings.TrimSpace(req.Mode)
	if mode == "" {
		mode = model.ChatModeAPI
	}
	sessionSettings := state.Settings
	if strings.TrimSpace(sessionSettings.Usernames) == "" {
		sessionSettings.Usernames = "your_snap"
	}
	session, err := bot.NewLocalChatSession(a.dataDir, sessionSettings, mode)
	if err != nil {
		return bot.LocalChatTurn{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	turn := session.Start(ctx)

	a.mu.Lock()
	a.session = session
	a.mu.Unlock()
	return turn, nil
}

// SendTestChatMessage sends one stranger message to the active test chat session.
func (a *StudioApp) SendTestChatMessage(message string) (bot.LocalChatTurn, error) {
	text := strings.TrimSpace(message)
	if text == "" {
		return bot.LocalChatTurn{}, errors.New("type a stranger message first")
	}
	a.mu.Lock()
	session := a.session
	a.mu.Unlock()

	if session == nil {
		settings, err := uistore.LoadSharedMessageSettings(a.dataDir)
		if err != nil {
			return bot.LocalChatTurn{}, err
		}
		if strings.TrimSpace(settings.Usernames) == "" {
			settings.Usernames = "your_snap"
		}
		session, err = bot.NewLocalChatSession(a.dataDir, settings, model.ChatModeAPI)
		if err != nil {
			return bot.LocalChatTurn{}, err
		}
		a.mu.Lock()
		a.session = session
		a.mu.Unlock()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	switch strings.ToLower(text) {
	case "/new", "/restart":
		return session.Start(ctx), nil
	case "/state", "/status":
		snap := session.Snapshot()
		return bot.LocalChatTurn{
			Tag:    snap.ModeLabel,
			Reason: "state snapshot",
			Ended:  snap.Ended,
			State:  snap,
		}, nil
	default:
		return session.Reply(ctx, text), nil
	}
}

// GetTestChatState returns the active test chat session snapshot.
func (a *StudioApp) GetTestChatState() (bot.LocalChatInfo, error) {
	a.mu.Lock()
	session := a.session
	a.mu.Unlock()
	if session == nil {
		return bot.LocalChatInfo{}, nil
	}
	return session.Snapshot(), nil
}

// HTTPHandler provides an HTTP JSON API fallback on /api/<Method> so the UI
// works both inside Wails and in any browser/test harness.
func (a *StudioApp) HTTPHandler() http.Handler {
	mux := http.NewServeMux()
	writeJSON := func(w http.ResponseWriter, val any, err error) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		_ = json.NewEncoder(w).Encode(val)
	}
	mux.HandleFunc("/api/GetStudioState", func(w http.ResponseWriter, _ *http.Request) {
		res, err := a.GetStudioState()
		writeJSON(w, res, err)
	})
	mux.HandleFunc("/api/SaveStudioState", func(w http.ResponseWriter, r *http.Request) {
		var req StudioSaveRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, nil, err)
			return
		}
		res, err := a.SaveStudioState(req)
		writeJSON(w, res, err)
	})
	mux.HandleFunc("/api/ResetPersonaDeck", func(w http.ResponseWriter, _ *http.Request) {
		res, err := a.ResetPersonaDeck()
		writeJSON(w, res, err)
	})
	mux.HandleFunc("/api/OpenAPIChatFolder", func(w http.ResponseWriter, _ *http.Request) {
		res, err := a.OpenAPIChatFolder()
		writeJSON(w, res, err)
	})
	mux.HandleFunc("/api/TestAPIConnection", func(w http.ResponseWriter, r *http.Request) {
		var req StudioSaveRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, nil, err)
			return
		}
		res, err := a.TestAPIConnection(req)
		writeJSON(w, res, err)
	})
	mux.HandleFunc("/api/StartTestChat", func(w http.ResponseWriter, r *http.Request) {
		var req StudioChatStartRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, nil, err)
			return
		}
		res, err := a.StartTestChat(req)
		writeJSON(w, res, err)
	})
	mux.HandleFunc("/api/SendTestChatMessage", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Message string `json:"message"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, nil, err)
			return
		}
		res, err := a.SendTestChatMessage(req.Message)
		writeJSON(w, res, err)
	})
	mux.HandleFunc("/api/GetTestChatState", func(w http.ResponseWriter, _ *http.Request) {
		res, err := a.GetTestChatState()
		writeJSON(w, res, err)
	})
	return mux
}

func main() {
	dataDir, err := resolveDataDir()
	if err != nil {
		log.Fatal(err)
	}

	if err := sites.EnsureGlobalDirs(dataDir); err != nil {
		log.Fatal(err)
	}

	diskLogger := applog.Logger{Path: applog.Path(dataDir), Source: "apichat"}
	log.SetOutput(io.MultiWriter(applog.Logger{Path: diskLogger.Path, Source: "apichat-go"}, os.Stderr))
	log.SetFlags(0)
	diskLogger.Info("APICHAT STUDIO START | diagnostic logging initialized")

	dist, err := fs.Sub(embeddedUI, "ui")
	if err != nil {
		log.Fatal(err)
	}

	app := NewStudioApp(dataDir)

	// Dedicated WebView2 user-data directory so apichat.exe and chitchatui.exe
	// can run side-by-side at the same time without 0x800700AA lock conflicts.
	webviewDir := filepath.Join(dataDir, "webview_apichat")
	if err := os.MkdirAll(webviewDir, 0o755); err != nil {
		log.Fatal(err)
	}

	err = wails.Run(&options.App{
		Title:              "ChitChat API Persona Studio & Test Chat",
		Logger:             diskLogger,
		LogLevelProduction: logger.INFO,
		Width:              1360,
		Height:             860,
		MinWidth:           1080,
		MinHeight:          700,
		BackgroundColour:   &options.RGBA{R: 18, G: 21, B: 19, A: 255},
		AssetServer: &assetserver.Options{
			Assets:  dist,
			Handler: app.HTTPHandler(),
		},
		OnStartup:  app.startup,
		OnShutdown: app.shutdown,
		Bind: []interface{}{
			app,
		},
		Windows: &windows.Options{
			Theme:               windows.SystemDefault,
			WebviewBrowserPath:  runtimeassets.BundledWebView2Path(),
			WebviewUserDataPath: webviewDir,
			ResizeDebounceMS:    10,
		},
	})
	if err != nil {
		log.Fatal(err)
	}
}

func resolveDataDir() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(filepath.Join(wd, "wails.json")); err != nil {
		if exe, exeErr := os.Executable(); exeErr == nil {
			exeDir := filepath.Dir(exe)
			cand := filepath.Clean(filepath.Join(exeDir, "..", ".."))
			if _, statErr := os.Stat(filepath.Join(cand, "wails.json")); statErr == nil {
				wd = cand
			} else if _, statErr := os.Stat(filepath.Join(exeDir, "wails.json")); statErr == nil {
				wd = exeDir
			}
		}
	}
	return filepath.Join(wd, "data"), nil
}
