package settings

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"chitchatbot/internal/appdirs"
	"chitchatbot/internal/proxy"
	"chitchatbot/internal/sites"
)

const (
	// GlobalFileName is the shared settings file inside _global/.
	GlobalFileName = "settings.json"
	// OverrideFileName is the per-site override file inside sites/<id>/.
	OverrideFileName = "settings_override.json"
	// MaxDurationSeconds bounds both delay windows (24h).
	MaxDurationSeconds = 86400
	// DefaultAPIBaseURL matches the API Chat default endpoint.
	DefaultAPIBaseURL = "https://api.featherless.ai/v1"
)

// parseProxyEntries is a thin wrapper so validation lives in one place and the
// settings package keeps a single credential-parsing dependency.
func parseProxyEntries(raw string) ([]proxy.Entry, error) {
	entries, err := proxy.ParseList(raw)
	if err != nil {
		return nil, fmt.Errorf("proxy list: %w", err)
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("proxy list is enabled but empty")
	}
	return entries, nil
}

// Store reads and writes the shared settings file and per-site overrides.
//
// It is safe for concurrent use: saves are serialised by a mutex and written to
// a temporary file before being renamed, so a crash can never leave a truncated
// settings.json behind. Reads always return fully normalised values.
type Store struct {
	dataDir string
	mu      sync.Mutex
}

// Open returns a store rooted at dataDir, creating the shared directory tree
// (and migrating a Phase-1 <dataDir>/_global tree) on first use.
func Open(dataDir string) (*Store, error) {
	clean := filepath.Clean(strings.TrimSpace(dataDir))
	if clean == "" || clean == "." {
		return nil, errors.New("settings: empty data directory")
	}
	if err := appdirs.EnsureGlobalDirs(clean); err != nil {
		return nil, err
	}
	return &Store{dataDir: clean}, nil
}

// DataDir returns the runtime data directory this store was opened with.
func (s *Store) DataDir() string { return s.dataDir }

// GlobalPath returns <sites-root>/_global/settings.json.
func (s *Store) GlobalPath() string { return appdirs.SettingsPath(s.dataDir) }

// OverridePath returns <sites-root>/<site_id>/settings_override.json. The site
// ID is validated by sites.NewSiteContext, so a hostile ID cannot escape the
// sites root.
func (s *Store) OverridePath(siteID string) (string, error) {
	ctx, err := sites.NewSiteContext(s.dataDir, siteID)
	if err != nil {
		return "", err
	}
	return ctx.Resolve(OverrideFileName)
}

// Paths returns the global file and the override file for one site, so the
// dashboard can show both without exposing credentials.
func (s *Store) Paths(siteID string) map[string]string {
	out := map[string]string{"global": s.GlobalPath()}
	if path, err := s.OverridePath(siteID); err == nil {
		out["override"] = path
	}
	return out
}

// Load reads the global settings. A missing or unreadable file is not an error:
// the defaults are returned so the app always starts.
func (s *Store) Load() (Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadLocked()
}

func (s *Store) loadLocked() (Settings, error) {
	var loaded Settings
	data, err := os.ReadFile(s.GlobalPath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Default(), nil
		}
		return Default(), err
	}
	if err := json.Unmarshal(data, &loaded); err != nil {
		return Default(), fmt.Errorf("settings.json is not valid JSON: %w", err)
	}
	return loaded.Normalize(), nil
}

// Save validates, normalises and atomically writes the global settings. It
// returns the stored value so the dashboard can render exactly what is on disk.
func (s *Store) Save(settings Settings) (Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Reject an unknown mode instead of silently falling back to the default:
	// a typo must not change which engine a site runs.
	if raw := strings.TrimSpace(settings.MessageMode); raw != "" && NormalizeMessageMode(raw) == "" {
		return Settings{}, fmt.Errorf("unknown message mode %q", raw)
	}
	// Validate the submitted windows before normalising them; otherwise an
	// inverted range would be "repaired" and the user would never learn.
	if err := settings.ReplyDelay.Validate("reply delay"); err != nil {
		return Settings{}, err
	}
	if err := settings.SkipTimeout.Validate("skip timeout"); err != nil {
		return Settings{}, err
	}
	if err := settings.FirstMessage.Validate(); err != nil {
		return Settings{}, err
	}
	normalized := settings.Normalize()
	// Preserve a key the form left blank but that is already stored: the
	// dashboard never re-displays an API key, so an empty field means "keep".
	if !normalized.API.Configured() {
		current, err := s.loadLocked()
		if err != nil {
			return Settings{}, err
		}
		if normalized.API.BaseURL == current.API.BaseURL && current.API.APIKey != "" {
			normalized.API.APIKey = current.API.APIKey
		}
	}
	if err := normalized.Validate(); err != nil {
		return Settings{}, err
	}
	normalized.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if err := writeJSONAtomic(s.GlobalPath(), normalized); err != nil {
		return Settings{}, err
	}
	return normalized, nil
}

// LoadOverride reads one site's override. A site without an override returns a
// zero Override (every field inherits global), not an error.
func (s *Store) LoadOverride(siteID string) (Override, error) {
	path, err := s.OverridePath(siteID)
	if err != nil {
		return Override{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var loaded Override
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Override{}, nil
		}
		return Override{}, err
	}
	if err := json.Unmarshal(data, &loaded); err != nil {
		return Override{}, fmt.Errorf("%s is not valid JSON: %w", OverrideFileName, err)
	}
	return loaded.Normalize(), nil
}

// SaveOverride stores a per-site override. An all-nil override deletes the file
// instead of persisting an empty object, so "no override" has exactly one
// representation on disk.
func (s *Store) SaveOverride(siteID string, override Override) (Override, error) {
	path, err := s.OverridePath(siteID)
	if err != nil {
		return Override{}, err
	}
	normalized := override.Normalize()
	if err := normalized.Validate(); err != nil {
		return Override{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if normalized.IsZero() {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return Override{}, err
		}
		return Override{}, nil
	}
	normalized.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if err := writeJSONAtomic(path, normalized); err != nil {
		return Override{}, err
	}
	return normalized, nil
}

// ClearOverride removes a site's override so it follows the global settings
// again. Removing a file that does not exist is not an error.
func (s *Store) ClearOverride(siteID string) error {
	path, err := s.OverridePath(siteID)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// Effective merges the global settings with one site's override. This is the
// function every worker, watcher and dashboard read must go through.
func (s *Store) Effective(siteID string) (Effective, error) {
	global, err := s.Load()
	if err != nil {
		return Effective{}, err
	}
	override, err := s.LoadOverride(siteID)
	if err != nil {
		return Effective{}, err
	}
	effective := Merge(global, override)
	effective.SiteID = strings.TrimSpace(siteID)
	return effective, nil
}

// EffectiveAll returns the effective settings of every site. Sites without an
// override all share the same values, which is what makes a single global save
// apply to all 101 sites that never customised anything.
func (s *Store) EffectiveAll() ([]Effective, error) {
	global, err := s.Load()
	if err != nil {
		return nil, err
	}
	list, err := sites.List(s.dataDir)
	if err != nil {
		return nil, err
	}
	out := make([]Effective, 0, len(list))
	for _, cfg := range list {
		override, err := s.LoadOverride(cfg.ID)
		if err != nil {
			return nil, err
		}
		effective := Merge(global, override)
		effective.SiteID = cfg.ID
		out = append(out, effective)
	}
	return out, nil
}

// SiteSummary is the dashboard row for one site: which sections it overrides and
// which values it will actually run with.
type SiteSummary struct {
	SiteID    string    `json:"site_id"`
	Overrides []string  `json:"overrides"`
	Effective Effective `json:"effective"`
	HasFile   bool      `json:"has_override_file"`
}

// Summaries lists every site with its effective settings and override keys.
func (s *Store) Summaries() ([]SiteSummary, error) {
	list, err := sites.List(s.dataDir)
	if err != nil {
		return nil, err
	}
	global, err := s.Load()
	if err != nil {
		return nil, err
	}
	out := make([]SiteSummary, 0, len(list))
	for _, cfg := range list {
		override, err := s.LoadOverride(cfg.ID)
		if err != nil {
			return nil, err
		}
		path, pathErr := s.OverridePath(cfg.ID)
		hasFile := false
		if pathErr == nil {
			if _, statErr := os.Stat(path); statErr == nil {
				hasFile = true
			}
		}
		effective := Merge(global, override)
		effective.SiteID = cfg.ID
		out = append(out, SiteSummary{
			SiteID:    cfg.ID,
			Overrides: override.Keys(),
			Effective: effective,
			HasFile:   hasFile,
		})
	}
	return out, nil
}

// writeJSONAtomic writes JSON via a temporary file in the same directory, then
// renames it into place. Readers therefore never observe a partial file.
func writeJSONAtomic(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}()
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, 0o600); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

// ProxyEntries returns the parsed rotation list of an effective configuration.
func (e Effective) ProxyEntries() ([]proxy.Entry, error) {
	if !e.Settings.Proxy.Enabled {
		return nil, nil
	}
	return parseProxyEntries(e.Settings.Proxy.Entries)
}

// MessageMode returns the effective message mode as a model.ChatMode* value.
func (e Effective) MessageMode() string { return NormalizeMessageMode(e.Settings.MessageMode) }

// Overridden reports whether the named section comes from the site override.
func (e Effective) Overridden(section string) bool {
	return e.Sources[section] == SourceSite
}
