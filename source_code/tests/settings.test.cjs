// Two-page settings + one-click orchestration.
const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { JSDOM } = require('jsdom');

const assets = path.join(__dirname, '../cmd/chitchatui/frontend/dist');
const html = fs.readFileSync(path.join(assets, 'index.html'), 'utf8');
const appJs = fs.readFileSync(path.join(assets, 'app.js'), 'utf8');
const plain = (v) => JSON.parse(JSON.stringify(v));

function snapshot(over = {}) {
  return {
    running: false,
    show_mode_running: false,
    home: { bot_accounts: 1, running_accounts: 0, messages_sent: 6, traffic_users: 2, snap_tg_sent_count: 0, dead_accounts: 0 },
    browser: { profile: 'default', backend: 'rod', headless: false, use_saved_session: false },
    message: { chat_mode: 'single', sequence: 'hello\nwait', usernames: '@u', loaded_sequences: [], ai_enabled: false, ai_base_url: '', ai_model: '', ai_api_key: '' },
    proxy: { enabled: false, entries: '' },
    general: {
      first_message: { mode: 'user', bot_first_percent: 40, wait_timeout: true, wait_min_seconds: 10, wait_max_seconds: 20 },
      poll_min_seconds: 21, poll_max_seconds: 46,
      action_min_seconds: 22, action_max_seconds: 47,
      match_reply_timeout_min_seconds: 61, match_reply_timeout_max_seconds: 90,
      match_close_wait_min_seconds: 3, match_close_wait_max_seconds: 7,
      reply_delay_min_seconds: 7, reply_delay_max_seconds: 9,
    },
    show_mode: {
      first_message: { mode: 'random', bot_first_percent: 55, wait_timeout: true, wait_min_seconds: 11, wait_max_seconds: 22 },
      reply_delay_min_seconds: 4, reply_delay_max_seconds: 8,
      auto_skip: true, no_reply_min_seconds: 40, no_reply_max_seconds: 90,
      next_chat_min_seconds: 1, next_chat_max_seconds: 5,
    },
    browser_options: [],
    logs: [],
    ...over,
  };
}

function setup(snap = snapshot(), api = {}) {
  const dom = new JSDOM(html, { runScripts: 'outside-only', url: 'http://localhost/' });
  const w = dom.window;
  w.addEventListener = () => {};
  w.go = { main: { App: { GetInitialState: async () => snap, ...api } } };
  w.eval(appJs);
  w.bindForms();
  w.bindActions();
  return { dom, w, d: w.document, snap };
}

function waitFor(predicate, label) {
  return new Promise((resolve, reject) => {
    const started = Date.now();
    const timer = setInterval(() => {
      if (predicate()) { clearInterval(timer); resolve(); }
      else if (Date.now() - started > 1500) { clearInterval(timer); reject(new Error(`timeout: ${label}`)); }
    }, 10);
  });
}

test('Show Mode settings populate from show_mode only', () => {
  const snap = snapshot();
  const { dom, w, d } = setup(snap);
  try {
    w.fillForms(snap);
    assert.equal(d.getElementById('sm-reply-min').value, '4');
    assert.equal(d.getElementById('sm-reply-max').value, '8');
    assert.equal(d.getElementById('sm-noreply-min').value, '40');
    assert.equal(d.getElementById('sm-next-max').value, '5');
    assert.equal(d.getElementById('sm-auto-skip').checked, true);
    assert.equal(d.getElementById('sm-first-mode').value, 'random');
    assert.equal(d.getElementById('sm-first-percent').value, '55');
    assert.equal(d.getElementById('sm-first-wait').value, 'skip');
  } finally { dom.window.close(); }
});

test('Background Bot settings populate from general only', () => {
  const snap = snapshot();
  const { dom, w, d } = setup(snap);
  try {
    w.fillForms(snap);
    assert.equal(d.getElementById('bg-poll-min').value, '21');
    assert.equal(d.getElementById('bg-poll-max').value, '46');
    assert.equal(d.getElementById('bg-skip-min').value, '61');
    assert.equal(d.getElementById('bg-close-max').value, '7');
    assert.equal(d.getElementById('bg-reply-max').value, '9');
    assert.equal(d.getElementById('bg-first-mode').value, 'user');
    assert.equal(d.getElementById('bg-first-percent').value, '40');
    assert.notEqual(d.getElementById('bg-reply-max').value, d.getElementById('sm-reply-max').value);
  } finally { dom.window.close(); }
});

test('Show Mode validation blocks bad values before any save or launch', async () => {
  const calls = [];
  const { dom, w, d, snap } = setup(snapshot(), {
    SaveShowModeSettings: async (p) => calls.push(['save', plain(p)]),
    RunBrowser: async (...a) => calls.push(['run', plain(a)]),
  });
  try {
    w.fillForms(snap);
    d.getElementById('sm-reply-min').value = '10';
    d.getElementById('sm-reply-max').value = '6';
    d.getElementById('sm-run').click();
    await waitFor(() => d.getElementById('sm-run-status').textContent.includes('Reply delay'), 'reply error');
    assert.deepEqual(calls, []);

    d.getElementById('sm-reply-min').value = '4';
    d.getElementById('sm-auto-skip').checked = true;
    d.getElementById('sm-noreply-min').value = '0';
    d.getElementById('sm-run').click();
    await waitFor(() => d.getElementById('sm-run-status').textContent.includes('Auto skip'), 'auto skip error');
    assert.deepEqual(calls, []);
  } finally { dom.window.close(); }
});

test('Run Browser saves Setting, Proxy, Message, then launches show_mode', async () => {
  const calls = [];
  const { dom, w, d, snap } = setup(snapshot(), {
    SaveShowModeSettings: async (p) => calls.push(['show', plain(p)]),
    SaveProxySettings: async (p) => calls.push(['proxy', plain(p)]),
    SaveMessageSettings: async (p) => calls.push(['message', plain(p)]),
    SaveBrowserSettings: async (p) => calls.push(['browser', plain(p)]),
    RunBrowser: async (...a) => calls.push(['run', plain(a)]),
  });
  try {
    w.fillForms(snap);
    d.getElementById('sm-proxy-enabled').checked = true;
    d.getElementById('sm-proxy-entries').value = 'socks5://h:1080:u:p';
    d.getElementById('sm-sequence').value = 'line one\nline two';
    d.getElementById('sm-run').click();
    await waitFor(() => calls.some((c) => c[0] === 'run'), 'run call');

    assert.deepEqual(calls.map((c) => c[0]), ['show', 'proxy', 'message', 'browser', 'run']);
    assert.equal(calls[0][1].reply_delay_min_seconds, 4);
    assert.equal(calls[0][1].auto_skip, true);
    assert.equal('poll_min_seconds' in calls[0][1], false);
    assert.deepEqual(calls[1][1], { enabled: true, entries: 'socks5://h:1080:u:p' });
    assert.equal(calls[2][1].sequence, 'line one\nline two');
    assert.deepEqual(calls[3][1], { profile: 'default', backend: 'rod', headless: false, use_saved_session: false });
    assert.deepEqual(calls[4][1], [{ profile: 'default', backend: 'show_mode', headless: false, use_saved_session: false }, 1]);
    assert.match(d.getElementById('sm-run-status').textContent, /Browser requested/);
  } finally { dom.window.close(); }
});

test('Folder mode without loaded sequences cannot be saved or launched', async () => {
  const calls = [];
  const snap = snapshot({ message: { ...snapshot().message, chat_mode: 'folder', loaded_sequences: [] } });
  const { dom, w, d } = setup(snap, {
    SaveShowModeSettings: async () => calls.push('show'),
    SaveProxySettings: async () => calls.push('proxy'),
    SaveMessageSettings: async () => calls.push('message'),
    RunBrowser: async () => calls.push('run'),
  });
  try {
    w.fillForms(snap);
    d.getElementById('sm-run').click();
    await waitFor(() => d.getElementById('sm-run-status').textContent.includes('Load a folder'), 'folder error');
    assert.deepEqual(calls, []);
  } finally { dom.window.close(); }
});

test('Session login mode is saved and passed to the run', async () => {
  const calls = [];
  const { dom, w, d, snap } = setup(snapshot(), {
    ListProfiles: async () => [{ name: 'default', has_session: true, saved_at: '03:32' }],
    SaveShowModeSettings: async () => {},
    SaveProxySettings: async () => {},
    SaveMessageSettings: async () => {},
    SaveBrowserSettings: async (p) => calls.push(['browser', plain(p)]),
    RunBrowser: async (...a) => calls.push(['run', plain(a)]),
  });
  try {
    w.fillForms(snap);
    await w.refreshProfiles();
    const sessionRadio = d.getElementById('sm-profile-session-choice');
    sessionRadio.checked = true;
    sessionRadio.dispatchEvent(new w.Event('change'));
    d.getElementById('sm-run').click();
    await waitFor(() => calls.some((c) => c[0] === 'run'), 'run call');
    assert.equal(calls[0][1].profile, 'default');
    assert.equal(calls[0][1].use_saved_session, true);
    assert.equal(calls[1][1][0].profile, 'default');
    assert.equal(calls[1][1][0].use_saved_session, true);
  } finally { dom.window.close(); }
});

test('Save & Start Bot saves every store in order, then starts workers', async () => {
  const calls = [];
  const { dom, w, d, snap } = setup(snapshot(), {
    SaveBrowserSettings: async (p) => calls.push(['browser', plain(p)]),
    SaveProxySettings: async (p) => calls.push(['proxy', plain(p)]),
    SaveMessageSettings: async (p) => calls.push(['message', plain(p)]),
    SaveGeneralSettings: async (p) => calls.push(['general', plain(p)]),
    StartBot: async () => calls.push(['start']),
  });
  try {
    w.fillForms(snap);
    d.getElementById('bg-profile').value = 'worker';
    d.getElementById('bg-backend').value = 'chromedp';
    d.getElementById('bg-headless').checked = true;
    d.getElementById('bg-start').click();
    await waitFor(() => calls.some((c) => c[0] === 'start'), 'start call');

    assert.deepEqual(calls.map((c) => c[0]), ['browser', 'proxy', 'message', 'general', 'start']);
    assert.deepEqual(calls[0][1], { profile: 'worker', backend: 'chromedp', headless: true, use_saved_session: false });
    assert.equal(calls[3][1].poll_min_seconds, 21);
    assert.equal(calls[3][1].reply_delay_max_seconds, 9);
    assert.equal(calls[3][1].first_message.mode, 'user');
  } finally { dom.window.close(); }
});

test('Background validation blocks poll 0 and bad ranges', async () => {
  const calls = [];
  const { dom, w, d, snap } = setup(snapshot(), { StartBot: async () => calls.push('start') });
  try {
    w.fillForms(snap);
    d.getElementById('bg-poll-min').value = '0';
    d.getElementById('bg-start').click();
    await waitFor(() => d.getElementById('bg-status').textContent.includes('Poll'), 'poll error');
    assert.deepEqual(calls, []);
  } finally { dom.window.close(); }
});

test('run failures are visible on the Show Mode page', async () => {
  const { dom, w, d, snap } = setup(snapshot(), {
    SaveShowModeSettings: async () => { throw new Error('disk unavailable'); },
  });
  try {
    w.fillForms(snap);
    d.getElementById('sm-run').click();
    await waitFor(() => d.getElementById('sm-run-status').textContent.includes('disk unavailable'), 'failure');
    assert.ok(d.getElementById('sm-run-status').classList.contains('status-error'));
  } finally { dom.window.close(); }
});
