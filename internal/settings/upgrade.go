package settings

import (
	"encoding/json"
	"fmt"
	"slices"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/linusxiong/ggboard/internal/model"
)

const (
	ticketUserReplyEvent = "ticket_user_reply"
	// ticketUserReplyUpgrade is a row outside the setting definitions that marks addTicketUserReplyEvent as done.
	ticketUserReplyUpgrade = "upgrade_ticket_user_reply_event"
)

// addTicketUserReplyEvent turns ticket_user_reply on for a site that saved telegram_events before the event existed:
// a saved list replaces the default, so the alert would stay off without any sign. An empty list is left alone, as
// the administrator chose no alerts. It runs once, so turning the event off later sticks.
//
// This belongs here and not in db.Migrate, because the marker would make a freshly migrated database non-empty and
// `db copy` refuses such a target.
func addTicketUserReplyEvent(db *gorm.DB) error {
	err := db.Transaction(func(tx *gorm.DB) error {
		var done, saved []model.Setting
		if err := tx.Where(&model.Setting{Key: ticketUserReplyUpgrade}).Limit(1).Find(&done).Error; err != nil {
			return err
		}
		if len(done) > 0 {
			return nil
		}
		if err := tx.Where(&model.Setting{Key: "telegram_events"}).Limit(1).Find(&saved).Error; err != nil {
			return err
		}
		if len(saved) > 0 {
			var events []string
			if err := json.Unmarshal([]byte(saved[0].Value), &events); err == nil && len(events) > 0 &&
				!slices.Contains(events, ticketUserReplyEvent) {
				value, err := json.Marshal(append(events, ticketUserReplyEvent))
				if err != nil {
					return err
				}
				saved[0].Value = string(value)
				if err := tx.Save(&saved[0]).Error; err != nil {
					return err
				}
			}
		}
		return tx.Clauses(clause.OnConflict{DoNothing: true}).
			Create(&model.Setting{Key: ticketUserReplyUpgrade, Value: "true"}).Error
	})
	if err != nil {
		return fmt.Errorf("settings: add %s: %w", ticketUserReplyEvent, err)
	}
	return nil
}
