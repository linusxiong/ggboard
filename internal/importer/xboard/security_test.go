package xboardimport

import (
	"encoding/json"
	"log/slog"
	"slices"
	"strings"
	"testing"

	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/testutil"
)

// Security audit 2026-10-22: an empty coupon limit means "every plan / period" in ggboard, so a non-empty Xboard
// list whose every element is dropped must not import as a wildcard. The coupon is imported disabled instead.

type couponWant struct {
	show     bool
	plans    []uint64
	periods  []string
	disabled bool // the "imported disabled" warning and checklist entry
}

func checkCoupons(t *testing.T, flavor Flavor, extra []string, want map[uint64]couponWant) {
	t.Helper()
	a := testutil.NewTestApp(t)
	rep, err := importInto(t, a, newSource(t, flavor, extra...))
	if err != nil {
		t.Fatalf("import %s: %v", flavor, err)
	}
	var summary strings.Builder
	rep.WriteSummary(&summary)
	for id, w := range want {
		var c model.Coupon
		if err := a.DB.First(&c, id).Error; err != nil {
			t.Fatalf("coupon %d: %v", id, err)
		}
		if c.Show != w.show || !slices.Equal(c.LimitPlanIDs, w.plans) || !slices.Equal(c.LimitPeriods, w.periods) {
			t.Errorf("coupon %d: show %v plans %v periods %v, want %v %v %v", id, c.Show, c.LimitPlanIDs, c.LimitPeriods, w.show, w.plans, w.periods)
		}
		warned := hasWarning(rep, scopeID("coupons", id), "imported disabled")
		listed := strings.Contains(summary.String(), c.Code+" (#")
		if warned != w.disabled || listed != w.disabled {
			t.Errorf("coupon %d (%s): disabled warning %v, checklist %v, want %v", id, c.Code, warned, listed, w.disabled)
		}
	}
	if !strings.Contains(summary.String(), "Coupons imported disabled because none of their limited plans or periods could be imported") {
		t.Errorf("the checklist must list the disabled coupons:\n%s", summary.String())
	}
}

func TestSecurityCouponEmptiedLimitsFailClosedLegacy(t *testing.T) {
	extra := []string{
		`UPDATE "v2_coupon" SET "limit_plan_ids" = '["99"]' WHERE "id" = 4;`, // already hidden in Xboard
		`INSERT INTO "v2_coupon" ("id", "code", "name", "type", "value", "show", "limit_use", "limit_use_with_user", "limit_plan_ids", "limit_period", "started_at", "ended_at", "created_at", "updated_at") VALUES
		(5, 'GONEPLANS', 'Deleted plans', 2, 100, 1, NULL, NULL, '["98","99"]', NULL, 1700000000, 1900000000, 1700000000, 1700000000),
		(6, 'BADIDS', 'Invalid ids', 2, 100, 1, NULL, NULL, '["abc","0",null]', NULL, 1700000000, 1900000000, 1700000000, 1700000000),
		(7, 'BADPERIODS', 'Unknown periods', 2, 100, 1, NULL, NULL, '["1"]', '["bogus","weekly",{"x":1}]', 1700000000, 1900000000, 1700000000, 1700000000),
		(8, 'PARTIAL', 'Partly dropped', 2, 100, 1, NULL, NULL, '["2","99"]', '["year_price","bogus"]', 1700000000, 1900000000, 1700000000, 1700000000),
		(9, 'MALFORMED', 'Malformed JSON', 2, 100, 1, NULL, NULL, '[1,', '["monthly"', 1700000000, 1900000000, 1700000000, 1700000000),
		(10, 'OPEN', 'No limits', 2, 100, 1, NULL, NULL, '[]', '', 1700000000, 1900000000, 1700000000, 1700000000);`,
	}
	checkCoupons(t, FlavorLegacy, extra, map[uint64]couponWant{
		1:  {show: true, plans: []uint64{1, 2}, periods: []string{"monthly", "yearly"}}, // fixture: partial drop narrows
		4:  {show: false, plans: []uint64{}, periods: []string{}, disabled: true},
		5:  {show: false, plans: []uint64{}, periods: []string{}, disabled: true},
		6:  {show: false, plans: []uint64{}, periods: []string{}, disabled: true},
		7:  {show: false, plans: []uint64{1}, periods: []string{}, disabled: true},
		8:  {show: true, plans: []uint64{2}, periods: []string{"yearly"}},
		9:  {show: true, plans: []uint64{}, periods: []string{}}, // Xboard's array cast reads malformed JSON as no limit
		10: {show: true, plans: []uint64{}, periods: []string{}},
	})
}

func TestSecurityCouponEmptiedLimitsFailClosedUpstream(t *testing.T) {
	extra := []string{
		`INSERT INTO "v2_coupon" ("id", "code", "name", "type", "value", "show", "limit_use", "limit_use_with_user", "limit_plan_ids", "limit_period", "started_at", "ended_at", "created_at", "updated_at") VALUES
		(2, 'UPGONE', 'Deleted plans', 1, 500, 1, 5, NULL, '[7,8]', '["monthly"]', 1700000000, 1900000000, 1700000000, 1700000000),
		(3, 'UPPERIOD', 'Unknown period', 1, 500, 1, NULL, NULL, '[1]', '["weekly"]', 1700000000, 1900000000, 1700000000, 1700000000),
		(4, 'UPPARTIAL', 'Partly dropped', 1, 500, 1, NULL, NULL, '[1,7]', '["yearly","weekly"]', 1700000000, 1900000000, 1700000000, 1700000000);`,
	}
	checkCoupons(t, FlavorUpstream, extra, map[uint64]couponWant{
		1: {show: true, plans: []uint64{1}, periods: []string{"monthly", "yearly"}},
		2: {show: false, plans: []uint64{}, periods: []string{"monthly"}, disabled: true},
		3: {show: false, plans: []uint64{1}, periods: []string{}, disabled: true},
		4: {show: true, plans: []uint64{1}, periods: []string{"yearly"}},
	})
}

func TestSecurityListCount(t *testing.T) {
	for _, c := range []struct {
		in   any
		want int
	}{
		{nil, 0}, {"", 0}, {"  ", 0}, {"[]", 0}, {"null", 0}, {"{}", 0}, {"[1,", 0},
		{`[""]`, 1}, {"[null]", 1}, {"abc", 1}, {"1,2", 2}, {`{"0":"1","2":"3"}`, 2}, {`"1,2"`, 2},
	} {
		if got := listCount(c.in, splitIDText); got != c.want {
			t.Errorf("listCount(%#v) = %d, want %d", c.in, got, c.want)
		}
	}
}

// Security audit 2026-10-22: the importer wrote any non-empty Xboard server_token, bypassing the 16-character
// minimum admins get. A shorter token is not imported; ggboard keeps the token it generated on first boot.
func TestSecurityShortServerTokenNotImported(t *testing.T) {
	for _, c := range []struct {
		name, token string
		kept        bool
	}{
		{"short", "  short-token  ", false},
		{"fifteen", "abcdefghijklmno", false},
		{"sixteen", "abcdefghijklmnop", true},
	} {
		t.Run(c.name, func(t *testing.T) {
			a := testutil.NewTestApp(t)
			generated := a.Settings.String("server_token")
			if len(generated) < 16 {
				t.Fatalf("the target's generated token %q is too short", generated)
			}
			src := newSource(t, FlavorLegacy, `UPDATE "v2_settings" SET "value" = '`+c.token+`' WHERE "name" = 'server_token';`)
			rep, err := importInto(t, a, src)
			if err != nil {
				t.Fatalf("import: %v", err)
			}
			want := generated
			if c.kept {
				want = strings.TrimSpace(c.token)
			}
			var row model.Setting
			if err := a.DB.Where("key = ?", "server_token").First(&row).Error; err != nil {
				t.Fatal(err)
			}
			var stored string
			if err := json.Unmarshal([]byte(row.Value), &stored); err != nil || stored != want {
				t.Errorf("settings row server_token = %s (%v), want %q", row.Value, err, want)
			}
			if got := a.Settings.String("server_token"); got != want {
				t.Errorf("running server_token = %q, want %q", got, want)
			}
			var summary strings.Builder
			rep.WriteSummary(&summary)
			out := summary.String()
			if rep.ServerTokenKept != c.kept || rep.ServerTokenShort == c.kept {
				t.Errorf("report kept=%v short=%v", rep.ServerTokenKept, rep.ServerTokenShort)
			}
			warned := hasWarning(rep, "settings", "server_token is shorter than 16 characters: not imported")
			listed := strings.Contains(out, "shorter than 16 characters and was not imported")
			if warned == c.kept || listed == c.kept {
				t.Errorf("short-token warning %v, checklist %v:\n%s", warned, listed, out)
			}
			if !c.kept && strings.Contains(out, strings.TrimSpace(c.token)) {
				t.Error("the summary must not print the rejected token")
			}
		})
	}
}

// validateSettings mirrors the settings service's checks, MinLen included (a row that fails them is ignored on
// load), and never prints the value of a MinLen key.
func TestSecurityValidateSettingsMinLen(t *testing.T) {
	im := &importer{rep: &Report{}, log: slog.New(slog.DiscardHandler)}
	out := map[string]any{"server_token": "tiny-secret", "app_name": "Example"}
	im.validateSettings(out)
	if _, ok := out["server_token"]; ok {
		t.Error("a server_token below the minimum length must be dropped")
	}
	if out["app_name"] != "Example" {
		t.Errorf("app_name = %#v", out["app_name"])
	}
	if !hasWarning(im.rep, "settings", "server_token: shorter than 16 characters") || hasWarning(im.rep, "", "tiny-secret") {
		t.Errorf("warnings: %v", im.rep.Warnings)
	}
}
