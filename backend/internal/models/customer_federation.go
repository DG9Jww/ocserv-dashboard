package models

import "time"

type PortalCustomer struct {
	ID           string   `json:"id" gorm:"type:uuid;primaryKey;default:uuidv7()"`
	Username     string   `json:"username" gorm:"not null;uniqueIndex"`
	PasswordHash []byte   `json:"-" gorm:"not null"`
	Nodes        []string `json:"nodes" gorm:"serializer:json;type:jsonb"`
	Groups       []string `json:"groups" gorm:"serializer:json;type:jsonb"`
	DefaultNode  string   `json:"default_node"`
	Locked       bool     `json:"locked"`
	// DeletedAt is set while permanent deletion is waiting for node confirmation.
	DeletedAt *time.Time `json:"deleted_at,omitempty"`
	CreatedAt time.Time
}

type PortalNodeGroup struct {
	ID    string   `json:"id" gorm:"type:uuid;primaryKey;default:uuidv7()"`
	Name  string   `json:"name" gorm:"not null;uniqueIndex"`
	Nodes []string `json:"nodes" gorm:"serializer:json;type:jsonb"`
}

type PortalNodeDefault struct {
	Node        string      `json:"node" gorm:"primaryKey"`
	Group       string      `json:"group"`
	TrafficType TrafficType `json:"traffic_type"`
	TrafficSize int64       `json:"traffic_size"`
	ExpiryDays  int         `json:"expiry_days"`
}

type PortalAssignment struct {
	CustomerID string    `json:"customer_id" gorm:"type:uuid;primaryKey"`
	Node       string    `json:"node" gorm:"primaryKey"`
	Desired    bool      `json:"desired"`
	Remove     bool      `json:"remove" gorm:"not null;default:false"`
	Version    uint64    `json:"version"`
	Applied    uint64    `json:"applied"`
	Error      string    `json:"error"`
	RetryAt    time.Time `json:"-" gorm:"index"`
}
