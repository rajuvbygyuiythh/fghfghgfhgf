// ChitChat Bot — two self-contained pages.
//
// Show Mode: Setting + Proxy + Message + Run Browser + Live Logs. One click
// saves the three stores and launches the visible browser; the Go runner emits
// rule-aware events for the live log.
// Background Bot: Login + Proxy + Message + Settings + Live Logs + Start/Stop.
// Its live log is fed by the API workers (IN/OUT/skip/system/error).
//
// No backend behavior is merged: StartBot and the Show Mode DOM runner stay
// separate Go paths.

const state = {
  snapshot: null,
  dashTab: "control-center",
  tab: "show",
  runningShow: false,
  runningBot: false,
  busy: { show: false, bot: false },
  loadedSequences: { sm: 0, bg: 0 },
  profiles: [],
  siteFilter: "all",
  smgrGroup: "ALL",
  smgrStatusFilter: "all",
  smgrSessionFilter: "all",
  collapsedGroups: {},
  selectedSites: [],
  siteStatuses: {},
  sessionStatuses: {},
  sessFilterStatus: "all",
  detailOpen: true,
  activeSub: { show: "show-setting", bot: "bg-login" },
  showLog: { after: 0, entries: [], pending: false },
  botLog: { after: 0, entries: [], pending: false },
};

// Global form store: every field is bound by id, so switching sub-pages never
// loses typed values (the DOM may hide, but the store is the source of truth).
const formStore = { show: {}, bot: {} };

function fieldValue(el) {
  return el.type === "checkbox" ? el.checked : el.value;
}

function setFieldValue(el, stored) {
  if (el.type === "checkbox") el.checked = !!stored;
  else el.value = stored;
}

function captureGlobalState() {
  for (const [slice, panel] of [["show", '[data-panel="show"]'], ["bot", '[data-panel="bot"]']]) {
    document.querySelectorAll(`${panel} input, ${panel} select, ${panel} textarea`).forEach((el) => {
      if (el.id) formStore[slice][el.id] = fieldValue(el);
    });
  }
}

function restoreGlobalState(slice) {
  const panel = slice === "show" ? '[data-panel="show"]' : '[data-panel="bot"]';
  document.querySelectorAll(`${panel} input, ${panel} select, ${panel} textarea`).forEach((el) => {
    if (!el.id || !(el.id in formStore[slice]) || el === document.activeElement) return;
    setFieldValue(el, formStore[slice][el.id]);
  });
}

function bindGlobalState() {
  for (const [slice, panel] of [["show", '[data-panel="show"]'], ["bot", '[data-panel="bot"]']]) {
    document.querySelectorAll(`${panel} input, ${panel} select, ${panel} textarea`).forEach((el) => {
      if (!el.id) return;
      formStore[slice][el.id] = fieldValue(el);
      const save = () => { formStore[slice][el.id] = fieldValue(el); };
      el.addEventListener("input", save);
      el.addEventListener("change", save);
    });
  }
}

function byId(id) { return document.getElementById(id); }
function value(id) { const el = byId(id); return el ? String(el.value || "").trim() : ""; }
function numberValue(id) { return Number(byId(id).value); }

function backend() {
  return window.go && window.go.main && window.go.main.App;
}

async function invoke(method, ...args) {
  try {
    const api = backend();
    if (!api || typeof api[method] !== "function") throw new Error(`Wails binding not ready: ${method}`);
    return await api[method](...args);
  } catch (error) {
    reportFrontendError(`Backend ${method} failed`, error);
    throw error;
  }
}

function runtimeEvents() { return window.runtime || {}; }

function reportFrontendError(context, error) {
  try {
    const runtime = runtimeEvents();
    if (typeof runtime.LogError === "function") {
      const detail = error && error.stack ? error.stack : String(error);
      runtime.LogError(`Frontend | ${context} | ${detail}`);
    }
  } catch (_) { /* logging bridge unavailable */ }
}

function setStatus(id, message, kind) {
  const el = byId(id);
  if (!el) return;
  el.textContent = message;
  el.classList.toggle("status-error", kind === "error");
  el.classList.toggle("status-ok", kind === "ok");
}

// ---------------------------------------------------------------------------
// Tabs / running badges
// ---------------------------------------------------------------------------
function normalizeDashboardTab(tab) {
  if (!tab) return null;
  const t = String(tab).toLowerCase();
  if (t === "control-center" || t === "control" || t === "tab1") return "control-center";
  if (t === "site-manager" || t === "sites" || t === "tab2") return "site-manager";
  if (t === "sessions" || t === "session" || t === "tab3") return "sessions";
  return null;
}

function activateDashboardTab(dashTab) {
  const normalized = normalizeDashboardTab(dashTab) || "control-center";
  state.dashTab = normalized;
  document.querySelectorAll(".dash-tab-button").forEach((button) => {
    const active = button.dataset.dashTab === normalized;
    button.classList.toggle("active", active);
    button.setAttribute("aria-selected", active ? "true" : "false");
  });
  document.querySelectorAll(".dash-tab-panel").forEach((panel) => {
    const active = panel.dataset.dashPanel === normalized;
    panel.classList.toggle("active", active);
    panel.hidden = !active;
  });
  document.querySelectorAll(".dash-sidebar-section").forEach((sec) => {
    const active = sec.dataset.dashSidebar === normalized;
    sec.classList.toggle("active", active);
    sec.hidden = !active;
  });
  if (normalized === "site-manager") {
    renderSiteManager();
  } else if (normalized === "sessions") {
    renderSessionsTab();
    refreshSessionStatuses();
  }
}

function activateSubpanel(panel) {
  if (!panel) return;
  const mode = panel.startsWith("bg-") ? "bot" : "show";
  state.activeSub[mode] = panel;
  document.querySelectorAll(".subpanel").forEach((el) => {
    el.classList.toggle("active", el.dataset.subpanel === panel);
  });
  document.querySelectorAll(".subnav-button").forEach((button) => {
    button.classList.toggle("active", button.dataset.subpanel === panel);
  });
  restoreGlobalState(mode);
  if (panel === "show-logs") refreshShowLog();
  if (panel === "bg-logs") refreshBotLog();
}

function activateTab(tab) {
  const dashTarget = normalizeDashboardTab(tab);
  if (dashTarget) {
    activateDashboardTab(dashTarget);
    return;
  }
  if (state.dashTab !== "control-center") {
    activateDashboardTab("control-center");
  }
  state.tab = tab === "bot" ? "bot" : "show";
  document.querySelectorAll(".nav-button").forEach((button) => {
    button.classList.toggle("active", button.dataset.tab === state.tab);
  });
  document.querySelectorAll(".tab-panel").forEach((panel) => {
    panel.classList.toggle("active", panel.dataset.panel === state.tab);
  });
  const showSubnav = byId("subnav-show");
  const botSubnav = byId("subnav-bot");
  if (showSubnav) showSubnav.hidden = state.tab !== "show";
  if (botSubnav) botSubnav.hidden = state.tab !== "bot";
  activateSubpanel(state.activeSub[state.tab]);
  if (state.tab === "show") refreshShowLog();
  if (state.tab === "bot") refreshBotLog();
}

function bindTabs() {
  document.querySelectorAll(".dash-tab-button").forEach((button) => {
    button.addEventListener("click", () => activateDashboardTab(button.dataset.dashTab));
  });
  document.querySelectorAll(".nav-button").forEach((button) => {
    button.addEventListener("click", () => activateTab(button.dataset.tab));
  });
  document.querySelectorAll(".subnav-button").forEach((button) => {
    button.addEventListener("click", () => activateSubpanel(button.dataset.subpanel));
  });
}

function renderRunning() {
  const pill = byId("running-pill");
  pill.textContent = state.runningBot ? "Running" : "Stopped";
  pill.className = `pill ${state.runningBot ? "running" : "stopped"}`;

  const badge = byId("show-run-badge");
  badge.textContent = state.runningShow ? "Running" : "Idle";
  badge.classList.toggle("live", state.runningShow);

  byId("sm-run").disabled = state.runningShow || state.busy.show;
  byId("bg-start").disabled = state.runningBot || state.busy.bot;
  byId("bg-stop").disabled = (!state.runningBot && !state.busy.bot) || state.busy.bot;
}

function setBusy(which, busy) {
  state.busy[which] = busy;
  renderRunning();
}

// ---------------------------------------------------------------------------
// Settings: first message + timing (per page prefix)
// ---------------------------------------------------------------------------
function firstValues(settings) {
  const s = settings || {};
  return {
    mode: s.mode || "bot",
    bot_first_percent: s.bot_first_percent ?? 70,
    wait_timeout: !!s.wait_timeout,
    wait_min_seconds: s.wait_min_seconds ?? 30,
    wait_max_seconds: s.wait_max_seconds ?? 60,
  };
}

function updateFirst(prefix) {
  const mode = byId(`${prefix}-first-mode`).value;
  const wait = byId(`${prefix}-first-wait`).value === "skip";
  byId(`${prefix}-first-percent-row`).hidden = mode !== "random";
  byId(`${prefix}-first-percent`).disabled = mode !== "random";
  byId(`${prefix}-first-wait-row`).hidden = mode === "bot";
  byId(`${prefix}-first-wait`).disabled = mode === "bot";
  byId(`${prefix}-first-timeout-row`).hidden = mode === "bot" || !wait;
  byId(`${prefix}-first-min`).disabled = mode === "bot" || !wait;
  byId(`${prefix}-first-max`).disabled = mode === "bot" || !wait;
}

function fillFirst(prefix, settings) {
  const v = firstValues(settings);
  byId(`${prefix}-first-mode`).value = v.mode;
  byId(`${prefix}-first-percent`).value = v.bot_first_percent;
  byId(`${prefix}-first-wait`).value = v.wait_timeout ? "skip" : "unlimited";
  byId(`${prefix}-first-min`).value = v.wait_min_seconds;
  byId(`${prefix}-first-max`).value = v.wait_max_seconds;
  updateFirst(prefix);
}

function readFirst(prefix) {
  const mode = byId(`${prefix}-first-mode`).value;
  if (!["bot", "user", "random"].includes(mode)) throw new Error("Choose a first-message policy.");
  const percent = Number(byId(`${prefix}-first-percent`).value);
  if (!Number.isInteger(percent) || percent < 0 || percent > 100) {
    throw new Error("Bot first chance must be a whole number from 0 to 100.");
  }
  const wait = byId(`${prefix}-first-wait`).value === "skip";
  const min = Number(byId(`${prefix}-first-min`).value);
  const max = Number(byId(`${prefix}-first-max`).value);
  if (!Number.isInteger(min) || !Number.isInteger(max) || min < 0 || max < min || max > 86400) {
    throw new Error("First-message timeout: use whole seconds, 0 ≤ min ≤ max ≤ 86400.");
  }
  if (mode !== "bot" && wait && min < 1) throw new Error("A first-message timeout needs a positive minimum.");
  return {
    mode,
    bot_first_percent: percent,
    wait_timeout: mode !== "bot" && wait,
    wait_min_seconds: min,
    wait_max_seconds: max,
  };
}

function timingPair(minId, maxId, label, floor) {
  const min = numberValue(minId);
  const max = numberValue(maxId);
  if (!Number.isInteger(min) || !Number.isInteger(max) || min < floor || max < min || max > 86400) {
    throw new Error(`${label}: use whole seconds with ${floor} ≤ min ≤ max ≤ 86400.`);
  }
}

// ---------------------------------------------------------------------------
// Show Mode settings
// ---------------------------------------------------------------------------
function fillShowSettings(settings) {
  const s = settings || {};
  fillFirst("sm", s.first_message);
  byId("sm-reply-min").value = s.reply_delay_min_seconds ?? 0;
  byId("sm-reply-max").value = s.reply_delay_max_seconds ?? 0;
  byId("sm-auto-skip").checked = !!s.auto_skip;
  byId("sm-noreply-min").value = s.no_reply_min_seconds ?? 60;
  byId("sm-noreply-max").value = s.no_reply_max_seconds ?? 60;
  byId("sm-next-min").value = s.next_chat_min_seconds ?? 0;
  byId("sm-next-max").value = s.next_chat_max_seconds ?? 0;
}

function readShowSettings() {
  const first = readFirst("sm");
  timingPair("sm-reply-min", "sm-reply-max", "Reply delay", 0);
  timingPair("sm-noreply-min", "sm-noreply-max", "No-reply timeout", 0);
  timingPair("sm-next-min", "sm-next-max", "Next-chat wait", 0);
  const autoSkip = byId("sm-auto-skip").checked;
  if (autoSkip && numberValue("sm-noreply-min") < 1) {
    throw new Error("Auto skip needs a positive no-reply timeout.");
  }
  return {
    first_message: first,
    reply_delay_min_seconds: numberValue("sm-reply-min"),
    reply_delay_max_seconds: numberValue("sm-reply-max"),
    auto_skip: autoSkip,
    no_reply_min_seconds: numberValue("sm-noreply-min"),
    no_reply_max_seconds: numberValue("sm-noreply-max"),
    next_chat_min_seconds: numberValue("sm-next-min"),
    next_chat_max_seconds: numberValue("sm-next-max"),
  };
}

// ---------------------------------------------------------------------------
// Background Bot settings
// ---------------------------------------------------------------------------
function fillBotSettings(general) {
  const g = general || {};
  fillFirst("bg", g.first_message);
  byId("bg-poll-min").value = g.poll_min_seconds ?? 20;
  byId("bg-poll-max").value = g.poll_max_seconds ?? 45;
  byId("bg-action-min").value = g.action_min_seconds ?? 20;
  byId("bg-action-max").value = g.action_max_seconds ?? 45;
  byId("bg-skip-min").value = g.match_reply_timeout_min_seconds ?? 60;
  byId("bg-skip-max").value = g.match_reply_timeout_max_seconds ?? 60;
  byId("bg-close-min").value = g.match_close_wait_min_seconds ?? 0;
  byId("bg-close-max").value = g.match_close_wait_max_seconds ?? 0;
  byId("bg-reply-min").value = g.reply_delay_min_seconds ?? 0;
  byId("bg-reply-max").value = g.reply_delay_max_seconds ?? 0;
}

function readBotSettings() {
  const first = readFirst("bg");
  timingPair("bg-poll-min", "bg-poll-max", "Poll", 1);
  timingPair("bg-action-min", "bg-action-max", "Action", 1);
  timingPair("bg-skip-min", "bg-skip-max", "Skip timeout", 0);
  timingPair("bg-close-min", "bg-close-max", "Chat closed wait", 0);
  timingPair("bg-reply-min", "bg-reply-max", "Worker reply delay", 0);
  return {
    first_message: first,
    poll_min_seconds: numberValue("bg-poll-min"),
    poll_max_seconds: numberValue("bg-poll-max"),
    action_min_seconds: numberValue("bg-action-min"),
    action_max_seconds: numberValue("bg-action-max"),
    match_reply_timeout_min_seconds: numberValue("bg-skip-min"),
    match_reply_timeout_max_seconds: numberValue("bg-skip-max"),
    match_close_wait_min_seconds: numberValue("bg-close-min"),
    match_close_wait_max_seconds: numberValue("bg-close-max"),
    reply_delay_min_seconds: numberValue("bg-reply-min"),
    reply_delay_max_seconds: numberValue("bg-reply-max"),
  };
}

// ---------------------------------------------------------------------------
// Proxy (shared store, both pages)
// ---------------------------------------------------------------------------
function fillProxy(prefix, proxy) {
  byId(`${prefix}-proxy-enabled`).checked = !!(proxy || {}).enabled;
  byId(`${prefix}-proxy-entries`).value = (proxy || {}).entries || "";
}

function proxyInputLines(prefix) {
  const raw = byId(`${prefix}-proxy-entries`).value;
  if (byId(`${prefix}-proxy-scheme`).value !== "socks5") return raw;
  return raw.split(/\r?\n/).map((line) => {
    const text = line.trim();
    return text && !/^[a-z][a-z0-9+.-]*:\/\//i.test(text) ? "socks5://" + text : line;
  }).join("\n");
}

function readProxy(prefix) {
  return { enabled: byId(`${prefix}-proxy-enabled`).checked, entries: proxyInputLines(prefix) };
}

async function testProxy(prefix) {
  const button = byId(`${prefix}-test-proxy`);
  const statusId = `${prefix}-proxy-status`;
  const results = byId(`${prefix}-proxy-results`);
  if (button.disabled) return;
  const raw = proxyInputLines(prefix);
  results.replaceChildren();
  if (!raw.trim()) {
    setStatus(statusId, "Enter at least one proxy to test.", "error");
    return;
  }
  button.disabled = true;
  button.textContent = "Testing…";
  setStatus(statusId, "Testing HTTPS access through each line (four at a time)…");
  try {
    const rows = await invoke("TestProxyEntries", raw);
    let passed = 0;
    let reachable = 0;
    for (const row of rows || []) {
      if (row.ok) passed++;
      if (row.tunnel_ok) reachable++;
      const item = document.createElement("p");
      item.className = row.ok ? "proxy-test-pass" : row.tunnel_ok ? "proxy-test-warn" : "proxy-test-fail";
      item.textContent = `Line ${row.line}: ${row.ok ? "PASS" : row.tunnel_ok ? "REACHABLE / SITE RESTRICTED" : "FAIL"} — ${row.label || "Invalid entry"} — ${row.latency_ms} ms${row.status ? ` — HTTP ${row.status}` : ""} — ${row.detail}`;
      if (row.provider_message) item.textContent += `\nProvider said: “${row.provider_message}”`;
      if (row.proxy_authenticate) item.textContent += `\nAuth challenge: ${row.proxy_authenticate}`;
      if (row.hint) item.textContent += `\nNext step: ${row.hint}`;
      results.appendChild(item);
    }
    const total = (rows || []).length;
    setStatus(statusId, `${passed}/${total} passed; ${reachable} HTTPS tunnel(s) verified. A test is not a browser-login guarantee.`, passed === total ? "ok" : "");
    if (proxyInputLines(prefix) !== raw) {
      byId(statusId).textContent += " List edited: these results are for the previous values; test again.";
    }
  } catch (error) {
    setStatus(statusId, error.message || String(error), "error");
  } finally {
    button.disabled = false;
    button.textContent = "Test Proxy";
  }
}

// ---------------------------------------------------------------------------
// Message settings (shared store, both pages)
// ---------------------------------------------------------------------------
function setChatMode(prefix, mode) {
  const valid = ["single", "folder", "eva_chat_flow", "api_chat"].includes(mode) ? mode : "single";
  for (const [key, modeValue] of [["single", "single"], ["folder", "folder"], ["evachat", "eva_chat_flow"], ["api", "api_chat"]]) {
    const active = modeValue === valid;
    byId(`${prefix}-chat-mode-${key}`).checked = active;
    byId(`${prefix}-mode-option-${key}`).classList.toggle("active", active);
    byId(`${prefix}-section-${key}-mode`).style.display = active ? "block" : "none";
  }
}

function selectedChatMode(prefix) {
  return document.querySelector(`input[name="${prefix}-chat-mode"]:checked`)?.value || "single";
}

function fillMessage(prefix, message) {
  const m = message || {};
  setChatMode(prefix, m.chat_mode || "single");
  byId(`${prefix}-sequence`).value = m.sequence || "";
  byId(`${prefix}-usernames`).value = m.usernames || "";
  const apiDataStatus = byId(`${prefix}-api-data-status`);
  if (apiDataStatus) {
    const hasKey = !!(m.ai_api_key && String(m.ai_api_key).trim());
    const modelName = (m.ai_model && String(m.ai_model).trim()) || "deepseek-ai/DeepSeek-V3-0324";
    apiDataStatus.textContent = hasKey
      ? `Auto-loaded from apichat.exe · Model: ${modelName} · API Key: Ready`
      : `Auto-loaded from apichat.exe · Configure API Key & Persona in apichat.exe`;
  }
  const loaded = (m.loaded_sequences || []).length;
  state.loadedSequences[prefix] = loaded;
  byId(`${prefix}-loaded-status`).textContent = `${loaded} sequence${loaded === 1 ? "" : "s"} loaded`;
}

function syncMessageFields(sourcePrefix) {
  const targetPrefix = sourcePrefix === "sm" ? "bg" : "sm";
  const targetSlice = targetPrefix === "sm" ? "show" : "bot";
  for (const suffix of ["sequence", "usernames"]) {
    const src = byId(`${sourcePrefix}-${suffix}`);
    const dst = byId(`${targetPrefix}-${suffix}`);
    if (src && dst && dst !== document.activeElement) {
      dst.value = src.value;
      formStore[targetSlice][dst.id] = src.value;
    }
  }
  if (state.snapshot && state.snapshot.message) {
    state.snapshot.message.sequence = byId(`${sourcePrefix}-sequence`).value;
    state.snapshot.message.usernames = byId(`${sourcePrefix}-usernames`).value;
  }
}

function messagePayload(prefix) {
  const mode = selectedChatMode(prefix);
  const snapMsg = state.snapshot?.message || {};
  return {
    chat_mode: mode,
    use_loaded_sequences: mode === "folder",
    loaded_sequences: snapMsg.loaded_sequences || [],
    sequence: byId(`${prefix}-sequence`).value,
    usernames: byId(`${prefix}-usernames`).value,
    ai_base_url: snapMsg.ai_base_url || "",
    ai_model: snapMsg.ai_model || "",
    ai_api_key: snapMsg.ai_api_key || "",
    api_temperature: snapMsg.api_temperature || 0.9,
    api_share_hot: snapMsg.api_share_hot || 4,
    api_share_warm: snapMsg.api_share_warm || 6,
    api_share_cold: snapMsg.api_share_cold || 8,
  };
}

function modeDisplayLabel(mode) {
  return {
    single: "Single Sequence",
    folder: "Folder Sequences",
    eva_chat_flow: "EVAChat Flow",
    api_chat: "API Chat (via apichat.exe)",
  }[mode] || mode;
}

async function saveMessageSettings(prefix, silent = false) {
  try {
    syncMessageFields(prefix);
    const payload = messagePayload(prefix);
    const otherPrefix = prefix === "sm" ? "bg" : "sm";
    setChatMode(otherPrefix, payload.chat_mode);
    captureGlobalState();
    if (!silent) {
      assertFolderReady(prefix);
    }
    if (backend() && typeof backend().SaveMessageSettings === "function") {
      await invoke("SaveMessageSettings", payload);
    }
    if (state.snapshot) {
      state.snapshot.message = { ...(state.snapshot.message || {}), ...payload };
    }
    if (!silent) {
      setStatus(`${prefix}-message-save-status`, `Saved! Active reply mode: ${modeDisplayLabel(payload.chat_mode)}.`, "ok");
    }
  } catch (error) {
    if (!silent) {
      setStatus(`${prefix}-message-save-status`, error.message || String(error), "error");
    }
  }
}

function assertFolderReady(prefix) {
  if (selectedChatMode(prefix) === "folder" && state.loadedSequences[prefix] === 0) {
    throw new Error("Load a folder of sequences before using Folder Sequences mode.");
  }
}

async function loadFolder(prefix) {
  try {
    const sequences = await invoke("LoadMessageFolder");
    if (sequences && sequences.length > 0) {
      await refreshState();
      setChatMode(prefix, "folder");
      setStatus(`${prefix}-loaded-status`, `${sequences.length} sequences loaded.`, "ok");
    }
  } catch (error) {
    setStatus(`${prefix}-loaded-status`, error.message || String(error), "error");
  }
}

async function cleanLoaded(prefix) {
  try {
    await invoke("ClearLoadedMessages");
    await refreshState();
    setChatMode(prefix, "single");
    setStatus(`${prefix}-loaded-status`, "Loaded sequences cleared.", "ok");
  } catch (error) {
    setStatus(`${prefix}-loaded-status`, error.message || String(error), "error");
  }
}

async function openEvaFolder(prefix) {
  try {
    const summary = await invoke("OpenEVAChatDataFolder");
    if (summary) setStatus(`${prefix}-eva-status`, summary, "ok");
  } catch (error) {
    setStatus(`${prefix}-eva-status`, error.message || String(error), "error");
  }
}

async function openApiFolder(prefix) {
  try {
    const summary = await invoke("OpenAPIChatDataFolder");
    if (summary) setStatus(`${prefix}-api-data-status`, summary, "ok");
  } catch (error) {
    setStatus(`${prefix}-api-data-status`, error.message || String(error), "error");
  }
}

async function testApi(prefix) {
  try {
    syncApiFields(prefix);
    setStatus(`${prefix}-api-status`, "Testing API Chat model...");
    const payload = { ...messagePayload(prefix), chat_mode: "api_chat" };
    const answer = await invoke("TestAPIChat", payload);
    if (state.snapshot) {
      state.snapshot.message = { ...(state.snapshot.message || {}), ...payload };
    }
    setStatus(`${prefix}-api-status`, answer || "API OK", "ok");
  } catch (error) {
    setStatus(`${prefix}-api-status`, `API test failed: ${error.message || String(error)}`, "error");
  }
}

// ---------------------------------------------------------------------------
// Browser settings
// ---------------------------------------------------------------------------
function fillBrowser(prefix, browser) {
  const b = browser || {};
  if (prefix === "sm") {
    state.browserProfile = b.profile || "default";
    state.browserSession = !!b.use_saved_session;
  } else {
    byId(`${prefix}-profile`).value = b.profile || "default";
  }
  if (byId(`${prefix}-backend`)) byId(`${prefix}-backend`).value = b.backend === "chromedp" ? "chromedp" : "rod";
  if (byId(`${prefix}-headless`)) byId(`${prefix}-headless`).checked = !!b.headless;
}

// Show Mode profile: two radio choices, exactly as on screen.
//   default = fresh browser (manual login; session saved afterwards)
//   session = default account + its newest saved session (auto-login)
function selectedProfileChoice() {
  const checked = document.querySelector('input[name="sm-profile-choice"]:checked');
  return checked ? checked.value : "default";
}

function updateProfileStatus(prefix) {
  const status = byId(`${prefix}-profile-session`);
  if (!status) return;
  if (prefix === "sm") {
    const selection = selectedProfileChoice();
    const defaultProfile = state.profiles.find((p) => p.name.toLowerCase() === "default") || { name: "default" };
    if (selection === "session") {
      if (defaultProfile.has_session) {
        status.textContent = `Saved default session${defaultProfile.saved_at ? ` (${defaultProfile.saved_at})` : ""} — Run Browser logs in automatically.`;
        status.classList.add("status-ok");
      } else {
        status.textContent = "No saved default session yet — manual login will save one for the session option.";
        status.classList.remove("status-ok");
      }
    } else {
      status.textContent = "Fresh browser — manual login; the session is saved for the session option.";
      status.classList.remove("status-ok");
    }
    return;
  }
  const name = value(`${prefix}-profile`) || "default";
  const profile = state.profiles.find((p) => p.name.toLowerCase() === name.toLowerCase());
  if (profile && profile.has_session) {
    status.textContent = `Saved session${profile.saved_at ? ` (${profile.saved_at})` : ""} — Run Browser will log in automatically.`;
    status.classList.add("status-ok");
  } else {
    status.textContent = "No saved session — the browser will wait for manual login.";
    status.classList.remove("status-ok");
  }
}

function fillProfileOptions() {
  const defaultRadio = byId("sm-profile-default");
  const sessionRadio = byId("sm-profile-session-choice");
  if (defaultRadio && sessionRadio) {
    const wantSession = !!state.browserSession;
    defaultRadio.checked = !wantSession;
    sessionRadio.checked = wantSession;
    updateProfileStatus("sm");
  }
  const bgList = byId("bg-profile-options");
  if (bgList) {
    const fragment = document.createDocumentFragment();
    for (const profile of state.profiles) {
      const option = document.createElement("option");
      option.value = profile.name;
      option.label = profile.has_session ? `${profile.name} · session ${profile.saved_at || ""}`.trim() : `${profile.name} · no session`;
      fragment.appendChild(option);
    }
    bgList.replaceChildren(fragment);
    updateProfileStatus("bg");
  }
}

async function refreshProfiles() {
  try {
    const profiles = await invoke("ListProfiles");
    state.profiles = Array.isArray(profiles) ? profiles : [];
    fillProfileOptions();
  } catch (_) { /* old build or transient read error */ }
}

// ---------------------------------------------------------------------------
// State
// ---------------------------------------------------------------------------
function renderStats(home) {
  const h = home || {};
  byId("bg-stat-running").textContent = String(h.running_accounts || 0);
  byId("bg-stat-profiles").textContent = String(h.bot_accounts || 0);
  byId("bg-stat-messages").textContent = String(h.messages_sent || 0);
  byId("bg-stat-traffic").textContent = String(h.traffic_users || 0);
  byId("bg-stat-snap").textContent = String(h.snap_tg_sent_count || 0);
  byId("bg-stat-dead").textContent = String(h.dead_accounts || 0);
}

function siteAccountMode(site) {
  if (!site) return "browser_setup";
  if (site.account_mode === "auto_register" || site.account_mode === "browser_setup") {
    return site.account_mode;
  }
  return site.id === "chitchat" || site.auth_mode === "chitchat_jwt" ? "browser_setup" : "auto_register";
}

function setSiteFilter(filter) {
  state.siteFilter = filter || "all";
  const allBtn = byId("site-filter-all");
  const autoBtn = byId("site-filter-autoreg");
  const browserBtn = byId("site-filter-browser");
  if (allBtn) allBtn.classList.toggle("active", state.siteFilter === "all");
  if (autoBtn) autoBtn.classList.toggle("active", state.siteFilter === "auto_register");
  if (browserBtn) browserBtn.classList.toggle("active", state.siteFilter === "browser_setup");
  if (state.snapshot) {
    fillSites(state.snapshot);
  }
}

function updateSiteAccountCards(activeObj) {
  const siteId = (activeObj && activeObj.id) || "chitchat";
  const accMode = siteAccountMode(activeObj);
  const accCount = Number(activeObj?.account_count || 0);
  const isAuto = accMode === "auto_register";

  const badge = byId("active-site-mode-badge");
  if (badge) {
    const modeLabel = isAuto ? "⚡ Auto-Register (No Browser)" : "🌐 First Browser Account Setup";
    badge.textContent = `${modeLabel} · ${accCount} saved in sites/${siteId}/accounts/`;
  }

  const smAutoFolder = byId("sm-autoreg-folder-code");
  const smBrowserFolder = byId("sm-browser-folder-code");
  const bgAutoFolder = byId("bg-autoreg-folder-code");
  const bgBrowserFolder = byId("bg-browser-folder-code");
  if (smAutoFolder) smAutoFolder.textContent = `sites/${siteId}/accounts/`;
  if (smBrowserFolder) smBrowserFolder.textContent = `sites/${siteId}/profiles/`;
  if (bgAutoFolder) bgAutoFolder.textContent = `sites/${siteId}/accounts/`;
  if (bgBrowserFolder) bgBrowserFolder.textContent = `sites/${siteId}/profiles/`;

  const smAutoCard = byId("sm-autoreg-card");
  const smBrowserCard = byId("sm-browser-setup-card");
  const bgAutoCard = byId("bg-autoreg-card");
  const bgBrowserCard = byId("bg-browser-setup-card");
  if (smAutoCard) smAutoCard.classList.toggle("recommended-option", isAuto);
  if (smBrowserCard) smBrowserCard.classList.toggle("recommended-option", !isAuto);
  if (bgAutoCard) bgAutoCard.classList.toggle("recommended-option", isAuto);
  if (bgBrowserCard) bgBrowserCard.classList.toggle("recommended-option", !isAuto);

  const smAutoRec = byId("sm-autoreg-rec-badge");
  const smBrowserRec = byId("sm-browser-rec-badge");
  const bgAutoRec = byId("bg-autoreg-rec-badge");
  const bgBrowserRec = byId("bg-browser-rec-badge");
  if (smAutoRec) smAutoRec.textContent = isAuto ? "Recommended for this site" : "Optional";
  if (smBrowserRec) smBrowserRec.textContent = !isAuto ? "Recommended for this site" : "Optional";
  if (bgAutoRec) bgAutoRec.textContent = isAuto ? "Recommended for this site" : "Optional";
  if (bgBrowserRec) bgBrowserRec.textContent = !isAuto ? "Recommended for this site" : "Optional";
}

function fillSites(snapshot) {
  const select = byId("active-site-select");
  const status = byId("active-site-status");
  const searchEl = byId("site-search-input");
  if (!select) return;
  const sitesList = Array.isArray(snapshot?.sites) && snapshot.sites.length > 0
    ? snapshot.sites
    : [{ id: "chitchat", name: "ChitChat.gg", account_mode: "browser_setup", start_url: "https://app.chitchat.gg/text?auth=email" }];
  const activeId = snapshot?.active_site || "chitchat";
  const query = searchEl ? (searchEl.value || "").trim().toLowerCase() : "";
  const modeFilter = state.siteFilter || "all";

  let autoTotal = 0;
  let browserTotal = 0;
  let savedTotal = 0;
  for (const s of sitesList) {
    if (siteAccountMode(s) === "auto_register") autoTotal += 1;
    else browserTotal += 1;
    if (s.has_saved_session || Number(s.account_count || 0) > 0) savedTotal += 1;
  }
  const allBtn = byId("site-filter-all");
  const savedBtn = byId("site-filter-saved");
  const autoBtn = byId("site-filter-autoreg");
  const browserBtn = byId("site-filter-browser");
  if (allBtn) allBtn.textContent = `All (${sitesList.length})`;
  if (savedBtn) savedBtn.textContent = `✓ Saved (${savedTotal})`;
  if (autoBtn) autoBtn.textContent = `⚡ Auto-Reg (${autoTotal})`;
  if (browserBtn) browserBtn.textContent = `🌐 Browser (${browserTotal})`;

  select.replaceChildren();
  const activeObj = sitesList.find((s) => s.id === activeId) || sitesList[0];
  const filteredSites = [];
  for (const s of sitesList) {
    const sMode = siteAccountMode(s);
    const hasSaved = Boolean(s.has_saved_session || Number(s.account_count || 0) > 0);
    if (modeFilter === "saved_session" && s.id !== activeId && !hasSaved) continue;
    if ((modeFilter === "auto_register" || modeFilter === "browser_setup") && s.id !== activeId && sMode !== modeFilter) continue;
    const hay = `${s.id || ""} ${s.name || ""} ${s.start_url || ""} ${sMode}`.toLowerCase();
    if (query && s.id !== activeId && !hay.includes(query)) continue;
    filteredSites.push(s);
    const opt = document.createElement("option");
    opt.value = s.id;
    const prefix = sMode === "auto_register" ? "⚡ " : "🌐 ";
    const accTag = hasSaved ? ` [✓ ${s.account_count || 1} session]` : "";
    opt.textContent = `${prefix}${s.name || s.id}${accTag}`;
    if (s.id === activeId) {
      opt.selected = true;
    }
    select.appendChild(opt);
  }
  if (status && activeObj) {
    status.textContent = `sites/${activeObj.id} (${sitesList.length} sites) · ${activeObj.start_url || ""}`;
  }
  updateSiteAccountCards(activeObj);
  renderSiteSessionLists(filteredSites, activeId);
  renderSiteManager();
  renderSessionsTab();
}

function siteGroupCode(site) {
  if (!site) return "B";
  if (site.group) return String(site.group).toUpperCase();
  if (site.classification_group) return String(site.classification_group).toUpperCase();
  if (site.classification && site.classification.group) return String(site.classification.group).toUpperCase();
  const id = String(site.id || "").toLowerCase();
  if (["chatrandom_text", "emeraldchat", "omegleweb_com", "umingle"].includes(id)) return "E";
  if (id === "chitchat" || site.account_mode === "browser_setup") return "C";
  if (site.auth_mode === "guest_Socket" || site.auth_mode === "guest_api") return "A";
  return "B";
}

function groupDisplayTitle(grp) {
  return {
    A: "Group A · Browserless",
    B: "Group B · Auto-Signup",
    C: "Group C · Browser-Once",
    D: "Group D · DOM-Only",
    E: "Group E · Manual",
  }[grp] || `Group ${grp}`;
}

function resolveSiteRunnerState(siteId) {
  const st = state.siteStatuses[siteId] || {};
  const raw = String(st.status || st.state || "").toLowerCase();
  const errType = String(st.last_error_type || "").toLowerCase();
  if (raw === "banned" || errType === "banned") return "banned";
  if (raw === "running" || st.running) return "running";
  if (raw === "failed" || st.failed || errType === "crash") return "failed";
  if (raw === "paused" || st.paused || st.auto_paused) return "paused";
  return "stopped";
}

function resolveSiteSessionState(site) {
  if (!site) return "missing";
  const explicit = String(site.session_status || "").toLowerCase();
  if (explicit === "valid" || explicit === "expiring" || explicit === "missing") {
    return explicit;
  }
  const st = state.siteStatuses[site.id] || {};
  const errType = String(st.last_error_type || "").toLowerCase();
  const hasSaved = Boolean(site.has_saved_session || Number(site.account_count || 0) > 0);
  if (!hasSaved) return "missing";
  if (errType === "auth expired") return "expiring";
  const savedAt = String(site.session_saved_at || "").trim();
  if (savedAt) {
    if (savedAt.toLowerCase().includes("expir")) return "expiring";
    const parsed = Date.parse(savedAt);
    if (!Number.isNaN(parsed) && Date.now() - parsed > 20 * 3600 * 1000) {
      return "expiring";
    }
  }
  return "valid";
}

function sessionBadgeText(sessState) {
  if (sessState === "valid") return "✅ valid";
  if (sessState === "expiring") return "⚠️ expiring";
  return "❌ missing";
}

function setSiteManagerGroup(group) {
  state.smgrGroup = group || "ALL";
  const map = {
    ALL: "smgr-group-all",
    A: "smgr-group-a",
    B: "smgr-group-b",
    C: "smgr-group-c",
    D: "smgr-group-d",
    E: "smgr-group-e",
  };
  for (const [g, btnId] of Object.entries(map)) {
    const btn = byId(btnId);
    if (btn) btn.classList.toggle("active", state.smgrGroup === g);
  }
  const grpSelect = byId("smgr-filter-group");
  if (grpSelect && grpSelect.value !== state.smgrGroup) {
    grpSelect.value = state.smgrGroup;
  }
  renderSiteManager();
}

function toggleGroupCollapse(groupCode) {
  state.collapsedGroups[groupCode] = !state.collapsedGroups[groupCode];
  renderSiteManager();
}

function toggleSelectAllInGroup(groupCode, siteIds) {
  if (!Array.isArray(siteIds) || siteIds.length === 0) return;
  const allSelected = siteIds.every((id) => state.selectedSites.includes(id));
  if (allSelected) {
    state.selectedSites = state.selectedSites.filter((id) => !siteIds.includes(id));
  } else {
    for (const id of siteIds) {
      if (!state.selectedSites.includes(id)) {
        state.selectedSites.push(id);
      }
    }
  }
  renderSiteManager();
}

async function startGroupSites(groupCode, siteIds) {
  if (!Array.isArray(siteIds) || siteIds.length === 0) return;
  try {
    setStatus("smgr-status", `Starting Group ${groupCode} (${siteIds.length} sites)…`);
    await invoke("StartSites", siteIds);
    await refreshSiteStatuses();
    setStatus("smgr-status", `Started Group ${groupCode} (${siteIds.length} sites).`, "ok");
  } catch (error) {
    setStatus("smgr-status", error.message || String(error), "error");
  }
}

async function stopGroupSites(groupCode, siteIds) {
  if (!Array.isArray(siteIds) || siteIds.length === 0) return;
  try {
    setStatus("smgr-status", `Stopping Group ${groupCode} (${siteIds.length} sites)…`);
    await invoke("StopSites", siteIds);
    await refreshSiteStatuses();
    setStatus("smgr-status", `Stopped Group ${groupCode} (${siteIds.length} sites).`, "ok");
  } catch (error) {
    setStatus("smgr-status", error.message || String(error), "error");
  }
}

async function openSiteDetailPanel(siteId) {
  state.detailOpen = true;
  const detailCard = byId("smgr-site-detail");
  if (detailCard) {
    detailCard.classList.remove("closed");
    detailCard.classList.add("open");
  }
  await selectTargetSite(siteId);
  await loadSiteDetailView(siteId);
}

function closeSiteDetailPanel() {
  state.detailOpen = false;
  const detailCard = byId("smgr-site-detail");
  if (detailCard) {
    detailCard.classList.remove("open");
    detailCard.classList.add("closed");
  }
}

function syncOverrideToggleRows() {
  const proxyChk = byId("smgr-ov-proxy-enabled");
  const modeChk = byId("smgr-ov-mode-enabled");
  const delayChk = byId("smgr-ov-delay-enabled");
  const proxyRow = byId("smgr-ov-proxy-row");
  const modeRow = byId("smgr-ov-mode-row");
  const delayRow = byId("smgr-ov-delay-row");
  if (proxyRow && proxyChk) proxyRow.hidden = !proxyChk.checked;
  if (modeRow && modeChk) modeRow.hidden = !modeChk.checked;
  if (delayRow && delayChk) delayRow.hidden = !delayChk.checked;
}

function applySiteDetailView(detail) {
  if (!detail) return;
  const siteId = detail.site_id || detail.id || "chitchat";
  const grp = String(detail.group || siteGroupCode(detail)).toUpperCase();
  const accMode = siteAccountMode(detail);
  const sessState = detail.session_status || resolveSiteSessionState(detail);
  const runnerState = detail.runner_status || resolveSiteRunnerState(siteId);
  const connType = detail.connection_type || (grp === "A" ? "websocket" : grp === "B" || grp === "C" ? "api" : "dom");

  const titleEl = byId("smgr-detail-title");
  const badgeEl = byId("smgr-detail-status-badge");
  const urlEl = byId("smgr-detail-url");
  const folderEl = byId("smgr-detail-folder");
  const groupEl = byId("smgr-detail-group");
  const connEl = byId("smgr-detail-conn-type");
  const sessionEl = byId("smgr-detail-session");
  const capturedEl = byId("smgr-detail-session-captured");
  const expiryEl = byId("smgr-detail-session-expiry");
  const overrideEl = byId("smgr-detail-override");
  const sentEl = byId("smgr-detail-stat-sent");
  const recvEl = byId("smgr-detail-stat-recv");
  const uptimeEl = byId("smgr-detail-stat-uptime");
  const errorsEl = byId("smgr-detail-stat-errors");
  const lastErrEl = byId("smgr-detail-last-error");

  if (titleEl) titleEl.textContent = `${detail.name || siteId} (${siteId})`;
  if (badgeEl) {
    badgeEl.textContent = runnerState;
    badgeEl.className = `smgr-status-badge status-${runnerState}`;
  }
  if (urlEl) urlEl.textContent = detail.start_url || "";
  if (folderEl) folderEl.textContent = `sites/${siteId}/`;
  if (groupEl) groupEl.textContent = `Group ${grp} · ${accMode}`;
  if (connEl) connEl.textContent = connType;
  if (sessionEl) {
    sessionEl.textContent = `${sessionBadgeText(sessState)} · ${detail.account_count ?? (sessState === "missing" ? 0 : 1)} saved in sites/${siteId}/sessions/`;
  }
  if (capturedEl) {
    capturedEl.textContent = detail.session_captured_at || detail.session_saved_at || "Not captured yet";
  }
  if (expiryEl) {
    expiryEl.textContent = detail.session_expires_at || (sessState === "valid" ? "24h rolling window" : "N/A");
  }

  const st = state.siteStatuses[siteId] || {};
  const stats = st.stats || {};
  if (sentEl) sentEl.textContent = String(detail.messages_sent ?? stats.messages_sent ?? st.messages_sent ?? 0);
  if (recvEl) recvEl.textContent = String(detail.messages_received ?? stats.messages_received ?? 0);
  if (uptimeEl) uptimeEl.textContent = detail.uptime_text || (runnerState === "running" ? "Active" : "0s");
  if (errorsEl) errorsEl.textContent = String(detail.error_count ?? st.total_errors ?? st.consecutive_errors ?? 0);

  const lastErr = detail.last_error || st.last_error || "";
  if (lastErrEl) {
    lastErrEl.textContent = lastErr ? `${detail.last_error_type || st.last_error_type ? `[${detail.last_error_type || st.last_error_type}] ` : ""}${lastErr}` : "No errors recorded.";
  }

  const ov = detail.override || {};
  const hasOv = Boolean(detail.has_override || ov.proxy || ov.message_mode || ov.reply_delay);
  if (overrideEl) {
    overrideEl.textContent = hasOv
      ? `sites/${siteId}/settings_override.json`
      : "sites/_global/settings.json";
  }

  if (detail.override !== undefined) {
    const proxyChk = byId("smgr-ov-proxy-enabled");
    const proxyInp = byId("smgr-ov-proxy-input");
    const modeChk = byId("smgr-ov-mode-enabled");
    const modeSel = byId("smgr-ov-mode-select");
    const delayChk = byId("smgr-ov-delay-enabled");
    const delayMin = byId("smgr-ov-delay-min");
    const delayMax = byId("smgr-ov-delay-max");

    if (proxyChk) proxyChk.checked = Boolean(ov.proxy && (ov.proxy.enabled || ov.proxy.entries));
    if (proxyInp) proxyInp.value = ov.proxy?.entries || ov.proxy?.address || "";
    if (modeChk) modeChk.checked = Boolean(ov.message_mode);
    if (modeSel && ov.message_mode) modeSel.value = ov.message_mode;
    if (delayChk) delayChk.checked = Boolean(ov.reply_delay);
    if (delayMin && ov.reply_delay) delayMin.value = String(ov.reply_delay.min_seconds ?? 0);
    if (delayMax && ov.reply_delay) delayMax.value = String(ov.reply_delay.max_seconds ?? 0);
    syncOverrideToggleRows();
    setStatus(
      "smgr-ov-status",
      hasOv ? `Active per-site override in sites/${siteId}/settings_override.json` : "Following global settings.",
      hasOv ? "ok" : "",
    );
  }
}

async function loadSiteDetailView(siteId) {
  const target = siteId || value("active-site-select") || "chitchat";
  if (!backend() || typeof backend().GetSiteDetail !== "function") return;
  try {
    const detail = await invoke("GetSiteDetail", target);
    if (detail) applySiteDetailView(detail);
  } catch (_) { /* optional in lightweight mocks */ }
}

async function saveActiveSiteOverride() {
  const siteId = value("active-site-select") || "chitchat";
  const override = {};
  if (byId("smgr-ov-proxy-enabled")?.checked) {
    const rawProxy = value("smgr-ov-proxy-input");
    override.proxy = { enabled: Boolean(rawProxy), entries: rawProxy };
  }
  if (byId("smgr-ov-mode-enabled")?.checked) {
    override.message_mode = value("smgr-ov-mode-select") || "single";
  }
  if (byId("smgr-ov-delay-enabled")?.checked) {
    const minSec = Number(byId("smgr-ov-delay-min")?.value ?? 0);
    const maxSec = Number(byId("smgr-ov-delay-max")?.value ?? 0);
    if (!Number.isInteger(minSec) || !Number.isInteger(maxSec) || minSec < 0 || maxSec < minSec) {
      setStatus("smgr-ov-status", "Reply delay min/max must be valid whole seconds (min <= max).", "error");
      return;
    }
    override.reply_delay = { min_seconds: minSec, max_seconds: maxSec };
  }
  try {
    setStatus("smgr-ov-status", `Saving override to sites/${siteId}/settings_override.json…`);
    const updated = await invoke("SaveSiteOverride", siteId, override);
    if (updated) applySiteDetailView(updated);
    setStatus("smgr-ov-status", `Saved override for sites/${siteId}/ and applied immediately.`, "ok");
  } catch (error) {
    setStatus("smgr-ov-status", error.message || String(error), "error");
  }
}

async function clearActiveSiteOverride() {
  const siteId = value("active-site-select") || "chitchat";
  try {
    await invoke("ClearSiteSettingsOverride", siteId);
    if (byId("smgr-ov-proxy-enabled")) byId("smgr-ov-proxy-enabled").checked = false;
    if (byId("smgr-ov-mode-enabled")) byId("smgr-ov-mode-enabled").checked = false;
    if (byId("smgr-ov-delay-enabled")) byId("smgr-ov-delay-enabled").checked = false;
    syncOverrideToggleRows();
    await loadSiteDetailView(siteId);
    setStatus("smgr-ov-status", `Cleared override for sites/${siteId}/ — now using global settings.`, "ok");
  } catch (error) {
    setStatus("smgr-ov-status", error.message || String(error), "error");
  }
}

async function refreshActiveSiteSession() {
  const siteId = value("active-site-select") || "chitchat";
  try {
    setStatus("smgr-status", `Refreshing session status for sites/${siteId}/…`);
    const detail = await invoke("RefreshSiteSession", siteId);
    if (detail) applySiteDetailView(detail);
    await refreshState();
    setStatus("smgr-status", `Session refreshed for sites/${siteId}/`, "ok");
  } catch (error) {
    setStatus("smgr-status", error.message || String(error), "error");
  }
}

function renderSiteDetail(activeObj) {
  if (!activeObj) return;
  applySiteDetailView(activeObj);
}

function renderSiteManager() {
  const grid = byId("smgr-site-grid");
  const countEl = byId("smgr-grid-count");
  const totalBadge = byId("smgr-total-badge");
  const selCountEl = byId("smgr-selected-count");
  if (!grid) return;

  const sitesList = Array.isArray(state.snapshot?.sites) && state.snapshot.sites.length > 0
    ? state.snapshot.sites
    : [{ id: "chitchat", name: "ChitChat.gg", account_mode: "browser_setup", start_url: "https://app.chitchat.gg/text?auth=email" }];
  const activeId = state.snapshot?.active_site || "chitchat";
  const searchEl = byId("smgr-search-input");
  const query = searchEl ? (searchEl.value || "").trim().toLowerCase() : "";
  const groupFilter = state.smgrGroup || "ALL";
  const statusFilter = byId("smgr-filter-status")?.value || state.smgrStatusFilter || "all";
  const sessionFilter = byId("smgr-filter-session")?.value || state.smgrSessionFilter || "all";

  if (totalBadge) totalBadge.textContent = `${sitesList.length} Isolated Sites`;
  if (selCountEl) selCountEl.textContent = `${state.selectedSites.length} site(s) selected`;

  const grouped = { A: [], B: [], C: [], D: [], E: [] };
  let shownCount = 0;

  for (const s of sitesList) {
    const grp = siteGroupCode(s);
    const rState = resolveSiteRunnerState(s.id);
    const sessState = resolveSiteSessionState(s);

    if (groupFilter !== "ALL" && grp !== groupFilter) continue;
    if (statusFilter !== "all" && rState !== statusFilter) continue;
    if (sessionFilter !== "all" && sessState !== sessionFilter) continue;
    const hay = `${s.id || ""} ${s.name || ""} ${s.start_url || ""} group ${grp} ${rState} ${sessState}`.toLowerCase();
    if (query && !hay.includes(query)) continue;

    if (!grouped[grp]) grouped[grp] = [];
    grouped[grp].push({ site: s, grp, rState, sessState });
    shownCount += 1;
  }

  if (countEl) countEl.textContent = `${shownCount} of ${sitesList.length} sites shown`;

  const fragment = document.createDocumentFragment();
  for (const grpCode of ["A", "B", "C", "D", "E"]) {
    const items = grouped[grpCode] || [];
    if (items.length === 0) continue;
    const grpSiteIds = items.map((entry) => entry.site.id);
    const isCollapsed = !!state.collapsedGroups[grpCode];
    const allInGroupSelected = grpSiteIds.every((id) => state.selectedSites.includes(id));

    const section = document.createElement("section");
    section.className = "smgr-group-section" + (isCollapsed ? " collapsed" : "");
    section.dataset.groupSection = grpCode;

    const header = document.createElement("div");
    header.className = "smgr-group-header";

    const titleBtn = document.createElement("button");
    titleBtn.type = "button";
    titleBtn.className = "smgr-group-title-btn";
    titleBtn.dataset.groupToggle = grpCode;
    const arrowSpan = document.createElement("span");
    arrowSpan.textContent = isCollapsed ? "▶" : "▼";
    const titleSpan = document.createElement("span");
    titleSpan.textContent = groupDisplayTitle(grpCode);
    const countBadge = document.createElement("span");
    countBadge.className = "smgr-group-count";
    countBadge.dataset.groupCount = grpCode;
    countBadge.textContent = `${items.length} site${items.length === 1 ? "" : "s"}`;
    titleBtn.append(arrowSpan, titleSpan, countBadge);
    titleBtn.addEventListener("click", () => toggleGroupCollapse(grpCode));

    const grpActions = document.createElement("div");
    grpActions.className = "smgr-group-actions";

    const selectAllBtn = document.createElement("button");
    selectAllBtn.type = "button";
    selectAllBtn.className = "site-mini-btn";
    selectAllBtn.dataset.groupSelectAll = grpCode;
    selectAllBtn.textContent = allInGroupSelected ? "Unselect All" : "Select All";
    selectAllBtn.addEventListener("click", () => toggleSelectAllInGroup(grpCode, grpSiteIds));

    const startGrpBtn = document.createElement("button");
    startGrpBtn.type = "button";
    startGrpBtn.className = "site-mini-btn";
    startGrpBtn.dataset.groupStart = grpCode;
    startGrpBtn.textContent = "▶ Start Group";
    startGrpBtn.addEventListener("click", () => startGroupSites(grpCode, grpSiteIds));

    const stopGrpBtn = document.createElement("button");
    stopGrpBtn.type = "button";
    stopGrpBtn.className = "site-mini-btn";
    stopGrpBtn.dataset.groupStop = grpCode;
    stopGrpBtn.textContent = "■ Stop Group";
    stopGrpBtn.addEventListener("click", () => stopGroupSites(grpCode, grpSiteIds));

    grpActions.append(selectAllBtn, startGrpBtn, stopGrpBtn);
    header.append(titleBtn, grpActions);

    const body = document.createElement("div");
    body.className = "smgr-group-body";

    for (const { site: s, rState, sessState } of items) {
      const isChecked = state.selectedSites.includes(s.id);
      const row = document.createElement("div");
      row.className = "smgr-grid-item smgr-site-row" + (s.id === activeId ? " active" : "");
      row.dataset.siteCard = s.id;

      const checkbox = document.createElement("input");
      checkbox.type = "checkbox";
      checkbox.checked = isChecked;
      checkbox.dataset.siteSelectCheck = s.id;
      checkbox.setAttribute("aria-label", `Select ${s.name || s.id}`);
      checkbox.addEventListener("click", (e) => e.stopPropagation());
      checkbox.addEventListener("change", (e) => {
        if (e.target.checked) {
          if (!state.selectedSites.includes(s.id)) state.selectedSites.push(s.id);
        } else {
          state.selectedSites = state.selectedSites.filter((id) => id !== s.id);
        }
        if (selCountEl) selCountEl.textContent = `${state.selectedSites.length} site(s) selected`;
      });

      const nameBtn = document.createElement("button");
      nameBtn.type = "button";
      nameBtn.className = "smgr-site-name-btn";
      nameBtn.dataset.siteName = s.id;
      const nameSpan = document.createElement("span");
      nameSpan.className = "smgr-grid-name";
      nameSpan.textContent = s.name || s.id;
      const metaSpan = document.createElement("span");
      metaSpan.className = "smgr-grid-meta";
      metaSpan.textContent = `sites/${s.id}/ · ${s.start_url || ""}`;
      nameBtn.append(nameSpan, metaSpan);
      nameBtn.addEventListener("click", (e) => {
        e.stopPropagation();
        openSiteDetailPanel(s.id);
      });

      const statusBadge = document.createElement("span");
      statusBadge.className = `smgr-status-badge status-${rState}`;
      statusBadge.dataset.statusBadge = s.id;
      statusBadge.textContent = rState;

      const sessionBadge = document.createElement("span");
      sessionBadge.className = `smgr-session-badge session-${sessState}`;
      sessionBadge.dataset.sessionBadge = s.id;
      sessionBadge.textContent = sessionBadgeText(sessState);

      const actions = document.createElement("div");
      actions.className = "smgr-grid-actions";

      const startBtn = document.createElement("button");
      startBtn.type = "button";
      startBtn.className = "site-mini-btn";
      startBtn.dataset.smgrStart = s.id;
      startBtn.textContent = "Start";
      startBtn.addEventListener("click", (e) => {
        e.stopPropagation();
        startSingleSiteRunner(s.id);
      });

      const stopBtn = document.createElement("button");
      stopBtn.type = "button";
      stopBtn.className = "site-mini-btn";
      stopBtn.dataset.smgrStop = s.id;
      stopBtn.textContent = "Stop";
      stopBtn.addEventListener("click", (e) => {
        e.stopPropagation();
        stopSingleSiteRunner(s.id);
      });

      const browserBtn = document.createElement("button");
      browserBtn.type = "button";
      browserBtn.className = "site-mini-btn";
      browserBtn.dataset.smgrBrowser = s.id;
      browserBtn.textContent = "Open Browser";
      browserBtn.addEventListener("click", (e) => {
        e.stopPropagation();
        runSiteBrowserFor(s.id);
      });

      const detailBtn = document.createElement("button");
      detailBtn.type = "button";
      detailBtn.className = "site-mini-btn";
      detailBtn.dataset.smgrDetail = s.id;
      detailBtn.textContent = "Detail";
      detailBtn.addEventListener("click", (e) => {
        e.stopPropagation();
        openSiteDetailPanel(s.id);
      });

      actions.append(startBtn, stopBtn, browserBtn, detailBtn);
      row.append(checkbox, nameBtn, statusBadge, sessionBadge, actions);
      row.addEventListener("click", () => openSiteDetailPanel(s.id));
      body.appendChild(row);
    }

    section.append(header, body);
    fragment.appendChild(section);
  }

  grid.replaceChildren(fragment);

  const activeObj = sitesList.find((s) => s.id === activeId) || sitesList[0];
  renderSiteDetail(activeObj);
}

function renderSessionsTab() {
  const listEl = byId("sess-health-list");
  if (!listEl) return;
  const sitesList = Array.isArray(state.snapshot?.sites) && state.snapshot.sites.length > 0
    ? state.snapshot.sites
    : [{ id: "chitchat", name: "ChitChat.gg", account_mode: "browser_setup", start_url: "https://app.chitchat.gg/text?auth=email" }];
  const activeId = state.snapshot?.active_site || "chitchat";

  const activeLabel = byId("sess-active-site-label");
  if (activeLabel) activeLabel.textContent = activeId;

  const query = (byId("sess-search-input")?.value || "").trim().toLowerCase();
  const statusFilter = byId("sess-filter-status")?.value || state.sessFilterStatus || "all";

  let validCount = 0;
  let expiringCount = 0;
  let missingCount = 0;
  let runningCount = 0;
  let issueCount = 0;
  for (const s of sitesList) {
    const sessSummary = state.sessionStatuses[s.id] || {};
    const sessState = sessSummary.status || resolveSiteSessionState(s);
    const rState = resolveSiteRunnerState(s.id);
    if (sessState === "valid") validCount += 1;
    else if (sessState === "expiring") expiringCount += 1;
    else missingCount += 1;
    if (rState === "running") runningCount += 1;
    if (rState === "paused" || rState === "failed" || rState === "banned" || sessState === "expiring") issueCount += 1;
  }

  const totalEl = byId("sess-stat-total");
  const validEl = byId("sess-stat-valid");
  const expiringEl = byId("sess-stat-expiring");
  const missingEl = byId("sess-stat-missing");
  const savedEl = byId("sess-stat-saved");
  const runningEl = byId("sess-stat-running");
  const issuesEl = byId("sess-stat-issues");
  if (totalEl) totalEl.textContent = String(sitesList.length);
  if (validEl) validEl.textContent = String(validCount);
  if (expiringEl) expiringEl.textContent = String(expiringCount);
  if (missingEl) missingEl.textContent = String(missingCount);
  if (savedEl) savedEl.textContent = String(validCount);
  if (runningEl) runningEl.textContent = String(runningCount);
  if (issuesEl) issuesEl.textContent = String(issueCount);

  const fragment = document.createDocumentFragment();
  for (const s of sitesList) {
    const sessSummary = state.sessionStatuses[s.id] || {};
    const sessState = sessSummary.status || resolveSiteSessionState(s);
    if (statusFilter !== "all" && sessState !== statusFilter) continue;
    if (query) {
      const hay = `${s.id} ${s.name || ""} ${s.start_url || ""}`.toLowerCase();
      if (!hay.includes(query)) continue;
    }

    const grp = siteGroupCode(s);
    const capturedAt = sessSummary.captured_at || s.session_saved_at || "Not captured yet";
    const expiresAt = sessSummary.expires_at || (sessState === "valid" ? "24h window" : "N/A");
    const sessionDir = sessSummary.session_dir || `sites/${s.id}/sessions/default`;

    const row = document.createElement("div");
    row.className = "site-session-row";
    row.dataset.sessionHealthRow = s.id;

    const info = document.createElement("div");
    info.className = "site-session-info";
    const nameEl = document.createElement("div");
    nameEl.className = "site-session-name";
    nameEl.textContent = `${s.name || s.id} (${s.id}) · Group ${grp}`;
    const metaEl = document.createElement("div");
    metaEl.className = "site-session-meta";
    metaEl.textContent = `Captured: ${capturedAt} · Expires: ${expiresAt} · Store: ${sessionDir}/`;
    info.append(nameEl, metaEl);

    const badge = document.createElement("span");
    badge.className = `smgr-session-badge session-${sessState}`;
    badge.dataset.sessStatusBadge = s.id;
    badge.textContent = sessionBadgeText(sessState);

    const actions = document.createElement("div");
    actions.className = "site-session-actions";

    const captureBtn = document.createElement("button");
    captureBtn.type = "button";
    captureBtn.className = "primary-button site-mini-btn";
    captureBtn.dataset.sessCapture = s.id;
    captureBtn.textContent = "Capture";
    captureBtn.addEventListener("click", () => captureSiteSessionFor(s.id, false));

    const recaptureBtn = document.createElement("button");
    recaptureBtn.type = "button";
    recaptureBtn.className = "secondary-button site-mini-btn";
    recaptureBtn.dataset.sessRecapture = s.id;
    recaptureBtn.textContent = "Re-capture";
    recaptureBtn.addEventListener("click", () => captureSiteSessionFor(s.id, true));

    const clearBtn = document.createElement("button");
    clearBtn.type = "button";
    clearBtn.className = "site-mini-btn";
    clearBtn.dataset.sessClear = s.id;
    clearBtn.textContent = "Clear";
    clearBtn.addEventListener("click", () => clearSiteSessionFor(s.id));

    actions.append(badge, captureBtn, recaptureBtn, clearBtn);
    row.append(info, actions);
    fragment.appendChild(row);
  }
  listEl.replaceChildren(fragment);
}

async function refreshSessionStatuses() {
  if (!backend() || typeof backend().GetSessionStatuses !== "function") return;
  try {
    const overview = await invoke("GetSessionStatuses");
    if (overview && Array.isArray(overview.sites)) {
      const map = {};
      for (const item of overview.sites) {
        if (item && item.site_id) {
          map[item.site_id] = item;
        }
      }
      state.sessionStatuses = map;
      if (state.snapshot && Array.isArray(state.snapshot.sites)) {
        for (const s of state.snapshot.sites) {
          const item = map[s.id];
          if (item) {
            s.session_status = item.status;
            s.has_saved_session = item.status === "valid" || item.status === "expiring";
            if (item.captured_at) s.session_saved_at = item.captured_at;
          }
        }
      }
      renderSessionsTab();
      if (state.dashTab === "site-manager") renderSiteManager();
    }
  } catch (_) { /* optional in lightweight mocks */ }
}

async function captureSiteSessionFor(siteId, isRecapture) {
  const target = siteId || value("active-site-select") || "chitchat";
  const label = isRecapture ? "Re-capturing" : "Capturing";
  try {
    setStatus("sess-status", `${label} isolated session in sites/${target}/sessions/default/…`);
    setStatus("sess-capture-status", `${label} cookies, localStorage, WS URLs & auth tokens for sites/${target}/…`);
    const summary = await invoke("CaptureSession", target);
    if (summary && summary.site_id) {
      state.sessionStatuses[summary.site_id] = summary;
    }
    if (state.snapshot && Array.isArray(state.snapshot.sites)) {
      const siteObj = state.snapshot.sites.find((s) => s.id === target);
      if (siteObj) {
        siteObj.session_status = summary?.status || "valid";
        siteObj.has_saved_session = true;
        siteObj.session_saved_at = summary?.captured_at || new Date().toISOString();
      }
    }
    renderSessionsTab();
    renderSiteManager();
    setStatus("sess-status", `✅ Session captured for sites/${target}/sessions/default/`, "ok");
    setStatus("sess-capture-status", `Saved cookies.json, local_storage.json, ws_endpoints.json, auth_tokens.json, meta.json in sites/${target}/sessions/default/`, "ok");
  } catch (error) {
    const msg = error.message || String(error);
    setStatus("sess-status", msg, "error");
    setStatus("sess-capture-status", msg, "error");
  }
}

async function validateSiteSessionFor(siteId) {
  const target = siteId || value("active-site-select") || "chitchat";
  try {
    const summary = await invoke("ValidateSession", target);
    if (summary && summary.site_id) {
      state.sessionStatuses[summary.site_id] = summary;
    }
    renderSessionsTab();
    setStatus("sess-capture-status", `Validated sites/${target}/sessions/default/: status=${summary?.status || "unknown"}`, "ok");
  } catch (error) {
    setStatus("sess-capture-status", error.message || String(error), "error");
  }
}

async function clearSiteSessionFor(siteId) {
  const target = siteId || value("active-site-select") || "chitchat";
  try {
    setStatus("sess-status", `Clearing session in sites/${target}/sessions/default/…`);
    const summary = await invoke("ClearSession", target);
    if (summary && summary.site_id) {
      state.sessionStatuses[summary.site_id] = summary;
    } else {
      state.sessionStatuses[target] = { site_id: target, status: "missing" };
    }
    if (state.snapshot && Array.isArray(state.snapshot.sites)) {
      const siteObj = state.snapshot.sites.find((s) => s.id === target);
      if (siteObj) {
        siteObj.session_status = "missing";
        siteObj.has_saved_session = false;
        siteObj.account_count = 0;
        siteObj.session_saved_at = "";
      }
    }
    renderSessionsTab();
    renderSiteManager();
    setStatus("sess-status", `Cleared session in sites/${target}/sessions/default/`, "ok");
    setStatus("sess-capture-status", `Cleared session in sites/${target}/sessions/default/`, "ok");
  } catch (error) {
    setStatus("sess-status", error.message || String(error), "error");
  }
}

async function captureAllMissingSessionsBatch() {
  const BATCH_SIZE = 5;
  try {
    setStatus("sess-status", `Capturing missing sessions (batch of ${BATCH_SIZE})…`);
    if (backend() && typeof backend().CaptureMissingSessionsBatch === "function") {
      const captured = await invoke("CaptureMissingSessionsBatch", BATCH_SIZE);
      if (Array.isArray(captured)) {
        for (const item of captured) {
          if (item && item.site_id) {
            state.sessionStatuses[item.site_id] = item;
            const siteObj = state.snapshot?.sites?.find((s) => s.id === item.site_id);
            if (siteObj) {
              siteObj.session_status = item.status || "valid";
              siteObj.has_saved_session = true;
              siteObj.session_saved_at = item.captured_at || new Date().toISOString();
            }
          }
        }
      }
      renderSessionsTab();
      renderSiteManager();
      const count = Array.isArray(captured) ? captured.length : 0;
      setStatus("sess-status", `Captured ${count} missing session(s) in batch of ${BATCH_SIZE}.`, "ok");
      return;
    }

    const sitesList = Array.isArray(state.snapshot?.sites) ? state.snapshot.sites : [];
    const missingIds = sitesList
      .filter((s) => ((state.sessionStatuses[s.id]?.status || resolveSiteSessionState(s)) === "missing"))
      .slice(0, BATCH_SIZE)
      .map((s) => s.id);
    for (const id of missingIds) {
      await captureSiteSessionFor(id, false);
    }
    setStatus("sess-status", `Captured ${missingIds.length} missing session(s) in batch of ${BATCH_SIZE}.`, "ok");
  } catch (error) {
    setStatus("sess-status", error.message || String(error), "error");
  }
}

async function refreshSiteStatuses() {
  if (!backend() || typeof backend().GetSiteStatuses !== "function") return;
  try {
    const statuses = await invoke("GetSiteStatuses");
    state.siteStatuses = statuses && typeof statuses === "object" ? statuses : {};
    let runningNow = 0;
    for (const siteId of Object.keys(state.siteStatuses)) {
      if (resolveSiteRunnerState(siteId) === "running") runningNow += 1;
    }
    const multiEl = byId("cc-multi-status");
    if (multiEl) {
      multiEl.textContent = `Supervisor active: ${runningNow} site(s) running · Global store: sites/_global/settings.json`;
    }
    if (state.dashTab === "site-manager") renderSiteManager();
    if (state.dashTab === "sessions") renderSessionsTab();
  } catch (_) { /* optional in mock tests */ }
}

async function startAllSitesRunner() {
  try {
    setStatus("cc-multi-status", "Starting all isolated site runners via Supervisor…");
    await invoke("StartAll");
    await refreshSiteStatuses();
    setStatus("cc-multi-status", "All sites started in isolated goroutines.", "ok");
  } catch (error) {
    setStatus("cc-multi-status", error.message || String(error), "error");
  }
}

async function stopAllSitesRunner() {
  try {
    setStatus("cc-multi-status", "Stopping all site runners…");
    setStatus("smgr-status", "Stopping all site runners…");
    await invoke("StopAll");
    await refreshSiteStatuses();
    setStatus("cc-multi-status", "All site runners stopped.", "ok");
    setStatus("smgr-status", "All site runners stopped.", "ok");
  } catch (error) {
    const msg = error.message || String(error);
    setStatus("cc-multi-status", msg, "error");
    setStatus("smgr-status", msg, "error");
  }
}

async function startSelectedSitesRunner() {
  const targets = state.selectedSites.length > 0
    ? state.selectedSites.slice()
    : [value("active-site-select") || "chitchat"];
  try {
    setStatus("smgr-status", `Starting ${targets.length} selected site(s)…`);
    await invoke("StartSites", targets);
    await refreshSiteStatuses();
    setStatus("smgr-status", `Started ${targets.length} site(s): ${targets.join(", ")}`, "ok");
  } catch (error) {
    setStatus("smgr-status", error.message || String(error), "error");
  }
}

async function stopSelectedSitesRunner() {
  const targets = state.selectedSites.length > 0
    ? state.selectedSites.slice()
    : [value("active-site-select") || "chitchat"];
  try {
    setStatus("smgr-status", `Stopping ${targets.length} selected site(s)…`);
    await invoke("StopSites", targets);
    await refreshSiteStatuses();
    setStatus("smgr-status", `Stopped ${targets.length} site(s): ${targets.join(", ")}`, "ok");
  } catch (error) {
    setStatus("smgr-status", error.message || String(error), "error");
  }
}

async function startSingleSiteRunner(siteId) {
  const target = siteId || value("active-site-select") || "chitchat";
  try {
    setStatus("smgr-status", `Starting runner for sites/${target}/…`);
    await invoke("StartSite", target);
    await refreshSiteStatuses();
    setStatus("smgr-status", `Runner started for sites/${target}/`, "ok");
  } catch (error) {
    setStatus("smgr-status", error.message || String(error), "error");
  }
}

async function stopSingleSiteRunner(siteId) {
  const target = siteId || value("active-site-select") || "chitchat";
  try {
    setStatus("smgr-status", `Stopping runner for sites/${target}/…`);
    await invoke("StopSite", target);
    await refreshSiteStatuses();
    setStatus("smgr-status", `Runner stopped for sites/${target}/`, "ok");
  } catch (error) {
    setStatus("smgr-status", error.message || String(error), "error");
  }
}

async function runRecipeForActiveSite() {
  const target = value("active-site-select") || "chitchat";
  try {
    setStatus("smgr-status", `Running setup recipe for sites/${target}/…`);
    setStatus("sess-capture-status", `Running setup recipe for sites/${target}/…`);
    const res = await invoke("RunSiteRecipe", target, "");
    await refreshState();
    const msg = res?.summary || `Setup recipe executed for sites/${target}/`;
    setStatus("smgr-status", msg, "ok");
    setStatus("sess-capture-status", msg, "ok");
  } catch (error) {
    const errMsg = error.message || String(error);
    setStatus("smgr-status", errMsg, "error");
    setStatus("sess-capture-status", errMsg, "error");
  }
}

function renderSiteSessionLists(sitesList, activeId) {
  const quickList = byId("site-quick-list");
  if (quickList) {
    quickList.replaceChildren();
    for (const s of sitesList) {
      const hasSaved = Boolean(s.has_saved_session || Number(s.account_count || 0) > 0);
      const item = document.createElement("div");
      item.className = "site-quick-item" + (s.id === activeId ? " active" : "");

      const title = document.createElement("span");
      title.className = "site-quick-title";
      title.textContent = `${hasSaved ? "✓" : "○"} ${s.name || s.id}`;
      title.title = `sites/${s.id}/sessions/ · ${s.start_url || ""}`;

      const actions = document.createElement("div");
      actions.className = "site-quick-actions";

      const browserBtn = document.createElement("button");
      browserBtn.type = "button";
      browserBtn.className = "site-mini-btn site-run-browser-btn";
      browserBtn.dataset.siteBrowser = s.id;
      browserBtn.textContent = "🌐 Browser";
      browserBtn.title = `Run browser for ${s.name || s.id} and auto-save session in sites/${s.id}/sessions/`;
      browserBtn.addEventListener("click", () => runSiteBrowserFor(s.id));

      const botBtn = document.createElement("button");
      botBtn.type = "button";
      botBtn.className = "site-mini-btn site-run-bot-btn";
      botBtn.dataset.siteBot = s.id;
      botBtn.textContent = "🤖 Bot";
      botBtn.title = `Run bot for ${s.name || s.id} using ONLY its saved session in sites/${s.id}/sessions/`;
      botBtn.addEventListener("click", () => runSiteBotFor(s.id));

      actions.append(browserBtn, botBtn);
      item.append(title, actions);
      quickList.appendChild(item);
    }
  }

  for (const containerId of ["sm-site-session-list", "bg-site-session-list"]) {
    const container = byId(containerId);
    if (!container) continue;
    container.replaceChildren();
    for (const s of sitesList) {
      const hasSaved = Boolean(s.has_saved_session || Number(s.account_count || 0) > 0);
      const row = document.createElement("div");
      row.className = "site-session-row";
      row.dataset.siteRow = s.id;

      const info = document.createElement("div");
      info.className = "site-session-info";
      const nameEl = document.createElement("div");
      nameEl.className = "site-session-name";
      nameEl.textContent = `${s.name || s.id} (${s.id})`;
      const metaEl = document.createElement("div");
      metaEl.className = "site-session-meta";
      const sessionPath = s.session_path || `sites/${s.id}/sessions/latest_session.json`;
      metaEl.textContent = `${s.start_url || ""} · Isolated Store: ${sessionPath}`;
      info.append(nameEl, metaEl);

      const actions = document.createElement("div");
      actions.className = "site-session-actions";

      const badge = document.createElement("span");
      badge.className = "site-session-badge" + (hasSaved ? " saved" : "");
      badge.textContent = hasSaved
        ? `✓ Session Saved (${s.account_count || 1})`
        : "○ No Session Yet";

      const browserBtn = document.createElement("button");
      browserBtn.type = "button";
      browserBtn.className = "secondary-button site-row-browser-btn";
      browserBtn.dataset.siteBrowser = s.id;
      browserBtn.textContent = "🌐 Browser (Save Session)";
      browserBtn.addEventListener("click", () => runSiteBrowserFor(s.id));

      const botBtn = document.createElement("button");
      botBtn.type = "button";
      botBtn.className = "primary-button site-row-bot-btn";
      botBtn.dataset.siteBot = s.id;
      botBtn.textContent = "🤖 Run Bot (Saved Session)";
      botBtn.addEventListener("click", () => runSiteBotFor(s.id));

      const folderBtn = document.createElement("button");
      folderBtn.type = "button";
      folderBtn.className = "site-mini-btn site-row-folder-btn";
      folderBtn.dataset.siteFolder = s.id;
      folderBtn.textContent = "Folder";
      folderBtn.addEventListener("click", () => openSiteFolderById(s.id));

      actions.append(badge, browserBtn, botBtn, folderBtn);
      row.append(info, actions);
      container.appendChild(row);
    }
  }
}

async function runSiteBrowserFor(siteId) {
  try {
    setStatus("active-site-status", `Opening browser for sites/${siteId}/ (auto-saving session to sites/${siteId}/sessions/)…`);
    await invoke("RunSiteBrowser", siteId, "default");
    await refreshState();
    const msg = `Browser launched for sites/${siteId}/. Session auto-saves to sites/${siteId}/sessions/!`;
    setStatus("active-site-status", msg, "ok");
    setStatus("sm-run-status", msg, "ok");
    setStatus("bg-login-status", msg, "ok");
  } catch (error) {
    const errMsg = error.message || String(error);
    setStatus("active-site-status", errMsg, "error");
    setStatus("sm-run-status", errMsg, "error");
    setStatus("bg-login-status", errMsg, "error");
  }
}

async function runSiteBotFor(siteId) {
  try {
    setStatus("active-site-status", `Starting bot for sites/${siteId}/ with its saved session…`);
    await invoke("RunSiteBotWithSession", siteId, "default");
    await refreshState();
    const msg = `Bot running for sites/${siteId}/ using ONLY its saved session in sites/${siteId}/sessions/!`;
    setStatus("active-site-status", msg, "ok");
    setStatus("sm-run-status", msg, "ok");
    setStatus("bg-status", msg, "ok");
  } catch (error) {
    const errMsg = error.message || String(error);
    setStatus("active-site-status", errMsg, "error");
    setStatus("sm-run-status", errMsg, "error");
    setStatus("bg-status", errMsg, "error");
  }
}

async function openSiteFolderById(siteId) {
  try {
    const summary = await invoke("OpenSiteFolder", siteId);
    if (summary) setStatus("active-site-status", summary, "ok");
  } catch (error) {
    setStatus("active-site-status", error.message || String(error), "error");
  }
}

async function selectTargetSite(siteId) {
  try {
    const chosen = await invoke("SelectSite", siteId);
    if (state.snapshot) {
      state.snapshot.active_site = chosen.id;
    }
    setStatus("active-site-status", `sites/${chosen.id} · ${chosen.start_url || ""}`, "ok");
    await refreshState();
  } catch (error) {
    setStatus("active-site-status", error.message || String(error), "error");
  }
}

function toggleNewSiteForm() {
  const form = byId("site-new-form");
  if (!form) return;
  form.hidden = !form.hidden;
}

async function createNewSite() {
  const name = value("site-new-name");
  const url = value("site-new-url");
  const mode = value("site-new-mode") || "auto_register";
  if (!name || !url) {
    setStatus("active-site-status", "Enter both Site Name and URL.", "error");
    return;
  }
  try {
    let created;
    if (mode === "browser_setup" && backend() && typeof backend().CreateSiteWithMode === "function") {
      created = await invoke("CreateSiteWithMode", name, url, mode);
    } else {
      created = await invoke("CreateSite", name, url);
    }
    byId("site-new-name").value = "";
    byId("site-new-url").value = "";
    byId("site-new-form").hidden = true;
    await refreshState();
    setStatus("active-site-status", `Created sites/${created.id}`, "ok");
  } catch (error) {
    setStatus("active-site-status", error.message || String(error), "error");
  }
}

async function autoRegisterSiteAccounts(prefix) {
  const statusId = `${prefix}-autoreg-status`;
  try {
    const siteId = value("active-site-select") || "chitchat";
    const countRaw = Number(value(`${prefix}-autoreg-count`) || 3);
    const count = Math.max(1, Math.min(20, countRaw));
    const profile = prefix === "bg" ? (value("bg-profile") || "default") : "default";
    setStatus(statusId, `Auto-registering ${count} account(s) without browser for sites/${siteId}/…`);
    const created = await invoke("AutoRegisterAccounts", siteId, profile, count);
    await refreshState();
    const n = Array.isArray(created) ? created.length : count;
    setStatus(statusId, `Saved ${n} account(s) without browser in sites/${siteId}/accounts/ & profiles/${profile}/!`, "ok");
  } catch (error) {
    setStatus(statusId, error.message || String(error), "error");
  }
}

async function runBrowserAccountSetup() {
  try {
    const siteId = value("active-site-select") || "chitchat";
    const countRaw = Number(value("sm-browser-setup-count") || 1);
    const count = Math.max(1, Math.min(5, countRaw));
    setStatus("sm-browser-setup-status", `Launching ${count} browser window(s) for sites/${siteId}/ account setup…`);
    await invoke("RunBrowserAccountSetup", siteId, "default", count);
    await refreshState();
    setStatus("sm-browser-setup-status", `Browser launched for sites/${siteId}/. Log in or verify once; account saves to sites/${siteId}/accounts/.`, "ok");
  } catch (error) {
    setStatus("sm-browser-setup-status", error.message || String(error), "error");
  }
}

async function openActiveSiteFolder() {
  try {
    const siteId = value("active-site-select") || "chitchat";
    const summary = await invoke("OpenSiteFolder", siteId);
    if (summary) setStatus("active-site-status", summary, "ok");
  } catch (error) {
    setStatus("active-site-status", error.message || String(error), "error");
  }
}

async function runActiveSiteDiagnostics() {
  const box = byId("site-debug-box");
  const summaryEl = byId("site-debug-summary");
  const checksEl = byId("site-debug-checks");
  try {
    const siteId = value("active-site-select") || "chitchat";
    setStatus("active-site-status", `Running 6-step deep diagnostics for sites/${siteId}/…`);
    const report = await invoke("RunSiteDiagnostics", siteId);
    if (box && summaryEl && checksEl && report) {
      box.hidden = false;
      summaryEl.textContent = report.summary || `[${report.overall_status || "PASS"}] sites/${siteId}/`;
      checksEl.replaceChildren();
      for (const c of report.checks || []) {
        const row = document.createElement("div");
        row.className = "site-debug-row";
        row.textContent = `${c.status === "PASS" ? "✓" : c.status === "WARN" ? "⚠" : "✗"} ${c.name}: ${c.detail}`;
        checksEl.appendChild(row);
      }
    }
    setStatus("active-site-status", report?.summary || `Diagnostics complete for sites/${siteId}/`, report?.overall_status === "FAIL" ? "error" : "ok");
  } catch (error) {
    setStatus("active-site-status", error.message || String(error), "error");
  }
}

function fillForms(snapshot) {
  state.snapshot = snapshot;
  state.runningShow = !!snapshot.show_mode_running;
  state.runningBot = !!snapshot.running;

  fillSites(snapshot);
  fillBrowser("sm", snapshot.browser);
  fillBrowser("bg", snapshot.browser);
  fillProxy("sm", snapshot.proxy);
  fillProxy("bg", snapshot.proxy);
  fillMessage("sm", snapshot.message);
  fillMessage("bg", snapshot.message);
  fillShowSettings(snapshot.show_mode);
  fillBotSettings(snapshot.general);
  renderStats(snapshot.home || {});
  renderRunning();

  const steps = String(snapshot.message.sequence || "").split(/\r?\n/).map((l) => l.trim()).filter(Boolean).length;
  byId("bg-summary").textContent = `Backend: ${snapshot.browser.backend} · Profiles auto-start · Message steps: ${steps}`;
  captureGlobalState();
  document.querySelector(".shell")?.classList.add("visible");
}

async function refreshState() {
  const snapshot = await invoke("GetInitialState");
  fillForms(snapshot);
  await refreshProfiles();
  await refreshShowLog();
}

async function refreshStatsOnly() {
  try {
    const snapshot = await invoke("GetInitialState");
    renderStats(snapshot.home || {});
    state.runningBot = !!snapshot.running;
    state.runningShow = !!snapshot.show_mode_running;
    renderRunning();
  } catch (_) { /* transient */ }
}

// ---------------------------------------------------------------------------
// Live logs
// ---------------------------------------------------------------------------
function logKindLabel(kind) {
  return { in: "IN", out: "OUT", skip: "SKIP", system: "SYS", error: "ERR" }[kind] || kind.toUpperCase();
}

function logFilterMatch(entry, filter) {
  if (filter === "sms") return entry.kind === "in" || entry.kind === "out";
  if (filter === "rules") return entry.kind === "out" || entry.kind === "skip" || entry.kind === "system";
  if (filter === "errors") return entry.kind === "error";
  return true;
}

function renderLogFeed(ids, entries) {
  const list = byId(ids.list);
  const filter = byId(ids.filter).value;
  const nearBottom = list.scrollHeight - list.scrollTop - list.clientHeight < 60;
  const fragment = document.createDocumentFragment();
  let shown = 0;
  for (const entry of entries) {
    if (!logFilterMatch(entry, filter)) continue;
    shown += 1;
    const row = document.createElement("div");
    row.className = `log-row log-${entry.kind}`;

    const time = document.createElement("span");
    time.className = "log-time";
    time.textContent = entry.time || "";

    const badge = document.createElement("span");
    badge.className = "log-badge";
    badge.textContent = logKindLabel(entry.kind);

    const body = document.createElement("div");
    body.className = "log-body";
    const text = document.createElement("p");
    text.className = "log-text";
    if (entry.kind === "in" || entry.kind === "out") {
      text.textContent = `"${entry.text || ""}"`;
    } else {
      text.textContent = entry.text || "";
    }
    body.appendChild(text);

    const metaParts = [];
    if (entry.rule) metaParts.push(entry.rule);
    if (entry.detail) metaParts.push(entry.detail);
    if (entry.kind === "out") metaParts.push(entry.setting_ok ? "✓" : "✗");
    if (metaParts.length) {
      const meta = document.createElement("p");
      meta.className = "log-meta";
      meta.textContent = metaParts.join(" · ");
      body.appendChild(meta);
    }

    row.append(time, badge, body);
    fragment.appendChild(row);
  }
  list.replaceChildren(fragment);
  byId(ids.empty).hidden = shown > 0;
  if (nearBottom) list.scrollTop = list.scrollHeight;
}

function renderShowLog() {
  renderLogFeed({ list: "sm-log-list", filter: "sm-log-filter", empty: "sm-log-empty" }, state.showLog.entries);
}

function renderBotLog() {
  renderLogFeed({ list: "bg-log-list", filter: "bg-log-filter", empty: "bg-log-empty" }, state.botLog.entries);
}

async function refreshBotLog() {
  if (!backend() || typeof backend().GetBotLiveLog !== "function") return;
  if (state.botLog.pending) return;
  state.botLog.pending = true;
  try {
    const snapshot = await invoke("GetBotLiveLog", state.botLog.after);
    if (snapshot.entries && snapshot.entries.length) {
      state.botLog.entries.push(...snapshot.entries);
      state.botLog.after = snapshot.version;
      if (state.botLog.entries.length > 600) {
        state.botLog.entries.splice(0, state.botLog.entries.length - 600);
      }
      renderBotLog();
    }
  } catch (_) { /* retry next tick */ }
  finally { state.botLog.pending = false; }
}

async function clearBotLog() {
  try {
    await invoke("ClearBotLiveLog");
    state.botLog.entries = [];
    renderBotLog();
    setStatus("bg-status", "Live log cleared (logs.txt is kept).", "ok");
  } catch (error) {
    setStatus("bg-status", error.message || String(error), "error");
  }
}

async function refreshShowLog() {
  if (!backend() || typeof backend().GetShowModeLog !== "function") return;
  if (state.showLog.pending) return;
  state.showLog.pending = true;
  try {
    const snapshot = await invoke("GetShowModeLog", state.showLog.after);
    if (snapshot.entries && snapshot.entries.length) {
      state.showLog.entries.push(...snapshot.entries);
      state.showLog.after = snapshot.version;
      if (state.showLog.entries.length > 600) {
        state.showLog.entries.splice(0, state.showLog.entries.length - 600);
      }
      renderShowLog();
    }
  } catch (_) { /* retry next tick */ }
  finally { state.showLog.pending = false; }
}

async function clearShowLog() {
  try {
    await invoke("ClearShowModeLog");
    state.showLog.entries = [];
    renderShowLog();
    setStatus("sm-run-status", "Live log cleared (logs.txt is kept).", "ok");
  } catch (error) {
    setStatus("sm-run-status", error.message || String(error), "error");
  }
}

// ---------------------------------------------------------------------------
// Actions
// ---------------------------------------------------------------------------
async function runShowBrowser() {
  if (state.runningShow || state.busy.show) return;
  setBusy("show", true);
  try {
    const show = readShowSettings();
    const message = messagePayload("sm");
    assertFolderReady("sm");
    const proxy = readProxy("sm");
    const selection = selectedProfileChoice();
    const profile = "default";
    const useSession = selection === "session";

    setStatus("sm-run-status", "Applying Setting, Proxy and Message…");
    await invoke("SaveShowModeSettings", show);
    await invoke("SaveProxySettings", proxy);
    await invoke("SaveMessageSettings", message);
    await invoke("SaveBrowserSettings", { profile, backend: "rod", headless: false, use_saved_session: useSession });

    state.showLog.entries = [];
    state.showLog.after = 0;
    renderShowLog();
    setStatus("sm-run-status", useSession ? "Restoring the saved profile session…" : "Launching a fresh browser (manual login)…");
    await invoke("RunBrowser", { profile, backend: "show_mode", headless: false, use_saved_session: useSession }, 1);
    await refreshState();
    setStatus("sm-run-status", "Browser requested. Log in manually; live logs will appear here. Close the browser to stop.", "ok");
  } catch (error) {
    setStatus("sm-run-status", error.message || String(error), "error");
  } finally {
    setBusy("show", false);
  }
}

async function startBackgroundBot() {
  if (state.runningBot || state.busy.bot) return;
  setBusy("bot", true);
  try {
    const general = readBotSettings();
    const message = messagePayload("bg");
    assertFolderReady("bg");
    const proxy = readProxy("bg");
    const profile = value("bg-profile") || "default";
    const backendValue = value("bg-backend") || "rod";
    const headless = byId("bg-headless").checked;

    setStatus("bg-status", "Saving browser, proxy, message and settings…");
    await invoke("SaveBrowserSettings", { profile, backend: backendValue, headless, use_saved_session: !!state.snapshot?.browser?.use_saved_session });
    await invoke("SaveProxySettings", proxy);
    await invoke("SaveMessageSettings", message);
    await invoke("SaveGeneralSettings", general);
    state.botLog.entries = [];
    state.botLog.after = 0;
    renderBotLog();
    setStatus("bg-status", "Starting workers…");
    await invoke("StartBot");
    await refreshState();
    setStatus("bg-status", "Started. One worker runs per saved profile.", "ok");
  } catch (error) {
    setStatus("bg-status", error.message || String(error), "error");
  } finally {
    setBusy("bot", false);
  }
}

async function stopBackgroundBot() {
  try {
    await invoke("StopBot");
    await refreshState();
    setStatus("bg-status", "Stopped.", "ok");
  } catch (error) {
    setStatus("bg-status", error.message || String(error), "error");
  }
}

async function launchLogin() {
  try {
    const settings = {
      profile: value("bg-profile") || "default",
      backend: value("bg-backend") || "rod",
      headless: byId("bg-headless").checked,
    };
    setStatus("bg-login-status", `Launching ${settings.backend} for login…`);
    await invoke("RunBrowser", settings, 1);
    await refreshState();
    setStatus("bg-login-status", "Launch requested. Finish the login in the browser; it closes after the session is saved.", "ok");
  } catch (error) {
    setStatus("bg-login-status", error.message || String(error), "error");
  }
}

// ---------------------------------------------------------------------------
// Wiring
// ---------------------------------------------------------------------------
function bindForms() {
  for (const prefix of ["sm", "bg"]) {
    document.querySelectorAll(`input[name="${prefix}-chat-mode"]`).forEach((radio) => {
      radio.addEventListener("change", (event) => setChatMode(prefix, event.target.value));
    });
    for (const key of ["single", "folder", "evachat", "api"]) {
      byId(`${prefix}-mode-option-${key}`).addEventListener("click", () => {
        setChatMode(prefix, key === "evachat" ? "eva_chat_flow" : key === "api" ? "api_chat" : key);
      });
    }
    byId(`${prefix}-first-mode`).addEventListener("change", () => updateFirst(prefix));
    byId(`${prefix}-first-wait`).addEventListener("change", () => updateFirst(prefix));
    byId(`${prefix}-profile`).addEventListener("input", () => updateProfileStatus(prefix));
    byId(`${prefix}-profile`).addEventListener("change", () => updateProfileStatus(prefix));
    for (const suffix of ["sequence", "usernames"]) {
      const el = byId(`${prefix}-${suffix}`);
      if (el) {
        el.addEventListener("input", () => syncMessageFields(prefix));
        el.addEventListener("change", () => syncMessageFields(prefix));
      }
    }
    if (prefix === "sm") {
      document.querySelectorAll('input[name="sm-profile-choice"]').forEach((radio) => {
        radio.addEventListener("change", () => updateProfileStatus("sm"));
      });
    }
  }
}

function bindActions() {
  byId("active-site-select").addEventListener("change", (e) => selectTargetSite(e.target.value));
  const siteSearch = byId("site-search-input");
  if (siteSearch) {
    siteSearch.addEventListener("input", () => {
      if (state.snapshot) fillSites(state.snapshot);
    });
  }
  const fAll = byId("site-filter-all");
  const fSaved = byId("site-filter-saved");
  const fAuto = byId("site-filter-autoreg");
  const fBrowser = byId("site-filter-browser");
  if (fAll) fAll.addEventListener("click", () => setSiteFilter("all"));
  if (fSaved) fSaved.addEventListener("click", () => setSiteFilter("saved_session"));
  if (fAuto) fAuto.addEventListener("click", () => setSiteFilter("auto_register"));
  if (fBrowser) fBrowser.addEventListener("click", () => setSiteFilter("browser_setup"));

  byId("site-toggle-new").addEventListener("click", toggleNewSiteForm);
  byId("site-create-submit").addEventListener("click", createNewSite);
  byId("site-open-folder").addEventListener("click", openActiveSiteFolder);
  const siteDebugBtn = byId("site-run-debug");
  if (siteDebugBtn) siteDebugBtn.addEventListener("click", runActiveSiteDiagnostics);

  const smAutoBtn = byId("sm-autoreg-btn");
  const smBrowserBtn = byId("sm-browser-setup-btn");
  const bgAutoBtn = byId("bg-autoreg-btn");
  if (smAutoBtn) smAutoBtn.addEventListener("click", () => autoRegisterSiteAccounts("sm"));
  if (smBrowserBtn) smBrowserBtn.addEventListener("click", runBrowserAccountSetup);
  if (bgAutoBtn) bgAutoBtn.addEventListener("click", () => autoRegisterSiteAccounts("bg"));

  byId("show-refresh").addEventListener("click", async () => {
    await refreshState();
    setStatus("sm-run-status", "Refreshed.", "ok");
  });
  byId("sm-run").addEventListener("click", runShowBrowser);
  byId("sm-test-proxy").addEventListener("click", () => testProxy("sm"));
  byId("sm-load-folder").addEventListener("click", () => loadFolder("sm"));
  byId("sm-clean-loaded").addEventListener("click", () => cleanLoaded("sm"));
  byId("sm-open-eva").addEventListener("click", () => openEvaFolder("sm"));
  byId("sm-open-api").addEventListener("click", () => openApiFolder("sm"));
  byId("sm-save-message").addEventListener("click", () => saveMessageSettings("sm"));
  byId("sm-log-clear").addEventListener("click", clearShowLog);
  byId("sm-log-filter").addEventListener("change", renderShowLog);

  byId("bg-refresh").addEventListener("click", async () => {
    await refreshState();
    setStatus("bg-status", "Refreshed.", "ok");
  });
  byId("bg-start").addEventListener("click", startBackgroundBot);
  byId("bg-stop").addEventListener("click", stopBackgroundBot);
  byId("bg-launch-login").addEventListener("click", launchLogin);
  byId("bg-test-proxy").addEventListener("click", () => testProxy("bg"));
  byId("bg-load-folder").addEventListener("click", () => loadFolder("bg"));
  byId("bg-clean-loaded").addEventListener("click", () => cleanLoaded("bg"));
  byId("bg-open-eva").addEventListener("click", () => openEvaFolder("bg"));
  byId("bg-open-api").addEventListener("click", () => openApiFolder("bg"));
  byId("bg-save-message").addEventListener("click", () => saveMessageSettings("bg"));
  byId("bg-log-clear").addEventListener("click", clearBotLog);
  byId("bg-log-filter").addEventListener("change", renderBotLog);

  // Dashboard 3-Tab bar & Tab 1/2/3 actions
  const tabControl = byId("dash-tab-control-center");
  const tabSites = byId("dash-tab-site-manager");
  const tabSessions = byId("dash-tab-sessions");
  if (tabControl) tabControl.addEventListener("click", () => activateDashboardTab("control-center"));
  if (tabSites) tabSites.addEventListener("click", () => activateDashboardTab("site-manager"));
  if (tabSessions) tabSessions.addEventListener("click", () => activateDashboardTab("sessions"));

  const ccStartAll = byId("cc-start-all-btn");
  const ccStopAll = byId("cc-stop-all-btn");
  const ccJumpLogs = byId("cc-jump-logs-btn");
  if (ccStartAll) ccStartAll.addEventListener("click", startAllSitesRunner);
  if (ccStopAll) ccStopAll.addEventListener("click", stopAllSitesRunner);
  if (ccJumpLogs) {
    ccJumpLogs.addEventListener("click", () => {
      activateSubpanel(state.tab === "bot" ? "bg-logs" : "show-logs");
    });
  }

  const smgrGroupAll = byId("smgr-group-all");
  const smgrGroupA = byId("smgr-group-a");
  const smgrGroupB = byId("smgr-group-b");
  const smgrGroupC = byId("smgr-group-c");
  const smgrGroupD = byId("smgr-group-d");
  const smgrGroupE = byId("smgr-group-e");
  if (smgrGroupAll) smgrGroupAll.addEventListener("click", () => setSiteManagerGroup("ALL"));
  if (smgrGroupA) smgrGroupA.addEventListener("click", () => setSiteManagerGroup("A"));
  if (smgrGroupB) smgrGroupB.addEventListener("click", () => setSiteManagerGroup("B"));
  if (smgrGroupC) smgrGroupC.addEventListener("click", () => setSiteManagerGroup("C"));
  if (smgrGroupD) smgrGroupD.addEventListener("click", () => setSiteManagerGroup("D"));
  if (smgrGroupE) smgrGroupE.addEventListener("click", () => setSiteManagerGroup("E"));

  const smgrSearch = byId("smgr-search-input");
  if (smgrSearch) smgrSearch.addEventListener("input", () => renderSiteManager());

  const smgrFilterGroup = byId("smgr-filter-group");
  const smgrFilterStatus = byId("smgr-filter-status");
  const smgrFilterSession = byId("smgr-filter-session");
  if (smgrFilterGroup) {
    smgrFilterGroup.addEventListener("change", (e) => setSiteManagerGroup(e.target.value));
  }
  if (smgrFilterStatus) {
    smgrFilterStatus.addEventListener("change", (e) => {
      state.smgrStatusFilter = e.target.value || "all";
      renderSiteManager();
    });
  }
  if (smgrFilterSession) {
    smgrFilterSession.addEventListener("change", (e) => {
      state.smgrSessionFilter = e.target.value || "all";
      renderSiteManager();
    });
  }

  const smgrStartSel = byId("smgr-start-selected-btn");
  const smgrFooterStartSel = byId("smgr-footer-start-selected-btn");
  const smgrStopSel = byId("smgr-stop-selected-btn");
  const smgrStopAll = byId("smgr-stop-all-btn");
  const smgrRefresh = byId("smgr-refresh-btn");
  const smgrFooterRefresh = byId("smgr-footer-refresh-btn");
  if (smgrStartSel) smgrStartSel.addEventListener("click", startSelectedSitesRunner);
  if (smgrFooterStartSel) smgrFooterStartSel.addEventListener("click", startSelectedSitesRunner);
  if (smgrStopSel) smgrStopSel.addEventListener("click", stopSelectedSitesRunner);
  if (smgrStopAll) smgrStopAll.addEventListener("click", stopAllSitesRunner);
  const doSmgrRefresh = async () => {
    await refreshState();
    await refreshSiteStatuses();
    setStatus("smgr-status", "Site Manager refreshed.", "ok");
  };
  if (smgrRefresh) smgrRefresh.addEventListener("click", doSmgrRefresh);
  if (smgrFooterRefresh) smgrFooterRefresh.addEventListener("click", doSmgrRefresh);

  const smgrDetailStart = byId("smgr-detail-start");
  const smgrDetailStop = byId("smgr-detail-stop");
  const smgrDetailRecipe = byId("smgr-detail-recipe");
  const smgrDetailBrowser = byId("smgr-detail-browser");
  const smgrDetailRefreshSession = byId("smgr-detail-refresh-session");
  const smgrDetailFolder = byId("smgr-detail-folder-btn");
  const smgrDetailClose = byId("smgr-detail-close-btn");
  if (smgrDetailStart) smgrDetailStart.addEventListener("click", () => startSingleSiteRunner());
  if (smgrDetailStop) smgrDetailStop.addEventListener("click", () => stopSingleSiteRunner());
  if (smgrDetailRecipe) smgrDetailRecipe.addEventListener("click", runRecipeForActiveSite);
  if (smgrDetailBrowser) smgrDetailBrowser.addEventListener("click", () => runSiteBrowserFor(value("active-site-select") || "chitchat"));
  if (smgrDetailRefreshSession) smgrDetailRefreshSession.addEventListener("click", refreshActiveSiteSession);
  if (smgrDetailFolder) smgrDetailFolder.addEventListener("click", openActiveSiteFolder);
  if (smgrDetailClose) smgrDetailClose.addEventListener("click", closeSiteDetailPanel);

  const ovProxyChk = byId("smgr-ov-proxy-enabled");
  const ovModeChk = byId("smgr-ov-mode-enabled");
  const ovDelayChk = byId("smgr-ov-delay-enabled");
  const ovSaveBtn = byId("smgr-ov-save-btn");
  const ovClearBtn = byId("smgr-ov-clear-btn");
  if (ovProxyChk) ovProxyChk.addEventListener("change", syncOverrideToggleRows);
  if (ovModeChk) ovModeChk.addEventListener("change", syncOverrideToggleRows);
  if (ovDelayChk) ovDelayChk.addEventListener("change", syncOverrideToggleRows);
  if (ovSaveBtn) ovSaveBtn.addEventListener("click", saveActiveSiteOverride);
  if (ovClearBtn) ovClearBtn.addEventListener("click", clearActiveSiteOverride);

  // Close slide-out detail panel when clicking outside the detail card and site rows
  document.addEventListener("click", (event) => {
    if (state.dashTab !== "site-manager" || !state.detailOpen) return;
    const detailCard = byId("smgr-site-detail");
    if (!detailCard || detailCard.classList.contains("closed")) return;
    const target = event.target;
    if (!target || typeof target.closest !== "function") return;
    if (
      detailCard.contains(target) ||
      target.closest(".smgr-site-row") ||
      target.closest(".smgr-site-name-btn") ||
      target.closest("[data-smgr-detail]")
    ) {
      return;
    }
    closeSiteDetailPanel();
  });

  const sessRefresh = byId("sess-refresh-btn");
  const sessCapMissing = byId("sess-capture-missing-btn");
  const sessCapActive = byId("sess-capture-active-btn");
  const sessValActive = byId("sess-validate-active-btn");
  const sessClrActive = byId("sess-clear-active-btn");
  const sessCapBrowser = byId("sess-capture-browser-btn");
  const sessCapAutoReg = byId("sess-capture-autoreg-btn");
  const sessCapRecipe = byId("sess-capture-recipe-btn");
  const sessSearch = byId("sess-search-input");
  const sessFilterStatus = byId("sess-filter-status");
  if (sessRefresh) {
    sessRefresh.addEventListener("click", async () => {
      await refreshState();
      await refreshSiteStatuses();
      await refreshSessionStatuses();
      setStatus("sess-status", "Session health refreshed.", "ok");
    });
  }
  if (sessCapMissing) {
    sessCapMissing.addEventListener("click", captureAllMissingSessionsBatch);
  }
  if (sessCapActive) {
    sessCapActive.addEventListener("click", () => captureSiteSessionFor(value("active-site-select") || "chitchat", false));
  }
  if (sessValActive) {
    sessValActive.addEventListener("click", () => validateSiteSessionFor(value("active-site-select") || "chitchat"));
  }
  if (sessClrActive) {
    sessClrActive.addEventListener("click", () => clearSiteSessionFor(value("active-site-select") || "chitchat"));
  }
  if (sessSearch) {
    sessSearch.addEventListener("input", () => renderSessionsTab());
  }
  if (sessFilterStatus) {
    sessFilterStatus.addEventListener("change", (e) => {
      state.sessFilterStatus = e.target.value || "all";
      renderSessionsTab();
    });
  }
  if (sessCapBrowser) {
    sessCapBrowser.addEventListener("click", async () => {
      const siteId = value("active-site-select") || "chitchat";
      await runSiteBrowserFor(siteId);
      setStatus("sess-capture-status", `Browser launched for sites/${siteId}/sessions/ capture.`, "ok");
    });
  }
  if (sessCapAutoReg) {
    sessCapAutoReg.addEventListener("click", async () => {
      await autoRegisterSiteAccounts("sm");
      setStatus("sess-capture-status", byId("sm-autoreg-status")?.textContent || "Guest session auto-registered.", "ok");
    });
  }
  if (sessCapRecipe) {
    sessCapRecipe.addEventListener("click", runRecipeForActiveSite);
  }
}

function bindEvents() {
  const runtime = runtimeEvents();
  if (typeof runtime.EventsOn !== "function") return;
  runtime.EventsOn("ui:showlog", () => refreshShowLog());
  runtime.EventsOn("ui:botlog", () => refreshBotLog());
  runtime.EventsOn("ui:site_statuses", (statuses) => {
    if (statuses && typeof statuses === "object") {
      state.siteStatuses = statuses;
      if (state.dashTab === "site-manager") renderSiteManager();
      if (state.dashTab === "sessions") renderSessionsTab();
    }
  });
  runtime.EventsOn("ui:session_captured", (summary) => {
    if (summary && summary.site_id) {
      state.sessionStatuses[summary.site_id] = summary;
      renderSessionsTab();
      if (state.dashTab === "site-manager") renderSiteManager();
    }
  });
  runtime.EventsOn("ui:session_cleared", (summary) => {
    if (summary && summary.site_id) {
      state.sessionStatuses[summary.site_id] = summary;
      renderSessionsTab();
      if (state.dashTab === "site-manager") renderSiteManager();
    }
  });
  runtime.EventsOn("ui:show_running", (running) => {
    state.runningShow = !!running;
    renderRunning();
  });
  runtime.EventsOn("ui:running", (running) => {
    state.runningBot = !!running;
    renderRunning();
  });
  runtime.EventsOn("ui:log", (entry) => {
    if (typeof entry.message === "string" && /\[[^\]]+ fresh \d+\/\d+\]/.test(entry.message)
        && !entry.message.includes("] Removed temporary browser data from ")) {
      setStatus("bg-login-status", entry.message, entry.level === "error" ? "error" : "");
    }
  });
}

async function bootstrap() {
  bindTabs();
  bindForms();
  bindGlobalState();
  bindActions();
  bindEvents();
  activateTab("show");

  let attempts = 0;
  while (attempts < 50) {
    attempts += 1;
    try {
      await refreshState();
      await refreshSiteStatuses();
      setInterval(refreshStatsOnly, 3000);
      setInterval(refreshSiteStatuses, 5000);
      setInterval(refreshProfiles, 10000);
      setInterval(() => { if (state.tab === "show") refreshShowLog(); }, 1000);
      setInterval(() => { if (state.tab === "bot") refreshBotLog(); }, 1000);
      return;
    } catch (error) {
      await new Promise((resolve) => setTimeout(resolve, 200));
    }
  }
  setStatus("sm-run-status", "Failed to connect to Wails bindings.", "error");
}

window.addEventListener("error", (event) => {
  reportFrontendError("Uncaught error", event.error || event.message);
});
window.addEventListener("unhandledrejection", (event) => {
  reportFrontendError("Unhandled promise rejection", event.reason);
});

window.addEventListener("DOMContentLoaded", () => {
  bootstrap().catch((error) => {
    reportFrontendError("Bootstrap failed", error);
    setStatus("sm-run-status", error.message || String(error), "error");
  });
});
