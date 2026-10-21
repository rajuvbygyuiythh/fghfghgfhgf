package bot

import (
	"fmt"
	"math/rand"
	"strings"

	"chitchatbot/internal/model"
)

// SequenceTurnResult reports the output of one sequence-mode turn executed by
// message_sequence.go.
type SequenceTurnResult struct {
	Lines       []string
	Rule        string
	Step        int
	TotalSteps  int
	HasUsername bool
	Ended       bool
}

// RenderMessageTemplate renders a sequence template line (`{opt1|opt2}` and `%username%`).
func RenderMessageTemplate(template string, rng *rand.Rand, assignedUsername string) string {
	return renderMessageTemplate(template, rng, assignedUsername)
}

// ChooseUsername picks one username from the configured username pool.
func ChooseUsername(usernames []string, rng *rand.Rand) string {
	return chooseUsername(usernames, rng)
}

// ExecuteSequenceTurn runs one step of Single Sequence or Folder Sequences mode
// using message_sequence.go and updates the site's isolated AccountState.
func ExecuteSequenceTurn(
	cfg Config,
	accountState *model.AccountState,
	targetUserID string,
	rng *rand.Rand,
) SequenceTurnResult {
	normalizeBotAccountState(accountState)
	if rng == nil {
		rng = rand.New(rand.NewSource(1))
	}
	userID := strings.TrimSpace(targetUserID)
	if userID == "" {
		userID = "stranger"
	}
	sentCount := accountState.SentMessageCounts[userID]
	activeSequence := getActiveSequence(cfg, accountState, userID, sentCount, rng)
	if len(activeSequence) == 0 {
		return SequenceTurnResult{
			Rule:  "sequence: no sequence configured",
			Ended: true,
		}
	}
	if sentCount >= len(activeSequence) {
		return SequenceTurnResult{
			Rule:       fmt.Sprintf("sequence exhausted (%d/%d)", sentCount, len(activeSequence)),
			Step:       sentCount,
			TotalSteps: len(activeSequence),
			Ended:      true,
		}
	}

	rawTemplate := activeSequence[sentCount]
	// Support both %username% and {snap} in sequence templates.
	normalizedTemplate := strings.ReplaceAll(rawTemplate, "{snap}", "%username%")
	text, hasUsername := renderMsg(cfg, accountState, userID, sentCount, rng, normalizedTemplate)
	text = strings.TrimSpace(text)
	step := sentCount + 1
	accountState.SentMessageCounts[userID] = step
	accountState.BaselineDone[userID] = true
	if hasUsername && text != "" {
		accountState.SnapTgSentCount++
		if dir := cfg.EffectiveDataDir(); dir != "" {
			_ = markUserFlagged(dir, userID)
		}
	}
	if text == "" {
		return SequenceTurnResult{
			Rule:       fmt.Sprintf("sequence step %d/%d rendered empty", step, len(activeSequence)),
			Step:       step,
			TotalSteps: len(activeSequence),
			Ended:      true,
		}
	}
	modeTag := "sequence"
	if cfg.ChatMode == model.ChatModeFolder {
		modeTag = "folder_sequence"
	}
	return SequenceTurnResult{
		Lines:       []string{text},
		Rule:        fmt.Sprintf("%s step %d/%d", modeTag, step, len(activeSequence)),
		Step:        step,
		TotalSteps:  len(activeSequence),
		HasUsername: hasUsername,
		Ended:       step >= len(activeSequence),
	}
}

func renderMessageTemplate(template string, rng *rand.Rand, assignedUsername string) string {
	template = strings.TrimSpace(template)
	if template == "" {
		return ""
	}

	if strings.Contains(template, "%username%") {
		template = strings.ReplaceAll(template, "%username%", assignedUsername)
	}

	parts := make([]string, 0)
	for index := 0; index < len(template); {
		if template[index] == '{' {
			closing := strings.IndexByte(template[index+1:], '}')
			if closing >= 0 {
				appendNormalizedParts(&parts, chooseTemplateOption(template[index+1:index+1+closing], rng))
				index += closing + 2
				continue
			}
		}

		nextGroup := strings.IndexByte(template[index:], '{')
		if nextGroup < 0 {
			appendNormalizedParts(&parts, template[index:])
			break
		}

		appendNormalizedParts(&parts, template[index:index+nextGroup])
		index += nextGroup
	}

	return strings.Join(parts, " ")
}

func chooseUsername(usernames []string, rng *rand.Rand) string {
	cleaned := make([]string, 0, len(usernames))
	for _, username := range usernames {
		username = strings.TrimSpace(username)
		if username == "" {
			continue
		}
		cleaned = append(cleaned, username)
	}

	if len(cleaned) == 0 {
		return ""
	}
	if len(cleaned) == 1 || rng == nil {
		return cleaned[0]
	}
	return cleaned[rng.Intn(len(cleaned))]
}

func chooseTemplateOption(group string, rng *rand.Rand) string {
	options := strings.Split(group, "|")
	cleaned := make([]string, 0, len(options))

	for _, option := range options {
		option = strings.TrimSpace(option)
		if option == "" {
			continue
		}
		cleaned = append(cleaned, option)
	}

	if len(cleaned) == 0 {
		return ""
	}
	if len(cleaned) == 1 || rng == nil {
		return cleaned[0]
	}

	return cleaned[rng.Intn(len(cleaned))]
}

func appendNormalizedParts(parts *[]string, value string) {
	for _, field := range strings.Fields(strings.TrimSpace(value)) {
		if field == "" {
			continue
		}
		*parts = append(*parts, field)
	}
}
