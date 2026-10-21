const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const {JSDOM}=require('jsdom');
const root=path.join(__dirname,'..');
const read=p=>fs.readFileSync(path.join(root,p),'utf8');
const ui=read('cmd/chitchatui/frontend/dist/app.js');
const go=read('app.go');
const goAll=go+read('show_log.go')+read('bot_log.go');
const bindings=read('cmd/chitchatui/frontend/wailsjs/go/main/App.js');
const dts=read('cmd/chitchatui/frontend/wailsjs/go/main/App.d.ts');

test('every literal UI backend invocation has a public Go method',()=>{
 const invoked=[...ui.matchAll(/invoke\("(\w+)"/g)].map(m=>m[1]);
 assert.ok(invoked.includes('RunBrowser'));
 assert.ok(invoked.includes('GetShowModeLog'));
 assert.ok(invoked.includes('ClearShowModeLog'));
 for(const method of invoked) assert.match(goAll,new RegExp(`func \\(a \\*App\\) ${method}\\(`),method);
});

test('Show Mode Run saves its own stores and launches show_mode in one click',()=>{
 assert.match(ui,/invoke\("SaveShowModeSettings", show\)/);
 assert.match(ui,/invoke\("SaveProxySettings", proxy\)/);
 assert.match(ui,/invoke\("SaveMessageSettings", message\)/);
 assert.match(ui,/invoke\("RunBrowser",\s*\{\s*profile,\s*backend:\s*"show_mode",\s*headless:\s*false[^}]*\},\s*1\)/);
 assert.match(go,/func \(a \*App\) RunBrowser\(settings model\.BrowserSettings, count int\) error/);
 assert.match(go,/if settings\.Backend == "show_mode" \{\s*settings\.Backend = "rod"\s*settings\.Headless = false/);
 assert.match(bindings,/RunBrowser\(arg1, arg2\)[\s\S]*?\['RunBrowser'\]\(arg1, arg2\)/);
 assert.match(dts,/RunBrowser\(arg1:model\.BrowserSettings,\s*arg2:number\):Promise<void>/);
 assert.match(bindings,/SaveShowModeSettings\(arg1\)/);
});

test('Show Mode exposes a bounded, rule-aware live log feed',()=>{ assert.match(read('show_log.go'),/const showLogCapacity = 500/);
 assert.match(goAll,/func \(a \*App\) GetShowModeLog\(/);
 assert.match(goAll,/func \(a \*App\) ClearShowModeLog\(\)/);
 assert.match(read('live_chat.go'),/type liveReply struct/);
 assert.match(read('live_chat.go'),/eventf\s+func\(kind, text, rule, detail string, settingOK bool\)/);
 assert.match(read('live_chat.go'),/r\.emit\("out", line, reply\.Rule, detail, ok\)/);
 assert.match(read('live_chat.go'),/r\.emit\("in", row\.Text/);
 assert.match(read('live_chat.go'),/r\.emit\("skip", "skipped conversation"/);
 assert.match(bindings,/GetShowModeLog\(arg1\)/);
 assert.match(dts,/GetShowModeLog\(arg1:number\):Promise<main\.ShowLogSnapshot>/);
 assert.match(read('internal/model/ui.go'),/ShowModeRunning bool\s+`json:"show_mode_running"`/);
});

test('Background Bot exposes its own bounded live log feed',()=>{
 assert.match(read('bot_log.go'),/const botLogCapacity = 500/);
 assert.match(goAll,/func \(a \*App\) GetBotLiveLog\(/);
 assert.match(goAll,/func \(a \*App\) ClearBotLiveLog\(\)/);
 assert.match(goAll,/func \(a \*App\) botWorkerActivity\(/);
 assert.match(goAll,/func \(a \*App\) botWorkerLog\(/);
 assert.match(go,/a\.botWorkerActivity\(profileName, message\)/);
 assert.match(go,/a\.botWorkerLog\(profileName, message\)/);
 assert.match(ui,/invoke\("GetBotLiveLog"/);
 assert.match(ui,/invoke\("ClearBotLiveLog"/);
 assert.match(bindings,/GetBotLiveLog\(arg1\)/);
 assert.match(dts,/GetBotLiveLog\(arg1:number\):Promise<main\.BotLogSnapshot>/);
});

test('saved browser profiles are listed and restored server-side',()=>{
 assert.match(goAll,/func \(a \*App\) ListProfiles\(\)/);
 assert.match(read('internal/browser/backend.go'),/type SessionSeeder interface/);
 assert.match(read('internal/browser/rod.go'),/func \(b \*RodBackend\) SeedSession\(/);
 assert.match(ui,/invoke\("ListProfiles"\)/);
 assert.match(bindings,/ListProfiles\(\)/);
 assert.match(goAll,/func \(a \*App\) restoreProfileSession\(/);
 assert.doesNotMatch(bindings,/Cookies\[|SeedSession/); // cookies stay server-side
});

test('the two timing profiles keep independent database keys',()=>{
 assert.match(read('internal/uistore/store.go'),/keyGeneral\s*=\s*"settings\/general"/);
 assert.match(read('internal/uistore/show_mode.go'),/keyShowMode\s*=\s*"settings\/show_mode\/v1"/);
 assert.match(ui,/invoke\("SaveShowModeSettings"/);
 assert.match(ui,/invoke\("SaveGeneralSettings"/);
});

test('all literal and templated DOM IDs exist, and HTML IDs are unique',()=>{
 const dom=new JSDOM(read('cmd/chitchatui/frontend/dist/index.html'));
 const doc=dom.window.document;
 const ids=[...doc.querySelectorAll('[id]')].map(n=>n.id);
 assert.equal(new Set(ids).size,ids.length,'duplicate HTML IDs');
 for(const m of ui.matchAll(/byId\("([^"]+)"\)/g)) assert.ok(ids.includes(m[1]),`missing #${m[1]}`);
 for(const prefix of ['sm','bg']){
  for(const suffix of ['profile','profile-session','first-mode','first-percent','first-wait','first-min','first-max','first-percent-row','first-wait-row','first-timeout-row','chat-mode-single','chat-mode-folder','chat-mode-evachat','chat-mode-api','mode-option-single','mode-option-folder','mode-option-evachat','mode-option-api','section-single-mode','section-folder-mode','section-evachat-mode','section-api-mode','sequence','usernames','api-data-status','loaded-status','eva-status','load-folder','clean-loaded','open-eva','open-api','proxy-enabled','proxy-scheme','proxy-entries','test-proxy','proxy-status','proxy-results']){
   assert.ok(ids.includes(`${prefix}-${suffix}`),`missing #${prefix}-${suffix}`);
  }
 }
 for(const id of ['sm-run','sm-run-status','sm-log-list','sm-log-filter','sm-log-clear','bg-profile-options','bg-start','bg-stop','bg-status','bg-login-status','bg-log-list','bg-log-filter','bg-log-clear','bg-log-empty','bg-poll-min','bg-poll-max','bg-action-min','bg-action-max','bg-skip-min','bg-skip-max','bg-close-min','bg-close-max','bg-reply-min','bg-reply-max']) assert.ok(ids.includes(id),`missing #${id}`);
 dom.window.close();
});

test('advertised drivers match factory and Show Mode resolves to visible Rod',()=>{
 const models=read('internal/model/ui.go').split('func DefaultBrowserOptions()')[1].split('func DefaultBrowserSettings()')[0];
 const offered=[...models.matchAll(/Key:\s*"([^"]+)"/g)].map(m=>m[1]).sort();
 const available=[...read('internal/browser/backend.go').matchAll(/case "([^"]+)":/g)].map(m=>m[1]).sort();
 assert.deepEqual(offered.filter(key=>key!=='show_mode'),available);
 assert.ok(offered.includes('show_mode'));
});

test('live browser integration test is opt-in, never part of the default suite',()=>{
 assert.ok(read('internal/browser/rod_proxy_auth_test.go').startsWith('//go:build browser_live\n'));
});

test('desktop startup records logs before UI startup even without a GUI console',()=>{
 const main=read('main.go');
 assert.match(main,/log\.SetOutput\(io\.MultiWriter\(applog\.Logger\{[^}]+\}, os\.Stderr\)\)/);
 assert.ok(main.indexOf('log.SetOutput(')<main.indexOf('NewApp(dataDir)'));
 assert.match(main,/Logger:\s+diskLogger/);
 const logFn=go.slice(go.indexOf('func (a *App) log('));
 assert.ok(logFn.indexOf('disk.Log(level, message)')<logFn.indexOf('a.uiStore.AppendLog(level, message)'));
 assert.match(logFn,/disk\.Error\(fmt\.Sprintf\("UI log persistence failed/);
});

test('process capture, crash hook and per-copy WebView2 profile are desktop-startup only',()=>{
 const main=read('main.go'),capture=read('internal/applog/capture.go');
 assert.ok(main.indexOf('applog.CaptureOutput(')<main.indexOf('NewApp(dataDir)'));
 assert.match(main,/capture\.Close\(\)/);
 assert.match(capture,/debug\.SetCrashOutput\(crash, debug\.CrashOptions\{\}\)/);
 assert.match(capture,/os\.Stdout = outWrite/);
 assert.match(capture,/os\.Stderr = errWrite/);
 assert.match(main,/LogLevelProduction:\s*logger\.INFO/);
 assert.match(main,/WebviewUserDataPath:\s+webviewDir/);
 assert.match(main,/filepath\.Join\(dataDir, "webview"\)/);
});

test('API Chat is a separate fourth mode, and EVAChat Flow is pure TXT',()=>{
 const dom=new JSDOM(read('cmd/chitchatui/frontend/dist/index.html'));
 const doc=dom.window.document;
 for(const prefix of ['sm','bg']){
  assert.ok(doc.getElementById(`${prefix}-chat-mode-api`),`missing ${prefix} API mode`);
  assert.ok(doc.getElementById(`${prefix}-section-api-mode`),`missing ${prefix} API panel`);
  assert.equal(doc.getElementById(`${prefix}-ai-enabled`),null,'EVAChat Flow must not carry an AI block anymore');
 }
 dom.window.close();
 assert.match(read('internal/model/ui.go'),/ChatModeAPI\s*=\s*"api_chat"/);
 assert.match(read('app.go'),/func \(a \*App\) TestAPIChat\(/);
 assert.match(read('live_chat.go'),/type apiLiveResponder/);
 assert.match(read('internal/bot/logic.go'),/func executeMatchStatefulChat\(/);
 assert.match(read('internal/bot/api_chat.go'),/func apiChatReply\(/);
});

test('API Chat ships editable persona/share/fallback decks and never writes the username',()=>{
 for(const f of ['README.txt','persona.txt','persona_countries.txt','share.txt','fallback.txt']){
  assert.ok(fs.existsSync(path.join(root,'internal/bot/api_assets',f)),`missing api_assets/${f}`);
 }
 assert.match(read('internal/bot/api_assets/share.txt'),/%username%/);
 assert.match(read('internal/bot/api_assets/persona.txt'),/\{country\}/);
 assert.match(read('internal/bot/api_chat.go'),/getOrAssignUsername/);
 assert.match(read('internal/apichat/prompt.go'),/never write the @ sign/);
 assert.match(read('internal/bot/api_assets/README.txt'),/never overwritten/);
});

test('API Chat quality layer: understanding, ledger, guard and style are wired',()=>{
 for(const f of ['understanding.go','ledger.go','repeat.go','guard.go','style.go','goal.go']){
  assert.ok(fs.existsSync(path.join(root,'internal/apichat',f)),`missing apichat/${f}`);
 }
 const engine=read('internal/apichat/engine.go');
 const director=read('internal/apichat/director.go');
 const guard=read('internal/apichat/guard.go');
 const prompt=read('internal/apichat/prompt.go');
 assert.match(engine,/validateReply\(/);
 assert.match(engine,/safeReply\(/);
 assert.match(engine,/UsedShareLines/);
 assert.match(director,/SnapDodges/);
 assert.match(director,/observeInterest\(/);
 assert.match(director,/d\.Ledger\.MarkAsked/);
 assert.match(guard,/repeatThreshold\s*=\s*0\.62/);
 assert.match(guard,/hasAnchor\(/);
 assert.match(guard,/askedFacts\(/);
 assert.match(prompt,/never ask about/);
 assert.match(prompt,/stage: %s/);
 assert.match(read('internal/apichat/signals.go'),/stage|tierCold/);
});

test('API Chat sharesnap deck is clean, playful and keeps the username in Go',()=>{
 const deck=read('internal/bot/api_assets/sharesnap.txt').split(/\r?\n/).filter(l=>l.trim()&&!l.trim().startsWith('#'));
 assert.ok(deck.length>=10,`too few share lines: ${deck.length}`);
 for(const line of deck){
  assert.match(line,/%username%/,'every share line needs the placeholder');
  assert.doesNotMatch(line,/@/,'share deck must never contain @ handles');
  assert.doesNotMatch(line,/\b(sex|nude|naked|horny|dick|pussy|fuck)\b/i,'share deck must stay clean');
 }
 assert.match(read('internal/bot/api_chat.go'),/sharesnap\.txt/);
 assert.match(read('internal/bot/api_chat.go'),/share\.txt/);
 assert.match(read('cmd/chitchatui/frontend/dist/index.html'),/Every line is written by the model/);
});

test('local test.bat covers all four dashboard Message options, offline',()=>{
 assert.ok(fs.existsSync(path.join(root,'..','test.bat')),'missing root test.bat');
 const rootBat=read('../test.bat');
 assert.match(rootBat,/call test\.bat/);
 assert.match(rootBat,/source_code/);

 const bat=read('test.bat');
 assert.match(bat,/cmd[\\/]chitchattest/);
 assert.match(bat,/build\\bin\\chitchattest\.exe/);
 assert.match(bat,/pause/);
 assert.match(bat,/chcp 65001/);

 const menu=read('internal/bot/localchat.go');
 for(const label of ['Single Sequence','Folder Sequences','EVAChat Flow','API Chat']){
  assert.match(menu,new RegExp(label),`local menu is missing ${label}`);
  assert.ok(read('cmd/chitchatui/frontend/dist/index.html').includes(label),`dashboard is missing ${label}`);
 }
 for(const symbol of ['statefulChatStart(','statefulChatReply(','renderMsg(','getActiveSequence(','LoadAPIChatConfig(','EnsureEVAChatData(','LoadEVAChatEngine(']){
  assert.ok(menu.includes(symbol),`local tester does not reuse ${symbol}`);
 }
 assert.match(menu,/This chat has ended|this chat has ended/i);
 assert.match(menu,/assignedUsernames|AssignedUsernames/);
 assert.doesNotMatch(menu,/SaveJSON|bot_state/,'the local tester must not write bot state');

 assert.match(read('cmd/chitchattest/main.go'),/bot\.RunLocalChat\(/);
 assert.match(read('cmd/chitchattest/main.go'),/LoadMessageSettings\(\)/);
 assert.match(read('cmd/chitchattest/main.go'),/Close the running dashboard first/);
});
