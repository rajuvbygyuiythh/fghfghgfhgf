// Command chitchattest is the local console chat used by test.bat.
//
// It reads the same saved dashboard Message settings as the app, opens no
// browser and sends nothing to the real site. Run it from source_code/ (the
// data folder then matches the dashboard) or through test.bat.
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"chitchatbot/internal/bot"
	"chitchatbot/internal/model"
	"chitchatbot/internal/uistore"
)

func main() {
	dataDir, err := resolveDataDir()
	if err != nil {
		fail("cannot resolve the data folder", err)
	}

	settings, err := loadMessageSettings(dataDir)
	if err != nil {
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Could not read the dashboard Message settings.")
		fmt.Fprintln(os.Stderr, "Close the running dashboard first (it locks data/ui/badger), then run test.bat again.")
		fail("settings error", err)
	}

	mode := ""
	if len(os.Args) > 1 {
		mode = strings.TrimSpace(os.Args[1])
	}

	opts := bot.LocalChatOptions{
		DataDir:  dataDir,
		Settings: settings,
		Mode:     mode,
		In:       os.Stdin,
		Out:      os.Stdout,
	}
	if err := bot.RunLocalChat(context.Background(), opts); err != nil {
		fmt.Fprintf(os.Stderr, "\n[test] %v\n", err)
		os.Exit(1)
	}
}

func resolveDataDir() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return filepath.Join(wd, "data"), nil
}

// loadMessageSettings opens the dashboard settings store just long enough to
// copy the Message page values, then releases it so the dashboard can run
// again right after the test.
func loadMessageSettings(dataDir string) (model.MessageSettings, error) {
	st, err := uistore.Open(dataDir)
	if err != nil {
		if snap, snapErr := uistore.LoadMessageSettingsMirror(dataDir); snapErr == nil {
			return snap, nil
		}
		return model.MessageSettings{}, err
	}
	defer st.Close()
	return st.LoadMessageSettings()
}

func fail(message string, err error) {
	fmt.Fprintf(os.Stderr, "[test] %s: %v\n", message, err)
	os.Exit(1)
}
