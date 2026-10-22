package admin_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/subscription/guard"
	"github.com/linusxiong/ggboard/internal/testutil"
)

func TestSubguardAdminEndpoints(t *testing.T) {
	e := newOpsEnv(t)
	ctx := t.Context()
	agePath, resetPath, logPath := usersPath(e.user.ID, "/subscription/age"), usersPath(e.user.ID, "/subscription/age/reset"),
		usersPath(e.user.ID, "/subscription/access-log")
	for _, rt := range []struct{ method, path string }{{http.MethodGet, agePath}, {http.MethodPost, resetPath}, {http.MethodGet, logPath}} {
		e.expect(t, rt.method, rt.path, nil, e.staffTok, http.StatusForbidden)
		e.expect(t, rt.method, rt.path, nil, e.userTok, http.StatusForbidden)
	}
	e.expect(t, http.MethodGet, usersPath(999, "/subscription/age"), nil, e.adminTok, http.StatusNotFound)
	e.expect(t, http.MethodPost, resetPath, nil, e.adminTok, http.StatusNotFound) // no key yet

	k, _, err := guard.GenerateKey(ctx, e.a, e.user.ID, false, testutil.Ptr(true))
	if err != nil {
		t.Fatal(err)
	}
	var v struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(e.expect(t, http.MethodGet, agePath, nil, e.adminTok, http.StatusOK), &v); err != nil {
		t.Fatal(err)
	}
	if v.Data["public_key"] != k.PublicKey || v.Data["enabled"] != true || v.Data["private_key"] != nil {
		t.Fatalf("admin view = %v", v.Data)
	}
	if err := json.Unmarshal(e.expect(t, http.MethodPost, resetPath, nil, e.adminTok, http.StatusOK), &v); err != nil {
		t.Fatal(err)
	}
	if v.Data["public_key"] == k.PublicKey || v.Data["enabled"] != true || v.Data["private_key"] != nil {
		t.Fatalf("reset view = %v", v.Data)
	}

	guard.Record(e.a, guard.Access{UserID: e.user.ID, Token: e.user.Token, IP: "2001:db8:1:2::5", UserAgent: "mihomo/1.19.27", Format: "clash-meta"})
	var log struct {
		Data []guard.AccessEntry `json:"data"`
	}
	if err := json.Unmarshal(e.expect(t, http.MethodGet, logPath, nil, e.adminTok, http.StatusOK), &log); err != nil {
		t.Fatal(err)
	}
	if len(log.Data) != 1 || log.Data[0].IP != "2001:db8:1:2::5" || log.Data[0].IPBucket != "2001:db8:1:2::/64" ||
		log.Data[0].TokenTag != guard.TokenTag(e.user.Token) {
		t.Fatalf("admin log = %+v", log.Data)
	}

	// Deleting the user removes the key and the log.
	e.expect(t, http.MethodDelete, usersPath(e.user.ID, ""), nil, e.adminTok, http.StatusOK)
	for _, m := range []any{&model.SubscriptionKey{}, &model.SubscriptionAccess{}} {
		var n int64
		e.a.DB.Model(m).Count(&n)
		if n != 0 {
			t.Fatalf("%T rows left: %d", m, n)
		}
	}
}
