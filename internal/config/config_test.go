package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
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

func TestTTSEngine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(path, []byte(`{"tts":{"lang":"th"}}`), 0o644)
	s, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if e := s.Get().TTS.Engine; e != "auto" {
		t.Errorf("a config from before tts.engine should get auto, got %q", e)
	}
	c := Default()
	c.TTS.Engine = "espeak"
	if c.Validate() == nil {
		t.Error("an unknown TTS engine should not validate")
	}
}

func TestAnnounceOnByDefault(t *testing.T) {
	if !Default().TTS.Announce {
		t.Error("request announcements should be on in a new config")
	}
	path := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(path, []byte(`{"tts":{"announce":false}}`), 0o644)
	s, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if s.Get().TTS.Announce {
		t.Error("a streamer who turned announcements off must keep them off")
	}
}

func TestAutoUpdate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(path, []byte(`{"connection":{"checkUpdates":true}}`), 0o644)
	s, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !s.Get().Connection.AutoUpdate {
		t.Error("a config from before automatic updates should get them")
	}
	os.WriteFile(path, []byte(`{"connection":{"autoUpdate":false}}`), 0o644)
	if s, _ := Load(path); s.Get().Connection.AutoUpdate {
		t.Error("a streamer who turned automatic updates off must keep them off")
	}
}

func TestOldSourcesGetNewMirrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(path, []byte(`{"download":{"sources":[
		{"name":"catboy","enabled":false,"url":"https://catboy.best/d/{id}","timeoutSec":30},
		{"name":"osu.direct","enabled":true,"url":"https://osu.direct/api/d/{id}","timeoutSec":60},
		{"name":"official","enabled":true,"url":"https://osu.ppy.sh/beatmapsets/{id}/download","timeoutSec":180,"official":true}]}}`), 0o644)
	s, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, src := range s.Get().Download.Sources {
		names = append(names, src.Name)
		if src.Name == "catboy" && (src.Enabled || src.TimeoutSec != 30) {
			t.Errorf("a mirror the streamer had keeps their settings: %+v", src)
		}
	}
	if got := strings.Join(names, " "); got != "hinai nekoha beatconnect osu.direct catboy sayobot official" {
		t.Errorf("sources = %s", got)
	}

	// A list saved after that is what the streamer chose: a mirror they
	// removed stays removed.
	c := s.Get()
	c.Download.Sources = c.Download.Sources[3:]
	c.Download.SourcesRev = 0
	if err := s.Set(c); err != nil {
		t.Fatal(err)
	}
	s, err = Load(path)
	if err != nil || len(s.Get().Download.Sources) != 4 {
		t.Errorf("a saved list should not get mirrors back: %v %+v", err, s.Get().Download.Sources)
	}
}

func TestHostAllowed(t *testing.T) {
	for u, want := range map[string]bool{
		"https://mirror.hinamizawa.ai/api/v1/hinai/d/1":                true,
		"https://mirror.nekoha.moe/api/download/1?noVideo=1":           true,
		"https://beatconnect.io/b/1/":                                  true,
		"https://tc1.sayobot.cn:25225/beatmaps/0/1/novideo?filename=x": true,
		"https://sayobot.cn.evil.example/x":                            false,
		"https://evilsayobot.cn/x":                                     false,
		"https://.sayobot.cn/x":                                        false,
		"https://osu.ppy.sh@evil.example/x":                            false,
		"https://osu.ppy.sh:abc/x":                                     false,
		"http://catboy.best/d/1":                                       false,
		"https://api.nerinyan.moe/d/1":                                 false,
	} {
		if got := HostAllowed(u); got != want {
			t.Errorf("HostAllowed(%q) = %v, want %v", u, got, want)
		}
	}
	for _, s := range DefaultSources() {
		for _, u := range []string{s.URL, s.NoVideoURL} {
			if u != "" && !HostAllowed(strings.ReplaceAll(u, "{id}", "1")) {
				t.Errorf("default source %s uses a host that is not allowed: %s", s.Name, u)
			}
		}
	}
}
