package user_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/linusxiong/ggboard/internal/geoip/geoiptest"
	"github.com/linusxiong/ggboard/internal/subscription/guard"
	"github.com/linusxiong/ggboard/internal/testutil"
)

// The user's access log masks the IP but resolves the location from the full address (docs/API.md).
func TestAccessLogLocationFromTheFullIP(t *testing.T) {
	a, r := setup(t)
	geoiptest.Install(t, a.Cfg.DataDir, geoiptest.Sample, geoiptest.SampleVersion)
	u := testutil.CreateUser(t, a, "where@example.com")
	now := time.Now()
	// 203.0.113.9 and .200 share the masked "203.0.113.*" but lie in different ranges.
	for i, ip := range []string{"203.0.113.9", "203.0.113.200", "2001:db8:7:8::9"} {
		guard.Record(a, guard.Access{UserID: u.ID, Token: u.Token, IP: ip, UserAgent: "mihomo/1.19.27", Format: "clash-meta",
			At: now.Add(time.Duration(-i) * time.Minute)})
	}
	type entry struct {
		IP       string  `json:"ip"`
		Location *string `json:"location"`
	}
	list := testutil.Decode[[]entry](t, testutil.Do(t, r, http.MethodGet, base+"/subscription/access-log", nil, testutil.Login(t, a, u)))
	if len(list) != 3 {
		t.Fatalf("rows = %+v", list)
	}
	want := []entry{{"203.0.113.*", testutil.Ptr("中国 广东 深圳 电信")}, {"203.0.113.*", testutil.Ptr("中国 广东 深圳")},
		{"2001:db8:7:8:*", testutil.Ptr("")}}
	for i, w := range want {
		if list[i].IP != w.IP || list[i].Location == nil || *list[i].Location != *w.Location {
			t.Errorf("row %d = %s %v, want %s %q", i, list[i].IP, list[i].Location, w.IP, *w.Location)
		}
	}
}
