# API Chat mode

API Chat is the fourth chat mode in the dashboard. It is deliberately separate
from **EVAChat Flow**:

| | EVAChat Flow | API Chat |
|---|---|---|
| Who writes the lines | TXT reply pools in `data/eva_chat/` | the model you configure |
| Network use | none | OpenAI-compatible `/chat/completions` |
| Flow control | stage machine + TXT rules | `internal/apichat` director |
| Data folder | `data/eva_chat/` | `data/api_chat/` |
| Snap username | `%username%` in TXT | added by Go after the model writes the lead-in |

Both modes are selectable in **Show Mode → 3 · Message** and
**Background Bot → 3 · Message**. EVAChat Flow has no AI settings anymore.

---

## 1. Setup (2 minutes)

1. Dashboard → **Message** → Chat mode → **API Chat**.
2. Fill in:
   * **API base URL** – default `https://api.featherless.ai/v1`
   * **Model** – default `deepseek-ai/DeepSeek-V3-0324`
   * **API key** – your provider key (local `data/` only, never logged)
   * **Temperature** – default `0.9`
   * **Share turn HOT / WARM / COLD** – default `4 / 6 / 8`
3. Add one snap handle per line in **Snap / Telegram users** — the share step
   needs it.
4. Press **Test API** once. It sends a single tiny message and shows the reply.
5. Press **Open API Chat Data Folder** to edit the persona (optional).
6. Close the dashboard and run **`test.bat`** → choose **API Chat** to chat with
   the bot locally before a real run (see `docs/LOCAL_TEST_CHAT.md`).

Any OpenAI-compatible endpoint works: OpenAI, Featherless, Groq, OpenRouter,
DeepSeek, Together, Gemini's OpenAI endpoint, a local llama.cpp server, etc.

## 2. What the mode does

* **Persona** — one random girl per conversation: age 20–25, country from
  `persona_countries.txt`, name/major/year/tonight/show/snack from the built-in
  lore lists. The persona is fixed for that conversation and never contradicts
  itself.
* **Director** (`internal/apichat/director.go`) — scores interest and decides
  every reply: mood rotation, ASL handling, name handling, hard-question dodge,
  bot-question deflection, pic requests, horny/flirty response, cold-streak
  hooks, no-question turns, double texting, snap hook, snap ask, share, and all
  hard gates (underage, insult, money, non-English, no-snap, repeat).
* **Share** — the model writes the lead-in, Go appends one random line from
  `sharesnap.txt` with the assigned `%username%`. A line is never sent twice in
  the same chat until the deck is exhausted, and the last line is never picked
  twice in a row. The model can never write a username or an `@`.
* **Guards** — `<think>` stripping, quote/list/prefix cleanup, lowercase, no
  `!`, line-length and repeated-emoji polish, plus the quality gate: repeats,
  known-fact questions, question spam, topic drift and AI talk are rejected
  with one retry. Details: `docs/API_CHAT_QUALITY.md`.
* **Fallback** — API error, timeout (25 s) or empty reply → one retry, then a
  neutral line from `fallback.txt`, so the stranger is never left hanging.

## 3. `data/api_chat/`

| File | Purpose |
|---|---|
| `persona.txt` | who the bot is; placeholders `{age} {country} {name} {major} {year} {tonight} {show} {snack}` |
| `persona_countries.txt` | countries the persona can come from, one per line |
| `sharesnap.txt` | snap share lines; `%username%` is replaced by the app (one random, never repeated) |
| `share.txt` | old name; read only when `sharesnap.txt` is missing |
| `fallback.txt` | lines used when the API fails |
| `README.txt` | this folder's own quick reference |

Files are seeded only when missing. Editing them never gets overwritten by an
app update; delete a file to restore the built-in starter on the next start.

## 4. Persistence

Per-user API Chat sessions (history + director state + persona) are stored in
`data/bot_state.json` under `api_chat_sessions`, exactly like EVAChat sessions.
A restart continues the same funnel instead of starting over.

## 5. Troubleshooting

| Symptom | Fix |
|---|---|
| `API Chat needs an API key` | Fill the key in Message → API Chat and save again. |
| `API 401` | Wrong key for this base URL. |
| `API 403 ... gated` | The model is gated on your plan. Try `Qwen/Qwen2.5-72B-Instruct`, `mistralai/Mistral-Nemo-Instruct-2407` or `NousResearch/Hermes-3-Llama-3.1-70B`. |
| Empty replies | A thinking model spent all tokens on reasoning. Raise max tokens or use a non-thinking model; the app already disables thinking for qwen3 / glm-4.5+ / deepseek-v3.1+ / kimi-k2.5+ / gpt-oss. |
| `Test API` works but chats fail | Check **Snap / Telegram users** — without a handle the share line is skipped (logged), never sent empty. |
| Every message repeats | You are running an old build; rebuild with `install.bat`. |
| Bot asks the same question again | Update to the current build — the fact ledger + guard block known-fact questions. |

## 6. Banglish quickstart

1. Message → **API Chat** select koro.
2. Base URL, Model, API key boshao → **Save**.
3. **Test API** chapo — reply ashle key thik.
4. **Snap / Telegram users** e handle dao (no hole share line jabe na).
5. **Open API Chat Data Folder** e `persona.txt` / `sharesnap.txt` moto khushi edit koro.
6. Start Bot / Run Browser — ekhon sob line model likhbe, snap username app
   nijei boshabe.
