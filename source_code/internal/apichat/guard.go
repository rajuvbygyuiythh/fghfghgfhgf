package apichat

// Reply quality gate. One ordered list of checks decides whether a generated
// reply may be sent: no repeats, no known-fact questions, no topic drift, no
// question spam and no AI-sounding text.

import (
	"fmt"
	"strings"
)

// repeatThreshold is the similarity above which a line counts as a repeat.
const repeatThreshold = 0.62

// validateReply returns "" when the reply is safe, otherwise a short reason.
func validateReply(lines []string, d *Director, p MessageProfile, userText string, history []Message) string {
	if len(lines) == 0 {
		return "empty reply"
	}
	joined := strings.ToLower(strings.Join(lines, " "))
	for _, line := range lines {
		if forbiddenRe.MatchString(line) {
			return "wrote @ / link / ai-talk"
		}
	}
	if m := aiPhraseRe.FindString(joined); m != "" {
		return "sounds like an ai/customer-service bot (" + m + ")"
	}
	if g := parrotGram(strings.ToLower(userText), joined); g != "" {
		return "parroted their own words back (\"" + g + "\")"
	}

	// Never reply to a question with nothing but another question.
	allQuestions := true
	for _, line := range lines {
		if !onlyQuestion(line) {
			allQuestions = false
		}
	}
	if p.IsQuestion && allQuestions {
		return "did not answer their question, only asked back"
	}

	// Repeated lines, including close paraphrases and the same opener.
	for _, line := range lines {
		if score, old := mostSimilar(line, history, 8); score >= repeatThreshold {
			return fmt.Sprintf("repeated an earlier line (%d%% similar to %q)", int(score*100), truncate(old, 40))
		}
	}
	if len(lines) == 1 && repeatedOpener(lines[0], history) {
		return "repeated opener — started with the same words as the last reply"
	}
	if repeatedQuestion(joined, history) {
		return "asked the same question again"
	}

	// Never ask for a fact the stranger already gave.
	for _, fact := range askedFacts(joined) {
		if fact == FactSnap {
			continue // mentioning snap is part of the funnel
		}
		if d.factAnswered(fact) {
			return "asked for something they already told you (" + string(fact) + ")"
		}
	}
	if (d.userGaveAsl || d.userAslAsk) && d.UserAge > 0 && d.UserGender != "" && aslFollowUpRe.MatchString(joined) {
		return "asked 'u?' although they already gave their age/gender"
	}

	// Question budget.
	if countQuestions(lines) > 1 {
		return "asked more than one question"
	}
	if countQuestions(lines) > 0 && strings.HasSuffix(strings.TrimSpace(lastBotLine(history)), "?") && !p.IsQuestion && p.Kind != KindGreeting {
		return "two questions in a row"
	}

	// Anchor: the reply must touch something from their message.
	if p.contentful() && !hasAnchor(lines, p) {
		return "did not refer to anything they said"
	}
	return ""
}

// softGuardProblem marks failures that are stylistic: after the retry the line
// is still usable, so the engine prefers it over a canned fallback.
func softGuardProblem(reason string) bool {
	switch {
	case strings.HasPrefix(reason, "did not answer"),
		strings.HasPrefix(reason, "asked more than one question"),
		strings.HasPrefix(reason, "two questions in a row"),
		strings.HasPrefix(reason, "did not refer to anything"):
		return true
	}
	return false
}

// safeReply is the last resort after two rejected attempts. It still matches
// the message type instead of saying something random.
func safeReply(p MessageProfile) []string {
	switch {
	case p.Kind == KindGreeting:
		return []string{"heyy"}
	case p.IsQuestion || p.ImplicitAsk:
		return []string{"idk tbh"}
	case p.Kind == KindFlirt || p.Kind == KindHorny:
		return []string{"stop ur making me blush"}
	case p.Kind == KindLowEffort:
		return []string{"lol fair"}
	case p.Kind == KindJoke:
		return []string{"lol same"}
	}
	return []string{"lol fair"}
}

// answerProblem keeps the old single-call API used by tests.
func answerProblem(lines []string, d *Director, userText string, history []Message) string {
	return validateReply(lines, d, analyzeMessage(userText), userText, history)
}
