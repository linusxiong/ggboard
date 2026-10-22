package model

import "time"

// Ticket is a support conversation.
type Ticket struct {
	ID              uint64            `gorm:"primaryKey" json:"id"`
	UserID          uint64            `gorm:"index;not null" json:"user_id"`
	User            *User             `gorm:"foreignKey:UserID" json:"user,omitempty"`
	Subject         string            `gorm:"size:255;not null" json:"subject"`
	Level           TicketLevel       `gorm:"not null;default:0" json:"level"`
	Status          TicketStatus      `gorm:"index;not null;default:0" json:"status"`
	ReplyStatus     TicketReplyStatus `gorm:"index;not null;default:0" json:"reply_status"`
	LastReplyUserID *uint64           `json:"last_reply_user_id"`
	Messages        []TicketMessage   `gorm:"foreignKey:TicketID" json:"messages,omitempty"`
	CreatedAt       time.Time         `json:"created_at"`
	UpdatedAt       time.Time         `json:"updated_at"`
}

// TicketMessage is one message of a ticket.
type TicketMessage struct {
	ID        uint64    `gorm:"primaryKey" json:"id"`
	TicketID  uint64    `gorm:"index;not null" json:"ticket_id"`
	UserID    uint64    `gorm:"not null" json:"user_id"`
	Message   string    `gorm:"type:text;not null" json:"message"`
	CreatedAt time.Time `json:"created_at"`
}

// Notice is an announcement (markdown).
type Notice struct {
	ID        uint64            `gorm:"primaryKey" json:"id"`
	Title     string            `gorm:"size:255;not null" json:"title"`
	Content   string            `gorm:"type:text" json:"content"`
	ImgURL    string            `gorm:"column:img_url;size:255" json:"img_url"`
	Tags      JSONSlice[string] `json:"tags"`
	Show      bool              `gorm:"not null;default:false" json:"show"`
	Sort      int               `gorm:"not null;default:0" json:"sort"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
}

// Knowledge is a help-center article (markdown).
type Knowledge struct {
	ID        uint64    `gorm:"primaryKey" json:"id"`
	Language  string    `gorm:"size:5;index;not null" json:"language"`
	Category  string    `gorm:"size:191;not null" json:"category"`
	Title     string    `gorm:"size:255;not null" json:"title"`
	Body      string    `gorm:"type:text" json:"body"`
	Sort      int       `gorm:"not null;default:0" json:"sort"`
	Show      bool      `gorm:"not null;default:false" json:"show"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName keeps the singular table name from docs/DATA_MODEL.md.
func (Knowledge) TableName() string { return "knowledge" }

// Setting is one key of the settings table; Value is JSON-encoded. Use internal/settings, not this model.
type Setting struct {
	Key       string    `gorm:"primaryKey;size:64" json:"key"`
	Value     string    `gorm:"type:text" json:"value"`
	UpdatedAt time.Time `json:"updated_at"`
}

// MailLog records the outcome of every mail sent by the queue.
type MailLog struct {
	ID        uint64    `gorm:"primaryKey" json:"id"`
	Email     string    `gorm:"size:191;index;not null" json:"email"`
	Subject   string    `gorm:"size:255" json:"subject"`
	Template  string    `gorm:"size:64" json:"template"`
	Error     *string   `gorm:"type:text" json:"error"`
	CreatedAt time.Time `json:"created_at"`
}

// RuleProfile is a user's subscription rule profile (see internal/subscription.RuleProfile for the schema).
type RuleProfile struct {
	UserID    uint64    `gorm:"primaryKey;autoIncrement:false" json:"user_id"`
	Data      RawJSON   `json:"data"`
	UpdatedAt time.Time `json:"updated_at"`
}
