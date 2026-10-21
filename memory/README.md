# Power-Safe Real-Time Project Memory (`memory/`)

This folder stores the live, disk-synced (`fsync`) operational memory of the bot across all 101 stranger chat sites (`sites/<site_id>/`). Even if the PC loses power (`কারেন্ট চলে গেলেও`), every incoming and outgoing SMS up to the exact millisecond is preserved on disk.

## Files in `memory/`
1. **`chat_history.jsonl`** — Append-only JSON Lines log synced with `os.File.Sync()` on every incoming (`in`), outgoing (`out`), `skip`, `system`, and `error` event.
2. **`operation_memory.json`** — Atomic snapshot of the currently active site (`active_site_id`), global incoming/outgoing counters, and per-site stats (`site_stats`).
3. **`sites_status.md`** — Complete registry of all 100 non-ChitChat stranger text-chat sites + ChitChat, including their Engine Family (`family_a` .. `family_e`), Start URL, Onboarding Flow, and link to `sites/<site_id>/<site_id>_protection.txt`.
