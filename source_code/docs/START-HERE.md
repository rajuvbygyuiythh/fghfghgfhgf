# Clean root project â€” start here

This folder is the complete editable application source, not a prebuilt EXE.
The dashboard uses nested navigation: the left sidebar selects the mode and a
second list selects the section; only that section renders in the main area.

- **Show Mode** â€” sub-menu: **Setting**, **Proxy**, **Message**, **Run
  Browser**, **Live Logs**. Run Browser saves Setting + Proxy + Message, then
  opens the visible Rod browser. Live Logs lists every user/bot SMS with the
  rule, chosen delay and skip reason.
- **Background Bot** â€” sub-menu: **Login Capture**, **Proxy**, **Message**,
  **Bot Settings**. The stats bar, Start/Stop and status stay visible above the
  sub-pages.

Every field is bound to a global form store (`show` / `bot` slices), so typed
values survive switching between sub-pages.

## Windows setup and manual test

1. Extract the root-project download into a **new folder**. This root-only
   package does not include the repository-level GitHub workflow.
2. Open `root-project/` and run `install.bat` for the existing Windows build/setup.
3. **Background Bot:** open Background Bot â†’ Login Capture. Leave Headless off,
   press Launch for Login and finish the login. Then fill Message + Bot Settings
   and press Save & Start Bot.
4. **Show Mode:** open Show Mode, fill Message (and Usernames), pick the Profile
   radio **default** (fresh manual login, session saved afterwards) or
   **session** (saved default session, auto-login straight to chat). Press
   **Run Browser**.

No account/session data is included. Do not overwrite an existing data folder
or delete older working copies just to test this source.

## Manual traffic capture (optional)

`capture/` contains a standalone recorder: browse any site manually in a
visible browser while network, WebSocket, DOM and storage evidence is saved
(redacted by default) into `capture/capture-sessions/`. Start with
`capture/install.bat` once, then `capture/run.bat`. See `capture/README.md`.
Capture sessions and the capture browser profile are excluded from source
packages and must not be shared when raw mode was used.

## Verification / debugging (no browser launched)

With Node 22, Go 1.25+ and Wails 2.12.0 installed, run `verify.bat`.
See `VERIFICATION.md` for exact prerequisites and commands.

- `verification-output/report.json`: each check's status, reason, command,
  exit code, duration and log filename/hash.
- `verification-output/*.txt`: detailed check output.
- `npm test`: UI, contract, packaging and verification-tool tests only.
- `npm run package:source`: package only after complete verification passes.

The download contains `VERIFICATION-REPORT.json`, `SOURCE-MANIFEST.json` and
`verification-evidence/` beside the source folder. BLOCKED is not PASS. An
UNVERIFIED source download is for manual testing, not a certified release.

## What â€œcleanâ€ means

The source package excludes node_modules, caches, private sessions, runtime
binaries, build output and older ZIPs. Tests, lockfile and necessary source assets
(including frontend/dist) stay included. Original backups and the reference
project are not deleted. Installer, chat/AI, license and browser drivers remain
outside this cleanup's scope.

See `DEBUG_CLEANUP.md` for the specific fixes in this delivery.

