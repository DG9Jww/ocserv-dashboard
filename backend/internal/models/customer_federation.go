package models

import "time"

const PortalManagedPassword = "!ocserv-managed-by-unified-customer-portal!"

type PortalCustomer struct {
	ID           string   `json:"id" gorm:"type:uuid;primaryKey;default:uuidv7()"`
	Username     string   `json:"username" gorm:"not null;uniqueIndex"`
	PasswordHash []byte   `json:"-" gorm:"not null"`
	PasswordBox  []byte   `json:"-" gorm:"not null"`
	Nodes        []string `json:"nodes" gorm:"serializer:json;type:jsonb"`
	Groups       []string `json:"groups" gorm:"serializer:json;type:jsonb"`
	DefaultNode  string   `json:"default_node"`
	CreatedAt    time.Time
}

type PortalNodeGroup struct {
	ID    string   `json:"id" gorm:"type:uuid;primaryKey;default:uuidv7()"`
	Name  string   `json:"name" gorm:"not null;uniqueIndex"`
	Nodes []string `json:"nodes" gorm:"serializer:json;type:jsonb"`
}

type PortalNodeDefault struct {
	Node             string      `json:"node" gorm:"primaryKey"`
	Group            string      `json:"group"`
	TrafficType      TrafficType `json:"traffic_type"`
	TrafficSize      int64       `json:"traffic_size"`
	ExpiryDays       int         `json:"expiry_days"`
	CertificateGroup string      `json:"certificate_group"`
	TrustVersion     uint64      `json:"-" gorm:"not null;default:1"`
}

type PortalAssignment struct {
	CustomerID string    `json:"customer_id" gorm:"type:uuid;primaryKey"`
	Node       string    `json:"node" gorm:"primaryKey"`
	Desired    bool      `json:"desired"`
	Version    uint64    `json:"version"`
	Applied    uint64    `json:"applied"`
	OcservID   uint      `json:"-"`
	Error      string    `json:"error"`
	RetryAt    time.Time `json:"-" gorm:"index"`
}

type PortalLocalAccount struct {
	CustomerID string `gorm:"type:uuid;primaryKey"`
	OcservID   uint   `gorm:"not null;uniqueIndex:portal_local_ocserv_id,where:ocserv_id > 0"`
	Configured bool
	Version    uint64
	Desired    bool
}

type PortalSigningCA struct {
	Scope string `gorm:"primaryKey"`
	Cert  []byte `gorm:"not null"`
	CRL   []byte `gorm:"not null"`
	Key   []byte `gorm:"not null"`
}

type PortalClientCertificate struct {
	CustomerID string `gorm:"type:uuid;primaryKey"`
	Scope      string `gorm:"primaryKey"`
	Cert       []byte `gorm:"not null"`
	Key        []byte `gorm:"not null"`
}
