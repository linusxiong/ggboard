package auth

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/model"
)

// Issuer is the JWT "iss" claim.
const Issuer = "ggboard"

// Claims are the session token claims: sub = user id, uuid, email, ver = password_version, iat, exp.
//
// sub is a reusable primary key (import xboard --force, a restored backup or an id-reusing MySQL hand it to
// another account), so a token is bound to the account instance by sub + uuid + email + ver. The UUID also
// prevents a recreated account with the same id, email and initial password_version from reviving old tokens.
type Claims struct {
	Email string `json:"email"`
	UUID  string `json:"uuid"`
	Ver   int    `json:"ver"`
	jwt.RegisteredClaims
}

// UserID parses the subject.
func (c *Claims) UserID() (uint64, error) {
	id, err := strconv.ParseUint(c.Subject, 10, 64)
	if err != nil || id == 0 {
		return 0, errors.New("auth: invalid subject")
	}
	return id, nil
}

// normEmail is the comparison form of an email (usersvc.NormalizeEmail; auth cannot import it).
func normEmail(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

// BoundTo reports whether the token was issued to u as it is now: same id, UUID, email and password_version.
func (c *Claims) BoundTo(u *model.User) bool {
	id, err := c.UserID()
	return err == nil && id == u.ID && c.UUID != "" && c.UUID == u.UUID && u.PasswordVersion == c.Ver &&
		normEmail(c.Email) != "" && normEmail(c.Email) == normEmail(u.Email)
}

// Issue signs a session token for u valid for cfg.JWT.TTL.
func Issue(a *app.App, u *model.User) (string, error) {
	return IssueWith([]byte(a.Cfg.JWT.Secret), a.Cfg.JWT.TTL, u, time.Now())
}

// IssueWith signs a token with an explicit secret, ttl and issue time.
func IssueWith(secret []byte, ttl time.Duration, u *model.User, now time.Time) (string, error) {
	if len(secret) == 0 {
		return "", errors.New("auth: empty jwt secret")
	}
	if normEmail(u.Email) == "" {
		return "", errors.New("auth: user has no email")
	}
	if u.UUID == "" {
		return "", errors.New("auth: user has no uuid")
	}
	claims := Claims{
		Email: u.Email,
		UUID:  u.UUID,
		Ver:   u.PasswordVersion,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    Issuer,
			Subject:   strconv.FormatUint(u.ID, 10),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}
	s, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(secret)
	if err != nil {
		return "", fmt.Errorf("auth: sign: %w", err)
	}
	return s, nil
}

// Parse verifies an HS256 token (signature, iss, exp, iat with 30s leeway, a numeric sub and non-empty email/uuid)
// and returns its claims.
func Parse(a *app.App, token string) (*Claims, error) {
	return ParseWith([]byte(a.Cfg.JWT.Secret), token)
}

// ParseWith is Parse with an explicit secret.
func ParseWith(secret []byte, token string) (*Claims, error) {
	claims := &Claims{}
	_, err := jwt.ParseWithClaims(token, claims, func(*jwt.Token) (any, error) { return secret, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(Issuer),
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
		jwt.WithLeeway(30*time.Second),
	)
	if err != nil {
		return nil, err
	}
	if _, err := claims.UserID(); err != nil {
		return nil, err
	}
	if normEmail(claims.Email) == "" {
		return nil, errors.New("auth: token has no email claim")
	}
	if claims.UUID == "" {
		return nil, errors.New("auth: token has no uuid claim")
	}
	return claims, nil
}
