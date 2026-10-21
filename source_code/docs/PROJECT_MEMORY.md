# Project Memory — decisions and current state

Purpose: when this project is shared with another agent/model, this file plus
`AGENTS.md` and `docs/PROJECT_MAP.md` should be enough to understand where
things stand without re-asking the user. The AUTO block below is refreshed by
`scripts/project-map.cjs`; the decision log is manual — **append an entry
after every meaningful change** (newest last). Never rewrite old entries.

<!-- AUTO:BEGIN -->
_Auto status 2026-10-21. Do not edit this block by hand._

- Source fingerprint: `028987b919e9fb0f…`
- Browser options: `rod`, `chromedp`, `show_mode`
- Capture tool: v1.2.1
- Last verification: no report found yet (`npm run verify`)
<!-- AUTO:END -->

## Decision log

- (template) YYYY-MM-DD — what changed, why, and how it was verified.
- 2026-10-14 — Browser direct-run: numbered Rod/ChromeDP rows each with their
  own Run button; shared Profile/Count (1–5)/Headless; accepted settings are
  remembered. Browser-free verification system added (see docs/VERIFICATION.md).
- 2026-10-14 — Show Mode added as Browser option 03: a visible Rod preset.
  Backend normalizes `show_mode` → `rod` + `headless=false` before saving or
  dispatching, so saved sessions stay compatible. Not an anti-detection mode.
- 2026-10-14 — Standalone manual capture tool added under `capture/`
  (dependency-free Node + the user's Chrome/Edge via CDP). Captures HTTP,
  WebSocket frames, DOM message hints and storage, redacted by default, into
  `capture/capture-sessions/` (excluded from source packages). Purpose: the
  user captures real traffic manually so future WebSocket/API automation can
  be built from evidence instead of guesses.
- 2026-10-14 — Project map/memory system added: this file plus
  docs/PROJECT_MAP.md are auto-refreshed by scripts/project-map.cjs on every
  `npm run verify`. Capture tool gained a WebSocket event catalog, manifest
  schema fingerprints and a session compare tool (`capture/tool/compare.cjs`)
  so a future chitchat.gg API/WebSocket change can be diagnosed by capturing
  again and diffing against a baseline instead of starting from zero.
- 2026-10-14 — Capture tool v1.2.0 after first real Windows run failed with a
  bare "fetch failed" (DevTools endpoint was polled only once). Fixes: free
  port selection + 45s DevTools polling with browser-stderr `browser.log`,
  `tool/doctor.cjs` smoke test (actually launches the browser via CDP),
  one-click `install.ps1` (winget → MSI fallback for Node, winget Chrome
  fallback), live capability checklist + `[live]` status lines in the
  terminal, and a session `SUMMARY.md`. User machine: Node 24 + Microsoft
  Edge confirmed by the old install check.
- 2026-10-14 — First real ChitChat capture analysed end-to-end and distilled
  into `docs/CHITCHAT_PROTOCOL.md` + `docs/chitchat-protocol.json` (baseline
  session 20261014-185017, schema fingerprints recorded there). Proven facts:
  Socket.IO `chatMessage` = incoming; sending = `POST
  /users/me/conversations/:id/messages` multipart `{content, nonce}`;
  skip = `PATCH /match/disconnect` + `POST /match`; session is cookie-based.
  Existing `internal/bot/realtime.go` speaks the same socket protocol, so the
  capture confirms it. Show Mode live-chat design (in-browser, CDP frames +
  page-context fetch, no DOM selectors) is documented in the protocol file
  and awaits user approval. Capture redactor no longer hides `author` fields
  (v1.2.1).
- 2026-10-14 — Show Mode live chat implemented (user-approved, evidence from
  docs/CHITCHAT_PROTOCOL.md): `BrowserSettings.LiveChat` is set only by the
  Show Mode preset; after login save, `app.runLiveChat` keeps the Rod browser
  open and chats in place — incoming from CDP Socket.IO `chatMessage` frames
  (`internal/browser` `ChatMessageSource`, Rod-only), replies from the
  selected mode (EVA engine via exported Start/Reply; sequences walk their
  lines; EVA AI compose not applied live), sent via the page's own
  authenticated fetch (multipart `content`+`nonce`), skip via
  `PATCH /match/disconnect` + `POST /match`. Closing the browser stops it.
  Browser-free fake-backend tests cover greeting/reply/own-ignore/dedupe/
  skip; full Go compile and live Windows run still pending user testing.
- 2026-10-14 — Show Mode live fix after the user's first real test (Folder
  Sequences sent one line, incoming never detected): ROOT CAUSE — the real
  `chatMessage` author is an object `{id, username}` (as the existing
  realtime.go parses), but the new live parser declared it a string, so
  every real frame failed to parse. Parser now accepts object/string/null
  authors (isolated tests pass with the real shape). Also: API
  history-polling fallback for detection, history seeding so old messages
  are never answered, Folder mode picks one sequence per conversation
  (not all files flattened), transient-empty partner-left tolerance, and
  live INCOMING/SEND/SKIP logs at ACTIVITY level so they show on Home.
- 2026-10-14 — Show Mode rebuilt DOM-only (user explicitly retired the
  WebSocket/API approach: "only browser diye, no websocket api"): the old
  `internal/browser/livechat*.go` transport was deleted; Show Mode live chat
  now reads message rows from the page DOM, types replies into the message
  box, and clicks Skip/START (`internal/browser/domchat.go` +
  `live_chat.go`). Structural heuristics: rows are elements containing a
  `10:24 PM`-style time whose first line is the author; input found by
  "message" placeholder; buttons matched by label. Runner seeds history,
  dedupes by author+text counts, suppresses own echoes, clicks START in the
  lobby, logs a one-line page probe when no chat box is visible. Verified:
  full Windows build (`GOOS=windows go build ./...`) PASSED in a temp copy
  using GitHub-mirror replaces for the blocked vanity module paths (real
  go.mod untouched); Go tests compile and `go vet` is clean; the runner
  state machine was EXECUTED against a fake DOM backend in a throwaway
  Linux harness (START→greeting→reply→echo-suppression→skip→re-match→
  partner-left; it caught and fixed a 6s START-throttle that delayed
  re-matching); jsdom simulation of the DOM heuristics 12/12 PASS; Node
  source tests PASS. The protocol docs remain as evidence only; real DOM
  behaviour on the live site awaits the user's Windows retest. Options
  01/02 untouched.
- 2026-10-14 — HANDOFF / session close (user request): DOM Show Mode
  packaged as `root-project-showmode-dom-UNVERIFIED.zip` (repo root;
  integrity: `node scripts/package-source.cjs --check <zip>`). Full
  `root-project/` source is committed to git from this push on (it was
  previously delivered as zip-only). **Next session start:** read
  AGENTS.md + docs/PROJECT_MAP.md + this file + docs/SHOW_MODE.md.
  **Awaiting:** user's Windows retest of the DOM build. If detection fails,
  the Home log's `Page probe:` line shows what the bot saw — fix the
  heuristics in `internal/browser/domchat.go` from that evidence; do not
  revive the WebSocket/API route for Show Mode.

- 2026-10-14 — Owner authorized targeted Show Mode incoming-message fix. Replaced clock/name guessing with an embedded exact-selector DOM observer, stable row IDs, grouped-author parsing, safe self/partner classification, serial queue and native Rod input/Enter confirmation. Generation checks prevent stale reply/skip; uncertain sends pause without automatic retry. Only domchat.go, new showmode_dom.js and live_chat.go are production changes; local live adapter renders configured handles and applies the existing EVA no-username gate without changing worker/AI core. Tests/wiring/docs updated; all other original files byte-compared against the original ZIP. New Node suite 52 PASS; isolated production-runner/Rod-boundary Go race tests/vet and Windows-target test compilation PASS, not full-app verification. Full standard gate BLOCKED by dependency proxy EOF; Windows live/site NOT_RUN. See SHOW_MODE_FIX.md. Source is now actually extracted under root-project/ in this repository; original ZIP unchanged, new download remains UNVERIFIED.

- 2026-10-14 — Owner reports the delivered Show Mode v2 works fully in their
  Windows retest (user report, not a sandbox live run). Keep that chat/browser
  implementation untouched. Next authorized scope: Home logs only. All App.log
  levels now append to `logs.txt` beside the active `data/` directory, independent
  of Badger/UI persistence; main routes Go and emitted Wails diagnostics there.
  Home log rows/rendering removed; counters/actions and Browser status events
  preserved. Frontend backend-call failures/uncaught errors use the existing Wails
  logging bridge without serializing request/settings arguments. Text file uses
  UTF-8 JSON lines with UTC time, level, source and untruncated message; appends
  across sessions and survives Clear Logs/Clean. No automatic rotation/truncation.
  Known credential forms are masked; review personal chat data before sharing.
  Disk-full/permission failures are reported to stderr (cannot promise a file
  write when the OS refuses it); Wails' existing framework log-level filtering
  is unchanged. Go logger tests/race/vet and Windows-target test compilation PASS;
  Node suite 57 PASS. Real root App/Badger tests and full desktop verification
  remain dependency-blocked; Windows runtime retest of this logging change is
  NOT_RUN. No chat/worker/driver/license/model/DB schema/binding changes. Added
  runtime logs.txt Git/source-package exclusion and scoped regression coverage.
  No new ZIP or remote push was requested for this step; previous working ZIP
  stays unchanged.

- 2026-10-15 — Owner requested restore of the working project and completion
  of Home file logging after audit found missing frontend assets and uncaptured
  driver stdout. Restored all missing UI assets from the owner-tested v2 ZIP;
  styles, Show Mode/browser/worker/license/proxy/model/store/lockfiles remain
  unchanged. Reapplied only Home log removal and frontend error forwarding.
  Desktop-only stdout/stderr capture now retains printed driver diagnostics,
  drains partial/long lines at normal shutdown and uses original stderr on disk
  errors (no recursion). Go SetCrashOutput appends unhandled goroutine panic/
  fatal runtime stack text directly to logs.txt; ordinary records remain JSON
  lines, so crash files may mix JSON lines and plain text. Known-token redaction
  applies to ordinary records, not raw runtime crash dumps; review before sharing.
  Wails INFO/WARNING/ERROR enabled, raw RPC DEBUG/TRACE deliberately not enabled.
  58 Node tests PASS; 13 actual Go logger race tests and vet PASS; Windows-target
  logger tests compile only. Full standard gate dependency-BLOCKED; root App/
  Badger and native Windows logging test NOT_RUN. See FILE_LOGGING.md. A separate
  UNVERIFIED source archive preserves all frontend files; previous working ZIPs
  kept unchanged. No remote push or main merge implied.

- 2026-10-15 — Owner approved read-only Home live User/Bot transcript plan.
  Implemented separate in-memory bounded feed from actual Show Mode DOM rows,
  ID/generation/run dedup, full-text safe UI render, row ordering reserved before
  hydration, multi-browser/conversation selector, closed-session replay and
  coalesced Wails notifications plus one-second resync. Optional native-send
  snapshot hook retains final actual own row before Skip; no intent/draft is
  displayed. runLiveChat only wraps/starts/stops a passive 250ms observer; all
  reply/queue/rotation decisions and shared drivers/worker/AI/license/model/stores
  remain unchanged. Diagnostic logger/logs.txt remains separate and unchanged;
  raw SMS is not persisted. History: 500 messages/session, 12 sessions, visible
  retention notices; desktop restart clears memory. Node 62 PASS; isolated real
  production transcript/runner/Rod compilation and race tests/vet PASS; actual
  logger race regressions PASS. Full standard gate dependency-BLOCKED and native
  Windows display acceptance NOT_RUN. See HOME_CHAT.md for scope, test boundaries
  and usage. Owner approval preceded all code edits; no remote push implied.

- 2026-10-15 — Owner requested fresh all-update recheck and full source ZIP.
  No production code or dependencies modified. Node 62 PASS; Show Mode subset
  separately 19 PASS; capture mock tests 12 PASS; actual logger's 13 race tests,
  vet and Windows-target compilation PASS. Actual standard-library package
  regressions/vet (workerplan/runtimeassets etc.) and standalone apitest unit
  tests/vet PASS; no real API/browser run. Isolated exact transcript/runner/real
  pinned Rod race tests/vet/Windows-target compilation repeated PASS; these are
  NOT full App/Badger/Wails proof. Full standard gate remains dependency-BLOCKED;
  native Windows/live display acceptance NOT_RUN. See RECHECK_2026-10-15.md. Do
  not call all checks green or rename the source archive VERIFIED. Previous
  working ZIPs unchanged; rechecked test-only download remains UNVERIFIED.

## Known limitations (keep honest)

- Full Go/Windows verification depends on module downloads and a Windows
  environment; sandbox runs may legitimately end BLOCKED. BLOCKED is not PASS.
- Live browser/login behaviour is only proven by the user's manual Windows
  test unless explicitly stated otherwise.

## How to add memory

1. Run your change and its tests.
2. Run `npm run map:update` (or `npm run verify`, which does it first).
3. Append one dated bullet to the decision log above: change + reason +
   verification evidence. Keep secrets, tokens and account data out.

## 2026-10-15 — HTTP/SOCKS5 and dashboard Test Proxy

Owner requested authenticated HTTP/SOCKS5 format support and in-dashboard tests.
Added safe parser formats/IPv6/aliases, plain-line default protocol selector,
unsaved 20-line/four-worker HTTPS checks with sanitized per-line results, and
one loopback relay per Rod/Show Mode/ChromeDP proxied browser. Both test and
browser use the same upstream CONNECT/SOCKS5 auth implementation; no direct
fallback. Cleanup, TLS, failure classification and literal UI rendering tested.
65 Node tests PASS; actual proxy race/vet PASS; actual browser/proxy sources
compile/race/vet and Windows-target test compilation in pinned-dependency isolated
module PASS. Not a native Wails/Windows/owner-proxy verification. Full gate BLOCKED.
See PROXY_TEST.md. Latest full source: root-project-proxy-dashboard-UNVERIFIED.zip.
No dependency lock, chat/AI/Home/worker/license changes. No new remote push.

## 2026-10-15 (later) — provider rejection diagnostics for Test Proxy

Owner reported a real Webshare line failing the dashboard proxy test with
`HTTP proxy rejected CONNECT (402)` and asked to stop looping and settle it.
Investigation result: the CONNECT request the app sends was verified
byte-for-byte (`CONNECT host:port HTTP/1.1`, `Host`, `Proxy-Authorization:
Basic …`) and matches curl's shape, so the 402 is the provider's own answer and
the app was previously throwing that answer away. Fix: `TunnelError` now carries
the proxy's status, `Proxy-Authenticate` and a bounded (512 B / 1.2 s) body
excerpt, sanitized to one credential-free line (tags stripped, control chars
removed, username/password and Basic token redacted, 200-char cap); the test
result gained `hint`, `provider_message`, `proxy_authenticate` and
`authenticated`; the dashboard renders "Provider said"/"Auth challenge"/"Next
step" and, for a line without credentials, says so explicitly. No new network
request was added and no exit IP is collected. `internal/proxy/diagnostics_test.go`
covers capture, redaction, sanitizing, truncation, 402/407/no-credential hints
and the network-failure hint. Evidence: `go test -race ./internal/proxy`,
`go vet ./internal/proxy`, `go build ./internal/browser`, 65 Node tests PASS.
The full standard gate stays dependency-BLOCKED (badger's
`golang.org/x/net`, `google.golang.org/protobuf`, `go.opentelemetry.io/otel`
vanity paths are unreachable from this sandbox), and the owner's real
proxy/Windows acceptance is still NOT_RUN. Behaviour docs: PROXY_TEST.md.

## 2026-10-15 (later) — Show Mode reply delay follows saved settings

Owner reported that Show Mode replied too quickly despite configuring reply
timing and asked for the browser runner to follow Settings. Removed the fixed
900 ms wait: `live_chat.go` now loads saved General Settings when Show Mode
starts and chooses a delay from Reply delay min/max before each incoming-message
reply, then retains the conversation/composer recheck. The opener remains
immediate; `0`–`0` means no intentional wait; settings are reloaded on the next
Show Mode run. Added selected-delay ACTIVITY logs, a Settings-page explanation,
Show Mode docs, and fake-runner/range tests. `npm test`: 65 PASS. This sandbox
has no Go toolchain, so the new Go tests/full Go verification are BLOCKED here;
real Windows/live-site testing is NOT_RUN. No transport, worker, settings schema,
license, proxy or dependency changes.

- 2026-10-17 — Owner approved independent timing settings under two Setting
  sub-pages. Extracted editable root-project source from the tracked original
  UNVERIFIED ZIP (original ZIP unchanged). Added ShowModeSettings with separate
  Badger storage, serialized one-time legacy reply-delay migration, independent
  Save API/forms and restart notices. Standard worker retains all its existing
  timing controls including its own reply delay. Show Mode runner now reads
  only its profile and supports opt-in no-reply auto skip and next-chat wait;
  incoming queues, generation checks and uncertain-send pauses remain guarded.
  AI/chat engines, browser drivers, login and proxy untouched. npm tests: 69
  PASS. Added Go store/App/runner regressions; NOT_RUN without Go toolchain.
  Native Windows and live-site acceptance NOT_RUN. See SETTINGS_PROFILES.md.

- 2026-10-17 — Owner approved scoped first-sender controls in BOTH timing
  profiles (Bot/User/Random + bot-first percentage and separate first-user wait).
  Added independent nested policy values, validation, conditional UI and legacy
  bot-first defaults. Show Mode waits without consuming sequence line 1;
  already-present partner input takes priority and EVA sees it in NEW stage.
  Worker first decisions/deadlines persist per account/conversation; a targeted
  first-match gate bridges initial input into existing engines and dedupes the
  corresponding realtime event. DM poll/realtime paths suppress idle user-first
  openers; timeout never deletes DMs. No driver/login/proxy/license/AI/EVA engine
  changes; original module locks unchanged. Node: 72 PASS. Original model tests
  PASS. Temporary pinned-GitHub-mirror copy: model/bot/uistore race tests PASS;
  production App/live/transcript/runner tests with forbidden-access Windows WMI
  boundary PASS; Windows cross-build and scoped vet PASS. Original full release
  gate is not certified; native/live-site acceptance NOT_RUN. New feature not
  pushed or packaged yet. See docs/FIRST_MESSAGE_SETTINGS.md for boundaries.

- 2026-10-17 — Owner requested packaging and GitHub push of first-message
  controls. Preparing clean source-only `root-project-first-message-final-UNVERIFIED.zip`
  with current verification evidence and checksums. Runtime/private data and
  build outputs remain excluded. Original release gate remains BLOCKED; prior
  isolated checks are documented separately, not substituted for certification.

- 2026-10-18 — Owner set a hard workspace boundary: agents may access only
  `C:\Users\papi\Desktop\New folder` (plus build toolchain/package-cache and
  OS-temp reads strictly needed to compile and test). Recorded at the top of
  AGENTS.md and as hard rule 7; no files outside the workspace may be changed.

- 2026-10-18 — Final audit + release candidate. Dashboard consolidated into
  three tabs and now opens with a Background Bot / Show Mode chooser; the
  choice only switches visible controls (UI state), never backend wiring.
  `SaveUnifiedSettings` maps one form onto both stores without
  merging keys or execution paths. Fixed: the Windows-only applog capture test
  (release the crash-output handle before replacing the path), Save Assets now
  also persists the login-browser defaults, Folder mode is blocked until
  sequences are loaded, bot-first percentage is no longer treated as a profile
  mismatch, and a per-copy WebView2 profile (`data/webview`) prevents
  `0x800700AA` when two copies run. Also fixed a real UI-log loss bug: two
  entries written in the same clock tick shared a Badger key; log keys now
  carry a monotonic sequence. Rewrote the Node UI/contract/settings
  suites for the 3-tab contract and updated the docs. Evidence: `npm test`
  68/68 PASS; `npm run verify` PASS 13/13 (Go regression tests, vet, Windows
  cross-build, app tests/vet, production Wails build); the built EXE was
  executed and all three tabs plus Refresh were verified. `liveLogin` and a
  real Show Mode chat remain NOT_RUN.

- 2026-10-18 — Show Mode rebuilt as its own page with exactly five sections
  (Setting, Proxy, Message, Run Browser, Live Logs); Background Bot is the
  second page. Run Browser saves show/proxy/message and launches in one click;
  Show Mode writes `settings/show_mode/v1`, Bot Settings writes
  `settings/general`. Added `show_log.go` (bounded rule-aware feed,
  `GetShowModeLog`, `ClearShowModeLog`, `ui:showlog`) and `liveReply` metadata
  in `live_chat.go` (sequence line, EVA pool, chosen delay vs setting, skip
  reason). Node suite is now 64 tests. Verification: Go tests PASS, Node 64/64,
  `npm run verify` PASS 13/13; the EXE was run, Run Browser opened the browser
  and Live Logs showed the applied settings. Browser runs now start at
  `https://app.chitchat.gg/text?auth=email` instead of the old landing page.

- 2026-10-18 — Show Mode reply fix after a real run: the current site no longer
  renders the "Open profile for" account control, so the observer reported
  `ownName=""` and the runner refused to send (`DOM WAIT`). The runner now
  proceeds when the partner identity is known, logs an `identity fallback`
  entry, and still ignores non-partner/echo rows. Added the regression
  `TestShowModeRepliesWithoutOwnProfileControl`; the real page snapshot
  confirmed `phase=chatting`, `canSend=true`, `ownName=""`.

- 2026-10-18 — Second live finding after login: chat started and the partner
  row was accepted, but every send hit `SEND UNCONFIRMED`. The send path still
  required `OwnName` and send confirmation matched the new row author to it.
  Removed that send guard and added a fallback: when `OwnName` is unavailable,
  a new unlabelled row with the exact text is our confirmed send (partner rows
  stay rejected). Added `TestShowModeSendConfirmationWithoutOwnProfileControl`.
  Logs now include the concrete send error in `SEND UNCONFIRMED`.

- 2026-10-18 — Modern DOM detection fix (verified live): the updated site wraps
  partner rows in a `pt-(--message-group-gap)` group with a name/profile header,
  while own rows have neither and no own profile control exists. Stopped
  carrying the previous author into those own rows (they were misclassified as
  the partner, which broke send confirmation and risked self-replies). Send
  confirmation now accepts a new row with the exact text that is not attributed
  to the partner, and waits up to 8s for the delayed echo. Live run confirmed
  `CHAT STARTED → SEND CONFIRMED → IN → REPLY DELAY → SEND CONFIRMED` over
  several exchanges.

- 2026-10-18 — Saved-profile auto-login (verified live): `ListProfiles` exposes
  only profile names/session metadata (never cookies). Show Mode injects the
  newest saved login for the selected profile (`RodBackend.SeedSession`:
  cookies + localStorage/sessionStorage, then navigate to `/text`), verifies
  `SessionAuthenticated`, and skips manual login. Failure falls back to the
  manual flow. The UI shows a profile datalist with a session hint. Also added
  auto-click for the post-login "Start Text Chat" landing screen. Live run:
  "Session restored ... no manual login needed" → START clicked → CHAT STARTED.

- 2026-10-18 — `default` profile is now the explicit fresh option: it never
  injects a session even if `data/profiles/default/login_info*.json` exists;
  Run Browser waits for manual login. `ListProfiles` reports it as
  `fresh_only` and other profiles keep session metadata. Updated Go + UI tests,
  SHOW_MODE/START-HERE docs.

- 2026-10-18 — Superseded by an explicit **Login mode** selector:
  `BrowserSettings.UseSavedSession` is saved with the browser settings.
  **Fresh browser** skips injection (and still auto-saves the session after a
  successful manual login); **Use saved session** injects the newest session
  for the selected profile (default included). `fresh_only` was removed from
  profile metadata; docs/tests updated.

- 2026-10-18 — Final UX: the Show Mode **Profile dropdown itself** has
  `default — fresh browser (login)` and `session — default account` as separate
  options, plus other profile directories. Choosing `session` runs the default
  directory with `UseSavedSession=true` (newest default session injected,
  auto-login); `default` runs it fresh with manual login and saves the session
  for later. No separate Login-mode control remains.

- 2026-10-18 — Profile UI changed to **two radio choices** (no dropdown):
  `(•) default  ( ) session` + Run Browser. `default` = fresh login;
  `session` = saved default session auto-inject. Selecting a radio drives
  `UseSavedSession`; the choice is remembered in browser settings. HTML/app.js
  tests updated; contract IDs keep `sm-profile` as the radio group.

- 2026-10-18 — Banned-account detection: `browser.BanChecker` checks the
  `/banned` URL (Ban ID/Reason/Remaining parsed from the page text). Login wait
  and the live runner (every 2s) stop on detection, emit red `BANNED`, and
  delete the profile's `login_info*.json` sessions. Live DOM simulation
  verified: BANNED logged, 9 session files removed, browser closed. Rod and
  ChromeDP implement the check.

- 2026-10-18 — Real banned-account live login confirmed the detector
  (`BANNED | Ban ID: 6a768321... · Remaining: Permanent`, browser closed).
  Refined deletion: the `/banned` page's **User ID** is parsed and only that
  account's `login_info*.json` is removed, so other accounts in the same
  profile keep their sessions. Regression test added.

- 2026-10-18 — Nested navigation refactor: Level 1 sidebar keeps Show Mode /
  Background Bot; Level 2 subnav lists Show Setting/Proxy/Message/Run/Live Logs
  and Background Login/Proxy/Message/Bot Settings; Level 3 renders only the
  active `.subpanel`. Added a global `formStore` (`show`/`bot` slices) bound to
  every input, plus save/restore on subpage switches, so typed values survive.
  Background stats/header and Start/Stop stay persistent above the subpages.

- 2026-10-18 — Wrapper restructure: the clean root is
  `ChitChat-App/` with only `install.bat`, `run-dashboard.bat` and
  `source_code/`. The root bats `cd /d "%~dp0source_code"` and proxy to the
  real scripts. Product markdown (README/START-HERE/TEST_REPORT/EVA guide) moved
  into `source_code/docs`; `AGENTS.md` stays at source root (agent hard rules),
  `logs.txt` stays at source root (runtime), and Go/config/test files stay in
  `package main` to keep Wails bindings intact. `npm run verify` PASS 13/13
  after the move; root proxy install/build and run were smoke-tested.

- 2026-10-20 — Background Bot **Live Logs** added (owner request: Show Mode
  had "5 · Live Logs", Background Bot did not). New `bot_log.go` keeps a
  separate 500-entry in-memory feed; `buildBotConfigs` now mirrors the worker
  `Logf`/`Activityf` callbacks into it, so `SEND | …` becomes OUT,
  `RECEIVE | …` becomes IN, skip lines become SKIP, failures become ERR and
  lifecycle messages become SYS. `StartBot` clears the feed per run and records
  start/stop/worker-end events; `handleBanned` adds the red BANNED entry.
  New RPCs `GetBotLiveLog(after)` / `ClearBotLiveLog()`; the dashboard adds
  `bg-logs` (nav + subpanel) with the same All/SMS/Rules/Errors filter,
  Clear button and 1 s polling only while the Background tab is open.
  Show Mode feed untouched apart from a shared `renderLogFeed` helper.
  Verified in the Linux sandbox with `node --check` and `npm test`
  (72/72 PASS, two new Background Bot log tests); Go syntax parsed with
  tree-sitter (82 files, 0 errors) because no Go toolchain exists here.

- 2026-10-20 — **API Chat** added as a separate fourth mode; EVAChat Flow is
  now pure TXT (owner: "api option and evaflow option alada thakbe"). New
  `internal/apichat/` package ports the apitest director: `signals.go`
  (tiers/regexes/ASL), `director.go` (interest scoring, mood rotation, all
  gates and middle-chat branches, double lines, snap hook/ask/share/close),
  `sanitize.go` (think/quote cleanup, parrot and AI-talk retry, emoji polish),
  `prompt.go` (editable persona + director note), `client.go`
  (OpenAI-compatible transport, no-think fields, 403/empty-reply hints),
  `session.go`, `engine.go` (one guard retry, fallback deck).
  Wiring: `internal/bot/api_chat.go` (per-user sessions in
  `AccountState.APIChatSessions`, username always rendered by Go),
  `internal/bot/api_assets.go` + `api_assets/{persona,persona_countries,share,
  fallback,README}.txt` seeded to `data/api_chat/` without overwriting edits,
  `app.go` (`LoadAPIChatConfig`, `TestAPIChat`, `OpenAPIChatDataFolder`,
  worker config), `live_chat.go` (`apiLiveResponder` for Show Mode), and
  `logic.go` (`statefulChatStart/Reply` + `executeMatchStatefulChat` shared by
  both modes). The old `eva_ai.go` hybrid layer was deleted and the dashboard
  AI block was replaced by the API panel in both Message pages. Settings reuse
  `AIBaseURL/AIAPIKey/AIModel` so old keys keep working, plus
  `APITemperature/APIShareHot/Warm/Cold`. Missing key in API mode refuses to
  start; the model can never write a username or `@`.
  Verified in the Linux sandbox with `node --check`, `npm test`
  (74/74 PASS incl. new mode + API asset contracts), wasm gofmt and
  tree-sitter (93 Go files, 0 syntax errors). No Go toolchain here, so the
  first real compile happens on the owner's Windows PC via `install.bat`.

- 2026-10-20 — **API Chat quality phase** (owner approved the deep plan with
  "apni jeta valo mone koren vai, sei vabei kore den"): the model now gets a
  structured picture of every SMS and the reply gate rejects cheap mistakes.
  New `internal/apichat/{understanding,ledger,repeat,guard,style,goal}.go`:
  `MessageProfile` (kind/implicit ask/command/parts/keywords/topics/sentiment),
  a persisted `Ledger` (`age/gender/place/name/doing/snap` never asked again,
  question types tracked per turn), Jaccard anti-repeat (0.62 threshold, same
  opener, same question within two replies), a single `validateReply` gate
  (anchor, answer-before-ask, one question per line, no AI talk) with one retry
  carrying the reason and an intent-based `safeReply` fallback, five style
  archetypes with user mirroring plus a −3…+5 topic interest score, and the
  RAPPORT→INTEREST→HOOK→ASK→SHARE→CLOSE stage ladder with a per-reply goal.
  Share deck moved to `api_assets/sharesnap.txt` (18 clean playful lines,
  `%username%` only), rotated per session (`UsedShareLines`, never the last
  line twice, deck reset only when exhausted); the legacy `share.txt` is read
  only when `sharesnap.txt` is missing. Snap nagging stops after two ignored
  asks and closes politely. `prompt.go` became prompt v2 (`[what they just
  sent]`, parts, keywords, `[known]`, `never ask`, style, stage/goal, last
  three bot lines, retry reason). Session JSON adds Style/Interest/Ledger/
  Turn/SnapMentions/SnapDodges/UsedShareLines so a restart keeps the same
  behaviour; `logic.go` gained the API branches for realtime first-message
  gating. Docs: `docs/API_CHAT_QUALITY.md` (Bengali) + updated
  `API_CHAT_MODE.md`/`AGENTS.md`. Verified here with `npm test` (76/76), wasm
  gofmt, tree-sitter and the unused-import scan; no Go toolchain, so the first
  real `go build` stays on the owner's PC.


- 2026-10-20 — **Local test chat** (`test.bat`) added as the final owner-facing
  tool. Double-clicking it opens a console menu with the same four Message
  options as the dashboard (Single Sequence, Folder Sequences, EVAChat Flow,
  API Chat); picking one lets the owner type as the stranger and see the bot's
  real replies. New `internal/bot/localchat.go` reuses the production paths
  (`statefulChatStart/Reply`, `renderMsg`, `getActiveSequence`,
  `LoadAPIChatConfig`, `LoadEVAChatEngine`) and assigns usernames through
  `getOrAssignUsername`, so `%username%` renders exactly as in a real run. New
  `cmd/chitchattest/main.go` reads the saved settings from `data/` (badger is
  closed again before the chat), resolves `data/` from the working directory,
  and prints a clear hint if the dashboard is still running. Nothing is sent
  to the real app and no `bot_state.json`/flag files are written. `test.bat`
  (root wrapper + `source_code/test.bat`) checks/builds `chitchattest.exe`,
  forwards an optional mode argument (`test.bat 4`), and pauses on errors.
  Guide: `docs/LOCAL_TEST_CHAT.md`; `install.bat` now prints a hint about it.
  Added Go tests for the menu, sequence rendering, username replacement, the
  API-key hint and the "no bot state" guarantee; Node contracts check the
  four labels match the dashboard and that the harness never writes state.
  Verified with `npm test` (77/77), gofmt, tree-sitter and the unused-import
  scan; the real Go compile still happens on the owner's PC.

- 2026-10-20 — **Dashboard `.exe` build/launch fix, license removal, and codebase cleanup**.
  Diagnosed and fixed the root causes preventing `chitchatui.exe` from building
  or opening via `install.bat` and `run-dashboard.bat`, and removed the license
  requirement across the entire project:
  (1) **Removed license requirement & fixed Windows COM / WebView2 startup crash**:
  `go-webview2`'s `edge.init()` locks the main OS thread (`M0`) and initializes
  COM in STA mode (`COINIT_APARTMENTTHREADED`). Previously, `NewApp` called
  `license.GenerateHardwareKey()` (`wmi.Query`) on `M0` (which tore down `M0`'s
  STA COM state via `ole.CoUninitialize()`) and `license.VerifyLicense()`, and
  `app.startup` called `runtime.WindowSetSize(500, 350)` from a background
  goroutine (`RPC_E_WRONG_THREAD` -> `os.Exit(1)`) or blocked the dashboard behind
  `#license-overlay`. Completely removed the license check, 24h license ticker,
  `internal/license/license.go`, `UISnapshot` license fields, and the `#license-overlay`
  lock screen in `cmd/chitchatui/frontend/dist/{index.html,app.js,styles.css}`;
  set `ResizeDebounceMS: 10` in `main.go`; and updated `resolveDataDir()` so
  `data/` resolves from the project root even when `build\bin\chitchatui.exe`
  is launched directly.
  (2) **Installer & launcher batch fixes (`install.bat`, `run-dashboard.bat`)**:
  Disabled delayed expansion (`DisableDelayedExpansion`), replaced the fragile
  PowerShell lock check with a pure `cmd.exe` delete/lock check, aligned Wails
  build flags to `wails build -clean -skipbindings -m -s -webview2 error -o chitchatui.exe`
  with automatic fallback to `go build -tags desktop,production`, lowered the
  minimum Go requirement to `1.24` (`go 1.24.0` in `go.mod`), and made both
  `install.bat run` and `run-dashboard.bat` auto-build `build\bin\chitchatui.exe`
  when missing.
  (3) **Go compile/test fixes & dead/duplicate/temp file removal**: Fixed
  `*Director` pointer arguments in `internal/apichat/guard_test.go`, unigram
  Jaccard fallback in `internal/apichat/repeat.go`, and nil `APIChatSessions`
  map initialization in `internal/bot/api_chat.go`; added `./internal/apichat`
  to `scripts/verify.cjs`. Removed 7 dead/duplicate/temp files:
  `internal/license/license.go`, `cmd/chitchatui/app.go`, `cmd/chitchatui/main.go`,
  `internal/bot/match_test.go`, `scripts/ban-nav.cjs`, `scripts/ban-sim.cjs`,
  and `scripts/page-status.cjs`.
  (4) **In-dashboard Local Test Chat & persistent API key settings**: Added the
  `Local Test Chat` button in `.sidebar-footer` right above `Workers` along with
  the interactive `#local-test-panel` roleplay & bot-memory inspector (`StartLocalTestChat`,
  `SendLocalTestChatMessage`, `GetLocalTestChatState`, `bot.LocalChatSession`),
  so all 4 Message modes can be tested interactively inside the dashboard without
  closing the app or hitting BadgerDB lock errors. Added live cross-tab API field
  syncing, auto-save on change / `Test API`, explicit `Save Message & API`
  buttons (`#sm-save-message`, `#bg-save-message`), non-empty API key
  preservation in `uistore.SaveMessageSettings`, and a `data/ui/message_settings.json`
  mirror fallback so `test.bat` also works while the dashboard is open. Verified
  with all Go unit tests and `npm test` (78/78 PASS).
  (5) **Greeting recognition fix, standalone `apichat.exe` Persona Studio & Live Test Chat dashboard, and main dashboard cleanup**:
  - Fixed `Director.Observe` and `Director.Plan` (`internal/apichat/director.go`,
    `internal/apichat/guard.go`, `internal/apichat/prompt.go`, `internal/bot/api_assets/persona.txt`)
    so short greetings (`"hi"`, `"hey"`, `"hello"`, `"yo"`, `"sup"`) are recognized
    as `userGreeting` (`Why = "greeting"`) instead of `userLowEffort` or forcing
    an unsolicited `f 21` (`Why = "asl"`) dump and `two questions in a row` retry.
  - Built the standalone **`apichat.exe`** desktop application (`cmd/apichat/main.go`,
    `cmd/apichat/ui/{index.html,app.js,styles.css}`, `apichat.bat`, and `install.bat`)
    where the user can directly edit or load `.txt` files into `persona.txt`,
    `sharesnap.txt`, `fallback.txt`, and `persona_countries.txt`, inspect the
    live rendered system prompt (`{age}`, `{country}`, `{name}`, etc.), test all
    4 modes in the built-in Live Test Chat, and automatically sync Persona + API
    settings with `chitchatui.exe` (`uistore.LoadSharedMessageSettings` /
    `uistore.SaveSharedMessageSettings`).
  - Removed the `Local Test Chat` button and panel from the main dashboard
    (`chitchatui.exe`) as requested, keeping `Local Test Chat` inside `apichat.exe`.

