package mail

import (
	"context"
	"errors"
	"html/template"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/linusxiong/ggboard/internal/config"
	"github.com/linusxiong/ggboard/internal/db"
	"github.com/linusxiong/ggboard/internal/logx"
	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/settings"
)

func TestRenderAllTemplatesBothLocales(t *testing.T) {
	r, err := NewRenderer()
	if err != nil {
		t.Fatal(err)
	}
	data := map[string]Data{
		TplVerifyCode:    {"Code": "482913"},
		TplRemindExpire:  {"ExpiredAt": "2026-11-01"},
		TplRemindTraffic: {"Percent": 85, "Used": "85 GB", "Total": "100 GB"},
		TplOrderPaid:     {"TradeNo": "20261021000000abc", "PlanName": "Pro", "Period": "monthly", "Amount": "¥10.00"},
		TplTicketReply:   {"Subject": "Help", "Message": "Hello <script>alert(1)</script>", "URL": "https://panel.example.com/tickets/1"},
		TplNotify:        {"Subject": "Maintenance", "Content": "Line one.\n\nLine two."},
	}
	for _, locale := range []string{"zh-CN", "en"} {
		for _, name := range TemplateNames {
			msg, err := r.Render(name, Brand{AppName: "Example Panel", AppURL: "https://panel.example.com", Locale: locale}, data[name])
			if err != nil {
				t.Fatalf("%s/%s: %v", locale, name, err)
			}
			if msg.Subject == "" || !strings.Contains(msg.HTML, "Example Panel") || !strings.Contains(msg.Text, "Example Panel") {
				t.Fatalf("%s/%s: incomplete message %+v", locale, name, msg)
			}
			for _, bad := range []string{"<no value>", "%!"} {
				if strings.Contains(msg.HTML, bad) || strings.Contains(msg.Text, bad) || strings.Contains(msg.Subject, bad) {
					t.Fatalf("%s/%s contains %q:\n%s\n%s", locale, name, bad, msg.HTML, msg.Text)
				}
			}
			if msg.Template != name {
				t.Fatal("template name not recorded")
			}
		}
	}
	msg, _ := r.Render(TplVerifyCode, Brand{AppName: "X", Locale: "zh-CN"}, Data{"Code": "123456"})
	if !strings.Contains(msg.HTML, "123456") || !strings.Contains(msg.Text, "123456") || !strings.Contains(msg.Subject, "验证码") {
		t.Fatalf("verify code = %+v", msg)
	}
	msg, _ = r.Render(TplTicketReply, Brand{AppName: "X", Locale: "en"}, data[TplTicketReply])
	if strings.Contains(msg.HTML, "<script>") || !strings.Contains(msg.HTML, "&lt;script&gt;") {
		t.Fatal("user content must be escaped in HTML")
	}
	msg, _ = r.Render(TplNotify, Brand{AppName: "X", Locale: "en"}, Data{"Subject": "S", "ContentHTML": template.HTML("<b>bold</b>"), "Content": "bold"})
	if !strings.Contains(msg.HTML, "<b>bold</b>") || msg.Subject != "S" {
		t.Fatalf("notify html = %s", msg.Subject)
	}
	msg, _ = r.Render(TplNotify, Brand{AppName: "X", Locale: "en"}, Data{"Subject": "S", "Content": "p1\n\np2"})
	if strings.Count(msg.HTML, "<p style") < 2 {
		t.Fatal("notify paragraphs")
	}
	msg, _ = r.Render(TplRemindTraffic, Brand{AppName: "X", Locale: "en"}, data[TplRemindTraffic])
	if !strings.Contains(msg.Text, "85% of your data") {
		t.Fatalf("percent formatting: %s", msg.Text)
	}
	if _, err := r.Render("nope", Brand{}, nil); err == nil {
		t.Fatal("unknown template must fail")
	}
	if NormalizeLocale("zh-TW") != "zh-CN" || NormalizeLocale("fr") != "en" {
		t.Fatal("NormalizeLocale")
	}
}

func setup(t *testing.T) (*gorm.DB, *settings.Service) {
	t.Helper()
	d, err := db.Open(config.DatabaseConfig{Driver: config.DriverSQLite, DSN: filepath.Join(t.TempDir(), "m.db")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close(d) })
	if err := db.Migrate(d); err != nil {
		t.Fatal(err)
	}
	st, err := settings.New(d)
	if err != nil {
		t.Fatal(err)
	}
	return d, st
}

type flaky struct {
	fails atomic.Int32
	Memory
}

func (f *flaky) Send(ctx context.Context, m Message) error {
	if f.fails.Add(-1) >= 0 {
		return errors.New("temporary failure")
	}
	return f.Memory.Send(ctx, m)
}

func TestQueueDeliveryRetryAndLogs(t *testing.T) {
	d, st := setup(t)
	q, err := NewQueue(d, st, logx.Discard(), QueueOptions{Retries: 2, Backoff: func(int) time.Duration { return time.Millisecond }})
	if err != nil {
		t.Fatal(err)
	}
	f := &flaky{}
	f.fails.Store(2) // two failures, then success on the 3rd attempt
	q.SetMailer(f)
	q.Start()

	if err := q.SendTemplate("a@example.com", TplVerifyCode, Data{"Code": "111111"}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := q.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	if m, ok := f.Last(); !ok || m.To != "a@example.com" || !strings.Contains(m.Text, "111111") {
		t.Fatalf("delivered = %+v", m)
	}

	f.fails.Store(10) // exhaust retries
	if err := q.Enqueue(Message{To: "b@example.com", Subject: "s", Text: "t", Template: "custom"}); err != nil {
		t.Fatal(err)
	}
	if err := q.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	st2 := q.Stats()
	if st2.Sent != 1 || st2.Failed != 1 || st2.Pending != 0 {
		t.Fatalf("stats = %+v", st2)
	}
	var logs []model.MailLog
	d.Order("id").Find(&logs)
	if len(logs) != 2 || logs[0].Error != nil || logs[1].Error == nil || logs[0].Template != TplVerifyCode || logs[1].Email != "b@example.com" {
		t.Fatalf("mail_logs = %+v", logs)
	}

	if err := q.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	if err := q.Enqueue(Message{To: "c@example.com"}); !errors.Is(err, ErrQueueClosed) {
		t.Fatalf("enqueue after stop = %v", err)
	}
	if err := q.Enqueue(Message{}); err == nil {
		t.Fatal("empty recipient must fail")
	}
}

func TestQueueFullAndSendNow(t *testing.T) {
	d, st := setup(t)
	q, err := NewQueue(d, st, logx.Discard(), QueueOptions{Size: 1})
	if err != nil {
		t.Fatal(err)
	}
	mem := &Memory{}
	q.SetMailer(mem)
	// Not started: the second message does not fit.
	if err := q.Enqueue(Message{To: "a@example.com"}); err != nil {
		t.Fatal(err)
	}
	if err := q.Enqueue(Message{To: "b@example.com"}); !errors.Is(err, ErrQueueFull) {
		t.Fatalf("expected ErrQueueFull, got %v", err)
	}
	if err := q.SendNow(context.Background(), Message{To: "now@example.com", Subject: "test"}); err != nil {
		t.Fatal(err)
	}
	if m, _ := mem.Last(); m.To != "now@example.com" {
		t.Fatal("SendNow did not deliver")
	}
	mem.Err = errors.New("down")
	if err := q.SendNow(context.Background(), Message{To: "x@example.com"}); err == nil {
		t.Fatal("SendNow must return driver errors")
	}
	mem.Reset()
	q.Start()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := q.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	if m, _ := mem.Last(); m.To != "a@example.com" {
		t.Fatal("Stop must drain queued messages")
	}
}

func TestDriverSelection(t *testing.T) {
	_, st := setup(t)
	m, err := FromSettings(st, logx.Discard())
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := m.(LogMailer); !ok {
		t.Fatalf("default driver = %T", m)
	}
	if err := m.Send(context.Background(), Message{To: "a@example.com", Subject: "s"}); err != nil {
		t.Fatal(err)
	}
	if err := st.Set(map[string]any{"email_driver": "smtp"}); err != nil {
		t.Fatal(err)
	}
	if _, err := FromSettings(st, logx.Discard()); err == nil {
		t.Fatal("smtp without host must fail")
	}
	if err := st.Set(map[string]any{"email_host": "smtp.example.com", "email_username": "bot@example.com", "email_encryption": "tls"}); err != nil {
		t.Fatal(err)
	}
	m, err = FromSettings(st, logx.Discard())
	if err != nil {
		t.Fatal(err)
	}
	s, ok := m.(*SMTPMailer)
	if !ok || s.cfg.FromAddress != "bot@example.com" || s.cfg.Port != 465 {
		t.Fatalf("smtp mailer = %#v", m)
	}
}
