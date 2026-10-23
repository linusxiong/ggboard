package mail

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"net/textproto"
	"strings"
	"sync"
	"testing"
	"time"

	gomail "github.com/wneessen/go-mail"

	"github.com/linusxiong/ggboard/internal/logx"
	"github.com/linusxiong/ggboard/internal/model"
)

// gate is a mailer whose first Send signals entered and then blocks until release is closed.
type gate struct {
	entered chan struct{}
	release chan struct{}
	once    sync.Once
	opened  sync.Once
	Memory
}

// open releases every blocked and future Send (idempotent).
func (g *gate) open() { g.opened.Do(func() { close(g.release) }) }

func newGate() *gate { return &gate{entered: make(chan struct{}), release: make(chan struct{})} }

func (g *gate) Send(ctx context.Context, m Message) error {
	g.once.Do(func() { close(g.entered) })
	select {
	case <-g.release:
	case <-ctx.Done():
		return ctx.Err()
	}
	return g.Memory.Send(ctx, m)
}

// Audit 2026-10-22 (send-code): anonymous verification codes could fill the single 1024-slot queue that order,
// ticket and leak mail share. ClassAnonymous messages are now refused at AnonymousSharePercent occupancy.
func TestAuditAnonymousHighWater(t *testing.T) {
	d, st := setup(t)
	q, err := NewQueue(d, st, logx.Discard(), QueueOptions{Size: 4})
	if err != nil {
		t.Fatal(err)
	}
	g := newGate()
	q.SetMailer(g)
	q.Start()
	t.Cleanup(func() { g.open(); _ = q.Stop(context.Background()) }) // a failed assertion must not hang here

	if err := q.Enqueue(Message{To: "held@example.com", Subject: "s"}); err != nil {
		t.Fatal(err)
	}
	<-g.entered // the worker holds the first message: the channel is empty again
	anon := func(i int) Message {
		return Message{To: fmt.Sprintf("anon%d@example.com", i), Subject: "code", Template: TplVerifyCode, Class: ClassAnonymous}
	}
	for i := range 3 { // 75 % of 4
		if err := q.Enqueue(anon(i)); err != nil {
			t.Fatalf("anonymous %d: %v", i, err)
		}
	}
	if err := q.Enqueue(anon(3)); !errors.Is(err, ErrQueueFull) {
		t.Fatalf("anonymous above the high-water mark = %v, want ErrQueueFull", err)
	}
	if err := q.Enqueue(Message{To: "order@example.com", Subject: "paid", Template: TplOrderPaid}); err != nil {
		t.Fatalf("transactional mail must still fit: %v", err)
	}
	if err := q.Enqueue(Message{To: "late@example.com", Subject: "s"}); !errors.Is(err, ErrQueueFull) {
		t.Fatalf("full queue = %v", err)
	}
	if p := q.Stats().Pending; p != 5 {
		t.Fatalf("pending = %d, want 5 (refused messages must not count)", p)
	}
	g.open()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := q.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	if s := q.Stats(); s.Sent != 5 || s.Failed != 0 {
		t.Fatalf("stats = %+v", s)
	}
}

// Concurrent anonymous senders cannot overshoot the high-water mark.
func TestAuditAnonymousHighWaterConcurrent(t *testing.T) {
	d, st := setup(t)
	q, err := NewQueue(d, st, logx.Discard(), QueueOptions{Size: 8}) // not started: nothing drains
	if err != nil {
		t.Fatal(err)
	}
	var (
		wg sync.WaitGroup
		mu sync.Mutex
		ok int
	)
	for i := range 64 {
		wg.Go(func() {
			if q.Enqueue(Message{To: fmt.Sprintf("a%d@example.com", i), Class: ClassAnonymous}) == nil {
				mu.Lock()
				ok++
				mu.Unlock()
			}
		})
	}
	wg.Wait()
	if ok != 6 {
		t.Fatalf("admitted %d anonymous messages, want 6 (75%% of 8)", ok)
	}
	for i := range 2 {
		if err := q.Enqueue(Message{To: fmt.Sprintf("t%d@example.com", i)}); err != nil {
			t.Fatalf("transactional %d: %v", i, err)
		}
	}
}

// script is a mailer that fails per recipient and records the order of attempts.
type script struct {
	mu    sync.Mutex
	calls []string
	at    map[string][]time.Time
	fail  func(to string) error
}

func (s *script) Send(_ context.Context, m Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, m.To)
	if s.at == nil {
		s.at = map[string][]time.Time{}
	}
	s.at[m.To] = append(s.at[m.To], time.Now())
	return s.fail(m.To)
}

func (s *script) snapshot() ([]string, map[string][]time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.calls...), s.at
}

// Audit 2026-10-22 (mail queue): every failure, permanent ones included, was retried 3 times with 1/2/4 s
// sleeps on the only worker. Permanent failures are now recorded after one attempt.
func TestAuditPermanentFailureNotRetried(t *testing.T) {
	d, st := setup(t)
	q, err := NewQueue(d, st, logx.Discard(), QueueOptions{Retries: 3, Backoff: func(int) time.Duration { return time.Millisecond }})
	if err != nil {
		t.Fatal(err)
	}
	s := &script{fail: func(to string) error {
		switch {
		case strings.HasSuffix(to, "@invalid.example"):
			return fmt.Errorf("mail: smtp send: %w", &gomail.SendError{Reason: gomail.ErrGetRcpts})
		case to == "config@example.com":
			return Permanent(errors.New("mail: to: no address"))
		}
		return nil
	}}
	q.SetMailer(s)
	q.Start()
	for _, to := range []string{"nobody@invalid.example", "config@example.com", "ok@example.com"} {
		if err := q.Enqueue(Message{To: to, Subject: "s", Text: "t"}); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := q.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	calls, _ := s.snapshot()
	if want := []string{"nobody@invalid.example", "config@example.com", "ok@example.com"}; strings.Join(calls, ",") != strings.Join(want, ",") {
		t.Fatalf("attempts = %v, want one each %v", calls, want)
	}
	if st := q.Stats(); st.Sent != 1 || st.Failed != 2 || st.Pending != 0 {
		t.Fatalf("stats = %+v", st)
	}
	var logs []model.MailLog
	d.Order("id").Find(&logs)
	if len(logs) != 3 || logs[0].Error == nil || logs[1].Error == nil || logs[2].Error != nil {
		t.Fatalf("mail_logs = %+v", logs)
	}
	if err := q.Stop(ctx); err != nil {
		t.Fatal(err)
	}
}

// A temporary failure is retried after its backoff without holding up the messages behind it.
func TestAuditTemporaryRetryDoesNotBlockWorker(t *testing.T) {
	d, st := setup(t)
	const backoff = 150 * time.Millisecond
	q, err := NewQueue(d, st, logx.Discard(), QueueOptions{Retries: 3, Backoff: func(int) time.Duration { return backoff }})
	if err != nil {
		t.Fatal(err)
	}
	s := &script{fail: func(to string) error {
		if to == "busy@example.com" {
			return errors.New("451 4.3.0 try again later") // not a SendError: temporary
		}
		return nil
	}}
	q.SetMailer(s)
	q.Start()
	for _, to := range []string{"busy@example.com", "ok@example.com", "ok2@example.com"} {
		if err := q.Enqueue(Message{To: to, Subject: "s", Text: "t"}); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := q.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	calls, at := s.snapshot()
	want := "busy@example.com,ok@example.com,ok2@example.com,busy@example.com,busy@example.com,busy@example.com"
	if strings.Join(calls, ",") != want {
		t.Fatalf("attempt order = %v, want %s", calls, want)
	}
	if okAt, busy := at["ok2@example.com"][0], at["busy@example.com"]; okAt.Sub(busy[0]) >= backoff {
		t.Fatalf("later messages waited %v for the retry backoff", okAt.Sub(busy[0]))
	}
	for i := 1; i < len(at["busy@example.com"]); i++ {
		if gap := at["busy@example.com"][i].Sub(at["busy@example.com"][i-1]); gap < backoff {
			t.Fatalf("retry %d after %v, want ≥ %v", i, gap, backoff)
		}
	}
	if st := q.Stats(); st.Sent != 2 || st.Failed != 1 || st.Pending != 0 {
		t.Fatalf("stats = %+v", st)
	}
	if err := q.Stop(ctx); err != nil {
		t.Fatal(err)
	}
}

// A message waiting for its retry keeps Wait and Stop waiting; after Stop's deadline it is recorded as failed.
func TestAuditStopAbortsWaitingRetry(t *testing.T) {
	d, st := setup(t)
	q, err := NewQueue(d, st, logx.Discard(), QueueOptions{Retries: 3, Backoff: func(int) time.Duration { return time.Hour }})
	if err != nil {
		t.Fatal(err)
	}
	s := &script{fail: func(string) error { return errors.New("connection reset") }}
	q.SetMailer(s)
	q.Start()
	if err := q.Enqueue(Message{To: "retry@example.com", Subject: "s"}); err != nil {
		t.Fatal(err)
	}
	short, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := q.Wait(short); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Wait = %v, want the retrying message to keep it waiting", err)
	}
	if p := q.Stats().Pending; p != 1 {
		t.Fatalf("pending = %d", p)
	}
	ctx, cancel2 := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel2()
	start := time.Now()
	if err := q.Stop(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Stop = %v", err)
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("Stop took %v", d)
	}
	if st := q.Stats(); st.Failed != 1 || st.Pending != 0 {
		t.Fatalf("stats = %+v", st)
	}
	var log model.MailLog
	d.First(&log)
	if log.Error == nil || !strings.Contains(*log.Error, "shutdown before delivery") || !strings.Contains(*log.Error, "connection reset") {
		t.Fatalf("mail_log error = %v", log.Error)
	}
	if calls, _ := s.snapshot(); len(calls) != 1 {
		t.Fatalf("attempts = %v", calls)
	}
}

func TestAuditIsPermanent(t *testing.T) {
	for name, tc := range map[string]struct {
		err  error
		want bool
	}{
		"nil":                     {nil, false},
		"plain":                   {errors.New("boom"), false},
		"timeout":                 {context.DeadlineExceeded, false},
		"marked":                  {Permanent(errors.New("bad address")), true},
		"marked and wrapped":      {fmt.Errorf("outer: %w", Permanent(errors.New("x"))), true},
		"recipient list":          {fmt.Errorf("send failed: %w", &gomail.SendError{Reason: gomail.ErrGetRcpts}), true},
		"no reply to RCPT":        {&gomail.SendError{Reason: gomail.ErrSMTPRcptTo}, false},
		"connection check":        {&gomail.SendError{Reason: gomail.ErrConnCheck}, false},
		"auth refused while dial": {fmt.Errorf("dial failed: %w", &textproto.Error{Code: 535, Msg: "5.7.8 bad credentials"}), true},
		"busy while dial":         {fmt.Errorf("dial failed: %w", &textproto.Error{Code: 421, Msg: "busy"}), false},
		"nil Permanent":           {Permanent(nil), false},
	} {
		if got := IsPermanent(tc.err); got != tc.want {
			t.Errorf("%s: IsPermanent(%v) = %v, want %v", name, tc.err, got, tc.want)
		}
	}
}

// fakeSMTP is a minimal SMTP server: RCPT to @invalid.example gets 550, to @busy.example 451, others 250.
type fakeSMTP struct {
	ln   net.Listener
	mu   sync.Mutex
	rcpt map[string]int
	data int
}

func startFakeSMTP(t *testing.T) *fakeSMTP {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := &fakeSMTP{ln: ln, rcpt: map[string]int{}}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go s.serve(c)
		}
	}()
	return s
}

func (s *fakeSMTP) serve(c net.Conn) {
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(10 * time.Second))
	r := bufio.NewReader(c)
	reply := func(line string) { _, _ = c.Write([]byte(line + "\r\n")) }
	reply("220 mx.example.com ESMTP")
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		cmd := strings.ToUpper(line)
		switch {
		case strings.HasPrefix(cmd, "EHLO"), strings.HasPrefix(cmd, "HELO"):
			reply("250 mx.example.com")
		case strings.HasPrefix(cmd, "MAIL FROM:"), cmd == "RSET", cmd == "NOOP":
			reply("250 2.0.0 OK")
		case strings.HasPrefix(cmd, "RCPT TO:"):
			addr := strings.ToLower(strings.Trim(line[len("RCPT TO:"):], " <>"))
			s.mu.Lock()
			s.rcpt[addr]++
			s.mu.Unlock()
			switch {
			case strings.HasSuffix(addr, "@invalid.example"):
				reply("550 5.1.1 no such user")
			case strings.HasSuffix(addr, "@busy.example"):
				reply("451 4.3.0 try again later")
			default:
				reply("250 2.1.5 OK")
			}
		case cmd == "DATA":
			reply("354 go ahead")
			for {
				l, err := r.ReadString('\n')
				if err != nil {
					return
				}
				if l == ".\r\n" {
					break
				}
			}
			s.mu.Lock()
			s.data++
			s.mu.Unlock()
			reply("250 2.0.0 queued")
		case cmd == "QUIT":
			reply("221 2.0.0 bye")
			return
		default:
			reply("502 5.5.2 command not implemented")
		}
	}
}

func (s *fakeSMTP) counts(addr string) (rcpt, data int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.rcpt[addr], s.data
}

// The SMTP driver's real errors are classified: a 550 at RCPT is permanent and sent once, a 451 is retried.
func TestAuditSMTPRejectionsClassified(t *testing.T) {
	srv := startFakeSMTP(t)
	port := srv.ln.Addr().(*net.TCPAddr).Port
	m, err := NewSMTP(SMTPConfig{Host: "127.0.0.1", Port: port, Encryption: "none", FromAddress: "panel@example.com", Timeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	err = m.Send(ctx, Message{To: "nobody@invalid.example", Subject: "s", Text: "t"})
	var se *gomail.SendError
	if !errors.As(err, &se) || se.ErrorCode() != 550 || !IsPermanent(err) {
		t.Fatalf("550 at RCPT: err = %v, permanent = %v", err, IsPermanent(err))
	}
	if err := m.Send(ctx, Message{To: "someone@busy.example", Subject: "s", Text: "t"}); err == nil || IsPermanent(err) {
		t.Fatalf("451 at RCPT: err = %v, permanent = %v", err, IsPermanent(err))
	}
	if err := m.Send(ctx, Message{To: "ok@example.com", Subject: "s", Text: "t"}); err != nil {
		t.Fatal(err)
	}

	d, st := setup(t)
	q, err := NewQueue(d, st, logx.Discard(), QueueOptions{Retries: 3, Backoff: func(int) time.Duration { return time.Millisecond }})
	if err != nil {
		t.Fatal(err)
	}
	q.SetMailer(m)
	q.Start()
	for _, to := range []string{"queued@invalid.example", "queued@busy.example", "queued@example.com"} {
		if err := q.Enqueue(Message{To: to, Subject: "s", Text: "t"}); err != nil {
			t.Fatal(err)
		}
	}
	wctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := q.Wait(wctx); err != nil {
		t.Fatal(err)
	}
	if n, _ := srv.counts("queued@invalid.example"); n != 1 {
		t.Fatalf("permanent rejection attempted %d times, want 1", n)
	}
	if n, _ := srv.counts("queued@busy.example"); n != 4 {
		t.Fatalf("temporary rejection attempted %d times, want 1 + 3 retries", n)
	}
	if n, data := srv.counts("queued@example.com"); n != 1 || data != 2 {
		t.Fatalf("ok@example.com: rcpt %d, data %d", n, data)
	}
	if s := q.Stats(); s.Sent != 1 || s.Failed != 2 {
		t.Fatalf("stats = %+v", s)
	}
	if err := q.Stop(wctx); err != nil {
		t.Fatal(err)
	}
}
