package sites

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"chitchatbot/internal/model"
)

var femaleGuestAliases = []string{
	"Sophia", "Emma", "Olivia", "Mia", "Chloe",
	"Ava", "Lily", "Zoe", "Hannah", "Maya",
	"Ella", "Grace", "Bella", "Ruby", "Aria",
	"Nora", "Lila", "Sienna", "Elena", "lara",
}

// SiteAccount represents an isolated account/session saved inside
// sites/<site_id>/accounts/ and mirrored to sites/<site_id>/profiles/<profile>/.
type SiteAccount struct {
	ID          string            `json:"id"`
	SiteID      string            `json:"site_id"`
	SiteName    string            `json:"site_name"`
	AccountMode string            `json:"account_mode"`
	Profile     string            `json:"profile"`
	Nickname    string            `json:"nickname"`
	Gender      string            `json:"gender"`
	Age         int               `json:"age"`
	Token       string            `json:"token"`
	Cookies     map[string]string `json:"cookies,omitempty"`
	LocalStore  map[string]string `json:"local_storage,omitempty"`
	HTTPStatus  int               `json:"http_status,omitempty"`
	Status      string            `json:"status"`
	CreatedAt   string            `json:"created_at"`
	FilePath    string            `json:"file_path,omitempty"`
	LoginPath   string            `json:"login_path,omitempty"`
}

func randomHex(bytesLen int) string {
	buf := make([]byte, bytesLen)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%x", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}

func generateGuestAlias(index int) (string, int) {
	base := femaleGuestAliases[(index+int(time.Now().UnixNano()%100))%len(femaleGuestAliases)]
	age := 20 + (index % 4)
	suffix := 10 + (int(time.Now().UnixNano()/1000)+index*7)%89
	return fmt.Sprintf("%s%d", base, suffix), age
}

// AutoRegisterSiteAccounts registers 1..count guest/session accounts for a site
// WITHOUT launching any browser, saving all account & session files strictly
// inside sites/<site_id>/accounts/ and sites/<site_id>/profiles/<profile>/.
func AutoRegisterSiteAccounts(
	ctx context.Context,
	dataDir string,
	siteID string,
	profile string,
	count int,
	probeHTTP bool,
) ([]SiteAccount, error) {
	if count < 1 || count > 20 {
		return nil, fmt.Errorf("auto-register count must be between 1 and 20")
	}
	cfg, err := LoadSite(dataDir, siteID)
	if err != nil {
		return nil, err
	}
	profile = strings.TrimSpace(profile)
	if profile == "" {
		profile = "default"
	}

	parsedURL, _ := url.Parse(cfg.StartURL)
	domain := "localhost"
	if parsedURL != nil && parsedURL.Hostname() != "" {
		domain = parsedURL.Hostname()
	}

	userAgent := "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
	created := make([]SiteAccount, 0, count)

	for i := 1; i <= count; i++ {
		nickname, age := generateGuestAlias(i)
		token := "autoreg_" + cfg.ID + "_" + randomHex(12)
		accountID := fmt.Sprintf("%s_%s_%s", cfg.ID, strings.ToLower(nickname), randomHex(4))

		cookieMap := map[string]string{
			"token":        token,
			"guest_nick":   nickname,
			"guest_gender": "female",
			"guest_age":    fmt.Sprintf("%d", age),
			"age_verified": "1",
		}
		httpStatus := 200
		if probeHTTP && strings.HasPrefix(strings.ToLower(cfg.StartURL), "http") {
			reqCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			req, reqErr := http.NewRequestWithContext(reqCtx, http.MethodGet, cfg.StartURL, nil)
			if reqErr == nil {
				req.Header.Set("User-Agent", userAgent)
				req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
				client := &http.Client{Timeout: 3 * time.Second}
				if resp, respErr := client.Do(req); respErr == nil {
					httpStatus = resp.StatusCode
					for _, c := range resp.Cookies() {
						if strings.TrimSpace(c.Name) != "" && strings.TrimSpace(c.Value) != "" {
							cookieMap[c.Name] = c.Value
						}
					}
					_ = resp.Body.Close()
				}
			}
			cancel()
		}

		localStore := map[string]string{
			"cachedUserId":            accountID,
			"chitchat_guest_nickname": nickname,
			"chitchat_guest_gender":   "female",
			"chitchat_guest_age":      fmt.Sprintf("%d", age),
			"guest_nick":              nickname,
			"guest_gender":            "female",
			"age_verified":            "true",
			"terms_accepted":          "true",
		}

		cookies := make([]model.Cookie, 0, len(cookieMap))
		for k, v := range cookieMap {
			cookies = append(cookies, model.Cookie{
				Name:   k,
				Value:  v,
				Domain: domain,
				Path:   "/",
				Secure: strings.HasPrefix(strings.ToLower(cfg.StartURL), "https"),
			})
		}

		now := time.Now().UTC()
		loginInfo := model.LoginInfo{
			UserID:         accountID,
			Profile:        profile,
			Backend:        "auto_register",
			Cookies:        cookies,
			UserAgent:      userAgent,
			LocalStorage:   localStore,
			SessionStorage: map[string]string{"cachedUserId": accountID},
			SavedAt:        now,
		}

		acc := SiteAccount{
			ID:          accountID,
			SiteID:      cfg.ID,
			SiteName:    cfg.Name,
			AccountMode: AccountModeAutoRegister,
			Profile:     profile,
			Nickname:    nickname,
			Gender:      "female",
			Age:         age,
			Token:       token,
			Cookies:     cookieMap,
			LocalStore:  localStore,
			HTTPStatus:  httpStatus,
			Status:      "ready",
			CreatedAt:   now.Format(time.RFC3339),
		}

		if err := persistSiteAccountAndLogin(dataDir, cfg.ID, profile, acc, loginInfo, &acc); err != nil {
			return nil, err
		}
		created = append(created, acc)
	}

	return created, nil
}

// RecordBrowserSetupAccount records a browser-captured account inside
// sites/<site_id>/accounts/ and mirrors the profile session across roots so all
// files for that site stay inside its own sites/<site_id>/ folder.
func RecordBrowserSetupAccount(
	dataDir string,
	siteID string,
	profile string,
	login model.LoginInfo,
	loginPath string,
) (SiteAccount, error) {
	cfg, err := LoadSite(dataDir, siteID)
	if err != nil {
		return SiteAccount{}, err
	}
	profile = strings.TrimSpace(profile)
	if profile == "" {
		profile = "default"
	}
	userID := strings.TrimSpace(login.CurrentUserID())
	if userID == "" {
		userID = fmt.Sprintf("browser_%s_%s", cfg.ID, randomHex(4))
		login.UserID = userID
	}
	nickname := userID
	if login.LocalStorage != nil {
		if n := strings.TrimSpace(login.LocalStorage["chitchat_guest_nickname"]); n != "" {
			nickname = n
		}
	}
	token := login.Token()
	if token == "" {
		token = "browser_session_" + randomHex(8)
	}
	cookieMap := map[string]string{}
	for _, c := range login.Cookies {
		if c.Name != "" {
			cookieMap[c.Name] = c.Value
		}
	}
	now := time.Now().UTC()
	acc := SiteAccount{
		ID:          userID,
		SiteID:      cfg.ID,
		SiteName:    cfg.Name,
		AccountMode: AccountModeBrowserSetup,
		Profile:     profile,
		Nickname:    nickname,
		Gender:      "female",
		Age:         21,
		Token:       token,
		Cookies:     cookieMap,
		LocalStore:  login.LocalStorage,
		HTTPStatus:  200,
		Status:      "ready",
		CreatedAt:   now.Format(time.RFC3339),
		LoginPath:   loginPath,
	}
	if err := persistSiteAccountAndLogin(dataDir, cfg.ID, profile, acc, login, &acc); err != nil {
		return SiteAccount{}, err
	}
	return acc, nil
}

func persistSiteAccountAndLogin(
	dataDir string,
	siteID string,
	profile string,
	acc SiteAccount,
	login model.LoginInfo,
	outAcc *SiteAccount,
) error {
	ctx, err := NewSiteContext(dataDir, siteID)
	if err != nil {
		return err
	}
	profileName, err := safeSegment(profile, "default")
	if err != nil {
		return err
	}
	if err := ctx.Ensure(); err != nil {
		return err
	}
	if err := ctx.EnsureDir("accounts"); err != nil {
		return err
	}
	profileDir, err := ctx.ProfilePath(profileName)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(profileDir, 0o700); err != nil {
		return err
	}

	accFileName := fmt.Sprintf("account_%s", SanitizeID(acc.ID))
	loginFileName := fmt.Sprintf("login_info_%s.json", SanitizeID(acc.ID))
	accPath, err := ctx.AccountPath(accFileName)
	if err != nil {
		return err
	}
	loginPath, err := ctx.Resolve(filepath.Join("profiles", profileName, loginFileName))
	if err != nil {
		return err
	}
	acc.FilePath = accPath
	acc.LoginPath = loginPath
	if err := ctx.SaveJSON(filepath.Join("accounts", filepath.Base(accPath)), acc); err != nil {
		return err
	}
	if err := ctx.SaveJSON(filepath.Join("profiles", profileName, loginFileName), login); err != nil {
		return err
	}
	if outAcc != nil {
		*outAcc = acc
	}
	return nil
}

// ListSiteAccounts returns all saved accounts inside this site's accounts/
// directory. It never scans another site's folder.
func ListSiteAccounts(dataDir, siteID string) ([]SiteAccount, error) {
	ctx, err := NewSiteContext(dataDir, siteID)
	if err != nil {
		return nil, err
	}
	entries, err := ctx.ReadDir("accounts")
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	seen := map[string]bool{}
	out := make([]SiteAccount, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".json") {
			continue
		}
		rel := filepath.Join("accounts", entry.Name())
		var acc SiteAccount
		ok, loadErr := ctx.LoadJSON(rel, &acc)
		if loadErr != nil || !ok || acc.ID == "" {
			continue
		}
		if seen[acc.ID] {
			continue
		}
		seen[acc.ID] = true
		acc.FilePath, _ = ctx.Resolve(rel)
		out = append(out, acc)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt > out[j].CreatedAt
	})
	return out, nil
}

// CountSiteAccounts returns the number of saved accounts for a site. It counts
// only files inside <sites-root>/<site_id>/accounts/.
func CountSiteAccounts(dataDir, siteID string) int {
	list, err := ListSiteAccounts(dataDir, siteID)
	if err != nil {
		return 0
	}
	return len(list)
}
