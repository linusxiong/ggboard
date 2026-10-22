package user

import (
	"time"

	"github.com/linusxiong/ggboard/internal/model"
)

// NextResetAt computes the next traffic reset strictly after now (evaluated in now's location):
//
//	0 ResetFirstDayOfMonth: 00:00 on the 1st of next month
//	1 ResetMonthly:         monthly on expiredAt's day-of-month and time (clamped to the month length)
//	2 ResetNever:           nil
//	3 ResetFirstDayOfYear:  00:00 on Jan 1 of next year
//	4 ResetYearly:          yearly on expiredAt's month/day and time (Feb 29 clamps to Feb 28)
//
// It returns nil when expiredAt is nil (one-time / never-expiring plans keep their traffic, as in Xboard).
func NextResetAt(method model.ResetMethod, expiredAt *time.Time, now time.Time) *time.Time {
	if expiredAt == nil {
		return nil
	}
	loc := now.Location()
	exp := expiredAt.In(loc)
	var t time.Time
	switch method {
	case model.ResetFirstDayOfMonth:
		t = time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, loc)
	case model.ResetMonthly:
		t = monthDay(now.Year(), now.Month(), exp, loc)
		if !t.After(now) {
			t = monthDay(now.Year(), now.Month()+1, exp, loc)
		}
	case model.ResetFirstDayOfYear:
		t = time.Date(now.Year()+1, time.January, 1, 0, 0, 0, 0, loc)
	case model.ResetYearly:
		t = monthDay(now.Year(), exp.Month(), exp, loc)
		if !t.After(now) {
			t = monthDay(now.Year()+1, exp.Month(), exp, loc)
		}
	default:
		return nil
	}
	return &t
}

// monthDay returns year/month (normalized) at ref's day (clamped to the month length) and time of day.
func monthDay(year int, month time.Month, ref time.Time, loc *time.Location) time.Time {
	first := time.Date(year, month, 1, 0, 0, 0, 0, loc) // normalizes month overflow
	day := min(ref.Day(), daysIn(first.Year(), first.Month(), loc))
	return time.Date(first.Year(), first.Month(), day, ref.Hour(), ref.Minute(), ref.Second(), 0, loc)
}

func daysIn(year int, month time.Month, loc *time.Location) int {
	return time.Date(year, month+1, 0, 0, 0, 0, 0, loc).Day()
}
