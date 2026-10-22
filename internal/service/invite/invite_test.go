package invite_test

import (
	"errors"
	"sync"
	"testing"

	"github.com/linusxiong/ggboard/internal/model"
	invitesvc "github.com/linusxiong/ggboard/internal/service/invite"
	"github.com/linusxiong/ggboard/internal/testutil"
)

// Concurrent transfers can never spend more commission than the user has.
func TestTransferIsAtomic(t *testing.T) {
	a := testutil.NewTestApp(t)
	u := testutil.CreateUser(t, a, "alice@example.com")
	if err := a.DB.Model(&model.User{}).Where("id = ?", u.ID).Update("commission_balance", 1000).Error; err != nil {
		t.Fatal(err)
	}
	svc := invitesvc.New(a)
	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		ok   int
		errs []error
	)
	for range 10 {
		wg.Go(func() {
			_, err := svc.Transfer(t.Context(), u.ID, 300)
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				ok++
			} else if !errors.Is(err, invitesvc.ErrInsufficientCommission) {
				errs = append(errs, err)
			}
		})
	}
	wg.Wait()
	var got model.User
	if err := a.DB.First(&got, u.ID).Error; err != nil {
		t.Fatal(err)
	}
	if ok != 3 || len(errs) != 0 || got.CommissionBalance != 100 || got.Balance != 900 {
		t.Fatalf("ok=%d errs=%v commission=%d balance=%d", ok, errs, got.CommissionBalance, got.Balance)
	}
	if _, err := svc.Transfer(t.Context(), u.ID, 0); !errors.Is(err, invitesvc.ErrInsufficientCommission) {
		t.Fatalf("zero amount: %v", err)
	}
}

// Concurrent code generation respects invite_gen_limit.
func TestGenerateCodeLimitUnderConcurrency(t *testing.T) {
	a := testutil.NewTestApp(t, testutil.WithSettings(map[string]any{"invite_gen_limit": 3}))
	u := testutil.CreateUser(t, a, "alice@example.com")
	svc := invitesvc.New(a)
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() { _, _ = svc.GenerateCode(t.Context(), u.ID) })
	}
	wg.Wait()
	var n int64
	if err := a.DB.Model(&model.InviteCode{}).Where("user_id = ?", u.ID).Count(&n).Error; err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Fatalf("codes = %d, want 3", n)
	}
	if _, err := svc.GenerateCode(t.Context(), 424242); !errors.Is(err, invitesvc.ErrUserNotFound) {
		t.Fatalf("unknown user: %v", err)
	}
}
