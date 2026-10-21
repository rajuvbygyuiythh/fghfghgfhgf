package bot

// API Chat ships with a small editable data deck (persona + share + fallback).
// Files are seeded into data/api_chat only when missing, so a user's edits are
// never overwritten by an app update.

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

//go:embed api_assets/README.txt api_assets/persona.txt api_assets/persona_countries.txt api_assets/sharesnap.txt api_assets/share.txt api_assets/fallback.txt
var apiChatAssets embed.FS

// EnsureAPIChatData creates data/api_chat and restores any missing starter
// file. It returns the runtime folder path.
func EnsureAPIChatData(dataDir string) (string, error) {
	root := filepath.Join(dataDir, "api_chat")
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", fmt.Errorf("create API Chat data directory: %w", err)
	}
	err := fs.WalkDir(apiChatAssets, "api_assets", func(assetPath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if assetPath == "api_assets" || entry.IsDir() {
			return nil
		}
		rel := strings.TrimPrefix(assetPath, "api_assets/")
		destination := filepath.Join(root, filepath.FromSlash(rel))
		if _, err := os.Stat(destination); err == nil {
			return nil
		} else if !os.IsNotExist(err) {
			return err
		}
		data, err := apiChatAssets.ReadFile(assetPath)
		if err != nil {
			return err
		}
		return os.WriteFile(destination, data, 0o644)
	})
	if err != nil {
		return "", fmt.Errorf("bootstrap API Chat starter data: %w", err)
	}
	return root, nil
}

// readAPILines reads a small TXT deck: one entry per line, blank lines and
// lines starting with # are ignored.
func readAPILines(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []string
	for _, line := range strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, line)
	}
	return out, nil
}

// APIChatDeck holds the editable text files stored in data/api_chat.
type APIChatDeck struct {
	DataRoot  string `json:"data_root"`
	Persona   string `json:"persona"`
	ShareSnap string `json:"sharesnap"`
	Fallback  string `json:"fallback"`
	Countries string `json:"countries"`
}

// LoadAPIChatDeck ensures data/api_chat exists and reads its editable TXT files.
func LoadAPIChatDeck(dataDir string) (APIChatDeck, error) {
	root, err := EnsureAPIChatData(dataDir)
	if err != nil {
		return APIChatDeck{}, err
	}
	persona, err := readAPIText(filepath.Join(root, "persona.txt"))
	if err != nil {
		return APIChatDeck{}, err
	}
	shareSnap, err := readAPIText(filepath.Join(root, "sharesnap.txt"))
	if err != nil {
		return APIChatDeck{}, err
	}
	if strings.TrimSpace(shareSnap) == "" {
		shareSnap, _ = readAPIText(filepath.Join(root, "share.txt"))
	}
	fallback, err := readAPIText(filepath.Join(root, "fallback.txt"))
	if err != nil {
		return APIChatDeck{}, err
	}
	countries, err := readAPIText(filepath.Join(root, "persona_countries.txt"))
	if err != nil {
		return APIChatDeck{}, err
	}
	return APIChatDeck{
		DataRoot:  root,
		Persona:   persona,
		ShareSnap: shareSnap,
		Fallback:  fallback,
		Countries: countries,
	}, nil
}

// SaveAPIChatDeck writes the editable TXT files into data/api_chat so both
// apichat.exe and the main dashboard use the updated persona immediately.
func SaveAPIChatDeck(dataDir string, deck APIChatDeck) (APIChatDeck, error) {
	root, err := EnsureAPIChatData(dataDir)
	if err != nil {
		return APIChatDeck{}, err
	}
	write := func(name, content string) error {
		trimmed := strings.TrimSpace(strings.ReplaceAll(content, "\r\n", "\n"))
		if trimmed == "" {
			return nil
		}
		return os.WriteFile(filepath.Join(root, name), []byte(trimmed+"\n"), 0o644)
	}
	if err := write("persona.txt", deck.Persona); err != nil {
		return APIChatDeck{}, err
	}
	if err := write("sharesnap.txt", deck.ShareSnap); err != nil {
		return APIChatDeck{}, err
	}
	if err := write("fallback.txt", deck.Fallback); err != nil {
		return APIChatDeck{}, err
	}
	if err := write("persona_countries.txt", deck.Countries); err != nil {
		return APIChatDeck{}, err
	}
	return LoadAPIChatDeck(dataDir)
}

// ResetAPIChatDeck restores the built-in starter files in data/api_chat.
func ResetAPIChatDeck(dataDir string) (APIChatDeck, error) {
	root := filepath.Join(dataDir, "api_chat")
	for _, name := range []string{"persona.txt", "sharesnap.txt", "share.txt", "fallback.txt", "persona_countries.txt"} {
		_ = os.Remove(filepath.Join(root, name))
	}
	return LoadAPIChatDeck(dataDir)
}
