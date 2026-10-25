package client_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/linusxiong/ggboard/internal/testutil"
)

// sing-box gets the rule-set download form of the core version its User-Agent names (docs/SUBSCRIPTION.md, Rule
// emission per format): one body and ETag per version bucket, and a tag from one bucket never answers 304 to the other.
func TestSingboxVersionBuckets(t *testing.T) {
	e := setup(t, nil)
	type config struct {
		HTTPClients []map[string]string `json:"http_clients"`
		Route       struct {
			RuleSet           []map[string]string `json:"rule_set"`
			DefaultHTTPClient string              `json:"default_http_client"`
		} `json:"route"`
	}
	get := func(query, ua, inm string) (*config, string, int) {
		t.Helper()
		headers := map[string]string{"User-Agent": ua}
		if inm != "" {
			headers["If-None-Match"] = inm
		}
		rec := e.get(t, e.sub(query), headers)
		if rec.Code == http.StatusNotModified {
			return nil, rec.Header().Get("ETag"), rec.Code
		}
		testutil.ExpectStatus(t, rec, http.StatusOK)
		var cfg config
		if err := json.Unmarshal(rec.Body.Bytes(), &cfg); err != nil {
			t.Fatalf("%s: %v", ua, err)
		}
		return &cfg, rec.Header().Get("ETag"), rec.Code
	}

	old, oldTag, _ := get("", "SFA/1.13.3 (613; sing-box 1.13.3; language en_US)", "")
	if len(old.HTTPClients) != 0 || old.Route.DefaultHTTPClient != "" || len(old.Route.RuleSet) == 0 {
		t.Fatalf("1.13: http_clients %v, default %q, %d rule sets", old.HTTPClients, old.Route.DefaultHTTPClient, len(old.Route.RuleSet))
	}
	for _, rs := range old.Route.RuleSet {
		if rs["download_detour"] != "Proxy" {
			t.Fatalf("1.13 rule set %v", rs)
		}
	}

	modern, modernTag, _ := get("", "SFA (sing-box 1.14.4; language zh_CN)", "")
	if len(modern.HTTPClients) != 1 || modern.HTTPClients[0]["tag"] != "rule-set" || modern.HTTPClients[0]["detour"] != "Proxy" ||
		modern.Route.DefaultHTTPClient != "rule-set" || len(modern.Route.RuleSet) != len(old.Route.RuleSet) {
		t.Fatalf("1.14: http_clients %v, default %q, %d rule sets", modern.HTTPClients, modern.Route.DefaultHTTPClient, len(modern.Route.RuleSet))
	}
	for _, rs := range modern.Route.RuleSet {
		if _, ok := rs["download_detour"]; ok {
			t.Fatalf("1.14 rule set %v", rs)
		}
	}
	if oldTag == modernTag {
		t.Fatal("both buckets share an ETag")
	}

	// The tag names the bucket, not the exact User-Agent.
	if _, tag, _ := get("", "SFM (sing-box 1.15.0; language en_US)", ""); tag != modernTag {
		t.Errorf("1.15 ETag %s, want %s", tag, modernTag)
	}
	if _, tag, _ := get("&flag=singbox", "HiddifyNext/2.5.7 (android) like ClashMeta v2ray sing-box", ""); tag != oldTag {
		t.Errorf("unnamed version ETag %s, want %s", tag, oldTag)
	}
	if _, _, code := get("", "SFI (sing-box 1.14.4; language en_US)", modernTag); code != http.StatusNotModified {
		t.Errorf("same bucket If-None-Match = %d", code)
	}
	if _, _, code := get("", "SFI (sing-box 1.14.4; language en_US)", oldTag); code != http.StatusOK {
		t.Errorf("other bucket If-None-Match = %d", code)
	}
}
