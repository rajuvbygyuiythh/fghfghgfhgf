export namespace main {
	
	export class ProfileOption {
	    name: string;
	    has_session: boolean;
	    saved_at?: string;
	
	    static createFrom(source: any = {}) {
	        return new ProfileOption(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.has_session = source["has_session"];
	        this.saved_at = source["saved_at"];
	    }
	}
	export class ShowLogEntry {
	    seq: number;
	    time: string;
	    kind: string;
	    text?: string;
	    rule?: string;
	    detail?: string;
	    level?: string;
	    setting_ok: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ShowLogEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.seq = source["seq"];
	        this.time = source["time"];
	        this.kind = source["kind"];
	        this.text = source["text"];
	        this.rule = source["rule"];
	        this.detail = source["detail"];
	        this.level = source["level"];
	        this.setting_ok = source["setting_ok"];
	    }
	}
	export class ShowLogSnapshot {
	    version: number;
	    entries: ShowLogEntry[];
	    dropped: number;
	
	    static createFrom(source: any = {}) {
	        return new ShowLogSnapshot(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.entries = this.convertValues(source["entries"], ShowLogEntry);
	        this.dropped = source["dropped"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class UnifiedSettingsPayload {
	    first_message: model.FirstMessageSettings;
	    reply_delay_min_seconds: number;
	    reply_delay_max_seconds: number;
	    auto_skip: boolean;
	    no_reply_min_seconds: number;
	    no_reply_max_seconds: number;
	    next_chat_min_seconds: number;
	    next_chat_max_seconds: number;
	    poll_min_seconds: number;
	    poll_max_seconds: number;
	    action_min_seconds: number;
	    action_max_seconds: number;
	
	    static createFrom(source: any = {}) {
	        return new UnifiedSettingsPayload(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.first_message = this.convertValues(source["first_message"], model.FirstMessageSettings);
	        this.reply_delay_min_seconds = source["reply_delay_min_seconds"];
	        this.reply_delay_max_seconds = source["reply_delay_max_seconds"];
	        this.auto_skip = source["auto_skip"];
	        this.no_reply_min_seconds = source["no_reply_min_seconds"];
	        this.no_reply_max_seconds = source["no_reply_max_seconds"];
	        this.next_chat_min_seconds = source["next_chat_min_seconds"];
	        this.next_chat_max_seconds = source["next_chat_max_seconds"];
	        this.poll_min_seconds = source["poll_min_seconds"];
	        this.poll_max_seconds = source["poll_max_seconds"];
	        this.action_min_seconds = source["action_min_seconds"];
	        this.action_max_seconds = source["action_max_seconds"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class homeChatMessage {
	    id: string;
	    role: string;
	    text: string;
	    order: number;
	
	    static createFrom(source: any = {}) {
	        return new homeChatMessage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.role = source["role"];
	        this.text = source["text"];
	        this.order = source["order"];
	    }
	}
	export class homeChatSessionInfo {
	    id: string;
	    label: string;
	    active: boolean;
	    dropped: number;
	
	    static createFrom(source: any = {}) {
	        return new homeChatSessionInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.label = source["label"];
	        this.active = source["active"];
	        this.dropped = source["dropped"];
	    }
	}
	export class homeChatSnapshot {
	    version: number;
	    selected: string;
	    sessions: homeChatSessionInfo[];
	    messages: homeChatMessage[];
	    evicted: number;
	
	    static createFrom(source: any = {}) {
	        return new homeChatSnapshot(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.selected = source["selected"];
	        this.sessions = this.convertValues(source["sessions"], homeChatSessionInfo);
	        this.messages = this.convertValues(source["messages"], homeChatMessage);
	        this.evicted = source["evicted"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace model {
	
	export class BrowserOption {
	    key: string;
	    label: string;
	    description: string;
	
	    static createFrom(source: any = {}) {
	        return new BrowserOption(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.label = source["label"];
	        this.description = source["description"];
	    }
	}
	export class BrowserSettings {
	    profile: string;
	    backend: string;
	    headless: boolean;
	    use_saved_session: boolean;
	    live_chat?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new BrowserSettings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.profile = source["profile"];
	        this.backend = source["backend"];
	        this.headless = source["headless"];
	        this.use_saved_session = source["use_saved_session"];
	        this.live_chat = source["live_chat"];
	    }
	}
	export class FirstMessageSettings {
	    mode: string;
	    bot_first_percent: number;
	    wait_timeout: boolean;
	    wait_min_seconds: number;
	    wait_max_seconds: number;
	
	    static createFrom(source: any = {}) {
	        return new FirstMessageSettings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.mode = source["mode"];
	        this.bot_first_percent = source["bot_first_percent"];
	        this.wait_timeout = source["wait_timeout"];
	        this.wait_min_seconds = source["wait_min_seconds"];
	        this.wait_max_seconds = source["wait_max_seconds"];
	    }
	}
	export class GeneralSettings {
	    first_message: FirstMessageSettings;
	    poll_min_seconds: number;
	    poll_max_seconds: number;
	    action_min_seconds: number;
	    action_max_seconds: number;
	    match_reply_timeout_min_seconds: number;
	    match_reply_timeout_max_seconds: number;
	    match_close_wait_min_seconds: number;
	    match_close_wait_max_seconds: number;
	    reply_delay_min_seconds: number;
	    reply_delay_max_seconds: number;
	
	    static createFrom(source: any = {}) {
	        return new GeneralSettings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.first_message = this.convertValues(source["first_message"], FirstMessageSettings);
	        this.poll_min_seconds = source["poll_min_seconds"];
	        this.poll_max_seconds = source["poll_max_seconds"];
	        this.action_min_seconds = source["action_min_seconds"];
	        this.action_max_seconds = source["action_max_seconds"];
	        this.match_reply_timeout_min_seconds = source["match_reply_timeout_min_seconds"];
	        this.match_reply_timeout_max_seconds = source["match_reply_timeout_max_seconds"];
	        this.match_close_wait_min_seconds = source["match_close_wait_min_seconds"];
	        this.match_close_wait_max_seconds = source["match_close_wait_max_seconds"];
	        this.reply_delay_min_seconds = source["reply_delay_min_seconds"];
	        this.reply_delay_max_seconds = source["reply_delay_max_seconds"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class HomeStats {
	    bot_accounts: number;
	    running_accounts: number;
	    user_send_count: number;
	    dead_accounts: number;
	    traffic_users: number;
	    messages_sent: number;
	    snap_tg_sent_count: number;
	    skipped_accounts: number;
	
	    static createFrom(source: any = {}) {
	        return new HomeStats(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.bot_accounts = source["bot_accounts"];
	        this.running_accounts = source["running_accounts"];
	        this.user_send_count = source["user_send_count"];
	        this.dead_accounts = source["dead_accounts"];
	        this.traffic_users = source["traffic_users"];
	        this.messages_sent = source["messages_sent"];
	        this.snap_tg_sent_count = source["snap_tg_sent_count"];
	        this.skipped_accounts = source["skipped_accounts"];
	    }
	}
	export class LogEntry {
	    time: string;
	    level: string;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new LogEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.time = source["time"];
	        this.level = source["level"];
	        this.message = source["message"];
	    }
	}
	export class MessageSettings {
	    chat_mode: string;
	    sequence: string;
	    usernames: string;
	    first_message?: string;
	    reply_message?: string;
	    loaded_sequences?: string[];
	    use_loaded_sequences: boolean;
	    keyword_rules_content?: string;
	    keyword_file_path?: string;
	    ai_enabled?: boolean;
	    ai_base_url?: string;
	    ai_api_key?: string;
	    ai_model?: string;
	    api_temperature?: number;
	    api_share_hot?: number;
	    api_share_warm?: number;
	    api_share_cold?: number;
	
	    static createFrom(source: any = {}) {
	        return new MessageSettings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.chat_mode = source["chat_mode"];
	        this.sequence = source["sequence"];
	        this.usernames = source["usernames"];
	        this.first_message = source["first_message"];
	        this.reply_message = source["reply_message"];
	        this.loaded_sequences = source["loaded_sequences"];
	        this.use_loaded_sequences = source["use_loaded_sequences"];
	        this.keyword_rules_content = source["keyword_rules_content"];
	        this.keyword_file_path = source["keyword_file_path"];
	        this.ai_enabled = source["ai_enabled"];
	        this.ai_base_url = source["ai_base_url"];
	        this.ai_api_key = source["ai_api_key"];
	        this.ai_model = source["ai_model"];
        this.api_temperature = source["api_temperature"];
        this.api_share_hot = source["api_share_hot"];
        this.api_share_warm = source["api_share_warm"];
        this.api_share_cold = source["api_share_cold"];
	    }
	}
	export class ProxySettings {
	    enabled: boolean;
	    entries: string;
	    address?: string;
	    username?: string;
	    password?: string;
	
	    static createFrom(source: any = {}) {
	        return new ProxySettings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.entries = source["entries"];
	        this.address = source["address"];
	        this.username = source["username"];
	        this.password = source["password"];
	    }
	}
	export class ShowModeSettings {
	    first_message: FirstMessageSettings;
	    reply_delay_min_seconds: number;
	    reply_delay_max_seconds: number;
	    auto_skip: boolean;
	    no_reply_min_seconds: number;
	    no_reply_max_seconds: number;
	    next_chat_min_seconds: number;
	    next_chat_max_seconds: number;
	
	    static createFrom(source: any = {}) {
	        return new ShowModeSettings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.first_message = this.convertValues(source["first_message"], FirstMessageSettings);
	        this.reply_delay_min_seconds = source["reply_delay_min_seconds"];
	        this.reply_delay_max_seconds = source["reply_delay_max_seconds"];
	        this.auto_skip = source["auto_skip"];
	        this.no_reply_min_seconds = source["no_reply_min_seconds"];
	        this.no_reply_max_seconds = source["no_reply_max_seconds"];
	        this.next_chat_min_seconds = source["next_chat_min_seconds"];
	        this.next_chat_max_seconds = source["next_chat_max_seconds"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class UISnapshot {
	    running: boolean;
	    show_mode_running: boolean;
	    home: HomeStats;
	    browser: BrowserSettings;
	    message: MessageSettings;
	    proxy: ProxySettings;
	    general: GeneralSettings;
	    show_mode: ShowModeSettings;
	    browser_options: BrowserOption[];
	    logs: LogEntry[];
	
	    static createFrom(source: any = {}) {
	        return new UISnapshot(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.running = source["running"];
	        this.show_mode_running = source["show_mode_running"];
	        this.home = this.convertValues(source["home"], HomeStats);
	        this.browser = this.convertValues(source["browser"], BrowserSettings);
	        this.message = this.convertValues(source["message"], MessageSettings);
	        this.proxy = this.convertValues(source["proxy"], ProxySettings);
	        this.general = this.convertValues(source["general"], GeneralSettings);
	        this.show_mode = this.convertValues(source["show_mode"], ShowModeSettings);
	        this.browser_options = this.convertValues(source["browser_options"], BrowserOption);
	        this.logs = this.convertValues(source["logs"], LogEntry);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace proxy {
	
	export class TestResult {
	    line: number;
	    label: string;
	    ok: boolean;
	    tunnel_ok: boolean;
	    status: number;
	    latency_ms: number;
	    detail: string;
	    hint?: string;
	    provider_message?: string;
	    proxy_authenticate?: string;
	    authenticated: boolean;
	
	    static createFrom(source: any = {}) {
	        return new TestResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.line = source["line"];
	        this.label = source["label"];
	        this.ok = source["ok"];
	        this.tunnel_ok = source["tunnel_ok"];
	        this.status = source["status"];
	        this.latency_ms = source["latency_ms"];
	        this.detail = source["detail"];
	        this.hint = source["hint"];
	        this.provider_message = source["provider_message"];
	        this.proxy_authenticate = source["proxy_authenticate"];
	        this.authenticated = source["authenticated"];
	    }
	}

}

