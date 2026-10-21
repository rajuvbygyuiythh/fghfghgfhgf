# Project Map — ChitChat Bot (root project)

This file is the architecture map for any human or AI agent joining the
project. The block between the AUTO markers is regenerated from the actual
source tree by `scripts/project-map.cjs` (automatically on every
`npm run verify`, or manually with `npm run map:update`). Read it together
with `AGENTS.md` (locked scope/rules) and `docs/PROJECT_MEMORY.md`
(decisions and current state).

<!-- AUTO:BEGIN -->
_Auto-generated 2026-10-21 by `scripts/project-map.cjs`. Do not edit this block by hand._

## Source inventory

- Total source files: **247** (1668.8 KiB, excluding runtime/private data)

| Area | Files | Size |
|---|---:|---:|
| `internal/` | 136 | 797.1 KiB |
| `(root files)` | 26 | 233.5 KiB |
| `capture/` | 24 | 114.3 KiB |
| `docs/` | 21 | 119.6 KiB |
| `cmd/` | 12 | 182.8 KiB |
| `apitest/` | 11 | 99.2 KiB |
| `scripts/` | 9 | 46.5 KiB |
| `tests/` | 8 | 75.7 KiB |

## Internal Go packages

- `internal/api/` — 1 Go file(s)
- `internal/apichat/` — 22 Go file(s)
- `internal/applog/` — 4 Go file(s)
- `internal/bot/` — 25 Go file(s)
- `internal/browser/` — 11 Go file(s)
- `internal/memory/` — 2 Go file(s)
- `internal/model/` — 4 Go file(s)
- `internal/proxy/` — 7 Go file(s)
- `internal/runtimeassets/` — 2 Go file(s)
- `internal/sites/` — 8 Go file(s)
- `internal/store/` — 1 Go file(s)
- `internal/uistore/` — 5 Go file(s)
- `internal/workerplan/` — 2 Go file(s)

## Browser options (Dashboard → Browser)

1. `rod` — Rod
2. `chromedp` — ChromeDP
3. `show_mode` — Show Mode

## Key entry points

- `main.go` / `app.go` — Wails application and bound methods
- `cmd/chitchatui/` — dashboard frontend (embedded `frontend/dist/`)
- `capture/` — standalone manual traffic capture tool (see `capture/README.md`)
- `scripts/verify.cjs` — browser-free verification gate (`verify.bat`)
- `scripts/package-source.cjs` — source ZIP packaging (refuses stale reports)
- `install.bat` / `run-dashboard.bat` — Windows setup and launch
- Capture tool version: **1.2.1**

## Documentation

- `docs/API_CHAT_MODE.md`
- `docs/API_CHAT_QUALITY.md`
- `docs/BROWSER_DIRECT_RUN.md`
- `docs/CHAT_ENGINE_MAPPING.md`
- `docs/CHITCHAT_PROTOCOL.md`
- `docs/DEBUG_CLEANUP.md`
- `docs/EVACHAT_FLOW_GUIDE.md`
- `docs/FILE_LOGGING.md`
- `docs/FIRST_MESSAGE_SETTINGS.md`
- `docs/HOME_CHAT.md`
- `docs/LOCAL_TEST_CHAT.md`
- `docs/PROJECT_MAP.md`
- `docs/PROJECT_MEMORY.md`
- `docs/PROXY_TEST.md`
- `docs/README.md`
- `docs/RECHECK_2026-10-15.md`
- `docs/SETTINGS_PROFILES.md`
- `docs/SHOW_MODE.md`
- `docs/SHOW_MODE_FIX.md`
- `docs/START-HERE.md`
- `docs/TEST_REPORT.md`
- `docs/VERIFICATION.md`
- `AGENTS.md` — locked scope and hard rules (read first)
- `docs/START-HERE.md` — packaging/testing entry guide
<!-- AUTO:END -->

## Manual notes (agents: keep this section current)

- Product: Windows desktop app (Go + Wails) + CLI worker for chitchat.gg.
- Dashboard Browser page has numbered per-row Run buttons; option behaviour is
  documented in `docs/BROWSER_DIRECT_RUN.md` and `docs/SHOW_MODE.md`.
- `capture/` is a standalone manual capture tool. It is NOT part of the
  desktop app runtime and must stay dependency-free (Node built-ins only).
- Verification never launches a browser or a real login by default.

## Two-page dashboard / independent timing profiles (2026-10-18)

- **Show Mode** page: Setting · Proxy · Message · Run Browser · Live Logs.
  Run Browser saves `settings/show_mode/v1` + proxy + message, then launches.
- **Background Bot** page: Login Capture · Proxy · Message · Bot Settings
  (`settings/general`) · Start/Stop + counters.
- `show_log.go`: bounded rule-aware feed; `GetShowModeLog` / `ui:showlog`.
- `live_chat.go`: `liveReply` metadata (sequence line, EVA pool, delay, skip).
- `internal/model/ui.go`: ShowModeSettings; UISnapshot.show_mode_running.
- `internal/uistore/show_mode.go`: dedicated key, validation and migration.
- `docs/SETTINGS_PROFILES.md`: semantics, limits and acceptance checklist.

## First-sender policy (2026-10-17)

- `internal/model/first_message.go`: independently saved policy + stored decision.
- `internal/bot/first_message.go`: persisted worker choice and first-match gate.
- Show Mode first-wait state is in `live_chat.go`; sequence/EVA engines remain
  unchanged except the sequence responder's lazy first-line initialization.
- `docs/FIRST_MESSAGE_SETTINGS.md`: scope, DM distinction, evidence and limits.
