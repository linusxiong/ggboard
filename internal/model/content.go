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

// Rule profile defaults: the profile migrated from the one-profile-per-user table (and created for users that
// had none) is "default" / "默认"; a user has at most MaxRuleProfiles.
const (
	RuleProfileDefaultSlug = "default"
	RuleProfileDefaultName = "默认"
	MaxRuleProfiles        = 10
)

// RuleProfile is one of a user's named subscription rule profiles (see internal/subscription.RuleProfile for
// the data schema). A user has at most 10; exactly one is the default (picked when ?profile= names none of
// them). Data is null for a profile that follows the admin's default template. The composite unique index
// (user_id, slug) also serves lookups by user_id.
type RuleProfile struct {
	ID        uint64    `gorm:"primaryKey" json:"id"`
	UserID    uint64    `gorm:"not null;uniqueIndex:idx_rule_profiles_user_slug,priority:1" json:"user_id"`
	Slug      string    `gorm:"size:32;not null;uniqueIndex:idx_rule_profiles_user_slug,priority:2" json:"slug"`
	Name      string    `gorm:"size:64;not null" json:"name"`
	IsDefault bool      `gorm:"not null;default:false" json:"is_default"`
	Data      RawJSON   `json:"data"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// MailTemplate is an administrator's replacement of one built-in e-mail (internal/mail.TemplateNames) for one
// locale (zh-CN or en): a one-line subject and a Markdown body with {{name}} placeholders. No row = the built-in
// template. Rows are validated on write (docs/API.md "Mail templates"); the mail queue loads them all.
type MailTemplate struct {
	ID        uint64    `gorm:"primaryKey" json:"id"`
	Name      string    `gorm:"size:32;not null;uniqueIndex:uniq_mail_templates_name_locale,priority:1" json:"name"`
	Locale    string    `gorm:"size:5;not null;uniqueIndex:uniq_mail_templates_name_locale,priority:2" json:"locale"`
	Subject   string    `gorm:"size:255;not null" json:"subject"`
	Body      string    `gorm:"type:text;not null" json:"body"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
