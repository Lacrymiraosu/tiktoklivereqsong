"use strict";
// Dashboard for the osu! request helper. Talks to the helper on 127.0.0.1 only.

const $ = s => document.querySelector(s);
const h = (tag, attrs = {}, ...kids) => {
  const e = document.createElement(tag);
  for (const [k, v] of Object.entries(attrs)) {
    if (v == null || v === false) continue;
    if (k === "class") e.className = v;
    else if (k === "style") e.style.cssText = v;
    else if (k.startsWith("on")) e.addEventListener(k.slice(2), v);
    else e.setAttribute(k, v === true ? "" : v);
  }
  for (const k of kids.flat()) if (k != null && k !== false) e.append(k instanceof Node ? k : String(k));
  return e;
};
const svg = d => { const s = document.createElementNS("http://www.w3.org/2000/svg", "svg"); s.setAttribute("viewBox", "0 0 24 24"); s.innerHTML = d; return s; };
const ICON = {
  play: '<path d="M7 5l12 7-12 7z"/>',
  up: '<path d="M6 15l6-6 6 6"/>',
  down: '<path d="M6 9l6 6 6-6"/>',
  x: '<path d="M6 6l12 12M18 6L6 18"/>',
  skip: '<path d="M5 5l9 7-9 7zM18 5v14"/>',
  retry: '<path d="M4 4v6h6"/><path d="M20 12A8 8 0 0 0 5.3 7.3L4 10m16 10v-6h-6"/><path d="M4 12a8 8 0 0 0 14.7 4.7L20 14"/>',
  next: '<path d="M5 12h14M13 6l6 6-6 6"/>',
  copy: '<rect x="9" y="9" width="11" height="11" rx="2"/><path d="M5 15V5a2 2 0 0 1 2-2h8"/>',
};

/* ---------- i18n (en / th) ---------- */
const T = {
  title: ["osu! requests", "ขอเพลง osu!"],
  subtitle: ["TikTok LIVE → TikFinity → osu!", "TikTok LIVE → TikFinity → osu!"],
  requests: ["Requests", "รับคำขอ"],
  queue: ["Queue", "คิว"],
  removeLast: ["Remove last", "ลบอันล่าสุด"],
  clear: ["Clear", "ล้างคิว"],
  rejections: ["Rejected", "ถูกปฏิเสธ"],
  activity: ["Activity", "กิจกรรม"],
  history: ["History", "ประวัติ"],
  tfEvents: ["TikFinity events", "อีเวนต์จาก TikFinity"],
  rawHint: ["The first event of each type is kept so you can check field names. Turn on “Log raw events” in Settings → Connection to see every event.", "เก็บอีเวนต์แรกของแต่ละประเภทไว้ให้ตรวจชื่อฟิลด์ เปิด “บันทึกอีเวนต์ดิบ” ใน ตั้งค่า → การเชื่อมต่อ เพื่อดูทุกอีเวนต์"],
  info: ["Info", "ข้อมูล"],
  settings: ["Settings", "ตั้งค่า"],
  save: ["Save", "บันทึก"],
  resetTab: ["Reset tab", "คืนค่าแท็บนี้"],
  testPanel: ["Test", "ทดสอบ"],
  modeChat: ["Simulate chat", "จำลองแชท"],
  modeQuick: ["Quick add", "เพิ่มด่วน"],
  modeGift: ["Gift", "ของขวัญ"],
  hintGift: ["Pretends @user sent a gift worth this many coins (gift priority or gift-only requests must be on in Settings → Queue).", "จำลองว่า @ผู้ใช้ ส่งของขวัญมูลค่าเท่านี้เหรียญ (ต้องเปิดคิวพิเศษจากของขวัญ หรือขอเพลงด้วยของขวัญ ใน ตั้งค่า → คิว)"],
  giftPh: ["coins, e.g. 30", "จำนวนเหรียญ เช่น 30"],
  giftDone: ["Gift sent: {n} priority and {r} request(s) earned", "ส่งของขวัญแล้ว: ได้คิวพิเศษ {n} ครั้ง และขอเพลงได้ {r} เพลง"],
  priority: ["priority", "คิวพิเศษ"],
  dryRun: ["Dry run", "ทดลองไม่เปิดเกม"],
  clearTest: ["Clear test data", "ล้างข้อมูลทดสอบ"],
  testPh: ["!req 2096297, a link or any chat message", "!req 2096297, ลิงก์ หรือแชททั่วไป"],
  spokenTest: ["Not a request: read aloud by chat TTS", "ไม่ใช่คำขอ: ส่งให้เสียงอ่านแชทแล้ว"],
  testUserPh: ["@user", "@ผู้ใช้"],
  testBtn: ["Send", "ส่ง"],
  hintChat: ["Sends the message as if @user typed it in TikTok chat: command, requests on/off, per-user limit, cooldown and every map rule apply. Works offline. A bare ID or link gets {cmd} added.", "ส่งข้อความเหมือน @ผู้ใช้ พิมพ์ในแชท TikTok ใช้กฎทั้งหมด (คำสั่ง เปิด/ปิดรับ จำนวนต่อคน คูลดาวน์ กฎแมพ) ใช้ได้แม้ไม่ได้ไลฟ์ ถ้าพิมพ์แค่ ID หรือลิงก์จะเติม {cmd} ให้"],
  hintQuick: ["Adds a map straight to the queue. Only map rules apply (no per-user limit or cooldown).", "เพิ่มแมพเข้าคิวทันที ใช้แค่กฎแมพ (ไม่นับจำนวนต่อคนและคูลดาวน์)"],
  hintDry: [" Dry run is on: Next and Play move the queue but never open osu! or copy into Songs.", " เปิดโหมดทดลองอยู่: กด ถัดไป/เล่น จะเลื่อนคิวแต่ไม่เปิด osu! และไม่คัดลอกเข้า Songs"],
  quit: ["Quit osureq", "ออกจาก osureq"],
  closeTosu: ["Close tosu", "ปิด tosu"], closeTikfinity: ["Close TikFinity", "ปิด TikFinity"], quitAll: ["Close everything", "ปิดทั้งหมด"],
  notRunning: ["{app} is not running", "{app} ไม่ได้เปิดอยู่"],
  confirmCloseTosu: ["Close tosu? Auto mode pauses until tosu runs again.", "ปิด tosu? โหมดอัตโนมัติจะหยุดจนกว่าจะเปิด tosu ใหม่"],
  confirmCloseTikfinity: ["Close TikFinity? Chat requests stop until it runs again.", "ปิด TikFinity? จะไม่รับคำขอจากแชทจนกว่าจะเปิดใหม่"],
  confirmQuitAll: ["Close tosu, TikFinity and osureq?", "ปิด tosu, TikFinity และ osureq?"],
  closedApp: ["Closed {app}", "ปิด {app} แล้ว"],
  quitAllDone: ["Everything is closed. You can close this tab.", "ปิดทั้งหมดแล้ว ปิดแท็บนี้ได้เลย"],
  masterVolume: ["Master volume", "ระดับเสียงหลัก"], mute: ["Mute", "ปิดเสียง"], unmute: ["Unmute", "เปิดเสียง"],
  volHint: ["Master volume for every sound the dashboard plays (map previews). Saved for all browsers.", "ระดับเสียงหลักของทุกเสียงที่แดชบอร์ดเล่น (ตัวอย่างเพลง) บันทึกไว้ใช้ทุกเบราว์เซอร์"],
  required: ["required", "จำเป็น"],
  ban: ["Ban…", "แบน…"], more: ["More…", "เพิ่มเติม…"],
  setupTitle: ["Before you go live", "ก่อนเริ่มไลฟ์"], hideDetails: ["hide details", "ซ่อนรายละเอียด"], showDetails: ["show details", "แสดงรายละเอียด"], banUser: ["Ban @{u}", "แบน @{u}"], banMap: ["Ban this map", "แบนแมพนี้"], banMapper: ["Ban mapper {m}", "แบนแมพเปอร์ {m}"],
  confirmBan_user: ["Ban @{v}? Their waiting requests are removed and new ones are rejected.", "แบน @{v}? คำขอที่รออยู่จะถูกลบ และคำขอใหม่จะถูกปฏิเสธ"],
  confirmBan_set: ["Ban beatmapset {v}? Waiting requests for it are removed.", "แบนเซ็ต {v}? คำขอที่รออยู่ของแมพนี้จะถูกลบ"],
  confirmBan_song: ["Ban the song {v}? Every version of it (other sets, TV size, cut…) is rejected and waiting requests for it are removed.", "แบนเพลง {v}? ทุกเวอร์ชันของเพลงนี้ (เซ็ตอื่น TV size ตัดสั้น ฯลฯ) จะถูกปฏิเสธ และคำขอที่รออยู่จะถูกลบ"],
  banSong: ["Ban this song (every version)", "แบนเพลงนี้ (ทุกเวอร์ชัน)"], banArtist: ["Ban every song by {a}", "แบนทุกเพลงของ {a}"],
  confirmBan_artist: ["Ban every song by {v}? Waiting requests for them are removed. Undo in Settings → Map rules.", "แบนทุกเพลงของ {v}? คำขอที่รออยู่จะถูกลบ ยกเลิกได้ใน ตั้งค่า → กฎแมพ"],
  n_banned_artist: ["Banned songs by {v} (removed {n})", "แบนเพลงของ {v} แล้ว (ลบ {n} รายการ)"],
  e_song_rule: ["Song rule {rule}: write artist=\"…\", title=\"…\", Artist - Title, or a title", "กฎเพลง {rule}: เขียนเป็น artist=\"…\", title=\"…\", ศิลปิน - ชื่อเพลง หรือชื่อเพลง"],
  n_banned_song: ["Banned the song {v} (removed {n})", "แบนเพลง {v} แล้ว (ลบ {n} รายการ)"],
  r_song_blacklisted: ["song blacklisted", "เพลงถูกแบน"], r_artist_blacklisted: ["artist blacklisted", "ศิลปินถูกแบน"],
  confirmBan_mapper: ["Ban maps by {v}? Waiting requests for their maps are removed.", "แบนแมพของ {v}? คำขอที่รออยู่ของแมพเหล่านี้จะถูกลบ"],
  banned: ["Banned {v} · removed {n}. Undo in Settings → Map rules.", "แบน {v} แล้ว · ลบ {n} คำขอ ยกเลิกได้ใน ตั้งค่า → กฎแมพ"],
  pvPlay: ["preview", "ฟังตัวอย่าง"], pvStop: ["stop", "หยุด"], pvFail: ["Could not play the preview", "เล่นตัวอย่างเพลงไม่ได้"],
  viewer: ["viewer", "viewer"], viewerTitle: ["Open in the beatmap viewer", "เปิดใน beatmap viewer"],
  confirmQuit: ["Stop osureq? Requests will not be taken until you start it again.", "ปิด osureq? จะไม่รับคำขอจนกว่าจะเปิดใหม่"],
  quitDone: ["osureq stopped. You can close this tab.", "ปิด osureq แล้ว ปิดแท็บนี้ได้เลย"],
  trayHint: ["osureq runs in its CMD window, which shows the log. Closing that window also stops osureq.", "osureq ทำงานอยู่ในหน้าต่าง CMD ซึ่งแสดง log ด้วย ปิดหน้าต่างนั้นก็เป็นการปิด osureq"],
  cmd_cmd_done: ["Command done (shown on the overlay)", "ทำตามคำสั่งแล้ว (แสดงบน overlay)"],
  cmd_cmd_not_mod: ["@{u} is not a mod, so the command was ignored (as in real chat). Add mods in Settings → Command.", "@{u} ไม่ใช่ม็อด คำสั่งจึงไม่ทำงาน (เหมือนในแชทจริง) เพิ่มม็อดได้ใน ตั้งค่า → คำสั่ง"],
  cmd_cmd_nothing: ["Nothing to do: no matching request, nothing is playing, already voted, or asked again too soon (20 s)", "ไม่มีอะไรให้ทำ: ไม่มีคำขอที่ตรงกัน ไม่มีเพลงที่กำลังเล่น โหวตไปแล้ว หรือถามซ้ำเร็วเกินไป (20 วินาที)"],
  giftNum: ["Enter the number of coins, e.g. 30", "ใส่จำนวนเหรียญเป็นตัวเลข เช่น 30"],
  e_busy: ["Already opening a map, try again in a moment", "กำลังเปิดแมพอยู่ ลองใหม่อีกครู่"],
  e_queue_empty: ["The queue is empty", "คิวว่างอยู่"],
  e_not_found: ["That request is no longer in the queue", "คำขอนี้ไม่อยู่ในคิวแล้ว"],
  e_get_failed: ["Could not get {title}: {msg}", "โหลด {title} ไม่ได้: {msg}"],
  e_ban_empty: ["Nothing to ban", "ไม่มีอะไรให้แบน"], e_ban_set_id: ["{v} is not a beatmapset ID", "{v} ไม่ใช่ ID เซ็ต"], e_ban_kind: ["Unknown ban type", "ไม่รู้จักประเภทการแบน"],
  e_gifts_off: ["Gift priority and gift-only requests are both off (Settings → Queue)", "ปิดทั้งคิวพิเศษและการขอเพลงด้วยของขวัญอยู่ (ตั้งค่า → คิว)"],
  e_tikfinity_missing: ["TikFinity is not installed", "ยังไม่ได้ติดตั้ง TikFinity"],
  e_tosu_no_path: ["Where tosu.exe is is not known yet: set it in Settings → Connection, or start tosu once", "ยังไม่รู้ที่อยู่ tosu.exe: ใส่ใน ตั้งค่า → การเชื่อมต่อ หรือเปิด tosu สักครั้ง"],
  e_vol_range: ["Master volume must be 0-100", "ระดับเสียงต้องอยู่ระหว่าง 0-100"],
  e_port_range: ["Dashboard port must be 1024-65535", "พอร์ตแดชบอร์ดต้องอยู่ระหว่าง 1024-65535"],
  e_tikfinity_url: ["TikFinity URL must be ws://127.0.0.1:<port>", "URL ของ TikFinity ต้องเป็น ws://127.0.0.1:<พอร์ต>"],
  e_tosu_url: ["tosu URL must be ws://127.0.0.1:<port>/websocket/v2", "URL ของ tosu ต้องเป็น ws://127.0.0.1:<พอร์ต>/websocket/v2"],
  e_viewer_url: ["Viewer URL must start with https://", "URL ของ viewer ต้องขึ้นต้นด้วย https://"],
  e_game: ["Game must be stable or lazer", "เกมต้องเป็น stable หรือ lazer"],
  e_command_name: ["The command must start with ! and have no spaces", "คำสั่งต้องขึ้นต้นด้วย ! และไม่มีช่องว่าง"],
  e_mod_cmd: ["Mod command {cmd} must start with ! and have no spaces", "คำสั่งม็อด {cmd} ต้องขึ้นต้นด้วย ! และไม่มีช่องว่าง"],
  e_bare_number: ["A bare number must mean beatmap or set", "ตัวเลขเปล่าต้องเป็น beatmap หรือ set"],
  e_stars_order: ["Min stars is above max stars", "ดาวขั้นต่ำมากกว่าดาวสูงสุด"],
  e_play_mode: ["Play mode must be manual or auto", "โหมดเล่นต้องเป็น manual หรือ auto"],
  e_md5_level: ["MD5 level must be strict or requested", "ระดับ MD5 ต้องเป็น strict หรือ requested"],
  e_all_fail: ["\"If every source fails\" must be skip or warn", "\"ถ้าทุกแหล่งล้มเหลว\" ต้องเป็น skip หรือ warn"],
  e_import_mode: ["Unknown import mode", "ไม่รู้จักวิธีนำเข้านี้"],
  e_preload_count: ["Preload count must be 0-10", "จำนวนโหลดล่วงหน้าต้องอยู่ระหว่าง 0-10"],
  e_source_host: ["Source {name} uses a host that is not allowed", "แหล่ง {name} ใช้โดเมนที่ไม่อนุญาต"],
  e_source_timeout: ["Source {name}: timeout must be at least 5 seconds", "แหล่ง {name}: เวลาหมดต้องอย่างน้อย 5 วินาที"],
  ignored: ["Ignored: real chat would not treat this as a request", "ไม่ใช่คำขอ: แชทจริงจะไม่นับข้อความนี้"],
  confirmClearTest: ["Remove all test requests, their history, rejections and cooldowns?", "ลบคำขอทดสอบทั้งหมด รวมประวัติ การปฏิเสธ และคูลดาวน์ที่เกิดจากการทดสอบ?"],
  nowPlaying: ["Now playing", "กำลังเล่น"],
  nothingPlaying: ["Nothing playing. Press Next to open the first map in the queue.", "ยังไม่มีเพลงที่เล่นอยู่ กด ถัดไป เพื่อเปิดแมพแรกในคิว"],
  next: ["Next", "ถัดไป"],
  skip: ["Skip", "ข้าม"],
  by: ["by", "โดย"],
  mappedBy: ["mapped by", "แมพโดย"],
  reqBy: ["requested by", "ขอโดย"],
  emptyQueue: ["The queue is empty. Viewers type {cmd} <ID or link> in TikTok chat.", "คิวว่าง ผู้ชมพิมพ์ {cmd} <ID หรือลิงก์> ในแชท TikTok"],
  emptyQueueName: ["The queue is empty. Viewers type {cmd} <song name, ID or link> in TikTok chat.", "คิวว่าง ผู้ชมพิมพ์ {cmd} <ชื่อเพลง, ID หรือลิงก์> ในแชท TikTok"],
  r_search_none: ["no map with that name", "ไม่พบแมพชื่อนี้"], r_needs_gift: ["no gift sent", "ยังไม่ได้ส่งของขวัญ"],
  e_request_coins: ["Coins per request cannot be negative", "เหรียญต่อคำขอติดลบไม่ได้"], r_search_error: ["song search failed", "ค้นหาเพลงไม่ได้"],
  resolving: ["looking up {n} request(s)…", "กำลังค้นหา {n} คำขอ…"],
  st_queued: ["waiting", "รอดาวน์โหลด"],
  st_downloading: ["downloading", "กำลังดาวน์โหลด"],
  st_ready: ["ready", "พร้อมเล่น"],
  st_failed: ["failed", "ล้มเหลว"],
  wholeSet: ["whole set", "ทั้งเซ็ต"],
  test: ["test", "ทดสอบ"],
  tf_not_installed: ["TikFinity not installed", "ยังไม่ได้ติดตั้ง TikFinity"],
  tf_not_running: ["TikFinity not running", "TikFinity ไม่ได้เปิดอยู่"],
  tf_connecting: ["Connecting to TikFinity…", "กำลังเชื่อมต่อ TikFinity…"],
  tf_waiting_stream: ["Waiting for stream", "รอเริ่มไลฟ์"],
  tf_live: ["Live, taking requests", "ไลฟ์อยู่ รับคำขอ"],
  b_not_installed: ["TikFinity Desktop is required. Install it, log in, and connect it to your TikTok account.", "ต้องใช้ TikFinity Desktop ติดตั้ง ล็อกอิน แล้วเชื่อมกับบัญชี TikTok"],
  b_not_running: ["Start TikFinity Desktop and keep it running while you stream.", "เปิด TikFinity Desktop ไว้ตลอดระหว่างไลฟ์"],
  b_connecting: ["TikFinity is running but its WebSocket API did not answer. Check the URL in Settings → Connection.", "TikFinity เปิดอยู่แต่ WebSocket ไม่ตอบ ตรวจ URL ใน ตั้งค่า → การเชื่อมต่อ"],
  b_waiting_stream: ["Connected. Requests start as soon as TikTok events arrive.", "เชื่อมต่อแล้ว จะเริ่มรับคำขอเมื่อมีอีเวนต์จาก TikTok"],
  b_note: ["Settings, queue and test requests still work. Chat requests are paused.", "ยังตั้งค่า จัดคิว และทดสอบได้ตามปกติ แต่หยุดรับคำขอจากแชทชั่วคราว"],
  getTikfinity: ["Download TikFinity", "ดาวน์โหลด TikFinity"],
  launch: ["Launch TikFinity", "เปิด TikFinity"],
  reconnect: ["Reconnect", "เชื่อมต่อใหม่"],
  chk_installed: ["installed", "ติดตั้งแล้ว"], chk_running: ["running", "เปิดอยู่"], chk_ws: ["WebSocket", "WebSocket"], chk_hs: ["handshake", "แฮนด์เชค"], chk_live: ["live", "ไลฟ์"],
  version: ["Version", "เวอร์ชัน"], cache: ["Cache", "แคช"], folder: ["Data folder", "โฟลเดอร์ข้อมูล"], overlayUrl: ["OBS overlay", "โอเวอร์เลย์ OBS"], mode: ["Play mode", "โหมดเล่น"],
  saved: ["Saved", "บันทึกแล้ว"],
  restart: ["Saved. Restart the helper to use the new dashboard port.", "บันทึกแล้ว รีสตาร์ทโปรแกรมเพื่อใช้พอร์ตใหม่"],
  added: ["Added: {t}", "เพิ่มแล้ว: {t}"],
  confirmClear: ["Remove every request from the queue?", "ลบคำขอทั้งหมดในคิว?"],
  none: ["Nothing yet", "ยังไม่มี"],
  copied: ["Copied", "คัดลอกแล้ว"],
  offline: ["Lost connection to the helper. Is it still running?", "ขาดการเชื่อมต่อกับโปรแกรม ยังเปิดอยู่ไหม?"],
  played: ["played", "เล่นแล้ว"], skipped: ["skipped", "ข้าม"],
  // rejection codes
  r_user_blacklisted: ["user blacklisted", "ผู้ใช้ถูกแบน"], r_queue_full: ["queue full", "คิวเต็ม"], r_user_limit: ["too many requests", "ขอเกินจำนวน"],
  r_cooldown: ["cooldown", "ติดคูลดาวน์"], r_set_blacklisted: ["map blacklisted", "แมพถูกแบน"], r_mapper_blacklisted: ["mapper blacklisted", "แมพเปอร์ถูกแบน"],
  r_status: ["status not allowed", "สถานะไม่อนุญาต"], r_duplicate: ["already queued", "อยู่ในคิวแล้ว"], r_recently_played: ["played recently", "เพิ่งเล่นไป"],
  r_not_found: ["not found", "ไม่พบแมพ"], r_mode: ["mode not allowed", "โหมดไม่อนุญาต"], r_stars_low: ["too easy", "ดาวต่ำเกิน"], r_stars_high: ["too hard", "ดาวสูงเกิน"],
  rejectedToast: ["rejected", "ถูกปฏิเสธ"], r_no_convert: ["mode cannot be converted", "แปลงโหมดไม่ได้"],
  convert: ["convert", "convert"], min: ["min ★", "ต่ำสุด ★"], max: ["max ★", "สูงสุด ★"], global: ["= overall", "= ค่ารวม"],
  modeWarn: ["osu! is in {game} mode, but this map is {map}. Press {key} in song select to switch.", "osu! อยู่โหมด {game} แต่แมพนี้เป็น {map} กด {key} ที่หน้าเลือกเพลงเพื่อเปลี่ยนโหมด"],
  e_mode_unknown: ["Unknown mode {mode} in stars per mode", "ไม่รู้จักโหมด {mode} ในดาวแยกตามโหมด"],
  e_stars_negative: ["{mode}: stars cannot be negative", "{mode}: ดาวติดลบไม่ได้"],
  e_stars_order_mode: ["{mode}: min stars is above max stars", "{mode}: ดาวขั้นต่ำมากกว่าดาวสูงสุด"],
  r_now_playing: ["playing right now", "กำลังเล่นอยู่ตอนนี้"],
  k_info: ["info", "ข้อมูล"], k_warn: ["warning", "คำเตือน"], k_error: ["error", "ผิดพลาด"],
  n_followUnknown: ["Followers-only is on, but TikFinity did not say whether @{u} follows; the request was let through.", "เปิดเฉพาะผู้ติดตามอยู่ แต่ TikFinity ไม่ได้บอกว่า @{u} ติดตามหรือไม่ จึงปล่อยคำขอผ่าน"],
  n_finished: ["Finished: {title} (@{u})", "เล่นจบ: {title} (@{u})"],
  n_mismatchUsed: ["{title}: no source matched osu!'s MD5; using the latest download from {src}", "{title}: ไม่มีแหล่งไหนตรงกับ MD5 ของ osu! ใช้ไฟล์ล่าสุดจาก {src}"],
  n_failed: ["{title}: {msg}", "{title}: {msg}"],
  n_dryOpen: ["Dry run: would open {title}", "ทดลอง: จะเปิด {title}"],
  n_removedLast: ["Removed {title} (@{u})", "ลบ {title} (@{u}) แล้ว"],
  n_reqOn: ["Requests are on", "เปิดรับคำขอแล้ว"], n_reqOff: ["Requests are off", "ปิดรับคำขอแล้ว"],
  n_clearedTest: ["Test data cleared ({n} request(s))", "ล้างข้อมูลทดสอบแล้ว ({n} คำขอ)"],
  n_dryOn: ["Dry run on: Next and Play will not open osu!", "เปิดโหมดทดลองแล้ว: ถัดไป/เล่น จะไม่เปิด osu!"], n_dryOff: ["Dry run off", "ปิดโหมดทดลองแล้ว"],
  n_hotkeyErr: ["Hotkey {key}: {msg}", "ปุ่มลัด {key}: {msg}"],
  n_banned_user: ["Banned @{v} (removed {n} waiting request(s))", "แบน @{v} แล้ว (ลบคำขอที่รอ {n} รายการ)"],
  n_banned_set: ["Banned beatmapset {v} (removed {n})", "แบนเซ็ต {v} แล้ว (ลบ {n} รายการ)"],
  n_banned_mapper: ["Banned mapper {v} (removed {n})", "แบนแมพเปอร์ {v} แล้ว (ลบ {n} รายการ)"],
  n_skipErr: ["!skip by @{u}: {msg}", "!skip โดย @{u}: {msg}"],
  n_usedCmd: ["@{u} used {cmd}", "@{u} ใช้คำสั่ง {cmd}"],
  updAvail: ["Update {v} ↗", "อัปเดต {v} ↗"], updTitle: ["osureq {v} is out. Opens the release page; download the zip there and replace osureq.exe.", "osureq {v} ออกแล้ว กดเพื่อเปิดหน้าดาวน์โหลด โหลด zip แล้วแทนที่ osureq.exe"],
  updReady: ["Update {v} ready", "อัปเดต {v} พร้อมแล้ว"], updReadyTitle: ["osureq {v} is downloaded and is installed the next time osureq starts. Click to install it now (osureq restarts for a few seconds).", "ดาวน์โหลด osureq {v} แล้ว จะติดตั้งให้เองตอนเปิดโปรแกรมครั้งหน้า กดเพื่อติดตั้งเลย (osureq จะปิดแล้วเปิดใหม่ไม่กี่วินาที)"],
  confirmUpdate: ["Install osureq {v} now? osureq closes and starts again in a few seconds; the queue and settings are kept. Better not in the middle of a stream.", "ติดตั้ง osureq {v} เลยไหม? osureq จะปิดแล้วเปิดใหม่ในไม่กี่วินาที คิวและการตั้งค่ายังอยู่ ไม่ควรกดระหว่างไลฟ์"],
  n_update: ["osureq {v} is available (button at the top)", "มี osureq {v} แล้ว (ปุ่มด้านบน)"],
  n_updateReady: ["osureq {v} is downloaded; it is installed the next time osureq starts (or press the button at the top)", "ดาวน์โหลด osureq {v} แล้ว จะติดตั้งตอนเปิดโปรแกรมครั้งหน้า (หรือกดปุ่มด้านบน)"],
  n_updateFailed: ["Could not download osureq {v}: {err}", "ดาวน์โหลด osureq {v} ไม่สำเร็จ: {err}"],
  n_installUpdate: ["Installing osureq {v} and restarting", "กำลังติดตั้ง osureq {v} แล้วเปิดใหม่"],
  session: ["This stream", "ไลฟ์นี้"], sumShow: ["Show on overlay", "แสดงบน overlay"], sumNew: ["New session", "เริ่มรอบใหม่"],
  sumStarted: ["Started", "เริ่ม"], sumPlayed: ["Played", "เล่นแล้ว"], sumPassed: ["{n} cleared", "ผ่าน {n}"], sumSkipped: ["Skipped", "ข้าม"],
  sumRejected: ["Rejected", "ถูกปฏิเสธ"], sumViewers: ["Viewers who asked", "ผู้ชมที่ขอเพลง"], sumBest: ["Best play", "ผลดีที่สุด"], sumTop: ["Most requests", "ขอมากที่สุด"],
  sumShown: ["The summary is on the overlay for 20 s", "แสดงสรุปบน overlay 20 วินาที"],
  confirmNewSession: ["Start a new session? The summary starts counting from zero (history is kept).", "เริ่มรอบใหม่? สรุปจะเริ่มนับจากศูนย์ (ประวัติยังอยู่)"],
  n_newSession: ["Started a new stream session", "เริ่มรอบไลฟ์ใหม่แล้ว"],
  pfRun: ["Pre-stream check", "เช็กก่อนไลฟ์"], pfRunning: ["Checking…", "กำลังเช็ก…"], pfAllOk: ["All set. Have a good stream!", "พร้อมแล้ว ไลฟ์ให้สนุกนะ!"],
  pfIssues: ["{n} thing(s) to look at", "มี {n} เรื่องที่ควรดู"], pfFailed: ["The check could not run", "เช็กไม่สำเร็จ"],
  pf_tf_live: ["TikFinity: connected and live", "TikFinity: เชื่อมต่อแล้วและกำลังไลฟ์"],
  pf_tf_ready: ["TikFinity: connected, waiting for the stream to start", "TikFinity: เชื่อมต่อแล้ว รอเริ่มไลฟ์"],
  pf_tf_not_installed: ["TikFinity is not installed", "ยังไม่ได้ติดตั้ง TikFinity"], pf_tf_not_running: ["TikFinity is not running", "TikFinity ไม่ได้เปิดอยู่"],
  pf_tf_connecting: ["TikFinity is running but its WebSocket does not answer (Settings → Connection)", "TikFinity เปิดอยู่แต่ WebSocket ไม่ตอบ (ตั้งค่า → การเชื่อมต่อ)"],
  pf_tosu_ok: ["tosu: sees osu! {client}", "tosu: เห็น osu! {client}"], pf_tosu_no_game: ["tosu is running, but osu! is not open", "tosu เปิดอยู่ แต่ยังไม่ได้เปิด osu!"],
  pf_tosu_not_running: ["tosu is not running: auto mode and results need it", "tosu ไม่ได้เปิดอยู่: โหมดอัตโนมัติและผลการเล่นต้องใช้"],
  pf_tosu_connecting: ["tosu is running but its WebSocket does not answer (Settings → Connection)", "tosu เปิดอยู่แต่ WebSocket ไม่ตอบ (ตั้งค่า → การเชื่อมต่อ)"],
  pf_game_lazer: ["osu! lazer: maps are opened directly, no Songs folder needed", "osu! lazer: เปิดแมพโดยตรง ไม่ต้องใช้โฟลเดอร์ Songs"],
  pf_songs_ok: ["Songs folder: {dir}", "โฟลเดอร์ Songs: {dir}"],
  pf_songs_fail: ["No Songs folder in {dir}: set Settings → osu! → osu! folder", "ไม่พบโฟลเดอร์ Songs ใน {dir}: ตั้งที่ ตั้งค่า → osu! → โฟลเดอร์ osu!"],
  pf_hotkeys_off: ["Global hotkeys are off", "ปิดปุ่มลัดทั้งระบบอยู่"], pf_hotkeys_ok: ["Global hotkeys are registered", "ลงทะเบียนปุ่มลัดแล้ว"],
  pf_hotkeys_fail: ["Hotkeys could not be registered: {keys} (another app may use them)", "ลงทะเบียนปุ่มลัดไม่ได้: {keys} (อาจมีโปรแกรมอื่นใช้อยู่)"],
  pf_overlay_ok: ["Overlay is open ({n})", "เปิด overlay อยู่ ({n})"],
  pf_overlay_none: ["No overlay connected: add the Browser Source in OBS or LIVE Studio", "ยังไม่มี overlay เชื่อมต่อ: เพิ่ม Browser Source ใน OBS หรือ LIVE Studio"],
  pf_requests_off: ["Requests are off", "ปิดรับคำขออยู่"], pf_dryrun_on: ["Dry run is on: Next will not open osu!", "เปิดโหมดทดลองอยู่: กด ถัดไป จะไม่เปิด osu!"],
  pf_testdata: ["{n} test request(s) still in the queue: Clear test data", "ยังมีคำขอทดสอบ {n} รายการในคิว: กด ล้างข้อมูลทดสอบ"],
  pf_update: ["osureq {v} is available", "มี osureq {v} แล้ว"],
  pf_osuweb_ok: ["osu! website answers (map info and MD5)", "เว็บ osu! ตอบปกติ (ข้อมูลแมพและ MD5)"],
  pf_osuweb_fail: ["osu! website did not answer: requests cannot be checked", "เว็บ osu! ไม่ตอบ: ตรวจคำขอไม่ได้"],
  pf_mirror_ok: ["Mirror {src} answers ({ms} ms)", "mirror {src} ตอบปกติ ({ms} ms)"], pf_mirror_fail: ["Mirror {src} failed", "mirror {src} ใช้ไม่ได้"],
  pf_mirror_none: ["No download mirror is enabled (Settings → Download)", "ไม่ได้เปิด mirror สำหรับดาวน์โหลดเลย (ตั้งค่า → ดาวน์โหลด)"],
  supportFile: ["Save support file", "บันทึกไฟล์ช่วยแก้ปัญหา"],
  supportHint: ["After a stream with a problem, save this file (log, raw TikFinity events, settings) and send it to whoever helps you. It has viewers' names from chat.", "ถ้าไลฟ์มีปัญหา กดบันทึกไฟล์นี้ (log, อีเวนต์ดิบจาก TikFinity, การตั้งค่า) แล้วส่งให้คนที่ช่วยแก้ ในไฟล์มีชื่อผู้ชมจากแชทด้วย"],
  supportSaved: ["Support file saved (see your Downloads)", "บันทึกไฟล์แล้ว (ดูในโฟลเดอร์ดาวน์โหลด)"],
  n_supportFile: ["Support file saved", "บันทึกไฟล์ช่วยแก้ปัญหาแล้ว"],
  n_autoClosed: ["Requests closed by themselves: {n} waiting", "ปิดรับคำขออัตโนมัติ: รออยู่ {n} เพลง"],
  n_autoReopened: ["Requests open again: {n} waiting", "เปิดรับคำขออีกครั้ง: รออยู่ {n} เพลง"],
  n_timeClosed: ["Requests closed: it is {t}", "ปิดรับคำขอแล้ว: ถึงเวลา {t}"],
  e_auto_close: ["Auto-close: reopen must be below the close size", "ปิดอัตโนมัติ: จำนวนที่เปิดใหม่ต้องน้อยกว่าจำนวนที่ปิด"],
  e_close_at: ["Close time must be HH:MM, e.g. 23:30", "เวลาปิดต้องเป็น HH:MM เช่น 23:30"],
  modPanel: ["Mod panel", "แผงม็อด"], modOff: ["off", "ปิด"], modSessions: ["{n} signed in", "เข้าใช้อยู่ {n}"],
  modUrls: ["Open on a phone on the same Wi-Fi", "เปิดบนมือถือที่ใช้ Wi-Fi เดียวกัน"], modNoUrl: ["Turn it on and save to see the address.", "เปิดแล้วกดบันทึกเพื่อดูที่อยู่"],
  modNoLan: ["No home network address found on this PC.", "ไม่พบที่อยู่วงแลนของเครื่องนี้"],
  genPin: ["New PIN", "สุ่ม PIN ใหม่"],
  e_remote_port: ["Mod panel port must be 1024-65535 and not the dashboard port", "พอร์ตแผงม็อดต้องอยู่ระหว่าง 1024-65535 และไม่ซ้ำกับพอร์ตแดชบอร์ด"],
  e_remote_pin: ["Mod panel PIN must be 6-12 digits, not all the same and not 123456-style", "PIN แผงม็อดต้องเป็นตัวเลข 6-12 หลัก ไม่ซ้ำกันทั้งหมด และไม่ใช่แบบ 123456"],
  n_modLogin: ["Mod panel: signed in from {ip}", "แผงม็อด: มีคนเข้าใช้จาก {ip}"],
  n_modAction: ["Mod panel ({ip}): {action}", "แผงม็อด ({ip}): {action}"],
  modsTitle: ["Mods the viewer asked for. Select them in osu! yourself; the stars shown are without mods.", "mod ที่ผู้ชมขอ เลือกใน osu! เอง ดาวที่แสดงเป็นแบบไม่มี mod"],
  ph_missing: ["played without {m}", "เล่นโดยไม่ได้ใส่ {m}"],
  n_modsMissing: ["{title} was played without {missing} (asked +{mods})", "{title} เล่นโดยไม่ได้ใส่ {missing} (ขอ +{mods})"],
  r_bad_mods: ["unknown or clashing mods", "mod ไม่ถูกต้องหรือใช้ด้วยกันไม่ได้"], r_mods_off: ["mods not allowed", "ไม่รับคำขอที่มี mod"],
  r_mod_not_allowed: ["mod not allowed", "mod นี้ไม่อนุญาต"], r_mod_stars: ["too hard with that mod", "ยากเกินไปเมื่อใส่ mod นี้"],
  e_hard_mod_stars: ["The star limit for DT/HR cannot be negative", "ดาวสูงสุดสำหรับ DT/HR ติดลบไม่ได้"], e_banned_mod: ["{mod} is not a mod", "{mod} ไม่ใช่ mod"],
  tab_tts: ["TTS", "เสียงอ่าน"],
  tplHelp: ["Placeholders: {user} {title} {artist} {mapper} {diff} {mods} {stars} {pos} {setid} ({setid_digits} reads it digit by digit); !np also {requester}; rejections also {reason} {max} {min} {length} {wait}; results {acc} {rank} {combo} {misses} {pp}; gifts {coins} {gift}. Empty or - = say nothing. A rejection without its own line uses \"Rejected (any other reason)\". Try them with Simulate chat in the Test panel.", "ตัวแปร: {user} {title} {artist} {mapper} {diff} {mods} {stars} {pos} {setid} ({setid_digits} อ่านทีละตัวเลข) !np มี {requester} การปฏิเสธมี {reason} {max} {min} {length} {wait} ผลการเล่นมี {acc} {rank} {combo} {misses} {pp} ของขวัญมี {coins} {gift} เว้นว่างหรือใส่ - = ไม่พูด การปฏิเสธที่ไม่มีบรรทัดของตัวเองจะใช้ \"ถูกปฏิเสธ (เหตุผลอื่น)\" ลองได้ด้วย จำลองแชท ในแผงทดสอบ"],
  tplThai: ["Thai examples", "ตัวอย่างภาษาไทย"], tplEnglish: ["English examples", "ตัวอย่างภาษาอังกฤษ"], tplSilent: ["(say nothing)", "(ไม่พูด)"],
  tpl_added: ["Request added", "ขอเพลงสำเร็จ"], tpl_rejected: ["Rejected (any other reason)", "ถูกปฏิเสธ (เหตุผลอื่น)"],
  tpl_rejected_stars_high: ["Rejected: too hard", "ถูกปฏิเสธ: ยากเกินไป"], tpl_rejected_stars_low: ["Rejected: too easy", "ถูกปฏิเสธ: ง่ายเกินไป"],
  tpl_rejected_mod_stars: ["Rejected: too hard with DT/HR", "ถูกปฏิเสธ: ยากเกินไปเมื่อใส่ DT/HR"], tpl_rejected_too_long: ["Rejected: too long", "ถูกปฏิเสธ: ยาวเกินไป"],
  tpl_rejected_duplicate: ["Rejected: already in the queue", "ถูกปฏิเสธ: อยู่ในคิวแล้ว"], tpl_rejected_recently_played: ["Rejected: played recently", "ถูกปฏิเสธ: เพิ่งเล่นไป"],
  tpl_rejected_cooldown: ["Rejected: cooldown", "ถูกปฏิเสธ: ติดคูลดาวน์"], tpl_rejected_user_limit: ["Rejected: too many requests", "ถูกปฏิเสธ: ขอเกินจำนวน"],
  tpl_rejected_search_none: ["Rejected: no map with that name", "ถูกปฏิเสธ: ไม่พบแมพชื่อนี้"],
  tpl_nowplaying: ["Next map opens", "เปิดเพลงถัดไป"], tpl_np: ["!np (what is playing)", "!np (เพลงที่กำลังเล่น)"], tpl_np_none: ["!np with nothing playing", "!np ตอนยังไม่มีเพลง"], tpl_pos: ["!pos (queue position)", "!pos (ลำดับคิว)"], tpl_result_passed: ["Result: cleared", "ผลการเล่น: ผ่าน"], tpl_result_failed: ["Result: failed", "ผลการเล่น: ไม่ผ่าน"],
  tpl_gift: ["Gift", "ของขวัญ"], tpl_voteskip: ["Vote skip passed", "โหวตข้ามเพลงสำเร็จ"], tpl_auto_off: ["Requests closed (queue full or time)", "ปิดรับคำขอ (คิวเต็มหรือถึงเวลา)"], tpl_auto_on: ["Requests open again", "เปิดรับคำขออีกครั้ง"],
  ttsStart: ["Start TTS", "เริ่มเสียงอ่าน"], ttsOn: ["TTS on", "เสียงอ่านเปิดอยู่"],
  ttsTitleOff: ["Browsers only speak after a click: click to start TTS (chat and announcements) in this tab.", "เบราว์เซอร์จะพูดได้หลังคลิกเท่านั้น: คลิกเพื่อเริ่มเสียงอ่าน (แชทและประกาศ) ในแท็บนี้"],
  ttsTitleOn: ["TTS is speaking in this tab (commands in chat are skipped). Click to stop.", "กำลังใช้เสียงอ่านในแท็บนี้ (ไม่อ่านคำสั่งในแชท) คลิกเพื่อหยุด"],
  sg_chat: ["Reading chat", "อ่านแชท"], sg_voice: ["Voice", "เสียง"], sg_announce: ["Announcements", "ประกาศเหตุการณ์"],
  modOn: ["on", "เปิดอยู่"], ttsSkip: ["Skip this message", "ข้ามข้อความนี้"],
  ttsNoVoice: ["This browser has no voice for {lang}. Set Settings → TTS → Who speaks to auto or Google, or open the dashboard in Microsoft Edge.", "เบราว์เซอร์นี้ไม่มีเสียงภาษา{lang} ตั้ง การตั้งค่า → TTS → ใครพูด เป็น auto หรือ Google หรือเปิดแดชบอร์ดใน Microsoft Edge"],
  ttsOnlineFail: ["Google's voice did not answer ({err}).", "เสียงของ Google ไม่ตอบ ({err})"],
  ttsSays_en: ["{name} says {text}", "{name} says {text}"], ttsSays_th: ["{name} บอกว่า {text}", "{name} บอกว่า {text}"],
  ttsSample: ["This is how chat will sound.", "เสียงอ่านแชทจะเป็นแบบนี้"], ttsAuto: ["best for the language", "เลือกให้ตามภาษา"],
  e_tts_lang: ["TTS language must be auto, Thai or English", "ภาษาอ่านแชทต้องเป็น auto ไทย หรืออังกฤษ"],
  e_tts_range: ["TTS: speed 0.5-2, volume 0-100, max length 10-500", "อ่านแชท: ความเร็ว 0.5-2 ระดับเสียง 0-100 ความยาว 10-500"],
  n_ttsSubUnknown: ["TTS: TikFinity did not say whether viewers are subscribers (ใจส้ม), so only your other groups are read", "TTS: TikFinity ไม่ได้บอกว่าผู้ชมเป็นสมาชิก (ใจส้ม) หรือไม่ จึงอ่านเฉพาะกลุ่มอื่นที่เลือกไว้"],
  n_ttsFollowUnknown: ["TTS: TikFinity did not say whether viewers follow, so only your other groups are read", "TTS: TikFinity ไม่ได้บอกว่าผู้ชมกดติดตามหรือไม่ จึงอ่านเฉพาะกลุ่มอื่นที่เลือกไว้"],
  e_tts_template: ["Unknown announcement {event}", "ไม่รู้จักประกาศ {event}"], e_tts_template_long: ["Announcement {event} is longer than 300 letters", "ประกาศ {event} ยาวเกิน 300 ตัวอักษร"],
  e_tts_allow: ["Unknown TTS group {who}", "ไม่รู้จักกลุ่ม {who} ของเสียงอ่าน"],
  n_gameDetected: ["tosu sees osu! {game} running, so Settings → osu! → Game is now {game}", "tosu เห็นว่าเปิด osu! {game} อยู่ จึงเปลี่ยน ตั้งค่า → osu! → เกม เป็น {game} ให้"],
  n_voteSkipped: ["{n} viewers voted to skip {title}", "ผู้ชม {n} คนโหวตข้าม {title}"],
  e_vote_skip: ["Votes to skip cannot be negative", "จำนวนโหวตข้ามเพลงติดลบไม่ได้"],
  ph_votes: ["skip votes {v}/{n}", "โหวตข้าม {v}/{n}"],
  n_giftMoved: ["@{u}'s gift moved {title} up the queue", "ของขวัญจาก @{u} ทำให้ {title} ขึ้นคิวก่อน"],
  n_cacheCleanup: ["Cache cleanup removed {n} file(s)", "ล้างแคชแล้ว ลบ {n} ไฟล์"],
  n_quit: ["Quit from the dashboard", "สั่งปิดจากแดชบอร์ด"],
  n_quitAll: ["Closed everything from the dashboard", "สั่งปิดทั้งหมดจากแดชบอร์ด"],
  n_closed: ["Closed {app}", "ปิด {app} แล้ว"],
  n_closeNone: ["{app} was not running", "{app} ไม่ได้เปิดอยู่"],
  n_closeFailed: ["Could not close {app}: {err}", "ปิด {app} ไม่ได้: {err}"],
  tosu_not_running: ["tosu not running", "tosu ไม่ได้เปิดอยู่"],
  tosu_connecting: ["Connecting to tosu…", "กำลังเชื่อมต่อ tosu…"],
  tosu_no_game: ["tosu: osu! not detected", "tosu: ไม่พบ osu!"],
  tosu_ok: ["tosu", "tosu"],
  tb_title: ["tosu is required", "ต้องใช้ tosu"],
  tb_not_running: ["Start tosu. The helper reads osu!'s game state from it to know when a requested song has ended (auto mode).", "เปิด tosu ไว้ โปรแกรมใช้ tosu อ่านสถานะเกม osu! เพื่อรู้ว่าเพลงที่ขอเล่นจบแล้ว (โหมดอัตโนมัติ)"],
  tb_connecting: ["tosu is running but its WebSocket did not answer. Check the tosu URL in Settings → Connection (default ws://127.0.0.1:24050/websocket/v2).", "tosu เปิดอยู่แต่ WebSocket ไม่ตอบ ตรวจ URL ของ tosu ใน ตั้งค่า → การเชื่อมต่อ (ค่าเริ่มต้น ws://127.0.0.1:24050/websocket/v2)"],
  tb_no_game: ["tosu is connected but does not see osu!. Start osu!.", "เชื่อมต่อ tosu แล้วแต่ยังไม่เห็น osu! ให้เปิด osu!"],
  tb_note: ["Until then auto mode is paused; Next still works.", "ระหว่างนี้โหมดอัตโนมัติจะหยุดรอ แต่ยังกด ถัดไป เองได้"],
  getTosu: ["Download tosu", "ดาวน์โหลด tosu"],
  launchTosu: ["Launch tosu", "เปิด tosu"],
  chk_osu: ["osu!", "osu!"],
  res_passed: ["cleared", "ผ่าน"], res_failed: ["failed", "ตาย"], res_quit: ["quit", "ออกกลางเพลง"],
  ph_waiting: ["waiting for you to play it", "รอเริ่มเล่นแมพนี้"],
  ph_playing: ["playing", "กำลังเล่น"],
  ph_results: ["results screen", "หน้าผลคะแนน"],
  ph_ended: ["finished", "เล่นจบแล้ว"],
  ph_nextIn: ["next map in {n}s", "แมพถัดไปใน {n} วิ"],
  ph_pressNext: ["press Next", "กด ถัดไป"],
  ph_noTosu: ["auto mode paused: tosu not connected", "โหมดอัตโนมัติหยุดรอ: ยังไม่เชื่อมต่อ tosu"],
  ph_queueEmpty: ["queue is empty", "คิวว่าง"],
  r_too_long: ["too long", "ยาวเกิน"], r_meta_error: ["osu! lookup failed", "ดึงข้อมูล osu! ไม่ได้"], r_bad_input: ["invalid input", "รูปแบบไม่ถูกต้อง"], r_requests_off: ["requests off", "ปิดรับคำขอ"], r_followers_only: ["not a follower", "ไม่ได้ติดตาม"],
  // settings tabs
  tab_connection: ["Connection", "การเชื่อมต่อ"], tab_osu: ["osu!", "osu!"], tab_command: ["Command", "คำสั่ง"], tab_rules: ["Map rules", "กฎแมพ"],
  tab_remote: ["Mod panel", "แผงม็อด"], tab_queue: ["Queue", "คิว"], tab_download: ["Download", "ดาวน์โหลด"], tab_md5: ["MD5", "MD5"], tab_import: ["Import", "นำเข้า"], tab_overlay: ["Overlay", "โอเวอร์เลย์"], tab_hotkeys: ["Hotkeys", "ปุ่มลัด"],
};
let LANG = (navigator.language || "").startsWith("th") ? "th" : "en", langPicked = false;
try { const l = localStorage.getItem("osureq.lang"); if (l) { LANG = l; langPicked = true; } } catch {}
const t = (k, vars) => {
  const row = T[k];
  let s = row ? row[LANG === "th" ? 1 : 0] : k;
  if (vars) for (const [a, b] of Object.entries(vars)) s = s.replaceAll("{" + a + "}", b);
  return s;
};
const L = (en, th) => LANG === "th" ? th : en;
function applyLang() {
  document.documentElement.lang = LANG;
  document.querySelectorAll("[data-t]").forEach(e => e.textContent = t(e.dataset.t));
  document.querySelectorAll("[data-tp]").forEach(e => e.placeholder = t(e.dataset.tp));
  $("#langSel").value = LANG;
}

/* ---------- helpers ---------- */
async function api(method, path, body) {
  const r = await fetch(path, { method, headers: { "X-Osureq": "1", "Content-Type": "application/json" }, body: body === undefined ? undefined : JSON.stringify(body) });
  let j = {};
  try { j = await r.json(); } catch {}
  if (!r.ok && j.ok === undefined) j = { ok: false, error: "HTTP " + r.status };
  return j;
}
let toastT;
function toast(msg, bad) {
  const e = $("#toast");
  e.textContent = msg; e.classList.toggle("bad", !!bad); e.classList.add("on");
  clearTimeout(toastT); toastT = setTimeout(() => e.classList.remove("on"), bad ? 4200 : 2400);
}
// errText shows a coded error in the dashboard's language, else the English text.
const errText = j => j.code && T["e_" + j.code] ? t("e_" + j.code, j.vars || {}) : (j.error || "error");
async function act(path, body) {
  const j = await api("POST", path, body);
  if (j.ok === false) toast(errText(j), true);
  return j;
}
// Star colour ramp, same as the beatmap viewer.
function starColor(s) {
  if (s == null || s < 0) return "#8a7fa3";
  const stops = [[0, "#4290fb"], [2, "#4fc0ff"], [2.5, "#4fffd5"], [3.3, "#7cff4f"], [4.2, "#f6f05c"], [4.9, "#ff8068"], [5.8, "#ff4e6f"], [6.7, "#c645b8"], [7.7, "#6563de"], [9, "#18158e"]];
  if (s >= 9) return "#000";
  for (let i = 1; i < stops.length; i++) if (s <= stops[i][0]) {
    const [a, ca] = stops[i - 1], [b, cb] = stops[i], f = (s - a) / (b - a);
    const p = c => [1, 3, 5].map(j => parseInt(c.slice(j, j + 2), 16));
    const x = p(ca), y = p(cb);
    return "rgb(" + x.map((v, j) => Math.round(v + (y[j] - v) * f)).join(",") + ")";
  }
  return "#18158e";
}
const clock = s => s == null || s < 0 ? "?" : Math.floor(s / 60) + ":" + String(s % 60).padStart(2, "0");
const ago = iso => { const d = (Date.now() - new Date(iso)) / 1000; if (d < 60) return Math.max(0, Math.floor(d)) + "s"; if (d < 3600) return Math.floor(d / 60) + "m"; return Math.floor(d / 3600) + "h"; };
const timeOf = iso => new Date(iso).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
const mb = n => n >= 1 << 20 ? (n / (1 << 20)).toFixed(1) + " MB" : Math.round(n / 1024) + " KB";
const MODE = { osu: "osu!", taiko: "taiko", fruits: "catch", mania: "mania" };
const MODE_KEY = { osu: 1, taiko: 2, fruits: 3, mania: 4 }; // Ctrl+N switches mode in osu!
// modeText: "osu!", "taiko", "catch", "mania 7K", with "· convert" for a converted osu! map.
const modeText = it => (MODE[it.mode] || it.mode) + (it.mode === "mania" && it.keys ? " " + it.keys + "K" : "") + (it.convert ? " · " + t("convert") : "");
const coverOf = it => it.cover || `https://assets.ppy.sh/beatmaps/${it.setId}/covers/cover.jpg`;
const setLink = it => `https://osu.ppy.sh/beatmapsets/${it.setId}` + (it.beatmapId ? `#${it.mode || "osu"}/${it.beatmapId}` : "");

/* ---------- state & rendering ---------- */
let S = null;
function render() {
  if (!S) return;
  if (!langPicked && S.lang === "th" && LANG !== "th") { LANG = "th"; langPicked = true; applyLang(); }
  const tf = S.tikfinity;
  $("#tfDot").className = "dot " + tf.level;
  $("#tfText").textContent = t("tf_" + tf.code);
  $("#tfPill").title = tf.error || "";
  $("#logo").classList.toggle("beat", tf.level === "green" && S.requests);
  $("#reqSw").checked = S.requests;
  if (!volEditing) showVolume(S.masterVolume ?? 10);
  $("#dryRunSw").checked = S.dryRun;
  $("#dryPill").hidden = !S.dryRun;
  renderTTS();
  const up = $("#updPill");
  up.hidden = !S.update;
  if (S.update) {
    const u = S.update;
    up.href = u.ready ? "#" : u.url;
    up.target = u.ready ? "" : "_blank";
    up.textContent = t(u.ready ? "updReady" : "updAvail", { v: u.version });
    up.title = t(u.ready ? "updReadyTitle" : "updTitle", { v: u.version });
  }
  renderTestHint();
  const ts = S.tosu;
  $("#tosuDot").className = "dot " + ts.level;
  $("#tosuText").textContent = t("tosu_" + ts.code);
  $("#tosuPill").title = ts.error || (ts.game && ts.game.state ? `${ts.game.client} · ${ts.game.state}` : "");
  renderStatus(tf, ts);
  for (const [sel, running, app] of [["#stopTosuBtn", ts.running, "tosu"], ["#stopTfBtn", tf.running, "TikFinity"]]) {
    $(sel).disabled = !running;
    $(sel).title = running ? "" : t("notRunning", { app });
  }
  renderNow(S.queue.now);
  updatePhase();
  renderQueue(S.queue.items || []);
  renderSide();
}

// same() remembers what an element was last drawn from, so the 4-per-second
// progress pushes during a download don't rebuild (and re-animate) it.
function same(el, key) {
  if (el.dataset.key === key) return true;
  el.dataset.key = key;
  return false;
}

// renderStatus shows TikFinity and tosu problems as compact rows in one box
// (nothing when both are green). Details (hint, checklist, error) fold away.
let statusOpen = true;
try { statusOpen = localStorage.getItem("osureq.statusOpen") !== "0"; } catch {}
function renderStatus(tf, ts) {
  const b = $("#banner");
  const { lastEvent, ...shownTf } = tf;
  if (same(b, JSON.stringify([shownTf, ts.level, ts.code, ts.running, ts.connected, !!ts.path, ts.error, LANG, statusOpen]))) return;
  b.replaceChildren();
  const chk = (ok, k) => h("em", { class: ok ? "ok" : "no" }, (ok ? "✓ " : "✕ ") + t(k));
  const button = (main, label, path) => h("button", { class: "btn sm" + (main ? " main" : ""), type: "button", onclick: () => act(path) }, label);
  const row = (level, title, hint, error, chips, acts) => h("div", { class: "srow" },
    h("i", { class: "dot " + level }),
    h("div", { class: "stext" }, h("b", {}, title),
      statusOpen ? [h("small", { class: "shint" }, hint), error ? h("small", { class: "serr", title: error }, error) : null, h("div", { class: "steps" }, chips)] : []),
    h("div", { class: "sacts" }, acts));
  const rows = [];
  if (tf.level !== "green") {
    const acts = [];
    if (!tf.installed) acts.push(button(true, t("getTikfinity"), "/api/action/openTikfinityDownload"));
    else if (!tf.running) acts.push(button(true, t("launch"), "/api/action/launchTikfinity"));
    if (tf.installed) acts.push(button(false, t("reconnect"), "/api/action/reconnect"));
    rows.push(row(tf.level, t("tf_" + tf.code), t("b_" + tf.code) + " " + t("b_note"), tf.error,
      [chk(tf.installed, "chk_installed"), chk(tf.running, "chk_running"), chk(tf.connected, "chk_ws"), chk(tf.handshake, "chk_hs"), chk(tf.live, "chk_live")], acts));
  }
  if (ts.level !== "green") {
    const acts = [];
    if (!ts.running && !ts.path) acts.push(button(true, t("getTosu"), "/api/action/openTosuDownload"));
    if (!ts.running && ts.path) acts.push(button(true, t("launchTosu"), "/api/action/launchTosu"));
    acts.push(button(false, t("reconnect"), "/api/action/reconnectTosu"));
    rows.push(row(ts.level, t("tosu_" + ts.code) + " · " + t("required"), t("tb_" + ts.code) + " " + t("tb_note"), ts.error,
      [chk(ts.running, "chk_running"), chk(ts.connected, "chk_ws"), chk(ts.code === "ok", "chk_osu")], acts));
  }
  if (!rows.length) return;
  const red = tf.level === "red" || ts.level === "red";
  b.append(h("section", { class: "status " + (red ? "red" : "yellow"), "aria-label": t("setupTitle") },
    h("div", { class: "status-h" }, h("span", {}, t("setupTitle")),
      h("button", { class: "mlink", type: "button", "aria-expanded": String(statusOpen), onclick: () => {
        statusOpen = !statusOpen;
        try { localStorage.setItem("osureq.statusOpen", statusOpen ? "1" : "0"); } catch {}
        render();
      } }, statusOpen ? t("hideDetails") + " ▴" : t("showDetails") + " ▾")),
    rows));
}

function tagsFor(it) {
  return h("div", { class: "tags" },
    h("em", { class: "star", style: `background:${starColor(it.stars)};color:${it.stars >= 6.7 ? "#fff" : "#1c1726"}` }, it.stars >= 0 ? it.stars.toFixed(2) + "★" : "?★"),
    h("em", { class: "mode mode-" + it.mode }, modeText(it)),
    it.mods ? h("em", { class: "mods", title: t("modsTitle") }, "+" + it.mods) : null,
    h("em", {}, clock(it.length)),
    it.status ? h("em", { class: "st-" + it.status }, it.status) : null,
    it.md5 ? h("em", { class: "md5-" + it.md5 }, "MD5 " + it.md5) : null,
    it.source ? h("em", {}, it.source) : null,
    it.bySet ? h("em", {}, t("wholeSet")) : null,
    it.priority ? h("em", { class: "prio" }, "★ " + t("priority")) : null,
    it.test ? h("em", {}, t("test")) : null);
}



/* ---------- audio preview & beatmap viewer ---------- */
const preview = new Audio();
let previewSet = 0;
preview.volume = 0.1; // replaced by the master volume (default 10%) on the first state push
function syncPreview() {
  document.querySelectorAll("[data-pv]").forEach(b => {
    const on = +b.dataset.pv === previewSet && !preview.paused;
    b.textContent = on ? "❚❚ " + t("pvStop") : "♪ " + t("pvPlay");
    b.classList.toggle("on", on);
  });
  document.querySelectorAll(".card[data-set]").forEach(c => c.classList.toggle("pv-on", +c.dataset.set === previewSet && !preview.paused));
}
["play", "pause", "ended"].forEach(ev => preview.addEventListener(ev, syncPreview));
function togglePreview(setId) {
  if (previewSet === setId && !preview.paused) { preview.pause(); return; }
  if (previewSet !== setId) { preview.src = `https://b.ppy.sh/preview/${setId}.mp3`; previewSet = setId; }
  preview.play().catch(() => toast(t("pvFail"), true));
}
// mapLinks: " · ♪ preview · viewer ↗" after the osu! link (viewer hidden when no URL is set).
function mapLinks(it) {
  const pv = h("button", { class: "mlink pv", type: "button", "data-pv": it.setId, onclick: e => { e.stopPropagation(); togglePreview(it.setId); } });
  const out = [" · ", pv];
  if (S && S.viewerUrl) {
    const u = new URL(S.viewerUrl);
    u.searchParams.set("s", it.setId);
    if (it.beatmapId) u.searchParams.set("b", it.beatmapId);
    out.push(" · ", h("a", { class: "mlink", href: u.toString(), target: "_blank", rel: "noopener", title: t("viewerTitle") }, t("viewer") + " ↗"));
  }
  queueMicrotask(syncPreview);
  return out;
}

function viewerHref(it) {
  if (!S || !S.viewerUrl) return "";
  const u = new URL(S.viewerUrl);
  u.searchParams.set("s", it.setId);
  if (it.beatmapId) u.searchParams.set("b", it.beatmapId);
  return u.toString();
}

// resultShort mirrors app.ResultShort: "98.52% S · 812x · 2 miss · 301pp · HDHR".
function resultShort(r) {
  if (!r) return "";
  const p = [r.accuracy.toFixed(2) + "%" + (r.rank && r.outcome === "passed" ? " " + r.rank : ""), r.maxCombo + "x"];
  if (r.misses > 0) p.push(r.misses + " miss");
  if (r.pp >= 1 && r.outcome === "passed") p.push(Math.round(r.pp) + "pp");
  if (r.mods && r.mods !== "NM") p.push(r.mods);
  return p.join(" · ");
}

// The phase line under now playing: what tosu has seen of the map, and in
// auto mode when the next one opens. Updated every second without a rebuild.
// updateModeWarn: tosu says which mode osu! is in; if the map just opened is
// for another mode, tell the streamer how to switch (Ctrl+1-4 in song select).
function updateModeWarn() {
  const el = $("#modeWarn");
  if (!el || !S || !S.queue.now) return;
  const n = S.queue.now, g = S.tosu && S.tosu.connected && S.tosu.game ? S.tosu.game.mode : "";
  const show = !!g && !!n.mode && g !== n.mode && (n.playPhase || "waiting") === "waiting";
  el.hidden = !show;
  if (show) el.textContent = "⚠ " + t("modeWarn", { game: MODE[g] || g, map: modeText(n), key: "Ctrl+" + MODE_KEY[n.mode] });
}

function updatePhase() {
  updateModeWarn();
  const el = $("#phase");
  if (!el || !S || !S.queue.now) return;
  const n = S.queue.now, auto = S.playMode === "auto";
  const phase = n.playPhase || "waiting";
  let cls = phase, text = t("ph_" + phase);
  if (auto && !S.tosu.connected) { cls = "paused"; text = t("ph_noTosu"); }
  else if (phase === "ended") {
    const left = Math.ceil(S.autoGap - (Date.now() - new Date(n.phaseAt)) / 1000);
    const hasNext = (S.queue.items || []).some(i => i.state !== "failed");
    text += " · " + (!auto ? t("ph_pressNext") : !hasNext ? t("ph_queueEmpty") : t("ph_nextIn", { n: Math.max(0, left) }));
  }
  if (n.result) text += " · " + t("res_" + n.result.outcome) + " " + resultShort(n.result);
  if (n.result && n.result.missing) text += " · ⚠ " + t("ph_missing", { m: n.result.missing });
  const vs = S.voteSkip || {};
  if (vs.needed > 0 && vs.votes > 0) text += " · " + t("ph_votes", { v: vs.votes, n: vs.needed });
  el.className = "phase " + cls;
  el.lastChild.textContent = text;
}
setInterval(updatePhase, 1000);

function renderNow(n) {
  const box = $("#np");
  if (same(box, JSON.stringify([n, LANG, S.viewerUrl]))) return;
  box.replaceChildren();
  const next = h("button", { class: "btn main", onclick: e => busy(e.currentTarget, () => act("/api/action/next")) }, svg(ICON.next), t("next"));
  if (!n) {
    box.append(h("div", { class: "np empty" }, h("div", { class: "npi", style: "justify-content:center;text-align:center" },
      h("div", { class: "npt", style: "align-items:center" }, h("small", {}, t("nothingPlaying"))), h("div", { class: "npa" }, next))));
    return;
  }
  box.append(h("div", { class: "np", style: `background-image:url("${coverOf(n)}")` },
    h("div", { class: "npi" },
      h("div", { class: "npt" },
        h("span", { class: "lab" }, t("nowPlaying")),
        h("b", {}, n.title),
        h("span", {}, `${n.artist} · [${n.version}] · ${t("mappedBy")} ${n.creator}`),
        h("small", {}, t("reqBy") + " ", h("span", { class: "req" }, "@" + n.user), n.nick && n.nick !== n.user ? ` (${n.nick})` : "", " · ", h("a", { class: "mlink", href: setLink(n), target: "_blank", rel: "noopener" }, "osu.ppy.sh"), mapLinks(n)),
        tagsFor(n),
        h("span", { class: "phase", id: "phase" }, h("i"), h("span")),
        h("span", { class: "modewarn", id: "modeWarn", hidden: true })),
      h("div", { class: "npa" },
        h("button", { class: "btn", onclick: e => busy(e.currentTarget, () => act("/api/action/skip")) }, svg(ICON.skip), t("skip")),
        next))));
}

async function busy(btn, f) {
  btn.disabled = true;
  try { await f(); } finally { btn.disabled = false; }
}

let dragId = null;
const cards = new Map(); // item id -> card element currently shown
const seen = new Set();  // ids whose card already played its entrance animation
function renderQueue(items) {
  const list = $("#queue");
  $("#qCount").textContent = items.length ? `(${items.length})` : "";
  const res = $("#resolving");
  res.hidden = !(S.resolving > 0);
  res.textContent = S.resolving > 0 ? t("resolving", { n: S.resolving }) : "";
  if (!items.length) {
    cards.clear();
    if (!same(list, "empty" + LANG + S.command + S.search)) list.replaceChildren(h("div", { class: "empty" }, t(S.search ? "emptyQueueName" : "emptyQueue", { cmd: S.command })));
    return;
  }
  list.dataset.key = "";
  const next = items.map((it, i) => {
    const { attempts: _, ...rest } = it;
    const key = JSON.stringify([rest, i, items.length, LANG, ago(it.addedAt), S.viewerUrl]);
    let card = cards.get(it.id);
    if (!card || card.dataset.key !== key) {
      card = buildCard(it, i, items.length);
      card.dataset.key = key;
    }
    updateAttempts(card, it);
    return card;
  });
  const cur = [...list.children];
  if (cur.length === next.length && cur.every((c, i) => c.dataset.id === next[i].dataset.id)) {
    cur.forEach((c, i) => { if (c !== next[i]) c.replaceWith(next[i]); }); // swap only rebuilt cards
  } else {
    list.replaceChildren(...next); // added, removed or reordered
  }
  cards.clear();
  items.forEach((it, i) => { cards.set(it.id, next[i]); seen.add(it.id); });
}

function buildCard(it, i, total) {
  const q = (op, body) => act(`/api/queue/${it.id}/${op}`, body);
  const btn = (ico, title, f, cls, dis) => h("button", { class: "cbtn " + (cls || ""), title, "aria-label": title, disabled: dis, onclick: e => busy(e.currentTarget, f) }, svg(ico));
  const card = h("div", { class: "card" + (it.state === "failed" ? " failed" : "") + (seen.has(it.id) ? " seen" : ""), style: `--i:${i};background-image:url("${coverOf(it)}")`, draggable: "true", "data-id": it.id, "data-set": it.setId },
    h("span", { class: "pos", title: "drag" }, i + 1),
    h("div", { class: "ci" },
      h("b", { title: it.title }, it.title),
      h("span", { title: `${it.artist} · [${it.version}] · ${t("mappedBy")} ${it.creator}` }, `${it.artist} · [${it.version}] · ${t("mappedBy")} ${it.creator}`),
      h("small", { title: "@" + it.user }, h("span", { class: "s-" + it.state }, t("st_" + it.state)), " · " + ago(it.addedAt) + " · ", h("span", { class: "req" }, "@" + it.user)),
      tagsFor(it)),
    h("div", { class: "cact" },
      it.state === "failed" ? btn(ICON.retry, L("Retry", "ลองใหม่"), () => q("retry")) : btn(ICON.play, L("Play now", "เล่นเลย"), () => q("play"), "go"),
      btn(ICON.up, L("Move up", "เลื่อนขึ้น"), () => q("move", { to: i - 1 }), "", i === 0),
      btn(ICON.down, L("Move down", "เลื่อนลง"), () => q("move", { to: i + 1 }), "", i === total - 1),
      btn(ICON.skip, t("skip"), () => q("skip")),
      btn(ICON.x, L("Remove", "ลบ"), () => q("remove")),
      cardMenu(it)));
  if (it.warning) card.append(h("div", { class: "warnline" }, "⚠ " + it.warning));
  if (it.error) card.append(h("div", { class: "errline" }, it.error));
  card.addEventListener("dragstart", e => { dragId = it.id; card.classList.add("drag"); e.dataTransfer.effectAllowed = "move"; });
  card.addEventListener("dragend", () => { dragId = null; card.classList.remove("drag"); });
  card.addEventListener("dragover", e => { if (dragId && dragId !== it.id) { e.preventDefault(); card.classList.add("over"); } });
  card.addEventListener("dragleave", () => card.classList.remove("over"));
  card.addEventListener("drop", e => { e.preventDefault(); card.classList.remove("over"); if (dragId) act(`/api/queue/${dragId}/move`, { to: i }); });
  return card;
}

// updateAttempts keeps the download rows on a card current. When only the
// numbers changed it edits the existing bars, so progress animates smoothly.
function updateAttempts(card, it) {
  let list = [];
  if (it.attempts && it.attempts.length && it.state !== "ready" || it.state === "downloading") list = it.attempts || [];
  else if (it.state === "ready" && it.attempts && it.attempts.length) list = it.attempts.slice(-1);
  const old = card.querySelector(".att");
  if (!list.length) { if (old) old.remove(); return; }
  const shape = JSON.stringify(list.map(a => [a.source, a.status, a.status === "running" && !(a.total > 0)]));
  if (old && old.dataset.shape === shape) {
    list.forEach((a, i) => {
      const v = attemptView(a), row = old.children[i];
      row.title = a.message || "";
      row.querySelector(".bar>div").style.width = v.pct + "%";
      row.querySelector("i").textContent = v.text;
    });
    return;
  }
  const el = attempts(list);
  el.dataset.shape = shape;
  if (old) old.replaceWith(el);
  else card.querySelector(".cact").after(el);
}

function attemptView(a) {
  const pct = a.total > 0 ? Math.min(100, a.received / a.total * 100) : (a.status === "running" ? 0 : 100);
  const right = a.status === "running" ? (a.total > 0 ? Math.round(pct) + "%" : a.received ? mb(a.received) : "…") : a.status;
  return { pct, text: a.message ? `${right} · ${a.message}` : right };
}

// cardMenu is the "⋯" button on a card: preview, open the map elsewhere, ban.
function cardMenu(it) {
  const close = () => closeBanMenus();
  const viewer = viewerHref(it);
  const pop = h("div", { class: "banpop", hidden: true, role: "menu" },
    h("button", { type: "button", role: "menuitem", "data-pv": it.setId, onclick: e => { e.stopPropagation(); togglePreview(it.setId); } }),
    viewer ? h("a", { role: "menuitem", href: viewer, target: "_blank", rel: "noopener", onclick: close }, t("viewerTitle") + " ↗") : null,
    h("a", { role: "menuitem", href: setLink(it), target: "_blank", rel: "noopener", onclick: close }, "osu.ppy.sh · " + it.setId + " ↗"),
    h("hr"),
    h("button", { type: "button", role: "menuitem", class: "danger", onclick: () => ban("user", it.user) }, "⛔ " + t("banUser", { u: it.user })),
    h("button", { type: "button", role: "menuitem", class: "danger", onclick: () => ban("set", String(it.setId)) }, "⛔ " + t("banMap")),
    h("button", { type: "button", role: "menuitem", class: "danger", onclick: () => ban("song", it.artist + " - " + it.title) }, "⛔ " + t("banSong")),
    it.artist ? h("button", { type: "button", role: "menuitem", class: "danger", onclick: () => ban("artist", it.artist) }, "⛔ " + t("banArtist", { a: it.artist })) : null,
    it.creator ? h("button", { type: "button", role: "menuitem", class: "danger", onclick: () => ban("mapper", it.creator) }, "⛔ " + t("banMapper", { m: it.creator })) : null);
  const b = h("button", { class: "cbtn more", type: "button", title: t("more"), "aria-label": t("more"), "aria-haspopup": "menu",
    onclick: e => {
      e.stopPropagation(); closeBanMenus(pop);
      // fixed position next to the button: cards clip anything that overflows them
      const r = e.currentTarget.getBoundingClientRect();
      pop.hidden = !pop.hidden;
      syncPreview();
      if (pop.hidden) return;
      // below the button, or above it when there is no room; never off-screen
      pop.style.right = "auto";
      const w = pop.offsetWidth, ht = pop.offsetHeight;
      let top = r.bottom + 6;
      if (top + ht > innerHeight - 8) top = Math.max(8, r.top - 6 - ht);
      pop.style.top = top + "px";
      pop.style.left = Math.max(8, Math.min(r.right - w, innerWidth - w - 8)) + "px";
    } }, "⋯");
  return h("span", { class: "banwrap" }, b, pop);
}
function closeBanMenus(except) { document.querySelectorAll(".banpop").forEach(p => { if (p !== except) p.hidden = true; }); }
document.addEventListener("click", e => { if (!e.target.closest(".banwrap")) closeBanMenus(); });
addEventListener("scroll", () => closeBanMenus(), { passive: true });
async function ban(kind, value) {
  closeBanMenus();
  if (!confirm(t("confirmBan_" + kind, { v: value }))) return;
  const j = await api("POST", "/api/ban", { kind, value });
  if (j.ok) toast(t("banned", { v: kind === "user" ? "@" + value : value, n: j.removed }));
  else toast(errText(j), true);
}

function attempts(list) {
  return h("div", { class: "att" }, list.map(a => {
    const { pct, text } = attemptView(a);
    const cls = { ok: "ok", failed: "bad", mismatch: "mm", running: "run" }[a.status] || "";
    const indet = a.status === "running" && !(a.total > 0);
    return h("div", { class: "arow", title: a.message || "" },
      h("b", {}, a.source),
      h("div", { class: `bar ${cls} ${indet ? "indet" : ""}` }, h("div", { style: `width:${pct}%` })),
      h("i", {}, text));
  }));
}

function logItem(cls, head, who, at, body, code) {
  return h("div", { class: "tp " + cls },
    h("b", { class: "th", title: head }, head),
    h("div", { class: "h" }, who ? h("span", { class: "who", title: who }, who) : null, h("time", { datetime: at, title: new Date(at).toLocaleString() }, timeOf(at))),
    body ? h("p", {}, body) : null, code ? h("code", {}, code) : null);
}
function fill(sel, items, key) {
  const e = $(sel);
  if (same(e, JSON.stringify([key, LANG]))) return;
  e.replaceChildren(...(items.length ? items : [h("div", { class: "hint" }, t("none"))]));
}

function renderSide() {
  const rej = S.queue.rejected || [];
  $("#rejCount").textContent = rej.length || "";
  const notes = (S.notices || []).slice(0, 30), hist = S.queue.history || [];
  const raw = (S.raw && S.raw.length ? S.raw : Object.values(S.firstRaw || {})).slice(0, 30);
  const banned = new Set((S.blacklistUsers || []).map(u => u.replace(/^@/, "").toLowerCase()));
  fill("#rejLog", rej.slice(0, 40).map(r => {
    const li = logItem("err", t("r_" + r.code), "@" + r.user, r.at, r.short || "", r.input);
    li.title = r.detail; // the English detail stays available on hover
    if (!r.test && !banned.has(r.user.toLowerCase())) li.append(h("button", { class: "mlink banlink", type: "button", onclick: () => ban("user", r.user) }, "⛔ " + t("banUser", { u: r.user })));
    return li;
  }), [rej.slice(0, 40), [...banned]]);
  fill("#noteLog", notes.map(n => logItem({ error: "err", warn: "warn" }[n.kind] || "info", t("k_" + n.kind), "", n.at,
    n.key && T["n_" + n.key] ? t("n_" + n.key, n.vars || {}) : n.text)), notes);
  fill("#histLog", hist.map(p => logItem(p.result === "played" ? "ok" : "warn", p.title + (p.mods ? " +" + p.mods : ""), "@" + p.user, p.at,
    t(p.result) + (p.play ? " · " + t("res_" + p.play.outcome) + " " + resultShort(p.play) : ""))), hist);
  fill("#rawLog", raw.map(r => logItem("info", r.event, "", r.at, "", r.raw)), raw.map(r => [r.at, r.event]));
  renderSummary(S.summary);
  const ov = location.origin + "/overlay";
  const kv = $("#infoKv");
  kv.replaceChildren(
    h("dt", {}, t("version")), h("dd", {}, S.version),
    h("dt", {}, t("mode")), h("dd", {}, S.playMode),
    h("dt", {}, t("cache")), h("dd", {}, S.cacheMb.toFixed(1) + " MB"),
    h("dt", {}, t("overlayUrl")), h("dd", {}, h("button", { class: "mlink", onclick: () => copy(ov) }, ov)),
    ...modPanelRow(S.extra && S.extra.remote),
    ...(S.extra ? [h("dt", {}, t("folder")), h("dd", {}, S.extra.baseDir)] : []));
}

// renderSummary fills the "This stream" box (see queue.Summary).
function renderSummary(s) {
  const kv = $("#sumKv");
  if (!s || same(kv, JSON.stringify([s, LANG]))) return;
  const row = (k, ...v) => [h("dt", {}, t(k)), h("dd", {}, ...v)];
  kv.replaceChildren(
    ...row("sumStarted", timeOf(s.start)),
    ...row("sumPlayed", String(s.played), s.passed ? " · " + t("sumPassed", { n: s.passed }) : ""),
    ...row("sumSkipped", String(s.skipped)),
    ...row("sumRejected", String(s.rejected)),
    ...row("sumViewers", String(s.viewers)),
    ...(s.best ? row("sumBest", h("span", { title: s.best.title }, s.best.title), h("br"), resultShort(s.best.play) + " · @" + s.best.user) : []),
    ...(s.top && s.top.length ? row("sumTop", s.top.map(u => "@" + u.user + " ×" + u.count).join(", ")) : []));
}
$("#sumShowBtn").addEventListener("click", async () => { const j = await act("/api/action/showSummary"); if (j.ok) toast(t("sumShown")); });
$("#sumNewBtn").addEventListener("click", () => { if (confirm(t("confirmNewSession"))) act("/api/action/newSession"); });

// The support zip comes from a POST (a plain link would let other sites trigger it), saved through a blob.
$("#supportBtn").addEventListener("click", e => busy(e.currentTarget, async () => {
  const r = await fetch("/api/support", { method: "POST", headers: { "X-Osureq": "1" } });
  if (!r.ok) { toast("HTTP " + r.status, true); return; }
  const name = (/filename="([^"]+)"/.exec(r.headers.get("Content-Disposition") || "") || [])[1] || "osureq-support.zip";
  const a = h("a", { href: URL.createObjectURL(await r.blob()), download: name });
  document.body.append(a); a.click(); a.remove();
  setTimeout(() => URL.revokeObjectURL(a.href), 10000);
  toast(t("supportSaved"));
}));

// modPanelRow: the mod panel's addresses in the Info box, or why there are none.
function modPanelRow(r) {
  if (!r || (!r.enabled && !r.error)) return [];
  const body = r.error ? [h("span", { class: "serr", title: r.error }, r.error)]
    : r.urls && r.urls.length ? r.urls.map(u => h("div", {}, h("button", { class: "mlink", onclick: () => copy(u) }, u)))
    : [t("modOn"), h("small", { class: "hint", style: "display:block;white-space:normal" }, t("modNoLan"))];
  return [h("dt", {}, t("modPanel")), h("dd", {}, ...body, r.sessions ? h("small", { class: "hint", style: "display:block" }, t("modSessions", { n: r.sessions })) : null)];
}

function copy(text) {
  navigator.clipboard.writeText(text).then(() => toast(t("copied")), () => toast(text));
}

/* ---------- live connection ---------- */
let ws, wsRetry = 0, offlineT, quitting = false;
function connect() {
  ws = new WebSocket((location.protocol === "https:" ? "wss://" : "ws://") + location.host + "/ws");
  ws.onopen = () => { wsRetry = 0; clearTimeout(offlineT); };
  ws.onmessage = e => {
    const prev = S;
    S = JSON.parse(e.data);
    // A new osureq (after an update) serves a new dashboard.
    if (prev && prev.version !== S.version) { location.reload(); return; }
    handleSpeech();
    // Chat rejections are easy to miss in the side panel; flash the newest one.
    const r = (S.queue.rejected || [])[0], p = prev && (prev.queue.rejected || [])[0];
    if (prev && r && !r.test && (!p || r.at !== p.at)) toast(`@${r.user} ${t("rejectedToast")}: ${t("r_" + r.code)}${r.short ? " (" + r.short + ")" : ""}`, true);
    render();
  };
  ws.onclose = () => {
    if (!quitting) offlineT = setTimeout(() => toast(t("offline"), true), 3000);
    setTimeout(connect, Math.min(5000, 500 * 2 ** wsRetry++));
  };
}

/* ---------- top bar & forms ---------- */
$("#reqSw").addEventListener("change", () => act("/api/action/toggleRequests"));
$("#langSel").addEventListener("change", e => { LANG = e.target.value; langPicked = true; try { localStorage.setItem("osureq.lang", LANG); } catch {} applyLang(); render(); });
$("#updPill").addEventListener("click", e => {
  const u = S && S.update;
  if (!u || !u.ready) return; // not downloaded yet: the link opens the release page
  e.preventDefault();
  if (confirm(t("confirmUpdate", { v: u.version }))) act("/api/action/installUpdate");
});
$("#quitBtn").addEventListener("click", async () => {
  if (!confirm(t("confirmQuit"))) return;
  quitting = true;
  await act("/api/action/quit");
  toast(t("quitDone"));
});
// Close tosu / TikFinity from here (auto-launch then waits until they are started again).
function closeApp(btn, path, app, ask) {
  $(btn).addEventListener("click", async () => {
    if (!confirm(t(ask))) return;
    const j = await act(path);
    if (j.ok !== false) toast(t("closedApp", { app }));
  });
}
closeApp("#stopTosuBtn", "/api/action/stopTosu", "tosu", "confirmCloseTosu");
closeApp("#stopTfBtn", "/api/action/stopTikfinity", "TikFinity", "confirmCloseTikfinity");
$("#quitAllBtn").addEventListener("click", async () => {
  if (!confirm(t("confirmQuitAll"))) return;
  quitting = true;
  await act("/api/action/quitAll");
  toast(t("quitAllDone"));
});
$("#rmLastBtn").addEventListener("click", () => act("/api/action/removeLast"));
$("#clearBtn").addEventListener("click", () => { if (confirm(t("confirmClear"))) act("/api/action/clear"); });
/* ---------- chat TTS ---------- */
// The helper sends chat to read (S.speech, newest first, commands already
// left out, see app.speakChat). Browsers only speak after a click, so the
// streamer starts it once per tab with the top-bar button.
let ttsOn = false, ttsSeen = -1, ttsBusy = false;
const ttsQueue = [], ttsWarned = {};
const hasThai = s => /[\u0E00-\u0E7F]/.test(s);
let ttsAudio = null; // Google's voice playing, if any
// ttsVoice picks the browser voice for lang. The chosen voice is only used
// for its own language: an English voice reads nothing of a Thai message.
function ttsVoice(lang, voiceName) {
  const voices = "speechSynthesis" in window ? speechSynthesis.getVoices() : [];
  const m = voices.filter(v => v.lang.toLowerCase().startsWith(lang));
  if (voiceName) { const v = m.find(v => v.name === voiceName); if (v) return v; }
  return m.find(v => /natural|online/i.test(v.name)) || m.find(v => v.localService) || m[0] || null;
}
// ttsSay reads one line and calls done when it has finished (or failed).
// Who speaks follows tts.engine: the browser's voice, or Google's through
// the helper (/api/tts) when the browser has none for the language, which
// is the usual case for Thai in Chrome-based browsers.
function ttsSay(text, name, cfg, done) {
  const lang = cfg.lang !== "auto" ? cfg.lang : hasThai(text + " " + (name || "")) ? "th" : "en";
  const line = cfg.readName && name ? (T["ttsSays_" + lang][0]).replace("{name}", name).replace("{text}", text) : text;
  const volume = Math.max(0, Math.min(1, (cfg.volume ?? 80) / 100)), rate = cfg.rate || 1;
  const v = ttsVoice(lang, cfg.voice), engine = cfg.engine || "auto";
  if (engine === "google" || (engine === "auto" && !v)) {
    const r = fetch("/api/tts", { method: "POST", headers: { "X-Osureq": "1", "Content-Type": "application/json" }, body: JSON.stringify({ lang, text: line }) })
      .then(async r => { if (!r.ok) throw new Error((await r.text()).trim() || "HTTP " + r.status); return r.blob(); });
    r.then(b => {
      const a = new Audio(URL.createObjectURL(b));
      a.volume = volume; a.playbackRate = rate;
      let over = false; // pause and ended both fire at the end
      const end = () => { if (over) return; over = true; URL.revokeObjectURL(a.src); if (ttsAudio === a) ttsAudio = null; done(); };
      a.onended = a.onerror = a.onpause = end;
      ttsAudio = a;
      a.play().catch(end);
    }).catch(e => {
      if (!ttsWarned.online) { ttsWarned.online = true; toast(t("ttsOnlineFail", { err: e.message }), true); }
      done();
    });
    return;
  }
  if (!v && !ttsWarned[lang]) { ttsWarned[lang] = true; toast(t("ttsNoVoice", { lang: lang === "th" ? "ไทย" : "English" }), true); }
  const u = new SpeechSynthesisUtterance(line);
  if (v) u.voice = v;
  u.lang = v ? v.lang : lang === "th" ? "th-TH" : "en-US";
  u.rate = rate; u.volume = volume;
  u.onend = u.onerror = () => done();
  speechSynthesis.speak(u);
}
// ttsStop silences whichever voice is speaking.
function ttsStop() {
  if ("speechSynthesis" in window) speechSynthesis.cancel();
  if (ttsAudio) ttsAudio.pause(); // its onpause ends the line
}
function ttsNext() {
  if (ttsBusy || !ttsOn || !ttsQueue.length) return;
  const s = ttsQueue.shift();
  ttsBusy = true;
  ttsSay(s.text, s.name, S.tts, () => { ttsBusy = false; renderTTS(); ttsNext(); });
  renderTTS();
}
function handleSpeech() {
  const list = S.speech || [];
  if (ttsSeen < 0 || (list.length && list[0].id < ttsSeen)) { ttsSeen = list.length ? list[0].id : 0; return; } // first load or restart: no backlog
  const fresh = list.filter(s => s.id > ttsSeen).reverse();
  if (fresh.length) ttsSeen = fresh[fresh.length - 1].id;
  if (!S.tts || !(S.tts.enabled || S.tts.announce) || !ttsOn) return;
  ttsQueue.push(...fresh);
  while (ttsQueue.length > 5) ttsQueue.shift(); // a busy chat: keep up instead of falling behind
  ttsNext();
}
function renderTTS() {
  const on = !!(S && S.tts && (S.tts.enabled || S.tts.announce));
  $("#ttsWrap").hidden = !on || !("speechSynthesis" in window);
  if (!on && ttsOn) { ttsOn = false; ttsQueue.length = 0; ttsStop(); }
  const p = $("#ttsPill");
  p.textContent = ttsOn ? "🔊 " + t("ttsOn") : "🔈 " + t("ttsStart");
  p.title = t(ttsOn ? "ttsTitleOn" : "ttsTitleOff");
  p.classList.toggle("off", !ttsOn);
  $("#ttsSkip").hidden = !ttsOn || !ttsBusy;
  $("#ttsSkip").title = t("ttsSkip");
}
$("#ttsPill").addEventListener("click", () => {
  ttsOn = !ttsOn;
  if (!ttsOn) { ttsQueue.length = 0; ttsStop(); ttsBusy = false; }
  else speechSynthesis.speak(new SpeechSynthesisUtterance("")); // the click unlocks speech in this tab
  renderTTS();
});
$("#ttsSkip").addEventListener("click", ttsStop);
if ("speechSynthesis" in window) speechSynthesis.addEventListener("voiceschanged", () => { if (!$("#settings").hidden && tab === "tts") renderSettings(); });

/* ---------- master volume ---------- */
// One volume (0-100, default 10%) for every sound the dashboard plays. It is
// stored in config.json; the slider applies it at once and saves shortly after.
const VOL_ON = '<path d="M4 9v6h4l5 4V5L8 9z"/><path d="M16 9a4 4 0 0 1 0 6"/><path d="M18.5 6.5a8 8 0 0 1 0 11"/>';
const VOL_LOW = '<path d="M4 9v6h4l5 4V5L8 9z"/><path d="M16 9a4 4 0 0 1 0 6"/>';
const VOL_OFF = '<path d="M4 9v6h4l5 4V5L8 9z"/><path d="M17 9l5 6M22 9l-5 6"/>';
let volEditing = false, volSaveT, volBeforeMute = 10;
function showVolume(v) {
  v = Math.max(0, Math.min(100, Math.round(v)));
  preview.volume = v / 100;
  $("#volTxt").textContent = v + "%";
  $("#volOut").textContent = v + "%";
  $("#volRange").value = v;
  const ico = v === 0 ? VOL_OFF : v < 50 ? VOL_LOW : VOL_ON;
  $("#volIco").innerHTML = ico;
  $("#volIco2").innerHTML = ico;
  $("#volMute").setAttribute("aria-label", t(v === 0 ? "unmute" : "mute"));
  $("#volBtn").title = t("masterVolume") + " " + v + "%";
}
function setVolume(v) {
  showVolume(v);
  volEditing = true;
  clearTimeout(volSaveT);
  volSaveT = setTimeout(async () => {
    await api("POST", "/api/volume", { volume: +$("#volRange").value });
    volEditing = false;
  }, 300);
}
$("#volBtn").addEventListener("click", e => {
  e.stopPropagation();
  const pop = $("#volPop");
  pop.hidden = !pop.hidden;
  $("#volBtn").setAttribute("aria-expanded", String(!pop.hidden));
});
$("#volRange").addEventListener("input", e => setVolume(+e.target.value));
$("#volMute").addEventListener("click", () => {
  const v = +$("#volRange").value;
  if (v > 0) { volBeforeMute = v; setVolume(0); } else setVolume(volBeforeMute || 10);
});
document.addEventListener("click", e => { if (!e.target.closest(".volwrap")) { $("#volPop").hidden = true; $("#volBtn").setAttribute("aria-expanded", "false"); } });
showVolume(10);

/* ---------- test panel ---------- */
let testMode = "chat";
try { testMode = localStorage.getItem("osureq.testMode") || "chat"; } catch {}
function renderTestHint() {
  document.querySelectorAll("#testMode button").forEach(b => b.classList.toggle("on", b.dataset.mode === testMode));
  const cmd = S ? S.command : "!req";
  $("#testHint").textContent = t({ chat: "hintChat", quick: "hintQuick", gift: "hintGift" }[testMode] || "hintChat", { cmd }) + (S && S.dryRun ? t("hintDry") : "");
  $("#testIn").placeholder = t(testMode === "gift" ? "giftPh" : "testPh");
}
$("#testMode").addEventListener("click", e => {
  const b = e.target.closest("button[data-mode]");
  if (!b) return;
  testMode = b.dataset.mode;
  try { localStorage.setItem("osureq.testMode", testMode); } catch {}
  renderTestHint();
});
$("#dryRunSw").addEventListener("change", () => act("/api/action/toggleDryRun"));
/* ---------- pre-stream check ---------- */
// Runs app.Preflight and lists the result; problems first. Stays until closed.
let pfChecks = null;
function renderPreflight() {
  const out = $("#pfOut");
  out.hidden = !pfChecks;
  if (!pfChecks) return;
  const rank = { fail: 0, warn: 1, ok: 2, off: 3 };
  const list = [...pfChecks].sort((a, b) => rank[a.status] - rank[b.status]);
  const bad = list.filter(c => c.status === "fail" || c.status === "warn").length;
  const worst = list.some(c => c.status === "fail") ? "fail" : bad ? "warn" : "ok";
  out.replaceChildren(
    h("div", { class: "pfh " + worst }, bad ? t("pfIssues", { n: bad }) : t("pfAllOk"), h("span", { class: "sp" }),
      h("button", { class: "icon", type: "button", "aria-label": "Close", style: "width:32px;height:32px", onclick: () => { pfChecks = null; renderPreflight(); } }, svg(ICON.x))),
    ...list.map(c => h("div", { class: "pfr " + c.status, title: c.detail || "" },
      h("i", {}, { ok: "✓", warn: "!", fail: "✕", off: "–" }[c.status]),
      h("span", {}, T["pf_" + c.key] ? t("pf_" + c.key, c.vars || {}) : c.key),
      c.detail && c.status !== "ok" ? h("small", {}, c.detail) : null)));
}
$("#pfBtn").addEventListener("click", async e => {
  const btn = e.currentTarget;
  btn.disabled = true; btn.textContent = t("pfRunning");
  try {
    const j = await api("POST", "/api/preflight");
    if (j.ok) { pfChecks = j.checks; renderPreflight(); } else toast(t("pfFailed") + ": " + errText(j), true);
  } finally { btn.disabled = false; btn.textContent = t("pfRun"); }
});
$("#langSel").addEventListener("change", () => renderPreflight());
// The test panel is folded away by default: it is rarely needed during a stream.
function setTestOpen(open) {
  $("#testBody").hidden = !open;
  $("#testToggle").setAttribute("aria-expanded", String(open));
  try { localStorage.setItem("osureq.testOpen", open ? "1" : "0"); } catch {}
}
$("#testToggle").addEventListener("click", () => setTestOpen($("#testBody").hidden));
try { setTestOpen(localStorage.getItem("osureq.testOpen") === "1"); } catch { setTestOpen(false); }
$("#clearTestBtn").addEventListener("click", () => { if (confirm(t("confirmClearTest"))) act("/api/action/clearTest"); });
$("#testForm").addEventListener("submit", async e => {
  e.preventDefault();
  let input = $("#testIn").value.trim();
  if (!input) return;
  const user = $("#testUser").value;
  const btn = e.submitter; btn.disabled = true;
  try {
    let j;
    if (testMode === "gift") {
      const coins = Number(input);
      if (!Number.isInteger(coins) || coins <= 0) { toast(t("giftNum"), true); return; }
      j = await api("POST", "/api/simulateGift", { user, coins });
      if (j.ok) { toast(t("giftDone", { n: j.earned, r: j.requests })); $("#testIn").value = ""; } else toast(errText(j), true);
      return;
    }
    if (testMode === "chat") {
      // only a bare ID or osu! link gets the command; other text is plain chat (read by TTS)
      if (/^(\d+|(https?:\/\/)?(osu|old)\.ppy\.sh\/\S+)$/i.test(input)) input = (S ? S.command : "!req") + " " + input;
      j = await api("POST", "/api/simulate", { user, message: input });
    } else {
      j = await api("POST", "/api/test", { input, user });
    }
    if (j.command) { toast(t("cmd_" + j.command, { u: user || "viewer" }), !j.ok); if (j.ok) $("#testIn").value = ""; }
    else if (j.code === "spoken") { toast(t("spokenTest")); $("#testIn").value = ""; }
    else if (j.ok) { toast(t("added", { t: `${j.item.artist} - ${j.item.title} [${j.item.version}]` })); $("#testIn").value = ""; }
    else if (j.code === "ignored") toast(t("ignored"), true);
    else toast(j.code && T["r_" + j.code] ? t("r_" + j.code) + (j.short ? " · " + j.short : "") : j.error, true);
  } finally { btn.disabled = false; }
});
setInterval(() => S && renderQueue(S.queue.items || []), 30000); // refresh "x min ago"

/* ---------- settings ---------- */
const MODES = [["osu", "osu!"], ["taiko", "taiko"], ["fruits", "catch"], ["mania", "mania"]];
const STATUSES = ["ranked", "approved", "qualified", "loved", "pending", "wip", "graveyard"];
const F = (path, type, en, th, hintEn, hintTh, extra) => ({ path, type, label: [en, th], hint: hintEn ? [hintEn, hintTh] : null, ...extra });
const SCHEMA = {
  connection: [
    F("connection.tikfinityUrl", "text", "TikFinity WebSocket URL", "URL WebSocket ของ TikFinity", "Default ws://127.0.0.1:21213/", "ค่าเริ่มต้น ws://127.0.0.1:21213/"),
    F("connection.reconnectSec", "number", "Reconnect every (s)", "เชื่อมต่อใหม่ทุก (วินาที)", null, null, { min: 1, max: 120 }),
    F("connection.autoLaunchTikfinity", "switch", "Auto-launch TikFinity", "เปิด TikFinity อัตโนมัติ", "Starts TikFinity if it is installed but not running.", "เปิด TikFinity ให้ถ้าติดตั้งแล้วแต่ยังไม่ได้เปิด"),
    F("connection.tikfinityPath", "text", "TikFinity.exe path", "ที่อยู่ TikFinity.exe", "Leave as is unless TikFinity lives somewhere unusual.", "ปล่อยไว้ได้ ยกเว้นติดตั้งไว้ที่อื่น"),
    F("connection.dashboardPort", "number", "Dashboard port", "พอร์ตแดชบอร์ด", "Takes effect after a restart.", "มีผลหลังรีสตาร์ทโปรแกรม", { min: 1024, max: 65535 }),
    F("connection.openDashboardOnStart", "switch", "Open dashboard on start", "เปิดแดชบอร์ดตอนเริ่มโปรแกรม"),
    F("connection.tosuUrl", "text", "tosu WebSocket URL (required)", "URL WebSocket ของ tosu (จำเป็น)", "Default ws://127.0.0.1:24050/websocket/v2. Auto mode uses it to see when a song has ended.", "ค่าเริ่มต้น ws://127.0.0.1:24050/websocket/v2 โหมดอัตโนมัติใช้ดูว่าเพลงจบแล้ว"),
    F("connection.tosuPath", "text", "tosu.exe path", "ที่อยู่ tosu.exe", "Filled in automatically the first time tosu is seen running.", "จะถูกกรอกให้อัตโนมัติเมื่อเห็น tosu เปิดอยู่ครั้งแรก"),
    F("connection.autoLaunchTosu", "switch", "Auto-launch tosu", "เปิด tosu อัตโนมัติ", "Starts tosu if it is not running (needs the path above).", "เปิด tosu ให้ถ้ายังไม่ได้เปิด (ต้องมีที่อยู่ด้านบน)"),
    F("connection.checkUpdates", "switch", "Check for updates", "ตรวจหาอัปเดต", "Asks GitHub twice a day whether a newer osureq exists and shows a button at the top.", "ถาม GitHub วันละ 2 ครั้งว่ามี osureq ใหม่หรือไม่ แล้วแสดงปุ่มด้านบน"),
    F("connection.autoUpdate", "switch", "Install updates automatically", "ติดตั้งอัปเดตอัตโนมัติ", "Downloads a new osureq from this project's GitHub releases, checks its SHA-256 and installs it the next time osureq starts, so it never restarts in the middle of a stream. The button at the top installs it right away. Needs Check for updates.", "ดาวน์โหลด osureq ใหม่จากหน้า release ของโปรเจกต์บน GitHub ตรวจ SHA-256 แล้วติดตั้งตอนเปิดโปรแกรมครั้งหน้า จะไม่รีสตาร์ทเองระหว่างไลฟ์ กดปุ่มด้านบนเพื่อติดตั้งทันทีได้ ต้องเปิดตรวจหาอัปเดตไว้ด้วย"),
    F("connection.logRawEvents", "switch", "Log raw events", "บันทึกอีเวนต์ดิบ", "Shows every TikFinity message in the side panel. Use it to check field names.", "แสดงทุกข้อความจาก TikFinity ที่แผงข้าง ใช้ตรวจชื่อฟิลด์"),
  ],
  osu: [
    F("osu.game", "seg", "Game", "เกม", "lazer has no Songs folder, so maps are always opened.", "lazer ไม่มีโฟลเดอร์ Songs จะใช้วิธีเปิดไฟล์เสมอ", { options: [["stable", "stable"], ["lazer", "lazer"]] }),
    F("osu.detectGameTosu", "switch", "Detect the game with tosu", "ให้ tosu ตรวจว่าเป็นเกมไหน", "Switches between stable and lazer to match the osu! that tosu sees running.", "สลับ stable / lazer ให้ตรงกับ osu! ที่ tosu เห็นว่าเปิดอยู่"),
    F("osu.folder", "text", "osu! folder", "โฟลเดอร์ osu!", "stable: the folder with osu!.exe and Songs.", "stable: โฟลเดอร์ที่มี osu!.exe และ Songs"),
    F("osu.cacheFolder", "text", "Cache folder", "โฟลเดอร์แคช", "Relative paths are next to the exe.", "ถ้าเป็นพาธสัมพัทธ์จะอยู่ข้าง exe"),
    F("osu.downloadsFolder", "text", "Browser Downloads folder", "โฟลเดอร์ดาวน์โหลดของเบราว์เซอร์", "Watched for official osu! downloads.", "ใช้รอไฟล์ที่ดาวน์โหลดจาก osu! โดยตรง"),
    F("masterVolume", "number", "Master volume (%)", "ระดับเสียงหลัก (%)", "Every sound the dashboard plays (map previews). Default 10%. Also on the speaker button at the top.", "ทุกเสียงที่แดชบอร์ดเล่น (ตัวอย่างเพลง) ค่าเริ่มต้น 10% ปรับได้ที่ปุ่มลำโพงด้านบนด้วย", { min: 0, max: 100 }),
    F("osu.viewerUrl", "text", "Beatmap viewer URL", "URL ของ beatmap viewer", "Queue cards get a \"viewer ↗\" link to ?s={set}&b={map} on this site. Empty hides it.", "การ์ดในคิวจะมีลิงก์ \"viewer ↗\" ไปที่ ?s={set}&b={map} ของเว็บนี้ เว้นว่างเพื่อซ่อน"),
    F("osu.useOsuDb", "switch", "Skip maps already in osu!", "ข้ามแมพที่มีใน osu! แล้ว", "stable only: reads osu!.db and opens osu://b/{id} instead of downloading. Best effort.", "เฉพาะ stable: อ่าน osu!.db แล้วเปิด osu://b/{id} แทนการดาวน์โหลด"),
  ],
  command: [
    F("command.enabled", "switch", "Accept requests", "รับคำขอ"),
    F("command.name", "text", "Command", "คำสั่ง", "Must start with !", "ต้องขึ้นต้นด้วย !"),
    F("command.aliases", "list", "Aliases", "คำสั่งอื่น", "Comma separated, e.g. !request, !r", "คั่นด้วยจุลภาค เช่น !request, !r"),
    F("command.mods", "list", "Mods (TikTok users)", "ม็อด (ผู้ใช้ TikTok)", "Comma separated @uniqueId. Mods can use !skip, !reqon / !reqoff and !remove @user.", "@uniqueId คั่นด้วยจุลภาค ม็อดใช้ !skip, !reqon / !reqoff และ !remove @ชื่อ ได้"),
    F("command.trustTikTokMods", "switch", "TikTok moderators are mods too", "ม็อดของ TikTok นับเป็นม็อดด้วย", "Uses TikTok's moderator flag when TikFinity sends it.", "ใช้สถานะม็อดจาก TikTok ถ้า TikFinity ส่งมา"),
    F("command.skipCmd", "text", "Skip command (mods)", "คำสั่งข้ามเพลง (ม็อด)", "Skips the song being played. Empty = off.", "ข้ามเพลงที่กำลังเล่น เว้นว่าง = ปิด"),
    F("command.removeCmd", "text", "Remove command", "คำสั่งลบคำขอ", "Anyone removes their own latest request; mods can add @user. Empty = off.", "ทุกคนลบคำขอล่าสุดของตัวเองได้ ม็อดเติม @ชื่อ เพื่อลบของคนอื่น เว้นว่าง = ปิด"),
    F("command.openCmd", "text", "Open requests (mods)", "เปิดรับคำขอ (ม็อด)", null, null),
    F("command.closeCmd", "text", "Close requests (mods)", "ปิดรับคำขอ (ม็อด)", null, null),
    F("command.posCmd", "text", "Queue position command", "คำสั่งดูลำดับคิว", "Anyone: the overlay shows where their requests are in the queue. Empty = off.", "ทุกคน: overlay จะบอกว่าคำขอของคนนั้นอยู่คิวที่เท่าไร เว้นว่าง = ปิด"),
    F("command.nowCmd", "text", "Now playing command", "คำสั่งดูเพลงที่กำลังเล่น", "Anyone: the overlay shows the song being played and who asked for it. Each viewer can ask once every 20 s. Empty = off.", "ทุกคน: overlay จะแสดงเพลงที่กำลังเล่นและคนที่ขอ แต่ละคนถามได้ทุก 20 วินาที เว้นว่าง = ปิด"),
    F("command.voteSkipCmd", "text", "Vote skip command", "คำสั่งโหวตข้ามเพลง", null, null),
    F("command.voteSkipVotes", "number", "Votes needed to skip", "จำนวนโหวตที่ต้องใช้ข้ามเพลง", "Different viewers who must vote to skip the song being played. 0 = vote skip off.", "จำนวนผู้ชม (ไม่ซ้ำคน) ที่ต้องโหวตเพื่อข้ามเพลงที่กำลังเล่น 0 = ปิดการโหวต", { min: 0 }),
    F("command.searchByName", "switch", "Requests by song name", "ขอเพลงด้วยชื่อ", "\"!req freedom dive\" searches catboy.best and takes the best title match (most played first) that passes your map rules. IDs and links work as before.", "\"!req freedom dive\" จะค้นใน catboy.best แล้วเลือกแมพที่ชื่อตรงที่สุด (เล่นเยอะสุดก่อน) ที่ผ่านกฎแมพ ID และลิงก์ใช้ได้เหมือนเดิม"),
    F("command.bareNumberAs", "seg", "A bare number is a…", "ตัวเลขเปล่าๆ หมายถึง…", null, null, { options: [["beatmap", ["beatmap (difficulty) ID", "ID บีทแมพ (ระดับ)"]], ["set", ["beatmapset ID", "ID เซ็ต"]]] }),
  ],
  rules: [
    F("rules.minStars", "number", "Min stars", "ดาวขั้นต่ำ", "0 = no limit", "0 = ไม่จำกัด", { step: 0.1, min: 0 }),
    F("rules.maxStars", "number", "Max stars", "ดาวสูงสุด", "0 = no limit", "0 = ไม่จำกัด", { step: 0.1, min: 0 }),
    F("rules.modeStars", "modestars", "Stars per mode", "ดาวแยกตามโหมด", "Stars are not comparable between modes. Leave a box empty to use the min/max above for that mode. Converts use the stars of the mode they are played in.", "ดาวแต่ละโหมดเทียบกันไม่ได้ เว้นว่างเพื่อใช้ค่าต่ำสุด/สูงสุดด้านบน แมพ convert ใช้ดาวของโหมดที่เล่นจริง"),
    F("rules.maxLengthSec", "number", "Max length (s)", "ความยาวสูงสุด (วินาที)", "0 = no limit", "0 = ไม่จำกัด", { min: 0 }),
    F("rules.modes", "multi", "Allowed modes", "โหมดที่อนุญาต", null, null, { options: MODES }),
    F("rules.statuses", "multi", "Allowed statuses", "สถานะที่อนุญาต", null, null, { options: STATUSES.map(s => [s, s]) }),
    F("rules.allowMods", "switch", "Requests with mods", "ขอเพลงพร้อม mod", "Viewers can add mods: !req 2096297 HDHR, or a song name ending in +HDHR. osu! cannot be told to use them: the mods show on the dashboard and overlay and you select them yourself (Activity warns if a result lacks one).", "ผู้ชมใส่ mod ได้ เช่น !req 2096297 HDHR หรือชื่อเพลงตามด้วย +HDHR โปรแกรมสั่ง osu! ให้ใส่ mod ไม่ได้ mod จะแสดงบนแดชบอร์ดและ overlay ให้คุณเลือกเอง (ถ้าเล่นไม่ครบ กิจกรรม จะแจ้งเตือน)"),
    F("rules.bannedMods", "multi", "Mods not allowed", "mod ที่ไม่อนุญาต", null, null, { bad: true, options: ["NF", "EZ", "HD", "HR", "SD", "DT", "HT", "NC", "FL", "SO", "PF", "FI", "MR"].map(m => [m, m]) }),
    F("rules.hardModMaxStars", "number", "Max stars with DT/NC/HR", "ดาวสูงสุดเมื่อใส่ DT/NC/HR", "Stars are always the no-mod rating (osu! gives modded ratings only through its login API). Use this to stop, say, DT on maps above 5★. 0 = only the normal limit. Length counts DT/HT speed.", "ดาวที่ใช้เป็นแบบไม่มี mod เสมอ (osu! ให้ดาวพร้อม mod ผ่าน API ที่ต้องล็อกอินเท่านั้น) ใช้ค่านี้กัน เช่น DT บนแมพเกิน 5★ 0 = ใช้แค่ขีดจำกัดปกติ ความยาวคิดตามความเร็วของ DT/HT"),
    F("rules.followersOnly", "switch", "Followers only", "เฉพาะผู้ติดตาม", "Rejects viewers TikTok reports as not following. If TikFinity does not send follow status, requests are let through and Activity shows a warning.", "ปฏิเสธผู้ชมที่ TikTok แจ้งว่ายังไม่ได้ติดตาม ถ้า TikFinity ไม่ส่งสถานะการติดตามมา จะปล่อยผ่านและแจ้งเตือนใน กิจกรรม"),
    F("rules.subscriberPerUserLimit", "number", "Requests per subscriber", "คำขอต่อคนสำหรับสมาชิก", "Waiting requests allowed for TikTok subscribers. 0 = same as everyone.", "จำนวนคำขอที่รอได้ของสมาชิก TikTok 0 = เท่ากับคนอื่น", { min: 0 }),
    F("rules.blacklistSets", "numlist", "Blacklisted set IDs", "ID เซ็ตที่แบน", "Comma separated", "คั่นด้วยจุลภาค"),
    F("rules.blacklistSongs", "lines", "Blacklisted songs and artists", "เพลงและศิลปินที่แบน", "One rule per line. artist=\"a_hisa\" bans every song by that artist; title=\"Ghost\" bans that song by anyone; artist=\"xi\" title=\"Blue Zenith\" (or xi - Blue Zenith) bans one song. Every version is caught (other sets, TV size, cut…), artists match as whole words (\"xi\" also catches \"xi remixed by…\"), and Japanese/Thai names work too. Also from the ⋯ menu on a queue card.", "บรรทัดละกฎ artist=\"a_hisa\" แบนทุกเพลงของศิลปินนั้น title=\"Ghost\" แบนเพลงนั้นไม่ว่าใครทำ artist=\"xi\" title=\"Blue Zenith\" (หรือ xi - Blue Zenith) แบนเพลงเดียว จับได้ทุกเวอร์ชัน (เซ็ตอื่น TV size ตัดสั้น ฯลฯ) ชื่อศิลปินเทียบเป็นคำเต็ม (\"xi\" จับ \"xi remixed by…\" ด้วย) และใช้ชื่อภาษาญี่ปุ่น/ไทยได้ แบนจากเมนู ⋯ บนการ์ดในคิวได้ด้วย"),
    F("rules.blacklistMappers", "list", "Blacklisted mappers", "แมพเปอร์ที่แบน", "Comma separated osu! usernames", "ชื่อผู้ใช้ osu! คั่นด้วยจุลภาค"),
    F("rules.blacklistUsers", "list", "Blacklisted TikTok users", "ผู้ใช้ TikTok ที่แบน", "Comma separated @uniqueId", "@uniqueId คั่นด้วยจุลภาค"),
  ],
  queue: [
    F("queue.maxSize", "number", "Max queue size", "คิวสูงสุด", "0 = no limit", "0 = ไม่จำกัด", { min: 0 }),
    F("queue.perUserLimit", "number", "Requests per user", "คำขอต่อคน", "Maps waiting in the queue (the one playing does not count). 0 = no limit", "แมพที่รออยู่ในคิว (ไม่นับเพลงที่กำลังเล่น) 0 = ไม่จำกัด", { min: 0 }),
    F("queue.cooldownSec", "number", "Per-user cooldown (s)", "คูลดาวน์ต่อคน (วินาที)", null, null, { min: 0 }),
    F("rules.exemptUsers", "list", "No-limit users", "ผู้ใช้ที่ไม่จำกัด", "Comma separated @uniqueId (you, your mods). They skip the per-user limit and cooldown.", "@uniqueId คั่นด้วยจุลภาค (คุณ หรือม็อด) ไม่ติดจำนวนคำขอต่อคนและคูลดาวน์"),
    F("queue.autoCloseAt", "number", "Close requests at (waiting)", "ปิดรับคำขอเมื่อคิวถึง", "Requests close by themselves when this many are waiting, and the overlay says so. 0 = off.", "ปิดรับคำขอเองเมื่อมีคำขอรออยู่เท่านี้ และแจ้งบน overlay 0 = ปิดฟีเจอร์นี้", { min: 0 }),
    F("queue.autoReopenAt", "number", "Open again at (waiting)", "เปิดรับอีกครั้งเมื่อเหลือ", "Requests closed by the line above open again at this many or fewer. Closing by hand is never undone.", "คำขอที่ปิดเองจากบรรทัดบนจะเปิดอีกครั้งเมื่อเหลือเท่านี้หรือน้อยกว่า ถ้าปิดเองด้วยมือจะไม่เปิดให้", { min: 0 }),
    F("queue.closeAt", "time", "Close requests at (time)", "ปิดรับคำขอตอน (เวลา)", "E.g. 23:30, so the last requests fit before you end the stream. Once a day; ✕ = off.", "เช่น 23:30 เพื่อให้เล่นคำขอสุดท้ายทันก่อนจบไลฟ์ ทำวันละครั้ง ✕ = ปิด"),
    F("queue.fair", "switch", "Fair queue (take turns)", "คิวแบบผลัดกัน", "A viewer's second request waits until everyone else's first one, the third until everyone's second, and so on. Priority requests stay on top; moving cards by hand still works.", "คำขอที่ 2 ของผู้ชมคนหนึ่งจะรอจนทุกคนได้คำขอแรกก่อน คำขอที่ 3 รอทุกคนได้คำขอที่ 2 ไปเรื่อย ๆ คิวพิเศษยังอยู่บนสุด และยังลากการ์ดจัดเองได้"),
    F("queue.noRepeatMin", "number", "No repeat within (min)", "ห้ามซ้ำภายใน (นาที)", null, null, { min: 0 }),
    F("gifts.requestCoins", "number", "Gift-only requests: coins per request", "ขอเพลงด้วยของขวัญ: เหรียญต่อคำขอ", "Viewers must send gifts worth this many coins (added up) for each request; a rejected map costs nothing. No-limit users are free. Paid requests last as long as \"Unused priority lasts\". 0 = requests are free.", "ผู้ชมต้องส่งของขวัญรวมเท่านี้เหรียญต่อ 1 คำขอ ถ้าแมพถูกปฏิเสธจะไม่เสียสิทธิ์ ผู้ใช้ที่ไม่จำกัดขอได้ฟรี สิทธิ์อยู่ได้นานเท่ากับ \"สิทธิ์ที่ยังไม่ใช้อยู่ได้\" 0 = ขอฟรี", { min: 0 }),
    F("gifts.enabled", "switch", "Gifts buy priority", "ของขวัญได้คิวพิเศษ", "A viewer's gift coins add up; every full amount below moves their oldest waiting request to the top (after other priority requests), or is kept for their next request.", "เหรียญของขวัญของแต่ละคนจะสะสม ครบตามจำนวนด้านล่างเมื่อไหร่ คำขอที่รออยู่เก่าที่สุดของคนนั้นจะขึ้นหน้าคิว (ต่อจากคำขอพิเศษอื่น) หรือเก็บไว้ใช้กับคำขอถัดไป"),
    F("gifts.coinsPerPriority", "number", "Coins per priority", "เหรียญต่อคิวพิเศษ 1 ครั้ง", null, null, { min: 1 }),
    F("gifts.creditMinutes", "number", "Unused priority lasts (min)", "สิทธิ์ที่ยังไม่ใช้อยู่ได้ (นาที)", null, null, { min: 1 }),
    F("queue.playMode", "seg", "Play mode", "โหมดเล่น", "auto opens the next map when tosu sees that you played the current one and went back to song select (requires tosu). Without tosu, auto waits.", "auto จะเปิดแมพถัดไปเมื่อ tosu เห็นว่าเล่นแมพปัจจุบันจบและกลับไปหน้าเลือกเพลงแล้ว (ต้องใช้ tosu) ถ้าไม่มี tosu จะหยุดรอ", { options: [["manual", ["manual (press Next)", "กดเอง (ถัดไป)"]], ["auto", ["auto", "อัตโนมัติ"]]] }),
    F("queue.autoGapSec", "number", "Wait after song ends (s)", "รอหลังเพลงจบ (วินาที)", "Seconds after you return to song select before the next map opens.", "จำนวนวินาทีหลังกลับไปหน้าเลือกเพลง ก่อนเปิดแมพถัดไป", { min: 0 }),
  ],
  download: [
    F("download.sources", "sources", "Sources (in order)", "แหล่งดาวน์โหลด (ตามลำดับ)", "The official source opens osu.ppy.sh in your browser (you must be logged in there) and waits for the file in Downloads. It is only used when MD5 checking is on.", "แหล่ง official จะเปิด osu.ppy.sh ในเบราว์เซอร์ (ต้องล็อกอินไว้) แล้วรอไฟล์ในโฟลเดอร์ดาวน์โหลด ใช้เฉพาะตอนเปิดตรวจ MD5"),
    F("download.noVideo", "switch", "No video", "ไม่เอาวิดีโอ", "Smaller downloads where the source supports it.", "ไฟล์เล็กลงถ้าแหล่งรองรับ"),
    F("download.cacheLimitMb", "number", "Cache limit (MB)", "ขนาดแคชสูงสุด (MB)", "0 = no limit", "0 = ไม่จำกัด", { min: 0 }),
    F("download.autoCleanup", "switch", "Auto cleanup", "ล้างแคชอัตโนมัติ", "Deletes the oldest unused files when over the limit.", "ลบไฟล์เก่าที่ไม่ได้ใช้เมื่อเกินขนาด"),
    F("", "button", "Clean cache now", "ล้างแคชเดี๋ยวนี้", null, null, { action: "cleanup" }),
  ],
  md5: [
    F("md5.enabled", "switch", "Check MD5", "ตรวจ MD5", "Compares every downloaded .osu with the checksum on osu.ppy.sh, so outdated mirror copies are rejected.", "เทียบไฟล์ .osu ที่โหลดมากับ checksum บน osu.ppy.sh เพื่อกันไฟล์เวอร์ชันเก่าจาก mirror"),
    F("md5.level", "seg", "Level", "ระดับ", "strict: every difficulty must match. requested: only the requested one (whole set when requested by set ID).", "strict: ทุกระดับต้องตรง requested: เฉพาะระดับที่ขอ (ทั้งเซ็ตถ้าขอด้วย ID เซ็ต)", { options: [["requested", "requested"], ["strict", "strict"]] }),
    F("md5.verifyCache", "switch", "Verify cache before use", "ตรวจแคชก่อนใช้"),
    F("md5.onAllFail", "seg", "If every source fails", "ถ้าทุกแหล่งล้มเหลว", "warn: open the latest download anyway and show a warning.", "warn: เปิดไฟล์ล่าสุดที่โหลดได้พร้อมแจ้งเตือน", { options: [["skip", ["skip request", "ข้ามคำขอ"]], ["warn", ["open with warning", "เปิดพร้อมเตือน"]]] }),
    F("md5.metaTimeoutSec", "number", "osu! metadata timeout (s)", "เวลารอข้อมูล osu! (วินาที)", null, null, { min: 5, max: 120 }),
  ],
  import: [
    F("import.mode", "seg", "Import mode", "วิธีนำเข้า", "open: osu! imports and jumps to the map (stable + lazer). copy / extract: stable only, picked up on song select or F5.", "open: osu! นำเข้าแล้วกระโดดไปที่แมพ (stable + lazer) copy / extract: เฉพาะ stable จะเห็นเมื่อเข้าหน้าเลือกเพลงหรือกด F5",
      { options: [["open", "open"], ["copy-to-songs", "copy to Songs"], ["extract-to-songs", "extract to Songs"]] }),
    F("import.preload", "switch", "Preload upcoming maps", "โหลดแมพถัดไปล่วงหน้า", "Downloads and verifies the next maps in the background (stable: also copies them into Songs) so Next is instant.", "ดาวน์โหลดและตรวจแมพถัดไปไว้ก่อน (stable: คัดลอกเข้า Songs ด้วย) กด ถัดไป แล้วเปิดได้ทันที"),
    F("import.preloadCount", "number", "Preload count", "จำนวนที่โหลดล่วงหน้า", null, null, { min: 0, max: 10 }),
  ],
  overlay: [
    F("", "overlaylink", "OBS Browser Source URL", "URL สำหรับ Browser Source ใน OBS", "Add a Browser Source in OBS or TikTok LIVE Studio with this URL. The background is transparent.", "เพิ่ม Browser Source ใน OBS หรือ TikTok LIVE Studio ด้วย URL นี้ พื้นหลังโปร่งใส"),
    F("overlay.feedback", "switch", "Pop-ups for viewers", "ป๊อปอัปแจ้งผู้ชม", "Shows on the overlay when a request is added or rejected (with the reason), mod commands, gifts and play results. The helper cannot reply in TikTok chat, so this is how viewers find out.", "แสดงบน overlay เมื่อคำขอถูกเพิ่มหรือถูกปฏิเสธ (พร้อมเหตุผล) คำสั่งม็อด ของขวัญ และผลการเล่น เพราะโปรแกรมตอบในแชท TikTok ไม่ได้"),
    F("overlay.feedbackSec", "number", "Pop-up time (s)", "เวลาแสดงป๊อปอัป (วินาที)", null, null, { min: 2, max: 30 }),
    F("overlay.lang", "seg", "Overlay language", "ภาษาของ overlay", "auto follows the browser of OBS, which is usually English.", "auto จะตามภาษาของเบราว์เซอร์ใน OBS ซึ่งมักเป็นอังกฤษ", { options: [["auto", "auto"], ["en", "English"], ["th", "ไทย"]] }),
    F("overlay.visible", "number", "Upcoming maps shown", "จำนวนแมพถัดไปที่แสดง", null, null, { min: 0, max: 20 }),
    F("overlay.showRequester", "switch", "Show requester", "แสดงผู้ขอ"),
    F("overlay.avatars", "switch", "Viewers' pictures", "รูปโปรไฟล์ผู้ชม", "Shows the TikTok profile picture next to the name in pop-ups and now playing, when TikFinity sends it.", "แสดงรูปโปรไฟล์ TikTok ข้างชื่อในป๊อปอัปและเพลงที่กำลังเล่น ถ้า TikFinity ส่งมา"),
    F("overlay.theme", "seg", "Theme", "ธีม", null, null, { options: [["pink", "pink"], ["sky", "sky"], ["mono", "mono"]] }),
    F("overlay.scale", "number", "Size (%)", "ขนาด (%)", null, null, { min: 50, max: 250 }),
    F("overlay.position", "select", "Position", "ตำแหน่ง", null, null, { options: [["top-left", "top left"], ["top-right", "top right"], ["bottom-left", "bottom left"], ["bottom-right", "bottom right"]] }),
  ],
  remote: [
    F("remote.enabled", "switch", "Mod panel on a phone", "แผงม็อดบนมือถือ", "A small page for a phone on the same Wi-Fi: Next, Skip, remove and move requests, requests on/off. Nothing else (no settings, bans or quit). Only devices on your home network can open it, and only with the PIN. Windows may ask to allow osureq through the firewall: allow it for private networks only.", "หน้าเล็ก ๆ สำหรับมือถือที่ใช้ Wi-Fi เดียวกัน: ถัดไป ข้าม ลบ/เลื่อนคำขอ และเปิด/ปิดรับคำขอ ไม่มีอย่างอื่น (ไม่มีการตั้งค่า แบน หรือปิดโปรแกรม) เปิดได้เฉพาะอุปกรณ์ในวงแลนบ้านและต้องใช้ PIN Windows อาจถามให้อนุญาต osureq ผ่านไฟร์วอลล์: อนุญาตเฉพาะเครือข่ายส่วนตัว (Private)"),
    F("remote.pin", "pin", "PIN", "PIN", "6-12 digits. Changing it signs everyone out. Share it only with your mods.", "ตัวเลข 6-12 หลัก เปลี่ยนแล้วทุกคนจะถูกออกจากระบบ ให้เฉพาะม็อดของคุณ"),
    F("remote.port", "number", "Port", "พอร์ต", null, null, { min: 1024, max: 65535 }),
    F("", "remoteinfo", "Address", "ที่อยู่"),
  ],
  tts: [
    F("", "group", "sg_chat"),
    F("tts.enabled", "switch", "Read chat aloud (without commands)", "อ่านแชทออกเสียง (ไม่อ่านคำสั่ง)", "Reads TikTok chat in this dashboard with the voices of Windows, and never reads messages starting with ! (!req, !np…), links or emoji. Turn off TikFinity's own TTS so chat is not read twice. Browsers only speak after a click: press \"🔈 Start TTS\" at the top once per tab. OBS picks it up as desktop audio.", "อ่านแชท TikTok ในแดชบอร์ดนี้ด้วยเสียงของ Windows และไม่อ่านข้อความที่ขึ้นต้นด้วย ! (!req, !np…) ลิงก์ หรืออีโมจิ ให้ปิด TTS ของ TikFinity เพื่อไม่ให้อ่านซ้ำ เบราว์เซอร์จะพูดได้หลังคลิกเท่านั้น: กดปุ่ม \"🔈 เริ่มเสียงอ่าน\" ด้านบนหนึ่งครั้งต่อแท็บ OBS จับเสียงได้ผ่าน Desktop Audio"),
    F("tts.readName", "switch", "Say who wrote it", "บอกชื่อคนพิมพ์", "\"name says …\"", "\"ชื่อ บอกว่า …\""),
    F("tts.maxLength", "number", "Max letters per message", "ความยาวสูงสุดต่อข้อความ", "Longer messages are cut.", "ข้อความที่ยาวกว่านี้จะถูกตัด", { min: 10, max: 500 }),
    F("tts.cooldownSec", "number", "Per-viewer cooldown (s)", "คูลดาวน์ต่อคน (วินาที)", "A viewer's next message within this time is not read.", "ข้อความถัดไปของคนเดิมภายในเวลานี้จะไม่ถูกอ่าน", { min: 0 }),
    F("tts.allow", "multi", "Whose chat is read", "อ่านแชทของใคร", "None selected = everyone. Otherwise a viewer needs any one: a subscription (ใจส้ม), following, a gift during this stream, or being a mod. No-limit users are always read. Subscriber and follower come from TikFinity; when it does not send them, they count as no (Activity says so once).", "ไม่เลือกเลย = อ่านทุกคน ถ้าเลือก ผู้ชมต้องตรงอย่างน้อยหนึ่งข้อ: สมัครสมาชิก (ใจส้ม), กดติดตาม, ส่งของขวัญในไลฟ์นี้ หรือเป็นม็อด ผู้ใช้ที่ไม่จำกัดจะถูกอ่านเสมอ สถานะสมาชิกและผู้ติดตามมาจาก TikFinity ถ้าไม่ส่งมาจะถือว่าไม่ใช่ (แจ้งใน กิจกรรม ครั้งเดียว)",
      { options: [["subscriber", ["ใจส้ม (subscriber)", "ใจส้ม (สมาชิก)"]], ["follower", ["follower", "ผู้ติดตาม"]], ["gifter", ["sent a gift", "ส่งของขวัญแล้ว"]], ["mod", ["mod", "ม็อด"]]] }),
    F("tts.blockedWords", "list", "Blocked words", "คำต้องห้าม", "Comma separated. Messages containing one are not read.", "คั่นด้วยจุลภาค ข้อความที่มีคำเหล่านี้จะไม่ถูกอ่าน"),
    F("", "group", "sg_voice"),
    F("tts.lang", "seg", "Language", "ภาษา", "auto picks Thai or English for each message by its letters.", "auto เลือกไทยหรืออังกฤษตามตัวอักษรของแต่ละข้อความ", { options: [["auto", "auto"], ["th", "ไทย"], ["en", "English"]] }),
    F("tts.engine", "seg", "Who speaks", "ใครพูด", "auto uses the browser's voice when it has one for the language and Google's voice when it does not (Windows has no Thai voice, and Chrome or Dia only use Windows voices). Google sends the text being read to Google and needs internet.", "auto ใช้เสียงของเบราว์เซอร์ถ้ามีเสียงภาษานั้น ถ้าไม่มีจะใช้เสียงของ Google (Windows ไม่มีเสียงไทยมาให้ และ Chrome หรือ Dia ใช้ได้แค่เสียงของ Windows) แบบ Google จะส่งข้อความที่อ่านไปที่ Google และต้องต่ออินเทอร์เน็ต", { options: [["auto", "auto"], ["browser", ["browser", "เบราว์เซอร์"]], ["google", "Google"]] }),
    F("tts.voice", "voice", "Browser voice", "เสียงของเบราว์เซอร์", "Used for its own language only. Voices come from Windows and the browser (Edge has natural ones, including Thai).", "ใช้กับภาษาของเสียงนั้นเท่านั้น เสียงมาจาก Windows และเบราว์เซอร์ (Edge มีเสียงแบบธรรมชาติ รวมถึงภาษาไทย)"),
    F("tts.rate", "number", "Speed", "ความเร็ว", null, null, { min: 0.5, max: 2, step: 0.1 }),
    F("tts.volume", "number", "Volume (%)", "ระดับเสียง (%)", null, null, { min: 0, max: 100 }),
    F("", "ttstest", "Test the voice", "ทดลองเสียง"),
    F("", "group", "sg_announce"),
    F("tts.announce", "switch", "Announce requests", "ประกาศคำขอด้วยเสียง", "Says what happens to requests with the lines below, e.g. \"test1 ขอเพลง …\" or \"เพลง … ยากเกินไป 7.25 ดาว\". Uses the same voice settings and the same TTS button at the top; works with chat reading on or off.", "พูดว่าเกิดอะไรกับคำขอตามข้อความด้านล่าง เช่น \"test1 ขอเพลง …\" หรือ \"เพลง … ยากเกินไป 7.25 ดาว\" ใช้การตั้งค่าเสียงและปุ่มเสียงด้านบนเดียวกัน ใช้ได้ทั้งตอนเปิดหรือปิดการอ่านแชท", { rerender: true }),
    F("", "tplpreset", "Announcement lines", "ข้อความที่ประกาศ", "tplHelp", null, { when: "tts.announce" }),
    F("tts.templates.added", "template", "", "", null, null, { when: "tts.announce" }),
    F("tts.templates.rejected", "template", "", "", null, null, { when: "tts.announce" }),
    F("tts.templates.rejected_stars_high", "template", "", "", null, null, { when: "tts.announce" }),
    F("tts.templates.rejected_stars_low", "template", "", "", null, null, { when: "tts.announce" }),
    F("tts.templates.rejected_mod_stars", "template", "", "", null, null, { when: "tts.announce" }),
    F("tts.templates.rejected_too_long", "template", "", "", null, null, { when: "tts.announce" }),
    F("tts.templates.rejected_duplicate", "template", "", "", null, null, { when: "tts.announce" }),
    F("tts.templates.rejected_recently_played", "template", "", "", null, null, { when: "tts.announce" }),
    F("tts.templates.rejected_cooldown", "template", "", "", null, null, { when: "tts.announce" }),
    F("tts.templates.rejected_user_limit", "template", "", "", null, null, { when: "tts.announce" }),
    F("tts.templates.rejected_search_none", "template", "", "", null, null, { when: "tts.announce" }),
    F("tts.templates.nowplaying", "template", "", "", null, null, { when: "tts.announce" }),
    F("tts.templates.np", "template", "", "", null, null, { when: "tts.announce" }),
    F("tts.templates.np_none", "template", "", "", null, null, { when: "tts.announce" }),
    F("tts.templates.pos", "template", "", "", null, null, { when: "tts.announce" }),
    F("tts.templates.result_passed", "template", "", "", null, null, { when: "tts.announce" }),
    F("tts.templates.result_failed", "template", "", "", null, null, { when: "tts.announce" }),
    F("tts.templates.gift", "template", "", "", null, null, { when: "tts.announce" }),
    F("tts.templates.voteskip", "template", "", "", null, null, { when: "tts.announce" }),
    F("tts.templates.auto_off", "template", "", "", null, null, { when: "tts.announce" }),
    F("tts.templates.auto_on", "template", "", "", null, null, { when: "tts.announce" }),
  ],
  hotkeys: [
    F("hotkeys.enabled", "switch", "Global hotkeys", "ปุ่มลัดทั้งระบบ", "Work while osu! is focused. Format: Ctrl+Alt+N, Shift+F5, F9…", "ใช้ได้แม้กำลังอยู่ใน osu! รูปแบบ: Ctrl+Alt+N, Shift+F5, F9…"),
    F("hotkeys.next", "hotkey", "Next", "ถัดไป"),
    F("hotkeys.skip", "hotkey", "Skip", "ข้าม"),
    F("hotkeys.removeLast", "hotkey", "Remove last", "ลบอันล่าสุด"),
    F("hotkeys.toggleRequests", "hotkey", "Toggle requests", "เปิด/ปิดรับคำขอ"),
  ],
};
let draft = null, defaults = null, tab = "connection";
const getP = (o, p) => p.split(".").reduce((a, k) => a == null ? a : a[k], o);
const setP = (o, p, v) => { const ks = p.split("."); const last = ks.pop(); ks.reduce((a, k) => a[k], o)[last] = v; };
const lbl = x => Array.isArray(x) ? L(x[0], x[1]) : x;

async function openSettings() {
  const j = await api("GET", "/api/config");
  draft = j.config; defaults = j.defaults;
  $("#settings").hidden = false;
  renderSettings();
}
function closeSettings() { $("#settings").hidden = true; draft = null; }

function renderSettings() {
  const tabs = $("#setTabs");
  tabs.replaceChildren(...Object.keys(SCHEMA).map(k => h("button", { class: "tab" + (k === tab ? " on" : ""), role: "tab", "aria-selected": k === tab ? "true" : "false", onclick: () => { tab = k; renderSettings(); } }, t("tab_" + k))));
  const body = $("#setBody");
  const box = h("div", { class: "card2" });
  for (const f of SCHEMA[tab]) {
    if (f.when && !getP(draft, f.when)) continue; // e.g. announcement lines only while announcements are on
    box.append(field(f));
  }
  body.replaceChildren(box);
}

function field(f) {
  const v = f.path ? getP(draft, f.path) : null;
  const text = h("div", { class: "swt" }, h("b", {}, lbl(f.label)), f.hint ? h("small", {}, lbl(f.hint)) : null);
  const set = x => setP(draft, f.path, x);
  switch (f.type) {
    case "group":
      return h("div", { class: "sgroup" }, t(f.label[0]));
    case "time": {
      const i = h("input", { type: "time", value: v || "", "aria-label": lbl(f.label) });
      const clear = h("button", { class: "btn sm ghost", type: "button", onclick: () => { i.value = ""; set(""); } }, "✕");
      i.addEventListener("input", () => set(i.value));
      return h("div", { class: "row" }, text, h("div", { class: "ovlink" }, i, clear));
    }
    case "switch": {
      const i = h("input", { type: "checkbox", class: "switch", "aria-label": lbl(f.label) }); i.checked = !!v;
      i.addEventListener("change", () => { set(i.checked); if (f.rerender) renderSettings(); });
      return h("label", { class: "row", style: "cursor:pointer" }, text, i);
    }
    case "text": case "hotkey": {
      const i = h("input", { type: "text", value: v ?? "", title: v ?? "", "aria-label": lbl(f.label), spellcheck: "false" });
      i.addEventListener("input", () => { set(i.value); i.title = i.value; });
      const err = f.type === "hotkey" && S && S.extra && S.extra.hotkeyErrors && S.extra.hotkeyErrors[f.path.split(".")[1]];
      if (err) text.append(h("small", { style: "color:#ff9aa6" }, err));
      return h("div", { class: f.type === "text" && String(v || "").length > 30 ? "row stack" : "row" }, text, i);
    }
    case "number": {
      const i = h("input", { type: "number", value: v ?? 0, step: f.step || 1, min: f.min, max: f.max, "aria-label": lbl(f.label) });
      i.addEventListener("input", () => set(i.value === "" ? 0 : Number(i.value)));
      return h("div", { class: "row" }, text, i);
    }
    case "list": case "numlist": {
      const i = h("input", { type: "text", value: (v || []).join(", "), "aria-label": lbl(f.label), spellcheck: "false" });
      i.addEventListener("input", () => {
        const parts = i.value.split(/[,\n]/).map(s => s.trim()).filter(Boolean);
        set(f.type === "numlist" ? parts.map(Number).filter(n => Number.isInteger(n) && n > 0) : parts);
      });
      return h("div", { class: "row stack" }, text, i);
    }
    case "lines": { // one entry per line, for values that may contain commas
      const i = h("textarea", { rows: 3, "aria-label": lbl(f.label), spellcheck: "false" });
      i.value = (v || []).join("\n");
      i.addEventListener("input", () => set(i.value.split("\n").map(s => s.trim()).filter(Boolean)));
      return h("div", { class: "row stack" }, text, i);
    }
    case "seg": {
      const seg = h("div", { class: "seg" });
      const draw = () => seg.replaceChildren(...f.options.map(([val, name]) => h("button", { type: "button", class: getP(draft, f.path) === val ? "on" : "", onclick: () => { set(val); draw(); } }, lbl(name))));
      draw();
      return h("div", { class: "row stack" }, text, seg);
    }
    case "select": {
      const s = h("select", { "aria-label": lbl(f.label) }, f.options.map(([val, name]) => h("option", { value: val }, lbl(name))));
      s.value = v; s.addEventListener("change", () => set(s.value));
      return h("div", { class: "row" }, text, s);
    }
    case "multi": {
      const c = h("div", { class: "chips" + (f.bad ? " bad" : "") });
      const draw = () => {
        const cur = getP(draft, f.path) || [];
        c.replaceChildren(...f.options.map(([val, name]) => h("button", { type: "button", class: "chip" + (cur.includes(val) ? " on" : ""), onclick: () => {
          set(cur.includes(val) ? cur.filter(x => x !== val) : [...cur, val]); draw();
        } }, lbl(name))));
      };
      draw();
      return h("div", { class: "row stack" }, text, c);
    }
    case "button":
      return h("div", { class: "row" }, text, h("button", { class: "btn sm", type: "button", onclick: () => act("/api/action/" + f.action) }, lbl(f.label)));
    case "overlaylink": {
      const url = location.origin + "/overlay";
      return h("div", { class: "row stack" }, text, h("div", { class: "ovlink" }, h("input", { type: "text", value: url, readonly: true }),
        h("button", { class: "icon", type: "button", "aria-label": "Copy", onclick: () => copy(url) }, svg(ICON.copy)),
        h("a", { class: "btn sm", href: "/overlay", target: "_blank" }, L("Preview", "ดูตัวอย่าง"))));
    }
    case "sources": return sourcesField(f, text);
    case "pin": {
      const i = h("input", { type: "text", inputmode: "numeric", value: v ?? "", maxlength: 12, "aria-label": lbl(f.label), spellcheck: "false", autocomplete: "off" });
      i.addEventListener("input", () => set(i.value.replace(/\D/g, "")));
      const gen = h("button", { class: "btn sm", type: "button", onclick: () => { i.value = newPin(); set(i.value); } }, t("genPin"));
      return h("div", { class: "row" }, text, h("div", { class: "ovlink" }, i, gen));
    }
    case "voice": {
      const voices = "speechSynthesis" in window ? speechSynthesis.getVoices() : [];
      const s = h("select", { "aria-label": lbl(f.label) }, h("option", { value: "" }, t("ttsAuto")),
        ...voices.filter(x => /^(th|en)/i.test(x.lang)).map(x => h("option", { value: x.name }, `${x.name} (${x.lang})`)));
      s.value = v || ""; s.addEventListener("change", () => set(s.value));
      return h("div", { class: "row stack" }, text, s);
    }
    case "tplpreset": {
      const preset = which => api("GET", "/api/config").then(j => { draft.tts.templates = structuredClone(j.announcements[which]); renderSettings(); });
      return h("div", { class: "row stack" }, h("div", { class: "swt" }, h("b", {}, lbl(f.label)), h("small", {}, t(f.hint[0]))),
        h("div", { class: "ovlink" }, h("button", { class: "btn sm", type: "button", onclick: () => preset("th") }, t("tplThai")),
          h("button", { class: "btn sm", type: "button", onclick: () => preset("en") }, t("tplEnglish"))));
    }
    case "template": {
      const ev = f.path.split(".").pop();
      draft.tts.templates = draft.tts.templates || {};
      const i = h("input", { type: "text", value: draft.tts.templates[ev] ?? "", placeholder: t("tplSilent"), "aria-label": t("tpl_" + ev), spellcheck: "false" });
      i.addEventListener("input", () => { draft.tts.templates[ev] = i.value; });
      return h("div", { class: "row stack" }, h("div", { class: "swt" }, h("b", {}, t("tpl_" + ev))), i);
    }
    case "ttstest": {
      // reads what is typed (Thai or English, picked like chat), or the sample when empty
      const i = h("input", { type: "text", placeholder: t("ttsSample"), maxlength: "500", "aria-label": lbl(f.label) });
      const say = () => { ttsStop(); ttsSay(i.value.trim() || t("ttsSample"), "osureq", draft.tts, () => {}); };
      i.addEventListener("keydown", e => { if (e.key === "Enter") { e.preventDefault(); say(); } });
      return h("div", { class: "row stack" }, text, h("div", { class: "ovlink" }, i, h("button", { class: "btn sm", type: "button", onclick: say }, "▶ " + lbl(f.label))));
    }
    case "remoteinfo": {
      const r = S && S.extra && S.extra.remote;
      const body = !r || !r.enabled ? h("small", { class: "hint" }, r && r.error ? r.error : t("modNoUrl"))
        : r.urls && r.urls.length ? h("div", {}, h("small", { class: "hint" }, t("modUrls")), ...r.urls.map(u => h("div", {}, h("button", { class: "mlink", type: "button", onclick: () => copy(u) }, u))))
        : h("small", { class: "hint" }, t("modNoLan"));
      return h("div", { class: "row stack" }, text, body);
    }
    case "modestars": {
      const grid = h("div", { class: "modestars" }, h("span"), h("small", {}, t("min")), h("small", {}, t("max")));
      for (const [mode, name] of MODES) {
        const num = key => {
          const cur = ((draft.rules.modeStars || {})[mode] || {})[key];
          const i = h("input", { type: "number", min: 0, step: 0.1, value: cur > 0 ? cur : "", placeholder: t("global"), "aria-label": name + " " + key });
          i.addEventListener("input", () => {
            const ms = draft.rules.modeStars = draft.rules.modeStars || {};
            const r = ms[mode] = ms[mode] || { min: 0, max: 0 };
            r[key] = i.value === "" ? 0 : Number(i.value);
            if (!r.min && !r.max) delete ms[mode];
          });
          return i;
        };
        grid.append(h("b", {}, name), num("min"), num("max"));
      }
      return h("div", { class: "row stack" }, text, grid);
    }
  }
  return h("div");
}

function sourcesField(f, text) {
  const wrap = h("div", { class: "row stack" }, text);
  const list = h("div");
  const draw = () => {
    const src = draft.download.sources;
    list.replaceChildren(...src.map((s, i) => {
      const en = h("input", { type: "checkbox", class: "switch", "aria-label": s.name }); en.checked = s.enabled;
      en.addEventListener("change", () => s.enabled = en.checked);
      const to = h("input", { type: "number", class: "num", value: s.timeoutSec, min: 5, title: L("timeout (s)", "หมดเวลา (วินาที)") });
      to.addEventListener("input", () => s.timeoutSec = Number(to.value));
      const mv = (d, ico, dis) => h("button", { type: "button", class: "cbtn", disabled: dis, "aria-label": d < 0 ? "up" : "down", onclick: () => { src.splice(i + d, 0, src.splice(i, 1)[0]); draw(); } }, svg(ico));
      const url = (k, ph) => { const x = h("input", { type: "text", value: s[k] || "", placeholder: ph, spellcheck: "false" }); x.addEventListener("input", () => s[k] = x.value); return x; };
      return h("div", { class: "src" }, en, h("b", {}, s.name + (s.official ? " ★" : "")), to, mv(-1, ICON.up, i === 0), mv(1, ICON.down, i === src.length - 1),
        h("div", { class: "urls" }, url("url", "https://…/{id}"), url("noVideoUrl", L("no-video URL (optional)", "URL ไม่เอาวิดีโอ (ไม่บังคับ)"))));
    }));
  };
  draw();
  wrap.append(list, h("small", { class: "hint" }, L("{id} is replaced with the beatmapset ID. Only catboy.best, osu.direct and osu.ppy.sh are allowed. Timeout is in seconds.", "{id} จะถูกแทนด้วย ID เซ็ต อนุญาตเฉพาะ catboy.best, osu.direct และ osu.ppy.sh เวลาหมดเป็นวินาที")));
  return wrap;
}

$("#setBtn").addEventListener("click", openSettings);
$("#setClose").addEventListener("click", closeSettings);
$("#scrim").addEventListener("click", closeSettings);
document.addEventListener("keydown", e => { if (e.key === "Escape" && !$("#settings").hidden) closeSettings(); });
$("#setReset").addEventListener("click", () => {
  const key = { connection: "connection", osu: "osu", command: "command", rules: "rules", queue: "queue", download: "download", md5: "md5", import: "import", overlay: "overlay", remote: "remote", tts: "tts", hotkeys: "hotkeys" }[tab];
  draft[key] = structuredClone(defaults[key]);
  renderSettings();
});
// newPin makes an 8-digit PIN that passes config.GoodPIN.
function newPin() {
  for (;;) {
    const p = Array.from(crypto.getRandomValues(new Uint32Array(8)), n => n % 10).join("");
    if (!/^(\d)\1+$/.test(p) && !"0123456789".includes(p) && !"9876543210".includes(p)) return p;
  }
}
$("#setSave").addEventListener("click", async e => {
  draft.language = LANG;
  if (draft.remote && draft.remote.enabled && !draft.remote.pin) draft.remote.pin = newPin();
  const btn = e.currentTarget; btn.disabled = true;
  try {
    const j = await api("PUT", "/api/config", draft);
    if (j.ok) { toast(j.restartNeeded ? t("restart") : t("saved")); closeSettings(); }
    else toast(errText(j), true);
  } finally { btn.disabled = false; }
});

applyLang();
renderTestHint();
connect();
