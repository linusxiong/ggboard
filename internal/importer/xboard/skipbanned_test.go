package xboardimport

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/cli"
	"github.com/linusxiong/ggboard/internal/console"
	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/testutil"
)

// --skip-banned (docs/IMPORT_XBOARD.md §3.5).

const fixtureBcrypt = "$2y$10$JSVs9TpWmq34RiU1ShgOYu/q/tsdYkEto6AuJr5.WOgSkUT8QiiJK"

// legacyBannedSQL adds rows that belong to, or point at, banned users to the legacy seed (user 12 is banned there
// already): banned staff member 15, user 16 invited by 12, a code, logs, tickets and stats of 12 and 15, a log
// whose invitee is 12, and a kept ticket answered last by 15.
var legacyBannedSQL = []string{
	`INSERT INTO "v2_user" ("id", "invite_user_id", "email", "password", "banned", "is_admin", "is_staff", "uuid", "token", "created_at", "updated_at") VALUES
	(15, NULL, 'bannedstaff@example.com', '` + fixtureBcrypt + `', 1, 0, 1, '00000000-0000-4000-8000-000000000015', '00000000000000000000000000000015', 1690000000, 1700000000),
	(16, 12, 'invitee16@example.com', '` + fixtureBcrypt + `', 0, 0, 0, '00000000-0000-4000-8000-000000000016', '00000000000000000000000000000016', 1690000000, 1700000000);`,
	`INSERT INTO "v2_invite_code" ("id", "user_id", "code", "status", "pv", "created_at", "updated_at") VALUES
	(4, 12, 'BANNED12', 0, 0, 1700000000, 1700000000);`,
	`INSERT INTO "v2_commission_log" ("id", "invite_user_id", "user_id", "trade_no", "order_amount", "get_amount", "created_at", "updated_at") VALUES
	(3, 12, 2, '2024010112000012345680', 3000, 300, 1700000000, 1700000000),
	(4, 1, 12, '2024010112000012345681', 4000, 400, 1700000000, 1700000000);`,
	`INSERT INTO "v2_ticket" ("id", "user_id", "subject", "level", "status", "reply_status", "created_at", "updated_at") VALUES
	(5, 12, 'Banned user ticket', 0, 0, 0, 1700000000, 1700000100),
	(6, 3, 'Answered by banned staff', 0, 0, 0, 1700000000, 1700000300);`,
	`INSERT INTO "v2_ticket_message" ("id", "user_id", "ticket_id", "message", "created_at", "updated_at") VALUES
	(6, 12, 5, 'banned user asks', 1700000000, 1700000000),
	(7, 1, 5, 'staff answers the banned user', 1700000100, 1700000100),
	(8, 3, 6, 'kept user asks', 1700000200, 1700000200),
	(9, 15, 6, 'banned staff answers', 1700000300, 1700000300);`,
	`INSERT INTO "v2_stat_user" ("id", "user_id", "server_rate", "u", "d", "record_type", "record_at", "created_at", "updated_at") VALUES
	(5, 12, 1.00, 5, 5, 'd', 1729440000, 1729440000, 1729440000),
	(6, 15, 1.00, 7, 7, 'd', 1729440000, 1729440000, 1729440000);`,
}

// upstreamBannedSQL bans the upstream seed's only admin (user 1: inviter of 2, owner of the code and the log, staff
// reply in ticket 2, stored last replier of ticket 1) and adds banned user 3 with a ticket, a stat row and a log
// whose inviter (2) is kept.
var upstreamBannedSQL = []string{
	`UPDATE "v2_user" SET "banned" = 1 WHERE "id" = 1;`,
	`INSERT INTO "v2_user" ("id", "invite_user_id", "email", "password", "banned", "uuid", "token", "created_at", "updated_at") VALUES
	(3, 2, 'banned3@example.com', '` + fixtureBcrypt + `', 1, '10000000-0000-4000-8000-000000000003', 'a0000000000000000000000000000003', 1690000000, 1700000000);`,
	`INSERT INTO "v2_ticket" ("id", "user_id", "subject", "level", "status", "reply_status", "created_at", "updated_at", "last_reply_user_id") VALUES
	(3, 3, 'Banned user ticket', 0, 0, 0, 1700000000, 1700000000, 3);`,
	`INSERT INTO "v2_ticket_message" ("id", "user_id", "ticket_id", "message", "created_at", "updated_at") VALUES
	(3, 3, 3, 'banned user asks', 1700000000, 1700000000);`,
	`INSERT INTO "v2_commission_log" ("id", "invite_user_id", "user_id", "trade_no", "order_amount", "get_amount", "created_at", "updated_at") VALUES
	(2, 2, 3, '2025010112000012345679', 500, 50, 1700000000, 1700000000);`,
	`INSERT INTO "v2_stat_user" ("id", "user_id", "server_rate", "u", "d", "record_type", "record_at", "created_at", "updated_at") VALUES
	(2, 3, 1.00, 10, 20, 'd', 1729440000, 1729440000, 1729440000);`,
}

func withStats(o *Options) { o.WithStats = true; o.StatsDays = 1000 }

func skipBanned(o *Options) { o.SkipBanned = true }

// applySkipRules turns an import without the flag into the expected result of one with it, by the per-table rules
// of §3.5 (the same order: a banned owner's tickets go with their messages before other messages are rewritten).
func applySkipRules(t *testing.T, a *app.App, banned []uint64) {
	t.Helper()
	for _, q := range []string{
		"DELETE FROM ticket_messages WHERE ticket_id IN (SELECT id FROM tickets WHERE user_id IN ?)",
		"DELETE FROM tickets WHERE user_id IN ?",
		"UPDATE ticket_messages SET user_id = 0 WHERE user_id IN ?",
		"UPDATE tickets SET last_reply_user_id = NULL WHERE last_reply_user_id IN ?",
		"DELETE FROM invite_codes WHERE user_id IN ?",
		"DELETE FROM commission_logs WHERE invite_user_id IN ?",
		"UPDATE commission_logs SET user_id = 0 WHERE user_id IN ?",
		"DELETE FROM stat_users WHERE user_id IN ?",
		"UPDATE users SET invite_user_id = NULL WHERE invite_user_id IN ?",
		"DELETE FROM users WHERE id IN ?",
	} {
		if err := a.DB.Exec(q, banned).Error; err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
}

// maskSkipCompare masks the per-run random values and the fresh stat_* ids (§3.2: assigned in insert order, and
// SQLite's AUTOINCREMENT keeps counting after a --force).
func maskSkipCompare(table string, r map[string]any) {
	maskRegenerated(table, r)
	if strings.HasPrefix(table, "stat") {
		delete(r, "id")
	}
}

// userRefs are every target column that holds a user id.
var userRefs = []struct{ table, col string }{
	{"users", "id"}, {"users", "invite_user_id"}, {"invite_codes", "user_id"}, {"commission_logs", "invite_user_id"},
	{"commission_logs", "user_id"}, {"tickets", "user_id"}, {"tickets", "last_reply_user_id"}, {"ticket_messages", "user_id"},
	{"stat_users", "user_id"}, {"orders", "user_id"}, {"orders", "invite_user_id"}, {"passkeys", "user_id"},
	{"user_mfa", "user_id"}, {"subscription_keys", "user_id"}, {"subscription_access", "user_id"}, {"rule_profiles", "user_id"},
}

func assertNoReference(t *testing.T, a *app.App, ids []uint64) {
	t.Helper()
	for _, ref := range userRefs {
		var n int64
		if err := a.DB.Table(ref.table).Where(ref.col+" IN ?", ids).Count(&n).Error; err != nil {
			t.Fatalf("%s.%s: %v", ref.table, ref.col, err)
		}
		if n != 0 {
			t.Errorf("%s.%s: %d rows still reference a skipped user %v", ref.table, ref.col, n, ids)
		}
	}
}

type skipCase struct {
	flavor  Flavor
	extra   []string
	banned  []uint64
	want    BannedSkip
	counts  map[string][3]int // source, imported, skipped with the flag
	without map[string][3]int // the same lines without the flag
	notes   map[string]string // note prefix with the flag
	warns   []struct{ scope, text string }
	emails  []string // banned users' addresses: never logged
}

func runSkipCase(t *testing.T, c skipCase) {
	t.Helper()
	src := newSource(t, c.flavor, c.extra...)

	// without the flag: nothing changes, banned users and their rows are imported
	plain := testutil.NewTestApp(t)
	prep, err := importInto(t, plain, src, withStats)
	if err != nil {
		t.Fatal(err)
	}
	if prep.SkipBanned || prep.Banned != (BannedSkip{}) {
		t.Errorf("without the flag: SkipBanned %v, Banned %+v", prep.SkipBanned, prep.Banned)
	}
	for name, w := range c.without {
		if st := prep.Lookup(name); st == nil || [3]int{st.Source, st.Imported, st.Skipped} != w {
			t.Errorf("without the flag, %s: %+v, want %v", name, st, w)
		}
		if st := prep.Lookup(name); st != nil && strings.Contains(st.Note, "banned") {
			t.Errorf("without the flag, %s note %q mentions banned users", name, st.Note)
		}
	}
	if hasWarning(prep, "", "--skip-banned") {
		t.Error("without the flag no warning may mention --skip-banned")
	}
	var banned int64
	plain.DB.Model(&model.User{}).Where("banned = ?", true).Count(&banned)
	if int(banned) != len(c.banned) {
		t.Errorf("without the flag %d banned users imported, want %d", banned, len(c.banned))
	}
	plainDump := dump(t, plain, nil)

	// with the flag
	a := testutil.NewTestApp(t)
	rep, err := importInto(t, a, src, withStats, skipBanned)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.SkipBanned || rep.Banned != c.want {
		t.Errorf("Banned = %+v, want %+v", rep.Banned, c.want)
	}
	for name, w := range c.counts {
		st := rep.Lookup(name)
		if st == nil || [3]int{st.Source, st.Imported, st.Skipped} != w {
			t.Errorf("%s: %+v, want %v", name, st, w)
			continue
		}
		if p := c.notes[name]; !strings.HasPrefix(st.Note, p) {
			t.Errorf("%s note %q, want prefix %q", name, st.Note, p)
		}
	}
	for _, w := range c.warns {
		if !hasWarning(rep, w.scope, w.text) {
			t.Errorf("missing warning [%s] %q", w.scope, w.text)
		}
	}
	assertNoReference(t, a, c.banned)

	// kept data is byte-identical to the import without the flag after the rules of §3.5
	applySkipRules(t, plain, c.banned)
	if d := dumpDiff(dump(t, a, maskSkipCompare), maskDumpWith(t, dump(t, plain, nil), maskSkipCompare)); d != "" {
		t.Errorf("--skip-banned differs from the rules applied to a full import:\n%s", d)
	}

	// summary: the users line, the mode and the checklist; INFO summary in the log, no banned e-mail anywhere
	var out bytes.Buffer
	rep.WriteSummary(&out)
	u := rep.Lookup("users")
	line := fmt.Sprintf("%-20s %8d %9d %8d   skipped banned %d · bcrypt ", "users", u.Source, u.Imported, u.Skipped, c.want.Users)
	for _, s := range []string{line, "· " + skipBannedMode + " ·", fmt.Sprintf("Banned users: %d were not imported", c.want.Users)} {
		if !strings.Contains(out.String(), s) {
			t.Errorf("summary lacks %q:\n%s", s, out.String())
		}
	}
	var info map[string]any
	for _, e := range a.Logs.Entries(slog.LevelDebug, 0) {
		if e.Msg == "xboard import skip-banned" {
			if e.Level != "info" {
				t.Errorf("skip-banned summary logged at %q, want info", e.Level)
			}
			info = e.Attrs
		}
		text := e.Msg + fmt.Sprint(e.Attrs)
		for _, email := range c.emails {
			if strings.Contains(strings.ToLower(text), email) {
				t.Errorf("log entry mentions banned user %s: %s", email, text)
			}
		}
	}
	if info == nil {
		t.Fatal("no INFO summary of --skip-banned in the log")
	}
	for k, v := range map[string]int{"users": c.want.Users, "tickets": c.want.Tickets, "stat_users": c.want.StatUsers,
		"invite_user_id_nulled": c.want.InviteUserIDNulled, "message_user_id_zeroed": c.want.MessageUserIDZeroed} {
		if fmt.Sprint(info[k]) != fmt.Sprint(v) {
			t.Errorf("log attr %s = %v, want %d", k, info[k], v)
		}
	}

	// --dry-run reports exactly the real run and writes nothing
	dryApp := testutil.NewTestApp(t)
	dry, err := importInto(t, dryApp, src, withStats, skipBanned, func(o *Options) { o.DryRun = true })
	if err != nil {
		t.Fatal(err)
	}
	targetEmpty(t, dryApp)
	if !reflect.DeepEqual(dry.Tables, rep.Tables) || !reflect.DeepEqual(dry.Warnings, rep.Warnings) || dry.Banned != rep.Banned {
		t.Errorf("dry run differs from the real run:\n%+v\n%+v", dry.Banned, rep.Banned)
	}

	// --force: with the flag it removes banned users an earlier full import brought in, and the reverse
	forced := testutil.NewTestApp(t)
	if _, err := importInto(t, forced, src, withStats); err != nil {
		t.Fatal(err)
	}
	if _, err := importInto(t, forced, src, withStats, skipBanned, func(o *Options) { o.Force = true }); err != nil {
		t.Fatal(err)
	}
	if d := dumpDiff(dump(t, forced, maskSkipCompare), dump(t, a, maskSkipCompare)); d != "" {
		t.Errorf("--force --skip-banned after a full import:\n%s", d)
	}
	assertNoReference(t, forced, c.banned)
	if _, err := importInto(t, forced, src, withStats, func(o *Options) { o.Force = true }); err != nil {
		t.Fatal(err)
	}
	if d := dumpDiff(dump(t, forced, maskSkipCompare), maskDumpWith(t, plainDump, maskSkipCompare)); d != "" {
		t.Errorf("--force without the flag after a --skip-banned import:\n%s", d)
	}
}

// dumpDiff returns the table lines of two dumps that differ ("" when the dumps are equal).
func dumpDiff(got, want string) string {
	g, w := strings.Split(got, "\n"), strings.Split(want, "\n")
	var out strings.Builder
	for i := range max(len(g), len(w)) {
		var a, b string
		if i < len(g) {
			a = g[i]
		}
		if i < len(w) {
			b = w[i]
		}
		if a == b {
			continue
		}
		// print only the rows that differ
		name, ga, _ := strings.Cut(a, " ")
		_, wb, _ := strings.Cut(b, " ")
		var gr, wr []json.RawMessage
		if json.Unmarshal([]byte(ga), &gr) != nil || json.Unmarshal([]byte(wb), &wr) != nil || len(gr) != len(wr) {
			fmt.Fprintf(&out, "got:  %s\nwant: %s\n", a, b)
			continue
		}
		for j := range gr {
			if !bytes.Equal(gr[j], wr[j]) {
				fmt.Fprintf(&out, "%s got:  %s\n%s want: %s\n", name, gr[j], name, wr[j])
			}
		}
	}
	return out.String()
}

// maskDumpWith applies mask to a dump produced without one.
func maskDumpWith(t *testing.T, d string, mask func(string, map[string]any)) string {
	t.Helper()
	var out strings.Builder
	for _, line := range strings.Split(strings.TrimSuffix(d, "\n"), "\n") {
		name, body, _ := strings.Cut(line, " ")
		var rows []map[string]any
		if err := json.Unmarshal([]byte(body), &rows); err != nil {
			t.Fatal(err)
		}
		for _, r := range rows {
			mask(name, r)
		}
		b, _ := json.Marshal(rows)
		out.WriteString(name + " " + string(b) + "\n")
	}
	return out.String()
}

func TestSkipBannedLegacy(t *testing.T) {
	runSkipCase(t, skipCase{
		flavor: FlavorLegacy,
		extra:  legacyBannedSQL,
		banned: []uint64{12, 15},
		want: BannedSkip{Users: 2, AdminsOrStaff: 1, InviteCodes: 1, CommissionLogs: 1, Tickets: 1, TicketMessages: 2,
			StatUsers: 2, InviteUserIDNulled: 1, CommissionUserIDZeroed: 1, LastReplyUserIDNulled: 1, MessageUserIDZeroed: 1},
		counts: map[string][3]int{
			"users": {16, 14, 2}, "invite_codes": {4, 1, 3}, "commission_logs": {4, 3, 1}, "tickets": {6, 4, 2},
			"ticket_messages": {9, 6, 3}, "stat_users": {5, 1, 3},
		},
		without: map[string][3]int{
			"users": {16, 16, 0}, "invite_codes": {4, 2, 2}, "commission_logs": {4, 4, 0}, "tickets": {6, 5, 1},
			"ticket_messages": {9, 8, 1}, "stat_users": {5, 3, 1},
		},
		notes: map[string]string{
			"users": "skipped banned 2 · bcrypt ", "invite_codes": "skipped banned 1", "commission_logs": "skipped banned 1",
			"tickets": "skipped banned 1/2", "ticket_messages": "", "stat_users": "skipped banned 2 · aggregated",
		},
		warns: []struct{ scope, text string }{
			{"users#15", "banned staff member not imported"},
			{"users", "invite_user_id of 1 users pointed to a banned user → NULL"},
			{"commission_logs", "1 logs whose invitee is banned are kept for their inviter with user_id 0"},
			{"tickets", "last_reply_user_id of 1 tickets pointed to a banned user → NULL"},
			{"ticket_messages", "1 messages written by banned users in kept tickets are kept with user_id 0"},
			{"ticket_messages", "1 messages of skipped tickets were skipped"}, // the orphan ticket of the seed, not the banned one
			{"stat_users", "1 rows of users that were not imported"},
			{"tickets#4", "user 99 not found"},
		},
		emails: []string{"banned@example.com", "bannedstaff@example.com"},
	})
}

func TestSkipBannedUpstream(t *testing.T) {
	runSkipCase(t, skipCase{
		flavor: FlavorUpstream,
		extra:  upstreamBannedSQL,
		banned: []uint64{1, 3},
		want: BannedSkip{Users: 2, AdminsOrStaff: 1, InviteCodes: 1, CommissionLogs: 1, Tickets: 1, TicketMessages: 1,
			StatUsers: 1, InviteUserIDNulled: 1, CommissionUserIDZeroed: 1, LastReplyUserIDNulled: 1, MessageUserIDZeroed: 1},
		counts: map[string][3]int{
			"users": {3, 1, 2}, "invite_codes": {1, 0, 1}, "commission_logs": {2, 1, 1}, "tickets": {3, 2, 1},
			"ticket_messages": {3, 2, 1}, "stat_users": {2, 1, 1},
		},
		without: map[string][3]int{
			"users": {3, 3, 0}, "invite_codes": {1, 1, 0}, "commission_logs": {2, 2, 0}, "tickets": {3, 3, 0},
			"ticket_messages": {3, 3, 0}, "stat_users": {2, 2, 0},
		},
		notes: map[string]string{
			"users": "skipped banned 2 · bcrypt ", "invite_codes": "skipped banned 1", "commission_logs": "skipped banned 1",
			"tickets": "skipped banned 1/1", "stat_users": "skipped banned 1 · aggregated",
		},
		warns: []struct{ scope, text string }{
			{"users#1", "banned admin not imported"},
			{"users", "no admin was imported"},
			{"users", "invite_user_id of 1 users pointed to a banned user → NULL"},
			{"commission_logs", "1 logs whose invitee is banned"},
			{"tickets", "last_reply_user_id of 1 tickets"},
			{"ticket_messages", "1 messages written by banned users"},
		},
		emails: []string{"admin@example.com", "banned3@example.com"},
	})

	// the kept rows themselves: the staff reply survives as user 0, the stored last replier is cleared, the
	// derived reply status is unchanged
	src := newSource(t, FlavorUpstream, upstreamBannedSQL...)
	b := testutil.NewTestApp(t)
	if _, err := importInto(t, b, src, skipBanned); err != nil {
		t.Fatal(err)
	}
	var tickets []model.Ticket
	b.DB.Order("id").Preload("Messages", func(db *gorm.DB) *gorm.DB { return db.Order("id") }).Find(&tickets)
	two := uint64(2)
	if len(tickets) != 2 || tickets[0].LastReplyUserID != nil || tickets[0].ReplyStatus != model.TicketReplied ||
		!reflect.DeepEqual(tickets[1].LastReplyUserID, &two) || tickets[1].ReplyStatus != model.TicketAwaitingStaff ||
		len(tickets[1].Messages) != 2 || tickets[1].Messages[0].UserID != 0 || tickets[1].Messages[0].Message != "Hello" {
		t.Errorf("kept tickets: %+v", tickets)
	}
	var logs []model.CommissionLog
	b.DB.Order("id").Find(&logs)
	if len(logs) != 1 || logs[0].ID != 2 || logs[0].InviteUserID != 2 || logs[0].UserID != 0 || logs[0].GetAmount != 50 {
		t.Errorf("commission logs: %+v", logs)
	}
	if u := loadUser(t, b, 2); u.InviteUserID != nil {
		t.Errorf("user 2 invite_user_id = %v, want NULL", *u.InviteUserID)
	}
}

// Collisions are resolved among imported users only: a kept user that collided with a banned one keeps its
// address, uuid, token and invite code.
func TestSkipBannedCollisions(t *testing.T) {
	extra := []string{
		`INSERT INTO "v2_user" ("id", "email", "password", "banned", "last_login_at", "uuid", "token", "created_at", "updated_at") VALUES
		(15, 'Clash@example.com', '` + fixtureBcrypt + `', 1, 1700000900, '00000000-0000-4000-8000-000000000016', '00000000000000000000000000000016', 1690000000, 1700000000),
		(16, 'clash@example.com', '` + fixtureBcrypt + `', 0, 1700000100, '00000000-0000-4000-8000-000000000016', '00000000000000000000000000000016', 1690000000, 1700000000);`,
		`INSERT INTO "v2_invite_code" ("id", "user_id", "code", "status", "pv", "created_at", "updated_at") VALUES
		(4, 15, 'CLASHCODE', 0, 0, 1700000000, 1700000000), (5, 16, 'clashcode', 0, 0, 1700000000, 1700000000);`,
	}
	src := newSource(t, FlavorLegacy, extra...)

	plain := testutil.NewTestApp(t)
	prep, err := importInto(t, plain, src)
	if err != nil {
		t.Fatal(err)
	}
	if u := loadUser(t, plain, 16); u.Email != "clash+dup16@example.com" || u.UUID == "00000000-0000-4000-8000-000000000016" ||
		u.Token == "00000000000000000000000000000016" {
		t.Errorf("without the flag user 16 must lose the collisions: %q %q %q", u.Email, u.UUID, u.Token)
	}
	if !hasWarning(prep, "invite_codes#5", "duplicate code") {
		t.Error("without the flag code #5 duplicates #4")
	}

	a := testutil.NewTestApp(t)
	rep, err := importInto(t, a, src, skipBanned)
	if err != nil {
		t.Fatal(err)
	}
	if u := loadUser(t, a, 16); u.Email != "clash@example.com" || u.UUID != "00000000-0000-4000-8000-000000000016" ||
		u.Token != "00000000000000000000000000000016" {
		t.Errorf("with the flag user 16 keeps its values: %q %q %q", u.Email, u.UUID, u.Token)
	}
	var codes []model.InviteCode
	a.DB.Order("id").Find(&codes)
	if len(codes) != 2 || codes[1].ID != 5 || codes[1].UserID != 16 {
		t.Errorf("invite codes: %+v", codes)
	}
	for _, scope := range []string{"users#16", "invite_codes#5"} {
		if len(rep.WarningsFor(scope)) != 0 {
			t.Errorf("%s: no collision with a banned user may be reported: %v", scope, rep.WarningsFor(scope))
		}
	}
}

// The styled report shows the flag like --force; the plain one is still exactly WriteSummary.
func TestSkipBannedReportPrint(t *testing.T) {
	r := printReport()
	r.SkipBanned, r.Banned = true, BannedSkip{Users: 1}
	var plain, want bytes.Buffer
	r.Print(console.New(&plain, console.Style{}))
	r.WriteSummary(&want)
	if plain.String() != want.String() || !strings.Contains(want.String(), "mode: DRY-RUN (rolled back) · "+skipBannedMode+" · target") {
		t.Fatalf("plain:\n%s", plain.String())
	}
	var pretty bytes.Buffer
	r.Print(console.New(&pretty, console.Style{Pretty: true}))
	for _, s := range []string{"Skip banned", skipBannedMode, "Banned users: 1 were not imported"} {
		if !strings.Contains(pretty.String(), s) {
			t.Errorf("styled report lacks %q:\n%s", s, pretty.String())
		}
	}
}

// The cobra command parses --skip-banned (off by default) and passes it through to Run.
func TestCLISkipBanned(t *testing.T) {
	testutil.FastHashing()
	dir := t.TempDir()
	target := filepath.Join(dir, "ggboard.db")
	src := newSource(t, FlavorUpstream, upstreamBannedSQL...)
	run := func(args ...string) (int, string, string) {
		var out, errb bytes.Buffer
		base := []string{"--data-dir", dir, "--db-driver", "sqlite", "--db-dsn", target, "--log-level", "error"}
		code := cli.Run(context.Background(), "test", append(base, args...), &out, &errb)
		return code, out.String(), errb.String()
	}
	users := func() int64 {
		db, err := gorm.Open(sqlite.Open(target), &gorm.Config{Logger: logger.Discard})
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			if s, err := db.DB(); err == nil {
				_ = s.Close()
			}
		}()
		var n int64
		db.Model(&model.User{}).Count(&n)
		return n
	}
	if code, out, errOut := run("import", "xboard", "--dsn", src, "--skip-banned", "--dry-run"); code != 0 ||
		!strings.Contains(out, skipBannedMode) || !strings.Contains(out, "skipped banned 2") {
		t.Fatalf("--skip-banned --dry-run: exit %d\n%s\n%s", code, out, errOut)
	}
	if code, out, errOut := run("import", "xboard", "--dsn", src); code != 0 || strings.Contains(out, "skipped banned") {
		t.Fatalf("default (no --skip-banned): exit %d\n%s\n%s", code, out, errOut)
	}
	if n := users(); n != 3 {
		t.Errorf("without the flag: %d users, want 3", n)
	}
	if code, out, errOut := run("import", "xboard", "--dsn", src, "--force", "--skip-banned"); code != 0 ||
		!strings.Contains(out, "--force") || !strings.Contains(out, skipBannedMode) {
		t.Fatalf("--force --skip-banned: exit %d\n%s\n%s", code, out, errOut)
	}
	if n := users(); n != 1 {
		t.Errorf("with the flag: %d users, want 1", n)
	}
	if code, _, _ := run("import", "xboard", "--dsn", src, "--skip-banned=maybe"); code != 1 {
		t.Errorf("--skip-banned=maybe: exit %d, want 1", code)
	}
}
