// Two-page UI: Show Mode (5 sections + live log) and Background Bot.
const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { JSDOM } = require('jsdom');

const assets = path.join(__dirname, '../cmd/chitchatui/frontend/dist');
const html = fs.readFileSync(path.join(assets, 'index.html'), 'utf8');
const appJs = fs.readFileSync(path.join(assets, 'app.js'), 'utf8');
const tick = () => new Promise((resolve) => setTimeout(resolve, 0));

function snapshot(over = {}) {
  return {
    running: false,
    show_mode_running: false,
    home: { bot_accounts: 1, running_accounts: 0, messages_sent: 6, traffic_users: 2, snap_tg_sent_count: 0, dead_accounts: 0 },
    browser: { profile: 'default', backend: 'rod', headless: false, use_saved_session: false },
    message: { chat_mode: 'single', sequence: 'hello\nwait', usernames: '@u', loaded_sequences: [], ai_enabled: false, ai_base_url: '', ai_model: '', ai_api_key: '' },
    proxy: { enabled: false, entries: '' },
    general: {
      first_message: { mode: 'bot', bot_first_percent: 70, wait_timeout: false, wait_min_seconds: 30, wait_max_seconds: 60 },
      poll_min_seconds: 20, poll_max_seconds: 45,
      action_min_seconds: 20, action_max_seconds: 45,
      match_reply_timeout_min_seconds: 60, match_reply_timeout_max_seconds: 60,
      match_close_wait_min_seconds: 0, match_close_wait_max_seconds: 0,
      reply_delay_min_seconds: 5, reply_delay_max_seconds: 9,
    },
    show_mode: {
      first_message: { mode: 'bot', bot_first_percent: 70, wait_timeout: false, wait_min_seconds: 30, wait_max_seconds: 60 },
      reply_delay_min_seconds: 5, reply_delay_max_seconds: 9,
      auto_skip: true, no_reply_min_seconds: 60, no_reply_max_seconds: 90,
      next_chat_min_seconds: 3, next_chat_max_seconds: 7,
    },
    browser_options: [],
    logs: [],
    ...over,
  };
}

function setup(api = {}, snap = snapshot()) {
  const dom = new JSDOM(html, { runScripts: 'outside-only', url: 'http://localhost/' });
  const w = dom.window;
  w.addEventListener = () => {};
  w.go = { main: { App: { GetInitialState: async () => snap, ...api } } };
  w.eval(appJs);
  w.bindForms();
  w.bindGlobalState();
  w.bindActions();
  return { dom, w, d: w.document, snap };
}

test('the app is two pages: Show Mode first, Background Bot second', () => {
  const { dom, d } = setup();
  try {
    assert.deepEqual([...d.querySelectorAll('.nav-button')].map((b) => b.dataset.tab), ['show', 'bot']);
    assert.equal(d.querySelectorAll('.tab-panel').length, 2);
    assert.ok(d.querySelector('[data-panel="show"]').classList.contains('active'));
    const headings = [...d.querySelectorAll('[data-panel="show"] h2')].map((h) => h.textContent.trim());
    assert.ok(headings.some((h) => h.startsWith('1 · Setting')));
    assert.ok(headings.some((h) => h.startsWith('2 · Proxy')));
    assert.ok(headings.some((h) => h.startsWith('3 · Message')));
    assert.ok(headings.some((h) => h.startsWith('4 · Run Browser')));
    assert.ok(headings.some((h) => h.startsWith('5 · Live Logs')));
    assert.notEqual(d.getElementById('sm-run'), null);
    assert.equal(d.querySelector('#start-bot, #start-show-mode, #dashboard-chooser, #home-chat'), null);
    assert.notEqual(d.getElementById('bg-start'), null);
    assert.notEqual(d.getElementById('bg-launch-login'), null);
  } finally { dom.window.close(); }
});

test('sidebar switches between the two pages', () => {
  const { dom, w, d } = setup();
  try {
    w.activateTab('bot');
    assert.ok(d.querySelector('[data-panel="bot"]').classList.contains('active'));
    assert.ok(!d.querySelector('[data-panel="show"]').classList.contains('active'));
    assert.ok(d.querySelector('.nav-button[data-tab="bot"]').classList.contains('active'));
    w.activateTab('show');
    assert.ok(d.querySelector('[data-panel="show"]').classList.contains('active'));
  } finally { dom.window.close(); }
});

test('3-tab dashboard structure below Target Site bar switches Control Center, Site Manager, and Sessions', async () => {
  const startAllCalls = [];
  const startSitesCalls = [];
  const multiSnap = snapshot({
    active_site: 'chitchat',
    sites: [
      { id: 'chitchat', name: 'ChitChat.gg', account_mode: 'browser_setup', account_count: 1, has_saved_session: true, start_url: 'https://app.chitchat.gg/text?auth=email', auth_mode: 'chitchat_jwt' },
      { id: 'omezy', name: 'Omezy.io', account_mode: 'auto_register', account_count: 3, has_saved_session: true, start_url: 'https://omezy.io/text/', auth_mode: 'dom_only' },
    ],
  });
  const { dom, w, d } = setup({
    StartAll: async () => { startAllCalls.push(true); },
    StartSites: async (list) => { startSitesCalls.push(list); },
    GetSiteStatuses: async () => ({
      chitchat: { site_id: 'chitchat', state: 'running', messages_sent: 4 },
      omezy: { site_id: 'omezy', state: 'stopped', messages_sent: 0 },
    }),
  }, multiSnap);
  try {
    w.bindTabs();
    w.fillForms(multiSnap);

    // 1. Tab bar is placed directly below Target Site bar (.site-selector-box)
    const siteBox = d.querySelector('.site-selector-box');
    const tabBar = d.getElementById('dashboard-tab-bar');
    assert.notEqual(siteBox, null);
    assert.notEqual(tabBar, null);
    assert.equal(siteBox.nextElementSibling, tabBar);

    const tabButtons = [...d.querySelectorAll('.dash-tab-button')];
    assert.deepEqual(tabButtons.map((b) => b.dataset.dashTab), ['control-center', 'site-manager', 'sessions']);
    assert.match(tabButtons[0].textContent, /Control Center/);
    assert.match(tabButtons[1].textContent, /Site Manager/);
    assert.match(tabButtons[2].textContent, /Sessions/);

    // 2. Default active tab is Tab 1 ("Control Center") and contains Show Mode & Background Bot
    const pControl = d.getElementById('dash-panel-control-center');
    const pSites = d.getElementById('dash-panel-site-manager');
    const pSessions = d.getElementById('dash-panel-sessions');
    assert.equal(pControl.classList.contains('active'), true);
    assert.equal(pControl.hidden, false);
    assert.equal(pSites.hidden, true);
    assert.equal(pSessions.hidden, true);
    assert.notEqual(pControl.querySelector('[data-panel="show"]'), null);
    assert.notEqual(pControl.querySelector('[data-panel="bot"]'), null);
    assert.equal(d.getElementById('control-center-sidebar-nav').hidden, false);

    // 3. Clicking Tab 2 ("Site Manager") shows site grid + group view + per-site detail and hides Tab 1 & Tab 3
    d.getElementById('dash-tab-site-manager').click();
    assert.equal(d.getElementById('dash-tab-site-manager').classList.contains('active'), true);
    assert.equal(d.getElementById('dash-tab-control-center').classList.contains('active'), false);
    assert.equal(pSites.classList.contains('active'), true);
    assert.equal(pSites.hidden, false);
    assert.equal(pControl.hidden, true);
    assert.equal(pSessions.hidden, true);
    assert.equal(d.getElementById('control-center-sidebar-nav').hidden, true);
    assert.equal(d.querySelectorAll('#smgr-site-grid .smgr-grid-item').length, 2);
    assert.match(d.getElementById('smgr-detail-title').textContent, /ChitChat\.gg/);

    // Group filter in Site Manager
    d.getElementById('smgr-group-c').click();
    assert.equal(d.getElementById('smgr-group-c').classList.contains('active'), true);
    assert.equal(d.querySelectorAll('#smgr-site-grid .smgr-grid-item').length, 1);
    d.getElementById('smgr-group-all').click();
    assert.equal(d.querySelectorAll('#smgr-site-grid .smgr-grid-item').length, 2);

    // Start Selected in Site Manager
    d.getElementById('smgr-start-selected-btn').click();
    await tick();
    assert.deepEqual(JSON.parse(JSON.stringify(startSitesCalls)), [['chitchat']]);

    // 4. Clicking Tab 3 ("Sessions") shows session capture + health monitor and hides Tab 1 & Tab 2
    d.getElementById('dash-tab-sessions').click();
    assert.equal(d.getElementById('dash-tab-sessions').classList.contains('active'), true);
    assert.equal(pSessions.classList.contains('active'), true);
    assert.equal(pSessions.hidden, false);
    assert.equal(pControl.hidden, true);
    assert.equal(pSites.hidden, true);
    assert.equal(d.getElementById('sess-stat-total').textContent, '2');
    assert.equal(d.getElementById('sess-stat-saved').textContent, '2');
    assert.equal(d.querySelectorAll('#sess-health-list .site-session-row').length, 2);

    // 5. Clicking Tab 1 ("Control Center") returns to Control Center and Start All works
    d.getElementById('dash-tab-control-center').click();
    assert.equal(d.getElementById('dash-tab-control-center').classList.contains('active'), true);
    assert.equal(pControl.hidden, false);
    assert.equal(pSites.hidden, true);
    assert.equal(pSessions.hidden, true);
    d.getElementById('cc-start-all-btn').click();
    await tick();
    assert.equal(startAllCalls.length, 1);
  } finally { dom.window.close(); }
});

test('Prompt 8: Site Grid (Tab 2) collapsible A-E groups, 5 status badges, 3 session badges, group actions, and 102-site lag-free rendering', async () => {
  const startedLists = [];
  const stoppedLists = [];
  const startedSingle = [];
  const stoppedSingle = [];
  const browserOpened = [];
  const selectedTarget = [];
  let stopAllCount = 0;

  const mockSites = [
    { id: 'talkwithstranger', name: 'TalkWithStranger', group: 'A', account_mode: 'auto_register', account_count: 1, has_saved_session: true, session_status: 'valid', start_url: 'https://talkwithstranger.com' },
    { id: 'omezy', name: 'Omezy.io', group: 'B', account_mode: 'auto_register', account_count: 2, has_saved_session: true, session_status: 'expiring', start_url: 'https://omezy.io/text/' },
    { id: 'chatib', name: 'Chatib.us', group: 'B', account_mode: 'auto_register', account_count: 0, has_saved_session: false, session_status: 'missing', start_url: 'https://www.chatib.us' },
    { id: 'chitchat', name: 'ChitChat.gg', group: 'C', account_mode: 'browser_setup', account_count: 1, has_saved_session: true, session_status: 'valid', start_url: 'https://app.chitchat.gg/text?auth=email' },
    { id: 'uhmegle', name: 'Uhmegle', group: 'D', account_mode: 'auto_register', account_count: 0, has_saved_session: false, session_status: 'missing', start_url: 'https://uhmegle.com/text' },
    { id: 'emeraldchat', name: 'EmeraldChat', group: 'E', account_mode: 'browser_setup', account_count: 0, has_saved_session: false, session_status: 'missing', start_url: 'https://emeraldchat.com' },
  ];

  const mockStatuses = {
    talkwithstranger: { site_id: 'talkwithstranger', status: 'running', running: true },
    omezy: { site_id: 'omezy', status: 'paused', paused: true },
    chatib: { site_id: 'chatib', status: 'failed', failed: true },
    chitchat: { site_id: 'chitchat', status: 'paused', last_error_type: 'banned' },
    uhmegle: { site_id: 'uhmegle', status: 'stopped' },
    emeraldchat: { site_id: 'emeraldchat', status: 'stopped' },
  };

  const snap = snapshot({ active_site: 'chitchat', sites: mockSites });
  const { dom, w, d } = setup({
    GetSiteStatuses: async () => mockStatuses,
    StartSites: async (list) => { startedLists.push(JSON.parse(JSON.stringify(list))); },
    StopSites: async (list) => { stoppedLists.push(JSON.parse(JSON.stringify(list))); },
    StartSite: async (id) => { startedSingle.push(id); },
    StopSite: async (id) => { stoppedSingle.push(id); },
    StopAll: async () => { stopAllCount += 1; },
    RunSiteBrowser: async (id) => { browserOpened.push(id); },
    SelectSite: async (id) => {
      selectedTarget.push(id);
      const found = mockSites.find((s) => s.id === id) || mockSites[0];
      return found;
    },
  }, snap);

  try {
    w.bindTabs();
    w.fillForms(snap);
    await w.refreshSiteStatuses();
    d.getElementById('dash-tab-site-manager').click();

    // 1. Group-wise collapsible sections A/B/C/D/E with site count in each group header
    const sections = [...d.querySelectorAll('#smgr-site-grid .smgr-group-section')];
    assert.deepEqual(sections.map((s) => s.dataset.groupSection), ['A', 'B', 'C', 'D', 'E']);
    assert.equal(d.querySelector('[data-group-count="A"]').textContent, '1 site');
    assert.equal(d.querySelector('[data-group-count="B"]').textContent, '2 sites');
    assert.equal(d.querySelector('[data-group-count="C"]').textContent, '1 site');

    // Collapsible toggle works
    d.querySelector('[data-group-toggle="B"]').click();
    assert.equal(d.querySelector('[data-group-section="B"]').classList.contains('collapsed'), true);
    d.querySelector('[data-group-toggle="B"]').click();
    assert.equal(d.querySelector('[data-group-section="B"]').classList.contains('collapsed'), false);

    // 2. Status badges: green=running, yellow=paused, red=failed, orange=banned, gray=stopped
    assert.ok(d.querySelector('[data-status-badge="talkwithstranger"]').classList.contains('status-running'));
    assert.ok(d.querySelector('[data-status-badge="omezy"]').classList.contains('status-paused'));
    assert.ok(d.querySelector('[data-status-badge="chatib"]').classList.contains('status-failed'));
    assert.ok(d.querySelector('[data-status-badge="chitchat"]').classList.contains('status-banned'));
    assert.ok(d.querySelector('[data-status-badge="uhmegle"]').classList.contains('status-stopped'));

    // 3. Session badges: ✅ valid, ⚠️ expiring, ❌ missing
    assert.ok(d.querySelector('[data-session-badge="talkwithstranger"]').classList.contains('session-valid'));
    assert.equal(d.querySelector('[data-session-badge="talkwithstranger"]').textContent, '✅ valid');
    assert.ok(d.querySelector('[data-session-badge="omezy"]').classList.contains('session-expiring'));
    assert.equal(d.querySelector('[data-session-badge="omezy"]').textContent, '⚠️ expiring');
    assert.ok(d.querySelector('[data-session-badge="chatib"]').classList.contains('session-missing'));
    assert.equal(d.querySelector('[data-session-badge="chatib"]').textContent, '❌ missing');

    // 4. Group header actions: Select All, Start Group, Stop Group
    d.querySelector('[data-group-select-all="B"]').click();
    assert.equal(d.querySelector('[data-site-select-check="omezy"]').checked, true);
    assert.equal(d.querySelector('[data-site-select-check="chatib"]').checked, true);
    assert.equal(d.getElementById('smgr-selected-count').textContent, '2 site(s) selected');

    d.querySelector('[data-group-start="B"]').click();
    await tick();
    assert.deepEqual(startedLists, [['omezy', 'chatib']]);

    d.querySelector('[data-group-stop="B"]').click();
    await tick();
    assert.deepEqual(stoppedLists, [['omezy', 'chatib']]);

    // 5. Per-site action buttons: Start, Stop, Open Browser, Detail + Site Name click opens Detail panel
    d.querySelector('[data-smgr-start="talkwithstranger"]').click();
    await tick();
    assert.deepEqual(startedSingle, ['talkwithstranger']);

    d.querySelector('[data-smgr-stop="talkwithstranger"]').click();
    await tick();
    assert.deepEqual(stoppedSingle, ['talkwithstranger']);

    d.querySelector('[data-smgr-browser="omezy"]').click();
    await tick();
    assert.deepEqual(browserOpened, ['omezy']);

    d.querySelector('[data-site-name="omezy"]').click();
    await tick();
    assert.ok(selectedTarget.includes('omezy'));
    assert.equal(d.getElementById('smgr-site-detail').classList.contains('open'), true);

    // 6. Status & Session filters and Bottom bar (Start All Selected, Stop All, Refresh)
    const statusSelect = d.getElementById('smgr-filter-status');
    statusSelect.value = 'banned';
    statusSelect.dispatchEvent(new w.Event('change'));
    assert.equal(d.querySelectorAll('#smgr-site-grid .smgr-site-row').length, 1);
    statusSelect.value = 'all';
    statusSelect.dispatchEvent(new w.Event('change'));

    const sessionSelect = d.getElementById('smgr-filter-session');
    sessionSelect.value = 'expiring';
    sessionSelect.dispatchEvent(new w.Event('change'));
    assert.equal(d.querySelectorAll('#smgr-site-grid .smgr-site-row').length, 1);
    sessionSelect.value = 'all';
    sessionSelect.dispatchEvent(new w.Event('change'));

    d.getElementById('smgr-footer-start-selected-btn').click();
    await tick();
    assert.deepEqual(startedLists[startedLists.length - 1], ['omezy', 'chatib']);

    d.getElementById('smgr-stop-all-btn').click();
    await tick();
    assert.equal(stopAllCount, 1);

    // 7. Verify 102 sites render smoothly in < 100ms
    const full102 = Array.from({ length: 102 }, (_, i) => ({
      id: `site_${i + 1}`,
      name: `Site ${i + 1}`,
      group: ['A', 'B', 'C', 'D', 'E'][i % 5],
      account_mode: 'auto_register',
      has_saved_session: i % 2 === 0,
      start_url: `https://site${i + 1}.example.com`,
    }));
    const t0 = Date.now();
    w.fillForms(snapshot({ active_site: 'site_1', sites: full102 }));
    const elapsedMs = Date.now() - t0;
    assert.equal(d.querySelectorAll('#smgr-site-grid .smgr-site-row').length, 102);
    assert.ok(elapsedMs < 500, `102 sites rendered in ${elapsedMs}ms`);
  } finally { dom.window.close(); }
});

test('chat mode reveals exactly one editor per page, independently', () => {
  const { dom, w, d, snap } = setup();
  try {
    w.fillForms(snap);
    const visible = (id) => d.getElementById(id).style.display !== 'none';
    assert.ok(visible('sm-section-single-mode'));
    assert.ok(!visible('sm-section-folder-mode'));

    w.setChatMode('sm', 'folder');
    assert.ok(visible('sm-section-folder-mode'));
    assert.ok(!visible('sm-section-single-mode'));
    assert.equal(d.getElementById('sm-chat-mode-folder').checked, true);

    assert.equal(d.getElementById('bg-chat-mode-single').checked, true);
    w.setChatMode('bg', 'eva_chat_flow');
    assert.ok(visible('bg-section-evachat-mode'));
    assert.equal(d.getElementById('bg-chat-mode-evachat').checked, true);

    w.setChatMode('bg', 'api_chat');
    assert.ok(visible('bg-section-api-mode'));
    assert.ok(!visible('bg-section-evachat-mode'));
    assert.equal(d.getElementById('bg-chat-mode-api').checked, true);
    assert.equal(d.getElementById('bg-section-single-mode').style.display, 'none');

    w.setChatMode('sm', 'api_chat');
    assert.ok(visible('sm-section-api-mode'));
    assert.equal(d.getElementById('sm-chat-mode-api').checked, true);
  } finally { dom.window.close(); }
});

test('Show Mode live log renders SMS, rules, settings and literal text', async () => {
  const entries = [
    { seq: 1, time: '14:03:21', kind: 'in', text: 'hey what up', detail: 'partner · pending=1' },
    { seq: 2, time: '14:03:23', kind: 'out', text: 'hi', rule: 'EVA · greeting', detail: 'opener · no delay', setting_ok: true },
    { seq: 3, time: '14:03:44', kind: 'out', text: 'f 20 u?', rule: 'EVA · age_gender', detail: 'reply #2 · delay 3.2s (setting 3.0s–6.0s)', setting_ok: true },
    { seq: 4, time: '14:04:10', kind: 'skip', text: 'skipped conversation', detail: 'no-reply auto-skip · waited 12.0s (setting 10.0s–20.0s)' },
    { seq: 5, time: '14:04:11', kind: 'error', text: 'SEND UNCONFIRMED', detail: 'inspect the page' },
    { seq: 6, time: '14:04:12', kind: 'in', text: '<img src=x onerror=alert(1)>', detail: 'partner' },
  ];
  const { dom, w, d } = setup({ GetShowModeLog: async () => ({ version: 6, entries }) });
  try {
    await w.refreshShowLog();
    const rows = [...d.querySelectorAll('#sm-log-list .log-row')];
    assert.equal(rows.length, 6);
    assert.match(d.getElementById('sm-log-list').textContent, /"hey what up"/);
    assert.match(d.getElementById('sm-log-list').textContent, /EVA · age_gender/);
    assert.match(d.getElementById('sm-log-list').textContent, /delay 3\.2s \(setting 3\.0s–6\.0s\)/);
    assert.match(d.getElementById('sm-log-list').textContent, /no-reply auto-skip/);
    assert.equal(d.querySelector('#sm-log-list img, #sm-log-list script'), null);
    assert.match(d.getElementById('sm-log-list').textContent, /<img src=x/);
    assert.equal(d.getElementById('sm-log-empty').hidden, true);

    d.getElementById('sm-log-filter').value = 'sms';
    d.getElementById('sm-log-filter').dispatchEvent(new w.Event('change'));
    assert.equal(d.querySelectorAll('#sm-log-list .log-row').length, 4);

    d.getElementById('sm-log-filter').value = 'errors';
    d.getElementById('sm-log-filter').dispatchEvent(new w.Event('change'));
    assert.equal(d.querySelectorAll('#sm-log-list .log-row').length, 1);
  } finally { dom.window.close(); }
});

test('Background Bot live log renders worker IN/OUT, skips and errors', async () => {
  const entries = [
    { seq: 1, time: '09:00:00', kind: 'system', text: 'bot started', detail: '1 worker(s)' },
    { seq: 2, time: '09:00:05', kind: 'out', text: 'hey there', rule: 'default', detail: '→ traffic bob (2)', setting_ok: true },
    { seq: 3, time: '09:00:08', kind: 'in', text: 'hi', rule: 'default', detail: '← traffic bob (2)' },
    { seq: 4, time: '09:00:09', kind: 'skip', text: 'Skipped reply to 9.', rule: 'default' },
    { seq: 5, time: '09:00:10', kind: 'error', text: 'GET dm conversations failed: 500', rule: 'default' },
  ];
  const { dom, w, d } = setup({ GetBotLiveLog: async () => ({ version: 5, entries }) });
  try {
    await w.refreshBotLog();
    const rows = [...d.querySelectorAll('#bg-log-list .log-row')];
    assert.equal(rows.length, 5);
    assert.match(d.getElementById('bg-log-list').textContent, /"hey there"/);
    assert.match(d.getElementById('bg-log-list').textContent, /→ traffic bob \(2\)/);
    assert.match(d.getElementById('bg-log-list').textContent, /GET dm conversations failed/);
    assert.equal(d.getElementById('bg-log-empty').hidden, true);

    d.getElementById('bg-log-filter').value = 'sms';
    d.getElementById('bg-log-filter').dispatchEvent(new w.Event('change'));
    assert.equal(d.querySelectorAll('#bg-log-list .log-row').length, 2);

    d.getElementById('bg-log-filter').value = 'errors';
    d.getElementById('bg-log-filter').dispatchEvent(new w.Event('change'));
    assert.equal(d.querySelectorAll('#bg-log-list .log-row').length, 1);
  } finally { dom.window.close(); }
});

test('Background Bot log clear resets the in-memory feed only', async () => {
  let cleared = 0;
  const { dom, w, d } = setup({
    ClearBotLiveLog: async () => { cleared++; },
    GetBotLiveLog: async () => ({ version: 1, entries: [{ seq: 1, time: '10:00:00', kind: 'in', text: 'hello', rule: 'default' }] }),
  });
  try {
    await w.refreshBotLog();
    assert.equal(d.querySelectorAll('#bg-log-list .log-row').length, 1);
    d.getElementById('bg-log-clear').click();
    await tick();
    assert.equal(cleared, 1);
    assert.equal(d.querySelectorAll('#bg-log-list .log-row').length, 0);
    assert.equal(d.getElementById('bg-log-empty').hidden, false);
  } finally { dom.window.close(); }
});

test('live log fetches only new entries and advances the sequence', async () => {
  const calls = [];
  const { dom, w, d } = setup({
    GetShowModeLog: async (after) => {
      calls.push(after);
      if (after === 0) {
        return { version: 2, entries: [
          { seq: 1, time: '10:00:00', kind: 'system', text: 'settings applied', detail: 'profile=default' },
          { seq: 2, time: '10:00:01', kind: 'in', text: 'hello', detail: 'partner' },
        ] };
      }
      return { version: 2, entries: [] };
    },
  });
  try {
    await w.refreshShowLog();
    assert.deepEqual(calls, [0]);
    assert.equal(d.querySelectorAll('#sm-log-list .log-row').length, 2);

    await w.refreshShowLog();
    assert.deepEqual(calls, [0, 2]);
    assert.equal(d.querySelectorAll('#sm-log-list .log-row').length, 2);
  } finally { dom.window.close(); }
});

test('Clear removes rendered entries without calling the browser', async () => {
  let cleared = 0;
  const { dom, w, d } = setup({
    ClearShowModeLog: async () => { cleared++; },
    GetShowModeLog: async () => ({ version: 1, entries: [{ seq: 1, time: '10:00:00', kind: 'in', text: 'hello', detail: 'partner' }] }),
  });
  try {
    await w.refreshShowLog();
    d.getElementById('sm-log-clear').click();
    await tick();
    assert.equal(cleared, 1);
    assert.equal(d.querySelectorAll('#sm-log-list .log-row').length, 0);
    assert.equal(d.getElementById('sm-log-empty').hidden, false);
  } finally { dom.window.close(); }
});

test('Test Proxy renders literal per-line results and handles empty input', async () => {
  const calls = [];
  const { dom, w, d } = setup({
    TestProxyEntries: async (raw) => {
      calls.push(raw);
      return [
        { line: 1, label: 'socks5://host:1080', ok: true, status: 200, latency_ms: 12, detail: 'PASS' },
        { line: 2, label: '<img src=x>', ok: false, tunnel_ok: true, status: 403, latency_ms: 9, detail: '<script>bad</script>', provider_message: 'quota <b>exhausted</b>', proxy_authenticate: 'Basic realm="proxy"', hint: 'Try another plan.' },
      ];
    },
  });
  try {
    d.getElementById('sm-proxy-entries').value = '';
    d.getElementById('sm-test-proxy').click();
    await tick();
    assert.equal(calls.length, 0);
    assert.match(d.getElementById('sm-proxy-status').textContent, /Enter at least one proxy/);

    d.getElementById('sm-proxy-entries').value = 'socks5://host:1080';
    d.getElementById('sm-test-proxy').click();
    await new Promise((resolve) => setTimeout(resolve, 20));
    assert.equal(calls.length, 1);
    assert.equal(d.querySelector('#sm-proxy-results img, #sm-proxy-results script'), null);
    assert.match(d.getElementById('sm-proxy-results').textContent, /REACHABLE \/ SITE RESTRICTED.*HTTP 403/);
    assert.match(d.getElementById('sm-proxy-results').textContent, /Provider said: “quota <b>exhausted<\/b>”/);
    assert.equal(d.getElementById('sm-test-proxy').disabled, false);
  } finally { dom.window.close(); }
});

test('profile list marks saved sessions and explains auto-login', async () => {
  const { dom, w, d, snap } = setup({
    ListProfiles: async () => [
      { name: 'default', has_session: true, saved_at: '03:32' },
      { name: 'work', has_session: true, saved_at: '03:35' },
    ],
  });
  try {
    w.fillForms(snap);
    await w.refreshProfiles();
    assert.equal(d.getElementById('sm-profile-default').checked, true);
    assert.equal(d.getElementById('sm-profile-session-choice').checked, false);
    assert.match(d.getElementById('sm-profile-session').textContent, /Fresh browser/);

    d.getElementById('sm-profile-session-choice').checked = true;
    d.getElementById('sm-profile-session-choice').dispatchEvent(new w.Event('change'));
    assert.match(d.getElementById('sm-profile-session').textContent, /Saved default session/);
    assert.ok(d.getElementById('sm-profile-session').classList.contains('status-ok'));

    d.getElementById('sm-profile-default').checked = true;
    d.getElementById('sm-profile-default').dispatchEvent(new w.Event('change'));
    assert.match(d.getElementById('sm-profile-session').textContent, /session is saved for the session option/);
  } finally { dom.window.close(); }
});

test('nested subnav lists the required sections per mode', () => {
  const { dom, d } = setup();
  try {
    const show = [...d.querySelectorAll('#subnav-show .subnav-button')].map((b) => b.dataset.subpanel);
    assert.deepEqual(show, ['show-setting', 'show-proxy', 'show-message', 'show-run', 'show-logs']);
    const bot = [...d.querySelectorAll('#subnav-bot .subnav-button')].map((b) => b.dataset.subpanel);
    assert.deepEqual(bot, ['bg-login', 'bg-proxy', 'bg-message', 'bg-settings', 'bg-logs']);
    assert.equal(d.querySelector('#subnav-show').hidden, false);
    assert.equal(d.querySelector('#subnav-bot').hidden, true);
  } finally { dom.window.close(); }
});

test('sub-pages render one section and keep typed values across switches', () => {
  const { dom, w, d, snap } = setup();
  try {
    w.fillForms(snap);
    w.bindGlobalState();

    d.getElementById('sm-reply-min').value = '42';
    d.getElementById('sm-reply-min').dispatchEvent(new w.Event('input'));
    d.getElementById('sm-proxy-entries').value = 'socks5://kept:1080';
    d.getElementById('sm-proxy-entries').dispatchEvent(new w.Event('input'));

    w.activateSubpanel('show-proxy');
    assert.ok(d.querySelector('[data-subpanel="show-proxy"]').classList.contains('active'));
    assert.ok(!d.querySelector('[data-subpanel="show-setting"]').classList.contains('active'));
    w.activateSubpanel('show-setting');
    assert.equal(d.getElementById('sm-reply-min').value, '42');
    w.activateSubpanel('show-proxy');
    assert.equal(d.getElementById('sm-proxy-entries').value, 'socks5://kept:1080');

    // Background slice keeps its own values independently.
    w.activateTab('bot');
    assert.equal(d.querySelector('#subnav-bot').hidden, false);
    d.getElementById('bg-proxy-entries').value = 'http://bg-kept:8080';
    d.getElementById('bg-proxy-entries').dispatchEvent(new w.Event('input'));
    w.activateSubpanel('bg-message');
    assert.ok(!d.querySelector('[data-subpanel="bg-proxy"]').classList.contains('active'));
    w.activateSubpanel('bg-proxy');
    assert.equal(d.getElementById('bg-proxy-entries').value, 'http://bg-kept:8080');
    // Stats stay outside the sub-pages and remain in the DOM.
    assert.equal(d.getElementById('bg-stat-running').textContent, '0');
  } finally { dom.window.close(); }
});

test('running state disables Run Browser and drives the badges', () => {
  const { dom, w, d } = setup();
  try {
    w.fillForms(snapshot({ show_mode_running: true, running: true }));
    assert.equal(d.getElementById('sm-run').disabled, true);
    assert.equal(d.getElementById('show-run-badge').textContent, 'Running');
    assert.equal(d.getElementById('running-pill').textContent, 'Running');
    assert.equal(d.getElementById('bg-start').disabled, true);
    assert.equal(d.getElementById('bg-stop').disabled, false);

    w.fillForms(snapshot());
    assert.equal(d.getElementById('sm-run').disabled, false);
    assert.equal(d.getElementById('show-run-badge').textContent, 'Idle');
  } finally { dom.window.close(); }
});

test('events update the badges and login status', () => {
  const events = {};
  const { dom, w, d } = setup();
  try {
    w.runtime = { EventsOn: (name, fn) => { events[name] = fn; } };
    w.bindEvents();
    events['ui:show_running'](true);
    assert.equal(d.getElementById('show-run-badge').textContent, 'Running');
    assert.equal(d.getElementById('sm-run').disabled, true);
    events['ui:running'](true);
    assert.equal(d.getElementById('running-pill').textContent, 'Running');
    events['ui:log']({ level: 'info', message: '[default fresh 1/1] Login saved to login_info.json' });
    assert.match(d.getElementById('bg-login-status').textContent, /Login saved/);
  } finally { dom.window.close(); }
});

test('backend failures are forwarded without their arguments', async () => {
  const { dom, w } = setup({ RunBrowser: async () => { throw new Error('synthetic backend failure'); } });
  const messages = [];
  try {
    w.runtime = { LogError: (message) => messages.push(message) };
    await assert.rejects(w.invoke('RunBrowser', { password: 'DO_NOT_LOG_ARGUMENTS' }), /synthetic backend failure/);
    assert.equal(messages.length, 1);
    assert.doesNotMatch(messages[0], /DO_NOT_LOG_ARGUMENTS/);
  } finally { dom.window.close(); }
});

test('Main dashboard has no duplicate API inputs, syncs usernames/modes, and auto-loads API settings from apichat.exe', async () => {
  const savedMessages = [];
  const snapWithApi = snapshot({
    message: {
      chat_mode: 'single',
      sequence: 'hello\nwait',
      usernames: '@u',
      loaded_sequences: [],
      ai_enabled: false,
      ai_base_url: 'https://api.featherless.ai/v1',
      ai_model: 'deepseek-ai/DeepSeek-V3-0324',
      ai_api_key: 'sk-persist-once-123',
      api_temperature: 0.9,
      api_share_hot: 4,
      api_share_warm: 6,
      api_share_cold: 8,
    },
  });
  const { dom, w, d } = setup({
    SaveMessageSettings: async (msg) => { savedMessages.push(msg); },
  }, snapWithApi);
  try {
    w.fillForms(snapWithApi);

    // 1. Main dashboard no longer renders duplicate API Key / Base URL / Model inputs.
    assert.equal(d.getElementById('sm-api-key'), null);
    assert.equal(d.getElementById('bg-api-key'), null);
    assert.equal(d.getElementById('sm-api-base-url'), null);
    assert.equal(d.getElementById('bg-api-base-url'), null);
    assert.match(d.getElementById('sm-api-data-status').textContent, /Auto-loaded from apichat\.exe.*API Key: Ready/);

    // 2. Selecting API Chat and typing usernames in Background Bot syncs to Show Mode and preserves apichat.exe settings on save.
    w.setChatMode('bg', 'api_chat');
    const bgUsers = d.getElementById('bg-usernames');
    bgUsers.value = 'snap_bella_01';
    bgUsers.dispatchEvent(new w.Event('input'));
    assert.equal(d.getElementById('sm-usernames').value, 'snap_bella_01');

    d.getElementById('bg-save-message').click();
    await tick();
    assert.equal(savedMessages.length, 1);
    assert.equal(savedMessages[0].chat_mode, 'api_chat');
    assert.equal(savedMessages[0].usernames, 'snap_bella_01');
    assert.equal(savedMessages[0].ai_api_key, 'sk-persist-once-123');
    assert.equal(d.getElementById('sm-chat-mode-api').checked, true);

    // 3. Main dashboard sidebar no longer has Local Test Chat (moved to apichat.exe).
    assert.equal(d.getElementById('local-test-open'), null);
    assert.equal(d.getElementById('local-test-panel'), null);
  } finally { dom.window.close(); }

  // 4. Multi-Site selector switches isolated sites, filters by account mode, creates new site folders, and triggers Auto-Register vs Browser Account Setup.
  const selectedSites = [];
  const createdSites = [];
  const autoRegCalls = [];
  const browserSetupCalls = [];
  const siteBrowserCalls = [];
  const siteBotCalls = [];
  const multiSnap = snapshot({
    active_site: 'chitchat',
    sites: [
      { id: 'chitchat', name: 'ChitChat.gg', account_mode: 'browser_setup', account_count: 1, has_saved_session: true, start_url: 'https://app.chitchat.gg/text?auth=email', auth_mode: 'chitchat_jwt' },
      { id: 'omezy', name: 'Omezy.io', account_mode: 'auto_register', account_count: 3, has_saved_session: true, start_url: 'https://omezy.io/text/', auth_mode: 'dom_only' },
      { id: 'template_new_site', name: 'Template New Site (Example)', account_mode: 'auto_register', account_count: 0, has_saved_session: false, start_url: 'https://example.com/chat', auth_mode: 'dom_only' },
    ],
  });
  const { dom: mDom, w: mw, d: md } = setup({
    SelectSite: async (id) => {
      selectedSites.push(id);
      return { id, name: 'Template New Site (Example)', start_url: 'https://example.com/chat' };
    },
    CreateSite: async (name, url) => {
      createdSites.push({ name, url });
      return { id: 'uhmegle', name, start_url: url };
    },
    AutoRegisterAccounts: async (siteId, profile, count) => {
      autoRegCalls.push({ siteId, profile, count });
      return [{ id: 'acc1' }, { id: 'acc2' }, { id: 'acc3' }];
    },
    RunBrowserAccountSetup: async (siteId, profile, count) => {
      browserSetupCalls.push({ siteId, profile, count });
    },
    RunSiteBrowser: async (siteId, profile) => {
      siteBrowserCalls.push({ siteId, profile });
    },
    RunSiteBotWithSession: async (siteId, profile) => {
      siteBotCalls.push({ siteId, profile });
    },
    RunSiteDiagnostics: async (siteId) => ({
      site_id: siteId,
      overall_status: 'PASS',
      summary: `[PASS] ${siteId} — 6 PASS, 0 WARN, 0 FAIL`,
      checks: [
        { name: '1. Isolated Folder Structure', status: 'PASS', detail: 'All 7 required files verified' },
        { name: '2. DOM Selectors & Auto-Onboard Driver', status: 'PASS', detail: 'Selectors verified' },
      ],
    }),
  }, multiSnap);
  try {
    mw.fillForms(multiSnap);
    const select = md.getElementById('active-site-select');
    assert.equal(select.options.length, 3);
    assert.equal(select.value, 'chitchat');

    // Filter pills filter dropdown options while keeping active site selectable
    md.getElementById('site-filter-autoreg').click();
    assert.equal(md.getElementById('site-filter-autoreg').classList.contains('active'), true);
    md.getElementById('site-filter-all').click();
    assert.equal(select.options.length, 3);

    select.value = 'template_new_site';
    select.dispatchEvent(new mw.Event('change'));
    await tick();
    assert.deepEqual(selectedSites, ['template_new_site']);

    // Option A: Auto-Register Without Browser
    md.getElementById('sm-autoreg-count').value = '4';
    md.getElementById('sm-autoreg-btn').click();
    await tick();
    assert.deepEqual(autoRegCalls, [{ siteId: 'template_new_site', profile: 'default', count: 4 }]);
    assert.match(md.getElementById('sm-autoreg-status').textContent, /Saved 3 account\(s\) without browser/);

    // Option B: First Browser Run (Account Setup)
    md.getElementById('sm-browser-setup-count').value = '2';
    md.getElementById('sm-browser-setup-btn').click();
    await tick();
    assert.deepEqual(browserSetupCalls, [{ siteId: 'template_new_site', profile: 'default', count: 2 }]);
    assert.match(md.getElementById('sm-browser-setup-status').textContent, /Browser launched/);

    // Deep Debug button runs 6-step diagnostics and renders results
    md.getElementById('site-run-debug').click();
    await tick();
    assert.equal(md.getElementById('site-debug-box').hidden, false);
    assert.match(md.getElementById('site-debug-summary').textContent, /\[PASS\] template_new_site/);
    assert.equal(md.querySelectorAll('#site-debug-checks .site-debug-row').length, 2);

    // Per-site unified list has Browser (Auto-Save Session) and Run Bot (Saved Session) buttons next to each site
    assert.equal(md.querySelectorAll('#sm-site-session-list .site-session-row').length, 3);
    md.querySelector('#sm-site-session-list [data-site-browser="omezy"]').click();
    await tick();
    assert.deepEqual(siteBrowserCalls, [{ siteId: 'omezy', profile: 'default' }]);
    assert.match(md.getElementById('sm-run-status').textContent, /sites\/omezy\/sessions/);

    md.querySelector('#sm-site-session-list [data-site-bot="omezy"]').click();
    await tick();
    assert.deepEqual(siteBotCalls, [{ siteId: 'omezy', profile: 'default' }]);
    assert.match(md.getElementById('sm-run-status').textContent, /Bot running for sites\/omezy\//);

    md.getElementById('site-toggle-new').click();
    assert.equal(md.getElementById('site-new-form').hidden, false);
    md.getElementById('site-new-name').value = 'Uhmegle';
    md.getElementById('site-new-url').value = 'https://uhmegle.com/text';
    md.getElementById('site-create-submit').click();
    await tick();
    assert.deepEqual(createdSites, [{ name: 'Uhmegle', url: 'https://uhmegle.com/text' }]);
  } finally { mDom.window.close(); }

  // 3. Standalone apichat.exe UI loads persona.txt, saves/syncs, and runs Live Test Chat.
  const repoRoot = path.join(__dirname, '..');
  const studioHtml = fs.readFileSync(path.join(repoRoot, 'cmd/apichat/ui/index.html'), 'utf8');
  const studioJs = fs.readFileSync(path.join(repoRoot, 'cmd/apichat/ui/app.js'), 'utf8');
  const savedPayloads = [];
  const startedPayloads = [];
  const sentMessages = [];
  const studioDom = new JSDOM(studioHtml, {
    url: 'http://wails.localhost/',
    runScripts: 'outside-only',
    pretendToBeVisual: true,
  });
  const sw = studioDom.window;
  const sd = sw.document;
  sw.go = {
    main: {
      StudioApp: {
        GetStudioState: async () => ({
          data_dir: 'data',
          settings: {
            chat_mode: 'api_chat',
            ai_base_url: 'https://api.featherless.ai/v1',
            ai_model: 'deepseek-ai/DeepSeek-V3-0324',
            ai_api_key: 'sk-persist-once-123',
            api_temperature: 0.9,
            api_share_hot: 4,
            api_share_warm: 6,
            api_share_cold: 8,
            usernames: 'snap_bella',
          },
          deck: {
            persona: 'you are {name}, a {age} year old girl from {country}',
            sharesnap: 'add me on snap %username%',
            fallback: 'lol my wifi lagged',
            countries: 'usa\nuk',
          },
        }),
        SaveStudioState: async (req) => {
          savedPayloads.push(req);
          return { settings: req.settings, deck: req.deck };
        },
        StartTestChat: async (req) => {
          startedPayloads.push(req);
          return {
            lines: ['heyy wyd tonight'],
            tag: 'apichat',
            reason: 'reply 1 · opener',
            ended: false,
            state: {
              mode: 'api_chat',
              mode_label: 'API Chat',
              persona_name: 'emma',
              persona_age: 21,
              persona_country: 'usa',
              rendered_persona: 'you are emma, a 21 year old girl from usa',
              turn: 1,
              tier: 'COLD',
              score: 0,
              style: 'chill',
              asked_snap: false,
              shared: false,
            },
          };
        },
        SendTestChatMessage: async (text) => {
          sentMessages.push(text);
          return {
            lines: ['heyy hows ur night going'],
            tag: 'apichat',
            reason: 'reply 2 · greeting',
            rule_why: 'greeting',
            mood: 'friendly',
            instruction: 'they said hi: greet back warmly and ask how their day or night is going. one short line.',
            user_kind: 'greeting',
            raw_output: 'heyy hows ur night going',
            diagnostic: '',
            ended: false,
            state: {
              mode: 'api_chat',
              mode_label: 'API Chat',
              persona_name: 'emma',
              persona_age: 21,
              persona_country: 'usa',
              rendered_persona: 'you are emma, a 21 year old girl from usa',
              turn: 2,
              tier: 'COLD',
              score: 0,
              style: 'chill',
              asked_snap: false,
              shared: false,
              known_facts: 'm 22, from usa',
              never_repeat: ['pure greeting'],
            },
          };
        },
      },
    },
  };
  try {
    sw.eval(studioJs);
    sd.dispatchEvent(new sw.Event('DOMContentLoaded'));
    await new Promise((resolve) => setTimeout(resolve, 20));

    assert.equal(sd.getElementById('studio-api-key').value, 'sk-persist-once-123');
    assert.match(sd.getElementById('studio-persona').value, /\{country\}/);

    // Edit persona directly in the single Persona Prompt box and click Save Persona & Test Chat
    sd.getElementById('studio-persona').value = 'custom persona {name} {age} {country} greets warmly';
    sd.getElementById('studio-apply-persona').click();
    await new Promise((resolve) => setTimeout(resolve, 25));

    assert.equal(startedPayloads.length, 1);
    assert.match(startedPayloads[0].deck.persona, /custom persona/);
    assert.match(sd.getElementById('studio-chat-log').textContent, /heyy wyd tonight/);
    assert.match(sd.getElementById('studio-rendered-persona').textContent, /you are emma, a 21 year old girl from usa/);

    // Send "hi" greeting via quick button and verify both Chat Log and Logic & Prompt Diagnostic Logs
    sd.querySelector('.quick-msg[data-msg="hi"]').click();
    await new Promise((resolve) => setTimeout(resolve, 25));
    assert.deepEqual(sentMessages, ['hi']);
    assert.match(sd.getElementById('studio-chat-log').textContent, /heyy hows ur night going/);
    assert.match(sd.getElementById('studio-logic-logs').textContent, /Logic Rule: greeting/);
    assert.match(sd.getElementById('studio-logic-logs').textContent, /Detected as \[greeting\]/);
    assert.match(sd.getElementById('studio-logic-logs').textContent, /they said hi: greet back warmly/);
  } finally { studioDom.window.close(); }
});

