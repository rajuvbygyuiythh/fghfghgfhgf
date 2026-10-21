package apichat

// Anti-repetition helpers: near-duplicate lines, repeated openers and repeated
// questions are cheaper to detect here than to hope the model behaves.

import (
	"strings"
)

// normalizeForCompare lowercases and strips punctuation/emoji.
func normalizeForCompare(s string) string {
	s = strings.ToLower(s)
	s = compareClean.ReplaceAllString(s, " ")
	return strings.Join(strings.Fields(s), " ")
}

// shingles builds the unigram + bigram set used for Jaccard similarity.
func shingles(s string) map[string]bool {
	words := strings.Fields(normalizeForCompare(s))
	set := make(map[string]bool, len(words)*2)
	for i, w := range words {
		set["w:"+w] = true
		if i+1 < len(words) {
			set["b:"+w+" "+words[i+1]] = true
		}
	}
	return set
}

// similarity is the Jaccard overlap of two lines, 0..1.
func similarity(a, b string) float64 {
	na, nb := normalizeForCompare(a), normalizeForCompare(b)
	if na == nb {
		return 1
	}
	sa, sb := shingles(a), shingles(b)
	if len(sa) == 0 || len(sb) == 0 {
		return 0
	}
	score := jaccardSet(sa, sb)
	wa, wb := wordSet(na), wordSet(nb)
	if wScore := jaccardSet(wa, wb); wScore > score {
		score = wScore
	}
	return score
}

func wordSet(norm string) map[string]bool {
	words := strings.Fields(norm)
	set := make(map[string]bool, len(words))
	for _, w := range words {
		set[w] = true
	}
	return set
}

func jaccardSet(sa, sb map[string]bool) float64 {
	if len(sa) == 0 || len(sb) == 0 {
		return 0
	}
	inter := 0
	for k := range sa {
		if sb[k] {
			inter++
		}
	}
	union := len(sa) + len(sb) - inter
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}

// lastBotLines returns the newest bot lines first.
func lastBotLines(history []Message, n int) []string {
	out := make([]string, 0, n)
	for i := len(history) - 1; i >= 0 && len(out) < n; i-- {
		if history[i].Role == "assistant" && strings.TrimSpace(history[i].Content) != "" {
			out = append(out, history[i].Content)
		}
	}
	return out
}

// mostSimilar returns the closest earlier bot line and its score.
func mostSimilar(line string, history []Message, n int) (float64, string) {
	best, match := 0.0, ""
	for _, old := range lastBotLines(history, n) {
		if score := similarity(line, old); score > best {
			best, match = score, old
		}
	}
	return best, match
}

// openerOf returns the first two normalized words.
func openerOf(line string) string {
	words := strings.Fields(normalizeForCompare(line))
	switch len(words) {
	case 0:
		return ""
	case 1:
		return words[0]
	default:
		return words[0] + " " + words[1]
	}
}

// repeatedOpener reports an identical opening to one of the last two lines.
func repeatedOpener(line string, history []Message) bool {
	opener := openerOf(line)
	if opener == "" {
		return false
	}
	for i, old := range lastBotLines(history, 2) {
		if i >= 2 {
			break
		}
		if openerOf(old) == opener {
			return true
		}
	}
	return false
}

// questionKeys names the personal questions present in a line.
func questionKeys(line string) []string {
	keys := make([]string, 0, 3)
	for _, fact := range askedFacts(line) {
		switch fact {
		case FactSnap:
			// mentioning snap is fine; asking is handled by the ledger.
			continue
		default:
			keys = append(keys, string(fact))
		}
	}
	low := strings.ToLower(line)
	if howAreYouRe.MatchString(low) {
		keys = append(keys, "howareyou")
	}
	if doingAskRe.MatchString(low) {
		keys = append(keys, "doing")
	}
	return keys
}

// repeatedQuestion reports the same question in the previous two bot lines.
func repeatedQuestion(line string, history []Message) bool {
	keys := questionKeys(line)
	if len(keys) == 0 {
		return false
	}
	previous := map[string]bool{}
	for _, old := range lastBotLines(history, 2) {
		for _, key := range questionKeys(old) {
			previous[key] = true
		}
	}
	for _, key := range keys {
		if previous[key] {
			return true
		}
	}
	return false
}

// lastBotLine returns the newest bot line, or "".
func lastBotLine(history []Message) string {
	lines := lastBotLines(history, 1)
	if len(lines) == 0 {
		return ""
	}
	return lines[0]
}

// countQuestions counts question marks across the generated lines.
func countQuestions(lines []string) int {
	total := 0
	for _, line := range lines {
		total += strings.Count(line, "?")
	}
	return total
}
