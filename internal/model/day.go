package model

import "time"

// Day returns the calendar day of t (evaluated in t's location) encoded as midnight UTC. Every `date` column
// (stat_users.record_at, stat_nodes.record_at, stats.record_at) stores values produced by Day so that equality
// and range queries behave identically on SQLite, PostgreSQL and MySQL.
func Day(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// Today is Day(time.Now()) in the process's local time zone.
func Today() time.Time { return Day(time.Now()) }
