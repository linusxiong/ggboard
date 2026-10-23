package adminuser

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/linusxiong/ggboard/internal/auth"
	"github.com/linusxiong/ggboard/internal/httpx"
	"github.com/linusxiong/ggboard/internal/mail"
	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/service/maintenance"
	usersvc "github.com/linusxiong/ggboard/internal/service/user"
)

// ---------------------------------------------------------------- generate

// GenerateInput is POST /admin/users/generate.
type GenerateInput struct {
	EmailPrefix string     `json:"email_prefix" binding:"max=64"`
	EmailSuffix string     `json:"email_suffix" binding:"required,max=120"`
	Count       int        `json:"count" binding:"required,gte=1,lte=500"`
	PlanID      *uint64    `json:"plan_id"`
	ExpiredAt   *time.Time `json:"expired_at"`
	Password    string     `json:"password" binding:"omitempty,min=8,max=128"`
}

// GenerateResult is the response of POST /admin/users/generate: count users and a CSV "email,password".
type GenerateResult struct {
	Count int    `json:"count"`
	CSV   string `json:"csv"`
}

const (
	randomLocalLen    = 8
	randomPasswordLen = 12
	lowerAlnum        = "abcdefghijklmnopqrstuvwxyz0123456789"
	passwordAlphabet  = "abcdefghijkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"
)

// randomString returns n characters of alphabet chosen with crypto/rand (rejection sampling, no bias).
func randomString(n int, alphabet string) string {
	limit := 256 - 256%len(alphabet)
	out := make([]byte, 0, n)
	buf := make([]byte, n*2)
	for len(out) < n {
		if _, err := rand.Read(buf); err != nil {
			panic("adminuser: crypto/rand failed: " + err.Error())
		}
		for _, c := range buf {
			if int(c) >= limit {
				continue
			}
			out = append(out, alphabet[int(c)%len(alphabet)])
			if len(out) == n {
				break
			}
		}
	}
	return string(out)
}

// Generate creates in.Count users named <prefix><8 random chars>@<suffix> (exactly <prefix>@<suffix> when
// count is 1 and a prefix is given). All users share one password: in.Password, or a random 12-character
// password when omitted (hashed once: argon2id per user would take seconds for a batch). A plan grants its
// group, traffic and limits; next_reset_at follows the plan and expired_at.
func (s *Service) Generate(ctx context.Context, in GenerateInput, now time.Time) (*GenerateResult, error) {
	prefix := strings.ToLower(strings.TrimSpace(in.EmailPrefix))
	suffix := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(in.EmailSuffix), "@"))
	if err := httpx.Validate(&struct {
		Host string `json:"email_suffix" binding:"required,hostname_rfc1123"`
	}{suffix}); err != nil {
		return nil, err
	}
	if prefix != "" {
		if err := httpx.Validate(&struct {
			Email string `json:"email_prefix" binding:"email"`
		}{prefix + "@" + suffix}); err != nil {
			return nil, err
		}
	}
	db := s.a.DB.WithContext(ctx)
	var plan *model.Plan
	if id := nonZero(in.PlanID); id != nil {
		p, err := loadPlan(db, *id, "plan_id")
		if err != nil {
			return nil, err
		}
		plan = p
	}

	emails, err := s.uniqueEmails(db, prefix, suffix, in.Count)
	if err != nil {
		return nil, err
	}
	password := in.Password
	if password == "" {
		password = randomString(randomPasswordLen, passwordAlphabet)
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return nil, err
	}

	svc := usersvc.New(s.a)
	tmpl := model.User{
		PasswordHash:  hash,
		ExpiredAt:     in.ExpiredAt,
		RemindExpire:  s.a.Settings.Bool("default_remind_expire"),
		RemindTraffic: s.a.Settings.Bool("default_remind_traffic"),
		// Admin-created: a later password reset is not an ownership change (usersvc.ReclaimAccount).
		EmailVerifiedAt: &now,
	}
	if plan != nil {
		gid := plan.GroupID
		tmpl.PlanID, tmpl.GroupID = &plan.ID, &gid
		tmpl.TransferEnable = plan.TransferBytes()
		tmpl.SpeedLimit, tmpl.DeviceLimit = plan.SpeedLimit, plan.DeviceLimit
		tmpl.NextResetAt = svc.ComputeNextResetAt(&tmpl, plan, now)
	}
	users := make([]model.User, len(emails))
	for i, e := range emails {
		u := tmpl
		u.Email, u.UUID, u.Token = e, uuid.NewString(), usersvc.NewToken()
		users[i] = u
	}
	err = db.Transaction(func(tx *gorm.DB) error { return tx.CreateInBatches(&users, 100).Error })
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return nil, httpx.Conflict("an email in the batch is already registered").WithField("email_prefix", "is already registered")
	}
	if err != nil {
		return nil, fmt.Errorf("adminuser: generate: %w", err)
	}
	svc.InvalidateNodeUsers()

	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	_ = w.Write([]string{"email", "password"})
	for _, e := range emails {
		_ = w.Write([]string{csvSafe(e), password}) // the password is shown verbatim
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, err
	}
	s.a.Log.Info("admin generated users", "count", len(users), "suffix", suffix)
	return &GenerateResult{Count: len(users), CSV: buf.String()}, nil
}

// uniqueEmails returns n distinct addresses that are not registered yet.
func (s *Service) uniqueEmails(db *gorm.DB, prefix, suffix string, n int) ([]string, error) {
	if prefix != "" && n == 1 {
		e := prefix + "@" + suffix
		var taken int64
		if err := db.Model(&model.User{}).Where("email = ?", e).Count(&taken).Error; err != nil {
			return nil, err
		}
		if taken > 0 {
			return nil, httpx.Conflict("email already registered").WithField("email_prefix", "is already registered")
		}
		return []string{e}, nil
	}
	seen := make(map[string]struct{}, n)
	out := make([]string, 0, n)
	for attempt := 0; len(out) < n; attempt++ {
		if attempt >= 10 {
			return nil, httpx.Conflict("could not generate unique email addresses")
		}
		cand := make([]string, 0, n-len(out))
		for len(cand) < n-len(out) {
			e := prefix + randomString(randomLocalLen, lowerAlnum) + "@" + suffix
			if _, dup := seen[e]; !dup {
				seen[e] = struct{}{}
				cand = append(cand, e)
			}
		}
		var taken []string
		if err := db.Model(&model.User{}).Where("email IN ?", cand).Pluck("email", &taken).Error; err != nil {
			return nil, err
		}
		takenSet := make(map[string]struct{}, len(taken))
		for _, t := range taken {
			takenSet[t] = struct{}{}
		}
		for _, e := range cand {
			if _, t := takenSet[e]; !t {
				out = append(out, e)
			}
		}
	}
	return out, nil
}

// csvSafe neutralizes spreadsheet formula injection (cells starting with = + - @ tab or CR).
func csvSafe(v string) string {
	if v != "" && strings.ContainsRune("=+-@\t\r", rune(v[0])) {
		return "'" + v
	}
	return v
}

// ---------------------------------------------------------------- export

// ExportHeader is the CSV header of POST /admin/users/export.
var ExportHeader = []string{"email", "plan", "expired_at", "transfer_enable", "u", "d", "balance", "created_at"}

// Export writes every user matching f as CSV (ExportHeader; times RFC3339, traffic in bytes, balance in cents)
// to w, reading the table in batches.
func (s *Service) Export(ctx context.Context, w io.Writer, f Filters, now time.Time) error {
	db := s.a.DB.WithContext(ctx)
	var plans []model.Plan
	if err := db.Select("id", "name").Find(&plans).Error; err != nil {
		return fmt.Errorf("adminuser: export plans: %w", err)
	}
	names := make(map[uint64]string, len(plans))
	for _, p := range plans {
		names[p.ID] = p.Name
	}
	cw := csv.NewWriter(w)
	if err := cw.Write(ExportHeader); err != nil {
		return err
	}
	var batch []model.User
	q := f.Apply(db.Model(&model.User{}), now).
		Select("id", "email", "plan_id", "expired_at", "transfer_enable", "u", "d", "balance", "created_at")
	res := q.FindInBatches(&batch, 500, func(_ *gorm.DB, _ int) error {
		for i := range batch {
			u := &batch[i]
			plan, expired := "", ""
			if u.PlanID != nil {
				plan = names[*u.PlanID]
			}
			if u.ExpiredAt != nil {
				expired = u.ExpiredAt.UTC().Format(time.RFC3339)
			}
			if err := cw.Write([]string{
				csvSafe(u.Email), csvSafe(plan), expired,
				strconv.FormatInt(u.TransferEnable, 10), strconv.FormatInt(u.U, 10), strconv.FormatInt(u.D, 10),
				strconv.FormatInt(u.Balance, 10), u.CreatedAt.UTC().Format(time.RFC3339),
			}); err != nil {
				return err
			}
		}
		cw.Flush()
		return cw.Error()
	})
	if res.Error != nil {
		return fmt.Errorf("adminuser: export: %w", res.Error)
	}
	cw.Flush()
	return cw.Error()
}

// ---------------------------------------------------------------- bulk mail

// MailInput is POST /admin/users/mail: the list filters plus the message (plain text; blank lines separate
// paragraphs). The broadcast ignores remind_* preferences.
type MailInput struct {
	Filters
	Subject string `json:"subject" binding:"required,max=255"`
	Content string `json:"content" binding:"required,max=65535"`
}

// Mail queues the notify template to every user matching in.Filters and returns how many were queued.
// What does not fit into the mail queue right away is handed to a background sender that waits for room.
func (s *Service) Mail(ctx context.Context, in MailInput, now time.Time) (int, error) {
	var emails []string
	if err := in.Apply(s.a.DB.WithContext(ctx).Model(&model.User{}), now).Order("users.id").
		Pluck("users.email", &emails).Error; err != nil {
		return 0, fmt.Errorf("adminuser: mail recipients: %w", err)
	}
	if len(emails) == 0 {
		return 0, nil
	}
	msg, err := s.a.Mailer.Render(emails[0], mail.TplNotify, mail.Data{"Subject": in.Subject, "Content": in.Content})
	if err != nil {
		return 0, err
	}
	for i, e := range emails {
		m := msg
		m.To = e
		err := s.a.Mailer.Enqueue(m)
		if errors.Is(err, mail.ErrQueueFull) {
			go s.mailRest(context.WithoutCancel(ctx), msg, emails[i:]) // outlives the request on purpose
			break
		}
		if err != nil {
			return i, err
		}
	}
	s.a.Log.Info("admin bulk mail queued", "recipients", len(emails), "subject", in.Subject)
	return len(emails), nil
}

// mailRest queues the remaining recipients, waiting for room in the mail queue (stops when it closes).
func (s *Service) mailRest(ctx context.Context, msg mail.Message, emails []string) {
	for i, e := range emails {
		m := msg
		m.To = e
		if err := maintenance.Enqueue(ctx, s.a.Mailer, m); err != nil {
			s.a.Log.Warn("admin bulk mail stopped", "unsent", len(emails)-i, "err", err)
			return
		}
	}
}
