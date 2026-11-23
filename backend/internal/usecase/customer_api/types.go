package customer

import (
	"errors"
	"time"

	"github.com/mmtaee/ocserv-dashboard/backend/internal/models"
	"github.com/mmtaee/ocserv-dashboard/backend/internal/repository"
)

var ErrInvalidCredentials = errors.New("invalid username or password")
var ErrInvalidToken = errors.New("invalid token")

type Credentials struct {
	Username string `json:"username" validate:"required,min=2,max=32"`
	Password string `json:"password" validate:"required,min=2,max=32"`
}

type Customer struct {
	Owner                          string             `json:"owner"`
	Username                       string             `json:"username"`
	IsLocked                       bool               `json:"is_locked"`
	ExpiryMode                     models.ExpiryMode  `json:"expiry_mode"`
	ExpireAt                       *time.Time         `json:"expire_at"`
	ExpireDaysAfterFirstConnection *int               `json:"expire_days_after_first_connection"`
	FirstConnectedAt               *time.Time         `json:"first_connected_at"`
	DeactivatedAt                  *time.Time         `json:"deactivated_at"`
	TrafficType                    models.TrafficType `json:"traffic_type"`
	TrafficSize                    int64              `json:"traffic_size"`
	RunningRx                      int                `json:"running_rx"`
	RunningTx                      int                `json:"running_tx"`
}

type Usage struct {
	DateStart  time.Time                  `json:"date_start"`
	DateEnd    time.Time                  `json:"date_end"`
	Bandwidths repository.TotalBandwidths `json:"bandwidths"`
}

type Summary struct {
	OcservUser Customer `json:"ocserv_user"`
	Usage      Usage    `json:"usage"`
}

type LoginResponse struct {
	User      Customer  `json:"user" validate:"required"`
	Token     string    `json:"token" validate:"required"`
	ExpiresAt time.Time `json:"expires_at" validate:"required"`
}

type ChangePasswordInput struct {
	Password string `json:"password" validate:"required,min=2,max=32"`
}

type DateRange struct {
	DateStart string `json:"date_start" query:"date_start" validate:"omitempty" example:"2025-01-31"`
	DateEnd   string `json:"date_end" query:"date_end" validate:"omitempty" example:"2025-12-31"`
}

type Activities struct {
	Logs  *[]models.OcservUserSessionLog
	Total int64
}

type CiscoSetup struct {
	Radius              bool   `json:"radius,omitempty"`
	ConnectionCreateURI string `json:"connection_create_uri"`
	ConnectionName      string `json:"connection_name"`
	ServerAddress       string `json:"server_address"`
	ServerPort          int    `json:"server_port"`
}
