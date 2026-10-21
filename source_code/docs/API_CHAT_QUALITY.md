# API Chat — Quality Guide (মানুষের মতো, কিন্তু লক্ষ্যের দিকে)

এই গাইডটা API Chat মোডের **কোয়ালিটি স্তর** বোঝায়: বট এখন কীভাবে ইউজারের SMS
বোঝে, কী মনে রাখে, কী লিখবে না, আর কীভাবে ধাপে ধাপে snap-এর দিকে নিয়ে যায়।

কোড: `source_code/internal/apichat/` (+ `internal/bot/api_chat.go`)।
এই ফোল্ডারের বাইরে EVAChat Flow-এর TXT ইঞ্জিনে কোনো পরিবর্তন নেই।

---

## ১. এক নজরে ৮টা নিয়ম

| নিয়ম | কোথায় | কীভাবে |
|---|---|---|
| একই SMS আর নয় | `repeat.go` + `guard.go` | শেষ ৮টা বট লাইনের সাথে ৬২%+ মিল, একই opener, একই প্রশ্ন — reject |
| যা জানে তা আর জিজ্ঞেস নয় | `ledger.go` | age / gender / place / name / doing / snap — উত্তর পেলে চিরতরে বন্ধ |
| প্রশ্ন ছাড়াও বোঝা | `understanding.go` | implicit ask, command, multi-part, topic, sentiment, keywords |
| এলোমেলো কথা নয় | `guard.go` | ইউজারের কথার অন্তত ১টা concrete anchor/text-topic থাকতেই হবে |
| পুরো SMS বোঝা | `prompt.go` | `[what they just sent]` + `parts` + `keywords` + message type |
| লক্ষ্যের দিকে চালানো | `goal.go` | RAPPORT → INTEREST → HOOK → ASK → SHARE → CLOSE |
| প্রতি চ্যাট আলাদা, তবু ইউজারের মতো | `style.go` | ৫টা archetype + ইউজারের mood mirror + topic interest score |
| sharesnap থেকে random, repeat নয় | `engine.go` | `UsedShareLines`, ডেক শেষ হলে reset, শেষ লাইন এড়ানো |

---

## ২. বোঝা (understanding)

প্রতিটা ইনকামিং SMS-এর জন্য `MessageProfile` তৈরি হয়:

* **Kind** — greeting, question, implicit_ask, command, sharing, flirt, horny,
  joke, insult, money, meta, leaving, low_effort, non_english, other
* **ImplicitAsk** — `"i bet you..."`, `"u probably..."`, `"wish u were..."`,
  `"i wonder..."`, `"u should..."` — প্রশ্নচিহ্ন ছাড়াই প্রশ্ন
* **Command** — `send pics`, `come over`, `add me` — কী চাইছে সেটাও ধরা হয়
* **Parts** — এক SMS-এ দুই কথা (`"im tired from work and my boss is annoying"`)
  → `im tired from work | my boss is annoying`
* **Topics** — work, school, sleep, bored, lonely, music, shows, food, gym,
  gaming, relationship, looks, place, age, snap
* **Keywords / HasEmoji / Sentiment / Words**

Prompt-এ এগুলো যায়, তাই মডেল অন্ধভাবে প্রশ্ন করে না — জানে কে কী বলেছে।

## ৩. মনে রাখা (ledger + interest)

`Ledger` persist হয় `bot_state.json`-এ:

```
age: 21 (turn 2), place: texas (turn 2), name: sam (turn 5)   ← আর কখনো জিজ্ঞেস নয়
asked: age → 2, doing → 4                                     ← একই প্রশ্ন টানা নয়
```

* উত্তরে থাকা fact **hard-lock** — guard মডেলের লাইনে সেই প্রশ্ন পেলে reject।
* জিজ্ঞেস করা হয়েছে কিন্তু উত্তর নেই → ভিন্ন ভাষায় সর্বোচ্চ একবার।
* `Interest` স্কোর −৩…+৫: বট যে topic তুলল, ইউজার ৪+ শব্দে/প্রশ্নে সাড়া দিলে
  +১, এড়িয়ে গেলে −১। পরের প্রশ্ন সবচেয়ে বেশি score করা topic থেকেই আসে।

## ৪. না লেখার নিয়ম (quality gate)

`validateReply()` এক জায়গায় ধারাবাহিক চেক করে:

1. খালি / `@` / লিংক / AI-টক (`as an ai`, `feel free`, `tell me more` …)
2. আগের লাইনের সাথে ৬২%+ মিল, একই opener, টানা একই প্রশ্ন
3. যে fact ইউজার দিয়েছে সেই প্রশ্ন (age/gender/place/name/doing)
4. ইউজার প্রশ্ন করলে উত্তরে শুধু প্রশ্ন — নিষিদ্ধ
5. এক রিপ্লাইতে ২+ প্রশ্ন, বা টানা ২ রিপ্লাই প্রশ্ন
6. ইউজারের কথার সাথে কোনো anchor নেই (শুধু sharing/flirt ইত্যাদিতে)

ব্যর্থ হলে **একবার retry**, prompt-এ reject-এর কারণ যায়
(`your previous attempt was rejected: asked for something they already told you (age)`).
দ্বিতীয়বারও ব্যর্থ হলে intent-ভিত্তিক ছোট লাইন:
প্রশ্ন → `idk tbh`, flirt → `stop ur making me blush`, dry → `lol fair`.
কখনো এলোমেলো অপ্রাসঙ্গিক লাইন নয়।

## ৫. Style ও Goal

**Archetype** (কনভার্সেশনের শুরুতে একবার, পুরো চ্যাটে fixed):
`playful`, `soft`, `confident`, `chill`, `flirty`

**Mirror** (প্রতি মেসেজে override):

| ইউজার | বট |
|---|---|
| joke/laugh | playful, ৮–১০ শব্দ |
| ১–২ শব্দ / dry | chill, ≤৭ শব্দ, প্রশ্নের চাপ নেই |
| flirty/horny | flirty, suggestive নয় explicit নয় |
| lonely/sad | soft, comforting |
| সিরিয়াস প্রশ্ন | সরাসরি উত্তর, তারপর হালকা লাইন |

**Stage ladder**: RAPPORT (১–২) → INTEREST (৩–৫) → HOOK (target−২) →
ASK (target−১) → SHARE (target) → CLOSE (added / ২ turn / no-snap / dodged)।

প্রতি রিপ্লাইয়ের prompt-এ থাকে একটাই goal — যেমন
`stage: interest — goal: show you get them, then dig into their job or shift`।

## ৬. sharesnap.txt

`data/api_chat/sharesnap.txt`, এক লাইন = এক share:

```
ok fine, add me on snap %username%
my snap is %username%, add me rn
sc: %username% .. come find me there
```

* একই চ্যাটে একই লাইন **দুবার নয়**; ডেক শেষ হলে তবেই reset, আর শেষ লাইনটা
  সাথে সাথে আবার pick হয় না।
* `%username%` সবসময় Go বসায় (`getOrAssignUsername`, per-user sticky)।
* ড্যাশবোর্ডে কোনো username না থাকলে share লাইন **বাদ** যায় + লাইভ লগে
  `share line skipped: no dashboard username configured` — কখনো খালি
  "add me on snap" যাবে না।
* ডেকটা clean/playful; চাইলে নিজে এডিট করুন — অ্যাপ আপনার ফাইল overwrite করে না।

## ৭. Snap nagging নেই

* target টার্নে একবার ask/share।
* ইউজার এড়িয়ে গেলে `SnapDodges` বাড়ে; **২ বার** এড়ালে বট জোর করা বন্ধ করে
  ভদ্রভাবে close করে (`snap dodged -> close`)।
* আবার snap চাইলে বা "yes/ok" বললে dodge counter reset হয়।
* ask করার আগে target−২ টার্নে একবার casual hook (কিন্তু প্রশ্ন নয়)।

## ৮. লাইভ লগে কী দেখবেন

```
-> [apichat] reply from api (reply 5 · keep going | tier=WARM score=3 | snap at reply 6 (asked=true) | stage=interest style=playful · retry: did not refer to anything they said)
```

* `stage` / `style` — ওই রিপ্লাইয়ের লক্ষ্য ও টোন
* `retry: ...` — প্রথম attempt কেন reject হল
* `safe reply` — দুইবার reject হলে intent-fallback
* `share line skipped...` — username কনফিগার করা নেই

## ৯. টিউনিং (কোডে)

| জিনিস | মান | ফাইল |
|---|---|---|
| repeat similarity | `0.62` | `guard.go` (`repeatThreshold`) |
| retry | ১ বার | `engine.go` |
| ইতিহাসে রাখা বট-লাইন | শেষ ৮টা চেক / ২৪টা মেসেজ রাখা | `repeat.go` / `engine.go` |
| style | ৫ archetype | `style.go` (`allStyles`) |
| interest সীমা | −৩…+৫ | `style.go` (`Interest.bump`) |
| share ডেক | `sharesnap.txt` | `data/api_chat/` |

সব threshold এক জায়গায় বলে বদলানো সহজ; বদলালে `install.bat` দিয়ে rebuild
করলেই কার্যকর হবে।

## ১০. টেস্ট

Go (আপনার PC-তে): `go test ./internal/apichat ./internal/bot`
Node (এখানে/আপনার PC-তে): `npm test`

Go টেস্টগুলো কভার করে — implicit ask, multi-part, ledger no-re-ask, near-duplicate
repeat, opener, guard reasons, style mirror, stage progression, share rotation,
JSON persistence, retry prompt। Node contract টেস্ট নিশ্চিত করে ফাইল, ডেক,
`%username%` আর ড্যাশবোর্ড hint সব ঠিক আছে।
