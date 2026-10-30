package mods

import (
	"errors"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	ok := map[string]string{
		"HDHR":   "HDHR",
		"+hrhd":  "HDHR",
		"hd,dt":  "HDDT",
		"DTNC":   "NC",
		"SDPF":   "PF",
		"nm":     "",
		"":       "",
		"+EZFL":  "EZFL",
		"FLHDHR": "HDHRFL",
		"ht":     "HT",
	}
	for in, want := range ok {
		got, err := Parse(in)
		if err != nil || got != want {
			t.Errorf("Parse(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	if got, err := Parse("HD", "DT"); err != nil || got != "HDDT" {
		t.Errorf("separate words: %q %v", got, err)
	}
	bad := map[string]string{"EZHR": "conflict", "HTDT": "conflict", "NFSD": "conflict", "HDX": "unknown", "XY": "unknown", "HDAP": "unknown", "please": "unknown"}
	for in, code := range bad {
		_, err := Parse(in)
		var e *Error
		if !errors.As(err, &e) || e.Code != code {
			t.Errorf("Parse(%q) = %v, want %s", in, err, code)
		}
	}
}

func TestHelpers(t *testing.T) {
	if Rate("HDDT") != 1.5 || Rate("NC") != 1.5 || Rate("HT") != 0.75 || Rate("HDHR") != 1 {
		t.Error("rate")
	}
	if !Harder("NC") || !Harder("HDHR") || Harder("HDEZ") || Harder("") {
		t.Error("harder")
	}
	if got := strings.Join(Missing("HDHR", "HD"), ","); got != "HR" {
		t.Errorf("missing = %q", got)
	}
	if got := Missing("HDDT", "HDNC"); len(got) != 0 {
		t.Errorf("NC covers DT: %v", got)
	}
	if got := Missing("HD", "HDHR"); len(got) != 0 {
		t.Errorf("extra mods are not missing: %v", got)
	}
}
