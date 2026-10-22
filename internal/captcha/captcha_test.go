package captcha_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/linusxiong/ggboard/internal/captcha"
	"github.com/linusxiong/ggboard/internal/testutil"
)

func TestNoneAndSelection(t *testing.T) {
	a := testutil.NewTestApp(t)
	v := captcha.FromSettings(a.Settings)
	if err := v.Verify(context.Background(), "", ""); err != nil {
		t.Fatal("none must accept everything")
	}
	if captcha.Enabled(a.Settings) {
		t.Fatal("captcha disabled by default")
	}
	if _, err := captcha.New("mystery", "s", nil); err == nil {
		t.Fatal("unknown provider")
	}
	if v, err := captcha.New("", "", nil); err != nil || v.Verify(context.Background(), "", "") != nil {
		t.Fatal("empty provider means none")
	}
	if err := a.Settings.Set(map[string]any{"captcha_provider": "turnstile", "captcha_secret": "sec"}); err != nil {
		t.Fatal(err)
	}
	if !captcha.Enabled(a.Settings) {
		t.Fatal("enabled")
	}
	sv, ok := captcha.FromSettings(a.Settings).(*captcha.SiteVerify)
	if !ok || sv.Endpoint != captcha.TurnstileURL || sv.Secret != "sec" {
		t.Fatalf("turnstile = %#v", sv)
	}
	if len(captcha.Providers()) != 4 {
		t.Fatalf("providers = %v", captcha.Providers())
	}
}

func TestSiteVerify(t *testing.T) {
	var gotForm map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotForm = map[string]string{"secret": r.PostForm.Get("secret"), "response": r.PostForm.Get("response"), "remoteip": r.PostForm.Get("remoteip")}
		switch r.PostForm.Get("response") {
		case "good":
			_, _ = w.Write([]byte(`{"success":true}`))
		case "error":
			w.WriteHeader(http.StatusBadGateway)
		case "junk":
			_, _ = w.Write([]byte(`not json`))
		default:
			_, _ = w.Write([]byte(`{"success":false,"error-codes":["invalid-input-response"]}`))
		}
	}))
	defer srv.Close()
	v := &captcha.SiteVerify{Endpoint: srv.URL, Secret: "sec", Client: srv.Client()}
	ctx := context.Background()
	if err := v.Verify(ctx, "good", "203.0.113.1"); err != nil {
		t.Fatal(err)
	}
	if gotForm["secret"] != "sec" || gotForm["response"] != "good" || gotForm["remoteip"] != "203.0.113.1" {
		t.Fatalf("form = %v", gotForm)
	}
	if err := v.Verify(ctx, "bad", ""); !errors.Is(err, captcha.ErrFailed) {
		t.Fatalf("rejected = %v", err)
	}
	if err := v.Verify(ctx, "", ""); !errors.Is(err, captcha.ErrFailed) {
		t.Fatal("empty token")
	}
	if err := v.Verify(ctx, "error", ""); err == nil || errors.Is(err, captcha.ErrFailed) {
		t.Fatalf("upstream error = %v", err)
	}
	if err := v.Verify(ctx, "junk", ""); err == nil {
		t.Fatal("junk response")
	}
	if err := (&captcha.SiteVerify{Endpoint: srv.URL}).Verify(ctx, "good", ""); err == nil {
		t.Fatal("missing secret")
	}
}
