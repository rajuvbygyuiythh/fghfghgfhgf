package sites

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"chitchatbot/internal/model"
)

// SiteSessionsDir returns the isolated session store directory for a site:
// <sites-root>/<site_id>/sessions. It returns "" for an invalid site ID.
func SiteSessionsDir(dataDir, siteID string) string {
	ctx, err := NewSiteContext(dataDir, siteID)
	if err != nil {
		return ""
	}
	return ctx.SessionsPath
}

// SaveSiteSession persists a browser session inside exactly one site folder:
//
//	<sites-root>/<site_id>/sessions/latest_session.json
//	<sites-root>/<site_id>/sessions/session_<profile>.json
//	<sites-root>/<site_id>/profiles/<profile>/login_info_<site>_<profile>.json
//	<sites-root>/<site_id>/accounts/account_<user>.json
//
// There is no legacy mirror into <dataDir>/profiles/: a session for one site can
// never be read or written by another site.
func SaveSiteSession(dataDir, siteID, profile string, login model.LoginInfo) (string, error) {
	ctx, err := NewSiteContext(dataDir, siteID)
	if err != nil {
		return "", err
	}
	profileName, err := safeSegment(profile, "default")
	if err != nil {
		return "", err
	}
	if login.SavedAt.IsZero() {
		login.SavedAt = time.Now().UTC()
	}
	login.Profile = profileName
	if strings.TrimSpace(login.Backend) == "" {
		login.Backend = "rod"
	}
	if login.LocalStorage == nil {
		login.LocalStorage = map[string]string{}
	}
	login.LocalStorage["site_id"] = ctx.SiteID

	userID := strings.TrimSpace(login.CurrentUserID())
	if userID == "" {
		userID = fmt.Sprintf("%s_%s", ctx.SiteID, profileName)
	}
	login.UserID = userID
	if strings.TrimSpace(login.LocalStorage["cachedUserId"]) == "" {
		login.LocalStorage["cachedUserId"] = userID
	}
	if strings.TrimSpace(login.Token()) == "" {
		login.Cookies = append(login.Cookies, model.Cookie{
			Name:   "token",
			Value:  fmt.Sprintf("site_session_%s_%s_%d", ctx.SiteID, profileName, login.SavedAt.Unix()),
			Domain: ctx.SiteID,
			Path:   "/",
		})
	}

	if err := ctx.Ensure(); err != nil {
		return "", err
	}
	if err := ctx.EnsureDir("sessions"); err != nil {
		return "", err
	}
	profileDir, err := ctx.ProfilePath(profileName)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(profileDir, 0o700); err != nil {
		return "", err
	}

	latestPath, err := ctx.LatestSessionPath()
	if err != nil {
		return "", err
	}
	if err := ctx.SaveJSON(filepath.Join("sessions", "latest_session.json"), login); err != nil {
		return "", err
	}
	if err := ctx.SaveJSON(filepath.Join("sessions", "session_"+profileName+".json"), login); err != nil {
		return "", err
	}
	loginRel := filepath.Join("profiles", profileName, fmt.Sprintf("login_info_%s_%s.json", ctx.SiteID, profileName))
	loginPath, err := ctx.Resolve(loginRel)
	if err != nil {
		return "", err
	}
	if err := ctx.SaveJSON(loginRel, login); err != nil {
		return "", err
	}

	if _, err := RecordBrowserSetupAccount(dataDir, ctx.SiteID, profileName, login, loginPath); err != nil {
		return "", err
	}
	_ = AppendSiteDebugLog(dataDir, ctx.SiteID, "session_autosave", "info",
		fmt.Sprintf("Auto-saved isolated session for %s (profile=%s)", ctx.SiteID, profileName), latestPath)
	return latestPath, nil
}

// LoadSiteSession loads the newest saved session strictly from this site's own
// folder. It never reads data/profiles, data/sites or another site.
func LoadSiteSession(dataDir, siteID, profile string) (model.LoginInfo, string, bool) {
	ctx, err := NewSiteContext(dataDir, siteID)
	if err != nil {
		return model.LoginInfo{}, "", false
	}
	profileName, err := safeSegment(profile, "default")
	if err != nil {
		return model.LoginInfo{}, "", false
	}

	candidates := []string{}
	if path, err := ctx.SessionPath(profileName); err == nil {
		candidates = append(candidates, path)
	}
	if path, err := ctx.LatestSessionPath(); err == nil {
		candidates = append(candidates, path)
	}
	profilePattern := filepath.Join("profiles", profileName, "login_info*.json")
	if matches, err := ctx.Glob(profilePattern); err == nil {
		sortFilesNewestFirst(matches)
		candidates = append(candidates, matches...)
	}
	anyProfilePattern := filepath.Join("profiles", "*", "login_info*.json")
	if matches, err := ctx.Glob(anyProfilePattern); err == nil {
		sortFilesNewestFirst(matches)
		candidates = append(candidates, matches...)
	}

	seen := map[string]bool{}
	for _, path := range candidates {
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		rel, relErr := filepath.Rel(ctx.FolderPath, path)
		if relErr != nil {
			continue
		}
		var login model.LoginInfo
		ok, err := ctx.LoadJSON(rel, &login)
		if err != nil || !ok {
			continue
		}
		if login.Token() != "" || len(login.Cookies) > 0 || len(login.LocalStorage) > 0 {
			return login, path, true
		}
	}

	// Fallback: accounts stored in this site's accounts/ directory.
	accounts, err := ListSiteAccounts(dataDir, ctx.SiteID)
	if err == nil && len(accounts) > 0 {
		acc := accounts[0]
		savedAt, _ := time.Parse(time.RFC3339, acc.CreatedAt)
		if savedAt.IsZero() {
			savedAt = time.Now().UTC()
		}
		cookies := make([]model.Cookie, 0, len(acc.Cookies)+1)
		for k, v := range acc.Cookies {
			cookies = append(cookies, model.Cookie{Name: k, Value: v, Path: "/"})
		}
		if acc.Token != "" {
			cookies = append(cookies, model.Cookie{Name: "token", Value: acc.Token, Path: "/"})
		}
		login := model.LoginInfo{
			Profile:      acc.Profile,
			Backend:      "rod",
			UserID:       acc.ID,
			SavedAt:      savedAt,
			Cookies:      cookies,
			LocalStorage: acc.LocalStore,
		}
		filePath := strings.TrimSpace(acc.FilePath)
		if filePath == "" {
			if p, pErr := ctx.Resolve(filepath.Join("accounts", fmt.Sprintf("account_%s.json", acc.ID))); pErr == nil {
				filePath = p
			}
		}
		return login, filePath, true
	}

	return model.LoginInfo{}, "", false
}

// SiteSessionStatus returns whether a site has a saved session in its isolated
// folder, when it was saved, and the site-relative session store path.
func SiteSessionStatus(dataDir, siteID string) (hasSession bool, savedAt string, sessionPath string) {
	id, err := validateSiteID(siteID)
	if err != nil {
		return false, "", ""
	}
	relPath := filepath.ToSlash(filepath.Join("sites", id, "sessions", "latest_session.json"))
	login, path, ok := LoadSiteSession(dataDir, id, "default")
	if !ok {
		return false, "", relPath
	}
	stamp := ""
	if !login.SavedAt.IsZero() {
		stamp = login.SavedAt.UTC().Format("2006-01-02 15:04 UTC")
	} else if info, err := os.Stat(path); err == nil {
		stamp = info.ModTime().UTC().Format("2006-01-02 15:04 UTC")
	}
	return true, stamp, relPath
}

func sortFilesNewestFirst(paths []string) {
	sort.Slice(paths, func(i, j int) bool {
		left, lErr := os.Stat(paths[i])
		right, rErr := os.Stat(paths[j])
		if lErr != nil || rErr != nil {
			return paths[i] > paths[j]
		}
		return left.ModTime().After(right.ModTime())
	})
}
