package app

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/mods"
	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/queue"
)

// Spoken announcements (config tts.announce): what happens to a request is
// said aloud from the streamer's templates, e.g. "{user} ขอเพลง {title}".
//
// Placeholders: {user} {title} {artist} {mapper} {diff} {mods} {stars}
// {pos} and, for rejections, {reason} {max} {min} {length} {wait}; for
// results {acc} {rank} {combo} {misses} {pp}; for gifts {coins} {gift}. Unknown or
// empty ones are left out.

var rePlaceholder = regexp.MustCompile(`\{([a-z]+)\}`)

// RenderAnnouncement fills a template. "@" is dropped (voices read it as
// "at"), and so are the spaces an empty placeholder leaves behind.
func RenderAnnouncement(tpl string, vars map[string]string) string {
	out := rePlaceholder.ReplaceAllStringFunc(tpl, func(m string) string { return vars[m[1:len(m)-1]] })
	out = strings.ReplaceAll(out, "@", "")
	return strings.TrimSpace(reSpaces.ReplaceAllString(out, " "))
}

// announceKey picks the template for a feed event ("" = nothing to say).
func announceKey(e FeedEvent) string {
	switch e.Kind {
	case "added", "nowplaying":
		return e.Kind
	case "rejected":
		return "rejected_" + e.Code
	case "result":
		return "result_" + e.Code // passed | failed | quit
	case "giftthanks": // once per gift (a gift can also make several "gift" events)
		return "gift"
	case "command":
		if e.Code == "voteskip" {
			return "voteskip"
		}
	case "status":
		if e.Code == "time_off" {
			return "auto_off"
		}
		return e.Code
	}
	return ""
}

// announce speaks a feed event if it has a template.
func (a *App) announce(e FeedEvent) {
	t := a.Cfg.Get().TTS
	key := announceKey(e)
	if !t.Announce || key == "" {
		return
	}
	tpl, ok := t.Templates[key]
	if !ok && strings.HasPrefix(key, "rejected_") {
		tpl = t.Templates["rejected"]
	}
	tpl = strings.TrimSpace(tpl)
	if tpl == "" || tpl == "-" {
		return
	}
	vars := map[string]string{"user": speakName(e.Nick, e.User), "title": e.Title, "artist": e.Artist, "mapper": e.Mapper,
		"diff": e.Version, "mods": spokenMods(e.Mods)}
	if e.Stars > 0 {
		vars["stars"] = strconv.FormatFloat(e.Stars, 'f', 2, 64)
	}
	if e.Pos > 0 {
		vars["pos"] = strconv.Itoa(e.Pos)
	}
	if e.Kind == "rejected" {
		vars["reason"] = reasonText(e.Code, hasThai(tpl))
	}
	for k, v := range e.Vars {
		if k == "mods" {
			v = spokenMods(v)
		}
		vars[k] = v
	}
	if text := RenderAnnouncement(tpl, vars); text != "" {
		a.queueSpeech(Speech{User: e.User, Text: text, Test: e.Test, Kind: "say"})
	}
}

// announcePlaying says which request opens now (template "nowplaying").
func (a *App) announcePlaying(it queue.Item) {
	a.announce(FeedEvent{Kind: "nowplaying", User: it.User, Nick: it.Nick, Title: it.Title, Artist: it.Artist, Mapper: it.Creator,
		Version: it.Version, Mods: it.Mods, Stars: it.Stars, Test: it.Test})
}

func speakName(nick, user string) string {
	if n := SpeakableText(nick, 40); n != "" {
		return n
	}
	return user
}

// spokenMods spaces mods so voices spell them: "HDHR" -> "HD HR".
func spokenMods(m string) string { return strings.Join(mods.List(m), " ") }

func hasThai(s string) bool {
	return strings.IndexFunc(s, func(r rune) bool { return unicode.Is(unicode.Thai, r) }) >= 0
}

// reasons are spoken {reason}s, English and Thai, by rejection code.
var reasons = map[string][2]string{
	"user_blacklisted":   {"you cannot request right now", "ตอนนี้ขอเพลงไม่ได้"},
	"queue_full":         {"the queue is full", "คิวเต็มแล้ว"},
	"user_limit":         {"you already have requests waiting", "มีคำขอรออยู่แล้ว"},
	"cooldown":           {"please wait a little", "รอสักครู่ก่อนนะ"},
	"followers_only":     {"follow the stream to request", "ต้องกดติดตามก่อน"},
	"requests_off":       {"requests are closed", "ปิดรับคำขออยู่"},
	"set_blacklisted":    {"that map is blocked", "แมพนี้ถูกบล็อก"},
	"mapper_blacklisted": {"that mapper is blocked", "แมพเปอร์นี้ถูกบล็อก"},
	"song_blacklisted":   {"that song is blocked", "เพลงนี้ถูกบล็อก"},
	"artist_blacklisted": {"that artist is blocked", "ศิลปินนี้ถูกบล็อก"},
	"status":             {"that map status is not allowed", "สถานะแมพนี้ไม่รับ"},
	"now_playing":        {"it is playing right now", "เพลงนี้กำลังเล่นอยู่"},
	"duplicate":          {"it is already in the queue", "อยู่ในคิวแล้ว"},
	"recently_played":    {"it was played recently", "เพิ่งเล่นไป"},
	"not_found":          {"the map was not found", "ไม่พบแมพ"},
	"mode":               {"that mode is not accepted", "ไม่รับโหมดนี้"},
	"no_convert":         {"that mode cannot be converted", "แปลงโหมดไม่ได้"},
	"stars_low":          {"it is too easy", "ง่ายเกินไป"},
	"stars_high":         {"it is too hard", "ยากเกินไป"},
	"mod_stars":          {"it is too hard with those mods", "ยากเกินไปเมื่อใส่ mod"},
	"too_long":           {"it is too long", "ยาวเกินไป"},
	"meta_error":         {"the map could not be checked", "ตรวจแมพไม่ได้"},
	"bad_input":          {"that is not a map", "ไม่ใช่แมพ"},
	"search_none":        {"no map has that name", "ไม่พบแมพชื่อนี้"},
	"search_error":       {"the search did not work", "ค้นหาไม่ได้"},
	"needs_gift":         {"send a gift to request", "ต้องส่งของขวัญก่อน"},
	"bad_mods":           {"those mods do not work", "mod ไม่ถูกต้อง"},
	"mods_off":           {"mods are not allowed", "ไม่รับคำขอที่มี mod"},
	"mod_not_allowed":    {"that mod is not allowed", "ไม่อนุญาต mod นี้"},
}

func reasonText(code string, thai bool) string {
	r, ok := reasons[code]
	if !ok {
		return ""
	}
	if thai {
		return r[1]
	}
	return r[0]
}
