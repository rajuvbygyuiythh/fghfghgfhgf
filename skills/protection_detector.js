// skills/protection_detector.js
// Open-Source Sub-Agent Skill: Real-Time Anti-Bot & Site Protection Detector
// Inspects the live DOM for Cloudflare Turnstile, hCaptcha, reCAPTCHA, Didit Age
// Verification, VPN blocks, Multi-Tab locks, and Skip Cooldown rate limits.
(function () {
  function detectProtections() {
    var html = document.documentElement ? document.documentElement.innerHTML : "";
    var text = document.body ? document.body.innerText : "";
    var findings = [];

    if (/challenges\.cloudflare\.com\/turnstile|cf-turnstile/i.test(html) || /Quick verification/i.test(text)) {
      findings.push("cloudflare_turnstile");
    }
    if (/hcaptcha\.com|data-sitekey.*hcaptcha/i.test(html) || /verify you're human to continue/i.test(text)) {
      findings.push("hcaptcha");
    }
    if (/google\.com\/recaptcha|g-recaptcha/i.test(html)) {
      findings.push("recaptcha");
    }
    if (/didit|age verification required|verify your age/i.test(text)) {
      findings.push("age_verification_gate");
    }
    if (/vpn detected|disable your vpn|proxy detected/i.test(text)) {
      findings.push("vpn_ip_block");
    }
    if (/another tab opened|multiple tabs/i.test(text)) {
      findings.push("single_tab_lock");
    }
    if (/daily allowance|rate limit|slow down|wait a few seconds|banned|suspended/i.test(text)) {
      findings.push("rate_limit_or_ban");
    }

    return {
      url: location.href,
      protected: findings.length > 0,
      protections: findings,
      checkedAt: new Date().toISOString()
    };
  }

  window.__chitchatDetectProtection = detectProtections;
  return detectProtections();
})();
