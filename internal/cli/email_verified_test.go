package cli

import (
	"testing"

	"github.com/spf13/cobra"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/auth"
	"github.com/linusxiong/ggboard/internal/model"
)

// Security audit 2026-10-22 §3: an operator-created administrator counts as email-verified, so its first password
// reset is not treated as an ownership change (usersvc.ReclaimAccount).
func TestAdminCreateMarksEmailVerified(t *testing.T) {
	auth.SetParams(auth.Params{Memory: 1024, Time: 1, Threads: 1, KeyLen: 32, SaltLen: 16})
	t.Cleanup(func() { auth.SetParams(auth.DefaultParams) })
	t.Chdir(t.TempDir())
	t.Setenv("GGBOARD_DATA_DIR", t.TempDir())
	t.Setenv("GGBOARD_LOG_LEVEL", "error")

	if out, code := run(t, "admin", "create", "--email", "root@example.com", "--password", "password123"); code != 0 {
		t.Fatalf("create = %d %q", code, out)
	}
	var verified *bool
	Register(func(a *app.App) *cobra.Command {
		return &cobra.Command{Use: "zzz-check-verified", RunE: func(cmd *cobra.Command, _ []string) error {
			var u model.User
			if err := a.DB.Where("email = ?", "root@example.com").First(&u).Error; err != nil {
				return err
			}
			ok := u.EmailVerifiedAt != nil
			verified = &ok
			return nil
		}}
	})
	t.Cleanup(func() { mu.Lock(); factories = factories[:len(factories)-1]; mu.Unlock() })
	if out, code := run(t, "zzz-check-verified"); code != 0 || verified == nil || !*verified {
		t.Fatalf("check = %d %q verified=%v", code, out, verified)
	}
}
