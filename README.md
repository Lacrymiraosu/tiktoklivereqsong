# osureq: TikTok LIVE → osu! beatmap requests

A small Windows helper (one `osureq.exe`, ~8 MB, Go, no installer, no runtime) that lets
TikTok LIVE viewers request osu! beatmaps in chat. The helper:

1. reads chat from **TikFinity Desktop**,
2. checks each request against the streamer's rules,
3. downloads it from a mirror,
4. confirms the `.osu` files match what osu! currently serves (MD5),
5. opens the map in osu! when the streamer presses **Next**, or automatically once **tosu** sees the previous song has ended.

A local dashboard (English/Thai) and a transparent OBS overlay run inside the same exe.

> **This README is also the handoff document.** It is written so that another developer can continue the project
> from it alone. Start with [Project status](#project-status), then [Architecture](#architecture),
> [File structure](#file-structure) and [Rules for contributors](#rules-for-contributors).

### สรุปภาษาไทย

* **คืออะไร:** โปรแกรม Windows ไฟล์เดียว (`osureq.exe`) ให้ผู้ชม TikTok LIVE ขอเพลง osu! ผ่านแชทด้วย `!req <ID หรือลิงก์>`
* **ทำอะไรได้:** รับแชทจาก TikFinity → ตรวจกฎ (ดาว ความยาว โหมด สถานะ แบน คูลดาวน์ ผู้ติดตาม ฯลฯ) → ดาวน์โหลดจาก mirror → ตรวจ MD5 เทียบกับ osu! → เปิดแมพใน osu! เมื่อกด ถัดไป หรือเปิดอัตโนมัติเมื่อ tosu เห็นว่าเพลงก่อนหน้าเล่นจบ
* **เพิ่มเติม:** ป๊อปอัปแจ้งผู้ชมบน overlay, คำสั่งม็อด (`!skip`, `!remove`, `!reqon`/`!reqoff`), เก็บผลการเล่นจาก tosu, ของขวัญได้คิวพิเศษ, ฟังตัวอย่างเพลง/เปิดใน beatmap viewer, ปุ่มแบนคลิกเดียว
* **ต้องมี:** Windows 10/11 x64, osu! (stable หรือ lazer), **TikFinity Desktop** และ **tosu** (เปิดไว้ระหว่างไลฟ์)
* **สำหรับนักพัฒนา:** Go 1.23+ บน Windows, รัน `go test ./...`, build ด้วย `build.ps1`
* **โครงสร้าง:** `main.go` ต่อสายทุกอย่าง, โค้ดหลักอยู่ใน `internal/<โมดูล>` แยกหน้าที่ชัดเจน, หน้าเว็บอยู่ใน `internal/server/web` และถูกฝังลงใน exe
* **คนที่มาทำต่อ:** อ่าน [Rules for contributors](#rules-for-contributors) ก่อนแก้โค้ด และห้ามกด Next บนเครื่องจริงโดยไม่เปิด Dry run

---

## Table of contents

* [Project status](#project-status)
* [Features](#features)
* [Requirements](#requirements)
* [Quick start (streamer)](#quick-start-streamer)
* [What viewers type](#what-viewers-type)
* [How a request is handled](#how-a-request-is-handled)
* [Auto mode (requires tosu)](#auto-mode-requires-tosu)
* [Testing without going live](#testing-without-going-live)
* [Architecture](#architecture)
* [File structure](#file-structure)
* [External integrations and their contracts](#external-integrations-and-their-contracts)
* [Local HTTP / WebSocket API](#local-http--websocket-api)
* [Dashboard internals](#dashboard-internals)
* [Settings reference](#settings-reference-configjson)
* [Runtime files](#runtime-files)
* [Security](#security)
* [Development](#development)
* [Rules for contributors](#rules-for-contributors)
* [Known limitations and open items](#known-limitations-and-open-items)
* [Troubleshooting](#troubleshooting)
* [Changelog](#changelog)

---

## Project status

| | |
|---|---|
| Version | **1.3.0** (`git log` for history, [Changelog](#changelog) below) |
| Language / build | Go 1.23+, `CGO_ENABLED=0`, Windows x64 only |
| Tests | `go test ./...`: 11 packages with tests, all passing |
| Repository | local git repo, branch `main`, no remote yet |

**Verified for real on the developer's PC:** TikFinity detection, WebSocket connection and `config` handshake;
osu! metadata scraping; catboy downloads; MD5 verification (single diff and whole set); rejections; preload copy;
osu! folder auto-detection from the registry; the dashboard, settings and overlay; simulated chat; dry run;
the anti-flicker rendering, overlay pop-ups, mod commands and gifts through simulated chat, second launch and Quit,
audio preview and one-click bans. Auto mode and play results were verified against a **fake** tosu server.

**Not yet verified with the real thing:** see [Known limitations and open items](#known-limitations-and-open-items)
(live TikTok chat, gift and role fields, real tosu + osu! gameplay, opening `.osz` into osu! via Next, hotkeys, official-download fallback, lazer).

---

## Features

* **Chat requests** via TikFinity only (`!req` + aliases, configurable), accepting IDs and every common osu! link form.
* **Validation**: star range, max length, modes, ranked statuses, set/mapper/user blacklists, queue size,
  per-user limit (the map being played does not count), cooldown, duplicates, no-repeat window, no-limit (exempt) users.
* **Metadata and reference MD5** scraped from osu.ppy.sh (no API key), with a raw `.osu` fallback.
* **Downloads** from catboy.best → osu.direct → official site (browser + Downloads folder), each optional and reorderable,
  with per-source timeouts, ZIP checks, a cache, size limit and auto-cleanup.
* **MD5 verification** (`requested` or `strict`) against osu!'s current checksums; an outdated mirror copy is rejected.
* **Import** by opening the `.osz` (stable + lazer), copying it into Songs, or extracting it (stable), plus **preload** of the next N maps.
* **Play modes**: manual (**Next** button/hotkey) or **auto**, which uses tosu to see when the song has really ended.
* **Play results** from tosu (accuracy, rank, combo, misses, pp, mods; passed/failed/quit) on the request, in history and on the overlay.
* **Viewer feedback on the overlay**: since the helper cannot write in TikTok chat, pop-ups say when a request was added (with its place)
  or rejected (with the reason, e.g. "too hard · 5.88★ > 5.00★"), plus mod commands, gifts and results. English or Thai.
* **Chat commands**: `!remove` (own latest request), and for mods `!skip`, `!remove @user`, `!reqon`, `!reqoff`. Mods = a list, plus TikTok moderators.
* **Gift priority** (opt-in): gift coins add up per viewer; each full amount moves their oldest waiting request to the top or is saved for the next one.
* **Viewer roles**: followers-only mode and a higher per-user limit for subscribers (when TikFinity sends these flags).
* **One-click bans** of a requester, map or mapper from a queue card or the rejection log.
* **Audio preview** of queued maps and a link to open them in the owner's **beatmap viewer**.
* **CMD window** with the live log (titled `osureq <version>`); closing it stops the helper cleanly. A second launch opens the running copy's dashboard.
* **Dashboard** (EN/TH): status of TikFinity and tosu, now playing, queue with drag-to-reorder, per-source download
  progress, MD5 result, rejection log, activity, history, raw TikFinity events, full settings editor.
* **Test panel**: simulated chat through the real rules (requests and commands), quick add, simulated gifts, dry run (never touches osu!), clear test data.
* **OBS overlay**: transparent page with now playing, requester, upcoming queue (priority marked) and pop-ups (themes, size, corner, language).
* **Global hotkeys**: next, skip, remove last, toggle requests.
* **Security**: 127.0.0.1-only server, Host and header checks, whitelisted download hosts, path-traversal-safe extraction.

Out of scope for now: sending messages to TikTok chat (TikFinity's API does not offer it), macOS/Linux.

---

## Requirements

### To run it (streamer)

| | |
|---|---|
| OS | Windows 10/11 x64, internet connection |
| osu! | osu! **stable** or **lazer** installed, with `.osz` files opening in osu! (double-click a `.osz` once to check) |
| **TikFinity Desktop** (required) | Installed, logged in, connected to the TikTok account, and **running during the stream**. Default WebSocket `ws://127.0.0.1:21213/`. Download: <https://tikfinity.zerody.one/> |
| **tosu** (required) | Running during the stream, WebSocket on the default `127.0.0.1:24050`. Auto mode depends on it; without tosu, auto mode pauses (manual Next still works). Download: <https://github.com/tosuapp/tosu/releases/latest> |
| Optional | OBS or TikTok LIVE Studio, for the overlay |
| Optional | Logged in to osu.ppy.sh in the default browser. Only needed for the official-download fallback |

### To develop it

| | |
|---|---|
| Go | 1.23 or newer (`winget install GoLang.Go`). Uses `net/http` routing patterns (1.22) and `maps`/`slices` iterators (1.23) |
| OS | Windows (the `platform` package uses Win32 APIs; everything else is portable Go) |
| Modules | `github.com/coder/websocket` (WebSocket client and server), `golang.org/x/sys` (Windows APIs, registry) |
| Optional | Node.js, only for `node --check internal/server/web/app.js` as a quick syntax check |
| Optional | TikFinity and tosu installed locally for manual end-to-end tests |

---

## Quick start (streamer)

1. Put `osureq.exe` in its own folder, for example `C:\osureq\`. It creates `config.json`, `queue.json`, `osureq.log` and a `cache\` folder next to itself.
2. Start TikFinity Desktop and connect it to the TikTok account, then start **tosu** and osu!.
3. Double-click `osureq.exe`. A **CMD window** opens and shows the log; keep it open while streaming (closing it, or Ctrl+C, stops osureq).
   The dashboard opens at <http://127.0.0.1:21280/>. Starting it a second time just opens the dashboard of the running copy.
4. In **Settings → osu!**, choose stable or lazer. The osu! folder is detected from the `.osz` file association if the default one has no Songs folder.
5. Check the two status pills at the top:
   * **TikFinity**: 🔴 not installed/not running · 🟡 not connected, or connected and "Waiting for stream" · 🟢 live and taking requests.
   * **tosu**: 🔴 not running · 🟡 not connected or osu! not detected · 🟢 reading osu!. The first time tosu is seen running, its path is remembered so the banner can offer **Launch tosu**.
6. Use the **Test** panel to check the whole chain without going live.
7. Optional: in OBS, add a **Browser Source** with `http://127.0.0.1:21280/overlay` (transparent background).

Command-line flags: `-dir <folder>` keeps config, queue, cache and log somewhere else; `-no-browser` skips opening the dashboard.

---

## What viewers type

```
!req 2096297                                              ← beatmap (difficulty) ID by default; if no such beatmap exists it is tried as a set ID
!req https://osu.ppy.sh/beatmapsets/962870#mania/2096297
!req https://osu.ppy.sh/beatmapsets/962870                ← whole set: picks the hardest diff that fits the rules
!req https://osu.ppy.sh/b/2096297
!req https://osu.ppy.sh/beatmaps/2096297
!req https://osu.ppy.sh/s/962870
```

Anything else is ignored. Only numeric IDs on `osu.ppy.sh` (or `old.ppy.sh`) are accepted.

Other chat commands (names are configurable; an empty name turns one off):

| Command | Who | What it does |
|---|---|---|
| `!remove` | anyone | removes your own latest waiting request |
| `!remove @user` | mods | removes that user's latest waiting request |
| `!skip` | mods | skips the song being played (opens the next one) |
| `!reqon` / `!reqoff` | mods | opens / closes requests |

Mods are the users in **Settings → Command → Mods**, plus TikTok moderators when TikFinity sends that flag (`command.trustTikTokMods`).

---

## How a request is handled

1. **Parse** (`internal/parser`): the chat message must start with the command or an alias, followed by an ID or link.
2. **User checks** (`validator.CheckUser`, no network): blacklisted user, queue full, followers-only, per-user limit (higher for subscribers if set), cooldown.
   Exempt users skip everything after "queue full".
3. **Metadata** (`internal/osumeta`): `GET https://osu.ppy.sh/beatmapsets/{setId}` (or `/beatmaps/{id}`, which redirects to the set) and parse
   the `<script id="json-beatmapset">` JSON: title, artist, mapper, status, video flag, and per difficulty its id, mode, stars, length and
   `checksum` (MD5 of the `.osu`). Fallback: `GET https://osu.ppy.sh/osu/{beatmapId}` and hash it locally. Stars, length and status are
   then unknown, so they are not checked and the item gets a warning. A bare number that is not found is retried with the other meaning (beatmap ↔ set).
4. **Map rules** (`validator.CheckMap`): set/mapper blacklist, status, duplicate (queued or playing), no-repeat window, then mode, stars and length
   on the requested diff. For whole-set requests, the hardest diff that passes is chosen, preferring the set's main mode (the mode most of its diffs use).
5. **Queue** (`internal/queue`): saved to `queue.json` and kept across restarts. A saved gift priority is applied right away (see [Gifts](#gift-priority)).
   The overlay gets an "added" or "rejected" pop-up for every request (`app.FeedEvent`).
6. **Download** (`internal/downloader`), sources in configured order:
   1. `https://catboy.best/d/{setId}` (no video: `…/d/{setId}n`, sent with `Referer: https://catboy.best/`)
   2. `https://osu.direct/api/d/{setId}` (no video: `?noVideo=1`)
   3. **official**: opens `https://osu.ppy.sh/beatmapsets/{setId}/download` in the default browser and waits for `{setId} *.osz` in the Downloads folder. Only used when MD5 checking is on. The osu! password is never asked for or stored.

   The cache is checked first. Each source has its own timeout, and the response must be a real ZIP.
7. **MD5 check** (`internal/verifier`): unzip in memory and hash every `.osu`:
   * `requested`: the requested difficulty must match (every difficulty when the request was a whole set).
   * `strict`: every difficulty on osu! must match, and the archive may not contain extra `.osu` files.

   A mismatch tries the next source. If all fail: `skip` the request, or `warn` and use the latest download.
8. **Import** (`internal/importer`), on **Next** / **Play** / auto mode:
   * `open` (stable + lazer): Windows opens the `.osz`; osu! imports it and jumps to it.
   * `copy-to-songs` (stable): copied into Songs; picked up on song select or F5.
   * `extract-to-songs` (stable): extracted to `Songs\{setId} {Artist} - {Title}\`.

   **Preload** downloads and verifies the next N maps in the background, and on stable copies them into Songs so Next is instant.

Rejection reason codes (shown translated on the dashboard): `user_blacklisted`, `queue_full`, `user_limit`, `cooldown`, `set_blacklisted`,
`mapper_blacklisted`, `status`, `duplicate`, `recently_played`, `not_found`, `mode`, `stars_low`, `stars_high`, `too_long`, `meta_error`,
`bad_input`, `requests_off`, `followers_only`. Each rejection also has a language-neutral `short` detail (e.g. `5.88★ > 5.00★`, `60s`, `2/2`) used by the overlay.

### Gift priority

Off by default (`gifts.enabled`). TikFinity `gift` events are counted per viewer (streakable gifts once, when the streak ends: `repeatEnd`).
Every full `gifts.coinsPerPriority` coins (default 30) gives one priority: the viewer's oldest waiting request moves to the top, behind earlier
priority requests; with nothing waiting, it is saved for their next request for `gifts.creditMinutes`. Priority requests show ★ on the dashboard and overlay.

---

## Auto mode (requires tosu)

The helper follows the map it opened (matched by **beatmapset**, so any difficulty counts) through tosu's game state
(`internal/tosu.Tracker`):

| What tosu shows | Phase (`Item.playPhase`) |
|---|---|
| Map opened, not started (song select alone does not count) | `waiting` |
| Gameplay on that map (retries and fails stay here) | `playing` |
| Results screen (retry from here goes back to `playing`) | `results` |
| Song select or menu after playing, or gameplay of another map | `ended` → the next map opens after `autoGapSec` |

`busy`, `unknown`, `update` and empty states are ignored, because tosu reports them briefly while osu! loads.
There is **no timer fallback**: if tosu is not connected, auto mode pauses and the dashboard says so; it resumes by itself when tosu returns.
When nothing is playing, the first ready map opens straight away. Manual **Next** always works.

**Play results:** while the map is in `playing`, the latest live numbers from `play` are kept. Entering `results` stores tosu's `resultsScreen`
as **passed**; leaving gameplay without a results screen stores the last live numbers as **failed** (tosu's `play.failed`) or **quit**.
A quit before any score leaves no result. The result is saved on the item (`Item.result`), copied into history (`Played.play`), shown on the phase line,
and announced on the overlay: `98.52% S · 812x · 2 miss · 301pp · HDHR`.

---

## Testing without going live

The **Test** panel on the dashboard works while TikFinity is offline:

* **Simulate chat** (`POST /api/simulate`): a message as any `@user`, through the real chat path (command, aliases, requests on/off,
  per-user limit, cooldown, map rules). Non-requests are reported as "ignored". A bare ID or link gets the command prepended.
* **Quick add** (`POST /api/test`): straight into the queue, map rules only.
* **Gift** (`POST /api/simulateGift`): pretends `@user` sent a gift worth N coins (gift priority must be on).
* Chat commands (`!skip`, `!remove`, …) can be typed in Simulate chat too; mods are checked against the Mods list.
* **Dry run** (`test.dryRun`): Next/Play move the queue (auto mode runs too) but never open osu! or copy into Songs. A `DRY RUN` badge shows. Downloads and MD5 checks still happen.
* **Clear test data**: removes test requests (also from now playing), their history and rejections, cooldowns set by simulated chat, and test gift credits.

---

## Architecture

```
                        ┌──────────────────────────── osureq.exe ────────────────────────────┐
TikTok LIVE             │                                                                    │
   │                    │  tikfinity.Client ──chat──► app.HandleChat ─► parser ─► validator  │
   ▼                    │        (status)                    │               (CheckUser)     │
TikFinity Desktop ─ws──►│                                    ▼                               │
 ws://127.0.0.1:21213   │                             osumeta.Client ─► validator.CheckMap   │──https──► osu.ppy.sh
                        │                                    │                               │
                        │                                    ▼                               │
                        │                              queue.Queue ◄──── queue.json          │
                        │                                    │                               │
                        │   app.Run/tick (every 2 s or on wake): preload, auto mode          │
                        │                                    ▼                               │
                        │      downloader ─► verifier ─► cache\{setId}[n].osz                │──https──► catboy.best / osu.direct
                        │                                    │                               │           (official: browser + Downloads)
                        │                                    ▼                               │
osu! ◄──ShellExecute────│                               importer (open / copy / extract)    │
  │                     │                                                                    │
  ▼                     │  tosu.Client ──game state──► app.OnGame ─► tosu.Tracker (phase)    │
tosu ─────ws───────────►│                                                                    │
 ws://127.0.0.1:24050   │  server: http://127.0.0.1:21280  /  /overlay  /api/*  /ws (push)   │◄── browser dashboard, OBS
                        └────────────────────────────────────────────────────────────────────┘
```

**Concurrency model** (all in `main.go`):

* `tikfinity.Client.Run` and `tosu.Client.Run`: one goroutine each; reconnect every `reconnectSec`.
* `app.Run`: a loop woken by queue changes (`App.Wake`) or a 2 s ticker; starts **one download at a time** and drives auto mode.
* Every chat request is resolved in its own goroutine (`App.submit`), because metadata lookups take network time.
* `server`: `net/http` on 127.0.0.1; state pushes are coalesced (120 ms) and sent to every WebSocket client.
* Hotkeys: a locked OS thread with a Win32 message loop (`platform.Hotkeys`).
* Overlay pop-ups: `App.emit` appends to a 30-entry feed (`App.Feed`) that is part of every state push; the overlay shows entries newer than the last one it saw.

**State ownership:** `queue.Queue` (mutex-guarded) is the single source of truth for requests; `config.Store` for settings
(subscribers via `OnChange`). The UI never holds state of its own beyond view preferences in `localStorage`.

**Dependency direction:** `main` → `app` → (`parser`, `osumeta`, `validator`, `queue`, `downloader`, `verifier`, `importer`, `tikfinity`, `tosu`) → `config`.
`server` depends on `app`. Only `main` imports `platform` (Windows APIs are injected into `tikfinity`, `tosu`, `downloader`, `importer` as functions,
which keeps those packages testable).

---

## File structure

```
otiktokdownloader/
├── main.go                      entry point: flags, logging, wiring, osu! folder detection, hotkeys, startup
├── go.mod / go.sum              module github.com/draft/osureq (Go 1.23)
├── build.ps1                    vet + test + build → dist\osureq.exe
├── rsrc_windows_amd64.syso      Windows resources linked into the exe (generated by tools/winres)
├── tools/winres/main.go         generates the .syso: logo icon, version info, manifest
├── README.md                    this file (user guide + handoff doc)
├── .gitignore                   ignores dist/, cache/, config.json, queue.json, logs
├── .gitattributes               *.osz binary (keeps test fixtures byte-exact on Windows)
├── dist/                        (ignored) built exe; when run from here also holds config/queue/cache/log
└── internal/
    ├── app/          app.go                orchestration: request pipeline, preload, play/next/skip, auto mode, results, test panel
    │                 feed.go               overlay pop-up events (FeedEvent)
    │                 commands.go (+_test)  chat commands: !skip, !remove, !reqon, !reqoff; who is a mod
    │                 gifts.go              gift coins → priority, simulated gifts
    │                 ban.go (+_test)       one-click bans (blacklist + drop matching requests)
    │                 results_test.go       tosu → play result tests
    ├── config/       config.go             config.json schema, defaults, validation, store with change subscribers
    ├── parser/       parser.go (+_test)    "!req …" → Request{Kind, SetID, BeatmapID, Mode, Bare}
    ├── osumeta/      osumeta.go (+_test)   osu.ppy.sh scraping: set metadata, reference MD5, beatmap→set, raw .osu fallback
    ├── validator/    validator.go (+_test) CheckUser / CheckMap → *Rejection{Code, Detail}
    ├── queue/        queue.go (+_test)     items, now playing, history, rejections, cooldowns; queue.json persistence
    │                 gifts.go (+_test)     gift credits, priority tokens, prioritise
    ├── downloader/   downloader.go (+_test) sources, timeouts, ZIP sniffing, official browser flow, cache, cleanup
    ├── verifier/     verifier.go (+_test)  in-memory unzip + MD5 compare (strict / requested)
    │   └── testdata/                       .osz fixtures (regenerate with -update)
    ├── importer/     importer.go, osudb.go (+_test)  open / copy / extract, Songs dir, safe paths, osu!.db hash scan
    ├── tikfinity/    tikfinity.go (+_test) TikFinity detection + WebSocket client + chat parsing
    ├── tosu/         tosu.go (+_test)      tosu WebSocket client + song-end Tracker
    ├── platform/     platform_windows.go, hotkey.go (+_test)  Win32: ShellExecute, processes, registry, hotkeys
    └── server/       server.go             HTTP routes, guard, state snapshot, WebSocket push
        └── web/                            embedded into the exe with go:embed
            ├── index.html                  dashboard markup
            ├── app.js                      dashboard logic (no framework, no build step)
            ├── app.css                     dashboard styles (design tokens)
            ├── overlay.html                OBS overlay (self-contained HTML+CSS+JS)
            └── icon.svg                    favicon
```

### File by file

| File | Lines | What is in it | Key symbols |
|---|---:|---|---|
| `main.go` | ~250 | Parses `-dir` / `-no-browser`, titles the CMD window, picks the base folder (exe folder, or cwd under `go run`), sets up `osureq.log` (rotates at 5 MB), loads config, fixes the osu! folder from the registry, builds every module, wires callbacks (chat/gift → app, tosu → app, status → push, config changes → hotkeys/reconnects), hands off to an already running copy, starts the goroutines, waits for Ctrl+C / window close / Quit | `main`, `exeDir`, `setupLog`, `fatal`, `alreadyRunning` |
| `internal/app/app.go` | ~750 | The brain. Turns chat into queue items, runs downloads/preload, handles Next/Play/Skip/Remove/Retry, auto mode and play results via tosu, dry run, test panel, notices | `App`, `HandleChat`, `SimulateChat`, `TestRequest`, `submit`/`resolve`/`lookup`, `Run`/`tick`, `OnGame`, `ResultShort`, `startDownload`/`download`, `preImport`, `Next`, `Play`/`play`, `Skip`, `ClearTest`, `ToggleDryRun`, `Hotkey`, `warnOnce`; `origin` (`fromChat`/`fromSim`/`fromQuick`) |
| `internal/app/feed.go` | ~50 | Events for the overlay pop-ups (added, rejected, command, gift, result), newest first, 30 kept | `FeedEvent`, `emit`, `Feed` |
| `internal/app/commands.go` | ~100 | Chat commands other than requests; `IsMod` (Mods list or TikTok moderator flag) | `command`, `IsMod` |
| `internal/app/gifts.go` | ~60 | Gift coins → priority tokens → prioritised requests; simulated gifts | `HandleGift`, `SimulateGift`, `applyCredit` |
| `internal/app/ban.go` | ~70 | Adds to a blacklist and removes matching waiting requests | `Ban` |
| `internal/config/config.go` | ~310 | All settings structs with JSON names, `Default()`, `Validate()`, download host whitelist, `Store` (atomic save via `.tmp` + rename, `OnChange` subscribers). Unknown/missing fields fall back to defaults because the file is unmarshalled over `Default()` | `Config`, `Connection`, `Osu`, `Command`, `Rules`, `Queue`, `Source`, `Download`, `MD5`, `Import`, `Overlay`, `Hotkeys`, `Test`, `HostAllowed`, `Store` |
| `internal/parser/parser.go` | ~150 | Command matching (case-insensitive, aliases), ID and link parsing, `#mode/id` fragments, rejects foreign hosts and non-numeric IDs. `Bare` marks plain numbers so the app can retry as the other kind | `Parse`, `ParseArg`, `Request`, `Request.Flip`, `Kind` |
| `internal/osumeta/osumeta.go` | ~290 | **The only code that talks to osu.ppy.sh for metadata.** Rate-limited (700 ms between requests), 10 min cache, `json-beatmapset` extraction, beatmap→set resolution, raw `.osu` fallback (`Partial` sets) | `Client`, `Get`, `ResolveSet`, `ParsePageJSON`, `ParseOsuHeader`, `Set`, `Diff`, `ErrNotFound` |
| `internal/validator/validator.go` | ~210 | Pure functions, no I/O. User checks (cheap, before any network: blacklist, queue size, followers-only, per-user/subscriber limit, cooldown) and map checks; picks the diff for whole-set requests (main mode first) | `CheckUser`, `CheckMap`, `State`, `Roles`, `Input`, `Rejection` (`Code`, `Detail`, `Short`) |
| `internal/queue/queue.go` | ~550 | Thread-safe queue + now playing + history (200) + rejections (100) + per-user last request; debounced save to `queue.json`; on load, unfinished downloads and ready items go back to `queued` so files are re-verified | `Queue`, `Item`, `State` (`queued`/`downloading`/`ready`/`failed`/`playing`), `Attempt`, `Played`, `Rejected`, `ValidationState`, `Promote`, `Drop`, `Move`, `RemoveLatestBy`, `Position`, `ClearTest`, `Snapshot`; `Result` (play result) |
| `internal/queue/gifts.go` | ~100 | Per-user gift coins and priority tokens (with expiry); moving a request behind the other priority requests | `Credit`, `AddCoins`, `UseToken`, `Tokens` |
| `internal/downloader/downloader.go` | ~450 | Cache lookup, source loop, streaming download with progress, early "not a ZIP" detection, official browser flow (watches Downloads, waits for the size to settle, ignores `.crdownload`/`.part`), verified-file registry (`Trusted`), LRU-ish cleanup | `Downloader`, `Get`, `Job`, `Outcome`, `Trusted`, `Cleanup`, `CacheSize` |
| `internal/verifier/verifier.go` | ~140 | ZIP magic check, in-memory unzip (64 MB per `.osu` cap), MD5 per `.osu`, comparison at `strict`/`requested` level | `Verify`, `Compare`, `OsuHashes`, `LooksLikeZip`, `Reference`, `Result` |
| `internal/importer/importer.go` | ~210 | Refuses untrusted files, `open`/`copy`/`extract`, Songs folder from `osu!.<user>.cfg` `BeatmapDirectory`, Windows-safe names, traversal-safe joins | `Importer.Import`, `SongsDir`, `CopyToSongs`, `ExtractToSongs`, `SafeJoin`, `SafeName` |
| `internal/importer/osudb.go` | ~70 | Finds beatmap MD5s in `osu!.db` by scanning for `0x0b 0x20 <32 hex>` instead of parsing the versioned format | `OsuDB.Has`, `ScanHashes` |
| `internal/tikfinity/tikfinity.go` | ~440 | Install/process checks, WebSocket loop, `config` handshake, live detection (any event; times out after 5 min of silence), raw event log (first event of each type kept), chat field extraction with fallbacks, viewer roles (yes/no/unknown), gift parsing (streaks counted once) | `Client`, `Status` (`red`/`yellow`/`green`, codes), `Chat`, `Tri`, `ParseChat`, `Gift`, `ParseGift`, `Accepting` |
| `internal/tosu/tosu.go` | ~420 | tosu WebSocket loop (`/websocket/v2`), payload parsing (state, beatmap, live `play` numbers, `resultsScreen`), status levels (only the short `Brief` state drives pushes), remembers `tosu.exe` path, song-end state machine | `Client`, `Status`, `Game`, `Brief`, `Score`, `Parse`, `Tracker`, `Phase` (`waiting`/`playing`/`results`/`ended`) |
| `internal/platform/platform_windows.go` | ~280 | `ShellExecuteW` open, Toolhelp process list, process image path, TikFinity lookup (path + Uninstall registry), osu! stable lookup (`HKCR\osustable.File.osz\shell\open\command`), detached launch, `RegisterHotKey` message loop, console window title | `ShellOpen`, `ProcessRunning`, `ProcessPath`, `FindTikFinity`, `FindOsuStable`, `Launch`, `Hotkeys`, `SetConsoleTitle` |
| `internal/platform/hotkey.go` | ~50 | Parses `Ctrl+Alt+N`, `Shift+F5`, `F9`, `Win+Num5`… into modifier + virtual key | `ParseHotkey` |
| `internal/server/server.go` | ~420 | Routes, `guard` (Host + `X-Osureq` header), `State` snapshot, coalesced WebSocket push + 20 s keepalive state, config GET/PUT, actions (incl. quit), queue operations, test/simulate/simulateGift, ban | `Server`, `State`, `Handler`, `Push`, `Listen`, `Quit` |
| `internal/server/web/index.html` | ~90 | Dashboard skeleton: top bar (status pills, requests switch, language, settings), banners, now playing, test panel, queue, side logs, settings sheet, toast | |
| `internal/server/web/app.js` | ~860 | Everything on the dashboard: `T` i18n table (en/th), API helper, WebSocket client, incremental rendering, queue cards, drag & drop, ban menu, audio preview + viewer links, results, test panel (chat / quick / gift), settings `SCHEMA` → form | `render`, `renderBanner`, `renderTosuBanner`, `renderNow`, `updatePhase`, `resultShort`, `renderQueue`, `buildCard`, `banMenu`, `mapLinks`, `updateAttempts`, `renderSide`, `same`, `SCHEMA`, `field` |
| `internal/server/web/app.css` | ~230 | Design tokens and components copied from the owner's osu! beatmap viewer: pills, banners, cover cards, star chips, switches, tabs, sheet, toast | `:root` tokens |
| `internal/server/web/overlay.html` | ~160 | OBS page: transparent, now playing + upcoming (priority ★), pop-ups from the feed with their own en/th text table `TX`, theme/scale/position/language from config, reconnecting WebSocket | `render`, `showToasts`, `toastHTML` |
| `internal/verifier/testdata/*.osz` | | `set_good`, `set_outdated` (old Hard diff), `set_extra` (extra `.osu`), `set_no_osu`, `not_zip` (HTML) | regenerate: `go test ./internal/verifier -update` |
| `*_test.go` | | parser (link forms, hostile inputs), validator (every rule, exempt users), verifier (fixtures, levels), downloader (fake mirrors: fallthrough, cache, skip/warn, MD5 off, cleanup), importer (traversal, Songs dir, osu!.db scan), queue (limits, ClearTest, gift credits and ordering), app (chat commands, play results, bans), osumeta (page JSON, .osu header), tikfinity (chat parsing, roles, gifts, levels), tosu (payload incl. scores, Tracker paths, client vs fake server), platform (hotkey parsing) | |

---

## External integrations and their contracts

These are the assumptions most likely to break when a third party changes something. Each lives in exactly one package.

### TikFinity Desktop (`internal/tikfinity`)

* WebSocket `ws://127.0.0.1:21213/` (configurable). Messages are JSON `{"event": "<name>", "data": {...}}`.
* On connect TikFinity sends `{"event":"config", ...}`, which is used as the handshake (**verified**).
* Chat: `{"event":"chat","data":{"comment": "...", "uniqueId": "...", "nickname": "..."}}`. These field names follow the TikTok payload TikFinity
  forwards; **not yet verified on a live stream**. `ParseChat` also accepts `text`/`message`, `username`/`userId`, `displayName`, and the same keys under `data.user`.
* Process name `TikFinity.exe`; default install `%LOCALAPPDATA%\Programs\tikfinity\TikFinity.exe`; also found through
  `HKCU|HKLM\Software\Microsoft\Windows\CurrentVersion\Uninstall\*` where `DisplayName` starts with "TikFinity".
* Roles (**unverified**): `isModerator`, `isSubscriber`, `followRole` (0 none, 1 follower, 2 friend) on `data`, `data.user`, or `data.userIdentity`
  (`isModeratorOfAnchor`, `isSubscriberOfAnchor`, `isFollowerOfAnchor`). Missing = unknown, which never blocks a viewer.
* Gifts (**unverified**): `{"event":"gift","data":{"uniqueId","nickname","giftName","giftType","repeatCount","repeatEnd","diamondCount"}}`.
  Coins = `diamondCount × repeatCount`; for `giftType` 1 (streakable) only the event with `repeatEnd: true` counts.
* The dashboard keeps the first event of every type, and with `logRawEvents` the last 100 events, so field names can be checked without a debugger.

### tosu (`internal/tosu`)

* WebSocket `ws://127.0.0.1:24050/websocket/v2` (configurable); tosu pushes the full state every poll (default 100 ms). Only changes are forwarded to the app.
* Fields used: `client` (`stable`/`lazer`), `state.name` (or `state.number`), `beatmap.set`, `beatmap.id`, `beatmap.checksum`,
  `play.{failed, accuracy, score, combo.max, rank.current, pp.current, mods.name, hits."0"}` and
  `resultsScreen.{accuracy, score, maxCombo, rank, pp.current, mods.name, hits."0"}` (types from `packages/tosu/src/api/types/v2.ts`). A payload with `error` is ignored.
* `state` names follow tosu's `GameState` enum: `menu 0, edit 1, play 2, exit 3, selectEdit 4, selectPlay 5, selectDrawings 6, resultScreen 7, update 8,
  busy 9, unknown 10, lobby 11, matchSetup 12, selectMulti 13, rankingVs 14, onlineSelection 15, optionsOffsetWizard 16, rankingTagCoop 17, rankingTeam 18,
  beatmapImport 19, packageUpdater 20, benchmark 21, tourney 22, charts 23` (source: `tosuapp/tosu/packages/common/enums/osu.ts`).
* Process name `tosu.exe`; the path is learned from the running process (`QueryFullProcessImageName`) and saved as `connection.tosuPath`.

### osu! website (`internal/osumeta`, unofficial, no API key)

| Purpose | Request | Notes |
|---|---|---|
| Set metadata + checksums | `GET https://osu.ppy.sh/beatmapsets/{setId}` | parse `<script id="json-beatmapset" type="application/json">` |
| Beatmap → set | `GET https://osu.ppy.sh/beatmaps/{id}` (redirects to the set page) | `/b/{id}` also redirects |
| Fallback MD5 | `GET https://osu.ppy.sh/osu/{beatmapId}` | raw `.osu`; `[Metadata] BeatmapSetID` gives the set |
| Covers (browser only) | `https://assets.ppy.sh/beatmaps/{setId}/covers/cover.jpg` | |
| Audio preview (browser only) | `https://b.ppy.sh/preview/{setId}.mp3` | played by the dashboard, not the helper |

**Beatmap viewer link:** `osu.viewerUrl` (default `https://osu-beatmap-viewer.vercel.app/`) + `?s={setId}&b={beatmapId}`. It is only a link opened in the
browser; the helper sends nothing to the viewer. The viewer loads map files from public mirrors itself.

User-Agent `osureq/1.0 (TikTok LIVE request helper)`. Never embed osu! OAuth secrets.

### Mirrors (`internal/downloader`, URL templates in config)

| Source | URL | No-video URL | Notes |
|---|---|---|---|
| catboy | `https://catboy.best/d/{id}` | `https://catboy.best/d/{id}n` | send `Referer: https://catboy.best/` (verified working) |
| osu.direct | `https://osu.direct/api/d/{id}` | `https://osu.direct/api/d/{id}?noVideo=1` | the no-video parameter is a best guess |
| official | `https://osu.ppy.sh/beatmapsets/{id}/download` | `…?noVideo=1` | opened in the browser; the file is picked up from Downloads |

nerinyan is deliberately not used (it was not working). Allowed hosts: `catboy.best`, `osu.direct`, `osu.ppy.sh` (`config.AllowedHosts`).

### Windows

* `.osz` association `HKCR\osustable.File.osz\shell\open\command` → osu! stable folder.
* `osu!.<WindowsUser>.cfg` → `BeatmapDirectory` (relative to the osu! folder, or absolute).
* `ShellExecuteW("open", file)` for `.osz`, URLs and `osu://b/{id}`.

---

## Local HTTP / WebSocket API

Server: `http://127.0.0.1:<dashboardPort>` (default 21280). Requests must have `Host` = `127.0.0.1:<port>` or `localhost:<port>`.
Every non-GET request must send the header `X-Osureq: 1`. Responses are JSON; actions return `{"ok": true}` or `{"ok": false, "error": "…"}`.

| Method & path | Purpose |
|---|---|
| `GET /` | dashboard |
| `GET /overlay` | OBS overlay |
| `GET /assets/{file}` | embedded `app.js`, `app.css`, `icon.svg` |
| `GET /ws` | WebSocket; pushes the full `State` JSON on every change (and every 20 s) |
| `GET /api/state` | the same `State` once |
| `GET /api/config` | `{config, defaults}` |
| `PUT /api/config` | replace the whole config (validated); `{ok, restartNeeded}` |
| `POST /api/action/{name}` | `next`, `skip`, `removeLast`, `toggleRequests`, `clear`, `cleanup`, `toggleDryRun`, `clearTest`, `launchTikfinity`, `reconnect`, `openTikfinityDownload`, `launchTosu`, `reconnectTosu`, `openTosuDownload`, `quit` |
| `POST /api/queue/{id}/{op}` | `remove`, `skip`, `play`, `retry`, `move` (body `{"to": <index>}`) |
| `POST /api/test` | quick add: `{"input": "<id or link>", "user": "<name>"}` → `{ok, item}` or `{ok:false, code, error}` |
| `POST /api/simulate` | simulated chat: `{"message": "!req …", "user": "<name>"}`; `code: "ignored"` when it is not a request; `{ok, command: true}` for a chat command |
| `POST /api/simulateGift` | `{"user": "<name>", "coins": 30}` → `{ok, earned}` (priorities earned) |
| `POST /api/ban` | `{"kind": "user" \| "set" \| "mapper", "value": "…"}` → `{ok, removed}` |

`State` fields: `version`, `tikfinity` (status), `tosu` (status incl. `game`), `autoGap`, `queue` (`items`, `now`, `history`, `rejected`),
`requests`, `resolving`, `notices`, `feed` (overlay pop-up events), `cacheMb`, `overlay`, `command`, `lang`, `playMode`, `viewerUrl`, `blacklistUsers`, `dryRun`, `raw`, `firstRaw`,
`extra` (`hotkeyErrors`, `baseDir`, `cacheDir`).

Example (PowerShell):

```powershell
Invoke-RestMethod -Method Post -Uri http://127.0.0.1:21280/api/simulate -Headers @{ "X-Osureq" = "1" } -ContentType application/json -Body '{"user":"fan","message":"!req 2096297"}'
```

---

## Dashboard internals

* **No framework, no build step.** `app.js` uses a tiny `h(tag, attrs, ...children)` helper. Edit the files and rebuild the exe (they are embedded with `go:embed`).
* **Rendering rule (important):** state arrives up to ~4 times per second during downloads. Never rebuild a section unconditionally.
  `same(el, key)` skips a section whose input did not change; queue cards are kept in a `Map` by item id and rebuilt only when their own data changes;
  download bars are updated in place (`updateAttempts`); cards that were already shown get the `seen` class so their entrance animation does not replay.
  Breaking this brings back the flicker fixed in 1.1.1.
* **i18n:** every UI string is a key in the `T` table: `key: ["English", "ไทย"]`. Use `t("key", {vars})`; static HTML uses `data-t` / `data-tp`. Add both languages.
* **Settings form** is generated from `SCHEMA` (`F(path, type, labelEn, labelTh, hintEn, hintTh, extra)`). Types: `switch`, `text`, `hotkey`, `number`,
  `list`, `numlist`, `seg`, `select`, `multi`, `button`, `overlaylink`, `sources`. A new config field needs: Go struct field + default + validation (if any) +
  a `SCHEMA` entry + a row in the [Settings reference](#settings-reference-configjson).
* **Design language** comes from the owner's osu! beatmap viewer (`F:\osubeatmapviewer\assets\app.css`): tokens
  `--bg #1c1726, --bg2 #241d33, --panel #2d2540, --line #443a5a, --ink #f3eefb, --dim #b3a8cc, --pink #ff66aa, --pinkdk #2a0f1f, --sky #66ccff`,
  fonts Varela Round + IBM Plex Sans Thai, pill buttons, cover-art cards, the osu! star colour ramp (`starColor`). Keep new UI consistent with it.

---

## Settings reference (`config.json`)

All settings are editable in the dashboard (gear icon). Missing keys take their defaults.

| Section | Key | Default | Meaning |
|---|---|---|---|
| (root) | `language` | `en` | saved by the settings sheet; the dashboard language itself is per browser |
| connection | `tikfinityUrl` | `ws://127.0.0.1:21213/` | TikFinity WebSocket API (must be on this PC) |
| | `dashboardPort` | `21280` | Dashboard/overlay port (applies after restart) |
| | `reconnectSec` | `5` | Retry interval for TikFinity and tosu |
| | `autoLaunchTikfinity` | `false` | Start TikFinity if it is installed but not running |
| | `tikfinityPath` | `%LOCALAPPDATA%\Programs\tikfinity\TikFinity.exe` | Also found through the registry |
| | `openDashboardOnStart` | `true` | |
| | `logRawEvents` | `false` | Show every TikFinity message in the dashboard |
| | `tosuUrl` | `ws://127.0.0.1:24050/websocket/v2` | tosu's v2 WebSocket (must be on this PC) |
| | `tosuPath` | empty | Filled in automatically the first time tosu is seen running |
| | `autoLaunchTosu` | `false` | Start tosu if it is not running (needs `tosuPath`) |
| osu | `game` | `stable` | `stable` or `lazer` |
| | `folder` | `%LOCALAPPDATA%\osu!` | osu! stable folder. If it has no Songs folder, the folder from the `.osz` association is used at startup and saved |
| | `cacheFolder` | `cache` | Relative to the exe (or `-dir`) |
| | `downloadsFolder` | `%USERPROFILE%\Downloads` | Watched by the official source |
| | `useOsuDb` | `false` | Experimental (stable): if `osu!.db` already has the map, skip the download and open `osu://b/{id}` |
| | `viewerUrl` | `https://osu-beatmap-viewer.vercel.app/` | Beatmap viewer for the "viewer ↗" link; empty hides it |
| command | `name` / `aliases` | `!req` / `!request`, `!r` | |
| | `enabled` | `true` | Requests on/off (also the switch in the top bar) |
| | `bareNumberAs` | `beatmap` | `beatmap` or `set` (the other meaning is tried if not found) |
| | `mods` | empty | TikTok users who may use mod commands |
| | `trustTikTokMods` | `true` | TikTok moderators count as mods (when TikFinity sends the flag) |
| | `skipCmd` / `removeCmd` / `openCmd` / `closeCmd` | `!skip` / `!remove` / `!reqon` / `!reqoff` | empty turns a command off |
| rules | `minStars` / `maxStars` | `0` / `0` | 0 = no limit |
| | `maxLengthSec` | `0` | 0 = no limit |
| | `modes` | all four | `osu`, `taiko`, `fruits`, `mania` |
| | `statuses` | ranked, approved, qualified, loved | plus `pending`, `wip`, `graveyard` |
| | `blacklistSets` / `blacklistMappers` / `blacklistUsers` | empty | |
| | `exemptUsers` | empty | TikTok users who skip followers-only, the per-user limit and cooldown |
| | `followersOnly` | `false` | Reject viewers TikTok reports as not following (unknown is let through) |
| | `subscriberPerUserLimit` | `0` | Per-user limit for subscribers; 0 = same as everyone |
| queue | `maxSize` | `30` | 0 = no limit |
| | `perUserLimit` | `2` | Maps **waiting** per user (the one playing does not count) |
| | `cooldownSec` | `60` | Per user |
| | `noRepeatMin` | `60` | Block maps played within this many minutes |
| | `playMode` | `manual` | `manual` or `auto` (tosu) |
| | `autoGapSec` | `5` | Seconds after the song ends before the next map opens |
| download | `sources` | catboy, osu.direct, official | Order, on/off, URL templates (`{id}`), no-video URL, referer, timeout, `official` flag |
| | `noVideo` | `true` | Uses the no-video URL when the set has a video |
| | `cacheLimitMb` / `autoCleanup` | `2048` / `true` | Deletes the oldest unreferenced files when over the limit |
| md5 | `enabled` | `true` | |
| | `level` | `requested` | `requested` or `strict` |
| | `verifyCache` | `true` | Re-hash cached files before use |
| | `onAllFail` | `skip` | `skip` or `warn` |
| | `metaTimeoutSec` | `15` | osu! metadata timeout |
| import | `mode` | `open` | `open`, `copy-to-songs`, `extract-to-songs` |
| | `preload` / `preloadCount` | `true` / `2` | |
| overlay | `visible`, `showRequester`, `theme` (`pink`/`sky`/`mono`), `scale` (%), `position` | `5`, `true`, `pink`, `100`, `top-left` | |
| | `feedback` / `feedbackSec` | `true` / `7` | Pop-ups for viewers and how long they stay |
| | `lang` | `auto` | Overlay language: `auto` (OBS browser language), `en`, `th` |
| gifts | `enabled` | `false` | Gifts buy queue priority |
| | `coinsPerPriority` | `30` | Coins (added up per viewer) for one priority |
| | `creditMinutes` | `60` | How long an unused priority waits for a request |
| hotkeys | `enabled` | `false` | Global hotkeys (work while osu! has focus) |
| | `next` / `skip` / `removeLast` / `toggleRequests` | `Ctrl+Alt+N` / `Ctrl+Alt+S` / `Ctrl+Alt+Backspace` / `Ctrl+Alt+R` | |
| test | `dryRun` | `false` | Next/Play never open osu! or touch Songs |

---

## Runtime files

All live in the base folder (next to the exe, or `-dir`):

| File | Written by | Contents |
|---|---|---|
| `config.json` | `config.Store` (atomic) | settings; safe to edit while stopped |
| `queue.json` | `queue.Queue` (debounced, atomic) | queue items, now playing, history, rejections, last request per user, simulated-chat users |
| `cache\{setId}.osz`, `cache\{setId}n.osz` | downloader | verified archives (`n` = no video) |
| `osureq.log` (+ `.old`) | `log` | everything important: status changes, queued/rejected requests, now playing, tosu phases, errors |

---

## Security

* The server listens on `127.0.0.1` only. Requests with another `Host` header are refused (DNS rebinding). State-changing calls need `X-Osureq: 1`,
  which other websites cannot send without a CORS preflight that is never answered. The WebSocket accepts same-origin connections only.
* Only numeric IDs are accepted. Downloads go only to whitelisted HTTPS hosts; redirects elsewhere are refused.
* Only `.osz` files that this run of the helper downloaded and verified are opened or imported (`Downloader.Trusted`). After a restart, queued items
  go back through the cache check.
* File names are sanitised; extraction refuses absolute paths, drive letters, `..` and symlinks.
* No osu! OAuth secret, password or API key is used or stored.

---

## Development

```powershell
go vet ./...
go test ./...
.\build.ps1 -Version 1.3.1          # vet + test + build → dist\osureq.exe
```

If PowerShell blocks scripts, build directly:

```powershell
$env:GOOS="windows"; $env:GOARCH="amd64"; $env:CGO_ENABLED="0"
go run ./tools/winres -version 1.3.3      # icon, version info, manifest → rsrc_windows_amd64.syso
go build -trimpath -ldflags "-X main.version=1.3.3" -o dist\osureq.exe .
```

* `main.version` is set at build time (`dev` otherwise).
* `rsrc_windows_amd64.syso` (generated by `tools/winres`, committed) gives the exe its icon, name, description, version and an `asInvoker`
  manifest. Keep it, and do not strip the binary with `-s -w`: anonymous, stripped Go binaries are flagged by antivirus heuristics far more often.
* osureq is a **console app on purpose**: the owner wants the CMD window with the log visible. Do not switch it to `-H windowsgui` or add a tray icon without asking;
  a tray version existed briefly in 1.3.0 and was removed in 1.3.1. The log goes to the window and to `osureq.log`.
* Format with `gofmt -w .`; check the dashboard script with `node --check internal/server/web/app.js`.
* Regenerate `.osz` fixtures: `go test ./internal/verifier -update`.
* **Replacing a running exe:** Windows will not overwrite a running `osureq.exe`, but it allows renaming it. Rename it to `osureq.old.exe`, build the new one,
  and ask the streamer to restart.

### Safe manual testing (sandbox)

Never test against the streamer's real setup by accident. Run a second copy with its own folder and port:

1. Make a folder, e.g. `%TEMP%\osureq-sandbox`, and put a `config.json` there with at least:
   `{"connection":{"dashboardPort":21282,"openDashboardOnStart":false},"test":{"dryRun":true},"import":{"preload":false}}`.
   If preload must be tested, keep `dryRun` on (preload copying into Songs is skipped in dry run) or point `osu.folder` at a fake folder that contains an empty `Songs`.
2. `go build -o osureq-test.exe . ; .\osureq-test.exe -dir %TEMP%\osureq-sandbox -no-browser`
3. Open `http://127.0.0.1:21282/` and use the Test panel, or call `/api/simulate` and `/api/test`.
4. **Fake tosu** for auto-mode tests: a ~40-line Go program that serves `/websocket/v2` pushing
   `{"client":"stable","state":{"name":"play"},"beatmap":{"id":1,"set":<setId>,"checksum":"ab"},"play":{"failed":false}}` and lets you change
   `state`/`set` through an HTTP endpoint. Point `connection.tosuUrl` at it (for example port 24051). `internal/tosu/tosu_test.go` contains the same idea (`fakeTosu`).

### Common extension recipes

* **New download source:** add it to `config.Default().Download.Sources` (URL template with `{id}`), add its host to `config.AllowedHosts`, and nothing else; the downloader is generic.
* **New validation rule:** add the field to `config.Rules` or `config.Queue`, the check to `validator` (user-level checks in `CheckUser`, map-level in `CheckMap`/`checkDiff`)
  with a new `Code`, a `r_<code>` string in `T` (app.js), a `SCHEMA` entry, and a test case in `validator_test.go`.
* **New dashboard action:** a method on `App`, a `case` in `server.action`, a button calling `act("/api/action/<name>")`.
* **Different chat field names from TikFinity:** change only `tikfinity.ParseChat` and its test.
* **osu! website layout change:** change only `osumeta` (`fetchPage`/`ParsePageJSON`) and its test.

---

## Rules for contributors

Read this before changing anything.

1. **Run `go vet ./...` and `go test ./...` before and after every change.** Add or update tests for any behaviour you change.
2. **Never trigger real side effects on the streamer's PC while testing.** Do not press Next/Play, call `/api/action/next`, or enable preload against the real
   osu! folder unless dry run is on. Use the sandbox above. Do not start or stop the streamer's running `osureq.exe`, TikFinity, tosu or osu!.
3. **Keep the module boundaries.** Only `osumeta` scrapes osu!. Only `tikfinity` knows TikFinity's payload. Only `tosu` knows tosu's payload. Only `platform` calls Win32.
   Only `downloader` performs downloads, and only to whitelisted hosts.
4. **Do not guess third-party field names silently.** If you change what is read from TikFinity or tosu, verify it against a real message (raw event log, or tosu's `/json/v2`) and say so in the commit.
5. **Auto mode is tosu-only by the owner's decision.** Do not add a timer or window-title fallback without asking.
6. **Dashboard performance:** follow the [rendering rule](#dashboard-internals); never re-render whole sections on every push.
7. **UI text must exist in both English and Thai** (`T` table). Follow the existing design tokens.
8. **Security invariants:** keep the 127.0.0.1 bind, the `guard` checks, the host whitelist, `Trusted` before opening files, and `SafeJoin` for extraction.
9. **Windows/encoding pitfalls:** write files as UTF-8 without BOM; a literal U+FEFF inside a `.go` file breaks compilation (use `\ufeff` or bytes).
   Git on Windows converts line endings; `.osz` fixtures are marked binary in `.gitattributes`.
10. **Config changes must stay backward compatible:** new fields need defaults in `Default()`; old `config.json` files must still load.
11. **Commits:** small, descriptive messages explaining *why*. The built exe (`dist/`) is not committed.

---

## Known limitations and open items

| Item | Status / next step |
|---|---|
| Live TikTok chat through TikFinity | field names (`comment`, `uniqueId`, `nickname`) not yet seen on a real stream; check the raw event panel during the first stream |
| Auto mode with real tosu + osu! | verified only with a fake tosu server; confirm state names during real play (stable and lazer) |
| Next → `.osz` opens in osu! | not exercised on the real machine during development; confirm once |
| Global hotkeys | implemented, not exercised |
| TikFinity gift and role fields | follow TikTok's payload names; check the raw event panel for `gift` and `chat` during a stream |
| Official download fallback | implemented, not exercised (needs a logged-in browser) |
| osu! lazer | supported via `open` and tosu, not tested |
| osu.direct no-video URL | `?noVideo=1` is a guess |
| `useOsuDb` / `osu://b/{id}` | experimental, off by default |
| Overlay "now playing" card | rendered and styled, only partly checked visually |
| Ideas | GitHub repo + release builds, update check, auto-detect stable vs lazer from tosu `client`, show tosu's live map on the dashboard, a short session summary (maps played, best result) at the end of a stream |

---

## Troubleshooting

| Problem | Fix |
|---|---|
| "TikFinity not installed" while it is installed | Set **Settings → Connection → TikFinity.exe path**. |
| Yellow "Connecting…" (TikFinity) | TikFinity's WebSocket API is off or uses another port. Check the URL in TikFinity and in Settings. |
| Yellow "Waiting for stream" while live | No TikTok events are reaching TikFinity. Reconnect TikFinity to the account. |
| tosu pill red/yellow | Start tosu. Different port → **Settings → Connection → tosu WebSocket URL**. "osu! not detected" means osu! is not running. |
| Auto mode does not move on | Needs a green tosu pill, and the opened map must actually be **played** (song select alone does not count). Press **Next** to move on by hand. |
| My `!req` was not added | Check the **Rejected** panel (a red pop-up also appears). Usually the per-user limit or cooldown: add yourself to **No-limit users**. |
| Chat requests are ignored | **Requests** on? Status green? Message starts with the command? Turn on **Log raw events** and look at the `chat` event. |
| Every download is "MD5 mismatch" | The map was updated recently and mirrors lag behind. Enable the official source, or set **If every source fails** to "open with warning". |
| Official download times out | Be logged in to osu.ppy.sh in the **default** browser, and let it save to the configured Downloads folder without asking. |
| Next does nothing in osu! | Double-click an `.osz` once so Windows opens it with osu!. Make sure dry run is off. |
| Port already in use | Another copy is running, or change `dashboardPort`. |
| Windows says the app is unsafe / "Windows protected your PC" | That is SmartScreen warning about an **unsigned** program, usually one that was downloaded or received in a zip. Windows Defender finds nothing in it. Click **More info → Run anyway**, or right-click `osureq.exe` → **Properties** → tick **Unblock**. Only a code-signing certificate removes the warning for everyone. |
| Anything else | `osureq.log` next to the exe. |

---

## Changelog

| Version | Commit | Changes |
|---|---|---|
| 1.0.0 | `30f541c` | First version: TikFinity chat, parser, osu! metadata, validation, queue, mirrors + MD5, import modes, preload, dashboard (EN/TH) and overlay, hotkeys, tests |
| 1.0.1 | `854719d` | The map being played no longer counts toward the per-user limit; exempt users; bare number retried as set ID; osu! folder from the registry and `BeatmapDirectory`; pop-up on rejected chat requests |
| 1.1.0 | `0452c08` | Test panel: simulated chat, quick add, dry run, clear test data |
| 1.1.1 | `6f6f7f3` | Dashboard no longer flickers during downloads (incremental rendering) |
| 1.2.0 | `aa3cf23` | Auto mode detects the end of a song with tosu (no timer); tosu becomes a requirement; tosu status, banner and phase line |
| 1.3.0 | `c665402` | Overlay pop-ups for added/rejected requests (with reasons); whole-set requests prefer the main mode |
| | `a4134a8` | Chat commands: `!remove`, and for mods `!skip`, `!remove @user`, `!reqon`, `!reqoff` |
| | `bd94357` | Play results from tosu on requests, history, dashboard and overlay |
| | `bd318e8` | Gifts buy queue priority (opt-in) |
| | `e7cc01a` | System tray app (no console), second launch opens the running copy, Quit button (tray removed again in 1.3.1) |
| | `bb8e34f` | Audio preview and "open in beatmap viewer" links |
| | `02449a6` | One-click bans from queue cards and the rejection log |
| | `eb4b13b` | Followers-only and per-subscriber limits |
| 1.3.3 | | Windows resources (icon, name, description, version, `asInvoker` manifest) and no symbol stripping, so Windows treats the exe as a normal app |
| 1.3.2 | `f5298cf`, `a2e703d` | UX/UI audit fixes: narrow layout, Info box, set mode order, now-playing reason, Thai text; compact status, folded test panel, card "⋯" menu, calmer overlay |
| 1.3.1 | | Back to a console app: the CMD window shows the log (titled `osureq <version>`); no tray icon. Closing the window stops cleanly |

---

Credits: [TikFinity](https://tikfinity.zerody.one/) for TikTok events, [tosu](https://github.com/tosuapp/tosu) for osu! game state,
catboy.best and osu.direct mirrors, and the owner's osu! beatmap viewer for the visual design.
