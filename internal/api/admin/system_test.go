package admin_test

import (
	"net/http"
	"runtime"
	"slices"
	"testing"
	"time"

	"github.com/linusxiong/ggboard/internal/cache"
	"github.com/linusxiong/ggboard/internal/mail"
	"github.com/linusxiong/ggboard/internal/scheduler"
	"github.com/linusxiong/ggboard/internal/service/maintenance"
	"github.com/linusxiong/ggboard/internal/testutil"
)

func TestAdminSystemStatus(t *testing.T) {
	e := newOpsEnv(t)
	type status struct {
		Version    string                `json:"version"`
		GoVersion  string                `json:"go_version"`
		Uptime     int64                 `json:"uptime"`
		DBDriver   string                `json:"db_driver"`
		StartedAt  time.Time             `json:"started_at"`
		Scheduler  []scheduler.JobStatus `json:"scheduler"`
		Mail       mail.QueueStats       `json:"mail"`
		MailQueue  *int64                `json:"mail_queue"`
		Cache      cache.Stats           `json:"cache"`
		Online     map[string]int        `json:"online"`
		Goroutines int                   `json:"goroutines"`
		Memory     map[string]uint64     `json:"memory"`
	}
	if err := e.a.Scheduler.RunNow(maintenance.JobStatsDaily); err != nil {
		t.Fatal(err)
	}
	st := testutil.Decode[status](t, testutil.Do(t, e.r, http.MethodGet, "/api/v1/admin/system/status", nil, e.adminTok))
	var names []string
	var statsJob scheduler.JobStatus
	for _, j := range st.Scheduler {
		names = append(names, j.Name)
		if j.Name == maintenance.JobStatsDaily {
			statsJob = j
		}
	}
	for _, want := range []string{maintenance.JobTrafficReset, maintenance.JobExpireRemind, maintenance.JobTrafficRemind, maintenance.JobStatsDaily} {
		if !slices.Contains(names, want) {
			t.Errorf("scheduler misses %s: %v", want, names)
		}
	}
	if st.Version != "test" || st.GoVersion != runtime.Version() || st.DBDriver != "sqlite" || st.Uptime < 0 || st.StartedAt.IsZero() ||
		st.MailQueue == nil || st.Goroutines < 1 || st.Memory["alloc"] == 0 || st.Memory["sys"] == 0 || statsJob.LastRun == nil || statsJob.Runs != 1 {
		t.Fatalf("status = %+v", st)
	}
	if _, ok := st.Online["nodes"]; !ok {
		t.Fatalf("online = %v", st.Online)
	}
}

func TestAdminSystemLogs(t *testing.T) {
	e := newOpsEnv(t)
	e.a.Log.Debug("ops debug line")
	e.a.Log.Warn("ops warning line", "component", "test", "count", 3)

	type entry struct {
		Time  time.Time      `json:"time"`
		Level string         `json:"level"`
		Msg   string         `json:"msg"`
		Attrs map[string]any `json:"attrs"`
	}
	logs := testutil.Decode[[]entry](t, testutil.Do(t, e.r, http.MethodGet, "/api/v1/admin/system/logs?level=warn&limit=50", nil, e.adminTok))
	if len(logs) == 0 || logs[0].Msg != "ops warning line" || logs[0].Level != "warn" || logs[0].Attrs["component"] != "test" || logs[0].Attrs["count"] != float64(3) {
		t.Fatalf("warn logs = %+v", logs)
	}
	for _, l := range logs {
		if l.Level == "debug" || l.Level == "info" {
			t.Fatalf("level filter leaked %+v", l)
		}
	}
	all := testutil.Decode[[]entry](t, testutil.Do(t, e.r, http.MethodGet, "/api/v1/admin/system/logs", nil, e.adminTok))
	found := false
	for _, l := range all {
		if l.Msg == "ops debug line" {
			found = l.Attrs != nil
		}
	}
	if !found {
		t.Fatalf("debug entry missing or without attrs object: %+v", all)
	}
	if one := testutil.Decode[[]entry](t, testutil.Do(t, e.r, http.MethodGet, "/api/v1/admin/system/logs?limit=1", nil, e.adminTok)); len(one) != 1 {
		t.Fatalf("limit=1 returned %d", len(one))
	}
	for _, q := range []string{"?level=verbose", "?limit=1001", "?limit=-1"} {
		e.expect(t, http.MethodGet, "/api/v1/admin/system/logs"+q, nil, e.adminTok, http.StatusBadRequest)
	}
}
