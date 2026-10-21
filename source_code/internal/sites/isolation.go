package sites

// Site folder isolation.
//
// Every mutable file that belongs to a site lives under exactly one directory:
//
//	<sites-root>/<site_id>/
//
// The only shared location is <dataDir>/_global/, which stores application
// settings and other explicitly global state. No site context can resolve a
// path outside its own folder; traversal (".."), absolute paths, volume names,
// Windows alternate data streams, and symlinks that escape the folder are all
// rejected before any file operation happens.
//
// The package deliberately exposes small file helpers instead of returning
// strings for callers to join themselves. Callers that need a path can use
// Resolve, but every helper re-validates it. This keeps the hard rule in one
// place and makes accidental cross-site access a reviewed, explicit change.

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"chitchatbot/internal/appdirs"
)

const (
	// GlobalDirName is the only directory shared by all sites. It holds
	// application settings, never per-site sessions, profiles or logs.
	GlobalDirName = "_global"
	// RecipesDirName holds the repeatable Group A-E setup recipes (sites/_recipes/).
	RecipesDirName = "_recipes"
	// SitesDirName is the canonical sites root when no workspace root exists.
	SitesDirName = "sites"
)

var (
	// ErrUnsafePath is returned whenever a path would leave its allowed root.
	ErrUnsafePath = errors.New("sites: unsafe path outside the site folder")
	// ErrInvalidSiteID is returned for IDs that are not already in canonical
	// form. This prevents "../x" and "a/b" from being silently sanitized into
	// a different site.
	ErrInvalidSiteID = errors.New("sites: invalid site id")
)

// SiteContext is the single source of truth for one site's paths. It is a
// value, not a pointer, so it cannot be mutated by a caller after creation.
//
// FolderPath is the site's own directory. Every other path is a subdirectory
// of it. GlobalRoot is the only allowed shared directory and is never used for
// site runtime data.
type SiteContext struct {
	SiteID             string
	DataDir            string
	FolderPath         string
	AccountsPath       string
	ProfilesPath       string
	SessionsPath       string
	LogsPath           string
	DefaultProfilePath string
	DefaultSessionPath string
	DebugLogPath       string
	GlobalRoot         string
}

// ProfilePath returns the absolute path of one browser profile inside the
// site. The profile name is validated as a single path segment.
func (c SiteContext) ProfilePath(profile string) (string, error) {
	segment, err := safeSegment(profile, "default")
	if err != nil {
		return "", err
	}
	return c.Resolve(filepath.Join("profiles", segment))
}

// BrowserProfileDir returns the Chromium/WebView user-data directory for a
// profile. Keeping it inside the profile directory means browser cookies and
// local storage can never be shared with another site.
func (c SiteContext) BrowserProfileDir(profile string) (string, error) {
	segment, err := safeSegment(profile, "default")
	if err != nil {
		return "", err
	}
	return c.Resolve(filepath.Join("profiles", segment, "browser"))
}

// SessionPath returns the per-profile session JSON path.
func (c SiteContext) SessionPath(profile string) (string, error) {
	segment, err := safeSegment(profile, "default")
	if err != nil {
		return "", err
	}
	return c.Resolve(filepath.Join("sessions", "session_"+segment+".json"))
}

// LatestSessionPath returns the "latest" session pointer for this site.
func (c SiteContext) LatestSessionPath() (string, error) {
	return c.Resolve(filepath.Join("sessions", "latest_session.json"))
}

// AccountPath returns the JSON path for one account file. The account ID is
// treated as a single file segment and gets a .json suffix when missing.
func (c SiteContext) AccountPath(accountID string) (string, error) {
	segment, err := safeSegment(accountID, "")
	if err != nil {
		return "", err
	}
	if !strings.HasSuffix(strings.ToLower(segment), ".json") {
		segment += ".json"
	}
	return c.Resolve(filepath.Join("accounts", segment))
}

// LogPath returns a log file inside the site's own logs directory.
func (c SiteContext) LogPath(name string) (string, error) {
	segment, err := safeSegment(name, "debug.jsonl")
	if err != nil {
		return "", err
	}
	return c.Resolve(filepath.Join("logs", segment))
}

// Resolve validates a relative path and returns its absolute location inside
// the site folder. Empty and "." resolve to the folder itself.
func (c SiteContext) Resolve(rel string) (string, error) {
	if strings.TrimSpace(c.FolderPath) == "" {
		return "", fmt.Errorf("%w: empty site folder", ErrUnsafePath)
	}
	return SafeJoin(c.FolderPath, rel)
}

// GlobalPath is an alias for GlobalResolve, for callers that prefer the
// "path" naming used by the other helpers.
func (c SiteContext) GlobalPath(rel string) (string, error) { return c.GlobalResolve(rel) }

// GlobalResolve resolves a path inside _global/. It is intentionally a
// separate method so site code cannot accidentally treat global paths as site
// paths.
func (c SiteContext) GlobalResolve(rel string) (string, error) {
	if strings.TrimSpace(c.GlobalRoot) == "" {
		return "", fmt.Errorf("%w: empty global folder", ErrUnsafePath)
	}
	return SafeJoin(c.GlobalRoot, rel)
}

// Ensure creates the site folder and all runtime subdirectories with private
// permissions. It never creates anything outside FolderPath.
func (c SiteContext) Ensure() error {
	for _, dir := range []string{
		c.FolderPath,
		c.AccountsPath,
		c.ProfilesPath,
		c.SessionsPath,
		c.LogsPath,
	} {
		if strings.TrimSpace(dir) == "" {
			return fmt.Errorf("%w: empty directory", ErrUnsafePath)
		}
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
	}
	// A default profile is required by the dashboard and by the worker plan.
	if _, err := c.ProfilePath("default"); err != nil {
		return err
	}
	defaultProfile, err := c.ProfilePath("default")
	if err != nil {
		return err
	}
	return os.MkdirAll(defaultProfile, 0o700)
}

// Stat is a safe os.Stat wrapper.
func (c SiteContext) Stat(rel string) (os.FileInfo, error) {
	path, err := c.Resolve(rel)
	if err != nil {
		return nil, err
	}
	return os.Stat(path)
}

// Lstat is a safe os.Lstat wrapper (does not follow the final symlink).
func (c SiteContext) Lstat(rel string) (os.FileInfo, error) {
	path, err := c.Resolve(rel)
	if err != nil {
		return nil, err
	}
	return os.Lstat(path)
}

// ReadDir is a safe os.ReadDir wrapper.
func (c SiteContext) ReadDir(rel string) ([]os.DirEntry, error) {
	path, err := c.Resolve(rel)
	if err != nil {
		return nil, err
	}
	return os.ReadDir(path)
}

// ReadFile is a safe os.ReadFile wrapper.
func (c SiteContext) ReadFile(rel string) ([]byte, error) {
	path, err := c.Resolve(rel)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}

// WriteFile is a safe os.WriteFile wrapper. Parent directories are created
// only when they are inside the site folder.
func (c SiteContext) WriteFile(rel string, data []byte, perm os.FileMode) error {
	path, err := c.Resolve(rel)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, data, perm)
}

// OpenFile is a safe os.OpenFile wrapper.
func (c SiteContext) OpenFile(rel string, flag int, perm os.FileMode) (*os.File, error) {
	path, err := c.Resolve(rel)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	return os.OpenFile(path, flag, perm)
}

// EnsureDir creates a directory inside the site folder.
func (c SiteContext) EnsureDir(rel string) error {
	path, err := c.Resolve(rel)
	if err != nil {
		return err
	}
	return os.MkdirAll(path, 0o700)
}

// Remove removes a file or empty directory inside the site folder. Recursive
// deletion is deliberately not exposed; callers must opt in with RemoveAll.
func (c SiteContext) Remove(rel string) error {
	path, err := c.Resolve(rel)
	if err != nil {
		return err
	}
	return os.Remove(path)
}

// RemoveAll removes a path inside the site folder recursively. It is intended
// for cleanup commands only and still validates the path first.
func (c SiteContext) RemoveAll(rel string) error {
	path, err := c.Resolve(rel)
	if err != nil {
		return err
	}
	return os.RemoveAll(path)
}

// Rename renames a file inside the site folder.
func (c SiteContext) Rename(oldRel, newRel string) error {
	oldPath, err := c.Resolve(oldRel)
	if err != nil {
		return err
	}
	newPath, err := c.Resolve(newRel)
	if err != nil {
		return err
	}
	return os.Rename(oldPath, newPath)
}

// SaveJSON marshals v and writes it atomically inside the site folder.
func (c SiteContext) SaveJSON(rel string, v any) error {
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	return c.WriteAtomic(rel, raw, 0o600)
}

// LoadJSON reads and decodes a JSON file inside the site folder. A missing file
// returns (false, nil), matching store.LoadJSON.
func (c SiteContext) LoadJSON(rel string, dest any) (bool, error) {
	raw, err := c.ReadFile(rel)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	if err := json.Unmarshal(raw, dest); err != nil {
		return false, err
	}
	return true, nil
}

// WriteAtomic writes a file via a temporary sibling + fsync + rename. This is
// used for sessions and account state so a power loss cannot leave a partial
// JSON file.
func (c SiteContext) WriteAtomic(rel string, data []byte, perm os.FileMode) error {
	path, err := c.Resolve(rel)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// Glob returns matches for a relative glob pattern. The pattern itself must
// resolve inside the site folder and every result is filtered through Resolve,
// so a symlinked match can never leak an outside path.
func (c SiteContext) Glob(pattern string) ([]string, error) {
	if strings.TrimSpace(pattern) == "" {
		return nil, fmt.Errorf("%w: empty pattern", ErrUnsafePath)
	}
	if filepath.IsAbs(pattern) || filepath.VolumeName(pattern) != "" {
		return nil, fmt.Errorf("%w: absolute glob", ErrUnsafePath)
	}
	// Validate the static prefix before globbing. filepath.Glob would accept
	// ".." and return outside paths if it existed.
	for _, part := range splitPathSegments(pattern) {
		if part == ".." {
			return nil, fmt.Errorf("%w: parent traversal in glob", ErrUnsafePath)
		}
	}
	base, err := c.Resolve(".")
	if err != nil {
		return nil, err
	}
	matches, err := filepath.Glob(filepath.Join(base, filepath.FromSlash(pattern)))
	if err != nil {
		return nil, err
	}
	out := matches[:0]
	for _, match := range matches {
		rel, relErr := filepath.Rel(base, match)
		if relErr != nil {
			continue
		}
		if _, err := c.Resolve(rel); err != nil {
			continue
		}
		out = append(out, match)
	}
	sort.Strings(out)
	return out, nil
}

// NewSiteContext creates the context for one site. dataDir is the runtime data
// directory (the directory that contains _global/). The sites root is the
// workspace sites/ directory when it exists, otherwise dataDir/sites.
func NewSiteContext(dataDir, siteID string) (SiteContext, error) {
	cleanData := filepath.Clean(strings.TrimSpace(dataDir))
	if cleanData == "" || cleanData == "." {
		return SiteContext{}, fmt.Errorf("%w: empty data directory", ErrUnsafePath)
	}
	root := SitesRoot(cleanData)
	ctx, err := newSiteContextAtRoot(root, siteID)
	if err != nil {
		return SiteContext{}, err
	}
	ctx.DataDir = cleanData
	ctx.GlobalRoot = GlobalDir(cleanData)
	return ctx, nil
}

// NewSiteContextAt is the explicit-root form used by tests and tools that
// already know the sites root. GlobalRoot is not set; use NewSiteContext for
// normal application code.
func NewSiteContextAt(sitesRoot, siteID string) (SiteContext, error) {
	return newSiteContextAtRoot(sitesRoot, siteID)
}

func newSiteContextAtRoot(sitesRoot, siteID string) (SiteContext, error) {
	root := filepath.Clean(strings.TrimSpace(sitesRoot))
	if root == "" || root == "." {
		return SiteContext{}, fmt.Errorf("%w: empty sites root", ErrUnsafePath)
	}
	id, err := validateSiteID(siteID)
	if err != nil {
		return SiteContext{}, err
	}
	folder, err := SafeJoin(root, id)
	if err != nil {
		return SiteContext{}, err
	}
	if info, statErr := os.Lstat(folder); statErr == nil && info.Mode()&os.ModeSymlink != 0 {
		return SiteContext{}, fmt.Errorf("%w: site folder is a symlink", ErrUnsafePath)
	}
	return SiteContext{
		SiteID:             id,
		FolderPath:         folder,
		AccountsPath:       filepath.Join(folder, "accounts"),
		ProfilesPath:       filepath.Join(folder, "profiles"),
		SessionsPath:       filepath.Join(folder, "sessions"),
		LogsPath:           filepath.Join(folder, "logs"),
		DefaultProfilePath: filepath.Join(folder, "profiles", "default"),
		DefaultSessionPath: filepath.Join(folder, "sessions", "session_default.json"),
		DebugLogPath:       filepath.Join(folder, "logs", "debug.jsonl"),
	}, nil
}

// SitesRoot returns the one canonical sites root for a data directory.
//
// Workspace layout (source checkout):
//
//	<repo>/sites/<site_id>/            <- configs and runtime site data
//	<repo>/sites/_global/              <- the only shared folder
//
// Portable layout (installed app):
//
//	<dataDir>/sites/<site_id>/
//	<dataDir>/sites/_global/
func SitesRoot(dataDir string) string { return appdirs.SitesRoot(dataDir) }

// RootDir is kept for compatibility with earlier callers. It now returns the
// same single canonical root as SitesRoot.
func RootDir(dataDir string) string { return SitesRoot(dataDir) }

// ExternalRootDir is retained for API compatibility. The redesign has exactly
// one sites root, so it returns that root rather than a second copy.
//
// Deprecated: use SitesRoot.
func ExternalRootDir(dataDir string) string { return SitesRoot(dataDir) }

// GlobalDir returns the only shared directory: <sites-root>/_global. It sits
// next to the site folders, never inside one of them.
func GlobalDir(dataDir string) string { return appdirs.GlobalDir(dataDir) }

// GlobalPath resolves a relative path under _global/. Untrusted input is
// validated by SafeJoin, exactly like site paths.
func GlobalPath(dataDir, rel string) (string, error) {
	return SafeJoin(GlobalDir(dataDir), rel)
}

// RecipesDir returns the directory for repeatable Group A-E setup recipes:
// <sites-root>/_recipes.
func RecipesDir(dataDir string) string { return appdirs.RecipesDir(dataDir) }

// RecipesPath resolves a relative path under _recipes/. Untrusted input is
// validated by SafeJoin.
func RecipesPath(dataDir, rel string) (string, error) {
	return SafeJoin(RecipesDir(dataDir), rel)
}

// EnsureGlobalDirs creates the shared directory tree:
//
//	_global/settings.json   (created by the settings store, not here)
//	_global/ui/             Badger DB + message settings mirror
//	_global/logs/           structured per-site debug mirrors
//	_global/logs.txt        application diagnostics
//	_global/memory/         chat history / operation memory
func EnsureGlobalDirs(dataDir string) error { return appdirs.EnsureGlobalDirs(dataDir) }

// SafeJoin joins root and rel and proves the result stays under root. It is the
// single path-safety primitive used by every site file operation.
//
// Rules:
//   - rel must be relative (no absolute paths, drive letters or UNC paths)
//   - no ".." segment is accepted, even if it would normalize back inside
//   - no NUL or control characters
//   - on Windows, no ":" (alternate data streams), trailing dots/spaces, or
//     reserved device names
//   - the existing part of the path is resolved through symlinks; the resolved
//     target must still be inside the resolved root
func SafeJoin(root, rel string) (string, error) {
	if strings.TrimSpace(root) == "" {
		return "", fmt.Errorf("%w: empty root", ErrUnsafePath)
	}
	if strings.ContainsRune(root, 0) {
		return "", fmt.Errorf("%w: NUL in root", ErrUnsafePath)
	}
	rootAbs, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrUnsafePath, err)
	}

	rel = strings.TrimSpace(rel)
	if rel == "" {
		rel = "."
	}
	// Normalize Windows separators for parsing. On Unix a backslash is a legal
	// filename character, but treating it as a separator is safer and makes
	// cross-platform traversal attempts fail identically.
	rel = filepath.ToSlash(rel)
	if strings.ContainsRune(rel, 0) {
		return "", fmt.Errorf("%w: NUL byte", ErrUnsafePath)
	}
	for _, r := range rel {
		if r < 0x20 {
			return "", fmt.Errorf("%w: control character", ErrUnsafePath)
		}
	}
	if filepath.IsAbs(rel) || filepath.VolumeName(rel) != "" {
		return "", fmt.Errorf("%w: absolute path", ErrUnsafePath)
	}
	// UNC/device paths can be relative-looking on some platforms.
	if strings.HasPrefix(rel, `\\`) || strings.HasPrefix(rel, "//") {
		return "", fmt.Errorf("%w: UNC path", ErrUnsafePath)
	}

	if strings.Contains(rel, "\\") {
		return "", fmt.Errorf("%w: backslash separator", ErrUnsafePath)
	}
	// Reject traversal before Clean normalizes it away.
	for _, part := range splitPathSegments(rel) {
		if part == ".." {
			return "", fmt.Errorf("%w: parent traversal", ErrUnsafePath)
		}
	}
	cleanRel := filepath.Clean(rel)
	if cleanRel == ".." || strings.HasPrefix(cleanRel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("%w: parent traversal", ErrUnsafePath)
	}
	for _, part := range splitPathSegments(cleanRel) {
		if part == ".." {
			return "", fmt.Errorf("%w: parent traversal", ErrUnsafePath)
		}
		if runtime.GOOS == "windows" {
			if err := validateWindowsSegment(part); err != nil {
				return "", err
			}
		}
	}

	target := filepath.Join(rootAbs, cleanRel)
	if !pathWithin(rootAbs, target) {
		return "", fmt.Errorf("%w: %s", ErrUnsafePath, rel)
	}

	// Symlink check. The target may not exist yet (creating a file), so walk up
	// to the nearest existing ancestor and resolve that.
	realRoot, err := evalExistingPath(rootAbs)
	if err != nil {
		return "", fmt.Errorf("%w: resolve root: %v", ErrUnsafePath, err)
	}
	realTarget, err := evalExistingPath(target)
	if err != nil {
		return "", fmt.Errorf("%w: resolve target: %v", ErrUnsafePath, err)
	}
	if !pathWithin(realRoot, realTarget) {
		return "", fmt.Errorf("%w: symlink escapes site folder", ErrUnsafePath)
	}
	return target, nil
}

// PathWithin reports whether target is root or a descendant of root. It is
// exported for tests and diagnostics; file operations should use SafeJoin.
func PathWithin(root, target string) bool { return pathWithin(root, target) }

// IsPathWithin is an alias used by some callers/tests.
func IsPathWithin(root, target string) bool { return pathWithin(root, target) }

func pathWithin(root, target string) bool {
	root = filepath.Clean(root)
	target = filepath.Clean(target)
	if runtime.GOOS == "windows" {
		root = strings.ToLower(root)
		target = strings.ToLower(target)
	}
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return false
	}
	if filepath.IsAbs(rel) {
		return false
	}
	return true
}

func evalExistingPath(path string) (string, error) {
	abs, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", err
	}
	current := abs
	var missing []string
	for {
		resolved, err := filepath.EvalSymlinks(current)
		if err == nil {
			for i := len(missing) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, missing[i])
			}
			return filepath.Clean(resolved), nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", err
		}
		missing = append(missing, filepath.Base(current))
		current = parent
	}
}

func splitPathSegments(path string) []string {
	path = filepath.ToSlash(path)
	return strings.Split(path, "/")
}

func validateWindowsSegment(segment string) error {
	if segment == "" || segment == "." {
		return nil
	}
	if strings.Contains(segment, ":") {
		return fmt.Errorf("%w: Windows alternate data stream", ErrUnsafePath)
	}
	if strings.HasSuffix(segment, ".") || strings.HasSuffix(segment, " ") {
		return fmt.Errorf("%w: Windows trailing dot/space", ErrUnsafePath)
	}
	stem := strings.ToUpper(strings.SplitN(segment, ".", 2)[0])
	reserved := stem == "CON" || stem == "PRN" || stem == "AUX" || stem == "NUL"
	if len(stem) == 4 && (strings.HasPrefix(stem, "COM") || strings.HasPrefix(stem, "LPT")) && stem[3] >= '1' && stem[3] <= '9' {
		reserved = true
	}
	if reserved {
		return fmt.Errorf("%w: Windows reserved device name", ErrUnsafePath)
	}
	return nil
}

func validateSiteID(siteID string) (string, error) {
	raw := strings.TrimSpace(siteID)
	if raw == "" || raw == "." || raw == ".." {
		return "", ErrInvalidSiteID
	}
	if raw != SanitizeID(raw) {
		return "", ErrInvalidSiteID
	}
	if raw == GlobalDirName || raw == RecipesDirName || raw == SitesDirName {
		return "", ErrInvalidSiteID
	}
	return raw, nil
}

func safeSegment(name, fallback string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = fallback
	}
	if name == "" || name == "." || name == ".." {
		return "", ErrUnsafePath
	}
	if strings.ContainsAny(name, `/\`) || strings.ContainsRune(name, 0) {
		return "", ErrUnsafePath
	}
	if runtime.GOOS == "windows" {
		if err := validateWindowsSegment(name); err != nil {
			return "", err
		}
	}
	return name, nil
}

// CleanSiteRuntime removes only runtime state inside one site folder. It is
// used by the dashboard's "Clean Profiles" action and never touches settings or
// another site's folder.
func CleanSiteRuntime(dataDir, siteID string) error {
	ctx, err := NewSiteContext(dataDir, siteID)
	if err != nil {
		return err
	}
	for _, rel := range []string{"profiles", "tmp", "fresh-runs", "bot_registry.json"} {
		if err := ctx.RemoveAll(rel); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return ctx.Ensure()
}

// CopyTree copies a directory tree. It is used only by the one-time legacy
// migration; it refuses to follow symlinks and never writes outside dst.
func CopyTree(src, dst string) error {
	src = filepath.Clean(src)
	dst = filepath.Clean(dst)
	if !pathWithin(filepath.Dir(dst), dst) {
		return ErrUnsafePath
	}
	return filepath.WalkDir(src, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(src, path)
		if relErr != nil {
			return relErr
		}
		target := filepath.Join(dst, rel)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o700)
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil // never copy links out of a legacy profile
		}
		in, openErr := os.Open(path)
		if openErr != nil {
			return openErr
		}
		if mkErr := os.MkdirAll(filepath.Dir(target), 0o700); mkErr != nil {
			_ = in.Close()
			return mkErr
		}
		out, createErr := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
		if createErr != nil {
			_ = in.Close()
			return createErr
		}
		_, copyErr := io.Copy(out, in)
		inErr := in.Close()
		outErr := out.Close()
		if copyErr != nil {
			return copyErr
		}
		if inErr != nil {
			return inErr
		}
		return outErr
	})
}
