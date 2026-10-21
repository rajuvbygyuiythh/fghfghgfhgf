# Phase 1 & Phase 2 — Site folder isolation + Global Settings

Completed source update: 2026-10-21 (owner timezone).

Owner requirement: the app ships 102 sites. Every site must own a real folder that
holds its browser profile, saved session, cookies, accounts and logs. One site's
code must never read or write another site's folder. The only shared directory is
`sites/_global/`, used for application settings (`sites/_global/settings.json`).

## Resulting layout

```
<project>/                       # repository root (or <dataDir> in portable mode)
└─ sites/                        # canonical sites root (appdirs.SitesRoot)
   ├─ _global/                   # THE ONLY SHARED FOLDER
   │  ├─ settings.json           # global settings (message_mode, proxy, reply_delay, skip_timeout, first_message, api)
   │  ├─ ui/                     # Badger DB + message_settings.json mirror
   │  ├─ logs.txt                # desktop/app diagnostics (JSON lines)
   │  ├─ logs/                   # structured global debug mirrors
   │  ├─ memory/                 # chat_history.jsonl, operation_memory.json
   │  └─ settings/               # reserved for cross-site preferences
   ├─ chitchat/                  # the default site also gets a real folder (1 of 102)
   │  ├─ site.json
   │  ├─ dom_driver.js
   │  ├─ README.txt
   │  ├─ chitchat_protection.txt
   │  ├─ settings_override.json  # optional per-site override (only when customised)
   │  ├─ accounts/               # accounts/<profile>.json + saved credentials
   │  ├─ profiles/<profile>/     # Chromium user-data dir + login_info*.json
   │  ├─ sessions/               # latest_session.json, session_<profile>.json
   │  └─ logs/                   # debug.jsonl, last_diagnostics.json
   ├─ omezy/
   └─ … 100 more (102 isolated site folders total)
```

`<project>/sites` is reused when it already exists next to `data/`; otherwise the
root falls back to `data/sites`. Both cases resolve through one function,
`SitesRoot`, so callers never hardcode a root.

## The isolation module

`internal/sites/isolation.go` owns every path decision.

`SiteContext` is the only supported way to reach site files:

| Field / method | Purpose |
| --- | --- |
| `SiteID`, `FolderPath` | identity and the one allowed root |
| `AccountsPath`, `ProfilesPath`, `SessionsPath`, `LogsPath` | fixed sub-roots |
| `DefaultProfilePath`, `DefaultSessionPath`, `DebugLogPath` | well-known files |
| `Resolve(rel)` | validate + join inside the site folder |
| `EnsureDir`, `ReadFile`, `WriteFile`, `OpenFile`, `Rename`, `Remove`, `RemoveAll`, `Glob`, `Stat`, `Lstat`, `ReadDir` | file operations that cannot leave the folder |
| `SaveJSON`, `LoadJSON`, `WriteAtomic` | atomic structured state |
| `ProfilePath(profile)`, `BrowserProfileDir(profile)`, `SessionPath(profile)`, `LatestSessionPath()`, `AccountPath(name)`, `LogPath(name)` | named entry points |
| `GlobalPath(rel)`, `GlobalResolve(rel)` | the only door to `_global/` |

`NewSiteContext(dataDir, siteID)` refuses to build a context for an invalid ID or
for a site folder that is a symlink, so a tampered directory tree cannot redirect
a site to another site's data.

## Path safety contract

`SafeJoin(root, rel)` is the single gatekeeper. It rejects, in this order:

1. empty root, NUL bytes, control characters;
2. absolute paths, Windows volume names, UNC/device paths;
3. any backslash separator (accepted on Windows, rejected everywhere for parity);
4. any `..` segment **before** `filepath.Clean` can normalise it away;
5. `..` in the cleaned relative path;
6. Windows-reserved names and alternate-data-stream suffixes;
7. any target that resolves outside the root, including through symlinks —
   the target (or its nearest existing ancestor) is resolved with
   `filepath.EvalSymlinks` and re-checked against the resolved root.

`SiteContext` uses `SafeJoin` for every operation, so the guarantee is enforced
once instead of being re-implemented per feature.

## Legacy migration

On first start the manager merges the pre-isolation shared state into the default
site, never overwriting newer files:

| Before | After |
| --- | --- |
| `data/profiles/**` | `sites/chitchat/profiles/**` |
| `data/login_info_*.json` | `sites/chitchat/profiles/default/` |
| `data/bot_registry.json` | `sites/chitchat/bot_registry.json` |
| `data/tmp`, `data/fresh-runs` | `sites/chitchat/{tmp,fresh-runs}` |
| `data/ui/**` | `data/_global/ui/**` |
| `data/memory/**` | `data/_global/memory/**` |
| `<project>/logs.txt` | `data/_global/logs.txt` |

Directories are merged recursively; a one-time rename is used where the whole
subtree moves as a unit. After migration, `data/` contains only `_global/`.

## Behaviour changes that are part of the contract

- `sites.List`, `sites.SiteDir`, `sites.DataDirForSite`, `sites.LoadSite` and
  `sites.CreateSiteWithMode` all resolve through `SiteContext`.
- Browser profiles, `login_info` files, session snapshots, account files, debug
  logs and per-site `api_chat/` / `eva_chat/` packs live inside the site folder.
- `ListProfiles` and the banned-user cleanup resolve the active site folder, so
  two sites with a profile called `default` never share cookies.
- `applog.Path(dataDir)` is `<dataDir>/_global/logs.txt`; both entry points create
  `_global/` before the first log line, so no startup diagnostic is lost.
- `memory.NewStore` writes to `<dataDir>/_global/memory` and renames a legacy
  `data/memory` once.
- `uistore.Open` writes Badger and the message-settings mirror under
  `<dataDir>/_global/ui`.

## Regression coverage

`internal/sites/isolation_test.go` proves the rule, not just the layout:

- 102 sites ⇒ 102 folders, and only `_global/` is shared;
- site A cannot read, write, glob or resolve site B's files, including via `..`,
  absolute paths, sibling IDs, and a symlink planted inside site A;
- invalid/aliasing IDs (`""`, `.`, `..`, `../x`, `a/b`, `a\b`, `_global`, `Sites`,
  `Chatib`, …) are rejected;
- `CleanSiteRuntime` deletes only the requested site's runtime folders;
- session and account writers stay inside the site folder.

Run the suite:

```
go build ./... && go vet ./...
go test ./... -count=1
go test -race ./internal/sites/ ./internal/settings/ ./internal/appdirs/ ./internal/memory/ ./internal/uistore/ ./internal/applog/ .
```

## Phase 2 — Global Settings Store & Per-Site Overrides (`internal/settings`)

- `sites/_global/settings.json` holds the global defaults shared across all 102 sites:
  - `message_mode` (`single`, `folder`, `eva_chat_flow`, `api_chat`)
  - `proxy` (`enabled`, `entries`)
  - `reply_delay` (`min_seconds`, `max_seconds`)
  - `skip_timeout` (`min_seconds`, `max_seconds`)
  - `first_message` (`mode`, `bot_first_percent`, `wait_timeout`, `wait_min_seconds`, `wait_max_seconds`)
  - `api` (`enabled`, `base_url`, `api_key`, `model`, `temperature`)
- `sites/<site_id>/settings_override.json` (resolved through `sites.NewSiteContext`) holds optional pointer fields (`Override`). Any `nil` field inherits from `sites/_global/settings.json`; any non-`nil` field overrides that section for that site only.
- `settings.Merge(global, override)` computes `Effective{Settings, SiteID, Sources}` without mutating its inputs.
- Saving settings from the dashboard (`SaveMessageSettings`, `SaveProxySettings`, `SaveGeneralSettings`, `SaveUnifiedSettings`, or `SaveGlobalSettings`) updates `sites/_global/settings.json` atomically (`writeJSONAtomic` with `0600` permissions) and immediately applies to all sites that have not overridden that section.

## Phase 3 — Site Classification (Group A–E) & Live Re-Prober (`internal/sites/classification.go`)

Every site is classified into one of five execution groups stored inside `sites/<site_id>/site.json` under `"classification"`:

| Group | Name | Meaning |
| --- | --- | --- |
| `A` | `Browserless` | signup লাগে না, direct chat (no captcha, no login, direct WS/API) |
| `B` | `Auto-Signup` | API দিয়ে account বানানো যায়, captcha নেই |
| `C` | `Browser-Once` | captcha/login লাগে একবার, পরে browserless |
| `D` | `DOM-Only` | API/WS নেই, headless browser দিয়ে DOM chat |
| `E` | `Manual` | heavy bot detection, manual only |

- `ProbeURL(ctx, rawURL, client)` / `AnalyzeHTML(...)` detects:
  - Captcha (`has_captcha`, `captcha_keywords`: `recaptcha`, `hcaptcha`, `cloudflare`, `arkose`, `funcaptcha`, `datadome`, `perimeterx`)
  - Signup form (`has_signup_form`, `signup_indicators`)
  - WebSocket endpoint (`has_websocket`, `websocket_endpoints`)
  - API endpoint (`has_api_endpoint`, `api_endpoints`)
  - Login required (`login_required`, `login_indicators`)
  - Heavy bot detection (`heavy_bot_detection`, `bot_check_keywords`)
- `ClassifyProbe(probe)` deterministically assigns Group `A`–`E` with `confidence: "high"` (or `"low"` when offline).
- All 102 existing `sites/<site_id>/site.json` files ship with an initial classification (`confidence: "low"`, `source: "initial_guess"`).
- Classification is re-runnable at any time via `ProbeAndClassifySite(ctx, dataDir, siteID, client)` and `ProbeAndClassifyAllSites(ctx, dataDir, client)` (exposed to Wails via `App.ProbeAndClassifySite` and `App.ProbeAndClassifyAllSites`).

## Phase 4 — Repeatable Site Setup Recipe System (`sites/_recipes/` & `internal/sites/recipes.go`)

- Five canonical setup recipes live under `sites/_recipes/`:
  - `sites/_recipes/group_A.json` (`Browserless`): `folder create` → `WS detect` → `direct connect test` → `save config`
  - `sites/_recipes/group_B.json` (`Auto-Signup`): `folder create` → `signup endpoint detect` → `fake account generate` → `POST register` → `token save` → `chat test`
  - `sites/_recipes/group_C.json` (`Browser-Once`): `folder create` → `login URL detect` → `mark "needs browser"` → `wait user login` → `capture session` → `verify browserless`
  - `sites/_recipes/group_D.json` (`DOM-Only`): `folder create` → `headless profile setup` → `DOM selector detect` → `save driver` → `headless test`
  - `sites/_recipes/group_E.json` (`Manual`): `folder create` → `mark manual only`
- `ExecuteSiteRecipe(ctx, dataDir, siteID, opts)` executes steps sequentially:
  - Any step failure is logged inside the site's own `sites/<site_id>/logs/recipe.jsonl` and `sites/<site_id>/logs/debug.jsonl`, updates the failed group's statistics, and automatically triggers fallback along `B → C → D → E` (or `A → B → C → D → E`).
  - Successful setups update the recipe's `success_rate`, `learning.verified_sites`, `learning.learned_endpoints`, `learning.learned_selectors`, and per-step `step_metrics` inside `sites/_recipes/group_<X>.json`.

## Phase 5 — Isolated Site Runner Supervisor (`internal/runner/supervisor.go`)

- `runner.Supervisor` manages per-site goroutines, each with its own `context.Context`, its own `sites.SiteContext` (`sites/<site_id>/`), and its own `recover()` crash barrier.
- **Absolute Crash Isolation**: If one site panics or crashes, only that site is marked `"failed"` (`LastErrorType = "crash"`), its stack trace is logged to `sites/<site_id>/logs/errors.jsonl`, and all other running sites continue uninterrupted.
- **8 Supervisor Methods** (exposed on both `*runner.Supervisor` and `*main.App`):
  - `StartSite(siteID)` — start isolated goroutine for one site
  - `StopSite(siteID)` — clean stop for one site
  - `StartSelected(siteIDs)` — start multiple selected sites in parallel
  - `StopAll()` — stop all running sites cleanly
  - `GetStatus(siteID)` — current status, error counters, and chat statistics
  - `GetAllStatuses()` — map of all tracked sites' `SiteStatus`
  - `PauseSite(siteID, reason)` — pause a site runner with a reason
  - `RestartSite(siteID)` — clean stop + start with incremented `RestartCount`
- **5 Consecutive Error Auto-Pause & 6 Error Classifications**:
  - Classifies errors into `connection`, `auth expired`, `banned`, `captcha`, `timeout`, and `crash`.
  - Logs every error strictly to `sites/<site_id>/logs/errors.jsonl`, `sites/<site_id>/logs/runner.jsonl`, and `sites/<site_id>/logs/debug.jsonl`.
  - Automatically pauses a site (`AutoPaused: true`) and notifies the UI (`ui:runner-notification`) when `ConsecutiveErrors` reaches `5`.
- **Integration with `internal/bot` & `internal/browser`**:
  - Executes `bot/` chat logic (`sequence`, `folder`, `evachat`, `apichat`) rooted at `sites/<site_id>/` and connects via `browser.Backend` using `sites/<site_id>/profiles/<profile>/browser` as `UserDataDir`.

## Boundaries of this phase

- The site **catalog** (`sites/<id>/site.json`, `dom_driver.js`, `README.txt`) is
  the same shipped tree; isolation protects runtime state, not the shared code.
- Older documents (`docs/PROJECT_MEMORY.md`, `docs/FILE_LOGGING.md`,
  `docs/EVACHAT_FLOW_GUIDE.md`) still describe the pre-Phase-1 paths. This file is
  authoritative for paths until those documents are revised.
- `cmd/apichat` (Persona Studio) intentionally keeps its own `data/api_chat`
  working folder; it is a separate tool, not a site worker.
