package xboardimport

import (
	"testing"

	"github.com/linusxiong/ggboard/internal/model"
)

// Security audit 2026-10-22 §3: Xboard does not record whether an address was verified, so imported accounts are
// trusted (email_verified_at = created_at): their first password reset must not be treated as an ownership change.
func TestImportedUsersAreEmailVerified(t *testing.T) {
	for _, flavor := range []Flavor{FlavorLegacy, FlavorUpstream} {
		t.Run(string(flavor), func(t *testing.T) {
			a, _ := mustImport(t, flavor)
			var users []model.User
			if err := a.DB.Order("id").Find(&users).Error; err != nil {
				t.Fatal(err)
			}
			if len(users) == 0 {
				t.Fatal("no users imported")
			}
			for _, u := range users {
				if u.EmailVerifiedAt == nil || !u.EmailVerifiedAt.Equal(u.CreatedAt) {
					t.Fatalf("user %d email_verified_at = %v, created_at = %v", u.ID, u.EmailVerifiedAt, u.CreatedAt)
				}
			}
		})
	}
}
