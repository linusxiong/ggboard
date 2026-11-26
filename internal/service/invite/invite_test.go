package invite_test

import (
	"errors"
	"sync"
	"testing"
	"time"

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
			_, err := svc.Transfer(t.Context(), u, 300)
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
	if _, err := svc.Transfer(t.Context(), u, 0); !errors.Is(err, invitesvc.ErrInsufficientCommission) {
		t.Fatalf("zero amount: %v", err)
	}
}

// A withdrawal request and a transfer of the same commission racing each other: exactly one goes through. Either
// the request is filed (quoting the whole balance) and the transfer is refused, or the commission is moved and the
// request fails the minimum; never a ticket quoting commission that has already been moved to the balance.
func TestWithdrawAndTransferRace(t *testing.T) {
	a := testutil.NewTestApp(t, testutil.WithSettings(map[string]any{"commission_withdraw_limit": 1}))
	u := testutil.CreateUser(t, a, "alice@example.com")
	svc := invitesvc.New(a)
	filed, moved := 0, 0
	for i := range 24 {
		if err := a.DB.Where("user_id = ?", u.ID).Delete(&model.Ticket{}).Error; err != nil {
			t.Fatal(err)
		}
		if err := a.DB.Model(&model.User{}).Where("id = ?", u.ID).Updates(map[string]any{"commission_balance": 10000, "balance": 0}).Error; err != nil {
			t.Fatal(err)
		}
		var (
			wg                    sync.WaitGroup
			withdrawErr, transErr error
		)
		// Stagger the start of one side by 0–1.5 ms, alternating sides, so both orders and true overlaps occur.
		delay := time.Duration(i/2%4) * 500 * time.Microsecond
		start := make(chan struct{})
		wg.Go(func() {
			<-start
			if i%2 == 1 {
				time.Sleep(delay)
			}
			_, withdrawErr = svc.Withdraw(t.Context(), u, "USDT", "T-wallet-example")
		})
		wg.Go(func() {
			<-start
			if i%2 == 0 {
				time.Sleep(delay)
			}
			_, transErr = svc.Transfer(t.Context(), u, 10000)
		})
		close(start)
		wg.Wait()
		var got model.User
		if err := a.DB.First(&got, u.ID).Error; err != nil {
			t.Fatal(err)
		}
		var open int64
		if err := a.DB.Model(&model.Ticket{}).Where("user_id = ? AND status = ? AND kind = ?", u.ID, model.TicketStatusOpen, model.TicketKindWithdraw).
			Count(&open).Error; err != nil {
			t.Fatal(err)
		}
		switch {
		case withdrawErr == nil && errors.Is(transErr, invitesvc.ErrWithdrawalPending):
			filed++
			if open != 1 || got.CommissionBalance != 10000 || got.Balance != 0 {
				t.Fatalf("round %d: request filed but commission=%d balance=%d open=%d", i, got.CommissionBalance, got.Balance, open)
			}
		case transErr == nil && errors.Is(withdrawErr, invitesvc.ErrWithdrawLimit):
			moved++
			if open != 0 || got.CommissionBalance != 0 || got.Balance != 10000 {
				t.Fatalf("round %d: transferred but commission=%d balance=%d open=%d", i, got.CommissionBalance, got.Balance, open)
			}
		default:
			t.Fatalf("round %d: withdraw=%v transfer=%v commission=%d balance=%d open=%d", i, withdrawErr, transErr, got.CommissionBalance, got.Balance, open)
		}
	}
	t.Logf("request filed first: %d rounds, commission moved first: %d rounds", filed, moved)
}

// Concurrent code generation respects invite_gen_limit.
func TestGenerateCodeLimitUnderConcurrency(t *testing.T) {
	a := testutil.NewTestApp(t, testutil.WithSettings(map[string]any{"invite_gen_limit": 3}))
	u := testutil.CreateUser(t, a, "alice@example.com")
	svc := invitesvc.New(a)
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() { _, _ = svc.GenerateCode(t.Context(), u) })
	}
	wg.Wait()
	var n int64
	if err := a.DB.Model(&model.InviteCode{}).Where("user_id = ?", u.ID).Count(&n).Error; err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Fatalf("codes = %d, want 3", n)
	}
	if _, err := svc.GenerateCode(t.Context(), &model.User{ID: 424242, UUID: "missing"}); !errors.Is(err, invitesvc.ErrUserNotFound) {
		t.Fatalf("unknown user: %v", err)
	}
}
