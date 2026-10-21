# Local test chat — `test.bat`

এই ফিচারটা দিয়ে আপনি **নিজেই ইউজার হয়ে** বটের সাথে চ্যাট করতে পারবেন —
রিয়েল অ্যাপে কিছু পাঠানো ছাড়াই, কোনো ব্রাউজার ছাড়াই। ড্যাশবোর্ডের
**Message** পেজে যে **৪টা চ্যাট অপশন** আছে, `test.bat` খুললে ঠিক সেই ৪টাই
মেনু হিসেবে আসে। আপনি যেটা বেছে নেবেন, বট **সেই মোডেই** উত্তর দেবে — আর
সেটিংস পড়বে আপনার ড্যাশবোর্ডে সেভ করা `data/` থেকেই।

---

## ১. কীভাবে চালাবেন

1. **ড্যাশবোর্ড বন্ধ করুন** (সেটিংস ডেটাবেস একসাথে দুই প্রসেসে খোলে না)।
   মেসেজ সেটিংস আগে ড্যাশবোর্ডে একবার **Save** করা থাকতে হবে।
2. প্রজেক্ট ফোল্ডারের **`test.bat`** ডাবল ক্লিক করুন।
3. প্রথমবার Go দিয়ে ছোট একটা কনসোল টুল বিল্ড হবে
   (`build\bin\chitchattest.exe`) — কয়েক সেকেন্ড।
4. মেনু থেকে **১–৪** টাইপ করে Enter দিন, তারপর সাধারণ চ্যাটের মতো লিখুন।

সরাসরি মোডও দেওয়া যায়:

```
test.bat 1     Single Sequence
test.bat 2     Folder Sequences
test.bat 3     EVAChat Flow
test.bat 4     API Chat
```

> Go না থাকলে `install.bat` একবার চালান — এটাই Go ইনস্টল করে। এরপর
> `test.bat` চলবে। আগে একবার বিল্ড হয়ে থাকলে Go ছাড়াও পুরোনো
> `chitchattest.exe` চলবে।

## ২. চারটা মোড

| মেনু | ড্যাশবোর্ডের নাম | কী টেস্ট হয় |
|---|---|---|
| 1 | **Single Sequence** | Message → Sequence-এর লাইনগুলো একটার পর একটা (প্রতি ইউজার মেসেজে পরের লাইন) |
| 2 | **Folder Sequences** | Load Folder করা সিকোয়েন্সগুলো থেকে প্রতিটা চ্যাটে একটা র‍্যান্ডম সিকোয়েন্স |
| 3 | **EVAChat Flow** | `data/eva_chat/`-এর TXT পুল, স্টেজ মেশিন, snap ask/share |
| 4 | **API Chat** | `data/api_chat/` persona + ডিরেক্টর + মডেল (API key ড্যাশবোর্ড থেকে) |

### মোড অনুযায়ী যা মনে রাখবেন

* **API Chat (4):** `Test API` আগে ড্যাশবোর্ডে একবার চালিয়ে নিন। key না থাকলে
  `test.bat` পরিষ্কারভাবে বলবে কী করতে হবে।
* **EVAChat (3):** প্রথমবার `data/eva_chat/` সিড হবে (ড্যাশবোর্ডের মতোই)।
* **Folder (2):** কিছু লোড না থাকলে বটের আসল fallback অনুযায়ী Single
  Sequence-এ চলে যাবে, আর স্ক্রিনে নোট দেখাবে।
* **Snap username:** ড্যাশবোর্ডে **Snap / Telegram users**-এ হ্যান্ডেল না
  দিলে শেয়ার লাইন বাদ যাবে — ঠিক আসল বটের মতো। একটা করে হ্যান্ডেল দিলে
  প্রতি চ্যাটে একটা অ্যাসাইন হবে, `%username%` বসে যাবে।

## ৩. কমান্ড

| কমান্ড | কাজ |
|---|---|
| `/new` | নতুন কনভার্সেশন (API Chat-এ নতুন persona/style) |
| `/state` | বট এখন কী জানে/মনে রাখে — known facts, tier, stage, snap অবস্থা |
| `/help` | কমান্ডের তালিকা |
| `/quit` | বেরিয়ে যাওয়া |

## ৪. যা **হয় না**

* রিয়েল অ্যাপে কোনো মেসেজ যায় না, ব্রাউজার খোলে না।
* `data/bot_state.json` বা `flagged_users.json`-এ কিছু লেখা হয় না।
* ড্যাশবোর্ডের সেটিংস বদলায় না — শুধু পড়া হয়।
* শুধু `data/api_chat` / `data/eva_chat`-এ **অনুপস্থিত** স্টার্টার ফাইল
  থাকলে সেগুলো সিড হয় (ড্যাশবোর্ডও এটাই করে)।

## ৫. ট্রাবলশুটিং

| সমস্যা | সমাধান |
|---|---|
| `Could not read the dashboard Message settings` | ড্যাশবোর্ড বন্ধ করুন (সেটিংস DB লক থাকে), তারপর আবার চালান। |
| `Go was not found` | একবার `install.bat` চালান। |
| `Build failed` | উপরের Go error-টা পাঠান; বাকি অ্যাপের বিল্ড ঠিক থাকলে এটাও ঠিক হয়ে যাবে। |
| `API Chat is not ready: ... API key` | ড্যাশবোর্ড → 3 · Message → API Chat → key দিন → Save/Test API। |
| `no message sequence is configured` | ড্যাশবোর্ড → 3 · Message → Sequence লিখে Save করুন। |
| মেনুতে ফিরে যেতে চাই | `test.bat` বন্ধ করে আবার খুলুন, বা `/quit` দিয়ে বেরিয়ে আবার চালান। |

## ৬. ডেভেলপার নোট

| ফাইল | কাজ |
|---|---|
| `test.bat` (root) | `source_code\test.bat`-এ পাঠায় |
| `source_code/test.bat` | Go চেক → `go build .\cmd\chitchattest` → চালায় → pause |
| `cmd/chitchattest/main.go` | ড্যাশবোর্ডের `data/` থেকে Message settings পড়ে, কনসোল চালায় |
| `internal/bot/localchat.go` | মেনু + চ্যাট লুপ; **একই** `statefulChatStart/Reply`, `renderMsg`, `getActiveSequence`, `LoadAPIChatConfig`, `LoadEVAChatEngine` ব্যবহার করে |
| `internal/bot/localchat_test.go` | ৪ মোডের মেনু, username render, bot-state না লেখা, API-key hint |

বিল্ড কমান্ড (হাতে চালাতে চাইলে):

```
cd source_code
go build -o build\bin\chitchattest.exe .\cmd\chitchattest
build\bin\chitchattest.exe
```
