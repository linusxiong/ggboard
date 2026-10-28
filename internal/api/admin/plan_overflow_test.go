package admin_test

import (
	"net/http"
	"testing"

	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/testutil"
)

func TestAdminPlanTransferLimit(t *testing.T) {
	e := newCommerceEnv(t)
	groupID := e.group(t)
	base := map[string]any{"name": "Safe", "group_id": groupID, "transfer_enable": model.MaxPlanTransferGB}
	created := testutil.Decode[commercePlan](t, e.do(t, http.MethodPost, "/plans", base))
	if created.ID == 0 {
		t.Fatal("maximum representable traffic was rejected")
	}
	base["transfer_enable"] = model.MaxPlanTransferGB + 1
	if got := testutil.DecodeError(t, e.do(t, http.MethodPost, "/plans", base)); got.Status != 400 || len(got.Errors["transfer_enable"]) == 0 {
		t.Fatalf("overflow create: %+v", got)
	}
	if got := testutil.DecodeError(t, e.do(t, http.MethodPatch, commercePath("/plans", created.ID),
		map[string]any{"transfer_enable": model.MaxPlanTransferGB + 1, "force_update": true})); got.Status != 400 || len(got.Errors["transfer_enable"]) == 0 {
		t.Fatalf("overflow patch: %+v", got)
	}
	var stored model.Plan
	if err := e.a.DB.First(&stored, created.ID).Error; err != nil || stored.TransferEnable != model.MaxPlanTransferGB {
		t.Fatalf("rejected patch changed plan: %+v, %v", stored, err)
	}
}
