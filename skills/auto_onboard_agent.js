// skills/auto_onboard_agent.js
// Open-Source Sub-Agent Skill: Universal Guest / 18+ / Female / Nickname Auto-Onboarder
// Automatically bypasses non-CAPTCHA pre-chat onboarding modals across all 5 site families:
// - Cookie consent ("Accept All", "I Agree")
// - 18+ age confirmation checkboxes & buttons ("Yes, I'm 18 or older", "I am 18+", "Agree")
// - Gender selection ("Female", "Woman", "I'm a girl")
// - Guest nickname / age inputs (auto-fills a natural female alias + age 21)
// - Text-only mode toggle ("Text Chat", "Chat anonymously", "No camera")
(function () {
  var NAMES = ["Sophia", "Emma", "Olivia", "Mia", "Chloe", "Ava", "Lily", "Zoe", "Hannah", "Maya"];

  function visible(el) {
    if (!el || el.nodeType !== 1) return false;
    var st = window.getComputedStyle(el);
    var r = el.getBoundingClientRect();
    return st.display !== "none" && st.visibility !== "hidden" && r.width > 0 && r.height > 0;
  }

  function setNativeValue(input, val) {
    if (!input) return;
    var setter = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, "value");
    if (setter && setter.set) setter.set.call(input, val);
    else input.value = val;
    input.dispatchEvent(new Event("input", { bubbles: true }));
    input.dispatchEvent(new Event("change", { bubbles: true }));
  }

  function runOnboardingStep() {
    var actions = [];
    // 1. Check any unchecked 18+ / terms / age checkboxes
    var checkboxes = Array.from(document.querySelectorAll('input[type="checkbox"]')).filter(visible);
    checkboxes.forEach(function (cb) {
      if (!cb.checked) {
        cb.click();
        actions.push("checkbox:18+");
      }
    });

    // 2. Fill empty nickname / username / display name input (if not the main chat composer)
    var textInputs = Array.from(document.querySelectorAll('input[type="text"], input:not([type])')).filter(visible);
    textInputs.forEach(function (inp) {
      if (inp.value && inp.value.trim() !== "") return;
      var hint = ((inp.placeholder || "") + " " + (inp.name || "") + " " + (inp.id || "") + " " + (inp.getAttribute("aria-label") || "")).toLowerCase();
      if (/(nick|name|handle|alias|username|your name)/i.test(hint) && !/(message|chat|type a|search)/i.test(hint)) {
        var alias = NAMES[Math.floor(Math.random() * NAMES.length)] + String(18 + Math.floor(Math.random() * 9));
        setNativeValue(inp, alias);
        actions.push("nickname:" + alias);
      }
    });

    // 3. Fill age input or select if present
    var numInputs = Array.from(document.querySelectorAll('input[type="number"], select')).filter(visible);
    numInputs.forEach(function (el) {
      var hint = ((el.name || "") + " " + (el.id || "") + " " + (el.placeholder || "") + " " + (el.getAttribute("aria-label") || "")).toLowerCase();
      if (/\bage\b/i.test(hint)) {
        if (el.tagName.toLowerCase() === "select") {
          var opt = Array.from(el.options || []).find(function (o) { return /^(20|21|22)$/.test(String(o.value || o.text).trim()); });
          if (opt && el.value !== opt.value) {
            el.value = opt.value;
            el.dispatchEvent(new Event("change", { bubbles: true }));
            actions.push("age:select:21");
          }
        } else if (!el.value) {
          setNativeValue(el, "21");
          actions.push("age:input:21");
        }
      }
    });

    // 4. Select "Female" / "Woman" / "I'm a girl" button or radio when choosing own gender
    var clickables = Array.from(document.querySelectorAll('button, label, [role="button"], [role="radio"]')).filter(visible);
    clickables.forEach(function (el) {
      var txt = (el.innerText || el.textContent || "").replace(/\s+/g, " ").trim();
      if (/^(♀\s*female|female|woman|i'm a girl|i am a girl|girl)$/i.test(txt)) {
        if (!el.classList.contains("active") && !el.classList.contains("selected") && el.getAttribute("aria-pressed") !== "true" && el.getAttribute("aria-checked") !== "true") {
          el.click();
          actions.push("gender:" + txt);
        }
      }
    });

    // 5. Click 18+ / Cookie / Guest entry confirmation buttons
    var modalBtns = clickables.filter(function (el) {
      var txt = (el.innerText || el.textContent || "").replace(/\s+/g, " ").trim();
      return /^(yes,?\s*i'?m 18( or older)?|i am 18( or older)?|agree|i agree|accept all|chat anonymously|continue as guest|guest login|yes,?\s*skip)$/i.test(txt);
    });
    if (modalBtns[0]) {
      modalBtns[0].click();
      actions.push("modal:" + (modalBtns[0].innerText || "").trim());
    }

    return actions;
  }

  window.__chitchatAutoOnboard = runOnboardingStep;
  return runOnboardingStep();
})();
