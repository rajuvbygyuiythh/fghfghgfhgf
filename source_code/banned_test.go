package main

import (
	"os"
	"path/filepath"
	"testing"

	"chitchatbot/internal/browser"
	"chitchatbot/internal/model"
	"chitchatbot/internal/store"
)

func TestHandleBannedDeletesSavedSessions(t *testing.T) {
	a := browserTestApp(t)
	profileDir := filepath.Join(testSiteDataDir(t, a), "profiles", "default")
	if err := store.EnsureDir(profileDir); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"login_info_a.json", "login_info_b.json"} {
		if err := store.SaveJSON(filepath.Join(profileDir, name), model.LoginInfo{
			Cookies: []model.Cookie{{Name: "token", Value: "synthetic-not-a-real-token"}},
		}); err != nil {
			t.Fatal(err)
		}
	}

	a.handleBanned(profileDir, "default", browser.BanReport{
		Banned: true, BanID: "ban-1", Reason: "test reason", Remaining: "Permanent",
	})

	matches, err := filepath.Glob(filepath.Join(profileDir, "login_info*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("banned session files still present: %v", matches)
	}
}

func TestHandleBannedDeletesOnlyTheBannedUserSession(t *testing.T) {
	a := browserTestApp(t)
	profileDir := filepath.Join(testSiteDataDir(t, a), "profiles", "default")
	if err := store.EnsureDir(profileDir); err != nil {
		t.Fatal(err)
	}
	bannedPath := filepath.Join(profileDir, "login_info_banned.json")
	otherPath := filepath.Join(profileDir, "login_info_other.json")
	if err := store.SaveJSON(bannedPath, model.LoginInfo{
		UserID: "user-banned", Cookies: []model.Cookie{{Name: "token", Value: "synthetic-banned"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveJSON(otherPath, model.LoginInfo{
		UserID: "user-other", Cookies: []model.Cookie{{Name: "token", Value: "synthetic-other"}},
	}); err != nil {
		t.Fatal(err)
	}

	a.handleBanned(profileDir, "default", browser.BanReport{
		Banned: true, UserID: "user-banned", BanID: "ban-1", Reason: "test", Remaining: "Permanent",
	})

	if _, err := os.Stat(bannedPath); !os.IsNotExist(err) {
		t.Fatalf("banned user's session was not deleted: %v", err)
	}
	if _, err := os.Stat(otherPath); err != nil {
		t.Fatalf("other account's session was deleted: %v", err)
	}
}
