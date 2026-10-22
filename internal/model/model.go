// Package model holds every GORM model (docs/DATA_MODEL.md is the contract), JSON column types, enums,
// typed protocol settings and credential derivation helpers.
//
// Conventions: ids are uint64, money is int64 cents, traffic is int64 bytes, nullable scalars are pointers,
// JSON columns are TEXT. Foreign-key constraints are not created (imports may contain dangling references);
// relations exist only for Preload. When creating rows that carry loaded relations, use Omit(clause.Associations).
package model

// All returns every model for AutoMigrate, in dependency-friendly order.
func All() []any {
	return []any{
		&ServerGroup{},
		&Plan{},
		&User{},
		&Node{},
		&NodeRoute{},
		&Payment{},
		&Order{},
		&Coupon{},
		&InviteCode{},
		&CommissionLog{},
		&Ticket{},
		&TicketMessage{},
		&Notice{},
		&Knowledge{},
		&Setting{},
		&StatUser{},
		&StatNode{},
		&Stat{},
		&MailLog{},
		&RuleProfile{},
	}
}
