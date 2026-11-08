# osureq: osu! song requests from TikTok LIVE chat

**English** · [Thai](docs/README.th.md)

A Windows helper for osu! streamers that lets TikTok LIVE viewers request maps in chat with `!req <ID or link>`.
osureq checks the rules, downloads the map, verifies the file and opens the maps in osu! one at a time. It comes with a
dashboard for managing the queue (English/Thai) and an overlay for OBS. Everything lives in a single `osureq.exe`, with no installer.

[![build](https://github.com/Lacrymiraosu/tiktoklivereqsong/actions/workflows/build.yml/badge.svg)](https://github.com/Lacrymiraosu/tiktoklivereqsong/actions/workflows/build.yml)

## Overview

**Dashboard**: the map being played, the queue with one-click controls, rejected requests, history and a summary of the stream.

![osureq dashboard: now playing, the request queue, rejected requests and the stream summary](docs/images/dashboard-en.jpg)

| OBS overlay | Mod panel on a phone |
|---|---|
| <img src="docs/images/overlay-en.png" alt="OBS overlay: now playing, upcoming requests and a viewer's !pos answer" width="420"> | <img src="docs/images/mod-panel-en.png" alt="Mod panel on a phone: requests switch, now playing with Skip and Next, and the queue" width="260"> |
| What viewers see on stream: now playing, the next requests, how to request, and answers to `!pos`, `!np` and rejected requests. | Mods run the queue from a phone on the same Wi-Fi, with a PIN. |

<sub>Screenshots use example data.</sub>

## Features

* Takes requests from TikTok chat through TikFinity, or **straight from TikTok** without any other program (see [Chat without TikFinity](#chat-without-tikfinity)), as IDs, any common osu! link or a **song name** (`!req freedom dive`), optionally **with mods** (`!req 2096297 HDHR`)
* **Several maps in one message** when you allow it (`!req 2096297 1859353 HDHR`, up to 5; see [Several maps in one message](#several-maps-in-one-message))
* Rules: star range (per mode if you like), length, mode, ranked status, requests per viewer, cooldown, duplicates, bans, followers only
* Downloads from mirrors (hinai → Nekoha → beatconnect → osu.direct → catboy.best → Sayobot → the official site; a mirror that fails hands over to the next) and checks the MD5 against the current version on osu!
* Opens the map in osu! (stable and lazer) when you press **Next**, or automatically once the previous song has ended (seen through tosu)
* All four modes: osu!, taiko, catch and mania (converts use osu!'s star rating for that mode)
* The overlay tells viewers whether a request was added or why it was rejected, and shows now playing, the upcoming queue and play results
* Viewer commands (`!pos` queue position, `!np` now playing, `!voteskip`) answered on the overlay, with viewers' TikTok pictures
* Mod commands (`!skip`, `!remove`, `!reqon`, `!reqoff`), queue priority for gifts, gift-only requests, one-click bans, audio previews
* Test panel: simulated chat and gifts, a dry run that never touches the game, and a **pre-stream check** of the whole chain
* Session summary (maps played, best play, top requesters) on the dashboard and, on request, the overlay
* Fair queue (viewers take turns), requests that close by themselves at a queue size or a time, bans for a whole song or artist (`artist="a_hisa"`)
* **▶ view map** on queue, now-playing and history cards: [KIKI Beatmap Viewer](https://api.kiki-bm.app/)'s player opens under the card with the requested mods (osu!standard maps; see [Watching a map](#watching-a-map))
* How many people are watching (👁 1,234) next to the connection status, in the dashboard and on the mod panel
* Optional mod panel: run the queue from a phone on the same Wi-Fi, protected by a PIN
* Streaming from **two PCs**: OBS on the other PC loads the overlay with a secret link ([Two PCs: overlay on the OBS PC](#two-pcs-overlay-on-the-obs-pc))
* Optional chat TTS that reads chat aloud but never the commands (`!req …`), links or emoji, instead of TikFinity's TTS, in Thai, English, Japanese, Korean, Chinese or Hindi with one voice per language and a female or male preference (the browser's own voices, free, no key), and spoken announcements ("test1 requested …", "… is too hard, 7.25 stars") from lines you write ([recommended](#text-to-speech-use-osureqs-own-tts))
* A support file (log, raw events, settings) to send after a stream with a problem
* Switches between stable and lazer to match tosu, and updates itself: a new osureq is downloaded from the GitHub releases, checked and installed the next time you start it
* Close tosu, TikFinity or everything from the dashboard

## Requirements

* Windows 10/11, 64-bit
* osu! **stable** or **lazer**
* [**TikFinity Desktop**](https://tikfinity.zerody.one/), connected to the TikTok account and running while you are live (osureq reads chat through it),
  **or** nothing at all with **TikTok direct** (Settings → Connection, see [Chat without TikFinity](#chat-without-tikfinity))
* [**tosu**](https://github.com/tosuapp/tosu/releases/latest), running while you are live (tells when a song has ended and records results)
* OBS or TikTok LIVE Studio, for the overlay
* A recent browser for the dashboard: Chrome or Edge 130, Firefox 132 or Safari 18, or newer

## Getting started

1. Download `osureq-<version>-windows-x64.zip` from [Releases](https://github.com/Lacrymiraosu/tiktoklivereqsong/releases/latest)
   and unzip it into a folder of its own, e.g. `C:\osureq\` (it creates `config.json`, `queue.json`, `osureq.log` and a `cache\` folder next to itself).
2. Start TikFinity, tosu and osu!.
3. Double-click `osureq.exe`. A CMD window shows the log (closing it stops osureq) and the dashboard opens at <http://127.0.0.1:21280/>.
4. In **Settings → osu!**, choose stable or lazer. Set the rules you want (stars, length, modes, bans …) with the **Map rules** button above the queue.
5. For the overlay, add a **Browser Source** in OBS with `http://127.0.0.1:21280/overlay` (transparent background).

Use the **Test** panel on the dashboard to try the whole chain before you go live.

### Chat without TikFinity

In **Settings → Connection → Chat comes from**, pick **TikTok direct** and type your TikTok @name. osureq then reads the LIVE
itself: chat, gifts and viewer roles, with nothing else to install or keep open. When you are not live it checks every 30 seconds
and connects as soon as the LIVE starts. The status pill at the top of the dashboard then also shows how many people are watching
(👁 1,234), and so does the mod panel. With TikFinity the number appears only when TikFinity passes it on.

TikTok only lets a connection in when its address is signed. [Euler Stream](https://www.eulerstream.com) signs it (the sign
server TikTok-Live-Connector uses). Only connecting counts, chat does not: without an API key that is 30 connections an hour and
100 a day, plenty for one stream. A free key from eulerstream.com allows more; put it in **Euler Stream API key**. It stays on
your PC and is hidden in the support file. TikFinity stays available with the same switch.

### Text to speech: use osureq's own TTS

We recommend osureq's TTS over TikFinity's, because **osureq can say what was requested**. TikFinity only sees the chat
text, so it reads `!req 129891 HDHR` out as a string of numbers and letters. osureq knows the map behind the request and says its name:

> “test1 requested FREEDOM DiVE by xi, mapped by Nakagawa-Kanon, FOUR DIMENSIONS HDHR”

* It announces what happens to requests: added, rejected with the reason (“FREEDOM DiVE is too hard, 7.25 stars”), the next
  map, `!np`, play results and gifts. Every line can be changed in **Settings → TTS**, with `{title}`, `{artist}`, `{mapper}`,
  `{diff}`, `{mods}`, `{stars}` and more (the full list is in [What you can put in an announcement line](#what-you-can-put-in-an-announcement-line)).
* It reads the rest of the chat but never commands (`!req …`), links or emoji.
* It reads each message in its own language: Thai, English, Japanese, Korean, Chinese or Hindi, told apart by the letters (Thai letters
  in the text or the viewer's name mean Thai; kana, Japanese; Hangul, Korean; Han characters without kana, Chinese; Devanagari, Hindi). Languages
  written with Latin letters (Indonesian, Malay, Vietnamese, Spanish, French, German) cannot be told apart from English by letters, so they are read as English.
* In **Settings → TTS** you pick one voice per language and prefer a female or male voice. Each voice shows ♀, ♂ or ? after its name. Browsers do not
  say a voice's gender, so osureq knows it only for the voices whose names are in its list (taken from Microsoft's voice lists); any other voice shows
  `?` and is never guessed. When the language has no voice of the gender you chose, another voice reads and a message says which.
* The **Test the voice** box says which voice read the line, for example “Read by Microsoft Niwat ♂”.
* It can read only subscribers (ใจส้ม), followers, gifters or mods.

To use it, turn on **Read chat aloud** and **Announce requests** in **Settings → TTS**, and if TikFinity is running, **turn its TTS off** so chat
is not read twice. TTS starts when the dashboard opens (if the browser asks for a click first, click anywhere on the page) and
OBS records it as desktop audio. Windows has no Thai voice, so **Who speaks: auto** uses Google's voice for Thai, which sends the
text being read to Google.

Only Thai and English can go to Google. Japanese, Korean, Chinese and Hindi are read by a browser voice only and are never sent to Google;
with no voice for the language the line is skipped, and one message per language says to install a voice in Windows
(Settings → Time & language → Speech) or to use Microsoft Edge.
Which voices you can pick depends on your browser and Windows: Microsoft Edge may offer natural voices, while Chrome-based browsers (Chrome, Dia)
only have the voices Windows provides. So a female or male voice for a language exists only if your browser has one; the voice lists and the
**Test the voice** box show what it really has. Google's voice is a single voice.

#### What you can put in an announcement line

An announcement line is plain text with **placeholders** in curly braces. When something happens, osureq fills in the real values and says the result:

| Line | Said aloud |
|---|---|
| `Got it, {user}! {title} by {artist} is number {pos} in the queue.` | Got it, test1! freedom DiVE by xi is number 1 in the queue. |

Change the lines in **Settings → TTS → Announcement lines** (**Announce requests** must be on), then press **Save**. The **Thai examples** and **English examples**
buttons load a whole ready-made set into the boxes (every line is replaced; nothing is saved until you press Save). A placeholder is lowercase letters
and underscores between `{` and `}`, and a line can hold at most 300 letters.

**Placeholders.** Which ones have a value depends on the event; see the second table.

| Placeholder | What it says | Example |
|---|---|---|
| `{user}` | The viewer's name (their TikTok nickname with emoji removed and cut at 40 letters, or the plain user name when there is no nickname). It is whoever made the request; for `!np` and `!pos` the viewer who typed it, for a vote skip the viewer whose vote passed, for a gift the sender | `test1` |
| `{title}` | Song title | `freedom DiVE` |
| `{artist}` | Artist | `xi` |
| `{mapper}` | Mapper | `Nakagawa-Kanon` |
| `{diff}` | Difficulty name | `four dimensions` |
| `{mods}` | Mods, spaced so the voice spells them | `HD HR` |
| `{stars}` | Star rating, always two decimals; empty when osureq does not know it | `7.25` |
| `{pos}` | Place in the queue (1 = next) | `2` |
| `{setid}` | Beatmap set ID | `39804` |
| `{setid_digits}` | The same ID read digit by digit | `3 9 8 0 4` |
| `{requester}` | Only for `!np`: the user name of who requested the map that is playing (empty when it was not a request) | `fan` |
| `{reason}` | Only for rejections: why, in a short phrase (see below) | `it is too hard` |
| `{max}` | The limit that was exceeded: stars (`6.00`), song length (`1:40`) or queue size / requests per viewer (`30`) | `6.00` |
| `{min}` | Too easy: the minimum stars (`3.00`). Played recently: how many minutes ago it was played (`0` when under a minute) | `3.00` |
| `{length}` | Too long: the song's length with the requested mods (minutes:seconds) | `3:20` |
| `{wait}` | Cooldown: seconds left, rounded up | `600` |
| `{n}` | Queue full or too many requests: how many are waiting now | `1` |
| `{mode}` | Mode not accepted: `osu!`, `taiko`, `catch` or `mania` | `osu!` |
| `{acc}` | Result: accuracy with two decimals, no percent sign | `98.52` |
| `{rank}` | Result: the rank letter from osu! | `S` |
| `{combo}` | Result: max combo | `812` |
| `{misses}` | Result: number of misses | `2` |
| `{pp}` | Result: pp, rounded to a whole number | `301` |
| `{coins}` | Gift: the gift's coin value | `5` |
| `{gift}` | Gift: the gift's name | `Rose` |

**Which line says what, and when.** A placeholder that an event does not have is simply left out of the sentence. "Map details" means
`{title} {artist} {mapper} {diff} {mods} {stars} {setid} {setid_digits}`.

| Line in Settings (English / Thai) | When it is said | Placeholders it has | Default (English set) |
|---|---|---|---|
| Request added (ขอเพลงสำเร็จ) | A request is added to the queue | `{user}`, map details (`{mods}` = the mods asked for), `{pos}` | `{user} requested {title} by {artist}, mapped by {mapper}, {diff} {mods}` |
| Rejected (any other reason) (ถูกปฏิเสธ (เหตุผลอื่น)) | A request is rejected for a reason that has no line of its own below | `{user}`, `{reason}`, `{mods}`; once the map is found also `{title} {artist} {mapper} {setid} {setid_digits}`; `{diff}` with a too-easy, too-hard, too-long or mode rejection; `{n}`, `{max}` or `{mode}` where the table above says so | `{user}, that request did not work: {reason}` |
| Rejected: too hard (ถูกปฏิเสธ: ยากเกินไป) | Over the maximum stars | `{user}`, `{title} {artist} {mapper} {diff} {mods} {setid} {setid_digits}`, `{stars}`, `{max}`, `{reason}` | `{title} is too hard, {stars} stars` |
| Rejected: too easy (ถูกปฏิเสธ: ง่ายเกินไป) | Under the minimum stars | the same, with `{min}` instead of `{max}` | `{title} is too easy, {stars} stars` |
| Rejected: too hard with DT/HR (ถูกปฏิเสธ: ยากเกินไปเมื่อใส่ DT/HR) | Over the star limit for DT, NC or HR requests | like too hard (`{max}` is that limit) | `{title} is too hard with {mods}` |
| Rejected: too long (ถูกปฏิเสธ: ยาวเกินไป) | Longer than the maximum length | `{user}`, `{title} {artist} {mapper} {diff} {mods} {setid} {setid_digits}`, `{length}`, `{max}`, `{reason}` | `{title} is too long` |
| Rejected: already in the queue (ถูกปฏิเสธ: อยู่ในคิวแล้ว) | The map is already waiting | `{user}`, `{title} {artist} {mapper} {mods} {setid} {setid_digits}`, `{reason}` | `{title} is already in the queue` |
| Rejected: played recently (ถูกปฏิเสธ: เพิ่งเล่นไป) | The map was played within the no-repeat time | the same, plus `{min}` (minutes ago) | `{title} was played recently` |
| Rejected: cooldown (ถูกปฏิเสธ: ติดคูลดาวน์) | The viewer asked again too soon | `{user}`, `{wait}`, `{mods}`, `{reason}` (no map details) | `-` (silent) |
| Rejected: too many requests (ถูกปฏิเสธ: ขอเกินจำนวน) | The viewer already has the most requests allowed waiting | `{user}`, `{n}`, `{max}`, `{mods}`, `{reason}` (no map details) | `-` (silent) |
| Rejected: no map with that name (ถูกปฏิเสธ: ไม่พบแมพชื่อนี้) | A song-name request found nothing | `{user}`, `{mods}`, `{reason}` (no map details) | `No map found for {user}` |
| Next map opens (เปิดเพลงถัดไป) | A request is opened as now playing (Next, Play, auto mode, or going back to the previous map) | `{user}` (the requester), map details | `Next up, {title}, requested by {user}` |
| !np (what is playing) (!np (เพลงที่กำลังเล่น)) | A viewer types `!np` while a map is open | `{user}` (who asked), map details, `{requester}` | `Now playing {artist} - {title}, set {setid}` |
| !np with nothing playing (!np ตอนยังไม่มีเพลง) | `!np` with no map open | `{user}` (who asked) | `Nothing is playing yet` |
| !pos (queue position) (!pos (ลำดับคิว)) | A viewer with a waiting request types `!pos` (a viewer with none gets no answer) | `{user}`, the details of their first waiting request, `{pos}` | `-` (silent) |
| Result: cleared (ผลการเล่น: ผ่าน) | The play reaches the results screen (needs tosu) | `{user}` (the requester), `{title} {artist} {mapper} {diff}`, `{mods}` (the mods played), `{acc} {rank} {combo} {misses} {pp}` | `Cleared {title} with {acc} percent` |
| Result: failed (ผลการเล่น: ไม่ผ่าน) | The player failed the map (needs tosu) | the same as cleared | `-` (silent) |
| Gift (ของขวัญ) | Every gift a viewer sends, whether or not it gives priority | `{user}` (the sender), `{coins}`, `{gift}` | `Thank you {user} for the gift` |
| Vote skip passed (โหวตข้ามเพลงสำเร็จ) | Enough viewers voted to skip the song that is playing (votes before that are not announced) | `{user}` (the viewer whose vote made it pass), `{title} {artist} {diff}` | `Chat voted to skip {title}` |
| Requests closed (queue full or time) (ปิดรับคำขอ (คิวเต็มหรือถึงเวลา)) | Requests close by themselves because the queue is full, or at the closing time | none | `The queue is full, requests are paused` |
| Requests open again (เปิดรับคำขออีกครั้ง) | Requests open again by themselves | none | `Requests are open again` |

The default lines above are the English set; the Thai set is in [`docs/README.th.md`](docs/README.th.md) and in the **Thai examples** button. A player who quits a map
is never announced, and neither are other things the activity feed shows, such as a gift that moved a request up.

**`{reason}` by group.** It is a short phrase for the rejection that happened, for example "the queue is full" or "it is too hard". The groups:
viewer-level (you cannot request right now, the queue is full, you already have requests waiting, please wait a little, follow the stream to request,
requests are closed, send a gift to request); blocked (the map, mapper, song or artist is blocked); map rules (too easy, too hard, too hard with those mods,
too long, that map status or mode is not accepted, the mode cannot be converted, it was played recently, it is already in the queue, it is playing right now);
problems finding the map (the map was not found, no map has that name, the search did not work, the map could not be checked, that is not a map);
and mods (those mods do not work, mods are not allowed, that mod is not allowed). The full list with the Thai wording is in
[`docs/DEVELOPMENT.md`](docs/DEVELOPMENT.md#announcement-lines-and-placeholders).

**Tips**

* **Say nothing:** leave a line empty or write `-` (the box then shows "(say nothing)"). A line that ends up empty after the placeholders are filled is not said either.
  A box of its own that is silent stays silent: it does not fall back to "Rejected (any other reason)", which is used only for rejections that have no box of their own.
* **A missing value just disappears,** together with the extra spaces, so one line can serve several events. The line `{user}, {title} was not added: {reason}`
  says "test2, was not added: the queue is full" when there is no map to name, and "test1, Mid Song was not added: that mode is not accepted" when there is.
* **Spell placeholders exactly,** in lowercase: `{User}` or `{user2}` is not recognised and is read out with its braces, while a misspelled lowercase one such as `{usr}` quietly says nothing.
* **Reading an ID:** `{setid_digits}` says the ID one digit at a time ("3 9 8 0 4") instead of as a big number.
* **`{reason}` follows the language of the line:** Thai when your line contains Thai letters, otherwise English.
* **Capital letters:** an ALL-CAPS word of three letters or more is lowercased so the voice reads it as a word (FREEDOM becomes freedom; two-letter words such as HD or GG stay).
  This applies to the values filled in (title, artist, mapper, difficulty, viewer name, gift name; not `{requester}`), not to the text you type. A name written entirely in capitals is lowercased as a whole, so `GG ROSE` is read as "gg rose".
* **`@`** is removed from the sentence (a voice would say "at").
* **Which voice speaks:** the finished sentence goes to the same voice as chat. With **Language** on auto in **Settings → TTS**, a sentence with Thai letters in it (your text or a viewer's name) is read in Thai, any other in English; a fixed language wins.
  The voice, gender, speed and volume settings apply. Announcements do not get "name says" in front, ignore the "who is read" groups and the blocked words, and are read only in a dashboard page where TTS is on (🔊).
* **Several maps in one message:** each kind of outcome is said once, and at most two lines are said for the whole message.
* **Trying a line:** after saving, use **Simulate chat** in the Test panel (for example `!req 129891`, or `!np`), which makes the same announcement a real viewer would.
  The **Test the voice** box reads exactly what you type, without filling in placeholders, so use it to check the voice, not the line.
* A line longer than 300 letters (counting the placeholders) is refused when you press Save, with the message "Announcement … is longer than 300 letters".

**Ready-to-copy lines.** Each was filled in by osureq's own code with a sample map (FREEDOM DiVE by xi, 7.25 stars, set 39804) and the values in the table above; your values will differ.

| Line | English line, said as | Thai line, said as |
|---|---|---|
| Request added | `Got it, {user}! {title} by {artist} is number {pos} in the queue.`<br>Got it, test1! freedom DiVE by xi is number 1 in the queue. | `{user} ขอเพลง {title} ของ {artist} อยู่คิวที่ {pos} {mods}`<br>test1 ขอเพลง freedom DiVE ของ xi อยู่คิวที่ 1 HD HR |
| Rejected: too hard | `Sorry {user}, {title} is {stars} stars and I only take up to {max}.`<br>Sorry test1, freedom DiVE is 7.25 stars and I only take up to 6.00. | `{user} เพลง {title} ยากไป {stars} ดาว เรารับไม่เกิน {max} ดาวนะ`<br>test1 เพลง freedom DiVE ยากไป 7.25 ดาว เรารับไม่เกิน 6.00 ดาวนะ |
| Rejected: too easy | `{title} is only {stars} stars, the minimum is {min}`<br>Easy One is only 1.50 stars, the minimum is 3.00 | `เพลง {title} ง่ายไป {stars} ดาว ต้องอย่างน้อย {min} ดาว`<br>เพลง Easy One ง่ายไป 1.50 ดาว ต้องอย่างน้อย 3.00 ดาว |
| Rejected (any other reason) | `{user}, that request did not work: {reason}`<br>test1, that request did not work: please wait a little | `{user} ขอเพลงไม่ได้ {reason}`<br>test1 ขอเพลงไม่ได้ รอสักครู่ก่อนนะ |
| Result: cleared | `{user} cleared {title}: {acc} percent, rank {rank}, {combo} combo, {misses} misses, {pp} pp`<br>Kiki cleared freedom DiVE: 98.52 percent, rank S, 812 combo, 2 misses, 301 pp | `ยินดีด้วย {user} ผ่านเพลง {title} ความแม่นยำ {acc} เปอร์เซ็นต์ แรงก์ {rank} คอมโบ {combo} พลาด {misses} ครั้ง {pp} พีพี`<br>ยินดีด้วย Kiki ผ่านเพลง freedom DiVE ความแม่นยำ 98.52 เปอร์เซ็นต์ แรงก์ S คอมโบ 812 พลาด 2 ครั้ง 301 พีพี |
| Gift | `Thank you {user} for the {gift}! {coins} coins`<br>Thank you Kid for the Rose! 5 coins | `ขอบคุณ {user} สำหรับ {gift} {coins} เหรียญ`<br>ขอบคุณ Kid สำหรับ Rose 5 เหรียญ |
| !np with the set ID read out | `{title} set {setid_digits} for {requester}`<br>freedom DiVE set 3 9 8 0 4 for fan | `กำลังเล่น {artist} - {title} ขอโดย {requester}`<br>กำลังเล่น xi - freedom DiVE ขอโดย fan |

### What viewers type

```
!req 2096297                                      ← difficulty ID (tried as a set ID if no such difficulty exists)
!req https://osu.ppy.sh/beatmapsets/962870#mania/2096297
!req https://osu.ppy.sh/beatmapsets/962870        ← whole set: the hardest difficulty that passes the rules
!req freedom dive                                 ← song name: the most played title match that passes the rules
!req 2096297 HDHR                                 ← with mods (a song name takes them as +HDHR at the end)
!remove                                           ← removes your own latest request
!pos                                              ← where are my requests in the queue
!np                                               ← what osu! is playing (from tosu) and who asked for it
```

With vote skip turned on (Settings → Command), viewers can type `!voteskip`. Mods can also use `!skip` (skip the song being played), `!remove @user` and `!reqon` / `!reqoff` (open/close requests).
Every command name can be changed in the settings.

### Several maps in one message

A message asks for one map by default. Raise **Settings → Command → Maps per message** (1 to 5) and a viewer can request
several at once, with mods written after the map they belong to:

```
!req 2096297 1859353 HDHR 1521430                 ← three requests; HDHR goes with 1859353
```

* It only counts when every word after the command is a map ID, an osu! link or mods. A song name, or an ID followed by chat
  (`!req 2096297 please`), works as before.
* Every map goes through all the rules on its own and counts toward the viewer's requests per person. The same map twice counts
  once, and a wrong mod (`+HDXX`) rejects only that map.
* The cooldown is checked once per message. When the problem is the viewer (cooldown, request limit, full queue, requests closed,
  ban, followers only, no gift), the rest of the message is dropped with one pop-up. Maps over the limit are ignored, and a pop-up says so.
* With gift-only requests each map costs one request.
* The overlay shows every map; TTS says each outcome once and at most two lines per message.
* In the **Test** panel, type IDs or links (each may be followed by mods, e.g. `2096297 HDHR`) and the command is added for you.

### Watching a map

The **▶ view map** button on queue, now-playing and history cards opens a map player under the card, with the map, its hitsounds and
the mods that were requested. One player is open at a time, and starting it pauses the ♪ preview (and the other way round).

* The player is [KIKI Beatmap Viewer](https://api.kiki-bm.app/) (player.kiki-bm.app): free, no key. It runs in your browser, osureq loads
  no script from KIKI: it shows their page in a frame and sends it messages.
* osu!standard only. Taiko, catch, mania and converts have no ▶, and neither have history entries saved before osureq recorded the mode.
* When osureq has already downloaded and checked the map, the player gets that file. Otherwise it loads the map from the community
  mirrors by itself. If loading fails, press ▶ in the player again; osureq then lets it use the mirrors.
* The player's own ⓘ credits stay visible, as KIKI asks (its default skin and hitsounds are by ppy, CC BY-NC 4.0).
* Turn it off in **Settings → osu! → KIKI map viewer** (one switch for the dashboard, one for the mod panel). The mod panel has the same ▶,
  always from the mirrors. The "viewer ↗" link to a beatmap viewer is still there: by default KIKI Beatmap Viewer's site (kiki-bm.app),
  or your own in **Settings → osu! → Beatmap viewer URL**.

### Two PCs: overlay on the OBS PC

OBS on another PC cannot open `http://127.0.0.1:21280/overlay`. Give it a secret link instead:

1. **Settings → Mod panel**: turn the mod panel on and save. The link uses its port (21281 by default).
2. **Settings → Overlay**: turn on **Let another PC load the overlay** and save. The link appears
   (`http://<this PC's address>:21281/o/<secret>`) with a Copy button.
3. On the OBS PC, add a **Browser Source** with that link (transparent background, like the normal overlay).

* Both PCs must be on the same network (not a guest network), and osureq must be allowed through the Windows firewall for private networks, as for the mod panel.
* The link only shows the overlay: it cannot change anything, and the dashboard stays on this PC. But the overlay shows viewers' names and the
  songs they asked for, so anyone on your network who has the link can see them. Keep it to yourself. **New link** (same place) makes a new
  secret and the old link stops working at once, so paste the new one into OBS.
* Wrong links from one address are blocked for 10 minutes after a few tries, like wrong PINs.
* TTS still plays on the PC where the dashboard is open. The **pre-stream check** counts this overlay too.
* So far tested in a browser on one PC (also through its network address), not yet with OBS on a second PC.

## How it works

```
TikTok chat ──► TikFinity ──WebSocket──► osureq ──► osu.ppy.sh (map info, MD5)
                                           │
                                           ├──► mirrors: hinai, Nekoha, … (download .osz) ──► cache\
                                           ├──► osu! (opens the .osz or copies it into Songs)
           tosu ──WebSocket (game state)──►│
                                           └──► dashboard + overlay (http://127.0.0.1:21280)
```

1. **Chat in**: TikFinity sends chat messages and gifts over WebSocket (`ws://127.0.0.1:21213/`), or with TikTok direct osureq
   reads them from TikTok's own LIVE WebSocket (the connection signed by Euler Stream).
   osureq only looks at messages that start with a command such as `!req`.
2. **Requester checks**: ban list, full queue, requests per viewer and cooldown, all without network access.
3. **Map info**: the map's page on osu.ppy.sh is read (no API key needed) for title, stars, length, mode, status and the MD5 of every difficulty.
   A song name is first searched on the mirrors (hinai, then Nekoha, osu.direct, catboy.best if one fails); the best title matches (most played first) are tried until one passes the rules.
4. **Map rules**: stars, length, mode, status, duplicates and recently played maps.
   A rejected request shows its reason on the overlay, because TikFinity cannot write back into TikTok chat.
5. **Queue**: saved in `queue.json`, so it survives a restart. Viewers whose gifts add up to enough coins get priority (when turned on).
6. **Download and verify**: the `.osz` is downloaded from the mirrors in the configured order, one map at a time, and the MD5 of every
   `.osu` inside is compared with osu!. An outdated mirror copy makes it try the next source. The next maps are downloaded ahead of time so they open instantly (the **Auto download** switch above the queue turns this off).
7. **Open**: press **Next** on the dashboard (or the hotkey) and osureq opens the `.osz`; osu! imports it and jumps to the map.
8. **Auto mode**: osureq follows the game state from tosu. Once the opened map has really been played and the player leaves the results
   screen, the next map opens after a few seconds. The play result (accuracy, rank, combo, misses, pp, mods) is saved and shown on the overlay.
   If tosu disconnects, auto mode waits; there is no timer guesswork.

The dashboard only listens on `127.0.0.1`, so other devices on the network cannot reach it. The optional mod panel and overlay link are the
only things they can open, and only with the PIN or the secret link. osureq only downloads from whitelisted hosts.

The details (code layout, API, every setting, testing) are in [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md).

## Building

You need [Go](https://go.dev/dl/) 1.23 or newer on Windows (`winget install GoLang.Go`).

```powershell
git clone https://github.com/Lacrymiraosu/tiktoklivereqsong.git
cd tiktoklivereqsong
.\build.ps1 -Version 1.5.1
```

`build.ps1` runs `go vet` and `go test`, generates the Windows resources (icon, name, version) and builds `dist\osureq.exe`.
If PowerShell refuses to run scripts, build by hand:

```powershell
$env:CGO_ENABLED = "0"
go run ./tools/winres -version 1.5.1
go build -trimpath -ldflags "-X main.version=1.5.1" -o dist\osureq.exe .
```

### Automatic builds and releases (GitHub Actions)

* Every push to `main` and every pull request is vetted, tested and built; the exe stays on the Actions page for 14 days.
* To release a new version, update the version number in `build.ps1` (and the changelog) and merge it into `main`. Then either
  open **Actions → build → Run workflow**, keep `main`, type the version (e.g. `1.5.1`) and press **Run workflow**, or push a tag:

  ```powershell
  git tag v1.5.1
  git push origin v1.5.1
  ```

  GitHub builds with the tag's version and publishes a release with `osureq-<version>-windows-x64.zip` (the exe, `LICENSE` and `THIRD_PARTY_LICENSES.txt`) and its `.sha256`.

## Credits

Beatmap data, covers and audio previews come from osu! (ppy Pty Ltd). Beatmaps belong to their mappers, music to its artists.

* Idea: [chaiwat2183](https://osu.ppy.sh/users/21039251)
* [osu!](https://osu.ppy.sh) by ppy Pty Ltd: the game
* [TikFinity](https://tikfinity.zerody.one/) by zerody: chat and gifts from TikTok LIVE
* [Euler Stream](https://www.eulerstream.com): signs the TikTok direct connection; [gotiktoklive](https://github.com/steampoweredtaco/gotiktoklive)
  for the TikTok LIVE message types, and [TikTok-Live-Connector](https://github.com/zerodytrash/TikTok-Live-Connector) for showing how the connection works
* [tosu](https://github.com/tosuapp/tosu): live osu! game state
* [KIKI Beatmap Viewer](https://api.kiki-bm.app/): the map player behind ▶ (player.kiki-bm.app), free and without a key
* Beatmap mirrors, where the `.osz` files are downloaded from: [hinai](https://mirror.hinamizawa.ai), [Nekoha](https://mirror.nekoha.moe), [beatconnect](https://beatconnect.io), [osu.direct](https://osu.direct), [catboy.best](https://catboy.best) and [Sayobot](https://osu.sayobot.cn) (hinai, Nekoha, osu.direct and catboy.best also power the song search)
* Go libraries: [coder/websocket](https://github.com/coder/websocket), [golang.org/x/sys](https://pkg.go.dev/golang.org/x/sys), [protobuf-go](https://github.com/protocolbuffers/protobuf-go),
  [tc-hib/winres](https://github.com/tc-hib/winres), [nfnt/resize](https://github.com/nfnt/resize), [golang.org/x/image](https://pkg.go.dev/golang.org/x/image)
* [Bootstrap](https://getbootstrap.com) 6 (with [Floating UI](https://floating-ui.com) and [Vanilla Calendar Pro](https://github.com/uvarov-frontend/vanilla-calendar-pro)): the dashboard's components
* Fonts: [Kanit](https://fonts.google.com/specimen/Kanit), [IBM Plex Sans Thai](https://fonts.google.com/specimen/IBM+Plex+Sans+Thai), [IBM Plex Mono](https://fonts.google.com/specimen/IBM+Plex+Mono) and [Varela Round](https://fonts.google.com/specimen/Varela+Round) (overlay) from Google Fonts

osureq is a fan project and is not affiliated with or endorsed by ppy Pty Ltd, TikTok, TikFinity, Euler Stream, KIKI Beatmap Viewer or tosu.
osu! is a trademark of ppy Pty Ltd. TikTok is a trademark of its respective owner.

## License

[MIT](LICENSE) © 2026 kikinomae. The licenses of the third-party code built into `osureq.exe` (the Go standard library,
`golang.org/x/sys`, `coder/websocket`, `protobuf-go` and gotiktoklive's message types) are in [THIRD_PARTY_LICENSES.txt](THIRD_PARTY_LICENSES.txt).
