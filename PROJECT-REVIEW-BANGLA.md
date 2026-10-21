# ChitChat Bot — সম্পূর্ণ প্রজেক্ট রিভিউ (বাংলা)

**রিভিউ তারিখ:** ২১ অক্টোবর ২০২৬  
**রিভিউ ধরন:** সোর্স-লেভেল আর্কিটেকচার রিভিউ + বাস্তব টেস্ট রান + সিকিউরিটি/লিগ্যাল রিস্ক অ্যাসেসমেন্ট  
**রিভিউ করা উপাদান:** `ChitChat-App-clean-fixed-2026-10-20 (11).zip` (২৪.২ MB), ভেতরের ৬৭৫টি ফাইল, `source_code/`, `sites/`, `skills/`, `memory/`, `build/`  
**পদ্ধতি:** ZIP আলাদা করে স্ট্যাটিক কোড রিড, `npm ci --ignore-scripts`, `node --test`, `node scripts/verify.cjs`, `node scripts/deep-debug.cjs` চালানো; Go টুলচেইন স্যান্ডবক্সে না থাকায় Go টেস্ট/বিল্ড চালানো যায়নি — সেটি স্পষ্টভাবে **BLOCKED**, PASS নয়।

> **সতর্কতা:** নিচের লিগ্যাল/নীতি অংশটি সাধারণ প্রকৌশল পর্যবেক্ষণ, আইনি পরামর্শ নয়। বাণিজ্যিক ব্যবহারের আগে অবশ্যই সংশ্লিষ্ট দেশের আইনজীবী ও প্রতিটি প্ল্যাটফর্মের ToS যাচাই করতে হবে।

---

## ১. এক নজরে রায় (Executive Summary)

এটি একটি **Windows ডেস্কটপ অ্যাপ (Go + Wails) + CLI worker**, যা `chitchat.gg` এবং আরও ১০০+ স্ট্রেঞ্জার-চ্যাট সাইটে স্বয়ংক্রিয়ভাবে চ্যাট চালায়। ভেতরে চারটি রিপ্লাই ইঞ্জিন, ডম-ভিত্তিক ব্রাউজার অটোমেশন, REST + Socket.IO ক্লায়েন্ট, প্রক্সি রিলে, একাধিক প্রোফাইল/ওয়ার্কার, Badger ডেটাবেস, এবং একটি হাতে-লেখা (vanilla JS) ড্যাশবোর্ড আছে।

**প্রকৌশল হিসেবে সংক্ষিপ্ত রায়:**

| দিক | স্কোর (১০-এ) | মন্তব্য |
|---|---:|---|
| আর্কিটেকচারাল আইডিয়া | ৭.০ | আলাদা ইঞ্জিন, সাইট-আইসোলেশন, DOM state machine — ভালো ভাবনা |
| কোড কোয়ালিটি | ৫.৫ | কিছু জায়গায় যত্নশীল; তবে `app.go`/`logic.go` বিশাল god-object |
| টেস্টিং | ৫.০ | ইউনিট টেস্ট ভালো; রিয়েল ইন্টিগ্রেশন নেই, অ্যাকচুয়াল গেট ফেল |
| সিকিউরিটি | ৩.৫ | সিক্রেট এনক্রিপশন নেই; তৃতীয় পক্ষের কাছে চ্যাট ডেটা যায় |
| ডেলিভারি/রিপ্রোডিউসিবিলিটি | ১.৫ | Git-এ শুধু ZIP, বিল্ড করা EXE, কোনো ম্যানিফেস্ট/সাইন নেই |
| ডকুমেন্টেশন | ৭.০ | বিস্তারিত, কিন্তু কিছু জায়গায় পুরনো/পরস্পরবিরোধী |
| লিগ্যাল/কমপ্লায়েন্স | ১.০ | সর্বোচ্চ ঝুঁকি — নিচে বিস্তারিত |
| **সামগ্রিক (বাণিজ্যিক প্রোডাক্ট হিসেবে)** | **~৪.৫/১০** | পরিশ্রমী প্রোটোটাইপ; verified product নয় |

**এক লাইনে:** ভালো ফিচার-আইডিয়া ও চমৎকার কিছু লোকাল প্যাটার্ন আছে, কিন্তু **ডেলিভারি ইন্টিগ্রিটি, ভেরিফিকেশনের সততা, ভুয়া অটো-রেজিস্ট্রেশন এবং লিগ্যাল এক্সপোজার** এটিকে এখনই বাণিজ্যিকভাবে ব্যবহারযোগ্য করে না।

---

## ২. রেপো আসলে কেমন

GitHub রেপোতে একটি মাত্র কমিট আছে:

```text
4479cb1  Add files via upload
ChitChat-App-clean-fixed-2026-10-20 (11).zip   24,214,927 bytes
```

অর্থাৎ **রেপোতে সোর্স কোড নেই — শুধু একটি ZIP আছে।** এর ফলে:

- কোডের ইতিহাস, ব্লেম, PR রিভিউ, ডিফুশন — কিছুই নেই।
- কোন সোর্স থেকে কোন EXE তৈরি হয়েছে তা প্রমাণ করার উপায় নেই।
- রুটে `README.md`, `LICENSE`, `.gitignore`, CI workflow — কিছুই নেই।
- ২৪ MB বাইনারি ZIP Git-এ রাখা হয়েছে; ভেতরে আবার ৫৫ MB-র তিনটি Windows EXE।

ZIP খুললে যা পাওয়া যায়:

```text
ChitChat-App-clean-fixed-2026-10-20 (11)/
├── install.bat / apichat.bat / debug.bat      # Windows wrappers
├── source_code/                               # ১৯ MB — আসল প্রজেক্ট
│   ├── main.go, app.go                        # Wails entry + ৬০+ bound method
│   ├── internal/                              # ১৩টি Go package
│   │   ├── bot/         (২৫ ফাইল)             # worker, EVA, API chat, realtime, registry
│   │   ├── apichat/     (২২ ফাইল)             # persona + director + quality layer
│   │   ├── sites/       (৬ ফাইল)              # ১০২ সাইট ক্যাটালগ, manager, autoreg
│   │   ├── browser/     (১১ ফাইল)             # Rod, ChromeDP, DOM observer
│   │   ├── proxy/       (৭ ফাইল)              # HTTP/SOCKS5, relay, test
│   │   ├── model/, uistore/, store/            # types, Badger settings, file utils
│   │   ├── applog/, runtimeassets/, workerplan/
│   ├── cmd/                                   # chitchatui, chitchatbot, chitchattest, apichat
│   │   └── chitchatui/frontend/dist/          # index.html + app.js + styles.css
│   ├── docs/                                  # ২১টি ডকুমেন্ট, ~২,৭৪৪ লাইন
│   ├── tests/                                 # ৭টি Node test suite
│   ├── scripts/                               # verify, package, deep-debug, project-map
│   ├── capture/                               # ২৫ ফাইল — ম্যানুয়াল ট্রাফিক ক্যাপচার টুল
│   └── apitest/                               # ১২ ফাইল — আলাদা Go module (AI ল্যাব)
├── sites/                                     # ১০২টি সাইট ফোল্ডার
├── skills/                                    # ৪টি ইন-পেজ JS স্কিল
├── memory/                                    # ৫টি মেমরি রিপোর্ট
└── build/                                     # ৩টি Windows EXE (৫৫ MB)
```

**পরিমাণগত তথ্য:**

| মেট্রিক | মান |
|---|---:|
| মোট ফাইল (ZIP-এ) | ৬৭৫ |
| Go production ফাইল | ৬৬ |
| Go production LOC | ~২২,১১৬ |
| Go test ফাইল / test function | ৪৭ / ২২০ |
| Go test LOC | ~৬,৪৯৮ |
| মোট Go LOC | ~২৮,৬১৪ |
| ফ্রন্টএন্ড | ৩,৪৫৯ লাইন (app.js ১,৪২৩ + HTML ৬৬২ + CSS ১,৩৭৪) |
| সাইট ফোল্ডার | ১০২ (১০০ ক্যাটালগ + chitchat + template) |
| খালি ডিরেক্টরি | ৪০৮ (`accounts/`, `profiles/`, `logs/`, `sessions/`) |
| ডকুমেন্টেশন | ২১ ফাইল, ~২,৭৪৪ লাইন |
| বিল্ড করা EXE | ৩টি, ~৫৫ MB |

---

## ৩. টেক স্ট্যাক

- **ভাষা:** Go 1.24 (`go.mod`), Windows target (`GOOS=windows`)
- **ডেস্কটপ ফ্রেমওয়ার্ক:** Wails v2.12.0 + WebView2
- **ব্রাউজার অটোমেশন:** `go-rod/rod` (CDP), বিকল্প `chromedp`
- **HTTP/সকেট:** `net/http`, `gorilla/websocket`, `refraction-networking/utls` (TLS fingerprint)
- **স্টোরেজ:** BadgerDB v4 (settings/logs), JSON ফাইল (sessions/state)
- **ফ্রন্টএন্ড:** vanilla HTML/CSS/JS — কোনো React/Vue/TypeScript/build step নেই
- **টেস্ট/টুলিং:** Node 22 `node:test`, jsdom, fflate; Go `testing`; PowerShell bundle script
- **AI:** OpenAI-compatible API, ডিফল্ট `https://api.featherless.ai/v1`, মডেল `deepseek-ai/DeepSeek-V3-0324`

---

## ৪. হাই-লেভেল আর্কিটেকচার

```text
┌─────────────────────────────────────────────────────────────┐
│                    Windows Desktop (Wails)                  │
│  index.html + app.js  ──Wails bindings──►  app.go (App)    │
└───────────────┬─────────────────────────────┬───────────────┘
                │                             │
       Show Mode path                  Background Bot path
                │                             │
       Rod visible browser            bot.Run(ctx, Config)
       + DOM observer                 ├── internal/api (REST)
       + live_chat.go                 ├── internal/bot/realtime (Socket.IO)
       + showmode_dom.js              ├── registry / workerplan
       + reply engines                ├── proxy.Rotator
                │                     └── chat engines
                └──────────────┬──────────────┘
                               ▼
             ┌──────────────────────────────────┐
             │        Chat Engine Layer         │
             │ 1 Single · 2 Folder · 3 EVA TXT  │
             │ 4 API Chat (director + LLM)      │
             └──────────────────────────────────┘
                               │
             ┌──────────────────────────────────┐
             │  Data: Badger + JSON + logs.txt  │
             │  sites/<id>/ (configs+sessions)  │
             └──────────────────────────────────┘
```

**মূল ডিজাইন সিদ্ধান্ত:** সাইট-ভিত্তিক কনফিগ (`sites/<site_id>/site.json`) আর ইঞ্জিন-ভিত্তিক লজিক (`internal/bot`, `internal/apichat`) আলাদা রাখা হয়েছে। একটি সাইটের DOM বদলালে যেন অন্য সাইট বা রিপ্লাই ইঞ্জিন না ভাঙে — এই আইসোলেশনের ভাবনাটি ভালো।

---

## ৫. মডিউল-বাই-মডিউল দায়িত্ব

| মডিউল | কাজ | মন্তব্য |
|---|---|---|
| `main.go` | Wails অ্যাপ বুট, data dir resolve, WebView2 profile, embedded assets | পরিষ্কার, ছোট |
| `app.go` | ৬০+ Wails-bound RPC: site, browser, settings, proxy, bot start/stop, logs, test chat | **৬১ KB god-object** |
| `internal/bot/run.go` | worker lifecycle, login, cycle loop, match-and-chat | মূল অর্কেস্ট্রেশন |
| `internal/bot/logic.go` | relationship/DM fetch, send, accept, state machine, sequence, EVA dispatch | **৫৩ KB**, অনেক দায়িত্ব |
| `internal/bot/realtime.go` | `wss://api.chitchat.gg/socket.io` listener, `utls` fingerprint, release hash scrape | নেটওয়ার্ক-নির্ভর, ভঙ্গুর |
| `internal/bot/registry.go` | একাধিক worker-এর মধ্যে user claim, duplicate reply প্রতিরোধ | ভালো ধারণা |
| `internal/bot/eva_chat.go` | TXT trigger/reply engine, stage machine, no-repeat pools, age gate | প্রজেক্টের সবচেয়ে সুসংগঠিত অংশ |
| `internal/apichat/*` | persona, director, understanding, ledger, repeat, guard, style, goal, sanitize | ভালো decomposition |
| `internal/api` | chitchat REST client, headers, proxy | ছোট, focused |
| `internal/browser/rod.go` | browser launch, proxy auth, session capture, ban check, page recovery | জটিল কিন্তু ভাগ করা |
| `internal/browser/domchat.go` | DOM snapshot, click, native type + Enter, send confirmation | Show Mode-এর হৃদয় |
| `internal/browser/showmode_dom.js` | MutationObserver, row/author classification, START/SKIP detection | চমৎকার DOM heuristics |
| `internal/sites/catalog.go` | ১০০ সাইট seed + ৫টি selector family | **৪১ KB generated-ish ডেটা** |
| `internal/sites/manager.go` | per-site folder, config load/save, dual-root sync, JS driver generation | জটিল, split-brain ঝুঁকি |
| `internal/sites/autoreg.go` | "browserless auto registration" | **ভুয়া টোকেন/কুকি তৈরি করে** — নিচে P0 |
| `internal/sites/diagnostics.go` | per-site debug log, diagnostics report | উপযোগী |
| `internal/proxy/*` | HTTP CONNECT / SOCKS5 auth, loopback relay, test proxy | ভালো error diagnostics |
| `internal/uistore/*` | Badger-backed settings (`settings/browser`, `message`, `proxy`, `general`, `show_mode/v1`) | key-value, no encryption |
| `internal/applog/*` | JSONL log, secret redaction, stdout capture, crash output | দরকারি, তবে rotation নেই |
| `internal/workerplan` | login file discover, profile-worker assignment, proxy distribution | পরিষ্কার |
| `cmd/apichat` | আলাদা Persona Studio + Live Test Chat | ভালো separation |
| `capture/` | ম্যানুয়াল CDP traffic recorder, redacted by default | standalone, ভালো টুল |
| `apitest/` | আলাদা Go module, AI funnel design lab | production app import করে না |

---

## ৬. দুটি রানটাইম পাথ

### ৬.১ Show Mode (দৃশ্যমান ব্রাউজার + DOM)

1. ড্যাশবোর্ডের **Run Browser** বাটন Setting + Proxy + Message সেভ করে।
2. `app.go` → `runBrowser` → Rod ব্রাউজার **দৃশ্যমান** অবস্থায় চালু হয় (headless=false)।
3. সেভ করা সেশন থাকলে inject, নাহলে ম্যানুয়াল লগইন; লগইন শেষে সেশন সংরক্ষণ।
4. `live_chat.go` DOM observer চালু করে (`showmode_dom.js`)।
5. observer `li[data-message-id]` rows পড়ে, author/partner/own আলাদা করে, পুরনো মেসেজ seed করে (যাতে আগের মেসেজে রিপ্লাই না যায়)।
6. নতুন ইনকামিং মেসেজ এলে selected engine (Sequence / Folder / EVA / API) দিয়ে রিপ্লাই বানায়।
7. Rod দিয়ে কম্পোজারে নেটিভ টাইপ + Enter; নিজের row দেখা গেলে send নিশ্চিত; সংশয় হলে থেমে যায়, অটো-রিট্রাই করে না।
8. Auto Skip/no-reply timeout হলে Skip বাটন (Esc) ক্লিক করে পরের স্ট্রেঞ্জার।
9. Live Logs-এ IN/OUT/SKIP + rule + delay + skip reason stream হয়।

**ভালো দিক:** send confirmation, generation/dedupe, নিজের echo suppress, uncertain send-এ pause — ডুপ্লিকেট মেসেজ প্রতিরোধে পরিণত ডিজাইন।

### ৬.২ Background Bot (API worker)

1. `StartBot()` → `buildBotConfigs()`: `workerplan` দিয়ে প্রতিটি প্রোফাইল/লগইন ফাইল খুঁজে worker config বানায়, প্রক্সি ভাগ করে দেয়।
2. `bot.Run(ctx, cfg)`:
   - সেভ করা login token লোড; না থাকলে ব্রাউজার দিয়ে login capture।
   - নিজের user ID বের করে registry-তে register।
   - Socket.IO realtime listener চালু।
3. প্রতি cycle:
   - relationships + DM conversations fetch
   - pending friend request accept
   - registry-তে user claim (একই user-কে দুই worker রিপ্লাই দেবে না)
   - DM-এ নতুন মেসেজ থাকলে selected engine দিয়ে reply
   - idle হলে `POST /match` → নতুন স্ট্রেঞ্জার → chat → disconnect
4. random delay, proxy rotation, ban check, state JSON-এ persist।
5. Stop করলে context cancel; সব worker বন্ধ।

**প্রোটোকল (ডক ও কোড অনুযায়ী):**
- incoming: Socket.IO `chatMessage`
- send: `POST /users/me/conversations/:id/messages`, multipart `{content, nonce}`
- skip: `PATCH /match/disconnect` + `POST /match`
- auth: cookie/session-based; `utls` দিয়ে TLS fingerprint; release hash scrape

---

## ৭. চারটি চ্যাট ইঞ্জিন

| # | মোড | কীভাবে কাজ করে | অবস্থা |
|---|---|---|---|
| ১ | Single Sequence | একটানা লাইন-ভিত্তিক স্ক্রিপ্ট | মূল, স্থিতিশীল |
| ২ | Folder Sequences | ফোল্ডারের প্রতিটি ফাইল একটি sequence; conversation অনুযায়ী একটি বেছে নেয় | মূল |
| ৩ | EVAChat Flow | ১০০% TXT trigger/reply pool + stage machine | ভালো টেস্টেড |
| ৪ | API Chat | persona + deterministic director + LLM | সবচেয়ে জটিল |

### EVAChat Flow

- `data/eva_chat/input/<category>.txt` = user যা বলতে পারে (trigger)
- `data/eva_chat/output/<category>.txt` = bot-এর reply pool
- stages: `NEW → WAIT_AGE_GENDER → WAIT_COUNTRY → MIDDLE → END`
- MIDDLE-এ snap request / horny / flirty / greeting / age / busy / fallback ইত্যাদি শাখা
- horny/flirty → পরের মেসেজ `ask_snap.txt` → রাজি হলে `share_snap.txt` (`%username%`) → END; না হলে `retry_nudge.txt` → second refusal → `closer.txt`
- uncovered age < 18 → snap share block, polite end
- reply cap 8 → closer → END
- pool no-repeat (repeatable pool ছাড়া)
- username গুলো dashboard-এর **Usernames** লিস্ট থেকে; counter increment

এটি মূলত Python-এর `eva_flow.py`-র Go port; `test_flow.py`-র আচরণ Go টেস্টে মিরর করা হয়েছে। **প্রজেক্টের সবচেয়ে ভালো ইঞ্জিনিয়ারিং এই অংশে।**

### API Chat

- persona `data/api_chat/persona.txt` (built-in default seed)
- deterministic director funnel: rapport → interest → hook → ask → share → close
- quality layer:
  - `understanding.go` — message type/parts/keywords/topics
  - `ledger.go` — facts + asked questions
  - `repeat.go` — 0.62 similarity, opener/question repeat
  - `guard.go` — anchor, known facts, question budget, one retry → fallback
  - `style.go` — ৫টি archetype + mirror + interest score
  - `goal.go` — stage goal
  - `sanitize.go` — model special tokens, quotes, links, AI-talk strip
- snap username **সবসময় Go কোড** যোগ করে, মডেল নয়
- API key না থাকলে mode শুরুই হয় না (silent TXT fallback নেই) — ভালো সিদ্ধান্ত
- ডিফল্ট persona স্পষ্টভাবে বলছে: "real girl roleplay... never say you are an AI" — নিচের নৈতিক ঝুঁকি অংশে এটি গুরুত্বপূর্ণ

---

## ৮. ১০২ সাইটের সিস্টেম — বাস্তবতা

- `sites/<id>/site.json` — URL, family, account mode, selectors
- `sites/<id>/dom_driver.js` — universal driver (প্রায় সব সাইটে একই টেমপ্লেট, শুধু config বসানো)
- `sites/<id>/<id>_protection.txt` — entry flow, anti-bot নোট, pacing
- `sites/<id>/accounts|profiles|sessions|logs/` — ৪০৮টি খালি ডিরেক্টরি (ডেটা নেই)
- ৫টি family:
  - `family_a` — pure 1v1 text roulette (৩০)
  - `family_b` — hybrid text/video roulette (২০)
  - `family_c` — guest nickname/DM (২৫)
  - `family_d` — classic anonymous networks (১৫)
  - `family_e` — indie/regional (১০)
  - + chitchat + template = ১০২

**সমস্যা:** এগুলো ১০২টি স্বতন্ত্র ইন্টিগ্রেশন নয়; কার্যত **৫টি selector family + ১টি generic driver**। প্রতিটি সাইটের জন্য আলাদা লাইভ ভেরিফিকেশন, DOM capture বা per-site test নেই। `deep-debug` শুধু ফাইল **আছে কি না** দেখে, সাইটে কাজ করে কি না দেখে না। তাই "১০২ সাইট সাপোর্ট" এখনই একটি **কাঠামোগত দাবি**, প্রমাণিত সাপোর্ট নয়।

আরও একটি স্থাপত্য-জটিলতা: `SiteDir()` টপ-লেভেল `sites/<id>` রিটার্ন করে, কিন্তু `LoadSite()` পড়ে `data/sites/<id>` থেকে; `EnsureSites()` দুটো জায়গায় seed করে এবং `syncExternalSites()` বাইরের ফোল্ডার ভেতরে কপি করে। এই dual-root ডিজাইন debug করা কঠিন এবং ভবিষ্যতে split-brain তৈরি করতে পারে।

---

## ৯. প্রক্সি, ট্রান্সপোর্ট, রিয়েলটাইম

- HTTP CONNECT ও SOCKS5, username/password auth, IPv6, plain-line default protocol
- প্রতিটি Rod/ChromeDP/Show Mode ব্রাউজারের জন্য লুপব্যাক relay
- `Test Proxy` ২০ লাইন/৪ worker সমান্তরালে পরীক্ষা করে; provider-এর 402/407 réponse, `Proxy-Authenticate`, sanitized body excerpt দেখায়
- ব্যর্থ হলে direct fallback নেই — ভালো
- `utls` দিয়ে TLS fingerprint, custom nonce/multipart, release hash scrape — এগুলো স্পষ্টভাবে **anti-bot/evasion-adjacent**; কারিগরি দিক থেকে চতুর, কিন্তু নীতিগত ও আইনি ঝুঁকি বাড়ায়

---

## ১০. ডেটা, সেটিংস, সিক্রেট

- **ডেটা ডির:** `wails.json`-এর পাশে `data/`; `build/bin` থেকে চালালে root-এর `data/`
- **Badger keys:** `settings/browser`, `settings/message`, `settings/proxy`, `settings/general`, `settings/active_site`, `settings/show_mode/v1`, log prefix
- **ফাইল:** `data/profiles/`, `login_info_*.json`, `storage_state.json`, `sites/<id>/accounts/`, `sessions/`, `logs/`, `logs.txt`
- **সিক্রেট:** API key (`AIAPIKey`), proxy username/password, session cookies/tokens
- **এনক্রিপশন:** নেই। Badger/JSON ডিস্কে plaintext-এর মতোই পড়া যায় (OS-level ফাইল পারমিশন ছাড়া কোনো সুরক্ষা নেই)।
- **লগ:** `logs.txt` JSONL, UTC, regex-based redaction; UI-তে bounded feed; **কোনো rotation/retention নেই**; crash dump-এ raw stack থাকতে পারে (ডক নিজেই স্বীকার করে)।
- **ফ্রন্টএন্ড:** `textContent` ব্যবহার করে, `innerHTML` নেই — XSS প্রতিরোধে ভালো।

**Privacy issue:** API Chat-এ stranger-এর মেসেজ, persona, conversational history তৃতীয় পক্ষের LLM provider-এ যায়। এর জন্য user consent, retention policy, DPA বা data-minimisation নেই।

---

## ১১. ভেরিফিকেশন — আমি যা নিজে চালালাম

### ১১.১ Node টেস্ট

```text
npm ci --ignore-scripts
node --test tests/*.test.cjs

# tests 78
# pass 77
# fail 1
```

ব্যর্থ টেস্ট:

```text
not ok 32 - local test.bat covers all four dashboard Message options, offline
error: 'missing root test.bat'
```

অর্থাৎ ZIP-এ root-level `test.bat` নেই, যদিও contract test সেটি আশা করে। এটি একটি **প্যাকেজিং রিগ্রেশন**।

`node --check cmd/chitchatui/frontend/dist/app.js` → **PASS**।

### ১১.২ প্রকৃত verification gate

```text
node scripts/verify.cjs

[PASS] frontend-syntax
[FAIL] ui-contract-package-tests
[BLOCKED] go-toolchain
[BLOCKED] changed-go-format
[BLOCKED] go-dependencies
[BLOCKED] module-checksums
[BLOCKED] go-regression-tests
[BLOCKED] go-static-analysis
[BLOCKED] windows-cross-build
[BLOCKED] windows-app-tests
[BLOCKED] windows-app-vet
[BLOCKED] wails-production-build
[PASS] source-unchanged-during-verification

Overall: FAIL
```

Go টুলচেইন না থাকায় ১০টি চেক **BLOCKED** — এগুলো PASS নয়। অর্থাৎ এই মুহূর্তে প্রজেক্টটি **নিজের ডিফাইন করা রিলিজ গেটও পাস করছে না**।

### ১১.৩ "Deep Debug" এর সমস্যা

```text
node scripts/deep-debug.cjs
Overall Deep Debug Status: PASS
```

কিন্তু এই স্ক্রিপ্টের প্রতিটি ধাপ মূলত **ফাইল/ফোল্ডার আছে কি না** দেখে:
- ১০২টি সাইট ফোল্ডার ও নির্দিষ্ট ফাইল আছে?
- account_mode count ঠিক?
- memory/skills ফাইল আছে?
- `app.js` syntax ঠিক?
- কিছু core Go ফাইল আছে?

এটি কোনো আচরণ, লাইভ সাইট, DOM, API বা ব্রাউজার টেস্ট নয়। এটিকে "PASS" বলে প্রচার করা **green-washing** — বিশেষ করে যখন আসল `verify` FAIL করছে।

### ১১.৪ ডকুমেন্ট বনাম বাস্তবতা

- `docs/START-HERE.md` বলছে ডাউনলোডে `VERIFICATION-REPORT.json`, `SOURCE-MANIFEST.json`, `verification-evidence/` থাকবে — **কোনোটিই ZIP-এ নেই**।
- `AGENTS.md` বলছে Go 1.25+ / Wails 2.12; `go.mod` বলছে `go 1.24.0`; `install.bat` বলছে Go 1.24+ — তিন জায়গায় তিন কথা।
- `docs/README.md`-এ encoding mojibake (`â€”`)।
- `AGENTS.md`-এ license-এর উল্লেখ আছে, কিন্তু বাস্তবে কোনো license package/enforcement কোড পাওয়া যায়নি।

---

## ১২. যা সত্যিই ভালো (Credit যেখানে প্রাপ্য)

1. **EVAChat engine** — stage machine, no-repeat pool, age gate, reply cap, username rendering; Python spec-এর সাথে মিলিয়ে টেস্ট করা। এটাই প্রজেক্টের সেরা অংশ।
2. **API Chat decomposition** — director, understanding, ledger, repeat, guard, style, goal, sanitize আলাদা ফাইলে; deterministic funnel + LLM wording — ভাবনাটি পরিণত।
3. **DOM state machine** — MutationObserver, message ID dedupe, author classification, own echo suppression, send confirmation, uncertain pause; বাস্তব DOM-এর জটিলতা বোঝা যায়।
4. **Defensive coding** — `pathInside()` দিয়ে data dir-এর বাইরে delete বন্ধ, `safeProfileName()` Windows reserved name/path traversal handle, no API key → refuse, age <18 → no share, proxy failure → no direct fallback।
5. **Multi-worker registry** — একই user-কে duplicate reply না করার claim mechanism।
6. **Proxy diagnostics** — শুধু "failed" না বলে provider status/body excerpt দেখানো; debugging-এ দুর্দান্ত।
7. **Docs/memory system** — `AGENTS.md`, `PROJECT_MAP`, `PROJECT_MEMORY` — দীর্ঘমেয়াদে agent/human onboarding-এ সহায়ক (যদিও কিছু তথ্য পুরনো)।
8. **XSS hygiene** — UI-তে `innerHTML` নেই, সব `textContent`।
9. **Test culture** — ২২০টি Go test function, ৭৮টি Node test; fake backend, temp dir, synthetic data ব্যবহার — নীতি সঠিক।
10. **Capture tool** — standalone, dependency-free, redacted by default — future debugging-এর জন্য বুদ্ধিমান 투자।

---

## ১৩. গুরুতর সমস্যা (P0 — এখনই ঠিক করা দরকার)

### P0-1. রেপো/সাপ্লাই-চেইন ইন্টিগ্রিটি

**প্রমাণ:**
- Git-এ একটি কমিট, শুধু ZIP; সোর্স ট্রি নেই।
- `build/`-এ ৩টি EXE (৫৫ MB), কোনো checksum/manifest/signature নেই।
- `chitchat-Check.exe`-এর কোনো source reference বা build target পাওয়া যায়নি (cmd-এ chitchatui, chitchatbot, chitchattest, apichat আছে, Check নেই)।
- `.gitignore`/`LICENSE`/CI নেই।

**প্রভাব:** কোন বাইনারি কোন সোর্স থেকে এসেছে তা যাচাই করা অসম্ভব; কোড রিভিউ/অডিট/রিপ্রোডিউসিবল বিল্ড নেই; ভবিষ্যতে ভুল করে সিক্রেট কমিট হওয়ার ঝুঁকি।

**সমাধান:** সোর্স ট্রি Git-এ কমিট করুন, ZIP ও EXE বাদ দিন, `.gitignore` + `LICENSE` + README + CI যোগ করুন, SBOM/checksum/signing চালু করুন, reproducible build করুন।

### P0-2. "Auto-Register" আসলে রেজিস্ট্রেশন করে না

`internal/sites/autoreg.go` শুধু:

1. সাইটের start URL-এ একটি HTTP `GET` করে (৩ সেকেন্ড timeout);
2. নিজের পক্ষ থেকে টোকেন বানায়: `token = "autoreg_<site>_<random hex>"`;
3. কুকি বসায়: `age_verified=1`, `terms_accepted=true`, `guest_gender=female`, `guest_age=20..23`;
4. localStorage-এ একই মান;
5. status `"ready"` লিখে account সেভ করে।

এটি কোনো সাইট API-তে account তৈরি করে না, CAPTCHA/Turnstile solve করে না, email/verification করে না। বাস্তব সাইট এই কুকি গ্রহণ করবে না। অথচ **৮৬টি সাইট** `account_mode: auto_register` হিসেবে চিহ্নিত, আর `deep-debug` এটিকে PASS বলে।

**প্রভাব:** ভুয়া "ready" account count, worker-এ login failure, ভুল capability matrix, ভবিষ্যতে ban-ঝুঁকি।

**সমাধান:** যতক্ষণ বাস্তব রেজিস্ট্রেশন implement না হয়, `auto_register` mode ও UI থেকে সরান বা স্পষ্টভাবে "experimental/unsupported" লেবেল দিন। `browser_setup` (manual capture) কে default করুন। সিন্থেটিক টোকেন/কুকি কখনো "ready" হিসেবে সেভ করবেন না।

### P0-3. ভেরিফিকেশন ও "PASS" সংস্কৃতি

- আসল গেট FAIL + ১০টি BLOCKED;
- `deep-debug` শুধু existence check করে PASS দেয়;
- ডক দাবি করে রিপোর্ট ZIP-এ আছে, নেই;
- `PROJECT_MEMORY`-তে "PASS" ভাষা ব্যবহৃত।

**প্রভাব:** release-এ অজানা ভাঙা ফিচার, ভুল সিদ্ধান্ত, audit-এ বিশ্বাসযোগ্যতা হারানো।

**সমাধান:** `deep-debug`-কে "advisory smoke check" নাম দিন, PASS/FAIL শব্দ বাদ দিন; আসল `verify` PASS না হলে release বন্ধ; CI-তে Windows+Linux গেট চালান; রিপোর্ট ZIP-এ বাধ্যতামূলক করুন; "BLOCKED ≠ PASS" নীতিটি কঠোরভাবে প্রয়োগ করুন।

### P0-4. লিগ্যাল, ToS ও নৈতিক ঝুঁকি (সবচেয়ে বড়)

প্রজেক্টটি যা করে:
- ১০০+ থার্ড-পার্টি চ্যাট সাইটে **স্বয়ংক্রিয়,大规模, fake female persona** চালায়;
- `18+` checkbox নিজে থেকে টিক করে, বয়স ২১ বসায়, female alias নেয়;
- proxy rotation + `utls` fingerprint + rate pacing দিয়ে detection এড়ানোর চেষ্টা করে;
- stranger-দের Snapchat username-এ funnel করে;
- API Chat-এ মডেলকে নির্দেশ দেওয়া হয় "never say you are an AI", "roleplaying a real girl"।

এটি অধিকাংশ সাইটের **Terms of Service** (automation, fake identity, spam, harassment) ভাঙে। অনেক এখতিয়ারে এটি deceptive practice, spam, unauthorized automation, privacy/age-related আইনের আওতায় পড়তে পারে। বাংলাদেশ/সিঙ্গাপুর/ইইউ/যুক্তরাষ্ট্র — সব জায়গায় ঝুঁকি ভিন্ন, কিন্তু ঝুঁকি আছে। প্ল্যাটফর্ম ban, IP/device ban, civil action,甚至在特殊情况 criminal exposure 가능।

**করণীয় (প্রকৌশল হিসেবে):**
- আইনজীবী দিয়ে প্রতিটি target site-এর ToS ও স্থানীয় আইন যাচাই;
- fake persona/automation বাদ দিয়ে opt-in, disclosed, consent-based product-এ pivot;
- age verification ও safeguarding নীতি;
- proxy/anti-bot evasion functionality সরানো;
- data minimisation, retention, user consent, DPA;
- "Snapchat funnel" ব্যবসায়িক মডেলটিই পুনর্বিবেচনা।

> আমি এই রিভিউতে evasion আরও শক্তিশালী করার কোনো পরামর্শ দিচ্ছি না — কারণ সেটি ঝুঁকি বাড়ায়, কমায় না।

---

## ১৪. গুরুত্বপূর্ণ সমস্যা (P1)

| # | সমস্যা | প্রমাণ/কারণ | প্রভাব |
|---|---|---|---|
| P1-1 | **God objects** | `app.go` ৬১ KB, `logic.go` ৫৩ KB, `catalog.go` ৪১ KB, `eva_chat.go` ৩৮ KB | টেস্ট/মেইনটেইন/মার্জ করা কঠিন; change ripple |
| P1-2 | **১০২ সাইটের ভ্রম** | ৫টি selector family, একই driver template, ৪০৮ খালি ডিরেক্টরি, per-site live test নেই | ভুল প্রতিশ্রুতি, maintenance burden |
| P1-3 | **সিক্রেট/PII at rest** | Badger/JSON-এ API key, proxy password, cookies; encryption নেই | ডিস্ক/ব্যাকআপ/ম্যালওয়্যার থেকে credential theft |
| P1-4 | **ভঙ্গুর DOM selectors** | chitchat-এর `data-message-id`, `pt-(--message-group-gap)` class-এর উপর নির্ভর | সাইট রিডিজাইনে ভাঙবে; custom JS override-ও ভাঙতে পারে |
| P1-5 | **Dual-root site split-brain** | `SiteDir` external, `LoadSite` data/sites, sync copy | কোথায় edit হচ্ছে তা বিভ্রান্তিকর; overwrite ঝুঁকি |
| P1-6 | **প্যাকেজিং ভাঙা** | root `test.bat` নেই → contract test fail; রিপোর্ট ZIP-এ নেই | user experience ও release gate ব্যর্থ |
| P1-7 | **ডক অসমঞ্জস** | Go 1.24 vs 1.25; license উল্লেখ কিন্তু কোড নেই; mojibake | নতুন ইঞ্জিনিয়ার ভুল পথে যাবে |
| P1-8 | **লাইসেন্স নেই** | `LICENSE` ফাইল নেই | বিতরণ/ব্যবসায়িক ব্যবহারের আইনি ভিত্তি অস্পষ্ট |
| P1-9 | **তৃতীয় পক্ষের LLM-এ PII** | Featherless/DeepSeek-এ chat content | privacy/GDPR/PDPA ঝুঁকি, provider ToS |
| P1-10 | **Safe send-এর পরও global queue** | multi-browser/conversation concurrency জটিল | race/ordering bug সম্ভাবনা (যদিও টেস্ট আছে) |

### P2 (মাঝারি)

- লগ rotation/retention নেই → unbounded `logs.txt` ও Badger growth।
- vanilla JS ১,৪২৩ লাইন, কোনো module/type/build → refactor ও টেস্ট কঠিন (তবে `textContent` ভালো)।
- accessibility/i18n নেই।
- no metrics/tracing; শুধু log।
- govulncheck/SBOM/dependency audit নেই।
- browser automation inherently flaky; retry policy সীমিত।
- `capture/` raw mode-এ sensitive data capture করতে পারে — operator discipline-এর উপর নির্ভরশীল।
- `site.json`-এ arbitrary URL/driver JS — local config compromise হলে logged-in session-এ JS 실행 가능।

---

## ১৫. সিকিউরিটি থ্রেট মডেল (সংক্ষেপ)

**Assets:** session cookies/tokens, API key, proxy credentials, stranger chat PII, Snapchat handles, account profiles.

**Threats:**
1. **Local disk theft** → plaintext Badger/JSON থেকে সব সিক্রেট।
2. **Malicious/compromised site config** → `dom_driver.js` পেজ কনটেক্সটে arbitrary JS, session exfiltration সম্ভব।
3. **Third-party LLM** → chat content ও persona provider-এ; provider breach/retention।
4. **Proxy provider** → সব ট্রাফিক তাদের মাধ্যমে; logs/metadata।
5. **Log leakage** → regex redaction সীমিত; crash dump raw থাকতে পারে।
6. **Supply chain** → unsigned EXE, no checksum, no reproducible build।
7. **Platform detection** → ban, device fingerprint, IP reputation; evasion বাড়ালে পাল্টা ব্যবস্থাও বাড়ে।

**ভালো নিয়ন্ত্রণ:** `pathInside`, safe profile names, no `innerHTML`, no key → refuse, age gate, per-site isolation, bounded UI logs, send confirmation, registry claims।

**অনুপস্থিত নিয়ন্ত্রণ:** secret encryption, keychain/DPAPI, config signing, integrity check, audit log tamper-evidence, data retention, consent, network egress allowlist, sandboxing of injected JS, rate-limit budget across workers।

---

## ১৬. রোডম্যাপ — কীভাবে এটিকে প্রকৃত প্রোডাক্ট বানানো যায়

### ০–২ সপ্তাহ (P0)

1. **Git ঠিক করুন:** সোর্স ট্রি কমিট, ZIP/EXE বাদ, `.gitignore`, `LICENSE`, README, `SECURITY.md`।
2. **Verification gate ঠিক করুন:** root `test.bat` ফেরান; Windows CI-তে `npm run verify` চালান; রিপোর্ট ZIP-এ যুক্ত করুন; `deep-debug`-কে "advisory" করুন।
3. **Auto-register সরান** যতক্ষণ বাস্তব implementation না হয়; ৮৬ সাইটের capability matrix-এ "untested" লিখুন।
4. **সিক্রেট সুরক্ষা:** Windows DPAPI/Keychain; API key proxy password আলাদা করুন; logs থেকে PII scrub।
5. **লিগ্যাল রিভিউ:** ToS, consent, disclosure, age assurance; evasion ফিচার বন্ধ।

### ৩০–৬০ দিন (P1)

6. `app.go`/`logic.go` ভেঙে service packages; interface + DI; handler thin করুন।
7. ১০২ ডিরেক্টরি → ৫টি family template + per-site override; empty dirs সরান; JSON schema validation; selector health check।
8. CI: Linux + Windows matrix, `go vet`, `staticcheck`, `govulncheck`, race detector, coverage threshold।
9. Site root একটাই করুন; migration লিখুন; sync logic সরল করুন।
10. PII minimisation: LLM-এ পাঠানোর আগে redact; retention policy; local model option।

### ৬০–৯০ দিন (P2)

11. Local fake chat site দিয়ে end-to-end DOM/bot test (Playwright/Rod) — আসল সাইটে নয়।
12. Observability: structured metrics, crash report, per-site success rate।
13. Reproducible build + signing + SBOM + signed updater।
14. Frontend modularize (Vite/TS) বা অন্তত ES modules + unit tests।
15. Log rotation, Badger GC, performance profiling (১০২ সাইট sync, startup time)।

---

## ১৭. নতুন ইঞ্জিনিয়ারের পড়ার ক্রম

1. `AGENTS.md` — scope/rules (কিন্তু তারিখ ও পুরনো দাবি যাচাই করুন)
2. `docs/PROJECT_MAP.md` — auto-generated inventory
3. `docs/PROJECT_MEMORY.md` — সিদ্ধান্তের ইতিহাস (শেষ অংশে বাস্তব অবস্থা)
4. `docs/SHOW_MODE.md` → `internal/browser/showmode_dom.js`
5. `docs/CHITCHAT_PROTOCOL.md` + `chitchat-protocol.json`
6. `internal/bot/run.go` → `internal/bot/logic.go` → `internal/bot/registry.go`
7. `internal/bot/eva_chat.go` → `docs/CHAT_ENGINE_MAPPING.md`
8. `internal/apichat/director.go` → `engine.go` → `guard.go`
9. `app.go` → `cmd/chitchatui/frontend/dist/app.js`
10. `scripts/verify.cjs` + `tests/` — কীভাবে যাচাই হয়

---

## ১৮. শেষ কথা

এই প্রজেক্টে **প্রকৃত ইঞ্জিনিয়ারিং পরিশ্রম** আছে — বিশেষ করে EVAChat state machine, API Chat quality layer, DOM observation, proxy diagnostics এবং per-site isolation-এর ধারণা। ডকুমেন্টেশনও সাধারণ হobby প্রজেক্টের চেয়ে অনেক ভালো।

কিন্তু এটি এখন **"polished demo + ambitious prototype"**, **"verified commercial product" নয়**। তিনটি কারণেই এটি বাণিজ্যিকভাবে ঝুঁকিপূর্ণ:

1. **ডেলিভারি ও ভেরিফিকেশন বিশ্বাসযোগ্য নয়** — ZIP-only Git, unsigned EXE, FAIL করা gate, existence-only "PASS"।
2. **Auto-register ভুয়া** — ৮৬ সাইটের "ready account" বাস্তবে কাজ করার প্রমাণ নেই।
3. **লিগ্যাল/নৈতিক এক্সপোজার সর্বোচ্চ** — fake persona + mass automation + proxy evasion + Snapchat funnel।

যদি লক্ষ্য হয় **শেখা/R&D**, তাহলে এটি চমৎকার একটি স্যান্ডবক্স — বিশেষ করে EVA ও API Chat ডিজাইন পড়ার জন্য।  
যদি লক্ষ্য হয় **বাণিজ্যিক প্রোডাক্ট**, তাহলে আগে P0 চারটি ঠিক করুন; তারপর আর্কিটেকচার ও সাইট কভারেজে বিনিয়োগ করুন। তা না করে ফিচার বাড়ালে শুধু технический ঋণ ও আইনি ঝুঁকি বাড়বে।

---

### পরিশিষ্ট: এই রিভিউতে ব্যবহৃত কমান্ড

```bash
# ZIP extract (analysis workspace, repo-এর বাইরে)
unzip -q "ChitChat-App-clean-fixed-2026-10-20 (11).zip"

# Node dependencies
cd source_code
npm ci --ignore-scripts --no-audit --no-fund

# Syntax + unit tests
node --check cmd/chitchatui/frontend/dist/app.js
node --test tests/*.test.cjs          # 78 tests, 77 pass, 1 fail

# Verification gate
node scripts/verify.cjs               # FAIL; 10 BLOCKED

# Advisory existence checks
node scripts/deep-debug.cjs           # "PASS" (existence only)

# Go checks: NOT RUN — Go toolchain unavailable in this environment
```

**সীমাবদ্ধতা:** Windows-এ আসল অ্যাপ, Rod ব্রাউজার, লাইভ chitchat.gg লগইন, প্রক্সি ও DOM আচরণ এখানে চালানো হয়নি। তাই সেগুলো **NOT_RUN**, PASS নয়। Go টেস্ট/বিল্ড Windows মেশিনে বা CI-তে চালিয়ে `verification-output/report.json` দেখতে হবে।
