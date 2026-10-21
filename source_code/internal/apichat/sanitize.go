package apichat

// Output guards: the model writes the words, these functions make sure the
// words are safe and human. Ported from the apitest tester.

import (
	"fmt"
	"regexp"
	"strings"
)

var aslFollowUpRe = regexp.MustCompile(`(?i)\b(u|you|hbu|wbu|and u|n u|urs|yours)\s*\?`)
var ageAskRe = regexp.MustCompile(`(?i)\b(how old|ur age|your age|asl)\b`)
var genderAskRe = regexp.MustCompile(`(?i)\b(m or f|f or m|r u a (girl|guy)|are (u|you) a (girl|guy)|guy or girl|boy or girl)\b`)
var placeAskRe = regexp.MustCompile(`(?i)\b(where (r|are) (u|you) from|where u from|where from|where (r|are) (u|you) at|wya)\b`)
var nameAskAgainRe = regexp.MustCompile(`(?i)\b(ur name|your name|whats ur name|what's your name)\b`)
var stopMarkerRe = regexp.MustCompile(`(?i)<\|(?:end|eot_id|im_end|endoftext|user|human)\|>|</s>`)
var specialTokenRe = regexp.MustCompile(`(?i)<\|[^|>]{1,32}\|>|</?s>`)

// sanitizeLines turns raw model output into chat lines: strips thinking,
// special model end tokens (<|end|>, <|eot_id|>, <|im_end|>), quotes, list
// markers, prefixes, exclamation marks and trailing punctuation.
// The second return value is a warning string for logs (empty when clean).
func sanitizeLines(raw string, double bool) ([]string, string) {
	text := strings.TrimSpace(thinkRe.ReplaceAllString(raw, ""))
	var warns []string
	if strings.Contains(raw, "<think>") {
		warns = append(warns, "model emitted <think> reasoning (stripped)")
	}
	if loc := stopMarkerRe.FindStringIndex(text); loc != nil {
		text = strings.TrimSpace(text[:loc[0]])
	}
	text = strings.TrimSpace(specialTokenRe.ReplaceAllString(text, ""))

	var lines []string
	for _, l := range strings.Split(text, "\n") {
		l = strings.TrimSpace(strings.Trim(strings.TrimSpace(l), "\"'“”`"))
		l = strings.TrimLeft(l, "-•*0123456789. ")
		lower := strings.ToLower(l)
		for _, prefix := range []string{"you:", "me:", "eva:", "bot:", "line 1:", "line 2:", "line1:", "line2:"} {
			if strings.HasPrefix(lower, prefix) {
				l = strings.TrimSpace(l[len(prefix):])
				break
			}
		}
		l = strings.ReplaceAll(l, "—", ", ")
		l = strings.ReplaceAll(l, " - ", ", ")
		l = strings.Join(strings.Fields(l), " ")
		l = strings.ReplaceAll(l, "!", "")
		l = trailingDotRe.ReplaceAllString(l, " $1")
		l = strings.TrimRight(strings.Join(strings.Fields(l), " "), ".,! ")
		if l != "" {
			lines = append(lines, strings.ToLower(l))
		}
	}
	if len(lines) == 0 {
		return nil, "empty reply"
	}

	want := 1
	if double {
		want = 2
	}
	if len(lines) > want {
		warns = append(warns, fmt.Sprintf("model wrote %d lines, kept %d", len(lines), want))
		lines = lines[:want]
	}
	for _, l := range lines {
		if len([]rune(l)) > 120 {
			warns = append(warns, "line too long (>120 chars)")
		}
		if n := len(strings.Fields(l)); n > 14 {
			warns = append(warns, fmt.Sprintf("%d words (target <=10)", n))
		}
		if forbiddenRe.MatchString(l) {
			warns = append(warns, "contains @ / link / ai-talk")
		}
		if strings.Count(l, "?") > 1 {
			warns = append(warns, "two questions in one line")
		}
	}
	return lines, strings.Join(warns, "; ")
}

// onlyQuestion reports whether the line is nothing but a question.
func onlyQuestion(l string) bool {
	l = strings.TrimSpace(strings.ToLower(l))
	if !strings.HasSuffix(l, "?") {
		return false
	}
	f := strings.Fields(strings.Trim(l, "?"))
	if len(f) == 0 {
		return true
	}
	switch strings.Trim(f[0], ",.") {
	case "what", "how", "where", "why", "who", "when", "which", "wyd", "hbu", "wbu", "r", "are", "do", "did", "does", "u", "you", "can", "could", "would", "will", "is", "got", "any", "so", "and", "ur", "your", "whats", "what's", "hows", "how's", "wanna", "want", "ever", "have", "wya", "asl":
		return len(f) <= 7
	}
	return false
}

// parrotGram returns a 4-word run of the stranger's message that appears
// verbatim in the reply (models love to echo).
func parrotGram(user, reply string) string {
	clean := func(t string) []string {
		return strings.Fields(regexp.MustCompile(`[^a-z0-9' ]+`).ReplaceAllString(t, " "))
	}
	uw := clean(user)
	rw := " " + strings.Join(clean(reply), " ") + " "
	for i := 0; i+4 <= len(uw); i++ {
		g := strings.Join(uw[i:i+4], " ")
		if strings.Contains(rw, " "+g+" ") {
			return g
		}
	}
	return ""
}

// polish removes bot-tells that are cheaper to fix than to retry: duplicated
// emoji, emoji in a recent reply, and repeated "lol"/"haha" openers.
func polish(lines []string, history []Message) []string {
	var recent []string
	for i := len(history) - 1; i >= 0 && len(recent) < 3; i-- {
		if history[i].Role == "assistant" {
			recent = append(recent, history[i].Content)
		}
	}
	recentEmoji := false
	usedEmoji := map[rune]bool{}
	for _, m := range history {
		if m.Role != "assistant" {
			continue
		}
		for _, r := range m.Content {
			if isEmoji(r) {
				usedEmoji[r] = true
			}
		}
	}
	for _, r := range recent {
		for _, c := range r {
			if isEmoji(c) {
				recentEmoji = true
			}
		}
	}
	lastOpeners := map[string]bool{}
	for i, r := range recent {
		if i > 1 {
			break
		}
		if f := strings.Fields(r); len(f) > 0 {
			w := strings.Trim(f[0], ",.")
			if w == "lol" || w == "haha" || w == "lmao" || w == "omg" {
				lastOpeners[w] = true
			}
		}
	}

	out := make([]string, 0, len(lines))
	for _, l := range lines {
		var b strings.Builder
		kept := false
		for _, c := range l {
			if isEmoji(c) {
				if recentEmoji || usedEmoji[c] || kept {
					continue
				}
				kept = true
			}
			b.WriteRune(c)
		}
		l = strings.TrimRight(strings.TrimSpace(strings.Join(strings.Fields(b.String()), " ")), ".,! ")
		if f := strings.Fields(l); len(f) > 1 && lastOpeners[strings.Trim(f[0], ",.")] {
			l = strings.TrimSpace(strings.TrimLeft(strings.Join(f[1:], " "), ", "))
		}
		if l != "" {
			out = append(out, l)
		}
	}
	if len(out) == 0 {
		return lines
	}
	return out
}

func isEmoji(r rune) bool {
	return r >= 0x1F300 && r <= 0x1FAFF || r >= 0x2600 && r <= 0x27BF || r == 0x2764 || r == 0xFE0F
}
