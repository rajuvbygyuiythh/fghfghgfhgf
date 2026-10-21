package apichat

// Signal detection for the API chat director. This is a production port of the
// regex layer proven in apitest/ (standalone tester). Everything here is pure
// text matching: no network, no model, no side effects.

import (
	"regexp"
	"strconv"
	"strings"
)

const (
	tierCold = "COLD"
	tierWarm = "WARM"
	tierHot  = "HOT"
)

var (
	thinkRe     = regexp.MustCompile(`(?s)<think>.*?</think>`)
	forbiddenRe = regexp.MustCompile(`(?i)(@|https?://|www\.|as an ai|language model|i am a bot|i'm a bot)`)
	hornyRe     = regexp.MustCompile(`(?i)\b(h[o0]rny|hrny|s[e3]x|sexy|nudes?|n[o0]des|dick|cock|pussy|boobs?|tits|wet|dtf|f[u*]ck\w*|naked|nsfw|cum|turned on|in the mood|hard rn|send pics?|ur body|strip|hookup|hook up|make ?out|kinky|18\+)\b`)
	flirtyRe    = regexp.MustCompile(`(?i)\b(cute|pretty|beautiful|gorgeous|hot|babe|baby|bae|love|luv|kiss|date|single|gf|bf|boyfriend|girlfriend|miss u|miss you|wanna meet|meet up|ur eyes|ur smile|sweet|crush|flirt\w*|marry|wifey|selfie)\b`)
	snapAskRe   = regexp.MustCompile(`(?i)(\bsnap\b|\bsc\b|\bsnapchat\b|\bsnp\b|\binsta\b|\big\b|\binstagram\b|\btelegram\b|\btg\b|\bdiscord\b|\bnumber\b|\bwhatsapp\b)`)
	refuseRe    = regexp.MustCompile(`(?i)\b(no|nope|nah|naw|dont have|don't have|do not have|not on|deleted|never|not really)\b`)
	yesRe       = regexp.MustCompile(`(?i)\b(yes+|yea+h*|yep+|yeop|ye|ya|sure|ofc|ok+|okay|k|yup|i do|got it|send|drop|gimme|add me|bet|fr|why not|lets go|let's go)\b`)

	agePatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\b([mf])[\s\-_./ ]{0,2}(\d{1,2})\b`),
		regexp.MustCompile(`(?i)\b(\d{1,2})[\s\-_./ ]{0,2}([mf])\b`),
		regexp.MustCompile(`(?i)\b(male|female|guy|girl|boy|man|woman|dude)[\s,]{0,3}(\d{1,2})\b`),
		regexp.MustCompile(`(?i)\b(\d{1,2})[\s,]{0,3}(male|female|guy|girl|boy|man|woman|dude)\b`),
		regexp.MustCompile(`(?i)\bi'?m (\d{2})\b`),
		regexp.MustCompile(`(?i)\b(\d{2}) ?(yo|years? old|yrs)\b`),
	}

	byeRe       = regexp.MustCompile(`(?i)(\b(bye+|goodbye|gtg|g2g|ttyl|cya|gotta go|got to go|have to go|im out|i'm out|peace out|night night|gn\b|good ?night)\b|^(later|laters|nite|gn)\W*$)`)
	questionRe  = regexp.MustCompile(`(?i)\b(wyd|hbu|wbu|how|what|where|why|when|who|r u|are u|do u|u got|got any|asl|age)\b`)
	botQRe      = regexp.MustCompile(`(?i)(r u (a )?(bot|real|fake|ai|human)|are (u|you) (a )?(bot|real|fake|ai|human)|\bbot\b|\bfake\b|\bscam|\bai\b|\breal\?)`)
	picAskRe    = regexp.MustCompile(`(?i)(\b(send|show|got|have|any|ur|your)\b.*\b(pics?|photo|selfie|face)\b|\b(pics?|photo|selfie)\?)`)
	laughRe     = regexp.MustCompile(`(?i)\b(lol|lmao|haha+|hehe|rofl)`)
	lonelyRe    = regexp.MustCompile(`(?i)\b(bored|boring|alone|lonely|cant sleep|can't sleep|nothing much|nm|nothin|chilling|chillin)\b`)
	lowEffortRe = regexp.MustCompile(`(?i)^(k|kk|ok|okay|oh|ohh|lol|lmao|haha+|hmm+|cool|nice|yea+h*|ya|yep|nah|idk|same|true|fr|wow|good|fine|nothing|nm|u\??|wbu\??|hbu\??|and\??|so\??|\W*)$`)
	hesitantRe  = regexp.MustCompile(`(?i)\b(idk|maybe|why|what for|hmm+|depends|not sure|later|dunno|we'?ll see|for what)\b`)
	aslAskRe    = regexp.MustCompile(`(?i)(\basl\b|\bage\b|how old|\bold r u|\bold are (u|you)|\bm or f\b|\bf or m\b|\bgender\b|r u a (girl|guy|boy|female|male)|are (u|you) a (girl|guy|boy|female|male)|\bgirl or\b|^(u|you|wbu|hbu|and u|n u)\s*\??$)`)

	placeRe       = regexp.MustCompile(`(?i)\b(?:from|live in|living in|based in|i'm in|im in)\s+(?:the\s+)?([a-z]{3,}(?:\s[a-z]{3,})?)\b`)
	placeStop     = regexp.MustCompile(`(?i)^(the|here|there|bed|home|class|school|work|college|uni|my|a|an|this|that|love|mood|fact|case|general|front|time|real|life|town|it|and|but|lol|rn|now|bored|chat)$`)
	knownPlaces   = regexp.MustCompile(`(?i)\b(usa|us|america|uk|england|london|canada|australia|germany|france|italy|spain|netherlands|sweden|norway|poland|ireland|scotland|mexico|brazil|india|texas|california|cali|florida|new york|nyc|ny|la|chicago|ohio|georgia|arizona|nevada|vegas|seattle|boston|michigan|colorado|utah|oregon|jersey|virginia|carolina|tennessee|kentucky|alabama|louisiana|miami|dallas|houston|austin|atlanta|philly|detroit|toronto|vancouver|sydney|melbourne|berlin|paris|amsterdam|dubai|pakistan|bangladesh|nigeria|philippines|indonesia|turkey|egypt|kenya|south africa|russia|ukraine|romania|greece|portugal|belgium|denmark|finland|austria|switzerland|czech|hungary|serbia|croatia)\b`)
	nameRe        = regexp.MustCompile(`(?i)\b(?:i'm|im|my name is|my name's|name is|names|call me)\s+([a-z]{3,12})\b`)
	nameStop      = regexp.MustCompile(`(?i)^(bored|horny|good|fine|home|here|alone|tired|from|just|not|also|single|new|down|kinda|lowkey|sorry|okay|chilling|chillin|lol|male|female|guy|girl|real|sure|done|back|still|only|like|about|gonna|going|trying|looking|cool|busy|lost|sad|happy|high|drunk|sleepy|hungry|the|that|this|what|wondering|curious|asking|serious|joking|kidding|saying|talking|thinking|waiting|lying|laying|sitting|doing|dying|dead|cold|hot|late|nice|weird|funny|crazy|wild|dark|quiet|boring|lit|whatever|nothing|true|fake|over|early|ready|free|stuck|outside|inside|awake|asleep|sick|bad|mad|straight|bi|gay|taken|married|old|young|tall|short|big|small|hard|soft|wet|naked|close|near|far|online|offline|texting|watching|working|studying|driving|walking|eating|drinking|smoking|gaming|listening|scrolling|interested|intrested|lonely|shy|honest|legit|actually|literally|really|pretty|very|super|so|too|already|always|never|kind|sort|type|way|more|less|better|worse|def|probably|maybe|guess|think|feel|mean|means|meant|supposed|about|around|almost|half|full|all|gone|off|out|in|on|up|at|by|for|to|with|and|but|or|if|as|an|a)$`)
	doingRe       = regexp.MustCompile(`(?i)\b(in bed|laying|lying|chilling|chillin|watching|gaming|playing|working|studying|eating|cooking|driving|at work|at home|at school|at the gym|gym|smoking|drinking|netflix|youtube|tiktok|cant sleep|can't sleep|just woke|about to sleep|nothing much|nothin|nm\b)`)
	bareMaleRe    = regexp.MustCompile(`(?i)\b(i'?m|im) (a )?(guy|male|boy|man|dude)\b|^(m|male|guy|boy|dude)\W*$`)
	bareFemaleRe  = regexp.MustCompile(`(?i)\b(i'?m|im) (a )?(girl|female|woman)\b|^(f|female|girl)\W*$`)
	bareAgeRe     = regexp.MustCompile(`(?i)^(?:im |i'm |i am |age )?(\d{2})\W*$`)
	nameAskRe     = regexp.MustCompile(`(?i)(ur name|your name|whats ur name|what's your name|name\?|who r u|who are u|who are you|what do i call u|call u\?|ur name\?|name tho)`)
	insultRe      = regexp.MustCompile(`(?i)(\b(bitch|slut|whore|hoe|stfu|kys|retard\w*|cunt|fuck (you|u|off))\b|\b(u|you|ur|you'?re|youre|your|she'?s)\b[^.?!]{0,20}\b(ugly|stupid|dumb|fat|fake|pathetic|loser|idiot|boring af|trash|disgusting|catfish|a bot bitch)\b|\b(ugly|stupid|dumb|fat|fake|pathetic|loser|idiot|trash|catfish)\b[^.?!]{0,12}\b(bitch|girl|hoe|ass|whore)\b)`)
	moneyRe       = regexp.MustCompile(`(?i)(\b(onlyfans|only fans|of link|of page|paypal|cashapp|cash app|venmo|zelle|sugar (daddy|baby|mommy)|gift ?cards?|crypto|bitcoin|btc|usdt|premium (snap|content)|my (rates|prices|menu))\b|\b(pay|paying|paid|send|give|got|need|want|for|make|earn)\b[^.?!]{0,12}\b(money|cash|dollars?|bucks)\b|\$ ?\d|\b(i'?ll|ill|i will|i can|can|will|wanna|would|gonna) pay (u|you|for)\b|\bpay (u|you) (for|to)\b|\b(how much|what'?s? (ur|your) price|do (u|you) charge|u charge|sell(ing)? (pics|nudes|content|vids|videos)|buy (ur|your) (pics|nudes|content))\b)`)
	hardQRe       = regexp.MustCompile(`(?i)\b(calculate|solve|math problem|capital of|president of|prime minister|politic\w*|election|write (me )?(some |a )?(code|script|poem|story|essay|program)|in python|in javascript|programming|explain (how|why|what|the|quantum|relativity|crypto)|meaning of life|define [a-z]+|definition of|history of|religion|philosophy|translate (this|to|into)|what is \d|\d+ ?(times|plus|minus|divided by|x|\+|-|\*|/) ?\d+|who won the|square root|quantum|stock market|gdp|theorem|equation|chemistry|physics)\b`)
	noSnapRe      = regexp.MustCompile(`(?i)(don'?t have (a )?snap|dont have (a )?sc|no snap|not on snap|deleted (my )?snap|snap got banned|don'?t use snap|i don'?t have (it|one|that)|dont have (it|one|that)|no sc\b|got no snap)`)
	nonLatinRe    = regexp.MustCompile(`[\p{Cyrillic}\p{Arabic}\p{Bengali}\p{Devanagari}\p{Han}\p{Hiragana}\p{Katakana}\p{Hangul}\p{Thai}\p{Hebrew}\p{Greek}]`)
	foreignRe     = regexp.MustCompile(`(?i)\b(hola|que|qué|como|cómo|estas|estás|guapa|bonita|bonjour|salut|ciao|bella|hallo|wie geht|kya|kaise|kaisi|haan|nahi|tum|aap|kemon|acho|tumi|apni|bhai|kaha|kahan|koi|merhaba|nasılsın|privet|kak dela|oi|tudo bem|voce|você|olá|habibi|kaif|halik|mabuhay|kumusta|apa kabar|selamat|xin chào|annyeong|konnichiwa|ni hao)\b`)
	aiPhraseRe    = regexp.MustCompile(`(?i)\b(absolutely|sounds like (you|u)|feel free|that'?s (great|awesome|wonderful|fantastic|amazing)|i'?d love to|tell me more|i'?m here to|how can i help|great question|i appreciate|certainly|definitely|wonderful|fantastic|delightful|i understand|let'?s chat|chat with you|get to know (you|u)|what about yourself|interesting!|as an ai|language model|i'?m (just )?an ai|virtual|assistant|i cannot|i can't help with|inappropriate|respectful|boundaries|comfortable with|let'?s keep (it|this)|i'?m not able to)\b`)
	addedRe       = regexp.MustCompile(`(?i)\b(added|sent (u|you)? ?(a )?(req|request)|just added|adding|requested|follow(ed)?)\b`)
	trailingDotRe = regexp.MustCompile(`[.,]+\s*([^\w\s]*)\s*$`)
)

// englishWords is the small vocabulary used to tell "my name is X" from
// "im bored". It is intentionally conservative.
var englishWords = map[string]bool{}

func init() {
	const words = "the a an i im i'm u you ur your we they he she it is are am was be been do did does have has had not no yes yea yeah ok okay k lol lmao haha hey hi hello sup yo wyd hbu wbu what where when why who how which and or but so if then than too also just only like wanna gonna want need can could would should will may might from in on at to for of with about by up down out off over here there this that these those my me mine our us them him her his its one two 18 19 20 21 22 23 24 25 m f male female guy girl boy man woman bored horny cute hot sexy nice cool fine good bad great love hate miss know think feel see look talk chat text snap sc insta ig add send pic pics photo show tell ask say said says go going come came back home bed sleep tired night tonight today now rn later soon old age asl live living school college uni work working job study studying class single taken bf gf friend friends real fake bot ai nah nope sure maybe idk idc tbh ngl fr lowkey kinda sorta btw omg bruh bro dude babe baby bae damn shit fuck wtf lmfao pls please thanks thx ty np wya hmu dm dms call vc number phone whats thats that's it's dont don't cant can't wont won't isnt isn't aint doesnt didnt wasnt werent"
	for _, w := range strings.Fields(words) {
		englishWords[w] = true
	}
}

// looksNonEnglish reports a non-latin script or 2+ words with no common
// English word at all.
func looksNonEnglish(low string) bool {
	if nonLatinRe.MatchString(low) || foreignRe.MatchString(low) {
		return true
	}
	cleaned := regexp.MustCompile(`[^a-z0-9' ]+`).ReplaceAllString(low, " ")
	words := strings.Fields(cleaned)
	if len(words) < 2 {
		return false
	}
	for _, w := range words {
		if englishWords[w] {
			return false
		}
	}
	return true
}

// genderAge extracts a short-form gender + age ("m 21", "f21", "i'm 23").
func genderAge(low string) (string, int, bool) {
	for i, re := range agePatterns {
		m := re.FindStringSubmatch(low)
		if len(m) < 2 {
			continue
		}
		var g, a string
		switch i {
		case 0, 2:
			g, a = m[1], m[2]
		case 1, 3:
			a, g = m[1], m[2]
		default:
			a = m[1]
		}
		switch g {
		case "male", "guy", "boy", "man", "dude":
			g = "m"
		case "female", "girl", "woman":
			g = "f"
		}
		if g != "" && g != "m" && g != "f" {
			g = ""
		}
		n, err := strconv.Atoi(a)
		if err == nil && n >= 13 && n <= 99 {
			return g, n, true
		}
	}
	return "", 0, false
}
