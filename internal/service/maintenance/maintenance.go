// Package maintenance registers the periodic maintenance jobs (docs/ARCHITECTURE.md §Scheduler jobs):
//
//	traffic_reset   0 0 * * *   reset u/d of users whose next_reset_at has passed and schedule the next reset
//	expire_remind   0 10 * * *  mail users whose subscription expires within 24 h (remind_expire)
//	traffic_remind  0 10 * * *  mail users who used at least 80 % of their traffic (remind_traffic)
//	stats_daily     5 0 * * *   upsert yesterday's `stats` row
//
// The jobs register themselves in init(); the binary links them through cmd/ggboard/jobs_maintenance.go.
// Every job is idempotent and exported with an explicit clock so tests can run it at a fixed time.
package maintenance

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"gorm.io/gorm"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/mail"
	"github.com/linusxiong/ggboard/internal/model"
	statsvc "github.com/linusxiong/ggboard/internal/service/stat"
	usersvc "github.com/linusxiong/ggboard/internal/service/user"
)

// Job names.
const (
	JobTrafficReset  = "traffic_reset"
	JobExpireRemind  = "expire_remind"
	JobTrafficRemind = "traffic_remind"
	JobStatsDaily    = "stats_daily"
)

// Clock returns the current time; jobs read it once per run.
type Clock func() time.Time

// BatchSize is the number of users processed per query / transaction.
const BatchSize = 500

// RemindWindow is how far ahead expire_remind looks, and how long a sent reminder suppresses another one.
const RemindWindow = 24 * time.Hour

// TrafficRemindPercent is the usage threshold of traffic_remind.
const TrafficRemindPercent = 80

// KV key prefixes marking reminders as sent.
const (
	kvExpireRemind  = "maintenance:expire_remind:"
	kvTrafficRemind = "maintenance:traffic_remind:"
)

func init() {
	app.RegisterJob(JobTrafficReset, "0 0 * * *", func(ctx context.Context, a *app.App) error {
		_, err := TrafficReset(ctx, a, time.Now)
		return err
	})
	app.RegisterJob(JobExpireRemind, "0 10 * * *", func(ctx context.Context, a *app.App) error {
		_, err := ExpireRemind(ctx, a, time.Now)
		return err
	})
	app.RegisterJob(JobTrafficRemind, "0 10 * * *", func(ctx context.Context, a *app.App) error {
		_, err := TrafficRemind(ctx, a, time.Now)
		return err
	})
	app.RegisterJob(JobStatsDaily, "5 0 * * *", func(ctx context.Context, a *app.App) error {
		_, err := StatsDaily(ctx, a, time.Now)
		return err
	})
}

// ---------------------------------------------------------------- traffic_reset

// TrafficReset processes users with next_reset_at <= now in batches of BatchSize (one transaction each).
// A user whose plan still resets (plan present, effective method not "never", expired_at set; see
// usersvc.NextResetAt) gets u = d = 0 and the next next_reset_at; any other user only has the stale
// next_reset_at cleared. It returns the number of users whose traffic was reset.
func TrafficReset(ctx context.Context, a *app.App, now Clock) (int, error) {
	t := now()
	svc := usersvc.New(a)
	plans, err := loadPlans(ctx, a.DB)
	if err != nil {
		return 0, err
	}
	type group struct {
		reset bool
		next  int64 // UnixNano, 0 = nil
	}
	var lastID uint64
	resetCount, touched := 0, false
	for {
		var users []model.User
		err := a.DB.WithContext(ctx).Select("id", "plan_id", "expired_at", "next_reset_at").
			Where("next_reset_at IS NOT NULL AND next_reset_at <= ?", t).
			Where("id > ?", lastID).
			Order("id").Limit(BatchSize).Find(&users).Error
		if err != nil {
			return resetCount, fmt.Errorf("traffic_reset: load users: %w", err)
		}
		if len(users) == 0 {
			break
		}
		lastID = users[len(users)-1].ID

		groups := map[group][]uint64{}
		nexts := map[group]*time.Time{}
		for i := range users {
			u := &users[i]
			var plan *model.Plan
			if u.PlanID != nil {
				plan = plans[*u.PlanID]
			}
			next := svc.ComputeNextResetAt(u, plan, t)
			g := group{reset: next != nil}
			if next != nil {
				g.next = next.UnixNano()
			}
			groups[g] = append(groups[g], u.ID)
			nexts[g] = next
		}
		err = a.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			for g, ids := range groups {
				set := map[string]any{"next_reset_at": nexts[g]}
				if g.reset {
					set["u"], set["d"] = 0, 0
				}
				res := tx.Model(&model.User{}).Where("id IN ?", ids).
					Where("next_reset_at IS NOT NULL AND next_reset_at <= ?", t).Updates(set)
				if res.Error != nil {
					return res.Error
				}
				if g.reset {
					resetCount += int(res.RowsAffected)
				}
			}
			return nil
		})
		if err != nil {
			return resetCount, fmt.Errorf("traffic_reset: update users: %w", err)
		}
		touched = true
		if len(users) < BatchSize {
			break
		}
	}
	if touched {
		svc.InvalidateNodeUsers()
	}
	if resetCount > 0 {
		a.Log.Info("traffic reset", "users", resetCount)
	}
	return resetCount, nil
}

func loadPlans(ctx context.Context, db *gorm.DB) (map[uint64]*model.Plan, error) {
	var plans []model.Plan
	if err := db.WithContext(ctx).Find(&plans).Error; err != nil {
		return nil, fmt.Errorf("maintenance: load plans: %w", err)
	}
	out := make(map[uint64]*model.Plan, len(plans))
	for i := range plans {
		out[plans[i].ID] = &plans[i]
	}
	return out, nil
}

// ---------------------------------------------------------------- reminders

// ExpireRemind mails remind_expire to users (not banned, remind_expire on) whose expired_at is in
// (now, now + 24h], at most once per 24 h per user. Nothing is sent when remind_mail_enable is off.
// It returns the number of queued mails.
func ExpireRemind(ctx context.Context, a *app.App, now Clock) (int, error) {
	if !a.Settings.Bool("remind_mail_enable") {
		return 0, nil
	}
	t := now()
	q := a.DB.WithContext(ctx).Model(&model.User{}).Select("id", "email", "expired_at").
		Where("banned = ? AND remind_expire = ?", false, true).
		Where("expired_at > ? AND expired_at <= ?", t, t.Add(RemindWindow))
	return remind(ctx, a, q, kvExpireRemind, func(u *model.User) (string, mail.Data) {
		return mail.TplRemindExpire, mail.Data{"ExpiredAt": u.ExpiredAt.In(t.Location()).Format("2006-01-02 15:04")}
	})
}

// TrafficRemind mails remind_traffic to users (not banned, not expired, remind_traffic on) who used at least
// 80 % but less than 100 % of transfer_enable, at most once per 24 h per user. Nothing is sent when
// remind_mail_enable is off. It returns the number of queued mails.
func TrafficRemind(ctx context.Context, a *app.App, now Clock) (int, error) {
	if !a.Settings.Bool("remind_mail_enable") {
		return 0, nil
	}
	t := now()
	q := a.DB.WithContext(ctx).Model(&model.User{}).Select("id", "email", "u", "d", "transfer_enable").
		Where("banned = ? AND remind_traffic = ?", false, true).
		Where("expired_at IS NULL OR expired_at > ?", t).
		Where("transfer_enable > 0").
		Where("(u + d) * 100 >= transfer_enable * ?", TrafficRemindPercent).
		Where("u + d < transfer_enable")
	return remind(ctx, a, q, kvTrafficRemind, func(u *model.User) (string, mail.Data) {
		used := u.Used()
		return mail.TplRemindTraffic, mail.Data{
			"Percent": used * 100 / u.TransferEnable,
			"Used":    FormatBytes(used),
			"Total":   FormatBytes(u.TransferEnable),
		}
	})
}

// remind walks q in id batches and queues one mail per user not yet marked under kvPrefix.
func remind(ctx context.Context, a *app.App, q *gorm.DB, kvPrefix string, build func(*model.User) (string, mail.Data)) (int, error) {
	var lastID uint64
	sent := 0
	for {
		var users []model.User
		if err := q.Session(&gorm.Session{}).Where("id > ?", lastID).Order("id").Limit(BatchSize).
			Find(&users).Error; err != nil {
			return sent, fmt.Errorf("maintenance: load users: %w", err)
		}
		for i := range users {
			u := &users[i]
			key := kvPrefix + strconv.FormatUint(u.ID, 10)
			if _, done := a.KV.Get(key); done {
				continue
			}
			tpl, data := build(u)
			msg, err := a.Mailer.Render(u.Email, tpl, data)
			if err != nil {
				return sent, err
			}
			if err := Enqueue(ctx, a.Mailer, msg); err != nil {
				return sent, err
			}
			a.KV.Set(key, true, RemindWindow)
			sent++
		}
		if len(users) < BatchSize {
			break
		}
		lastID = users[len(users)-1].ID
	}
	if sent > 0 {
		a.Log.Info("reminders queued", "kind", kvPrefix, "mails", sent)
	}
	return sent, nil
}

// Enqueue queues msg, waiting (with backoff, until ctx is done) while the mail queue is full.
func Enqueue(ctx context.Context, q *mail.Queue, msg mail.Message) error {
	delay := 50 * time.Millisecond
	for {
		err := q.Enqueue(msg)
		if !errors.Is(err, mail.ErrQueueFull) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
		delay = min(2*delay, 2*time.Second)
	}
}

// FormatBytes renders n with binary units ("1.50 GB").
func FormatBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return strconv.FormatInt(n, 10) + " B"
	}
	units := []string{"KB", "MB", "GB", "TB", "PB", "EB"}
	v, i := float64(n)/unit, 0
	for v >= unit && i < len(units)-1 {
		v /= unit
		i++
	}
	return fmt.Sprintf("%.2f %s", v, units[i])
}

// ---------------------------------------------------------------- stats_daily

// StatsDaily upserts the `stats` row of the day before now (idempotent).
func StatsDaily(ctx context.Context, a *app.App, now Clock) (model.Stat, error) {
	yesterday := statsvc.DayStart(now()).AddDate(0, 0, -1)
	return statsvc.New(a).SaveDay(ctx, yesterday)
}
