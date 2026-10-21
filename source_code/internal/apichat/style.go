package apichat

// Style and interest: every conversation gets its own personality archetype,
// but the tone of each message mirrors the stranger's energy.

import "sort"

// Style is the base personality of one conversation.
type Style string

const (
	StylePlayful   Style = "playful"
	StyleSoft      Style = "soft"
	StyleConfident Style = "confident"
	StyleChill     Style = "chill"
	StyleFlirty    Style = "flirty"
)

var allStyles = []Style{StylePlayful, StyleSoft, StyleConfident, StyleChill, StyleFlirty}

// pickStyle chooses the conversation personality once.
func pickStyle(d *Director) Style {
	return allStyles[d.roll(len(allStyles))]
}

// mirrorStyle adapts the base style to the stranger's last message.
func mirrorStyle(base Style, p MessageProfile) Style {
	switch {
	case p.Kind == KindHorny || p.Kind == KindFlirt:
		return StyleFlirty
	case p.Kind == KindJoke:
		return StylePlayful
	case p.Kind == KindLowEffort || p.Words <= 2:
		return StyleChill
	case p.Sentiment < 0 || containsTopic(p.Topics, "lonely"):
		return StyleSoft
	}
	return base
}

// styleFor resolves the style used for one reply.
func (d *Director) styleFor() Style {
	base := d.Style
	if base == "" {
		base = StylePlayful
	}
	return mirrorStyle(base, d.Profile)
}

func styleHint(s Style) string {
	switch s {
	case StylePlayful:
		return "playful and cheeky, light teasing, quick jokes"
	case StyleSoft:
		return "soft and warm, a little shy, comforting"
	case StyleConfident:
		return "confident and direct, short and sure of yourself"
	case StyleChill:
		return "low energy and chill, very short replies, unbothered"
	case StyleFlirty:
		return "subtly flirty, compliments back, suggestive but never explicit"
	}
	return "casual and natural"
}

func styleMaxWords(s Style) int {
	switch s {
	case StyleChill:
		return 7
	case StyleConfident:
		return 9
	case StyleSoft:
		return 11
	}
	return 10
}

// Interest scores topics by how much the stranger engaged with them.
type Interest map[string]int

func (in Interest) bump(topic string, delta int) {
	if topic == "" {
		return
	}
	in[topic] += delta
	if in[topic] < -3 {
		in[topic] = -3
	}
	if in[topic] > 5 {
		in[topic] = 5
	}
}

// top returns the highest scored chat topics.
func (in Interest) top(n int) []string {
	type pair struct {
		topic string
		score int
	}
	pairs := make([]pair, 0, len(in))
	for topic, score := range in {
		if !chattableTopic(topic) || score <= 0 {
			continue
		}
		pairs = append(pairs, pair{topic, score})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].score == pairs[j].score {
			return pairs[i].topic < pairs[j].topic
		}
		return pairs[i].score > pairs[j].score
	})
	out := make([]string, 0, n)
	for _, p := range pairs {
		if len(out) == n {
			break
		}
		out = append(out, p.topic)
	}
	return out
}

// observeInterest scores how the stranger reacted to the previous bot topic.
func (d *Director) observeInterest() {
	if d.LastTopic == "" {
		return
	}
	if d.Interest == nil {
		d.Interest = Interest{}
	}
	engaged := d.Profile.Words >= 4 || d.Profile.IsQuestion || d.Profile.HasEmoji
	delta := -1
	if engaged {
		delta = 1
	}
	d.Interest.bump(d.LastTopic, delta)
	d.LastTopic = ""
}

// pickTopic chooses the next thing to talk about from interest, then the
// stranger's own message.
func (d *Director) pickTopic() string {
	if tops := d.Interest.top(2); len(tops) > 0 {
		return tops[d.roll(len(tops))]
	}
	candidates := make([]string, 0, len(d.Profile.Topics))
	for _, topic := range d.Profile.Topics {
		if chattableTopic(topic) {
			candidates = append(candidates, topic)
		}
	}
	if len(candidates) == 0 {
		return ""
	}
	return candidates[d.roll(len(candidates))]
}

func chattableTopic(topic string) bool {
	switch topic {
	case "work", "school", "sleep", "bored", "lonely", "music", "shows", "food", "gym", "gaming", "relationship":
		return true
	}
	return false
}

func containsTopic(topics []string, want string) bool {
	for _, topic := range topics {
		if topic == want {
			return true
		}
	}
	return false
}

func topicHint(topic string) string {
	switch topic {
	case "work":
		return "their job or shift"
	case "school":
		return "college/classes"
	case "sleep":
		return "being tired / sleep"
	case "bored":
		return "being bored / what they do for fun"
	case "lonely":
		return "feeling alone / late night mood"
	case "music":
		return "music / what they are listening to"
	case "shows":
		return "shows / movies / tiktok"
	case "food":
		return "food / snacks"
	case "gym":
		return "gym / working out"
	case "gaming":
		return "games"
	case "relationship":
		return "being single / dating"
	}
	return topic
}
