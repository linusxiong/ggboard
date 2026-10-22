package telegram

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"time"

	"gorm.io/gorm"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/auth"
	"github.com/linusxiong/ggboard/internal/cache"
	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/settings"
)

// Binding parameters.
const (
	BindCodeTTL    = 10 * time.Minute // one-time /start code
	BindCodeLength = 24               // alphanumeric (a valid t.me start parameter)
	BotCacheTTL    = time.Hour        // getMe result
	botFailureTTL  = time.Minute      // after a failed getMe, GET /user/telegram stops asking for this long
)

// CodeUnavailable is the API error code (HTTP 502) when the Bot API cannot be reached.
const CodeUnavailable = "telegram_unavailable"

// Errors.
var (
	ErrNotConfigured = errors.New("telegram: bot token is not set")
	ErrUnavailable   = errors.New("telegram: Bot API unavailable")
)

func kvBindCode(code string) string     { return "telegram:bind:" + code }
func kvUserCode(userID uint64) string   { return "telegram:bind:user:" + strconv.FormatUint(userID, 10) }
func botCacheKey(token string) string   { return "telegram:me:" + fingerprint(token) }
func botFailureKey(token string) string { return "telegram:me:failed:" + fingerprint(token) }

// fingerprint keys caches by token without keeping the token itself in cache keys.
func fingerprint(token string) string {
	sum := sha256.Sum256([]byte("ggboard-telegram:" + token))
	return hex.EncodeToString(sum[:8])
}

// BotUsername returns the bot's @username (without "@") through getMe, cached for BotCacheTTL.
func BotUsername(ctx context.Context, a *app.App) (string, error) {
	tok := Token(a)
	if tok == "" {
		return "", ErrNotConfigured
	}
	name, err := cache.GetOrLoad(a.Cache, botCacheKey(tok), BotCacheTTL, func() (string, error) {
		me, err := ClientFor(a).GetMe(ctx)
		if err != nil {
			return "", err
		}
		if me.Username == "" {
			return "", errors.New("telegram getMe: the bot has no username")
		}
		return me.Username, nil
	})
	if err != nil {
		a.Cache.Set(botFailureKey(tok), true, botFailureTTL)
		return "", fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	a.Cache.Delete(botFailureKey(tok))
	return name, nil
}

// cachedBotUsername returns the cached username without calling Telegram ("" when unknown).
func cachedBotUsername(a *app.App) string {
	name, _ := cache.GetAs[string](a.Cache, botCacheKey(Token(a)))
	return name
}

// BindCode is the answer of POST /user/telegram/bind-code.
type BindCode struct {
	Code        string `json:"code"`
	BotUsername string `json:"bot_username"`
	DeepLink    string `json:"deep_link"`
}

// IssueBindCode creates a one-time code (BindCodeTTL) that binds userID when sent to the bot as /start <code>.
// A user holds one live code: issuing a new one revokes the previous.
func IssueBindCode(ctx context.Context, a *app.App, userID uint64) (*BindCode, error) {
	bot, err := BotUsername(ctx, a)
	if err != nil {
		return nil, err
	}
	code := settings.RandomToken(BindCodeLength)
	a.KV.Set(kvBindCode(code), userID, BindCodeTTL)
	var prev string
	a.KV.Update(kvUserCode(userID), BindCodeTTL, false, func(old any, ok bool) any {
		if ok {
			prev, _ = old.(string)
		}
		return code
	})
	if prev != "" {
		a.KV.Delete(kvBindCode(prev))
	}
	return &BindCode{Code: code, BotUsername: bot, DeepLink: "https://t.me/" + bot + "?start=" + code}, nil
}

// Status is the answer of GET /user/telegram.
type Status struct {
	Enabled     bool    `json:"enabled"`                // a bot token is configured
	Bound       bool    `json:"bound"`                  // the account is bound to a Telegram account
	Username    *string `json:"username,omitempty"`     // Telegram @username at binding time (without "@")
	BotUsername string  `json:"bot_username,omitempty"` // omitted when the bot is off or Telegram is unreachable
	DiscussLink string  `json:"discuss_link"`           // telegram_discuss_link
}

// UserStatus describes u's binding. The bot username comes from the getMe cache; after a failed getMe it is
// omitted for a minute instead of delaying the page.
func UserStatus(ctx context.Context, a *app.App, u *model.User) Status {
	st := Status{Enabled: Enabled(a), DiscussLink: a.Settings.String(KeyDiscussLink)}
	if u.TelegramID != nil && *u.TelegramID != 0 {
		st.Bound, st.Username = true, u.TelegramUsername
	}
	if st.Enabled {
		if _, failed := a.Cache.Get(botFailureKey(Token(a))); !failed {
			ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
			defer cancel()
			if name, err := BotUsername(ctx, a); err == nil {
				st.BotUsername = name
			} else {
				a.Log.Warn("telegram: getMe failed", "err", err)
			}
		}
	}
	return st
}

// Unbind clears userID's binding and revokes its pending bind code.
func Unbind(ctx context.Context, a *app.App, userID uint64) error {
	if code, ok := a.KV.Take(kvUserCode(userID)); ok {
		if s, _ := code.(string); s != "" {
			a.KV.Delete(kvBindCode(s))
		}
	}
	err := a.DB.WithContext(ctx).Model(&model.User{}).Where("id = ?", userID).
		Updates(map[string]any{"telegram_id": nil, "telegram_username": nil}).Error
	if err != nil {
		return err
	}
	auth.InvalidateUser(a, userID)
	a.Log.Info("telegram: account unbound", "user_id", userID)
	return nil
}

// bind consumes code and binds its user to the Telegram chat. Any other account bound to the same chat is
// unbound first (one Telegram account ↔ one panel account). It returns the bound user, or nil for an unknown,
// expired or used code.
func bind(ctx context.Context, a *app.App, code string, chat int64, username string) (*model.User, error) {
	v, ok := a.KV.Take(kvBindCode(code))
	if !ok {
		return nil, nil
	}
	userID, _ := v.(uint64)
	a.KV.Delete(kvUserCode(userID))
	var name *string
	if username != "" {
		name = &username
	}
	var (
		u      model.User
		others []uint64
	)
	err := a.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.User{}).Where("telegram_id = ? AND id <> ?", chat, userID).Pluck("id", &others).Error; err != nil {
			return err
		}
		if len(others) > 0 {
			if err := tx.Model(&model.User{}).Where("id IN ?", others).
				Updates(map[string]any{"telegram_id": nil, "telegram_username": nil}).Error; err != nil {
				return err
			}
		}
		res := tx.Model(&model.User{}).Where("id = ?", userID).
			Updates(map[string]any{"telegram_id": chat, "telegram_username": name})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return tx.Select("id", "email").Where("id = ?", userID).First(&u).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil // the account was deleted meanwhile
	}
	if err != nil {
		return nil, err
	}
	for _, id := range append(others, userID) {
		auth.InvalidateUser(a, id)
	}
	a.Log.Info("telegram: account bound", "user_id", userID, "replaced", len(others))
	return &u, nil
}

// unbindChat clears every account bound to chat and returns how many there were.
func unbindChat(ctx context.Context, a *app.App, chat int64) (int, error) {
	var ids []uint64
	if err := a.DB.WithContext(ctx).Model(&model.User{}).Where("telegram_id = ?", chat).Pluck("id", &ids).Error; err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, nil
	}
	if err := a.DB.WithContext(ctx).Model(&model.User{}).Where("id IN ?", ids).
		Updates(map[string]any{"telegram_id": nil, "telegram_username": nil}).Error; err != nil {
		return 0, err
	}
	for _, id := range ids {
		auth.InvalidateUser(a, id)
	}
	a.Log.Info("telegram: account unbound from the bot", "users", len(ids))
	return len(ids), nil
}
