package model

import "time"

// Node is a proxy server entry. Type-specific options live in ProtocolSettings (see protocol.go).
type Node struct {
	ID               uint64            `gorm:"primaryKey" json:"id"`
	Type             string            `gorm:"size:16;index;not null" json:"type"`
	Name             string            `gorm:"size:191;not null" json:"name"`
	GroupIDs         JSONSlice[uint64] `json:"group_ids"`
	RouteIDs         JSONSlice[uint64] `json:"route_ids"`
	ParentID         *uint64           `gorm:"index" json:"parent_id"`
	Parent           *Node             `gorm:"foreignKey:ParentID" json:"-"`
	Host             string            `gorm:"size:255;not null" json:"host"`
	Port             string            `gorm:"size:32;not null" json:"port"` // may be a range "10000-20000"
	ServerPort       int               `gorm:"not null" json:"server_port"`
	Tags             JSONSlice[string] `json:"tags"`
	Rate             float64           `gorm:"not null" json:"rate"`
	Show             bool              `gorm:"not null;default:false" json:"show"`
	Sort             int               `gorm:"not null;default:0" json:"sort"`
	ProtocolSettings JSONMap           `json:"protocol_settings"`
	CreatedAt        time.Time         `json:"created_at"`
	UpdatedAt        time.Time         `json:"updated_at"`
}

// InGroup reports whether the node is visible to server group id.
func (n *Node) InGroup(id uint64) bool { return SliceContains(n.GroupIDs, id) }

// InAnyGroup reports whether the node is visible to any of the group ids.
func (n *Node) InAnyGroup(ids ...uint64) bool {
	for _, id := range ids {
		if n.InGroup(id) {
			return true
		}
	}
	return false
}

// NodeRoute is a routing rule delivered to nodes (XrayR routes / soga audit rules).
type NodeRoute struct {
	ID          uint64            `gorm:"primaryKey" json:"id"`
	Remarks     string            `gorm:"size:191" json:"remarks"`
	Match       JSONSlice[string] `json:"match"`
	Action      string            `gorm:"size:16;not null" json:"action"` // block | dns
	ActionValue string            `gorm:"size:255" json:"action_value"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}
