package model

import "testing"

func TestPlanTransferBytesRejectsOverflow(t *testing.T) {
	for _, tc := range []struct{ gb, want int64 }{
		{0, 0}, {1, BytesPerGB}, {MaxPlanTransferGB, MaxPlanTransferGB * BytesPerGB},
		{MaxPlanTransferGB + 1, 0}, {-1, 0},
	} {
		if got := (&Plan{TransferEnable: tc.gb}).TransferBytes(); got != tc.want {
			t.Errorf("%d GB: got %d bytes, want %d", tc.gb, got, tc.want)
		}
	}
}
