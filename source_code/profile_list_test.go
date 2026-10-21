package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"chitchatbot/internal/model"
	"chitchatbot/internal/store"
)

func TestListProfilesShowsSavedSession(t *testing.T) {
	a := browserTestApp(t)
	defaultDir := filepath.Join(testSiteDataDir(t, a), "profiles", "default")
	if err := store.EnsureDir(defaultDir); err != nil {
		t.Fatal(err)
	}
	login := model.LoginInfo{
		SavedAt: time.Now().UTC(),
		Cookies: []model.Cookie{{Name: "token", Value: "synthetic-not-a-real-token"}},
	}
	if err := store.SaveJSON(filepath.Join(defaultDir, "login_info_test.json"), login); err != nil {
		t.Fatal(err)
	}
	secondDir := filepath.Join(testSiteDataDir(t, a), "profiles", "second")
	if err := store.EnsureDir(secondDir); err != nil {
		t.Fatal(err)
	}
	workDir := filepath.Join(testSiteDataDir(t, a), "profiles", "work")
	if err := store.EnsureDir(workDir); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveJSON(filepath.Join(workDir, "login_info_work.json"), login); err != nil {
		t.Fatal(err)
	}

	options, err := a.ListProfiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(options) != 3 || options[0].Name != "default" {
		t.Fatalf("profiles=%+v", options)
	}
	byName := map[string]ProfileOption{}
	for _, option := range options {
		byName[option.Name] = option
	}
	if !byName["default"].HasSession || byName["default"].SavedAt == "" {
		t.Fatalf("default session metadata=%+v", byName["default"])
	}
	if !byName["work"].HasSession || byName["work"].SavedAt == "" {
		t.Fatalf("work profile=%+v", byName["work"])
	}
	if byName["second"].HasSession {
		t.Fatalf("second profile must have no session: %+v", byName["second"])
	}
}

func TestLatestProfileLoginPrefersNewestFile(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "login_info_old.json")
	newPath := filepath.Join(dir, "login_info_new.json")
	if err := store.SaveJSON(oldPath, model.LoginInfo{Cookies: []model.Cookie{{Name: "token", Value: "old"}}}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveJSON(newPath, model.LoginInfo{Cookies: []model.Cookie{{Name: "token", Value: "new"}}}); err != nil {
		t.Fatal(err)
	}
	oldTime := time.Now().Add(-time.Hour)
	if err := os.Chtimes(oldPath, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}
	login, path, ok := latestProfileLogin(dir)
	if !ok || path != newPath || login.Token() != "new" {
		t.Fatalf("latest login=%+v path=%s ok=%t", login, path, ok)
	}
}

func TestLatestProfileLoginSkipsFilesWithoutToken(t *testing.T) {
	dir := t.TempDir()
	if err := store.SaveJSON(filepath.Join(dir, "login_info_empty.json"), model.LoginInfo{}); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := latestProfileLogin(dir); ok {
		t.Fatal("session without token must not be restoreable")
	}
}
