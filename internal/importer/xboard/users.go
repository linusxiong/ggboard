package xboardimport

import (
	"encoding/hex"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/linusxiong/ggboard/internal/model"
	usersvc "github.com/linusxiong/ggboard/internal/service/user"
)

// users ← v2_user (§4.5).

// Password hash kinds counted in the summary.
const (
	hashBcrypt   = "bcrypt"
	hashLegacy   = "legacy"
	hashUnusable = "unusable"
)

// unusableHash never verifies (auth.VerifyPassword rejects hashes starting with "!").
const unusableHash = "!xb-unusable"

// mapPassword converts Xboard's password / password_algo / password_salt (§4.5.1).
func mapPassword(password, algo, salt string) (string, string) {
	password = strings.TrimSpace(password)
	switch a := strings.ToLower(strings.TrimSpace(algo)); a {
	case "md5", "sha256", "md5salt", "sha256salt":
		want := 32
		if strings.HasPrefix(a, "sha256") {
			want = 64
		}
		h := strings.ToLower(password)
		if _, err := hex.DecodeString(h); err != nil || len(h) != want {
			return unusableHash, hashUnusable
		}
		if a == "md5" || a == "sha256" {
			salt = ""
		}
		return "xb$" + a + "$" + salt + "$" + h, hashLegacy
	}
	switch {
	case strings.HasPrefix(password, "$2"):
		return password, hashBcrypt
	case strings.HasPrefix(password, "$argon2id$"):
		return password, hashBcrypt // PHP password_hash(PASSWORD_ARGON2ID) uses the same PHC string
	}
	return unusableHash, hashUnusable
}

type emailCandidate struct {
	id       uint64
	activity int64
}

// resolveEmails lower-cases every email and renames case-insensitive duplicates (§5): the account with the
// latest max(last_login_at, t, updated_at) keeps the address (tie: lowest id), the others become
// <local>+dup<id>@<domain>.
func (im *importer) resolveEmails() (map[uint64]string, error) {
	groups := map[string][]emailCandidate{}
	err := im.src.each(im.ctx, "v2_user", "", nil, func(r row) error {
		e := strings.ToLower(strings.TrimSpace(r.str("email")))
		act := max(r.i64("last_login_at"), r.i64("t"), r.i64("updated_at"))
		groups[e] = append(groups[e], emailCandidate{id: r.id(), activity: act})
		return nil
	})
	if err != nil {
		return nil, err
	}
	out := map[uint64]string{}
	used := map[string]bool{}
	for e := range groups {
		used[e] = true
	}
	emails := make([]string, 0, len(groups))
	for e := range groups {
		emails = append(emails, e)
	}
	sort.Strings(emails)
	for _, e := range emails {
		c := groups[e]
		sort.Slice(c, func(i, j int) bool {
			if c[i].activity != c[j].activity {
				return c[i].activity > c[j].activity
			}
			return c[i].id < c[j].id
		})
		out[c[0].id] = e
		for _, loser := range c[1:] {
			renamed := dupEmail(e, loser.id, used)
			used[renamed] = true
			out[loser.id] = renamed
			im.warn(scopeID("users", loser.id), "email %q collides with user #%d (case-insensitive) → renamed to %q; merge the accounts if needed", e, c[0].id, renamed)
		}
	}
	return out, nil
}

func dupEmail(email string, id uint64, used map[string]bool) string {
	local, domain := email, ""
	if i := strings.LastIndex(email, "@"); i >= 0 {
		local, domain = email[:i], email[i:]
	}
	for n := 0; ; n++ {
		suffix := fmt.Sprintf("+dup%d", id)
		if n > 0 {
			suffix += fmt.Sprintf("-%d", n)
		}
		if c := local + suffix + domain; !used[c] {
			return c
		}
	}
}

func (im *importer) importUsers() error {
	stat := im.rep.Table("users")
	emails, err := im.resolveEmails()
	if err != nil {
		return err
	}
	// every user is imported, so the id set is known before the main pass (invite_user_id resolves in one pass)
	for id := range emails {
		im.users[id] = true
	}
	nowL := im.start.In(im.loc)
	seenUUID := map[string]uint64{}
	seenToken := map[string]uint64{}
	var bcrypt, legacy, unusable int
	b := newBatcher[model.User](im.tx)
	err = im.src.each(im.ctx, "v2_user", "", nil, func(r row) error {
		stat.Source++
		id := r.id()
		scope := scopeID("users", id)
		u := model.User{
			ID:                id,
			Email:             emails[id],
			TransferEnable:    r.i64("transfer_enable"),
			U:                 r.i64("u"),
			D:                 r.i64("d"),
			SpeedLimit:        nz(r["speed_limit"]),
			Balance:           r.i64("balance"),
			CommissionBalance: r.i64("commission_balance"),
			Discount:          nz(r["discount"]),
			IsAdmin:           r.bool("is_admin"),
			IsStaff:           r.bool("is_staff"),
			Banned:            r.bool("banned"),
			RemindExpire:      r.bool("remind_expire"),
			RemindTraffic:     r.bool("remind_traffic"),
			LastLoginAt:       im.tsPtr(scope, r["last_login_at"]),
			Remarks:           r.str("remarks"),
			CreatedAt:         im.ts(scope, r["created_at"]),
			UpdatedAt:         im.ts(scope, r["updated_at"]),
		}
		if !strings.Contains(u.Email, "@") {
			im.warn(scope, "email %q is not a valid address — imported as is", u.Email)
		}

		hash, kind := mapPassword(r.str("password"), r.str("password_algo"), r.str("password_salt"))
		u.PasswordHash = hash
		switch kind {
		case hashBcrypt:
			bcrypt++
		case hashLegacy:
			legacy++
		default:
			unusable++
			im.warn(scope, "password hash (algo %q) cannot be verified — the user must reset the password", r.str("password_algo"))
		}

		// uuid and token are credentials: warnings never include their values
		u.UUID = strings.TrimSpace(r.str("uuid"))
		if prev, dup := seenUUID[strings.ToLower(u.UUID)]; u.UUID == "" || dup || len(u.UUID) > 36 {
			u.UUID = uuid.NewString()
			if dup {
				im.warn(scope, "uuid duplicates user #%d → new uuid: the user must re-import the subscription", prev)
			} else {
				im.warn(scope, "uuid is empty or longer than 36 characters → new uuid: the user must re-import the subscription")
			}
		}
		seenUUID[strings.ToLower(u.UUID)] = id
		u.Token = strings.TrimSpace(r.str("token"))
		if prev, dup := seenToken[strings.ToLower(u.Token)]; u.Token == "" || dup || len(u.Token) > 32 {
			u.Token = usersvc.NewToken()
			if dup {
				im.warn(scope, "token duplicates user #%d → new token: the old subscription URL stops working", prev)
			} else {
				im.warn(scope, "token is empty or longer than 32 characters → new token: the old subscription URL stops working")
			}
		}
		seenToken[strings.ToLower(u.Token)] = id

		if pid, ok := r.int("plan_id"); ok && pid > 0 {
			if im.plans[uint64(pid)] != nil {
				p := uint64(pid)
				u.PlanID = &p
			} else {
				im.warn(scope, "plan_id %d is not an imported plan → NULL", pid)
			}
		}
		if gid, ok := r.int("group_id"); ok && gid > 0 {
			if im.groups[uint64(gid)] {
				g := uint64(gid)
				u.GroupID = &g
			} else {
				im.warn(scope, "group_id %d is not an imported server group → NULL", gid)
			}
		}
		if u.U < 0 || u.D < 0 {
			im.warn(scope, "negative traffic u=%d d=%d → 0", u.U, u.D)
			u.U, u.D = max(u.U, 0), max(u.D, 0)
		}
		if r.has("device_limit") {
			u.DeviceLimit = nz(r["device_limit"])
		}
		switch ct := r.i64("commission_type"); ct {
		case 0, 1, 2:
			u.CommissionType = model.CommissionType(ct)
		default:
			im.warn(scope, "commission_type %d unknown → 0 (system default)", ct)
		}
		if f, ok := flexFloat(r["commission_rate"]); ok {
			v := int(math.Round(f))
			u.CommissionRate = &v
		}
		if inv, ok := r.int("invite_user_id"); ok && inv > 0 {
			switch {
			case uint64(inv) == id:
				im.warn(scope, "invite_user_id points to the user itself → NULL")
			case !im.users[uint64(inv)]:
				im.warn(scope, "invite_user_id %d not found → NULL", inv)
			default:
				v := uint64(inv)
				u.InviteUserID = &v
			}
		}
		if tg, ok := r.int("telegram_id"); ok && tg != 0 {
			u.TelegramID = &tg
		}

		// expired_at: NULL = never expires; 0 = expired / never bought (Xboard treats it as unavailable)
		if !r.isNull("expired_at") {
			if t, _, ok := unixTime(r["expired_at"]); ok {
				exp := t
				if n := r.i64("expired_at"); n > msThreshold {
					im.warn(scope, "expired_at %d looks like milliseconds; divided by 1000", n)
				}
				u.ExpiredAt = &exp
			} else {
				exp := u.CreatedAt
				u.ExpiredAt = &exp
			}
		}
		u.NextResetAt = im.nextResetAt(&u, nowL)

		if u.PlanID != nil {
			if p := im.plans[*u.PlanID]; p != nil && (u.ExpiredAt == nil || u.ExpiredAt.After(im.start)) {
				p.active++
			}
		}
		return b.add(u)
	})
	if err != nil {
		return err
	}
	if err := b.flush(); err != nil {
		return err
	}
	stat.Imported = b.n
	stat.Note = fmt.Sprintf("bcrypt %d · md5/sha256 legacy hashes %d · unusable %d", bcrypt, legacy, unusable)
	im.rep.LegacyHashes, im.rep.UnusableHashes = legacy, unusable

	ids := make([]uint64, 0, len(im.plans))
	for id := range im.plans {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		p := im.plans[id]
		if p.capacity != nil && p.active > *p.capacity {
			im.warn(scopeID("plans", id), "capacity_limit %d is already exceeded by %d active users: the plan is sold out", *p.capacity, p.active)
		}
	}
	return nil
}

// nextResetAt recomputes next_reset_at (§4.5.2) in --source-tz: nil without an imported plan, for method 2,
// for lifetime users (expired_at NULL) and for expired users.
func (im *importer) nextResetAt(u *model.User, nowL time.Time) *time.Time {
	if u.PlanID == nil || u.ExpiredAt == nil || !u.ExpiredAt.After(im.start) {
		return nil
	}
	method := im.resetMethod
	if p := im.plans[*u.PlanID]; p != nil && p.reset != nil {
		method = *p.reset
	}
	t := usersvc.NextResetAt(method, u.ExpiredAt, nowL)
	if t == nil {
		return nil
	}
	utc := t.UTC()
	return &utc
}
