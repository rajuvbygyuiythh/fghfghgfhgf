package bot

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"chitchatbot/internal/model"
)

const (
	SiteChatHistoryFileName = "chat_history.jsonl"
	SiteActivityLogFileName = "activity.log"
)

var chatHistoryMu sync.Mutex

// ChatHistoryEntry records one incoming or outgoing message in a site's
// isolated logs/chat_history.jsonl file.
type ChatHistoryEntry struct {
	Timestamp      string `json:"ts"`
	SiteID         string `json:"site_id"`
	Profile        string `json:"profile"`
	BotUserID      string `json:"bot_user_id,omitempty"`
	ConversationID string `json:"conversation_id,omitempty"`
	PartnerID      string `json:"partner_id,omitempty"`
	PartnerName    string `json:"partner_name,omitempty"`
	Direction      string `json:"direction"` // "in" or "out"
	Transport      string `json:"transport,omitempty"`
	ChatMode       string `json:"chat_mode,omitempty"`
	Rule           string `json:"rule,omitempty"`
	Message        string `json:"message"`
}

// LogSendActivity records an outgoing bot message to the activity stream and,
// when SiteFolder is set, persists it to sites/<site_id>/logs/chat_history.jsonl
// and sites/<site_id>/logs/activity.log.
func LogSendActivity(cfg Config, currentUserID string, target model.DMTarget, message string) {
	logSendActivity(cfg, currentUserID, target, message)
}

// LogReceiveActivity records an incoming partner message to the activity stream
// and, when SiteFolder is set, persists it to sites/<site_id>/logs/chat_history.jsonl
// and sites/<site_id>/logs/activity.log.
func LogReceiveActivity(cfg Config, currentUserID string, target model.DMTarget, message string) {
	logReceiveActivity(cfg, currentUserID, target, message)
}

// AppendSiteChatHistory appends a structured ChatHistoryEntry to
// <siteFolder>/logs/chat_history.jsonl and <siteFolder>/logs/activity.log.
func AppendSiteChatHistory(siteFolder string, entry ChatHistoryEntry) error {
	folder := strings.TrimSpace(siteFolder)
	if folder == "" {
		return errors.New("site folder is required")
	}
	if strings.TrimSpace(entry.Timestamp) == "" {
		entry.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	}
	logsDir := filepath.Join(folder, "logs")
	chatHistoryMu.Lock()
	defer chatHistoryMu.Unlock()

	if err := os.MkdirAll(logsDir, 0o700); err != nil {
		return err
	}
	raw, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	histPath := filepath.Join(logsDir, SiteChatHistoryFileName)
	f, err := os.OpenFile(histPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	_, writeErr := f.Write(raw)
	_ = f.Close()

	actPath := filepath.Join(logsDir, SiteActivityLogFileName)
	dirLabel := "SEND"
	if strings.EqualFold(entry.Direction, "in") {
		dirLabel = "RECEIVE"
	}
	actLine := fmt.Sprintf(
		"%s | %s | site=%s profile=%s mode=%s rule=%s | %s\n",
		entry.Timestamp,
		dirLabel,
		entry.SiteID,
		entry.Profile,
		entry.ChatMode,
		entry.Rule,
		activityMessage(entry.Message),
	)
	if af, aErr := os.OpenFile(actPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600); aErr == nil {
		_, _ = af.WriteString(actLine)
		_ = af.Close()
	}
	return writeErr
}

// ReadSiteChatHistory reads all ChatHistoryEntry records from
// <siteFolder>/logs/chat_history.jsonl.
func ReadSiteChatHistory(siteFolder string) ([]ChatHistoryEntry, error) {
	folder := strings.TrimSpace(siteFolder)
	if folder == "" {
		return nil, errors.New("site folder is required")
	}
	histPath := filepath.Join(folder, "logs", SiteChatHistoryFileName)
	chatHistoryMu.Lock()
	raw, err := os.ReadFile(histPath)
	chatHistoryMu.Unlock()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	out := make([]ChatHistoryEntry, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var item ChatHistoryEntry
		if json.Unmarshal([]byte(line), &item) == nil {
			out = append(out, item)
		}
	}
	return out, nil
}

func logSendActivity(cfg Config, currentUserID string, target model.DMTarget, message string) {
	activityf(cfg, "%s\n", formatSendActivity(cfg, currentUserID, target, message))
	if folder := strings.TrimSpace(cfg.SiteFolder); folder != "" {
		_ = AppendSiteChatHistory(folder, ChatHistoryEntry{
			SiteID:         cfg.SiteID,
			Profile:        cfg.Profile,
			BotUserID:      currentUserID,
			ConversationID: target.ConversationID,
			PartnerID:      target.UserID,
			PartnerName:    target.Username,
			Direction:      "out",
			ChatMode:       cfg.ChatMode,
			Message:        message,
		})
	}
}

func logReceiveActivity(cfg Config, currentUserID string, target model.DMTarget, message string) {
	activityf(cfg, "%s\n", formatReceiveActivity(cfg, currentUserID, target, message))
	if folder := strings.TrimSpace(cfg.SiteFolder); folder != "" {
		_ = AppendSiteChatHistory(folder, ChatHistoryEntry{
			SiteID:         cfg.SiteID,
			Profile:        cfg.Profile,
			BotUserID:      currentUserID,
			ConversationID: target.ConversationID,
			PartnerID:      target.UserID,
			PartnerName:    target.Username,
			Direction:      "in",
			ChatMode:       cfg.ChatMode,
			Message:        message,
		})
	}
}

func formatSendActivity(cfg Config, currentUserID string, target model.DMTarget, message string) string {
	return fmt.Sprintf(
		"SEND | %s -> %s | message: %s",
		botActivityLabel(cfg, currentUserID, target),
		trafficActivityLabel(target),
		activityMessage(message),
	)
}

func formatReceiveActivity(cfg Config, currentUserID string, target model.DMTarget, message string) string {
	return fmt.Sprintf(
		"RECEIVE | %s -> %s | message: %s",
		trafficActivityLabel(target),
		botActivityLabel(cfg, currentUserID, target),
		activityMessage(message),
	)
}

func botActivityLabel(cfg Config, currentUserID string, target model.DMTarget) string {
	username := strings.TrimSpace(target.BotUsername)
	profile := strings.TrimSpace(cfg.Profile)
	currentUserID = strings.TrimSpace(currentUserID)

	label := ""
	switch {
	case username != "":
		label = "bot " + username
	case profile != "":
		label = "bot " + profile
	default:
		label = "bot unknown"
	}
	if currentUserID != "" {
		label += fmt.Sprintf(" (%s)", currentUserID)
	}
	return label
}

func trafficActivityLabel(target model.DMTarget) string {
	username := strings.TrimSpace(target.Username)
	userID := strings.TrimSpace(target.UserID)

	label := ""
	switch {
	case username != "":
		label = "traffic " + username
	default:
		label = "traffic unknown"
	}
	if userID != "" {
		label += fmt.Sprintf(" (%s)", userID)
	}
	return label
}

func activityMessage(message string) string {
	message = strings.Join(strings.Fields(message), " ")
	if message == "" {
		return "(empty)"
	}
	if len(message) > 500 {
		return message[:500] + "..."
	}
	return message
}
