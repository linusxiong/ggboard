package engine

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/cache"
	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/subscription"
	"github.com/linusxiong/ggboard/internal/subscription/rulesets"
)

// Multiple rule profiles per user (docs/SUBSCRIPTION.md "Multiple profiles"): up to model.MaxRuleProfiles
// named profiles with a slug unique per user, exactly one of them the default. A user without rows has a
// virtual default profile that follows the admin's default template; the first write creates its row.

// Profile store errors (the API maps them to 404 / 409 / 400).
var (
	ErrProfileNotFound = errors.New("engine: rule profile not found")
	ErrProfileLimit    = fmt.Errorf("engine: at most %d rule profiles", model.MaxRuleProfiles)
	ErrSlugTaken       = errors.New("engine: slug already used by another profile")
	ErrDefaultProfile  = errors.New("engine: the default profile cannot be deleted")
)

// MaxProfileName bounds profile names (runes).
const MaxProfileName = 64

// SlugPattern is the format of profile slugs.
var SlugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)

// ValidSlug reports why s cannot be a slug ("" when it can).
func ValidSlug(s string) string {
	if !SlugPattern.MatchString(s) {
		return "must be 1-32 characters: a-z, 0-9, - and _, starting with a letter or digit"
	}
	return ""
}

// ValidProfileName reports why s cannot be a profile name ("" when it can).
func ValidProfileName(s string) string {
	switch {
	case strings.TrimSpace(s) == "":
		return "is required"
	case utf8.RuneCountInString(s) > MaxProfileName:
		return fmt.Sprintf("must be at most %d characters", MaxProfileName)
	}
	for _, r := range s {
		if subscription.UnsafeRune(r) {
			return "must not contain control or invisible characters"
		}
	}
	return ""
}

// LoadProfiles returns the user's profiles ordered by id (cached; every write below invalidates). Callers must
// not modify the rows.
func LoadProfiles(ctx context.Context, a *app.App, userID uint64) ([]model.RuleProfile, error) {
	key := profileKey(a, userID)
	lctx := context.WithoutCancel(ctx) // singleflight: shared by concurrent callers
	return cache.GetOrLoad(a.Cache, key, ProfileTTL, func() ([]model.RuleProfile, error) {
		var rows []model.RuleProfile
		if err := a.DB.WithContext(lctx).Where("user_id = ?", userID).Order("id").Find(&rows).Error; err != nil {
			return nil, err
		}
		if rows == nil {
			rows = []model.RuleProfile{}
		}
		return rows, nil
	})
}

// DefaultOf returns the default profile among rows: the one flagged is_default, else the oldest (nil when
// rows is empty).
func DefaultOf(rows []model.RuleProfile) *model.RuleProfile {
	for i := range rows {
		if rows[i].IsDefault {
			return &rows[i]
		}
	}
	if len(rows) > 0 {
		return &rows[0]
	}
	return nil
}

// FindSlug returns the profile with slug (nil when none).
func FindSlug(rows []model.RuleProfile, slug string) *model.RuleProfile {
	for i := range rows {
		if rows[i].Slug == slug {
			return &rows[i]
		}
	}
	return nil
}

// rawData returns the stored profile document of r ([]byte{} when it follows the defaults).
func rawData(r *model.RuleProfile) []byte {
	if r == nil || len(r.Data) == 0 || string(r.Data) == "null" {
		return []byte{}
	}
	return []byte(r.Data)
}

// LoadStoredProfile returns the raw stored document of the user's default profile ([]byte{} when there is
// none or it follows the defaults).
func LoadStoredProfile(ctx context.Context, a *app.App, userID uint64) ([]byte, error) {
	rows, err := LoadProfiles(ctx, a, userID)
	if err != nil {
		return nil, err
	}
	return rawData(DefaultOf(rows)), nil
}

// decodeStored decodes a stored profile document (nil when empty or no longer decodable).
func decodeStored(a *app.App, c *subscription.Catalog, userID uint64, raw []byte) *subscription.RuleProfile {
	if len(raw) == 0 {
		return nil
	}
	p, err := rulesets.DecodeProfile(c, raw, rulesets.DefaultsFrom(a.Settings))
	if err != nil {
		a.Log.Warn("subscription: ignoring undecodable rule profile", "user_id", userID, "err", err)
		return nil // a broken stored profile falls back to the defaults
	}
	return &p
}

// StoredProfile decodes the user's default profile (nil when none, following the defaults, or undecodable).
func StoredProfile(ctx context.Context, a *app.App, c *subscription.Catalog, userID uint64) (*subscription.RuleProfile, error) {
	raw, err := LoadStoredProfile(ctx, a, userID)
	if err != nil {
		return nil, err
	}
	return decodeStored(a, c, userID, raw), nil
}

// StoredProfileSlug decodes the profile with slug ("" = the default profile). The row is nil when the slug
// does not exist (ErrProfileNotFound, except for "" and "default" of a user without rows, which is the
// virtual default profile).
func StoredProfileSlug(ctx context.Context, a *app.App, c *subscription.Catalog, userID uint64, slug string) (*model.RuleProfile, *subscription.RuleProfile, error) {
	rows, err := LoadProfiles(ctx, a, userID)
	if err != nil {
		return nil, nil, err
	}
	var row *model.RuleProfile
	if slug == "" {
		row = DefaultOf(rows)
	} else {
		row = FindSlug(rows, slug)
	}
	if row == nil {
		if len(rows) == 0 && (slug == "" || slug == model.RuleProfileDefaultSlug) {
			return nil, nil, nil
		}
		return nil, nil, ErrProfileNotFound
	}
	return row, decodeStored(a, c, userID, rawData(row)), nil
}

// EffectiveProfile returns the profile a request uses. ov.Template carries ?profile=: a slug of one of the
// user's profiles selects that profile; otherwise a catalog template id overrides the default profile's
// rules (the original meaning of ?profile=); anything else selects the default profile. With
// allow_user_rule_profile=false everyone gets the default template (stored profiles and overrides ignored).
func EffectiveProfile(ctx context.Context, a *app.App, c *subscription.Catalog, u *model.User, ov rulesets.Overrides) (subscription.RuleProfile, error) {
	def := rulesets.DefaultsFrom(a.Settings)
	if !a.Settings.Bool("allow_user_rule_profile") {
		return rulesets.DefaultProfile(c, def), nil
	}
	rows, err := LoadProfiles(ctx, a, u.ID)
	if err != nil {
		return subscription.RuleProfile{}, err
	}
	row := DefaultOf(rows)
	if ov.Template != "" {
		if r := FindSlug(rows, ov.Template); r != nil {
			row, ov.Template = r, ""
		} else if _, ok := c.Template(ov.Template); !ok {
			ov.Template = "" // unknown slug: the default profile
		}
	}
	return rulesets.ResolveProfile(c, decodeStored(a, c, u.ID, rawData(row)), def, ov)
}

// invalidateProfiles drops the cached profiles of a user. A per-user namespace generation rather than Delete:
// a load that read the old rows before a write can only store them under the previous generation's key.
func invalidateProfiles(a *app.App, userID uint64) {
	a.Cache.InvalidateNamespace(profileNamespace(userID))
}

// withProfiles runs fn in a transaction holding the user's row lock (serializes profile writes of one user)
// with the user's current profiles, then drops the cache.
func withProfiles(ctx context.Context, a *app.App, userID uint64, fn func(tx *gorm.DB, rows []model.RuleProfile) error) error {
	err := a.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// A no-op write takes the row lock on every driver (MySQL reports 0 affected rows for it, so the
		// count is not checked).
		if err := tx.Model(&model.User{}).Where("id = ?", userID).UpdateColumn("token", gorm.Expr("token")).Error; err != nil {
			return err
		}
		var rows []model.RuleProfile
		if err := tx.Where("user_id = ?", userID).Order("id").Find(&rows).Error; err != nil {
			return err
		}
		return fn(tx, rows)
	})
	invalidateProfiles(a, userID)
	return err
}

// ensureDefaultTx creates the default row of a user without profiles and returns the (possibly new) rows.
func ensureDefaultTx(tx *gorm.DB, userID uint64, rows []model.RuleProfile) ([]model.RuleProfile, error) {
	if len(rows) > 0 {
		return rows, nil
	}
	now := time.Now()
	r := model.RuleProfile{UserID: userID, Slug: model.RuleProfileDefaultSlug, Name: model.RuleProfileDefaultName,
		IsDefault: true, Data: model.RawJSON("null"), CreatedAt: now, UpdatedAt: now}
	if err := tx.Create(&r).Error; err != nil {
		return nil, err
	}
	return []model.RuleProfile{r}, nil
}

// EnsureDefault creates the default profile row of a user who has none (idempotent).
func EnsureDefault(ctx context.Context, a *app.App, userID uint64) error {
	rows, err := LoadProfiles(ctx, a, userID)
	if err != nil || len(rows) > 0 {
		return err
	}
	return withProfiles(ctx, a, userID, func(tx *gorm.DB, rows []model.RuleProfile) error {
		_, err := ensureDefaultTx(tx, userID, rows)
		return err
	})
}

// SaveProfile stores a validated profile document as the user's default profile (created when missing).
func SaveProfile(ctx context.Context, a *app.App, userID uint64, raw []byte) error {
	return SaveProfileSlug(ctx, a, userID, "", raw)
}

// SaveProfileSlug stores a validated profile document in the profile with slug ("" = the default profile; for
// a user without profiles "" and "default" create the default row). ErrProfileNotFound for unknown slugs.
func SaveProfileSlug(ctx context.Context, a *app.App, userID uint64, slug string, raw []byte) error {
	return withProfiles(ctx, a, userID, func(tx *gorm.DB, rows []model.RuleProfile) error {
		if len(rows) == 0 && (slug == "" || slug == model.RuleProfileDefaultSlug) {
			var err error
			if rows, err = ensureDefaultTx(tx, userID, rows); err != nil {
				return err
			}
		}
		row := DefaultOf(rows)
		if slug != "" {
			row = FindSlug(rows, slug)
		}
		if row == nil {
			return ErrProfileNotFound
		}
		return tx.Model(&model.RuleProfile{}).Where("id = ?", row.ID).
			Updates(map[string]any{"data": model.RawJSON(raw), "updated_at": time.Now()}).Error
	})
}

// slugFrom derives a slug from a profile name: ASCII letters and digits, other runs become "-"; "" when
// nothing is left.
func slugFrom(name string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			b.WriteRune(r)
			dash = false
		case b.Len() > 0 && !dash:
			b.WriteByte('-')
			dash = true
		}
		if b.Len() >= 28 {
			break
		}
	}
	return strings.Trim(b.String(), "-_")
}

// freeSlug returns base (or base-2, base-3, ...) not used by rows; "profile-N" when base is empty.
func freeSlug(rows []model.RuleProfile, base string) string {
	if base == "" {
		base = "profile"
	} else if FindSlug(rows, base) == nil {
		return base
	}
	for n := 2; ; n++ {
		s := base + "-" + strconv.Itoa(n)
		if FindSlug(rows, s) == nil {
			return s
		}
	}
}

// CreateProfile adds a named profile. slug "" derives one from the name; copyFrom (a slug, "" = none) copies
// that profile's document, otherwise the new profile follows the defaults. A user without profiles gets the
// default row first, so the new profile never silently becomes the default.
func CreateProfile(ctx context.Context, a *app.App, userID uint64, name, slug, copyFrom string) (*model.RuleProfile, error) {
	var out model.RuleProfile
	err := withProfiles(ctx, a, userID, func(tx *gorm.DB, rows []model.RuleProfile) error {
		rows, err := ensureDefaultTx(tx, userID, rows)
		if err != nil {
			return err
		}
		if len(rows) >= model.MaxRuleProfiles {
			return ErrProfileLimit
		}
		data := model.RawJSON("null")
		if copyFrom != "" {
			src := FindSlug(rows, copyFrom)
			if src == nil {
				return ErrProfileNotFound
			}
			if len(src.Data) > 0 {
				data = append(model.RawJSON(nil), src.Data...)
			}
		}
		if slug == "" {
			slug = freeSlug(rows, slugFrom(name))
		} else if FindSlug(rows, slug) != nil {
			return ErrSlugTaken
		}
		now := time.Now()
		out = model.RuleProfile{UserID: userID, Slug: slug, Name: strings.TrimSpace(name), Data: data, CreatedAt: now, UpdatedAt: now}
		return tx.Create(&out).Error
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateProfileMeta renames a profile and/or changes its slug (nil = unchanged); it returns the updated row.
func UpdateProfileMeta(ctx context.Context, a *app.App, userID uint64, slug string, name, newSlug *string) (*model.RuleProfile, error) {
	var out model.RuleProfile
	err := withProfiles(ctx, a, userID, func(tx *gorm.DB, rows []model.RuleProfile) error {
		rows, err := ensureDefaultTx(tx, userID, rows)
		if err != nil {
			return err
		}
		row := FindSlug(rows, slug)
		if row == nil {
			return ErrProfileNotFound
		}
		changes := map[string]any{"updated_at": time.Now()}
		if name != nil {
			changes["name"] = strings.TrimSpace(*name)
		}
		if newSlug != nil && *newSlug != row.Slug {
			if FindSlug(rows, *newSlug) != nil {
				return ErrSlugTaken
			}
			changes["slug"] = *newSlug
		}
		if err := tx.Model(&model.RuleProfile{}).Where("id = ?", row.ID).Updates(changes).Error; err != nil {
			return err
		}
		return tx.First(&out, row.ID).Error
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteProfile removes a profile; the default profile cannot be deleted (ErrDefaultProfile).
func DeleteProfile(ctx context.Context, a *app.App, userID uint64, slug string) error {
	return withProfiles(ctx, a, userID, func(tx *gorm.DB, rows []model.RuleProfile) error {
		row := FindSlug(rows, slug)
		if row == nil {
			if len(rows) == 0 && slug == model.RuleProfileDefaultSlug {
				return ErrDefaultProfile
			}
			return ErrProfileNotFound
		}
		if def := DefaultOf(rows); def != nil && def.ID == row.ID {
			return ErrDefaultProfile
		}
		return tx.Delete(&model.RuleProfile{}, row.ID).Error
	})
}

// SetDefaultProfile makes the profile with slug the default and returns it.
func SetDefaultProfile(ctx context.Context, a *app.App, userID uint64, slug string) (*model.RuleProfile, error) {
	var out model.RuleProfile
	err := withProfiles(ctx, a, userID, func(tx *gorm.DB, rows []model.RuleProfile) error {
		rows, err := ensureDefaultTx(tx, userID, rows)
		if err != nil {
			return err
		}
		row := FindSlug(rows, slug)
		if row == nil {
			return ErrProfileNotFound
		}
		if err := tx.Model(&model.RuleProfile{}).Where("user_id = ? AND id <> ?", userID, row.ID).
			Update("is_default", false).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.RuleProfile{}).Where("id = ?", row.ID).Update("is_default", true).Error; err != nil {
			return err
		}
		return tx.First(&out, row.ID).Error
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}
