# Sub-Agent Browser Skills (`skills/`)

This directory contains open-source browser reverse-engineering and autonomous onboarding skills used across all 100+ isolated sites (`sites/<site_id>/`) and integrated with `source_code/capture/tool/hooks.js`:

1. **`dom_auto_scout.js` (`window.__chitchatAutoScout`)**
   - Heuristically discovers CSS selectors for `composer_input`, `send_button`, `skip_button`, `start_button`, `message_history`, `message_row`, `partner_message_selector`, and `own_message_selector` on any stranger chat site.
2. **`ws_protocol_sniffer.js` (`window.__chitchatWSFrames`)**
   - Hooks `window.WebSocket` to classify real-time Socket.IO / JSON frames (`match_found`, `incoming_sms`, `outgoing_sms`, `typing`, `chat_ended`).
3. **`auto_onboard_agent.js` (`window.__chitchatAutoOnboard`)**
   - Automatically handles pre-chat guest onboarding across all 5 site families: ticks `18+` checkboxes, selects `Female / Woman / I'm a girl`, auto-fills guest nicknames & age `21`, and clicks `Agree / Continue / Chat Anonymously / Yes, skip`.
4. **`protection_detector.js` (`window.__chitchatDetectProtection`)**
   - Detects Cloudflare Turnstile, hCaptcha, reCAPTCHA, Didit Age Verification, VPN blocks (`VPN Detected`), single-tab locks (`Another Tab Opened`), and skip cooldowns in real time.
