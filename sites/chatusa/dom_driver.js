// Isolated DOM Driver + Auto-Onboard Agent for site: ChatUSA.club (Fast Stranger Match) (chatusa | family_e)
// Edit selectors in site.json or customize this script directly.
// One site breaking or changing layout will never affect any other site.
const __sm = window.__chitchatUniversalSiteDOM || (() => {
  const CFG = {"composer_input":"textarea:not([disabled]), input[type=\"text\"]:not([disabled])[placeholder*=\"message\" i], input[type=\"text\"]:not([disabled])[placeholder*=\"type\" i], #messageInput, #chatInput","send_button":"button#sendBtn, button[type=\"submit\"], button.send-btn, button[aria-label*=\"Send\" i]","skip_button":"button#skipBtn, button#nextBtn, button.skip-btn, button.next-btn, button[aria-keyshortcuts=\"Escape\"]","start_button":"button#startBtn, button.start-btn, button.new-chat, button[type=\"submit\"]","message_history":"#messages, .messages, .chat-box, .chat-messages, .logbox, main","message_row":".message, .msg, .chat-msg, .logitem, li","message_text":".text, .msg-text, p, span","partner_message_selector":".stranger, .incoming, .partner, .other, .left","own_message_selector":".you, .outgoing, .self, .me, .right","partner_name":".stranger-name, .partner-name","chat_ended_text":"stranger has disconnected|partner left|chat ended|disconnected"};
  const FAMILY = "family_e";
  const ALIASES = ['Sophia', 'Emma', 'Olivia', 'Mia', 'Chloe', 'Ava', 'Lily', 'Zoe', 'Hannah', 'Maya'];
  const clean = s => (s || '').replace(/\s+/g, ' ').trim();
  const visible = el => {
    if (!el || el.nodeType !== 1 || el.closest('[hidden], [aria-hidden="true"]')) return false;
    const st = getComputedStyle(el), r = el.getBoundingClientRect();
    return st.display !== 'none' && st.visibility !== 'hidden' && r.width > 0 && r.height > 0;
  };
  const usable = el => visible(el) && !el.disabled && el.getAttribute('aria-disabled') !== 'true' && !el.readOnly;
  const findFirst = (sel, pred = visible) => {
    if (!sel) return null;
    try { return [...document.querySelectorAll(sel)].find(pred) || null; } catch (_) { return null; }
  };
  const setNativeValue = (inp, val) => {
    try {
      const desc = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, 'value');
      if (desc && desc.set) desc.set.call(inp, val);
      else inp.value = val;
      inp.dispatchEvent(new Event('input', { bubbles: true }));
      inp.dispatchEvent(new Event('change', { bubbles: true }));
    } catch (_) {}
  };
  const state = {
    document: String(Date.now()),
    generation: 1,
    phase: 'matching',
    rows: new Map(),
    nextOrder: 0,
    revision: 0,
    lastOnboardAt: 0
  };
  function autoOnboardIfNeeded(hasComposer) {
    const now = Date.now();
    if (hasComposer || now - state.lastOnboardAt < 1800) return;
    state.lastOnboardAt = now;
    try {
      // 1. Tick 18+ / terms / age checkboxes
      [...document.querySelectorAll('input[type="checkbox"]')].filter(visible).forEach(cb => {
        if (!cb.checked) cb.click();
      });
      // 2. Fill empty guest nickname / handle input (never touch composer)
      let savedAlias = '';
      try { savedAlias = localStorage.getItem('chitchat_guest_nickname') || localStorage.getItem('guest_nick') || ''; } catch (_) {}
      [...document.querySelectorAll('input[type="text"], input:not([type])')].filter(usable).forEach(inp => {
        if (inp.getAttribute('data-sm-composer') === 'true' || (inp.value && inp.value.trim() !== '')) return;
        const hint = ((inp.placeholder || '') + ' ' + (inp.name || '') + ' ' + (inp.id || '') + ' ' + (inp.getAttribute('aria-label') || '')).toLowerCase();
        if (/(nick|name|handle|alias|username|your name)/i.test(hint) && !/(message|chat|type a|write|send|search)/i.test(hint)) {
          const chosen = savedAlias || (ALIASES[Math.floor(Math.random() * ALIASES.length)] + String(19 + Math.floor(Math.random() * 6)));
          setNativeValue(inp, chosen);
        }
      });
      // 3. Fill age input/select if visible
      let savedAge = '21';
      try { savedAge = localStorage.getItem('chitchat_guest_age') || '21'; } catch (_) {}
      [...document.querySelectorAll('input[type="number"], select')].filter(usable).forEach(el => {
        const hint = ((el.name || '') + ' ' + (el.id || '') + ' ' + (el.placeholder || '') + ' ' + (el.getAttribute('aria-label') || '')).toLowerCase();
        if (/\bage\b/i.test(hint)) {
          if (el.tagName.toLowerCase() === 'select') {
            const opt = [...(el.options || [])].find(o => clean(o.value || o.text) === savedAge || /^(20|21|22)$/.test(clean(o.value || o.text)));
            if (opt && el.value !== opt.value) { el.value = opt.value; el.dispatchEvent(new Event('change', { bubbles: true })); }
          } else if (!el.value) {
            setNativeValue(el, savedAge);
          }
        }
      });
      // 4. Select Female / Woman / Girl option
      const clickables = [...document.querySelectorAll('button, label, a, [role="button"], [role="radio"]')].filter(visible);
      clickables.forEach(el => {
        const txt = clean(el.innerText || el.textContent);
        if (/^(♀\s*female|female|woman|i'm a girl|i am a girl|girl)$/i.test(txt)) {
          if (!el.classList.contains('active') && !el.classList.contains('selected') && el.getAttribute('aria-pressed') !== 'true' && el.getAttribute('aria-checked') !== 'true') {
            el.click();
          }
        }
      });
      // 5. Click 18+ / Cookie / Guest / Text-mode modal buttons
      const modalBtn = clickables.find(el => {
        const txt = clean(el.innerText || el.textContent);
        return /^(yes,?\s*i'?m 18( or older)?|i am 18( or older)?|agree|i agree|accept all|chat anonymously|continue as guest|guest login|text chat|text only|yes,?\s*skip)$/i.test(txt);
      });
      if (modalBtn) modalBtn.click();
    } catch (_) {}
  }
  function tagActiveComposer(composer) {
    try {
      [...document.querySelectorAll('[data-sm-composer="true"]')].forEach(el => {
        if (el !== composer) el.removeAttribute('data-sm-composer');
      });
      if (composer) composer.setAttribute('data-sm-composer', 'true');
    } catch (_) {}
  }
  function classifyAuthor(el, rawText) {
    if (CFG.own_message_selector) {
      try { if (el.matches(CFG.own_message_selector) || el.querySelector(CFG.own_message_selector)) return 'self'; } catch (_) {}
    }
    if (CFG.partner_message_selector) {
      try { if (el.matches(CFG.partner_message_selector) || el.querySelector(CFG.partner_message_selector)) return 'partner'; } catch (_) {}
    }
    const cls = ((el.className || '') + ' ' + (el.id || '') + ' ' + (el.getAttribute('data-author') || '') + ' ' + (el.getAttribute('data-from') || '')).toLowerCase();
    if (/\b(youmsg|self-msg|outgoing|self|own|mine|sent|from-me|by-me|right)\b/.test(cls)) return 'self';
    if (/\b(strangermsg|partner-msg|incoming|partner|stranger|them|other|received|from-them|left)\b/.test(cls)) return 'partner';
    if (/^(you|me)\s*:/i.test(rawText)) return 'self';
    return 'partner';
  }
  function stripSpeakerPrefix(text) {
    return clean(text.replace(/^(stranger|partner|you|me|guest[0-9]*)\s*:\s*/i, ''));
  }
  function detectProtectionState(bodyText) {
    try {
      if (document.querySelector('iframe[src*="challenges.cloudflare.com"], .cf-turnstile, [data-sitekey][class*="turnstile"]')) return 'cloudflare_turnstile';
      if (document.querySelector('iframe[src*="hcaptcha.com"], .h-captcha')) return 'hcaptcha';
      if (document.querySelector('iframe[src*="google.com/recaptcha"], .g-recaptcha')) return 'recaptcha';
      if (/\b(vpn detected|proxy detected|disable your vpn)\b/i.test(bodyText)) return 'vpn_blocked';
      if (/\b(another tab opened|multiple tabs|already open in another tab)\b/i.test(bodyText)) return 'single_tab_lock';
      if (/\b(too many requests|rate limit|daily limit|slow down)\b/i.test(bodyText)) return 'rate_limited';
    } catch (_) {}
    return 'clean';
  }
  function scan() {
    let composer = findFirst(CFG.composer_input, usable);
    autoOnboardIfNeeded(!!composer);
    if (!composer) composer = findFirst(CFG.composer_input, usable);
    tagActiveComposer(composer);
    const skipBtn = findFirst(CFG.skip_button, visible);
    const startBtn = findFirst(CFG.start_button, usable);
    const endedRe = CFG.chat_ended_text ? new RegExp(CFG.chat_ended_text, 'i') : null;
    const bodyText = document.body ? document.body.innerText : '';
    const protection = detectProtectionState(bodyText);
    const chatEnded = (endedRe && endedRe.test(bodyText)) || (!composer && !!startBtn);
    const phase = chatEnded ? 'lobby' : (composer ? 'chatting' : 'matching');
    if (phase !== state.phase && phase === 'chatting') {
      state.generation++;
      state.rows.clear();
      state.nextOrder = 0;
    }
    state.phase = phase;
    const history = findFirst(CFG.message_history, () => true) || document.body;
    let elements = [];
    try {
      elements = history && CFG.message_row ? [...history.querySelectorAll(CFG.message_row)] : [];
    } catch (_) {}
    elements.forEach((el, idx) => {
      const id = el.getAttribute('data-message-id') || el.id || ('row-' + idx);
      let textEl = el;
      if (CFG.message_text) {
        try { textEl = el.querySelector(CFG.message_text) || el; } catch (_) {}
      }
      const rawText = clean(textEl.textContent);
      if (!rawText) return;
      if (/^(stranger is typing|looking for a|connecting to|connected! say hi|chat ended|stranger has disconnected)/i.test(rawText)) return;
      const author = classifyAuthor(el, rawText);
      const text = stripSpeakerPrefix(rawText);
      if (!text) return;
      const old = state.rows.get(id);
      if (!old || old.text !== text) {
        state.rows.set(id, { id, order: old ? old.order : ++state.nextOrder, author, text });
        state.revision++;
      }
    });
    const partnerEl = findFirst(CFG.partner_name, () => true);
    const partner = clean(partnerEl ? partnerEl.textContent : 'stranger') || 'stranger';
    const buttons = [];
    if (chatEnded && startBtn) buttons.push('START');
    else if (skipBtn) buttons.push('SKIP');
    return {
      controls: {
        hasInput: !!composer,
        inputDesc: composer ? 'composer' : '',
        buttons,
        chatEnded: phase !== 'chatting',
        partner,
        ownName: 'self',
        conversation: state.document + ':' + state.generation,
        phase,
        protection,
        canSend: phase === 'chatting' && !!composer,
        observerReady: true,
        overflow: false
      },
      messages: [...state.rows.values()],
      revision: state.revision
    };
  }
  return {
    snapshot: () => scan(),
    probe: () => {
      const s = scan();
      return 'family=' + FAMILY + ' phase=' + s.controls.phase + ' input=' + s.controls.hasInput + ' msgs=' + s.messages.length + ' prot=' + (s.controls.protection || 'clean');
    },
    click: (labels) => {
      const want = (labels || []).map(l => String(l).toUpperCase());
      if (want.includes('SEND')) {
        const sb = findFirst(CFG.send_button, usable);
        if (sb) { sb.click(); return 'SEND'; }
      }
      if (want.includes('START')) {
        const b = findFirst(CFG.start_button, usable);
        if (b) { b.click(); return 'START'; }
      }
      if (want.includes('SKIP') || want.includes('CONFIRM?')) {
        const b = findFirst(CFG.skip_button, usable);
        if (b) { b.click(); return 'SKIP'; }
      }
      return '';
    },
    focusInput: () => {
      const c = findFirst(CFG.composer_input, usable);
      if (!c) return false;
      tagActiveComposer(c);
      c.focus();
      return document.activeElement === c;
    },
    readInput: () => {
      const c = findFirst(CFG.composer_input, usable);
      return c ? String(c.value || '') : '';
    },
    clearInput: () => {
      const c = findFirst(CFG.composer_input, usable);
      if (!c) return false;
      c.value = '';
      c.dispatchEvent(new Event('input', { bubbles: true }));
      return true;
    }
  };
})();
