====================================================================
ISOLATED SITE FOLDER: Camgo.com (Text & Video) (camgo)
====================================================================

This folder is 100% isolated from other sites and from the core bot engine.
- Account Setup Mode: auto_register
- Bot reply rules (Single Sequence, Folder Sequences, EVAChat Flow, API Chat),
  apichat.exe Persona & API Key, Timing, Proxy, and Usernames are shared
  automatically from the main engine.
- Only this site's URL, DOM selectors, protection notes, accounts, and browser sessions live here.

FILES IN THIS FOLDER:
1. site.json              -> Site Name, Start URL, Family Archetype, Account Mode,
                             and CSS selectors for chat input, skip/start buttons, and messages.
2. dom_driver.js          -> Isolated browser DOM script + Auto-Onboard Agent for this site.
3. camgo_protection.txt -> Anti-bot protection notes, entry flow, and safe pacing rules.
4. accounts/              -> Auto-registered or browser-captured account JSON files for this site.
5. profiles/              -> Saved browser sessions and login_info_*.json states for this site only.
