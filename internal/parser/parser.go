// Package parser turns a chat comment into a beatmap request.
package parser

import (
	"errors"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/mods"
)

type Kind int

const (
	KindNone Kind = iota
	KindBeatmap
	KindSet
	KindSearch // a song name: Query holds the text
)

func (k Kind) String() string {
	switch k {
	case KindBeatmap:
		return "beatmap"
	case KindSet:
		return "set"
	case KindSearch:
		return "search"
	}
	return "none"
}

// Request is what a viewer asked for. Exactly one of BeatmapID / SetID is
// guaranteed; links of the form /beatmapsets/{set}#{mode}/{map} carry both.
type Request struct {
	Kind      Kind
	SetID     int
	BeatmapID int
	Mode      string      // only from #mode fragments
	Bare      bool        // a plain number: its kind is a guess (see Options.BareNumberAs)
	Query     string      // KindSearch: the song name as typed
	Mods      string      // requested mods in osu!'s order, e.g. "HDHR"; "" = none
	ModsErr   *mods.Error // mods were asked for but are unknown or conflict
}

type Options struct {
	Command      string
	Aliases      []string
	BareNumberAs string // "beatmap" (default) or "set"
	Search       bool   // text that is not an ID or link is a song name
}

// Search queries shorter than this are ignored, longer ones are cut.
const (
	MinQuery = 3
	MaxQuery = 80
)

var (
	reSetMap = regexp.MustCompile(`^/beatmapsets/(\d+)/?$`)
	reB      = regexp.MustCompile(`^/(?:b|beatmaps)/(\d+)/?$`)
	reS      = regexp.MustCompile(`^/(?:s|beatmapsets)/(\d+)/?$`)
	reFrag   = regexp.MustCompile(`^(osu|taiko|fruits|mania)/(\d+)$`)
	reDigits = regexp.MustCompile(`^\d{1,10}$`)
)

// Parse returns ok=false for anything that is not a well formed request.
func Parse(comment string, o Options) (Request, bool) {
	fields := strings.Fields(strings.TrimSpace(comment))
	if len(fields) < 2 || !isCommand(fields[0], o) {
		return Request{}, false
	}
	return ParseInput(fields[1:], o.BareNumberAs, o.Search)
}

// ParseInput reads what follows the command. An ID or osu! link in the
// first word wins (anything after it is ignored, as before); otherwise, with
// search on, the words are a song name. Links to other sites are never
// searched for.
func ParseInput(words []string, bareAs string, search bool) (Request, bool) {
	if len(words) == 0 {
		return Request{}, false
	}
	if r, ok := ParseArg(words[0], bareAs); ok {
		r.Mods, r.ModsErr = modsAfter(words[1:])
		return r, true
	}
	if !search {
		return Request{}, false
	}
	// A song name may end with mods: "+HDHR" always, "HDHR" when written in
	// capitals (so "blue zenith hd" stays a name).
	var m string
	var merr *mods.Error
	if last := words[len(words)-1]; len(words) > 1 && (strings.HasPrefix(last, "+") || looksLikeMods(last)) {
		got, err := mods.Parse(last)
		switch {
		case err == nil:
			m, words = got, words[:len(words)-1]
		case strings.HasPrefix(last, "+"):
			merr, words = asModsErr(err), words[:len(words)-1]
		}
	}
	for _, w := range words {
		lw := strings.ToLower(w)
		if strings.Contains(lw, "://") || strings.Contains(lw, "/") || strings.HasPrefix(lw, "www.") {
			return Request{}, false
		}
	}
	q := strings.Join(words, " ")
	if len([]rune(q)) < MinQuery {
		return Request{}, false
	}
	if r := []rune(q); len(r) > MaxQuery {
		q = strings.TrimSpace(string(r[:MaxQuery]))
	}
	return Request{Kind: KindSearch, Query: q, Mods: m, ModsErr: merr}, true
}

// modsAfter reads mods written after an ID or link. Chat often follows a
// request ("so good", "OK", "GG"), and some chat words are mod codes, so:
//   - everything after the ID is mods ("hd hr", "HDHR"), in any case;
//   - or the first word starts with "+" ("+HD please"), or is in capitals and
//     made only of mod codes ("HDDT pls"); more such words may follow.
//
// Anything else is ignored, as before. A word that is meant as mods but is
// wrong ("+HDXX", "EZHR") is an error.
func modsAfter(rest []string) (string, *mods.Error) {
	if len(rest) == 0 {
		return "", nil
	}
	if got, err := mods.Parse(rest...); err == nil {
		return got, nil
	}
	if !strings.HasPrefix(rest[0], "+") && !looksLikeMods(rest[0]) {
		return "", nil
	}
	got, err := mods.Parse(rest[0])
	if err != nil {
		return "", asModsErr(err)
	}
	for n := 1; n < len(rest) && (strings.HasPrefix(rest[n], "+") || looksLikeMods(rest[n])); n++ {
		more, err := mods.Parse(rest[:n+1]...)
		if err != nil {
			break
		}
		got = more
	}
	return got, nil
}

// looksLikeMods: capital letters that are all mod codes ("HDHR", "EZHR"),
// so "OK", "GG" or "LOL" are not taken for mods.
func looksLikeMods(w string) bool {
	if len(w) < 2 || len(w) > 26 || len(w)%2 != 0 {
		return false
	}
	for i := 0; i < len(w); i += 2 {
		if !mods.Known(w[i : i+2]) {
			return false
		}
	}
	return true
}

func asModsErr(err error) *mods.Error {
	var e *mods.Error
	if errors.As(err, &e) {
		return e
	}
	return &mods.Error{Code: "unknown"}
}

func isCommand(word string, o Options) bool {
	w := strings.ToLower(word)
	if w == strings.ToLower(o.Command) {
		return true
	}
	for _, a := range o.Aliases {
		if a != "" && w == strings.ToLower(a) {
			return true
		}
	}
	return false
}

// ParseArg parses a single ID or osu! link (also used by the dashboard test box).
func ParseArg(arg, bareAs string) (Request, bool) {
	arg = strings.Trim(arg, "<>()[]\"'")
	if reDigits.MatchString(arg) {
		id, ok := num(arg)
		if !ok {
			return Request{}, false
		}
		if bareAs == "set" {
			return Request{Kind: KindSet, SetID: id, Bare: true}, true
		}
		return Request{Kind: KindBeatmap, BeatmapID: id, Bare: true}, true
	}

	if !strings.Contains(arg, "://") {
		arg = "https://" + arg
	}
	u, err := url.Parse(arg)
	if err != nil {
		return Request{}, false
	}
	host := strings.ToLower(u.Hostname())
	if host != "osu.ppy.sh" && host != "old.ppy.sh" {
		return Request{}, false
	}
	path := u.Path

	if m := reSetMap.FindStringSubmatch(path); m != nil {
		set, ok := num(m[1])
		if !ok {
			return Request{}, false
		}
		if f := reFrag.FindStringSubmatch(u.Fragment); f != nil {
			if id, ok := num(f[2]); ok {
				return Request{Kind: KindBeatmap, SetID: set, BeatmapID: id, Mode: f[1]}, true
			}
			return Request{}, false
		}
		if u.Fragment != "" && !reFrag.MatchString(u.Fragment) && !isModeOnly(u.Fragment) {
			return Request{}, false
		}
		return Request{Kind: KindSet, SetID: set}, true
	}
	if m := reB.FindStringSubmatch(path); m != nil {
		if id, ok := num(m[1]); ok {
			return Request{Kind: KindBeatmap, BeatmapID: id}, true
		}
		return Request{}, false
	}
	if m := reS.FindStringSubmatch(path); m != nil {
		if id, ok := num(m[1]); ok {
			return Request{Kind: KindSet, SetID: id}, true
		}
	}
	return Request{}, false
}

// Flip returns a bare request reinterpreted as the other kind of ID.
func (r Request) Flip() Request {
	if r.Kind == KindSet {
		return Request{Kind: KindBeatmap, BeatmapID: r.SetID, Bare: true}
	}
	return Request{Kind: KindSet, SetID: r.BeatmapID, Bare: true}
}

func isModeOnly(f string) bool {
	switch f {
	case "osu", "taiko", "fruits", "mania":
		return true
	}
	return false
}

func num(s string) (int, bool) {
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 || n > 1<<31-1 {
		return 0, false
	}
	return n, true
}
