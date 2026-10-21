package apichat

// Message understanding: turn one stranger SMS into a structured profile.
// The director uses it to react to what was actually said instead of guessing
// from a question mark, and the reply guard uses it as the anchor check.

import (
	"regexp"
	"strings"
)

// MsgKind is the dominant type of one incoming message.
type MsgKind string

const (
	KindOther       MsgKind = "other"
	KindGreeting    MsgKind = "greeting"
	KindQuestion    MsgKind = "question"
	KindImplicitAsk MsgKind = "implicit_ask"
	KindCommand     MsgKind = "command"
	KindSharing     MsgKind = "sharing"
	KindFlirt       MsgKind = "flirt"
	KindHorny       MsgKind = "horny"
	KindJoke        MsgKind = "joke"
	KindInsult      MsgKind = "insult"
	KindMoney       MsgKind = "money"
	KindMeta        MsgKind = "meta"
	KindLeaving     MsgKind = "leaving"
	KindLowEffort   MsgKind = "low_effort"
	KindNonEnglish  MsgKind = "non_english"
)

// MessageProfile is the parsed form of the stranger's last message.
type MessageProfile struct {
	Kind        MsgKind
	IsQuestion  bool
	ImplicitAsk bool
	Command     string
	Topics      []string
	Sentiment   int // -1 negative, 0 neutral, 1 positive
	Words       int
	Runes       int
	Parts       []string
	Keywords    []string
	HasEmoji    bool
}

var (
	implicitAskRe = regexp.MustCompile(`(?i)\b(i bet|u probably|you probably|i wonder|wish (u|you)|tell me|u should|you should|how about (u|you)|what about (u|you)|u ever|you ever|would u|you look|u look|u seem|you seem|im curious|i'm curious|guess what|u think|you think)\b`)
	commandRe     = regexp.MustCompile(`(?i)\b(send|show|drop|give)\b[^.?!]{0,16}\b(pics?|photo|selfie|snap|number|nudes?)\b|\b(come|call|add) (me|here|over)\b`)
	greetingRe    = regexp.MustCompile(`(?i)^\s*(hey+|hi+|hello+|yo+|sup|hola|good (morning|evening|night)|gm|gn)\b`)
	positiveRe    = regexp.MustCompile(`(?i)\b(nice|cool|good|great|love|like|fun|happy|thanks|thx|sweet|cute|pretty|amazing)\b`)
	negativeRe    = regexp.MustCompile(`(?i)\b(bad|sad|tired|hate|boring|annoyed|annoying|angry|mad|upset|lonely|sick|hurt|stress\w*)\b`)
	partSplitRe   = regexp.MustCompile(`[.!?]+|\s+and\s+|\s+but\s+|\s+because\s+`)
	compareClean  = regexp.MustCompile(`[^a-z0-9 ]+`)
	doingAskRe    = regexp.MustCompile(`(?i)\b(wyd|what (r|are) (u|you) doing|what do (u|you) do|whats ur job|what's your job|where do (u|you) work)\b`)
	howAreYouRe   = regexp.MustCompile(`(?i)\b(how are (u|you)|how r (u|you)|hru|how's it going|hows it going)\b`)
)

// topicOrder keeps topic output deterministic (maps are random in Go).
var topicOrder = []string{"work", "school", "sleep", "bored", "lonely", "music", "shows", "food", "gym", "gaming", "relationship", "looks", "place", "age", "snap"}

var topicRe = map[string]*regexp.Regexp{
	"work":         regexp.MustCompile(`(?i)\b(work|working|job|shift|office|boss|busy)\b`),
	"school":       regexp.MustCompile(`(?i)\b(school|college|uni|university|class|exam|exams|study|studying|major|semester)\b`),
	"sleep":        regexp.MustCompile(`(?i)\b(sleep|sleepy|tired|awake|insomnia|bed|nap)\b`),
	"bored":        regexp.MustCompile(`(?i)\b(bored|boring|nothing much|nothin|nm|chillin|chilling)\b`),
	"lonely":       regexp.MustCompile(`(?i)\b(alone|lonely|nobody|no one|miss (u|you)|miss someone)\b`),
	"music":        regexp.MustCompile(`(?i)\b(music|song|songs|spotify|playlist|album|band|listening)\b`),
	"shows":        regexp.MustCompile(`(?i)\b(netflix|movie|movies|show|shows|series|episode|tiktok|youtube)\b`),
	"food":         regexp.MustCompile(`(?i)\b(food|eat|eating|hungry|snack|snacks|pizza|coffee|ramen|cereal)\b`),
	"gym":          regexp.MustCompile(`(?i)\b(gym|workout|work out|running|lifting)\b`),
	"gaming":       regexp.MustCompile(`(?i)\b(game|games|gaming|ps5|xbox|valorant|fortnite|minecraft)\b`),
	"relationship": regexp.MustCompile(`(?i)\b(single|bf|gf|boyfriend|girlfriend|ex|dating|taken)\b`),
	"looks":        regexp.MustCompile(`(?i)\b(cute|pretty|hot|beautiful|gorgeous|handsome|sexy)\b`),
	"place":        knownPlaces,
	"age":          regexp.MustCompile(`(?i)\b(asl|age|old)\b`),
	"snap":         snapAskRe,
}

var stopWords = map[string]bool{}

func init() {
	const words = "the a an i im i'm u you ur your we they he she it is are am was be been do did does have has had not no yes yea yeah ok okay k lol lmao haha hey hi hello sup yo wyd hbu wbu what where when why who how which and or but so if then than too also just only like wanna gonna want need can could would should will may might from in on at to for of with about by up down out off over here there this that these those my me mine our us them him her his its one two three just really very pretty kinda sorta tbh ngl fr lowkey omg bruh bro dude babe baby bae damn shit wtf pls please thanks thx ty np btw send show tell give add call come"
	for _, w := range strings.Fields(words) {
		stopWords[w] = true
	}
}

// analyzeMessage parses one incoming chat line. It never calls the network.
func analyzeMessage(text string) MessageProfile {
	low := strings.ToLower(strings.TrimSpace(text))
	p := MessageProfile{Runes: len([]rune(low))}
	words := strings.Fields(low)
	p.Words = len(words)
	p.Parts = splitParts(low)
	p.Keywords = keywordsOf(low)
	p.Topics = topicsOf(low)
	p.HasEmoji = hasEmoji(low)
	p.Sentiment = sentimentOf(low)
	p.IsQuestion = strings.Contains(low, "?") || questionRe.MatchString(low) || aslAskRe.MatchString(low)
	p.ImplicitAsk = implicitAskRe.MatchString(low)
	if m := commandRe.FindString(low); m != "" {
		p.Command = commandKind(m)
	}

	switch {
	case looksNonEnglish(low):
		p.Kind = KindNonEnglish
	case insultRe.MatchString(low):
		p.Kind = KindInsult
	case moneyRe.MatchString(low):
		p.Kind = KindMoney
	case hornyRe.MatchString(low):
		p.Kind = KindHorny
	case botQRe.MatchString(low):
		p.Kind = KindMeta
	case byeRe.MatchString(low):
		p.Kind = KindLeaving
	case flirtyRe.MatchString(low):
		p.Kind = KindFlirt
	case p.Command != "":
		p.Kind = KindCommand
	case p.IsQuestion:
		p.Kind = KindQuestion
	case p.ImplicitAsk:
		p.Kind = KindImplicitAsk
	case laughRe.MatchString(low):
		p.Kind = KindJoke
	case greetingRe.MatchString(low):
		p.Kind = KindGreeting
	case lowEffortRe.MatchString(strings.TrimSpace(low)):
		p.Kind = KindLowEffort
	case len(words) >= 4:
		p.Kind = KindSharing
	default:
		p.Kind = KindOther
	}
	return p
}

func commandKind(match string) string {
	m := strings.ToLower(match)
	switch {
	case strings.Contains(m, "pic"), strings.Contains(m, "photo"), strings.Contains(m, "selfie"), strings.Contains(m, "nude"):
		return "pic"
	case strings.Contains(m, "snap"):
		return "snap"
	case strings.Contains(m, "number"):
		return "number"
	case strings.Contains(m, "call"):
		return "call"
	case strings.Contains(m, "come"):
		return "meet"
	case strings.Contains(m, "add"):
		return "add"
	}
	return "request"
}

// splitParts splits a multi-topic message into at most three meaningful parts.
func splitParts(low string) []string {
	raw := partSplitRe.Split(low, -1)
	out := make([]string, 0, 3)
	seen := map[string]bool{}
	for _, part := range raw {
		part = strings.Join(strings.Fields(part), " ")
		if len(strings.Fields(part)) < 2 || strings.Contains(part, "%username%") {
			continue
		}
		if seen[part] {
			continue
		}
		seen[part] = true
		out = append(out, part)
		if len(out) == 3 {
			break
		}
	}
	return out
}

// keywordsOf keeps the concrete words a reply must react to.
func keywordsOf(low string) []string {
	cleaned := compareClean.ReplaceAllString(strings.ToLower(low), " ")
	out := make([]string, 0, 6)
	seen := map[string]bool{}
	for _, word := range strings.Fields(cleaned) {
		if len(word) < 3 || stopWords[word] || seen[word] || englishWords[word] && len(word) < 4 {
			continue
		}
		seen[word] = true
		out = append(out, word)
		if len(out) == 6 {
			break
		}
	}
	return out
}

func topicsOf(low string) []string {
	out := make([]string, 0, 4)
	for _, topic := range topicOrder {
		if re := topicRe[topic]; re != nil && re.MatchString(low) {
			out = append(out, topic)
		}
	}
	return out
}

func sentimentOf(low string) int {
	pos := len(positiveRe.FindAllString(low, -1))
	neg := len(negativeRe.FindAllString(low, -1))
	switch {
	case pos > neg:
		return 1
	case neg > pos:
		return -1
	}
	return 0
}

func hasEmoji(s string) bool {
	for _, r := range s {
		if isEmoji(r) {
			return true
		}
	}
	return false
}

// hasAnchor reports whether the reply mentions something from the message.
func hasAnchor(lines []string, p MessageProfile) bool {
	if len(p.Keywords) == 0 && len(p.Topics) == 0 {
		return true
	}
	joined := strings.ToLower(strings.Join(lines, " "))
	clean := " " + compareClean.ReplaceAllString(joined, " ") + " "
	for _, kw := range p.Keywords {
		if strings.Contains(clean, " "+kw+" ") {
			return true
		}
	}
	for _, topic := range p.Topics {
		if re := topicRe[topic]; re != nil && re.MatchString(joined) {
			return true
		}
	}
	return false
}

// contentful reports whether the message carries something worth anchoring to.
func (p MessageProfile) contentful() bool {
	switch p.Kind {
	case KindLowEffort, KindOther, KindGreeting, KindLeaving, KindNonEnglish,
		KindQuestion, KindImplicitAsk, KindCommand, KindMeta, KindFlirt, KindHorny:
		// Questions and asks are handled by their own rules (answer first,
		// obey or dodge the command). Flirting and horny messages are answered
		// with the matching vibe, so they do not need a lexical anchor.
		return false
	}
	return len(p.Keywords) >= 2 || len(p.Topics) > 0
}
