package adminuser

import (
	"strings"
	"time"

	"gorm.io/gorm"
)

// Expiry filter values.
const (
	ExpiredActive  = "active"  // expired_at set and in the future
	ExpiredExpired = "expired" // expired_at set and not in the future
	ExpiredNever   = "never"   // expired_at null
)

// Filters are the user list filters shared by GET /admin/users (query string) and the export / mail
// endpoints (JSON body). Zero values do not filter. For plan_id, group_id and invite_user_id the value 0
// selects users where the column is null.
type Filters struct {
	ID           *uint64 `form:"id" json:"id"`
	Email        string  `form:"email" json:"email" binding:"max=191"` // case-insensitive substring
	UUID         string  `form:"uuid" json:"uuid" binding:"max=36"`
	Token        string  `form:"token" json:"token" binding:"max=32"`
	PlanID       *uint64 `form:"plan_id" json:"plan_id"`
	GroupID      *uint64 `form:"group_id" json:"group_id"`
	InviteUserID *uint64 `form:"invite_user_id" json:"invite_user_id"`
	Banned       *bool   `form:"banned" json:"banned"`
	IsAdmin      *bool   `form:"is_admin" json:"is_admin"`
	IsStaff      *bool   `form:"is_staff" json:"is_staff"`
	Expired      string  `form:"expired" json:"expired" binding:"omitempty,oneof=active expired never"`
	Remarks      string  `form:"remarks" json:"remarks" binding:"max=255"` // case-insensitive substring
}

// Apply adds the filters to q (a users query) evaluated at now.
func (f *Filters) Apply(q *gorm.DB, now time.Time) *gorm.DB {
	if f.ID != nil {
		q = q.Where("users.id = ?", *f.ID)
	}
	if v := strings.ToLower(strings.TrimSpace(f.Email)); v != "" {
		q = q.Where("users.email LIKE ? ESCAPE '!'", "%"+escapeLike(v)+"%")
	}
	if v := strings.TrimSpace(f.UUID); v != "" {
		q = q.Where("users.uuid = ?", v)
	}
	if v := strings.TrimSpace(f.Token); v != "" {
		q = q.Where("users.token = ?", v)
	}
	q = nullableID(q, "users.plan_id", f.PlanID)
	q = nullableID(q, "users.group_id", f.GroupID)
	q = nullableID(q, "users.invite_user_id", f.InviteUserID)
	if f.Banned != nil {
		q = q.Where("users.banned = ?", *f.Banned)
	}
	if f.IsAdmin != nil {
		q = q.Where("users.is_admin = ?", *f.IsAdmin)
	}
	if f.IsStaff != nil {
		q = q.Where("users.is_staff = ?", *f.IsStaff)
	}
	switch f.Expired {
	case ExpiredActive:
		q = q.Where("users.expired_at IS NOT NULL AND users.expired_at > ?", now)
	case ExpiredExpired:
		q = q.Where("users.expired_at IS NOT NULL AND users.expired_at <= ?", now)
	case ExpiredNever:
		q = q.Where("users.expired_at IS NULL")
	}
	if v := strings.ToLower(strings.TrimSpace(f.Remarks)); v != "" {
		q = q.Where("LOWER(users.remarks) LIKE ? ESCAPE '!'", "%"+escapeLike(v)+"%")
	}
	return q
}

func nullableID(q *gorm.DB, col string, v *uint64) *gorm.DB {
	switch {
	case v == nil:
		return q
	case *v == 0:
		return q.Where(col + " IS NULL")
	default:
		return q.Where(col+" = ?", *v)
	}
}

// escapeLike escapes LIKE wildcards with '!' (a portable ESCAPE character: backslash is itself an escape in
// MySQL string literals).
func escapeLike(s string) string {
	return strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(s)
}
