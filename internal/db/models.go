package db

import (
	"time"

	"github.com/google/uuid"
)

type AdminUser struct {
	ID             uuid.UUID
	Username       string
	PasswordHash   string
	Role           string // super_admin | admin | operator
	IsActive       bool
	TelegramChatID *int64
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type APIToken struct {
	ID        uuid.UUID
	Name      string
	TokenHash string
	Role      string
	CreatedBy *uuid.UUID
	ExpiresAt *time.Time
	CreatedAt time.Time
}

type Node struct {
	ID             uuid.UUID
	Name           string
	XrayAPIAddr    *string
	XrayAPITLS     bool
	Hy2APIURL      *string
	Hy2APISecret   *string
	PublicHost     string
	AgentURL       *string
	AgentSecret    *string
	AgentVersion   *string
	AgentLastSeen  *time.Time
	IsActive       bool
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type Inbound struct {
	ID        uuid.UUID
	NodeID    uuid.UUID
	Tag       string
	Protocol  string // vless-xhttp | vless-reality | vless-ws | hysteria2
	Port      int
	Settings  []byte // JSONB
	IsActive  bool
	CreatedAt time.Time
}

type Plan struct {
	ID                 uuid.UUID
	Name               string
	Description        *string
	PriceUSD           *float64
	TrafficLimitBytes  *int64
	DurationDays       *int
	IsActive           bool
	CreatedAt          time.Time
}

type Subscription struct {
	ID                uuid.UUID
	Token             string
	Name              *string
	PlanID            *uuid.UUID
	UUID              uuid.UUID
	Hy2Password       string
	TrafficLimitBytes *int64
	TrafficUsedBytes  int64
	ExpiresAt         *time.Time
	IsEnabled         bool
	IsTrafficExceeded bool
	IsExpired         bool
	TelegramChatID    *int64
	CreatedBy         *uuid.UUID
	CreatedAt         time.Time
	UpdatedAt         time.Time
	LastUsedAt        *time.Time
}

type TrafficSnapshot struct {
	ID             int64
	SubscriptionID uuid.UUID
	InboundID      uuid.UUID
	BytesUp        int64
	BytesDown      int64
	SnapshotAt     time.Time
}

type Invoice struct {
	ID              uuid.UUID
	SubscriptionID  *uuid.UUID
	PlanID          *uuid.UUID
	AmountUSD       float64
	Currency        string
	Status          string // pending | paid | expired | cancelled
	PaymentProvider *string
	ExternalID      *string
	PaidAt          *time.Time
	ExpiresAt       *time.Time
	CreatedAt       time.Time
}
