package ticket

import (
	"time"

	"github.com/linusxiong/ggboard/internal/model"
)

// MessageView is a ticket message in API responses.
type MessageView struct {
	ID        uint64    `json:"id"`
	UserID    uint64    `json:"user_id"`
	IsMe      bool      `json:"is_me"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
}

// OwnerView identifies the ticket owner in staff responses.
type OwnerView struct {
	ID    uint64 `json:"id"`
	Email string `json:"email"`
}

// View is the API shape of a ticket (docs/API.md "Ticket"). Messages are present on detail responses only;
// User only in staff responses.
type View struct {
	ID              uint64                  `json:"id"`
	UserID          uint64                  `json:"user_id"`
	Subject         string                  `json:"subject"`
	Level           model.TicketLevel       `json:"level"`
	Status          model.TicketStatus      `json:"status"`
	ReplyStatus     model.TicketReplyStatus `json:"reply_status"`
	LastReplyUserID *uint64                 `json:"last_reply_user_id"`
	CreatedAt       time.Time               `json:"created_at"`
	UpdatedAt       time.Time               `json:"updated_at"`
	User            *OwnerView              `json:"user,omitempty"`
	Messages        []MessageView           `json:"messages,omitempty"`
}

// ForOwner renders t for its owner: is_me marks the owner's own messages.
func ForOwner(t *model.Ticket) View {
	return view(t, func(m *model.TicketMessage) bool { return m.UserID == t.UserID })
}

// ForStaff renders t for staff with the owner's e-mail: is_me marks staff messages (anyone but the owner), as
// in Xboard's admin view.
func ForStaff(t *model.Ticket, email string) View {
	v := view(t, func(m *model.TicketMessage) bool { return m.UserID != t.UserID })
	v.User = &OwnerView{ID: t.UserID, Email: email}
	return v
}

func view(t *model.Ticket, isMe func(*model.TicketMessage) bool) View {
	v := View{
		ID:              t.ID,
		UserID:          t.UserID,
		Subject:         t.Subject,
		Level:           t.Level,
		Status:          t.Status,
		ReplyStatus:     t.ReplyStatus,
		LastReplyUserID: t.LastReplyUserID,
		CreatedAt:       t.CreatedAt,
		UpdatedAt:       t.UpdatedAt,
	}
	if t.Messages != nil {
		v.Messages = make([]MessageView, 0, len(t.Messages))
		for i := range t.Messages {
			m := &t.Messages[i]
			v.Messages = append(v.Messages, MessageView{ID: m.ID, UserID: m.UserID, IsMe: isMe(m), Message: m.Message, CreatedAt: m.CreatedAt})
		}
	}
	return v
}
