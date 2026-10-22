// Package ticket implements support tickets: creation (one open ticket per user, as in Xboard), replies by the
// owner and by staff, closing, filtered listing and the reply e-mail sent to the owner.
package ticket

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/mail"
	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/notify"
)

// Errors.
var (
	ErrNotFound   = errors.New("ticket: not found")
	ErrOpenExists = errors.New("ticket: the user already has an open ticket")
	ErrClosed     = errors.New("ticket: ticket is closed")
)

// NotifyThrottle is the minimum delay between two reply e-mails for the same ticket. A reply by the owner resets
// it, so every staff answer to a new user message is notified while bursts of staff messages send one e-mail.
const NotifyThrottle = 30 * time.Minute

// Limits on user input (characters).
const (
	MaxSubjectLen = 255
	MaxMessageLen = 10000
)

// Service is cheap to construct: ticket.New(a) per request is fine.
type Service struct{ a *app.App }

// New returns a Service bound to a.
func New(a *app.App) *Service { return &Service{a: a} }

// Create opens a ticket with its first message for userID. It returns ErrOpenExists when the user still has an
// open ticket (Xboard rule: one open ticket per user). The new ticket awaits a staff reply; administrators are
// notified (ticket_new).
func (s *Service) Create(ctx context.Context, userID uint64, subject string, level model.TicketLevel, message string) (*model.Ticket, error) {
	t, email, err := s.create(ctx, userID, subject, level, message)
	if err != nil {
		return nil, err
	}
	notify.Emit(s.a, notify.TicketNew(s.a, notify.Ticket{
		ID: t.ID, UserID: userID, Email: email, Subject: subject, Level: int(level), Message: message,
	}))
	return t, nil
}

// CreateSystem is Create for tickets the panel opens on the user's behalf (withdrawal requests): the caller
// sends its own notification, so no ticket_new is emitted.
func (s *Service) CreateSystem(ctx context.Context, userID uint64, subject string, level model.TicketLevel, message string) (*model.Ticket, error) {
	t, _, err := s.create(ctx, userID, subject, level, message)
	return t, err
}

func (s *Service) create(ctx context.Context, userID uint64, subject string, level model.TicketLevel, message string) (*model.Ticket, string, error) {
	t := &model.Ticket{
		UserID:          userID,
		Subject:         subject,
		Level:           level,
		Status:          model.TicketStatusOpen,
		ReplyStatus:     model.TicketAwaitingStaff,
		LastReplyUserID: &userID,
	}
	var owner model.User
	err := s.a.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Serialize ticket creation per user: FOR UPDATE on PostgreSQL/MySQL; SQLite transactions are IMMEDIATE.
		if err := tx.Clauses(clause.Locking{Strength: clause.LockingStrengthUpdate}).
			Select("id", "email").Where("id = ?", userID).First(&owner).Error; err != nil {
			return err
		}
		var open int64
		if err := tx.Model(&model.Ticket{}).
			Where(map[string]any{"user_id": userID, "status": model.TicketStatusOpen}).
			Count(&open).Error; err != nil {
			return err
		}
		if open > 0 {
			return ErrOpenExists
		}
		if err := tx.Create(t).Error; err != nil {
			return err
		}
		m := model.TicketMessage{TicketID: t.ID, UserID: userID, Message: message}
		if err := tx.Create(&m).Error; err != nil {
			return err
		}
		t.Messages = []model.TicketMessage{m}
		return nil
	})
	if err != nil {
		return nil, "", wrap(err)
	}
	return t, owner.Email, nil
}

// Get loads a ticket with its messages (oldest first). ownerID, when non-zero, restricts the lookup to tickets
// of that user (ErrNotFound otherwise).
func (s *Service) Get(ctx context.Context, id, ownerID uint64) (*model.Ticket, error) {
	return s.get(s.a.DB.WithContext(ctx), id, ownerID)
}

func (s *Service) get(db *gorm.DB, id, ownerID uint64) (*model.Ticket, error) {
	q := db.Preload("Messages", func(db *gorm.DB) *gorm.DB { return db.Order("id ASC") }).Where("id = ?", id)
	if ownerID != 0 {
		q = q.Where("user_id = ?", ownerID)
	}
	var t model.Ticket
	if err := q.First(&t).Error; err != nil {
		return nil, wrap(err)
	}
	return &t, nil
}

// ReplyAsOwner appends a message by the ticket owner. The ticket then awaits a staff reply. Closed tickets
// cannot be replied to (ErrClosed); tickets of other users are ErrNotFound.
func (s *Service) ReplyAsOwner(ctx context.Context, id, userID uint64, message string) (*model.Ticket, error) {
	err := s.a.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		t, err := s.lock(tx, id)
		if err != nil {
			return err
		}
		if t.UserID != userID {
			return ErrNotFound
		}
		if t.Status == model.TicketStatusClosed {
			return ErrClosed
		}
		return addMessage(tx, t, userID, message, map[string]any{
			"reply_status":       model.TicketAwaitingStaff,
			"last_reply_user_id": userID,
		})
	})
	if err != nil {
		return nil, wrap(err)
	}
	s.a.KV.Delete(notifyKey(id))
	return s.Get(ctx, id, 0)
}

// ReplyAsStaff appends a staff message: the ticket is (re)opened and marked replied, and the owner is e-mailed
// (see NotifyThrottle) and notified (ticket_reply, every reply). A staff member answering their own ticket
// counts as an owner reply.
func (s *Service) ReplyAsStaff(ctx context.Context, id, staffID uint64, message string) (*model.Ticket, error) {
	var t *model.Ticket
	err := s.a.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		if t, err = s.lock(tx, id); err != nil {
			return err
		}
		reply := model.TicketReplied
		if staffID == t.UserID {
			reply = model.TicketAwaitingStaff
		}
		return addMessage(tx, t, staffID, message, map[string]any{
			"status":             model.TicketStatusOpen,
			"reply_status":       reply,
			"last_reply_user_id": staffID,
		})
	})
	if err != nil {
		return nil, wrap(err)
	}
	if staffID != t.UserID {
		s.notify(ctx, t, message)
		notify.Emit(s.a, notify.TicketReply(s.a, notify.Ticket{
			ID: t.ID, UserID: t.UserID, Subject: t.Subject, Level: int(t.Level), Message: message,
		}))
	}
	return s.Get(ctx, id, 0)
}

// Close closes a ticket (idempotent). ownerID, when non-zero, restricts it to that user's tickets.
func (s *Service) Close(ctx context.Context, id, ownerID uint64) error {
	q := s.a.DB.WithContext(ctx).Model(&model.Ticket{}).Where("id = ?", id)
	if ownerID != 0 {
		q = q.Where("user_id = ?", ownerID)
	}
	var n int64
	if err := q.Session(&gorm.Session{}).Count(&n).Error; err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return q.Where(map[string]any{"status": model.TicketStatusOpen}).
		Update("status", model.TicketStatusClosed).Error
}

// CountOpen returns the number of open tickets of userID.
func (s *Service) CountOpen(ctx context.Context, userID uint64) (int64, error) {
	var n int64
	err := s.a.DB.WithContext(ctx).Model(&model.Ticket{}).
		Where(map[string]any{"user_id": userID, "status": model.TicketStatusOpen}).Count(&n).Error
	return n, err
}

// Filter narrows List queries; nil / empty fields are ignored.
type Filter struct {
	UserID      *uint64
	Email       string // exact (normalized) owner e-mail
	Status      *model.TicketStatus
	ReplyStatus *model.TicketReplyStatus
}

// Query returns the tickets matching f (no order, no limit) for httpx.Paginate.
func (s *Service) Query(ctx context.Context, f Filter) *gorm.DB {
	q := s.a.DB.WithContext(ctx).Model(&model.Ticket{})
	if f.UserID != nil {
		q = q.Where("user_id = ?", *f.UserID)
	}
	if email := strings.ToLower(strings.TrimSpace(f.Email)); email != "" {
		q = q.Where("user_id IN (?)", s.a.DB.WithContext(ctx).Model(&model.User{}).Select("id").Where("email = ?", email))
	}
	if f.Status != nil {
		q = q.Where(map[string]any{"status": *f.Status})
	}
	if f.ReplyStatus != nil {
		q = q.Where(map[string]any{"reply_status": *f.ReplyStatus})
	}
	return q
}

// Owners returns id → e-mail for the owners of tickets (one query).
func (s *Service) Owners(ctx context.Context, tickets []model.Ticket) (map[uint64]string, error) {
	out := make(map[uint64]string, len(tickets))
	if len(tickets) == 0 {
		return out, nil
	}
	ids := make([]uint64, 0, len(tickets))
	seen := make(map[uint64]bool, len(tickets))
	for _, t := range tickets {
		if !seen[t.UserID] {
			seen[t.UserID] = true
			ids = append(ids, t.UserID)
		}
	}
	var rows []struct {
		ID    uint64
		Email string
	}
	if err := s.a.DB.WithContext(ctx).Model(&model.User{}).Select("id", "email").Where("id IN ?", ids).Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[r.ID] = r.Email
	}
	return out, nil
}

// lock loads the ticket row FOR UPDATE (PostgreSQL/MySQL) inside tx.
func (s *Service) lock(tx *gorm.DB, id uint64) (*model.Ticket, error) {
	var t model.Ticket
	if err := tx.Clauses(clause.Locking{Strength: clause.LockingStrengthUpdate}).Where("id = ?", id).First(&t).Error; err != nil {
		return nil, err
	}
	return &t, nil
}

func addMessage(tx *gorm.DB, t *model.Ticket, userID uint64, message string, updates map[string]any) error {
	if err := tx.Create(&model.TicketMessage{TicketID: t.ID, UserID: userID, Message: message}).Error; err != nil {
		return err
	}
	return tx.Model(&model.Ticket{}).Where("id = ?", t.ID).Updates(updates).Error
}

func notifyKey(id uint64) string { return "ticket:notified:" + strconv.FormatUint(id, 10) }

// notify e-mails the owner about a staff reply unless one was sent for this ticket within NotifyThrottle.
func (s *Service) notify(ctx context.Context, t *model.Ticket, message string) {
	send := false
	s.a.KV.Update(notifyKey(t.ID), NotifyThrottle, true, func(_ any, ok bool) any {
		send = !ok
		return true
	})
	if !send {
		return
	}
	var owner model.User
	if err := s.a.DB.WithContext(ctx).Select("id", "email").Where("id = ?", t.UserID).First(&owner).Error; err != nil {
		s.a.Log.Warn("ticket: reply notification skipped", "ticket_id", t.ID, "err", err)
		return
	}
	url := ""
	if base := strings.TrimRight(s.a.Settings.String("app_url"), "/"); base != "" {
		url = fmt.Sprintf("%s/tickets/%d", base, t.ID)
	}
	if err := s.a.Mailer.SendTemplate(owner.Email, mail.TplTicketReply, mail.Data{
		"Subject": t.Subject,
		"Message": message,
		"URL":     url,
	}); err != nil {
		s.a.Log.Warn("ticket: reply notification not queued", "ticket_id", t.ID, "err", err)
	}
}

func wrap(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	return err
}
