package model

import "time"

// BytesPerGB converts plans.transfer_enable (GB) to bytes.
const BytesPerGB int64 = 1 << 30

// Plan is a purchasable subscription package.
type Plan struct {
	ID                 uint64            `gorm:"primaryKey" json:"id"`
	GroupID            uint64            `gorm:"index;not null" json:"group_id"`
	Name               string            `gorm:"size:191;not null" json:"name"`
	Content            string            `gorm:"type:text" json:"content"`
	Tags               JSONSlice[string] `json:"tags"`
	TransferEnable     int64             `gorm:"not null;default:0" json:"transfer_enable"` // GB
	SpeedLimit         *int              `json:"speed_limit"`
	DeviceLimit        *int              `json:"device_limit"`
	CapacityLimit      *int              `json:"capacity_limit"`
	Prices             JSONObject[int64] `json:"prices"` // period -> cents; absent = not for sale
	ResetTrafficMethod *ResetMethod      `json:"reset_traffic_method"`
	Show               bool              `gorm:"not null;default:false" json:"show"`
	Sell               bool              `gorm:"not null;default:false" json:"sell"`
	Renew              bool              `gorm:"not null;default:false" json:"renew"`
	Sort               int               `gorm:"not null;default:0" json:"sort"`
	CreatedAt          time.Time         `json:"created_at"`
	UpdatedAt          time.Time         `json:"updated_at"`
}

// Price returns the price (cents) for period and whether the plan is sold for it.
func (p *Plan) Price(period string) (int64, bool) {
	v, ok := p.Prices[period]
	return v, ok
}

// TransferBytes returns the plan traffic in bytes.
func (p *Plan) TransferBytes() int64 { return p.TransferEnable * BytesPerGB }

// ServerGroup is a set of nodes granted by plans.
type ServerGroup struct {
	ID        uint64    `gorm:"primaryKey" json:"id"`
	Name      string    `gorm:"size:191;not null" json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
