# osureq: TikTok LIVE → osu! beatmap requests

A small Windows helper (one `osureq.exe`, ~8 MB, no installer, no runtime) that lets
TikTok LIVE viewers request osu! beatmaps in chat. The helper:

1. reads chat from **TikFinity Desktop**,
2. checks each request against your rules,
3. downloads it from a mirror,
4. confirms the files match what osu! currently serves (MD5),
5. opens the map in osu! when you press **Next**.

```
TikTok LIVE → TikFinity Desktop → ws://127.0.0.1:21213 → osureq → osu!
                                                           ↓
                                    http://127.0.0.1:21280 → Dashboard / OBS overlay
```

TikFinity is the only event source. The helper never connects to TikTok itself.

---

## Requirements

| | |
|---|---|
| OS | Windows 10/11 x64, internet connection |
| osu! | osu! **stable** or **lazer** installed, with `.osz` files opening in osu! (double-click a `.osz` once to check) |
| **TikFinity Desktop** (required) | Installed, logged in, connected to your TikTok account, and **running during the stream**. Download: <https://tikfinity.zerody.one/> |
| Optional | OBS or TikTok LIVE Studio, for the overlay |
| Optional | Logged in to osu.ppy.sh in your default browser. Only needed for the official-download fallback |

## Setup

1. Put `osureq.exe` in its own folder, for example `C:\osureq\`. It creates `config.json`, `queue.json`, `osureq.log` and a `cache\` folder next to itself.
2. Start TikFinity Desktop and connect it to your TikTok account.
3. Double-click `osureq.exe`. The dashboard opens at <http://127.0.0.1:21280/>.
4. In **Settings → osu!**, choose stable or lazer and check the osu! folder (stable default: `%LOCALAPPDATA%\osu!`).
5. Check the status pill at the top:
   * 🔴 **red**: TikFinity is not installed or not running. Use the button in the banner.
   * 🟡 **yellow**: TikFinity is running but not connected yet, or connected and no TikTok events have arrived yet ("Waiting for stream").
   * 🟢 **green**: live and taking requests.
6. Type a beatmap link in **Test request** to check the whole chain (download, MD5, preload) without going live.
7. Optional: in OBS, add a **Browser Source** with the URL `http://127.0.0.1:21280/overlay`. The background is transparent.

The dashboard works in English and Thai (language picker at the top).

## What viewers type

```
!req 2096297                                              ← beatmap (difficulty) ID by default; if no such beatmap exists it is tried as a set ID
!req https://osu.ppy.sh/beatmapsets/962870#mania/2096297
!req https://osu.ppy.sh/beatmapsets/962870                ← whole set: picks the hardest diff that fits your rules
!req https://osu.ppy.sh/b/2096297
!req https://osu.ppy.sh/beatmaps/2096297
!req https://osu.ppy.sh/s/962870
```

Anything else is ignored. Only numeric IDs on `osu.ppy.sh` are accepted. The command and its aliases can be changed in Settings.

## How a request is handled

1. **Parse**: the chat message must start with the command, followed by an ID or link.
2. **User checks** (no network needed): blacklisted user, queue full, per-user limit, cooldown.
3. **Metadata**: the helper fetches `https://osu.ppy.sh/beatmapsets/{setId}` and reads the beatmapset JSON embedded in the page: title, artist, mapper, status, and each difficulty's mode, stars, length and `checksum` (the MD5 of its `.osu` file). Beatmap IDs are resolved to their set the same way. If that page cannot be read, it falls back to `https://osu.ppy.sh/osu/{beatmapId}` and computes the MD5 itself. In that case stars, length and status are unknown and are not checked, and the request shows a warning. No API key is used. These are unofficial endpoints, so all of this logic lives in `internal/osumeta`.
4. **Map rules**: star range, max length, modes, statuses, set/mapper blacklist, already in the queue, played within the no-repeat window. Rejections appear in the dashboard's **Rejected** panel with the reason.
5. **Queue**: accepted requests are saved to `queue.json` and survive restarts.
6. **Download** (sources in order; each can be turned off or reordered):
   1. `https://catboy.best/d/{setId}` (sent with `Referer: https://catboy.best/`)
   2. `https://osu.direct/api/d/{setId}`
   3. **official**: opens `https://osu.ppy.sh/beatmapsets/{setId}/download` in your default browser (where you are logged in) and waits for `{setId} *.osz` to appear in your Downloads folder. Your osu! password is never asked for or stored.

   The cache is checked before any download. Each download has its own timeout and must be a real ZIP (an HTML/JSON error page counts as a failure).
7. **MD5 check**: the archive is opened in memory and every `.osu` is hashed and compared with the reference:
   * `requested`: only the requested difficulty must match (the whole set if it was requested by set ID).
   * `strict`: every difficulty on osu! must match, and the archive may not contain extra `.osu` files.

   A mismatch (for example, an outdated mirror copy) discards that file and tries the next source. If every source fails, the request is either skipped or the latest download is used with a warning (setting). With MD5 checking off, the first valid ZIP is accepted and the official source is never used.
8. **Import**, when you press **Next** (or automatically in auto mode):
   * `open` (stable + lazer): the `.osz` is opened with Windows, and osu! imports it and jumps to the map.
   * `copy-to-songs` (stable): copied into `Songs\`, then picked up on song select or F5.
   * `extract-to-songs` (stable): extracted to `Songs\{setId} {Artist} - {Title}\`.

   lazer has no Songs folder, so it always uses `open`.

   **Preload** downloads and verifies the next N maps in the background. On stable it also copies them into `Songs\`, so pressing **Next** is instant.

## Settings reference (`config.json`, editable in the dashboard)

| Section | Key | Default | Meaning |
|---|---|---|---|
| connection | `tikfinityUrl` | `ws://127.0.0.1:21213/` | TikFinity WebSocket API (must be on this PC) |
| | `dashboardPort` | `21280` | Dashboard/overlay port (applies after restart) |
| | `reconnectSec` | `5` | How often to retry TikFinity |
| | `autoLaunchTikfinity` | `false` | Start TikFinity if it is installed but not running |
| | `tikfinityPath` | `%LOCALAPPDATA%\Programs\tikfinity\TikFinity.exe` | Also found through the registry |
| | `openDashboardOnStart` | `true` | |
| | `logRawEvents` | `false` | Show every TikFinity message in the dashboard |
| osu | `game` | `stable` | `stable` or `lazer` |
| | `folder` | `%LOCALAPPDATA%\osu!` | osu! stable folder. If it has no Songs folder, the helper uses the folder from the `.osz` file association at startup and saves it. A `BeatmapDirectory` in `osu!.<user>.cfg` is respected |
| | `cacheFolder` | `cache` | Relative to the exe |
| | `downloadsFolder` | `%USERPROFILE%\Downloads` | Watched by the official source |
| | `useOsuDb` | `false` | stable: if `osu!.db` already has the map, skip the download and open `osu://b/{id}` instead. Experimental: turn it on only if your client jumps to the map from an `osu://b/` link |
| command | `name` / `aliases` | `!req` / `!request`, `!r` | |
| | `enabled` | `true` | Requests on/off (also a switch in the top bar) |
| | `bareNumberAs` | `beatmap` | `beatmap` or `set` |
| rules | `minStars` / `maxStars` | `0` / `0` | 0 = no limit |
| | `maxLengthSec` | `0` | 0 = no limit |
| | `modes` | all four | `osu`, `taiko`, `fruits`, `mania` |
| | `statuses` | ranked, approved, qualified, loved | plus `pending`, `wip`, `graveyard` |
| | `blacklistSets` / `blacklistMappers` / `blacklistUsers` | empty | |
| | `exemptUsers` | empty | TikTok users (you, your mods) who skip the per-user limit and cooldown |
| queue | `maxSize` | `30` | 0 = no limit |
| | `perUserLimit` | `2` | Maps waiting in the queue per user (the one playing does not count) |
| | `cooldownSec` | `60` | Per user |
| | `noRepeatMin` | `60` | Block maps played within this many minutes |
| | `playMode` | `manual` | `auto` opens the next ready map once the current one has run its length plus `autoGapSec` |
| download | `sources` | catboy, osu.direct, official | Order, on/off, URL templates (`{id}`), no-video URL, timeout |
| | `noVideo` | `true` | Uses the no-video URL when the set has a video |
| | `cacheLimitMb` / `autoCleanup` | `2048` / `true` | Deletes the oldest unused files when over the limit |
| md5 | `enabled` | `true` | |
| | `level` | `requested` | `requested` or `strict` |
| | `verifyCache` | `true` | Re-hash cached files before use |
| | `onAllFail` | `skip` | `skip` or `warn` |
| | `metaTimeoutSec` | `15` | osu! metadata timeout |
| import | `mode` | `open` | `open`, `copy-to-songs`, `extract-to-songs` |
| | `preload` / `preloadCount` | `true` / `2` | |
| overlay | `visible`, `showRequester`, `theme` (`pink`/`sky`/`mono`), `scale` (%), `position` | | |
| hotkeys | `enabled` | `false` | Global hotkeys (they work while osu! has focus) |
| | `next` / `skip` / `removeLast` / `toggleRequests` | `Ctrl+Alt+N` / `Ctrl+Alt+S` / `Ctrl+Alt+Backspace` / `Ctrl+Alt+R` | Format: `Ctrl+Alt+N`, `Shift+F5`, `F9`, `Win+Num5`… |

Command-line flags: `-dir <folder>` keeps config, queue, cache and log somewhere else; `-no-browser` skips opening the dashboard.

## Security

* The dashboard and overlay listen on `127.0.0.1` only. Requests with any other `Host` header are refused (protection against DNS rebinding). State-changing API calls need an `X-Osureq` header, which other websites cannot send. The WebSocket only accepts same-origin connections.
* Only numeric IDs are accepted. Downloads go only to `catboy.best`, `osu.direct` and `osu.ppy.sh` over HTTPS, and redirects to other hosts are refused.
* Only `.osz` files that the helper downloaded and verified **in this session** are opened or imported. After a restart, queued items go back through the cache check.
* File names are sanitised. Extraction refuses absolute paths, drive letters, `..` and symlinks.
* No osu! OAuth secret, password or API key is used or stored.

## Troubleshooting

| Problem | Fix |
|---|---|
| "TikFinity not installed" while it is installed | Set **Settings → Connection → TikFinity.exe path**. |
| Yellow "Connecting…" | TikFinity's WebSocket API is off or uses another port. Check the URL in TikFinity and in Settings. |
| Yellow "Waiting for stream" while live | No TikTok events are reaching TikFinity. Reconnect TikFinity to your account. |
| My `!req` was not added | Check the **Rejected** panel (a red pop-up also appears). Usually it is the per-user limit or cooldown: add yourself to **Settings → Queue → No-limit users**. |
| Chat requests are ignored | Check that **Requests** is on, the status is green, and the message starts with the command. Turn on **Log raw events** and look at the `chat` event in the side panel. If TikFinity renamed its fields, tell the developer what you see there. |
| Every download is "MD5 mismatch" | The map was updated recently and the mirrors are not in sync yet. Enable the official source, or set **If every source fails** to "open with warning". |
| Official download times out | Make sure you are logged in to osu.ppy.sh in your **default** browser and that the browser saves to the configured Downloads folder without asking. |
| Next does nothing in osu! | Double-click an `.osz` once to make sure Windows opens it with osu!. |
| Port already in use | Another copy is running, or change `dashboardPort` in `config.json`. |
| Anything else | Check `osureq.log` next to the exe. |

## Building from source

Requires Go 1.23+ (`winget install GoLang.Go`).

```powershell
go test ./...
.\build.ps1                     # vet + test + build → dist\osureq.exe
```

If PowerShell blocks scripts, run the same build command directly:

```powershell
$env:GOOS="windows"; $env:GOARCH="amd64"; $env:CGO_ENABLED="0"
go build -trimpath -ldflags "-s -w -X main.version=1.0.0" -o dist\osureq.exe .
```

Regenerate the `.osz` test fixtures with `go test ./internal/verifier -update`.

### Modules

| Package | Responsibility |
|---|---|
| `internal/tikfinity` | TikFinity install/process/WebSocket checks, status levels, chat events |
| `internal/parser` | `!req` command and link parsing |
| `internal/osumeta` | Beatmapset metadata and reference MD5 from osu.ppy.sh (the only module that scrapes osu!) |
| `internal/validator` | User and map rules → accept/reject with a reason code |
| `internal/queue` | Queue, now playing, history, rejection log; saved to `queue.json` |
| `internal/downloader` | Source order, timeouts, ZIP check, official browser fallback, cache and cleanup |
| `internal/verifier` | In-memory unzip + MD5 comparison (`strict` / `requested`) |
| `internal/importer` | open / copy-to-songs / extract-to-songs, path-traversal-safe extraction, `osu!.db` hash scan |
| `internal/server` | Dashboard, overlay, JSON API and WebSocket push (web files are embedded in the exe) |
| `internal/config` | `config.json` load/validate/save |
| `internal/platform` | Windows APIs: ShellExecute, process list, registry lookup, global hotkeys |
| `internal/app` | Wires everything together: request pipeline, preload, auto play |

The dashboard reuses the look of the osu! beatmap viewer project (palette, Varela Round + IBM Plex Sans Thai, pill buttons, cover cards, star colours).
