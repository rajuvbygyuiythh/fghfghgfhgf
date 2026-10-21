package sites

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"chitchatbot/internal/model"
)

// Recipe step identifiers across Group A-E.
const (
	// Common step 1 across all groups.
	StepFolderCreate = "folder_create"

	// Group A (Browserless) steps:
	// folder create -> WS detect -> direct connect test -> save config
	StepWSDetect          = "ws_detect"
	StepDirectConnectTest = "direct_connect_test"
	StepSaveConfig        = "save_config"

	// Group B (Auto-Signup) steps:
	// folder create -> signup endpoint detect -> fake account generate -> POST register -> token save -> chat test
	StepSignupEndpointDetect = "signup_endpoint_detect"
	StepFakeAccountGenerate  = "fake_account_generate"
	StepPostRegister         = "post_register"
	StepTokenSave            = "token_save"
	StepChatTest             = "chat_test"

	// Group C (Browser-Once) steps:
	// folder create -> login URL detect -> mark "needs browser" -> wait user login -> capture session -> verify browserless
	StepLoginURLDetect    = "login_url_detect"
	StepMarkNeedsBrowser  = "mark_needs_browser"
	StepWaitUserLogin     = "wait_user_login"
	StepCaptureSession    = "capture_session"
	StepVerifyBrowserless = "verify_browserless"

	// Group D (DOM-Only) steps:
	// folder create -> headless profile setup -> DOM selector detect -> save driver -> headless test
	StepHeadlessProfileSetup = "headless_profile_setup"
	StepDOMSelectorDetect    = "dom_selector_detect"
	StepSaveDriver           = "save_driver"
	StepHeadlessTest         = "headless_test"

	// Group E (Manual) steps:
	// folder create -> mark manual only
	StepMarkManualOnly = "mark_manual_only"
)

const (
	// RecipeLogFileName is the per-site JSONL log file inside sites/<site_id>/logs/.
	RecipeLogFileName = "recipe.jsonl"
	// LastRecipeRunFileName stores the latest recipe execution summary inside sites/<site_id>/logs/.
	LastRecipeRunFileName = "last_recipe_run.json"
)

var recipeMu sync.Mutex

// RecipeStep defines one sequential step inside a group setup recipe.
type RecipeStep struct {
	Order       int     `json:"order"`
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Action      string  `json:"action"`
	Description string  `json:"description"`
	Required    bool    `json:"required"`
	Runs        int     `json:"runs"`
	Successes   int     `json:"successes"`
	Failures    int     `json:"failures"`
	SuccessRate float64 `json:"success_rate"`
}

// RecipeStepMetric tracks historical step reliability inside a recipe's learning section.
type RecipeStepMetric struct {
	Runs        int     `json:"runs"`
	Successes   int     `json:"successes"`
	Failures    int     `json:"failures"`
	SuccessRate float64 `json:"success_rate"`
	LastError   string  `json:"last_error,omitempty"`
}

// RecipeLearning stores accumulated setup knowledge and success rate metrics inside
// sites/_recipes/group_<X>.json.
type RecipeLearning struct {
	TotalRuns        int                         `json:"total_runs"`
	SuccessfulRuns   int                         `json:"successful_runs"`
	FailedRuns       int                         `json:"failed_runs"`
	SuccessRate      float64                     `json:"success_rate"`
	SuccessRatio     float64                     `json:"success_ratio"`
	VerifiedSites    []string                    `json:"verified_sites,omitempty"`
	LearnedEndpoints map[string]string           `json:"learned_endpoints,omitempty"`
	LearnedSelectors map[string]Selectors        `json:"learned_selectors,omitempty"`
	StepMetrics      map[string]RecipeStepMetric `json:"step_metrics,omitempty"`
	LastSiteID       string                      `json:"last_site_id,omitempty"`
	LastRunAt        string                      `json:"last_run_at,omitempty"`
	LastSuccessAt    string                      `json:"last_success_at,omitempty"`
	UpdatedAt        string                      `json:"updated_at,omitempty"`
}

// Recipe represents one repeatable site setup recipe stored in sites/_recipes/group_<X>.json.
type Recipe struct {
	Group          string         `json:"group"`
	GroupName      string         `json:"group_name"`
	FileName       string         `json:"file_name"`
	Description    string         `json:"description"`
	FallbackGroup  string         `json:"fallback_group,omitempty"`
	FallbackChain  []string       `json:"fallback_chain,omitempty"`
	Steps          []RecipeStep   `json:"steps"`
	TotalRuns      int            `json:"total_runs"`
	SuccessfulRuns int            `json:"successful_runs"`
	FailedRuns     int            `json:"failed_runs"`
	SuccessRate    float64        `json:"success_rate"`
	SuccessRatio   float64        `json:"success_ratio"`
	Learning       RecipeLearning `json:"learning"`
	UpdatedAt      string         `json:"updated_at"`
}

// RecipeStepLog records the outcome of one step execution.
type RecipeStepLog struct {
	StepOrder  int               `json:"step_order"`
	StepID     string            `json:"step_id"`
	StepName   string            `json:"step_name"`
	Group      string            `json:"group"`
	Status     string            `json:"status"` // "passed", "failed", "skipped"
	Message    string            `json:"message"`
	Error      string            `json:"error,omitempty"`
	Artifacts  map[string]string `json:"artifacts,omitempty"`
	StartedAt  string            `json:"started_at"`
	FinishedAt string            `json:"finished_at"`
	DurationMs int64             `json:"duration_ms"`
}

// RecipeAttemptResult records one group recipe attempt (before or after fallback).
type RecipeAttemptResult struct {
	Group       string          `json:"group"`
	GroupName   string          `json:"group_name"`
	Success     bool            `json:"success"`
	FailedStep  string          `json:"failed_step,omitempty"`
	Error       string          `json:"error,omitempty"`
	StepResults []RecipeStepLog `json:"step_results"`
	StartedAt   string          `json:"started_at"`
	FinishedAt  string          `json:"finished_at"`
	DurationMs  int64           `json:"duration_ms"`
}

// RecipeExecutionResult is the complete outcome of running ExecuteSiteRecipe,
// including any fallback transitions (e.g. B -> C -> D -> E).
type RecipeExecutionResult struct {
	SiteID          string                `json:"site_id"`
	InitialGroup    string                `json:"initial_group"`
	FinalGroup      string                `json:"final_group"`
	FinalGroupName  string                `json:"final_group_name"`
	Success         bool                  `json:"success"`
	UsedFallback    bool                  `json:"used_fallback"`
	FallbackHistory []string              `json:"fallback_history,omitempty"`
	Attempts        []RecipeAttemptResult `json:"attempts"`
	LogFilePath     string                `json:"log_file_path"`
	DebugLogPath    string                `json:"debug_log_path"`
	RecipeFilePath  string                `json:"recipe_file_path"`
	SuccessRate     float64               `json:"success_rate"`
	StartedAt       string                `json:"started_at"`
	FinishedAt      string                `json:"finished_at"`
	DurationMs      int64                 `json:"duration_ms"`
}

// RecipeRunOptions customizes how ExecuteSiteRecipe runs for a site.
type RecipeRunOptions struct {
	// Group overrides the starting group ("A".."E"). If empty, uses the site's
	// current Classification.Group (or InitialClassification).
	Group string
	// DisableFallback disables automatic fallback (B->C->D->E / A->B->C->D->E) when true.
	DisableFallback bool
	// Profile is the isolated site profile name (defaults to "default").
	Profile string
	// HTTPClient is an optional HTTP client for live endpoint probing and registration.
	HTTPClient *http.Client
	// ProbeLive forces live HTTP probing against cfg.StartURL even when not localhost.
	ProbeLive bool
	// UserLoginWaiter is an optional callback for Group C ("wait user login").
	// When nil, Group C checks whether a saved browser session/account already exists
	// in sites/<site_id>/sessions/ or sites/<site_id>/accounts/.
	UserLoginWaiter func(ctx context.Context, siteCfg Config) (*model.LoginInfo, error)
	// StepHook is an optional hook invoked before each step executes. Returning a
	// non-nil error causes that step to fail (triggering logging and fallback).
	StepHook func(group string, step RecipeStep, siteCfg Config) error
}

// RecipeFileName returns the canonical JSON file name for a group ("group_A.json".."group_E.json").
func RecipeFileName(group string) string {
	g := NormalizeGroup(group)
	if g == "" {
		g = GroupD
	}
	return fmt.Sprintf("group_%s.json", g)
}

// FallbackGroupFor returns the next fallback group when a group's recipe fails:
// A -> B -> C -> D -> E.
func FallbackGroupFor(group string) string {
	switch NormalizeGroup(group) {
	case GroupA:
		return GroupB
	case GroupB:
		return GroupC
	case GroupC:
		return GroupD
	case GroupD:
		return GroupE
	default:
		return ""
	}
}

// FallbackChainFor returns the ordered fallback sequence after group.
func FallbackChainFor(group string) []string {
	switch NormalizeGroup(group) {
	case GroupA:
		return []string{GroupB, GroupC, GroupD, GroupE}
	case GroupB:
		return []string{GroupC, GroupD, GroupE}
	case GroupC:
		return []string{GroupD, GroupE}
	case GroupD:
		return []string{GroupE}
	default:
		return nil
	}
}

// DefaultRecipe returns the canonical repeatable setup recipe for a group (A-E).
func DefaultRecipe(group string) Recipe {
	g := NormalizeGroup(group)
	if g == "" {
		g = GroupD
	}
	var steps []RecipeStep
	var desc string

	switch g {
	case GroupA:
		desc = "Browserless setup recipe: folder create -> WS detect -> direct connect test -> save config"
		steps = []RecipeStep{
			{
				Order:       1,
				ID:          StepFolderCreate,
				Name:        "folder create",
				Action:      StepFolderCreate,
				Description: "Create and verify isolated site folder structure (sites/<site_id>/, accounts/, profiles/, sessions/, logs/)",
				Required:    true,
			},
			{
				Order:       2,
				ID:          StepWSDetect,
				Name:        "WS detect",
				Action:      StepWSDetect,
				Description: "Detect WebSocket or direct realtime chat endpoint and verify no CAPTCHA or login gate is present",
				Required:    true,
			},
			{
				Order:       3,
				ID:          StepDirectConnectTest,
				Name:        "direct connect test",
				Action:      StepDirectConnectTest,
				Description: "Perform browserless direct connection handshake test against the target endpoint",
				Required:    true,
			},
			{
				Order:       4,
				ID:          StepSaveConfig,
				Name:        "save config",
				Action:      StepSaveConfig,
				Description: "Save verified browserless config and ready session inside sites/<site_id>/",
				Required:    true,
			},
		}
	case GroupB:
		desc = "Auto-Signup setup recipe: folder create -> signup endpoint detect -> fake account generate -> POST register -> token save -> chat test"
		steps = []RecipeStep{
			{
				Order:       1,
				ID:          StepFolderCreate,
				Name:        "folder create",
				Action:      StepFolderCreate,
				Description: "Create and verify isolated site folder structure (sites/<site_id>/)",
				Required:    true,
			},
			{
				Order:       2,
				ID:          StepSignupEndpointDetect,
				Name:        "signup endpoint detect",
				Action:      StepSignupEndpointDetect,
				Description: "Detect API/form signup or guest registration endpoint with no CAPTCHA wall",
				Required:    true,
			},
			{
				Order:       3,
				ID:          StepFakeAccountGenerate,
				Name:        "fake account generate",
				Action:      StepFakeAccountGenerate,
				Description: "Generate isolated guest/persona credentials (nickname, gender, age, password) for this site",
				Required:    true,
			},
			{
				Order:       4,
				ID:          StepPostRegister,
				Name:        "POST register",
				Action:      StepPostRegister,
				Description: "Submit registration payload to signup endpoint and obtain session token/cookies",
				Required:    true,
			},
			{
				Order:       5,
				ID:          StepTokenSave,
				Name:        "token save",
				Action:      StepTokenSave,
				Description: "Save registered token and cookies into sites/<site_id>/accounts/ and sites/<site_id>/sessions/",
				Required:    true,
			},
			{
				Order:       6,
				ID:          StepChatTest,
				Name:        "chat test",
				Action:      StepChatTest,
				Description: "Verify saved token/session can authenticate for chat without browser intervention",
				Required:    true,
			},
		}
	case GroupC:
		desc = "Browser-Once setup recipe: folder create -> login URL detect -> mark \"needs browser\" -> wait user login -> capture session -> verify browserless"
		steps = []RecipeStep{
			{
				Order:       1,
				ID:          StepFolderCreate,
				Name:        "folder create",
				Action:      StepFolderCreate,
				Description: "Create and verify isolated site folder structure (sites/<site_id>/)",
				Required:    true,
			},
			{
				Order:       2,
				ID:          StepLoginURLDetect,
				Name:        "login URL detect",
				Action:      StepLoginURLDetect,
				Description: "Detect login/CAPTCHA entry URL and verify no unrecoverable manual-only anti-bot block",
				Required:    true,
			},
			{
				Order:       3,
				ID:          StepMarkNeedsBrowser,
				Name:        `mark "needs browser"`,
				Action:      StepMarkNeedsBrowser,
				Description: "Mark site as needing one-time browser login/CAPTCHA solve in sites/<site_id>/",
				Required:    true,
			},
			{
				Order:       4,
				ID:          StepWaitUserLogin,
				Name:        "wait user login",
				Action:      StepWaitUserLogin,
				Description: "Wait for one-time user browser login or load existing authenticated browser session",
				Required:    true,
			},
			{
				Order:       5,
				ID:          StepCaptureSession,
				Name:        "capture session",
				Action:      StepCaptureSession,
				Description: "Capture and persist browser cookies, localStorage, and auth tokens inside sites/<site_id>/sessions/",
				Required:    true,
			},
			{
				Order:       6,
				ID:          StepVerifyBrowserless,
				Name:        "verify browserless",
				Action:      StepVerifyBrowserless,
				Description: "Verify captured session allows subsequent browserless operation without re-opening the browser",
				Required:    true,
			},
		}
	case GroupD:
		desc = "DOM-Only setup recipe: folder create -> headless profile setup -> DOM selector detect -> save driver -> headless test"
		steps = []RecipeStep{
			{
				Order:       1,
				ID:          StepFolderCreate,
				Name:        "folder create",
				Action:      StepFolderCreate,
				Description: "Create and verify isolated site folder structure (sites/<site_id>/)",
				Required:    true,
			},
			{
				Order:       2,
				ID:          StepHeadlessProfileSetup,
				Name:        "headless profile setup",
				Action:      StepHeadlessProfileSetup,
				Description: "Initialize isolated headless browser profile directory under sites/<site_id>/profiles/<profile>/browser",
				Required:    true,
			},
			{
				Order:       3,
				ID:          StepDOMSelectorDetect,
				Name:        "DOM selector detect",
				Action:      StepDOMSelectorDetect,
				Description: "Detect and validate DOM chat selectors (composer, send, skip, start, message history)",
				Required:    true,
			},
			{
				Order:       4,
				ID:          StepSaveDriver,
				Name:        "save driver",
				Action:      StepSaveDriver,
				Description: "Generate and persist isolated sites/<site_id>/dom_driver.js and selectors in site.json",
				Required:    true,
			},
			{
				Order:       5,
				ID:          StepHeadlessTest,
				Name:        "headless test",
				Action:      StepHeadlessTest,
				Description: "Verify dom_driver.js API surface and headless profile readiness for DOM chat",
				Required:    true,
			},
		}
	default:
		g = GroupE
		desc = "Manual setup recipe: folder create -> mark manual only"
		steps = []RecipeStep{
			{
				Order:       1,
				ID:          StepFolderCreate,
				Name:        "folder create",
				Action:      StepFolderCreate,
				Description: "Create and verify isolated site folder structure (sites/<site_id>/)",
				Required:    true,
			},
			{
				Order:       2,
				ID:          StepMarkManualOnly,
				Name:        "mark manual only",
				Action:      StepMarkManualOnly,
				Description: "Mark site as manual-only due to heavy bot detection and persist manual marker in sites/<site_id>/",
				Required:    true,
			},
		}
	}

	stepMetrics := make(map[string]RecipeStepMetric, len(steps))
	for _, st := range steps {
		stepMetrics[st.ID] = RecipeStepMetric{}
	}

	return Recipe{
		Group:         g,
		GroupName:     GroupName(g),
		FileName:      RecipeFileName(g),
		Description:   desc,
		FallbackGroup: FallbackGroupFor(g),
		FallbackChain: FallbackChainFor(g),
		Steps:         steps,
		Learning: RecipeLearning{
			VerifiedSites:    []string{},
			LearnedEndpoints: map[string]string{},
			LearnedSelectors: map[string]Selectors{},
			StepMetrics:      stepMetrics,
		},
		UpdatedAt: "2026-10-22T00:00:00Z",
	}
}

// AllDefaultRecipes returns the 5 canonical recipes for Groups A, B, C, D, and E.
func AllDefaultRecipes() []Recipe {
	return []Recipe{
		DefaultRecipe(GroupA),
		DefaultRecipe(GroupB),
		DefaultRecipe(GroupC),
		DefaultRecipe(GroupD),
		DefaultRecipe(GroupE),
	}
}

// EnsureRecipes ensures sites/_recipes/ exists and contains group_A.json .. group_E.json.
func EnsureRecipes(dataDir string) (string, error) {
	if strings.TrimSpace(dataDir) == "" {
		return "", fmt.Errorf("sites: empty data directory")
	}
	recipeMu.Lock()
	defer recipeMu.Unlock()

	return ensureRecipesLocked(dataDir)
}

func ensureRecipesLocked(dataDir string) (string, error) {
	dir := RecipesDir(filepath.Clean(dataDir))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	for _, def := range AllDefaultRecipes() {
		filePath, err := SafeJoin(dir, def.FileName)
		if err != nil {
			return "", err
		}
		if _, statErr := os.Stat(filePath); errors.Is(statErr, os.ErrNotExist) {
			if err := writeRecipeFileAtomic(filePath, def); err != nil {
				return "", err
			}
			continue
		}
		// Ensure existing file is valid and normalized.
		raw, readErr := os.ReadFile(filePath)
		if readErr != nil {
			continue
		}
		var existing Recipe
		if jsonErr := json.Unmarshal(raw, &existing); jsonErr != nil || len(existing.Steps) == 0 {
			if err := writeRecipeFileAtomic(filePath, def); err != nil {
				return "", err
			}
			continue
		}
		normalized := normalizeRecipe(existing, def)
		if normalized.Group != existing.Group || normalized.FileName != existing.FileName || len(existing.Steps) != len(def.Steps) {
			if err := writeRecipeFileAtomic(filePath, normalized); err != nil {
				return "", err
			}
		}
	}
	return dir, nil
}

func normalizeRecipe(rec, def Recipe) Recipe {
	rec.Group = def.Group
	rec.GroupName = def.GroupName
	rec.FileName = def.FileName
	if strings.TrimSpace(rec.Description) == "" {
		rec.Description = def.Description
	}
	rec.FallbackGroup = def.FallbackGroup
	rec.FallbackChain = def.FallbackChain
	if len(rec.Steps) == 0 {
		rec.Steps = def.Steps
	}
	if rec.Learning.LearnedEndpoints == nil {
		rec.Learning.LearnedEndpoints = map[string]string{}
	}
	if rec.Learning.LearnedSelectors == nil {
		rec.Learning.LearnedSelectors = map[string]Selectors{}
	}
	if rec.Learning.StepMetrics == nil {
		rec.Learning.StepMetrics = map[string]RecipeStepMetric{}
	}
	for _, st := range rec.Steps {
		if _, ok := rec.Learning.StepMetrics[st.ID]; !ok {
			rec.Learning.StepMetrics[st.ID] = RecipeStepMetric{
				Runs:        st.Runs,
				Successes:   st.Successes,
				Failures:    st.Failures,
				SuccessRate: st.SuccessRate,
			}
		}
	}
	return rec
}

func writeRecipeFileAtomic(filePath string, rec Recipe) error {
	if err := os.MkdirAll(filepath.Dir(filePath), 0o700); err != nil {
		return err
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(rec); err != nil {
		return err
	}
	tmp := fmt.Sprintf("%s.%d.tmp", filePath, time.Now().UnixNano())
	if err := os.WriteFile(tmp, buf.Bytes(), 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, filePath); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// LoadRecipe loads one recipe (group A-E) from sites/_recipes/group_<X>.json.
func LoadRecipe(dataDir, group string) (Recipe, error) {
	recipeMu.Lock()
	defer recipeMu.Unlock()
	return loadRecipeLocked(dataDir, group)
}

func loadRecipeLocked(dataDir, group string) (Recipe, error) {
	g := NormalizeGroup(group)
	if g == "" {
		return Recipe{}, fmt.Errorf("sites: invalid recipe group %q", group)
	}
	dir, err := ensureRecipesLocked(dataDir)
	if err != nil {
		return Recipe{}, err
	}
	def := DefaultRecipe(g)
	filePath, err := SafeJoin(dir, def.FileName)
	if err != nil {
		return Recipe{}, err
	}
	raw, err := os.ReadFile(filePath)
	if err != nil {
		return Recipe{}, err
	}
	var rec Recipe
	if err := json.Unmarshal(raw, &rec); err != nil {
		return Recipe{}, err
	}
	return normalizeRecipe(rec, def), nil
}

// SaveRecipe saves a Recipe atomically to sites/_recipes/group_<X>.json.
func SaveRecipe(dataDir string, recipe Recipe) error {
	recipeMu.Lock()
	defer recipeMu.Unlock()
	return saveRecipeLocked(dataDir, recipe)
}

func saveRecipeLocked(dataDir string, recipe Recipe) error {
	g := NormalizeGroup(recipe.Group)
	if g == "" {
		return fmt.Errorf("sites: invalid recipe group %q", recipe.Group)
	}
	dir, err := ensureRecipesLocked(dataDir)
	if err != nil {
		return err
	}
	def := DefaultRecipe(g)
	recipe = normalizeRecipe(recipe, def)
	filePath, err := SafeJoin(dir, def.FileName)
	if err != nil {
		return err
	}
	return writeRecipeFileAtomic(filePath, recipe)
}

// ListRecipes returns all 5 recipes (Groups A, B, C, D, E) from sites/_recipes/.
func ListRecipes(dataDir string) ([]Recipe, error) {
	recipeMu.Lock()
	defer recipeMu.Unlock()

	if _, err := ensureRecipesLocked(dataDir); err != nil {
		return nil, err
	}
	groups := []string{GroupA, GroupB, GroupC, GroupD, GroupE}
	out := make([]Recipe, 0, len(groups))
	for _, g := range groups {
		rec, err := loadRecipeLocked(dataDir, g)
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, nil
}

func roundRate(num, den int) (float64, float64) {
	if den <= 0 {
		return 0, 0
	}
	ratio := float64(num) / float64(den)
	pct := math.Round(ratio*10000) / 100 // e.g. 75.25
	ratioRounded := math.Round(ratio*10000) / 10000
	return pct, ratioRounded
}

// recordRecipeOutcomeLocked updates the recipe's run statistics, step metrics,
// and learned site artifacts inside sites/_recipes/group_<X>.json.
func recordRecipeOutcomeLocked(
	dataDir string,
	group string,
	siteID string,
	attempt RecipeAttemptResult,
	learnedEndpoint string,
	learnedSelectors *Selectors,
) (Recipe, string, error) {
	rec, err := loadRecipeLocked(dataDir, group)
	if err != nil {
		return Recipe{}, "", err
	}
	now := time.Now().UTC().Format(time.RFC3339)

	rec.TotalRuns++
	if attempt.Success {
		rec.SuccessfulRuns++
	} else {
		rec.FailedRuns++
	}
	rec.SuccessRate, rec.SuccessRatio = roundRate(rec.SuccessfulRuns, rec.TotalRuns)
	rec.UpdatedAt = now

	rec.Learning.TotalRuns = rec.TotalRuns
	rec.Learning.SuccessfulRuns = rec.SuccessfulRuns
	rec.Learning.FailedRuns = rec.FailedRuns
	rec.Learning.SuccessRate = rec.SuccessRate
	rec.Learning.SuccessRatio = rec.SuccessRatio
	rec.Learning.LastSiteID = siteID
	rec.Learning.LastRunAt = now
	rec.Learning.UpdatedAt = now

	if attempt.Success {
		rec.Learning.LastSuccessAt = now
		rec.Learning.VerifiedSites = appendUniqueStr(rec.Learning.VerifiedSites, siteID)
		if strings.TrimSpace(learnedEndpoint) != "" {
			rec.Learning.LearnedEndpoints[siteID] = strings.TrimSpace(learnedEndpoint)
		}
		if learnedSelectors != nil && strings.TrimSpace(learnedSelectors.ComposerInput) != "" {
			rec.Learning.LearnedSelectors[siteID] = *learnedSelectors
		}
	}

	stepResultByID := make(map[string]RecipeStepLog, len(attempt.StepResults))
	for _, sr := range attempt.StepResults {
		stepResultByID[sr.StepID] = sr
	}

	for i := range rec.Steps {
		st := &rec.Steps[i]
		sr, ran := stepResultByID[st.ID]
		if !ran || sr.Status == "skipped" {
			continue
		}
		st.Runs++
		if sr.Status == "passed" {
			st.Successes++
		} else {
			st.Failures++
		}
		st.SuccessRate, _ = roundRate(st.Successes, st.Runs)

		metric := rec.Learning.StepMetrics[st.ID]
		metric.Runs = st.Runs
		metric.Successes = st.Successes
		metric.Failures = st.Failures
		metric.SuccessRate = st.SuccessRate
		if sr.Status != "passed" && sr.Error != "" {
			metric.LastError = sr.Error
		}
		rec.Learning.StepMetrics[st.ID] = metric
	}

	if err := saveRecipeLocked(dataDir, rec); err != nil {
		return Recipe{}, "", err
	}
	filePath, _ := RecipesPath(dataDir, rec.FileName)
	return rec, filePath, nil
}

func appendUniqueStr(slice []string, item string) []string {
	item = strings.TrimSpace(item)
	if item == "" {
		return slice
	}
	for _, existing := range slice {
		if existing == item {
			return slice
		}
	}
	return append(slice, item)
}

// appendSiteRecipeLog writes one JSONL entry strictly inside sites/<site_id>/logs/recipe.jsonl
// and mirrors a structured event to sites/<site_id>/logs/debug.jsonl.
func appendSiteRecipeLog(siteCtx SiteContext, event string, fields map[string]any) {
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
	if line, err := json.Marshal(entry); err == nil {
		line = append(line, '\n')
		if f, openErr := siteCtx.OpenFile(filepath.Join("logs", RecipeLogFileName), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600); openErr == nil {
			_, _ = f.Write(line)
			_ = f.Close()
		}
	}
	detailBytes, _ := json.Marshal(fields)
	_ = AppendSiteDebugLog(siteCtx.DataDir, siteCtx.SiteID, event, "info", event, string(detailBytes))
}

// ReadSiteRecipeLogs reads all JSONL entries from sites/<site_id>/logs/recipe.jsonl.
func ReadSiteRecipeLogs(dataDir, siteID string) ([]map[string]any, error) {
	siteCtx, err := NewSiteContext(dataDir, siteID)
	if err != nil {
		return nil, err
	}
	raw, err := siteCtx.ReadFile(filepath.Join("logs", RecipeLogFileName))
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
		if err := json.Unmarshal([]byte(line), &item); err == nil {
			out = append(out, item)
		}
	}
	return out, nil
}

// ExecuteRecipeGroup runs the setup recipe for siteID starting at group, with automatic
// fallback (A->B->C->D->E or B->C->D->E) enabled.
func ExecuteRecipeGroup(ctx context.Context, dataDir, siteID, group string) (RecipeExecutionResult, error) {
	return ExecuteSiteRecipe(ctx, dataDir, siteID, RecipeRunOptions{
		Group: group,
	})
}

// ExecuteSiteRecipe executes the setup recipe for siteID step by step.
//
//   - If any step fails, the error is logged to sites/<site_id>/logs/recipe.jsonl,
//     the recipe's failure stats are updated in sites/_recipes/group_<X>.json, and
//     (unless opts.DisableFallback is true) the executor automatically tries the next
//     fallback group in order (A -> B -> C -> D -> E / B -> C -> D -> E).
//   - When a group recipe succeeds, its learned patterns and updated success_rate are
//     persisted to sites/_recipes/group_<X>.json and the site's classification is updated
//     in sites/<site_id>/site.json.
//   - Every execution log is written strictly inside sites/<site_id>/logs/.
func ExecuteSiteRecipe(ctx context.Context, dataDir, siteID string, opts RecipeRunOptions) (RecipeExecutionResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	started := time.Now()
	if _, err := EnsureRecipes(dataDir); err != nil {
		return RecipeExecutionResult{}, err
	}

	siteCtx, err := NewSiteContext(dataDir, siteID)
	if err != nil {
		return RecipeExecutionResult{}, err
	}
	if err := siteCtx.Ensure(); err != nil {
		return RecipeExecutionResult{}, err
	}

	cfg, err := LoadSite(dataDir, siteID)
	if err != nil {
		// If the site is not yet on disk, initialize a minimal config inside its own folder.
		cfg = Config{
			ID:          siteCtx.SiteID,
			Name:        siteCtx.SiteID,
			Family:      "family_a",
			AccountMode: AccountModeAutoRegister,
			Enabled:     true,
			StartURL:    "https://" + strings.ReplaceAll(siteCtx.SiteID, "_", ".") + "/",
			AppURL:      "https://" + strings.ReplaceAll(siteCtx.SiteID, "_", ".") + "/",
			AuthMode:    "dom_only",
			Selectors:   ArchetypeSelectors("family_a"),
		}
		cfg.Classification = InitialClassification(cfg)
	}

	startGroup := NormalizeGroup(opts.Group)
	if startGroup == "" {
		startGroup = NormalizeGroup(cfg.Classification.Group)
	}
	if startGroup == "" {
		startGroup = InitialClassification(cfg).Group
	}

	profile := strings.TrimSpace(opts.Profile)
	if profile == "" {
		profile = "default"
	}

	chain := []string{startGroup}
	if !opts.DisableFallback {
		chain = append(chain, FallbackChainFor(startGroup)...)
	}

	logFilePath, _ := siteCtx.Resolve(filepath.Join("logs", RecipeLogFileName))
	result := RecipeExecutionResult{
		SiteID:       siteCtx.SiteID,
		InitialGroup: startGroup,
		LogFilePath:  logFilePath,
		DebugLogPath: siteCtx.DebugLogPath,
		StartedAt:    started.UTC().Format(time.RFC3339Nano),
	}

	appendSiteRecipeLog(siteCtx, "recipe_execution_started", map[string]any{
		"initial_group":    startGroup,
		"fallback_enabled": !opts.DisableFallback,
		"candidate_chain":  chain,
		"profile":          profile,
	})

	for idx, group := range chain {
		if idx > 0 {
			result.UsedFallback = true
			result.FallbackHistory = append(result.FallbackHistory, fmt.Sprintf("%s->%s", chain[idx-1], group))
			appendSiteRecipeLog(siteCtx, "recipe_fallback_transition", map[string]any{
				"from_group": chain[idx-1],
				"to_group":   group,
				"reason":     result.Attempts[len(result.Attempts)-1].Error,
			})
		}

		recipe, loadErr := LoadRecipe(dataDir, group)
		if loadErr != nil {
			return result, loadErr
		}

		attempt, learnedEndpoint, learnedSel := executeSingleGroupAttempt(ctx, siteCtx, &cfg, recipe, profile, opts)
		result.Attempts = append(result.Attempts, attempt)

		recipeMu.Lock()
		updatedRec, recPath, saveErr := recordRecipeOutcomeLocked(dataDir, group, siteCtx.SiteID, attempt, learnedEndpoint, learnedSel)
		recipeMu.Unlock()
		if saveErr == nil {
			result.RecipeFilePath = recPath
			result.SuccessRate = updatedRec.SuccessRate
		}

		if attempt.Success {
			result.Success = true
			result.FinalGroup = group
			result.FinalGroupName = GroupName(group)
			result.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
			result.DurationMs = time.Since(started).Milliseconds()

			appendSiteRecipeLog(siteCtx, "recipe_execution_succeeded", map[string]any{
				"initial_group":  result.InitialGroup,
				"final_group":    result.FinalGroup,
				"used_fallback":  result.UsedFallback,
				"fallback_chain": result.FallbackHistory,
				"recipe_file":    result.RecipeFilePath,
				"success_rate":   result.SuccessRate,
				"duration_ms":    result.DurationMs,
			})
			_ = siteCtx.SaveJSON(filepath.Join("logs", LastRecipeRunFileName), result)
			return result, nil
		}

		appendSiteRecipeLog(siteCtx, "recipe_attempt_failed", map[string]any{
			"group":               group,
			"failed_step":         attempt.FailedStep,
			"error":               attempt.Error,
			"next_fallback_group": FallbackGroupFor(group),
		})
	}

	result.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
	result.DurationMs = time.Since(started).Milliseconds()
	if len(result.Attempts) > 0 {
		last := result.Attempts[len(result.Attempts)-1]
		result.FinalGroup = last.Group
		result.FinalGroupName = last.GroupName
	}

	appendSiteRecipeLog(siteCtx, "recipe_execution_failed", map[string]any{
		"initial_group":  result.InitialGroup,
		"final_group":    result.FinalGroup,
		"used_fallback":  result.UsedFallback,
		"fallback_chain": result.FallbackHistory,
		"duration_ms":    result.DurationMs,
	})
	_ = siteCtx.SaveJSON(filepath.Join("logs", LastRecipeRunFileName), result)

	if len(result.Attempts) > 0 {
		last := result.Attempts[len(result.Attempts)-1]
		return result, fmt.Errorf("recipe %s failed at step %q: %s", last.Group, last.FailedStep, last.Error)
	}
	return result, errors.New("recipe execution failed")
}

// stepExecutionState holds intermediate artifacts passed between sequential steps
// of a single recipe attempt.
type stepExecutionState struct {
	probe            *ProbeResult
	wsEndpoint       string
	signupEndpoint   string
	loginURL         string
	generatedAccount *SiteAccount
	capturedLogin    *model.LoginInfo
	detectedSel      *Selectors
}

func shouldProbeLive(cfg Config, opts RecipeRunOptions) bool {
	if opts.ProbeLive || opts.HTTPClient != nil {
		return true
	}
	u := strings.ToLower(strings.TrimSpace(cfg.StartURL))
	return strings.HasPrefix(u, "http://127.0.0.1") ||
		strings.HasPrefix(u, "https://127.0.0.1") ||
		strings.HasPrefix(u, "http://localhost") ||
		strings.HasPrefix(u, "https://localhost")
}

func executeSingleGroupAttempt(
	ctx context.Context,
	siteCtx SiteContext,
	cfg *Config,
	recipe Recipe,
	profile string,
	opts RecipeRunOptions,
) (RecipeAttemptResult, string, *Selectors) {
	started := time.Now()
	attempt := RecipeAttemptResult{
		Group:       recipe.Group,
		GroupName:   recipe.GroupName,
		StepResults: make([]RecipeStepLog, 0, len(recipe.Steps)),
		StartedAt:   started.UTC().Format(time.RFC3339Nano),
	}

	state := &stepExecutionState{}

	for _, step := range recipe.Steps {
		stepStart := time.Now()
		stepLog := RecipeStepLog{
			StepOrder: step.Order,
			StepID:    step.ID,
			StepName:  step.Name,
			Group:     recipe.Group,
			Artifacts: map[string]string{},
			StartedAt: stepStart.UTC().Format(time.RFC3339Nano),
		}

		appendSiteRecipeLog(siteCtx, "recipe_step_started", map[string]any{
			"group":      recipe.Group,
			"step_order": step.Order,
			"step_id":    step.ID,
			"step_name":  step.Name,
		})

		var stepErr error
		var msg string
		if opts.StepHook != nil {
			stepErr = opts.StepHook(recipe.Group, step, *cfg)
		}
		if stepErr == nil {
			msg, stepErr = runRecipeStep(ctx, siteCtx, cfg, recipe.Group, step, profile, opts, state, stepLog.Artifacts)
		}

		stepLog.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
		stepLog.DurationMs = time.Since(stepStart).Milliseconds()

		if stepErr != nil {
			stepLog.Status = "failed"
			stepLog.Error = stepErr.Error()
			stepLog.Message = fmt.Sprintf("step %q failed: %v", step.Name, stepErr)
			attempt.StepResults = append(attempt.StepResults, stepLog)
			attempt.Success = false
			attempt.FailedStep = step.ID
			attempt.Error = stepErr.Error()
			attempt.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
			attempt.DurationMs = time.Since(started).Milliseconds()

			appendSiteRecipeLog(siteCtx, "recipe_step_failed", map[string]any{
				"group":       recipe.Group,
				"step_order":  step.Order,
				"step_id":     step.ID,
				"step_name":   step.Name,
				"error":       stepErr.Error(),
				"duration_ms": stepLog.DurationMs,
			})
			return attempt, "", nil
		}

		stepLog.Status = "passed"
		stepLog.Message = msg
		attempt.StepResults = append(attempt.StepResults, stepLog)

		appendSiteRecipeLog(siteCtx, "recipe_step_passed", map[string]any{
			"group":       recipe.Group,
			"step_order":  step.Order,
			"step_id":     step.ID,
			"step_name":   step.Name,
			"message":     msg,
			"artifacts":   stepLog.Artifacts,
			"duration_ms": stepLog.DurationMs,
		})
	}

	attempt.Success = true
	attempt.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
	attempt.DurationMs = time.Since(started).Milliseconds()

	learnedEndpoint := state.wsEndpoint
	if learnedEndpoint == "" {
		learnedEndpoint = state.signupEndpoint
	}
	if learnedEndpoint == "" {
		learnedEndpoint = state.loginURL
	}
	return attempt, learnedEndpoint, state.detectedSel
}

var formActionPattern = regexp.MustCompile(`(?i)<form[^>]*\baction\s*=\s*["']([^"']+)["']`)

func runRecipeStep(
	ctx context.Context,
	siteCtx SiteContext,
	cfg *Config,
	group string,
	step RecipeStep,
	profile string,
	opts RecipeRunOptions,
	state *stepExecutionState,
	artifacts map[string]string,
) (string, error) {
	switch step.ID {
	case StepFolderCreate:
		if err := siteCtx.Ensure(); err != nil {
			return "", err
		}
		for _, rel := range []string{
			filepath.Join("profiles", profile, ".gitkeep"),
			filepath.Join("accounts", ".gitkeep"),
			filepath.Join("sessions", ".gitkeep"),
			filepath.Join("logs", ".gitkeep"),
		} {
			if _, statErr := siteCtx.Stat(rel); errors.Is(statErr, os.ErrNotExist) {
				_ = siteCtx.WriteFile(rel, nil, 0o644)
			}
		}
		if _, statErr := siteCtx.Stat("site.json"); errors.Is(statErr, os.ErrNotExist) {
			toSave := *cfg
			toSave.FolderPath = ""
			if err := siteCtx.SaveJSON("site.json", toSave); err != nil {
				return "", err
			}
		}
		if _, statErr := siteCtx.Stat("dom_driver.js"); errors.Is(statErr, os.ErrNotExist) {
			if err := siteCtx.WriteFile("dom_driver.js", []byte(UniversalDOMDriverJS(*cfg)), 0o644); err != nil {
				return "", err
			}
		}
		artifacts["folder_path"] = siteCtx.FolderPath
		return fmt.Sprintf("isolated folder ready at %s", siteCtx.FolderPath), nil

	// =========================================================================
	// Group A: Browserless (folder create -> WS detect -> direct connect test -> save config)
	// =========================================================================
	case StepWSDetect:
		if shouldProbeLive(*cfg, opts) {
			pr, err := ProbeURL(ctx, cfg.StartURL, opts.HTTPClient)
			if err != nil {
				return "", fmt.Errorf("WS detect probe failed: %w", err)
			}
			state.probe = &pr
			if pr.HeavyBotDetection {
				return "", fmt.Errorf("heavy bot detection (%v) blocks browserless WS", pr.BotCheckKeywords)
			}
			if pr.HasCaptcha {
				return "", fmt.Errorf("CAPTCHA (%v) blocks direct browserless WS", pr.CaptchaKeywords)
			}
			if pr.LoginRequired {
				return "", fmt.Errorf("login required (%v) blocks anonymous browserless WS", pr.LoginIndicators)
			}
			if pr.HasSignupForm {
				return "", fmt.Errorf("signup form (%v) required before chat; falling back from Group A", pr.SignupIndicators)
			}
			if len(pr.WebSocketEndpoints) > 0 {
				state.wsEndpoint = pr.WebSocketEndpoints[0]
				for _, ep := range pr.WebSocketEndpoints {
					low := strings.ToLower(ep)
					if strings.HasPrefix(low, "ws://") || strings.HasPrefix(low, "wss://") {
						state.wsEndpoint = ep
						break
					}
				}
			} else if len(pr.APIEndpoints) > 0 {
				state.wsEndpoint = pr.APIEndpoints[0]
			} else if strings.TrimSpace(cfg.APIBaseURL) != "" {
				state.wsEndpoint = strings.TrimSpace(cfg.APIBaseURL)
			} else {
				return "", errors.New("no WebSocket or direct API endpoint found on page")
			}
		} else {
			if cfg.Classification.Probe != nil {
				pr := *cfg.Classification.Probe
				state.probe = &pr
			}
			initial := InitialClassification(*cfg)
			if cfg.Classification.Group != "" && cfg.Classification.Group != GroupA && initial.Group != GroupA {
				return "", fmt.Errorf("site %s requires group %s setup (%s), not direct Browserless Group A", cfg.ID, cfg.Classification.Group, cfg.Classification.Reason)
			}
			if strings.TrimSpace(cfg.APIBaseURL) != "" {
				state.wsEndpoint = cfg.APIBaseURL
			} else {
				u, _ := url.Parse(cfg.StartURL)
				host := "localhost"
				if u != nil && u.Host != "" {
					host = u.Host
				}
				state.wsEndpoint = "wss://" + host + "/ws"
			}
		}
		artifacts["ws_endpoint"] = state.wsEndpoint
		return fmt.Sprintf("detected direct endpoint %s", state.wsEndpoint), nil

	case StepDirectConnectTest:
		if state.wsEndpoint == "" {
			return "", errors.New("cannot run direct connect test without detected WS/API endpoint")
		}
		if shouldProbeLive(*cfg, opts) {
			client := opts.HTTPClient
			if client == nil {
				client = &http.Client{Timeout: 5 * time.Second}
			}
			testURL := cfg.StartURL
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, testURL, nil)
			if err != nil {
				return "", err
			}
			req.Header.Set("Accept", "application/json, text/plain, */*")
			resp, err := client.Do(req)
			if err != nil {
				return "", fmt.Errorf("direct connect request failed: %w", err)
			}
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
			_ = resp.Body.Close()
			if resp.StatusCode >= 400 {
				return "", fmt.Errorf("direct connect returned HTTP %d", resp.StatusCode)
			}
			artifacts["http_status"] = fmt.Sprintf("%d", resp.StatusCode)
		}
		state.capturedLogin = &model.LoginInfo{
			UserID:  "anon_" + cfg.ID,
			Backend: "browserless_ws",
			LocalStorage: map[string]string{
				"ws_endpoint": state.wsEndpoint,
				"mode":        "browserless",
			},
		}
		artifacts["connection"] = "verified"
		return fmt.Sprintf("direct browserless connection verified for %s", state.wsEndpoint), nil

	case StepSaveConfig:
		if state.capturedLogin != nil {
			sessionPath, err := SaveSiteSession(siteCtx.DataDir, cfg.ID, profile, *state.capturedLogin)
			if err != nil {
				return "", err
			}
			artifacts["session_path"] = sessionPath
		}
		cfg.AccountMode = AccountModeAutoRegister
		cfg.Classification = Classification{
			Group:        GroupA,
			GroupName:    GroupName(GroupA),
			Confidence:   ConfidenceHigh,
			Source:       "recipe_group_A",
			Reason:       fmt.Sprintf("Verified Browserless setup via Group A recipe (endpoint: %s)", state.wsEndpoint),
			ClassifiedAt: time.Now().UTC().Format(time.RFC3339),
			ProbeCount:   cfg.Classification.ProbeCount + 1,
			Probe:        state.probe,
		}
		toSave := *cfg
		toSave.FolderPath = ""
		if err := siteCtx.SaveJSON("site.json", toSave); err != nil {
			return "", err
		}
		artifacts["site_json"] = filepath.Join(siteCtx.FolderPath, "site.json")
		return "saved verified Group A browserless config to site.json", nil

	// =========================================================================
	// Group B: Auto-Signup (folder create -> signup endpoint detect -> fake account generate -> POST register -> token save -> chat test)
	// =========================================================================
	case StepSignupEndpointDetect:
		if shouldProbeLive(*cfg, opts) {
			client := opts.HTTPClient
			if client == nil {
				client = &http.Client{Timeout: 6 * time.Second}
			}
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, cfg.StartURL, nil)
			if err != nil {
				return "", err
			}
			resp, err := client.Do(req)
			if err != nil {
				return "", fmt.Errorf("signup endpoint probe failed: %w", err)
			}
			rawBody, _ := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
			_ = resp.Body.Close()

			finalURL := cfg.StartURL
			if resp.Request != nil && resp.Request.URL != nil {
				finalURL = resp.Request.URL.String()
			}
			pr := AnalyzeHTML(cfg.StartURL, finalURL, resp.StatusCode, resp.Header, string(rawBody))
			state.probe = &pr

			if pr.HeavyBotDetection {
				return "", fmt.Errorf("heavy bot detection (%v) blocks auto-signup", pr.BotCheckKeywords)
			}
			if pr.HasCaptcha {
				return "", fmt.Errorf("CAPTCHA (%v) blocks automated signup", pr.CaptchaKeywords)
			}
			if pr.LoginRequired && !pr.HasSignupForm {
				return "", fmt.Errorf("login required (%v) without open signup endpoint", pr.LoginIndicators)
			}

			endpoint := detectSignupActionURL(finalURL, string(rawBody), pr)
			if endpoint == "" {
				return "", errors.New("no signup form or registration API endpoint detected")
			}
			state.signupEndpoint = endpoint
		} else {
			normGroup := cfg.Classification.Group
			if normGroup == "" {
				normGroup = InitialClassification(*cfg).Group
			}
			if normGroup == GroupC || normGroup == GroupD || normGroup == GroupE || NormalizeAccountMode(cfg.AccountMode, cfg.ID, cfg.AuthMode) == AccountModeBrowserSetup {
				return "", fmt.Errorf("site %s requires %s (%s), cannot auto-signup without browser", cfg.ID, GroupName(normGroup), normGroup)
			}
			if strings.TrimSpace(cfg.APIBaseURL) != "" {
				state.signupEndpoint = strings.TrimRight(cfg.APIBaseURL, "/") + "/api/register"
			} else {
				state.signupEndpoint = strings.TrimRight(cfg.StartURL, "/") + "/api/guest"
			}
		}
		artifacts["signup_endpoint"] = state.signupEndpoint
		return fmt.Sprintf("detected signup endpoint %s", state.signupEndpoint), nil

	case StepFakeAccountGenerate:
		nickname, age := generateGuestAlias(1)
		accountID := fmt.Sprintf("%s_%s_%s", cfg.ID, strings.ToLower(nickname), randomHex(4))
		token := "autoreg_" + cfg.ID + "_" + randomHex(12)
		state.generatedAccount = &SiteAccount{
			ID:          accountID,
			SiteID:      cfg.ID,
			SiteName:    cfg.Name,
			AccountMode: AccountModeAutoRegister,
			Profile:     profile,
			Nickname:    nickname,
			Gender:      "female",
			Age:         age,
			Token:       token,
			Cookies: map[string]string{
				"token":        token,
				"guest_nick":   nickname,
				"guest_gender": "female",
				"guest_age":    fmt.Sprintf("%d", age),
				"age_verified": "1",
			},
			LocalStore: map[string]string{
				"nickname":      nickname,
				"gender":        "female",
				"age":           fmt.Sprintf("%d", age),
				"session_token": token,
				"age_verified":  "true",
			},
			Status:    "generated",
			CreatedAt: time.Now().UTC().Format(time.RFC3339),
		}
		artifacts["account_id"] = accountID
		artifacts["nickname"] = nickname
		return fmt.Sprintf("generated fake account %s (nickname=%s, age=%d)", accountID, nickname, age), nil

	case StepPostRegister:
		if state.generatedAccount == nil {
			return "", errors.New("no generated account available for POST register")
		}
		if state.signupEndpoint == "" {
			return "", errors.New("no signup endpoint available for POST register")
		}
		acc := state.generatedAccount
		if shouldProbeLive(*cfg, opts) {
			client := opts.HTTPClient
			if client == nil {
				client = &http.Client{Timeout: 6 * time.Second}
			}
			payload := map[string]any{
				"nickname": acc.Nickname,
				"username": acc.Nickname,
				"gender":   acc.Gender,
				"age":      acc.Age,
				"site_id":  cfg.ID,
			}
			bodyBytes, _ := json.Marshal(payload)
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, state.signupEndpoint, bytes.NewReader(bodyBytes))
			if err != nil {
				return "", err
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", "application/json, text/html, */*")
			resp, err := client.Do(req)
			if err != nil {
				return "", fmt.Errorf("POST register failed: %w", err)
			}
			respBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
			_ = resp.Body.Close()

			if resp.StatusCode >= 400 {
				return "", fmt.Errorf("POST register to %s returned HTTP %d", state.signupEndpoint, resp.StatusCode)
			}
			acc.HTTPStatus = resp.StatusCode
			for _, ck := range resp.Cookies() {
				if strings.TrimSpace(ck.Name) != "" && strings.TrimSpace(ck.Value) != "" {
					acc.Cookies[ck.Name] = ck.Value
				}
			}
			var parsed map[string]any
			if json.Unmarshal(respBytes, &parsed) == nil {
				for _, key := range []string{"token", "access_token", "jwt", "session_token"} {
					if v, ok := parsed[key].(string); ok && strings.TrimSpace(v) != "" {
						acc.Token = strings.TrimSpace(v)
						acc.Cookies["token"] = acc.Token
						acc.LocalStore["session_token"] = acc.Token
						break
					}
				}
			}
		} else {
			acc.HTTPStatus = 200
		}
		acc.Status = "registered"
		artifacts["token_prefix"] = acc.Token[:minInt(len(acc.Token), 14)]
		return fmt.Sprintf("registered account %s against %s", acc.ID, state.signupEndpoint), nil

	case StepTokenSave:
		if state.generatedAccount == nil {
			return "", errors.New("no registered account to save")
		}
		acc := state.generatedAccount
		accRel := filepath.Join("accounts", acc.ID+".json")
		accPath, err := siteCtx.Resolve(accRel)
		if err != nil {
			return "", err
		}
		acc.FilePath = accPath

		parsedURL, _ := url.Parse(cfg.StartURL)
		domain := "localhost"
		if parsedURL != nil && parsedURL.Hostname() != "" {
			domain = parsedURL.Hostname()
		}
		cookies := make([]model.Cookie, 0, len(acc.Cookies))
		for k, v := range acc.Cookies {
			cookies = append(cookies, model.Cookie{
				Name:   k,
				Value:  v,
				Domain: domain,
				Path:   "/",
			})
		}
		login := model.LoginInfo{
			UserID:       acc.ID,
			Backend:      "autoreg_http",
			LocalStorage: acc.LocalStore,
			Cookies:      cookies,
		}
		sessionPath, err := SaveSiteSession(siteCtx.DataDir, cfg.ID, profile, login)
		if err != nil {
			return "", err
		}
		acc.LoginPath = sessionPath
		acc.Status = "ready"
		if err := siteCtx.SaveJSON(accRel, acc); err != nil {
			return "", err
		}
		state.capturedLogin = &login
		artifacts["account_file"] = accPath
		artifacts["session_file"] = sessionPath
		return fmt.Sprintf("saved token & session to %s", sessionPath), nil

	case StepChatTest:
		if CountSiteAccounts(siteCtx.DataDir, cfg.ID) == 0 {
			return "", errors.New("chat test failed: no saved account found in sites/<site_id>/accounts")
		}
		hasSession, _, sessionPath := SiteSessionStatus(siteCtx.DataDir, cfg.ID)
		if !hasSession {
			return "", errors.New("chat test failed: no active session found in sites/<site_id>/sessions")
		}
		if shouldProbeLive(*cfg, opts) {
			client := opts.HTTPClient
			if client == nil {
				client = &http.Client{Timeout: 5 * time.Second}
			}
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, cfg.StartURL, nil)
			if err != nil {
				return "", err
			}
			if state.generatedAccount != nil && state.generatedAccount.Token != "" {
				req.Header.Set("Authorization", "Bearer "+state.generatedAccount.Token)
			}
			resp, err := client.Do(req)
			if err != nil {
				return "", fmt.Errorf("chat test request failed: %w", err)
			}
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
			_ = resp.Body.Close()
			if resp.StatusCode >= 400 {
				return "", fmt.Errorf("chat test returned HTTP %d", resp.StatusCode)
			}
		}
		cfg.AccountMode = AccountModeAutoRegister
		cfg.Classification = Classification{
			Group:        GroupB,
			GroupName:    GroupName(GroupB),
			Confidence:   ConfidenceHigh,
			Source:       "recipe_group_B",
			Reason:       fmt.Sprintf("Verified Auto-Signup setup via Group B recipe (signup: %s)", state.signupEndpoint),
			ClassifiedAt: time.Now().UTC().Format(time.RFC3339),
			ProbeCount:   cfg.Classification.ProbeCount + 1,
			Probe:        state.probe,
		}
		toSave := *cfg
		toSave.FolderPath = ""
		if err := siteCtx.SaveJSON("site.json", toSave); err != nil {
			return "", err
		}
		artifacts["session_path"] = sessionPath
		return "verified token and chat readiness for Group B Auto-Signup", nil

	// =========================================================================
	// Group C: Browser-Once (folder create -> login URL detect -> mark "needs browser" -> wait user login -> capture session -> verify browserless)
	// =========================================================================
	case StepLoginURLDetect:
		if shouldProbeLive(*cfg, opts) {
			pr, err := ProbeURL(ctx, cfg.StartURL, opts.HTTPClient)
			if err != nil {
				return "", fmt.Errorf("login URL detect failed: %w", err)
			}
			state.probe = &pr
			if pr.HeavyBotDetection {
				return "", fmt.Errorf("heavy bot detection (%v) blocks Browser-Once setup", pr.BotCheckKeywords)
			}
			if pr.FinalURL != "" {
				state.loginURL = pr.FinalURL
			} else {
				state.loginURL = cfg.StartURL
			}
		} else {
			if cfg.Classification.Group == GroupE {
				return "", fmt.Errorf("site %s is classified as Group E Manual (heavy bot detection)", cfg.ID)
			}
			state.loginURL = cfg.StartURL
		}
		if strings.TrimSpace(state.loginURL) == "" {
			return "", errors.New("could not determine login URL")
		}
		artifacts["login_url"] = state.loginURL
		return fmt.Sprintf("detected login/auth URL %s", state.loginURL), nil

	case StepMarkNeedsBrowser:
		marker := map[string]any{
			"site_id":       cfg.ID,
			"needs_browser": true,
			"login_url":     state.loginURL,
			"profile":       profile,
			"marked_at":     time.Now().UTC().Format(time.RFC3339),
		}
		if err := siteCtx.SaveJSON(filepath.Join("sessions", "needs_browser.json"), marker); err != nil {
			return "", err
		}
		cfg.AccountMode = AccountModeBrowserSetup
		toSave := *cfg
		toSave.FolderPath = ""
		if err := siteCtx.SaveJSON("site.json", toSave); err != nil {
			return "", err
		}
		artifacts["needs_browser"] = "true"
		return fmt.Sprintf("marked site %s as needing one-time browser login", cfg.ID), nil

	case StepWaitUserLogin:
		if opts.UserLoginWaiter != nil {
			login, err := opts.UserLoginWaiter(ctx, *cfg)
			if err != nil {
				return "", fmt.Errorf("user login failed: %w", err)
			}
			if login == nil || (strings.TrimSpace(login.Token()) == "" && len(login.Cookies) == 0 && len(login.LocalStorage) == 0) {
				return "", errors.New("user login returned empty session credentials")
			}
			state.capturedLogin = login
			artifacts["login_source"] = "user_login_waiter"
			return fmt.Sprintf("user login completed for %s", cfg.ID), nil
		}
		// Check if an existing session or account was already captured in sites/<site_id>/.
		if existing, _, ok := LoadSiteSession(siteCtx.DataDir, cfg.ID, profile); ok {
			if strings.TrimSpace(existing.Token()) != "" || len(existing.Cookies) > 0 || len(existing.LocalStorage) > 0 {
				state.capturedLogin = &existing
				artifacts["login_source"] = "existing_saved_session"
				return "loaded existing authenticated browser session", nil
			}
		}
		if accounts, err := ListSiteAccounts(siteCtx.DataDir, cfg.ID); err == nil && len(accounts) > 0 {
			acc := accounts[0]
			if strings.TrimSpace(acc.Token) != "" || len(acc.Cookies) > 0 {
				cookies := make([]model.Cookie, 0, len(acc.Cookies)+1)
				for k, v := range acc.Cookies {
					cookies = append(cookies, model.Cookie{Name: k, Value: v, Path: "/"})
				}
				if acc.Token != "" {
					cookies = append(cookies, model.Cookie{Name: "token", Value: acc.Token, Path: "/"})
				}
				state.capturedLogin = &model.LoginInfo{
					UserID:       acc.ID,
					Backend:      "browser_once",
					LocalStorage: acc.LocalStore,
					Cookies:      cookies,
				}
				artifacts["login_source"] = "existing_site_account"
				return "loaded existing site account credentials", nil
			}
		}
		return "", errors.New("waiting for one-time user browser login: no captured session found yet")

	case StepCaptureSession:
		if state.capturedLogin == nil {
			return "", errors.New("no authenticated session to capture")
		}
		sessionPath, err := SaveSiteSession(siteCtx.DataDir, cfg.ID, profile, *state.capturedLogin)
		if err != nil {
			return "", err
		}
		marker := map[string]any{
			"site_id":       cfg.ID,
			"needs_browser": false,
			"login_url":     state.loginURL,
			"profile":       profile,
			"captured_at":   time.Now().UTC().Format(time.RFC3339),
		}
		_ = siteCtx.SaveJSON(filepath.Join("sessions", "needs_browser.json"), marker)
		artifacts["session_path"] = sessionPath
		return fmt.Sprintf("captured browser session into %s", sessionPath), nil

	case StepVerifyBrowserless:
		saved, _, ok := LoadSiteSession(siteCtx.DataDir, cfg.ID, profile)
		if !ok {
			return "", errors.New("verify browserless failed: no saved session found")
		}
		if strings.TrimSpace(saved.Token()) == "" && len(saved.Cookies) == 0 && len(saved.LocalStorage) == 0 {
			return "", errors.New("verify browserless failed: saved session has no reusable tokens or cookies")
		}
		if shouldProbeLive(*cfg, opts) {
			client := opts.HTTPClient
			if client == nil {
				client = &http.Client{Timeout: 5 * time.Second}
			}
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, cfg.StartURL, nil)
			if err != nil {
				return "", err
			}
			if tok := saved.Token(); tok != "" {
				req.Header.Set("Authorization", "Bearer "+tok)
			}
			for _, ck := range saved.Cookies {
				if ck.Name != "" && ck.Value != "" {
					req.AddCookie(&http.Cookie{Name: ck.Name, Value: ck.Value})
				}
			}
			resp, err := client.Do(req)
			if err != nil {
				return "", fmt.Errorf("verify browserless request failed: %w", err)
			}
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
			_ = resp.Body.Close()
			if resp.StatusCode >= 400 {
				return "", fmt.Errorf("verify browserless returned HTTP %d", resp.StatusCode)
			}
		}
		cfg.AccountMode = AccountModeBrowserSetup
		cfg.Classification = Classification{
			Group:        GroupC,
			GroupName:    GroupName(GroupC),
			Confidence:   ConfidenceHigh,
			Source:       "recipe_group_C",
			Reason:       "Verified Browser-Once setup with captured reusable session",
			ClassifiedAt: time.Now().UTC().Format(time.RFC3339),
			ProbeCount:   cfg.Classification.ProbeCount + 1,
			Probe:        state.probe,
		}
		toSave := *cfg
		toSave.FolderPath = ""
		if err := siteCtx.SaveJSON("site.json", toSave); err != nil {
			return "", err
		}
		artifacts["browserless_ready"] = "true"
		return "verified captured session is ready for browserless reuse", nil

	// =========================================================================
	// Group D: DOM-Only (folder create -> headless profile setup -> DOM selector detect -> save driver -> headless test)
	// =========================================================================
	case StepHeadlessProfileSetup:
		browserDir, err := siteCtx.BrowserProfileDir(profile)
		if err != nil {
			return "", err
		}
		if err := os.MkdirAll(browserDir, 0o700); err != nil {
			return "", err
		}
		metaRel := filepath.Join("profiles", profile, "headless_profile.json")
		meta := map[string]any{
			"site_id":       cfg.ID,
			"profile":       profile,
			"headless":      true,
			"user_data_dir": browserDir,
			"created_at":    time.Now().UTC().Format(time.RFC3339),
		}
		if err := siteCtx.SaveJSON(metaRel, meta); err != nil {
			return "", err
		}
		artifacts["browser_profile_dir"] = browserDir
		return fmt.Sprintf("headless profile initialized at %s", browserDir), nil

	case StepDOMSelectorDetect:
		if shouldProbeLive(*cfg, opts) {
			pr, err := ProbeURL(ctx, cfg.StartURL, opts.HTTPClient)
			if err != nil {
				return "", fmt.Errorf("DOM selector probe failed: %w", err)
			}
			state.probe = &pr
			if pr.HeavyBotDetection {
				return "", fmt.Errorf("heavy bot detection (%v) blocks headless DOM chat", pr.BotCheckKeywords)
			}
		} else if cfg.Classification.Group == GroupE {
			return "", fmt.Errorf("site %s has heavy bot detection (Group E Manual), cannot run headless DOM driver", cfg.ID)
		}

		sel := cfg.Selectors
		fallbackSel := ArchetypeSelectors(cfg.Family)
		if strings.TrimSpace(sel.ComposerInput) == "" {
			sel.ComposerInput = fallbackSel.ComposerInput
		}
		if strings.TrimSpace(sel.SkipButton) == "" {
			sel.SkipButton = fallbackSel.SkipButton
		}
		if strings.TrimSpace(sel.StartButton) == "" {
			sel.StartButton = fallbackSel.StartButton
		}
		if strings.TrimSpace(sel.MessageHistory) == "" {
			sel.MessageHistory = fallbackSel.MessageHistory
		}
		if strings.TrimSpace(sel.MessageRow) == "" {
			sel.MessageRow = fallbackSel.MessageRow
		}
		if strings.TrimSpace(sel.MessageText) == "" {
			sel.MessageText = fallbackSel.MessageText
		}
		cfg.Selectors = sel
		state.detectedSel = &sel
		artifacts["composer_input"] = sel.ComposerInput
		artifacts["message_history"] = sel.MessageHistory
		return fmt.Sprintf("detected DOM selectors (composer=%q)", sel.ComposerInput), nil

	case StepSaveDriver:
		if state.detectedSel != nil {
			cfg.Selectors = *state.detectedSel
		}
		driverJS := UniversalDOMDriverJS(*cfg)
		if err := siteCtx.WriteFile("dom_driver.js", []byte(driverJS), 0o644); err != nil {
			return "", err
		}
		toSave := *cfg
		toSave.FolderPath = ""
		if err := siteCtx.SaveJSON("site.json", toSave); err != nil {
			return "", err
		}
		driverPath, _ := siteCtx.Resolve("dom_driver.js")
		artifacts["dom_driver_path"] = driverPath
		return fmt.Sprintf("saved isolated DOM driver to %s", driverPath), nil

	case StepHeadlessTest:
		rawDriver, err := siteCtx.ReadFile("dom_driver.js")
		if err != nil {
			return "", fmt.Errorf("headless test failed reading dom_driver.js: %w", err)
		}
		driverText := string(rawDriver)
		for _, symbol := range []string{"snapshot", "probe", "click", "focusInput", "readInput", "clearInput"} {
			if !strings.Contains(driverText, symbol) {
				return "", fmt.Errorf("headless test failed: dom_driver.js missing required method %q", symbol)
			}
		}
		browserDir, err := siteCtx.BrowserProfileDir(profile)
		if err != nil {
			return "", err
		}
		if info, statErr := os.Stat(browserDir); statErr != nil || !info.IsDir() {
			return "", fmt.Errorf("headless test failed: browser profile dir %s missing", browserDir)
		}
		cfg.Classification = Classification{
			Group:        GroupD,
			GroupName:    GroupName(GroupD),
			Confidence:   ConfidenceHigh,
			Source:       "recipe_group_D",
			Reason:       "Verified DOM-Only headless profile and dom_driver.js via Group D recipe",
			ClassifiedAt: time.Now().UTC().Format(time.RFC3339),
			ProbeCount:   cfg.Classification.ProbeCount + 1,
			Probe:        state.probe,
		}
		toSave := *cfg
		toSave.FolderPath = ""
		if err := siteCtx.SaveJSON("site.json", toSave); err != nil {
			return "", err
		}
		artifacts["headless_verified"] = "true"
		return "verified headless profile and isolated dom_driver.js", nil

	// =========================================================================
	// Group E: Manual (folder create -> mark manual only)
	// =========================================================================
	case StepMarkManualOnly:
		manualMarker := map[string]any{
			"site_id":     cfg.ID,
			"manual_only": true,
			"reason":      "Heavy bot detection or manual-only workflow required",
			"marked_at":   time.Now().UTC().Format(time.RFC3339),
		}
		if err := siteCtx.SaveJSON(filepath.Join("sessions", "manual_only.json"), manualMarker); err != nil {
			return "", err
		}
		cfg.AccountMode = AccountModeBrowserSetup
		cfg.Classification = Classification{
			Group:        GroupE,
			GroupName:    GroupName(GroupE),
			Confidence:   ConfidenceHigh,
			Source:       "recipe_group_E",
			Reason:       "Marked manual-only via Group E recipe (heavy bot detection / manual operation required)",
			ClassifiedAt: time.Now().UTC().Format(time.RFC3339),
			ProbeCount:   cfg.Classification.ProbeCount + 1,
			Probe:        state.probe,
		}
		toSave := *cfg
		toSave.FolderPath = ""
		if err := siteCtx.SaveJSON("site.json", toSave); err != nil {
			return "", err
		}
		artifacts["manual_only"] = "true"
		return fmt.Sprintf("marked site %s as Group E Manual Only", cfg.ID), nil

	default:
		return "", fmt.Errorf("unknown recipe step %q", step.ID)
	}
}

func detectSignupActionURL(baseURL, htmlBody string, pr ProbeResult) string {
	base, _ := url.Parse(baseURL)
	resolve := func(candidate string) string {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			return ""
		}
		if strings.HasPrefix(strings.ToLower(candidate), "http://") || strings.HasPrefix(strings.ToLower(candidate), "https://") {
			return candidate
		}
		if base != nil {
			if ref, refErr := url.Parse(candidate); refErr == nil {
				return base.ResolveReference(ref).String()
			}
		}
		return candidate
	}

	if m := formActionPattern.FindStringSubmatch(htmlBody); len(m) > 1 {
		if resolved := resolve(m[1]); resolved != "" {
			return resolved
		}
	}
	for _, ep := range pr.APIEndpoints {
		low := strings.ToLower(ep)
		if strings.Contains(low, "register") || strings.Contains(low, "signup") || strings.Contains(low, "guest") || strings.Contains(low, "auth") {
			if resolved := resolve(ep); resolved != "" {
				return resolved
			}
		}
	}
	if len(pr.APIEndpoints) > 0 {
		if resolved := resolve(pr.APIEndpoints[0]); resolved != "" {
			return resolved
		}
	}
	if pr.HasSignupForm {
		return baseURL
	}
	return ""
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
