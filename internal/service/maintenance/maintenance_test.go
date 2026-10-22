package maintenance_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/mail"
	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/service/maintenance"
	usersvc "github.com/linusxiong/ggboard/internal/service/user"
	"github.com/linusxiong/ggboard/internal/testutil"
)

func clock(t time.Time) maintenance.Clock { return func() time.Time { return t } }

func local(y int, m time.Month, d, h, mi int) time.Time {
	return time.Date(y, m, d, h, mi, 0, 0, time.Local)
}

func ptr[T any](v T) *T { return &v }

func plan(t *testing.T, a *app.App, method *model.ResetMethod) *model.Plan {
	t.Helper()
	p := &model.Plan{GroupID: 1, Name: "Plan", TransferEnable: 100, ResetTrafficMethod: method}
	if err := a.DB.Create(p).Error; err != nil {
		t.Fatal(err)
	}
	return p
}

func reload(t *testing.T, a *app.App, id uint64) model.User {
	t.Helper()
	var u model.User
	if err := a.DB.First(&u, id).Error; err != nil {
		t.Fatal(err)
	}
	return u
}

func sameTime(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Equal(*b)
}

func TestTrafficReset(t *testing.T) {
	a := testutil.NewTestApp(t)
	ctx := context.Background()
	now := local(2026, 10, 1, 0, 0).Add(30 * time.Second)
	m := func(r model.ResetMethod) *model.ResetMethod { return &r }
	firstOfMonth := plan(t, a, m(model.ResetFirstDayOfMonth))
	monthly := plan(t, a, m(model.ResetMonthly))
	never := plan(t, a, m(model.ResetNever))
	firstOfYear := plan(t, a, m(model.ResetFirstDayOfYear))
	yearly := plan(t, a, m(model.ResetYearly))
	followSetting := plan(t, a, nil) // settings.reset_traffic_method = 0

	tests := []struct {
		name      string
		plan      *model.Plan
		expiredAt *time.Time
		next      *time.Time
		wantReset bool
		wantNext  *time.Time
	}{
		{"first of month", firstOfMonth, ptr(local(2027, 1, 15, 0, 0)), ptr(local(2026, 10, 1, 0, 0)), true, ptr(local(2026, 11, 1, 0, 0))},
		{"monthly on expiry day", monthly, ptr(local(2027, 3, 15, 10, 0)), ptr(local(2026, 9, 15, 10, 0)), true, ptr(local(2026, 10, 15, 10, 0))},
		{"monthly on day 31", monthly, ptr(local(2027, 1, 31, 8, 0)), ptr(local(2026, 9, 30, 8, 0)), true, ptr(local(2026, 10, 31, 8, 0))},
		{"first of year", firstOfYear, ptr(local(2027, 5, 1, 0, 0)), ptr(local(2026, 1, 1, 0, 0)), true, ptr(local(2027, 1, 1, 0, 0))},
		{"yearly on expiry day", yearly, ptr(local(2028, 3, 2, 12, 0)), ptr(local(2026, 3, 2, 12, 0)), true, ptr(local(2027, 3, 2, 12, 0))},
		{"plan follows setting", followSetting, ptr(local(2027, 1, 1, 0, 0)), ptr(local(2026, 10, 1, 0, 0)), true, ptr(local(2026, 11, 1, 0, 0))},
		{"never clears next", never, ptr(local(2027, 1, 1, 0, 0)), ptr(local(2026, 9, 1, 0, 0)), false, nil},
		{"no plan clears next", nil, ptr(local(2027, 1, 1, 0, 0)), ptr(local(2026, 9, 1, 0, 0)), false, nil},
		{"one-time (no expiry) clears next", firstOfMonth, nil, ptr(local(2026, 9, 1, 0, 0)), false, nil},
		{"future reset untouched", firstOfMonth, ptr(local(2027, 1, 1, 0, 0)), ptr(local(2026, 10, 1, 0, 1)), false, ptr(local(2026, 10, 1, 0, 1))},
		{"no next untouched", firstOfMonth, ptr(local(2027, 1, 1, 0, 0)), nil, false, nil},
	}
	ids := make([]uint64, len(tests))
	for i, tc := range tests {
		opts := usersvc.CreateOptions{ExpiredAt: tc.expiredAt, TransferEnable: 1000}
		if tc.plan != nil {
			opts.PlanID = &tc.plan.ID
		}
		u := testutil.CreateUser(t, a, fmt.Sprintf("u%d@example.com", i), opts)
		if err := a.DB.Model(&model.User{}).Where("id = ?", u.ID).
			Updates(map[string]any{"u": 11, "d": 22, "next_reset_at": tc.next}).Error; err != nil {
			t.Fatal(err)
		}
		ids[i] = u.ID
	}

	n, err := maintenance.TrafficReset(ctx, a, clock(now))
	if err != nil {
		t.Fatal(err)
	}
	wantResets := 0
	for i, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			u := reload(t, a, ids[i])
			if reset := u.U == 0 && u.D == 0; reset != tc.wantReset {
				t.Errorf("reset = %v (u=%d d=%d), want %v", reset, u.U, u.D, tc.wantReset)
			}
			if !sameTime(u.NextResetAt, tc.wantNext) {
				t.Errorf("next_reset_at = %v, want %v", u.NextResetAt, tc.wantNext)
			}
		})
		if tc.wantReset {
			wantResets++
		}
	}
	if n != wantResets {
		t.Fatalf("reset count = %d, want %d", n, wantResets)
	}
	// Idempotent: nothing is due any more.
	if n, err := maintenance.TrafficReset(ctx, a, clock(now)); err != nil || n != 0 {
		t.Fatalf("second run = %d, %v", n, err)
	}
}

func TestTrafficResetBatches(t *testing.T) {
	a := testutil.NewTestApp(t)
	ctx := context.Background()
	now := local(2026, 10, 1, 0, 0)
	p := plan(t, a, nil)
	exp := local(2027, 1, 1, 0, 0)
	due := local(2026, 9, 30, 0, 0)
	const count = 2*maintenance.BatchSize + 7
	users := make([]model.User, count)
	for i := range users {
		users[i] = model.User{
			Email: fmt.Sprintf("bulk%d@example.com", i), PasswordHash: "!", UUID: uuid.NewString(), Token: usersvc.NewToken(),
			PlanID: &p.ID, ExpiredAt: &exp, NextResetAt: &due, U: 5, D: 5, TransferEnable: 100,
		}
	}
	if err := a.DB.CreateInBatches(&users, 200).Error; err != nil {
		t.Fatal(err)
	}
	n, err := maintenance.TrafficReset(ctx, a, clock(now))
	if err != nil || n != count {
		t.Fatalf("reset = %d, %v; want %d", n, err, count)
	}
	var left int64
	if err := a.DB.Model(&model.User{}).Where("u + d > 0 OR next_reset_at <= ?", now).Count(&left).Error; err != nil || left != 0 {
		t.Fatalf("left = %d, %v", left, err)
	}
}

func TestTrafficResetInvalidatesNodeUsers(t *testing.T) {
	a := testutil.NewTestApp(t)
	ctx := context.Background()
	now := local(2026, 10, 1, 0, 0)
	p := plan(t, a, nil)
	gid := uint64(1)
	exp := local(2027, 1, 1, 0, 0)
	u := testutil.CreateUser(t, a, "full@example.com", usersvc.CreateOptions{PlanID: &p.ID, GroupID: &gid, ExpiredAt: &exp, TransferEnable: 100})
	if err := a.DB.Model(u).Updates(map[string]any{"u": 100, "next_reset_at": local(2026, 9, 30, 0, 0)}).Error; err != nil {
		t.Fatal(err)
	}
	svc := usersvc.New(a)
	if list, _ := svc.AvailableUsersForGroups(ctx, []uint64{gid}); len(list) != 0 {
		t.Fatalf("user without traffic is available: %v", list)
	}
	if _, err := maintenance.TrafficReset(ctx, a, clock(now)); err != nil {
		t.Fatal(err)
	}
	if list, _ := svc.AvailableUsersForGroups(ctx, []uint64{gid}); len(list) != 1 {
		t.Fatalf("node user cache not invalidated: %v", list)
	}
}

func mailsTo(msgs []mail.Message, tpl string) []string {
	var out []string
	for _, m := range msgs {
		if m.Template == tpl {
			out = append(out, m.To)
		}
	}
	return out
}

func TestExpireRemind(t *testing.T) {
	a := testutil.NewTestApp(t, testutil.WithSettings(map[string]any{"default_locale": "en"}))
	ctx := context.Background()
	now := local(2026, 10, 22, 10, 0)
	mk := func(email string, exp *time.Time, remind, banned bool) {
		u := testutil.CreateUser(t, a, email, usersvc.CreateOptions{ExpiredAt: exp})
		if err := a.DB.Model(u).Updates(map[string]any{"remind_expire": remind, "banned": banned}).Error; err != nil {
			t.Fatal(err)
		}
	}
	mk("soon@example.com", ptr(now.Add(12*time.Hour)), true, false)
	mk("edge@example.com", ptr(now.Add(24*time.Hour)), true, false)
	mk("later@example.com", ptr(now.Add(36*time.Hour)), true, false)
	mk("expired@example.com", ptr(now.Add(-time.Hour)), true, false)
	mk("never@example.com", nil, true, false)
	mk("optout@example.com", ptr(now.Add(time.Hour)), false, false)
	mk("banned@example.com", ptr(now.Add(time.Hour)), true, true)

	n, err := maintenance.ExpireRemind(ctx, a, clock(now))
	if err != nil || n != 2 {
		t.Fatalf("queued = %d, %v", n, err)
	}
	msgs := testutil.Mails(t, a)
	got := mailsTo(msgs, mail.TplRemindExpire)
	if strings.Join(got, ",") != "soon@example.com,edge@example.com" {
		t.Fatalf("recipients = %v", got)
	}
	if want := now.Add(12 * time.Hour).Format("2006-01-02 15:04"); !strings.Contains(msgs[0].Text, want) {
		t.Fatalf("mail does not mention %s:\n%s", want, msgs[0].Text)
	}
	// Once per 24 h.
	if n, err := maintenance.ExpireRemind(ctx, a, clock(now.Add(time.Hour))); err != nil || n != 0 {
		t.Fatalf("second run = %d, %v", n, err)
	}
	// Disabled globally.
	if err := a.Settings.Set(map[string]any{"remind_mail_enable": false}); err != nil {
		t.Fatal(err)
	}
	testutil.CreateUser(t, a, "other@example.com", usersvc.CreateOptions{ExpiredAt: ptr(now.Add(time.Hour))})
	if n, err := maintenance.ExpireRemind(ctx, a, clock(now)); err != nil || n != 0 {
		t.Fatalf("disabled run = %d, %v", n, err)
	}
}

func TestTrafficRemind(t *testing.T) {
	a := testutil.NewTestApp(t, testutil.WithSettings(map[string]any{"default_locale": "en"}))
	ctx := context.Background()
	now := local(2026, 10, 22, 10, 0)
	const gb = int64(1 << 30)
	mk := func(email string, used, total int64, remind bool, exp *time.Time) {
		u := testutil.CreateUser(t, a, email, usersvc.CreateOptions{TransferEnable: total, ExpiredAt: exp})
		if err := a.DB.Model(u).Updates(map[string]any{"u": used / 2, "d": used - used/2, "remind_traffic": remind}).Error; err != nil {
			t.Fatal(err)
		}
	}
	mk("p85@example.com", 85*gb/10, 10*gb, true, nil)
	mk("p80@example.com", 8*gb, 10*gb, true, ptr(now.Add(time.Hour)))
	mk("p79@example.com", 79*gb/10, 10*gb, true, nil)
	mk("p100@example.com", 10*gb, 10*gb, true, nil)
	mk("zero@example.com", 0, 0, true, nil)
	mk("optout@example.com", 9*gb, 10*gb, false, nil)
	mk("expired@example.com", 9*gb, 10*gb, true, ptr(now.Add(-time.Hour)))

	n, err := maintenance.TrafficRemind(ctx, a, clock(now))
	if err != nil || n != 2 {
		t.Fatalf("queued = %d, %v", n, err)
	}
	msgs := testutil.Mails(t, a)
	if got := mailsTo(msgs, mail.TplRemindTraffic); strings.Join(got, ",") != "p85@example.com,p80@example.com" {
		t.Fatalf("recipients = %v", got)
	}
	if !strings.Contains(msgs[0].Text, "85%") || !strings.Contains(msgs[0].Text, "8.50 GB") || !strings.Contains(msgs[0].Text, "10.00 GB") {
		t.Fatalf("unexpected body:\n%s", msgs[0].Text)
	}
	if n, err := maintenance.TrafficRemind(ctx, a, clock(now.Add(2*time.Hour))); err != nil || n != 0 {
		t.Fatalf("second run = %d, %v", n, err)
	}
}

func TestStatsDaily(t *testing.T) {
	a := testutil.NewTestApp(t)
	ctx := context.Background()
	u := testutil.CreateUser(t, a, "buyer@example.com")
	paid := local(2026, 10, 21, 18, 0)
	if err := a.DB.Create(&model.Order{
		TradeNo: "T1", UserID: u.ID, PlanID: 1, Type: model.OrderTypeNew, Period: model.PeriodMonthly,
		Status: model.OrderStatusCompleted, TotalAmount: 1234, PaidAt: &paid, CreatedAt: paid,
	}).Error; err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		now     time.Time
		wantDay string
		paid    int64
	}{
		{local(2026, 10, 22, 0, 5), "2026-10-21", 1234},
		{local(2026, 10, 23, 0, 5), "2026-10-22", 0},
		{local(2026, 10, 22, 0, 5), "2026-10-21", 1234}, // re-run: same row
	}
	for _, tc := range tests {
		st, err := maintenance.StatsDaily(ctx, a, clock(tc.now))
		if err != nil {
			t.Fatal(err)
		}
		if got := st.RecordAt.UTC().Format(time.DateOnly); got != tc.wantDay || st.PaidTotal != tc.paid {
			t.Fatalf("now %v: day %s paid %d, want %s %d", tc.now, got, st.PaidTotal, tc.wantDay, tc.paid)
		}
	}
	var rows int64
	if err := a.DB.Model(&model.Stat{}).Count(&rows).Error; err != nil || rows != 2 {
		t.Fatalf("stats rows = %d, %v", rows, err)
	}
}

func TestJobsAreRegistered(t *testing.T) {
	a := testutil.NewTestApp(t)
	want := map[string]string{
		maintenance.JobTrafficReset:  "0 0 * * *",
		maintenance.JobExpireRemind:  "0 10 * * *",
		maintenance.JobTrafficRemind: "0 10 * * *",
		maintenance.JobStatsDaily:    "5 0 * * *",
	}
	found := 0
	for _, st := range a.Scheduler.Status() {
		if spec, ok := want[st.Name]; ok {
			found++
			if st.Spec != spec {
				t.Errorf("%s spec = %q, want %q", st.Name, st.Spec, spec)
			}
			if err := a.Scheduler.RunNow(st.Name); err != nil {
				t.Errorf("RunNow(%s) = %v", st.Name, err)
			}
		}
	}
	if found != len(want) {
		t.Fatalf("registered %d of %d jobs", found, len(want))
	}
}

func TestFormatBytes(t *testing.T) {
	for in, want := range map[int64]string{
		0: "0 B", 1023: "1023 B", 1024: "1.00 KB", 1536: "1.50 KB", 1 << 30: "1.00 GB", 5 << 40: "5.00 TB",
	} {
		if got := maintenance.FormatBytes(in); got != want {
			t.Errorf("FormatBytes(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestEnqueueWaitsForRoom(t *testing.T) {
	a := testutil.NewTestApp(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for i := range 1100 { // more than the default queue capacity (1024)
		if err := maintenance.Enqueue(ctx, a.Mailer, mail.Message{To: fmt.Sprintf("q%d@example.com", i), Subject: "s", Text: "t"}); err != nil {
			t.Fatalf("enqueue %d: %v", i, err)
		}
	}
	if got := len(testutil.Mails(t, a)); got != 1100 {
		t.Fatalf("delivered %d", got)
	}
}
