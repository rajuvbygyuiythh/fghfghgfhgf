API CHAT — editable data folder
===============================

This folder belongs to the API Chat mode. The whole conversation is written by
the model you configure in the dashboard (Message → API). These TXT files are
the only things the model is allowed to lean on:

  persona.txt            who the bot is (placeholders below are filled per chat)
  persona_countries.txt  countries the persona can be from, one per line
  sharesnap.txt          snap share lines; %username% is replaced by the app
                         (one random line per share, never repeated in one chat)
  share.txt              old name, read only when sharesnap.txt is missing
  fallback.txt           lines used when the API fails (never leave the user hanging)

Placeholders usable in persona.txt:
  {age} {country} {name} {major} {year} {tonight} {show} {snack}

Rules that are enforced in code (not written by the model):
  * the snap username is always sent by the app, never by the model
  * one guard retry when a reply breaks the persona rules
  * repeats, known-fact questions, question spam and off-topic lines are
    rejected before sending (see docs/API_CHAT_QUALITY.md)
  * the funnel gate (underage / insult / money / no-snap / language) always wins

Edit the files freely. They are only re-created when a file is missing, so your
changes are never overwritten by app updates. Delete a file to restore the
built-in starter version on the next start.

The API key is stored only in your local data settings file. It is never logged
and never written into this folder.
