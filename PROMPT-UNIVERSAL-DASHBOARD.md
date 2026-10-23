# UNIVERSAL DASHBOARD PROMPT — build the **same** chat-bot dashboard for **any** chat site

> **এই ফাইলটা কী (আপনার জন্য, একবার পড়ুন):**
> এটা এমন একটা prompt যেটা আপনি **যেকোনো সময়, যেকোনো agent** (Claude Code /
> Cursor / Codex / v0 / Replit / Arena) কে দিতে পারবেন। Agent এটা পড়ে —
>
> 1. প্রথমে **সাইটের logic বুঝে নেবে** (login, message আনা-পাঠানো, skip, DM,
>    protocol) — নিজে থেকে অনুমান করবে না, দরকার হলে আপনাকে নির্দিষ্ট প্রশ্ন করবে;
> 2. তারপর ঠিক **একই dashboard** বানাবে — layout, section, field, default,
>    validation, status text, log feed, counter — সব হুবহু এক, শুধু ভেতরের
>    adapter বদলাবে;
> 3. dashboard-এ **কোনো সাইটের নাম/URL কোথাও থাকবে না** — শুধু নিরপেক্ষ
>    branding (`Chat Bot`, `CB`, `Desktop Engine`) আর capability state;
> 4. শেষে একটা **parity test** চালাবে যা প্রমাণ করবে dashboard আগের মতোই আছে
>    আর কোনো সাইট-নাম leak হয়নি।
>
> **কীভাবে দেবেন:** PART A পুরোটা কপি করুন → ওপরে শুধু **"THIS BUILD"** অংশের
> ৪টা লাইন ভরুন (কোন সাইট, কীভাবে ধরবে, capture আছে কি না) → agent-কে দিন।
> সাইট না জানা থাকলেও দিতে পারেন — তখন agent Discovery phase চালাবে।
>
> **নতুন সাইটে পুরোনো dashboard রাখতে:** কিছুই বদলাতে হবে না — agent শুধু নতুন
> `adapters/<site_id>/` লিখবে; UI ফাইল ছোঁবে না (parity test fail করবে যদি ছোঁয়)।
>
> PART B = একই জিনিসের ছোট ভার্সন (ছোট context window হলে)।
> Appendix = parity test কোড, capability matrix, "new site in 30 minutes" runbook।

---

# PART A — THE PROMPT (copy everything below)

## THIS BUILD (fill these 4 lines, nothing else)

```
TARGET SITE (internal only):   <<site name or "unknown — run Discovery">>
ENTRY / LOGIN:                 <<e.g. guest token, email login, manual browser login / "unknown">>
SITE BRIEF / CAPTURE PROVIDED: <<yes: path/URL to brief, HAR, screenshots, docs / no>>
PRODUCT NAME FOR THE UI:       <<default "Chat Bot" — must NOT contain the site name>>
```

Everything below is **frozen**. Do not add, remove, rename or simplify screens.
The only thing that changes per site is the adapter (section 4) and the values in
`config/site.json` (section 3) — **never the UI, never the option set**.

---

## 1. MISSION

Build (or extend) a **desktop-class chat-automation dashboard** that drives
**one** chat site at a time through a pluggable adapter. The dashboard is a
product of its own: **the same binary, the same screens, the same options** for
every site. Swapping sites = swapping an adapter folder + a config file, no UI
change, no rebuild of the front end.

The dashboard has exactly two pages:

- **Show Mode** — launches a *visible* browser, the operator logs in once, and the
  bot chats inside that browser while every IN/OUT/SKIP event streams into Live
  Logs. Closing the browser stops the run.
- **API Mode** — headless background workers (one per saved profile) that match,
  send the first line, reply, skip, DM, and log, with six live counters.

Both pages share proxy, message content and chat-mode selection; each has its own
timing settings and its own log feed.

## 2. NON-NEGOTIABLE RULES

1. **The UI never names a site.** No site name, brand name, domain, URL, logo or
   "target site" label anywhere in `frontend/dist/**` or in any string the UI
   renders. Allowed only: the neutral product name from config (default
   **"Chat Bot"**), the neutral initials (**"CB"**), the neutral subtitle
   (**"Desktop Engine"**), and neutral state words (section 6.1).
2. **Frozen UI contract.** Every screen, section, field id, default, validation
   message, status text, log format and counter in sections 5–8 exists in every
   build. A build for a different site must produce a **byte-identical UI bundle**
   (verified by the parity test in Appendix B).
3. **One site per build.** No site picker, search, filters, site list, per-site
   table, "add site" form, no `ListSites/SelectSite/CreateSite`. The site id lives
   in `config/site.json` and is never shown.
4. **Capability-driven, not layout-driven.** If a site lacks a feature (DMs,
   skip button, report, guest login), the control is **disabled with a neutral
   reason** (`Not supported by the current adapter`) — it is never removed, moved
   or hidden.
5. **Adapter isolation.** All site-specific knowledge (URLs, selectors, HTTP/WS
   endpoints, payloads, auth, rate limits) lives in `adapters/<site_id>/` +
   `config/site.json`. Nothing site-specific may appear in `ui/`, `engine/` or
   `cmd/`.
6. **No fake behaviour.** Every button calls a real backend operation; its status
   line shows the real result. Failure → `.status-error`, never success.
7. **Discovery before code.** If the site brief is missing or incomplete, run the
   Discovery phase (section 3) and ask the user — do **not** silently invent a
   URL, selector, header or payload.
8. **Save-on-run only.** No Save button in Setting / Proxy / Message / Bot
   Settings. Run Browser and Save & Start Bot persist everything first.
9. **Typed values survive navigation.** Global form store with `show` / `api`
   slices; switching tab/section never loses input.
10. **Idle / busy / running** are distinct and enforced on the buttons.
11. **No secrets in the UI.** API keys are `type="password"`, stored only in
    local `data/`, never logged, never echoed to the page or the log feed.
12. **Safety floor.** Age < 18 ⇒ no personal-contact sharing; the engine refuses
    it in code (not just in copy). Never log credentials, cookies or session
    tokens. Respect the site's rate limits and terms; the operator is told when
    an action can get an account banned.

## 3. PHASE 1 — DISCOVERY (do this first, ≤ 10 questions)

Goal: learn **how the site actually works** before writing a line of UI.

### 3.1 Inputs to look for, in this order

1. A **site brief** the user provides (`docs/sites/<site_id>.md`, screenshots,
   notes). Highest priority — treat it as the source of truth.
2. A **capture bundle** (HAR / WebSocket frames / DOM snapshots / storage dump)
   recorded manually by the operator. Parse it; never re-record it yourself.
3. The public site, **only read-only** and only if the user explicitly allows it.
4. Nothing. Then ask the questions in 3.3 and stop.

### 3.2 What must be learned (fill `docs/sites/<site_id>.md`)

| Area | What you must know |
|---|---|
| Identity | internal `site_id` (slug), start URL, whether it is 1v1 text / room / DM |
| Login | guest vs account, fields, token shape, storage keys, cookie/localStorage/JWT, "logged-in" detection |
| Session | how to persist & restore it (`sites/<site_id>/sessions/latest_session.json`) |
| Matching | how a stranger/partner is acquired (queue POST, auto-match, click), and its response |
| Incoming messages | WebSocket frame type / polling endpoint / DOM node + selector |
| Outgoing messages | endpoint + payload **or** DOM input + send button |
| Typing state | optional typing indicator endpoint/event |
| Skip / next | endpoint or DOM button; what happens to the partner |
| DMs | how a private message is started/sent (or "unsupported") |
| Report / block | optional (or "unsupported") |
| Rate limits | min interval between sends, per-chat caps, soft-ban signals |
| Errors | how the site reports a dead session / banned account / captcha |
| Assets | avatar/name fields if the site shows them |

### 3.3 Ask only what the brief cannot answer

One message, max 10 bullets, each with a concrete default you will use if they
don't answer. Example shapes:
- "Login: guest token auto-created, or manual browser login once? (default: manual once, then reuse session)"
- "Incoming messages: WebSocket or polling or DOM? (default: whichever the capture shows; if no capture: DOM)"
- "Skip: button only, or API? (default: button)"

### 3.4 Discovery deliverables (before any UI work)

`docs/sites/<site_id>.md` with every row of 3.2 filled or marked
`TODO(site)` **plus** `config/site.json` (section 3.5) plus a short
`adapters/<site_id>/README.md` explaining the flow end-to-end.

### 3.5 `config/site.json` (never rendered in the UI)

```jsonc
{
  "site_id": "site_a",                 // slug only, internal
  "start_url": "https://example.chat/", // internal only
  "adapter": "adapters/site_a",
  "capabilities": {
    "guest_login": true, "manual_login": true,
    "ws_incoming": true, "poll_incoming": false, "dom_incoming": true,
    "dom_send": true, "api_send": false,
    "skip": true, "report": false, "dms": false, "typing": true,
    "auto_match": true
  },
  "limits": { "send_interval_ms": 1200, "max_replies_per_chat": 8 }
}
```

Capabilities drive only **enabled/disabled** states in the UI, never layout.

## 4. PHASE 2 — ADAPTER CONTRACT (the only site-specific code)

Implement **one interface set** and one concrete package per site. The engine and
the UI depend only on these interfaces.

```go
package adapter

type Site interface {
    ID() string
    StartURL() string
    Capabilities() Capabilities

    // Login & session
    LoginManual(ctx context.Context, b Browser) (Session, error)   // visible browser, save on success
    RestoreSession(ctx context.Context, b Browser) (Session, error)
    SessionValid(ctx context.Context, s Session) bool

    // Matching
    Match(ctx context.Context, s Session) (Partner, error)         // acquire a chat partner
    Partner(ctx context.Context, s Session) (Partner, error)

    // Conversation
    Receive(ctx context.Context, s Session) (<-chan Incoming, error) // ws/poll/dom → one channel
    Send(ctx context.Context, s Session, text string) error
    Typing(ctx context.Context, s Session, on bool) error            // ErrUnsupported ok
    Skip(ctx context.Context, s Session, reason string) error
    Close(ctx context.Context, s Session, reason string) error

    // Optional
    DM(ctx context.Context, s Session, text string) error            // ErrUnsupported ok
    Report(ctx context.Context, s Session) error                     // ErrUnsupported ok
}

type Incoming struct { Text string; At time.Time; From Partner }
type Partner  struct { ID, Name string; Meta map[string]string }
type Session  struct { Data json.RawMessage; SavedAt time.Time } // opaque to the engine
type Capabilities struct { GuestLogin, ManualLogin, WSIncoming, PollIncoming,
    DOMIncoming, DOMSend, APISend, Skip, Report, DMs, Typing, AutoMatch bool }
```

Rules for adapters:
- Return `ErrUnsupported` (a sentinel) for missing features — the UI translates it
  into a neutral disabled state, never an error toast.
- Never print the site id, URL or any payload into the log feed; log neutral
  events (`matched`, `sent`, `skipped`, `session saved`).
- Respect `limits.send_interval_ms`; the engine also enforces it.
- Use the **browser transport** for Show Mode and the **API/WS transport** for API
  Mode when both exist; otherwise fall back to the one that exists (the engine
  decides from `Capabilities`).

## 5. PHASE 3 — THE DASHBOARD (frozen spec)

### 5.1 Shell

```
┌──────────── 238px sidebar (#191b17) ─────────────┬──────── main (scroll) ────────┐
│ [CB] Chat Bot                                    │  Show Mode   [Idle]   Refresh │
│      Desktop Engine                              │  frozen one-line description  │
│                                                  │                               │
│ ┌ Show Mode ───────────────────────────────────┐ │  ┌── 1 · Setting ───────────┐ │
│ │  API Mode     (active = accent tint+border)  │ │  │ hint + fields            │ │
│ └──────────────────────────────────────────────┘ │  └──────────────────────────┘ │
│                                                  │  ┌── 2 · Proxy ─────────────┐ │
│   Setting · Proxy · Message · Run Browser ·      │  │ …                        │ │
│   Live Logs        (active = accent pill)        │  └──────────────────────────┘ │
│                                                  │                               │
│ ┌ Adapter ─────────┐                             │                               │
│ │  ready · session │                             │                               │
│ └──────────────────┘                             │                               │
│ ┌ Workers ─────────┐                             │                               │
│ │        [Stopped] │                             │                               │
└──────────────────────────────────────────────────┴───────────────────────────────┘
```

- Brand: 40px accent square with **CB**, title **Chat Bot**, subtitle
  **Desktop Engine**. All three come from
  `config/branding.json` = `{ "mark": "CB", "name": "Chat Bot", "subtitle": "Desktop Engine" }`
  and are the **only** branding strings. The parity test fails if any other
  product/site name appears in the bundle.
- Adapter box (bottom, above Workers): one line, neutral:
  `Adapter ready` / `Adapter not configured`, and `· session saved {date}` when a
  session exists. **No site name, no URL, no folder path.**
- Workers footer: label **Workers** + pill **Stopped** / **Running**.
- Sub-nav ids `subnav-show`, `subnav-api`; only the active one is visible.
- **Nothing else in the sidebar.**

### 5.2 Design tokens (exact)

```css
:root{
  color-scheme: dark;
  --bg:#161716; --panel:#20221e; --panel-alt:#282b25; --panel-strong:#30342d;
  --line:#3e4439; --text:#eef2e8; --muted:#b5beb0; --accent:#83cb73;
  --accent-strong:#6bb25b; --warn:#d9b74d; --danger:#d36a6a;
}
```

Font `"Segoe UI", Arial, sans-serif`; 15px / 1.45.
`.shell{display:none}` → `.shell.visible{display:flex}` after the first state load.
Sidebar: `flex:0 0 238px`, sticky, full height, `gap:22px`, `padding:22px 16px`,
`background:#191b17`, right border 1px `var(--line)`.
`.brand-mark` 40×40, radius 9, accent background, dark bold initials.
`.brand-title` 18/700 · `.brand-subtitle` 12 muted.
`.nav-button` full width, `11px 14px`, radius 8, transparent, 14px; active →
`background:rgba(131,203,115,.10); border-color:rgba(131,203,115,.45); color:var(--accent)`.
`.subnav` column, `gap:4px`, `padding:6px 0 6px 10px`, left rail 1px `var(--line)`;
`.subnav-button` `7px 10px`, radius 7, 12.5px muted; active →
`border-color:rgba(131,203,115,.28); background:rgba(131,203,115,.10); color:var(--accent)`.
`.pill` radius 999, `4px 10px`, 12/700; `.running` green-tint, `.stopped` red-tint.
`.panel-header` flex row space-between, gap 18, margin-bottom 18; `h1` 26/700;
description `max-width:720px; 14px muted`; `.mode-badge` small pill, `.live` blue-grey.
`.subpanel{display:none}` / `.active{display:block}`; `.subpage{max-width:1100px}`,
`.subpage.narrow{max-width:760px}`.
`.card` flex column, gap 11, padding 18, 1px `var(--line)`, radius 14,
`background:var(--panel)`; `h2` 17/700 numbered `{n} · {Name}`.
`.form-grid` 2-col gap 14; `.span-2` spans both.
Inputs: `var(--panel-alt)`, 1px line, radius 8, `9px 11px`; focus accent border.
Labels muted 12.5px above the control; `.hint-text` 13 muted with `<code>` chips.
Buttons: 1px line, radius 8, `9px 14px`, 13.5/600; `.primary-button` accent fill +
`#162010`; `.secondary-button` panel-alt; `.danger-button` transparent/red.
`.run-button{width:100%;padding:12px 16px;font-size:14.5px}`.
`.status-line` 13 muted; `.status-ok` accent; `.status-error` danger;
all with `role="status" aria-live="polite"`.
`.home-stats` 6 columns, gap 10; `.stat-capsule` radius 999, padding 11/14,
green-tint border, label 12 muted + `strong` 17 accent; `.danger` variant red.
`.chat-mode-card` grid gap 9, padding 12/14, radius 10, `background:#23261f`;
`.mode-label` 11.5/700 uppercase letter-spacing .5 muted; `.mode-options` flex wrap
gap 8; `.mode-option` inline-flex gap 8, padding 8/12, 1px line, radius 8,
13.5px; `input{width:auto;margin:0;accent-color:var(--accent)}`; `.active` →
accent border + tint + accent text.
`.log-list` overflow auto, `min-height:280px`, padding 6/12, radius 10,
`background:#191b17`; inside `.log-card` it fills the height.
`.log-row` grid `60px 44px minmax(0,1fr)`, gap 8, padding 7/0, bottom hairline.
`.log-time` 12 muted. `.log-badge` 10/700 centered radius 5 panel-strong,
per-kind tint (in/out accent, skip warn, system blue-grey, error danger).
`.log-body` 13px, `.log-meta` 11.5 muted.

### 5.3 Show Mode page

Header: `h1` **Show Mode** + `#show-run-badge` (**Idle** muted / **Running**
accent + `.live`), right `#show-refresh` **Refresh**.
Description (frozen): *"Configure, press Run Browser, log in once. The visible
browser chats with these rules; close it to stop."*

**`1 · Setting`** — hint: *"Run Browser applies these values. No separate save."*

| id | label | control | range | default |
|---|---|---|---|---|
| `sm-first-mode` | First-message policy | select: Bot first `bot` / User first `user` / Random `random` | | `bot` |
| `sm-first-percent` | Bot first chance (%) | number, row `sm-first-percent-row` only when random | 0–100 | 70 |
| `sm-first-wait` | Wait for first message | select: Wait indefinitely `unlimited` / Skip after timeout `skip`; row hidden when mode=bot | | unlimited |
| `sm-first-min` / `sm-first-max` | First timeout min (s) / First timeout max (s) | number, row `sm-first-timeout-row` only when mode≠bot AND wait=skip | 1–86400 | 30 / 60 |
| `sm-reply-min` / `sm-reply-max` | Reply delay min (s) / Reply delay max (s) | number | 0–86400 | 0 / 0 |
| `sm-auto-skip` | Auto-skip when the partner stops replying | checkbox | | off |
| `sm-noreply-min` / `sm-noreply-max` | No-reply timeout min (s) / No-reply timeout max (s) | number | 0–86400 | 60 / 60 |
| `sm-next-min` / `sm-next-max` | Next chat wait min (s) / Next chat wait max (s) | number | 0–86400 | 0 / 0 |

Hidden rows are also **disabled**.

**`2 · Proxy`** (`.subpage.narrow`) — `sm-proxy-enabled` **Enable proxy** ·
`sm-proxy-scheme` **Default protocol for plain lines** (HTTP `http` / SOCKS5
`socks5`) · `sm-proxy-entries` textarea 5 rows (placeholder shows
`host:port`, `host:port:user:pass`, `socks5://user:pass@host:port`) ·
`sm-test-proxy` **Test Proxy** · `sm-proxy-status` default
**"Not tested."** · `sm-proxy-results`.
Test: socks5 scheme prefixes plain lines with `socks5://`; while running the
button is disabled and reads "Testing…"; per row
`Line {n}: PASS | REACHABLE / SITE RESTRICTED | FAIL — {label} — {latency} ms[ — HTTP {status}] — {detail}`
plus optional `Provider said: "…"`, `Auth challenge: …`, `Next step: …`;
summary `{passed}/{total} passed; {reachable} HTTPS tunnel(s) verified. A test is
not a browser-login guarantee.`; if the textarea changed during the test append
` List edited: these results are for the previous values; test again.`;
empty input → **"Enter at least one proxy to test."**

**`3 · Message`** — `.chat-mode-card` with `.mode-label` **Chat mode** and
exactly three chips (`name="sm-chat-mode"`):
`sm-mode-option-single`/`sm-chat-mode-single` `single` **Single Sequence** ·
`sm-mode-option-folder`/`sm-chat-mode-folder` `folder` **Folder Sequences** ·
`sm-mode-option-flow`/`sm-chat-mode-flow` `flow` **TXT Flow**.
(Third chip label is **TXT Flow** — neutral; never a product/site name.)
Only the chosen block is visible:
- `sm-section-single-mode` — `sm-sequence` textarea 7 rows **Message sequence**,
  placeholder `hello how are you %username%` ⏎ `ok i am checking. wait a bit.`,
  hint *"One line = one step. `%username%`, `{hello|hi}`."*
- `sm-section-folder-mode` — `sm-load-folder` **Load Folder**,
  `sm-clean-loaded` **Clean Loaded** (danger), `sm-loaded-status`
  (`0 sequences loaded` → `{n} sequences loaded`). Load forces mode=folder;
  clean forces mode=single.
- `sm-section-flow-mode` — `sm-open-flow` **Open Flow Data Folder** +
  `sm-flow-status` (default "Flow data is initialized on first run.") + hint
  *"Pure text flow: the reply pools in this folder decide every line."*
  Optional AI-rewrite block (kept for every site, hidden if unsupported):
  checkbox `sm-ai-enabled` **AI rewrite (text rules decide the flow)** ·
  `sm-ai-base-url` **API base URL** · `sm-ai-model` **Model** ·
  `sm-ai-api-key` **API key (stored locally)** (`type="password"`) ·
  `sm-ai-test` **Test AI** + `sm-ai-status`.
- Always: **Snap / Telegram users** `sm-usernames` textarea 3 rows + hint
  *"`%username%` inserts one random handle."*
- **No Save button here.**

**`4 · Run Browser`** (`.subpage.narrow`, `.run-card`) —
`.mode-label` **Select Profile** + chips `sm-profile-default` `default`
(default checked) / `sm-profile-session-choice` `session`;
`sm-profile-session` hint (fresh / saved-session variants as in 7.3);
`sm-run` **▶ Run Browser** (`.primary-button.run-button`, full width);
`sm-run-status` default **"Idle."**; hint *"Saves Setting + Proxy + Message, then
opens a visible browser. Log in manually; live logs start automatically. Close
the browser to stop."*

**`5 · Live Logs`** — `.log-card` header `h2` **5 · Live Logs** + `sm-log-filter`
(All / SMS / Rules / Errors) + `sm-log-clear` **Clear**; `sm-log-list`;
`sm-log-empty` = **"No activity yet. Press Run Browser."**

### 5.4 API Mode page

Header: `h1` **API Mode** + badge **API worker**; actions `#api-start`
**Save & Start Bot** (primary), `#api-stop` **Stop** (disabled while idle),
`#api-refresh` **Refresh**.
Description: *"Match, DM and reply without a visible browser. A saved login and
message settings are required."*
`#api-summary` = `Backend: {browser.backend} · Profiles auto-start · Message
steps: {n}`; `#api-status` default **"Idle."**
Six counters: `api-stat-running` **Running** · `api-stat-profiles` **Profiles** ·
`api-stat-messages` **Messages sent** · `api-stat-traffic` **Traffic users** ·
`api-stat-snap` **snap/tg sent** · `api-stat-dead` **Dead accounts** (danger).

**`1 · Login Capture`** (`.subpage.narrow`) — hint *"Opens a fresh browser for a
manual login. The session is saved and the browser closes."*
`api-profile` **Profile** text (`list="api-profile-options"`, placeholder
`default`) + `<datalist id="api-profile-options">` from saved profiles
(label `{name} · session {date}` / `{name} · no session`);
`api-profile-session` hint (saved → "Saved session ({date}) — Run Browser will
log in automatically.", else "No saved session — the browser will wait for manual
login.");
`api-backend` **Browser mode** (Rod `rod` / ChromeDP `chromedp`);
`api-headless` checkbox **Headless (keep off for manual login)**;
`api-launch-login` **Launch for Login**;
`api-login-status` default **"No browser launched."**

**`2 · Proxy`** — identical to Show Mode with `api-` ids.

**`3 · Message`** — identical to Show Mode with `api-` ids (three chips, three
blocks, usernames); same saved record; no Save button.

**`4 · Bot Settings`** — first-message block with `api-` ids, then:

| id | label | min | default |
|---|---|---|---|
| `api-poll-min` / `api-poll-max` | Poll min (s) / Poll max (s) | 1 | 20 / 45 |
| `api-action-min` / `api-action-max` | Action min (s) / Action max (s) | 1 | 20 / 45 |
| `api-skip-min` / `api-skip-max` | Skip timeout min (s) / Skip timeout max (s) | 0 | 60 / 60 |
| `api-close-min` / `api-close-max` | Chat closed wait min (s) / Chat closed wait max (s) | 0 | 0 / 0 |
| `api-reply-min` / `api-reply-max` | Worker reply delay min (s) / Worker reply delay max (s) | 0 | 0 / 0 |

**`5 · Live Logs`** — same component, own cursor, `api-log-filter`,
`api-log-clear`, `api-log-list`, `api-log-empty` =
**"No activity yet. Press Save & Start Bot."**

### 5.5 Neutral text rules (grep-enforced)

Allowed strings only: `Chat Bot`, `CB`, `Desktop Engine`, `Show Mode`, `API Mode`,
`Setting`, `Proxy`, `Message`, `Run Browser`, `Live Logs`, `Login Capture`,
`Bot Settings`, `Workers`, `Adapter ready`, `Adapter not configured`,
`Not supported by the current adapter`, plus the fixed labels/status texts in
this spec. **Forbidden in shipped UI files:** any domain, `http://`, `https://`,
any site/brand name, any filesystem path, any account id.
A build-time test greps the bundle for `https?://` and for the blocklist defined
in `tests/ui-blocklist.json` and fails on a match.

## 6. PHASE 4 — ENGINE LOGIC (site-independent)

### 6.1 Show Mode run

1. Read + validate Setting / Message / Proxy → on error print in `sm-run-status`
   and abort.
2. `SaveShowModeSettings` → `SaveProxySettings` → `SaveMessageSettings` →
   `SaveBrowserSettings{profile, backend:"rod", headless:false, use_saved_session}`.
3. Clear the Show log feed; status "Restoring the saved profile session…" or
   "Launching a fresh browser (manual login)…".
4. `RunBrowser({profile, backend:"show_mode", headless:false, use_saved_session}, 1)`.
5. Status (ok) "Browser requested. Log in manually; live logs will appear here.
   Close the browser to stop."

Inside the run: `adapter.RestoreSession` (session profile) or
`adapter.LoginManual` (fresh) → `adapter.Match` → loop:
`adapter.Receive` → engine decides reply → `adapter.Send` after the configured
delay → on partner silence beyond the no-reply window, `adapter.Skip` when
`auto_skip` → wait `next_chat` → `adapter.Match` again. Closing the browser ends
the loop and flips the badge to Idle.

### 6.2 First-message policy (both pages, independent stores)

- `bot` — the bot opens the chat.
- `user` — wait for the partner's first message; `wait_timeout=true` means skip
  after `wait_min..wait_max` seconds, `false` means wait indefinitely.
- `random` — bot opens with probability `bot_first_percent`, otherwise behaves
  like `user`. The decision is stored per partner so it is stable inside a chat.

### 6.3 Chat modes

- **Single Sequence** — the textarea is a linear script; one line per bot turn;
  `{a|b}` alternatives, `%username%` random handle.
- **Folder Sequences** — a folder of `.txt` files, one sequence per file; the
  engine picks a file and walks it; `Clean Loaded` forgets them.
- **TXT Flow** — stateful engine over two text corpora
  (`data/flow/input/*.txt` triggers, `data/flow/output/*.txt` reply pools, same
  file name = same category). Stages: `NEW → WAIT_AGE_GENDER → WAIT_COUNTRY →
  MIDDLE → END`; every output pool fires once per partner except repeatable
  pools; reply cap from `config/site.json#limits.max_replies_per_chat` → closer →
  END; age < 18 revealed ⇒ never share contact, polite end.
- **AI rewrite (optional)** — when enabled and a key exists, the TXT flow decides
  *what* to say and the model only rewrites the wording; no key ⇒ never silently
  fall back to raw pool text without saying so in the log.

### 6.4 API Mode

`Save & Start Bot` saves browser + proxy + message + general settings, then
`StartBot()` launches one worker per saved profile. Each worker: match →
first-message policy → reply → skip → DM, emitting IN/OUT/SKIP/SYS/ERR. Counters
update every 3 s from `home`. `Stop` ends all workers.

### 6.5 Logs

Feed entry `{time, kind: in|out|skip|system|error, text, rule, detail, setting_ok}`.
Rendering: `time · badge(IN/OUT/SKIP/SYS/ERR) · text`, IN/OUT quoted, meta line
`rule · detail · ✓|✗` (✓/✗ only on OUT). Filters: `sms`=IN+OUT,
`rules`=OUT+SKIP+SYS, `errors`=ERR, `all`=everything. Poll `GetShowModeLog(after)` /
`GetBotLog(after)` every 1 s while the page is active + on events `ui:showlog` /
`ui:botlog`; cursor = `version`; skip a tick while pending; cap 600 rows;
autoscroll only within 60 px of the bottom. `Clear` empties the in-memory feed and
prints **"Live log cleared (logs.txt is kept)."** — the file is never truncated.
Log lines must stay neutral (no URLs, no site name, no tokens).

## 7. INTERACTION & LIFECYCLE

**7.1 Bootstrap** — bind nav → forms → store → actions → events → activate
Show Mode → Setting → `GetInitialState()` retried up to **50× at 200 ms**
before `.shell.visible`; then intervals **3 s** counters, **10 s** profiles,
**1 s** active-page logs. Total failure → `sm-run-status` =
**"Failed to connect to Wails bindings."**
**7.2 Tabs/sections** — only one tab panel and one subpanel visible; the other
sub-nav gets `hidden`; section switch restores that page's typed values and
refreshes logs when opening a logs section.
**7.3 Profiles hint** — fresh → "Fresh browser — manual login; the session is
saved for the session option."; session+saved → "Saved default session ({date}) —
Run Browser logs in automatically."; session without → "No saved default session
yet — manual login will save one for the session option."
**7.4 Running states** — `runningShow`, `runningBot`, `busy{show,api}`;
`#sm-run` disabled while running/busy; `#api-start` disabled while running/busy;
`#api-stop` disabled while idle; pill and badge follow; events `ui:running` /
`ui:show_running` update live.
**7.5 Capability gating** — when a capability is `false`: the control stays in
place, gets `disabled`, and its hint/status shows **"Not supported by the current
adapter"**; skip/DM-specific buttons behave the same. Never hide, never reorder.
**7.6 License gate (optional module)** — overlay with neutral copy
(`License Required`, reason, readonly key, **Copy**); shown on
`license_invalid` / `ui:license_invalid`; the overlay text may never contain a
site name.
**7.7 Errors** — `invoke()` wrapper logs `Frontend | {context} | {stack}` and
rethrows; `window.onerror` + `unhandledrejection` do the same; every action prints
its own status line.

## 8. VALIDATION, STORAGE, CONTRACT

### 8.1 Exact validation messages

| Rule | Message |
|---|---|
| policy ∉ bot/user/random | `Choose a first-message policy.` |
| percent not integer 0–100 | `Bot first chance must be a whole number from 0 to 100.` |
| first timeout not integers / min>max / >86400 | `First-message timeout: use whole seconds, 0 ≤ min ≤ max ≤ 86400.` |
| mode≠bot, wait=skip, min<1 | `A first-message timeout needs a positive minimum.` |
| timing pair outside `floor ≤ min ≤ max ≤ 86400` | `{Label}: use whole seconds with {floor} ≤ min ≤ max ≤ 86400.` |
| auto-skip on, no-reply min < 1 | `Auto skip needs a positive no-reply timeout.` |
| folder mode with 0 loaded | `Load a folder of sequences before using Folder Sequences mode.` |
| proxy test with empty list | `Enter at least one proxy to test.` |

Labels for the timing rule: Reply delay · No-reply timeout · Next-chat wait ·
Poll · Action · Skip timeout · Chat closed wait · Worker reply delay.

### 8.2 Storage (site-agnostic paths)

```
config/site.json            adapter + capabilities + limits (never rendered)
config/branding.json        mark / name / subtitle (neutral only)
settings/show_mode/v1       Show Mode timings
settings/general            API Mode timings + first-message policy
settings/message            chat_mode, sequence, usernames, ai config, loaded seqs
settings/proxy              enabled, entries
settings/browser            profile, backend, headless, use_saved_session
sites/<site_id>/profiles/   browser profiles                (git-ignored)
sites/<site_id>/sessions/   latest_session.json             (git-ignored)
data/flow/input|output/     text trigger pools / reply pools
logs.txt                    append-only history (UI Clear never truncates)
```

`<site_id>` appears **only** on disk and in the adapter — never in the UI.

### 8.3 RPC contract (same names for every site)

```
GetInitialState() -> UISnapshot
GetAdapterStatus() -> { configured:bool, ready:bool, session_saved_at:string,
                        capabilities:Capabilities }
ListProfiles() -> [{ name, has_session, saved_at }]
SaveShowModeSettings(ShowModeSettings) -> void
SaveGeneralSettings(GeneralSettings) -> void
SaveMessageSettings(MessageSettings) -> void
SaveProxySettings(ProxySettings) -> void
SaveBrowserSettings(BrowserSettings) -> void
RunBrowser(BrowserSettings, count:int) -> void
StartBot() -> void
StopBot() -> void
GetShowModeLog(after:uint) -> { version, entries[] }
GetBotLog(after:uint) -> { version, entries[] }
ClearShowModeLog() -> void
ClearBotLog() -> void
TestProxyEntries(rawLines) -> [{ line, ok, tunnel_ok, label, latency_ms, status,
                                detail, provider_message, proxy_authenticate, hint }]
LoadMessageFolder() -> [string]
ClearLoadedMessages() -> void
OpenFlowDataFolder() -> string
TestAIRewrite(MessageSettings) -> string
```

```jsonc
UISnapshot {
  "running": false, "show_mode_running": false,
  "adapter": { "configured": true, "ready": true, "session_saved_at": "",
               "capabilities": { "skip": true, "dms": false, "typing": true } },
  "home": { "bot_accounts":2, "running_accounts":0, "user_send_count":0,
            "dead_accounts":0, "traffic_users":0, "messages_sent":0,
            "snap_tg_sent_count":0, "skipped_accounts":0 },
  "browser": { "profile":"default","backend":"rod","headless":false,
               "use_saved_session":false },
  "message": { "chat_mode":"single","sequence":"…","usernames":"…",
               "loaded_sequences":[],"use_loaded_sequences":false,
               "ai_enabled":false,"ai_base_url":"","ai_model":"","ai_api_key":"" },
  "proxy":   { "enabled":false,"entries":"" },
  "general": { "first_message":{ "mode":"bot","bot_first_percent":70,
                 "wait_timeout":false,"wait_min_seconds":30,"wait_max_seconds":60 },
               "poll_min_seconds":20,"poll_max_seconds":45,
               "action_min_seconds":20,"action_max_seconds":45,
               "match_reply_timeout_min_seconds":60,"match_reply_timeout_max_seconds":60,
               "match_close_wait_min_seconds":0,"match_close_wait_max_seconds":0,
               "reply_delay_min_seconds":0,"reply_delay_max_seconds":0 },
  "show_mode": { "first_message":{ …same… },
                 "reply_delay_min_seconds":0,"reply_delay_max_seconds":0,
                 "auto_skip":false,"no_reply_min_seconds":60,"no_reply_max_seconds":60,
                 "next_chat_min_seconds":0,"next_chat_max_seconds":0 },
  "license_invalid": false, "license_reason": "", "license_key": ""
}
LogEntry { "time":"HH:MM:SS","kind":"in|out|skip|system|error",
           "text":"…","rule":"…","detail":"…","setting_ok":true }
```

## 9. PHASE 5 — VERIFICATION (must all pass)

1. `go vet ./...` and `go test ./...` (adapter contract tests use a **fake**
   adapter; never a real site).
2. **UI parity test** (Appendix B): golden id list present, UI bundle hash equals
   `tests/fixtures/ui-golden.sha256`, zero blocklisted strings, zero `http(s)://`
   in `frontend/dist/**`.
3. **Adapter matrix test**: for every capability `false`, the matching control is
   `disabled` and shows the neutral reason — verified by a DOM test, not by eye.
4. **State test**: switch every tab/section, type in every field, switch back —
   values intact; hidden rows are disabled.
5. **Log test**: inject 600+ fake entries → cap respected, filters correct,
   no duplicates, Clear empties the feed and leaves `logs.txt` untouched.
6. **Error test**: make every RPC reject once → each status line turns
   `.status-error`, app never shows success.
7. **No-site-name audit**: `npm run test:ui-parity` plus a manual grep of the
   built bundle for the target site's name/domain — must be 0 hits.
8. Report **PASS / FAIL / BLOCKED / NOT_RUN** per item, with commands and output.
   Never call something PASS that you did not run.

## 10. DELIVERABLES

1. The app: frozen UI (sections 5–7), engine (6), adapter interface (4) and one
   concrete adapter for the requested site.
2. `docs/sites/<site_id>.md` (discovery record) — internal only.
3. `docs/UI-CONTRACT.md` — this dashboard spec, so the next agent never needs to
   re-derive it.
4. `docs/ADAPTER.md` — how to add a site in one folder, with a worked skeleton.
5. `tests/ui-parity.test.*` + `tests/fixtures/ui-golden.sha256` +
   `tests/ui-blocklist.json`.
6. `README.md` — build/run, where settings and sessions live, how to switch sites,
   how to verify without a real login.
7. A verification report (Appendix C template).

## 11. FORBIDDEN

- Any site name, domain, URL, logo or "Target Site" label in the UI or logs.
- A site picker, search, filter pills, site tables, "new site" form.
- Removing/renaming sections, fields, ids, counters, status texts or filters.
- Hiding a control because the site lacks the capability (disable it instead).
- A 4th chat mode, or merging the three modes.
- Save buttons in Setting / Proxy / Message / Bot Settings.
- Site-specific code outside `adapters/<site_id>/` + `config/site.json`.
- Mock data, fake progress, optimistic badges, silent `catch {}`.
- Keys/tokens/cookies in the DOM, localStorage, logs or git.
- Guessing a selector/endpoint instead of asking or marking `TODO(site)`.

## 12. NEXT TIME — "new site" runbook for the agent

1. Read `THIS BUILD` lines + `docs/UI-CONTRACT.md` (frozen) + `docs/ADAPTER.md`.
2. Run Discovery (section 3) → write `docs/sites/<site_id>.md` →
   `config/site.json` with capabilities.
3. Implement `adapters/<site_id>/` to satisfy the interface.
4. Run the adapter contract test with a fake server/fixture; fix until green.
5. Run the **full UI parity test** — it must pass **without touching** the UI.
   If it fails, revert the UI change instead of updating the golden hash.
6. Manual smoke: Show Mode with a test profile, then API Mode with one profile;
   confirm IN/OUT/SKIP events and counters, then stop.
7. Report PASS/FAIL/BLOCKED per section 9 and hand back.

# PART A ENDS HERE

---

# PART B — SHORT VERSION (small context window)

> Part A is authoritative; send it when possible.

Build/extend a **site-neutral** chat-bot dashboard. Rule #1: the UI never names a
site — only neutral branding (`Chat Bot` / `CB` / `Desktop Engine`) and neutral
state words. Everything site-specific lives in `adapters/<site_id>/` +
`config/site.json` (capabilities + limits); never in the UI, engine or cmd.

**Workflow:** (1) Discovery — read the provided brief/capture, learn login,
session, match, incoming/outgoing transports, skip, DMs, rate limits, errors;
ask ≤10 questions with defaults if missing; write `docs/sites/<site_id>.md` +
`config/site.json`. (2) Implement the adapter interface
(`LoginManual/RestoreSession/SessionValid/Match/Receive/Send/Typing/Skip/Close/DM/Report`,
`ErrUnsupported` for gaps). (3) UI is frozen — build it from the spec below
verbatim. (4) Verify with the parity test; the UI bundle hash must not change
between sites.

**Shell:** 238px sidebar `#191b17`: brand `CB` + **Chat Bot** + **Desktop
Engine** → tabs **Show Mode** / **API Mode** → 5 sub-items of the active tab →
`Adapter ready|Adapter not configured · session saved {date}` → `Workers` +
pill. Cards numbered `1 · … 5 · …`. Tokens: bg `#161716`, panel `#20221e`,
alt `#282b25`, line `#3e4439`, text `#eef2e8`, muted `#b5beb0`, accent `#83cb73`,
warn `#d9b74d`, danger `#d36a6a`; card radius 14/padding 18/gap 11; chips radius 8;
log box `#191b17`; every action has a status line (neutral/ok/error).

**Show Mode** (badge Idle→Running, Refresh; description "Configure, press Run
Browser, log in once. The visible browser chats with these rules; close it to
stop."; "Idle."):
1 Setting — first-message policy bot/user/random (+chance % 70), wait
unlimited/skip (+timeouts 30/60), reply delay 0/0, auto-skip checkbox,
no-reply 60/60, next-chat 0/0; hidden rows disabled.
2 Proxy — enable, HTTP/SOCKS5, lines textarea, Test Proxy, "Not tested.", per-line
PASS / REACHABLE-SITE RESTRICTED / FAIL + `{passed}/{total}` summary.
3 Message — **Chat mode** + exactly 3 chips: **Single Sequence** (textarea,
`%username%`, `{a|b}`), **Folder Sequences** (Load Folder / Clean Loaded /
"n sequences loaded"; 0 → cannot save), **TXT Flow** (Open Flow Data Folder +
optional AI rewrite: enable, base URL, model, key, Test AI); always Snap/Telegram
users + hint; no Save button.
4 Run Browser — Select Profile default/session + live session hint, full-width
**▶ Run Browser**, "Idle.", footer hint; one click saves 4 stores, clears the log,
launches, then "Browser requested. Log in manually; live logs will appear here.
Close the browser to stop."
5 Live Logs — All/SMS/Rules/Errors + Clear, rows `time · IN/OUT/SKIP/SYS/ERR ·
text · rule/detail/✓✗`, 1 s cursor polling, 600 rows, autoscroll near bottom,
empty "No activity yet. Press Run Browser."

**API Mode** (badge API worker; Save & Start Bot / Stop / Refresh; description
"Match, DM and reply without a visible browser. A saved login and message settings
are required."; summary "Backend: rod · Profiles auto-start · Message steps: n";
"Idle."; counters Running / Profiles / Messages sent / Traffic users / snap/tg
sent / Dead accounts(red)):
1 Login Capture — Profile (+ saved-profile datalist + session hint), Browser mode
Rod/ChromeDP, Headless (off for manual login), Launch for Login, "No browser
launched." → "Launching {backend} for login…" → success line.
2 Proxy — same as Show Mode.
3 Message — same 3 chips/blocks with own ids, same saved record, no Save button.
4 Bot Settings — first-message block + Poll 20/45, Action 20/45, Skip 60/60,
Chat closed 0/0, Worker reply delay 0/0.
5 Live Logs — same component, own cursor, empty "No activity yet. Press Save &
Start Bot."
Save & Start = save browser+proxy+message+general → "Starting workers…" →
StartBot → "Started. One worker runs per saved profile."; Stop → "Stopped."

**Logic:** first-message policy decides who opens (bot / wait-user / random with
stored per-partner decision); reply delay random in range; auto-skip after the
no-reply window; next-chat wait between partners; three chat modes as above
(TXT Flow = stateful pools with once-per-partner non-repeatable outputs, reply
cap, age<18 ⇒ no contact sharing); AI rewrite only rewrites wording and never
silently replaces the flow; capabilities `false` ⇒ control disabled with
"Not supported by the current adapter" (never hidden).

**RPC:** GetInitialState, GetAdapterStatus, ListProfiles, SaveShowModeSettings,
SaveGeneralSettings, SaveMessageSettings, SaveProxySettings, SaveBrowserSettings,
RunBrowser, StartBot, StopBot, GetShowModeLog, GetBotLog, ClearShowModeLog,
ClearBotLog, TestProxyEntries, LoadMessageFolder, ClearLoadedMessages,
OpenFlowDataFolder, TestAIRewrite — with the JSON shapes from Part A §8.3.

**Verify:** go vet/test (fake adapter only), UI parity test (golden ids + bundle
hash + blocklist + zero `http(s)://` in dist), capability-disable test, state
persistence test, log cap/filter test, error-path test, no-site-name audit.
Deliver UI-CONTRACT.md, ADAPTER.md, docs/sites/<site_id>.md, tests, README and a
PASS/FAIL/BLOCKED report.

---

# APPENDIX A — capability → UI effect matrix

| Capability | `true` | `false` |
|---|---|---|
| `manual_login` | Login Capture usable | Launch for Login disabled + neutral reason |
| `skip` | Skip buttons active, auto-skip honoured | auto-skip checkbox disabled + reason |
| `dms` | DM path in workers | DM-related statuses read "not supported" |
| `typing` | typing indicator sent | silently skipped, logged as `SYS` once |
| `ws_incoming` / `poll_incoming` / `dom_incoming` | engine uses it (priority: WS → poll → DOM) | falls to the next available; all false ⇒ adapter not ready |
| `dom_send` / `api_send` | send path chosen accordingly | all false ⇒ adapter not ready |
| `guest_login` | auto account creation possible | profile-based manual login only |
| `report` | report action available | hidden from logs (never a UI control change) |

Layout never changes: same cards, same ids, same order.

# APPENDIX B — parity test (drop in as `tests/ui-parity.test.cjs`)

```js
// Fails if the frozen UI drifts or a site name leaks into the bundle.
const fs = require("node:fs");
const path = require("node:path");
const crypto = require("node:crypto");

const DIST = "frontend/dist";
const files = ["index.html", "app.js", "styles.css"];
const GOLDEN = JSON.parse(fs.readFileSync("tests/fixtures/ui-golden.json", "utf8"));

const hash = (p) => crypto.createHash("sha256").update(fs.readFileSync(p)).digest("hex");

// 1) byte-identical UI bundle
for (const f of files) {
  const p = path.join(DIST, f);
  if (hash(p) !== GOLDEN[f]) {
    console.error(`UI DRIFT: ${p} changed. The dashboard must stay identical across sites.`);
    process.exit(1);
  }
}

// 2) every frozen id exists
const html = fs.readFileSync(path.join(DIST, "index.html"), "utf8");
const missing = GOLDEN.ids.filter((id) => !html.includes(`id="${id}"`));
if (missing.length) {
  console.error("MISSING IDS:", missing);
  process.exit(1);
}

// 3) no site names / urls / paths in the bundle
const bundle = files.map((f) => fs.readFileSync(path.join(DIST, f), "utf8")).join("\n");
const blocklist = JSON.parse(fs.readFileSync("tests/ui-blocklist.json", "utf8"));
const hits = [];
if (/https?:\/\//i.test(bundle)) hits.push("absolute URL in bundle");
for (const word of blocklist) {
  if (new RegExp(word, "i").test(bundle)) hits.push(`site name: ${word}`);
}
if (hits.length) {
  console.error("BUNDLE LEAK:", hits.join(", "));
  process.exit(1);
}

console.log("UI parity OK");
```

`tests/fixtures/ui-golden.json`:
```json
{ "index.html": "<sha256>", "app.js": "<sha256>", "styles.css": "<sha256>",
  "ids": ["sm-first-mode", "sm-first-wait", "sm-first-percent", "sm-first-min",
          "sm-first-max", "sm-reply-min", "sm-reply-max", "sm-auto-skip",
          "sm-noreply-min", "sm-noreply-max", "sm-next-min", "sm-next-max",
          "sm-proxy-enabled", "sm-proxy-scheme", "sm-proxy-entries",
          "sm-test-proxy", "sm-proxy-status", "sm-proxy-results",
          "sm-chat-mode-single", "sm-chat-mode-folder", "sm-chat-mode-flow",
          "sm-mode-option-single", "sm-mode-option-folder", "sm-mode-option-flow",
          "sm-section-single-mode", "sm-section-folder-mode", "sm-section-flow-mode",
          "sm-sequence", "sm-load-folder", "sm-clean-loaded", "sm-loaded-status",
          "sm-open-flow", "sm-flow-status", "sm-ai-enabled", "sm-ai-base-url",
          "sm-ai-model", "sm-ai-api-key", "sm-ai-test", "sm-ai-status",
          "sm-usernames", "sm-profile-default", "sm-profile-session-choice",
          "sm-profile-session", "sm-run", "sm-run-status",
          "sm-log-filter", "sm-log-clear", "sm-log-list", "sm-log-empty",
          "api-start", "api-stop", "api-refresh", "api-summary", "api-status",
          "api-stat-running", "api-stat-profiles", "api-stat-messages",
          "api-stat-traffic", "api-stat-snap", "api-stat-dead",
          "api-profile", "api-profile-options", "api-profile-session",
          "api-backend", "api-headless", "api-launch-login", "api-login-status",
          "api-proxy-enabled", "api-proxy-scheme", "api-proxy-entries",
          "api-test-proxy", "api-proxy-status", "api-proxy-results",
          "api-sequence", "api-load-folder", "api-clean-loaded",
          "api-loaded-status", "api-open-flow", "api-flow-status",
          "api-chat-mode-single", "api-chat-mode-folder", "api-chat-mode-flow",
          "api-mode-option-single", "api-mode-option-folder", "api-mode-option-flow",
          "api-section-single-mode", "api-section-folder-mode", "api-section-flow-mode",
          "api-ai-enabled", "api-ai-base-url", "api-ai-model", "api-ai-api-key",
          "api-ai-test", "api-ai-status",
          "api-usernames", "api-first-mode", "api-first-percent",
          "api-first-wait", "api-first-min", "api-first-max", "api-poll-min",
          "api-poll-max", "api-action-min", "api-action-max", "api-skip-min",
          "api-skip-max", "api-close-min", "api-close-max", "api-reply-min",
          "api-reply-max", "api-log-filter", "api-log-clear", "api-log-list",
          "api-log-empty"] }
```

`tests/ui-blocklist.json` — the only file allowed to know site names:
```json
["<<site-b-name>>", "<<site-c-name>>", "<<their-domains>>"]
```
(Keep it in `tests/`, never import it from the UI bundle.)

# APPENDIX C — verification report template

```
| # | Check | Status | Command | Evidence |
|---|-------|--------|---------|----------|
| 1 | go vet ./... | | | |
| 2 | go test ./... (fake adapter) | | | |
| 3 | UI parity (hash + ids + blocklist + no URL) | | | |
| 4 | Capability disable matrix | | | |
| 5 | Typed-value persistence | | | |
| 6 | Log cap/filter/clear | | | |
| 7 | Error paths (all RPCs reject) | | | |
| 8 | No site name/domain in bundle | | | |
| 9 | Show Mode smoke (test profile) | | | |
| 10 | API Mode smoke (1 profile) | | | |
```
Allowed statuses: **PASS / FAIL / BLOCKED / NOT_RUN**. Blocked is not pass.
