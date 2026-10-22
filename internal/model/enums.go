package model

// Node types (nodes.type).
const (
	NodeVmess        = "vmess"
	NodeVless        = "vless"
	NodeShadowsocks  = "shadowsocks"
	NodeShadowsocksR = "shadowsocksr"
	NodeTrojan       = "trojan"
	NodeHysteria     = "hysteria"
	NodeAnyTLS       = "anytls"
	NodeMieru        = "mieru"
	NodeTuic         = "tuic"
)

// NodeTypes lists every supported node type in display order.
func NodeTypes() []string {
	return []string{NodeVmess, NodeVless, NodeShadowsocks, NodeShadowsocksR, NodeTrojan, NodeHysteria, NodeAnyTLS, NodeMieru, NodeTuic}
}

// IsValidNodeType reports whether t is a supported node type.
func IsValidNodeType(t string) bool {
	for _, x := range NodeTypes() {
		if x == t {
			return true
		}
	}
	return false
}

// Price periods (plans.prices keys, orders.period).
const (
	PeriodMonthly      = "monthly"
	PeriodQuarterly    = "quarterly"
	PeriodHalfYearly   = "half_yearly"
	PeriodYearly       = "yearly"
	PeriodTwoYearly    = "two_yearly"
	PeriodThreeYearly  = "three_yearly"
	PeriodOnetime      = "onetime"
	PeriodResetTraffic = "reset_traffic"
)

// Periods lists every price period in display order.
func Periods() []string {
	return []string{PeriodMonthly, PeriodQuarterly, PeriodHalfYearly, PeriodYearly, PeriodTwoYearly, PeriodThreeYearly, PeriodOnetime, PeriodResetTraffic}
}

// IsValidPeriod reports whether p is a known period.
func IsValidPeriod(p string) bool {
	for _, x := range Periods() {
		if x == p {
			return true
		}
	}
	return false
}

// PeriodMonths returns the number of months a period adds to expired_at (0 for onetime / reset_traffic).
func PeriodMonths(p string) int {
	switch p {
	case PeriodMonthly:
		return 1
	case PeriodQuarterly:
		return 3
	case PeriodHalfYearly:
		return 6
	case PeriodYearly:
		return 12
	case PeriodTwoYearly:
		return 24
	case PeriodThreeYearly:
		return 36
	}
	return 0
}

// OrderStatus is orders.status.
type OrderStatus int8

// Order statuses.
const (
	OrderStatusPending    OrderStatus = 0
	OrderStatusProcessing OrderStatus = 1
	OrderStatusCancelled  OrderStatus = 2
	OrderStatusCompleted  OrderStatus = 3
	OrderStatusDiscounted OrderStatus = 4 // superseded by an upgrade (surplus consumed)
)

// OrderType is orders.type.
type OrderType int8

// Order types.
const (
	OrderTypeNew          OrderType = 1
	OrderTypeRenew        OrderType = 2
	OrderTypeUpgrade      OrderType = 3
	OrderTypeResetTraffic OrderType = 4
)

// CommissionStatus is orders.commission_status.
type CommissionStatus int8

// Commission statuses.
const (
	CommissionStatusPending    CommissionStatus = 0
	CommissionStatusProcessing CommissionStatus = 1
	CommissionStatusValid      CommissionStatus = 2
	CommissionStatusInvalid    CommissionStatus = 3
)

// CommissionType is users.commission_type.
type CommissionType int8

// Commission types.
const (
	CommissionTypeSystem  CommissionType = 0 // follow settings
	CommissionTypePeriod  CommissionType = 1 // every paid order
	CommissionTypeOneTime CommissionType = 2 // first order only
)

// ResetMethod is plans.reset_traffic_method (nil = follow setting reset_traffic_method).
type ResetMethod int8

// Traffic reset methods.
const (
	ResetFirstDayOfMonth ResetMethod = 0
	ResetMonthly         ResetMethod = 1 // monthly on the purchase (expiry) day
	ResetNever           ResetMethod = 2
	ResetFirstDayOfYear  ResetMethod = 3
	ResetYearly          ResetMethod = 4 // yearly on the purchase (expiry) day
)

// TicketStatus is tickets.status.
type TicketStatus int8

// Ticket statuses.
const (
	TicketStatusOpen   TicketStatus = 0
	TicketStatusClosed TicketStatus = 1
)

// TicketReplyStatus is tickets.reply_status.
type TicketReplyStatus int8

// Ticket reply statuses.
const (
	TicketAwaitingStaff TicketReplyStatus = 0
	TicketReplied       TicketReplyStatus = 1
)

// TicketLevel is tickets.level.
type TicketLevel int8

// Ticket levels.
const (
	TicketLevelLow    TicketLevel = 0
	TicketLevelMedium TicketLevel = 1
	TicketLevelHigh   TicketLevel = 2
)

// CouponType is coupons.type.
type CouponType int8

// Coupon types.
const (
	CouponTypeAmount  CouponType = 1 // value in cents
	CouponTypePercent CouponType = 2 // value in percent
)

// Node route actions (node_routes.action).
const (
	RouteActionBlock = "block"
	RouteActionDNS   = "dns"
)
