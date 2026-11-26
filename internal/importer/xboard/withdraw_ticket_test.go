package xboardimport

import (
	"testing"

	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/testutil"
)

// Xboard's withdrawal requests (its zh-CN and en subjects) are imported as tickets of kind withdraw, so an open one
// keeps blocking commission transfers in ggboard; every other ticket is a support ticket (§4.10).
func TestImportMarksWithdrawalTickets(t *testing.T) {
	for _, flavor := range []Flavor{FlavorLegacy, FlavorUpstream} {
		t.Run(string(flavor), func(t *testing.T) {
			sql := `INSERT INTO "v2_ticket" ("id", "user_id", "subject", "level", "status", "reply_status", "created_at", "updated_at") VALUES
	(20, 2, '[提现申请] 本工单由系统发出', 2, 0, 0, 1700000000, 1700000000),
	(21, 2, '[Commission Withdrawal Request] This ticket is opened by the system', 2, 1, 0, 1700000000, 1700000000),
	(22, 2, '提现', 2, 0, 0, 1700000000, 1700000000);`
			a := testutil.NewTestApp(t)
			if _, err := importInto(t, a, newSource(t, flavor, sql)); err != nil {
				t.Fatal(err)
			}
			var tickets []model.Ticket
			if err := a.DB.Order("id").Find(&tickets).Error; err != nil {
				t.Fatal(err)
			}
			got := map[uint64]model.TicketKind{}
			for _, tk := range tickets {
				got[tk.ID] = tk.Kind
			}
			want := map[uint64]model.TicketKind{20: model.TicketKindWithdraw, 21: model.TicketKindWithdraw, 22: model.TicketKindSupport}
			for id, kind := range want {
				if k, ok := got[id]; !ok || k != kind {
					t.Errorf("ticket %d kind = %q (imported %v), want %q", id, k, ok, kind)
				}
			}
			for id, k := range got {
				if _, listed := want[id]; !listed && k != model.TicketKindSupport {
					t.Errorf("fixture ticket %d kind = %q, want support", id, k)
				}
			}
		})
	}
}
