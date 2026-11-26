package geoip_test

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/linusxiong/ggboard/internal/geoip"
	"github.com/linusxiong/ggboard/internal/geoip/geoiptest"
)

// Security audit 2026-11-25 §3: the GeoIP URLs are admin settings and may point at internal services (accepted,
// MISTAKES #53), but the update error shown as last_error in GET /admin/geoip repeated what the other side
// answered: the "latest" field of any JSON, the JSON decoder's view of the first byte, index offsets read from the
// first 8 bytes plus the length of a downloaded body, the range count, a malformed status line quoted by Go's
// transport (a banner grab) and a redirect's target. The errors now say what was wrong without that content.
func TestReviewUpdateErrorsDoNotEchoResponses(t *testing.T) {
	const marker = "INTERNAL-SECRET-4242"
	fewer := geoiptest.Build(geoiptest.Sample[:2], "20261021")
	for _, tc := range []struct {
		name, want string
		version    func(w http.ResponseWriter)
		download   []byte
		hidden     []string
	}{
		{
			name:    "latest is not a date",
			version: func(w http.ResponseWriter) { _, _ = w.Write([]byte(`{"latest":"` + marker + `"}`)) },
			want:    `version check: "latest" in the version file is not a YYYYMMDD date`,
			hidden:  []string{marker},
		},
		{
			name:    "version file is not JSON",
			version: func(w http.ResponseWriter) { _, _ = w.Write([]byte(marker)) },
			want:    `version check: the version file is not JSON like {"latest": "YYYYMMDD"}`,
			hidden:  []string{"'I'", "character"},
		},
		{
			name: "download is not a qqwry.dat",
			// The first 8 bytes ("INTERNAL") read as little-endian offsets 1163152969 and 1279348306.
			download: []byte(marker + strings.Repeat(" padding", 8)),
			want:     "downloaded file rejected: not a valid qqwry.dat",
			hidden:   []string{"1163152969", "1279348306", "84", "bytes"},
		},
		{
			name:     "too few ranges",
			download: fewer,
			want:     "downloaded file rejected: fewer than 5 IP ranges", // was "3 ranges, expected at least 5"
			hidden:   []string{"3 ranges"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := newMirror(t, "20261021", tc.download)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/version.json" && tc.version != nil {
					tc.version(w)
					return
				}
				http.Redirect(w, r, m.URL+r.URL.Path, http.StatusFound) // another host:port, like GitHub's assets
			}))
			t.Cleanup(srv.Close)
			a := newApp(t, m, map[string]any{"geoip_download_url": srv.URL + "/qqwry.dat", "geoip_version_url": srv.URL + "/version.json"})
			s := geoip.Of(a)
			_, err := s.Update(t.Context())
			if err == nil || err.Error() != tc.want || s.Status().LastError != tc.want {
				t.Fatalf("err = %v, last_error = %q, want %q", err, s.Status().LastError, tc.want)
			}
			for _, h := range tc.hidden {
				if strings.Contains(s.Status().LastError, h) {
					t.Errorf("last_error %q repeats %q", s.Status().LastError, h)
				}
			}
		})
	}
}

// A non-HTTP service on the other end: Go's transport quotes its first line (`malformed HTTP response "SSH-2.0-…"`).
func TestReviewUpdateErrorsDoNotQuoteABanner(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			// Like a real SSH server: send the banner, then read what the client sends before hanging up. Closing at once
			// made the client's request write fail now and then (broken pipe) before it read the banner.
			_, _ = c.Write([]byte("SSH-2.0-OpenSSH_9.9 INTERNAL-BANNER\r\n"))
			_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
			_, _ = io.Copy(io.Discard, c)
			_ = c.Close()
		}
	}()
	m := newMirror(t, "20261021", nil)
	a := newApp(t, m, map[string]any{"geoip_download_url": "http://" + ln.Addr().String() + "/qqwry.dat", "geoip_version_url": ""})
	_, err = geoip.Of(a).Update(t.Context())
	if want := "download: GET qqwry.dat: the server did not answer with HTTP"; err == nil || err.Error() != want {
		t.Fatalf("err = %v, want %q", err, want)
	}
}

// A redirect's target (here with a token in its query) is not repeated when it fails, and a redirect loop stops.
func TestReviewUpdateErrorsDoNotNameARedirectTarget(t *testing.T) {
	closed := httptest.NewServer(http.NotFoundHandler())
	target := closed.URL + "/internal/admin?token=INTERNAL-SECRET-4242"
	closed.Close() // connection refused from now on
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/loop" {
			http.Redirect(w, r, "/loop", http.StatusFound)
			return
		}
		http.Redirect(w, r, target, http.StatusFound)
	}))
	t.Cleanup(srv.Close)
	m := newMirror(t, "20261021", nil)
	for path, want := range map[string]string{
		"/qqwry.dat": "download: GET qqwry.dat: connection refused",
		"/loop":      "download: GET loop: too many redirects",
	} {
		a := newApp(t, m, map[string]any{"geoip_download_url": srv.URL + path, "geoip_version_url": ""})
		if _, err := geoip.Of(a).Update(t.Context()); err == nil || err.Error() != want {
			t.Errorf("%s: err = %v, want %q", path, err, want)
		}
	}
}

// Redirects to other hosts stay allowed (the default release URL moves from github.com to
// release-assets.githubusercontent.com), but never from https down to plain http.
func TestReviewUpdateRedirectPolicy(t *testing.T) {
	req := func(raw string) *http.Request {
		u, err := url.Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		return &http.Request{URL: u}
	}
	gh := req("https://github.com/metowolf/qqwry.dat/releases/latest/download/qqwry.dat")
	for _, tc := range []struct {
		next string
		via  []*http.Request
		ok   bool
	}{
		{"https://release-assets.githubusercontent.com/x", []*http.Request{gh, req("https://github.com/x")}, true},
		{"http://mirror.example.com/qqwry.dat", []*http.Request{req("http://panel.example.com/qqwry.dat")}, true},
		{"https://mirror.example.com/qqwry.dat", []*http.Request{req("http://panel.example.com/qqwry.dat")}, true},
		{"http://mirror.example.com/qqwry.dat", []*http.Request{gh}, false},
		{"http://mirror.example.com/qqwry.dat", []*http.Request{req("http://a.example.com/"), req("https://b.example.com/")}, false},
		{"https://github.com/x", []*http.Request{gh, gh, gh, gh, gh, gh, gh, gh, gh}, true},
		{"https://github.com/x", []*http.Request{gh, gh, gh, gh, gh, gh, gh, gh, gh, gh}, false},
	} {
		if err := geoip.CheckRedirect(req(tc.next), tc.via); (err == nil) != tc.ok {
			t.Errorf("redirect to %s after %d hops: %v, want ok=%v", tc.next, len(tc.via), err, tc.ok)
		}
	}
}
