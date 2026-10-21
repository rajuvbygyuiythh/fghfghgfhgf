package sites

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"chitchatbot/internal/model"
)

func newTestContext(t *testing.T, dataDir, siteID string) SiteContext {
	t.Helper()
	ctx, err := NewSiteContext(dataDir, siteID)
	if err != nil {
		t.Fatalf("NewSiteContext(%q): %v", siteID, err)
	}
	if err := ctx.Ensure(); err != nil {
		t.Fatalf("Ensure(%q): %v", siteID, err)
	}
	return ctx
}

func TestSiteContextLayoutIsInsideOneFolder(t *testing.T) {
	dataDir := t.TempDir()
	ctx := newTestContext(t, dataDir, DefaultSiteID)

	if ctx.SiteID != DefaultSiteID {
		t.Fatalf("site id = %q", ctx.SiteID)
	}
	if filepath.Base(ctx.FolderPath) != DefaultSiteID {
		t.Fatalf("folder = %q", ctx.FolderPath)
	}
	for _, path := range []string{ctx.AccountsPath, ctx.ProfilesPath, ctx.SessionsPath, ctx.LogsPath} {
		if !PathWithin(ctx.FolderPath, path) {
			t.Fatalf("%q escapes %q", path, ctx.FolderPath)
		}
	}
	if PathWithin(ctx.FolderPath, ctx.GlobalRoot) {
		t.Fatalf("global root %q must not be inside the site folder", ctx.GlobalRoot)
	}
	if filepath.Base(ctx.GlobalRoot) != GlobalDirName {
		t.Fatalf("global root = %q", ctx.GlobalRoot)
	}
	for _, rel := range []string{
		"accounts",
		"profiles/default",
		"sessions",
		"logs",
	} {
		if _, err := ctx.Stat(rel); err != nil {
			t.Fatalf("missing %s: %v", rel, err)
		}
	}
}

func TestSafeJoinRejectsAllEscapes(t *testing.T) {
	root := t.TempDir()
	abs, err := filepath.Abs(filepath.Join(root, "outside"))
	if err != nil {
		t.Fatal(err)
	}
	cases := []string{
		"..",
		"../outside",
		"a/../../outside",
		"a/../b", // any ".." is rejected, even when it normalizes inside
		`..\outside`,
		abs,
		"/etc/passwd",
		"//server/share",
		"a\x00b",
		"a\nb",
	}
	for _, rel := range cases {
		if _, err := SafeJoin(root, rel); err == nil {
			t.Fatalf("SafeJoin(%q) accepted an unsafe path", rel)
		} else if !errors.Is(err, ErrUnsafePath) {
			t.Fatalf("SafeJoin(%q) error = %v, want ErrUnsafePath", rel, err)
		}
	}

	for _, rel := range []string{"", ".", "site.json", "profiles/default/login.json"} {
		got, err := SafeJoin(root, rel)
		if err != nil {
			t.Fatalf("SafeJoin(%q) rejected a safe path: %v", rel, err)
		}
		if !PathWithin(root, got) {
			t.Fatalf("SafeJoin(%q) = %q, outside %q", rel, got, root)
		}
	}
}

func TestOneSiteCannotReadOrWriteAnotherSite(t *testing.T) {
	dataDir := t.TempDir()
	a := newTestContext(t, dataDir, "site_a")
	b := newTestContext(t, dataDir, "site_b")

	if a.FolderPath == b.FolderPath {
		t.Fatal("two sites share one folder")
	}
	if err := b.WriteFile("secret.txt", []byte("b-only"), 0o600); err != nil {
		t.Fatalf("write b: %v", err)
	}

	attempts := []struct {
		name string
		fn   func() error
	}{
		{"read traversal", func() error {
			_, err := a.ReadFile(filepath.Join("..", "site_b", "secret.txt"))
			return err
		}},
		{"write traversal", func() error {
			return a.WriteFile(filepath.Join("..", "site_b", "pwn.txt"), []byte("x"), 0o600)
		}},
		{"resolve traversal", func() error {
			_, err := a.Resolve(filepath.Join("..", "site_b"))
			return err
		}},
		{"absolute sibling path", func() error {
			_, err := a.ReadFile(filepath.Join(b.FolderPath, "secret.txt"))
			return err
		}},
		{"global traversal", func() error {
			_, err := a.GlobalResolve(filepath.Join("..", "site_b", "secret.txt"))
			return err
		}},
		{"glob traversal", func() error {
			_, err := a.Glob(filepath.Join("..", "site_b", "*.txt"))
			return err
		}},
	}
	for _, attempt := range attempts {
		if err := attempt.fn(); err == nil {
			t.Fatalf("%s: cross-site access was allowed", attempt.name)
		} else if !errors.Is(err, ErrUnsafePath) {
			t.Fatalf("%s: error = %v, want ErrUnsafePath", attempt.name, err)
		}
	}

	data, err := b.ReadFile("secret.txt")
	if err != nil || string(data) != "b-only" {
		t.Fatalf("site_b content changed: %q err=%v", data, err)
	}
	if _, err := b.Stat("pwn.txt"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("site_b gained a file written by site_a: %v", err)
	}
}

func TestSymlinkEscapeIsRejected(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation needs extra Windows privileges")
	}
	dataDir := t.TempDir()
	a := newTestContext(t, dataDir, "site_a")
	b := newTestContext(t, dataDir, "site_b")
	if err := b.WriteFile("secret.txt", []byte("b-only"), 0o600); err != nil {
		t.Fatal(err)
	}
	linkPath := filepath.Join(a.FolderPath, "link_to_b")
	if err := os.Symlink(b.FolderPath, linkPath); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	if _, err := a.ReadFile(filepath.Join("link_to_b", "secret.txt")); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("read through symlink err = %v, want ErrUnsafePath", err)
	}
	if _, err := a.Resolve(filepath.Join("link_to_b", "new.txt")); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("write through symlink err = %v, want ErrUnsafePath", err)
	}
	if err := a.WriteFile(filepath.Join("link_to_b", "new.txt"), []byte("x"), 0o600); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("write through symlink err = %v, want ErrUnsafePath", err)
	}
}

func TestInvalidSiteIDsAreRejected(t *testing.T) {
	dataDir := t.TempDir()
	for _, id := range []string{"", ".", "..", "../evil", "a/b", `a\b`, GlobalDirName, RecipesDirName, "Sites", "Chatib"} {
		if _, err := NewSiteContext(dataDir, id); !errors.Is(err, ErrInvalidSiteID) {
			t.Fatalf("NewSiteContext(%q) err = %v, want ErrInvalidSiteID", id, err)
		}
	}
	for _, id := range []string{DefaultSiteID, "omezy", "y99_in", "template_new_site"} {
		if _, err := NewSiteContext(dataDir, id); err != nil {
			t.Fatalf("NewSiteContext(%q): %v", id, err)
		}
	}
}

func TestEnsureSitesCreatesOneFolderPerSiteAndOnlyGlobalShares(t *testing.T) {
	dataDir := t.TempDir()
	root, err := EnsureSites(dataDir)
	if err != nil {
		t.Fatalf("EnsureSites: %v", err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	dirCount := 0
	sharedCount := 0
	recipesCount := 0
	seenFolders := map[string]bool{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if entry.Name() == GlobalDirName {
			// The one shared folder lives inside the sites root, next to the
			// site folders, and is never treated as a site.
			sharedCount++
			continue
		}
		if entry.Name() == RecipesDirName {
			recipesCount++
			continue
		}
		dirCount++
		id := entry.Name()
		ctx, err := NewSiteContext(dataDir, id)
		if err != nil {
			t.Fatalf("context for %q: %v", id, err)
		}
		if ctx.FolderPath != filepath.Join(root, id) {
			t.Fatalf("%s folder = %q", id, ctx.FolderPath)
		}
		if seenFolders[ctx.FolderPath] {
			t.Fatalf("duplicate site folder %q", ctx.FolderPath)
		}
		seenFolders[ctx.FolderPath] = true
		for _, rel := range []string{"site.json", "dom_driver.js", "accounts", "profiles/default", "sessions", "logs"} {
			if _, err := ctx.Stat(rel); err != nil {
				t.Fatalf("%s missing %s: %v", id, rel, err)
			}
		}
	}
	if dirCount != 102 {
		t.Fatalf("expected 102 site folders, got %d", dirCount)
	}
	if sharedCount != 1 {
		t.Fatalf("expected exactly one shared %s folder in the sites root, got %d", GlobalDirName, sharedCount)
	}
	if recipesCount != 1 {
		t.Fatalf("expected exactly one %s folder in the sites root, got %d", RecipesDirName, recipesCount)
	}
	for _, shared := range []string{"profiles", "ui", "bot_registry.json"} {
		if _, err := os.Stat(filepath.Join(dataDir, shared)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("legacy shared path %q must not be created (err=%v)", shared, err)
		}
	}
	for _, global := range []string{"", "ui", "logs", "memory", "settings"} {
		if info, err := os.Stat(filepath.Join(GlobalDir(dataDir), global)); err != nil || !info.IsDir() {
			t.Fatalf("missing _global/%s: %v", global, err)
		}
	}
}

func TestCleanSiteRuntimeCannotTouchAnotherSite(t *testing.T) {
	dataDir := t.TempDir()
	a := newTestContext(t, dataDir, "site_a")
	b := newTestContext(t, dataDir, "site_b")
	if err := a.WriteFile(filepath.Join("profiles", "default", "keep.txt"), []byte("a"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := b.WriteFile(filepath.Join("profiles", "default", "keep.txt"), []byte("b"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := CleanSiteRuntime(dataDir, "site_a"); err != nil {
		t.Fatalf("CleanSiteRuntime: %v", err)
	}
	if _, err := a.Stat(filepath.Join("profiles", "default", "keep.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("site_a profile was not cleaned: %v", err)
	}
	data, err := b.ReadFile(filepath.Join("profiles", "default", "keep.txt"))
	if err != nil || string(data) != "b" {
		t.Fatalf("site_b was touched by cleaning site_a: %q err=%v", data, err)
	}
}

func TestSessionAndAccountFilesStayInsideSite(t *testing.T) {
	dataDir := t.TempDir()
	if _, err := EnsureSites(dataDir); err != nil {
		t.Fatal(err)
	}
	ctx := newTestContext(t, dataDir, "chatib")
	login := model.LoginInfo{
		UserID:       "chatib_test_user",
		Backend:      "rod",
		LocalStorage: map[string]string{"guest_nick": "Sophia21"},
		Cookies:      []model.Cookie{{Name: "chatib_sid", Value: "sid_test"}},
	}
	sessionPath, err := SaveSiteSession(dataDir, "chatib", "default", login)
	if err != nil {
		t.Fatalf("SaveSiteSession: %v", err)
	}
	if !PathWithin(ctx.FolderPath, sessionPath) {
		t.Fatalf("session path %q escapes %q", sessionPath, ctx.FolderPath)
	}
	accounts, err := ListSiteAccounts(dataDir, "chatib")
	if err != nil {
		t.Fatal(err)
	}
	if len(accounts) == 0 {
		t.Fatal("expected an account file")
	}
	for _, account := range accounts {
		if !PathWithin(ctx.FolderPath, account.FilePath) {
			t.Fatalf("account path %q escapes site", account.FilePath)
		}
	}
	if _, _, ok := LoadSiteSession(dataDir, "omezy", "default"); ok {
		t.Fatal("omezy must not see chatib's session")
	}
}

func TestSiteIDsCannotAliasThroughSanitization(t *testing.T) {
	dataDir := t.TempDir()
	ctx := newTestContext(t, dataDir, "chatib")
	if strings.HasSuffix(ctx.FolderPath, "chatib") == false {
		t.Fatalf("unexpected folder %q", ctx.FolderPath)
	}
	// The dangerous forms must error rather than silently becoming "chatib".
	for _, id := range []string{"../chatib", "chatib/", "chatib\\", "./chatib"} {
		if _, err := NewSiteContext(dataDir, id); err == nil {
			t.Fatalf("unsafe alias %q was accepted", id)
		}
	}
}

// LoadSite must stay read-only. A dashboard refresh or an active-site lookup
// runs on every screen and must never create folders for the other 101 sites.
func TestLoadSiteIsReadOnly(t *testing.T) {
	dataDir := t.TempDir()
	cfg, err := LoadSite(dataDir, DefaultSiteID)
	if err != nil {
		t.Fatalf("LoadSite: %v", err)
	}
	if cfg.ID != DefaultSiteID || cfg.FolderPath == "" {
		t.Fatalf("unexpected config: %#v", cfg)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "sites")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("LoadSite created sites/: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "_global")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("LoadSite created _global/: %v", err)
	}
}
