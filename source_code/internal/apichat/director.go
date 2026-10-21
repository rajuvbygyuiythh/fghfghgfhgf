package apichat

// The director is the deterministic funnel brain: it reads the stranger's
// interest, decides the mood, and tells the model exactly what the next line
// must do. The model only writes the words. This is a production port of the
// design validated in apitest/PLAN.md.

import (
	"fmt"
	"math/rand"
	"strconv"
	"strings"
)

// Plan is one instruction for the model for a single bot reply.
type Plan struct {
	Mood        string
	Instruction string
	Double      bool
	ShareNow    bool // append a share line (with the username) after the lead-in
	EndAfter    bool // conversation ends after this reply
	Fixed       string
	Why         string
}

// Director holds the funnel state for one conversation. Exported fields are
// persisted with the session; the per-message flags are recomputed every turn.
type Director struct {
	Score          int      `json:"score"`
	Tier           string   `json:"tier"`
	HornyHits      int      `json:"horny_hits"`
	FlirtyHits     int      `json:"flirty_hits"`
	ColdStreak     int      `json:"cold_streak"`
	Underage       bool     `json:"underage"`
	UserAge        int      `json:"user_age,omitempty"`
	UserGender     string   `json:"user_gender,omitempty"`
	UserPlace      string   `json:"user_place,omitempty"`
	UserName       string   `json:"user_name,omitempty"`
	UserDoing      string   `json:"user_doing,omitempty"`
	AskedSnap      bool     `json:"asked_snap,omitempty"`
	Shared         bool     `json:"shared,omitempty"`
	SharedTurn     int      `json:"shared_turn,omitempty"`
	Ended          bool     `json:"ended,omitempty"`
	EndReason      string   `json:"end_reason,omitempty"`
	Nudges         int      `json:"nudges,omitempty"`
	Hesitated      bool     `json:"hesitated,omitempty"`
	HookPlanted    bool     `json:"hook_planted,omitempty"`
	WantsSnapEarly bool     `json:"wants_snap_early,omitempty"`
	LastMood       string   `json:"last_mood,omitempty"`
	LastDouble     int      `json:"last_double,omitempty"`
	NoQuestionAt   int      `json:"no_question_at,omitempty"`
	NonEngHits     int      `json:"non_eng_hits,omitempty"`
	MoneyHits      int      `json:"money_hits,omitempty"`
	ToldAsl        bool     `json:"told_asl,omitempty"`
	ToldName       bool     `json:"told_name,omitempty"`
	LastUserText   string   `json:"last_user_text,omitempty"`
	Moods          []string `json:"moods,omitempty"`
	RngState       uint64   `json:"rng_state,omitempty"`

	// Quality layer state (persisted with the conversation).
	Turn         int            `json:"turn,omitempty"`
	Style        Style          `json:"style,omitempty"`
	Interest     Interest       `json:"interest,omitempty"`
	Ledger       Ledger         `json:"ledger,omitempty"`
	LastTopic    string         `json:"last_topic,omitempty"`
	SnapMentions int            `json:"snap_mentions,omitempty"`
	SnapDodges   int            `json:"snap_dodges,omitempty"`
	Profile      MessageProfile `json:"-"`

	// per-message flags, never persisted
	userRepeat    bool
	userInsult    bool
	userMoney     bool
	userHardQ     bool
	userNameAsk   bool
	userNoSnap    bool
	userNonEng    bool
	userAskedSnap bool
	userQuestion  bool
	userHorny     bool
	userFlirty    bool
	userBye       bool
	userBotQ      bool
	userPicAsk    bool
	userLaugh     bool
	userLonely    bool
	userRefused   bool
	userHesitant  bool
	userAdded     bool
	userAslAsk    bool
	userGaveAsl   bool
	userGreeting  bool
	userLowEffort bool
	userShared    bool
}

var allMoods = []string{"friendly", "playful", "lonely", "flirty", "fun/joke", "curious", "teasing"}
var hotMoods = []string{"flirty", "teasing", "playful"}

var moodHints = map[string]string{
	"friendly": "warm and easy, like texting a new friend",
	"playful":  "light, cheeky, a little sarcastic in a cute way",
	"lonely":   "a bit bored and alone tonight, wants someone to talk to, soft",
	"flirty":   "subtle flirting, compliments back, a little suggestive but never explicit",
	"fun/joke": "make a tiny joke or funny exaggeration about what they said",
	"curious":  "genuinely curious about them, ask one specific thing",
	"teasing":  "tease them playfully about what they said, confident",
}

// NewDirector builds a director with a shuffled mood rotation.
func NewDirector(seed int64) Director {
	shuffled := append([]string(nil), allMoods...)
	rng := rand.New(rand.NewSource(seed))
	rng.Shuffle(len(shuffled), func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })
	if seed == 0 {
		seed = 1
	}
	return Director{Tier: tierCold, Moods: shuffled, RngState: uint64(seed)}
}

// roll returns a deterministic pseudo-random number in [0,n) so the same
// session state always produces the same behaviour after a restart.
func (d *Director) roll(n int) int {
	if n <= 1 {
		return 0
	}
	d.RngState = d.RngState*6364136223846793005 + 1442695040888963407
	return int((d.RngState >> 33) % uint64(n))
}

// Observe scores one stranger message and updates the funnel state.
func (d *Director) Observe(text string) {
	low := strings.ToLower(strings.TrimSpace(text))
	words := len(strings.Fields(low))

	// Quality layer: understand the message and score how the stranger
	// reacted to the topic the bot picked last turn.
	d.Profile = analyzeMessage(low)
	d.observeInterest()

	d.userRepeat = low != "" && low == d.LastUserText && len(low) > 1
	d.LastUserText = low
	d.userInsult = insultRe.MatchString(low)
	d.userMoney = moneyRe.MatchString(low)
	if d.userMoney {
		d.MoneyHits++
	}
	d.userHardQ = hardQRe.MatchString(low)
	d.userNameAsk = nameAskRe.MatchString(low)
	d.userNoSnap = noSnapRe.MatchString(low)
	d.userNonEng = looksNonEnglish(low)
	if d.userNonEng {
		d.NonEngHits++
	}
	d.userAskedSnap = snapAskRe.MatchString(low) && !d.userNoSnap
	d.userQuestion = strings.Contains(low, "?") || questionRe.MatchString(low)
	d.userHorny = hornyRe.MatchString(low) && !d.userInsult
	d.userFlirty = flirtyRe.MatchString(low)
	d.userBye = byeRe.MatchString(low)
	d.userBotQ = botQRe.MatchString(low)
	d.userPicAsk = picAskRe.MatchString(low)
	d.userLaugh = laughRe.MatchString(low)
	d.userLonely = lonelyRe.MatchString(low)
	d.userAdded = d.Shared && addedRe.MatchString(low)
	d.userAslAsk = aslAskRe.MatchString(low)
	if d.userAslAsk {
		d.userQuestion = true
	}
	d.userGreeting = greetingRe.MatchString(low)

	_, _, hasAsl := genderAge(low)
	d.userLowEffort = !hasAsl && !d.userAskedSnap && !d.userGreeting && (words <= 2 && lowEffortRe.MatchString(low) || len([]rune(low)) <= 3)
	d.userShared = !d.userQuestion && !d.userLowEffort && words >= 4
	d.userRefused = d.AskedSnap && !d.Shared && refuseRe.MatchString(low) && !yesRe.MatchString(low)
	d.userHesitant = d.AskedSnap && !d.Shared && !d.userRefused && hesitantRe.MatchString(low) && !yesRe.MatchString(low)

	// Track how often the stranger ignored the snap question. After two
	// ignores the bot stops pushing instead of nagging.
	switch {
	case d.userAskedSnap || d.Shared || yesRe.MatchString(low):
		d.SnapDodges = 0
	case d.AskedSnap && !d.userRefused && !d.userHesitant:
		d.SnapDodges++
	}

	d.userGaveAsl = false
	if g, a, ok := genderAge(low); ok {
		d.userGaveAsl = true
		if d.UserAge == 0 {
			d.UserAge = a
		}
		if d.UserGender == "" && g != "" {
			d.UserGender = g
		}
		if a < 18 {
			d.Underage = true
		}
	}
	if d.UserGender == "" {
		switch {
		case bareMaleRe.MatchString(low):
			d.UserGender, d.userGaveAsl = "m", true
		case bareFemaleRe.MatchString(low):
			d.UserGender, d.userGaveAsl = "f", true
		}
	}
	if d.UserAge == 0 {
		if m := bareAgeRe.FindStringSubmatch(low); m != nil {
			if n, err := strconv.Atoi(m[1]); err == nil && n >= 13 && n <= 70 {
				d.UserAge, d.userGaveAsl = n, true
				if n < 18 {
					d.Underage = true
				}
			}
		}
	}
	if d.UserPlace == "" {
		if m := knownPlaces.FindString(low); m != "" {
			d.UserPlace = m
		} else if m := placeRe.FindStringSubmatch(low); m != nil {
			cand := m[1]
			first := strings.Fields(cand)
			if len(first) > 0 && !placeStop.MatchString(first[0]) && !d.userHorny && !lowEffortRe.MatchString(first[0]) {
				d.UserPlace = cand
			}
		}
	}
	if d.UserName == "" {
		if m := nameRe.FindStringSubmatch(low); m != nil && !nameStop.MatchString(m[1]) && !englishWords[m[1]] && !knownPlaces.MatchString(m[1]) && !doingRe.MatchString(m[1]) && !hornyRe.MatchString(m[1]) && !flirtyRe.MatchString(m[1]) {
			if _, _, isAsl := genderAge(m[1]); !isAsl {
				d.UserName = m[1]
			}
		}
	}
	if m := doingRe.FindString(low); m != "" {
		d.UserDoing = m
	}
	if d.userGaveAsl {
		d.userLowEffort = false
	}
	if d.userLowEffort {
		d.ColdStreak++
	} else {
		d.ColdStreak = 0
	}

	switch {
	case d.userHorny:
		d.Score += 3
		d.HornyHits++
	case d.userFlirty || d.userPicAsk:
		d.Score += 2
		d.FlirtyHits++
	}
	if d.userQuestion {
		d.Score++
	}
	if len([]rune(text)) > 25 {
		d.Score++
	}
	if strings.ContainsAny(text, "😏😘😍🥵😂❤️🔥") {
		d.Score++
	}
	switch {
	case d.HornyHits >= 2 || d.Score >= 6:
		d.Tier = tierHot
	case d.HornyHits > 0 || d.Score >= 3:
		d.Tier = tierWarm
	default:
		d.Tier = tierCold
	}

	// Facts learned in this very message must be protected for the reply we
	// are about to write, so the ledger sync happens last.
	d.syncLedger()
}

// Known returns what the stranger already told us, plus the topics the model
// must never ask about again.
func (d *Director) Known() (string, []string) {
	var parts []string
	var dontAsk []string
	if d.UserGender != "" || d.UserAge > 0 {
		parts = append(parts, strings.TrimSpace(fmt.Sprintf("%s %s", d.UserGender, nz(d.UserAge))))
	}
	if d.UserGender != "" {
		dontAsk = append(dontAsk, "gender / m or f")
	}
	if d.UserAge > 0 {
		dontAsk = append(dontAsk, "age")
	}
	if d.UserPlace != "" {
		parts = append(parts, "from "+d.UserPlace)
		dontAsk = append(dontAsk, "where they are from")
	}
	if d.UserName != "" {
		parts = append(parts, "name "+d.UserName)
		dontAsk = append(dontAsk, "their name")
	}
	if d.UserDoing != "" {
		parts = append(parts, "doing: "+d.UserDoing)
		dontAsk = append(dontAsk, "what they are doing (wyd)")
	}
	return strings.Join(parts, ", "), dontAsk
}

func nz(n int) string {
	if n == 0 {
		return ""
	}
	return strconv.Itoa(n)
}

func (d *Director) aslAnswer(withPlace bool) string {
	if withPlace {
		return "'f <your age> <your country>'"
	}
	return "'f <your age>'"
}

// ShareTurn is the bot reply number at which the snap is shared.
func (d *Director) ShareTurn(hot, warm, cold int) int {
	switch d.Tier {
	case tierHot:
		return hot
	case tierWarm:
		return warm
	}
	return cold
}

func (d *Director) mood(turn int) string {
	var m string
	switch {
	case d.userHorny:
		if d.LastMood == "teasing" {
			m = "flirty"
		} else {
			m = "teasing"
		}
	case d.userFlirty || d.userPicAsk:
		m = "flirty"
	case d.userLaugh:
		m = "fun/joke"
	case d.userLonely:
		m = "lonely"
	case d.ColdStreak >= 2:
		if d.LastMood == "lonely" {
			m = "teasing"
		} else {
			m = "lonely"
		}
	case d.userShared:
		m = "curious"
	case d.Tier == tierHot:
		m = hotMoods[(turn/2)%len(hotMoods)]
	default:
		moods := d.Moods
		if len(moods) == 0 {
			moods = allMoods
		}
		m = moods[(turn/2)%len(moods)]
		if m == d.LastMood && turn%2 == 1 && turn > 2 {
			m = moods[(turn/2+1)%len(moods)]
		}
	}
	d.LastMood = m
	return m
}

// Plan decides what bot reply number `turn` must do.
func (d *Director) Plan(turn int, userText string, hot, warm, cold int) Plan {
	p := Plan{Mood: d.mood(turn)}
	base := "reply to exactly what they just said"
	switch {
	case d.userHardQ:
		base = "they ask something nerdy / complicated: do NOT answer it, dodge in 2-5 words like a bored college girl ('idk lol', 'google it lmao', 'bro its 1am')"
	case d.userAslAsk:
		base = "they ask your asl / age / gender: answer in the short app style " + d.aslAnswer(strings.Contains(userText, "from") || strings.Contains(userText, "where")) + ", nothing like 'female' or 'i am a girl'"
	case d.userGaveAsl:
		extra := ""
		if d.UserPlace != "" {
			extra = " and where they are from"
		}
		base = "they just told you their age/gender" + extra + ": reply with yours the same short way " + d.aslAnswer(d.UserPlace != "") + " — never ask theirs back, you already know it"
	case d.userQuestion:
		if d.userGreeting {
			base = "they greeted you and asked something: greet them back casually in your persona voice first, then answer their question directly"
		} else {
			base = "they asked you something: answer it directly first, in your own words"
		}
		if len(d.Profile.Parts) > 1 {
			base += "; they also said something else (" + strings.Join(d.Profile.Parts, " | ") + ") so react to that part briefly too"
		}
	case d.userGreeting:
		base = "they greeted you: greet them back with ONLY 1-2 words ('heyy' / 'hii' / 'hey lol')"
	case d.userShared:
		if len(d.Profile.Parts) > 0 {
			base = "they just shared: " + strings.Join(d.Profile.Parts, " | ") + " — react to that exact thing, never change the subject"
		} else {
			base = "pick up the specific detail they just shared and react to it"
		}
	}

	// hard gates
	if d.Underage {
		d.Ended, d.EndReason = true, "underage"
		return Plan{Fixed: "oh ur too young for me lol. bye", EndAfter: true, Why: "underage"}
	}
	if d.userBye && !d.Shared {
		d.Ended, d.EndReason = true, "user left"
		return Plan{Mood: p.Mood, Instruction: "they are leaving: say bye casually, one short line, no questions", EndAfter: true, Why: "bye"}
	}
	if d.userInsult {
		d.Ended, d.EndReason = true, "insult"
		return Plan{Mood: "teasing", Instruction: "they insulted you: one dry unbothered line like 'ok bye weirdo' or 'lol k', 2-4 words, no question.", EndAfter: true, Why: "insult -> end"}
	}
	if d.userNonEng {
		if d.NonEngHits >= 2 {
			d.Ended, d.EndReason = true, "not english"
			return Plan{Fixed: "english only lol, bye", EndAfter: true, Why: "non-english x2"}
		}
		return Plan{Fixed: "english only lol", Why: "non-english"}
	}
	if d.userMoney {
		if d.MoneyHits >= 2 {
			d.Ended, d.EndReason = true, "money talk"
			return Plan{Mood: "teasing", Instruction: "they keep talking about money / paying / selling: shut it down and leave, 'lol no im good, bye', no question.", EndAfter: true, Why: "money x2 -> end"}
		}
		return Plan{Mood: "playful", Instruction: "they mention money / paying / onlyfans / selling: shut it down casually in one line ('lol no im not selling anything' / 'lol im not that kind of girl'), no question, do NOT mention snap.", Why: "money -> shut down"}
	}
	if d.userNoSnap && (d.AskedSnap || d.Shared) {
		d.Ended, d.EndReason = true, "no snap"
		return Plan{Mood: "friendly", Instruction: "they don't have snapchat: say 'aw ok' casually, no pressure, no other app, and close it nicely in one short line. no question.", EndAfter: true, Why: "no snap -> skip"}
	}
	if d.userRepeat && !d.userAskedSnap {
		return Plan{Mood: "teasing", Instruction: "they sent the exact same message again: react like 'u ok lol' / 'u said that already', 3-6 words, no question.", Why: "repeat"}
	}

	// after the share
	if d.Shared {
		switch {
		case d.userNoSnap:
			d.Ended, d.EndReason = true, "no snap"
			return Plan{Mood: "friendly", Instruction: "they say they don't have snapchat after all: 'aw ok' casually, no pressure, no other app, close it nicely in one short line. no question.", EndAfter: true, Why: "no snap -> skip"}
		case d.userAskedSnap:
			d.SharedTurn = turn
			return Plan{Mood: p.Mood, Instruction: "they ask for the snap again: say 'its' or 'here' style, 2-3 words, NO username.", ShareNow: true, Why: "resend snap"}
		case d.userAdded:
			d.Ended, d.EndReason = true, "added on snap"
			return Plan{Mood: "flirty", Instruction: "they added you on snap: say u will accept it and talk there. one short line, no question.", EndAfter: true, Why: "added"}
		case turn-d.SharedTurn >= 2 || d.userBye:
			d.Ended, d.EndReason = true, "done after share"
			return Plan{Mood: p.Mood, Instruction: base + ", then say ttyl and that you'll be on snap. one short line, no question.", EndAfter: true, Why: "close"}
		}
		return Plan{Mood: p.Mood, Instruction: base + ". remind them casually to add you on snap (do NOT write the username, it was already sent). one short line.", Why: "remind"}
	}

	// They ignored the snap question twice: stop pushing. Real people do not
	// nag, they just close the chat politely.
	if d.AskedSnap && d.SnapDodges >= 2 && !d.userAskedSnap {
		d.Ended, d.EndReason = true, "snap dodged"
		return Plan{Mood: p.Mood, Instruction: base + ". they ignored your snap question twice: drop it completely and close casually ('no worries lol, ttyl'). one short line, no question, no snap.", EndAfter: true, Why: "snap dodged -> close"}
	}

	// user asks for the snap / says yes after the ask -> share now
	if d.userAskedSnap && turn == 1 {
		d.WantsSnapEarly = true
		return Plan{Mood: "teasing", Instruction: "their very first message asks for your snap: tease them for being fast ('lol slow down, who even r u') and ask their asl. one line, NO username.", Why: "snap asked on msg 1 -> slow down"}
	}
	if d.userAskedSnap || d.WantsSnapEarly || (d.AskedSnap && (yesRe.MatchString(strings.ToLower(userText)) || d.userPicAsk)) {
		return d.sharePlan(turn, p.Mood, "they want your snap: react playfully in 2-4 words like 'ok fine lol' or 'sure lol'. one short line, do NOT mention snap or write any username, no question.", "user wants it")
	}
	if d.Hesitated && !d.userRefused {
		return d.sharePlan(turn, p.Mood, base+". react briefly in 2-4 words. one short line, do NOT mention snap or write any username, no question.", "share after reassure")
	}
	if d.userHesitant && !d.Hesitated {
		d.Hesitated = true
		return Plan{Mood: "playful", Instruction: base + ". reassure them casually in 2-5 words. one short line, NO username, no question.", Why: "hesitant -> reassure"}
	}
	if d.userRefused {
		if d.Nudges == 0 {
			d.Nudges++
			return Plan{Mood: "teasing", Instruction: "they said no: tease them lightly in 2-5 words. one short line, NO username.", Why: "refused -> nudge"}
		}
		d.Ended, d.EndReason = true, "refused twice"
		return Plan{Mood: "friendly", Instruction: "they really don't want to add: no worries, say bye nicely. one short line.", EndAfter: true, Why: "refused twice"}
	}

	target := d.ShareTurn(hot, warm, cold)
	if turn >= target && d.SnapDodges < 2 {
		return d.sharePlan(turn, p.Mood, base+". reply naturally in 2-4 words to what they said. do NOT mention snap, snapchat, or any username — the app sends the snap share line automatically. no question.", "share turn")
	}

	// middle chat, step by step: Greeting -> ASL -> Country -> Mood/Interest -> Direct Snap Share
	switch {
	case d.userNameAsk && !d.ToldName:
		d.ToldName = true
		p.Why = "name asked"
		p.Instruction = base + ". they ask your name: tell your first name casually ('im <your name>' / 'its <your name> lol') and ask theirs only if u don't know it. no other question."
	case d.userNameAsk:
		p.Why = "name asked again"
		p.Instruction = base + ". they ask your name again: u already said it, tease them for forgetting ('u forgot already? lol') and say it once more. no other question."
	case d.userHardQ:
		p.Why = "hard question -> dodge"
		p.Instruction = base + ". they ask something nerdy / complicated: don't actually answer it, dodge like a bored college girl ('idk lol im not that deep' / 'google it lmao' / 'bro its 1am'), then flip it to something about them."
	case d.userBotQ:
		p.Why = "bot question"
		p.Instruction = base + ". they ask if u are real / a bot: laugh it off in 3-6 words (like 'lol im real') and flip it back on them with one short question. never over-explain."
	case d.userPicAsk:
		p.Why = "pic request"
		p.Instruction = base + ". they want a pic: tease them lightly in 3-5 words ('slow down lol' / 'not that fast lol'), no question. do NOT mention snap."
	case d.userHorny:
		p.Why = "horny"
		p.Instruction = base + ". they are being horny: tease back playfully in 2-5 words (like 'maybe lol wbu' / 'behaving... mostly' / 'ur bold lol'), confident and flirty, never explicit. do NOT mention snap."
	case d.userFlirty:
		p.Why = "flirty"
		p.Instruction = base + ". they are flirting: flirt back in 2-6 words, give a small compliment or tease, keep them chasing. do NOT mention snap."
	case d.userGreeting && !d.userQuestion && !d.userAslAsk && !d.userGaveAsl:
		p.Why = "greeting"
		p.Instruction = "they sent a short greeting: reply with ONLY a 1-2 word casual greeting ('heyy' / 'hii' / 'hey lol'). do NOT add 'how's ur night going', 'what's up', 'just chilling', or 'f <age>'."
	case d.userGreeting && d.userQuestion && !d.userAslAsk && !d.userGaveAsl:
		p.Why = "greeting"
		p.Instruction = base + " in 2-6 words. do NOT dump 'f <age>' unsolicited unless they asked."
	case turn == 1:
		p.Why = "opener"
		p.Instruction = base + ". greet back casually in 2-4 words."
	case d.userAslAsk && d.UserAge > 0 && d.UserGender != "":
		p.Why = "asl answer"
		d.ToldAsl = true
		p.Instruction = base + ". u already know theirs so NO 'u?'. just the answer ('f <age>'). no other question."
	case d.userAslAsk:
		p.Why = "asl answer + ask theirs"
		d.ToldAsl = true
		d.Ledger.MarkAsked(FactAge, turn)
		d.Ledger.MarkAsked(FactGender, turn)
		p.Instruction = base + ", then ask theirs the same short way, like 'f <age>, u?'."
	case d.userGaveAsl && d.ToldAsl:
		p.Why = "asl completed"
		if d.UserPlace == "" && d.Ledger.ShouldAsk(FactPlace, turn, 20) {
			d.Ledger.MarkAsked(FactPlace, turn)
			p.Instruction = "they just gave their gender/age: react in 1-2 words ('nice' / 'cool') and ask where they're from ('where u from?'). never repeat your own asl."
		} else {
			p.Instruction = "they just added their gender/age: react in 1-3 words ('ok cool' / 'nice') and ask one easy thing (wyd / hows ur night). never repeat your own asl, and never ask theirs back."
		}
	case d.userGaveAsl && d.UserAge > 0 && d.UserGender != "":
		p.Why = "asl back (no u?)"
		d.ToldAsl = true
		p.Instruction = base + ". one short line like 'f <age>' — nothing more. never ask theirs back."
	case d.userGaveAsl && d.UserGender == "":
		p.Why = "asl back, gender missing"
		d.ToldAsl = true
		d.Ledger.MarkAsked(FactGender, turn)
		p.Instruction = base + " and only ask 'm or f?' since they didn't say it. one line like 'f <age>, m or f?'."
	case d.userGaveAsl && d.UserAge == 0:
		p.Why = "asl back, age missing"
		d.ToldAsl = true
		d.Ledger.MarkAsked(FactAge, turn)
		p.Instruction = base + " and only ask their age ('how old r u' style). one line."
	case turn == 2 && !d.userQuestion && !d.userGreeting && (d.UserAge == 0 || d.UserGender == ""):
		p.Why = "asl"
		d.ToldAsl = true
		p.Instruction = base + ". they didn't say their full age/gender yet: give yours the short way ('f <age>') and ask only the missing part (their age or 'm or f?'), never both again."
		if d.UserAge == 0 {
			d.Ledger.MarkAsked(FactAge, turn)
		}
		if d.UserGender == "" {
			d.Ledger.MarkAsked(FactGender, turn)
		}
	case (d.ToldAsl || d.UserAge > 0 || d.UserGender != "") && d.UserPlace == "" && d.Ledger.ShouldAsk(FactPlace, turn, 20) && !d.userQuestion:
		p.Why = "country"
		d.Ledger.MarkAsked(FactPlace, turn)
		p.Instruction = base + ". ask where they are from in 2-5 words (like 'where u from' or 'im from <your country>, u?'). do NOT mention snap."
	case d.ColdStreak >= 2:
		p.Why = "cold streak -> hook"
		hooks := []string{
			"they are giving one-word answers: tease them in 3-5 words ('ur a man of many words huh').",
			"they are giving one-word answers: ask a quick either/or question ('netflix or music rn').",
			"they are giving one-word answers: ask what they're up to tonight ('so wyd tonight').",
		}
		p.Instruction = base + ". " + hooks[(d.ColdStreak-2)%len(hooks)] + " one short line, do NOT mention snap."
	case d.userLonely:
		p.Why = "lonely match"
		p.Instruction = base + ". relate briefly (u are bored at home too) and ask what they usually do at night in 3-6 words. do NOT mention snap."
	case d.userShared:
		p.Why = "they opened up"
		p.Instruction = base + ". react in 2-6 words and ask ONE short follow-up about that exact thing. mood: " + moodHints[p.Mood] + ". do NOT mention snap."
	default:
		p.Why = "keep going"
		if topic := d.pickTopic(); topic != "" {
			d.LastTopic = topic
			p.Instruction = base + ". then ask ONE short question about " + topicHint(topic) + " in 3-6 words. mood: " + moodHints[p.Mood] + ". do NOT mention snap."
			if topic == "work" || topic == "school" {
				d.Ledger.MarkAsked(FactDoing, turn)
			}
		} else {
			p.Instruction = base + " in 2-6 words. mood: " + moodHints[p.Mood] + ". do NOT mention snap."
		}
	}

	// statement-only turns: real people do not end every text with a question.
	if (p.Why == "keep going" || p.Why == "they opened up" || p.Why == "lonely match") && !d.userQuestion && d.ColdStreak == 0 && turn > 2 && d.NoQuestionAt != turn-1 && turn != target-2 && d.roll(3) == 0 {
		d.NoQuestionAt = turn
		p.Why += " (no question)"
		p.Instruction = base + ". this time NO question at all: just react in 2-5 words like a real text. mood: " + moodHints[p.Mood] + ". do NOT mention snap."
		return p
	}

	return p
}

func (d *Director) sharePlan(turn int, mood, instruction, why string) Plan {
	d.Shared, d.SharedTurn = true, turn
	d.AskedSnap = true
	d.Ledger.MarkAnswered(FactSnap, turn, "shared")
	d.LastDouble = turn
	return Plan{Mood: mood, Instruction: instruction, ShareNow: true, Why: why}
}

// Status is a short human-readable funnel state for logs and the UI.
func (d *Director) Status(hot, warm, cold int) string {
	switch {
	case d.Ended:
		return fmt.Sprintf("ENDED (%s)", d.EndReason)
	case d.Shared:
		return fmt.Sprintf("tier=%s score=%d | snap SHARED on turn %d", d.Tier, d.Score, d.SharedTurn)
	}
	extra := ""
	if d.ColdStreak >= 2 {
		extra = fmt.Sprintf(" cold=%d", d.ColdStreak)
	}
	stage := stageFor(d, d.Turn, d.ShareTurn(hot, warm, cold))
	return fmt.Sprintf("tier=%s score=%d | snap at reply %d (asked=%v)%s | stage=%s style=%s", d.Tier, d.Score, d.ShareTurn(hot, warm, cold), d.AskedSnap, extra, stage, d.styleFor())
}
