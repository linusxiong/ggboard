package node_test

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/linusxiong/ggboard/internal/model"
	nodesvc "github.com/linusxiong/ggboard/internal/service/node"
	usersvc "github.com/linusxiong/ggboard/internal/service/user"
	"github.com/linusxiong/ggboard/internal/testutil"
)

func gunzip(t *testing.T, rec *httptest.ResponseRecorder) []byte {
	t.Helper()
	zr, err := gzip.NewReader(bytes.NewReader(rec.Body.Bytes()))
	if err != nil {
		t.Fatalf("not gzip: %v", err)
	}
	b, err := io.ReadAll(zr)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func merge(a, b map[string]string) map[string]string {
	out := map[string]string{}
	for _, m := range []map[string]string{a, b} {
		for k, v := range m {
			out[k] = v
		}
	}
	return out
}

// perf 2026-10-23 (docs/perf/README.md, finding 1): the UniProxy and soga user lists are cached with their gzip
// encoding, which is dropped together with the JSON when the users change; 304s and identity responses are unchanged.
func TestUserListsCachedGzip(t *testing.T) {
	e := newEnv(t)
	g := docSetup(e)
	for i := range 30 { // past the middleware's 1 KiB threshold
		e.user(fmt.Sprintf("gz%02d@example.com", i), "", g, nil, nil)
	}
	e.node(realityNode(g))
	e.node(model.Node{ID: 7, Type: "shadowsocks", ServerPort: 8388, Host: "ss.example.com", Rate: 1, CreatedAt: time.Unix(1735689600, 0),
		GroupIDs: model.JSONSlice[uint64]{g}, ProtocolSettings: model.JSONMap{"cipher": "2022-blake3-aes-128-gcm"}})
	gz := map[string]string{"Accept-Encoding": "gzip"}
	lists := []struct {
		name    string
		path    string
		headers map[string]string
	}{
		{"uniproxy", uniURL("user", 3, "vless"), nil},
		{"soga", "/soga/api/v1/users", sogaHeaders(7, "shadowsocks")},
	}
	for _, l := range lists {
		t.Run(l.name, func(t *testing.T) {
			check := func() (plain []byte, tag string) {
				t.Helper()
				// Identity (no Accept-Encoding): the JSON itself with a strong ETag, as before.
				id := e.do(http.MethodGet, l.path, nil, l.headers)
				testutil.ExpectStatus(t, id, http.StatusOK)
				tag = id.Header().Get("ETag")
				if id.Body.Len() < 1024 || id.Header().Get("Content-Encoding") != "" || id.Header().Get("Vary") != "" ||
					len(tag) != 42 || tag[0] != '"' || id.Header().Get("Content-Length") != fmt.Sprint(id.Body.Len()) {
					t.Fatalf("identity: %d bytes, %v", id.Body.Len(), id.Header())
				}
				// gzip: the cached encoding of the same JSON, weak ETag, Vary, its own length.
				for range 2 {
					rec := e.do(http.MethodGet, l.path, nil, merge(l.headers, gz))
					testutil.ExpectStatus(t, rec, http.StatusOK)
					if rec.Header().Get("Content-Encoding") != "gzip" || rec.Header().Get("Vary") != "Accept-Encoding" ||
						rec.Header().Get("ETag") != "W/"+tag || rec.Header().Get("Content-Type") != "application/json; charset=utf-8" ||
						rec.Header().Get("Content-Length") != fmt.Sprint(rec.Body.Len()) {
						t.Fatalf("gzip headers: %v", rec.Header())
					}
					if got := gunzip(t, rec); !bytes.Equal(got, id.Body.Bytes()) {
						t.Fatalf("gzip body decompresses to %d bytes, not the %d-byte JSON", len(got), id.Body.Len())
					}
				}
				// 304 for every form of the tag, with or without gzip, exactly as before: no body, strong ETag.
				for _, inm := range []string{tag, "W/" + tag, strings.Trim(tag, `"`)} {
					for _, enc := range []map[string]string{nil, gz} {
						rec := e.do(http.MethodGet, l.path, nil, merge(merge(l.headers, enc), map[string]string{"If-None-Match": inm}))
						if rec.Code != http.StatusNotModified || rec.Body.Len() != 0 || rec.Header().Get("ETag") != tag ||
							rec.Header().Get("Content-Encoding") != "" {
							t.Fatalf("If-None-Match %s %v: %d %q %v", inm, enc, rec.Code, rec.Body.String(), rec.Header())
						}
					}
				}
				return id.Body.Bytes(), tag
			}

			before, tag := check()
			nu := e.user(l.name+"-new@example.com", "", g, nil, nil) // calls InvalidateNodeUsers
			after, newTag := check()
			if newTag == tag || bytes.Equal(after, before) || !bytes.Contains(after, fmt.Appendf(nil, `"id":%d,`, nu.ID)) {
				t.Fatalf("user list did not change after a new user (tag %s → %s)", tag, newTag)
			}
			// The old tag no longer matches: the compressed body is the new one.
			rec := e.do(http.MethodGet, l.path, nil, merge(merge(l.headers, gz), map[string]string{"If-None-Match": tag}))
			testutil.ExpectStatus(t, rec, http.StatusOK)
			if got := gunzip(t, rec); !bytes.Equal(got, after) {
				t.Fatal("stale gzip body after the user list changed")
			}
		})
	}
}

// The encoding is made once per cached body: every full response for the same user list shares it, and a new list
// (InvalidateNodeUsers) gets a new body and a new encoding.
func TestBodyGzipCachedWithBody(t *testing.T) {
	e := newEnv(t)
	g := docSetup(e)
	e.node(realityNode(g))
	svc := nodesvc.New(e.a)
	ctx := context.Background()
	r, err := svc.Resolve(ctx, 3)
	if err != nil {
		t.Fatal(err)
	}
	b1, err := svc.UniProxyUsers(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	b2, _ := svc.UniProxyUsers(ctx, r)
	z1, z2 := b1.Gzip(), b2.Gzip()
	if b1 != b2 || len(z1) == 0 || &z1[0] != &z2[0] {
		t.Fatal("the cached body must carry one shared encoding")
	}
	zr, err := gzip.NewReader(bytes.NewReader(z1))
	if err != nil {
		t.Fatal(err)
	}
	if plain, _ := io.ReadAll(zr); !bytes.Equal(plain, b1.JSON) {
		t.Fatal("encoding does not decompress to the JSON")
	}

	e.user("three@example.com", "", g, nil, nil)
	usersvc.New(e.a).InvalidateNodeUsers()
	b3, _ := svc.UniProxyUsers(ctx, r)
	if b3 == b1 || b3.ETag == b1.ETag || &b3.Gzip()[0] == &z1[0] {
		t.Fatal("a changed user list must not reuse the old body or encoding")
	}
}
