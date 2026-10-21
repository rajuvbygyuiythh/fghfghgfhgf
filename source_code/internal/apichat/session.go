package apichat

// Session is one API Chat conversation with a single stranger. It is JSON
// serialisable because the worker persists it in bot_state.json, so a restart
// continues the same funnel instead of starting over.

import "math/rand"

// Message is one OpenAI-compatible chat message.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Session is per-account, per-stranger conversation state.
type Session struct {
	Started        bool      `json:"started,omitempty"`
	Turn           int       `json:"turn,omitempty"`
	PersonaAge     int       `json:"persona_age,omitempty"`
	PersonaCountry string    `json:"persona_country,omitempty"`
	PersonaName    string    `json:"persona_name,omitempty"`
	PersonaMajor   string    `json:"persona_major,omitempty"`
	PersonaYear    string    `json:"persona_year,omitempty"`
	PersonaTonight string    `json:"persona_tonight,omitempty"`
	PersonaShow    string    `json:"persona_show,omitempty"`
	PersonaSnack   string    `json:"persona_snack,omitempty"`
	History        []Message `json:"history,omitempty"`
	Director       Director  `json:"director"`
	// UsedShareLines prevents the same sharesnap line from being sent twice
	// inside one conversation.
	UsedShareLines []string `json:"used_share_lines,omitempty"`
}

// NewSession builds a fresh conversation with a random persona and mood
// rotation. The caller supplies the worker's RNG so runs stay reproducible
// enough for tests.
func NewSession(rng *rand.Rand) Session {
	seed := int64(1)
	pick := func(xs []string) string { return xs[0] }
	age := 22
	if rng != nil {
		seed = rng.Int63()
		pick = func(xs []string) string { return xs[rng.Intn(len(xs))] }
		age = 20 + rng.Intn(6)
	}
	s := Session{
		PersonaAge:     age,
		PersonaName:    pick(loreNames),
		PersonaMajor:   pick(loreMajors),
		PersonaYear:    pick(loreYears),
		PersonaTonight: pick(loreTonight),
		PersonaShow:    pick(loreShows),
		PersonaSnack:   pick(loreSnacks),
		Director:       NewDirector(seed),
	}
	return s
}

// ensurePersona fills any persona field a persisted session may be missing.
func (s *Session) ensurePersona(countries []string) {
	d := &s.Director
	rc := func(xs []string) string {
		if len(xs) == 0 {
			return ""
		}
		return xs[d.roll(len(xs))]
	}
	if s.PersonaAge < 18 || s.PersonaAge > 30 {
		s.PersonaAge = 20 + d.roll(6)
	}
	if s.PersonaCountry == "" {
		if len(countries) > 0 {
			s.PersonaCountry = countries[d.roll(len(countries))]
		} else {
			s.PersonaCountry = "uk"
		}
	}
	if s.PersonaName == "" {
		s.PersonaName = rc(loreNames)
	}
	if s.PersonaMajor == "" {
		s.PersonaMajor = rc(loreMajors)
	}
	if s.PersonaYear == "" {
		s.PersonaYear = rc(loreYears)
	}
	if s.PersonaTonight == "" {
		s.PersonaTonight = rc(loreTonight)
	}
	if s.PersonaShow == "" {
		s.PersonaShow = rc(loreShows)
	}
	if s.PersonaSnack == "" {
		s.PersonaSnack = rc(loreSnacks)
	}
}

// trimHistory keeps the last n messages (system prompt is built separately).
func (s *Session) trimHistory(n int) {
	if n <= 0 || len(s.History) <= n {
		return
	}
	s.History = append([]Message(nil), s.History[len(s.History)-n:]...)
}
