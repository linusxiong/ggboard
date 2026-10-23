package xboardimport

import (
	"fmt"
	"sort"
	"strings"

	"github.com/linusxiong/ggboard/internal/model"
)

// invite_codes ← v2_invite_code (§4.6). Codes are not unique in Xboard: the lowest id wins (compared
// case-insensitively, because a MySQL target's unique index is case-insensitive).
func (im *importer) importInviteCodes() error {
	stat := im.rep.Table("invite_codes")
	seen := map[string]uint64{}
	b := newBatcher[model.InviteCode](im.tx)
	err := im.src.each(im.ctx, "v2_invite_code", "", nil, func(r row) error {
		stat.Source++
		id := r.id()
		scope := scopeID("invite_codes", id)
		uid := uint64(max(r.i64("user_id"), 0))
		if !im.users[uid] {
			im.warn(scope, "user %d not found — skipped", uid)
			stat.Skipped++
			return nil
		}
		code := strings.TrimSpace(r.str("code"))
		key := strings.ToLower(code)
		if prev, dup := seen[key]; dup || code == "" || len(code) > 32 {
			if dup {
				im.warn(scope, "duplicate code %q (kept #%d) — skipped", code, prev)
			} else {
				im.warn(scope, "invalid code %q — skipped", code)
			}
			stat.Skipped++
			return nil
		}
		seen[key] = id
		return b.add(model.InviteCode{
			ID: id, UserID: uid, Code: code, Status: r.bool("status"), PV: int(r.i64("pv")),
			CreatedAt: im.ts(scope, r["created_at"]), UpdatedAt: im.ts(scope, r["updated_at"]),
		})
	})
	if err != nil {
		return err
	}
	if err := b.flush(); err != nil {
		return err
	}
	stat.Imported = b.n
	return nil
}

// commission_logs ← v2_commission_log (§4.7): history, imported even when the inviter is gone.
func (im *importer) importCommissionLogs() error {
	stat := im.rep.Table("commission_logs")
	b := newBatcher[model.CommissionLog](im.tx)
	err := im.src.each(im.ctx, "v2_commission_log", "", nil, func(r row) error {
		stat.Source++
		id := r.id()
		scope := scopeID("commission_logs", id)
		inviter := uint64(max(r.i64("invite_user_id"), 0))
		if !im.users[inviter] {
			im.warn(scope, "inviter %d not found — imported anyway (history)", inviter)
		}
		tradeNo := strings.TrimSpace(r.str("trade_no"))
		if len(tradeNo) > 32 {
			im.warn(scope, "trade_no %q truncated to 32 characters", tradeNo)
			tradeNo = tradeNo[:32]
		}
		return b.add(model.CommissionLog{
			ID: id, InviteUserID: inviter, UserID: uint64(max(r.i64("user_id"), 0)), TradeNo: tradeNo,
			OrderAmount: r.i64("order_amount"), GetAmount: r.i64("get_amount"), CreatedAt: im.ts(scope, r["created_at"]),
		})
	})
	if err != nil {
		return err
	}
	if err := b.flush(); err != nil {
		return err
	}
	stat.Imported = b.n
	return nil
}

// coupons ← v2_coupon (§4.8).
func (im *importer) importCoupons() error {
	stat := im.rep.Table("coupons")
	seen := map[string]uint64{}
	perUser := false
	b := newBatcher[model.Coupon](im.tx)
	err := im.src.each(im.ctx, "v2_coupon", "", nil, func(r row) error {
		stat.Source++
		id := r.id()
		scope := scopeID("coupons", id)
		code := strings.TrimSpace(r.str("code"))
		key := strings.ToLower(code)
		if prev, dup := seen[key]; dup {
			im.warn(scope, "duplicate code %q (kept #%d) — skipped", code, prev)
			stat.Skipped++
			return nil
		}
		if code == "" || len(code) > 64 {
			im.warn(scope, "code %q is empty or longer than 64 characters — skipped", code)
			stat.Skipped++
			return nil
		}
		typ := model.CouponType(r.i64("type"))
		if typ != model.CouponTypeAmount && typ != model.CouponTypePercent {
			im.warn(scope, "type %d is not 1 (amount) or 2 (percent) — skipped", typ)
			stat.Skipped++
			return nil
		}
		seen[key] = id
		c := model.Coupon{
			ID: id, Code: code, Name: im.fit(scope, "name", r.str("name"), 191), Type: typ, Value: r.i64("value"), Show: r.bool("show"),
			StartedAt: im.ts(scope, r["started_at"]), EndedAt: im.ts(scope, r["ended_at"]),
			CreatedAt: im.ts(scope, r["created_at"]), UpdatedAt: im.ts(scope, r["updated_at"]),
		}
		if n, ok := r.int("limit_use"); ok {
			v := int(n)
			c.LimitUse = &v
		}
		if n, ok := r.int("limit_use_with_user"); ok {
			v := int(n)
			c.LimitUseWithUser = &v
			perUser = true
		}
		c.LimitPlanIDs = im.idList(scope, "limit_plan_ids", r["limit_plan_ids"], func(pid uint64) (bool, string) {
			if im.plans[pid] != nil {
				return true, ""
			}
			return false, "not an imported plan"
		})
		periods, bad, err := parseStringList(r["limit_period"])
		if err != nil {
			im.warn(scope, "limit_period: %v → no period limit", err)
		}
		for _, x := range bad {
			im.warn(scope, "limit_period: dropped %s", x)
		}
		c.LimitPeriods = model.JSONSlice[string]{}
		for _, p := range periods {
			if mapped, ok := legacyPeriodNames[p]; ok {
				p = mapped
			}
			if !model.IsValidPeriod(p) {
				im.warn(scope, "limit_period: unknown period %q dropped", p)
				continue
			}
			c.LimitPeriods = append(c.LimitPeriods, p)
		}
		// An empty list means "every plan / period" in ggboard (validateCoupon), but Xboard rejects a plan or period
		// missing from a non-empty list. A restriction whose every element was dropped must not become a wildcard:
		// the coupon is imported disabled instead. Malformed JSON (count 0) stays unrestricted, as in Xboard.
		var emptied []string
		if listCount(r["limit_plan_ids"], splitIDText) > 0 && len(c.LimitPlanIDs) == 0 {
			emptied = append(emptied, "limit_plan_ids")
		}
		if listCount(r["limit_period"], splitStringText) > 0 && len(c.LimitPeriods) == 0 {
			emptied = append(emptied, "limit_period")
		}
		if len(emptied) > 0 {
			c.Show = false
			im.warn(scope, "coupon %q: no element of %s could be imported, and an empty list would allow everything — "+
				"imported disabled (show = false); set its limits again in Admin → Coupons, then enable it",
				code, strings.Join(emptied, " and "))
			im.rep.DisabledCoupons = append(im.rep.DisabledCoupons, fmt.Sprintf("%s (#%d)", code, id))
		}
		return b.add(c)
	})
	if err != nil {
		return err
	}
	if err := b.flush(); err != nil {
		return err
	}
	stat.Imported = b.n
	if perUser {
		im.warn("coupons", "limit_use_with_user is kept, but per-user usage history is lost with the orders: every user starts at 0 uses")
	}
	return nil
}

type lastMessage struct {
	id     uint64
	userID uint64
}

// tickets ← v2_ticket, ticket_messages ← v2_ticket_message (§4.10).
func (im *importer) importTickets() error {
	stat := im.rep.Table("tickets")
	mstat := im.rep.Table("ticket_messages")
	// newest message per ticket (one complete query before the ticket pass)
	last := map[uint64]lastMessage{}
	err := im.src.query(im.ctx, "SELECT id, ticket_id, user_id FROM v2_ticket_message", nil, func(r row) error {
		tid := uint64(max(r.i64("ticket_id"), 0))
		if id := r.id(); id >= last[tid].id {
			last[tid] = lastMessage{id: id, userID: uint64(max(r.i64("user_id"), 0))}
		}
		return nil
	})
	if err != nil {
		return err
	}
	fixedSemantics := im.src.hasCol("v2_ticket", "last_reply_user_id")
	b := newBatcher[model.Ticket](im.tx)
	err = im.src.each(im.ctx, "v2_ticket", "", nil, func(r row) error {
		stat.Source++
		id := r.id()
		scope := scopeID("tickets", id)
		uid := uint64(max(r.i64("user_id"), 0))
		if !im.users[uid] {
			im.warn(scope, "user %d not found — ticket and its messages skipped", uid)
			stat.Skipped++
			return nil
		}
		t := model.Ticket{
			ID: id, UserID: uid, Subject: im.fit(scope, "subject", r.str("subject"), 255),
			CreatedAt: im.ts(scope, r["created_at"]), UpdatedAt: im.ts(scope, r["updated_at"]),
		}
		switch lv := r.i64("level"); lv {
		case 0, 1, 2:
			t.Level = model.TicketLevel(lv)
		default:
			im.warn(scope, "level %d unknown → 0", lv)
		}
		if r.i64("status") != 0 {
			t.Status = model.TicketStatusClosed
		}
		if m, ok := last[id]; ok {
			lu := m.userID
			t.LastReplyUserID = &lu
			if m.userID == uid {
				t.ReplyStatus = model.TicketAwaitingStaff
			} else {
				t.ReplyStatus = model.TicketReplied
			}
		} else {
			v := r.i64("reply_status")
			if fixedSemantics {
				if lu, ok := r.int("last_reply_user_id"); ok && lu > 0 {
					x := uint64(lu)
					t.LastReplyUserID = &x
				}
			} else {
				v = 1 - v // legacy TicketService writes 0 after a staff reply
			}
			if v == 1 {
				t.ReplyStatus = model.TicketReplied
			}
		}
		im.tickets[id] = true
		return b.add(t)
	})
	if err != nil {
		return err
	}
	if err := b.flush(); err != nil {
		return err
	}
	stat.Imported = b.n

	mb := newBatcher[model.TicketMessage](im.tx)
	err = im.src.each(im.ctx, "v2_ticket_message", "", nil, func(r row) error {
		mstat.Source++
		id := r.id()
		tid := uint64(max(r.i64("ticket_id"), 0))
		if !im.tickets[tid] {
			mstat.Skipped++
			return nil
		}
		return mb.add(model.TicketMessage{
			ID: id, TicketID: tid, UserID: uint64(max(r.i64("user_id"), 0)), Message: r.str("message"),
			CreatedAt: im.ts(scopeID("ticket_messages", id), r["created_at"]),
		})
	})
	if err != nil {
		return err
	}
	if err := mb.flush(); err != nil {
		return err
	}
	mstat.Imported = mb.n
	if mstat.Skipped > 0 {
		im.warn("ticket_messages", "%d messages of skipped tickets were skipped", mstat.Skipped)
	}
	return nil
}

// notices ← v2_notice (§4.11). L has no sort column and lists notices by created_at DESC: sort is that rank.
func (im *importer) importNotices() error {
	stat := im.rep.Table("notices")
	var notices []model.Notice
	err := im.src.each(im.ctx, "v2_notice", "", nil, func(r row) error {
		stat.Source++
		id := r.id()
		scope := scopeID("notices", id)
		tags, bad, err := parseStringList(r["tags"])
		if err != nil {
			im.warn(scope, "tags: %v → []", err)
		}
		for _, x := range bad {
			im.warn(scope, "tags: dropped non-string element %s", x)
		}
		notices = append(notices, model.Notice{
			ID: id, Title: im.fit(scope, "title", r.str("title"), 255), Content: r.str("content"),
			ImgURL: im.fitURL(scope, "img_url", r.str("img_url"), 255), Tags: tags,
			Show: r.bool("show"), Sort: int(r.i64("sort")),
			CreatedAt: im.ts(scope, r["created_at"]), UpdatedAt: im.ts(scope, r["updated_at"]),
		})
		return nil
	})
	if err != nil {
		return err
	}
	if !im.src.hasCol("v2_notice", "sort") {
		order := make([]int, len(notices))
		for i := range order {
			order[i] = i
		}
		sort.SliceStable(order, func(a, b int) bool {
			x, y := notices[order[a]], notices[order[b]]
			if !x.CreatedAt.Equal(y.CreatedAt) {
				return x.CreatedAt.After(y.CreatedAt)
			}
			return x.ID > y.ID
		})
		for rank, i := range order {
			notices[i].Sort = rank
		}
	}
	b := newBatcher[model.Notice](im.tx)
	for _, n := range notices {
		if err := b.add(n); err != nil {
			return err
		}
	}
	if err := b.flush(); err != nil {
		return err
	}
	stat.Imported = b.n
	return nil
}

// knowledge ← v2_knowledge (§4.12).
func (im *importer) importKnowledge() error {
	stat := im.rep.Table("knowledge")
	b := newBatcher[model.Knowledge](im.tx)
	access := 0
	err := im.src.each(im.ctx, "v2_knowledge", "", nil, func(r row) error {
		stat.Source++
		id := r.id()
		scope := scopeID("knowledge", id)
		body := r.str("body")
		if strings.Contains(body, "<!--access start-->") {
			access++
			im.warn(scope, "article %q uses <!--access start--> blocks; ggboard hides them for users without an active plan", r.str("title"))
		}
		lang := strings.TrimSpace(r.str("language"))
		if len(lang) > 5 {
			lang = lang[:5]
		}
		return b.add(model.Knowledge{
			ID: id, Language: lang, Category: im.fit(scope, "category", r.str("category"), 191),
			Title: im.fit(scope, "title", r.str("title"), 255), Body: body,
			Sort: int(r.i64("sort")), Show: r.bool("show"),
			CreatedAt: im.ts(scope, r["created_at"]), UpdatedAt: im.ts(scope, r["updated_at"]),
		})
	})
	if err != nil {
		return err
	}
	if err := b.flush(); err != nil {
		return err
	}
	stat.Imported = b.n
	im.rep.AccessArticles = access
	if access > 0 {
		stat.Note = fmt.Sprintf("access blocks %d", access)
	}
	return nil
}
