package apichat

// Fact and question ledger. It remembers which personal facts the stranger
// already shared and which questions the bot already asked, so a known fact is
// never asked again and the same question never comes twice in a row.

import (
	"strconv"
	"strings"
)

// FactKey names one personal fact the funnel cares about.
type FactKey string

const (
	FactAge    FactKey = "age"
	FactGender FactKey = "gender"
	FactPlace  FactKey = "place"
	FactName   FactKey = "name"
	FactDoing  FactKey = "doing"
	FactSnap   FactKey = "snap"
)

var factOrder = []FactKey{FactAge, FactGender, FactPlace, FactName, FactDoing, FactSnap}

// Ledger is persisted with the conversation.
type Ledger struct {
	Facts    map[FactKey]string `json:"facts,omitempty"`
	Asked    map[FactKey]int    `json:"asked,omitempty"`
	Answered map[FactKey]int    `json:"answered,omitempty"`
}

func (l *Ledger) init() {
	if l.Facts == nil {
		l.Facts = make(map[FactKey]string)
	}
	if l.Asked == nil {
		l.Asked = make(map[FactKey]int)
	}
	if l.Answered == nil {
		l.Answered = make(map[FactKey]int)
	}
}

// MarkAsked records the turn a question about this fact was sent.
func (l *Ledger) MarkAsked(k FactKey, turn int) {
	l.init()
	l.Asked[k] = turn
}

// MarkAnswered records the turn the stranger answered, and the value.
func (l *Ledger) MarkAnswered(k FactKey, turn int, value string) {
	l.init()
	l.Facts[k] = value
	if _, ok := l.Answered[k]; !ok {
		l.Answered[k] = turn
	}
}

// ensureAnswered fills a fact only the first time it becomes known.
func (l *Ledger) ensureAnswered(k FactKey, turn int, value string) {
	if strings.TrimSpace(value) == "" {
		return
	}
	l.init()
	if _, ok := l.Answered[k]; !ok {
		l.Answered[k] = turn
	}
	l.Facts[k] = value
}

// Known reports an answered fact.
func (l *Ledger) Known(k FactKey) bool {
	if l == nil || l.Answered == nil {
		return false
	}
	_, ok := l.Answered[k]
	return ok
}

// ShouldAsk reports whether asking this fact is still acceptable.
func (l *Ledger) ShouldAsk(k FactKey, turn, window int) bool {
	if l.Known(k) {
		return false
	}
	if last, ok := l.Asked[k]; ok && window > 0 && turn-last < window {
		return false
	}
	return true
}

// NeverAsk lists the answered facts in a stable order for the prompt.
func (l *Ledger) NeverAsk() []FactKey {
	if l == nil {
		return nil
	}
	out := make([]FactKey, 0, len(factOrder))
	for _, k := range factOrder {
		if l.Known(k) {
			out = append(out, k)
		}
	}
	return out
}

// syncLedger copies the director's parsed facts into the ledger.
func (d *Director) syncLedger() {
	d.Ledger.init()
	if d.UserAge > 0 {
		d.Ledger.ensureAnswered(FactAge, d.Turn, strconv.Itoa(d.UserAge))
	}
	if d.UserGender != "" {
		d.Ledger.ensureAnswered(FactGender, d.Turn, d.UserGender)
	}
	if d.UserPlace != "" {
		d.Ledger.ensureAnswered(FactPlace, d.Turn, d.UserPlace)
	}
	if d.UserName != "" {
		d.Ledger.ensureAnswered(FactName, d.Turn, d.UserName)
	}
	if d.UserDoing != "" {
		d.Ledger.ensureAnswered(FactDoing, d.Turn, d.UserDoing)
	}
}

// askedFacts lists the personal questions present in a bot line.
func askedFacts(line string) []FactKey {
	low := strings.ToLower(line)
	out := make([]FactKey, 0, 3)
	if ageAskRe.MatchString(low) || strings.Contains(low, "how old") {
		out = append(out, FactAge)
	}
	if genderAskRe.MatchString(low) || strings.Contains(low, "m or f") {
		out = append(out, FactGender)
	}
	if placeAskRe.MatchString(low) {
		out = append(out, FactPlace)
	}
	if nameAskAgainRe.MatchString(low) {
		out = append(out, FactName)
	}
	if doingAskRe.MatchString(low) && !strings.Contains(low, "for fun") {
		out = append(out, FactDoing)
	}
	if snapAskRe.MatchString(low) && !strings.Contains(low, "%username%") {
		out = append(out, FactSnap)
	}
	return out
}

// factAnswered reports whether the stranger already gave this fact.
func (d *Director) factAnswered(k FactKey) bool {
	switch k {
	case FactAge:
		return d.UserAge > 0
	case FactGender:
		return d.UserGender != ""
	case FactPlace:
		return d.UserPlace != ""
	case FactName:
		return d.UserName != ""
	case FactDoing:
		return d.UserDoing != ""
	case FactSnap:
		return d.AskedSnap || d.Shared
	}
	return false
}
