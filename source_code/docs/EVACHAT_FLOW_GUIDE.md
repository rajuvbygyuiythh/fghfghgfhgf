# EVAChat Flow â€” Build and Data Guide

## What changed

The dashboard's **Accounts & Assets â†’ Message & AI** card still has three modes:

1. **Single Sequence** â€” unchanged
2. **Folder Sequences** â€” unchanged
3. **EVAChat Flow** â€” replaces the old Keyword Based mode

EVAChat Flow is a stateful, TXT-driven reply engine. It is implemented in Go and has no Python runtime dependency.

## Start and use

1. Build and start the app.
2. Open **Accounts & Assets â†’ Message & AI**.
3. Select **EVAChat Flow**.
4. Click **Save Assets**.
5. Click **Open EVAChat Data Folder** to create and open the editable dataset. The same dataset is also created automatically when EVAChat Flow starts.
6. Stop and start the bot after editing the TXT files so worker configurations reload them.

## Editable data layout

The app creates the following runtime folder next to its normal `data` directory
(seeded from the embedded EVA TXT database on first start):

```text
data/eva_chat/
â”œâ”€â”€ countries.txt            # keyword=country
â”œâ”€â”€ input/                   # USER triggers (1 per line, '#' = comment)
â”‚   â”œâ”€â”€ greeting.txt age_gender.txt country.txt how_are_you.txt your_age.txt
â”‚   â”œâ”€â”€ english_only.txt horny.txt horny_answer.txt share_snap.txt snapchat.txt
â”‚   â””â”€â”€ middle_chat/ flirty_reply.txt warm_reply.txt busy_later.txt
â””â”€â”€ output/                  # BOT replies (1 per line, '|' also splits)
    â”œâ”€â”€ greeting.txt age_gender.txt age_ask.txt country.txt how_are_you.txt
    â”œâ”€â”€ your_age.txt english_only.txt horny.txt horny_answer.txt
    â”œâ”€â”€ flirty_questions.txt ask_snap.txt retry_nudge.txt share_snap.txt
    â”œâ”€â”€ snapchat.txt closer.txt
    â””â”€â”€ middle_chat/ flirty_reply.txt warm_reply.txt busy_later.txt
```

A category is connected by file name (`input/x.txt` detects, `output/x.txt`
replies). Adding a new pair adds a category without any code change.

The full rule set (category table, matching layers, stage machine, no-repeat
rules, placeholders) is documented in `CHAT_ENGINE_MAPPING.md`.

Snap / Telegram username share (same mechanism as the other two modes):
`output/share_snap.txt` and `output/snapchat.txt` lines contain `%username%`
(or `{snap}`), filled from **Accounts & Assets â†’ Message & AI â†’ Usernames**
exactly like Single/Folder
sequences; the dashboard `snap/tg/user` counter and flagged-user tracking
increase in the same way. The share happens when the user asks for the handle,
or says yes after the bot's `ask_snap.txt` question. If the Usernames list is
empty the bot closes politely instead of sharing.

## AI reply text (optional, hybrid)

Tick **AI reply text (hybrid)** in the EVAChat panel, enter an OpenAI-compatible
base URL (OpenAI / Groq / OpenRouter / Gemini / DeepSeek â€¦), model and API key,
press **Test AI**, Save, then Start. The TXT rules still run the funnel (age,
snap ask â†’ yes â†’ share, cap); the AI only writes the reply text so it answers
what the stranger actually said, with a random persona per user (f 20â€“25,
random European country â€” edit `data/eva_chat/persona.txt` and
`persona_countries.txt`). If the API fails the TXT line is sent. The key is
stored only in the local `data/` settings file.

## Engine behavior (summary)

- Stages: `NEW â†’ WAIT_AGE_GENDER â†’ WAIT_COUNTRY â†’ MIDDLE â†’ END`
- Match layers: exact, word-boundary phrase, leet regex (horny), slang-normalized, token subset; `english_only.txt` is strict.
- Horny or flirty reply â†’ next bot line is always `ask_snap.txt` â†’ yes â†’ `share_snap.txt` â†’ END; "no" â†’ `retry_nudge.txt` once â†’ second "no" â†’ `closer.txt`.
- Horny QUESTION ("u horny?") â†’ 2 lines: `horny_answer.txt` + `horny.txt`.
- Every output file fires once per user except `flirty_questions, horny, middle_chat/flirty_reply, ask_snap, horny_answer, closer`; never the same line twice in a row.
- Reply cap 8 â†’ `closer.txt`. Age under 18 â†’ never shares.
- Conversation state (incl. a short transcript) is stored per user in the existing bot-state JSON, so it survives polling cycles and restarts.

## Windows build

Use the source root (`go_erp`) in PowerShell:

```powershell
# Required once: Go 1.25+ and Wails v2 CLI
wails doctor
.\scripts\bundle-windows.ps1 -Clean
```

The build script creates the Windows bundle under:

```text
dist\windows-x64\
```

For a development build:

```powershell
wails build -clean -webview2 error
```

## Security requirement before publishing

Do **not** add runtime files to Git or a public ZIP. This source package intentionally excludes `data/` and has a `.gitignore` rule for it.

Never publish:

```text
data/profiles/**
data/ui/badger/**
login_info_*.json
bot_state.json
browser cookies, tokens, session storage, logs, or account-specific JSON
```

The source contains only embedded starter templates under `internal/bot/eva_assets/`; these contain no accounts, cookies, sessions, or user data.

