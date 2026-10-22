package router_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/linusxiong/ggboard/internal/router"
	"github.com/linusxiong/ggboard/internal/testutil"
)

func TestRouterWiresAllPackages(t *testing.T) {
	a := testutil.NewTestApp(t)
	e, err := router.New(a)
	if err != nil {
		t.Fatal(err)
	}
	testutil.ExpectStatus(t, testutil.Do(t, e, http.MethodGet, "/healthz", nil, ""), 200)
	if e := testutil.DecodeError(t, testutil.Do(t, e, http.MethodGet, "/api/v1/unknown", nil, "")); e.Status != 404 {
		t.Fatalf("unknown api = %+v", e)
	}
	rec := testutil.Do(t, e, http.MethodGet, "/some/spa/route", nil, "")
	if rec.Code != 200 || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/html") {
		t.Fatalf("spa = %d %s", rec.Code, rec.Header().Get("Content-Type"))
	}
	if len(router.Packages) != 6 {
		t.Fatal("six API packages expected")
	}
}
