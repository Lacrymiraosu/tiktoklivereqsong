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
  dryRun: ["Dry run", "ทดลองไม่เปิดเกม"],
  clearTest: ["Clear test data", "ล้างข้อมูลทดสอบ"],
  testPh: ["!req 2096297 or a link", "!req 2096297 หรือลิงก์"],
  testUserPh: ["@user", "@ผู้ใช้"],
  testBtn: ["Send", "ส่ง"],
  hintChat: ["Sends the message as if @user typed it in TikTok chat: command, requests on/off, per-user limit, cooldown and every map rule apply. Works offline. A bare ID or link gets {cmd} added.", "ส่งข้อความเหมือน @ผู้ใช้ พิมพ์ในแชท TikTok ใช้กฎทั้งหมด (คำสั่ง เปิด/ปิดรับ จำนวนต่อคน คูลดาวน์ กฎแมพ) ใช้ได้แม้ไม่ได้ไลฟ์ ถ้าพิมพ์แค่ ID หรือลิงก์จะเติม {cmd} ให้"],
  hintQuick: ["Adds a map straight to the queue. Only map rules apply (no per-user limit or cooldown).", "เพิ่มแมพเข้าคิวทันที ใช้แค่กฎแมพ (ไม่นับจำนวนต่อคนและคูลดาวน์)"],
  hintDry: [" Dry run is on: Next and Play move the queue but never open osu! or copy into Songs.", " เปิดโหมดทดลองอยู่: กด ถัดไป/เล่น จะเลื่อนคิวแต่ไม่เปิด osu! และไม่คัดลอกเข้า Songs"],
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
  rejectedToast: ["rejected", "ถูกปฏิเสธ"],
  r_too_long: ["too long", "ยาวเกิน"], r_meta_error: ["osu! lookup failed", "ดึงข้อมูล osu! ไม่ได้"], r_bad_input: ["invalid input", "รูปแบบไม่ถูกต้อง"], r_requests_off: ["requests off", "ปิดรับคำขอ"],
  // settings tabs
  tab_connection: ["Connection", "การเชื่อมต่อ"], tab_osu: ["osu!", "osu!"], tab_command: ["Command", "คำสั่ง"], tab_rules: ["Map rules", "กฎแมพ"],
  tab_queue: ["Queue", "คิว"], tab_download: ["Download", "ดาวน์โหลด"], tab_md5: ["MD5", "MD5"], tab_import: ["Import", "นำเข้า"], tab_overlay: ["Overlay", "โอเวอร์เลย์"], tab_hotkeys: ["Hotkeys", "ปุ่มลัด"],
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
async function act(path, body) {
  const j = await api("POST", path, body);
  if (j.ok === false) toast(j.error || "error", true);
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
  $("#dryRunSw").checked = S.dryRun;
  $("#dryPill").hidden = !S.dryRun;
  renderTestHint();
  renderBanner(tf);
  renderNow(S.queue.now);
  renderQueue(S.queue.items || []);
  renderSide();
}

function renderBanner(tf) {
  const b = $("#banner");
  b.replaceChildren();
  if (tf.level === "green") return;
  const chk = (ok, k) => h("em", { class: ok ? "ok" : "no" }, (ok ? "✓ " : "✕ ") + t(k));
  const acts = h("div", { class: "npa" });
  if (!tf.installed) acts.append(h("button", { class: "btn main", onclick: () => act("/api/action/openTikfinityDownload") }, t("getTikfinity")));
  else if (!tf.running) acts.append(h("button", { class: "btn main", onclick: () => act("/api/action/launchTikfinity").then(j => j.ok && toast("…")) }, t("launch")));
  if (tf.installed) acts.append(h("button", { class: "btn", onclick: () => act("/api/action/reconnect") }, t("reconnect")));
  b.append(h("div", { class: "banner " + tf.level },
    h("i", { class: "dot " + tf.level }),
    h("div", { class: "bt" },
      h("b", {}, t("tf_" + tf.code)),
      h("small", {}, t("b_" + tf.code) + " " + t("b_note")),
      tf.error ? h("small", { style: "display:block;color:#ff9aa6" }, tf.error) : null,
      h("div", { class: "steps" }, chk(tf.installed, "chk_installed"), chk(tf.running, "chk_running"), chk(tf.connected, "chk_ws"), chk(tf.handshake, "chk_hs"), chk(tf.live, "chk_live"))),
    acts));
}

function tagsFor(it) {
  return h("div", { class: "tags" },
    h("em", { class: "star", style: `background:${starColor(it.stars)};color:${it.stars >= 6.7 ? "#fff" : "#1c1726"}` }, it.stars >= 0 ? it.stars.toFixed(2) + "★" : "?★"),
    h("em", {}, MODE[it.mode] || it.mode),
    h("em", {}, clock(it.length)),
    it.status ? h("em", { class: "st-" + it.status }, it.status) : null,
    it.md5 ? h("em", { class: "md5-" + it.md5 }, "MD5 " + it.md5) : null,
    it.source ? h("em", {}, it.source) : null,
    it.bySet ? h("em", {}, t("wholeSet")) : null,
    it.test ? h("em", {}, t("test")) : null);
}

function renderNow(n) {
  const box = $("#np");
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
        h("small", {}, t("reqBy") + " ", h("span", { class: "req" }, "@" + n.user), n.nick && n.nick !== n.user ? ` (${n.nick})` : "", " · ", h("a", { class: "mlink", href: setLink(n), target: "_blank", rel: "noopener" }, "osu.ppy.sh")),
        tagsFor(n)),
      h("div", { class: "npa" },
        h("button", { class: "btn", onclick: e => busy(e.currentTarget, () => act("/api/action/skip")) }, svg(ICON.skip), t("skip")),
        next))));
}

async function busy(btn, f) {
  btn.disabled = true;
  try { await f(); } finally { btn.disabled = false; }
}

let dragId = null;
function renderQueue(items) {
  const list = $("#queue");
  $("#qCount").textContent = items.length ? `(${items.length})` : "";
  list.replaceChildren();
  if (S.resolving > 0) list.append(h("div", { class: "hint", style: "margin:0 4px" }, t("resolving", { n: S.resolving })));
  if (!items.length) {
    list.append(h("div", { class: "empty" }, t("emptyQueue", { cmd: S.command })));
    return;
  }
  items.forEach((it, i) => {
    const q = (op, body) => act(`/api/queue/${it.id}/${op}`, body);
    const btn = (ico, title, f, cls, dis) => h("button", { class: "cbtn " + (cls || ""), title, "aria-label": title, disabled: dis, onclick: e => busy(e.currentTarget, f) }, svg(ico));
    const card = h("div", { class: "card " + (it.state === "failed" ? "failed" : ""), style: `--i:${i};background-image:url("${coverOf(it)}")`, draggable: "true", "data-id": it.id },
      h("span", { class: "pos", title: "drag" }, i + 1),
      h("div", { class: "ci" },
        h("b", {}, it.title),
        h("span", {}, `${it.artist} · [${it.version}] · ${t("mappedBy")} ${it.creator}`),
        h("small", {}, h("span", { class: "req" }, "@" + it.user), " · " + ago(it.addedAt) + " · ", h("span", { class: "s-" + it.state }, t("st_" + it.state)), " · ",
          h("a", { class: "mlink", href: setLink(it), target: "_blank", rel: "noopener" }, it.setId)),
        tagsFor(it)),
      h("div", { class: "cact" },
        it.state === "failed" ? btn(ICON.retry, L("Retry", "ลองใหม่"), () => q("retry")) : btn(ICON.play, L("Play now", "เล่นเลย"), () => q("play"), "go"),
        btn(ICON.up, L("Move up", "เลื่อนขึ้น"), () => q("move", { to: i - 1 }), "", i === 0),
        btn(ICON.down, L("Move down", "เลื่อนลง"), () => q("move", { to: i + 1 }), "", i === items.length - 1),
        btn(ICON.skip, t("skip"), () => q("skip")),
        btn(ICON.x, L("Remove", "ลบ"), () => q("remove"))));
    if (it.attempts && it.attempts.length && it.state !== "ready" || it.state === "downloading") card.append(attempts(it.attempts || []));
    else if (it.state === "ready" && it.attempts && it.attempts.length) card.append(attempts(it.attempts.slice(-1)));
    if (it.warning) card.append(h("div", { class: "warnline" }, "⚠ " + it.warning));
    if (it.error) card.append(h("div", { class: "errline" }, it.error));
    card.addEventListener("dragstart", e => { dragId = it.id; card.classList.add("drag"); e.dataTransfer.effectAllowed = "move"; });
    card.addEventListener("dragend", () => { dragId = null; card.classList.remove("drag"); });
    card.addEventListener("dragover", e => { if (dragId && dragId !== it.id) { e.preventDefault(); card.classList.add("over"); } });
    card.addEventListener("dragleave", () => card.classList.remove("over"));
    card.addEventListener("drop", e => { e.preventDefault(); card.classList.remove("over"); if (dragId) act(`/api/queue/${dragId}/move`, { to: i }); });
    list.append(card);
  });
}

function attempts(list) {
  return h("div", { class: "att" }, list.map(a => {
    const pct = a.total > 0 ? Math.min(100, a.received / a.total * 100) : (a.status === "running" ? 0 : 100);
    const cls = { ok: "ok", failed: "bad", mismatch: "mm", running: "run" }[a.status] || "";
    const indet = a.status === "running" && !(a.total > 0);
    const right = a.status === "running" ? (a.total > 0 ? Math.round(pct) + "%" : a.received ? mb(a.received) : "…") : a.status;
    return h("div", { class: "arow", title: a.message || "" },
      h("b", {}, a.source),
      h("div", { class: `bar ${cls} ${indet ? "indet" : ""}` }, h("div", { style: `width:${pct}%` })),
      h("i", {}, a.message ? `${right} · ${a.message}` : right));
  }));
}

function logItem(cls, head, who, at, body, code) {
  return h("div", { class: "tp " + cls },
    h("div", { class: "h" }, h("b", {}, head), who ? h("span", {}, who) : null, h("time", { datetime: at, title: new Date(at).toLocaleString() }, timeOf(at))),
    body ? h("p", {}, body) : null, code ? h("code", {}, code) : null);
}
function fill(sel, items) { const e = $(sel); e.replaceChildren(...(items.length ? items : [h("div", { class: "hint" }, t("none"))])); }

function renderSide() {
  const rej = S.queue.rejected || [];
  $("#rejCount").textContent = rej.length || "";
  fill("#rejLog", rej.slice(0, 40).map(r => logItem("err", t("r_" + r.code), "@" + r.user, r.at, r.detail, r.input)));
  fill("#noteLog", (S.notices || []).slice(0, 30).map(n => logItem({ error: "err", warn: "warn" }[n.kind] || "info", n.kind, "", n.at, n.text)));
  fill("#histLog", (S.queue.history || []).map(p => logItem(p.result === "played" ? "ok" : "warn", p.title, "@" + p.user, p.at, t(p.result))));
  const raw = (S.raw && S.raw.length ? S.raw : Object.values(S.firstRaw || {})).slice(0, 30);
  fill("#rawLog", raw.map(r => logItem("info", r.event, "", r.at, "", r.raw)));
  const ov = location.origin + "/overlay";
  const kv = $("#infoKv");
  kv.replaceChildren(
    h("dt", {}, t("version")), h("dd", {}, S.version),
    h("dt", {}, t("mode")), h("dd", {}, S.playMode),
    h("dt", {}, t("cache")), h("dd", {}, S.cacheMb.toFixed(1) + " MB"),
    h("dt", {}, t("overlayUrl")), h("dd", {}, h("button", { class: "mlink", onclick: () => copy(ov) }, ov)),
    S.extra ? [h("dt", {}, t("folder")), h("dd", {}, S.extra.baseDir)] : []);
}

function copy(text) {
  navigator.clipboard.writeText(text).then(() => toast(t("copied")), () => toast(text));
}

/* ---------- live connection ---------- */
let ws, wsRetry = 0, offlineT;
function connect() {
  ws = new WebSocket((location.protocol === "https:" ? "wss://" : "ws://") + location.host + "/ws");
  ws.onopen = () => { wsRetry = 0; clearTimeout(offlineT); };
  ws.onmessage = e => {
    const prev = S;
    S = JSON.parse(e.data);
    // Chat rejections are easy to miss in the side panel; flash the newest one.
    const r = (S.queue.rejected || [])[0], p = prev && (prev.queue.rejected || [])[0];
    if (prev && r && !r.test && (!p || r.at !== p.at)) toast(`@${r.user} ${t("rejectedToast")}: ${t("r_" + r.code)} (${r.detail})`, true);
    render();
  };
  ws.onclose = () => {
    offlineT = setTimeout(() => toast(t("offline"), true), 3000);
    setTimeout(connect, Math.min(5000, 500 * 2 ** wsRetry++));
  };
}

/* ---------- top bar & forms ---------- */
$("#reqSw").addEventListener("change", () => act("/api/action/toggleRequests"));
$("#langSel").addEventListener("change", e => { LANG = e.target.value; langPicked = true; try { localStorage.setItem("osureq.lang", LANG); } catch {} applyLang(); render(); });
$("#rmLastBtn").addEventListener("click", () => act("/api/action/removeLast"));
$("#clearBtn").addEventListener("click", () => { if (confirm(t("confirmClear"))) act("/api/action/clear"); });
/* ---------- test panel ---------- */
let testMode = "chat";
try { testMode = localStorage.getItem("osureq.testMode") || "chat"; } catch {}
function renderTestHint() {
  document.querySelectorAll("#testMode button").forEach(b => b.classList.toggle("on", b.dataset.mode === testMode));
  const cmd = S ? S.command : "!req";
  $("#testHint").textContent = t(testMode === "chat" ? "hintChat" : "hintQuick", { cmd }) + (S && S.dryRun ? t("hintDry") : "");
}
$("#testMode").addEventListener("click", e => {
  const b = e.target.closest("button[data-mode]");
  if (!b) return;
  testMode = b.dataset.mode;
  try { localStorage.setItem("osureq.testMode", testMode); } catch {}
  renderTestHint();
});
$("#dryRunSw").addEventListener("change", () => act("/api/action/toggleDryRun"));
$("#clearTestBtn").addEventListener("click", () => { if (confirm(t("confirmClearTest"))) act("/api/action/clearTest"); });
$("#testForm").addEventListener("submit", async e => {
  e.preventDefault();
  let input = $("#testIn").value.trim();
  if (!input) return;
  const user = $("#testUser").value;
  const btn = e.submitter; btn.disabled = true;
  try {
    let j;
    if (testMode === "chat") {
      if (!input.startsWith("!")) input = (S ? S.command : "!req") + " " + input;
      j = await api("POST", "/api/simulate", { user, message: input });
    } else {
      j = await api("POST", "/api/test", { input, user });
    }
    if (j.ok) { toast(t("added", { t: `${j.item.artist} - ${j.item.title} [${j.item.version}]` })); $("#testIn").value = ""; }
    else if (j.code === "ignored") toast(t("ignored"), true);
    else toast((j.code ? t("r_" + j.code) + ": " : "") + j.error, true);
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
    F("connection.logRawEvents", "switch", "Log raw events", "บันทึกอีเวนต์ดิบ", "Shows every TikFinity message in the side panel. Use it to check field names.", "แสดงทุกข้อความจาก TikFinity ที่แผงข้าง ใช้ตรวจชื่อฟิลด์"),
  ],
  osu: [
    F("osu.game", "seg", "Game", "เกม", "lazer has no Songs folder, so maps are always opened.", "lazer ไม่มีโฟลเดอร์ Songs จะใช้วิธีเปิดไฟล์เสมอ", { options: [["stable", "stable"], ["lazer", "lazer"]] }),
    F("osu.folder", "text", "osu! folder", "โฟลเดอร์ osu!", "stable: the folder with osu!.exe and Songs.", "stable: โฟลเดอร์ที่มี osu!.exe และ Songs"),
    F("osu.cacheFolder", "text", "Cache folder", "โฟลเดอร์แคช", "Relative paths are next to the exe.", "ถ้าเป็นพาธสัมพัทธ์จะอยู่ข้าง exe"),
    F("osu.downloadsFolder", "text", "Browser Downloads folder", "โฟลเดอร์ดาวน์โหลดของเบราว์เซอร์", "Watched for official osu! downloads.", "ใช้รอไฟล์ที่ดาวน์โหลดจาก osu! โดยตรง"),
    F("osu.useOsuDb", "switch", "Skip maps already in osu!", "ข้ามแมพที่มีใน osu! แล้ว", "stable only: reads osu!.db and opens osu://b/{id} instead of downloading. Best effort.", "เฉพาะ stable: อ่าน osu!.db แล้วเปิด osu://b/{id} แทนการดาวน์โหลด"),
  ],
  command: [
    F("command.enabled", "switch", "Accept requests", "รับคำขอ"),
    F("command.name", "text", "Command", "คำสั่ง", "Must start with !", "ต้องขึ้นต้นด้วย !"),
    F("command.aliases", "list", "Aliases", "คำสั่งอื่น", "Comma separated, e.g. !request, !r", "คั่นด้วยจุลภาค เช่น !request, !r"),
    F("command.bareNumberAs", "seg", "A bare number is a…", "ตัวเลขเปล่าๆ หมายถึง…", null, null, { options: [["beatmap", ["beatmap (difficulty) ID", "ID บีทแมพ (ระดับ)"]], ["set", ["beatmapset ID", "ID เซ็ต"]]] }),
  ],
  rules: [
    F("rules.minStars", "number", "Min stars", "ดาวขั้นต่ำ", "0 = no limit", "0 = ไม่จำกัด", { step: 0.1, min: 0 }),
    F("rules.maxStars", "number", "Max stars", "ดาวสูงสุด", "0 = no limit", "0 = ไม่จำกัด", { step: 0.1, min: 0 }),
    F("rules.maxLengthSec", "number", "Max length (s)", "ความยาวสูงสุด (วินาที)", "0 = no limit", "0 = ไม่จำกัด", { min: 0 }),
    F("rules.modes", "multi", "Allowed modes", "โหมดที่อนุญาต", null, null, { options: MODES }),
    F("rules.statuses", "multi", "Allowed statuses", "สถานะที่อนุญาต", null, null, { options: STATUSES.map(s => [s, s]) }),
    F("rules.blacklistSets", "numlist", "Blacklisted set IDs", "ID เซ็ตที่แบน", "Comma separated", "คั่นด้วยจุลภาค"),
    F("rules.blacklistMappers", "list", "Blacklisted mappers", "แมพเปอร์ที่แบน", "Comma separated osu! usernames", "ชื่อผู้ใช้ osu! คั่นด้วยจุลภาค"),
    F("rules.blacklistUsers", "list", "Blacklisted TikTok users", "ผู้ใช้ TikTok ที่แบน", "Comma separated @uniqueId", "@uniqueId คั่นด้วยจุลภาค"),
  ],
  queue: [
    F("queue.maxSize", "number", "Max queue size", "คิวสูงสุด", "0 = no limit", "0 = ไม่จำกัด", { min: 0 }),
    F("queue.perUserLimit", "number", "Requests per user", "คำขอต่อคน", "Maps waiting in the queue (the one playing does not count). 0 = no limit", "แมพที่รออยู่ในคิว (ไม่นับเพลงที่กำลังเล่น) 0 = ไม่จำกัด", { min: 0 }),
    F("queue.cooldownSec", "number", "Per-user cooldown (s)", "คูลดาวน์ต่อคน (วินาที)", null, null, { min: 0 }),
    F("rules.exemptUsers", "list", "No-limit users", "ผู้ใช้ที่ไม่จำกัด", "Comma separated @uniqueId (you, your mods). They skip the per-user limit and cooldown.", "@uniqueId คั่นด้วยจุลภาค (คุณ หรือม็อด) ไม่ติดจำนวนคำขอต่อคนและคูลดาวน์"),
    F("queue.noRepeatMin", "number", "No repeat within (min)", "ห้ามซ้ำภายใน (นาที)", null, null, { min: 0 }),
    F("queue.playMode", "seg", "Play mode", "โหมดเล่น", "auto opens the next map once the current one has had its length plus the gap below.", "auto จะเปิดแมพถัดไปเมื่อแมพปัจจุบันเล่นครบความยาวบวกช่วงพักด้านล่าง", { options: [["manual", ["manual (press Next)", "กดเอง (ถัดไป)"]], ["auto", ["auto", "อัตโนมัติ"]]] }),
    F("queue.autoGapSec", "number", "Auto gap after map end (s)", "ช่วงพักหลังจบแมพ (วินาที)", null, null, { min: 0 }),
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
    F("overlay.visible", "number", "Upcoming maps shown", "จำนวนแมพถัดไปที่แสดง", null, null, { min: 0, max: 20 }),
    F("overlay.showRequester", "switch", "Show requester", "แสดงผู้ขอ"),
    F("overlay.theme", "seg", "Theme", "ธีม", null, null, { options: [["pink", "pink"], ["sky", "sky"], ["mono", "mono"]] }),
    F("overlay.scale", "number", "Size (%)", "ขนาด (%)", null, null, { min: 50, max: 250 }),
    F("overlay.position", "select", "Position", "ตำแหน่ง", null, null, { options: [["top-left", "top left"], ["top-right", "top right"], ["bottom-left", "bottom left"], ["bottom-right", "bottom right"]] }),
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
  for (const f of SCHEMA[tab]) box.append(field(f));
  body.replaceChildren(box);
}

function field(f) {
  const v = f.path ? getP(draft, f.path) : null;
  const text = h("div", { class: "swt" }, h("b", {}, lbl(f.label)), f.hint ? h("small", {}, lbl(f.hint)) : null);
  const set = x => setP(draft, f.path, x);
  switch (f.type) {
    case "switch": {
      const i = h("input", { type: "checkbox", class: "switch", "aria-label": lbl(f.label) }); i.checked = !!v;
      i.addEventListener("change", () => set(i.checked));
      return h("label", { class: "row", style: "cursor:pointer" }, text, i);
    }
    case "text": case "hotkey": {
      const i = h("input", { type: "text", value: v ?? "", "aria-label": lbl(f.label), spellcheck: "false" });
      i.addEventListener("input", () => set(i.value));
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
      const c = h("div", { class: "chips" });
      const draw = () => {
        const cur = getP(draft, f.path) || [];
        c.replaceChildren(...f.options.map(([val, name]) => h("button", { type: "button", class: "chip" + (cur.includes(val) ? " on" : ""), onclick: () => {
          set(cur.includes(val) ? cur.filter(x => x !== val) : [...cur, val]); draw();
        } }, name)));
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
  const key = { connection: "connection", osu: "osu", command: "command", rules: "rules", queue: "queue", download: "download", md5: "md5", import: "import", overlay: "overlay", hotkeys: "hotkeys" }[tab];
  draft[key] = structuredClone(defaults[key]);
  renderSettings();
});
$("#setSave").addEventListener("click", async e => {
  draft.language = LANG;
  const btn = e.currentTarget; btn.disabled = true;
  try {
    const j = await api("PUT", "/api/config", draft);
    if (j.ok) { toast(j.restartNeeded ? t("restart") : t("saved")); closeSettings(); }
    else toast(j.error, true);
  } finally { btn.disabled = false; }
});

applyLang();
renderTestHint();
connect();
