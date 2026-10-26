package admin_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/linusxiong/ggboard/internal/api/admin"
	"github.com/linusxiong/ggboard/internal/geoip"
	"github.com/linusxiong/ggboard/internal/geoip/geoiptest"
	"github.com/linusxiong/ggboard/internal/subscription/guard"
	"github.com/linusxiong/ggboard/internal/testutil"
)

const geoipPath = "/api/v1/admin/geoip"

type geoipUpdate struct {
	geoip.Status
	Updated bool `json:"updated"`
}

// geoipMirror serves version.json (latest) and qqwry.dat (file); the download blocks while gate is open.
type geoipMirror struct {
	*httptest.Server
	mu     sync.Mutex
	latest string
	file   []byte
	gate   chan struct{}
}

func newGeoIPMirror(t *testing.T, latest string, file []byte) *geoipMirror {
	t.Helper()
	m := &geoipMirror{latest: latest, file: file}
	m.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		latest, file, gate := m.latest, m.file, m.gate
		m.mu.Unlock()
		if r.URL.Path == "/version.json" {
			_, _ = w.Write([]byte(`{"latest":"` + latest + `"}`))
			return
		}
		if gate != nil {
			<-gate
		}
		_, _ = w.Write(file)
	}))
	t.Cleanup(m.Close)
	return m
}

func geoipEnv(t *testing.T, m *geoipMirror) *opsEnv {
	t.Helper()
	e := newOpsEnv(t, testutil.WithSettings(map[string]any{
		"geoip_download_url": m.URL + "/qqwry.dat", "geoip_version_url": m.URL + "/version.json",
	}))
	geoiptest.Install(t, e.a.Cfg.DataDir, geoiptest.Sample, geoiptest.SampleVersion)
	return e
}

func TestAdminGeoIPStatusAndUpdate(t *testing.T) {
	m := newGeoIPMirror(t, geoiptest.SampleVersion, nil)
	e := geoipEnv(t, m)
	for _, rt := range []struct{ method, path string }{{http.MethodGet, geoipPath}, {http.MethodPost, geoipPath + "/update"}} {
		e.expect(t, rt.method, rt.path, nil, e.staffTok, http.StatusForbidden)
		e.expect(t, rt.method, rt.path, nil, e.userTok, http.StatusForbidden)
		e.expect(t, rt.method, rt.path, nil, "", http.StatusUnauthorized)
	}

	// The installed file is described from its content (no qqwry.json yet).
	var raw map[string]any
	var env struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(e.expect(t, http.MethodGet, geoipPath, nil, e.adminTok, http.StatusOK), &env); err != nil {
		t.Fatal(err)
	}
	_ = json.Unmarshal(env.Data, &raw)
	for _, k := range []string{"enabled", "loaded", "updating", "version", "records", "size", "sha256", "updated_at",
		"checked_at", "last_error", "last_error_at"} {
		if _, ok := raw[k]; !ok {
			t.Errorf("status has no %q: %s", k, env.Data)
		}
	}
	if raw["checked_at"] != nil || raw["last_error_at"] != nil || raw["last_error"] != "" {
		t.Fatalf("fresh status = %s", env.Data)
	}
	var st geoip.Status
	_ = json.Unmarshal(env.Data, &st)
	if !st.Enabled || !st.Loaded || st.Version != geoiptest.SampleVersion || st.Records != len(geoiptest.Sample)+1 || st.UpdatedAt == nil {
		t.Fatalf("status = %+v", st)
	}

	// Same version published: nothing downloaded, checked_at set.
	res := testutil.Decode[geoipUpdate](t, testutil.Do(t, e.r, http.MethodPost, geoipPath+"/update", nil, e.adminTok))
	if res.Updated || res.Version != geoiptest.SampleVersion || res.CheckedAt == nil || res.Updating {
		t.Fatalf("up to date = %+v", res)
	}

	// A newer release that fails validation (a handful of ranges is not a real database): 502, old data kept.
	m.mu.Lock()
	m.latest, m.file = "20261028", geoiptest.Build(geoiptest.Sample, "20261028")
	m.mu.Unlock()
	rec := testutil.Do(t, e.r, http.MethodPost, geoipPath+"/update", nil, e.adminTok)
	if he := testutil.DecodeError(t, rec); he.Status != http.StatusBadGateway || he.Code != "geoip_update_failed" ||
		!strings.Contains(he.Message, "ranges") {
		t.Fatalf("invalid release = %+v", he)
	}
	st = testutil.Decode[geoip.Status](t, testutil.Do(t, e.r, http.MethodGet, geoipPath, nil, e.adminTok))
	if st.Version != geoiptest.SampleVersion || st.LastError == "" || st.LastErrorAt == nil {
		t.Fatalf("after a failed update = %+v", st)
	}

	// Disabled: 409.
	if err := e.a.Settings.Set(map[string]any{"geoip_enable": false}); err != nil {
		t.Fatal(err)
	}
	if he := testutil.DecodeError(t, testutil.Do(t, e.r, http.MethodPost, geoipPath+"/update", nil, e.adminTok)); he.Status != http.StatusConflict || he.Code != "geoip_disabled" {
		t.Fatalf("disabled = %+v", he)
	}
}

func TestAdminGeoIPUpdateRunsInTheBackground(t *testing.T) {
	admin.SetGeoIPUpdateWait(t, 20*time.Millisecond)
	m := newGeoIPMirror(t, "20261028", geoiptest.Build(geoiptest.Sample, "20261028"))
	gate := make(chan struct{})
	var once sync.Once
	open := func() { once.Do(func() { close(gate) }) }
	t.Cleanup(open) // before the server closes, so a failure cannot hang on the blocked handler
	m.mu.Lock()
	m.gate = gate
	m.mu.Unlock()
	e := geoipEnv(t, m)

	rec := testutil.Do(t, e.r, http.MethodPost, geoipPath+"/update", nil, e.adminTok)
	testutil.ExpectStatus(t, rec, http.StatusAccepted)
	if res := testutil.Decode[geoipUpdate](t, rec); !res.Updating || res.Updated {
		t.Fatalf("202 body = %+v", res)
	}
	if he := testutil.DecodeError(t, testutil.Do(t, e.r, http.MethodPost, geoipPath+"/update", nil, e.adminTok)); he.Status != http.StatusConflict || he.Code != "geoip_update_running" {
		t.Fatalf("second update = %+v", he)
	}
	open()
	deadline := time.Now().Add(5 * time.Second)
	for {
		st := testutil.Decode[geoip.Status](t, testutil.Do(t, e.r, http.MethodGet, geoipPath, nil, e.adminTok))
		if !st.Updating {
			if st.LastError == "" || st.Version != geoiptest.SampleVersion {
				t.Fatalf("finished = %+v", st) // rejected (too few ranges): the old file stays
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the background update did not finish")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestAdminAccessLogLocations(t *testing.T) {
	e := newOpsEnv(t)
	geoiptest.Install(t, e.a.Cfg.DataDir, geoiptest.Sample, geoiptest.SampleVersion)
	for _, ip := range []string{"203.0.113.9", "2001:db8:1:2::5", "198.51.100.7"} {
		guard.Record(e.a, guard.Access{UserID: e.user.ID, Token: e.user.Token, IP: ip, UserAgent: "mihomo/1.19.27", Format: "clash-meta"})
		time.Sleep(time.Millisecond) // distinct timestamps keep the order below
	}
	var log struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(e.expect(t, http.MethodGet, usersPath(e.user.ID, "/subscription/access-log"), nil, e.adminTok, http.StatusOK), &log); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"203.0.113.9": "中国 广东 深圳 电信", "2001:db8:1:2::5": "", "198.51.100.7": ""}
	if len(log.Data) != 3 {
		t.Fatalf("rows = %v", log.Data)
	}
	for _, row := range log.Data {
		loc, ok := row["location"].(string)
		if !ok || loc != want[row["ip"].(string)] {
			t.Errorf("row %v: location %v, want %q", row["ip"], row["location"], want[row["ip"].(string)])
		}
	}
}
