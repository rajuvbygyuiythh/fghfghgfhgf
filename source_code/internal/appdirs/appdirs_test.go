package appdirs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGlobalDirAndSettingsPathUnderSitesRoot(t *testing.T) {
	dataDir := t.TempDir()
	wantSites := filepath.Join(dataDir, "sites")
	if got := SitesRoot(dataDir); got != wantSites {
		t.Fatalf("SitesRoot = %q, want %q", got, wantSites)
	}
	wantGlobal := filepath.Join(wantSites, "_global")
	if got := GlobalDir(dataDir); got != wantGlobal {
		t.Fatalf("GlobalDir = %q, want %q", got, wantGlobal)
	}
	wantSettings := filepath.Join(wantGlobal, "settings.json")
	if got := SettingsPath(dataDir); got != wantSettings {
		t.Fatalf("SettingsPath = %q, want %q", got, wantSettings)
	}
}

func TestEnsureGlobalDirsMigratesPhase1GlobalFolder(t *testing.T) {
	dataDir := t.TempDir()
	legacyFile := filepath.Join(dataDir, "_global", "logs.txt")
	if err := os.MkdirAll(filepath.Dir(legacyFile), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacyFile, []byte("legacy-log-line\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := EnsureGlobalDirs(dataDir); err != nil {
		t.Fatalf("EnsureGlobalDirs: %v", err)
	}

	migrated := LogsFile(dataDir)
	data, err := os.ReadFile(migrated)
	if err != nil {
		t.Fatalf("migrated logs.txt missing at %s: %v", migrated, err)
	}
	if string(data) != "legacy-log-line\n" {
		t.Fatalf("migrated content = %q", string(data))
	}
	for _, sub := range []string{"ui", "logs", "memory", "settings"} {
		if info, err := os.Stat(GlobalPath(dataDir, sub)); err != nil || !info.IsDir() {
			t.Fatalf("missing _global/%s: %v", sub, err)
		}
	}
}
