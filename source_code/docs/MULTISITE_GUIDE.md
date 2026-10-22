# Multi-Site Architecture & Operations Guide (102 Isolated Sites)

This guide documents the complete multi-site architecture of **ChitChat Bot**, covering strict per-site folder isolation, Group A–E classification, repeatable setup recipes, browserless chat execution, agent self-recovery (Scenarios A–E), and the 3-tab dashboard workflow.

---

## 1. Strict 102-Site Folder Isolation (`sites/<site_id>/`)

Every target site in the 102-site catalog lives inside its own isolated directory under `sites/<site_id>/`.

### Hard Isolation Rule
- **102 sites = 102 separate folders.**
- **Zero cross-site file sharing:** One site's runner, session store, browser profile, or DOM script can never read or write another site's folder. All paths are validated through `sites.ResolveSiteContext(baseDir, siteID)` and `SiteContext.SafeJoin(...)` (`internal/sites/isolation.go`), which blocks parent-directory traversal (`..`) and cross-site paths.
- **Only two shared directories exist:**
  - `sites/_global/settings.json` — Global settings shared by all sites that do not define a per-site override.
  - `sites/_recipes/group_A.json` .. `group_E.json` — Repeatable setup recipes per classification group.

### Per-Site Folder Layout
```text
sites/<site_id>/
├── site.json                   # Site metadata, URLs, selectors, and Group A–E classification
├── dom_driver.js               # Isolated DOM chat driver for this site
├── <site_id>_protection.txt    # Anti-bot / CAPTCHA / Cloudflare notes
├── settings_override.json      # Optional per-site settings override (proxy, mode, delay)
├── config_override.json        # Optional runtime connection override (e.g. DOM fallback)
├── accounts/                   # Isolated auto-registered or captured accounts
├── sessions/
│   └── default/                # 5-file isolated session bundle:
│       ├── cookies.json
│       ├── local_storage.json
│       ├── ws_endpoints.json
│       ├── auth_tokens.json
│       └── meta.json
├── profiles/
│   └── default/                # Isolated Rod/ChromeDP user-data profile
└── logs/
    ├── debug.jsonl             # Per-site structured debug log
    ├── chat_history.jsonl      # Isolated per-site chat transcript
    ├── recipe.jsonl            # Setup recipe step execution log
    └── recovery.jsonl          # Agent self-recovery attempt log
```

---

## 2. Global Settings, Per-Site Overrides & `apichat.exe` Sync

- **Global Settings (`sites/_global/settings.json`):** Stores global `message_mode`, `proxy`, `reply_delay`, `skip_timeout`, `first_message`, and `api` settings (`internal/settings`).
- **Per-Site Overrides (`sites/<site_id>/settings_override.json`):** Any site can override proxy, message mode, or reply delay via the Slide-out Detail Panel in Tab 2 (`Site Manager`). Saving an override immediately reloads that site's runner without restarting other sites.
- **Bidirectional `apichat.exe` Sync:** Saving API settings in `apichat.exe` (`SaveStudioState`) or in the main dashboard (`SaveMessageSettings` / `SaveGlobalSettings` / `SyncGlobalAndAPIChatSettings`) synchronizes both `sites/_global/settings.json` and the shared UI mirror so both executables stay in lockstep.

---

## 3. Group A–E Classification & Repeatable Setup Recipes

All 102 sites are classified into 5 operational groups (`internal/sites/classification.go`) with corresponding repeatable recipes in `sites/_recipes/group_<X>.json` (`internal/sites/recipes.go`):

| Group | Name | Execution Mode | Setup & Chat Flow |
|---|---|---|---|
| **Group A** | Pure Browserless Guest | Browserless (`WS` / `HTTP`) | Zero registration; connects directly via WebSocket or guest HTTP API without launching any browser. |
| **Group B** | HTTP Auto-Signup | Browserless (`WS` / `HTTP`) | Registers a guest/fake account via HTTP POST, saves tokens to `sites/<site_id>/sessions/default/`, and chats browserlessly. |
| **Group C** | Browser-Once + Headless Chat | Browser Setup → Browserless | Opens a visible browser once (or in batches of 5) for login/CAPTCHA, captures the 5-file session bundle, closes the browser, and chats browserlessly. |
| **Group D** | DOM-Only Headless Browser | Headless Rod (`DOM`) | Runs in an isolated headless browser using `sites/<site_id>/dom_driver.js` and the site's saved session/profile. |
| **Group E** | Manual Mode Only | Visible Browser | Strict anti-bot sites reserved for manual visible-browser operation. |

When a recipe step fails on a site, the recipe engine automatically falls back through the chain **`B → C → D → E`** and records learned overrides.

---

## 4. Multi-Site Runner Supervisor & Browserless Chat

- **Supervisor (`internal/runner/supervisor.go`):** Runs each active site in its own isolated goroutine with an independent `context.Context` and panic recovery barrier (`recover()`), ensuring a crash on one site never affects any other running site.
- **5-Error Auto-Pause:** If a site hits 5 consecutive errors, the supervisor automatically pauses only that site and emits a UI notification.
- **Browserless Connection (`internal/runner/browserless.go`):**
  - Loads `cookies.json`, `local_storage.json`, `ws_endpoints.json`, `auth_tokens.json`, and `meta.json` strictly from `sites/<site_id>/sessions/default/`.
  - If a partner disconnects, `Reconnect()` re-matches on the same session without re-logging in.
  - If a session is missing or expired on Group B/C, the runner pauses with status `"needs session"` (`ErrNeedsSession`) instead of launching an unexpected browser.

---

## 5. Agent Self-Recovery (Scenarios A–E)

`RecoveryManager` (`internal/runner/recovery.go`) handles automatic and manual self-recovery with a hard limit of **3 attempts per site** (`MaxRecoveryAttempts = 3`):

- **Scenario A (`session_expired`):** Loads the isolated profile, attempts auto-login (notifying the user if CAPTCHA is present), captures a fresh session into `sites/<site_id>/sessions/default/`, and resumes chat.
- **Scenario B (`signup_failed`):** Generates a fresh auto-account in `sites/<site_id>/accounts/auto_account.json`, checks for CAPTCHA, captures the session, and starts chat.
- **Scenario C (`ws_broken` / `api_broken`):** Falls back from broken WebSocket/HTTP API to DOM mode, writes `sites/<site_id>/dom_driver.js` and `config_override.json` (`connection_mode: "dom"`), and notifies `"switched to DOM mode"`.
- **Scenario D (`dom_changed`):** Inspects the page with `skills/dom_auto_scout.js` heuristics, updates `sites/<site_id>/dom_driver.js` with newly detected input/send/container selectors, and verifies DOM chat.
- **Scenario E (`manual` / `complete_failure`):** Prepares the isolated visible browser profile and notifies `"I'm stuck, please login manually"`, then captures the session once logged in.
- **`needs_human` Escalation:** After 3 failed recovery attempts, the site is marked `"needs_human"` (`ErrNeedsHuman`) and surfaced in the Control Center Error Dashboard and Site Manager banner.

---

## 6. Dashboard 3-Tab Structure, Combined Live Log & Bulk Operations

1. **Tab 1 — `Control Center`:**
   - Global Settings & Multi-Site Supervisor controls.
   - **Bulk Operations (`max 10 concurrent` + live progress bar + individual failure resilience):** `Start All Ready`, `Stop All`, `Auto-Register All Group B`, `Login All Group C (Batch 5)`, `Refresh All Sessions`, and `Full Report`.
   - **Error Summary & Self-Recovery Dashboard:** Per-site error counts, error type breakdown (`connection` / `auth` / `ban` / `captcha` / `timeout`), sites needing attention list, and recovery status counters.
2. **Tab 2 — `Site Manager`:**
   - Collapsible Group A–E sections for all 102 sites, status/session badges, group-level batch actions, per-site `Self-Recover` and Slide-out Detail Panel with instant overrides.
   - **`+ Add Site` 4-Step Wizard:** `Step 1 (URL Input) → Step 2 (Auto-Probing) → Step 3 (Setup Execution) → Step 4 (Result + Next Action)`.
3. **Tab 3 — `Sessions`:**
   - Aggregate Session Health Summary (`✅ Valid`, `⚠️ Expiring`, `❌ Missing`), per-site `Capture` / `Re-capture` / `Clear`, and `Capture All Missing (Batch of 5)`.
4. **Combined Live Log Bottom Bar (Visible Across All Tabs):**
   - Displays `timestamp + [Group X · Site Name] (group-colored) + OUT/IN/ERR/SYS + message`, with filters by **Site**, **Type**, and **Group**, auto-scroll, `Clear`, and a 500-line memory bound.
