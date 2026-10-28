package user_test

import (
	"math"
	"testing"

	"github.com/linusxiong/ggboard/internal/model"
	usersvc "github.com/linusxiong/ggboard/internal/service/user"
	"github.com/linusxiong/ggboard/internal/testutil"
)

func TestNodeUserQueryRejectsOverflowedTraffic(t *testing.T) {
	a := testutil.NewTestApp(t)
	group := uint64(1)
	u := testutil.CreateUser(t, a, "overflow@example.com", usersvc.CreateOptions{GroupID: &group, TransferEnable: 100})
	negative := testutil.CreateUser(t, a, "negative@example.com", usersvc.CreateOptions{GroupID: &group, TransferEnable: 100})
	if err := a.DB.Model(&model.User{}).Where("id = ?", u.ID).Updates(map[string]any{"u": math.MaxInt64, "d": 1}).Error; err != nil {
		t.Fatal(err)
	}
	if err := a.DB.Model(&model.User{}).Where("id = ?", negative.ID).Update("u", -1).Error; err != nil {
		t.Fatal(err)
	}
	rows, err := usersvc.New(a).AvailableUsersForGroups(t.Context(), []uint64{group})
	if err != nil || len(rows) != 0 {
		t.Fatalf("overflowed account sent to node: %+v, %v", rows, err)
	}
}
