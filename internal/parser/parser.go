// Package parser turns a chat comment into a beatmap request.
package parser

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

type Kind int

const (
	KindNone Kind = iota
	KindBeatmap
	KindSet
)

func (k Kind) String() string {
	switch k {
	case KindBeatmap:
		return "beatmap"
	case KindSet:
		return "set"
	}
	return "none"
}

// Request is what a viewer asked for. Exactly one of BeatmapID / SetID is
// guaranteed; links of the form /beatmapsets/{set}#{mode}/{map} carry both.
type Request struct {
	Kind      Kind
	SetID     int
	BeatmapID int
	Mode      string // only from #mode fragments
	Bare      bool   // a plain number: its kind is a guess (see Options.BareNumberAs)
}

type Options struct {
	Command      string
	Aliases      []string
	BareNumberAs string // "beatmap" (default) or "set"
}

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
	return ParseArg(fields[1], o.BareNumberAs)
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
