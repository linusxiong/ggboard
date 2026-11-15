package client_test

import (
	"net/http"
	"testing"

	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/testutil"
)

// TestNumericTokenPrefixDoesNotMatch pins the defence against the Xboard "numeric token" bug: a JSON number in
// the request body used to be compared with the CHAR token column on MySQL, which coerces the column to a number, so
// token=44155 matched any token whose leading digits are exactly 44155 ("44155abc..." but not "441550...", which
// converts to a different number). Here the token is only ever read from the query string or the path as a string, is
// validated, and is matched exactly.
//
// SQLite never coerces, so the numeric cases cannot fail on this suite by themselves. The case that catches a
// reintroduced body fallback is "full token in JSON body is ignored"; MySQL needs a manual run (MISTAKES #292).
func TestNumericTokenPrefixDoesNotMatch(t *testing.T) {
	e := setup(t, nil)
	const token = "44155" + "abcdef0123456789abcdef01234" // 32 characters; a letter must follow the digits
	if err := e.a.DB.Model(&model.User{}).Where("id = ?", e.u.ID).Update("token", token).Error; err != nil {
		t.Fatal(err)
	}
	e.u.Token = token

	// Control: the full token still works, so the 403s below are not a broken fixture.
	if rec := e.get(t, e.sub(""), nil); rec.Code != http.StatusOK {
		t.Fatalf("control request with the full token: %d %s", rec.Code, rec.Body.String())
	}

	jsonGET := func(path, body string) int {
		t.Helper()
		return testutil.DoWithHeaders(t, e.r, http.MethodGet, path, body, "", nil).Code
	}
	cases := []struct {
		name string
		code int
	}{
		{"numeric prefix in JSON body (the published PoC)", jsonGET("/api/v1/client/subscribe", `{"token":44155,"flag":"clashmeta"}`)},
		{"numeric prefix in JSON body, float", jsonGET("/api/v1/client/subscribe", `{"token":44155.0}`)},
		{"full token in JSON body is ignored", jsonGET("/api/v1/client/subscribe", `{"token":"`+token+`"}`)},
		{"prefix in query string", e.get(t, "/api/v1/client/subscribe?token=44155", nil).Code},
		{"prefix in short link", e.get(t, "/s/44155", nil).Code},
		{"upper-cased token (MySQL collation is case-insensitive)", e.get(t, "/api/v1/client/subscribe?token=44155"+"ABCDEF0123456789ABCDEF01234", nil).Code},
	}
	for _, c := range cases {
		if c.code != http.StatusForbidden {
			t.Errorf("%s: status %d, want 403", c.name, c.code)
		}
	}
}
