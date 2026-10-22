# AGENTS.md — Master Architecture, 16-Prompt Roadmap & Hard Execution Rules

Read this file first before doing **any** work in this repository. It is the single source of truth for the 102-site architecture, the mandatory **Plan → Approve → Execute → Verify** workflow, and the 16-prompt master roadmap.

---

## 0. Mandatory 4-Step Hard Workflow (NON-NEGOTIABLE)

Every agent working on this project MUST follow this strict 4-step workflow for each prompt from Prompt 1 to Prompt 16:

1. **STEP 1 — READ `AGENTS.md` & PRESENT TECHNICAL PLAN:**
   - Before touching any production code for the next prompt, read `AGENTS.md` to check completed prompts (`[DONE]`), active rules, and the current prompt's exact specification.
   - Inspect all relevant files and present a clear, file-by-file **Technical Plan** to the user in Bangla (with technical terms in English).
2. **STEP 2 — WAIT FOR USER APPROVAL:**
   - **Never** edit production files for a new prompt before the user explicitly approves the plan ("Approve" / "এগিয়ে যাও").
3. **STEP 3 — DELIVER 100% FULL WORKING CODE (NO SKETCHES / NO PSEUDO-CODE):**
   - Once approved, implement the complete, production-ready code across all targeted Go and Frontend files.
   - Never break existing functionality (`Show Mode`, `Background Bot`, `apichat.exe`, or previously completed prompts).
   - Strictly enforce **Site Folder Isolation**: 102 sites = 102 separate folders (`sites/<site_id>/`). Only `sites/_global/` (settings) and `sites/_recipes/` (Group A–E setup recipes) are shared. One site's code must NEVER read or write another site's folder.
4. **STEP 4 — VERIFY, COMMIT & UPDATE `AGENTS.md` LEDGER:**
   - Run real verification (`go build ./...`, `go vet ./...`, `go test ./...`, `node --test tests/*.test.cjs`, `node scripts/deep-debug.cjs`).
   - Update the prompt's status in `AGENTS.md` (and `source_code/AGENTS.md`) from `[IN PROGRESS]` to `[DONE]` with the commit hash and modified files, then present the next prompt's Technical Plan for approval.

---

## 1. Master Prompt Ledger (Prompts 1 to 16)

| Prompt # | Phase | Task Name | Priority | Status | Key Files & Commit |
|---|---|---|---|---|---|
| **Prompt 1** | Phase 1: Foundation | Site Folder Isolation (`sites/<site_id>/`) | 🔴 MOST IMPORTANT | ✅ **DONE** (`9f959b2`) | `source_code/internal/sites/isolation.go`, `isolation_test.go`, `manager.go` |
| **Prompt 2** | Phase 1: Foundation | Global Settings (`sites/_global/settings.json`) | 🔴 Critical | ✅ **DONE** (`9477ee6`) | `source_code/internal/settings/{settings,store,adapters}.go`, `app.go` |
| **Prompt 3** | Phase 1: Foundation | Site Classification (Group A–E) | 🔴 Critical | ✅ **DONE** (`6bf970d`) | `source_code/internal/sites/classification.go`, 102 `sites/<id>/site.json` |
| **Prompt 4** | Phase 2: Recipe + Runner | Repeatable Recipe System (`sites/_recipes/`) | 🟡 High | ✅ **DONE** (`c41f71b`) | `source_code/internal/sites/recipes.go`, `sites/_recipes/group_{A..E}.json` |
| **Prompt 5** | Phase 2: Recipe + Runner | Site Runner Supervisor | 🔴 Critical | ✅ **DONE** (`0493e40`) | `source_code/internal/runner/supervisor.go`, `supervisor_test.go` |
| **Prompt 6** | Phase 2: Recipe + Runner | Connect Supervisor to Existing Engine | 🔴 Critical | ✅ **DONE** (`38c2d98`, `da58842`) | `source_code/internal/runner/supervisor.go`, `internal/bot/*`, `app.go` |
| **Prompt 7** | Phase 3: Dashboard UI | Dashboard 3-Tab Structure | 🟡 High | ✅ **DONE** (`45500f5`) | `source_code/cmd/chitchatui/frontend/dist/{index.html,styles.css,app.js}` |
| **Prompt 8** | Phase 3: Dashboard UI | Site Grid (Tab 2) | 🟡 High | ✅ **DONE** | `source_code/app.go`, `internal/model/ui.go`, `cmd/chitchatui/frontend/dist/{index.html,styles.css,app.js}`, `tests/browser-ui.test.cjs` |
| **Prompt 9** | Phase 3: Dashboard UI | Per-Site Detail + Overrides | 🟡 Medium | ⏳ **IN PROGRESS (Awaiting Approval)** | `source_code/app.go`, `cmd/chitchatui/frontend/dist/{index.html,styles.css,app.js}` |
| **Prompt 10** | Phase 4: Session + Browserless | Session Capture (Tab 3) | 🔴 Critical | ⏳ **PENDING** | `source_code/internal/sites/session_store.go`, `app.go`, `index.html`, `app.js` |
| **Prompt 11** | Phase 4: Session + Browserless | Browserless Chat | 🔴 Critical | ⏳ **PENDING** | `source_code/internal/runner/browserless.go`, `supervisor.go`, `internal/bot/*` |
| **Prompt 12** | Phase 5: Self-Recovery + Polish | Agent Self-Recovery (Scenarios A–E) | 🔴 MOST IMPORTANT | ⏳ **PENDING** | `source_code/internal/runner/recovery.go`, `supervisor.go`, `app.go`, UI |
| **Prompt 13** | Phase 5: Self-Recovery + Polish | Combined Live Log + Error Dashboard | 🟡 Medium | ⏳ **PENDING** | `source_code/app.go`, `show_log.go`, `bot_log.go`, `index.html`, `app.js` |
| **Prompt 14** | Phase 5: Self-Recovery + Polish | Bulk Operations + Add Site Wizard | 🟢 Nice-to-have | ⏳ **PENDING** | `source_code/app.go`, `cmd/chitchatui/frontend/dist/{index.html,styles.css,app.js}` |
| **Prompt 15** | Phase 6: Final | Settings Sync + Cleanup + Docs | 🟡 Medium | ⏳ **PENDING** | `source_code/app.go`, `cmd/apichat/*`, `docs/PROJECT_MAP.md`, `docs/MULTISITE_GUIDE.md` |
| **Prompt 16** | Phase 6: Final | Full Integration Test + Final EXE Build | 🔴 Critical | ⏳ **PENDING** | `source_code/integration_test.go`, `tests/*.test.cjs`, `verify.bat` |

---

## 2. Full Specification of All 16 Prompts

### PHASE 1: FOUNDATION

#### Prompt 1 — Site Folder Isolation (সবচেয়ে গুরুত্বপূর্ণ) `[DONE — 9f959b2]`
- `internal/sites/` এ একটা isolation module তৈরি করো (`isolation.go`: `SiteContext`, `ResolveSiteContext`, `SafeJoin`)
- প্রতিটা site এর জন্য আলাদা context object থাকবে যেটাতে site ID, folder path, session path, profile path, log path, account path থাকবে
- একটা path safety function বানাও যেটা নিশ্চিত করবে কোনো file operation site folder এর বাইরে যেতে পারবে না (parent directory traversal block)
- Browser profile, session, cookies, logs — সবকিছু site এর নিজের folder এর ভিতরে থাকবে
- একটা site এর code কখনো অন্য site এর folder read/write করতে পারবে না — এটা hard rule
- Existing `manager.go` তে সব file path access এই isolation module এর মাধ্যমে redirect করো
- Test লেখো: একটা site অন্য site এর file access করতে পারছে না এটা verify করো (`isolation_test.go`)
- **MOST IMPORTANT:** ১০২টা site এর ১০২টা আলাদা folder, কোনো shared file নেই site এর মধ্যে। শুধু `_global/` folder shared (settings এর জন্য)।

#### Prompt 2 — Global Settings `[DONE — 9477ee6]`
- `sites/_global/` folder তৈরি করো
- সেখানে `settings.json` রাখো যেখানে message mode, proxy, reply delay, first message policy, API key/model, skip timeout থাকবে
- একটা load/save function বানাও (`internal/settings/store.go`)
- একটা merge function বানাও: global settings + per-site override মিলিয়ে effective settings return করবে (`Merge(global, override) Effective`)
- Per-site override (`sites/<site_id>/settings_override.json`): যদি কোনো site custom proxy বা custom message mode set করে তাহলে সেটা global এর বদলে ব্যবহার হবে, বাকি সব global থেকে আসবে
- Existing `uistore/` pattern follow করো
- Dashboard থেকে global settings save করলে সব site এ apply হবে (যাদের override নেই)

#### Prompt 3 — Site Classification (Group A–E) `[DONE — 6bf970d]`
- ৫টা group define করো (`internal/sites/classification.go`):
  - **Group A:** Browserless — signup লাগে না, direct chat (no captcha, no login)
  - **Group B:** Auto-Signup — API দিয়ে account বানানো যায়, captcha নেই
  - **Group C:** Browser-Once — captcha/login লাগে একবার, পরে browserless
  - **Group D:** DOM-Only — API/WS নেই, headless browser দিয়ে DOM chat
  - **Group E:** Manual — heavy bot detection, manual only
- একটা probe function বানাও যেটা site এর URL visit করে detect করবে: Captcha আছে কিনা (`recaptcha`, `hcaptcha`, `cloudflare` keywords), Signup form আছে কিনা, WebSocket endpoint আছে কিনা, API endpoint আছে কিনা, Login required কিনা
- Probe result অনুযায়ী auto-classify করো এবং classification result `site.json` এ save করো
- ১০২টা existing site এর `site.json` এ `classification` field add করো
- Classification re-runnable হবে — যেকোনো সময় re-probe করা যাবে (`ProbeAndClassifySite`)

---

### PHASE 2: RECIPE + RUNNER

#### Prompt 4 — Recipe System `[DONE — c41f71b]`
- `sites/_recipes/` folder তৈরি করো
- ৫টা recipe file বানাও (একটা per group): `group_A.json`, `group_B.json`, `group_C.json`, `group_D.json`, `group_E.json`
- প্রতিটা recipe তে sequential steps থাকবে:
  - **Group A:** folder create → WS detect → direct connect test → save config
  - **Group B:** folder create → signup endpoint detect → fake account generate → POST register → token save → chat test
  - **Group C:** folder create → login URL detect → mark "needs browser" → wait user login → capture session → verify browserless
  - **Group D:** folder create → headless profile setup → DOM selector detect → save driver → headless test
  - **Group E:** folder create → mark manual only
- Recipe executor function (`ExecuteSiteRecipe`) বানাও যেটা step by step run করবে
- কোনো step fail হলে log করবে, fallback group try করবে (`A→B→C→D→E` / `B→C→D→E`)
- Successful setup এর learning save করবে recipe তে (`success_rate` update)
- সব execution log site এর নিজের `logs/` folder এ যাবে (`sites/<site_id>/logs/recipe.jsonl`)

#### Prompt 5 — Site Runner Supervisor `[DONE — 0493e40]`
- `Supervisor` struct বানাও (`internal/runner/supervisor.go`) যেটা সব site runner manage করবে
- প্রতিটা site আলাদা goroutine এ চলবে, আলাদা context দিয়ে
- একটা site crash/panic হলে শুধু সেটা `"failed"` mark হবে, বাকি সব site চলতে থাকবে — এটা absolute guarantee
- Supervisor methods: `StartSite(siteID)`, `StopSite(siteID)`, `StartSelected(siteIDs list)`, `StopAll()`, `GetStatus(siteID)`, `GetAllStatuses()`, `PauseSite(siteID, reason)`, `RestartSite(siteID)`
- Error threshold: ৫টা consecutive error হলে auto-pause, UI তে notify
- Error types: `connection`, `auth expired`, `banned`, `captcha`, `timeout`, `crash`
- প্রতিটা error site এর নিজের `logs/` এ যাবে (`sites/<site_id>/logs/errors.jsonl`)

#### Prompt 6 — Connect Supervisor to Existing Engine `[DONE — 38c2d98, da58842]`
- Runner এর chat cycle এ existing bot logic call করো:
  - WebSocket site → existing `realtime.go`
  - API site → existing `api_chat.go`
  - DOM site → existing `domchat.go`
  - Sequence mode → existing `message_sequence.go`
  - EVA mode → existing `eva_chat.go`
- `app.go` তে নতুন methods add করো: `StartSites(list)`, `StopSites(list)`, `StartAll()`, `StopAll()`, `GetSiteStatuses()` (এবং existing `StartBot`/`StopBot` রাখো backward compatibility এর জন্য)
- Bot engine কে `SiteContext` pass করো যাতে সে isolated folder ব্যবহার করে (session load, log write, chat history — সব site এর নিজের folder এ)

---

### PHASE 3: DASHBOARD UI

#### Prompt 7 — Tab Structure `[DONE — 45500f5]`
- Tab bar add করো (Target Site bar এর নিচে):
  - **Tab 1:** `"Control Center"` — global settings + start/stop + live log
  - **Tab 2:** `"Site Manager"` — site grid + group view + per-site detail
  - **Tab 3:** `"Sessions"` — session capture + health monitor
- Tab click এ content show/hide হবে
- Existing UI elements (Show Mode, Background Bot sidebar) Tab 1 এর ভিতরে reorganize করো
- Tab switching `app.js` এ handle করো এবং Active tab styling CSS এ add করো

#### Prompt 8 — Site Grid (Tab 2) `[DONE]`
- Grid structure:
  - উপরে: search bar + filter (`Group`, `Status`, `Session`)
  - Group wise collapsible sections (`A`/`B`/`C`/`D`/`E`) — প্রতিটা group header এ site count
  - প্রতিটা site row: `checkbox | site name | status badge | session badge | action buttons`
  - Group header এ: `Select All`, `Start Group`, `Stop Group`
  - নিচে: `Start All Selected`, `Stop All`, `Refresh`
- Status badges: `green=running`, `red=failed`, `yellow=paused`, `orange=banned`, `gray=stopped`
- Session badges: `✅ valid`, `⚠️ expiring`, `❌ missing`
- Action buttons per site: `Start`, `Stop`, `Open Browser`, `Detail`
- Go side: `GetSiteStatuses()` method যেটা সব site এর status return করবে
- Auto-refresh ৫ সেকেন্ড পর পর
- ১০২টা site handle করতে পারবে lag ছাড়া
- Site name click করলে right side এ detail panel open হবে (Prompt 9)

#### Prompt 9 — Per-Site Detail + Overrides `[IN PROGRESS — Awaiting Plan Approval]`
- Site click করলে right side slide-out panel open হবে
- Panel content:
  - Site name, URL, group badge, connection type
  - Session status + captured date + expiry
  - Stats: messages sent/received, uptime, error count
  - Last error message
  - Per-site override toggles:
    - Custom proxy (toggle + input)
    - Custom message mode (toggle + selector)
    - Custom reply delay (toggle + min/max)
  - Action buttons: `Start`, `Stop`, `Open Browser`, `Refresh Session`, `Open Folder`
- Override save করলে তাৎক্ষণিক apply হবে running site এ
- Go side: `GetSiteDetail()`, `SaveSiteOverride()`, `OpenSiteFolder()` methods
- Panel close: `X` button বা outside click

---

### PHASE 4: SESSION + BROWSERLESS

#### Prompt 10 — Session Capture (Tab 3) `[PENDING]`
- Tab 3 content:
  - Site list with session status (`✅`/`⚠️`/`❌`) + captured date
  - Per site: `Capture` button, `Re-capture` button, `Clear` button
  - Bulk: `Capture All Missing` (batch of 5)
  - Health summary: `Valid` count, `Expiring` count, `Missing` count
- Capture flow:
  1. User click "Capture" → Rod browser open হবে site এর isolated profile দিয়ে
  2. Browser site এর login page এ যাবে
  3. User manually login করবে
  4. Agent background এ capture করবে: `cookies`, `localStorage`, `sessionStorage`, `WebSocket URLs`, `auth headers` from XHR
  5. User "Save Session" click করবে (বা agent auto-detect করবে login success)
  6. Browser close হবে
  7. Captured data site এর `sessions/default/` folder এ save হবে
  8. UI update হবে `✅`
- Session files (site এর নিজের folder এ): `cookies.json`, `local_storage.json`, `ws_endpoints.json`, `auth_tokens.json`, `meta.json` (`captured_at`, `expires_at`, `method`, `status`)
- Go side: `CaptureSession()`, `ValidateSession()`, `ClearSession()`, `GetSessionStatuses()`
- Browser profile MUST be site-isolated (Prompt 1)

#### Prompt 11 — Browserless Chat `[PENDING]`
- Browserless connection module বানাও:
  - Session load করো site এর folder থেকে
  - Session validity check করো (expired কিনা)
  - Site এর chat method অনুযায়ী connect করো:
    - WebSocket: WS URL dial করো auth headers সহ
    - API: HTTP client বানাও cookies সহ
    - DOM: Headless Rod launch করো cookies inject করে
- Supervisor এর chat cycle এ browserless connection ব্যবহার করো
- Session expired হলে site auto-pause হবে, UI তে `"needs session"` দেখাবে
- Reconnection same session দিয়ে try করবে (re-login না)
- Group A/B/C sites browserless চলবে (no visible browser)
- Group D headless browser এ চলবে (background, no UI)
- Group E manual mode এ থাকবে
- Existing `bot/realtime.go`, `api_chat.go`, `domchat.go` কে connection object accept করতে update করো

---

### PHASE 5: SELF-RECOVERY + POLISH

#### Prompt 12 — Agent Self-Recovery (আটকে গেলে নিজে browser চালাবে) `[PENDING]`
- **Scenarios A–E:**
  - **Scenario A (Session expired):** Rod browser launch (isolated profile) → login page → auto-login try → captcha থাকলে user notify → login success হলে session capture → browser close → chat resume
  - **Scenario B (Auto-signup fail):** Browser launch → signup page → form fill (fake data) → captcha check → account created → session capture → browser close → chat start
  - **Scenario C (WS/API broken, DOM fallback):** Headless browser launch → saved session load → DOM selectors re-detect → `dom_driver.js` এ save → DOM chat start → user notify `"switched to DOM mode"`
  - **Scenario D (Site structure changed):** Browser launch → page inspect (`skills/dom_auto_scout.js`) → new chat input/send/container detect → `dom_driver.js` update → test chat → save/notify
  - **Scenario E (Complete failure):** Visible browser launch → user notify `"I'm stuck, please login manually"` → user login → capture → browser close → chat resume
- Recovery limits: প্রতি site এ max ৩টা auto-recovery attempt; ৩ বার fail হলে `"needs_human"` mark হবে ও UI notification যাবে
- Go side: `RecoveryManager` struct with `RecoverSite(siteID, errorType)` integrated with `Supervisor`
- UI side: Recovery progress indicator, captcha notification, recovery success/fail notification

#### Prompt 13 — Combined Live Log + Error Dashboard `[PENDING]`
- Bottom bar (সব tab এ visible):
  - Combined log feed — সব site এর log একসাথে (`timestamp + site name (color coded by group) + type (OUT/IN/ERR/SYS) + message`)
  - Filter: by site, by type, by group
  - Max ৫০০ lines, auto-scroll, Clear button
- Go side: log event এ `siteID` field add করো
- Error summary in Control Center: Per site error count, Error type breakdown (`connection`/`auth`/`ban`/`captcha`/`timeout`), Sites needing attention list, Recovery status per site

#### Prompt 14 — Bulk Operations + Add Site Wizard `[PENDING]`
- Bulk operations (Control Center এ):
  - `Start All Ready`, `Stop All`, `Auto-Register All Group B`, `Login All Group C` (batch 5), `Refresh All Sessions`, `Full Report`
  - Parallel execution (max 10 concurrent) + progress bar + individual failure resilience
- Add Site Wizard (Site Manager এ `+` button):
  - Step 1: URL input → Step 2: Auto-probing (spinner + classification) → Step 3: Setup execution (step-by-step progress) → Step 4: Result (`ready` / `needs login` / `failed` + next action)

---

### PHASE 6: FINAL

#### Prompt 15 — Settings Sync + Cleanup `[PENDING]`
- Main dashboard এবং `apichat.exe` এর মধ্যে settings sync (`sites/_global/settings.json` ↔ `apichat.exe`)
- Unused RPC methods cleanup
- ১০২টা site folder verification (`site.json`, `classification`, `accounts`, `sessions`, `profiles`, `logs`, isolation rule)
- `docs/PROJECT_MAP.md` update এবং `docs/MULTISITE_GUIDE.md` তৈরি

#### Prompt 16 — Full Integration Test `[PENDING]`
- ৩টা mock site (Group A, B, C) add করে end-to-end integration test:
  - Isolated goroutine start, single-site crash isolation, global settings pickup, per-site override isolation, session capture mock, browserless connection mock, self-recovery mock, cross-site access block
- `go test ./...`, `npm test`, `verify.bat`, এবং `chitchatui.exe` + `apichat.exe` final build verification
