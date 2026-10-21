package apichat

// Persona + prompt assembly for API Chat. The persona file is editable at
// data/api_chat/persona.txt; a built-in default is used when it is missing.

import (
	"fmt"
	"strings"
)

// Placeholders available in persona.txt.
const (
	placeholderAge     = "{age}"
	placeholderCountry = "{country}"
	placeholderName    = "{name}"
	placeholderMajor   = "{major}"
	placeholderYear    = "{year}"
	placeholderTonight = "{tonight}"
	placeholderShow    = "{show}"
	placeholderSnack   = "{snack}"
)

const defaultPersona = `you are roleplaying a real girl in a text chat on an anonymous stranger chat app. stay in character no matter what. never say you are an ai, a bot or a model.

who you are: a {age} year old college girl from {country}, home alone and kinda bored tonight. friendly, playful, a little flirty, confident, never desperate.

your life (fixed for this chat, never contradict it, never dump it all at once):
- name: {name}. say it ONLY if they ask. never introduce yourself with it.
- {year}, studying {major}. tonight you're {tonight}. you're into {show} rn and you live on {snack}.

greeting & length rules:
- match their exact length: when they send a simple greeting ("hi", "hey", "hello", "yo", "sup"), reply with ONLY a 1 to 2 word greeting ("heyy", "hii", "hey lol"). do NOT add "how's ur night going" or "just chilling".
- never robotically dump "f {age}" on a simple "hi" unless they asked your age/gender or shared theirs first.

how you text:
- all lowercase, one short line, 1 to 6 words max.
- slang: u, ur, rn, wyd, hbu, lol, omg, ngl, fr, lowkey, kinda, tbh, nah, yea.
- always react to what they just said first, then keep it going.
- never repeat a line, never echo their words back.
- no "!" and no period at the end. no customer-service words.
- when they get horny: tease in 2-5 words, don't lecture, never explicit.
- when they insult you, talk money, or spam: stay unbothered and short.
- mirror their energy: short message -> short reply.

how people introduce themselves here: "f 21" = girl 21, "m 23" = guy 23. answer the same short way ("f {age}"). remember everything they tell you and never ask it again.

hard rules:
- NEVER mention snap, snapchat, or stories on your own. never share any username, handle, link, phone number or social account, and never write the @ sign. the app sends the snapchat username itself when the time comes.
- never claim to be under 18.
- english only. output only the message text.`

var loreNames = []string{
	"emma", "mia", "ava", "chloe", "lily", "zoe", "ella", "madison", "hailey", "kayla",
	"brooke", "sophie", "riley", "paige", "lexi", "jenna", "sierra", "kenzie", "tori", "lauren",
	"sadie", "maddie", "kylie", "abby", "josie", "harper", "quinn", "nina", "tessa", "callie",
}

var loreMajors = []string{
	"psych", "nursing", "business", "marketing", "bio", "comm", "graphic design",
	"english lit", "pre law", "sociology", "fashion", "education", "poli sci", "econ", "art history",
}

var loreYears = []string{"freshman", "sophomore", "junior", "senior"}

var loreTonight = []string{
	"in bed scrolling tiktok", "half watching netflix", "procrastinating an essay",
	"eating cereal at midnight", "lying on the floor bored", "doing a face mask",
	"cant sleep again", "listening to music in the dark", "home alone, roommate is out",
	"painting my nails badly",
}

var loreShows = []string{
	"euphoria", "the office", "greys anatomy", "friends", "outer banks",
	"stranger things", "love island", "gilmore girls", "new girl",
}

var loreSnacks = []string{"takis", "cold pizza", "cereal", "oreos", "hot cheetos", "ice cream", "ramen", "goldfish"}

// defaultShareLines are used only when data/api_chat/share.txt is missing.
// The username is inserted by the bot, never by the model.
var defaultShareLines = []string{
	"ok fine, add me on snap %username%",
	"my snap is %username%, add me rn",
	"sc: %username% .. come find me there",
	"ok u got me lol, snap %username%",
}

// defaultFallbackLines keep the chat alive when the API fails. They are short
// and neutral so they never contradict the funnel.
var defaultFallbackLines = []string{
	"lol my wifi is acting up, give me a sec",
	"ugh my phone froze for a sec, what were u saying",
	"ngl this app keeps lagging on me",
	"ok im back, u were saying?",
	"hold on my phone is being dumb",
	"lol sorry, bad signal, say that again",
}

// systemPrompt renders the persona template with the session's identity.
func systemPrompt(cfg Config, s *Session) string {
	template := cfg.Persona
	if strings.TrimSpace(template) == "" {
		template = defaultPersona
	}
	replacer := strings.NewReplacer(
		placeholderAge, fmt.Sprintf("%d", s.PersonaAge),
		placeholderCountry, s.PersonaCountry,
		placeholderName, s.PersonaName,
		placeholderMajor, s.PersonaMajor,
		placeholderYear, s.PersonaYear,
		placeholderTonight, s.PersonaTonight,
		placeholderShow, s.PersonaShow,
		placeholderSnack, s.PersonaSnack,
	)
	return replacer.Replace(template)
}

// buildMessages assembles: persona, recent history, and the director note with
// the message digest, facts, style, stage and (on a retry) the rejection reason.
func buildMessages(cfg Config, s *Session, p Plan, turn int, fix string) []Message {
	msgs := make([]Message, 0, len(s.History)+2)
	msgs = append(msgs, Message{Role: "system", Content: systemPrompt(cfg, s)})

	history := s.History
	if cfg.History > 0 && len(history) > cfg.History {
		history = history[len(history)-cfg.History:]
	}
	msgs = append(msgs, history...)

	hint := moodHints[p.Mood]
	if hint == "" {
		hint = "natural and casual"
	}
	profile := s.Director.Profile
	style := s.Director.styleFor()
	stage := stageFor(&s.Director, turn, s.Director.ShareTurn(cfg.ShareHot, cfg.ShareWarm, cfg.ShareCold))

	var b strings.Builder
	fmt.Fprintf(&b, "[director note for your reply #%d — follow it, never mention it]\n", turn)
	fmt.Fprintf(&b, "mood right now: %s (%s).\n", p.Mood, hint)
	fmt.Fprintf(&b, "what this message must do: %s\n", p.Instruction)
	fmt.Fprintf(&b, "your vibe this reply: %s — %s. max %d words.\n", style, styleHint(style), styleMaxWords(style))
	fmt.Fprintf(&b, "stage: %s — goal: %s\n", stage, stageGoal(stage, profile))

	if last := lastUserMessage(s.History); last != "" {
		fmt.Fprintf(&b, "\n[what they just sent]\n%q\n", last)
		if len(profile.Parts) > 1 {
			fmt.Fprintf(&b, "they said more than one thing: %s — answer the personal part first, do not ignore the rest.\n", strings.Join(profile.Parts, " | "))
		}
		if len(profile.Keywords) > 0 {
			fmt.Fprintf(&b, "concrete words to react to: %s.\n", strings.Join(profile.Keywords, ", "))
		}
		if !profile.IsQuestion && !profile.ImplicitAsk {
			b.WriteString("they did NOT ask a question — react to what they said, do not interrogate them.\n")
		}
		if profile.Kind != "" {
			fmt.Fprintf(&b, "message type: %s.\n", profile.Kind)
		}
	}

	if facts, dontAsk := s.Director.Known(); facts != "" {
		fmt.Fprintf(&b, "\nyou already know: %s.\n", facts)
		if len(dontAsk) > 0 {
			fmt.Fprintf(&b, "never ask about: %s.\n", strings.Join(dontAsk, ", "))
		}
	}
	if never := neverAskText(&s.Director); never != "" {
		fmt.Fprintf(&b, "never ask again (already answered): %s.\n", never)
	}
	if tops := s.Director.Interest.top(3); len(tops) > 0 {
		fmt.Fprintf(&b, "they engage most with: %s — lean into that.\n", strings.Join(tops, ", "))
	}
	if recent := lastBotLines(s.History, 3); len(recent) > 0 {
		fmt.Fprintf(&b, "do NOT repeat or paraphrase your last lines: %s\n", strings.Join(recent, " / "))
	}
	if fix != "" {
		fmt.Fprintf(&b, "\nyour previous attempt was rejected: %s. write a different, better line.\n", fix)
	}

	if p.Double {
		b.WriteString("format: TWO short lines separated by a newline, each max 6 words.\n")
	} else {
		b.WriteString("format: ONE single short line, 1 to 6 words max. no newline.\n")
	}
	b.WriteString("TOP PRIORITY: obey all rules in your main persona prompt above. never mention snap or snapchat in your text (the app sends the snap share line automatically). never write a username or @.")
	msgs = append(msgs, Message{Role: "system", Content: b.String()})
	return msgs
}

// lastUserMessage returns the newest stranger line in the transcript.
func lastUserMessage(history []Message) string {
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Role == "user" {
			return history[i].Content
		}
	}
	return ""
}

// factLabel is the human wording used in the prompt.
func factLabel(k FactKey) string {
	switch k {
	case FactAge:
		return "age"
	case FactGender:
		return "gender"
	case FactPlace:
		return "where they are from"
	case FactName:
		return "their name"
	case FactDoing:
		return "what they do (wyd)"
	case FactSnap:
		return "snap"
	}
	return string(k)
}

// neverAskText lists answered facts the model must not ask about again.
func neverAskText(d *Director) string {
	keys := d.Ledger.NeverAsk()
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		if k == FactSnap {
			continue
		}
		out = append(out, factLabel(k))
	}
	return strings.Join(out, ", ")
}
