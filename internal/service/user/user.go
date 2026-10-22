// Package user contains user-domain logic shared by the API packages: lookup, creation, authentication
// (with transparent hash upgrades), security resets, subscription URLs, traffic reset dates and the
// available-user lists delivered to nodes.
package user

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	mrand "math/rand/v2"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/auth"
	"github.com/linusxiong/ggboard/internal/cache"
	"github.com/linusxiong/ggboard/internal/model"
)

// Errors.
var (
	ErrInvalidCredentials = errors.New("user: invalid email or password")
	ErrBanned             = errors.New("user: account is banned")
	ErrEmailTaken         = errors.New("user: email already registered")
	ErrNotFound           = errors.New("user: not found")
)

// SubscribePath is the client subscription endpoint (token appended).
const SubscribePath = "/api/v1/client/subscribe?token="

// NodeUsersNamespace is the cache namespace of AvailableUsersForGroups (see InvalidateNodeUsers).
const NodeUsersNamespace = "node_users"

// NodeUsersTTL is how long available-user lists are cached.
const NodeUsersTTL = 60 * time.Second

// Service is cheap to construct: user.New(a) per request is fine.
type Service struct{ a *app.App }

// New returns a Service bound to a.
func New(a *app.App) *Service { return &Service{a: a} }

// NormalizeEmail lower-cases and trims an email address.
func NormalizeEmail(email string) string { return strings.ToLower(strings.TrimSpace(email)) }

// NewToken returns a 32-char random hex subscription token.
func NewToken() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("user: crypto/rand failed: " + err.Error())
	}
	return hex.EncodeToString(b[:])
}

// FindByID loads a user (ErrNotFound when missing).
func (s *Service) FindByID(ctx context.Context, id uint64) (*model.User, error) {
	return s.first(ctx, "id = ?", id)
}

// FindByEmail loads a user by (normalized) email.
func (s *Service) FindByEmail(ctx context.Context, email string) (*model.User, error) {
	return s.first(ctx, "email = ?", NormalizeEmail(email))
}

// FindByToken loads a user by subscription token.
func (s *Service) FindByToken(ctx context.Context, token string) (*model.User, error) {
	if token == "" {
		return nil, ErrNotFound
	}
	return s.first(ctx, "token = ?", token)
}

func (s *Service) first(ctx context.Context, query string, arg any) (*model.User, error) {
	var u model.User
	err := s.a.DB.WithContext(ctx).Where(query, arg).First(&u).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// CreateOptions are optional attributes of a new user.
type CreateOptions struct {
	PlanID         *uint64
	GroupID        *uint64
	TransferEnable int64 // bytes
	ExpiredAt      *time.Time
	SpeedLimit     *int
	DeviceLimit    *int
	InviteUserID   *uint64
	Balance        int64
	IsAdmin        bool
	IsStaff        bool
	Remarks        string
	// PasswordHash, when set, is stored as-is instead of hashing the password (imports).
	PasswordHash string
}

// Create registers a user with a hashed password, fresh uuid/token and remind defaults from settings.
// It returns ErrEmailTaken for duplicates.
func (s *Service) Create(ctx context.Context, email, password string, opts CreateOptions) (*model.User, error) {
	email = NormalizeEmail(email)
	if email == "" {
		return nil, errors.New("user: email is required")
	}
	hash := opts.PasswordHash
	if hash == "" {
		var err error
		if hash, err = auth.HashPassword(password); err != nil {
			return nil, err
		}
	}
	u := &model.User{
		Email:          email,
		PasswordHash:   hash,
		UUID:           uuid.NewString(),
		Token:          NewToken(),
		PlanID:         opts.PlanID,
		GroupID:        opts.GroupID,
		TransferEnable: opts.TransferEnable,
		ExpiredAt:      opts.ExpiredAt,
		SpeedLimit:     opts.SpeedLimit,
		DeviceLimit:    opts.DeviceLimit,
		InviteUserID:   opts.InviteUserID,
		Balance:        opts.Balance,
		IsAdmin:        opts.IsAdmin,
		IsStaff:        opts.IsStaff,
		Remarks:        opts.Remarks,
		RemindExpire:   s.a.Settings.Bool("default_remind_expire"),
		RemindTraffic:  s.a.Settings.Bool("default_remind_traffic"),
	}
	err := s.a.DB.WithContext(ctx).Create(u).Error
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return nil, ErrEmailTaken
	}
	if err != nil {
		return nil, fmt.Errorf("user: create: %w", err)
	}
	s.InvalidateNodeUsers()
	return u, nil
}

// SetPassword stores a new argon2id hash and bumps password_version (invalidating existing sessions).
func (s *Service) SetPassword(ctx context.Context, u *model.User, password string) error {
	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	res := s.a.DB.WithContext(ctx).Model(&model.User{}).Where("id = ?", u.ID).Updates(map[string]any{
		"password_hash":    hash,
		"password_version": gorm.Expr("password_version + 1"),
	})
	if res.Error != nil {
		return fmt.Errorf("user: set password: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	u.PasswordHash = hash
	u.PasswordVersion++
	auth.InvalidateUser(s.a, u.ID)
	return nil
}

// Authenticate verifies email/password. Legacy hashes (bcrypt, Xboard) are upgraded to argon2id without
// bumping password_version. Banned users get ErrBanned (after a correct password).
func (s *Service) Authenticate(ctx context.Context, email, password string) (*model.User, error) {
	u, err := s.FindByEmail(ctx, email)
	if errors.Is(err, ErrNotFound) {
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, err
	}
	if !auth.VerifyPassword(u.PasswordHash, password) {
		return nil, ErrInvalidCredentials
	}
	if auth.NeedsRehash(u.PasswordHash) {
		if hash, herr := auth.HashPassword(password); herr == nil {
			if err := s.a.DB.WithContext(ctx).Model(&model.User{}).Where("id = ?", u.ID).
				UpdateColumn("password_hash", hash).Error; err != nil {
				s.a.Log.Warn("user: password rehash failed", "user_id", u.ID, "err", err)
			} else {
				u.PasswordHash = hash
			}
		}
	}
	if u.Banned {
		return u, ErrBanned
	}
	return u, nil
}

// RecordLogin stores last_login_at / last_login_ip.
func (s *Service) RecordLogin(ctx context.Context, u *model.User, ip string) error {
	now := time.Now()
	if len(ip) > 45 {
		ip = ip[:45]
	}
	if err := s.a.DB.WithContext(ctx).Model(&model.User{}).Where("id = ?", u.ID).
		UpdateColumns(map[string]any{"last_login_at": now, "last_login_ip": ip}).Error; err != nil {
		return err
	}
	u.LastLoginAt, u.LastLoginIP = &now, ip
	return nil
}

// ResetSecurity issues a new uuid and subscription token and bumps password_version.
func (s *Service) ResetSecurity(ctx context.Context, u *model.User) error {
	newUUID, newToken := uuid.NewString(), NewToken()
	res := s.a.DB.WithContext(ctx).Model(&model.User{}).Where("id = ?", u.ID).Updates(map[string]any{
		"uuid":             newUUID,
		"token":            newToken,
		"password_version": gorm.Expr("password_version + 1"),
	})
	if res.Error != nil {
		return fmt.Errorf("user: reset security: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	u.UUID, u.Token = newUUID, newToken
	u.PasswordVersion++
	auth.InvalidateUser(s.a, u.ID)
	s.InvalidateNodeUsers()
	return nil
}

// SubscribeURL returns the subscription URL: a random entry of settings.subscribe_url, else app_url, plus
// SubscribePath + token. Without any configured base it returns the path only.
func (s *Service) SubscribeURL(u *model.User) string {
	bases := s.a.Settings.Strings("subscribe_url")
	base := ""
	if len(bases) > 0 {
		base = bases[mrand.IntN(len(bases))] //nolint:gosec // load spreading, not security sensitive
	} else {
		base = s.a.Settings.String("app_url")
	}
	return strings.TrimRight(base, "/") + SubscribePath + url.QueryEscape(u.Token)
}

// ResetMethodFor returns the effective reset method of plan (nil plan method -> settings.reset_traffic_method).
func (s *Service) ResetMethodFor(plan *model.Plan) model.ResetMethod {
	if plan != nil && plan.ResetTrafficMethod != nil {
		return *plan.ResetTrafficMethod
	}
	return model.ResetMethod(s.a.Settings.Int("reset_traffic_method"))
}

// ComputeNextResetAt returns when u's traffic resets next under plan, or nil (no plan, no expiry, or
// method "never"). See NextResetAt.
func (s *Service) ComputeNextResetAt(u *model.User, plan *model.Plan, now time.Time) *time.Time {
	if plan == nil {
		return nil
	}
	return NextResetAt(s.ResetMethodFor(plan), u.ExpiredAt, now)
}

// ResetDay returns the whole days until u.next_reset_at (rounded up, minimum 0), or nil if none.
func (s *Service) ResetDay(u *model.User) *int {
	return ResetDays(u.NextResetAt, time.Now())
}

// ResetDays returns ceil((next - now) / 24h) (minimum 0), or nil when next is nil.
func ResetDays(next *time.Time, now time.Time) *int {
	if next == nil {
		return nil
	}
	d := next.Sub(now)
	days := 0
	if d > 0 {
		days = int((d + 24*time.Hour - 1) / (24 * time.Hour))
	}
	return &days
}

// NodeUser is the per-user record delivered to node backends (0 = unlimited).
type NodeUser struct {
	ID          uint64 `json:"id"`
	UUID        string `json:"uuid"`
	SpeedLimit  int    `json:"speed_limit"`
	DeviceLimit int    `json:"device_limit"`
}

// AvailableUsersForGroups returns available users (not banned, not expired, u+d < transfer_enable) whose
// group_id is in groupIDs, ordered by id. Results are cached for 60s per group set and shared between
// callers: do not mutate the returned slice.
func (s *Service) AvailableUsersForGroups(ctx context.Context, groupIDs []uint64) ([]NodeUser, error) {
	ids := slices.Clone(groupIDs)
	slices.Sort(ids)
	ids = slices.Compact(ids)
	if len(ids) == 0 {
		return []NodeUser{}, nil
	}
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = strconv.FormatUint(id, 10)
	}
	key := s.a.Cache.Key(NodeUsersNamespace, strings.Join(parts, ","))
	return cache.GetOrLoad(s.a.Cache, key, NodeUsersTTL, func() ([]NodeUser, error) {
		var rows []model.User
		err := s.a.DB.WithContext(ctx).
			Select("id", "uuid", "speed_limit", "device_limit").
			Where("group_id IN ?", ids).
			Where("banned = ?", false).
			Where("expired_at IS NULL OR expired_at > ?", time.Now()).
			Where("u + d < transfer_enable").
			Order("id").
			Find(&rows).Error
		if err != nil {
			return nil, err
		}
		out := make([]NodeUser, len(rows))
		for i, r := range rows {
			out[i] = NodeUser{ID: r.ID, UUID: r.UUID}
			if r.SpeedLimit != nil {
				out[i].SpeedLimit = *r.SpeedLimit
			}
			if r.DeviceLimit != nil {
				out[i].DeviceLimit = *r.DeviceLimit
			}
		}
		return out, nil
	})
}

// InvalidateNodeUsers drops every cached AvailableUsersForGroups result (call after changing users' plan,
// group, ban, traffic or expiry when nodes must see it before the 60s TTL).
func (s *Service) InvalidateNodeUsers() { s.a.Cache.InvalidateNamespace(NodeUsersNamespace) }
