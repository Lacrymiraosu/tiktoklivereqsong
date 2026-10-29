# osureq: osu! song requests from TikTok LIVE chat

**English** · [Thai](docs/README.th.md)

A Windows helper for osu! streamers that lets TikTok LIVE viewers request maps in chat with `!req <ID or link>`.
osureq checks the rules, downloads the map, verifies the file and opens the maps in osu! one at a time. It comes with a
dashboard for managing the queue (English/Thai) and an overlay for OBS. Everything lives in a single `osureq.exe`, with no installer.

[![build](https://github.com/Lacrymiraosu/tiktoklivereqsong/actions/workflows/build.yml/badge.svg)](https://github.com/Lacrymiraosu/tiktoklivereqsong/actions/workflows/build.yml)

## Features

* Takes requests from TikTok chat through TikFinity, as IDs or any common osu! link
* Rules: star range (per mode if you like), length, mode, ranked status, requests per viewer, cooldown, duplicates, bans, followers only
* Downloads from mirrors (catboy.best → osu.direct → the official site) and checks the MD5 against the current version on osu!
* Opens the map in osu! (stable and lazer) when you press **Next**, or automatically once the previous song has ended (seen through tosu)
* All four modes: osu!, taiko, catch and mania (converts use osu!'s star rating for that mode)
* The overlay tells viewers whether a request was added or why it was rejected, and shows now playing, the upcoming queue and play results
* Mod commands (`!skip`, `!remove`, `!reqon`, `!reqoff`), queue priority for gifts, one-click bans, audio previews
* Test panel: simulated chat and gifts, and a dry run that never touches the game
* Close tosu, TikFinity or everything from the dashboard

## Requirements

* Windows 10/11, 64-bit
* osu! **stable** or **lazer**
* [**TikFinity Desktop**](https://tikfinity.zerody.one/), connected to the TikTok account and running while you are live (osureq reads chat through it)
* [**tosu**](https://github.com/tosuapp/tosu/releases/latest), running while you are live (tells when a song has ended and records results)
* OBS or TikTok LIVE Studio, for the overlay

## Getting started

1. Download `osureq-<version>-windows-x64.zip` from [Releases](https://github.com/Lacrymiraosu/tiktoklivereqsong/releases/latest)
   and unzip it into a folder of its own, e.g. `C:\osureq\` (it creates `config.json`, `queue.json`, `osureq.log` and a `cache\` folder next to itself).
2. Start TikFinity, tosu and osu!.
3. Double-click `osureq.exe`. A CMD window shows the log (closing it stops osureq) and the dashboard opens at <http://127.0.0.1:21280/>.
4. In **Settings → osu!**, choose stable or lazer, then set the rules you want.
5. For the overlay, add a **Browser Source** in OBS with `http://127.0.0.1:21280/overlay` (transparent background).

Use the **Test** panel on the dashboard to try the whole chain before you go live.

### What viewers type

```
!req 2096297                                      ← difficulty ID (tried as a set ID if no such difficulty exists)
!req https://osu.ppy.sh/beatmapsets/962870#mania/2096297
!req https://osu.ppy.sh/beatmapsets/962870        ← whole set: the hardest difficulty that passes the rules
!remove                                           ← removes your own latest request
```

Mods can also use `!skip` (skip the song being played), `!remove @user` and `!reqon` / `!reqoff` (open/close requests).
Every command name can be changed in the settings.

## How it works

```
TikTok chat ──► TikFinity ──WebSocket──► osureq ──► osu.ppy.sh (map info, MD5)
                                           │
                                           ├──► catboy.best / osu.direct (download .osz) ──► cache\
                                           ├──► osu! (opens the .osz or copies it into Songs)
           tosu ──WebSocket (game state)──►│
                                           └──► dashboard + overlay (http://127.0.0.1:21280)
```

1. **Chat in**: TikFinity sends chat messages and gifts over WebSocket (`ws://127.0.0.1:21213/`).
   osureq only looks at messages that start with a command such as `!req`.
2. **Requester checks**: ban list, full queue, requests per viewer and cooldown, all without network access.
3. **Map info**: the map's page on osu.ppy.sh is read (no API key needed) for title, stars, length, mode, status and the MD5 of every difficulty.
4. **Map rules**: stars, length, mode, status, duplicates and recently played maps.
   A rejected request shows its reason on the overlay, because TikFinity cannot write back into TikTok chat.
5. **Queue**: saved in `queue.json`, so it survives a restart. Viewers whose gifts add up to enough coins get priority (when turned on).
6. **Download and verify**: the `.osz` is downloaded from the mirrors in the configured order, one map at a time, and the MD5 of every
   `.osu` inside is compared with osu!. An outdated mirror copy makes it try the next source. The next maps are preloaded so they open instantly.
7. **Open**: press **Next** on the dashboard (or the hotkey) and osureq opens the `.osz`; osu! imports it and jumps to the map.
8. **Auto mode**: osureq follows the game state from tosu. Once the opened map has really been played and the player leaves the results
   screen, the next map opens after a few seconds. The play result (accuracy, rank, combo, misses, pp, mods) is saved and shown on the overlay.
   If tosu disconnects, auto mode waits; there is no timer guesswork.

The osureq server only listens on `127.0.0.1`, so other devices on the network cannot reach it, and it only downloads from whitelisted hosts.

The details (code layout, API, every setting, testing) are in [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md).

## Building

You need [Go](https://go.dev/dl/) 1.23 or newer on Windows (`winget install GoLang.Go`).

```powershell
git clone https://github.com/Lacrymiraosu/tiktoklivereqsong.git
cd tiktoklivereqsong
.\build.ps1 -Version 1.3.7
```

`build.ps1` runs `go vet` and `go test`, generates the Windows resources (icon, name, version) and builds `dist\osureq.exe`.
If PowerShell refuses to run scripts, build by hand:

```powershell
$env:CGO_ENABLED = "0"
go run ./tools/winres -version 1.3.7
go build -trimpath -ldflags "-X main.version=1.3.7" -o dist\osureq.exe .
```

### Automatic builds and releases (GitHub Actions)

* Every push to `main` and every pull request is vetted, tested and built; the exe stays on the Actions page for 14 days.
* To release a new version, update the version number in `build.ps1`, then push a tag:

  ```powershell
  git tag v1.3.7
  git push origin v1.3.7
  ```

  GitHub builds with the tag's version and publishes a release with `osureq-<version>-windows-x64.zip` (the exe, `LICENSE` and `THIRD_PARTY_LICENSES.txt`) and its `.sha256`.

## Credits

Beatmap data, covers and audio previews come from osu! (ppy Pty Ltd). Beatmaps belong to their mappers, music to its artists.

* [osu!](https://osu.ppy.sh) by ppy Pty Ltd: the game
* [TikFinity](https://tikfinity.zerody.one/) by zerody: chat and gifts from TikTok LIVE
* [tosu](https://github.com/tosuapp/tosu): live osu! game state
* Beatmap mirrors: [catboy.best](https://catboy.best) and [osu.direct](https://osu.direct), where the `.osz` files are downloaded from
* Go libraries: [coder/websocket](https://github.com/coder/websocket), [golang.org/x/sys](https://pkg.go.dev/golang.org/x/sys),
  [tc-hib/winres](https://github.com/tc-hib/winres), [nfnt/resize](https://github.com/nfnt/resize), [golang.org/x/image](https://pkg.go.dev/golang.org/x/image)
* Fonts: [Varela Round](https://fonts.google.com/specimen/Varela+Round) and [IBM Plex Sans Thai](https://fonts.google.com/specimen/IBM+Plex+Sans+Thai) from Google Fonts

osureq is a fan project and is not affiliated with or endorsed by ppy Pty Ltd, TikTok, TikFinity or tosu.
osu! is a trademark of ppy Pty Ltd. TikTok is a trademark of its respective owner.

## License

[MIT](LICENSE) © 2026 kikinomae. The licenses of the third-party code built into `osureq.exe` (the Go standard library,
`golang.org/x/sys` and `coder/websocket`) are in [THIRD_PARTY_LICENSES.txt](THIRD_PARTY_LICENSES.txt).
