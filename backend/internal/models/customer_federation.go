package models

import "time"

type PortalCustomer struct {
	ID               string      `json:"id" gorm:"type:uuid;primaryKey;default:uuidv7()"`
	Username         string      `json:"username" gorm:"not null;uniqueIndex"`
	Email            string      `json:"email"`
	LoginLockedUntil *time.Time  `json:"login_locked_until,omitempty"`
	LastLoginLockAt  *time.Time  `json:"last_login_lock_at,omitempty"`
	PasswordHash     []byte      `json:"-" gorm:"not null"`
	Node             string      `json:"node"`
	TrafficType      TrafficType `json:"traffic_type"`
	TrafficSize      int64       `json:"traffic_size"`
	ExpiresAt        *time.Time  `json:"expires_at"`
	DefaultNode      string      `json:"default_node"`
	PolicyGroupID    *string     `json:"policy_group_id" gorm:"type:uuid"`
	Locked           bool        `json:"locked"`
	// DeletedAt is set while permanent deletion is waiting for node confirmation.
	DeletedAt *time.Time `json:"deleted_at,omitempty"`
	CreatedAt time.Time
}

type PortalLoginProtection struct {
	ID            bool `json:"-" gorm:"primaryKey"`
	MaxFailures   int  `json:"max_failures"`
	WindowMinutes int  `json:"window_minutes"`
	LockMinutes   int  `json:"lock_minutes"`
}

type PortalPolicyGroup struct {
	ID                  string   `json:"id" gorm:"type:uuid;primaryKey;default:uuidv7()"`
	Name                string   `json:"name"`
	PrimaryDNS          string   `json:"primary_dns"`
	SecondaryDNS        string   `json:"secondary_dns"`
	Nodes               []string `json:"nodes" gorm:"serializer:json;type:jsonb"`
	Routes              []string `json:"routes" gorm:"serializer:json;type:jsonb"`
	UploadKBPerSecond   uint32   `json:"upload_kb_per_second"`
	DownloadKBPerSecond uint32   `json:"download_kb_per_second"`
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
