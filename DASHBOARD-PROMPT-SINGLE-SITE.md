# MASTER PROMPT — "ChitChat Bot" **Single-Site** Dashboard (A → Z)

> **এই dashboard-টা কোনটা (verified):**
> আপনার স্ক্রিনশটগুলোর build = **single-site ChitChat Bot dashboard**
> (`Wails + Badger`), দুইটা tab — **Show Mode** + **API Mode**, প্রতিটায় ৫টা
> section, Message-এ ঠিক ৩টা chat mode। কোনো site selector নেই।
>
> পুরোনো প্রজেক্টের zip (`ChitChat-App-clean-fixed-2026-10-20`) খুঁজে দেখলাম —
> ওটা এর **পরের** ভার্সন: ওখানে `API Mode` tab-টার নাম বদলে **Background Bot**
> হয়েছে, উপরে multi-site selector (100+ sites, filter, per-site table) যোগ
> হয়েছে, Message-এ ৪র্থ mode **API Chat** যোগ হয়েছে, আর auto-register/accounts
> টেবিল এসেছে। তাই ওই zip থেকে হুবহু কপি করলে আপনার চাওয়া dashboard হবে না।
>
> আপনার স্ক্রিনশটের সাথে **সবচেয়ে কাছের surviving source** পেয়েছি:
> repo `rajuvbygyuiythh/dfdgfhghg45454` → `cmd/chitchatui/frontend/dist/`
> (commit `ff502bc3`, 2026-10-19) — একই দুই-পেজ single-site layout, একই
> `Wails + Badger` brand, একই ৫+৫ section, ৩টা chat mode, `Login Capture`,
> `Bot Settings`, counters, `Live Logs` — সব হুবহু। ওই commit-এ tab-এর লেখা
> `Background Bot` (screenshot-এ `API Mode`), id `data-tab="bot"`, আর সব API-pane
> field-এর prefix `bg-`। নিচের prompt-এ আমি tab-এর লেখা **API Mode** রেখেছি
> (আপনার স্ক্রিনশট অনুযায়ী) এবং id prefix **`api-`** করেছি; mapping table
> শেষে দেওয়া আছে (`bg-` ↔ `api-`)।
>
> **অর্থাৎ:** আগের বারের মতো "ধরে নেওয়া" নয় — **Login Capture** আর
> **Message** সেকশন দুটো এখন ওই আসল কোড থেকেই লেখা (Section 6.3)।
>
> **কীভাবে দেবেন:** PART A পুরোটা কপি করে নতুন সাইটের তথ্য (Section 3) ভরে
> agent-কে দিন। ছোট context হলে PART B। PART A-ই authoritative।

---

# PART A — THE PROMPT (copy everything below)

## 0. PROVENANCE / SCOPE

Recreate, 1:1, a **single-site Windows-desktop-style chat-automation dashboard**
of this exact shape:

- One fixed target chat site (no site picker of any kind).
- Two pages (tabs): **Show Mode** and **API Mode**.
- Each page has exactly five sections, navigated by a nested sidebar.
- Show Mode = visible browser + manual login + live chat inside that browser.
- API Mode = headless background workers (one per saved profile) + counters + logs.

Only the site identity changes (URLs, selectors, auth, entry flow). Everything
else — screens, labels, ids, defaults, validations, status texts, polling,
storage keys — must match this specification.

## 1. WHAT THE PRODUCT IS

A control panel for automating conversations on one chat site:

- **Show Mode** — press **Run Browser**; a *visible* Rod browser opens at the
  site; the operator logs in once; the bot then chats inside that browser using
  the configured Setting + Message rules; every IN/OUT/SKIP event streams into
  **Live Logs**. Closing the browser stops the run.
- **API Mode** — press **Save & Start Bot**; the backend starts one headless
  worker per saved profile/account. Workers match strangers, send the first
  message, reply, skip, DM, and write IN/OUT/SKIP/SYS/ERR events to their own
  Live Logs. Six counters show live state.

Both pages share proxy config, message content and chat-mode choice, but have
independent timing settings and independent log feeds.

## 2. NON-NEGOTIABLE RULES

1. **Single site.** No site dropdown / search / filter pills / site list /
   "New site" form / per-site table. No `ListSites`, `SelectSite`, `CreateSite`,
   `RunSiteBrowser`, `RunSiteBotWithSession` in the contract.
2. **Exactly two tabs**, labelled **Show Mode** and **API Mode**.
3. **Exactly three chat modes** — `single` **Single Sequence**,
   `folder` **Folder Sequences**, `eva_chat_flow` **EVAChat Flow**. Never a
   4th ("API Chat", "Keyword Based").
4. **Nested navigation.** Sidebar selects the page; a second list selects the
   section; only that section renders in the main area.
5. **Save-on-run only.** No Save button in Setting / Proxy / Message /
   Bot Settings. Show Mode's Run Browser and API Mode's Save & Start Bot persist
   everything first.
6. **No fake behaviour** — every action writes a real backend result into its
   own status line; failures show `.status-error`, never success.
7. **No site values hard-coded in the front end** — name, start URL, auth mode,
   folder path come from the backend site config.
8. **Typed values survive navigation** — a global form store keeps
   `show` and `bot` slices; switching tab/section never loses input.
9. **Idle / busy / running** must be visually distinct and enforced on buttons.
10. **No secrets in the UI layer** — the AI API key is a `type="password"`
    field stored only in local `data/`; never logged, never shown in the log feed.
11. **Every id in section 11 is part of the contract** (automation and future
    agents depend on them).

## 3. SINGLE SITE PROFILE (fill before you start)

| Key | Value |
|---|---|
| Display name | `<<e.g. ChitChat.gg>>` |
| Product name (sidebar) | `<<e.g. ChitChat Bot>>` |
| Initials for the brand mark | `<<e.g. CB>>` |
| Tech subtitle | `<<e.g. Wails + Badger>>` |
| Fixed site id / folder slug | `<<lowercase, e.g. chitchat>>` |
| Start URL | `<<https://app.chitchat.gg/text>>` |
| Login / entry flow | `<<what the operator must click after login>>` |
| Account creation | `<<auto via API / manual first browser run>>` |
| Message input selector | `<<CSS>>` |
| Incoming message selector | `<<CSS>>` |
| Send button selector | `<<CSS>>` |
| Skip / Next selector | `<<CSS>>` |
| Login detection | `<<cookie / localStorage / JWT / URL>>` |
| Session store | `sites/<site_id>/sessions/latest_session.json` |
| AI endpoint (optional) | `<<OpenAI-compatible base URL>>` |

Unknown → keep `TODO(site)` in a config module. Do not invent URLs/selectors.

## 4. STACK & FILE LAYOUT

Original: **Go + Wails v2** + vanilla DOM front end, no framework, no build step:

```
cmd/chitchatui/frontend/dist/index.html   all markup
cmd/chitchatui/frontend/dist/app.js       all behaviour (~1000 lines)
cmd/chitchatui/frontend/dist/styles.css   all styling (~1400 lines)
cmd/chitchatui/frontend/wailsjs/…         generated bindings
```

Acceptable alternatives: Electron/Tauri, or a local web app (same three files +
a local backend). In every case expose **one adapter**:

```js
function backend() { return window.go && window.go.main && window.go.main.App; }
async function invoke(method, ...args) {
  try {
    const api = backend();
    if (!api || typeof api[method] !== "function") throw new Error(`Wails binding not ready: ${method}`);
    return await api[method](...args);
  } catch (error) { reportFrontendError(`Backend ${method} failed`, error); throw error; }
}
```

For a web build map the same names to `POST /api/<FunctionName>` with identical
argument order and JSON shapes. Never redesign the contract.

## 5. LAYOUT & DESIGN SYSTEM (exact)

```css
:root{
  color-scheme: dark;
  --bg:#161716; --panel:#20221e; --panel-alt:#282b25; --panel-strong:#30342d;
  --line:#3e4439; --text:#eef2e8; --muted:#b5beb0; --accent:#83cb73;
  --accent-strong:#6bb25b; --warn:#d9b74d; --danger:#d36a6a;
}
```

- Font `"Segoe UI", Arial, sans-serif`; `15px`; line-height `1.45`.
- `.shell{display:none}` + `.shell.visible{display:flex}` — the shell is revealed
  only after the first successful state load.
- `.sidebar`: sticky top, `flex:0 0 238px`, full height, `gap:22px`,
  `padding:22px 16px`, `background:#191b17`, right border `1px solid var(--line)`.
- `.brand`: 40px `.brand-mark` = `border-radius:9px; background:var(--accent)` with
  dark bold initials; `.brand-title` 18px/700; `.brand-subtitle` 12px muted.
- `.nav`: vertical grid, `gap:6px`. `.nav-button`: full width, `11px 14px`,
  1px transparent border, radius 8, transparent background, 14px text.
  Hover → `background:var(--panel)`. `.active` →
  `background:rgba(131,203,115,.10); border-color:rgba(131,203,115,.45); color:var(--accent)`.
- `.subnav`: column, `gap:4px`, `padding:6px 0 6px 10px`,
  `border-left:1px solid var(--line)`; `[hidden]{display:none}`.
  `.subnav-button`: `7px 10px`, radius 7, transparent, 12.5px muted, left-aligned.
  Hover → `rgba(255,255,255,.04)` + text. `.active` →
  `border-color:rgba(131,203,115,.28); background:rgba(131,203,115,.10); color:var(--accent)`.
- Sidebar footer pinned bottom (`margin-top:auto`): `.status-row` with muted
  label **Workers** + `.pill` (`border-radius:999px; 4px 10px; 12px/700`).
  `.pill.running{background:rgba(131,203,115,.18);color:var(--accent)}`,
  `.pill.stopped{background:rgba(211,106,106,.18);color:#efb2b2}`.
- `.panel-header`: flex row, `justify-content:space-between`, `gap:18px`,
  `margin-bottom:18px`; `h1` 26px/700 (letter-spacing −0.2px, margin-bottom 6px);
  description `max-width:720px; color:var(--muted); font-size:14px`.
- `.title-row`: inline flex, `gap:12px`, wraps; holds `h1` + `.mode-badge`.
  `.mode-badge`: small pill, muted/panel-strong background, 12px 700;
  `.mode-badge.live` → `background:rgba(124,192,210,.13); color:#7cc0d2`.
- `.subpanel{display:none}` / `.subpanel.active{display:block}`;
  `.subpage{max-width:1100px}` / `.subpage.narrow{max-width:760px}`.
- `.card`: flex column, `gap:11px`, `padding:18px`, `border:1px solid var(--line)`,
  `border-radius:14px`, `background:var(--panel)`, `min-width:0`.
  Card headings `h2`: 17px bold, numbered `{n} · {Name}`.
- `.form-grid`: 2-col grid `gap:14px`; `.two-column` = 2 equal cols,
  `.span-2`/`.card-wide` spans both. Inputs full width.
- Inputs/selects/textareas: `background:var(--panel-alt)`, `1px solid var(--line)`,
  radius 8, `9px 11px`; focus border → `var(--accent)`. Labels: muted 12.5px,
  field below. `.hint-text`: muted 13px (with `<code>` chips).
- Buttons base: `1px solid var(--line)`, radius 8, `9px 14px`, 13.5px/600.
  `.primary-button` = accent fill + `#162010` text; `.secondary-button` =
  `var(--panel-alt)`; `.danger-button` = transparent + `#efb2b2` text +
  `rgba(211,106,106,.55)` border. `.run-button{width:100%;padding:12px 16px;font-size:14.5px}`.
- `.status-line`: `color:var(--muted); font-size:13px`. `.status-ok{color:var(--accent)}`,
  `.status-error{color:var(--danger)}`. All carry
  `role="status" aria-live="polite"`.
- `.home-stats`: 6 equal columns, `gap:10px`, `margin-bottom:12px`.
  `.stat-capsule`: `padding:11px 14px`, `border:1px solid rgba(131,203,115,.35)`,
  `background:rgba(131,203,115,.06)`, `border-radius:999px`; label `span` muted
  12px, value `strong` accent 17px. `.danger` variant →
  `border-color:rgba(211,106,106,.4); background:rgba(211,106,106,.12)` and red value.
- `.chat-mode-card`: grid `gap:9px`, `padding:12px 14px`,
  `border:1px solid var(--line)`, radius 10, `background:#23261f`.
  `.mode-label`: 11.5px/700, `letter-spacing:.5px`, **uppercase**, muted.
  `.mode-options`: flex wrap, `gap:8px`. `.mode-option`: inline-flex, `gap:8px`,
  `padding:8px 12px`, 1px line border, radius 8,
  `background:rgba(255,255,255,.02)`, 13.5px, pointer, user-select none;
  `input{width:auto;margin:0;accent-color:var(--accent)}`.
  `.mode-option.active` → accent border + accent tint + accent text.
- `.log-list`: `overflow:auto`, `min-height:280px`, `padding:6px 12px`,
  1px line border, radius 10, `background:#191b17`; in `.log-card` it flexes to
  fill the available height. `.log-row`: grid `60px 44px minmax(0,1fr)`,
  `gap:8px`, `padding:7px 0`, bottom border `rgba(62,68,57,.45)` (none on last).
  `.log-time`: 12px muted. `.log-badge`: 10px/700 centered, radius 5,
  `background:var(--panel-strong)`; per-kind colours:
  `in`/`out` accent-tinted, `skip` warn-tinted, `system` blue-grey
  (`#7cc0d2`-ish), `error` danger-tinted. `.log-body` text 13px,
  `.log-meta` 11.5px muted.
- Scrollbars dark/thin; app must be usable from 1280×800 upward.

## 6. SCREENS

### 6.1 Sidebar (+ license overlay)

Exact order, top → bottom:

```
BRAND      [CB]  ChitChat Bot
                 Wails + Badger
NAV        Show Mode        (data-tab="show", default active)
           API Mode         (data-tab="api")
SUB-SHOW   Setting · Proxy · Message · Run Browser · Live Logs
SUB-API    Login Capture · Proxy · Message · Bot Settings · Live Logs
FOOTER     Workers                       [Stopped]
```

- Sub-nav ids: `subnav-show`, `subnav-api`; only the active page's sub-nav is
  visible (the other has `hidden`). Default sub-section per page: Show → Setting,
  API → Login Capture.
- Sub buttons carry `data-subpanel` values:
  `show-setting|show-proxy|show-message|show-run|show-logs` and
  `api-login|api-proxy|api-message|api-settings|api-logs`.
- **Nothing else in the sidebar** — no site selector, search, filters, site list,
  debug button, folder button, or "new site" form.

**License overlay** (part of the original app — keep it):
`#license-overlay.license-overlay` fixed full-screen `background:var(--bg)`,
`display:none` by default; contains `.license-card` with `h2` **License Required**,
`#license-message` (default *"Your license is invalid or expired."*),
`#license-key-input` (readonly text) and `#copy-license-btn` **Copy**
(copies the key, then button text becomes "Copied" briefly).
Shown when `snapshot.license_invalid` is true (message =
`license_reason || "Your license is invalid or expired."`, key =
`license_key || "UNKNOWN"`) and also on the runtime event `ui:license_invalid`
(`{reason, key}`); when shown, stop filling the rest of the form.

### 6.2 Show Mode page

Header: `h1` **Show Mode** + badge `#show-run-badge` (`Idle` muted / `Running`
accent with `.live`), right button `#show-refresh` **Refresh**.
Description: *"Configure, press Run Browser, log in once. The visible browser
chats with these rules; close it to stop."*
Refresh → `GetInitialState` → refill everything → `sm-run-status` = "Refreshed."

#### 6.2.1 Section `1 · Setting`
Card `h2` `1 · Setting`; hint above the grid:
*"Run Browser applies these values. No separate save."*

Row 1 (two columns): **First-message policy** (`sm-first-mode` select: Bot first
`bot` / User first `user` / Random `random`) and **Wait for first message**
(`sm-first-wait` select: Wait indefinitely `unlimited` / Skip after timeout
`skip`).
Conditional rows:
- `#sm-first-percent-row` (hidden unless mode = random) → **Bot first chance (%)**
  `sm-first-percent` number 0–100 step 1, value 70.
- `#sm-first-wait-row` hidden when mode = bot.
- `#sm-first-timeout-row` (two columns, spans card; visible only when mode ≠ bot
  **and** wait = skip) → **First timeout min (s)** `sm-first-min` and
  **First timeout max (s)** `sm-first-max`, 1–86400, defaults 30 / 60.
Hidden rows must also be **disabled** (`updateFirst(prefix)` does both).

Then: **Reply delay min (s)** `sm-reply-min` + **Reply delay max (s)**
`sm-reply-max` (0–86400, default 0/0); checkbox row
`sm-auto-skip` **Auto-skip when the partner stops replying**;
**No-reply timeout min (s)** `sm-noreply-min` + **max (s)** `sm-noreply-max`
(default 60/60); **Next chat wait min (s)** `sm-next-min` + **max (s)**
`sm-next-max` (default 0/0).

#### 6.2.2 Section `2 · Proxy`
Card `h2` `2 · Proxy`, `.subpage.narrow`:

1. checkbox `sm-proxy-enabled` **Enable proxy**
2. **Default protocol for plain lines** → `sm-proxy-scheme` (`HTTP` `http` /
   `SOCKS5` `socks5`)
3. **Proxy lines** → `sm-proxy-entries` textarea, 5 rows, placeholder
   `1.2.3.4:8080` / `1.2.3.5:8080:user:pass`
4. button `sm-test-proxy` **Test Proxy**
5. `sm-proxy-status` — default **"Not tested."**
6. `sm-proxy-results` (empty div)

Test behaviour: if scheme = socks5, prefix every plain line with `socks5://`
before sending. During the call: button disabled + "Testing…", status
"Testing HTTPS access through each line (four at a time)…". For each returned row
render a `<p>` with class `.proxy-test-pass` / `.proxy-test-warn` /
`.proxy-test-fail`:
`Line {line}: PASS | REACHABLE / SITE RESTRICTED | FAIL — {label} — {latency_ms} ms[ — HTTP {status}] — {detail}`
plus optional new lines `Provider said: "{provider_message}"`,
`Auth challenge: {proxy_authenticate}`, `Next step: {hint}`.
Summary (`status-ok` only when all pass):
`{passed}/{total} passed; {reachable} HTTPS tunnel(s) verified. A test is not a browser-login guarantee.`
If the textarea changed during the test, append
` List edited: these results are for the previous values; test again.`
Empty input → error **"Enter at least one proxy to test."**

#### 6.2.3 Section `3 · Message` (+ optional AI hybrid block)
Card `h2` `3 · Message`:
1. `.chat-mode-card` → `.mode-label` **Chat mode** (rendered uppercase) +
   three `.mode-option` chips, `name="sm-chat-mode"`:
   `sm-mode-option-single` / `sm-chat-mode-single` value `single` → **Single Sequence**;
   `sm-mode-option-folder` / `sm-chat-mode-folder` value `folder` → **Folder Sequences**;
   `sm-mode-option-evachat` / `sm-chat-mode-evachat` value `eva_chat_flow` → **EVAChat Flow**.
   Exactly one chip `.active`; exactly one block visible (`setChatMode`).
2. `#sm-section-single-mode` — **Message sequence** `sm-sequence` textarea 7 rows,
   placeholder
   `hello how are you %username%` ⏎ `ok i am checking. wait a bit.`,
   hint *"One line = one step. `%username%`, `{hello|hi}`."*
3. `#sm-section-folder-mode` (`display:none` by default) — inline actions:
   `sm-load-folder` **Load Folder**, `sm-clean-loaded` **Clean Loaded** (danger),
   `sm-loaded-status` = `0 sequences loaded` (text becomes
   `{n} sequences loaded`).
   Load → `LoadMessageFolder()` returns a list → refresh state → force mode
   folder → status `{n} sequences loaded.` (ok). Zero results → keep status as is.
   Clean → `ClearLoadedMessages()` → refresh → force mode single →
   `Loaded sequences cleared.` (ok).
4. `#sm-section-evachat-mode` (`display:none`) — inline: `sm-open-eva`
   **Open EVAChat Data Folder**, `sm-eva-status` (default
   "TXT data is initialized on first run."). Opens the folder via
   `OpenEVAChatDataFolder()` and prints the returned summary.
   **Optional AI-hybrid sub-block (present in the original build — include it):**
   - checkbox row `sm-ai-enabled`, label
     **AI reply text (hybrid)** — *"TXT rules decide the flow; AI rewrites
     wording (background bot only)."*
   - two-column grid: **API base URL** `sm-ai-base-url`
     (placeholder `https://api.openai.com/v1`), **Model** `sm-ai-model`
     (placeholder `gpt-4o-mini`), full-width **API key (local data/ only)**
     `sm-ai-api-key` (`type="password"`, placeholder `sk-...`,
     `autocomplete="off"`).
   - inline: `sm-ai-test` **Test AI** + `sm-ai-status`.
     Test → `TestEVAChatAI(payload)` → success shows the returned text
     ("AI OK" if empty); failure → `API test failed: {message}` in `.status-error`.
   - If the target site has no AI layer, hide the whole sub-block but keep the
     ids and the payload fields.
5. Always visible: **Snap / Telegram users** `sm-usernames` textarea 3 rows
   (placeholder `@snap_user_one` ⏎ `telegram_user_two`), then hint
   *"`%username%` inserts one random handle."*

**No Save button and no save-status line in this card** — Message is persisted by
Run Browser (Show Mode) / Save & Start Bot (API Mode).

`messagePayload(prefix)`:
`{chat_mode, use_loaded_sequences: mode==="folder", sequence, usernames,
ai_enabled, ai_base_url, ai_model, ai_api_key}`.

#### 6.2.4 Section `4 · Run Browser`
Card `h2` `4 · Run Browser`, `.subpage.narrow`, `.run-card`:
1. `.mode-label` **Select Profile** + two chips, `name="sm-profile-choice"`:
   `sm-profile-default` value `default` (**default**) checked;
   `sm-profile-session-choice` value `session` (**session**).
2. `#sm-profile-session` hint (live, from `ListProfiles()`):
   - default → "Fresh browser — manual login; the session is saved for the session option."
   - session + saved default session → "Saved default session ({saved_at}) — Run Browser logs in automatically." (`.status-ok`)
   - session without a saved session → "No saved default session yet — manual login will save one for the session option."
3. `sm-run` **▶ Run Browser** (`.primary-button.run-button`, full width).
4. `sm-run-status` — default **"Idle."**
5. hint *"Saves Setting + Proxy + Message, then opens a visible Rod browser. Log
   in manually; live logs start automatically. Close the browser to stop."*

Click sequence (nothing else needed):
`setBusy(show)` → read+validate Setting/Message/Proxy → status
`"Applying Setting, Proxy and Message…"` → `SaveShowModeSettings(show)` →
`SaveProxySettings(proxy)` → `SaveMessageSettings(message)` →
`SaveBrowserSettings({profile:"default", backend:"rod", headless:false,
use_saved_session: session})` → clear the Show log feed (`entries=[]`, `after=0`,
re-render) → status `"Restoring the saved profile session…"` (session) or
`"Launching a fresh browser (manual login)…"` (default) →
`RunBrowser({profile:"default", backend:"show_mode", headless:false,
use_saved_session}, 1)` → refresh state → status (ok)
`"Browser requested. Log in manually; live logs will appear here. Close the browser to stop."`
Any thrown error → its message in `sm-run-status` with `.status-error`.
`finally` → `setBusy(show,false)`.

#### 6.2.5 Section `5 · Live Logs`
Card `.log-card`, header row: `h2` **5 · Live Logs** + right tools:
`sm-log-filter` select (**All** `all` / **SMS** `sms` / **Rules** `rules` /
**Errors** `errors`) and `sm-log-clear` **Clear**.
`sm-log-list` = `.log-list` (fills the card height), `sm-log-empty` hint =
**"No activity yet. Press Run Browser."**
Behaviour: see 6.4.

### 6.3 API Mode page

Header: `h1` **API Mode** + badge **API worker**; actions right:
`#api-start` **Save & Start Bot** (primary), `#api-stop` **Stop** (secondary),
`#api-refresh` **Refresh** (secondary).
Description: *"Match, DM and reply without a visible browser. A saved login and
message settings are required."*
Second line `#api-summary` (`.hint-text`), filled after load:
`Backend: {browser.backend} · Profiles auto-start · Message steps: {n}`
(n = non-empty lines of `message.sequence`).
Status line `#api-status` — default **"Idle."**

Six counters, `.home-stats` → `.stat-capsule` each: label + `strong` id:
**Running** `api-stat-running` (`home.running_accounts`) · **Profiles**
`api-stat-profiles` (`home.bot_accounts`) · **Messages sent** `api-stat-messages`
(`home.messages_sent`) · **Traffic users** `api-stat-traffic`
(`home.traffic_users`) · **snap/tg sent** `api-stat-snap`
(`home.snap_tg_sent_count`) · **Dead accounts** `api-stat-dead`
(`home.dead_accounts`, `.stat-capsule.danger`).

#### 6.3.1 Section `1 · Login Capture`
Card `h2` `1 · Login Capture`, `.subpage.narrow`.
Hint: *"Opens a fresh browser for a manual login. The session is saved and the
browser closes."*
Fields (`form-grid`):
1. **Profile** `api-profile` text, placeholder `default`, `autocomplete="off"`,
   `list="api-profile-options"`; adjacent `<datalist id="api-profile-options">`
   filled from `ListProfiles()` — option label
   `{name} · session {saved_at}` or `{name} · no session`.
2. `#api-profile-session` hint — *"Saved profiles are restored automatically."*
   then live text: saved session → "Saved session ({saved_at}) — Run Browser will
   log in automatically." (`.status-ok`); otherwise "No saved session — the
   browser will wait for manual login."
3. **Browser mode** `api-backend` select — **Rod** `rod` / **ChromeDP** `chromedp`.
4. checkbox `api-headless` — **Headless (keep off for manual login)**.
Actions: `api-launch-login` **Launch for Login** (secondary), status
`api-login-status` — default **"No browser launched."**
Click → status `Launching {backend} for login…` →
`RunBrowser({profile, backend, headless}, 1)` → refresh → status (ok)
*"Launch requested. Finish the login in the browser; it closes after the session
is saved."* Errors print their message with `.status-error`.
**No auto-register card, no account table, no folder button in this build.**

#### 6.3.2 Section `2 · Proxy`
Identical to Show Mode (`2 · Proxy`, `.subpage.narrow`), ids with `api-`:
`api-proxy-enabled`, `api-proxy-scheme`, `api-proxy-entries`,
`api-test-proxy`, `api-proxy-status` (default "Not tested."), `api-proxy-results`.
Same test behaviour and the same summary sentence.

#### 6.3.3 Section `3 · Message`
Identical structure to Show Mode's Message card with `api-` ids:
`api-mode-option-single|folder|evachat` + `api-chat-mode-*`
(`name="api-chat-mode"`), `api-section-single-mode` (`api-sequence`),
`api-section-folder-mode` (`api-load-folder`, `api-clean-loaded`,
`api-loaded-status`), `api-section-evachat-mode` (`api-open-eva`,
`api-eva-status`, optional AI block `api-ai-enabled`, `api-ai-base-url`,
`api-ai-model`, `api-ai-api-key`, `api-ai-test`, `api-ai-status`), and
**Snap / Telegram users** `api-usernames`.
Differences from Show Mode's card (as in the original): the single-sequence block
has **no** `{hello|hi}` hint and the usernames field has **no** `%username%` hint.
Both pages read/write the **same** saved `message` record on load; they are
independent form slices while editing (Save & Start Bot writes the API slice,
Run Browser writes the Show slice).

#### 6.3.4 Section `4 · Bot Settings`
Card `h2` `4 · Bot Settings`.
First-message block (same semantics as Show Mode, `api-` ids):
`api-first-mode`, `api-first-percent` (+`api-first-percent-row`),
`api-first-wait` (+`api-first-wait-row`), `api-first-min` / `api-first-max`
(+`api-first-timeout-row`).
Then a two-column grid of timing pairs (min left, max right), all with
`min`/`max` attributes as shown and step 1:

| id | label (exact) | min | default |
|---|---|---|---|
| `api-poll-min` / `api-poll-max` | Poll min (s) / Poll max (s) | 1 | 20 / 45 |
| `api-action-min` / `api-action-max` | Action min (s) / Action max (s) | 1 | 20 / 45 |
| `api-skip-min` / `api-skip-max` | Skip timeout min (s) / Skip timeout max (s) | 0 | 60 / 60 |
| `api-close-min` / `api-close-max` | Chat closed wait min (s) / Chat closed wait max (s) | 0 | 0 / 0 |
| `api-reply-min` / `api-reply-max` | Worker reply delay min (s) / Worker reply delay max (s) | 0 | 0 / 0 |

Mapping to the payload (`GeneralSettings`): `poll_*`, `action_*`,
`match_reply_timeout_*` (skip), `match_close_wait_*` (close),
`reply_delay_*` (worker reply delay), plus `first_message`.

#### 6.3.5 Section `5 · Live Logs`
Card `.log-card`, header `h2` **5 · Live Logs**, tools `api-log-filter`
(All/SMS/Rules/Errors) + `api-log-clear` **Clear**, feed `api-log-list`,
empty text `api-log-empty` = **"No activity yet. Start the bot to see live worker
logs."** Same component behaviour as 6.4, with its own cursor.

#### 6.3.6 Save & Start Bot / Stop / Refresh
**Save & Start Bot** (`#api-start`, disabled while running or busy):
status `"Saving browser, proxy, message and settings…"` →
`SaveBrowserSettings({profile: api-profile||"default", backend: api-backend,
headless: api-headless, use_saved_session: snapshot.browser.use_saved_session})` →
`SaveProxySettings` → `SaveMessageSettings` → `SaveGeneralSettings` →
status `"Starting workers…"` → `StartBot()` → refresh state → status (ok)
**"Started. One worker runs per saved profile."**
Validation errors abort before any save and print in `#api-status`.
**Stop** (`#api-stop`, disabled while idle) → `StopBot()` → refresh →
"Stopped." (ok).
**Refresh** → reload state → "Refreshed." (ok).

### 6.4 Live Logs component (shared)

- Poll `GetShowModeLog(after)` / `GetBotLog(after)` **every 1 s** while that page
  is active, and immediately on backend events `ui:showlog` / `ui:botlog`.
  Response: `{version, entries[]}`; keep `after = version`; append new entries;
  skip a tick while a request is pending; cap the in-memory list at 600 rows.
- Row markup: `.log-row.log-{kind}` → `.log-time` (`entry.time`) +
  `.log-badge` (`IN|OUT|SKIP|SYS|ERR` from
  `{in:"IN",out:"OUT",skip:"SKIP",system:"SYS",error:"ERR"}`) +
  `.log-body` containing `.log-text` and optional `.log-meta`.
  `IN`/`OUT` text is rendered as `"{text}"` (quotes); other kinds plain.
  Meta joins `entry.rule` / `entry.detail` and, for `OUT` only, `✓` when
  `setting_ok` else `✗`, separated by `" · "`.
- Filters: `all` → everything; `sms` → in+out; `rules` → out+skip+system;
  `errors` → error.
- Re-render on filter change and on tab/section activation; preserve scroll:
  only auto-scroll to bottom when the user is already within 60 px of it.
- `…-log-empty` is hidden as soon as ≥1 row passes the filter; it is never
  removed from the DOM.
- Clear → `ClearShowModeLog()` / `ClearBotLog()` → empty the list → status
  **"Live log cleared (logs.txt is kept)."** (`sm-run-status` / `api-status`).
  `logs.txt` on disk is never truncated by the UI.
- Example system lines this feed must be able to show (from the real build):
  `AI OK (https://api.featherless.ai/v1 / deepseek-ai/DeepSeek-V4.1-Flash-1017) -> "just chillin bored af, wbu"`
  `EVAChat data folder ready: <path> (13 trigger categories, 18 reply pools)`
  `Wails UI ready.`
  `[default] REST DONE | account <id> returned to the active list`

### 6.5 Page skeleton parity

| | Show Mode | API Mode |
|---|---|---|
| Title badge | `Idle` / `Running` (+`.live`) | `API worker` |
| Header actions | Refresh | Save & Start Bot · Stop · Refresh |
| Extra header line | — | `Backend: … · Profiles auto-start · Message steps: n` |
| Status line | `sm-run-status` ("Idle.") | `api-status` ("Idle.") |
| Counters | — | 6 capsules (last one danger) |
| Section 1 | `1 · Setting` (wide subpage) | `1 · Login Capture` (narrow) |
| Section 2 | `2 · Proxy` (narrow) | `2 · Proxy` (narrow) |
| Section 3 | `3 · Message` (wide) | `3 · Message` (wide) |
| Section 4 | `4 · Run Browser` (narrow) | `4 · Bot Settings` (wide) |
| Section 5 | `5 · Live Logs` (wide, flex) | `5 · Live Logs` (wide, flex) |

## 7. INTERACTION & LIFECYCLE

**7.1 Bootstrap.** `bindTabs()` → `bindForms()` → `bindGlobalState()` →
`bindActions()` → `bindEvents()` → `activateTab("show")` → retry loop:
up to **50 attempts**, **200 ms** apart, calling `GetInitialState()`; on the
first success: `fillForms(snapshot)` (site-independent: browser, proxy, message
×2, show settings, bot settings, counters, badges, summary) →
`refreshProfiles()` → `refreshShowLog()` → `document.querySelector(".shell")
.classList.add("visible")` → start intervals: **3 s** stats, **10 s** profiles,
**1 s** active-page log. If all attempts fail:
`sm-run-status` = **"Failed to connect to Wails bindings."** (`.status-error`).

**7.2 Tabs & sections.** `activateTab(tab)` toggles `.nav-button.active` and
`.tab-panel.active`, hides the other `#subnav-*`, then activates the stored
sub-section for that page (`state.activeSub = {show, api}`) and refreshes that
page's log. `activateSubpanel(id)` toggles `.subpanel.active` /
`.subnav-button.active`, restores that page's typed values, and refreshes logs
when the logs section opens.

**7.3 Typed-value persistence.** `formStore = {show:{}, api:{}}`; every
`input/select/textarea` with an id inside `[data-panel="show"]` /
`[data-panel="api"]` is captured on `input`+`change`; restore on section switch,
never touching `document.activeElement`.

**7.4 Running states.** `state.runningShow`, `state.runningBot`,
`state.busy{show,api}`. Renderer:
`#running-pill` → "Running"/"Stopped" with `.pill.running|.stopped`;
`#show-run-badge` → "Running"/"Idle" + `.live`;
`#sm-run` disabled when `runningShow || busy.show`;
`#api-start` disabled when `runningBot || busy.api`;
`#api-stop` disabled when `!runningBot || busy.api`.
Runtime events: `ui:show_running` (bool) and `ui:running` (bool) update the
flags and re-render.

**7.5 Error surfaces.** If a runtime event `ui:log` carries a message matching
`/[^]]+ fresh \d+\/\d+/` and **not** containing
`"] Removed temporary browser data from "`, echo it into
`#api-login-status` (error styling when `level === "error"`).
Global handlers: `window.onerror` and `unhandledrejection` →
`reportFrontendError(context, error)` →
`runtime.LogError("Frontend | {context} | {stack}")` (no-op if absent).

**7.6 Accessibility.** Labels wrap their controls; status/log regions are
`aria-live`; buttons are real `<button type="button">`; visible focus ring.

## 8. BACKEND CONTRACT (single site — exact names, exact order)

```
GetInitialState() -> UISnapshot
ListProfiles() -> [{ name, has_session, saved_at }]
SaveShowModeSettings(ShowModeSettings) -> void
SaveGeneralSettings(GeneralSettings) -> void
SaveMessageSettings(MessageSettings) -> void
SaveProxySettings(ProxySettings) -> void
SaveBrowserSettings(BrowserSettings) -> void
RunBrowser(BrowserSettings, count:int) -> void        // "show_mode" backend = visible live chat
StartBot() -> void
StopBot() -> void
GetShowModeLog(after:uint) -> { version:uint, entries:[LogEntry] }
GetBotLog(after:uint) -> { version:uint, entries:[LogEntry] }
ClearShowModeLog() -> void
ClearBotLog() -> void
TestProxyEntries(rawLines:string) -> [{
  line:int, ok:bool, tunnel_ok:bool, label:string, latency_ms:int,
  status:string, detail:string, provider_message:string,
  proxy_authenticate:string, hint:string }]
LoadMessageFolder() -> [string]
ClearLoadedMessages() -> void
OpenEVAChatDataFolder() -> string
TestEVAChatAI(MessageSettings) -> string
```

Optional (present in the original backend, not required by the UI):
`ClearLogs`, `CleanProfiles`, `RunFreshBrowsers`, `GetLiveChatSnapshot`,
`StartLocalTestChat`, `SendLocalTestChatMessage`, `GetLocalTestChatState`,
`OpenSiteFolder`, `GetSiteConfig`.

```jsonc
UISnapshot {
  "running": false, "show_mode_running": false,
  "site": { "id":"chitchat", "name":"ChitChat.gg",
            "start_url":"https://app.chitchat.gg/text",
            "auth_mode":"chitchat_jwt", "folder_path":"sites/chitchat" },
  "home": { "bot_accounts":2, "running_accounts":0, "user_send_count":0,
            "dead_accounts":0, "traffic_users":0, "messages_sent":0,
            "snap_tg_sent_count":0, "skipped_accounts":0 },
  "browser": { "profile":"default", "backend":"rod", "headless":false,
               "use_saved_session":false },
  "message": { "chat_mode":"single|folder|eva_chat_flow",
               "sequence":"line1\nline2", "usernames":"@a\n@b",
               "loaded_sequences":["…"], "use_loaded_sequences":false,
               "ai_enabled":false, "ai_base_url":"", "ai_model":"",
               "ai_api_key":"" },                 // key: password field only
  "proxy": { "enabled":true, "entries":"host:port:user:pass" },
  "general": { "first_message": FirstMessage,
               "poll_min_seconds":20, "poll_max_seconds":45,
               "action_min_seconds":20, "action_max_seconds":45,
               "match_reply_timeout_min_seconds":60,
               "match_reply_timeout_max_seconds":60,
               "match_close_wait_min_seconds":0,
               "match_close_wait_max_seconds":0,
               "reply_delay_min_seconds":0, "reply_delay_max_seconds":0 },
  "show_mode": { "first_message": FirstMessage,
                 "reply_delay_min_seconds":3, "reply_delay_max_seconds":6,
                 "auto_skip":false,
                 "no_reply_min_seconds":20, "no_reply_max_seconds":30,
                 "next_chat_min_seconds":4, "next_chat_max_seconds":15 },
  "browser_options": [ { "key":"rod","label":"Rod","description":"…" },
                       { "key":"chromedp","label":"ChromeDP","description":"…" },
                       { "key":"show_mode","label":"Show Mode","description":"…" } ],
  "license_invalid": false, "license_reason": "", "license_key": "",
  "logs": [ { "time":"18:12:03", "level":"info", "message":"…" } ]
}
FirstMessage      { "mode":"bot|user|random", "bot_first_percent":70,
                    "wait_timeout":false, "wait_min_seconds":30, "wait_max_seconds":60 }
MessageSettings   { "chat_mode":"…", "sequence":"…", "usernames":"…",
                    "use_loaded_sequences":false, "ai_enabled":false,
                    "ai_base_url":"…", "ai_model":"…", "ai_api_key":"…" }
ProxySettings     { "enabled":false, "entries":"" }
BrowserSettings   { "profile":"default", "backend":"rod|chromedp|show_mode",
                    "headless":false, "use_saved_session":false }
LogEntry          { "time":"HH:MM:SS", "kind":"in|out|skip|system|error",
                    "text":"…", "rule":"…", "detail":"…", "setting_ok":true }
```

## 9. STORAGE

```
settings/show_mode/v1   Show Mode timings (independent of worker timings)
settings/general        API Mode timings + first-message policy
settings/message        chat_mode, sequence, usernames, AI config, loaded seqs
settings/proxy          enabled, entries
settings/browser        profile, backend, headless, use_saved_session
sites/<site_id>/profiles/          browser profiles            (never in git)
sites/<site_id>/sessions/          latest_session.json         (never in git)
data/eva_chat/input|output/        TXT trigger pools / reply pools
logs.txt                           append-only history (UI Clear never deletes)
```

The site id is config, not user input. Create `data/` and `sites/<site_id>/`
next to the executable on first run. Git-ignore runtime data, sessions, cookies,
keys and logs.

## 10. VALIDATION, DEFAULTS, EXACT MESSAGES

Timing helper: `timingPair(minId, maxId, label, floor)` passes only when both are
integers, `floor ≤ min ≤ max ≤ 86400`; otherwise
`"{label}: use whole seconds with {floor} ≤ min ≤ max ≤ 86400."`
Labels used: `Reply delay`, `No-reply timeout`, `Next-chat wait`, `Poll`,
`Action`, `Skip timeout`, `Chat closed wait`, `Worker reply delay`.

First-message helper (`readFirst(prefix)`):
- mode not in `bot|user|random` → `Choose a first-message policy.`
- percent not an integer 0–100 → `Bot first chance must be a whole number from 0 to 100.`
- min/max not integers, `min > max`, or `> 86400` →
  `First-message timeout: use whole seconds, 0 ≤ min ≤ max ≤ 86400.`
- mode ≠ bot, wait = skip, min < 1 → `A first-message timeout needs a positive minimum.`
- returns `{mode, bot_first_percent, wait_timeout: mode!=="bot" && skip,
  wait_min_seconds, wait_max_seconds}`.

Other rules:
- auto-skip checked and `no_reply_min < 1` → `Auto skip needs a positive no-reply timeout.`
- folder mode with 0 loaded sequences (on save/run) →
  `Load a folder of sequences before using Folder Sequences mode.`
- proxy test with empty textarea → `Enter at least one proxy to test.`
- The AI key is never validated client-side; `Test AI` surfaces the backend error.
- All messages appear in the owning status line with `.status-error`, and the
  action aborts before saving/launching.

Defaults after load (`fillFirst`): `mode:"bot"`, `bot_first_percent:70`,
`wait_timeout:false`, `wait_min:30`, `wait_max:60`.

## 11. A → Z FIELD INVENTORY (every control the UI must contain)

**Sidebar / global**
`#license-overlay` · `#license-message` · `#license-key-input` ·
`#copy-license-btn` · `.brand-mark` · brand title · brand subtitle ·
`#nav-show` (`data-tab="show"`) · `#nav-api` (`data-tab="api"`) ·
`#subnav-show` (`#sub-show-setting|proxy|message|run|logs`) ·
`#subnav-api` (`#sub-api-login|proxy|message|settings|logs`) · `#running-pill` ·
`#show-run-badge` · `#show-refresh`.

**Show Mode — Setting**: `sm-first-mode` · `sm-first-wait` ·
`sm-first-percent-row`+`sm-first-percent` · `sm-first-wait-row` ·
`sm-first-timeout-row`+`sm-first-min`+`sm-first-max` · `sm-reply-min` ·
`sm-reply-max` · `sm-auto-skip` · `sm-noreply-min` · `sm-noreply-max` ·
`sm-next-min` · `sm-next-max`.

**Show Mode — Proxy**: `sm-proxy-enabled` · `sm-proxy-scheme` ·
`sm-proxy-entries` · `sm-test-proxy` · `sm-proxy-status` · `sm-proxy-results`.

**Show Mode — Message**: `sm-chat-mode-single|folder|evachat` ·
`sm-mode-option-single|folder|evachat` · `sm-section-single-mode` ·
`sm-sequence` · `sm-section-folder-mode` · `sm-load-folder` ·
`sm-clean-loaded` · `sm-loaded-status` · `sm-section-evachat-mode` ·
`sm-open-eva` · `sm-eva-status` · `sm-ai-enabled` · `sm-ai-base-url` ·
`sm-ai-model` · `sm-ai-api-key` · `sm-ai-test` · `sm-ai-status` ·
`sm-usernames`.

**Show Mode — Run Browser**: `sm-profile-default` · `sm-profile-session-choice`
· `sm-profile-session` · `sm-run` · `sm-run-status`.

**Show Mode — Live Logs**: `sm-log-filter` · `sm-log-clear` · `sm-log-list` ·
`sm-log-empty`.

**API Mode — header/status**: `api-start` · `api-stop` · `api-refresh` ·
`api-summary` · `api-status` · `api-stat-running` · `api-stat-profiles` ·
`api-stat-messages` · `api-stat-traffic` · `api-stat-snap` · `api-stat-dead`.

**API Mode — Login Capture**: `api-profile` · `api-profile-options` (datalist) ·
`api-profile-session` · `api-backend` · `api-headless` · `api-launch-login` ·
`api-login-status`.

**API Mode — Proxy**: `api-proxy-enabled` · `api-proxy-scheme` ·
`api-proxy-entries` · `api-test-proxy` · `api-proxy-status` ·
`api-proxy-results`.

**API Mode — Message**: `api-chat-mode-single|folder|evachat` ·
`api-mode-option-single|folder|evachat` · `api-section-single-mode` ·
`api-sequence` · `api-section-folder-mode` · `api-load-folder` ·
`api-clean-loaded` · `api-loaded-status` · `api-section-evachat-mode` ·
`api-open-eva` · `api-eva-status` · `api-ai-enabled` · `api-ai-base-url` ·
`api-ai-model` · `api-ai-api-key` · `api-ai-test` · `api-ai-status` ·
`api-usernames`.

**API Mode — Bot Settings**: `api-first-mode` · `api-first-percent`
(+`api-first-percent-row`) · `api-first-wait` (+`api-first-wait-row`) ·
`api-first-min` · `api-first-max` (+`api-first-timeout-row`) · `api-poll-min` ·
`api-poll-max` · `api-action-min` · `api-action-max` · `api-skip-min` ·
`api-skip-max` · `api-close-min` · `api-close-max` · `api-reply-min` ·
`api-reply-max`.

**API Mode — Live Logs**: `api-log-filter` · `api-log-clear` · `api-log-list` ·
`api-log-empty`.

### 11.1 id mapping (if you prefer the original prefix)
The shipped build used `bg-` instead of `api-` and `data-tab="bot"` /
`data-panel="bot"`; `GetBotLiveLog` / `ClearBotLiveLog` instead of
`GetBotLog` / `ClearBotLog` in the newest revision. Either set is fine as long as
it is consistent — the visible labels must stay **API Mode / Login Capture /
Bot Settings** as in the screenshots.

## 12. ACCEPTANCE CHECKLIST

1. Sidebar = brand (`CB`, product name, tech subtitle) → **Show Mode / API Mode**
   → 5 sub-items each → `Workers` + `Stopped`. No site selector, search,
   filters, site list, debug/folder buttons, or "new site" form anywhere.
2. Show Mode header: title + `Idle` badge + Refresh + exact description.
3. `1 · Setting` renders every field with the documented labels/defaults;
   Random shows the chance row; User first/Random show the wait row; Skip
   reveals the timeout min/max; hidden rows are disabled; invalid values block
   Run Browser with the exact messages in §10.
4. `2 · Proxy` has enable / protocol / lines / Test Proxy / "Not tested.";
   a test renders PASS / REACHABLE / FAIL rows with latency and the
   `{passed}/{total}` summary, then the button returns to "Test Proxy".
5. `3 · Message` shows exactly 3 chips; each shows only its block; folder count
   text updates (`0 sequences loaded` → `n sequences loaded`); folder mode with 0
   loaded refuses to save; the EVAChat block opens the data folder and (if
   present) tests the AI with the exact status texts; usernames + hint always
   visible; **there is no Save button**.
6. `4 · Run Browser` = Select Profile chips + live session hint + full-width
   **▶ Run Browser** + "Idle." + footer hint; a click saves 4 stores, clears the
   Show log, prints the launch line, badge → Running, button disables; closing
   the browser returns to Idle.
7. `5 · Live Logs` = filter + Clear + rows `time · IN/OUT/SKIP/SYS/ERR · text`,
   1 s cursor polling, no duplicates, 600-row cap, autoscroll only near the
   bottom, empty text "No activity yet. Press Run Browser.", Clear prints
   "Live log cleared (logs.txt is kept)." and does not touch `logs.txt`.
8. API Mode header = title + `API worker` badge + Save & Start Bot / Stop /
   Refresh + description + summary line + "Idle." + six counters (Dead accounts
   red); counters refresh every 3 s.
9. `1 · Login Capture` = Profile (+ saved-profile datalist + live session hint),
   Browser mode Rod/ChromeDP, Headless checkbox, **Launch for Login**, "No
   browser launched."; clicking prints `Launching {backend} for login…` then the
   success line. No auto-register, no account table.
10. `2 · Proxy` and `3 · Message` behave exactly like Show Mode's equivalents
    (own ids, same texts, same three modes, same probes).
11. `4 · Bot Settings` = first-message block + the five timing pairs with the
    documented defaults; Save & Start Bot validates everything, saves 4 stores,
    then starts; Stop is disabled while idle; log feed is independent.
12. Typed values survive every tab/section switch; Refresh reloads from the
    backend without losing focus in the active field.
13. License overlay appears for `license_invalid` / `ui:license_invalid` with the
    key and a working Copy button.
14. Every backend failure is visible as `.status-error` text; nothing is claimed
    that the backend did not confirm.
15. Nothing site-specific is hard-coded outside the site config module.
16. No runtime data, sessions, keys or logs are committed.

## 13. FORBIDDEN

- Any multi-site concept: selector, search, filter pills, "100+ sites",
  per-site tables, "create site", `ListSites/SelectSite/CreateSite`,
  `RunSiteBrowser/RunSiteBotWithSession`, `sites/<id>/accounts/`.
- Renaming tabs away from **Show Mode** / **API Mode**.
- A 4th chat mode ("API Chat"), a "Keyword Based" mode, or merging the three.
- Auto-register cards, "Accounts & Saved Sessions" tables, site quick lists.
- Save buttons in Setting / Proxy / Message / Bot Settings (save-on-run only).
- Mock data, fake progress, optimistic badge flips.
- Hard-coded site URLs/selectors/DOM hooks outside config.
- API keys in localStorage/IndexedDB/the DOM/logs, or rendered back to the page.
- Removing status lines, the six counters, the license overlay or log filters.
- Swallowing errors silently.

## 14. DELIVERABLES

1. The working single-site app: two pages, 5 sections each, all ids above.
2. `README.md` — build/run, where settings/session files live, how to fill the
   site config, how to test without a login.
3. Short docs: chat-mode semantics (Single / Folder / EVAChat Flow), first-message
   policy, proxy test semantics, site configuration, log event kinds.
4. A report: implemented vs `TODO(site)` (real selectors / login flow), plus
   which checklist items you verified and how (commands + observed output).

Ask only if a SITE PROFILE value is genuinely required; otherwise ship with
`TODO(site)` config placeholders.

# PART A ENDS HERE

---

# PART B — SHORT VERSION (small context window)

> Part A is authoritative. Send it if possible.

Build a dark, desktop-style **single-site** chat-bot dashboard. No site
switching at all (no selector/search/filters/site list/add-site). Site identity
comes from one backend config object; unknown details stay `TODO(site)`.

**Shell:** sidebar `238px` `#191b17` = brand (`CB` square + product name + tech
subtitle) → two full-width tabs **Show Mode** / **API Mode** (active = accent
tint + accent border) → 5 sub-items of the active tab (active = accent pill,
left border rail) → footer `Workers` + pill (`Stopped` red-tint / `Running`
green-tint). Main = header (title + badge + right buttons) + one-line
description (+ summary line on API Mode) + status line + cards numbered
`1 · … 5 · …`. Tokens: bg `#161716`, panel `#20221e`, alt `#282b25`, line
`#3e4439`, text `#eef2e8`, muted `#b5beb0`, accent `#83cb73`, warn `#d9b74d`,
danger `#d36a6a`; cards radius 14, padding 18, gap 11; chips radius 8; log box
`#191b17`. Every action owns a status line (neutral / `.status-ok` /
`.status-error`).

**Show Mode** (badge `Idle`→`Running`, Refresh, description "Configure, press Run
Browser, log in once. The visible browser chats with these rules; close it to
stop."; status "Idle."):
1 · Setting — hint "Run Browser applies these values. No separate save.";
First-message policy Bot first/User first/Random (+ "Bot first chance (%)" 70
when random), Wait for first message Wait indefinitely/Skip after timeout
(+ First timeout min/max 30/60 when skip and mode≠bot), Reply delay min/max 3/6,
☐ Auto-skip, No-reply timeout min/max 20/30, Next chat wait min/max 4/15; hidden
rows disabled.
2 · Proxy — Enable proxy, Default protocol for plain lines HTTP/SOCKS5, Proxy
lines textarea, **Test Proxy**, "Not tested." + per-line PASS / REACHABLE-SITE
RESTRICTED / FAIL with latency/status/detail and
`{passed}/{total} passed; {reachable} HTTPS tunnel(s) verified. A test is not a
browser-login guarantee.`
3 · Message — "Chat mode" label + exactly 3 chips: **Single Sequence** (textarea,
"One line = one step. %username%, {hello|hi}."), **Folder Sequences**
(Load Folder / Clean Loaded / "0 sequences loaded"→"n sequences loaded"; 0 →
cannot save), **EVAChat Flow** (Open EVAChat Data Folder + status "TXT data is
initialized on first run." + optional AI-hybrid block: ☐ AI reply text (hybrid),
API base URL, Model, API key (password), **Test AI** + status); always
**Snap / Telegram users** textarea + hint "%username% inserts one random handle.";
**no Save button**.
4 · Run Browser (narrow) — "Select Profile" chips default/session + live hint
("Fresh browser — manual login; the session is saved for the session option." /
"Saved default session (date) — Run Browser logs in automatically." / "No saved
default session yet…"), full-width **▶ Run Browser**, "Idle.", hint "Saves
Setting + Proxy + Message, then opens a visible Rod browser. Log in manually;
live logs start automatically. Close the browser to stop." Click → save
show-mode + proxy + message + browser settings → clear log → "Restoring the saved
profile session…"/"Launching a fresh browser (manual login)…" → RunBrowser →
"Browser requested. Log in manually; live logs will appear here. Close the
browser to stop."
5 · Live Logs — All/SMS/Rules/Errors + Clear, rows `time · IN|OUT|SKIP|SYS|ERR ·
text · rule/detail/✓✗`, 1 s cursor polling, 600 rows, autoscroll only near
bottom, empty "No activity yet. Press Run Browser."

**API Mode** (badge `API worker`; Save & Start Bot / Stop / Refresh; description
"Match, DM and reply without a visible browser. A saved login and message
settings are required."; summary "Backend: rod · Profiles auto-start · Message
steps: n"; "Idle."; six counters Running / Profiles / Messages sent / Traffic
users / snap/tg sent / Dead accounts(red)):
1 · Login Capture (narrow) — hint "Opens a fresh browser for a manual login. The
session is saved and the browser closes."; Profile input (+ saved-profile
datalist, hint "Saved profiles are restored automatically." then live session
text), Browser mode Rod/ChromeDP, ☐ Headless (keep off for manual login),
**Launch for Login**, "No browser launched." → "Launching {backend} for login…"
→ "Launch requested. Finish the login in the browser; it closes after the session
is saved." No auto-register, no accounts table.
2 · Proxy — identical to Show Mode.
3 · Message — same 3 chips/blocks with own ids; same saved record as Show Mode;
no Save button.
4 · Bot Settings — first-message block + Poll 20/45, Action 20/45, Skip timeout
60/60, Chat closed wait 0/0, Worker reply delay 0/0.
5 · Live Logs — same component, own cursor, empty "No activity yet. Start the
bot to see live worker logs."
Save & Start = "Saving browser, proxy, message and settings…" → save browser +
proxy + message + general → "Starting workers…" → StartBot → "Started. One worker
runs per saved profile."; Stop → "Stopped."; Refresh → "Refreshed."

**Extras:** license overlay (`#license-overlay`, "License Required",
`#license-message`, readonly key + **Copy**) on `license_invalid` /
`ui:license_invalid`; bootstrap retries `GetInitialState` ~50×200 ms before
revealing `.shell.visible`, then 3 s stats / 10 s profiles / 1 s log polls;
buttons reflect idle/busy/running; global error handlers log
`Frontend | context | stack`.

**Contract:** `GetInitialState`, `ListProfiles`, `SaveShowModeSettings`,
`SaveGeneralSettings`, `SaveMessageSettings`, `SaveProxySettings`,
`SaveBrowserSettings`, `RunBrowser`, `StartBot`, `StopBot`, `GetShowModeLog`,
`GetBotLog`, `ClearShowModeLog`, `ClearBotLog`, `TestProxyEntries`,
`LoadMessageFolder`, `ClearLoadedMessages`, `OpenEVAChatDataFolder`,
`TestEVAChatAI` — with the JSON shapes from Part A §8. Persist
`settings/{show_mode/v1,general,message,proxy,browser}`, keep
`sites/<site_id>/{profiles,sessions}`, `data/eva_chat/`, `logs.txt`; git-ignore
runtime data/sessions/keys.

**Rules:** §10 validation messages exactly; save-on-run only; typed values
survive navigation; no mock data; no hard-coded site values; no keys in the DOM.
Deliver the app, README, short docs and a verification report.

---

# APPENDIX — Evidence & "don't copy these" list

## A. Where this spec came from (verified, not guessed)

| Source | What it is | Use |
|---|---|---|
| `ChitChat-App-clean-fixed-2026-10-20 (11).zip` (এই repo) | **পরের** ভার্সন: multi-site selector, 4 chat modes, auto-register, accounts table | ❌ এখান থেকে কপি করবেন না |
| `rajuvbygyuiythh/dfdgfhghg45454` → `cmd/chitchatui/frontend/dist/` @ `ff502bc3` (2026-10-19) | একই **single-site** দুই-পেজ dashboard, `Wails + Badger`, ৩ chat mode, `Login Capture`, `Bot Settings`, counters, `Live Logs` | ✅ এই prompt-এর ভিত্তি |
| আপনার স্ক্রিনশট 1–9 | tab-এর নাম **API Mode**, উপরে কোনো site box নেই | ✅ visible labels এখান থেকেই |

Old build vs new build (শুধু পার্থক্যগুলো):

| | Old (আপনার স্ক্রিনশট) | New (zip, 2026-10-20) |
|---|---|---|
| Second tab label | **API Mode** | Background Bot |
| Site selector (100+ sites, search, filters, per-site table, +New/Folder/Debug) | ❌ নেই | ✅ আছে |
| Message chat modes | ৩টা (Single / Folder / EVAChat Flow) | ৪টা (+ API Chat) |
| EVAChat block | AI-hybrid rewrite fields আছে (একাংশ build-এ) | সরানো হয়েছে, pure TXT |
| Auto-register cards / Accounts table | ❌ নেই (Login Capture শুধু browser login) | ✅ আছে |
| Message card-এ Save button | ❌ নেই | ✅ আছে (`Save Message Mode`) |
| Chat-mode label | `Chat mode` (CSS uppercase) | `Chat mode` |

## B. id prefix mapping

আসল কোডে API page-এর prefix `bg-`, tab `data-tab="bot"`, RPC
`GetBotLiveLog` / `ClearBotLiveLog`। নিচের prompt-এ বোঝার সুবিধার জন্য
**`api-` / `data-tab="api"` / `GetBotLog` / `ClearBotLog`** ব্যবহার করেছি।
দুটোর যেকোনো এক সেট ব্যবহার করা যাবে, শুধু **একটার ভেতরে মিশাবেন না**; আর
UI-তে দেখা যাওয়া লেখা (`API Mode`, `Login Capture`, `Bot Settings`, `Live
Logs`) স্ক্রিনশটের মতোই থাকবে।

## C. Two ways to use this prompt

1. **নতুন সাইটে clone:** Section 3-এর SITE PROFILE ভরে PART A দিন — agent
   আপনার নতুন সাইটের জন্য হুবহু একই dashboard বানাবে।
2. **পুরোনো build ফেরত চাইলে:** একই PART A + শুরুতে লিখে দিন
   *"This is a rebuild of the pre-multi-site single-site build; restore the API
   Mode label and keep three chat modes."* — agent multi-site কিছু যোগ করবে না।
