package traffic_test

import (
	"math"
	"testing"
	"time"

	"github.com/linusxiong/ggboard/internal/app/traffic"
	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/testutil"
)

func TestFlushSaturatesStoredCounters(t *testing.T) {
	a := testutil.NewTestApp(t)
	u := testutil.CreateUser(t, a, "traffic-overflow@example.com")
	if err := a.DB.Model(&model.User{}).Where("id = ?", u.ID).Update("transfer_enable", int64(math.MaxInt64)).Error; err != nil {
		t.Fatal(err)
	}
	acc := traffic.New()
	acc.Add(1, u.ID, 1, math.MaxInt64-5, math.MaxInt64-5)
	if err := acc.Flush(t.Context(), a.DB); err != nil {
		t.Fatal(err)
	}
	acc.Add(1, u.ID, 1, 10, 10)
	if err := acc.Flush(t.Context(), a.DB); err != nil {
		t.Fatal(err)
	}
	var got model.User
	if err := a.DB.First(&got, u.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.U != math.MaxInt64 || got.D != math.MaxInt64 || got.IsAvailable(time.Now()) {
		t.Fatalf("stored usage wrapped or restored access: u=%d d=%d", got.U, got.D)
	}
	var su model.StatUser
	if err := a.DB.Where("user_id = ? AND node_id = ?", u.ID, 1).First(&su).Error; err != nil || su.U != math.MaxInt64 || su.D != math.MaxInt64 {
		t.Fatalf("user statistics wrapped: %+v, %v", su, err)
	}
	var sn model.StatNode
	if err := a.DB.Where("node_id = ?", 1).First(&sn).Error; err != nil || sn.U != math.MaxInt64 || sn.D != math.MaxInt64 {
		t.Fatalf("node statistics wrapped: %+v, %v", sn, err)
	}
}
