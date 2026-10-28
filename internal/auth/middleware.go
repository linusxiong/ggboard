package auth

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/cache"
	"github.com/linusxiong/ggboard/internal/httpx"
	"github.com/linusxiong/ggboard/internal/model"
)

// Context keys.
const (
	CtxUser   = "auth.user"
	CtxClaims = "auth.claims"
)

// UserCacheTTL is how long authenticated users are cached by the middleware.
const UserCacheTTL = 30 * time.Second

func userCacheKey(id uint64) string { return "auth:user:" + strconv.FormatUint(id, 10) }

// InvalidateUser drops the middleware's cached copy of user id. Call it after changing a user's password,
// ban state, roles or security credentials so the change applies to the next request.
func InvalidateUser(a *app.App, id uint64) { a.Cache.InvalidateNamespace(userCacheKey(id)) }

// CurrentUser returns the authenticated user (a request-local copy that may be up to 30s stale: reload it
// inside a transaction before changing balances or traffic), or nil.
func CurrentUser(c *gin.Context) *model.User {
	if v, ok := c.Get(CtxUser); ok {
		if u, ok := v.(*model.User); ok {
			return u
		}
	}
	return nil
}

// CurrentClaims returns the verified JWT claims, or nil.
func CurrentClaims(c *gin.Context) *Claims {
	if v, ok := c.Get(CtxClaims); ok {
		if cl, ok := v.(*Claims); ok {
			return cl
		}
	}
	return nil
}

// BearerToken extracts the token from "Authorization: Bearer <token>".
func BearerToken(c *gin.Context) string {
	h := c.GetHeader("Authorization")
	if len(h) > 7 && strings.EqualFold(h[:7], "bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}

// LoadUser returns user id through the 30s cache (a fresh copy per call).
func LoadUser(a *app.App, id uint64) (*model.User, error) {
	for {
		key := a.Cache.Key(userCacheKey(id), "")
		cached, err := cache.GetOrLoad(a.Cache, key, UserCacheTTL, func() (model.User, error) {
			var u model.User
			err := a.DB.First(&u, id).Error
			return u, err
		})
		// A load already in flight must neither refill nor authenticate against an invalidated generation.
		if key != a.Cache.Key(userCacheKey(id), "") {
			continue
		}
		if err != nil {
			return nil, err
		}
		u := cached
		return &u, nil
	}
}

// authenticate resolves the user; on failure it writes the error and returns false.
func authenticate(c *gin.Context, a *app.App) (*model.User, bool) {
	if u := CurrentUser(c); u != nil {
		return u, true
	}
	u, claims, err := resolve(c, a)
	if err != nil {
		httpx.Fail(c, err)
		return nil, false
	}
	c.Set(CtxUser, u)
	c.Set(CtxClaims, claims)
	return u, true
}

func resolve(c *gin.Context, a *app.App) (*model.User, *Claims, error) {
	tok := BearerToken(c)
	if tok == "" {
		return nil, nil, httpx.Unauthorized("authentication required")
	}
	claims, err := Parse(a, tok)
	if err != nil {
		return nil, nil, httpx.Unauthorized("invalid or expired token")
	}
	id, _ := claims.UserID()
	u, err := LoadUser(a, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, httpx.Unauthorized("invalid or expired token")
	}
	if err != nil {
		return nil, nil, httpx.Internal(err)
	}
	if !claims.BoundTo(u) {
		// A token may come from a writer outside this process. Recheck before rejecting it.
		key := a.Cache.Key(userCacheKey(id), "")
		var fresh model.User
		if err := a.DB.First(&fresh, id).Error; err != nil || !claims.BoundTo(&fresh) {
			return nil, nil, httpx.Unauthorized("session expired, please sign in again")
		}
		a.Cache.Set(key, fresh, UserCacheTTL)
		u = &fresh
	}
	if u.Banned {
		return nil, nil, httpx.Forbidden("account is banned")
	}
	return u, claims, nil
}

// RequireUser rejects requests without a valid session (401), with a stale account binding (401), or from
// banned users (403); otherwise stores *model.User under CtxUser.
func RequireUser(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		if _, ok := authenticate(c, a); ok {
			c.Next()
		}
	}
}

// RequireAdmin is RequireUser plus is_admin (403 otherwise).
func RequireAdmin(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		u, ok := authenticate(c, a)
		if !ok {
			return
		}
		if !u.IsAdmin {
			httpx.Fail(c, httpx.Forbidden("administrator only"))
			return
		}
		c.Next()
	}
}

// RequireStaff is RequireUser plus is_admin || is_staff (403 otherwise).
func RequireStaff(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		u, ok := authenticate(c, a)
		if !ok {
			return
		}
		if !u.IsStaffOrAdmin() {
			httpx.Fail(c, httpx.Forbidden("staff only"))
			return
		}
		c.Next()
	}
}

// OptionalUser sets CtxUser when a valid session is present and never rejects the request.
func OptionalUser(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		if CurrentUser(c) == nil && BearerToken(c) != "" {
			if u, claims, err := resolve(c, a); err == nil {
				c.Set(CtxUser, u)
				c.Set(CtxClaims, claims)
			}
		}
		c.Next()
	}
}
