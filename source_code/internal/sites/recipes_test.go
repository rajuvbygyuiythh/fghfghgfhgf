package sites

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"chitchatbot/internal/model"
)

func TestEnsureRecipesCreatesFiveGroupRecipeFilesWithExpectedSequentialSteps(t *testing.T) {
	dataDir := t.TempDir()
	recipesDir, err := EnsureRecipes(dataDir)
	if err != nil {
		t.Fatalf("EnsureRecipes: %v", err)
	}
	if filepath.Base(recipesDir) != "_recipes" {
		t.Fatalf("recipesDir = %q, want basename _recipes", recipesDir)
	}

	expected := map[string][]string{
		GroupA: {"folder create", "WS detect", "direct connect test", "save config"},
		GroupB: {"folder create", "signup endpoint detect", "fake account generate", "POST register", "token save", "chat test"},
		GroupC: {"folder create", "login URL detect", `mark "needs browser"`, "wait user login", "capture session", "verify browserless"},
		GroupD: {"folder create", "headless profile setup", "DOM selector detect", "save driver", "headless test"},
		GroupE: {"folder create", "mark manual only"},
	}

	expectedFallbacks := map[string]string{
		GroupA: GroupB,
		GroupB: GroupC,
		GroupC: GroupD,
		GroupD: GroupE,
		GroupE: "",
	}

	for _, g := range []string{GroupA, GroupB, GroupC, GroupD, GroupE} {
		fileName := RecipeFileName(g)
		fullPath := filepath.Join(recipesDir, fileName)
		if _, err := os.Stat(fullPath); err != nil {
			t.Fatalf("missing recipe file %s: %v", fullPath, err)
		}
		rec, err := LoadRecipe(dataDir, g)
		if err != nil {
			t.Fatalf("LoadRecipe(%s): %v", g, err)
		}
		if rec.Group != g {
			t.Fatalf("rec.Group = %q, want %q", rec.Group, g)
		}
		if rec.FallbackGroup != expectedFallbacks[g] {
			t.Fatalf("group %s fallback = %q, want %q", g, rec.FallbackGroup, expectedFallbacks[g])
		}
		wantSteps := expected[g]
		if len(rec.Steps) != len(wantSteps) {
			t.Fatalf("group %s steps len = %d, want %d", g, len(rec.Steps), len(wantSteps))
		}
		for i, wantName := range wantSteps {
			if rec.Steps[i].Order != i+1 {
				t.Fatalf("group %s step[%d] order = %d, want %d", g, i, rec.Steps[i].Order, i+1)
			}
			if rec.Steps[i].Name != wantName {
				t.Fatalf("group %s step[%d] name = %q, want %q", g, i, rec.Steps[i].Name, wantName)
			}
		}
	}
}

func TestRecipeExecutorRunsEachGroupStepByStepAndUpdatesLearningSuccessRate(t *testing.T) {
	dataDir := t.TempDir()
	if _, err := EnsureSites(dataDir); err != nil {
		t.Fatalf("EnsureSites: %v", err)
	}

	// 1. Group A live server (WebSocket direct, no signup/captcha/login)
	srvA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><body><script>const ws = new WebSocket("wss://live.omezy.test/ws");</script></body></html>`))
	}))
	defer srvA.Close()

	cfgA, err := LoadSite(dataDir, "omezy")
	if err != nil {
		t.Fatal(err)
	}
	cfgA.StartURL = srvA.URL
	siteCtxA, _ := NewSiteContext(dataDir, "omezy")
	_ = siteCtxA.SaveJSON("site.json", cfgA)

	resA, err := ExecuteSiteRecipe(context.Background(), dataDir, "omezy", RecipeRunOptions{
		Group:      GroupA,
		HTTPClient: srvA.Client(),
	})
	if err != nil {
		t.Fatalf("Group A recipe failed: %v", err)
	}
	if !resA.Success || resA.FinalGroup != GroupA || resA.UsedFallback {
		t.Fatalf("unexpected Group A result: %+v", resA)
	}
	recA, err := LoadRecipe(dataDir, GroupA)
	if err != nil {
		t.Fatal(err)
	}
	if recA.TotalRuns != 1 || recA.SuccessfulRuns != 1 || recA.SuccessRate != 100 {
		t.Fatalf("Group A recipe stats not updated: %+v", recA)
	}
	if recA.Learning.LearnedEndpoints["omezy"] != "wss://live.omezy.test/ws" {
		t.Fatalf("Group A learned endpoint = %q, want wss://live.omezy.test/ws", recA.Learning.LearnedEndpoints["omezy"])
	}

	// 2. Group B live server (Auto-Signup with POST /api/register -> token)
	var srvBURL string
	srvB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/register" && r.Method == http.MethodPost {
			http.SetCookie(w, &http.Cookie{Name: "sid", Value: "cookie_sid_b"})
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"token":"jwt_group_b_verified_token"}`))
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><body><form action="` + srvBURL + `/api/register"><input name="nickname"/><input name="gender"/><input name="age"/></form></body></html>`))
	}))
	defer srvB.Close()
	srvBURL = srvB.URL

	cfgB, err := LoadSite(dataDir, "chatib")
	if err != nil {
		t.Fatal(err)
	}
	cfgB.StartURL = srvB.URL
	siteCtxB, _ := NewSiteContext(dataDir, "chatib")
	_ = siteCtxB.SaveJSON("site.json", cfgB)

	resB, err := ExecuteSiteRecipe(context.Background(), dataDir, "chatib", RecipeRunOptions{
		Group:      GroupB,
		HTTPClient: srvB.Client(),
	})
	if err != nil {
		t.Fatalf("Group B recipe failed: %v", err)
	}
	if !resB.Success || resB.FinalGroup != GroupB || len(resB.Attempts[0].StepResults) != 6 {
		t.Fatalf("unexpected Group B result: %+v", resB)
	}
	recB, _ := LoadRecipe(dataDir, GroupB)
	if recB.SuccessRate != 100 || recB.Learning.SuccessfulRuns != 1 {
		t.Fatalf("Group B learning not updated: %+v", recB.Learning)
	}

	// 3. Group C live server (Browser-Once with UserLoginWaiter)
	srvC := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><body><script src="https://www.google.com/recaptcha/api.js"></script><form action="/login"><input type="password"/></form></body></html>`))
	}))
	defer srvC.Close()

	cfgC, err := LoadSite(dataDir, "chitchat")
	if err != nil {
		t.Fatal(err)
	}
	cfgC.StartURL = srvC.URL
	siteCtxC, _ := NewSiteContext(dataDir, "chitchat")
	_ = siteCtxC.SaveJSON("site.json", cfgC)

	resC, err := ExecuteSiteRecipe(context.Background(), dataDir, "chitchat", RecipeRunOptions{
		Group:      GroupC,
		HTTPClient: srvC.Client(),
		UserLoginWaiter: func(ctx context.Context, siteCfg Config) (*model.LoginInfo, error) {
			return &model.LoginInfo{
				UserID:  "user_chitchat_1",
				Backend: "rod",
				Cookies: []model.Cookie{
					{Name: "token", Value: "jwt_from_one_time_browser_login", Domain: "localhost", Path: "/"},
					{Name: "cc_session", Value: "ok", Domain: "localhost", Path: "/"},
				},
			}, nil
		},
	})
	if err != nil {
		t.Fatalf("Group C recipe failed: %v", err)
	}
	if !resC.Success || resC.FinalGroup != GroupC || len(resC.Attempts[0].StepResults) != 6 {
		t.Fatalf("unexpected Group C result: %+v", resC)
	}

	// Verify logs stayed strictly inside each site's own logs/ directory.
	for _, id := range []string{"omezy", "chatib", "chitchat"} {
		logs, err := ReadSiteRecipeLogs(dataDir, id)
		if err != nil {
			t.Fatalf("ReadSiteRecipeLogs(%s): %v", id, err)
		}
		if len(logs) == 0 {
			t.Fatalf("expected recipe logs in sites/%s/logs/recipe.jsonl", id)
		}
		for _, entry := range logs {
			if entry["site_id"] != id {
				t.Fatalf("site %s log contains foreign site_id %v", id, entry["site_id"])
			}
		}
	}
	// An untouched site must have no recipe.jsonl.
	untouchedLogs, err := ReadSiteRecipeLogs(dataDir, "y99")
	if err != nil {
		t.Fatal(err)
	}
	if len(untouchedLogs) != 0 {
		t.Fatalf("untouched site y99 should have 0 recipe logs, got %d", len(untouchedLogs))
	}
}

func TestRecipeExecutorLogsFailureAndFallsBackBToCToDToE(t *testing.T) {
	dataDir := t.TempDir()
	if _, err := EnsureSites(dataDir); err != nil {
		t.Fatalf("EnsureSites: %v", err)
	}

	// Case 1: Start at Group B on a DOM-Only site (no signup API, no user login session yet).
	// Should fail B (signup_endpoint_detect) -> fallback to C -> fail C (wait_user_login) -> fallback to D -> succeed on D!
	domSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><body><div class="messages"></div><textarea class="chat-input"></textarea><button>Send</button></body></html>`))
	}))
	defer domSrv.Close()

	cfgD, err := LoadSite(dataDir, "chatblink")
	if err != nil {
		t.Fatal(err)
	}
	cfgD.StartURL = domSrv.URL
	siteCtxD, _ := NewSiteContext(dataDir, "chatblink")
	_ = siteCtxD.SaveJSON("site.json", cfgD)

	resFallbackD, err := ExecuteSiteRecipe(context.Background(), dataDir, "chatblink", RecipeRunOptions{
		Group:      GroupB,
		HTTPClient: domSrv.Client(),
	})
	if err != nil {
		t.Fatalf("ExecuteSiteRecipe B->C->D fallback failed: %v", err)
	}
	if !resFallbackD.Success || !resFallbackD.UsedFallback || resFallbackD.FinalGroup != GroupD {
		t.Fatalf("expected final group D via fallback, got %+v", resFallbackD)
	}
	wantHistoryD := "B->C,C->D"
	if got := strings.Join(resFallbackD.FallbackHistory, ","); got != wantHistoryD {
		t.Fatalf("fallback history = %q, want %q", got, wantHistoryD)
	}

	// Case 2: Start at Group B on a Heavy Bot Detection site (Cloudflare 403 challenge).
	// Should fail B -> fallback C -> fail C -> fallback D -> fail D -> fallback E -> succeed on E (mark manual only)!
	botSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cf-Mitigated", "challenge")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`<html><head><title>Just a moment...</title></head><body>window._cf_chl_opt={}; Checking your browser before accessing</body></html>`))
	}))
	defer botSrv.Close()

	cfgE, err := LoadSite(dataDir, "emeraldchat")
	if err != nil {
		t.Fatal(err)
	}
	cfgE.StartURL = botSrv.URL
	siteCtxE, _ := NewSiteContext(dataDir, "emeraldchat")
	_ = siteCtxE.SaveJSON("site.json", cfgE)

	resFallbackE, err := ExecuteSiteRecipe(context.Background(), dataDir, "emeraldchat", RecipeRunOptions{
		Group:      GroupB,
		HTTPClient: botSrv.Client(),
	})
	if err != nil {
		t.Fatalf("ExecuteSiteRecipe B->C->D->E fallback failed: %v", err)
	}
	if !resFallbackE.Success || !resFallbackE.UsedFallback || resFallbackE.FinalGroup != GroupE {
		t.Fatalf("expected final group E via fallback, got %+v", resFallbackE)
	}
	wantHistoryE := "B->C,C->D,D->E"
	if got := strings.Join(resFallbackE.FallbackHistory, ","); got != wantHistoryE {
		t.Fatalf("fallback history = %q, want %q", got, wantHistoryE)
	}

	// Verify emeraldchat's site.json is now marked Group E Manual and logs/recipe.jsonl contains all fallback events.
	updatedE, err := LoadSite(dataDir, "emeraldchat")
	if err != nil {
		t.Fatal(err)
	}
	if updatedE.Classification.Group != GroupE {
		t.Fatalf("emeraldchat classification.group = %q, want E", updatedE.Classification.Group)
	}

	logsE, err := ReadSiteRecipeLogs(dataDir, "emeraldchat")
	if err != nil {
		t.Fatal(err)
	}
	fallbackEvents := 0
	failedStepEvents := 0
	for _, entry := range logsE {
		switch entry["event"] {
		case "recipe_fallback_transition":
			fallbackEvents++
		case "recipe_step_failed":
			failedStepEvents++
		}
	}
	if fallbackEvents != 3 {
		t.Fatalf("expected 3 recipe_fallback_transition events in emeraldchat logs, got %d", fallbackEvents)
	}
	if failedStepEvents != 3 {
		t.Fatalf("expected 3 recipe_step_failed events in emeraldchat logs, got %d", failedStepEvents)
	}

	// Check Group B's recipe stats reflect the 2 failed fallback attempts (0% success rate in this temp dir).
	recB, err := LoadRecipe(dataDir, GroupB)
	if err != nil {
		t.Fatal(err)
	}
	if recB.TotalRuns != 2 || recB.FailedRuns != 2 || recB.SuccessRate != 0 {
		t.Fatalf("expected Group B recipe stats TotalRuns=2 FailedRuns=2 SuccessRate=0, got %+v", recB)
	}
	// Check Group E's recipe stats reflect 1 successful run (100% success rate).
	recE, err := LoadRecipe(dataDir, GroupE)
	if err != nil {
		t.Fatal(err)
	}
	if recE.TotalRuns != 1 || recE.SuccessfulRuns != 1 || recE.SuccessRate != 100 {
		t.Fatalf("expected Group E recipe stats TotalRuns=1 SuccessfulRuns=1 SuccessRate=100, got %+v", recE)
	}
}

func TestWorkspaceRecipesExistOnDisk(t *testing.T) {
	candidates := []string{
		filepath.Clean("../../../sites/_recipes"),
		"/home/user/fghfghgfhgf/sites/_recipes",
	}
	var recipesDir string
	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && info.IsDir() {
			recipesDir = c
			break
		}
	}
	if recipesDir == "" {
		t.Skip("workspace sites/_recipes not found")
	}
	for _, g := range []string{GroupA, GroupB, GroupC, GroupD, GroupE} {
		path := filepath.Join(recipesDir, RecipeFileName(g))
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		var rec Recipe
		if err := json.Unmarshal(raw, &rec); err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		if rec.Group != g || len(rec.Steps) == 0 {
			t.Fatalf("%s invalid recipe: %+v", path, rec)
		}
	}
}
