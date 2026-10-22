package user_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/linusxiong/ggboard/internal/api/user"
	"github.com/linusxiong/ggboard/internal/model"
	usersvc "github.com/linusxiong/ggboard/internal/service/user"
	"github.com/linusxiong/ggboard/internal/subscription/fixture"
	"github.com/linusxiong/ggboard/internal/testutil"
)

// Review 2026-10-22: rule profiles were unbounded in include tokens and regex cost (compiled on every render),
// accepted plain-http mirrors and group names with line separators.
func TestReviewRuleProfileBounds(t *testing.T) {
	a := testutil.NewTestApp(t)
	if _, err := fixture.SeedNodes(a.DB); err != nil {
		t.Fatal(err)
	}
	gid := uint64(fixture.GroupID)
	exp := time.Now().Add(24 * time.Hour)
	u := testutil.CreateUser(t, a, "bounds@example.com", usersvc.CreateOptions{GroupID: &gid, TransferEnable: 10 * fixture.GiB, ExpiredAt: &exp})
	tok := testutil.Login(t, a, u)
	r := testutil.NewRouterWith(t, a, user.Register)

	many := make([]string, 40)
	for i := range many {
		many[i] = "n" + strings.Repeat("x", i%5) + string(rune('a'+i%26))
	}
	for name, tc := range map[string]struct {
		body  map[string]any
		field string
	}{
		"too many regexes": {map[string]any{"template": "minimal", "groups": []any{
			map[string]any{"name": "Proxy", "type": "select", "include": strings.Join(many, ",")}}}, "groups[0].include"},
		"costly regex": {map[string]any{"template": "minimal", "groups": []any{
			map[string]any{"name": "Proxy", "type": "select", "include": `[\pL\pN]{1000}`}}}, "groups[0].include"},
		"line separator in name": {map[string]any{"template": "minimal", "groups": []any{
			map[string]any{"name": "Proxy", "type": "select", "include": "*"},
			map[string]any{"name": "A B", "type": "select", "include": "*"}}}, "groups[1].name"},
		"http mirror": {map[string]any{"template": "minimal", "mirror": "http://mirror.example.com/"}, "mirror"},
	} {
		rec := testutil.Do(t, r, http.MethodPut, "/api/v1/user/rule-profile", tc.body, tok)
		e := testutil.DecodeError(t, rec)
		if e.Status != http.StatusBadRequest || len(e.Errors[tc.field]) == 0 {
			t.Errorf("%s: %d %+v", name, e.Status, e.Errors)
		}
	}
	// An https mirror is still accepted.
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodPut, "/api/v1/user/rule-profile",
		map[string]any{"template": "minimal", "mirror": "https://mirror.example.com/"}, tok), http.StatusOK)

	// Query lists are bounded too.
	filter := strings.Repeat("k,", 65)
	rec := testutil.Do(t, r, http.MethodGet, "/api/v1/user/subscription/preview?client=clash-meta&filter="+filter, nil, tok)
	if e := testutil.DecodeError(t, rec); e.Status != http.StatusBadRequest || len(e.Errors["filter"]) == 0 {
		t.Fatalf("filter list: %d %+v", e.Status, e.Errors)
	}
}

// Review 2026-10-22: the knowledge keyword search matched inside <!--access start-->…<!--access end--> blocks,
// so users without a plan could confirm guessed gated content (private group links) by searching for it.
func TestReviewKnowledgeSearchSkipsGatedBlocks(t *testing.T) {
	a, r := ticketTestEnv(t, nil)
	contentSeed(t, a, &model.Knowledge{Language: "zh-CN", Category: "群组", Title: "加入群组",
		Body: "公开说明 <!--access start-->t.me/SECRETX<!--access end-->", Show: true})
	free := testutil.Login(t, a, testutil.CreateUser(t, a, "free@example.com"))
	gid := uint64(1)
	plan := &model.Plan{GroupID: gid, Name: "Pro", TransferEnable: 10, Show: true, Sell: true, Renew: true}
	if err := a.DB.Create(plan).Error; err != nil {
		t.Fatal(err)
	}
	exp := time.Now().Add(24 * time.Hour)
	paid := testutil.Login(t, a, testutil.CreateUser(t, a, "paid@example.com", usersvc.CreateOptions{PlanID: &plan.ID, GroupID: &gid, TransferEnable: 10 << 30, ExpiredAt: &exp}))
	count := func(tok, kw string) int {
		idx := testutil.Decode[map[string][]map[string]any](t, testutil.Do(t, r, http.MethodGet, "/api/v1/user/knowledge?keyword="+kw, nil, tok))
		return len(idx["群组"])
	}
	if n := count(free, "secretx"); n != 0 {
		t.Fatalf("gated text found by a user without a plan (%d)", n)
	}
	if n := count(free, "%E5%85%AC%E5%BC%80"); n != 1 { // 公开: visible text still matches
		t.Fatalf("visible text not found (%d)", n)
	}
	if n := count(paid, "secretx"); n != 1 {
		t.Fatalf("plan holder cannot search gated text (%d)", n)
	}
}
