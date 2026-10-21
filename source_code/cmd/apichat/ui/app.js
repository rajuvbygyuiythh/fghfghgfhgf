(() => {
  const $ = (id) => document.getElementById(id);

  let lastSettings = {};
  let currentDeck = {
    persona: "",
    sharesnap: "",
    fallback: "",
    countries: "",
  };

  function setBanner(msg, kind = "") {
    const el = $("studio-save-status");
    if (!el) return;
    el.textContent = msg;
    el.className = "status-banner" + (kind ? " " + kind : "");
  }

  function switchTab(tabName) {
    document.querySelectorAll(".studio-tab").forEach((btn) => {
      btn.classList.toggle("active", btn.dataset.tab === tabName);
    });
    document.querySelectorAll(".tab-view").forEach((sec) => {
      sec.classList.toggle("active", sec.id === "tab-" + tabName);
    });
  }

  async function callStudio(method, arg) {
    const bridge = window.go?.main?.StudioApp;
    if (bridge && typeof bridge[method] === "function") {
      return arg === undefined ? await bridge[method]() : await bridge[method](arg);
    }
    const opts = {
      method: arg === undefined ? "GET" : "POST",
      headers: { "Content-Type": "application/json" },
    };
    if (arg !== undefined) {
      const body = method === "SendTestChatMessage" && typeof arg === "string" ? { message: arg } : arg;
      opts.body = JSON.stringify(body);
    }
    const res = await fetch(`/api/${method}`, opts);
    const data = await res.json().catch(() => ({}));
    if (!res.ok) {
      throw new Error(data.error || `HTTP ${res.status}`);
    }
    return data;
  }

  function collectSettings() {
    return Object.assign({}, lastSettings, {
      chat_mode: lastSettings.chat_mode || "api_chat",
      ai_base_url: ($("studio-base-url")?.value || "").trim(),
      ai_key_unused: undefined,
      ai_api_key: ($("studio-api-key")?.value || "").trim(),
      ai_model: ($("studio-model")?.value || "").trim(),
      api_temperature: parseFloat($("studio-temperature")?.value || "0.9") || 0.9,
      api_share_hot: parseInt($("studio-share-hot")?.value || "4", 10) || 4,
      api_share_warm: parseInt($("studio-share-warm")?.value || "6", 10) || 6,
      api_share_cold: parseInt($("studio-share-cold")?.value || "8", 10) || 8,
      usernames: ($("studio-usernames")?.value || "").trim(),
    });
  }

  function collectSaveRequest() {
    currentDeck.persona = $("studio-persona")?.value || "";
    return {
      settings: collectSettings(),
      deck: Object.assign({}, currentDeck),
    };
  }

  function applyStudioState(st) {
    if (!st) return;
    const s = st.settings || {};
    lastSettings = Object.assign({}, s);
    $("studio-base-url").value = s.ai_base_url || "";
    $("studio-api-key").value = s.ai_api_key || "";
    $("studio-model").value = s.ai_model || "";
    $("studio-temperature").value = s.api_temperature ?? 0.9;
    $("studio-share-hot").value = s.api_share_hot ?? 4;
    $("studio-share-warm").value = s.api_share_warm ?? 6;
    $("studio-share-cold").value = s.api_share_cold ?? 8;
    $("studio-usernames").value = s.usernames || "";

    if (st.deck) {
      currentDeck = Object.assign({}, currentDeck, st.deck);
      $("studio-persona").value = currentDeck.persona || "";
    }
    if (st.chat && st.chat.rendered_persona) {
      $("studio-rendered-persona").textContent = st.chat.rendered_persona;
    }
  }

  async function loadStudioState() {
    try {
      const st = await callStudio("GetStudioState");
      applyStudioState(st);
    } catch (err) {
      setBanner("Failed to load settings: " + (err.message || err), "err");
    }
  }

  async function saveAll(silent = false) {
    try {
      const req = collectSaveRequest();
      const saved = await callStudio("SaveStudioState", req);
      applyStudioState(saved);
      if (!silent) {
        setBanner(
          "Saved & synced! Both apichat.exe and chitchatui.exe now use your updated settings & Persona prompt.",
          "ok"
        );
      }
      return true;
    } catch (err) {
      setBanner("Save failed: " + (err.message || err), "err");
      return false;
    }
  }

  async function testAPI() {
    setBanner("Testing API connection...", "");
    try {
      const req = collectSaveRequest();
      const summary = await callStudio("TestAPIConnection", req);
      setBanner(summary, "ok");
      appendLogicSystemLog(`API Connection Test OK — ${summary}`);
    } catch (err) {
      const msg = err.message || String(err);
      setBanner("API Test Failed: " + msg, "err");
      appendLogicSystemLog(`API Connection Test FAILED: ${msg}`, true);
    }
  }

  function clearTranscriptIfEmpty(container) {
    const empty = container?.querySelector(".chat-empty");
    if (empty) empty.remove();
  }

  function appendChatRow(kind, text, meta = "") {
    const log = $("studio-chat-log");
    if (!log) return;
    clearTranscriptIfEmpty(log);

    const row = document.createElement("div");
    row.className = `chat-row ${kind}`;
    const body = document.createElement("div");
    body.textContent = text;
    row.appendChild(body);

    if (meta) {
      const m = document.createElement("div");
      m.className = "chat-meta";
      m.textContent = meta;
      row.appendChild(m);
    }
    log.appendChild(row);
    log.scrollTop = log.scrollHeight;
  }

  function appendLogicSystemLog(message, isWarn = false) {
    const list = $("studio-logic-logs");
    if (!list) return;
    clearTranscriptIfEmpty(list);

    const card = document.createElement("div");
    card.className = "log-card" + (isWarn ? " warn" : "");
    const title = document.createElement("div");
    title.className = "log-title";
    title.textContent = message;
    card.appendChild(title);
    list.appendChild(card);
    list.scrollTop = list.scrollHeight;
  }

  function formatPersonaBadge(st) {
    if (!st) return "20F";
    const name = st.persona_name ? `${st.persona_name}, ` : "";
    const age = st.persona_age || 20;
    const country = st.persona_country ? ` (${st.persona_country})` : "";
    return `${name}${age}F${country}`;
  }

  function appendLogicTurnLog(userMessage, turn) {
    const list = $("studio-logic-logs");
    if (!list || !turn) return;
    clearTranscriptIfEmpty(list);

    const st = turn.state || {};
    const diag = (turn.diagnostic || "").trim();
    const hasWarn = Boolean(
      diag &&
        (diag.includes("retry") ||
          diag.includes("safe reply") ||
          diag.includes("api error") ||
          diag.includes("kept 1") ||
          diag.includes("empty"))
    );

    const card = document.createElement("div");
    card.className = "log-card" + (hasWarn ? " warn" : turn.tag === "SNAP SHARE" ? " snap" : "");

    const head = document.createElement("div");
    head.className = "log-head";

    const title = document.createElement("div");
    title.className = "log-title";
    const ruleLabel = turn.rule_why || turn.reason || turn.tag || "bot reply";
    title.textContent = `Turn ${st.turn || 1} · Logic Rule: ${ruleLabel}`;
    head.appendChild(title);

    const badges = document.createElement("div");
    badges.className = "log-badges";

    const tierBadge = document.createElement("span");
    tierBadge.className = "badge";
    tierBadge.textContent = `Tier: ${(st.tier || "COLD").toUpperCase()} · Mood: ${turn.mood || st.style || "chill"}`;
    badges.appendChild(tierBadge);

    const healthBadge = document.createElement("span");
    healthBadge.className = "badge " + (hasWarn ? "warn" : "ok");
    healthBadge.textContent = hasWarn ? `Prompt/Model Note: ${diag}` : "Prompt & Logic OK";
    badges.appendChild(healthBadge);

    head.appendChild(badges);
    card.appendChild(head);

    const grid = document.createElement("div");
    grid.className = "log-grid";

    const addRow = (label, value) => {
      if (!value) return;
      const l = document.createElement("div");
      l.className = "log-label";
      l.textContent = label;
      const v = document.createElement("div");
      v.className = "log-val";
      v.textContent = value;
      grid.appendChild(l);
      grid.appendChild(v);
    };

    if (userMessage) {
      addRow(
        "Stranger Input:",
        `"${userMessage}"` + (turn.user_kind ? `  →  Detected as [${turn.user_kind}]` : "")
      );
    } else {
      addRow("Trigger:", "New Chat Started (Turn 1 Opener)");
    }

    addRow("Why Bot Replied:", turn.instruction || `Executed rule "${ruleLabel}"`);
    addRow("Final Sent Reply:", (turn.lines || []).join(" | ") || "(silent)");

    if (turn.raw_output && turn.raw_output !== (turn.lines || []).join("\n")) {
      addRow("AI Raw Output:", turn.raw_output);
    }

    const facts = Array.isArray(st.known_facts)
      ? st.known_facts.join(" · ")
      : String(st.known_facts || "").trim();
    const locks = Array.isArray(st.never_repeat)
      ? st.never_repeat.join(", ")
      : String(st.never_repeat || "").trim();
    if (facts || locks) {
      addRow("Bot Memory:", [facts, locks ? `Locked: ${locks}` : ""].filter(Boolean).join(" | "));
    }

    card.appendChild(grid);
    list.appendChild(card);
    list.scrollTop = list.scrollHeight;
  }

  function renderTurnState(turn) {
    if (!turn || !turn.state) return;
    const st = turn.state;
    const stateEl = $("studio-chat-state");
    if (stateEl) {
      stateEl.textContent = `${formatPersonaBadge(st)} · Turn ${st.turn || 0} · Tier ${(st.tier || "COLD").toUpperCase()} (score ${st.score || 0})`;
    }
    const promptEl = $("studio-rendered-persona");
    if (promptEl && st.rendered_persona) {
      promptEl.textContent = st.rendered_persona;
    }
  }

  function renderBotTurn(turn, userMessage = "") {
    if (!turn) return;
    renderTurnState(turn);
    const lines = turn.lines || [];
    const shortRule = turn.rule_why || (turn.reason || "").split("|")[0].trim() || "reply";
    const metaLabel = [turn.tag || "BOT", shortRule].filter(Boolean).join(" · ");
    for (const line of lines) {
      appendChatRow("bot", line, metaLabel);
    }
    if (turn.note) {
      appendChatRow("sys", turn.note);
    }
    appendLogicTurnLog(userMessage, turn);
  }

  async function startChat() {
    try {
      const req = collectSaveRequest();
      const turn = await callStudio("StartTestChat", {
        mode: "api_chat",
        settings: req.settings,
        deck: req.deck,
      });
      const log = $("studio-chat-log");
      if (log) log.innerHTML = "";
      appendChatRow("sys", `New chat started · Bot persona: ${formatPersonaBadge(turn?.state)}`);
      renderBotTurn(turn, "");
      $("studio-chat-input")?.focus();
    } catch (err) {
      const msg = err.message || String(err);
      appendChatRow("sys", "Error starting chat: " + msg);
      appendLogicSystemLog("Error starting chat: " + msg, true);
    }
  }

  async function sendChatMessage(overrideText) {
    const input = $("studio-chat-input");
    const msg = (typeof overrideText === "string" ? overrideText : input?.value || "").trim();
    if (!msg) return;
    if (typeof overrideText !== "string" && input) input.value = "";
    appendChatRow("stranger", msg, "You (Stranger)");
    try {
      const turn = await callStudio("SendTestChatMessage", msg);
      renderBotTurn(turn, msg);
    } catch (err) {
      const errMsg = err.message || String(err);
      appendChatRow("sys", "Error: " + errMsg);
      appendLogicSystemLog("Chat error: " + errMsg, true);
    }
  }

  function bindEvents() {
    // 3 Top Tabs
    document.querySelectorAll(".studio-tab").forEach((btn) => {
      btn.addEventListener("click", () => switchTab(btn.dataset.tab));
    });

    // Save & Test API
    $("studio-save")?.addEventListener("click", () => saveAll(false));
    $("studio-save-settings")?.addEventListener("click", () => saveAll(false));
    $("studio-test-api")?.addEventListener("click", testAPI);

    // Save Persona & jump directly to Chat Test
    $("studio-apply-persona")?.addEventListener("click", async () => {
      switchTab("chat");
      await startChat();
    });

    // Load .txt file into the single Persona box
    $("studio-load-txt-btn")?.addEventListener("click", () => {
      $("studio-load-txt-input")?.click();
    });

    $("studio-load-txt-input")?.addEventListener("change", async (e) => {
      const file = e.target.files?.[0];
      if (!file) return;
      const text = await file.text();
      $("studio-persona").value = text;
      currentDeck.persona = text;
      setBanner(`Loaded "${file.name}" into Persona Prompt. Click "Save Persona & Test Chat" to apply.`, "ok");
      e.target.value = "";
    });

    // Export .txt file from the Persona box
    $("studio-export-txt-btn")?.addEventListener("click", () => {
      const content = $("studio-persona")?.value || "";
      const blob = new Blob([content], { type: "text/plain;charset=utf-8" });
      const a = document.createElement("a");
      a.href = URL.createObjectURL(blob);
      a.download = "persona.txt";
      a.click();
      URL.revokeObjectURL(a.href);
    });

    // Reset Default Persona
    $("studio-reset-deck")?.addEventListener("click", async () => {
      try {
        const deck = await callStudio("ResetPersonaDeck");
        currentDeck = Object.assign({}, currentDeck, deck);
        $("studio-persona").value = currentDeck.persona || "";
        setBanner("Persona Prompt reset to built-in default.", "ok");
      } catch (err) {
        setBanner("Reset failed: " + (err.message || err), "err");
      }
    });

    // Open data/api_chat folder
    $("studio-open-folder")?.addEventListener("click", async () => {
      try {
        const msg = await callStudio("OpenAPIChatFolder");
        setBanner(msg || "Opened data/api_chat folder.", "ok");
      } catch (err) {
        setBanner("Open folder error: " + (err.message || err), "err");
      }
    });

    // Quick-send buttons
    document.querySelectorAll(".quick-msg").forEach((btn) => {
      btn.addEventListener("click", () => {
        const msg = btn.dataset.msg || "";
        if (msg) sendChatMessage(msg);
      });
    });

    // Chat Test controls
    $("studio-chat-start")?.addEventListener("click", startChat);
    $("studio-chat-clear")?.addEventListener("click", () => {
      const log = $("studio-chat-log");
      if (log) {
        log.innerHTML = '<div class="chat-empty">Chat cleared. Click <b>New Chat</b> or type below to test again.</div>';
      }
    });
    $("studio-logs-clear")?.addEventListener("click", () => {
      const list = $("studio-logic-logs");
      if (list) {
        list.innerHTML = '<div class="chat-empty">Logs cleared. Send a chat message to inspect bot logic.</div>';
      }
    });
    $("studio-chat-send")?.addEventListener("click", () => sendChatMessage());
    $("studio-chat-input")?.addEventListener("keydown", (e) => {
      if (e.key === "Enter") {
        e.preventDefault();
        sendChatMessage();
      }
    });
  }

  window.addEventListener("DOMContentLoaded", async () => {
    bindEvents();
    await loadStudioState();
  });
})();
