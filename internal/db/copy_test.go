package db_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/schema"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/config"
	"github.com/linusxiong/ggboard/internal/db"
	"github.com/linusxiong/ggboard/internal/mail"
	"github.com/linusxiong/ggboard/internal/model"
	invitesvc "github.com/linusxiong/ggboard/internal/service/invite"
	mfasvc "github.com/linusxiong/ggboard/internal/service/mfa"
	"github.com/linusxiong/ggboard/internal/service/mfa/mfatest"
	ordersvc "github.com/linusxiong/ggboard/internal/service/order"
	statsvc "github.com/linusxiong/ggboard/internal/service/stat"
	ticketsvc "github.com/linusxiong/ggboard/internal/service/ticket"
	usersvc "github.com/linusxiong/ggboard/internal/service/user"
	"github.com/linusxiong/ggboard/internal/subscription/guard"
	"github.com/linusxiong/ggboard/internal/testutil"
)

const (
	seedRPID   = "panel.example.com"
	seedOrigin = "https://panel.example.com"
	// rawNodeSettings is written as raw text: unsorted keys, an integer beyond float64 and odd spacing must survive
	// byte for byte (a copy through model.JSONMap would reorder the keys and round the number).
	rawNodeSettings = `{"z":1, "a":{"big":12345678901234567890,"s":"\u00e9 🐷"},"list":[3,1,2]}`
)

var bg = context.Background()

// seedApp returns an App whose database has realistic rows in every table of model.All(), written through the
// services where they exist (users, orders, tickets, TOTP, passkeys, age keys, access log, traffic, stats, mail
// log, invite codes, settings) and directly otherwise, with NULLs, zero values, false booleans, unicode and
// nanosecond timestamps.
func seedApp(t *testing.T) *app.App {
	t.Helper()
	a := testutil.NewTestApp(t, testutil.WithSettings(map[string]any{"app_url": seedOrigin + "/"}))
	seed(t, a)
	return a
}

func seed(t *testing.T, a *app.App) {
	t.Helper()
	d := a.DB
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	ptr := func(v int) *int { return &v }
	now := time.Now()

	groups := []model.ServerGroup{{Name: "默认"}, {Name: "Premium"}}
	must(d.Create(&groups).Error)
	monthly := model.ResetMonthly
	plans := []model.Plan{
		{GroupID: groups[0].ID, Name: "Basic 基础", Content: "**100 GB**\n- 1 device", Tags: model.JSONSlice[string]{"热门", "hot"},
			TransferEnable: 100, DeviceLimit: ptr(1), Prices: model.JSONObject[int64]{"monthly": 1000, "yearly": 9900},
			ResetTrafficMethod: &monthly, Show: true, Sell: true, Renew: true, Sort: 1},
		// Zero values and NULLs everywhere: no prices, hidden, not sold, unlimited.
		{GroupID: groups[1].ID, Name: "Hidden"},
	}
	must(d.Create(&plans).Error)
	routes := []model.NodeRoute{{Remarks: "block ads", Match: model.JSONSlice[string]{"geosite:category-ads-all"}, Action: model.RouteActionBlock}}
	must(d.Create(&routes).Error)
	nodes := []model.Node{
		{Type: model.NodeShadowsocks, Name: "🇭🇰 HK 01", GroupIDs: model.JSONSlice[uint64]{groups[0].ID, groups[1].ID},
			RouteIDs: model.JSONSlice[uint64]{routes[0].ID}, Host: "hk.example.com", Port: "443", ServerPort: 8443,
			Tags: model.JSONSlice[string]{"HK"}, Rate: 1.5, Show: true, Sort: 2,
			ProtocolSettings: model.JSONMap{"cipher": "2022-blake3-aes-128-gcm"}},
		{Type: model.NodeHysteria, Name: "JP", GroupIDs: model.JSONSlice[uint64]{groups[1].ID}, Host: "jp.example.com",
			Port: "10000-20000", ServerPort: 443, Rate: 0.1, ProtocolSettings: model.JSONMap{"version": 2.0}},
	}
	must(d.Create(&nodes).Error)
	nodes[1].ParentID = &nodes[0].ID
	must(d.Model(&nodes[1]).Update("parent_id", nodes[0].ID).Error)
	must(d.Exec("UPDATE nodes SET protocol_settings = ? WHERE id = ?", rawNodeSettings, nodes[1].ID).Error)

	pays := []model.Payment{
		{UUID: "0123456789abcdef0123456789abcdef", Gateway: "epay", Name: "支付宝", Icon: "alipay",
			Config: model.JSONObject[string]{"url": "https://pay.example.com", "key": "fake-key"}, NotifyDomain: "https://panel.example.com",
			HandlingFeeFixed: 30, HandlingFeePercent: 0.6, Enabled: true, Sort: 1},
		{UUID: "fedcba9876543210fedcba9876543210", Gateway: "stripe", Name: "Stripe"},
	}
	must(d.Create(&pays).Error)
	coupons := []model.Coupon{{Code: "WELCOME", Name: "Welcome", Type: model.CouponTypePercent, Value: 20, LimitUse: ptr(100),
		LimitPlanIDs: model.JSONSlice[uint64]{plans[0].ID}, LimitPeriods: model.JSONSlice[string]{"monthly"},
		StartedAt: now.Add(-time.Hour), EndedAt: now.Add(30 * 24 * time.Hour)}}
	must(d.Create(&coupons).Error)

	users := usersvc.New(a)
	admin := testutil.CreateAdmin(t, a, "admin@example.com")
	staff := testutil.CreateStaff(t, a, "staff@example.com")
	expires := now.Add(30 * 24 * time.Hour)
	alice := testutil.CreateUser(t, a, "alice@example.com", usersvc.CreateOptions{
		PlanID: &plans[0].ID, GroupID: &groups[0].ID, TransferEnable: 100 * model.BytesPerGB, ExpiredAt: &expires,
		SpeedLimit: ptr(100), DeviceLimit: ptr(3), ConnLimit: ptr(0), InviteUserID: &admin.ID, Balance: 1234, Remarks: "VIP 用户 🐷\nline 2",
	})
	bob := testutil.CreateUser(t, a, "bob@example.com") // every nullable column NULL
	must(users.MarkEmailVerified(bg, alice))
	tgID, tgName := int64(-1001234567890), "alice_tg"
	must(d.Model(&model.User{}).Where("id = ?", alice.ID).Updates(map[string]any{
		// Sub-millisecond digits that round up: PostgreSQL / MySQL must get them truncated, not rounded.
		"telegram_id": tgID, "telegram_username": tgName, "banned": false, "last_login_ip": "2001:db8::1",
		"last_login_at":   time.Date(2026, 10, 25, 1, 2, 3, 987654321, time.FixedZone("CST", 8*3600)),
		"commission_rate": 15, "discount": 0,
	}).Error)
	must(d.Model(&model.User{}).Where("id = ?", bob.ID).Update("banned", true).Error)

	orders := ordersvc.New(a)
	paid, err := orders.Create(bg, alice.ID, ordersvc.CreateInput{PlanID: plans[0].ID, Period: model.PeriodMonthly})
	must(err)
	if paid.Status == model.OrderStatusPending { // the balance may already have paid it
		_, err = orders.MarkPaid(bg, paid.ID)
		must(err)
	}
	_, err = orders.Create(bg, alice.ID, ordersvc.CreateInput{PlanID: plans[0].ID, Period: model.PeriodYearly})
	must(err) // stays pending (NULL paid_at, payment_id, coupon_id)
	must(d.Create(&model.CommissionLog{InviteUserID: admin.ID, UserID: alice.ID, TradeNo: paid.TradeNo, OrderAmount: 1000, GetAmount: 150}).Error)
	_, err = invitesvc.New(a).GenerateCode(bg, admin.ID)
	must(err)

	tickets := ticketsvc.New(a)
	tk, err := tickets.Create(bg, alice.ID, "无法连接 HK", model.TicketLevelHigh, "节点超时\n```\nlog\n```")
	must(err)
	_, err = tickets.ReplyAsStaff(bg, tk.ID, staff.ID, "已修复")
	must(err)

	notices := []model.Notice{{Title: "维护通知", Content: "<b>tonight</b>", Tags: model.JSONSlice[string]{"维护"}, Show: true, Sort: 1},
		{Title: "draft"}}
	must(d.Create(&notices).Error)
	must(d.Create(&model.Knowledge{Language: "zh-CN", Category: "入门", Title: "Clash", Body: "# 安装\n{{subscribeUrl}}", Show: true}).Error)
	must(d.Create(&model.MailTemplate{Name: "verify", Locale: "en", Subject: "Your code {{.Code}}", Body: "Code: **{{.Code}}**"}).Error)
	must(d.Create(&model.RuleProfile{UserID: alice.ID, Slug: model.RuleProfileDefaultSlug, Name: model.RuleProfileDefaultName, IsDefault: true,
		Data: model.RawJSON(`{"template":"standard","source":"blackmatrix7","rules":["DOMAIN-SUFFIX,openai.com,AI"]}`)}).Error)
	must(d.Create(&model.RuleProfile{UserID: alice.ID, Slug: "work", Name: "工作"}).Error) // NULL-ish RawJSON ("null")
	must(a.Settings.Set(map[string]any{"app_name": "GG Bond 🐷", "email_verify": true, "subscribe_leak_max_ips": 3}))

	// Second factors through the services: TOTP with backup codes, a passkey from a software authenticator.
	clock := mfatest.FixClock(t, &mfasvc.TOTPNow, now)
	mfa := mfasvc.New(a)
	setup, err := mfa.SetupTOTP(bg, alice)
	must(err)
	_, err = mfa.EnableTOTP(bg, alice, mfatest.TOTP(t, setup.Secret, *clock))
	must(err)
	cer, err := mfa.BeginRegistration(bg, alice, "MacBook 🔑")
	must(err)
	opts, err := json.Marshal(cer.Options)
	must(err)
	key := mfatest.New(t, seedRPID, seedOrigin)
	_, err = mfa.FinishRegistration(bg, alice, cer.ChallengeID, key.Register(t, opts), "MacBook 🔑")
	must(err)

	// Subscription guard: an age key pair, access log rows.
	on := true
	_, _, err = guard.GenerateKey(bg, a, alice.ID, false, &on)
	must(err)
	guard.Record(a, guard.Access{UserID: alice.ID, Token: alice.Token, IP: "203.0.113.7", UserAgent: "clash-verge/v2.0.0", Format: "clash-meta"})
	guard.Record(a, guard.Access{UserID: alice.ID, Token: alice.Token, IP: "2001:db8:1:2::5", UserAgent: "sing-box 1.14.4", Format: "singbox"})
	must(guard.FlushAccess(bg, a))

	// Traffic (stat_users, stat_nodes, users.u/d) and the daily business summary.
	a.Traffic.Add(nodes[0].ID, alice.ID, nodes[0].Rate, 1<<20, 5<<20)
	a.Traffic.Add(nodes[1].ID, alice.ID, nodes[1].Rate, 7, 0)
	must(a.FlushTraffic(bg))
	_, err = statsvc.New(a).SaveDay(bg, now)
	must(err)

	must(a.Mailer.Enqueue(mail.Message{To: "alice@example.com", Subject: "测试 ✉️", Text: "hello", Template: "test"}))
	testutil.Mails(t, a)

	for _, m := range model.All() {
		var n int64
		must(d.Model(m).Count(&n).Error)
		if n == 0 {
			t.Fatalf("seed: table of %T is empty; every table must have rows so the copy test covers it", m)
		}
	}
}

func openSQLite(t *testing.T, path string) *gorm.DB {
	t.Helper()
	d, err := db.Open(config.DatabaseConfig{Driver: config.DriverSQLite, DSN: path}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close(d) })
	return d
}

// modelRows loads every row of m's table through GORM (an independent path from the copier: model types, scanners,
// JSON decoding) keyed by primary key, each row rendered as "column=value" lines. Timestamps are compared as UTC
// instants truncated to prec; date columns as calendar days.
func modelRows(t *testing.T, d *gorm.DB, m any, prec time.Duration) map[string]string {
	t.Helper()
	stmt := &gorm.Statement{DB: d}
	if err := stmt.Parse(m); err != nil {
		t.Fatal(err)
	}
	s := stmt.Schema
	list := reflect.New(reflect.SliceOf(reflect.TypeOf(m).Elem()))
	if err := d.Session(&gorm.Session{SkipHooks: true}).Unscoped().Model(m).Find(list.Interface()).Error; err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	rows := list.Elem()
	for i := range rows.Len() {
		rv := rows.Index(i)
		var b strings.Builder
		for _, name := range s.DBNames {
			f := s.FieldsByDBName[name]
			fmt.Fprintf(&b, "%s=%s\n", name, render(f, f.ReflectValueOf(bg, rv).Interface(), prec))
		}
		out[render(s.PrioritizedPrimaryField, s.PrioritizedPrimaryField.ReflectValueOf(bg, rv).Interface(), prec)] = b.String()
	}
	return out
}

func render(f *schema.Field, v any, prec time.Duration) string {
	norm := func(t time.Time) string {
		if strings.EqualFold(string(f.DataType), "date") {
			return t.Format(time.DateOnly)
		}
		return t.Truncate(prec).UTC().Format(time.RFC3339Nano)
	}
	switch x := v.(type) {
	case time.Time:
		return norm(x)
	case *time.Time:
		if x == nil {
			return "<nil>"
		}
		return "&" + norm(*x)
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return "<nil>"
		}
		return fmt.Sprintf("&%#v", rv.Elem().Interface())
	}
	return fmt.Sprintf("%#v", v)
}

// compareModels asserts that every table of model.All() holds the same rows in a and b (through GORM models).
func compareModels(t *testing.T, a, b *gorm.DB, prec time.Duration) {
	t.Helper()
	for _, m := range model.All() {
		ra, rb := modelRows(t, a, m, prec), modelRows(t, b, m, prec)
		if len(ra) == 0 {
			t.Errorf("%T: no rows to compare", m)
		}
		if !reflect.DeepEqual(ra, rb) {
			for k, v := range ra {
				if rb[k] != v {
					t.Errorf("%T row %s:\nsource %s\ntarget %s", m, k, v, rb[k])
				}
			}
			for k := range rb {
				if _, ok := ra[k]; !ok {
					t.Errorf("%T row %s exists only in the target", m, k)
				}
			}
		}
	}
}

// compareSQLiteRaw asserts that two SQLite databases store byte-identical values (SQLite's quote(): storage class and
// exact text/number/blob) in every column of every table; with skipTimes, datetime columns are left to compareModels.
func compareSQLiteRaw(t *testing.T, a, b *gorm.DB, skipTimes bool) {
	t.Helper()
	for _, m := range model.All() {
		stmt := &gorm.Statement{DB: a}
		if err := stmt.Parse(m); err != nil {
			t.Fatal(err)
		}
		s := stmt.Schema
		var cols []string
		for _, name := range s.DBNames {
			if skipTimes && s.FieldsByDBName[name].GORMDataType == schema.Time && !strings.EqualFold(string(s.FieldsByDBName[name].DataType), "date") {
				continue
			}
			cols = append(cols, `quote("`+name+`")`)
		}
		q := "SELECT " + strings.Join(cols, " || '|' || ") + ` FROM "` + s.Table + `" ORDER BY "` + s.PrioritizedPrimaryField.DBName + `"`
		var va, vb []string
		if err := a.Raw(q).Scan(&va).Error; err != nil {
			t.Fatal(err)
		}
		if err := b.Raw(q).Scan(&vb).Error; err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(va, vb) {
			t.Errorf("%s differs:\nsource %q\ntarget %q", s.Table, va, vb)
		}
	}
}

func counts(t *testing.T, d *gorm.DB) map[string]int64 {
	t.Helper()
	out := map[string]int64{}
	for _, m := range model.All() {
		stmt := &gorm.Statement{DB: d}
		if err := stmt.Parse(m); err != nil {
			t.Fatal(err)
		}
		var n int64
		if err := d.Model(m).Count(&n).Error; err != nil {
			t.Fatal(err)
		}
		out[stmt.Schema.Table] = n
	}
	return out
}

func reportCounts(r *db.CopyReport) map[string]int64 {
	out := map[string]int64{}
	for _, tc := range r.Tables {
		out[tc.Name] = tc.Rows
	}
	return out
}

func TestCopySQLiteToSQLiteEveryTableAndColumn(t *testing.T) {
	a := seedApp(t)
	dst := openSQLite(t, filepath.Join(t.TempDir(), "copy.db"))
	rep, err := db.Copy(bg, a.DB, dst, db.CopyOptions{BatchSize: 2}) // tiny batches: keyset pagination across pages
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Tables) != len(model.All()) || rep.Source != "sqlite" || rep.Target != "sqlite" || rep.DryRun || rep.Sequences != 0 {
		t.Fatalf("report = %+v", rep)
	}
	if rep.TimePrecision != time.Nanosecond {
		t.Fatalf("sqlite keeps nanoseconds, report says %v", rep.TimePrecision)
	}
	if want := counts(t, a.DB); !reflect.DeepEqual(reportCounts(rep), want) {
		t.Fatalf("report rows %v, source has %v", reportCounts(rep), want)
	}
	for _, tc := range rep.Tables {
		if len(tc.Digest) != 64 {
			t.Fatalf("%s digest %q", tc.Name, tc.Digest)
		}
	}
	compareModels(t, a.DB, dst, time.Nanosecond)
	compareSQLiteRaw(t, a.DB, dst, false)

	var raw string
	if err := dst.Raw("SELECT protocol_settings FROM nodes WHERE protocol_settings LIKE ?", "%big%").Scan(&raw).Error; err != nil || raw != rawNodeSettings {
		t.Fatalf("raw JSON = %q %v", raw, err)
	}
	// New rows continue after the copied ids.
	var maxID uint64
	if err := a.DB.Model(&model.Node{}).Select("MAX(id)").Scan(&maxID).Error; err != nil {
		t.Fatal(err)
	}
	n := model.Node{Type: model.NodeVless, Name: "new", Host: "x.example.com", Port: "443", ServerPort: 443, Rate: 1}
	if err := dst.Create(&n).Error; err != nil || n.ID != maxID+1 {
		t.Fatalf("new node id %d (max %d): %v", n.ID, maxID, err)
	}
}

func TestCopyRefusesNonEmptyTargetUnlessOverwrite(t *testing.T) {
	a := seedApp(t)
	dst := openSQLite(t, filepath.Join(t.TempDir(), "copy.db"))
	if _, err := db.Copy(bg, a.DB, dst, db.CopyOptions{}); err != nil {
		t.Fatal(err)
	}
	before := counts(t, dst)
	_, err := db.Copy(bg, a.DB, dst, db.CopyOptions{})
	if !errors.Is(err, db.ErrTargetNotEmpty) || !strings.Contains(err.Error(), "users ") || !strings.Contains(err.Error(), "--overwrite") {
		t.Fatalf("second copy: %v", err)
	}
	if after := counts(t, dst); !reflect.DeepEqual(after, before) {
		t.Fatalf("refused copy changed the target: %v -> %v", before, after)
	}

	// Drift the target, then overwrite: it must equal the source again and report what it deleted.
	if err := dst.Exec("DELETE FROM users WHERE email = ?", "bob@example.com").Error; err != nil {
		t.Fatal(err)
	}
	if err := dst.Create(&model.Notice{Title: "only in target"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := dst.Exec("UPDATE settings SET value = ? WHERE key = ?", `"changed"`, "app_name").Error; err != nil {
		t.Fatal(err)
	}
	rep, err := db.Copy(bg, a.DB, dst, db.CopyOptions{Overwrite: true})
	if err != nil {
		t.Fatal(err)
	}
	cleared := map[string]int64{}
	for _, tc := range rep.Tables {
		cleared[tc.Name] = tc.Cleared
	}
	if !rep.Overwrite || cleared["users"] != before["users"]-1 || cleared["notices"] != before["notices"]+1 {
		t.Fatalf("overwrite report = %+v", rep)
	}
	compareModels(t, a.DB, dst, time.Nanosecond)
	compareSQLiteRaw(t, a.DB, dst, false)
}

func TestCopyRefusesTheSameDatabase(t *testing.T) {
	a := seedApp(t)
	before := counts(t, a.DB)
	if _, err := db.Copy(bg, a.DB, a.DB, db.CopyOptions{Overwrite: true}); !errors.Is(err, db.ErrSameDatabase) {
		t.Fatalf("same handle: %v", err)
	}
	// The same file through another connection pool and another spelling of its path.
	var file string
	if err := a.DB.Raw("SELECT file FROM pragma_database_list WHERE name = 'main'").Scan(&file).Error; err != nil || file == "" {
		t.Fatalf("main file %q %v", file, err)
	}
	other := openSQLite(t, filepath.Join(filepath.Dir(file), ".", "..", filepath.Base(filepath.Dir(file)), filepath.Base(file)))
	if _, err := db.Copy(bg, a.DB, other, db.CopyOptions{Overwrite: true}); !errors.Is(err, db.ErrSameDatabase) {
		t.Fatalf("same file: %v", err)
	}
	if after := counts(t, a.DB); !reflect.DeepEqual(after, before) {
		t.Fatalf("refused copy changed the source: %v -> %v", before, after)
	}
}

func TestCopyRefusesUnknownTablesAndColumns(t *testing.T) {
	t.Run("source table", func(t *testing.T) {
		a := seedApp(t)
		if err := a.DB.Exec("CREATE TABLE plugin_data (id integer primary key, v text)").Error; err != nil {
			t.Fatal(err)
		}
		dst := openSQLite(t, filepath.Join(t.TempDir(), "copy.db"))
		_, err := db.Copy(bg, a.DB, dst, db.CopyOptions{})
		if !errors.Is(err, db.ErrUnknownTable) || !strings.Contains(err.Error(), "plugin_data") || !strings.Contains(err.Error(), "source") {
			t.Fatalf("copy: %v", err)
		}
		if names, err := dst.Migrator().GetTables(); err != nil || len(names) != 0 {
			t.Fatalf("the target must stay untouched: %v %v", names, err)
		}
	})
	t.Run("target table", func(t *testing.T) {
		a := seedApp(t)
		dst := openSQLite(t, filepath.Join(t.TempDir(), "copy.db"))
		if err := dst.Exec("CREATE TABLE v2_user (id integer primary key)").Error; err != nil {
			t.Fatal(err)
		}
		_, err := db.Copy(bg, a.DB, dst, db.CopyOptions{Overwrite: true})
		if !errors.Is(err, db.ErrUnknownTable) || !strings.Contains(err.Error(), "v2_user") || !strings.Contains(err.Error(), "target") {
			t.Fatalf("copy: %v", err)
		}
		if names, err := dst.Migrator().GetTables(); err != nil || len(names) != 1 {
			t.Fatalf("migrate must not run on a foreign database: %v %v", names, err)
		}
	})
	t.Run("source column", func(t *testing.T) {
		a := seedApp(t)
		if err := a.DB.Exec("ALTER TABLE users ADD COLUMN legacy_note text").Error; err != nil {
			t.Fatal(err)
		}
		_, err := db.Copy(bg, a.DB, openSQLite(t, filepath.Join(t.TempDir(), "copy.db")), db.CopyOptions{})
		if !errors.Is(err, db.ErrUnknownColumn) || !strings.Contains(err.Error(), "users") || !strings.Contains(err.Error(), "legacy_note") {
			t.Fatalf("copy: %v", err)
		}
	})
}

func TestCopyDryRunWritesNothing(t *testing.T) {
	a := seedApp(t)
	dst := openSQLite(t, filepath.Join(t.TempDir(), "copy.db"))
	rep, err := db.Copy(bg, a.DB, dst, db.CopyOptions{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if !rep.DryRun || rep.TotalRows() == 0 || !reflect.DeepEqual(reportCounts(rep), counts(t, a.DB)) {
		t.Fatalf("dry run report = %+v", rep)
	}
	for table, n := range counts(t, dst) {
		if n != 0 {
			t.Fatalf("dry run left %d rows in %s", n, table)
		}
	}
	// The real copy afterwards finds the (empty) schema and fills it.
	if _, err := db.Copy(bg, a.DB, dst, db.CopyOptions{}); err != nil {
		t.Fatal(err)
	}
	compareModels(t, a.DB, dst, time.Nanosecond)
}

// The verification reads both sides back: a target that changes, drops or adds rows on insert (SQLite triggers stand
// in for a broken driver or type conversion) fails the copy with ErrVerify and leaves the target empty.
func TestCopyVerificationDetectsDifferences(t *testing.T) {
	a := seedApp(t)
	cases := map[string]struct {
		trigger string
		want    []string
	}{
		"changed column": {
			trigger: "CREATE TRIGGER tamper AFTER INSERT ON users BEGIN UPDATE users SET remarks = remarks || '!' WHERE id = NEW.id; END",
			want:    []string{"users: id ", "differs in remarks"},
		},
		"missing row": {
			trigger: "CREATE TRIGGER tamper AFTER INSERT ON tickets BEGIN DELETE FROM tickets WHERE id = NEW.id; END",
			want:    []string{"tickets: ", "copied, 0 in the target", "is missing in the target"},
		},
		"extra row": {
			trigger: "CREATE TRIGGER tamper AFTER INSERT ON notices WHEN NEW.id < 1000 BEGIN " +
				"INSERT INTO notices (id, title, content, img_url, tags, show, sort, created_at, updated_at) " +
				"VALUES (NEW.id + 1000, 'x', '', '', '[]', 0, 0, NEW.created_at, NEW.updated_at); END",
			want: []string{"notices: ", "exists only in the target"},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			dst := openSQLite(t, filepath.Join(t.TempDir(), "copy.db"))
			if err := db.Migrate(dst); err != nil {
				t.Fatal(err)
			}
			if err := dst.Exec(tc.trigger).Error; err != nil {
				t.Fatal(err)
			}
			_, err := db.Copy(bg, a.DB, dst, db.CopyOptions{})
			if !errors.Is(err, db.ErrVerify) {
				t.Fatalf("copy: %v", err)
			}
			for _, w := range tc.want {
				if !strings.Contains(err.Error(), w) {
					t.Errorf("error %q lacks %q", err, w)
				}
			}
			for table, n := range counts(t, dst) {
				if n != 0 {
					t.Fatalf("failed copy left %d rows in %s", n, table)
				}
			}
		})
	}
}

// The copy order needs no foreign-key handling and soft-deleted rows need no Unscoped only while these hold; a model
// change that breaks them must revisit db.Copy.
func TestCopyAssumptionsNoForeignKeysNoSoftDelete(t *testing.T) {
	d := openSQLite(t, filepath.Join(t.TempDir(), "schema.db"))
	if err := db.Migrate(d); err != nil {
		t.Fatal(err)
	}
	deletedAt := reflect.TypeFor[gorm.DeletedAt]()
	for _, m := range model.All() {
		stmt := &gorm.Statement{DB: d}
		if err := stmt.Parse(m); err != nil {
			t.Fatal(err)
		}
		var fks []map[string]any
		if err := d.Raw("SELECT * FROM pragma_foreign_key_list(?)", stmt.Schema.Table).Scan(&fks).Error; err != nil {
			t.Fatal(err)
		}
		if len(fks) > 0 {
			t.Errorf("%s has foreign keys %v", stmt.Schema.Table, fks)
		}
		for _, f := range stmt.Schema.Fields {
			if f.FieldType == deletedAt {
				t.Errorf("%s.%s is a gorm.DeletedAt", stmt.Schema.Table, f.Name)
			}
		}
	}
	names := make([]string, 0, len(model.All()))
	for _, m := range model.All() {
		stmt := &gorm.Statement{DB: d}
		_ = stmt.Parse(m)
		names = append(names, stmt.Schema.Table)
	}
	got, err := d.Migrator().GetTables()
	if err != nil {
		t.Fatal(err)
	}
	got = slicesWithout(got, "sqlite_sequence")
	sort.Strings(got)
	sort.Strings(names)
	if !reflect.DeepEqual(got, names) {
		t.Fatalf("migrate creates %v, model.All() has %v: every table must be a model so db copy covers it", got, names)
	}
}

func slicesWithout(s []string, drop string) []string {
	out := s[:0]
	for _, v := range s {
		if v != drop {
			out = append(out, v)
		}
	}
	return out
}
