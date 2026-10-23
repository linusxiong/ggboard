package db

import (
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/linusxiong/ggboard/internal/model"
)

// Schema upgrades that AutoMigrate cannot do on its own run around it in Migrate: before* steps move tables
// whose primary key changed out of the way, after* steps copy their rows into the new table. Every step is
// idempotent and portable (GORM migrator calls and plain inserts only), so an interrupted upgrade finishes on
// the next start.

// LegacyRuleProfilesTable holds the rows of the one-profile-per-user rule_profiles table (user_id primary
// key) while they are copied into the multi-profile table; it is dropped afterwards.
const LegacyRuleProfilesTable = "rule_profiles_v1"

// legacyRuleProfile is a row of the v1 rule_profiles table.
type legacyRuleProfile struct {
	UserID    uint64 `gorm:"primaryKey;autoIncrement:false"`
	Data      model.RawJSON
	UpdatedAt time.Time
}

// beforeAutoMigrate renames the v1 rule_profiles table (no id column) so AutoMigrate creates the new one,
// and adds users.email_verified_at with its backfill (addEmailVerifiedAt).
func beforeAutoMigrate(db *gorm.DB) error {
	if err := addEmailVerifiedAt(db); err != nil {
		return err
	}
	m := db.Migrator()
	if !m.HasTable(&model.RuleProfile{}) || m.HasColumn(&model.RuleProfile{}, "id") {
		return nil
	}
	if m.HasTable(LegacyRuleProfilesTable) {
		return fmt.Errorf("db: migrate: both rule_profiles (v1) and %s exist; drop the stale copy after checking it", LegacyRuleProfilesTable)
	}
	if err := m.RenameTable("rule_profiles", LegacyRuleProfilesTable); err != nil {
		return fmt.Errorf("db: migrate: rename v1 rule_profiles: %w", err)
	}
	return nil
}

// afterAutoMigrate copies the v1 rule profiles into the new table as each user's default profile (slug
// "default", name "默认"); users that already have a "default" profile keep it. The v1 table is dropped once
// every row is copied.
func afterAutoMigrate(db *gorm.DB) error {
	m := db.Migrator()
	if !m.HasTable(LegacyRuleProfilesTable) {
		return nil
	}
	var batch []legacyRuleProfile
	res := db.Table(LegacyRuleProfilesTable).FindInBatches(&batch, 500, func(tx *gorm.DB, _ int) error {
		rows := make([]model.RuleProfile, 0, len(batch))
		for _, r := range batch {
			ts := r.UpdatedAt
			if ts.IsZero() {
				ts = time.Now()
			}
			rows = append(rows, model.RuleProfile{
				UserID: r.UserID, Slug: model.RuleProfileDefaultSlug, Name: model.RuleProfileDefaultName, IsDefault: true,
				Data: r.Data, CreatedAt: ts, UpdatedAt: ts,
			})
		}
		if len(rows) == 0 {
			return nil
		}
		return db.Clauses(clause.OnConflict{DoNothing: true}).Create(&rows).Error
	})
	if res.Error != nil && !errors.Is(res.Error, gorm.ErrRecordNotFound) {
		return fmt.Errorf("db: migrate: copy v1 rule_profiles: %w", res.Error)
	}
	if err := m.DropTable(LegacyRuleProfilesTable); err != nil {
		return fmt.Errorf("db: migrate: drop %s: %w", LegacyRuleProfilesTable, err)
	}
	return nil
}

// addEmailVerifiedAt adds users.email_verified_at to an existing users table and stamps every existing row with
// its created_at. Accounts created before the column existed cannot tell whether their address was verified;
// treating them as unverified would make each one's first password reset delete its passkeys, TOTP and Telegram
// binding and rotate its subscription (usersvc.ReclaimAccount), so they are trusted instead. The backfill runs
// only in the upgrade that adds the column (later NULLs are real "never verified" rows), in one transaction with
// the ALTER where the database allows it (SQLite, PostgreSQL; MySQL commits DDL implicitly).
func addEmailVerifiedAt(db *gorm.DB) error {
	m := db.Migrator()
	if !m.HasTable(&model.User{}) || m.HasColumn(&model.User{}, "email_verified_at") {
		return nil
	}
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Migrator().AddColumn(&model.User{}, "EmailVerifiedAt"); err != nil {
			return err
		}
		return tx.Model(&model.User{}).Where("email_verified_at IS NULL").
			UpdateColumn("email_verified_at", gorm.Expr("created_at")).Error
	})
	if err != nil {
		return fmt.Errorf("db: migrate: add users.email_verified_at: %w", err)
	}
	return nil
}
