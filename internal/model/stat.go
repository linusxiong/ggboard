package model

import "time"

// StatUser is per-day, per-node user traffic (u/d already multiplied by the node rate).
type StatUser struct {
	ID       uint64    `gorm:"primaryKey" json:"id"`
	UserID   uint64    `gorm:"not null;index;uniqueIndex:uniq_stat_users_day,priority:1" json:"user_id"`
	NodeID   uint64    `gorm:"not null;index;uniqueIndex:uniq_stat_users_day,priority:2" json:"node_id"`
	Rate     float64   `gorm:"not null" json:"rate"`
	U        int64     `gorm:"column:u;not null;default:0" json:"u"`
	D        int64     `gorm:"column:d;not null;default:0" json:"d"`
	RecordAt time.Time `gorm:"type:date;not null;index;uniqueIndex:uniq_stat_users_day,priority:3" json:"record_at"`
}

// StatNode is per-day node traffic (raw bytes).
type StatNode struct {
	ID       uint64    `gorm:"primaryKey" json:"id"`
	NodeID   uint64    `gorm:"not null;uniqueIndex:uniq_stat_nodes_day,priority:1" json:"node_id"`
	U        int64     `gorm:"column:u;not null;default:0" json:"u"`
	D        int64     `gorm:"column:d;not null;default:0" json:"d"`
	RecordAt time.Time `gorm:"type:date;not null;index;uniqueIndex:uniq_stat_nodes_day,priority:2" json:"record_at"`
}

// Stat is the daily business summary produced by the stats_daily job.
type Stat struct {
	ID                uint64    `gorm:"primaryKey" json:"id"`
	RecordAt          time.Time `gorm:"type:date;not null;uniqueIndex" json:"record_at"`
	OrderCount        int64     `gorm:"not null;default:0" json:"order_count"`
	OrderTotal        int64     `gorm:"not null;default:0" json:"order_total"`
	PaidCount         int64     `gorm:"not null;default:0" json:"paid_count"`
	PaidTotal         int64     `gorm:"not null;default:0" json:"paid_total"`
	CommissionCount   int64     `gorm:"not null;default:0" json:"commission_count"`
	CommissionTotal   int64     `gorm:"not null;default:0" json:"commission_total"`
	RegisterCount     int64     `gorm:"not null;default:0" json:"register_count"`
	InviteCount       int64     `gorm:"not null;default:0" json:"invite_count"`
	TransferUsedTotal int64     `gorm:"not null;default:0" json:"transfer_used_total"`
}
