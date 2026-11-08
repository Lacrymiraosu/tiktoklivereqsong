# osureq: developer guide

A small Windows helper (one `osureq.exe`, ~8 MB, Go, no installer, no runtime) that lets
TikTok LIVE viewers request osu! beatmaps in chat. The helper:

1. reads chat from **TikFinity Desktop**, or straight from TikTok (**TikTok direct**, the connection signed by Euler Stream),
2. checks each request against the streamer's rules,
3. downloads it from a mirror,
4. confirms the `.osu` files match what osu! currently serves (MD5),
5. opens the map in osu! when the streamer presses **Next**, or automatically once **tosu** sees the previous song has ended.

A local dashboard (English/Thai) and a transparent OBS overlay run inside the same exe. The overlay can also be loaded from a second PC
through the mod panel's port with a secret link ([Overlay link](#overlay-link)).

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
| Version | **1.5.1** (`git log` for history, [Changelog](#changelog) below) |
| Language / build | Go 1.23+, `CGO_ENABLED=0`, Windows x64 only |
| Tests | `go test ./...`: 20 packages with tests, all passing (`go test ./internal/...` also runs on Linux; only `main` needs Windows) |
| Repository | <https://github.com/Lacrymiraosu/tiktoklivereqsong>, branch `main`; builds and releases by GitHub Actions |

**Verified for real on the developer's PC:** TikFinity detection, WebSocket connection and `config` handshake;
osu! metadata scraping; mirror downloads; MD5 verification (single diff and whole set); rejections; preload copy;
osu! folder auto-detection from the registry; the dashboard, settings and overlay; simulated chat; dry run;
the anti-flicker rendering, overlay pop-ups, mod commands and gifts through simulated chat, second launch and Quit,
audio preview and one-click bans. Auto mode and play results were verified against a **fake** tosu server.

**Not yet verified with the real thing:** see [Known limitations and open items](#known-limitations-and-open-items)
(live TikTok chat, gift, role and profile-picture fields, real tosu + osu! gameplay, opening `.osz` into osu! via Next, hotkeys, official-download fallback, lazer).
The 1.4.0 features were tested with unit tests and a Linux harness (dashboard and overlay screenshots); song search, the
pre-stream check, the osu.direct fix and the update check were also run against the live services.

The 1.5.0 changes (viewer count, KIKI map player, several maps per message, overlay link for a second PC, the Map rules drawer, the kiki-bm.app
default viewer) are covered by unit tests and were tried together in a browser against a test server on one PC (the overlay link through its LAN
address; the rules merge also with a node script). They were not yet tried on a real stream, on a second PC or in OBS. What is still unverified is in
[Known limitations and open items](#known-limitations-and-open-items).

The 1.5.1 changes (toasts above dialogs; chat TTS voice gender, a voice per language and Japanese, Korean, Chinese and Hindi)
has Go tests (`TestSpeakableTextScripts`, `TestTTSVoicesAndGender`, `TestPutConfigTTSVoices`), and its browser side was tried only with fake voice lists
(`speechSynthesis.getVoices` and `speak` replaced), never with real voices or audible playback. Which voices Microsoft Edge exposes to web pages, and whether
its Natural voices work from a web page at all, are not verified (see [Known limitations and open items](#known-limitations-and-open-items)).

---

## Features

* **Chat requests** via TikFinity only (`!req` + aliases, configurable), accepting IDs, every common osu! link form and **song names**
  (`!req freedom dive`, searched on the mirrors (hinai → Nekoha → osu.direct → catboy.best) and re-ranked by title match and play count).
* **Validation**: star range, max length, modes, ranked statuses, set/mapper/user blacklists, queue size,
  per-user limit (the map being played does not count), cooldown, duplicates, no-repeat window, no-limit (exempt) users.
* **Metadata and reference MD5** scraped from osu.ppy.sh (no API key), with a raw `.osu` fallback.
* **Downloads** from hinai → Nekoha → beatconnect → osu.direct → catboy.best → Sayobot → official site (browser + Downloads folder), each optional and reorderable,
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
* **Several maps per message** (opt-in, `command.maxPerMessage` 1-5): `!req 2096297 1859353 HDHR 1521430` is three requests, each through every rule;
  see [Several maps in one message](#several-maps-in-one-message).
* **Fair queue** (opt-in): a viewer's second request waits behind everyone's first, and so on.
* **Automatic open/close**: requests close at a queue size and reopen when it is short again, and close at a set time.
* **Song and artist bans**: rules like `artist="a_hisa"`, `title="Ghost"` or both; every set of a song (TV size, cut, remaps) is caught, also by Japanese/Thai names.
* **Mod panel** (opt-in): a PIN-protected phone page on the home network for Next, Skip, moving/removing requests and requests on/off.
* **Overlay on a second PC** (opt-in, `overlay.remote`): the mod panel's listener serves the overlay and a read-only state feed under a secret link `/o/<token>`; see [Overlay link](#overlay-link).
* **Chat TTS** (opt-in): reads chat aloud in the dashboard and never reads commands (`!req …`), links or emoji; replaces TikFinity's TTS, which cannot skip them. Thai, English, Japanese, Korean, Chinese and Hindi, a browser voice per language and a female/male preference (see [Chat TTS](#chat-tts)).
* **Spoken announcements** (opt-in): "test1 ขอเพลง …", "เพลง … ยากเกินไป 7.25 ดาว", next map, results, gifts, from templates the streamer edits.
* **Support file**: one zip (log, raw TikFinity events, settings, state) to send after a stream with a problem.
* **Viewer roles**: followers-only mode and a higher per-user limit for subscribers (when TikFinity sends these flags).
* **One-click bans** of a requester, map or mapper from a queue card or the rejection log.
* **Map rules on the dashboard**: the **Map rules** button above the queue opens a drawer with every map rule (stars, length, modes, statuses, mods,
  followers only, blacklists); it is no longer a Settings tab. See [Map rules drawer](#map-rules-drawer).
* **Audio preview** of queued maps and a link to open them in the owner's **beatmap viewer**.
* **Map player** (`osu.kikiPlayer`, `osu.kikiPlayerMod`): **▶ view map** on queue, now-playing and history cards (and on the mod panel) opens
  [KIKI Beatmap Viewer](https://api.kiki-bm.app/)'s player under the card, osu!standard maps only; see [KIKI Beatmap Viewer player](#kiki-beatmap-viewer-player).
* **CMD window** with the live log (titled `osureq <version>`); closing it stops the helper cleanly. A second launch opens the running copy's dashboard.
* **Dashboard** (EN/TH): status of TikFinity and tosu, now playing, queue with drag-to-reorder, per-source download
  progress, MD5 result, rejection log, activity, history, raw TikFinity events, full settings editor (map rules in their own drawer).
* **Test panel**: simulated chat through the real rules (requests and commands), quick add, simulated gifts, dry run (never touches osu!), clear test data.
* **Pre-stream check**: one button checks TikFinity, tosu, the Songs folder, hotkeys, the overlay connection, osu.ppy.sh and every mirror.
* **Session summary** ("This stream"): maps played and cleared, best play, top requesters; shown on the overlay on request.
* **Viewer count**: "👁 1,234" in the dashboard's connection pill and next to the connection name on the mod panel, only when known (TikTok direct; TikFinity when it sends it).
* **stable/lazer detection** from tosu, and **automatic updates** from GitHub releases: downloaded and SHA-256 checked in the background, installed the next time osureq starts (or right away from the button at the top).
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
| **TikFinity Desktop** (or TikTok direct) | Installed, logged in, connected to the TikTok account, and **running during the stream**. Default WebSocket `ws://127.0.0.1:21213/`. Download: <https://tikfinity.zerody.one/>. Not needed with `connection.source` = `tiktok` |
| TikTok direct | The streamer's TikTok @name, and HTTPS to `www.tiktok.com`, `tiktok.eulerstream.com` (sign server; keyless or with a free API key) and TikTok's LIVE WebSocket (`webcast-ws*.tiktok.com`) |
| **tosu** (required) | Running during the stream, WebSocket on the default `127.0.0.1:24050`. Auto mode depends on it; without tosu, auto mode pauses (manual Next still works). Download: <https://github.com/tosuapp/tosu/releases/latest> |
| Optional | OBS or TikTok LIVE Studio, for the overlay |
| Optional | Logged in to osu.ppy.sh in the default browser. Only needed for the official-download fallback |

### To develop it

| | |
|---|---|
| Go | 1.23 or newer (`winget install GoLang.Go`). Uses `net/http` routing patterns (1.22) and `maps`/`slices` iterators (1.23) |
| OS | Windows (the `platform` package uses Win32 APIs; everything else is portable Go) |
| Modules | `github.com/coder/websocket` (WebSocket client and server), `golang.org/x/sys` (Windows APIs, registry), `google.golang.org/protobuf` and `github.com/steampoweredtaco/gotiktoklive/proto` (TikTok LIVE message types; only the `proto` package is used) |
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
!req 2096297 1859353 HDHR 1521430                         ← several maps (command.maxPerMessage ≥ 2); mods go with the ID before them
```

Mods (`internal/mods`): NF EZ HD HR SD DT HT NC FL SO PF FI MR, written back in osu!'s order (`HDHR`, `HDDT`). NC includes DT and PF includes SD.
After an ID or link, mods are read when everything after it is mods in any case (`hd hr`), or when the first word starts with `+` or
is in capitals made only of mod codes (`HDDT pls`); other words ("please", "so good", "OK") are ignored as before. Clashes (EZ+HR,
HT+DT/NC, NF+SD/PF, FI+HD, FL+FI) and unknown codes in a `+` word are rejected as `bad_mods`. A lower-case word at the end of a song
name is part of the name.

An ID or osu! link in the first word wins and anything after it is ignored. Otherwise the text (3-80 characters) is a song name;
anything that looks like a link to another site is never searched and is ignored, as is everything when search is off.

With `command.maxPerMessage` above 1, a message made only of IDs, osu! links and mods that names two or more different maps is several requests
(see [Several maps in one message](#several-maps-in-one-message)); every other message is read as described above.

Other chat commands (names are configurable; an empty name turns one off):

| Command | Who | What it does |
|---|---|---|
| `!remove` | anyone | removes your own latest waiting request |
| `!remove @user` | mods | removes that user's latest waiting request |
| `!skip` | mods | skips the song being played (opens the next one) |
| `!reqon` / `!reqoff` | mods | opens / closes requests |
| `!pos` | anyone | the overlay shows where the viewer's requests are (`#2, #5`); once per viewer every 20 s |
| `!np` | anyone | the overlay shows the song osu! has open according to tosu (the queue's now-playing map when tosu is not connected), and who asked for it when it is a request; once per viewer every 20 s |
| `!voteskip` | anyone | one vote per viewer per song; skips when `command.voteSkipVotes` different viewers voted (0 = off, the default) |

Mods are the users in **Settings → Command → Mods**, plus TikTok moderators when TikFinity sends that flag (`command.trustTikTokMods`).

---

## How a request is handled

1. **Parse** (`internal/parser`): the chat message must start with the command or an alias, followed by an ID, a link or a song name (`KindSearch`).
   With `command.maxPerMessage` above 1, `parser.ParseAll` first tries to read the message as several maps (see below).
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
   1. the mirrors, in the order of [Mirrors](#mirrors-internaldownloader-url-templates-in-config): hinai, Nekoha, beatconnect, osu.direct, catboy, Sayobot
   2. **official**: opens `https://osu.ppy.sh/beatmapsets/{setId}/download` in the default browser and waits for `{setId} *.osz` in the Downloads folder. Only used when MD5 checking is on. The osu! password is never asked for or stored.

   The cache is checked first. Each source has its own timeout, and the response must be a real ZIP. A refused download shows the mirror's own
   message (`HTTP 403: you have been banned…`); after HTTP 429 that mirror rests for its `Retry-After` (60 s when it gives none, at most an hour);
   hinai's `404` with `code: "content_removed"` (taken down for the rights holder) stops the request instead of asking the other mirrors.
7. **MD5 check** (`internal/verifier`): unzip in memory and hash every `.osu`:
   * `requested`: the requested difficulty must match (every difficulty when the request was a whole set).
   * `strict`: every difficulty on osu! must match, and the archive may not contain extra `.osu` files.

   A mismatch tries the next source. If all fail: `skip` the request, or `warn` and use the latest download.
8. **Import** (`internal/importer`), on **Next** / **Play** / auto mode:
   * `open` (stable + lazer): Windows opens the `.osz`; osu! imports it and jumps to it.
   * `copy-to-songs` (stable): copied into Songs; picked up on song select or F5.
   * `extract-to-songs` (stable): extracted to `Songs\{setId} {Artist} - {Title}\`.

   **Auto download** (`import.preload`) downloads and verifies the next N maps in the background, and on stable copies them into Songs so Next is instant.
   It has a switch above the queue on the dashboard (`toggleAutoDownload`, `State.autoDownload` / `autoDownloadN`). Off, a map is fetched when it is opened (Next, Play now, or the one auto mode opens next); downloads already running finish. Switching on with `preloadCount` 0 sets it to 2.

Rejection reason codes (shown translated on the dashboard): `user_blacklisted`, `queue_full`, `user_limit`, `cooldown`, `set_blacklisted`,
`mapper_blacklisted`, `status`, `duplicate`, `recently_played`, `not_found`, `mode`, `stars_low`, `stars_high`, `too_long`, `meta_error`,
`bad_input`, `requests_off`, `followers_only`, `now_playing`, `no_convert` (a taiko/catch/mania map requested in another mode: only osu! maps convert),
`search_none`, `search_error`, `needs_gift`, `song_blacklisted`, `artist_blacklisted`, `bad_mods`, `mods_off`, `mod_not_allowed`, `mod_stars`, `too_many_maps` (more maps in one message than `command.maxPerMessage`; shown on the overlay, never spoken).

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

### Several maps in one message

`command.maxPerMessage` (1-5, default 1 = one map per message as before; `max_per_message` when out of range; configs without the key get 1)
switches `parser.ParseAll` on. Settings → Command → **Maps per message**.

* **Parsing** (`parser.ParseAll(comment, Options{MaxMaps})`): used only when `MaxMaps` > 1, the message has more than two words and starts with the command.
  Every word after it must be an ID or osu! link (`ParseArg`) or a mods word, and the words must name two or more different maps. A mods word is read as belonging to
  the map before it: `+…` always (so `+HDXX` is a wrong mod, not chat), capitals made only of mod codes (`EZHR`), or words that parse as mods in any case (`hd`, `hdhr`).
  Anything else (a song name, chat after the ID, a mods word first) falls back to `Parse`, so `!req 123 please` and song searches behave as before. A bare ID and a link to the
  same map are one map; a map named twice counts once (the first, with its mods). Maps past `MaxMaps` are not requested and are counted in `extra`. All words after a map are
  parsed as its mods (`mods.Parse`); a clash or unknown code sets that map's `Request.ModsErr`, so only that map is rejected (`bad_mods`). Result: `[]Part{Request, Text}`.
* **Handling** (`App.submitAll`, one goroutine for the message, maps one after another, with a per-message `batch`): `HandleChat` and `SimulateMessage` call `ParseAll`; one part goes to
  `submit` as before. Each map runs through the same `resolve` (`CheckUser`, metadata, `CheckMap`), sees the maps before it in the queue (so each counts against `queue.perUserLimit`, and a set
  named twice is a duplicate), and with gift-only requests takes one paid request in order (a rejected map gives its request back, as for a single request).
* **Cooldown**: `resolve` clears `LastRequest` once the message's first map has been through the viewer checks (`batch.checked`), so only the first map is held to the cooldown and the maps do not block each other;
  the next message waits from the last map added (`queue.add` records it).
* **Viewer rejections** (`viewerCodes`: `user_blacklisted`, `queue_full`, `followers_only`, `user_limit`, `cooldown`, `needs_gift`, `requests_off`, also when requests were closed in the
  middle of the message) end the message: one rejection record stands for this map and the ones after it (its `input` lists them: `batch.rest`). Map rejections (stars, mode, duplicate, `bad_mods` …)
  do not stop the message.
* **Over the limit**: one `too_many_maps` rejection for the message (`short` `asked > allowed`, vars `n`, `max`), logged in the rejection list and shown on the overlay; marked as already spoken.
* **TTS**: `batch.quiet(outcome)` lets each outcome (`added`, `rejected_<code>`) be spoken once per message and at most `spokenPerMessage` (2) lines in all; the dashboard keeps only 5 lines waiting.
  The unexported `FeedEvent.quiet` keeps an event off `announce`; the overlay still gets every map.
* **API**: `/api/simulate` for several maps answers `ok` (at least one added), `item` (the first added), `code`/`short`/`error` (the first rejection, also when others were added), `maps` (asked),
  `count` (added), `rejected` and `ignored` (maps over the limit); single-map answers are unchanged. The dashboard's Test box adds the command in front of IDs/links that may be followed by mods
  (`123 HDHR`) and in front of several IDs; quick add (`TestRequest`, `POST /api/test`) stays one map.
* **Known gap**: `PUT /api/config` from a Settings dialog opened before the update would send 0 for the new key and fail validation (like other range fields); reload the dashboard.

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

The helper follows the map it opened (matched by **beatmapset**, so any difficulty counts, or by artist and title when tosu reports another set ID, as osu! does with -1 for maps whose .osu has no set ID) through tosu's game state
(`internal/tosu.Tracker`):

| What tosu shows | Phase (`Item.playPhase`) |
|---|---|
| Map opened, not started (song select alone does not count) | `waiting` |
| Gameplay on that map (retries and fails stay here) | `playing` |
| Results screen (retry from here goes back to `playing`) | `results` |
| Song select or menu after playing, or gameplay of another map | `ended` → the next map opens after `autoGapSec` (with `autoNeedsPass`, only if the play reached the results screen) |
| Gameplay on that map again after `ended` (a retry after a fail or quit) | `playing` |

`busy`, `unknown`, `update` and empty states are ignored, because tosu reports them briefly while osu! loads.
There is **no timer fallback**: if tosu is not connected, auto mode pauses and the dashboard says so; it resumes by itself when tosu returns.
When nothing is playing, the first ready map opens straight away. Requests that failed to download are passed over, as with Next. Manual **Next** always works.

**Play results:** while the map is in `playing`, the latest live numbers from `play` are kept. Entering `results` stores tosu's `resultsScreen`
as **passed**; leaving gameplay without a results screen stores the last live numbers as **failed** (tosu's `play.failed`) or **quit**.
A quit before any score leaves no result. The result is saved on the item (`Item.result`), copied into history (`Played.play`), shown on the phase line,
and announced on the overlay: `98.52% S · 812x · 2 miss · 301pp · HDHR`.

---

## Testing without going live

The **Test** panel on the dashboard works while TikFinity is offline:

* **Simulate chat** (`POST /api/simulate`): a message as any `@user`, through the real chat path (command, aliases, requests on/off,
  per-user limit, cooldown, map rules). Non-requests are reported as "ignored", or "spoken" when chat TTS reads them. The dashboard prepends the command only to text made of IDs or osu! links, each maybe followed by mods (`123 HDHR`, `123 456`);
  other text is sent as plain chat, so TTS can be tried from the panel. Several maps: see [Several maps in one message](#several-maps-in-one-message).
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
character, lowercases capital words of three letters or more (`SoftenCaps`: Google's Thai voice spells "KIKI" as K, I, K, I; two-letter ones such as GG or FC
stay), and cuts at `tts.maxLength`. Announcement values (title, artist, mapper, …) are softened the same way. The last 20 go out in the state as `speech` (newest first, increasing `id`).

The dashboard speaks them with the Web Speech API (`speechSynthesis`, i.e. the voices of Windows and the browser; Edge may offer natural ones, which is not verified for web pages: see [Known limitations and open items](#known-limitations-and-open-items)).
TTS starts by itself when the dashboard loads (`ttsAutoStart`): a tenth of a second of silence is played to see whether the browser allows sound yet;
if it answers `NotAllowedError`, the pill says **Click anywhere to start TTS** and the first click or key on the page starts it (Edge can allow it outright
under Media autoplay). The top-bar 🔊 button switches it off, which is remembered in that browser (`localStorage` `osureq.ttsOff`) until it is started again.
Nothing from before TTS started is read, and at most five messages wait. The language and the voice are picked as described under
the paragraph *Languages, voices and gender* below. OBS records it as desktop audio.
Streamers should turn TikFinity's TTS off.

Only one dashboard page speaks, so a second window or tab does not double the voice. Each page makes a random id at load and connects
with `/ws?page=<id>`; starting TTS (by itself, a click, or the pill) sends `POST /api/tts/speaker` `{page, on: true}`, and the last page to do
so has the voice (`State.ttsSpeaker`). The other pages with TTS on stay quiet and show **TTS in another window**; clicking it moves the voice
there. Switching TTS off in the speaking page sends `on: false`: it keeps the claim with `ttsSpeakerOn` false, so the others show TTS as off
instead of starting. When all of the speaking page's WebSocket connections close (tab closed, reloaded), the claim is cleared and the next
page with TTS on that gets speech takes it.

Windows ships no Thai voice, and Chrome-based browsers (Chrome, Dia) can only use Windows voices, so `tts.engine` decides who speaks:
`auto` (default) uses the browser's voice when it has one for the language and Google Translate's voice otherwise; `browser` never goes
online; `google` always does. Google's voice goes through the helper (`POST /api/tts`, `internal/gtts`): it fetches
`https://translate.google.com/translate_tts` in pieces of at most 180 letters, joins the MP3s, keeps the last 30 lines, and the dashboard
plays the result with `<audio>` at the TTS volume and speed. The text being read is sent to Google. Google reads only Thai and English.

**Languages, voices and gender** (free, browser voices only, no keys; `ttsScript`, `ttsLangOf`, `ttsPickVoice`, `ttsGenderOf` in `app.js`).
*Language.* With `tts.lang` `auto` (chat and the Settings voice test), a line's language is told by its letters: Thai letters in the text or the viewer's name
→ `th` (as before); else kana → `ja` (checked before Han, because Japanese mixes both); Hangul → `ko`; Han without kana → `zh`; Devanagari → `hi`; otherwise `en`.
Languages written with Latin letters (Indonesian, Malay, Vietnamese, Spanish, French, German) cannot be told apart by letters and are read as English. A fixed
`th` or `en` does no detection. Announcements (speech kind `say`) keep the old rule, Thai letters in the line or the name → Thai, else English, so a Japanese
nickname does not switch an English announcement. For `th` and `en` everything is as before (`tts.engine`, the Google fallback, "name says", volume, rate).
For `ja`, `ko`, `zh` and `hi` only browser voices are used: the line is never sent to `POST /api/tts` (`tts.engine` `google` does not apply to them); with no voice
for the language it is skipped and one toast per language (`ttsNoVoiceLang`) says to install a voice in Windows (Settings → Time & language → Speech) or to use
Microsoft Edge. The line read is `{name}: {text}` (there is no "says" template for these languages).

*Voice choice.* The voice chosen for the language wins when it is one of the language's voices (those whose `lang` starts with the code): `tts.voices[lang]`, else
the older `tts.voice`, which still counts for its own language only. Otherwise, with `tts.gender` `any` the choice is unchanged: a natural or online voice first, then
a local one, then the first. With `female` or `male`, the same order is applied among the voices whose gender is known to be that one. If none is known for the
language, the best available voice reads (or Google, when the browser has none for Thai or English) and one toast per language and gender (`ttsGenderMiss`) names it;
a Google voice chosen on purpose (`tts.engine` `google`) gives no such toast. The Settings voice test reports it together with its result (`ttsTested`) instead.

*Gender.* The Web Speech API has no gender field, so `ttsGenderOf` looks up the voice's first name in a table taken from Microsoft's official voice lists (the speech
service voice catalogue and Narrator's voice table, so Windows desktop voices are included). The name must be a whole word of the voice's name and the voice must be
of that table's language, so "Guy" cannot match another language's voice; any other voice is `?` in the Settings lists and is never guessed:

| Language | Female | Male |
|---|---|---|
| th | Premwadee, Achara | Niwat, Krit, Nattapong, Pattara |
| en | Jenny, Aria, Ava, Emma, Sonia, Neerja, Zira, Hazel, Susan, Catherine, Linda, Heera | Guy, Andrew, Brian, Christopher, Ryan, Prabhat, David, Mark, George, James, Richard, Ravi, Sean |
| ja | Nanami, Haruka, Ayumi | Keita, Ichiro |
| ko | SunHi, Heami | InJoon |
| zh | Xiaoxiao, Huihui, Yaoyao, Yating, Hanhan | Yunxi, Kangkang, Zhiwei |
| hi | Swara, Kalpana | Madhur, Hemant |

*Settings.* **Settings → TTS** has a gender selector (`tts.gender`) and a dropdown (`tts.voices`, field type `voices`) for each of the six languages that has at least one
installed voice, with ♀, ♂ or ? after each name. The voice list can load late, so it is drawn again when the browser says `voiceschanged`. The older single `tts.voice`
moves into the dropdown of its language when it names an installed voice: the form fills `tts.voices[lang]` (unless that is already set) and clears `tts.voice`, and Save
stores both. The **Test the voice** box reads what is typed (its language picked like chat), or a sample, and then says which voice read it with its mark (for example
"Read by Microsoft Niwat ♂"), "Read by Google", or the no-voice notice, and shows an error if a voice fails (`ttsTested`).

*Config and going back.* Everything new is a new key (`tts.gender`, `tts.voices`; `tts.lang` is still `auto`, `th` or `en` and `tts.engine` is unchanged) because the released
1.5.0 rejects unknown values of existing fields but ignores unknown keys, so it still starts with such a config. After the migration `tts.voice` is empty and 1.5.0 ignores
`tts.voices`, so going back to 1.5.0 loses the chosen voice. Validation: `tts_gender` (not `any`, `female` or `male`; empty becomes `any`) and `tts_voices` (a key not in
`config.TTSVoiceLangs`, a name over 120 letters or with a control character); the dashboard translates them as `e_tts_gender` and `e_tts_voices`.

*Length cut.* `SpeakableText` cuts at `tts.maxLength` letters but no longer parts a letter from the combining marks after it (Devanagari vowel signs, Thai tone marks): it keeps
the marks that follow, and adds no "…" when they reach the end of the text.

**Announcements** (`tts.announce`, `app/announce.go`): `App.emit` passes every feed event to `announce`, which picks the template
(`tts.templates[event]`; a rejection uses `rejected_<code>` if that key exists, else `rejected`), fills it (`RenderAnnouncement`: unknown or
empty lowercase placeholders vanish, `@` is dropped because voices say "at") and queues it as a `say` speech item. Events (`config.AnnounceEvents`):
`added`, `rejected`, `rejected_stars_high|stars_low|mod_stars|too_long|duplicate|recently_played|cooldown|user_limit|search_none`, `nowplaying`
(from `play`), `np` / `np_none` / `pos` (the `!np` and `!pos` answers; default `กำลังเล่น {artist} - {title} {setid}`), `result_passed|result_failed`, `gift` (once per gift, from `gift`), `voteskip`, `auto_off` (also the closing time), `auto_on`.
Empty or `-` is silent. Which placeholders each event has, where every value comes from, the rejection `Vars`, the `reasons` table, the limits and how to add a
placeholder or an event are in [Announcement lines and placeholders](#announcement-lines-and-placeholders) below. Defaults are `config.ThaiAnnouncements()`;
the settings tab can load those or `EnglishAnnouncements()` (sent with `GET /api/config` as `announcements`).
A `config.json` from an older version gets the lines added since then in the language of its own `added` line (`TTS.fillTemplates`).

### Announcement lines and placeholders

The streamer-facing version of this (what each placeholder says, examples, tips) is in the README, section *What you can put in an announcement line* ([English](../README.md#what-you-can-put-in-an-announcement-line), Thai in [README.th.md](README.th.md)). This is the precise one. The in-app help (`tplHelp` in `app.js`, English and Thai) is only a compact list of the names and does not mention `{n}` and `{mode}`; the labels of the boxes are the `tpl_<event>` strings next to it.

**Flow.** A code path builds a `FeedEvent` (`feed.go`) and calls `App.emit`, which numbers it, adds it to the feed and calls `announce(e)` unless the unexported `e.quiet` is set (see *Several maps in one message*). Two events skip the feed and call `announce` themselves: `nowplaying` (`announcePlaying`, from `play` and `Previous`) and the gift thanks (`Kind: "giftthanks"`, from `gift`). Then:

1. `announceKey(e)` maps the event to a template key (table below); `""` = nothing to say.
2. `announce` returns when `tts.announce` is off or the key is empty. It takes `tts.templates[key]`; only when that **key is missing** and starts with `rejected_` does it fall back to `tts.templates["rejected"]`. A key that is present but empty or `-` (after `TrimSpace`) is silent and does not fall back. The dashboard saves every box, so only the nine `rejected_<code>` codes have a box of their own; every other rejection code uses `rejected`.
3. It builds the `vars` map from the event (table below), then copies every entry of `e.Vars` over it: `mods` goes through `spokenMods`, every other value through `SoftenCaps`. So a key in `Vars` wins over a built-in one with the same name (a rejection's `title`, a result's `mods`). Last, `setid_digits` is derived from `vars["setid"]` (one space between the characters; empty when there is no set ID).
4. `RenderAnnouncement(tpl, vars)` replaces every `\{[a-z_]+\}` with `vars[name]` (unknown or empty = nothing; a token with capitals, digits or spaces such as `{User}` is not matched and stays as typed), deletes `@`, collapses runs of whitespace to one space and trims. An empty result is not spoken.
5. `queueSpeech(Speech{User: e.User, Text, Test: e.Test, Kind: "say"})`. `Name` stays empty, so the dashboard adds no "name says"; the text is not cut at `tts.maxLength` (that is for chat) and `tts.allow`, `tts.blockedWords` and `tts.cooldownSec` do not apply. The last 20 items go out as `speech`; the dashboard keeps 5 waiting. In the dashboard `ttsLangOf` picks the language: a fixed `tts.lang` wins; with `auto`, announcements are `th` when the finished text has Thai letters, else `en` (a Japanese nickname does not switch it). Only a page where TTS is on speaks, and only while `tts.enabled` or `tts.announce` is true.

**Events** (`config.AnnounceEvents`, which is also the order of the boxes in Settings; `config.ThaiAnnouncements()` and `config.EnglishAnnouncements()` hold the default line of every one, and the Settings buttons load them through `announcements` in `GET /api/config`):

| Template key | `FeedEvent` (`announceKey`) | Emitted from |
|---|---|---|
| `added` | `Kind: "added"` | `submit` after `Queue.Add`: chat, the Test panel and quick add (those two with `Test` set) |
| `rejected`, `rejected_<code>` | `Kind: "rejected"`, `Code` = the rejection code | `App.rejected`; `HandleChat` for `requests_off` |
| `nowplaying` | `Kind: "nowplaying"` | `announcePlaying`, after `Queue.Promote` in `play` and in `Previous` |
| `np` | `Kind: "command"`, `Code: "np"` | `infoEvent`, for `command.nowCmd` |
| `np_none` | `Code: "np_none"` | `infoEvent`: nothing playing |
| `pos` | `Code: "pos"` | `infoEvent`, for `command.posCmd` (`pos_none`, a viewer with no request, is not announced) |
| `result_passed`, `result_failed` | `Kind: "result"`, `Code` = `Outcome` | `OnGame`, when the tracker reaches the results screen (`passed`) or the map ended after a failed play with a score above 0 (`failed`) |
| `gift` | `Kind: "giftthanks"` | `gift`, once per gift, before the priority and paid-request logic (the `Kind: "gift"` events for priority, credit and paid requests are never spoken) |
| `voteskip` | `Kind: "command"`, `Code: "voteskip"` | `voteSkip`, only when the vote reaches `command.voteSkipVotes` (single votes are `Code: "vote"`, not spoken) |
| `auto_off` | `Kind: "status"`, `Code: "auto_off"` or `"time_off"` | `autoRequests`: queue reached `queue.autoCloseAt`, or the clock reached `queue.closeAt` |
| `auto_on` | `Code: "auto_on"` | `autoRequests`: reopened at `queue.autoReopenAt` |

Never spoken: `result_quit` (the key exists in `announceKey`, but it is not in `AnnounceEvents`, so it can have no line), the `command` codes `remove`, `skip`, `on`, `off`, `vote` and `pos_none`, the other `status` codes, and `too_many_maps` (marked as already spoken in `submitAll`). `!np` and `!pos` are answered once per viewer and command every 20 seconds (`infoAllowed`); a repeat inside that time is ignored completely, the overlay included.

**Placeholders and where they come from.** "Kinds" lists the events that fill it; any other event renders it as nothing.

| Placeholder | Set in | Value | Kinds |
|---|---|---|---|
| `{user}` | `announce`: `speakName(e.Nick, e.User)` | `SpeakableText(nick, 40)` (no links or emoji, runs of 5 or more equal characters trimmed to 4, `SoftenCaps`, cut at 40 letters with `…`), else `e.User` (the TikTok user name, without `@`). Whose name: `added`, `rejected*`, `nowplaying`, results: the requester; `np`, `pos`: the viewer who typed it; `voteskip`: the viewer whose vote made it pass; `gift`: the sender; empty for `auto_on`, `auto_off` | all but `auto_*` |
| `{title}` `{artist}` `{mapper}` `{diff}` | `announce`: `SoftenCaps(e.Title / Artist / Mapper / Version)` | text as osu! has it, ALL-CAPS softened | `added`, `nowplaying`, `np`, `pos`, results (`now.*`); rejections that got past the lookup (from `rej.Vars`); `voteskip` has title, artist and diff only |
| `{mods}` | `announce`: `spokenMods(e.Mods)` | `mods.List` (two-letter codes) joined by spaces: `HDHR` becomes `HD HR`; empty when none | `added`, `nowplaying`: the request's mods; rejections: the mods asked for (`req.Mods`); `np`: the mods being played (`g.Play.Mods`, only in the play state), else the request's; `pos`: the first request's; results: `Vars["mods"]` = `res.Mods`, the mods played, passed on as tosu gives them |
| `{stars}` | `announce`: `e.Stars > 0` | `FormatFloat(stars, 'f', 2, 64)`, `7.25`; empty when unknown (`<= 0`) | `added`, `nowplaying`, `np`, `pos`; `stars_high`, `stars_low`, `mod_stars` rejections through `Vars["stars"]` |
| `{pos}` | `announce`: `e.Pos > 0` | queue position, 1 = next | `added` (`Queue.Position`), `pos` (the viewer's first request) |
| `{setid}` | `announce`: `e.SetID > 0`; rejections through `Vars["setid"]` (`WithMap`) | `39804` | `added`, `nowplaying`, `np`, `pos`; rejections after the lookup |
| `{setid_digits}` | `announce`, from `vars["setid"]` | `3 9 8 0 4` | same as `{setid}` |
| `{requester}` | `announce`: `e.Target` when `Kind` is `command` and `Code` is `np` | the `User` (the user name, not the nickname, not softened) of the queue item being played; empty when tosu shows another map, or nothing is queued | `np` |
| `{reason}` | `announce`: `reasonText(e.Code, hasThai(tpl))` | the phrase from `reasons` below, Thai when the **template** has Thai letters, else English; empty for a code that is not in `reasons` | `rejected*` |
| `{n}` | `Vars["n"]` | `QueueLen` (`queue_full`) or `UserActive` (`user_limit`): how many are waiting | `queue_full`, `user_limit` (`too_many_maps` too, but it is never spoken) |
| `{max}` | `Vars["max"]` | integer limit for `queue_full` (`queue.maxSize`) and `user_limit` (`perUserLimit`, or `subscriberPerUserLimit` for a subscriber); `f2` stars for `stars_high` and `mod_stars` (`hardModMaxStars`); `m:ss` (`clock`) for `too_long` | those four |
| `{min}` | `Vars["min"]` | `f2` minimum stars for `stars_low`; **whole minutes since it was played** (`int(minutes)`, `0` under one minute) for `recently_played`: not a limit | `stars_low`, `recently_played` |
| `{length}` | `Vars["length"]` | `m:ss` of the map length divided by the mods' rate (DT/NC 1.5, HT 0.75) | `too_long` |
| `{wait}` | `Vars["wait"]` | seconds left of `queue.cooldownSec`, `int(seconds + 0.999)` | `cooldown` |
| `{mode}` | `Vars["mode"]` | `modeLabel`: `osu!`, `catch` (for `fruits`), `taiko`, `mania` | `mode` |
| `{acc}` | `OnGame` result `Vars["acc"]` | `FormatFloat(res.Accuracy, 'f', 2, 64)`, `98.52`, no `%` | results |
| `{rank}` | result `Vars["rank"]` | `res.Rank` as tosu sends it (results block, or the last live play block), for example `S` | results |
| `{combo}` `{misses}` | result `Vars` | `res.MaxCombo`, `res.Misses` as integers | results |
| `{pp}` | result `Vars["pp"]` | `int(res.PP + 0.5)`, `301` | results |
| `{coins}` `{gift}` | `gift`: `Vars` | `g.Coins` as an integer, `g.Name` | `gift` |

**What each event fills** (checked by running each event through `announce` with a probe line that lists every placeholder; `-` = renders as nothing):

| Event | `{user}` | `{title} {artist} {mapper}` | `{diff}` | `{mods}` | `{stars}` | `{pos}` | `{setid}`, `{setid_digits}` | Others |
|---|---|---|---|---|---|---|---|---|
| `added` | requester | yes | yes | asked | yes | yes | yes | - |
| `rejected*` after the lookup | requester | yes | only `stars_high`, `stars_low`, `mod_stars`, `too_long`, `mode` | asked | only `stars_high`, `stars_low`, `mod_stars` | - | yes | `{reason}` and the code's `Vars` |
| `rejected*` before the lookup (`CheckUser` codes, `bad_mods`, `needs_gift`, `requests_off`, `search_none`, `search_error`, and `not_found`, `meta_error` from the lookup itself) | requester | - | - | asked | - | - | - | `{reason}`; `{n} {max}` (`queue_full`, `user_limit`), `{wait}` |
| `nowplaying` | requester | yes | yes | request's | yes | - | yes | - |
| `np` | asker | yes | yes | playing or request's | yes | - | yes | `{requester}` |
| `np_none` | asker | - | - | - | - | - | - | - |
| `pos` | asker | yes | yes | yes | yes | yes | yes | - |
| `result_passed`, `result_failed` | requester | yes | yes | played | - | - | - | `{acc} {rank} {combo} {misses} {pp}` |
| `gift` | sender | - | - | - | - | - | - | `{coins} {gift}` |
| `voteskip` | voter | title and artist only | yes | - | - | - | - | - |
| `auto_off`, `auto_on` | - | - | - | - | - | - | - | - |

**Rejection `Vars`** (`validator.Rejection.Vars`, built with `Rejection.set`; `Rejection.Short` is for the overlay and is not a placeholder). `App.rejected` copies `title`, `artist`, `mapper` and `diff` into the event fields and passes the whole map as `e.Vars`.

| Code | Keys | Set in |
|---|---|---|
| `queue_full` | `n`, `max` | `validator.CheckUser`. The quick-add path in `resolve` (`fromQuick`) rejects it without `Vars` |
| `user_limit` | `n`, `max` | `CheckUser` |
| `cooldown` | `wait` | `CheckUser` |
| `recently_played` | `min` | `CheckMap` |
| `mode` | `mode`, `diff` | `checkDiff` |
| `stars_low` | `stars`, `min`, `diff` | `checkDiff` |
| `stars_high` | `stars`, `max`, `diff` | `checkDiff` |
| `mod_stars` | `stars`, `max`, `diff` | `checkDiff` |
| `too_long` | `length`, `max`, `diff` | `checkDiff` |
| `too_many_maps` | `n`, `max` | `submitAll` (never spoken) |
| every code from `CheckMap` and from `find` (search) | adds `title`, `artist`, `mapper`, `setid` | `Rejection.WithMap`, called in `resolve` after `CheckMap` and in `find` |

All other codes carry no `Vars`.

**`{reason}` phrases** (`reasons` in `announce.go`, code, English, Thai):

| Code | English | Thai |
|---|---|---|
| `user_blacklisted` | you cannot request right now | ตอนนี้ขอเพลงไม่ได้ |
| `queue_full` | the queue is full | คิวเต็มแล้ว |
| `user_limit` | you already have requests waiting | มีคำขอรออยู่แล้ว |
| `cooldown` | please wait a little | รอสักครู่ก่อนนะ |
| `followers_only` | follow the stream to request | ต้องกดติดตามก่อน |
| `requests_off` | requests are closed | ปิดรับคำขออยู่ |
| `set_blacklisted` | that map is blocked | แมพนี้ถูกบล็อก |
| `mapper_blacklisted` | that mapper is blocked | แมพเปอร์นี้ถูกบล็อก |
| `song_blacklisted` | that song is blocked | เพลงนี้ถูกบล็อก |
| `artist_blacklisted` | that artist is blocked | ศิลปินนี้ถูกบล็อก |
| `status` | that map status is not allowed | สถานะแมพนี้ไม่รับ |
| `now_playing` | it is playing right now | เพลงนี้กำลังเล่นอยู่ |
| `duplicate` | it is already in the queue | อยู่ในคิวแล้ว |
| `recently_played` | it was played recently | เพิ่งเล่นไป |
| `not_found` | the map was not found | ไม่พบแมพ |
| `mode` | that mode is not accepted | ไม่รับโหมดนี้ |
| `no_convert` | that mode cannot be converted | แปลงโหมดไม่ได้ |
| `stars_low` | it is too easy | ง่ายเกินไป |
| `stars_high` | it is too hard | ยากเกินไป |
| `mod_stars` | it is too hard with those mods | ยากเกินไปเมื่อใส่ mod |
| `too_long` | it is too long | ยาวเกินไป |
| `meta_error` | the map could not be checked | ตรวจแมพไม่ได้ |
| `bad_input` | that is not a map | ไม่ใช่แมพ |
| `search_none` | no map has that name | ไม่พบแมพชื่อนี้ |
| `search_error` | the search did not work | ค้นหาไม่ได้ |
| `needs_gift` | send a gift to request | ต้องส่งของขวัญก่อน |
| `bad_mods` | those mods do not work | mod ไม่ถูกต้อง |
| `mods_off` | mods are not allowed | ไม่รับคำขอที่มี mod |
| `mod_not_allowed` | that mod is not allowed | ไม่อนุญาต mod นี้ |

**Limits and validation.** `config.Validate` checks every key of `tts.templates` against `AnnounceEvents` (`tts_template`, vars `event`) and every line's length in letters, runes, not bytes, of the template text as typed, placeholders included, at most 300 (`tts_template_long`, vars `event`). The dashboard shows them as `e_tts_template` and `e_tts_template_long` (for example "Announcement rejected_cooldown is longer than 300 letters": the key is shown as is). The dashboard form has a box for every key, so the unknown-key error only comes from a hand-edited `config.json` or the API. A line with only spaces is the same as an empty one. `TTS.fillTemplates` (on load) adds every key a `config.json` lacks, in the Thai set unless the saved `added` line exists and has no Thai letters (then English).

**Several maps in one message.** `batch.quiet` (see [Several maps in one message](#several-maps-in-one-message)) lets each outcome (`added`, `rejected_<code>`) be spoken once per message and at most `spokenPerMessage` (2) lines in all; the quiet events are still in the feed.

**How to add a placeholder or an event**

A placeholder:

1. Decide where the value comes from. Either put it in `Vars` where the event is built (a `Rejection.set(...)` call, the result map in `OnGame`, the gift map in `gift`): `announce` copies every `Vars` key and softens its value, so nothing else in Go changes. Or add a field to `FeedEvent` (`feed.go`), fill it at the emit sites and add it to the `vars` map in `announce`, softening free text with `SoftenCaps`. The overlay's `OverlayEvent` (`remote.go`) only needs it if the overlay shows it.
2. Update the placeholder list in the comment at the top of `announce.go`.
3. Update `tplHelp` in `app.js`, English and Thai (keep it compact).
4. Update the placeholder and event tables here and in the README (`README.md`, `docs/README.th.md`), and the Changelog.
5. Test it in `announce_test.go` (use `newTestApp` or `searchApp`, `a.Cfg.Get().TTS.Templates[...]`, then `spoken(a)`), and in `validator_test.go` when it is a rejection `Var`.

An event:

1. Make sure something calls `emit` (or `announce`) with a `FeedEvent` and add a `case` to `announceKey` returning the new key; keep it out of the feed (`announce` directly) only when the overlay should not show it.
2. Add the key to `config.AnnounceEvents` (the order is the order in Settings) and a default line to **both** `ThaiAnnouncements()` and `EnglishAnnouncements()` (`-` to be silent by default). Old configs get it through `fillTemplates`.
3. In `app.js` add the box to the `tts` section of `SCHEMA` (`F("tts.templates.<key>", "template", …, { when: "tts.announce" })`) and the label `tpl_<key>` (English and Thai) next to `tpl_added`.
4. Add any new placeholder as above, then update the tables here and in both READMEs.
5. Add a test next to `TestAnnouncements` and check `config_test.go` (defaults).

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
 or TikTok direct ─wss─►│  (tiktok.Client = tikfinity.Client.Direct; signed by Euler Stream) │
                        │                                    │                               │
                        │                                    ▼                               │
                        │                              queue.Queue ◄──── queue.json          │
                        │                                    │                               │
                        │   app.Run/tick (every 2 s or on wake): preload, auto mode          │
                        │                                    ▼                               │
                        │      downloader ─► verifier ─► cache\{setId}[n].osz                │──https──► mirrors (hinai, Nekoha, …)
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

* `tikfinity.Client.Run` and `tosu.Client.Run`: one goroutine each; reconnect every `reconnectSec`. In TikTok direct mode
  `tikfinity.Client.Run` calls `tiktok.Client.Stream` instead (checks every 30 s while offline; failed connections back off up to 5 min).
* `app.Run`: a loop woken by queue changes (`App.Wake`) or a 2 s ticker; starts **one download at a time** and drives auto mode.
* Every chat request is resolved in its own goroutine (`App.submit`), because metadata lookups take network time. A message with several maps is one goroutine
  (`App.submitAll`): its maps are resolved one after another so each sees the ones before it, sharing one `batch` (cooldown, spoken lines).
* `server`: `net/http` on 127.0.0.1; state pushes are coalesced (120 ms) and sent to every WebSocket client.
* `remote` (mod panel listener, only when `remote.enabled`): each overlay connected through the overlay link (`/o/<token>/ws`) has its own loop in the handler (`overlayWS`): a 200 ms ticker builds
  `OverlayState`, sends it when the JSON changed (and at least every 20 s), and re-checks the link on every tick, so switching it off or **New link** closes open overlays at once.
  `Server.overlays` counts them for the pre-stream check.
* Hotkeys: a locked OS thread with a Win32 message loop (`platform.Hotkeys`).
* Overlay pop-ups: `App.emit` appends to a 30-entry feed (`App.Feed`) that is part of every state push; the overlay shows entries newer than the last one it saw.
* `update.Run`: one goroutine; checks GitHub 10 s after start and then every 12 h (skipped for dev builds).

**State ownership:** `queue.Queue` (mutex-guarded) is the single source of truth for requests; `config.Store` for settings
(subscribers via `OnChange`). The UI never holds state of its own beyond view preferences in `localStorage`.

**Dependency direction:** `main` → `app` → (`parser`, `osumeta`, `search`, `validator`, `queue`, `downloader`, `verifier`, `importer`, `tikfinity`, `tosu`, `update`) → `config`.
`tiktok` depends on `tikfinity` (it produces `tikfinity.Chat`/`Gift`); `main` wires `tiktok.New().Stream` into `tikfinity.Client.Direct`.
`server` depends on `app`. `remote` depends on `app`, `config` and `queue` but not on `server`: `main` hands it the overlay page (`server.OverlayPage()`) and the version, and hands `server`
the overlay count (`srv.RemoteOverlays = mods.Overlays`), which the pre-stream check adds to its own. Only `main` imports `platform` (Windows APIs are injected into `tikfinity`, `tosu`, `downloader`, `importer` as functions,
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
├── THIRD_PARTY_LICENSES.txt     licenses of the Go standard library, x/sys, coder/websocket, protobuf-go and gotiktoklive (shipped in the release zip)
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
    │                 search_test.go        song-name and gift-only requests against a fake mirror + osu!
    │                 multi_test.go         several maps per message (batch rules, cooldown, limits, TTS, gift-only)
    ├── config/       config.go             config.json schema, defaults, validation, store with change subscribers
    ├── parser/       parser.go (+_test)    "!req …" → Request{Kind, SetID, BeatmapID, Mode, Bare}; ParseAll → []Part
    ├── osumeta/      osumeta.go (+_test)   osu.ppy.sh scraping: set metadata, reference MD5, beatmap→set, raw .osu fallback
    ├── mods/         mods.go (+_test)      mod parsing, clashes, speed, "played without"
    ├── search/       search.go (+_test)    song search on the mirrors, re-ranking by title match and plays
    │   └── testdata/                       a real (trimmed) search response
    ├── update/       update.go (+_test)    GitHub "latest release" check
    ├── support/      support.go (+_test)   the support zip
    ├── remote/       remote.go (+_test)    mod panel: second listener, PIN sessions, private-network guard, overlay link (`/o/{token}`)
    │   └── web/mod.html                    the phone page (embedded), with the ▶ KIKI player
    ├── validator/    validator.go (+_test) CheckUser / CheckMap → *Rejection{Code, Detail}
    ├── queue/        queue.go (+_test)     items, now playing, history, rejections, cooldowns; queue.json persistence
    │                 gifts.go (+_test)     gift credits, priority tokens, prioritise, paid requests
    │                 summary.go (+_test)   stream session and its summary
    ├── downloader/   downloader.go (+_test) sources, timeouts, ZIP sniffing, official browser flow, cache, cleanup, CachedFile (the map player)
    ├── verifier/     verifier.go (+_test)  in-memory unzip + MD5 compare (strict / requested)
    │   └── testdata/                       .osz fixtures (regenerate with -update)
    ├── importer/     importer.go, osudb.go (+_test)  open / copy / extract, Songs dir, safe paths, osu!.db hash scan
    ├── tikfinity/    tikfinity.go (+_test, direct_test) TikFinity detection + WebSocket client + chat parsing; direct mode loop; viewer count
    ├── tiktok/       tiktok.go (+_test)    TikTok direct: room lookup, Euler Stream signing, LIVE WebSocket, chat/gift mapping, viewer count
    ├── tosu/         tosu.go (+_test)      tosu WebSocket client + song-end Tracker
    ├── platform/     platform_windows.go, hotkey.go (+_test)  Win32: ShellExecute, processes, registry, hotkeys
    └── server/       server.go (+_test)    HTTP routes, guard, state snapshot, WebSocket push, pre-stream check endpoint, cached .osz for the player, overlay token
        └── web/                            embedded into the exe with go:embed
            ├── index.html                  dashboard markup (Bootstrap 6 components)
            ├── app.js                      dashboard logic (ES module, no build step)
            ├── app.css                     dashboard styles: brand tokens and what Bootstrap lacks
            ├── bootstrap.min.css           Bootstrap 6.0.0-alpha.1, unmodified (MIT)
            ├── bootstrap.bundle.min.js     Bootstrap 6 JavaScript with Floating UI and Vanilla Calendar Pro (MIT)
            ├── overlay.html                OBS overlay (self-contained HTML+CSS+JS) (also served by `remote` at /o/{token})
            └── icon.svg                    favicon
```

### File by file

| File | Lines | What is in it | Key symbols |
|---|---:|---|---|
| `main.go` | ~250 | Parses `-dir` / `-no-browser`, titles the CMD window, picks the base folder (exe folder, or cwd under `go run`), sets up `osureq.log` (rotates at 5 MB), loads config, fixes the osu! folder from the registry, builds every module, wires callbacks (chat/gift → app, tosu → app, status → push, config changes → hotkeys/reconnects), hands off to an already running copy, starts the goroutines, waits for Ctrl+C / window close / Quit | `main`, `exeDir`, `setupLog`, `fatal`, `alreadyRunning` |
| `internal/app/app.go` | ~750 | The brain. Turns chat into queue items (a message with several maps: `submitAll`, one `batch` per message), runs downloads/preload, handles Next/Play/Skip/Remove/Retry, auto mode and play results via tosu, dry run, test panel, notices | `App`, `HandleChat`, `SimulateChat`, `TestRequest`, `submit`/`submitAll`/`resolve`/`lookup`, `SimulateMessage`, `batch`, `Run`/`tick`, `OnGame`, `ResultShort`, `startDownload`/`download`, `preImport`, `Next`, `Play`/`play`, `Skip`, `ClearTest`, `ToggleDryRun`, `Hotkey`, `warnOnce`; `origin` (`fromChat`/`fromSim`/`fromQuick`) |
| `internal/app/feed.go` | ~50 | Events for the overlay pop-ups (added, rejected, command, gift, result), newest first, 30 kept; the unexported `quiet` keeps an event off TTS | `FeedEvent`, `emit`, `Feed` |
| `internal/app/commands.go` | ~100 | Chat commands other than requests; `IsMod` (Mods list or TikTok moderator flag) | `command`, `IsMod` |
| `internal/app/gifts.go` | ~60 | Gift coins → priority tokens → prioritised requests; simulated gifts | `HandleGift`, `SimulateGift`, `applyCredit` |
| `internal/app/ban.go` | ~70 | Adds to a blacklist and removes matching waiting requests | `Ban` |
| `internal/app/preflight.go` | ~180 | The pre-stream check | `Preflight`, `PreflightEnv`, `Check` |
| `internal/gtts/gtts.go` | ~150 | Google Translate's voice for TTS when the browser has none for the language: 180-letter pieces joined as one MP3, last 30 lines cached | `Client`, `Speak`, `Split` |
| `internal/search/search.go` | ~420 | search on hinai → Nekoha → osu.direct → catboy (status filter, 429 rest, 1 request/s, 10 min cache), `Playable` (mode filter incl. converts), `Rank` | `Client`, `Mirror`, `Find`, `Parse`, `Rank`, `Result` |
| `internal/update/update.go` | ~150 | Release check: `Newer` compares x.y.z, dev builds and pre-releases are never offered, links and zip/sha256 assets only from this repo's releases | `Run`, `Latest`, `Newer`, `Release` |
| `internal/update/install.go` | ~200 | Automatic update: download the zip, check SHA-256, stage `osureq.update-<v>.exe` next to the exe, swap it in (`osureq.exe.old` kept until the next start), start the new copy in the same console | `Download`, `Staged`, `Install`, `Cleanup`, `Relaunch` |
| `internal/mods/mods.go` | ~120 | Mod words → canonical string, clash rules, `Rate`, `Harder` (DT/NC/HR), `Missing` | `Parse`, `Has`, `Rate`, `Harder`, `Missing`, `Error` |
| `internal/remote/remote.go` | ~800 | Mod panel: `Apply` starts/stops the listener on config changes; `guard` (private addresses, local Host names, header, CSP); PIN login with lockouts; limited state and actions; the overlay link (`/o/{token}`, `/o/{token}/ws`: `overlayOK`, `overlayWS`, `OverlayState`); `viewers`, `kikiPlayer` and `beatmapId` for the phone page | `Server`, `Apply`, `Status`, `Handler`, `Overlays`, `OverlayState` |
| `internal/remote/web/mod.html` | ~370 | The phone page (embedded): queue, now playing, actions, PIN login, viewer count; **▶ view map** puts KIKI's player (an iframe straight at `player.kiki-bm.app`, maps from the mirrors only) under a row | `kikiURL`, `kikiToggle`, `kikiMods` |
| `internal/support/support.go` | ~130 | Support zip, home-folder redaction, log tail | `Write`, `Input` |
| `internal/queue/summary.go` | ~110 | Session start (kept across restarts, new after 6 idle hours) and the summary | `Summary`, `NewSession` |
| `internal/config/config.go` | ~310 | All settings structs with JSON names, `Default()`, `Validate()`, download host whitelist, `Store` (atomic save via `.tmp` + rename, `OnChange` subscribers). Unknown/missing fields fall back to defaults because the file is unmarshalled over `Default()` | `Config`, `Connection`, `Osu`, `Command`, `Rules`, `Queue`, `Source`, `Download`, `MD5`, `Import`, `Overlay`, `Hotkeys`, `Test`, `HostAllowed`, `Store`, `NewOverlayToken`, `GoodOverlayToken` |
| `internal/parser/parser.go` | ~150 | Command matching (case-insensitive, aliases), ID and link parsing, `#mode/id` fragments, rejects foreign hosts and non-numeric IDs. `Bare` marks plain numbers so the app can retry as the other kind. `ParseAll` reads a message with several maps (`Options.MaxMaps`) into `Part`s | `Parse`, `ParseAll`, `Part`, `ParseArg`, `Request`, `Request.Flip`, `Kind` |
| `internal/osumeta/osumeta.go` | ~290 | **The only code that talks to osu.ppy.sh for metadata.** Rate-limited (700 ms between requests), 10 min cache, `json-beatmapset` extraction, beatmap→set resolution, raw `.osu` fallback (`Partial` sets) | `Client`, `Get`, `ResolveSet`, `ParsePageJSON`, `ParseOsuHeader`, `Set`, `Diff`, `ErrNotFound` |
| `internal/validator/validator.go` | ~210 | Pure functions, no I/O. User checks (cheap, before any network: blacklist, queue size, followers-only, per-user/subscriber limit, cooldown) and map checks; picks the diff for whole-set requests (main mode first) | `CheckUser`, `CheckMap`, `State`, `Roles`, `Input`, `Rejection` (`Code`, `Detail`, `Short`) |
| `internal/queue/queue.go` | ~550 | Thread-safe queue + now playing + history (200, `Played.Mode` for the ▶ button) + rejections (100) + per-user last request; debounced save to `queue.json`; on load, unfinished downloads and ready items go back to `queued` so files are re-verified | `Queue`, `Item`, `State` (`queued`/`downloading`/`ready`/`failed`/`playing`), `Attempt`, `Played`, `Rejected`, `ValidationState`, `Promote`, `Drop`, `Move`, `RemoveLatestBy`, `Position`, `ClearTest`, `Snapshot`; `Result` (play result) |
| `internal/queue/gifts.go` | ~100 | Per-user gift coins and priority tokens (with expiry); moving a request behind the other priority requests | `Credit`, `AddCoins`, `UseToken`, `Tokens` |
| `internal/downloader/downloader.go` | ~450 | Cache lookup, source loop, streaming download with progress, early "not a ZIP" detection, official browser flow (watches Downloads, waits for the size to settle, ignores `.crdownload`/`.part`), verified-file registry (`Trusted`; `CachedFile` serves the map player), LRU-ish cleanup | `Downloader`, `Get`, `Job`, `Outcome`, `Trusted`, `CachedFile`, `Cleanup`, `CacheSize` |
| `internal/verifier/verifier.go` | ~140 | ZIP magic check, in-memory unzip (64 MB per `.osu` cap), MD5 per `.osu`, comparison at `strict`/`requested` level | `Verify`, `Compare`, `OsuHashes`, `LooksLikeZip`, `Reference`, `Result` |
| `internal/importer/importer.go` | ~210 | Refuses untrusted files, `open`/`copy`/`extract`, Songs folder from `osu!.<user>.cfg` `BeatmapDirectory`, Windows-safe names, traversal-safe joins | `Importer.Import`, `SongsDir`, `CopyToSongs`, `ExtractToSongs`, `SafeJoin`, `SafeName` |
| `internal/importer/osudb.go` | ~70 | Finds beatmap MD5s in `osu!.db` by scanning for `0x0b 0x20 <32 hex>` instead of parsing the versioned format | `OsuDB.Has`, `ScanHashes` |
| `internal/tikfinity/tikfinity.go` | ~690 | Install/process checks, WebSocket loop, `config` handshake, live detection (any event; times out after 5 min of silence), raw event log (first event of each type kept), chat field extraction with fallbacks, viewer roles (yes/no/unknown), gift parsing (streaks counted once), viewer count (`Status.Viewers`, `OnViewers`, `ParseViewers`), TikTok direct loop (`Settings.Direct`: status codes `no_user`/`no_account`, back-off) | `Client`, `Status` (`red`/`yellow`/`green`, codes), `Chat`, `Tri`, `ParseChat`, `Gift`, `ParseGift`, `ParseViewers`, `Accepting` |
| `internal/tiktok/tiktok.go` | ~450 | TikTok direct: live check, Euler Stream signing, LIVE WebSocket (enter room, heartbeat, ack, gzip), chat/gift/role/picture mapping, viewer count (`WebcastRoomUserSeqMessage.Total` → `DirectEvents.Viewers`), stream end | `Client`, `New`, `Stream` (a `tikfinity.Direct`), `RoomID`, `CleanUser` |
| `internal/tosu/tosu.go` | ~420 | tosu WebSocket loop (`/websocket/v2`), payload parsing (state, beatmap, live `play` numbers, `resultsScreen`), status levels (only the short `Brief` state drives pushes), remembers `tosu.exe` path, song-end state machine | `Client`, `Status`, `Game`, `Brief`, `Score`, `Parse`, `Tracker`, `Phase` (`waiting`/`playing`/`results`/`ended`) |
| `internal/platform/platform_windows.go` | ~280 | `ShellExecuteW` open, Toolhelp process list, process image path, TikFinity lookup (path + Uninstall registry), osu! stable lookup (`HKCR\osustable.File.osz\shell\open\command`), launch in a new console window, close by exe name (`WM_CLOSE`, then `TerminateProcess` after 3 s), `RegisterHotKey` message loop, console window title | `ShellOpen`, `ProcessRunning`, `ProcessPath`, `FindTikFinity`, `FindOsuStable`, `Launch`, `Close`, `Hotkeys`, `SetConsoleTitle` |
| `internal/platform/hotkey.go` | ~50 | Parses `Ctrl+Alt+N`, `Shift+F5`, `F9`, `Win+Num5`… into modifier + virtual key | `ParseHotkey` |
| `internal/server/server.go` | ~420 | Routes, `guard` (Host + `X-Osureq` header), `State` snapshot, coalesced WebSocket push + 20 s keepalive state, config GET/PUT, actions (incl. quit and closing tosu/TikFinity), queue operations, test/simulate/simulateGift, ban, `GET /api/osz/{setId}` (cached `.osz` for the map player), `POST /api/overlay/token`, `OverlayPage` (the overlay page for the overlay link) | `Server`, `State`, `Handler`, `Push`, `Listen`, `Quit`, `osz`, `overlayToken` |
| `internal/server/web/index.html` | ~190 | Dashboard skeleton: sticky top bar (status pills, requests switch, volume menu, language, settings, ⋮ menu), status box, now playing, test panel, queue, side column (summary, tabbed feeds, mirrors, info), pre-stream check drawer, Map rules drawer, settings dialog, toast container | |
| `internal/server/web/app.js` | ~860 | Everything on the dashboard: `T` i18n table (en/th), API helper, WebSocket client, incremental rendering, queue cards, drag & drop, ban menu, audio preview + viewer links, results, test panel (chat / quick / gift), settings `SCHEMA` → form, the KIKI map player (iframe + `postMessage`, see [KIKI Beatmap Viewer player](#kiki-beatmap-viewer-player)) | `render`, `renderBanner`, `renderTosuBanner`, `renderNow`, `updatePhase`, `resultShort`, `renderQueue`, `buildCard`, `cardMenu`, `mapLinks`, `updateAttempts`, `renderSide`, `same`, `SCHEMA`, `field`, `kikiBtn`, `kikiToggle`, `kikiLoad` |
| `internal/server/web/app.css` | ~380 | Brand tokens (light and dark, fed into Bootstrap's `--bs-*` tokens) and the parts Bootstrap lacks: top bar pills, status box, now playing, queue rows, map tags, download bars, side logs, settings layout | `:root` tokens |
| `internal/server/web/bootstrap.min.css`, `bootstrap.bundle.min.js` | | Bootstrap 6.0.0-alpha.1 as published on npm (source map comments removed); the licences of Bootstrap, Floating UI and Vanilla Calendar Pro are in `THIRD_PARTY_LICENSES.txt` | |
| `internal/server/web/overlay.html` | ~160 | OBS page: transparent, now playing + upcoming (priority ★), pop-ups from the feed with their own en/th text table `TX`, theme/scale/position/language from config, reconnecting WebSocket (`/ws?client=overlay`; loaded from `/o/{token}` it uses `<that path>/ws`) | `render`, `showToasts`, `toastHTML` |
| `internal/verifier/testdata/*.osz` | | `set_good`, `set_outdated` (old Hard diff), `set_extra` (extra `.osu`), `set_no_osu`, `not_zip` (HTML) | regenerate: `go test ./internal/verifier -update` |
| `*_test.go` | | parser (link forms, hostile inputs, `ParseAll`), validator (every rule, exempt users), verifier (fixtures, levels), downloader (fake mirrors: fallthrough, cache, skip/warn, MD5 off, cleanup), importer (traversal, Songs dir, osu!.db scan), queue (limits, ClearTest, gift credits and ordering), app (chat commands, play results, bans, several maps per message), osumeta (page JSON, .osu header), tikfinity (chat parsing, roles, gifts, levels, viewer count), tosu (payload incl. scores, Tracker paths, client vs fake server), platform (hotkey parsing) | |

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
* Viewer count (**unverified**, `tikfinity.ParseViewers`): events named `roomUser` (TikTok-Live-Connector's name for TikTok's room user message), `viewerCount` or `viewer` (case-insensitive;
  the last two are guesses), with the number in `data.viewerCount`, else `viewers`, `total` or `totalUser`: a non-negative whole number up to 1e9, anything else is ignored.
  Check the raw events of a real LIVE. The count goes to `Status.Viewers`; it is reset to 0 when the connection or the LIVE drops.
* The dashboard keeps the first event of every type, and with `logRawEvents` the last 100 events, so field names can be checked without a debugger.

### TikTok direct (`internal/tiktok`)

Used when `connection.source` is `tiktok`. Every request sends osureq's own User-Agent (`osureq/1.0 (TikTok LIVE request helper)`); nothing pretends to be a browser.

* Live check: `GET https://www.tiktok.com/api-live/user/room/?aid=1988&sourceType=54&uniqueId={user}`. `statusCode` 0 and
  `data.liveRoom.status` (else `data.user.status`) 2 = live with `data.user.roomId`; 4 = ended; `statusCode` 19881007 = no such account
  (**verified**). Free (no sign server). TikTok's firewall answers 403 for a while after many quick lookups; normal use is one every 30 s.
* Signing: `GET https://tiktok.eulerstream.com/webcast/fetch/?client=osureq&room_id={room}&uuc=1&client_enter=true` (+ `apiKey` query and
  `x-api-key` header when a key is set). Answer: protobuf `WebcastResponse` with `pushServer`, `routeParamsMap` (signature parameters),
  `cursor`, `internalExt`, and header `X-Set-TT-Cookie` (`ttwid`) (**verified keyless**). 429 = limit (`tikfinity.ErrRateLimited`, retried after
  5 min). Keyless limits from `/webcast/rate_limits`: 5 a minute, 30 an hour, 100 a day. Euler's newer `/webcast/rooms/{id}/connect` refuses
  non-browser User-Agents, so it is not used. Errors never include the URL (the key could be in it).
* WebSocket: `pushServer` + fixed web parameters + `routeParamsMap` + `room_id`, `cursor`, `internal_ext`, `compress=gzip`, `client_enter=1`,
  with the `ttwid` cookie. Right after connecting the client sends a `WebcastPushFrame` `im_enter_room` (payload `WebcastImEnterRoomMessage`:
  room_id 1, live_id 4 = 12, identity 5 = "audience", enter_unique_id 8 random, filter_welcome_msg 9 = "0"); without it TikTok closes with
  `payload_handler_im_enter_room`. TikTok answers `im_enter_room_resp`. Heartbeat every 10 s: frame `hb` with `HeartbeatMessage` (room_id 1,
  send_packet_seq_id 2). Messages: frame `msg`, payload (gzip) `WebcastResponse`; `needsAck` → frame `ack` with `logId` and `internalExt`.
  No frame for 60 s = dead connection. (**verified** on a real LIVE: connect, enter, member/like/viewer messages.)
* Chat: `WebcastChatMessage` → `user.displayId` (the @name; **verified** on member messages, `idStr` was empty), `user.nickname`, `content`,
  `userIdentity.isModeratorOfAnchor`/`isAnchor` → moderator, `isFollowerOfAnchor`, `isSubscriberOfAnchor`; picture from `avatarThumb`/`Medium`/`Large`/`Jpg`
  `urlList` (same host filter as TikFinity). Chat older than 10 s before connecting and `isHistory` messages are skipped, so a reconnect does
  not replay requests. (**verified** on a real LIVE: chat with @name, nickname and picture.)
* Gifts: `WebcastGiftMessage` → coins = `gift.diamondCount × repeatCount`; `gift.type` 1 (streakable) counts only with `repeatEnd` 1.
* Viewer count: `WebcastRoomUserSeqMessage.Total` (0 to 1e9) → `DirectEvents.Viewers` → `Status.Viewers` (json `viewers`, omitted when 0 = unknown; reset to 0 when the connection drops).
  It also counts as the LIVE being alive. `Client.OnViewers` reports a change without going through `OnStatus`, which would log a line for every number; `main.go` only pushes the state.
* `WebcastControlMessage` action 3/4 (stream ended / ended by ban) → `tikfinity.ErrOffline`, back to checking every 30 s.
* gotiktoklive's own client is not used: it fills the user name with the numeric ID and reads some fields without nil checks.

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

**Beatmap viewer link:** `osu.viewerUrl` (default `https://kiki-bm.app/`, KIKI Beatmap Viewer) + `?s={setId}&b={beatmapId}`. It is only a link opened in the
browser; the helper sends nothing to the viewer. The viewer loads map files from public mirrors itself.
The default was `https://osu-beatmap-viewer.vercel.app/` before; `config.Load` turns exactly that value into the new default (kept in memory and written
with the next save). An empty value, any other URL and templates are left alone.
If `osu.viewerUrl` contains `{set}`, `{map}` or `{file}` it is used as a template instead: `{file}` becomes the URL-encoded hinai download
(`https://mirror.hinamizawa.ai/api/v1/hinai/d/{setId}`), e.g. `https://player.kiki-bm.app/s/{set}?file={file}&diff={map}`.
The embedded **▶ view map** player is separate from this link (see [KIKI Beatmap Viewer player](#kiki-beatmap-viewer-player)); both can be on.

User-Agent `osureq/1.0 (TikTok LIVE request helper)`. Never embed osu! OAuth secrets.

### KIKI Beatmap Viewer player

The **▶ view map** button embeds the player of [KIKI Beatmap Viewer](https://api.kiki-bm.app/) (`https://player.kiki-bm.app`; free, no key). osureq loads **no script** from KIKI:
it shows an `<iframe>` and talks to it with `postMessage` (code in `internal/server/web/app.js` for the dashboard and `internal/remote/web/mod.html` for the phone page).

* **osu!standard only** (`it.mode === "osu"`, `kikiShows`). History entries need `Played.Mode`, which older entries lack, so they get no ▶. The switches are `osu.kikiPlayer` (dashboard;
  `State.kikiPlayer`) and `osu.kikiPlayerMod` (mod panel; remote `State.kikiPlayer`); configs without the keys get `true`. `State.noVideo` (`download.noVideo`) says which cached file to ask for first.
* **Dashboard** (`kikiToggle`): a `.kiki` box is put right after the card (queue, now playing, history), one open at a time (module variable `kiki`). The iframe address is
  `https://player.kiki-bm.app/embed?lang=<th|en>&accent=%23<hex>&label=osureq` (`allow="autoplay; fullscreen"`, `loading="lazy"`; the accent is osureq's pink for the current theme).
  Messages are `{kiki: 1, v: 1, type, …}`; osureq accepts them only when `event.origin` is `https://player.kiki-bm.app` **and** `event.source` is that iframe's window, and sends with that origin as `targetOrigin`.
* **Flow**: the player sends `ready`; osureq sends `load` with `set` (`id`, `title`, `artist`, `creator`), `diff` (the beatmap id), `mods` (only EZ HD HR DT NC HT FL, with EZ+HR and DT/NC/HT clashes resolved
  as the player does: `kikiMods`), `lang`, `accent`, `label` and, when osureq has the map cached, `file: {ask: true}`. The player then asks (`need-file`, kind `map`) and osureq fetches
  `GET /api/osz/{setId}` and answers with the `.osz` as a `Blob`. Whether a file exists is found out once when the player opens (`kikiCached`: `HEAD /api/osz/{setId}[?novideo=1]`, which needs `X-Osureq: 1`
  like any non-GET), and it is offered only when the answer is 200 with a size of at most 100 MiB (`KIKI_MAX`). Without a file the player takes the map from the community mirrors itself (it chooses the mirror).
* **Failures**: on the player's `error` `fetch_failed`, `file_too_large` or `bad_file` (when osureq's file was used), osureq sends `cache-clear` for that set and `load` again without a file (`kikiMirrors`, once);
  the viewer presses ▶ in the player again.
* **One sound at a time**: the player's `play` pauses the ♪ preview, and starting a preview sends `pause` to the player.
* **Redraws**: the box is not a card: list rendering skips it (`.kiki`), it is never moved or rebuilt while open (an iframe that moves reloads), and it closes when its card goes away, the History tab is hidden,
  or the setting is turned off.
* **Mod panel**: no cached file (the phone cannot reach `/api/osz`), always the mirrors: the iframe points straight at `https://player.kiki-bm.app/s/{setId}?diff={beatmapId}&mods=…&lang=…&accent=%23ff66aa&label=osureq`
  (or `/b/{beatmapId}?…` without a set id; the accent is the page's `--pink`, `#ff66aa` as fallback). The mod panel's CSP gets exactly `frame-src https://player.kiki-bm.app`; the dashboard sets no CSP header.
* **`GET /api/osz/{setId}`** (and `HEAD`, `?novideo=1`) is in the API table: same origin only (no CORS headers), the path comes from the number, only `Downloader.CachedFile` archives (exist **and** `Trusted`).
  Trust is in memory, so after an osureq restart nothing is served until the set goes through the downloader again; the player then uses the mirrors.
* **Third-party: KIKI player.** Nothing from KIKI is bundled in `osureq.exe`, so `THIRD_PARTY_LICENSES.txt` has no entry for it; it is credited in both READMEs. The player's own ⓘ credits must stay visible
  (KIKI's terms; its default skin and hitsounds are by ppy, CC BY-NC 4.0). The message names and the `v: 1` envelope are KIKI's embed protocol as used here; if KIKI changes it, only the two pages above change
  (see [Known limitations and open items](#known-limitations-and-open-items)).

### Mirrors (`internal/downloader`, URL templates in config)

| Source | URL | No-video URL | Notes |
|---|---|---|---|
In the order kiki-bm.app uses (`config.DefaultSources`), all checked live on 2026-10-30:

| Source | URL | No-video URL | Notes |
|---|---|---|---|
| hinai | `https://mirror.hinamizawa.ai/api/v1/hinai/d/{id}` | `…?noVideo=1` | serves the file itself; 1000 `.osz`/min per IP; `503` + `Retry-After` when its sources are down for a set, `404 content_removed` for a takedown |
| nekoha | `https://mirror.nekoha.moe/api/download/{id}` | `…?noVideo=1` | `404` = not downloaded yet, `410` = DMCA / no audio |
| beatconnect | `https://beatconnect.io/b/{id}/` | none | always includes the video |
| osu.direct | `https://osu.direct/api/d/{id}` | `…?noVideo=1` | 302 to a signed `storage.osu.direct` URL; 120 requests/min, 200 downloads/h, 8 GiB/day per IP |
| catboy | `https://catboy.best/d/{id}` | `https://catboy.best/d/{id}n` | send `Referer: https://catboy.best/`; refuses requests without an identifying User-Agent |
| sayobot | `https://dl.sayobot.cn/beatmaps/download/full/{id}` | `…/download/novideo/{id}` | 302 to a numbered server such as `tc1.sayobot.cn:25225` |
| official | `https://osu.ppy.sh/beatmapsets/{id}/download` | `…?noVideo=1` | opened in the browser; the file is picked up from Downloads |

Nerinyan is left out (530 on search and 404 for every map on 2026-10-30). Allowed hosts (`config.AllowedHosts`): `mirror.hinamizawa.ai`,
`mirror.nekoha.moe`, `beatconnect.io`, `osu.direct`, `storage.osu.direct`, `catboy.best`, `dl.sayobot.cn`, subdomains of `sayobot.cn`, `osu.ppy.sh`;
HTTPS only, any port, no user info. Every redirect hop is checked against the list (`CheckRedirect`), not only the final URL.

`download.sourcesRev` records which default list a saved `sources` was made from. A `config.json` from before these mirrors gets them on load
(`mergeSources`: default order, mirrors the streamer already had keep their settings, their own mirrors come last); a list saved from the
dashboard counts as chosen, so a mirror removed there stays removed.

### Song search (`internal/search`)

The mirrors are asked in order (`search.DefaultMirrors`); one that fails, is resting after HTTP 429, or finds nothing hands over to the next
(hinai answers `200 []` when its own sources are down). Statuses come from `rules.statuses` (osu!'s numbers: `graveyard -2, wip -1, pending 0,
ranked 1, approved 2, qualified 3, loved 4`). `[` and `]` are dropped from the query: they start a filter block on hinai and osu.direct.

| Mirror | Request | Notes |
|---|---|---|
| hinai | `GET https://mirror.hinamizawa.ai/api/v2/search?q=&amount=100&sort=play_count:desc[&status=n]` | only the first status is searched, so with several none is sent and the results are filtered here (ranked counts approved) |
| nekoha | `GET https://mirror.nekoha.moe/api/search?q=&status=ranked,loved…` | status names; answers `{"beatmapsets": [...]}`, 100 a page; older sets have no `play_count`, so the difficulties' `playcount` is added up |
| osu.direct | `GET https://osu.direct/api/v2/search?q=&amount=100&sort=play_count:desc&status=1,4…` | every status in one request |
| catboy | `GET https://catboy.best/api/v2/search?q=&limit=50&status=1&status=4…` | refuses requests without an identifying User-Agent; a trimmed real response is the test fixture |

All answer beatmapsets in osu!'s format (`id`, `artist`, `title`, `creator`, `status`, `play_count`, `beatmaps[].mode`). Their own order
favours obscure uploads and tag matches, hence `Rank`: title contains every word → artist + title contains every word, each by play count; results that match only
  on tags are dropped (so a request answers "no map found" rather than queueing an unrelated map). `artist_unicode`/`title_unicode` count too.

### GitHub releases (`internal/update`)

* `GET https://api.github.com/repos/Lacrymiraosu/tiktoklivereqsong/releases/latest` (`tag_name`, `html_url`, `draft`, `prerelease`), unauthenticated
  (60 requests per hour per IP; osureq uses two a day). Only `html_url` under this repo's releases page is passed on.
* With `autoUpdate`, the `osureq-<v>-windows-x64.zip` and `.zip.sha256` assets are downloaded, but only from
  `https://github.com/Lacrymiraosu/tiktoklivereqsong/releases/download/` (GitHub then redirects to its file host). The zip must match the hash file;
  its root `osureq.exe` is written to `osureq.update-<v>.exe` next to the running exe (`.part` first, then renamed).
* Install: on start, before the dashboard or anything else runs, a staged update newer than the running version is swapped in
  (`osureq.exe` → `osureq.exe.old`, staged → `osureq.exe`; put back if the second rename fails) and started with the same arguments in the same console,
  and the old copy exits. The dashboard's update button (`installUpdate`) does the same after a clean shutdown and adds `-restarted`, which makes the new
  copy wait up to 15 s for the port and not open another browser tab. Open dashboards and overlays reload themselves when the version changes.
  osureq never restarts on its own while running.

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
| `GET /assets/{file}` | embedded `app.js`, `app.css`, `bootstrap.min.css`, `bootstrap.bundle.min.js`, `icon.svg`; the content type of `.js`, `.css` and `.svg` is set by the server (`assetTypes`), not taken from the Windows registry, because a module script needs a JavaScript type |
| `GET /ws` | WebSocket; pushes the full `State` JSON on every change (and every 20 s). The overlay adds `?client=overlay` so the pre-stream check can count it; a dashboard page adds `?page=<id>` (who reads TTS) |
| `GET /api/state` | the same `State` once |
| `GET /api/config` | `{config, defaults}` |
| `PUT /api/config` | replace the whole config (validated); `{ok, restartNeeded}`. `overlay.remoteToken` in the body is ignored: the stored token is kept, and one is made when `overlay.remote` is switched on for the first time. Everything else is stored as sent, so the dashboard builds the body from a fresh GET (see [Map rules drawer](#map-rules-drawer)) |
| `POST /api/overlay/token` | **New link**: makes a new secret for the overlay link (`overlay.remoteToken`) → `{ok, token}`. The old link stops working at once, also for open overlays (their connection re-checks every tick). The only way to replace the token |
| `POST /api/action/{name}` | `next`, `previous` (back to the map played before: osu! opens it again, the current map returns to the front of the queue), `skip`, `removeLast`, `toggleRequests`, `clear`, `cleanup`, `toggleDryRun`, `toggleAutoDownload`, `clearTest`, `launchTikfinity`, `reconnect`, `openTikfinityDownload`, `launchTosu`, `reconnectTosu`, `openTosuDownload`, `stopTosu`, `stopTikfinity`, `quit`, `quitAll` (closes tosu and TikFinity, then quits), `installUpdate` (only when a downloaded update is ready: quits, installs and starts the new version), `showSummary`, `newSession`, `checkMirrors` (probes every download mirror now; otherwise they are probed once, 15 s after start, and downloads keep the panel current) |
| `POST /api/queue/{id}/{op}` | `remove`, `skip`, `play`, `retry`, `move` (body `{"to": <index>}`) |
| `POST /api/test` | quick add: `{"input": "<id or link>", "user": "<name>"}` → `{ok, item}` or `{ok:false, code, error}` |
| `POST /api/simulate` | simulated chat: `{"message": "!req …", "user": "<name>"}`; `code: "ignored"` when it is not a request; for a chat command `{ok, command: "cmd_done" \| "cmd_not_mod" \| "cmd_nothing"}`; a message with several maps (see [Several maps in one message](#several-maps-in-one-message)) answers `{ok, item, code, short, error, maps, count, rejected, ignored}` |
| `POST /api/simulateGift` | `{"user": "<name>", "coins": 30}` → `{ok, earned, requests}` (priorities and paid requests earned) |
| `POST /api/support` | the support zip (`application/zip`, attachment) |
| `POST /api/preflight` | runs the pre-stream check (a few seconds) → `{ok, checks: [{id, status, key, vars, detail}]}` |
| `POST /api/volume` | `{"volume": 0-100}` sets the master volume |
| `POST /api/tts` | `{"lang": "th"\|"en", "text": "…"}` (at most 600 letters) → `audio/mpeg` from Google's voice; 502 with the reason if Google fails |
| `POST /api/tts/speaker` | `{"page": "<id>", "on": bool}`: this dashboard page reads TTS (`on`), or switched it off; see [Chat TTS](#chat-tts) |
| `PUT /api/preview` | `{"setId": n}`: the set the dashboard is playing a preview of (0 = none), shown on the mod panel |
| `POST /api/jump` | `{"setId": n, "beatmapId": n}`: osu! jumps to a history map (`osu://b/<beatmapId>`; without `beatmapId`, the set's first difficulty from osu!); the queue is not touched |
| `POST /api/ban` | `{"kind": "user" \| "set" \| "mapper", "value": "…"}` → `{ok, removed}` |
| `GET /api/osz/{setId}` (and `HEAD`) | the cached `.osz` for the map player, `?novideo=1` asks for the no-video file first (else the other one). Only a file that exists and is trusted (`Downloader.CachedFile`: checked by this run of the helper); otherwise 404, and the player uses the mirrors. 400 for a bad id. `Content-Type: application/x-osu-beatmap-archive`, `Cache-Control: no-store`; no CORS headers, so only the dashboard itself can read it. `HEAD` needs `X-Osureq: 1` like every non-GET |

`State` fields: `version`, `tikfinity` (status; `source` `tikfinity`/`tiktok` and `user` say which chat source it describes; `viewers`, people watching, left out when unknown), `tosu` (status incl. `game`), `autoGap`, `queue` (`items`, `now`, `history`, `rejected`),
`requests`, `resolving`, `notices`, `feed` (overlay pop-up events), `cacheMb`, `overlay` (`remoteToken` is blanked), `command`, `search`, `requestCoins`, `lang`, `playMode`, `viewerUrl`, `kikiPlayer` (`osu.kikiPlayer`), `noVideo` (`download.noVideo`), `blacklistUsers`, `dryRun`,
`voteSkip` (`votes`, `needed`), `tts` (settings), `speech` (chat to read), `ttsSpeaker`/`ttsSpeakerOn` (the dashboard page that reads it), `update` (`version`, `url`, only when newer), `summary`, `summaryShown`, `raw`, `firstRaw`,
`previewAsk` (`setId`, `n`: a mod panel preview press, `n` grows per press), `mirrors` (`name`, `state` ok|failed|resting|unknown, `error`, `ms`, `at`), `mirrorCheck`,
`extra` (`hotkeyErrors`, `baseDir`, `cacheDir`).

**Mod panel** (separate listener, `remote.port`, default 21281, only when `remote.enabled`): `GET /m` (page), `POST /m/login` `{"pin"}`
(sets the `osureq_mod` cookie; `401 wrong_pin`, `429 locked`), `POST /m/logout`, `GET /m/api/state` (now playing, queue, requests, TikFinity/tosu
level, votes, `viewers` when known, `kikiPlayer` = `osu.kikiPlayerMod`, and `beatmapId` on every item for the map player), `POST /m/api/action/{next|previous|skip|toggleRequests}`, `POST /m/api/queue/{id}/{remove|skip|move|preview}` (`preview` plays or stops the map's preview on the dashboard of the stream PC). The overlay link ([Overlay link](#overlay-link)) lives on the same listener under `/o/`. Everything else answers 404.

Example (PowerShell):

```powershell
Invoke-RestMethod -Method Post -Uri http://127.0.0.1:21280/api/simulate -Headers @{ "X-Osureq" = "1" } -ContentType application/json -Body '{"user":"fan","message":"!req 2096297"}'
```

### Overlay link

For OBS on a second PC (`overlay.remote`). It lives on the mod panel's listener (`internal/remote`, `remote.port`), the only listener other machines can reach, so `remote.enabled` must be on too; the dashboard
stays on 127.0.0.1. `main.go` gives `remote.Server` the page (`server.OverlayPage()`, the embedded `overlay.html`), the version and, the other way round, the overlay count (`srv.RemoteOverlays = mods.Overlays`);
`remote` does not import `server`.

| Method & path | Purpose |
|---|---|
| `GET /o/{token}` | `overlay.html`; the page opens its WebSocket at its own path + `/ws` instead of `/ws?client=overlay` |
| `GET /o/{token}/ws` | WebSocket pushing `remote.OverlayState` as JSON |
| any other path or method under `/o/` | 404 |

* **Link**: `http://<LAN IP>:<remote.port>/o/<token>`, built by Settings → Overlay from the mod panel's addresses (with a Copy button and **New link**; the dashboard asks `POST /api/overlay/token` when the switch is first
  turned on, so the link shows before saving, and the tab's Reset keeps the token). `overlay.remoteToken` is 32 random bytes as base64url, 43 characters (`config.NewOverlayToken`), made by the helper when
  `overlay.remote` is first switched on (`PUT /api/config`, or when `config.json` was edited by hand: `Load`) and by **New link**. A token typed into `config.json` must be 43-128 base64url characters (`overlay_token`).
  It is blanked in `/api/state`, shown as `(hidden)` in the support file and never logged.
* **Checks** (`overlayOK`): the feature must be on (`overlay.remote`, a token, mod panel on), else 404 and nothing is counted. The token is compared in constant time (SHA-256 hashes, `subtle.ConstantTimeCompare`).
  A wrong token gets the same 404 as a path that does not exist and counts toward the same per-address lockout as a wrong PIN (`lockedIP`/`failIP`: 5 failures, 10 minutes; a locked address gets 404 even with the right
  token). It does not count toward the panel-wide lock, so guesses cannot black out the PIN login, and the right token does not reset the PIN count. Clients outside the private networks (loopback passes) or with
  a bad `Host` get 403 from `guard`, as for the mod panel.
* **Departure from the PIN rule**: the hashes of the last 4 links replaced while osureq runs (`retired`) answer 404 without counting as failures, so an OBS left on the old link cannot lock its own PC out of the new one.
  They are in memory, lost on restart.
* **Unclean paths** (`//o/<token>`, `/o/./<token>`) are answered 404 by `guard`: Go's router would redirect to the clean path and put the token in `Location`.
* **Headers**: `Cache-Control: no-store`, `X-Content-Type-Options: nosniff`, `Referrer-Policy: no-referrer`, `X-Frame-Options: DENY` and a CSP made for the overlay (`overlayCSP`: `default-src 'none'`, its inline
  script and styles, Google Fonts, `img-src https: data:`, `connect-src 'self'`, `frame-ancestors 'none'`). The WebSocket accepts same-origin connections only.
* **Data** (`remote.OverlayState`): a whitelist built field by field, exactly what `overlay.html` reads: `version`, `overlay` (visible, showRequester, theme, scale, position, feedback, feedbackSec, lang, avatars),
  `queue` (`now`, `items`), `feed`, `requests`, `tikfinity.level`, `command`, `search`, `requestCoins`, `summary`, `summaryShown`. No raw events, paths, config, TTS, blacklist, tokens or keys. It does contain viewer
  names, requested songs and picture URLs, so anyone on the network with the link can watch the overlay, but cannot control anything.
* **Push**: see [Architecture](#architecture): every 200 ms when changed, the whole state at least every 20 s; the link is re-checked every tick. A connected overlay counts in the pre-stream check.
  TTS still plays in the dashboard page on the stream PC.

---

## Dashboard internals

* **Bootstrap 6, no build step.** The components (buttons `btn-solid theme-primary`, `menu`, `dialog`, `drawer`, tabs, toasts, `switch`, `form-control`, alerts, avatars) come from the vendored
  `bootstrap.min.css` and `bootstrap.bundle.min.js`; `app.js` is an ES module that imports the bundle and uses a tiny `h(tag, attrs, ...children)` helper to build the DOM.
  Edit the files and rebuild the exe (they are embedded with `go:embed`, so a running copy does not see edits). Bootstrap 6 is an alpha and its class names differ from v5
  (`btn-solid theme-primary` for `btn btn-primary`, `<dialog class="dialog">` for the modal, `drawer` for the offcanvas, `menu` for the dropdown, `md:col-6` for `col-md-6`);
  the migration guide is at getbootstrap.com/docs/6.0/guides/migration. To update it, replace the two files from the npm package, keep the licence block in `THIRD_PARTY_LICENSES.txt` current and run the dashboard in a browser.
* **Browsers:** Bootstrap 6 needs Chrome or Edge 130, Firefox 132 or Safari 18 or newer (`light-dark()`, `:has()`, native `<dialog>`). Older browsers show a broken page. The overlay and the mod panel do not use Bootstrap.
* **Menus on queue cards** are Bootstrap menus (`data-bs-container="body"`, `data-bs-strategy="fixed"`). A card whose menu is open is not rebuilt (`data-menu-open`), otherwise a state push would pull the menu away from its button.
* **Toasts in the top layer.** A toast raised while Settings, the pre-stream drawer or the Map rules drawer was open used to be hidden behind it (a server validation error on Save, for example). `#toasts` is now a manual popover
  (`popover="manual"` in `index.html`; `app.css` drops the browser's popover box with `.toast-container[popover]` and keeps it hidden while not open), shown by `toast()` while it holds a message and hidden when the last one goes.
  A popover shown before a modal lies below it, and a modal makes the rest of the page inert, so while a modal dialog or drawer is open `placeToasts` moves `#toasts` into the newest open one (the `modals` list, newest last) and shows it again;
  when that one closes it goes back to `<body>` (or to the next modal). `modalChange(d, open)` keeps the list; it is called by a `MutationObserver` on every dialog's `open` attribute and by `hide.bs.dialog` and `hide.bs.drawer` listeners on `document`.
  Those two events matter: Bootstrap hides the components inside a dialog (the toasts too) together with it, so the container has to leave when hiding starts, not when `open` is removed. This relies on Bootstrap firing the hide events before it hides
  child components: check it when the vendored Bootstrap is updated. It also needs the Popover API (`popover`, `showPopover()`, `:popover-open`), which every browser the dashboard already requires has (next bullet).
* **Rendering rule (important):** state arrives up to ~4 times per second during downloads. Never rebuild a section unconditionally.
  `same(el, key)` skips a section whose input did not change; queue cards are kept in a `Map` by item id and rebuilt only when their own data changes;
  download bars are updated in place (`updateAttempts`); cards that were already shown get the `seen` class so their entrance animation does not replay.
  Breaking this brings back the flicker fixed in 1.1.1.
* **Map player and viewer count**: an open ▶ player is a `.kiki` node next to its card that list rendering ignores and never moves or rebuilds (an iframe that moves reloads), see
  [KIKI Beatmap Viewer player](#kiki-beatmap-viewer-player). The viewer count is written straight into the status pill (`#tfViewers`) and left out of the banner's `same` key, so a new number does not redraw it.
* **i18n:** every UI string is a key in the `T` table: `key: ["English", "Thai"]`. Use `t("key", {vars})`; static HTML uses `data-t` / `data-tp`. Add both languages.
* **Settings form** is generated from `SCHEMA` (`F(path, type, labelEn, labelTh, hintEn, hintTh, extra)`); the map rules are the separate `RULES` list (see below). Types: `switch`, `text`, `hotkey`, `number`,
  `list`, `numlist`, `seg`, `select`, `multi`, `button`, `overlaylink`, `ovremote` (the overlay link), `sources`, `voices` (one browser voice per TTS language). A new config field needs: Go struct field + default + validation (if any) +
  a `SCHEMA` entry + a row in the [Settings reference](#settings-reference-configjson).
* **Design language:** osu! pink as the one accent on a violet-tinted neutral ground, light and dark by the browser's setting (`prefers-color-scheme`). All colours are tokens in `:root` of `app.css`
  (`--brand`, `--bg`, `--surface`, `--ink`, `--muted`, `--line`, `--ok`, `--warn`, `--bad`, `--info`) and Bootstrap's `--bs-*` tokens are set from them, so Bootstrap components follow both themes.
  Fonts: Kanit (headings), IBM Plex Sans Thai (text), IBM Plex Mono (numbers). The cover banner is `.cover-bg` (set `--cover` on the element; `--cover-opacity` tunes it). The osu! star colour ramp (`starColor`) and the mod colours stay the same in both themes. Keep new UI on the tokens; no literal colours.

### Map rules drawer

The **Map rules** button in the queue header (`#rulesBtn`) opens `#rulesDrawer`, a Bootstrap drawer like the pre-stream check. It lists `RULES`
(the fields that were the Settings tab "Map rules"; `rules.exemptUsers` stays in **Settings → Queue**), drawn by the same `field()` as the Settings form.
`field()` reads and writes the draft of `form` (`{draft, render}`), which `renderSettings` and `renderRules` set before drawing, so the dialog and the drawer
keep their own drafts. **Reset to defaults** resets only the `RULES` paths (exempt users are kept).

**Saving without losing changes made meanwhile.** `PUT /api/config` replaces the whole config, while bans (card ⋯ menu, rejection log, mod panel),
the requests switch (by hand, `!reqon`/`!reqoff`, automatic open/close), the volume, auto download, dry run, `osu.game` (tosu detection) and `connection.tosuPath`
are written by the server at any time. Both forms therefore save with `mergeEdits(base, draft, fresh)`: `base` is a copy of the config taken when the form
opened, and on Save the dashboard GETs `/api/config` again (`fresh`) and changes in it only the fields the streamer changed (draft differs from base).
A field is a key of a section (`rules.minStars`) or a top-level value (`masterVolume`); lists and objects (`rules.blacklistUsers`, `rules.modeStars`,
`tts.templates`, `download.sources`) count as one field; `null`, `[]` and `{}` are equal. The drawer merges only `rules`; the Settings dialog merges every section,
so a rule saved from the drawer, or a ban, made while Settings was open is kept. A list edited in the form replaces the server's list as a whole, and a
change the server makes between that GET and the PUT (milliseconds) is still lost.

A validation error from Save is shown in a toast (above the dialog or drawer, see [Dashboard internals](#dashboard-internals)) and, in the drawer, under its field (`RULE_ERR`: `stars_order` → min and max stars, `stars_negative`,
`stars_order_mode`, `mode_unknown` → stars per mode, `song_rule`, `hard_mod_stars`, `banned_mod`). The server reports only the first problem.

---

## Settings reference (`config.json`)

All settings are editable in the dashboard (gear icon; the `rules` section with the **Map rules** button above the queue). Missing keys take their defaults.

| Section | Key | Default | Meaning |
|---|---|---|---|
| (root) | `language` | `en` | saved by the settings sheet; the dashboard language itself is per browser |
| | `masterVolume` | `10` | Master volume (0-100 %) for every sound the dashboard plays (map previews); also the speaker button in the top bar |
| connection | `source` | `tikfinity` | Where chat comes from: `tikfinity`, or `tiktok` (straight from TikTok, signed by Euler Stream) |
| | `tiktokUser` | empty | `tiktok`: the streamer's @name (saved without `@`, lower case) |
| | `eulerApiKey` | empty | `tiktok`: optional Euler Stream API key; empty = keyless limits. Shown as `(hidden)` in the support file |
| | `tikfinityUrl` | `ws://127.0.0.1:21213/` | TikFinity WebSocket API (must be on this PC) |
| | `dashboardPort` | `21280` | Dashboard/overlay port (applies after restart) |
| | `reconnectSec` | `5` | Retry interval for TikFinity and tosu |
| | `autoLaunchTikfinity` | `false` | Start TikFinity if it is installed but not running; paused after **Close TikFinity** until it is started again |
| | `tikfinityPath` | `%LOCALAPPDATA%\Programs\tikfinity\TikFinity.exe` | Also found through the registry |
| | `openDashboardOnStart` | `true` | |
| | `logRawEvents` | `false` | Show every TikFinity message in the dashboard |
| | `tosuUrl` | `ws://127.0.0.1:24050/websocket/v2` | tosu's v2 WebSocket (must be on this PC) |
| | `tosuPath` | empty | Filled in automatically the first time tosu is seen running |
| | `autoLaunchTosu` | `false` | Start tosu if it is not running (needs `tosuPath`); paused after **Close tosu** until tosu is started again |
| | `checkUpdates` | `true` | Ask GitHub twice a day for a newer release and show a button at the top |
| | `autoUpdate` | `true` | With `checkUpdates`: download a newer release in the background and install it the next time osureq starts (the button installs it right away) |
| osu | `game` | `stable` | `stable` or `lazer` |
| | `detectGameTosu` | `true` | Switch `game` to the client tosu reports |
| | `folder` | `%LOCALAPPDATA%\osu!` | osu! stable folder. If it has no Songs folder, the folder from the `.osz` association is used at startup and saved |
| | `cacheFolder` | `cache` | Relative to the exe (or `-dir`) |
| | `downloadsFolder` | `%USERPROFILE%\Downloads` | Watched by the official source |
| | `useOsuDb` | `false` | Experimental (stable): if `osu!.db` already has the map, skip the download and open `osu://b/{id}` |
| | `viewerUrl` | `https://kiki-bm.app/` | Beatmap viewer (KIKI Beatmap Viewer) for the "viewer ↗" link; empty hides it. The old default `https://osu-beatmap-viewer.vercel.app/` becomes this one on load |
| | `kikiPlayer` | `true` | **▶ view map** on the dashboard's queue, now-playing and history cards: KIKI Beatmap Viewer's player under the card (osu!standard only); configs without the key get `true` |
| | `kikiPlayerMod` | `true` | The same ▶ on the mod panel (always from the community mirrors) |
| command | `name` / `aliases` | `!req` / `!request`, `!r` | |
| | `enabled` | `true` | Requests on/off (also the switch in the top bar) |
| | `bareNumberAs` | `beatmap` | `beatmap` or `set` (the other meaning is tried if not found) |
| | `searchByName` | `true` | Text that is not an ID or link is searched as a song name |
| | `maxPerMessage` | `1` | Maps one chat message may ask for (1-5, `max_per_message`); 1 = as before. See [Several maps in one message](#several-maps-in-one-message) |
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
| | `autoNeedsPass` | `false` | Auto mode opens the next map only after a clear (results screen); a fail or Esc waits for a retry or Next |
| | `fair` | `false` | New requests take turns between viewers |
| | `autoCloseAt` / `autoReopenAt` | `0` / `0` | Close requests when this many are waiting (0 = off); reopen at this many or fewer (must be lower) |
| | `closeAt` | empty | `HH:MM` local time: close requests once when the clock passes it |
| download | `sources` | hinai, nekoha, beatconnect, osu.direct, catboy, sayobot, official | Order, on/off, URL templates (`{id}`), no-video URL, referer, timeout, `official` flag |
| download | `sourcesRev` | 2 | Which default mirror list `sources` was made from; older lists get the new mirrors on load |
| | `noVideo` | `true` | Uses the no-video URL when the set has a video |
| | `cacheLimitMb` / `autoCleanup` | `2048` / `true` | Deletes the oldest unreferenced files when over the limit |
| md5 | `enabled` | `true` | |
| | `level` | `requested` | `requested` or `strict` |
| | `verifyCache` | `true` | Re-hash cached files before use |
| | `onAllFail` | `skip` | `skip` or `warn` |
| | `metaTimeoutSec` | `15` | osu! metadata timeout |
| import | `mode` | `open` | `open`, `copy-to-songs`, `extract-to-songs` |
| | `preload` / `preloadCount` | `true` / `2` | **Auto download**: fetch the next `preloadCount` maps in the background; also the switch above the queue on the dashboard |
| overlay | `visible`, `showRequester`, `theme` (`pink`/`sky`/`mono`), `scale` (%), `position` | `5`, `true`, `pink`, `100`, `top-left` | |
| | `feedback` / `feedbackSec` | `true` / `7` | Pop-ups for viewers and how long they stay |
| | `lang` | `auto` | Overlay language: `auto` (OBS browser language), `en`, `th` |
| | `avatars` | `true` | Viewers' TikTok pictures next to their names (when TikFinity sends them) |
| | `remote` | `false` | Let another PC load the overlay through the mod panel's port with a secret link (needs `remote.enabled`; see [Overlay link](#overlay-link)) |
| | `remoteToken` | empty | The link's secret: 43-128 base64url characters (`overlay_token`), made by the helper (32 random bytes) when `remote` is switched on, replaced by **New link** (`POST /api/overlay/token`). Not editable in the settings form, hidden in `/api/state` and the support file |
| gifts | `enabled` | `false` | Gifts buy queue priority |
| | `coinsPerPriority` | `30` | Coins (added up per viewer) for one priority |
| | `creditMinutes` | `60` | How long an unused priority (or paid request) waits for a request |
| | `requestCoins` | `0` | Gift-only requests: coins per request; 0 = requests are free |
| remote | `enabled` | `false` | Mod panel for a phone on the same network |
| | `port` | `21281` | Its port (not the dashboard port) |
| | `pin` | empty | 6-12 digits, not all the same, not `123456`/`987654`-style; required when enabled (the dashboard generates one) |
| tts | `enabled` | `false` | Read chat aloud in the dashboard, without commands |
| | `lang` | `auto` | `auto`, `th` or `en`. `auto` picks the language of each message by its letters (Thai, Japanese, Korean, Chinese, Hindi, else English); `th` or `en` does no detection. See [Chat TTS](#chat-tts) |
| | `voice` | empty | The older single browser voice name, still used for its own language when `voices` has no entry for it. The Settings form moves it into `voices` on save and clears it |
| | `voices` | `{}` | Browser voice per language: keys `th`, `en`, `ja`, `ko`, `zh`, `hi`, values a voice name (at most 120 letters, no control characters; `tts_voices`). Missing or empty = best for the language |
| | `gender` | `any` | `any`, `female` or `male` (`tts_gender`): preferred when a voice is picked by itself. A voice's gender is known only from the name table in [Chat TTS](#chat-tts) |
| | `engine` | `auto` | Who speaks: `auto` (browser voice, Google's when the browser has none for the language), `browser`, `google` (text goes to Google) |
| | `rate` / `volume` | `1` / `80` | Speed 0.5-2, volume 0-100 |
| | `readName` | `true` | "name says …" |
| | `maxLength` / `cooldownSec` | `120` / `3` | Letters per message (10-500); seconds between a viewer's read messages |
| | `allow` | empty | Whose chat is read: `subscriber` (ใจส้ม), `follower`, `gifter`, `mod`; any one; empty = everyone |
| | `blockedWords` | empty | |
| | `announce` | `true` | Speak what happens to requests (only in a dashboard tab where TTS is on) |
| | `templates` | Thai set | Event → line with placeholders (events, placeholders and limits: [Announcement lines and placeholders](#announcement-lines-and-placeholders); at most 300 letters, `tts_template_long`; unknown event `tts_template`); empty or `-` = silent |
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
* Only numeric IDs and links on osu.ppy.sh are fetched directly; song names go to the mirrors' search APIs only. Downloads go only to whitelisted
  HTTPS hosts, and every redirect hop is checked, so no request is sent elsewhere.
* Viewer pictures are used only when they are `https` URLs on TikTok's CDN hosts, and are escaped in the overlay.
* The update check reads GitHub's release list and links to this repository's releases page. With `autoUpdate` it downloads only this repository's release zip, refuses it unless it matches the release's SHA-256 file, and runs it only by restarting osureq (on the next start or from the dashboard button). The release builds are not signed, so the hash file on the same release is the only check.
* Only `.osz` files that this run of the helper downloaded and verified are opened or imported (`Downloader.Trusted`). After a restart, queued items
  go back through the cache check.
* File names are sanitised; extraction refuses absolute paths, drive letters, `..` and symlinks.
* No osu! OAuth secret, password or API key is used or stored.
* **Mod panel** (`internal/remote`, off by default) is the only listener on other interfaces. It refuses clients that are not on a private
  network (loopback, RFC 1918, link-local, IPv6 ULA, `100.64.0.0/10`) and `Host` headers that are not an IP, `localhost`, `*.local` or the PC's
  name (DNS rebinding). Login needs the PIN (SHA-256 compared in constant time); 5 wrong tries from one address or 20 in total lock it for
  10 minutes. Sessions are random 32-byte tokens in an HttpOnly, SameSite=Strict cookie, valid 12 h and ended by a PIN change; every POST
  also needs `X-Osureq: 1`. It offers only queue actions. It is plain HTTP, so the PIN can be seen by others on the same Wi-Fi: use it at home.
* **Overlay link** (`overlay.remote`, off by default, needs the mod panel): `/o/<token>` on the same listener, read-only. The token is 32 random bytes, compared in constant time; a wrong one is a plain 404 and
  counts toward the per-address lockout of the PIN (5 tries, 10 minutes); the page and feed are `no-store`, `no-referrer`, not frameable. The feed is a whitelist (`OverlayState`) without events, paths, config, TTS,
  tokens or keys, but it shows viewer names and requested songs to anyone who has the link. Like the PIN, it travels over plain HTTP on the home network. Details in [Overlay link](#overlay-link).
* **Map player**: the ▶ player is an iframe of `https://player.kiki-bm.app`. Messages from it are accepted only from that origin and that iframe's window; the cached `.osz` is sent to it only when it asks
  (`need-file`), through `GET /api/osz/{setId}`, which is same-origin only (no CORS), builds its path from a number, and serves only archives this run of the helper verified (`Downloader.CachedFile`).
  The mod panel's CSP allows exactly `frame-src https://player.kiki-bm.app`.
* The support file hides the mod panel PIN, the overlay link's token and the home folder; it still has viewer names and messages, which the README in the zip says.

---

## Development

```powershell
go vet ./...
go test ./...
.\build.ps1 -Version 1.5.1          # vet + test + build → dist\osureq.exe (also rewrites rsrc_windows_amd64.syso)
```

If PowerShell blocks scripts, build directly:

```powershell
$env:GOOS="windows"; $env:GOARCH="amd64"; $env:CGO_ENABLED="0"
go run ./tools/winres -version 1.5.1      # icon, version info, manifest → rsrc_windows_amd64.syso
go build -trimpath -ldflags "-X main.version=1.5.1" -o dist\osureq.exe .
```

### Releases (GitHub Actions)

`.github/workflows/build.yml` runs on every push to `main` and every pull request: `go vet`, `go test` and a build on
`windows-latest`; the exe is kept as a workflow artifact for 14 days (version `0.0.0-dev+<commit>`).
To publish a release, bump the version (Changelog row, Version row, `build.ps1` default) and merge it into `main`, then
either run the workflow by hand (**Actions → build → Run workflow** on `main`, version `1.5.1`) or tag and push:

```powershell
git tag v1.5.1
git push origin v1.5.1
```

Run workflow checks that the version is `x.y.z`, that it runs on `main` and that it matches the `build.ps1` default, then
creates the tag `v1.5.1` on that commit together with the release (a tag made by the workflow does not start another run).
The release text is `.github/release-notes/v<version>.md` when that file exists (write it in the version-bump PR), else a short default.
To fix the text of a release that is already out, change its file, merge it, and run the workflow with **notes** set to the version
(only `gh release edit --notes-file`, nothing is rebuilt). Every build runs `.github/check-release-notes.sh`, which fails on a bare `@word`
in release notes: GitHub reads it as a mention and lists that account under the release's Contributors (1.4.8 listed a user called `name`).
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
3. **Keep the module boundaries.** Only `osumeta` scrapes osu!. Only `tikfinity` knows TikFinity's payload. Only `tiktok` knows TikTok's LIVE protocol and the sign server. Only `tosu` knows tosu's payload. Only `platform` calls Win32.
   Only `downloader` performs downloads, and only to whitelisted hosts.
4. **Do not guess third-party field names silently.** If you change what is read from TikFinity or tosu, verify it against a real message (raw event log, or tosu's `/json/v2`) and say so in the commit.
5. **Auto mode is tosu-only by the owner's decision.** Do not add a timer or window-title fallback without asking.
6. **Dashboard performance:** follow the [rendering rule](#dashboard-internals); never re-render whole sections on every push.
7. **UI text must exist in both English and Thai** (`T` table). Follow the existing design tokens.
8. **Security invariants:** keep the dashboard's 127.0.0.1 bind (the mod panel is the one, opt-in exception, with its own guard; the overlay link rides on it and must keep its 404 behaviour and token checks), the `guard` checks, the host whitelist, `Trusted` before opening files, and `SafeJoin` for extraction.
9. **Windows/encoding pitfalls:** write files as UTF-8 without BOM; a literal U+FEFF inside a `.go` file breaks compilation (use `\ufeff` or bytes).
   Git on Windows converts line endings; `.osz` fixtures are marked binary in `.gitattributes`.
10. **Config changes must stay backward compatible:** new fields need defaults in `Default()`; old `config.json` files must still load.
11. **Commits:** small, descriptive messages explaining *why*. The built exe (`dist/`) is not committed.

---

## Known limitations and open items

| Item | Status / next step |
|---|---|
| Live TikTok chat through TikFinity | field names (`comment`, `uniqueId`, `nickname`) not yet seen on a real stream; check the raw event panel during the first stream |
| TikTok direct chat and gifts | connecting, entering the room, viewer @names and chat verified on a real LIVE; a gift not yet seen. With an API key: not tested (no key) |
| Auto mode with real tosu + osu! | verified only with a fake tosu server; confirm state names during real play (stable and lazer) |
| Next → `.osz` opens in osu! | not exercised on the real machine during development; confirm once |
| Global hotkeys | implemented, not exercised |
| TikFinity gift and role fields | follow TikTok's payload names; check the raw event panel for `gift` and `chat` during a stream |
| Profile pictures | `profilePictureUrl` and the other shapes are unverified; if no pictures appear, check the raw `chat` event for the field name |
| Mod panel | plain HTTP on the LAN; not tested on Windows yet (firewall prompt, phone on the same Wi-Fi) |
| Viewer count | with TikFinity the event names (`roomUser`, `viewerCount`, `viewer`) and keys (`viewerCount`, `viewers`, `total`, `totalUser`) are unverified; check the raw events of a real LIVE. TikTok direct reads `WebcastRoomUserSeqMessage.Total` |
| KIKI map player | depends on KIKI's embed protocol (`ready`, `load`, `need-file`, `file`, `cache-clear`, `pause`, `play`, `error`, envelope `v: 1`); if it changes the player stays empty until `app.js` and `mod.html` follow. osu!standard only, files over 100 MiB are not offered, and after a restart cached files are not trusted until the set is downloaded again (the player then uses the mirrors) |
| Several maps per message | `PUT /api/config` from a Settings dialog opened before updating would send 0 for `command.maxPerMessage` and fail validation (like other range fields): reload the dashboard first |
| Overlay link (second PC) | not tried on a real second PC or in real OBS: only in a browser on the same PC, also through the LAN address. An open connection from an address that is locked later stays connected, and if the mod panel port changes an open connection survives the listener restart. Replaced links (last 4) answer 404 without counting as failures, in memory only: after a restart the old link counts as a wrong guess |
| Chat TTS voices and gender | tried only with fake voice lists in a browser (`speechSynthesis.getVoices` and `speak` replaced), not with real voices or audible playback. Not verified: which voice names Microsoft Edge exposes to web pages, and whether its Natural (online) voices work from a web page at all (an unofficial report says Edge 156 failed with `synthesis-failed`; the older claim that Edge has natural voices, Thai included, is just as unverified); whether Windows' Thai voice Pattara reaches browsers. Chrome-based browsers only see Windows' voices, and macOS voice names and genders are not in the `ttsGenderOf` table (they show `?`). Female and male Thai voices therefore depend on the browser: Edge Natural voices, if they work for web pages, otherwise only what Windows provides. The test box in Settings → TTS shows what the user's own browser really has |
| Chat TTS languages | Indonesian, Malay, Vietnamese, Spanish, French and German use Latin letters like English and are read as English; announcements stay Thai or English; the new languages need a browser voice and are skipped without one (with a notice). Not built: a voice per viewer, and a cloud voice service |
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
| TikTok direct: "TikTok account not found" | Check **Settings → Connection → Your TikTok @name**. |
| TikTok direct: yellow "Connecting…" while live | Hover the pill for the reason. `sign server limit reached`: the keyless hourly/daily limit; it retries after 5 min, or add a free Euler Stream key. `HTTP 403` from TikTok: retried with a back-off. |
| tosu pill red/yellow | Start tosu. Different port → **Settings → Connection → tosu WebSocket URL**. "osu! not detected" means osu! is not running. |
| Auto mode does not move on | Needs a green tosu pill, and the opened map must actually be **played** (song select alone does not count). Press **Next** to move on by hand. |
| My `!req` was not added | Check the **Rejected** panel (a red pop-up also appears). Usually the per-user limit or cooldown: add yourself to **No-limit users**. |
| Chat requests are ignored | **Requests** on? Status green? Message starts with the command? Turn on **Log raw events** and look at the `chat` event. |
| Every download is "MD5 mismatch" | The map was updated recently and mirrors lag behind. Enable the official source, or set **If every source fails** to "open with warning". |
| Official download times out | Be logged in to osu.ppy.sh in the **default** browser, and let it save to the configured Downloads folder without asking. |
| Next does nothing in osu! | Double-click an `.osz` once so Windows opens it with osu!. Make sure dry run is off. |
| Port already in use | Another copy is running, or change `dashboardPort`. |
| Phone cannot open the mod panel | Same Wi-Fi (not a guest network, which isolates devices)? Allowed in the Windows firewall for **private** networks? Windows network profile set to Private? Use an address from Settings → Mod panel. |
| Overlay link answers 404 | **Settings → Overlay → Let another PC load the overlay** must be on **and saved**, and the mod panel must be on. The link must be the current one (**New link** replaces it at once). Five wrong links or PINs from one PC lock it for 10 minutes, and a locked PC gets 404 even with the right link; `osureq.log` has `overlay link: wrong link from <ip>` lines. A PC outside the private networks gets 403 |
| Overlay on the other PC is empty or never connects | Same network (not a guest Wi-Fi)? osureq allowed in the Windows firewall for **private** networks? Use an address from Settings → Overlay; the pre-stream check says whether an overlay is connected. TTS still plays on the PC where the dashboard is open |
| ▶ player says "Try again" or stays empty | Press ▶ in the player again (osureq then lets it use the mirrors). Still failing: the community mirrors are down, the set is over 100 MiB, or the browser or network blocks `https://player.kiki-bm.app` (offline, a content blocker, https/CORS). It never needs osureq to restart |
| No ▶ on a card | The map is not osu!standard (taiko, catch, mania and converts have none), it is a history entry from before the mode was recorded, or the setting is off (**Settings → osu! → KIKI map viewer**, one switch for the dashboard and one for the mod panel) |
| No 👁 viewer count | It is hidden while unknown. TikTok direct shows it after TikTok's first count; with TikFinity only if it sends the count (see Known limitations) |
| Several maps typed, only one was requested | `command.maxPerMessage` is 1, or the message holds other words than IDs, links and mods (a song name, chat): it is then read as one request |
| Windows says the app is unsafe / "Windows protected your PC" | That is SmartScreen warning about an **unsigned** program, usually one that was downloaded or received in a zip. Windows Defender finds nothing in it. Click **More info → Run anyway**, or right-click `osureq.exe` → **Properties** → tick **Unblock**. Only a code-signing certificate removes the warning for everyone. |
| TikFinity's TTS reads `!req 123…` | Turn TikFinity's TTS off and use **Settings → TTS**, which never reads commands. |
| Chat TTS is read twice | Another program reads chat too (turn TikFinity's TTS off). Two osureq dashboards no longer both speak: only the last one started reads, the other says **TTS in another window**. |
| Chat TTS is silent | If the top bar says **TTS in another window**, another dashboard window or tab reads it: close that one or press the button here. If it says **Click anywhere to start TTS**, click the page once (the browser wants a click before sound), or allow Media autoplay for the dashboard in Edge. If it says **Start TTS**, it was switched off: press it. Thai is silent with **Settings → TTS → Who speaks** on `browser` in Chrome or Dia (no Thai voice there): use `auto` or `Google`, or open the dashboard in Edge. |
| Chat in Japanese, Korean, Chinese or Hindi is not read | These languages are read only by a browser voice and never by Google. One message per language says so. Install a voice for it in Windows (Settings → Time & language → Speech) or open the dashboard in Microsoft Edge; the lines are skipped until then. **Settings → TTS** lists a voice dropdown only for languages that have a voice |
| Every voice shows `?` instead of ♀ or ♂ | The voice's first name is not in osureq's list (`ttsGenderOf`: Microsoft's voice names; macOS and other voices are not listed). The gender is never guessed: pick the voice by hand in its language's dropdown |
| The female/male choice is read by another voice | No voice of that gender is known for the language in this browser. The notice (once per language and gender) names the voice that reads; install such a voice, try Edge, or choose a voice by hand. Google's voice is a single voice |
| An error toast did not show while Settings was open | Fixed in 1.5.1: toasts were hidden behind Settings, the pre-stream drawer and the Map rules drawer, and now sit above them |
| Anything else | `osureq.log` next to the exe. |

---

## Changelog

| Version | Commit | Changes |
|---|---|---|
| unreleased | branch `docs/announcement-placeholders` | Documentation only, no code change: a reference for the placeholders of spoken announcement lines (`{title}`, `{user}`, …). The README (English and Thai) gets *What you can put in an announcement line* (placeholder table, which event has which placeholders, tips, example lines rendered by the real code); this guide gets *Announcement lines and placeholders* (flow from `emit` to `queueSpeech`, where each value is set, rejection `Vars`, the `reasons` table, limits, and how to add a placeholder or an event) |
| 1.5.1 | PR #20 | **Toasts above dialogs** (fix): messages raised while Settings, the pre-stream drawer or the Map rules drawer was open were hidden behind it; `#toasts` is now a manual popover in the top layer that moves into the newest open modal and back to the body (`placeToasts`, `modalChange`, a `MutationObserver` on dialogs' `open`, `hide.bs.dialog` / `hide.bs.drawer`); needs the Popover API and Bootstrap firing the hide events first. **Chat TTS: voice gender and more languages** (free, browser voices only): new `tts.gender` (`any`, `female`, `male`; `tts_gender`) and `tts.voices` (voice per language `th en ja ko zh hi`; `tts_voices`), the old `tts.voice` is the fallback for its language and the Settings form migrates it; with `tts.lang` `auto` chat is told apart as Thai, Japanese (kana), Korean, Chinese, Hindi or English (Latin-script languages are read as English); Japanese, Korean, Chinese and Hindi use browser voices only and are skipped, with one notice per language, when there is none; voice gender from a table of Microsoft voice first names (`ttsGenderOf`, `?` when unknown); Settings shows ♀/♂/? and the voice test says which voice read; `SpeakableText` no longer cuts between a letter and its combining marks. New keys only, so 1.5.0 still starts with the config (going back loses the chosen voice). Tests `TestSpeakableTextScripts`, `TestTTSVoicesAndGender`, `TestPutConfigTTSVoices`; browser side tried with fake voice lists only |
| 1.5.0 | PR #19 | **Viewer count**: 👁 in the status pill and the mod panel (`tikfinity.Status.Viewers`, `DirectEvents.Viewers`, `Client.OnViewers`; TikTok direct reads `WebcastRoomUserSeqMessage.Total`; TikFinity's `roomUser` / `viewerCount` / `viewer` event is unverified on a real stream). **KIKI map player** (KIKI Beatmap Viewer, player.kiki-bm.app): ▶ under queue, now-playing and history cards and on the mod panel, an iframe plus osureq's own postMessage code (no script is loaded from KIKI); the dashboard offers the cached .osz through `GET /api/osz/{setId}` (`Downloader.CachedFile`, checked files only), the mod panel and uncached maps use the mirrors; `osu.kikiPlayer`, `osu.kikiPlayerMod`. **Several maps per message** (`command.maxPerMessage`, 1-5, default 1; `parser.ParseAll`, `App.submitAll`: the cooldown is checked once per message, one paid request per map). **Overlay for a second PC**: `overlay.remote`, `overlay.remoteToken`; `/o/{token}` and `/o/{token}/ws` on the mod panel's server send a read-only `OverlayState`; `POST /api/overlay/token` makes a new link. **Map rules** moved from Settings to a drawer on the dashboard (button above the queue); saving merges with a fresh config (`mergeEdits`) so bans made meanwhile survive, and the Settings save merges the same way. The default `osu.viewerUrl` is now `https://kiki-bm.app/` (a stored old default is migrated on load) |
| 1.4.9 | PR #18 | **Auto download switch** above the queue on the dashboard (`import.preload`; `POST /api/action/toggleAutoDownload`; `State.autoDownload`, `autoDownloadN`); the setting is renamed Auto download. **New dashboard on Bootstrap 6** (6.0.0-alpha.1, vendored): sticky top bar (not sticky on phones) with the requests switch, volume, language, settings and a ⋮ menu (pre-stream check, support file, close tosu/TikFinity, quit); light and dark by the browser; now playing and queue rows with the cover as a soft banner inside the box (drawn with a CSS mask, so it fades into the box colour in light and dark; strongest on the right, fading out towards the text and at the edges), play and remove buttons and a ⋮ menu (move, skip, preview, open elsewhere, bans); rejected / activity / history / LIVE events as tabs; settings as a dialog of one fixed height on every tab, with a side menu that stays put while only the pane scrolls (a full-screen page on phones, with the tabs in one scrolling row); the pre-stream check in a drawer; Bootstrap toasts. The server sets the content type of `.js`, `.css` and `.svg` itself. Needs Chrome/Edge 130, Firefox 132 or Safari 18 or newer. Bootstrap, Floating UI and Vanilla Calendar Pro licences added |
| 1.4.8 | PR #13 | **TikTok direct**: chat and gifts straight from TikTok without TikFinity (`connection.source` `tiktok`, `tiktokUser`, optional `eulerApiKey`; new package `internal/tiktok`, signed by Euler Stream, keyless or with a key; switch back to TikFinity any time). Status pill, pre-stream check, mod panel and support file (key hidden) know the source. Only one dashboard page reads TTS (`POST /api/tts/speaker`, `State.ttsSpeaker`): the last one started, the others say **TTS in another window**. `osu.viewerUrl` can be a template with `{set}`, `{map}`, `{file}` |
| 1.4.7 | PR #11 | TTS starts by itself when the dashboard loads (`ttsAutoStart`); when the browser wants an interaction first, the first click or key anywhere starts it. Switching it off is remembered per browser (`osureq.ttsOff`). New `queue.autoNeedsPass` ("Auto: only after a clear", off by default): a fail or quit waits for a retry or Next; the tracker now goes from `ended` back to `playing` when the same map is played again. README credits chaiwat2183 for the idea |
| 1.4.6 | PR #10 | Auto mode no longer stalls: a failed first request is passed over (as Next does), and the tracker also matches by artist and title when tosu reports another set ID (osu! says -1 for maps without one in the .osu). **Previous** on the dashboard and the mod panel (`App.Previous`, `Queue.Back`, the last 10 now-playing maps in `recent`): osu! opens the map played before again and the current one returns to the front of the queue. History rows have **Open in osu!** (`POST /api/jump`, `osu://b/<id>`; history now records `beatmapId`). Mirrors are probed once after start instead of every 30 min, show OK, and a mirror refusing a download with HTTP 403 is reported in Activity (once per mirror until it works again) |
| 1.4.5 | PR #9 | `!np` answers with the map osu! has open according to tosu (`tosu.Game` now carries title, artist, mapper, difficulty and stars; `App.tosuMap`), naming the requester only when that map is the queue's now-playing request; without tosu it still answers from the queue. The overlay leaves out "requested by" when nobody asked for the map |
| 1.4.4 | PR #8 | Card menu (⋯) moves to `<body>` while open so the cards below no longer cover it. One-click ♪ preview button on every card; the mod panel's ♪ plays or stops the preview on the stream PC (`App.AskPreview` → `State.previewAsk`, the dashboard reports back with `PUT /api/preview`). Mirrors box on the home page: each download mirror's state, speed and last error (`downloader.Statuses`), checked 15 s after start, every 30 min, after each download and with **Check**. TTS reads all-caps titles, artists, mappers and difficulties as words (`SWITCHED-ON` was spelled out) |
| 1.4.3 | PR #7 | More mirrors in the order kiki-bm.app uses: downloads try hinai → Nekoha → beatconnect → osu.direct → catboy → Sayobot → official (`config.DefaultSources`; saved configs get them once via `download.sourcesRev`), song search tries hinai → Nekoha → osu.direct → catboy (`search.DefaultMirrors`; a failing, rate-limited or empty mirror hands over). A refused download shows the mirror's own message, HTTP 429 rests a mirror for its `Retry-After`, hinai's `404 content_removed` stops the request. Host whitelist allows ports and `.sayobot.cn` subdomains |
| 1.4.2 | PR #6 | Updates install themselves: with `connection.autoUpdate` (default on, needs `checkUpdates`) a newer release's zip is downloaded from this project's GitHub releases, checked against its `.sha256` and installed the next time osureq starts, or right away with the dashboard's update button (`installUpdate`, restarts osureq); the dashboard and overlay reload when the version changes. TTS no longer spells capital words letter by letter (`SoftenCaps`: capital words of 3+ letters are lowercased in chat, names and announcement values; Google's Thai voice spelled "KIKI" as K, I, K, I) |
| 1.4.1 | PR #5 | Chat TTS reads Thai: `tts.engine` (auto / browser / Google), auto uses Google Translate's voice through `POST /api/tts` when the browser has no voice for the language (Windows ships no Thai voice); a chosen browser voice is only used for its own language; Test panel plain chat is read aloud (`spoken`) instead of turned into `!req`; spoken request announcements on by default; Settings → TTS has a box to test the voice with your own line |
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
the hinai, Nekoha, beatconnect, osu.direct, catboy.best and Sayobot mirrors, the owner's osu! beatmap viewer for the visual design, and [KIKI Beatmap Viewer](https://api.kiki-bm.app/) for the map player.
