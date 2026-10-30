// Package mods reads the mods a viewer asks for ("HDHR", "+HD DT", "hd,hr")
// and checks they make sense together. Star ratings with mods are not
// known (osu! only gives them through its authenticated API), so the rules
// use the no-mod stars plus a separate limit for mods that make a map harder.
package mods

import (
	"fmt"
	"strings"
)

// known are the mods a request may carry, in osu!'s own order (its bit order),
// which is also how they are written back: "HDHR", "HDDT", "EZHDFL".
var known = []string{"NF", "EZ", "HD", "HR", "SD", "DT", "HT", "NC", "FL", "SO", "PF", "FI", "MR"}

// conflicts are pairs osu! does not allow together.
var conflicts = [][2]string{{"EZ", "HR"}, {"HT", "DT"}, {"HT", "NC"}, {"NF", "SD"}, {"NF", "PF"}, {"FI", "HD"}, {"FL", "FI"}}

// Error says why a mod string was refused; Short is language-neutral.
type Error struct {
	Code  string // unknown | conflict
	Short string // "XX", "EZ+HR"
}

func (e *Error) Error() string { return fmt.Sprintf("mods %s: %s", e.Code, e.Short) }

// Parse reads mods from one or more words. "NM" (or nothing) is no mods.
// NC includes DT and PF includes SD, so "DTNC" becomes "NC". The result is
// written in osu!'s order.
func Parse(words ...string) (string, error) {
	s := strings.ToUpper(strings.Join(words, ""))
	s = strings.NewReplacer("+", "", ",", "", " ", "", "/", "", "|", "").Replace(s)
	if s == "" || s == "NM" || s == "NOMOD" {
		return "", nil
	}
	if len(s)%2 != 0 {
		return "", &Error{Code: "unknown", Short: s}
	}
	have := map[string]bool{}
	for i := 0; i < len(s); i += 2 {
		m := s[i : i+2]
		if !Known(m) {
			return "", &Error{Code: "unknown", Short: m}
		}
		have[m] = true
	}
	if have["NC"] {
		delete(have, "DT")
	}
	if have["PF"] {
		delete(have, "SD")
	}
	for _, c := range conflicts {
		if have[c[0]] && have[c[1]] {
			return "", &Error{Code: "conflict", Short: c[0] + "+" + c[1]}
		}
	}
	var b strings.Builder
	for _, m := range known {
		if have[m] {
			b.WriteString(m)
		}
	}
	return b.String(), nil
}

// Known reports whether m is a mod code this package accepts ("HD", "DT").
func Known(m string) bool {
	for _, k := range known {
		if k == m {
			return true
		}
	}
	return false
}

// List splits "HDHR" into ["HD", "HR"].
func List(mods string) []string {
	var out []string
	for i := 0; i+1 < len(mods); i += 2 {
		out = append(out, mods[i:i+2])
	}
	return out
}

// Has reports whether mods contains m (NC counts as DT, PF as SD).
func Has(mods, m string) bool {
	for _, x := range List(mods) {
		if x == m || (m == "DT" && x == "NC") || (m == "SD" && x == "PF") {
			return true
		}
	}
	return false
}

// Rate is the song speed: 1.5 with DT/NC, 0.75 with HT, else 1.
func Rate(mods string) float64 {
	switch {
	case Has(mods, "DT"):
		return 1.5
	case Has(mods, "HT"):
		return 0.75
	}
	return 1
}

// Harder reports whether mods make a map harder to play than its no-mod
// star rating says (DT, NC, HR), which is what the extra star limit is for.
func Harder(mods string) bool { return Has(mods, "DT") || Has(mods, "HR") }

// Missing returns the requested mods that played (tosu's mod string) lacks.
func Missing(requested, played string) []string {
	var out []string
	for _, m := range List(requested) {
		if !Has(played, m) && !(m == "DT" && Has(played, "NC")) {
			out = append(out, m)
		}
	}
	return out
}
