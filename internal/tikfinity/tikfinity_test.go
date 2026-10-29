package tikfinity

import "testing"

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
