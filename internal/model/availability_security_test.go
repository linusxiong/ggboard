package model

import (
	"math"
	"testing"
	"time"
)

func TestOverflowedUsageNeverRestoresAccess(t *testing.T) {
	for _, u := range []User{
		{TransferEnable: 100, U: math.MaxInt64, D: 1},
		{TransferEnable: math.MaxInt64, U: math.MaxInt64 - 5, D: 10},
		{TransferEnable: 100, U: -1},
	} {
		if u.Used() != math.MaxInt64 || u.Remaining() != 0 || u.IsAvailable(time.Now()) {
			t.Fatalf("overflow or corrupt usage granted access: %+v", u)
		}
	}
}
