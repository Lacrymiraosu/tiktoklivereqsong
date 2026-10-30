# osureq: developer guide

A small Windows helper (one `osureq.exe`, ~8 MB, Go, no installer, no runtime) that lets
TikTok LIVE viewers request osu! beatmaps in chat. The helper:

1. reads chat from **TikFinity Desktop**,
2. checks each request against the streamer's rules,
3. downloads it from a mirror,
4. confirms the `.osu` files match what osu! currently serves (MD5),
5. opens the map in osu! when the streamer presses **Next**, or automatically once **tosu** sees the previous song has ended.

A local dashboard (English/Thai) and a transparent OBS overlay run inside the same exe.

> **This guide is also the handoff document.** It is written so that another developer can continue the project
> from it alone. Start with [Project status](#project-status), then [Architecture](#architecture),
> [File structure](#file-structure) and [Rules for contributors](#rules-for-contributors).

Thai: see [README.th.md](README.th.md).

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
| Version | **1.4.0** (`git log` for history, [Changelog](#changelog) below) |
| Language / build | Go 1.23+, `CGO_ENABLED=0`, Windows x64 only |
| Tests | `go test ./...`: 15 packages with tests, all passing (`go test ./internal/...` also runs on Linux; only `main` needs Windows) |
| Repository | <https://github.com/Lacrymiraosu/tiktoklivereqsong>, branch `main`; builds and releases by GitHub Actions |

**Verified for real on the developer's PC:** TikFinity detection, WebSocket connection and `config` handshake;
osu! metadata scraping; catboy downloads; MD5 verification (single diff and whole set); rejections; preload copy;
osu! folder auto-detection from the registry; the dashboard, settings and overlay; simulated chat; dry run;
the anti-flicker rendering, overlay pop-ups, mod commands and gifts through simulated chat, second launch and Quit,
audio preview and one-click bans. Auto mode and play results were verified against a **fake** tosu server.

**Not yet verified with the real thing:** see [Known limitations and open items](#known-limitations-and-open-items)
(live TikTok chat, gift, role and profile-picture fields, real tosu + osu! gameplay, opening `.osz` into osu! via Next, hotkeys, official-download fallback, lazer).
The 1.4.0 features were tested with unit tests and a Linux harness (dashboard and overlay screenshots); song search, the
pre-stream check, the osu.direct fix and the update check were also run against the live services.

---

## Features

* **Chat requests** via TikFinity only (`!req` + aliases, configurable), accepting IDs, every common osu! link form and **song names**
  (`!req freedom dive`, searched on catboy.best and re-ranked by title match and play count).
* **Validation**: star range, max length, modes, ranked statuses, set/mapper/user blacklists, queue size,
  per-user limit (the map being played does not count), cooldown, duplicates, no-repeat window, no-limit (exempt) users.
* **Metadata and reference MD5** scraped from osu.ppy.sh (no API key), with a raw `.osu` fallback.
* **Downloads** from catboy.best → osu.direct → official site (browser + Downloads folder), each optional and reorderable,
  with per-source timeouts, ZIP checks, a cache, size limit and auto-cleanup.
* **MD5 verification** (`requested` or `strict`) against osu!'s current checksums; an outdated mirror copy is rejected.
* **Import** by opening the `.osz` (stable + lazer), copying it into Songs, or extracting it (stable), plus **preload** of the next N maps.
* **Play modes**: manual (**Next** button/hotkey) or **auto**, which uses tosu to see when the song has really ended.
* **All four modes** (osu!, taiko, catch, mania): converts use osu!'s own star rating for the mode they are played in
  (`#mania/<id>` of an osu! map is rated as mania), mania shows its key count (4K/7K), star limits can be set per mode,
  mode tags on the dashboard and overlay, and a warning when osu! is in another mode than the opened map (via tosu).
* **Play results** from tosu (accuracy, rank, combo, misses, pp, mods; passed/failed/quit) on the request, in history and on the overlay.
* **Viewer feedback on the overlay**: since the helper cannot write in TikTok chat, pop-ups say when a request was added (with its place)
  or rejected (with the reason, e.g. "too hard · 5.88★ > 5.00★"), plus mod commands, gifts and results. English or Thai.
* **Chat commands**: `!remove` (own latest request), `!pos` (where are my requests), `!np` (what is playing), `!voteskip` (opt-in),
  and for mods `!skip`, `!remove @user`, `!reqon`, `!reqoff`. Mods = a list, plus TikTok moderators. Answers appear on the overlay.
* **Gift priority** (opt-in): gift coins add up per viewer; each full amount moves their oldest waiting request to the top or is saved for the next one.
* **Gift-only requests** (opt-in): each request costs gift coins; a rejected map costs nothing.
* **Requests with mods** (`!req 2096297 HDHR`, `!req freedom dive +DT`): shown as a tag everywhere; rules for allowed mods and a star cap for DT/NC/HR.
* **Fair queue** (opt-in): a viewer's second request waits behind everyone's first, and so on.
* **Automatic open/close**: requests close at a queue size and reopen when it is short again, and close at a set time.
* **Song and artist bans**: rules like `artist="a_hisa"`, `title="Ghost"` or both; every set of a song (TV size, cut, remaps) is caught, also by Japanese/Thai names.
* **Mod panel** (opt-in): a PIN-protected phone page on the home network for Next, Skip, moving/removing requests and requests on/off.
* **Chat TTS** (opt-in): reads chat aloud in the dashboard and never reads commands (`!req …`), links or emoji; replaces TikFinity's TTS, which cannot skip them.
* **Spoken announcements** (opt-in): "test1 ขอเพลง …", "เพลง … ยากเกินไป 7.25 ดาว", next map, results, gifts, from templates the streamer edits.
* **Support file**: one zip (log, raw TikFinity events, settings, state) to send after a stream with a problem.
* **Viewer roles**: followers-only mode and a higher per-user limit for subscribers (when TikFinity sends these flags).
* **One-click bans** of a requester, map or mapper from a queue card or the rejection log.
* **Audio preview** of queued maps and a link to open them in the owner's **beatmap viewer**.
* **CMD window** with the live log (titled `osureq <version>`); closing it stops the helper cleanly. A second launch opens the running copy's dashboard.
* **Dashboard** (EN/TH): status of TikFinity and tosu, now playing, queue with drag-to-reorder, per-source download
  progress, MD5 result, rejection log, activity, history, raw TikFinity events, full settings editor.
* **Test panel**: simulated chat through the real rules (requests and commands), quick add, simulated gifts, dry run (never touches osu!), clear test data.
* **Pre-stream check**: one button checks TikFinity, tosu, the Songs folder, hotkeys, the overlay connection, osu.ppy.sh and every mirror.
* **Session summary** ("This stream"): maps played and cleared, best play, top requesters; shown on the overlay on request.
* **stable/lazer detection** from tosu, and an **update check** against GitHub releases (a button, never an automatic download).
* **OBS overlay**: transparent page with now playing, requester (with TikTok picture), upcoming queue (priority marked), pop-ups and the session summary (themes, size, corner, language).
* **Global hotkeys**: next, skip, remove last, toggle requests.
* **Security**: 127.0.0.1-only server, Host and header checks, whitelisted download hosts, path-traversal-safe extraction.

Out of scope for now: sending messages to TikTok chat (TikFinity's own WebSocket API does not offer it; see Ideas), macOS/Linux.

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

1. Download `osureq-<version>-windows-x64.zip` from [Releases](https://github.com/Lacrymiraosu/tiktoklivereqsong/releases/latest) and unzip it.
   Put `osureq.exe` in its own folder, for example `C:\osureq\`. It creates `config.json`, `queue.json`, `osureq.log` and a `cache\` folder next to itself.
2. Start TikFinity Desktop and connect it to the TikTok account, then start **tosu** and osu!.
3. Double-click `osureq.exe`. A **CMD window** opens and shows the log; keep it open while streaming (closing it, or Ctrl+C, stops osureq).
   The dashboard opens at <http://127.0.0.1:21280/>. Starting it a second time just opens the dashboard of the running copy.
4. In **Settings → osu!**, choose stable or lazer. The osu! folder is detected from the `.osz` file association if the default one has no Songs folder.
5. Check the two status pills at the top:
   * **TikFinity**: 🔴 not installed/not running · 🟡 not connected, or connected and "Waiting for stream" · 🟢 live and taking requests.
   * **tosu**: 🔴 not running · 🟡 not connected or osu! not detected · 🟢 reading osu!. The first time tosu is seen running, its path is remembered so the banner can offer **Launch tosu**.
     tosu launched by osureq opens in its own console window.
   * To stop things, the **Info** box has **Close tosu**, **Close TikFinity**, **Quit osureq** and **Close everything** (all three).
     A program closed this way is not auto-launched again until you launch it from the dashboard or start it yourself.
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
!req freedom dive                                         ← a song name (command.searchByName, on by default)
!req 2096297 HDHR                                         ← with mods (also +HDHR, "hd hr", "hd,dt")
!req freedom dive +DT                                     ← a song name with mods: "+" (or capitals) at the end
```

Mods (`internal/mods`): NF EZ HD HR SD DT HT NC FL SO PF FI MR, written back in osu!'s order (`HDHR`, `HDDT`). NC includes DT and PF includes SD.
After an ID or link, mods are read when everything after it is mods in any case (`hd hr`), or when the first word starts with `+` or
is in capitals made only of mod codes (`HDDT pls`); other words ("please", "so good", "OK") are ignored as before. Clashes (EZ+HR,
HT+DT/NC, NF+SD/PF, FI+HD, FL+FI) and unknown codes in a `+` word are rejected as `bad_mods`. A lower-case word at the end of a song
name is part of the name.

An ID or osu! link in the first word wins and anything after it is ignored. Otherwise the text (3-80 characters) is a song name;
anything that looks like a link to another site is never searched and is ignored, as is everything when search is off.

Other chat commands (names are configurable; an empty name turns one off):

| Command | Who | What it does |
|---|---|---|
| `!remove` | anyone | removes your own latest waiting request |
| `!remove @user` | mods | removes that user's latest waiting request |
| `!skip` | mods | skips the song being played (opens the next one) |
| `!reqon` / `!reqoff` | mods | opens / closes requests |
| `!pos` | anyone | the overlay shows where the viewer's requests are (`#2, #5`); once per viewer every 20 s |
| `!np` | anyone | the overlay shows the song being played and who asked for it; once per viewer every 20 s |
| `!voteskip` | anyone | one vote per viewer per song; skips when `command.voteSkipVotes` different viewers voted (0 = off, the default) |

Mods are the users in **Settings → Command → Mods**, plus TikTok moderators when TikFinity sends that flag (`command.trustTikTokMods`).

---

## How a request is handled

1. **Parse** (`internal/parser`): the chat message must start with the command or an alias, followed by an ID, a link or a song name (`KindSearch`).
2. **User checks** (`validator.CheckUser`, no network): blacklisted user, queue full, followers-only, per-user limit (higher for subscribers if set), cooldown.
   Exempt users skip everything after "queue full". With gift-only requests a paid request is taken here (and given back if the map is rejected later).
3. **Metadata** (`internal/osumeta`): `GET https://osu.ppy.sh/beatmapsets/{setId}` (or `/beatmaps/{id}`, which redirects to the set) and parse
   the `<script id="json-beatmapset">` JSON: title, artist, mapper, status, video flag, and per difficulty its id, mode, stars, length and
   `checksum` (MD5 of the `.osu`). Fallback: `GET https://osu.ppy.sh/osu/{beatmapId}` and hash it locally. Stars, length and status are
   then unknown, so they are not checked and the item gets a warning. A bare number that is not found is retried with the other meaning (beatmap ↔ set).
   A **song name** is searched first (`internal/search`, see below); the first three results are looked up and checked by the map rules in turn,
   and the first that passes is taken as a whole-set request. If none passes, the first one's rejection is used with its title in front of `short`.
4. **Map rules** (`validator.CheckMap`): set/mapper/song blacklist, status, duplicate (queued or playing), no-repeat window, then mode, stars and length
   on the requested diff. A `#taiko/#fruits/#mania` link to an osu! diff is a **convert**: it is checked with osu!'s rating for that mode
   (`converts` in the page JSON; unknown stars when osu! has none). Star limits come from `rules.modeStars` for that mode when set, else
   `minStars`/`maxStars`. For whole-set requests, native diffs are tried mode by mode in the order of `rules.modes` (osu! first by default)
   and the hardest one that passes is chosen.
5. **Queue** (`internal/queue`): saved to `queue.json` and kept across restarts. With `queue.fair` the request goes to the end of its owner's round
   (`Queue.AddFair`: a viewer's n-th waiting request is in round n; priority requests stay on top), else to the end. A saved gift priority is applied right away (see [Gifts](#gift-priority)).
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
`bad_input`, `requests_off`, `followers_only`, `now_playing`, `no_convert` (a taiko/catch/mania map requested in another mode: only osu! maps convert),
`search_none`, `search_error`, `needs_gift`, `song_blacklisted`, `artist_blacklisted`, `bad_mods`, `mods_off`, `mod_not_allowed`, `mod_stars`.

**Mods and stars:** star ratings with mods are only available from osu!'s authenticated API (catboy's `/api/meta` says it is not implemented,
osu.direct has none), so every star check uses the real **no-mod** rating. `rules.hardModMaxStars` is a separate no-mod cap for requests with
DT, NC or HR. The length limit uses the real speed (DT/NC ÷1.5, HT ÷0.75). osu! cannot be told which mods to use: they are shown, and when
tosu's result lacks a requested mod, `Result.missing` is set and Activity warns.

Song rules (`rules.blacklistSongs`, parsed by `config.ParseSongRule`, one per line):

| Rule | Bans |
|---|---|
| `artist="a_hisa"` | every song by that artist (`artist_blacklisted`) |
| `title="Ghost"` | that song by anyone |
| `artist="xi" title="Blue Zenith"` or `xi - Blue Zenith` | that song by that artist |
| `Blue Zenith` | a title by anyone |

Keys are case-insensitive; quotes may be straight, curly (`“…”`, as phones type them) or left out for one word; `artist=""` or other text
after the pairs is refused when saving (`song_rule`). Titles are compared after `validator.SongKey` (lower case, letters and digits only,
trailing version tags such as `(TV Size)`, `[Extended Mix]`, `～Short ver.～`, `-Cut Ver.-` removed). Artists match as whole words in a row,
so `artist="xi"` also bans "xi remixed by cosMo" but not "xinxin". Both are tried against `artist`/`title` and `artist_unicode`/`title_unicode`
from the osu! page (**verified** field names).

### Automatic open/close

`app.autoRequests` runs on every tick (2 s and on queue changes). It closes requests when the waiting count **reaches** `queue.autoCloseAt`
(on the change only), reopens them at `queue.autoReopenAt` or fewer **only if it closed them**, and closes them once when the clock passes
`queue.closeAt`. Opening or closing by hand always wins. The overlay gets a `status` pop-up (`auto_off`, `auto_on`, `time_off`). Each rejection also has a language-neutral `short` detail (e.g. `5.88★ > 5.00★`, `60s`, `2/2`) used by the overlay.

### Gift priority

Off by default (`gifts.enabled`). TikFinity `gift` events are counted per viewer (streakable gifts once, when the streak ends: `repeatEnd`).
Every full `gifts.coinsPerPriority` coins (default 30) gives one priority: the viewer's oldest waiting request moves to the top, behind earlier
priority requests; with nothing waiting, it is saved for their next request for `gifts.creditMinutes`. Priority requests show ★ on the dashboard and overlay.

**Gift-only requests** (`gifts.requestCoins` > 0, works with or without priority): the same gift coins also add up toward paid requests, one per
full `requestCoins`, kept for `gifts.creditMinutes`. A request without one is rejected as `needs_gift` (`short`: coins so far, e.g. `10/30 coins`).
No-limit users and dashboard quick adds are free.

### Session summary

`queue.json` keeps `sessionStart` and a count of real rejections. A start after more than 6 idle hours (or **New session**) begins a new session.
`queue.Summary()` counts real (not test) history entries since then: played, skipped, passed, viewers, the best passed play (highest pp, then
accuracy) and the top three requesters. **Show on overlay** bumps `summaryShown` in the state; the overlay shows the card for 20 s when it changes.

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
* **Gift** (`POST /api/simulateGift`): pretends `@user` sent a gift worth N coins (gift priority or gift-only requests must be on).
* **Pre-stream check** (`POST /api/preflight`, button next to Dry run): see [Pre-stream check](#pre-stream-check).
* Chat commands (`!skip`, `!remove`, …) can be typed in Simulate chat too; mods are checked against the Mods list.
* **Dry run** (`test.dryRun`): Next/Play move the queue (auto mode runs too) but never open osu! or copy into Songs. A `DRY RUN` badge shows. Downloads and MD5 checks still happen.
* **Support file** (`POST /api/support`, Info box): a zip with the end of `osureq.log` (2 MB, plus `.old` if short), `tikfinity-events.json`
  (first event of each kind, the first command chat and the first counted gift, and the recent events when logged), `config.json` (mod panel
  PIN hidden) and `state.json`. The Windows home folder is replaced by `~` everywhere (`internal/support`).
* **Clear test data**: removes test requests (also from now playing), their history and rejections, cooldowns set by simulated chat, and test gift credits.

### Chat TTS

TikFinity's TTS has no way to skip commands (its comment filter is "all messages" or "only messages starting with `.` or `/`"), so osureq
can read chat itself (`tts.enabled`). `app.speakChat` runs for every chat event (and simulated chat): messages starting with `!` are never
queued; blacklisted users, `tts.blockedWords`, `tts.allow` and the per-viewer `tts.cooldownSec` filter the rest. `tts.allow` lists the groups
whose chat is read (any one is enough, empty = everyone, no-limit users always): `subscriber` (ใจส้ม, the TikTok LIVE subscription; TikFinity's
`isSubscriber`), `follower` (`followRole`/`isFollower`), `gifter` (sent a gift since the session started; kept in memory) and `mod`. A missing
subscriber or follower flag counts as no, with one Activity warning (`ttsSubUnknown`, `ttsFollowUnknown`); the old `followersOnly` is read as
`allow: ["follower"]`. `SpeakableText` removes links, emoji (symbol categories, joiners, skin tones, variation selectors) and runs longer than four of the same
character, and cuts at `tts.maxLength`. The last 20 go out in the state as `speech` (newest first, increasing `id`).

The dashboard speaks them with the Web Speech API (`speechSynthesis`, i.e. the voices of Windows and the browser; Edge has natural ones).
Browsers refuse to speak without a user gesture, so the top-bar **Start TTS** button starts it per tab; only tabs where it was clicked speak,
nothing from before the click is read, and at most five messages wait. With `tts.lang` `auto`, Thai letters pick a Thai voice, else English
(natural/online voices first); `tts.voice` forces one. OBS records it as desktop audio. Streamers should turn TikFinity's TTS off.

**Announcements** (`tts.announce`, `app/announce.go`): `App.emit` passes every feed event to `announce`, which picks the template
(`tts.templates[event]`; a rejection uses `rejected_<code>` if that key exists, else `rejected`), fills it (`RenderAnnouncement`: unknown or
empty placeholders vanish, `@` is dropped because voices say "at") and queues it as a `say` speech item. Events (`config.AnnounceEvents`):
`added`, `rejected`, `rejected_stars_high|stars_low|mod_stars|too_long|duplicate|recently_played|cooldown|user_limit|search_none`, `nowplaying`
(from `play`), `np` / `np_none` / `pos` (the `!np` and `!pos` answers; default `กำลังเล่น {artist} - {title} {setid}`), `result_passed|result_failed`, `gift` (once per gift, from `gift`), `voteskip`, `auto_off` (also the closing time), `auto_on`.
Empty or `-` is silent. Placeholders: `{user}` (display name) `{title}` `{artist}` `{mapper}` `{diff}` `{mods}` (spaced: "HD HR") `{stars}`
`{pos}` `{setid}` (`{setid_digits}`: "3 9 8 0 4", read digit by digit); `!np` adds `{requester}`; rejections add `validator.Rejection.Vars` (`{max}` `{min}` `{length}` `{wait}` `{n}` …) and `{reason}` (Thai when the template has
Thai letters, else English); results `{acc}` `{rank}` `{combo}` `{misses}` `{pp}`; gifts `{coins}` `{gift}`. Defaults are
`config.ThaiAnnouncements()`; the settings tab can load those or `EnglishAnnouncements()` (sent with `GET /api/config` as `announcements`).
A `config.json` from an older version gets the lines added since then in the language of its own `added` line (`TTS.fillTemplates`).

### Pre-stream check

`app.Preflight` only reads; nothing is stored, opened or changed. Each line is `ok`, `warn`, `fail` or `off`, problems first:

| Check | Fails / warns when |
|---|---|
| TikFinity | not installed, not running or not connected (connected and waiting for the stream is fine) |
| tosu | not running or not connected; warns when osu! is not open |
| osu! | stable: no Songs folder in `osu.folder`; lazer needs none |
| Hotkeys | enabled but one could not be registered (`off` when disabled) |
| Overlay | no overlay connected (the overlay opens `/ws?client=overlay`, which the server counts) |
| Requests, dry run, test data, update | warns when requests are off, dry run is on, test requests are queued, or a newer release exists |
| osu! website | `osumeta` cannot load set 39804 |
| Mirrors | each enabled mirror (not the official site) is asked for the first 1 KB of set 39804 (`Downloader.Probe`); one working mirror is enough, the others then only warn |

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
* `update.Run`: one goroutine; checks GitHub 10 s after start and then every 12 h (skipped for dev builds).

**State ownership:** `queue.Queue` (mutex-guarded) is the single source of truth for requests; `config.Store` for settings
(subscribers via `OnChange`). The UI never holds state of its own beyond view preferences in `localStorage`.

**Dependency direction:** `main` → `app` → (`parser`, `osumeta`, `search`, `validator`, `queue`, `downloader`, `verifier`, `importer`, `tikfinity`, `tosu`, `update`) → `config`.
`server` depends on `app`. Only `main` imports `platform` (Windows APIs are injected into `tikfinity`, `tosu`, `downloader`, `importer` as functions,
which keeps those packages testable).

---

## File structure

```
tiktoklivereqsong/
├── main.go                      entry point: flags, logging, wiring, osu! folder detection, hotkeys, startup
├── go.mod / go.sum              module github.com/Lacrymiraosu/tiktoklivereqsong (Go 1.23)
├── build.ps1                    vet + test + build → dist\osureq.exe
├── rsrc_windows_amd64.syso      Windows resources linked into the exe (generated by tools/winres)
├── tools/winres/main.go         generates the .syso: logo icon, version info, manifest
├── README.md                    short user guide (English)
├── docs/DEVELOPMENT.md          this file (developer guide + handoff doc)
├── docs/README.th.md            user guide in Thai
├── .github/workflows/build.yml  GitHub Actions: vet, test, build; a v* tag publishes a release
├── LICENSE                      MIT, © 2026 kikinomae
├── THIRD_PARTY_LICENSES.txt     licenses of the Go standard library, x/sys and coder/websocket (shipped in the release zip)
├── .gitignore                   ignores dist/, cache/, config.json, queue.json, logs
├── .gitattributes               *.osz binary (keeps test fixtures byte-exact on Windows)
├── dist/                        (ignored) built exe; when run from here also holds config/queue/cache/log
└── internal/
    ├── app/          app.go                orchestration: request pipeline, preload, play/next/skip, auto mode, results, test panel
    │                 feed.go               overlay pop-up events (FeedEvent)
    │                 commands.go (+_test)  chat commands: !skip, !remove, !reqon, !reqoff; who is a mod
    │                 gifts.go              gift coins → priority, simulated gifts
    │                 ban.go (+_test)       one-click bans (blacklist + drop matching requests)
    │                 preflight.go (+_test) pre-stream check
    │                 results_test.go       tosu → play result tests, stable/lazer detection
    │                 search_test.go        song-name and gift-only requests against a fake catboy + osu!
    ├── config/       config.go             config.json schema, defaults, validation, store with change subscribers
    ├── parser/       parser.go (+_test)    "!req …" → Request{Kind, SetID, BeatmapID, Mode, Bare}
    ├── osumeta/      osumeta.go (+_test)   osu.ppy.sh scraping: set metadata, reference MD5, beatmap→set, raw .osu fallback
    ├── mods/         mods.go (+_test)      mod parsing, clashes, speed, "played without"
    ├── search/       search.go (+_test)    catboy.best song search, re-ranking by title match and plays
    │   └── testdata/                       a real (trimmed) search response
    ├── update/       update.go (+_test)    GitHub "latest release" check
    ├── support/      support.go (+_test)   the support zip
    ├── remote/       remote.go (+_test)    mod panel: second listener, PIN sessions, private-network guard
    │   └── web/mod.html                    the phone page (embedded)
    ├── validator/    validator.go (+_test) CheckUser / CheckMap → *Rejection{Code, Detail}
    ├── queue/        queue.go (+_test)     items, now playing, history, rejections, cooldowns; queue.json persistence
    │                 gifts.go (+_test)     gift credits, priority tokens, prioritise, paid requests
    │                 summary.go (+_test)   stream session and its summary
    ├── downloader/   downloader.go (+_test) sources, timeouts, ZIP sniffing, official browser flow, cache, cleanup
    ├── verifier/     verifier.go (+_test)  in-memory unzip + MD5 compare (strict / requested)
    │   └── testdata/                       .osz fixtures (regenerate with -update)
    ├── importer/     importer.go, osudb.go (+_test)  open / copy / extract, Songs dir, safe paths, osu!.db hash scan
    ├── tikfinity/    tikfinity.go (+_test) TikFinity detection + WebSocket client + chat parsing
    ├── tosu/         tosu.go (+_test)      tosu WebSocket client + song-end Tracker
    ├── platform/     platform_windows.go, hotkey.go (+_test)  Win32: ShellExecute, processes, registry, hotkeys
    └── server/       server.go (+_test)    HTTP routes, guard, state snapshot, WebSocket push, pre-stream check endpoint
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
| `internal/app/preflight.go` | ~180 | The pre-stream check | `Preflight`, `PreflightEnv`, `Check` |
| `internal/search/search.go` | ~230 | catboy.best search (status filter, 1 request/s, 10 min cache), `Playable` (mode filter incl. converts), `Rank` | `Client`, `Find`, `Parse`, `Rank`, `Result` |
| `internal/update/update.go` | ~130 | Release check: `Newer` compares x.y.z, dev builds and pre-releases are never offered, links only to this repo's releases | `Run`, `Latest`, `Newer`, `Release` |
| `internal/mods/mods.go` | ~120 | Mod words → canonical string, clash rules, `Rate`, `Harder` (DT/NC/HR), `Missing` | `Parse`, `Has`, `Rate`, `Harder`, `Missing`, `Error` |
| `internal/remote/remote.go` | ~440 | Mod panel: `Apply` starts/stops the listener on config changes; `guard` (private addresses, local Host names, header, CSP); PIN login with lockouts; limited state and actions | `Server`, `Apply`, `Status`, `Handler` |
| `internal/support/support.go` | ~130 | Support zip, home-folder redaction, log tail | `Write`, `Input` |
| `internal/queue/summary.go` | ~110 | Session start (kept across restarts, new after 6 idle hours) and the summary | `Summary`, `NewSession` |
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
| `internal/platform/platform_windows.go` | ~280 | `ShellExecuteW` open, Toolhelp process list, process image path, TikFinity lookup (path + Uninstall registry), osu! stable lookup (`HKCR\osustable.File.osz\shell\open\command`), launch in a new console window, close by exe name (`WM_CLOSE`, then `TerminateProcess` after 3 s), `RegisterHotKey` message loop, console window title | `ShellOpen`, `ProcessRunning`, `ProcessPath`, `FindTikFinity`, `FindOsuStable`, `Launch`, `Close`, `Hotkeys`, `SetConsoleTitle` |
| `internal/platform/hotkey.go` | ~50 | Parses `Ctrl+Alt+N`, `Shift+F5`, `F9`, `Win+Num5`… into modifier + virtual key | `ParseHotkey` |
| `internal/server/server.go` | ~420 | Routes, `guard` (Host + `X-Osureq` header), `State` snapshot, coalesced WebSocket push + 20 s keepalive state, config GET/PUT, actions (incl. quit and closing tosu/TikFinity), queue operations, test/simulate/simulateGift, ban | `Server`, `State`, `Handler`, `Push`, `Listen`, `Quit` |
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
* Profile picture (**unverified**, `tikfinity.Avatar`): `data.profilePictureUrl` (TikTok-Live-Connector's name, same author as TikFinity), else
  `data.user.profilePictureUrl`, `data.userDetails.profilePictureUrls[]`, `data.user.profilePicture.url[]`, `data.user.avatarThumb.urlList[]`.
  Only `https` URLs on `*.tiktokcdn.com`, `*.tiktokcdn-us.com`, `*.tiktokcdn-eu.com` and `*.ibyteimg.com` are kept.
* Gifts (**unverified**): `{"event":"gift","data":{"uniqueId","nickname","giftName","giftType","repeatCount","repeatEnd","diamondCount"}}`.
  Coins = `diamondCount × repeatCount`; for `giftType` 1 (streakable) only the event with `repeatEnd: true` counts.
* The dashboard keeps the first event of every type, and with `logRawEvents` the last 100 events, so field names can be checked without a debugger.

### tosu (`internal/tosu`)

* WebSocket `ws://127.0.0.1:24050/websocket/v2` (configurable); tosu pushes the full state every poll (default 100 ms). Only changes are forwarded to the app.
* Fields used: `client` (`stable`/`lazer`: tosu's `ClientType` enum sent by name, **verified** in `packages/common/enums/tosu.ts`; with
  `osu.detectGameTosu` on it switches `osu.game`), `state.name` (or `state.number`), `beatmap.set`, `beatmap.id`, `beatmap.checksum`,
  `play.{failed, accuracy, score, combo.max, rank.current, pp.current, mods.name, hits."0"}` and
  `resultsScreen.{accuracy, score, maxCombo, rank, pp.current, mods.name, hits."0"}` and `settings.mode` (0-3: the mode selected in osu!,
  used for the mode-mismatch warning) (types from `packages/tosu/src/api/types/v2.ts`). A payload with `error` is ignored.
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
| osu.direct | `https://osu.direct/api/d/{id}` | `https://osu.direct/api/d/{id}?noVideo=1` | answers with a 302 to a signed `storage.osu.direct` URL (verified); the no-video parameter is a best guess |
| official | `https://osu.ppy.sh/beatmapsets/{id}/download` | `…?noVideo=1` | opened in the browser; the file is picked up from Downloads |

nerinyan is deliberately not used (it was not working). Allowed hosts: `catboy.best`, `osu.direct`, `storage.osu.direct`, `osu.ppy.sh`
(`config.AllowedHosts`). Every redirect hop is checked against the list (`CheckRedirect`), not only the final URL.

### catboy.best search (`internal/search`)

* `GET https://catboy.best/api/v2/search?q=<text>&limit=50&status=<n>…` with osu!'s numeric statuses (`graveyard -2, wip -1, pending 0,
  ranked 1, approved 2, qualified 3, loved 4`) from `rules.statuses`. Answers a JSON array of beatmapsets (`id`, `artist`, `title`,
  `creator`, `status`, `play_count`, `beatmaps[].mode`). **Verified** against the live API; a trimmed real response is the test fixture.
* catboy refuses requests without an identifying User-Agent. `sort` is ignored by the API and its own order favours obscure uploads and
  tag matches, hence `Rank`: title contains every word → artist + title contains every word, each by play count; results that match only
  on tags are dropped (so a request answers "no map found" rather than queueing an unrelated map). `artist_unicode`/`title_unicode` count too.

### GitHub releases (`internal/update`)

* `GET https://api.github.com/repos/Lacrymiraosu/tiktoklivereqsong/releases/latest` (`tag_name`, `html_url`, `draft`, `prerelease`), unauthenticated
  (60 requests per hour per IP; osureq uses two a day). Only `html_url` under this repo's releases page is passed on.

### Windows

* `.osz` association `HKCR\osustable.File.osz\shell\open\command` → osu! stable folder.
* `osu!.<WindowsUser>.cfg` → `BeatmapDirectory` (relative to the osu! folder, or absolute).
* `ShellExecuteW("open", file)` for `.osz`, URLs and `osu://b/{id}`.

---

## Local HTTP / WebSocket API

Server: `http://127.0.0.1:<dashboardPort>` (default 21280). Requests must have `Host` = `127.0.0.1:<port>` or `localhost:<port>`.
Every non-GET request must send the header `X-Osureq: 1`. Responses are JSON; actions return `{"ok": true}` or `{"ok": false, "error": "…"}`.
Errors meant for the streamer also carry `code` and `vars` (from `app.UserError` or `config.ValidationError`); the dashboard shows `T["e_" + code]` in its language and falls back to `error`.

| Method & path | Purpose |
|---|---|
| `GET /` | dashboard |
| `GET /overlay` | OBS overlay |
| `GET /assets/{file}` | embedded `app.js`, `app.css`, `icon.svg` |
| `GET /ws` | WebSocket; pushes the full `State` JSON on every change (and every 20 s). The overlay adds `?client=overlay` so the pre-stream check can count it |
| `GET /api/state` | the same `State` once |
| `GET /api/config` | `{config, defaults}` |
| `PUT /api/config` | replace the whole config (validated); `{ok, restartNeeded}` |
| `POST /api/action/{name}` | `next`, `skip`, `removeLast`, `toggleRequests`, `clear`, `cleanup`, `toggleDryRun`, `clearTest`, `launchTikfinity`, `reconnect`, `openTikfinityDownload`, `launchTosu`, `reconnectTosu`, `openTosuDownload`, `stopTosu`, `stopTikfinity`, `quit`, `quitAll` (closes tosu and TikFinity, then quits), `showSummary`, `newSession` |
| `POST /api/queue/{id}/{op}` | `remove`, `skip`, `play`, `retry`, `move` (body `{"to": <index>}`) |
| `POST /api/test` | quick add: `{"input": "<id or link>", "user": "<name>"}` → `{ok, item}` or `{ok:false, code, error}` |
| `POST /api/simulate` | simulated chat: `{"message": "!req …", "user": "<name>"}`; `code: "ignored"` when it is not a request; for a chat command `{ok, command: "cmd_done" \| "cmd_not_mod" \| "cmd_nothing"}` |
| `POST /api/simulateGift` | `{"user": "<name>", "coins": 30}` → `{ok, earned, requests}` (priorities and paid requests earned) |
| `POST /api/support` | the support zip (`application/zip`, attachment) |
| `POST /api/preflight` | runs the pre-stream check (a few seconds) → `{ok, checks: [{id, status, key, vars, detail}]}` |
| `POST /api/volume` | `{"volume": 0-100}` sets the master volume |
| `POST /api/ban` | `{"kind": "user" \| "set" \| "mapper", "value": "…"}` → `{ok, removed}` |

`State` fields: `version`, `tikfinity` (status), `tosu` (status incl. `game`), `autoGap`, `queue` (`items`, `now`, `history`, `rejected`),
`requests`, `resolving`, `notices`, `feed` (overlay pop-up events), `cacheMb`, `overlay`, `command`, `search`, `requestCoins`, `lang`, `playMode`, `viewerUrl`, `blacklistUsers`, `dryRun`,
`voteSkip` (`votes`, `needed`), `tts` (settings), `speech` (chat to read), `update` (`version`, `url`, only when newer), `summary`, `summaryShown`, `raw`, `firstRaw`,
`extra` (`hotkeyErrors`, `baseDir`, `cacheDir`).

**Mod panel** (separate listener, `remote.port`, default 21281, only when `remote.enabled`): `GET /m` (page), `POST /m/login` `{"pin"}`
(sets the `osureq_mod` cookie; `401 wrong_pin`, `429 locked`), `POST /m/logout`, `GET /m/api/state` (now playing, queue, requests, TikFinity/tosu
level, votes), `POST /m/api/action/{next|skip|toggleRequests}`, `POST /m/api/queue/{id}/{remove|skip|move}`. Everything else answers 404.

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
* **i18n:** every UI string is a key in the `T` table: `key: ["English", "Thai"]`. Use `t("key", {vars})`; static HTML uses `data-t` / `data-tp`. Add both languages.
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
| | `masterVolume` | `10` | Master volume (0-100 %) for every sound the dashboard plays (map previews); also the speaker button in the top bar |
| connection | `tikfinityUrl` | `ws://127.0.0.1:21213/` | TikFinity WebSocket API (must be on this PC) |
| | `dashboardPort` | `21280` | Dashboard/overlay port (applies after restart) |
| | `reconnectSec` | `5` | Retry interval for TikFinity and tosu |
| | `autoLaunchTikfinity` | `false` | Start TikFinity if it is installed but not running; paused after **Close TikFinity** until it is started again |
| | `tikfinityPath` | `%LOCALAPPDATA%\Programs\tikfinity\TikFinity.exe` | Also found through the registry |
| | `openDashboardOnStart` | `true` | |
| | `logRawEvents` | `false` | Show every TikFinity message in the dashboard |
| | `tosuUrl` | `ws://127.0.0.1:24050/websocket/v2` | tosu's v2 WebSocket (must be on this PC) |
| | `tosuPath` | empty | Filled in automatically the first time tosu is seen running |
| | `autoLaunchTosu` | `false` | Start tosu if it is not running (needs `tosuPath`); paused after **Close tosu** until tosu is started again |
| | `checkUpdates` | `true` | Ask GitHub twice a day for a newer release; shows a button, never downloads |
| osu | `game` | `stable` | `stable` or `lazer` |
| | `detectGameTosu` | `true` | Switch `game` to the client tosu reports |
| | `folder` | `%LOCALAPPDATA%\osu!` | osu! stable folder. If it has no Songs folder, the folder from the `.osz` association is used at startup and saved |
| | `cacheFolder` | `cache` | Relative to the exe (or `-dir`) |
| | `downloadsFolder` | `%USERPROFILE%\Downloads` | Watched by the official source |
| | `useOsuDb` | `false` | Experimental (stable): if `osu!.db` already has the map, skip the download and open `osu://b/{id}` |
| | `viewerUrl` | `https://osu-beatmap-viewer.vercel.app/` | Beatmap viewer for the "viewer ↗" link; empty hides it |
| command | `name` / `aliases` | `!req` / `!request`, `!r` | |
| | `enabled` | `true` | Requests on/off (also the switch in the top bar) |
| | `bareNumberAs` | `beatmap` | `beatmap` or `set` (the other meaning is tried if not found) |
| | `searchByName` | `true` | Text that is not an ID or link is searched as a song name |
| | `mods` | empty | TikTok users who may use mod commands |
| | `trustTikTokMods` | `true` | TikTok moderators count as mods (when TikFinity sends the flag) |
| | `skipCmd` / `removeCmd` / `openCmd` / `closeCmd` | `!skip` / `!remove` / `!reqon` / `!reqoff` | empty turns a command off |
| | `posCmd` / `nowCmd` / `voteSkipCmd` | `!pos` / `!np` / `!voteskip` | viewer commands; empty turns one off |
| | `voteSkipVotes` | `0` | Different viewers needed to skip by vote; 0 = vote skip off |
| rules | `minStars` / `maxStars` | `0` / `0` | 0 = no limit |
| | `modeStars` | empty | Per-mode star limits, e.g. `{"mania": {"min": 0, "max": 4}}`; 0 keeps the overall value |
| | `maxLengthSec` | `0` | 0 = no limit |
| | `modes` | all four | `osu`, `taiko`, `fruits`, `mania` |
| | `statuses` | ranked, approved, qualified, loved | plus `pending`, `wip`, `graveyard` |
| | `blacklistSets` / `blacklistMappers` / `blacklistUsers` | empty | |
| | `blacklistSongs` | empty | Song rules: `artist="…"`, `title="…"`, both, `Artist - Title` or a title (see [How a request is handled](#how-a-request-is-handled)) |
| | `exemptUsers` | empty | TikTok users who skip followers-only, the per-user limit and cooldown |
| | `allowMods` | `true` | Requests may carry mods |
| | `bannedMods` | empty | Mods that may not be requested, e.g. `["FL"]` |
| | `hardModMaxStars` | `0` | No-mod star cap for requests with DT/NC/HR; 0 = only the normal limit |
| | `followersOnly` | `false` | Reject viewers TikTok reports as not following (unknown is let through) |
| | `subscriberPerUserLimit` | `0` | Per-user limit for subscribers; 0 = same as everyone |
| queue | `maxSize` | `30` | 0 = no limit |
| | `perUserLimit` | `2` | Maps **waiting** per user (the one playing does not count) |
| | `cooldownSec` | `60` | Per user |
| | `noRepeatMin` | `60` | Block maps played within this many minutes |
| | `playMode` | `manual` | `manual` or `auto` (tosu) |
| | `autoGapSec` | `5` | Seconds after the song ends before the next map opens |
| | `fair` | `false` | New requests take turns between viewers |
| | `autoCloseAt` / `autoReopenAt` | `0` / `0` | Close requests when this many are waiting (0 = off); reopen at this many or fewer (must be lower) |
| | `closeAt` | empty | `HH:MM` local time: close requests once when the clock passes it |
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
| | `avatars` | `true` | Viewers' TikTok pictures next to their names (when TikFinity sends them) |
| gifts | `enabled` | `false` | Gifts buy queue priority |
| | `coinsPerPriority` | `30` | Coins (added up per viewer) for one priority |
| | `creditMinutes` | `60` | How long an unused priority (or paid request) waits for a request |
| | `requestCoins` | `0` | Gift-only requests: coins per request; 0 = requests are free |
| remote | `enabled` | `false` | Mod panel for a phone on the same network |
| | `port` | `21281` | Its port (not the dashboard port) |
| | `pin` | empty | 6-12 digits, not all the same, not `123456`/`987654`-style; required when enabled (the dashboard generates one) |
| tts | `enabled` | `false` | Read chat aloud in the dashboard, without commands |
| | `lang` / `voice` | `auto` / empty | `auto`, `th` or `en`; a voice name forces that voice |
| | `rate` / `volume` | `1` / `80` | Speed 0.5-2, volume 0-100 |
| | `readName` | `true` | "name says …" |
| | `maxLength` / `cooldownSec` | `120` / `3` | Letters per message (10-500); seconds between a viewer's read messages |
| | `allow` | empty | Whose chat is read: `subscriber` (ใจส้ม), `follower`, `gifter`, `mod`; any one; empty = everyone |
| | `blockedWords` | empty | |
| | `announce` | `false` | Speak what happens to requests |
| | `templates` | Thai set | Event → line with placeholders (see [Chat TTS](#chat-tts)); empty or `-` = silent |
| hotkeys | `enabled` | `false` | Global hotkeys (work while osu! has focus) |
| | `next` / `skip` / `removeLast` / `toggleRequests` | `Ctrl+Alt+N` / `Ctrl+Alt+S` / `Ctrl+Alt+Backspace` / `Ctrl+Alt+R` | |
| test | `dryRun` | `false` | Next/Play never open osu! or touch Songs |

---

## Runtime files

All live in the base folder (next to the exe, or `-dir`):

| File | Written by | Contents |
|---|---|---|
| `config.json` | `config.Store` (atomic) | settings; safe to edit while stopped |
| `queue.json` | `queue.Queue` (debounced, atomic) | queue items, now playing, history, rejections, last request per user, simulated-chat users, gift credits, session start |
| `cache\{setId}.osz`, `cache\{setId}n.osz` | downloader | verified archives (`n` = no video) |
| `osureq.log` (+ `.old`) | `log` | everything important: status changes, queued/rejected requests, now playing, tosu phases, errors |

---

## Security

* The dashboard server listens on `127.0.0.1` only. Requests with another `Host` header are refused (DNS rebinding). State-changing calls need `X-Osureq: 1`,
  which other websites cannot send without a CORS preflight that is never answered. The WebSocket accepts same-origin connections only.
* Only numeric IDs and links on osu.ppy.sh are fetched directly; song names go to catboy's search API only. Downloads go only to whitelisted
  HTTPS hosts, and every redirect hop is checked, so no request is sent elsewhere.
* Viewer pictures are used only when they are `https` URLs on TikTok's CDN hosts, and are escaped in the overlay.
* The update check only reads GitHub's release list and links to this repository's releases page; it never downloads or runs anything.
* Only `.osz` files that this run of the helper downloaded and verified are opened or imported (`Downloader.Trusted`). After a restart, queued items
  go back through the cache check.
* File names are sanitised; extraction refuses absolute paths, drive letters, `..` and symlinks.
* No osu! OAuth secret, password or API key is used or stored.
* **Mod panel** (`internal/remote`, off by default) is the only listener on other interfaces. It refuses clients that are not on a private
  network (loopback, RFC 1918, link-local, IPv6 ULA, `100.64.0.0/10`) and `Host` headers that are not an IP, `localhost`, `*.local` or the PC's
  name (DNS rebinding). Login needs the PIN (SHA-256 compared in constant time); 5 wrong tries from one address or 20 in total lock it for
  10 minutes. Sessions are random 32-byte tokens in an HttpOnly, SameSite=Strict cookie, valid 12 h and ended by a PIN change; every POST
  also needs `X-Osureq: 1`. It offers only queue actions. It is plain HTTP, so the PIN can be seen by others on the same Wi-Fi: use it at home.
* The support file hides the mod panel PIN and the home folder; it still has viewer names and messages, which the README in the zip says.

---

## Development

```powershell
go vet ./...
go test ./...
.\build.ps1 -Version 1.4.0          # vet + test + build → dist\osureq.exe (also rewrites rsrc_windows_amd64.syso)
```

If PowerShell blocks scripts, build directly:

```powershell
$env:GOOS="windows"; $env:GOARCH="amd64"; $env:CGO_ENABLED="0"
go run ./tools/winres -version 1.4.0      # icon, version info, manifest → rsrc_windows_amd64.syso
go build -trimpath -ldflags "-X main.version=1.4.0" -o dist\osureq.exe .
```

### Releases (GitHub Actions)

`.github/workflows/build.yml` runs on every push to `main` and every pull request: `go vet`, `go test` and a build on
`windows-latest`; the exe is kept as a workflow artifact for 14 days (version `0.0.0-dev+<commit>`).
To publish a release, bump the version (Changelog row, Version row, `build.ps1` default), commit, then tag and push:

```powershell
git tag v1.4.0
git push origin v1.4.0
```

The tag build uses the tag's number for `main.version` and the Windows resources, and publishes a GitHub release with
`osureq-<version>-windows-x64.zip` (`osureq.exe`, `LICENSE`, `THIRD_PARTY_LICENSES.txt`) and its `.sha256`.
When a new module ends up in the exe (`go list -deps -f '{{if not .Standard}}{{.Module.Path}}{{end}}' .`), add its license to `THIRD_PARTY_LICENSES.txt`.

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
8. **Security invariants:** keep the dashboard's 127.0.0.1 bind (the mod panel is the one, opt-in exception, with its own guard), the `guard` checks, the host whitelist, `Trusted` before opening files, and `SafeJoin` for extraction.
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
| Profile pictures | `profilePictureUrl` and the other shapes are unverified; if no pictures appear, check the raw `chat` event for the field name |
| Mod panel | plain HTTP on the LAN; not tested on Windows yet (firewall prompt, phone on the same Wi-Fi) |
| Song search | depends on catboy's search API; when it is down, names are rejected with `search_error` (IDs and links still work) |
| Official download fallback | implemented, not exercised (needs a logged-in browser) |
| osu! lazer | supported via `open` and tosu, not tested |
| osu.direct no-video URL | `?noVideo=1` is a guess |
| `useOsuDb` / `osu://b/{id}` | experimental, off by default |
| Overlay "now playing" card | rendered and styled, only partly checked visually |
| Ideas | show tosu's live map on the dashboard; answering in TikTok chat through TikFinity's chatbot (TikFinity can relay `sendChatbotMessage` from Streamer.bot; not built) |

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
| Phone cannot open the mod panel | Same Wi-Fi (not a guest network, which isolates devices)? Allowed in the Windows firewall for **private** networks? Windows network profile set to Private? Use an address from Settings → Mod panel. |
| Windows says the app is unsafe / "Windows protected your PC" | That is SmartScreen warning about an **unsigned** program, usually one that was downloaded or received in a zip. Windows Defender finds nothing in it. Click **More info → Run anyway**, or right-click `osureq.exe` → **Properties** → tick **Unblock**. Only a code-signing certificate removes the warning for everyone. |
| TikFinity's TTS reads `!req 123…` | Turn TikFinity's TTS off and use **Settings → TTS**, which never reads commands. |
| Chat TTS is silent | Press **Start TTS** at the top (browsers need a click). No Thai voice: Windows Settings → Time & language → Speech → add Thai, or use Edge. |
| Anything else | `osureq.log` next to the exe. |

---

## Changelog

| Version | Commit | Changes |
|---|---|---|
| 1.4.0 | PR #1 | TTS for subscribers (ใจส้ม) / followers / gifters / mods only; spoken announcements from templates; chat TTS without commands; artist/title ban rules (`artist="…"`); search ignores tag-only matches; requests with mods; support file; song bans; fair queue; automatic open/close of requests; mod panel on a phone (LAN + PIN); song-name requests (catboy search); `!pos`, `!np`, `!voteskip`; pre-stream check; session summary; gift-only requests; viewers' TikTok pictures; stable/lazer detection from tosu; update check; **fix:** osu.direct downloads (redirect to `storage.osu.direct` was refused) and every redirect hop is checked; tests for `server` |
| 1.0.0 | `d106295` | First version: TikFinity chat, parser, osu! metadata, validation, queue, mirrors + MD5, import modes, preload, dashboard (EN/TH) and overlay, hotkeys, tests |
| 1.0.1 | `9fabc98` | The map being played no longer counts toward the per-user limit; exempt users; bare number retried as set ID; osu! folder from the registry and `BeatmapDirectory`; pop-up on rejected chat requests |
| 1.1.0 | `15f15b3` | Test panel: simulated chat, quick add, dry run, clear test data |
| 1.1.1 | `7508aa4` | Dashboard no longer flickers during downloads (incremental rendering) |
| 1.2.0 | `5999bba` | Auto mode detects the end of a song with tosu (no timer); tosu becomes a requirement; tosu status, banner and phase line |
| 1.3.0 | `644f884` | Overlay pop-ups for added/rejected requests (with reasons); whole-set requests prefer the main mode |
| | `6ed5ce7` | Chat commands: `!remove`, and for mods `!skip`, `!remove @user`, `!reqon`, `!reqoff` |
| | `1821882` | Play results from tosu on requests, history, dashboard and overlay |
| | `a74d4b9` | Gifts buy queue priority (opt-in) |
| | `a545bb4` | System tray app (no console), second launch opens the running copy, Quit button (tray removed again in 1.3.1) |
| | `d12d958` | Audio preview and "open in beatmap viewer" links |
| | `af89662` | One-click bans from queue cards and the rejection log |
| | `0ecf47c` | Followers-only and per-subscriber limits |
| 1.3.7 | `f8a1741`, `73bb778` | tosu launched from osureq opens in its own console window instead of running hidden; **Close tosu**, **Close TikFinity** and **Close everything** buttons in the Info box |
| 1.3.6 | `b35a818` | All four modes: convert stars from osu!, mania key counts, per-mode star limits, mode tags, mode-mismatch warning |
| 1.3.5 | `07db532` | Click-through audit fixes: card ⋯ menu stays on screen (opens upward, never wider than the screen); card status shown first; overlay never cuts the reason; truthful test-panel feedback for commands and gifts; translated errors and settings validation; side-log titles on their own line; buttons and settings fit narrow screens |
| 1.3.4 | `f135452`, `10a3ba1` | Settings open as a centred dialog; master volume (default 10%) with a speaker button in the top bar |
| 1.3.3 | `8fc0004` | Windows resources (icon, name, description, version, `asInvoker` manifest) and no symbol stripping, so Windows treats the exe as a normal app |
| 1.3.2 | `272bd1c`, `6b37a4c` | UX/UI audit fixes: narrow layout, Info box, set mode order, now-playing reason, Thai text; compact status, folded test panel, card "⋯" menu, calmer overlay |
| 1.3.1 | `fcbd95c` | Back to a console app: the CMD window shows the log (titled `osureq <version>`); no tray icon. Closing the window stops cleanly |

---

Credits: [TikFinity](https://tikfinity.zerody.one/) for TikTok events, [tosu](https://github.com/tosuapp/tosu) for osu! game state,
catboy.best and osu.direct mirrors, and the owner's osu! beatmap viewer for the visual design.
