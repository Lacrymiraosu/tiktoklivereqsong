package tikfinity

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseChat(t *testing.T) {
	cases := []struct {
		raw  string
		ok   bool
		want Chat
	}{
		{`{"comment":"!req 123","uniqueId":"viewer1","nickname":"Viewer"}`, true, Chat{User: "viewer1", Nick: "Viewer", Comment: "!req 123"}},
		{`{"comment":"!req 1","user":{"uniqueId":"nested","nickname":"N"}}`, true, Chat{User: "nested", Nick: "N", Comment: "!req 1"}},
		{`{"comment":"hi","uniqueId":"u"}`, true, Chat{User: "u", Nick: "u", Comment: "hi"}},
		{`{"uniqueId":"u"}`, false, Chat{}},
		{`not json`, false, Chat{}},
	}
	for _, c := range cases {
		got, ok := ParseChat([]byte(c.raw))
		if ok != c.ok || got != c.want {
			t.Errorf("ParseChat(%s) = %+v %v, want %+v %v", c.raw, got, ok, c.want, c.ok)
		}
	}
}

func TestClassify(t *testing.T) {
	cases := []struct {
		st   Status
		want Level
		code string
	}{
		{Status{}, Red, "not_installed"},
		{Status{Installed: true}, Red, "not_running"},
		{Status{Installed: true, Running: true}, Yellow, "connecting"},
		{Status{Installed: true, Running: true, Connected: true, Handshake: true}, Yellow, "waiting_stream"},
		{Status{Installed: true, Running: true, Connected: true, Handshake: true, Live: true}, Green, "live"},
	}
	for _, c := range cases {
		if l, code := classify(c.st); l != c.want || code != c.code {
			t.Errorf("%+v: got %s/%s want %s/%s", c.st, l, code, c.want, c.code)
		}
	}
}

func TestParseChatRoles(t *testing.T) {
	ch, ok := ParseChat([]byte(`{"comment":"!skip","uniqueId":"m","isModerator":true,"isSubscriber":false,"followRole":1}`))
	if !ok || ch.Moderator != Yes || ch.Subscriber != No || ch.Follower != Yes {
		t.Errorf("flat fields: %+v", ch)
	}
	ch, _ = ParseChat([]byte(`{"comment":"x","uniqueId":"m","userIdentity":{"isModeratorOfAnchor":false,"isFollowerOfAnchor":true}}`))
	if ch.Moderator != No || ch.Follower != Yes || ch.Subscriber != Unknown {
		t.Errorf("userIdentity fields: %+v", ch)
	}
	ch, _ = ParseChat([]byte(`{"comment":"x","uniqueId":"m","followRole":0}`))
	if ch.Follower != No {
		t.Errorf("followRole 0 means not following: %+v", ch)
	}
}

func TestParseGift(t *testing.T) {
	g, ok := ParseGift([]byte(`{"uniqueId":"rich","nickname":"Rich","giftName":"Rose","giftType":1,"repeatCount":7,"repeatEnd":true,"diamondCount":1}`))
	if !ok || g.Coins != 7 || g.User != "rich" || g.Name != "Rose" {
		t.Errorf("finished streak: %+v %v", g, ok)
	}
	if _, ok := ParseGift([]byte(`{"uniqueId":"rich","giftType":1,"repeatCount":3,"repeatEnd":false,"diamondCount":1}`)); ok {
		t.Error("a running streak must not count yet")
	}
	g, ok = ParseGift([]byte(`{"user":{"uniqueId":"big"},"giftName":"Lion","giftType":2,"repeatCount":1,"diamondCount":29999}`))
	if !ok || g.Coins != 29999 || g.User != "big" {
		t.Errorf("non-streak gift: %+v %v", g, ok)
	}
	if _, ok := ParseGift([]byte(`{"uniqueId":"x","diamondCount":0}`)); ok {
		t.Error("zero-coin gift should be ignored")
	}
}

func TestAvatar(t *testing.T) {
	const pic = "https://p16-sign-va.tiktokcdn.com/tos-maliva-avt-0068/abc~c5_100x100.jpeg?x-expires=1"
	cases := []struct {
		name, data, want string
	}{
		{"top level", `{"uniqueId":"a","comment":"!req 1","profilePictureUrl":"` + pic + `"}`, pic},
		{"userDetails list", `{"uniqueId":"a","comment":"!req 1","userDetails":{"profilePictureUrls":["http://insecure.tiktokcdn.com/x.jpg","` + pic + `"]}}`, pic},
		{"nested user", `{"comment":"!req 1","user":{"uniqueId":"a","profilePicture":{"url":["` + pic + `"]}}}`, pic},
		{"us cdn", `{"uniqueId":"a","comment":"x","profilePictureUrl":"https://p19-pu-sign-useast8.tiktokcdn-us.com/a.webp"}`, "https://p19-pu-sign-useast8.tiktokcdn-us.com/a.webp"},
		{"foreign host", `{"uniqueId":"a","comment":"x","profilePictureUrl":"https://evil.example/tiktokcdn.com.jpg"}`, ""},
		{"lookalike host", `{"uniqueId":"a","comment":"x","profilePictureUrl":"https://tiktokcdn.com.evil.example/a.jpg"}`, ""},
		{"not https", `{"uniqueId":"a","comment":"x","profilePictureUrl":"javascript:alert(1)"}`, ""},
		{"missing", `{"uniqueId":"a","comment":"x"}`, ""},
	}
	for _, c := range cases {
		ch, ok := ParseChat(json.RawMessage(c.data))
		if !ok {
			t.Fatalf("%s: not parsed", c.name)
		}
		if ch.Avatar != c.want {
			t.Errorf("%s: avatar = %q, want %q", c.name, ch.Avatar, c.want)
		}
	}
	g, ok := ParseGift(json.RawMessage(`{"uniqueId":"a","giftName":"Rose","diamondCount":1,"repeatCount":1,"profilePictureUrl":"` + pic + `"}`))
	if !ok || g.Avatar != pic {
		t.Errorf("gift avatar: %+v", g)
	}
}

func TestKeepsFirstCommandAndCountedGift(t *testing.T) {
	c := New(Env{}, func() Settings { return Settings{} })
	c.handle([]byte(`{"event":"chat","data":{"uniqueId":"a","comment":"hello"}}`), false)
	c.handle([]byte(`{"event":"chat","data":{"uniqueId":"b","comment":"!req 1","isModerator":true}}`), false)
	c.handle([]byte(`{"event":"gift","data":{"uniqueId":"c","giftType":1,"repeatEnd":false,"diamondCount":1,"repeatCount":3}}`), false)
	c.handle([]byte(`{"event":"gift","data":{"uniqueId":"c","giftType":1,"repeatEnd":true,"diamondCount":1,"repeatCount":5}}`), false)
	_, first := c.Raw()
	if !strings.Contains(first["chat"].Raw, "hello") || !strings.Contains(first["chat (command)"].Raw, "!req 1") {
		t.Errorf("chat events: %+v", first)
	}
	if !strings.Contains(first["gift"].Raw, `"repeatEnd":false`) || !strings.Contains(first["gift (counted)"].Raw, `"repeatEnd":true`) {
		t.Errorf("gift events: %+v", first)
	}
}

func TestParseViewers(t *testing.T) {
	cases := []struct {
		raw  string
		want int
		ok   bool
	}{
		{`{"viewerCount":1234,"topViewers":[]}`, 1234, true},
		{`{"viewerCount":0}`, 0, true},
		{`{"total":57}`, 57, true},
		{`{"viewerCount":12,"total":99}`, 12, true},
		{`{"viewerCount":"1234"}`, 0, false},
		{`{"viewerCount":-3}`, 0, false},
		{`{"viewerCount":12.5}`, 0, false},
		{`{"viewerCount":1e12}`, 0, false},
		{`{"viewerCount":null}`, 0, false},
		{`{"topViewers":[]}`, 0, false},
		{`[1,2]`, 0, false},
		{`not json`, 0, false},
	}
	for _, c := range cases {
		got, ok := ParseViewers([]byte(c.raw))
		if got != c.want || ok != c.ok {
			t.Errorf("ParseViewers(%s) = %d %v, want %d %v", c.raw, got, ok, c.want, c.ok)
		}
	}
}

func TestViewersFromTikFinity(t *testing.T) {
	c := New(Env{}, func() Settings { return Settings{} })
	var seen []int
	c.OnViewers = func(n int) { seen = append(seen, n) }
	c.handle([]byte(`{"event":"roomUser","data":{"viewerCount":321}}`), false)
	if st := c.Status(); st.Viewers != 321 || !st.Live {
		t.Errorf("roomUser: %+v", st)
	}
	c.handle([]byte(`{"event":"roomUser","data":{"topViewers":[]}}`), false)
	c.handle([]byte(`{"event":"roomUser","data":{"viewerCount":"many"}}`), false)
	c.handle([]byte(`{"event":"chat","data":{"uniqueId":"a","comment":"hi","viewerCount":5}}`), false)
	if st := c.Status(); st.Viewers != 321 {
		t.Errorf("payloads without a number must change nothing: %+v", st)
	}
	if len(seen) != 1 || seen[0] != 321 {
		t.Errorf("OnViewers = %v", seen)
	}
}
