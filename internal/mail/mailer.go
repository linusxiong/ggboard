// Package mail sends email through pluggable drivers (smtp, log; memory for tests), renders the built-in
// bilingual templates and delivers asynchronously through Queue. Requests never send mail inline: use
// app.Mailer.SendTemplate / Enqueue.
package mail

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/textproto"
	"sync"
	"time"

	gomail "github.com/wneessen/go-mail"

	"github.com/linusxiong/ggboard/internal/settings"
)

// Message is one email. Template is recorded in mail_logs.
type Message struct {
	To       string `json:"to"`
	Subject  string `json:"subject"`
	HTML     string `json:"html,omitempty"`
	Text     string `json:"text,omitempty"`
	Template string `json:"template,omitempty"`
	Class    Class  `json:"-"` // queue admission class (ClassTransactional unless set)
}

// Class decides how much of the queue a message may use. Mail that anonymous requests can trigger is admitted
// only below a high-water mark, so a flood of it never crowds out order, ticket, leak and reminder mail.
type Class uint8

// Message classes.
const (
	// ClassTransactional (the zero value) may use the whole queue.
	ClassTransactional Class = iota
	// ClassAnonymous (verification codes from POST /passport/email/send-code) is refused with ErrQueueFull once
	// AnonymousSharePercent of the queue is occupied.
	ClassAnonymous
)

// permanentError marks a delivery error that retrying cannot fix.
type permanentError struct{ err error }

func (e permanentError) Error() string { return e.err.Error() }
func (e permanentError) Unwrap() error { return e.err }

// Permanent marks err as permanent: the queue records it once instead of retrying (nil stays nil). Drivers use
// it for errors that depend only on the message or the configuration (unparsable recipient, missing host).
func Permanent(err error) error {
	if err == nil {
		return nil
	}
	return permanentError{err}
}

// IsPermanent reports whether retrying err cannot succeed: errors marked with Permanent, and SMTP refusals
// that are final (RFC 5321 §4.2.1: 5yz replies; go-mail sender/recipient errors). Temporary 4yz replies,
// timeouts, broken connections and every other error are retried by the queue.
func IsPermanent(err error) bool {
	if err == nil {
		return false
	}
	var pe permanentError
	if errors.As(err, &pe) {
		return true
	}
	var se *gomail.SendError
	if errors.As(err, &se) {
		if se.IsTemp() {
			return false
		}
		switch se.Reason {
		case gomail.ErrGetSender, gomail.ErrGetRcpts, gomail.ErrNoUnencoded:
			return true // the message itself is unsendable
		}
		// A SendError carries the SMTP reply code; 0 means the command failed without a reply (the connection
		// broke), which is worth another attempt.
		return isFinalCode(se.ErrorCode())
	}
	var te *textproto.Error // e.g. a 535 AUTH or 554 greeting refusal while dialing
	if errors.As(err, &te) {
		return isFinalCode(te.Code)
	}
	return false
}

func isFinalCode(code int) bool { return code >= 500 && code < 600 }

// Mailer delivers a message synchronously.
type Mailer interface {
	Send(ctx context.Context, msg Message) error
}

// Data is the template data passed by callers.
type Data map[string]any

// LogMailer writes messages to the logger instead of sending them (driver "log").
type LogMailer struct{ Log *slog.Logger }

// Send logs the message (subject at INFO, bodies at DEBUG).
func (m LogMailer) Send(ctx context.Context, msg Message) error {
	log := m.Log
	if log == nil {
		log = slog.Default()
	}
	log.InfoContext(ctx, "mail (log driver)", "to", msg.To, "subject", msg.Subject, "template", msg.Template)
	log.DebugContext(ctx, "mail body", "to", msg.To, "text", msg.Text)
	return nil
}

// Memory records messages in memory (tests). Set Err to make Send fail.
type Memory struct {
	mu   sync.Mutex
	msgs []Message
	Err  error
}

// Send records msg or returns m.Err.
func (m *Memory) Send(_ context.Context, msg Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return m.Err
	}
	m.msgs = append(m.msgs, msg)
	return nil
}

// Messages returns a copy of every delivered message.
func (m *Memory) Messages() []Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]Message(nil), m.msgs...)
}

// Last returns the most recent message.
func (m *Memory) Last() (Message, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.msgs) == 0 {
		return Message{}, false
	}
	return m.msgs[len(m.msgs)-1], true
}

// Reset forgets every message and clears Err.
func (m *Memory) Reset() {
	m.mu.Lock()
	m.msgs, m.Err = nil, nil
	m.mu.Unlock()
}

// SMTPConfig configures the SMTP driver.
type SMTPConfig struct {
	Host        string
	Port        int
	Encryption  string // ssl (implicit TLS) | tls (STARTTLS required) | none
	Username    string
	Password    string
	FromAddress string
	FromName    string
	Timeout     time.Duration
}

// SMTPMailer sends through an SMTP server (one connection per message).
type SMTPMailer struct{ cfg SMTPConfig }

// NewSMTP validates cfg and returns an SMTP mailer.
func NewSMTP(cfg SMTPConfig) (*SMTPMailer, error) {
	if cfg.Host == "" {
		return nil, errors.New("mail: smtp host is not configured")
	}
	if cfg.Port == 0 {
		cfg.Port = 465
	}
	if cfg.FromAddress == "" {
		cfg.FromAddress = cfg.Username
	}
	if cfg.FromAddress == "" {
		return nil, errors.New("mail: from address is not configured")
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 15 * time.Second
	}
	return &SMTPMailer{cfg: cfg}, nil
}

// Send delivers msg.
func (m *SMTPMailer) Send(ctx context.Context, msg Message) error {
	c := m.cfg
	opts := []gomail.Option{gomail.WithPort(c.Port), gomail.WithTimeout(c.Timeout)}
	switch c.Encryption {
	case "ssl":
		opts = append(opts, gomail.WithSSL())
	case "tls":
		opts = append(opts, gomail.WithTLSPolicy(gomail.TLSMandatory))
	default:
		opts = append(opts, gomail.WithTLSPolicy(gomail.NoTLS))
	}
	if c.Username != "" {
		opts = append(opts, gomail.WithSMTPAuth(gomail.SMTPAuthAutoDiscover), gomail.WithUsername(c.Username), gomail.WithPassword(c.Password))
	}
	client, err := gomail.NewClient(c.Host, opts...)
	if err != nil {
		return Permanent(fmt.Errorf("mail: smtp client: %w", err)) // invalid configuration
	}
	gm := gomail.NewMsg()
	if c.FromName != "" {
		err = gm.FromFormat(c.FromName, c.FromAddress)
	} else {
		err = gm.From(c.FromAddress)
	}
	if err != nil {
		return Permanent(fmt.Errorf("mail: from: %w", err))
	}
	if err := gm.To(msg.To); err != nil {
		return Permanent(fmt.Errorf("mail: to: %w", err))
	}
	gm.Subject(msg.Subject)
	gm.SetMessageID()
	gm.SetDate()
	switch {
	case msg.Text != "" && msg.HTML != "":
		gm.SetBodyString(gomail.TypeTextPlain, msg.Text)
		gm.AddAlternativeString(gomail.TypeTextHTML, msg.HTML)
	case msg.HTML != "":
		gm.SetBodyString(gomail.TypeTextHTML, msg.HTML)
	default:
		gm.SetBodyString(gomail.TypeTextPlain, msg.Text)
	}
	if err := client.DialAndSendWithContext(ctx, gm); err != nil {
		return fmt.Errorf("mail: smtp send: %w", err)
	}
	return nil
}

// FromSettings builds the mailer selected by settings.email_driver.
func FromSettings(st *settings.Service, log *slog.Logger) (Mailer, error) {
	switch st.String("email_driver") {
	case "smtp":
		return NewSMTP(SMTPConfig{
			Host:        st.String("email_host"),
			Port:        st.Int("email_port"),
			Encryption:  st.String("email_encryption"),
			Username:    st.String("email_username"),
			Password:    st.String("email_password"),
			FromAddress: st.String("email_from_address"),
			FromName:    st.String("email_from_name"),
		})
	default:
		return LogMailer{Log: log}, nil
	}
}
