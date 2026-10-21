package sites

import "strings"

// ArchetypeSelectors returns the universal family-tuned CSS selector bundle
// for each of the 5 stranger-chat engine families discovered across the web.
func ArchetypeSelectors(family string) Selectors {
	switch strings.ToLower(strings.TrimSpace(family)) {
	case "family_b":
		// Hybrid Text/Video Roulette platforms (StrangerLine, Thundr, TalkRand, Louuz, Emerald, etc.)
		return Selectors{
			ComposerInput:          `textarea:not([disabled]), input[type="text"][placeholder*="message" i]:not([disabled]), input[type="text"][placeholder*="chat" i]:not([disabled]), input[placeholder*="type" i]:not([disabled]), [contenteditable="true"]`,
			SendButton:             `button[type="submit"], button[aria-label*="Send" i], button.send-btn, button.chat-send`,
			SkipButton:             `button[aria-keyshortcuts="Escape"], button.skip-btn, button.next-btn, button[aria-label*="Skip" i], button[aria-label*="Next" i], button[aria-label*="Stop" i]`,
			StartButton:            `button.start-btn, button.new-chat, button[aria-label*="Start" i], button.connect-btn`,
			MessageHistory:         `.chat-messages, .message-list, .messages-container, .chat-log, .logbox, [role="log"], main`,
			MessageRow:             `.message-row, .message-item, .chat-message, .logitem, .msg-row, li[data-message-id], .message`,
			MessageText:            `.message-text, .msg-content, .text, p, span`,
			PartnerMessageSelector: `.stranger, .partner, .incoming, .them, .other, .left, [data-author="partner"], [data-sender="stranger"]`,
			OwnMessageSelector:     `.you, .self, .outgoing, .me, .own, .right, [data-author="self"], [data-sender="me"]`,
			PartnerName:            `.partner-name, .stranger-name, .chat-header-name`,
			ChatEndedText:          `stranger has disconnected|partner disconnected|chat ended|left the chat|looking for a stranger`,
		}
	case "family_c":
		// Guest Nickname / Gender Form -> 1v1 Private DM & Random Match (Chatib, Chatix, Chatiw, Y99, TWS, ChatDesh, etc.)
		return Selectors{
			ComposerInput:          `textarea:not([disabled]), input[type="text"][placeholder*="message" i]:not([disabled]), input[type="text"][placeholder*="write" i]:not([disabled]), input[type="text"][placeholder*="type" i]:not([disabled]), #msg-input, #chat-input`,
			SendButton:             `button[type="submit"], button.send, button#send-btn, button[aria-label*="Send" i], .send-button`,
			SkipButton:             `button.skip, button.next, button.leave-chat, button[aria-label*="Close" i], button[aria-label*="Skip" i]`,
			StartButton:            `button[type="submit"], button.start-chat, button.enter-chat, a.start-chat, button.random-chat`,
			MessageHistory:         `.private-chat-messages, .dm-messages, .chat-messages, .message-area, .chat-history, .logbox, main`,
			MessageRow:             `.msg, .message, .chat-line, .dm-row, .message-row, li`,
			MessageText:            `.msg-text, .message-body, .text, p, span`,
			PartnerMessageSelector: `.incoming, .received, .from-them, .other-msg, .strangermsg, .left`,
			OwnMessageSelector:     `.outgoing, .sent, .from-me, .my-msg, .youmsg, .right`,
			PartnerName:            `.dm-partner-name, .active-chat-user, .stranger-name, .partner-name`,
			ChatEndedText:          `user is offline|conversation ended|stranger disconnected|has left the room`,
		}
	case "family_d":
		// Classic Anonymous Networks, Roleplay & Topic/DM Platforms (ChatBlink, ChatAvenue, E-Chat, MeroChat, Rolechat, etc.)
		return Selectors{
			ComposerInput:          `textarea:not([disabled]), input#content:not([disabled]), input#message_content:not([disabled]), input[type="text"][placeholder*="message" i]:not([disabled]), input[type="text"][placeholder*="say" i]:not([disabled])`,
			SendButton:             `button#submit_button, button[type="submit"], button.send_btn, input[type="submit"]`,
			SkipButton:             `button.disconnect, button.next-stranger, button.skip-btn, a.leave`,
			StartButton:            `button.start-chat, button#start, input[type="submit"], button.connect`,
			MessageHistory:         `#chat_logs_container, .chat-logs, .messages, .logbox, #messages, main`,
			MessageRow:             `.chat_log, .logitem, .message-row, .msg-item, li`,
			MessageText:            `.my_text, .chat_message, .message-text, p, span`,
			PartnerMessageSelector: `.other_user, .strangermsg, .partner, .incoming, .them`,
			OwnMessageSelector:     `.me_user, .youmsg, .self, .outgoing, .mine`,
			PartnerName:            `.private_target, .partner-name, .stranger-name`,
			ChatEndedText:          `disconnected|ended the chat|left the conversation`,
		}
	case "family_e":
		// Indie, Open-Source & Regional Specialized Platforms (StrangerSpace, ThXChat, GoTalks, Knotchat, etc.)
		return Selectors{
			ComposerInput:          `textarea:not([disabled]), input[type="text"]:not([disabled])[placeholder*="message" i], input[type="text"]:not([disabled])[placeholder*="type" i], #messageInput, #chatInput`,
			SendButton:             `button#sendBtn, button[type="submit"], button.send-btn, button[aria-label*="Send" i]`,
			SkipButton:             `button#skipBtn, button#nextBtn, button.skip-btn, button.next-btn, button[aria-keyshortcuts="Escape"]`,
			StartButton:            `button#startBtn, button.start-btn, button.new-chat, button[type="submit"]`,
			MessageHistory:         `#messages, .messages, .chat-box, .chat-messages, .logbox, main`,
			MessageRow:             `.message, .msg, .chat-msg, .logitem, li`,
			MessageText:            `.text, .msg-text, p, span`,
			PartnerMessageSelector: `.stranger, .incoming, .partner, .other, .left`,
			OwnMessageSelector:     `.you, .outgoing, .self, .me, .right`,
			PartnerName:            `.stranger-name, .partner-name`,
			ChatEndedText:          `stranger has disconnected|partner left|chat ended|disconnected`,
		}
	default:
		// Family A: Pure 1-on-1 Instant Text Roulette (Omezy, FunChatX, Chatrio, OpenTalk, HiStranger, StrangerTalk, etc.)
		return Selectors{
			ComposerInput:          `textarea:not([disabled]), input[type="text"][placeholder*="message" i]:not([disabled]), input[type="text"][placeholder*="type" i]:not([disabled]), input[type="text"][placeholder*="say" i]:not([disabled]), [contenteditable="true"]`,
			SendButton:             `button[type="submit"], button[aria-label*="Send" i], button.send-btn, button.send`,
			SkipButton:             `button[aria-keyshortcuts="Escape"], button.skip-btn, button.stop-btn, button.next-btn, button[aria-label*="Skip" i], button[aria-label*="Stop" i], button[aria-label*="Leave" i]`,
			StartButton:            `button.start-btn, button.new-chat, button[aria-label*="Start" i], button[type="submit"]`,
			MessageHistory:         `.chat-messages, .message-list, .logbox, .messages, [role="log"], main`,
			MessageRow:             `.message-row, .logitem, .message, .chat-message, .msg-row, li[data-message-id], li`,
			MessageText:            `.message-text, .msg-text, .content, p, span`,
			PartnerMessageSelector: `.strangermsg, .partner-msg, .incoming, .stranger, .them, .other, .left, [data-from="stranger"], [data-author="partner"]`,
			OwnMessageSelector:     `.youmsg, .self-msg, .outgoing, .you, .me, .own, .right, [data-from="you"], [data-author="self"]`,
			PartnerName:            `.partner-name, .stranger-name, .peer-name`,
			ChatEndedText:          `stranger has disconnected|stranger left|chat has ended|you have disconnected|looking for a partner|finding a stranger`,
		}
	}
}

type siteSeed struct {
	ID          string
	Name        string
	Family      string
	AccountMode string
	StartURL    string
	EntryFlow   string
	Protection  string
}

var browserSetupSiteIDs = map[string]bool{
	"chitchat":          true,
	"umingle":           true,
	"omegleweb_com":     true,
	"strangerline_io":   true,
	"strangerline_live": true,
	"thundr_com":        true,
	"emeraldchat":       true,
	"chatrandom_text":   true,
	"camsurf_text":      true,
	"chatspin_text":     true,
	"shagle_text":       true,
	"chatavenue":        true,
	"wireclub_guest":    true,
	"merochat":          true,
	"wusoup":            true,
	"omegleapp_me":      true,
}

// DefaultAccountMode returns whether a site requires a first browser run for
// account/Turnstile setup ("browser_setup") or supports browserless auto
// registration ("auto_register").
func DefaultAccountMode(siteID, authMode string) string {
	id := SanitizeID(siteID)
	if id == DefaultSiteID || strings.EqualFold(strings.TrimSpace(authMode), "chitchat_jwt") {
		return AccountModeBrowserSetup
	}
	if browserSetupSiteIDs[id] {
		return AccountModeBrowserSetup
	}
	return AccountModeAutoRegister
}

func makeSiteConfig(s siteSeed) Config {
	accMode := strings.TrimSpace(s.AccountMode)
	if accMode == "" {
		accMode = DefaultAccountMode(s.ID, "dom_only")
	}
	cfg := Config{
		ID:                s.ID,
		Name:              s.Name,
		Family:            s.Family,
		AccountMode:       accMode,
		Enabled:           true,
		StartURL:          s.StartURL,
		AppURL:            s.StartURL,
		APIBaseURL:        "",
		AuthMode:          "dom_only",
		EntryFlow:         s.EntryFlow,
		ProtectionSummary: s.Protection,
		Selectors:         ArchetypeSelectors(s.Family),
	}
	cfg.Classification = InitialClassification(cfg)
	return cfg
}

// CatalogByID looks up a built-in non-ChitChat site configuration by ID.
func CatalogByID(id string) (Config, bool) {
	id = SanitizeID(id)
	for _, cfg := range CatalogConfigs() {
		if cfg.ID == id {
			return cfg, true
		}
	}
	return Config{}, false
}

// CatalogConfigs returns all built-in non-ChitChat stranger text-chat sites.
func CatalogConfigs() []Config {
	seeds := []siteSeed{
		// =====================================================================
		// FAMILY A: Pure 1-on-1 Instant Text Roulette (Sites #1 - #30)
		// =====================================================================
		{
			ID: "omezy", Name: "Omezy.io (Text Roulette)", Family: "family_a",
			StartURL:   "https://omezy.io/text/",
			EntryFlow:  "Chat anonymously -> 18+ Agree modal -> Start / Esc / Send",
			Protection: "No initial CAPTCHA on text route. Enforces anonymous daily chat allowance cap; clear site storage or rotate profile if daily limit is reached.",
		},
		{
			ID: "funchatx", Name: "FunChatX (Random 1v1 Text)", Family: "family_a",
			StartURL:   "https://funchatx.com/random-chat/",
			EntryFlow:  "Ready to start modal -> Random username -> I am: Female -> 18+ checkbox -> Start Chat ->",
			Protection: "Zero CAPTCHA and zero signup. Shows 'Skip this stranger? -> Yes, skip' confirmation dialog on skip, handled automatically by Auto-Onboard Agent.",
		},
		{
			ID: "chatrio", Name: "Chatrio.app (Anonymous 1v1)", Family: "family_a",
			StartURL:   "https://chatrio.app/chat",
			EntryFlow:  "18+ Yes modal -> Accept cookies -> Your name + I am: Woman -> New Chat",
			Protection: "Zero CAPTCHA, zero account required. Ephemeral WebSocket session with no message retention; pace outbound links to avoid report triggers.",
		},
		{
			ID: "opentalk", Name: "OpenTalk.club (Text Chat)", Family: "family_a",
			StartURL:   "https://opentalk.club/text/",
			EntryFlow:  "Gender modal ('I'm a girl' / 'I'm a guy') -> Start / Space / Leave",
			Protection: "100% free browser-native text roulette with zero signup. AI + human report moderation; keep 2s-4s reply pacing.",
		},
		{
			ID: "opentalk_buzz", Name: "OpenTalk.buzz (Pure Text)", Family: "family_a",
			StartURL:   "https://opentalk.buzz/",
			EntryFlow:  "Direct anonymous text entry -> Start / Next",
			Protection: "Lightweight text-only mirror recommended by Emerald Chat directory. Low rate-limit friction.",
		},
		{
			ID: "strangertalk", Name: "StrangerTalk.live (1v1 Text)", Family: "family_a",
			StartURL:   "https://www.strangertalk.live/",
			EntryFlow:  "I am a: Female -> Optional Interest Tags -> Find a Stranger",
			Protection: "Zero registration, camera-free text chat. Uses <120ms in-memory NLP moderation that blocks raw URLs/commercial spam; share handles naturally on later turns.",
		},
		{
			ID: "histranger", Name: "HiStranger.lol (1v1 Anonymous)", Family: "family_a",
			StartURL:   "https://histranger.lol/en/chat",
			EntryFlow:  "Your name -> 18+ safety checkbox -> Start chatting -> Find a chat partner",
			Protection: "No account, no ads, RAM-only 1-on-1 messages. Instant report button; keep friendly natural conversation flow.",
		},
		{
			ID: "funchat_in", Name: "FunChat.in (Random 1-on-1)", Family: "family_a",
			StartURL:   "https://funchat.in/random-chat",
			EntryFlow:  "Direct 1-on-1 Random Chat -> Find New Stranger",
			Protection: "Explicitly states 'no captcha needed'. Has strict automated word and phone-number filters in chat text; avoid numeric phone strings.",
		},
		{
			ID: "talklive", Name: "TalkLive.app (Terminal Text Chat)", Family: "family_a",
			StartURL:   "https://talklive.app/chat?go=1",
			EntryFlow:  "18+ confirmation -> Tap to Chat (--random --no-mic)",
			Protection: "No signup, no mic/camera. Maintains a rolling 5,000-message server moderation log; avoid sending identical opener spam rapidly.",
		},
		{
			ID: "textchat_online", Name: "TextChat.Online (Encrypted 1v1)", Family: "family_a",
			StartURL:   "https://textchat.online/",
			EntryFlow:  "Optional interest tag -> Click Start Text Chat -> Leave/Switch anytime",
			Protection: "Pure 1v1 text only, zero login, AES-256-GCM browser-side encryption, zero server logs.",
		},
		{
			ID: "yapping", Name: "Yapping.me (Anonymous Text)", Family: "family_a",
			StartURL:   "https://yapping.me/text-chat",
			EntryFlow:  "Auto-assigned guest identity -> Tap Start -> Skip anytime",
			Protection: "No email/phone required. Auto-widens queue when filters are thin; moderated against bot flood.",
		},
		{
			ID: "omefree", Name: "OmeFree.com (Text & Video)", Family: "family_a",
			StartURL:   "https://omefree.com/",
			EntryFlow:  "Select Text Chat -> Click Start -> Leave session",
			Protection: "100% free, no signup, no ads. Uses standard 'Stranger is typing..' / 'Looking for a stranger..' DOM indicators.",
		},
		{
			ID: "strangerchat_online", Name: "StrangerChat.online (1-on-1)", Family: "family_a",
			StartURL:   "https://strangerchat.online/",
			EntryFlow:  "Click Text Chat -> Instant 1-on-1 connection",
			Protection: "No login, no email required. Basic per-IP connection rate limit.",
		},
		{
			ID: "strangerschat_online", Name: "StrangersChat.online (Private Text)", Family: "family_a",
			StartURL:   "https://www.strangerschat.online/",
			EntryFlow:  "Pick interests (optional) -> New connection text only (18+)",
			Protection: "No signup or profile, ephemeral message handling, language-aware matching.",
		},
		{
			ID: "chat_strangers", Name: "Chat-Strangers.com (1v1 Text)", Family: "family_a",
			StartURL:   "https://chat-strangers.com/",
			EntryFlow:  "Click Text Chat -> Instant random stranger match",
			Protection: "No sign-up required. Simple WebSocket pairing with minimal anti-bot friction.",
		},
		{
			ID: "liveglobalchat", Name: "LiveGlobalChat (1v1 Text)", Family: "family_a",
			StartURL:   "https://liveglobalchat.com/random-text-chat.html",
			EntryFlow:  "Country/Gender preference -> Start 1v1 Text Chat",
			Protection: "No camera, no mic, no account required. Browser-native text roulette.",
		},
		{
			ID: "randomstrangerchats", Name: "RandomStrangerChats.com", Family: "family_a",
			StartURL:   "https://www.randomstrangerchats.com/chat",
			EntryFlow:  "Pick a nickname -> Start Text Chat",
			Protection: "100% free, anonymous, no registration required. Supports auto-translated stranger chats.",
		},
		{
			ID: "strangermeetup", Name: "StrangerMeetup.com (1-on-1)", Family: "family_a",
			StartURL:   "https://strangermeetup.com/chat-with-strangers",
			EntryFlow:  "One click Start -> Private 1-on-1 anonymous text room",
			Protection: "No registration or login required. Classic Omegle-style DOM structure.",
		},
		{
			ID: "joingy", Name: "Joingy.com (Text Roulette)", Family: "family_a",
			StartURL:   "https://joingy.com/",
			EntryFlow:  "Free Text Chat section -> Click Start",
			Protection: "Zero signup, purely random pool, reactive-only moderation.",
		},
		{
			ID: "chatsizzle", Name: "ChatSizzle.com (Anonymous 1v1)", Family: "family_a",
			StartURL:   "https://chatsizzle.com/",
			EntryFlow:  "Select language/age/gender/interests -> Get Started",
			Protection: "Free browser-based 1-on-1 text chat; no account, email, or download required.",
		},
		{
			ID: "whisperly", Name: "Whisperly.live (Text Chat)", Family: "family_a",
			StartURL:   "https://www.whisperly.live/text-chat",
			EntryFlow:  "Instant nickname -> Start Text Chat",
			Protection: "No login, email, or camera required. Voice/Text only platform with zero video risk.",
		},
		{
			ID: "sillychat", Name: "Silly.chat (1v1 Text Chat)", Family: "family_a",
			StartURL:   "https://silly.chat/",
			EntryFlow:  "Select 1-on-1 Text Chat -> Press Start",
			Protection: "No sign-up or registration required. AI moderation across sessions.",
		},
		{
			ID: "hearme", Name: "HearMe.chat (18+ Text Chat)", Family: "family_a",
			StartURL:   "https://hearme.chat/",
			EntryFlow:  "Tick 18+ box -> Pick nickname -> Tap Start a text chat",
			Protection: "No signup, no camera, no daily guest cap. Moderated 18+ text and voice platform.",
		},
		{
			ID: "airtalk", Name: "AirTALK.live (Text & Voice)", Family: "family_a",
			StartURL:   "https://www.airtalk.live/",
			EntryFlow:  "Open page -> Start random text/voice chat (no account)",
			Protection: "No email or download required. AI moderation + user reports.",
		},
		{
			ID: "talknmeet", Name: "TalknMeet.com (1-on-1 Text)", Family: "family_a",
			StartURL:   "https://www.talknmeet.com/",
			EntryFlow:  "Open site -> Connect instantly in private 1-on-1 text chat",
			Protection: "No signup required. Private one-on-one WebSocket sessions.",
		},
		{
			ID: "komegle", Name: "Komegle.com (Smart Random Chat)", Family: "family_a",
			StartURL:   "https://komegle.com/",
			EntryFlow:  "No registration -> Click Start -> Smart Match",
			Protection: "Completely free, active moderation, mobile/desktop web compatible.",
		},
		{
			ID: "chaturro", Name: "Chaturro.com (No-Reg Chat)", Family: "family_a",
			StartURL:   "https://chaturro.com/",
			EntryFlow:  "Select Text-Only -> Start (<30 seconds to first conversation)",
			Protection: "True no-registration access with minimal tracking.",
		},
		{
			ID: "umingle", Name: "Umingle / Uhmegle (Text Mode)", Family: "family_a",
			StartURL:   "https://umingle.com/text/",
			EntryFlow:  "Optional interests -> Text mode -> Start / Esc / Stop",
			Protection: "Blocks datacenter/VPN IPs ('VPN Detected'), enforces single-tab lock ('Another Tab Opened'), and applies skip cooldowns on rapid skipping. Use residential IP and single tab.",
		},
		{
			ID: "omegleweb_com", Name: "OmegleWeb.com (Text Chat)", Family: "family_a",
			StartURL:   "https://omegleweb.com/chat",
			EntryFlow:  "18+ confirmation -> New Stranger / Next (Esc) / Send (Enter)",
			Protection: "Periodic Cloudflare Turnstile ('Quick verification') after rapid skipping + karma score ('Stranger's Score'). Avoid rapid consecutive skips.",
		},
		{
			ID: "omegleweb_io", Name: "OmegleWeb.io (Text Chat)", Family: "family_a",
			StartURL:   "https://omegleweb.io/chat",
			EntryFlow:  "18+ -> Start Text Chat -> Esc to skip",
			Protection: "Strict keyword filter ('Do not ask for gender. This is not a dating site. Violators will be banned'). Avoid 'M or F' / 'asl' keywords.",
		},

		// =====================================================================
		// FAMILY B: Modern Next-Gen & Hybrid 1v1 Text/Video Platforms (Sites #31 - #50)
		// =====================================================================
		{
			ID: "strangerline_io", Name: "StrangerLine.io (18+ Chat)", Family: "family_b",
			StartURL:   "https://strangerline.io/chat/",
			EntryFlow:  "Guest display name -> Gender: Female -> Age: 21 -> 18+ checkbox -> Continue -> Start New Chat",
			Protection: "Cloudflare Turnstile widget on initial guest entry; once solved in saved profile session, text matching and reconnect work smoothly.",
		},
		{
			ID: "strangerline_live", Name: "StrangerLine.live (1v1 Chat)", Family: "family_b",
			StartURL:   "https://strangerline.live/",
			EntryFlow:  "18+ Guest Form -> Text Chat -> Start",
			Protection: "Mirror domain of StrangerLine; AI real-time message filter + Turnstile on initial entry.",
		},
		{
			ID: "thundr_com", Name: "Thundr.com (Text & Video)", Family: "family_b",
			StartURL:   "https://thundr.com/talk-with-strangers",
			EntryFlow:  "Choose Text Mode -> Add Interests -> 18+ -> Start Chatting",
			Protection: "Encrypted sessions + AI moderation; keep text-only mode active to avoid camera prompts.",
		},
		{
			ID: "thundr_cc", Name: "Thundr.cc (Anonymous Text)", Family: "family_b",
			StartURL:   "https://www.thundr.cc/",
			EntryFlow:  "Select Text Chat -> Start -> Next",
			Protection: "No sign-up required for basic text chat; AI moderation active.",
		},
		{
			ID: "talkrand", Name: "TalkRand.com (1v1 Text/Video)", Family: "family_b",
			StartURL:   "https://talkrand.com/thundr-alternative",
			EntryFlow:  "Open TalkRand -> Pick Text (no form/email) -> Start",
			Protection: "No login or email required; shows a short ad break every few matches on free tier.",
		},
		{
			ID: "louuz", Name: "Louuz.com (Global Text/Video)", Family: "family_b",
			StartURL:   "https://www.louuz.com/talk-to-strangers",
			EntryFlow:  "Pick Text Mode -> Start Chatting -> Skip anytime",
			Protection: "100% free, zero registration, no old OmeTV bans carried over.",
		},
		{
			ID: "louuz_anime", Name: "Louuz Anime (Otaku Random Chat)", Family: "family_b",
			StartURL:   "https://www.louuz.com/chat/anime",
			EntryFlow:  "Pick Text Mode -> Connect with anime fans",
			Protection: "Zero login, pre-tagged anime/manga stranger queue.",
		},
		{
			ID: "ome_gg", Name: "Ome.gg (Anonymous Random Chat)", Family: "family_b",
			StartURL:   "https://ome.gg/",
			EntryFlow:  "Auto-generated anonymous handle -> Random Chat / Public Rooms",
			Protection: "Zero signup, zero email, free unlimited chats.",
		},
		{
			ID: "korutalk", Name: "KoruTalk.com (Vibe Match Text)", Family: "family_b",
			StartURL:   "https://www.korutalk.com/",
			EntryFlow:  "No account, no camera -> Select vibe -> Start Chat",
			Protection: "Free no-account, no-camera platform with vibe matching.",
		},
		{
			ID: "emeraldchat", Name: "EmeraldChat.com (Text Mode)", Family: "family_b",
			StartURL:   "https://emeraldchat.com/",
			EntryFlow:  "Optional guest/session -> Select Text Mode -> Start",
			Protection: "High anti-bot karma system + frequent CAPTCHA if skipping rapidly; persist session cookies in sites/emeraldchat/profiles/default.",
		},
		{
			ID: "chathub", Name: "ChatHub (Random Text/Cam)", Family: "family_b",
			StartURL:   "https://chathub.cam/",
			EntryFlow:  "Select Language/Text -> Click Start",
			Protection: "No registration required; basic browser fingerprint and rate-limit check.",
		},
		{
			ID: "chatrandom_text", Name: "ChatRandom (Text-Only Mode)", Family: "family_b",
			StartURL:   "https://chatrandom.com/",
			EntryFlow:  "Switch to Text-Only mode -> Click Start",
			Protection: "Fast connect without account; occasional CAPTCHA loop during traffic spikes.",
		},
		{
			ID: "camsurf_text", Name: "CamSurf (Browser Text/Chat)", Family: "family_b",
			StartURL:   "https://camsurf.com/",
			EntryFlow:  "Agree 18+ terms -> Start -> Use in-chat text box",
			Protection: "No account required for basic access; strict anti-link moderation.",
		},
		{
			ID: "chatspin_text", Name: "ChatSpin (Web Text/Video)", Family: "family_b",
			StartURL:   "https://chatspin.com/",
			EntryFlow:  "Accept terms -> Start -> Text chat panel",
			Protection: "No registration required for free trial/basic chat; gender filters paid.",
		},
		{
			ID: "shagle_text", Name: "Shagle (Anonymous Browser Chat)", Family: "family_b",
			StartURL:   "https://shagle.com/",
			EntryFlow:  "Select gender -> Start Chatting -> Text box",
			Protection: "No signup required for basic pairing; country/gender filters require premium.",
		},
		{
			ID: "bazoocam", Name: "Bazoocam.org (Random Chat)", Family: "family_b",
			StartURL:   "https://bazoocam.org/",
			EntryFlow:  "Click Start -> Chat via text or built-in icebreaker games",
			Protection: "No registration required; geolocation-based pairing.",
		},
		{
			ID: "camgo", Name: "Camgo.com (Text & Video)", Family: "family_b",
			StartURL:   "https://camgo.com/",
			EntryFlow:  "Select Text Chat toggle -> Add interests -> Start",
			Protection: "Free no-signup access with 'Safe Search' AI moderation toggle.",
		},
		{
			ID: "lemonchat", Name: "LemonChat.app (Stranger Chat)", Family: "family_b",
			StartURL:   "https://lemonchat.app/stranger-chat",
			EntryFlow:  "Enter nickname -> Pick Text or Video -> Accept terms -> Start",
			Protection: "No email required; free gender filter and instant report/skip.",
		},
		{
			ID: "nightcap", Name: "Nightcap.chat (No-Signup Chat)", Family: "family_b",
			StartURL:   "https://www.nightcap.chat/",
			EntryFlow:  "Pick an interest -> Start talking (no account)",
			Protection: "Zero signup, zero paywalled filters.",
		},
		{
			ID: "chatingly", Name: "Chatingly.com (1v1 Stranger Chat)", Family: "family_b",
			StartURL:   "https://chatingly.com/",
			EntryFlow:  "Auto random nickname -> Add interests -> Start / Next",
			Protection: "Free, no sign-up, 18+ only.",
		},

		// =====================================================================
		// FAMILY C: Guest Nickname / Gender Form -> 1v1 Private DM & Random Match (Sites #51 - #75)
		// =====================================================================
		{
			ID: "chatib", Name: "Chat-ib.com (Anonymous DMs & Match)", Family: "family_c",
			StartURL:   "https://chat-ib.com/",
			EntryFlow:  "Guest Nickname + Age 21 + Gender: Female -> Start Chatting -> Ephemeral Private DMs",
			Protection: "Zero data retention (server RAM only), 100% guest access. AI filter blocks raw external URLs in private DMs; share usernames naturally.",
		},
		{
			ID: "chatib_us", Name: "Chatib.us (Guest Rooms & Private DM)", Family: "family_c",
			StartURL:   "https://www.chatib.us/",
			EntryFlow:  "Nickname + Age + Gender: Female + Country -> Submit -> Incoming Private DM tabs",
			Protection: "9.9M+ monthly visits. Selecting Female triggers immediate incoming 1-on-1 DMs; pace replies (2.5s-5s) to avoid flood rate-limit.",
		},
		{
			ID: "chatib_chat", Name: "Chatib.chat (Global Random & DM)", Family: "family_c",
			StartURL:   "https://chatib.chat/",
			EntryFlow:  "Guest Nickname -> Random Chat or Country Room -> Private 1-on-1",
			Protection: "4,000+ online 24/7, no registration or email required.",
		},
		{
			ID: "chatix", Name: "Chatix.chat (Modern Chatib Rebuild)", Family: "family_c",
			StartURL:   "https://chatix.chat/",
			EntryFlow:  "Pick handle + Age 21 + Gender: Female + Country -> Start (20s guest entry)",
			Protection: "No VIP paywall, no ads inside chat window, automated enforcement with transparent 24h max IP blocks.",
		},
		{
			ID: "chatiw_com", Name: "Chatiw.com (Streamlined Guest Text)", Family: "family_c",
			StartURL:   "https://chatiw.com/",
			EntryFlow:  "Nickname + Age + Gender: Female + Country -> Start Chat",
			Protection: "Zero email confirmation. Known for silent IP bans if spamming identical openers or links rapidly; use varied API/EVA replies.",
		},
		{
			ID: "chatiw_us", Name: "Chatiw.us (US/Global Guest DM)", Family: "family_c",
			StartURL:   "https://chatiw.us/",
			EntryFlow:  "Guest Nickname + Age + Female -> Enter Chat -> Private DM inbox",
			Protection: "3.9M+ monthly visits; fast guest DM delivery with anti-flood pacing.",
		},
		{
			ID: "chatiwi", Name: "Chatiwi.com (Encrypted Guest Chat)", Family: "family_c",
			StartURL:   "https://chatiwi.com/",
			EntryFlow:  "Temporary Guest Nickname -> Enter Room / Private 1-on-1",
			Protection: "Encrypts messages and does not store usernames on disk.",
		},
		{
			ID: "y99_random", Name: "Y99.chat (Stranger 1-on-1 Mode)", Family: "family_c",
			StartURL:   "https://y99.chat/stranger-chat",
			EntryFlow:  "Enter guest nickname -> Click Chatrandom 1-on-1",
			Protection: "No login required; connects two strangers under alias 'Anonymous' or chosen nickname.",
		},
		{
			ID: "y99_in", Name: "Y99.in (Guest Chat & Private DM)", Family: "family_c",
			StartURL:   "https://y99.in/frontend/",
			EntryFlow:  "Enter guest username -> Go to chat / Random 1-on-1",
			Protection: "6.7M+ monthly visits, zero registration required; minimal moderation in private DMs.",
		},
		{
			ID: "tws_random", Name: "TalkWithStranger (Random 1-on-1)", Family: "family_c",
			StartURL:   "https://randomchat.talkwithstranger.com/",
			EntryFlow:  "Pick a nickname -> Start 1-on-1 Random Text Chat",
			Protection: "Dedicated 1-on-1 random chat subdomain of TalkWithStranger; zero registration, AI + 24/7 moderation.",
		},
		{
			ID: "tws_chat", Name: "TalkWithStranger (Web Chat App)", Family: "family_c",
			StartURL:   "https://chat.talkwithstranger.com/",
			EntryFlow:  "Optional nickname -> Join Random Chat or Private 1-1",
			Protection: "No email/phone required; supports group rooms and 1-on-1 private chats.",
		},
		{
			ID: "tws_text", Name: "TalkWithStranger (Text Strangers)", Family: "family_c",
			StartURL:   "https://talkwithstranger.com/free-chat-rooms/text-strangers",
			EntryFlow:  "Enter nickname -> Hit Enter to Free Chat Anonymously",
			Protection: "Direct text-strangers entry page with 1-1 private chat support without logging in.",
		},
		{
			ID: "chatdesh", Name: "ChatDesh.in (Random 1v1 & DMs)", Family: "family_c",
			StartURL:   "https://chatdesh.in/",
			EntryFlow:  "Select Random Chat (no account) or pick username -> ESC to match / Draggable DMs",
			Protection: "Supports English, Bengali, Hindi + 15 regional rooms and Omegle-style 1-on-1 Random Chat. Rate-limiting and real-time content filtering active.",
		},
		{
			ID: "yesichat", Name: "YesIChat.com (1-Click Guest Chat)", Family: "family_c",
			StartURL:   "https://yesichat.com/",
			EntryFlow:  "Enter nickname -> Click 'Enter The Chat ->' -> Private Chat",
			Protection: "No signup or login required for guest rooms and private 1-on-1 chats.",
		},
		{
			ID: "meetyou", Name: "Meetyou.me (Spam-Free Stranger Chat)", Family: "family_c",
			StartURL:   "https://meetyou.me/",
			EntryFlow:  "Click 'Start Chatting ->' (without logging in) -> Private/Disposable room",
			Protection: "Female-friendly stranger platform with strict anti-spam/anti-bot link filters.",
		},
		{
			ID: "chateasily", Name: "Chateasily.com (10s Guest Chat)", Family: "family_c",
			StartURL:   "https://chateasily.com/",
			EntryFlow:  "Pick a nickname (zero signup forms) -> Anonymous Chat / Private Messages",
			Protection: "100% free forever with guest access to all rooms and private messaging.",
		},
		{
			ID: "chatogo", Name: "Chatogo.com (Guest Chat Rooms & DM)", Family: "family_c",
			StartURL:   "https://www.chatogo.com/free-chat-rooms/",
			EntryFlow:  "Select Guest Login -> Pick nickname -> Private/Room chat",
			Protection: "No registration or app download required.",
		},
		{
			ID: "chatkr", Name: "Chatkr.com (Global Guest Private Chat)", Family: "family_c",
			StartURL:   "https://chatkr.com/",
			EntryFlow:  "Select room -> Join Chat -> Select Guest -> Login -> Private Chat",
			Protection: "USA/UK/Canada/India guest rooms with 1-on-1 private chat popups.",
		},
		{
			ID: "viby", Name: "Viby.chat (Guest 1-on-1 Text)", Family: "family_c",
			StartURL:   "https://viby.chat/free-chat-rooms",
			EntryFlow:  "Guest entry -> Private 1-on-1 Text Chat",
			Protection: "Free guest mode allows up to 10 chats per day per browser session; clear profile storage to reset.",
		},
		{
			ID: "buzzchat", Name: "BuzzChat.app (Anonymous Text & DM)", Family: "family_c",
			StartURL:   "https://buzzchat.app/en/sites-like-chatib",
			EntryFlow:  "Nickname-based start -> Instant/Smart Match or DM",
			Protection: "Uses account-health and reputation signals; keep natural reply timing.",
		},
		{
			ID: "secretchatroom", Name: "SecretChatRoom.com (Zero-Reg 1v1)", Family: "family_c",
			StartURL:   "https://secretchatroom.com/free-chat-rooms",
			EntryFlow:  "Open page -> Join 1-on-1 or group chat (no username/email/password needed)",
			Protection: "Zero registration, 100% free.",
		},
		{
			ID: "funchatroom", Name: "FunChatRoom.com (18+ Guest 1-on-1)", Family: "family_c",
			StartURL:   "https://funchatroom.com/",
			EntryFlow:  "Instant Guest Access (<10s) -> Private 1-on-1 Chat",
			Protection: "18+ adult guest chat with ignore/block list.",
		},
		{
			ID: "dinchat", Name: "DinChat.com (No-Membership Chat)", Family: "family_c",
			StartURL:   "https://dinchat.com/",
			EntryFlow:  "Enter as guest without registration -> All chat features unlocked",
			Protection: "Full guest feature access without login.",
		},
		{
			ID: "meetzur", Name: "Meetzur.com (Click & Chat Strangers)", Family: "family_c",
			StartURL:   "https://www.meetzur.com/",
			EntryFlow:  "Click 'Chat Now' -> Instant anonymous random chat",
			Protection: "No login or registration required; lightweight DOM.",
		},
		{
			ID: "chatbridge", Name: "ChatBridge.link (Real-Time Guest Chat)", Family: "family_c",
			StartURL:   "https://chatbridge.link/",
			EntryFlow:  "Join as guest (Name + Age 21 + Female + Country) -> Private/Public chat",
			Protection: "Real-time WebSocket guest chat with zero login requirement.",
		},

		// =====================================================================
		// FAMILY D: Classic Anonymous Networks, Roleplay & Topic/DM Platforms (Sites #76 - #90)
		// =====================================================================
		{
			ID: "chatblink", Name: "ChatBlink.com (Random Roulette & DM)", Family: "family_d",
			StartURL:   "https://www.chatblink.com/",
			EntryFlow:  "18+ Guest Login -> Free Chat Roulette / Private DM",
			Protection: "Long-running international guest chat; enforces anti-spam cooldown on repetitive messages.",
		},
		{
			ID: "chatavenue", Name: "Chat-Avenue.com (Guest Rooms & DM)", Family: "family_d",
			StartURL:   "https://www.chat-avenue.com/",
			EntryFlow:  "Select Room -> Guest Login (Nickname + Age + Gender) -> Private DM window",
			Protection: "CodyChat engine; some rooms require registration while guest rooms allow instant entry + private DMs.",
		},
		{
			ID: "chat321", Name: "321Chat.com (Guest Chat Network)", Family: "family_d",
			StartURL:   "https://www.321chat.com/",
			EntryFlow:  "Nickname + Age + Female -> Enter Chat -> Private 1-on-1",
			Protection: "337K+ monthly visits; partnered with TalkWithStranger guest infrastructure.",
		},
		{
			ID: "wireclub_guest", Name: "Wireclub.com (Private Chat Lobby)", Family: "family_d",
			StartURL:   "https://www.wireclub.com/",
			EntryFlow:  "Guest/Quick entry -> Private Chat Lobby -> Direct Messages",
			Protection: "769K+ monthly visits; moderated rooms with rate limits on new guest sessions.",
		},
		{
			ID: "echat", Name: "E-Chat.co (Zero-Signup DM Chat)", Family: "family_d",
			StartURL:   "https://www.e-chat.co/",
			EntryFlow:  "Enter guest nickname -> Join room -> Click user in sidebar for 1-on-1 DM",
			Protection: "Zero-signup WebSocket chat with persistent DM tab bar.",
		},
		{
			ID: "chatzy", Name: "Chatzy.com (Private Chat Service)", Family: "family_d",
			StartURL:   "https://www.chatzy.com/",
			EntryFlow:  "Enter alias -> Join public/private chat room",
			Protection: "1.2M+ monthly visits; no ads, no installation, no registration required.",
		},
		{
			ID: "merochat", Name: "MeroChat (Friendly 1v1 Text Only)", Family: "family_d",
			StartURL:   "https://mero.chat/",
			EntryFlow:  "Auto-generated profile (name, bio, avatar) -> 1-on-1 Text Chat",
			Protection: "Open-source (AGPL-3.0) text-only platform with community Karma system; strictly platonic conversation rules.",
		},
		{
			ID: "wusoup", Name: "Wusoup.com (Clean 1-on-1 Text)", Family: "family_d",
			StartURL:   "https://www.wusoup.com/",
			EntryFlow:  "Accept 19+ rules -> Short intro -> 1-on-1 Text Chat",
			Protection: "Strictly moderated 1-on-1 text chat; bans low-effort 'hey/asl' spam—use API Chat or thoughtful openers.",
		},
		{
			ID: "rolechat", Name: "Rolechat.org (Anonymous Topic 1v1)", Family: "family_d",
			StartURL:   "https://rolechat.org/",
			EntryFlow:  "Enter changeable username + topic tags -> Start 1-on-1 Chat",
			Protection: "Shamchat-style no-profile 1-on-1 text service.",
		},
		{
			ID: "fourscorechat", Name: "4ScoreChat.com (1v1 Text Improv)", Family: "family_d",
			StartURL:   "https://www.4scorechat.com/",
			EntryFlow:  "Choose character or real self -> Instant 1-on-1 Text Match",
			Protection: "Free browser-based stranger improv text chat.",
		},
		{
			ID: "sevenstrangers", Name: "7Strangers.com (Anonymous 1v1 Text)", Family: "family_d",
			StartURL:   "https://7strangers.com/",
			EntryFlow:  "One click -> 1-on-1 anonymous text chat",
			Protection: "Pure 1-on-1 text chat; stop/skip anytime.",
		},
		{
			ID: "chat42", Name: "Chat42.com (Instant Name -> 1v1)", Family: "family_d",
			StartURL:   "https://chat42.com/",
			EntryFlow:  "Enter your name -> Start instant random text chat",
			Protection: "Native messaging-style web UI with zero registration.",
		},
		{
			ID: "tohla", Name: "Tohla.in (1-on-1 Stranger Text)", Family: "family_d",
			StartURL:   "https://tohla.in/",
			EntryFlow:  "One click -> Paired in 1-on-1 random chat room",
			Protection: "Classic lightweight Omegle-style 1-on-1 text interface.",
		},
		{
			ID: "dixytalk", Name: "DixyTalk.com (Free Stranger Chat)", Family: "family_d",
			StartURL:   "https://dixytalk.com/",
			EntryFlow:  "Guest entry -> Start Free Online Chat with Strangers",
			Protection: "No registration required.",
		},
		{
			ID: "imeetzu", Name: "iMeetzu.com (Random Text & Rooms)", Family: "family_d",
			StartURL:   "https://imeetzu.com/",
			EntryFlow:  "Select Text Chat / Random 1v1 -> Start",
			Protection: "Classic Omegle/Chatroulette text alternative with guest access.",
		},

		// =====================================================================
		// FAMILY E: Indie, Open-Source & Regional Specialized Platforms (Sites #91 - #100)
		// =====================================================================
		{
			ID: "strangerspace", Name: "StrangerSpace.com (Filtered 1v1)", Family: "family_e",
			StartURL:   "https://strangerspace.com/",
			EntryFlow:  "Location/Age/Interest filter -> Start Text Chat (no sign-up)",
			Protection: "No registration needed; supports text, voice notes, and contact saving.",
		},
		{
			ID: "thxchat", Name: "ThXChat.com (23-Lang Translated 1v1)", Family: "family_e",
			StartURL:   "https://thxchat.com/",
			EntryFlow:  "Instant Random Match -> Text Chat (with 23-language AI translation)",
			Protection: "Moderated random matching with collaborative whiteboard and translation.",
		},
		{
			ID: "gotalks", Name: "GoTalks (Anonymous Global Text)", Family: "family_e",
			StartURL:   "https://gotalks.in/",
			EntryFlow:  "Click Start -> Anonymous 1-on-1 Text Conversation",
			Protection: "Web-based random chat SaaS; free and anonymous.",
		},
		{
			ID: "chattingzoo", Name: "ChattingZoo (1-Click Stranger Chat)", Family: "family_e",
			StartURL:   "https://chattingzoo.com/",
			EntryFlow:  "Click and Play -> Start chatting with strangers worldwide",
			Protection: "Minimalist UI with zero registration.",
		},
		{
			ID: "omegleapp_me", Name: "OmegleApp.me (Classic Omegle Clone)", Family: "family_e",
			StartURL:   "https://omegleapp.me/chat",
			EntryFlow:  "Pass initial hCaptcha ('Please verify you're human') -> Text Chat",
			Protection: "Gates initial entry behind hCaptcha; solve once in visible browser and session remains active.",
		},
		{
			ID: "knotchat", Name: "Knot.chat (Bot-Free Text & Video)", Family: "family_e",
			StartURL:   "https://knot.chat/",
			EntryFlow:  "Quick 30s alias setup -> Start with Text Chat",
			Protection: "24/7 AI + human moderation; text stranger chat is a core entry point.",
		},
		{
			ID: "randomtalker", Name: "RandomTalker.com (No-Camera 1v1)", Family: "family_e",
			StartURL:   "https://www.randomtalker.com/anonymous-voice-chat",
			EntryFlow:  "No signup -> Random 1-to-1 matching",
			Protection: "No camera required, free tier available.",
		},
		{
			ID: "chatusa", Name: "ChatUSA.club (Fast Stranger Match)", Family: "family_e",
			StartURL:   "https://www.chatusa.club/stranger-chat",
			EntryFlow:  "Quick guest entry -> Instant Stranger Chat (<3s match)",
			Protection: "AI moderation and mobile-friendly stranger-to-stranger text/video.",
		},
		{
			ID: "chatforfree", Name: "ChatForFree.org (Guest Private Chat)", Family: "family_e",
			StartURL:   "https://chatforfree.org/",
			EntryFlow:  "Pick guest nickname -> Enter -> Private 1-on-1 Chat",
			Protection: "Free guest entry with no registration required.",
		},
		{
			ID: "shkumar_chat", Name: "OpenSource 1v1 Stranger Chat (GitHub)", Family: "family_e",
			StartURL:   "https://shkumaraman.github.io/chat/index.html",
			EntryFlow:  "Instant Anonymous 1v1 -> Connected! Say Hi",
			Protection: "Open-source GitHub Pages + WebSocket 1v1 stranger text chat; zero CAPTCHA, zero login.",
		},
	}

	out := make([]Config, 0, len(seeds))
	for _, s := range seeds {
		out = append(out, makeSiteConfig(s))
	}
	return out
}
