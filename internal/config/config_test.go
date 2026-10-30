package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultIsValid(t *testing.T) {
	c := Default()
	if err := c.Validate(); err != nil {
		t.Fatalf("defaults must validate: %v", err)
	}
	if c.MasterVolume != 10 {
		t.Errorf("default master volume = %d, want 10", c.MasterVolume)
	}
}

func TestValidationProblemsHaveCodes(t *testing.T) {
	c := Default()
	c.Connection.DashboardPort = 80
	c.Command.SkipCmd = "skip"
	err := c.Validate()
	var ve *ValidationError
	if !errors.As(err, &ve) || len(ve.Problems) != 2 {
		t.Fatalf("want 2 problems, got %v", err)
	}
	if code, _ := ve.ErrCode(); code != "port_range" {
		t.Errorf("first code = %q", code)
	}
	if p := ve.Problems[1]; p.Code != "mod_cmd" || p.Vars["cmd"] != "skip" {
		t.Errorf("second problem = %+v", p)
	}
}

func TestPerModeStars(t *testing.T) {
	c := Default()
	c.Rules.MinStars, c.Rules.MaxStars = 2, 6
	c.Rules.ModeStars = map[string]StarRange{"mania": {Max: 4}, "taiko": {Min: 3}}
	if min, max, per := c.Rules.StarLimits("mania"); min != 2 || max != 4 || !per {
		t.Errorf("mania = %v %v %v", min, max, per)
	}
	if min, max, per := c.Rules.StarLimits("osu"); min != 2 || max != 6 || per {
		t.Errorf("osu = %v %v %v", min, max, per)
	}
	c.Rules.ModeStars["fruits"] = StarRange{Min: 7} // 7 > global max 6
	c.Rules.ModeStars["bogus"] = StarRange{}
	var ve *ValidationError
	if err := c.Validate(); !errors.As(err, &ve) || len(ve.Problems) != 2 {
		t.Fatalf("want 2 problems, got %v", err)
	}
}

func TestParseSongRule(t *testing.T) {
	cases := map[string]SongRule{
		`artist="a_hisa"`:                 {Artist: "a_hisa"},
		`Artist=“a_hisa”`:                 {Artist: "a_hisa"},
		`ARTIST = 'xi'`:                   {Artist: "xi"},
		`artist=xi`:                       {Artist: "xi"},
		`title="Blue Zenith"`:             {Title: "Blue Zenith"},
		`artist="xi" title="Blue Zenith"`: {Artist: "xi", Title: "Blue Zenith"},
		`title=“Ghost” artist=“Camellia”`: {Artist: "Camellia", Title: "Ghost"},
		`xi - FREEDOM DiVE`:               {Artist: "xi", Title: "FREEDOM DiVE"},
		`FREEDOM DiVE`:                    {Title: "FREEDOM DiVE"},
		`Kani*Do-Luck!`:                   {Title: "Kani*Do-Luck!"},
	}
	for in, want := range cases {
		if got, ok := ParseSongRule(in); !ok || got != want {
			t.Errorf("ParseSongRule(%q) = %+v %v, want %+v", in, got, ok, want)
		}
	}
	for _, bad := range []string{``, `   `, `artist=""`, `artist="xi" blah`, `title=`} {
		if r, ok := ParseSongRule(bad); ok {
			t.Errorf("%q should be refused, got %+v", bad, r)
		}
	}
	c := Default()
	c.Rules.BlacklistSongs = []string{`artist=""`}
	if c.Validate() == nil {
		t.Error("an empty artist rule should not validate")
	}
	if (SongRule{Artist: "a_hisa"}).String() != `artist="a_hisa"` {
		t.Error("String")
	}
}

func TestOldConfigGetsNewAnnouncements(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	os.WriteFile(path, []byte(`{"tts":{"announce":true,"templates":{"added":"{user} requested {title}","rejected":""}}}`), 0o644)
	s, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	tpl := s.Get().TTS.Templates
	if tpl["added"] != "{user} requested {title}" || tpl["rejected"] != "" {
		t.Errorf("the streamer's lines must stay: %v", tpl)
	}
	if tpl["np"] != EnglishAnnouncements()["np"] {
		t.Errorf("a new line should be added in the language of the others: %q", tpl["np"])
	}
}

func TestOldFollowersOnlyBecomesAllow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(path, []byte(`{"tts":{"followersOnly":true}}`), 0o644)
	s, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if tts := s.Get().TTS; len(tts.Allow) != 1 || tts.Allow[0] != "follower" || tts.FollowersOnly {
		t.Errorf("followersOnly should become allow [follower]: %+v", tts)
	}
}
