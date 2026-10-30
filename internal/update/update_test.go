package update

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewer(t *testing.T) {
	cases := []struct {
		latest, current string
		want            bool
	}{
		{"v1.4.0", "1.3.7", true},
		{"1.3.10", "1.3.9", true},
		{"v2.0.0", "1.99.99", true},
		{"v1.3.7", "1.3.7", false},
		{"v1.3.6", "1.3.7", false},
		{"v1.4.0", "dev", false},
		{"v1.4.0", "0.0.0-dev+abc1234", false},
		{"v1.4.0-rc1", "1.3.7", false},
		{"garbage", "1.3.7", false},
	}
	for _, c := range cases {
		if got := Newer(c.latest, c.current); got != c.want {
			t.Errorf("Newer(%q, %q) = %v, want %v", c.latest, c.current, got, c.want)
		}
	}
}

func serve(t *testing.T, body string, code int) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Error("GitHub requires a User-Agent")
		}
		w.WriteHeader(code)
		w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestLatest(t *testing.T) {
	ctx := context.Background()
	url := serve(t, `{"tag_name":"v1.4.0","html_url":"https://github.com/Lacrymiraosu/tiktoklivereqsong/releases/tag/v1.4.0"}`, 200)
	rel, err := Latest(ctx, http.DefaultClient, url, "1.3.7")
	if err != nil || rel == nil || rel.Version != "1.4.0" || rel.URL != ReleasesURL+"tag/v1.4.0" {
		t.Fatalf("got %+v, %v", rel, err)
	}
	if rel, err := Latest(ctx, http.DefaultClient, url, "1.4.0"); rel != nil || err != nil {
		t.Errorf("up to date: got %+v, %v", rel, err)
	}

	// a link anywhere else is replaced by this project's release page
	url = serve(t, `{"tag_name":"v9.0.0","html_url":"https://evil.example/osureq.exe"}`, 200)
	if rel, _ := Latest(ctx, http.DefaultClient, url, "1.3.7"); rel == nil || rel.URL != ReleasesURL+"latest" {
		t.Errorf("foreign link must not be passed on: %+v", rel)
	}

	url = serve(t, `{"tag_name":"v9.0.0","prerelease":true}`, 200)
	if rel, _ := Latest(ctx, http.DefaultClient, url, "1.3.7"); rel != nil {
		t.Errorf("pre-releases are not offered: %+v", rel)
	}
	url = serve(t, `{"message":"rate limited"}`, 403)
	if _, err := Latest(ctx, http.DefaultClient, url, "1.3.7"); err == nil {
		t.Error("an error status should be an error")
	}
}
