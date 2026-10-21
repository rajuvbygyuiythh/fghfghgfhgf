// skills/dom_auto_scout.js
// Open-Source Sub-Agent Skill: Universal Heuristic DOM Selector Scout
// Automatically discovers composer_input, send_button, skip_button, start_button,
// message_history, message_row, partner_message_selector, and own_message_selector
// on any unknown stranger text-chat website and outputs a ready-to-save site.json.
(function () {
  function isVisible(el) {
    if (!el || el.nodeType !== 1) return false;
    var st = window.getComputedStyle(el);
    var r = el.getBoundingClientRect();
    return st.display !== "none" && st.visibility !== "hidden" && r.width > 0 && r.height > 0;
  }

  function cssPath(el) {
    if (!el || el.nodeType !== 1) return "";
    if (el.id) return "#" + CSS.escape(el.id);
    var tag = el.tagName.toLowerCase();
    var aria = el.getAttribute("aria-label");
    if (aria) return tag + '[aria-label="' + aria.replace(/"/g, '\\"') + '"]';
    var ph = el.getAttribute("placeholder");
    if (ph) return tag + '[placeholder*="' + ph.slice(0, 24).replace(/"/g, '\\"') + '"]';
    if (typeof el.className === "string" && el.className.trim()) {
      var classes = el.className.trim().split(/\s+/).filter(function (c) {
        return c && !/^(active|focus|hover|open|show|hidden|ng-|svelte-|css-)/i.test(c);
      }).slice(0, 2);
      if (classes.length) return tag + "." + classes.join(".");
    }
    return tag;
  }

  function scoutSelectors() {
    var inputs = Array.from(document.querySelectorAll('textarea, input[type="text"], [contenteditable="true"]')).filter(isVisible);
    var composer = inputs.find(function (el) {
      var hint = ((el.placeholder || "") + " " + (el.getAttribute("aria-label") || "") + " " + (el.className || "") + " " + (el.id || "")).toLowerCase();
      return /(message|chat|type|write|say|send|stranger|reply)/i.test(hint);
    }) || inputs[inputs.length - 1] || null;

    var buttons = Array.from(document.querySelectorAll('button, input[type="button"], input[type="submit"], a[role="button"]')).filter(isVisible);
    var findBtn = function (re) {
      return buttons.find(function (b) {
        var t = ((b.innerText || b.value || "") + " " + (b.getAttribute("aria-label") || "") + " " + (b.title || "") + " " + (b.className || "")).trim();
        return re.test(t);
      }) || null;
    };

    var sendBtn = findBtn(/\b(send|submit|enter|reply)\b/i);
    var skipBtn = findBtn(/\b(skip|next|stop|leave|disconnect|end|esc|new stranger)\b/i);
    var startBtn = findBtn(/\b(start|new chat|find a stranger|find partner|talk|connect|chat now|go)\b/i);

    var containers = Array.from(document.querySelectorAll("ol, ul, div, main, section")).filter(function (el) {
      if (!isVisible(el)) return false;
      var hint = ((el.className || "") + " " + (el.id || "") + " " + (el.getAttribute("aria-label") || "") + " " + (el.getAttribute("role") || "")).toLowerCase();
      return /(messages|chat-log|logbox|conversation|chat-history|message-list|chat-box|stream)/i.test(hint);
    });

    return {
      url: location.href,
      title: document.title || "",
      selectors: {
        composer_input: composer ? cssPath(composer) : 'textarea:not([disabled]), input[type="text"]:not([disabled])',
        send_button: sendBtn ? cssPath(sendBtn) : 'button[type="submit"]',
        skip_button: skipBtn ? cssPath(skipBtn) : 'button[aria-keyshortcuts="Escape"], button.skip-btn',
        start_button: startBtn ? cssPath(startBtn) : 'button.start-btn, button.new-chat',
        message_history: containers[0] ? cssPath(containers[0]) : '.chat-messages, .message-list, .logbox, main',
        message_row: '.message-row, .logitem, .message, .chat-msg, li[data-message-id]',
        message_text: '.message-text, .msg-text, p, span',
        partner_message_selector: '.strangermsg, .partner-msg, .incoming, .them, .other, [data-author="partner"]',
        own_message_selector: '.youmsg, .self-msg, .outgoing, .me, .own, [data-author="self"]'
      }
    };
  }

  window.__chitchatAutoScout = scoutSelectors;
  return scoutSelectors();
})();
