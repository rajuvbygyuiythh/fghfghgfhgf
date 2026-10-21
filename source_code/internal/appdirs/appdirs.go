// Package appdirs resolves the two application roots: the sites root and the
// one shared (global) directory.
//
// Layout:
//
//	<repo>/sites/<site_id>/     one folder per site, isolated runtime state
//	<repo>/sites/_global/       the only shared folder: settings, UI DB, logs, memory
//
// Portable install (no workspace sites/ directory):
//
//	<dataDir>/sites/<site_id>/
//	<dataDir>/sites/_global/
//
// The package is deliberately a leaf: it imports nothing from this module, so
// sites, uistore, applog and memory can all agree on the layout without an
// import cycle. Path safety for *site* files stays in internal/sites; this
// package only answers "where is the root".
package appdirs

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	// GlobalDirName is the only directory shared by all sites.
	GlobalDirName = "_global"
	// SitesDirName is the canonical sites folder name.
	SitesDirName = "sites"
	// SettingsFileName is the shared settings file inside GlobalDirName.
	SettingsFileName = "settings.json"
)

// SitesRoot returns the one canonical sites root for a data directory.
//
// A workspace checkout keeps sites next to source_code; an installed build
// keeps them under the runtime data directory. The workspace form is preferred
// when it already contains at least one site folder, which is what makes
// <repo>/sites authoritative in this project.
func SitesRoot(dataDir string) string {
	if root := workspaceSitesRoot(dataDir); root != "" {
		return root
	}
	return filepath.Join(filepath.Clean(dataDir), SitesDirName)
}

// GlobalDir returns the only shared directory: <sites-root>/_global.
//
// Every site folder lives next to it, and nothing inside a site folder may ever
// point at it for site runtime state.
func GlobalDir(dataDir string) string {
	return filepath.Join(SitesRoot(dataDir), GlobalDirName)
}

// GlobalPath joins fixed, trusted names under the shared directory. It must only
// be used with literals from this code base (never with user input); path
// validation for untrusted input lives in sites.SafeJoin.
func GlobalPath(dataDir string, elem ...string) string {
	return filepath.Join(append([]string{GlobalDir(dataDir)}, elem...)...)
}

// SettingsPath is <global>/settings.json.
func SettingsPath(dataDir string) string {
	return GlobalPath(dataDir, SettingsFileName)
}

// UIDir is <global>/ui, the Badger database and message-settings mirror.
func UIDir(dataDir string) string { return GlobalPath(dataDir, "ui") }

// LogsDir is <global> (the shared logs.txt lives directly inside it).
func LogsDir(dataDir string) string { return GlobalDir(dataDir) }

// LogsFile is <global>/logs.txt.
func LogsFile(dataDir string) string { return GlobalPath(dataDir, "logs.txt") }

// MemoryDir is <global>/memory.
func MemoryDir(dataDir string) string { return GlobalPath(dataDir, "memory") }

// EnsureGlobalDirs creates the shared directory tree and migrates a Phase-1
// <dataDir>/_global tree into it. It is safe to call repeatedly and safe to call
// from several processes (MkdirAll is idempotent; migration only moves files
// that do not exist at the destination).
func EnsureGlobalDirs(dataDir string) error {
	clean := strings.TrimSpace(dataDir)
	if clean == "" {
		return fmt.Errorf("appdirs: empty data directory")
	}
	if err := MigrateLegacyGlobal(clean); err != nil {
		return err
	}
	for _, dir := range []string{
		GlobalDir(clean),
		GlobalPath(clean, "ui"),
		GlobalPath(clean, "logs"),
		GlobalPath(clean, "memory"),
		GlobalPath(clean, "settings"),
	} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
	}
	return nil
}

// MigrateLegacyGlobal moves the pre-Phase-2 shared tree <dataDir>/_global into
// <sites-root>/_global. Files that already exist at the destination win, so a
// half-finished migration never overwrites newer state.
func MigrateLegacyGlobal(dataDir string) error {
	legacy := filepath.Join(filepath.Clean(dataDir), GlobalDirName)
	target := GlobalDir(dataDir)
	if samePath(legacy, target) {
		return nil
	}
	info, err := os.Stat(legacy)
	if err != nil || !info.IsDir() {
		return nil
	}
	if err := os.MkdirAll(target, 0o700); err != nil {
		return err
	}
	return mergeTree(legacy, target)
}

// mergeTree copies entries from src into dst without overwriting anything that
// already exists, then removes src when it is finally empty.
func mergeTree(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		from := filepath.Join(src, entry.Name())
		to := filepath.Join(dst, entry.Name())
		if entry.IsDir() {
			if err := os.MkdirAll(to, 0o700); err != nil {
				return err
			}
			if err := mergeTree(from, to); err != nil {
				return err
			}
			_ = os.Remove(from) // only succeeds when empty
			continue
		}
		if _, statErr := os.Lstat(to); statErr == nil {
			continue // destination wins
		}
		if err := os.Rename(from, to); err != nil {
			if copyErr := copyFile(from, to); copyErr != nil {
				return copyErr
			}
			_ = os.Remove(from)
		}
	}
	_ = os.Remove(src)
	return nil
}

func copyFile(from, to string) error {
	data, err := os.ReadFile(from)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(to), 0o700); err != nil {
		return err
	}
	return os.WriteFile(to, data, 0o600)
}

func samePath(a, b string) bool {
	absA, errA := filepath.Abs(filepath.Clean(a))
	absB, errB := filepath.Abs(filepath.Clean(b))
	if errA != nil || errB != nil {
		return filepath.Clean(a) == filepath.Clean(b)
	}
	return absA == absB
}

func workspaceSitesRoot(dataDir string) string {
	abs, err := filepath.Abs(filepath.Clean(dataDir))
	if err != nil {
		return ""
	}
	parent := filepath.Dir(abs)
	grand := filepath.Dir(parent)
	for _, candidate := range []string{
		filepath.Join(parent, SitesDirName),
		filepath.Join(grand, SitesDirName),
	} {
		if isSitesRoot(candidate) {
			return candidate
		}
	}
	return ""
}

// isSitesRoot reports whether dir looks like a populated sites root. A folder
// that only contains _global is not enough: at least one site folder with a
// site.json must exist.
func isSitesRoot(dir string) bool {
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return false
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == GlobalDirName {
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, entry.Name(), "site.json")); err == nil {
			return true
		}
	}
	return false
}
