package xboardimport

import (
	"testing"

	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/testutil"
)

// Review 2026-10-22: --force re-creates users with Xboard's ids, so second factors left behind would attach to the
// imported user that receives the same id (a passkey would sign in as them). They are target tables now.
func TestReviewForceClearsSecondFactors(t *testing.T) {
	a := testutil.NewTestApp(t)
	src := newSource(t, FlavorLegacy)
	for _, row := range []any{
		&model.User{ID: 1, Email: "old@example.com", PasswordHash: "!", UUID: "30000000-0000-4000-8000-000000000009", Token: "c0000000000000000000000000000009"},
		&model.UserMFA{UserID: 1, TOTPEnabled: true},
		&model.Passkey{UserID: 1, RPID: "panel.example.com", CredentialHash: "h1", CredentialID: "c1", Credential: "{}", Name: "Laptop"},
	} {
		if err := a.DB.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	if _, err := importInto(t, a, src); err == nil {
		t.Fatal("a target with second factors must count as non-empty")
	}
	if _, err := importInto(t, a, src, func(o *Options) { o.Force = true }); err != nil {
		t.Fatalf("--force: %v", err)
	}
	for _, m := range []any{&model.UserMFA{}, &model.Passkey{}} {
		var n int64
		if err := a.DB.Model(m).Count(&n).Error; err != nil || n != 0 {
			t.Fatalf("%T rows after --force = %d (%v)", m, n, err)
		}
	}
}
