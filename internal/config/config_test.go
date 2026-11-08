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

func TestTTSVoicesAndGender(t *testing.T) {
	d := Default()
	if d.TTS.Gender != "any" || d.TTS.Voices == nil || len(d.TTS.Voices) != 0 {
		t.Errorf("defaults: gender any, no chosen voices: %q %v", d.TTS.Gender, d.TTS.Voices)
	}

	// a config.json from before the new keys gets the defaults
	path := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(path, []byte(`{"tts":{"lang":"auto","voice":"Microsoft Zira - English (United States)"}}`), 0o644)
	s, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if tts := s.Get().TTS; tts.Gender != "any" || len(tts.Voices) != 0 || tts.Voice != "Microsoft Zira - English (United States)" {
		t.Errorf("old config: %+v", tts)
	}

	// with the new keys: loaded, saved and loaded again unchanged
	path = filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(path, []byte(`{"tts":{"gender":"female","voices":{"th":"Microsoft Premwadee Online (Natural) - Thai (Thailand)","ja":"Microsoft Nanami Online (Natural) - Japanese (Japan)"}}}`), 0o644)
	if s, err = Load(path); err != nil {
		t.Fatal(err)
	}
	c := s.Get()
	if c.TTS.Gender != "female" || c.TTS.Voices["th"] != "Microsoft Premwadee Online (Natural) - Thai (Thailand)" || len(c.TTS.Voices) != 2 {
		t.Errorf("new keys: %+v", c.TTS)
	}
	c.TTS.Gender, c.TTS.Voices["hi"] = "male", "Microsoft Madhur Online (Natural) - Hindi (India)"
	if err := s.Set(c); err != nil {
		t.Fatal(err)
	}
	s2, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := s2.Get().TTS; got.Gender != "male" || len(got.Voices) != 3 || got.Voices["hi"] != "Microsoft Madhur Online (Natural) - Hindi (India)" {
		t.Errorf("round trip: %+v", got)
	}

	c = Default()
	c.TTS.Gender = ""
	if err := c.Validate(); err != nil || c.TTS.Gender != "any" {
		t.Errorf("an empty gender is any: %v %q", err, c.TTS.Gender)
	}
	c.TTS.Voices = nil
	if err := c.Validate(); err != nil {
		t.Errorf("voices may be null: %v", err)
	}
	c.TTS.Voices = map[string]string{"en": ""}
	if err := c.Validate(); err != nil {
		t.Errorf("an empty name is the automatic choice: %v", err)
	}

	code := func(c Config) string {
		var ve *ValidationError
		if err := c.Validate(); errors.As(err, &ve) && len(ve.Problems) == 1 {
			return ve.Problems[0].Code
		}
		return "not one problem"
	}
	bad := map[string]func(c *Config){
		"gender other":     func(c *Config) { c.TTS.Gender = "robot" },
		"gender caps":      func(c *Config) { c.TTS.Gender = "Female" },
		"voices language":  func(c *Config) { c.TTS.Voices = map[string]string{"vi": "Microsoft HoaiMy Online (Natural)"} },
		"voices long name": func(c *Config) { c.TTS.Voices = map[string]string{"th": strings.Repeat("ก", 121)} },
		"voices control":   func(c *Config) { c.TTS.Voices = map[string]string{"en": "Microsoft Guy\nOnline"} },
	}
	want := map[string]string{"gender other": "tts_gender", "gender caps": "tts_gender", "voices language": "tts_voices", "voices long name": "tts_voices", "voices control": "tts_voices"}
	for name, f := range bad {
		c := Default()
		f(&c)
		if got := code(c); got != want[name] {
			t.Errorf("%s: got %q, want %q", name, got, want[name])
		}
		for _, p := range c.Validate().(*ValidationError).Problems {
			if strings.Contains(p.Msg, "robot") || strings.Contains(p.Msg, "HoaiMy") || strings.Contains(p.Msg, "Guy") || len(p.Vars) != 0 {
				t.Errorf("%s: the message must not quote the value: %+v", name, p)
			}
		}
	}
	c = Default()
	c.TTS.Voices = map[string]string{"th": strings.Repeat("ก", 120), "zh": "Microsoft Xiaoxiao Online (Natural) - Chinese (Mainland)"}
	if err := c.Validate(); err != nil {
		t.Errorf("120 letters, spaces, parentheses and hyphens are fine: %v", err)
	}

	// the new languages and genders are new keys only: older fields keep their values
	for _, v := range []string{"ja", "ko", "zh", "hi", "female"} {
		c := Default()
		c.TTS.Lang = v
		if code(c) != "tts_lang" {
			t.Errorf("tts.lang must still refuse %q", v)
		}
	}
	for _, v := range []string{"edge", "female", "ja"} {
		c := Default()
		c.TTS.Engine = v
		if code(c) != "tts_engine" {
			t.Errorf("tts.engine must still refuse %q", v)
		}
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

func TestKikiPlayerOnByDefault(t *testing.T) {
	if d := Default().Osu; !d.KikiPlayer || !d.KikiPlayerMod {
		t.Errorf("the map player should be on in a new config: %+v", d)
	}
	path := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(path, []byte(`{"osu":{"game":"stable","viewerUrl":""}}`), 0o644)
	s, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if o := s.Get().Osu; !o.KikiPlayer || !o.KikiPlayerMod {
		t.Errorf("a config from before the map player should get it on: %+v", o)
	}
	c := s.Get()
	c.Osu.KikiPlayer, c.Osu.KikiPlayerMod = false, false
	if err := s.Set(c); err != nil {
		t.Fatal(err)
	}
	s2, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if o := s2.Get().Osu; o.KikiPlayer || o.KikiPlayerMod {
		t.Errorf("a streamer who turned the map player off must keep it off: %+v", o)
	}
}

func TestViewerURL(t *testing.T) {
	if v := Default().Osu.ViewerURL; v != "https://kiki-bm.app/" {
		t.Errorf("the default beatmap viewer should be KIKI Beatmap Viewer, got %q", v)
	}
	path := filepath.Join(t.TempDir(), "config.json")
	for _, c := range []struct{ saved, want string }{
		{`"https://osu-beatmap-viewer.vercel.app/"`, "https://kiki-bm.app/"}, // the old default
		{`""`, ""}, // button hidden
		{`"https://osu-beatmap-viewer.vercel.app"`, "https://osu-beatmap-viewer.vercel.app"}, // typed by hand
		{`"https://my-viewer.example/"`, "https://my-viewer.example/"},
		{`"https://player.kiki-bm.app/s/{set}?file={file}&diff={map}"`, "https://player.kiki-bm.app/s/{set}?file={file}&diff={map}"},
	} {
		raw := []byte(`{"osu":{"game":"stable","viewerUrl":` + c.saved + `}}`)
		os.WriteFile(path, raw, 0o644)
		s, err := Load(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := s.Get().Osu.ViewerURL; got != c.want {
			t.Errorf("viewerUrl %s: got %q, want %q", c.saved, got, c.want)
		}
		if b, _ := os.ReadFile(path); string(b) != string(raw) {
			t.Errorf("viewerUrl %s: Load should not rewrite config.json", c.saved)
		}
	}
	os.WriteFile(path, []byte(`{"osu":{"game":"stable"}}`), 0o644)
	if s, _ := Load(path); s.Get().Osu.ViewerURL != "https://kiki-bm.app/" {
		t.Errorf("a config without viewerUrl should get the default, got %q", s.Get().Osu.ViewerURL)
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

func TestMaxPerMessage(t *testing.T) {
	if n := Default().Command.MaxPerMessage; n != 1 {
		t.Errorf("default maps per message = %d, want 1", n)
	}
	path := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(path, []byte(`{"command":{"name":"!song","enabled":true,"bareNumberAs":"set"}}`), 0o644)
	s, err := Load(path)
	if err != nil {
		t.Fatalf("a config from before maps per message must load: %v", err)
	}
	if c := s.Get().Command; c.MaxPerMessage != 1 || c.Name != "!song" {
		t.Errorf("an old config gets one map per message: %+v", c)
	}
	for _, n := range []int{0, 6, -1} {
		c := Default()
		c.Command.MaxPerMessage = n
		var ve *ValidationError
		if err := c.Validate(); !errors.As(err, &ve) || ve.Problems[0].Code != "max_per_message" {
			t.Errorf("%d maps per message: want max_per_message, got %v", n, err)
		}
	}
	for _, n := range []int{1, 3, 5} {
		c := Default()
		c.Command.MaxPerMessage = n
		if err := c.Validate(); err != nil {
			t.Errorf("%d maps per message should be valid: %v", n, err)
		}
	}
}

func TestOverlayLinkSettings(t *testing.T) {
	if o := Default().Overlay; o.Remote || o.RemoteToken != "" {
		t.Errorf("the overlay link is off and has no secret by default: %+v", o)
	}
	dir := t.TempDir()
	// a config from before the overlay link
	old := filepath.Join(dir, "old.json")
	os.WriteFile(old, []byte(`{"overlay":{"visible":3,"theme":"sky"}}`), 0o644)
	s, err := Load(old)
	if err != nil {
		t.Fatal(err)
	}
	if o := s.Get().Overlay; o.Remote || o.RemoteToken != "" || o.Visible != 3 {
		t.Errorf("old config: %+v", o)
	}

	// switched on by hand in config.json: the secret is made and saved once
	hand := filepath.Join(dir, "hand.json")
	os.WriteFile(hand, []byte(`{"overlay":{"remote":true}}`), 0o644)
	s, err = Load(hand)
	if err != nil {
		t.Fatal(err)
	}
	tok := s.Get().Overlay.RemoteToken
	if !GoodOverlayToken(tok) {
		t.Fatalf("token made on load: %q", tok)
	}
	if again, _ := Load(hand); again.Get().Overlay.RemoteToken != tok {
		t.Error("the token made on load must be saved, not made anew on every start")
	}

	a, b := NewOverlayToken(), NewOverlayToken()
	if a == b || len(a) != 43 || !GoodOverlayToken(a) {
		t.Errorf("NewOverlayToken: %q %q", a, b)
	}
	for tok, ok := range map[string]bool{a: true, strings.Repeat("A", 42): false, strings.Repeat("A", 129): false, a[:40] + "a/b": false, a[:42] + "=": false} {
		if GoodOverlayToken(tok) != ok {
			t.Errorf("GoodOverlayToken(%q) = %v", tok, !ok)
		}
	}
	c := Default()
	c.Overlay.RemoteToken = "short-secret-123"
	err = c.Validate()
	var ve *ValidationError
	if !errors.As(err, &ve) || ve.Problems[0].Code != "overlay_token" {
		t.Fatalf("a short token: %v", err)
	}
	if strings.Contains(err.Error(), "short-secret-123") {
		t.Errorf("the error shows the token: %v", err)
	}
	c = Default()
	c.Overlay.Remote = true // the mod panel is off: Settings explains, saving still works
	if err := c.Validate(); err != nil {
		t.Errorf("overlay link on with the mod panel off: %v", err)
	}
}
