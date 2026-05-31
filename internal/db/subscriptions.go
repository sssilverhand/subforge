package db

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type SubscriptionStore struct {
	pool *pgxpool.Pool
}

func NewSubscriptionStore(pool *pgxpool.Pool) *SubscriptionStore {
	return &SubscriptionStore{pool: pool}
}

// GetByToken fetches a subscription with its enabled inbounds and node info.
func (s *SubscriptionStore) GetByToken(ctx context.Context, token string) (*Subscription, error) {
	const q = `
		SELECT id, token, name, plan_id, uuid, hy2_password,
		       traffic_limit_bytes, traffic_used_bytes, expires_at,
		       is_enabled, is_traffic_exceeded, is_expired,
		       created_by, created_at, updated_at, last_used_at
		FROM subscriptions
		WHERE token = $1`

	row := s.pool.QueryRow(ctx, q, token)
	sub, err := scanSubscription(row)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return sub, err
}

// GetByID fetches a subscription by primary key.
func (s *SubscriptionStore) GetByID(ctx context.Context, id uuid.UUID) (*Subscription, error) {
	const q = `
		SELECT id, token, name, plan_id, uuid, hy2_password,
		       traffic_limit_bytes, traffic_used_bytes, expires_at,
		       is_enabled, is_traffic_exceeded, is_expired,
		       created_by, created_at, updated_at, last_used_at
		FROM subscriptions
		WHERE id = $1`

	row := s.pool.QueryRow(ctx, q, id)
	sub, err := scanSubscription(row)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return sub, err
}

// GetByHy2Password fetches a subscription by its hysteria2 password.
// Used by the hysteria2 HTTP auth backend on every incoming connection.
func (s *SubscriptionStore) GetByHy2Password(ctx context.Context, password string) (*Subscription, error) {
	const q = `
		SELECT id, token, name, plan_id, uuid, hy2_password,
		       traffic_limit_bytes, traffic_used_bytes, expires_at,
		       is_enabled, is_traffic_exceeded, is_expired,
		       created_by, created_at, updated_at, last_used_at
		FROM subscriptions
		WHERE hy2_password = $1`
	row := s.pool.QueryRow(ctx, q, password)
	sub, err := scanSubscription(row)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return sub, err
}

type CreateSubscriptionParams struct {
	Token             string
	Name              *string
	PlanID            *uuid.UUID
	UUID              uuid.UUID
	Hy2Password       string
	TrafficLimitBytes *int64
	ExpiresAt         *time.Time
	CreatedBy         *uuid.UUID
}

func (s *SubscriptionStore) Create(ctx context.Context, p CreateSubscriptionParams) (*Subscription, error) {
	const q = `
		INSERT INTO subscriptions
		    (token, name, plan_id, uuid, hy2_password, traffic_limit_bytes, expires_at, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, token, name, plan_id, uuid, hy2_password,
		          traffic_limit_bytes, traffic_used_bytes, expires_at,
		          is_enabled, is_traffic_exceeded, is_expired,
		          created_by, created_at, updated_at, last_used_at`

	row := s.pool.QueryRow(ctx, q,
		p.Token, p.Name, p.PlanID, p.UUID, p.Hy2Password,
		p.TrafficLimitBytes, p.ExpiresAt, p.CreatedBy,
	)
	return scanSubscription(row)
}

// SetEnabled enables or disables a subscription.
func (s *SubscriptionStore) SetEnabled(ctx context.Context, id uuid.UUID, enabled bool) error {
	const q = `UPDATE subscriptions SET is_enabled = $2, updated_at = NOW() WHERE id = $1`
	_, err := s.pool.Exec(ctx, q, id, enabled)
	return err
}

// AddTraffic atomically adds bytes to traffic_used_bytes and sets exceeded flag if over limit.
func (s *SubscriptionStore) AddTraffic(ctx context.Context, id uuid.UUID, bytes int64) error {
	const q = `
		UPDATE subscriptions
		SET
		    traffic_used_bytes  = traffic_used_bytes + $2,
		    is_traffic_exceeded = CASE
		        WHEN traffic_limit_bytes IS NOT NULL
		             AND (traffic_used_bytes + $2) >= traffic_limit_bytes
		        THEN TRUE
		        ELSE is_traffic_exceeded
		    END,
		    updated_at = NOW()
		WHERE id = $1`
	_, err := s.pool.Exec(ctx, q, id, bytes)
	return err
}

// MarkExpired sets is_expired=true for all subscriptions past their expires_at.
// Returns the IDs of newly expired subscriptions so the caller can clean up binaries.
func (s *SubscriptionStore) MarkExpired(ctx context.Context) ([]uuid.UUID, error) {
	const q = `
		UPDATE subscriptions
		SET is_expired = TRUE, is_enabled = FALSE, updated_at = NOW()
		WHERE expires_at IS NOT NULL
		  AND expires_at < NOW()
		  AND is_expired = FALSE
		RETURNING id`

	rows, err := s.pool.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// MarkTrafficExceeded disables subscriptions that are over traffic limit.
// Returns affected IDs.
func (s *SubscriptionStore) MarkTrafficExceeded(ctx context.Context) ([]uuid.UUID, error) {
	const q = `
		UPDATE subscriptions
		SET is_traffic_exceeded = TRUE, is_enabled = FALSE, updated_at = NOW()
		WHERE traffic_limit_bytes IS NOT NULL
		  AND traffic_used_bytes >= traffic_limit_bytes
		  AND is_enabled = TRUE
		RETURNING id`

	rows, err := s.pool.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// UpdateLastUsed sets last_used_at without touching updated_at (avoid noise).
func (s *SubscriptionStore) UpdateLastUsed(ctx context.Context, id uuid.UUID, t time.Time) error {
	const q = `UPDATE subscriptions SET last_used_at = $2 WHERE id = $1`
	_, err := s.pool.Exec(ctx, q, id, t)
	return err
}

// ListAll returns all subscriptions (for admin panel, paginated).
func (s *SubscriptionStore) ListAll(ctx context.Context, limit, offset int) ([]Subscription, error) {
	const q = `
		SELECT id, token, name, plan_id, uuid, hy2_password,
		       traffic_limit_bytes, traffic_used_bytes, expires_at,
		       is_enabled, is_traffic_exceeded, is_expired,
		       created_by, created_at, updated_at, last_used_at
		FROM subscriptions
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2`

	rows, err := s.pool.Query(ctx, q, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var subs []Subscription
	for rows.Next() {
		sub, err := scanSubscription(rows)
		if err != nil {
			return nil, err
		}
		subs = append(subs, *sub)
	}
	return subs, rows.Err()
}

// GetInboundsForSubscription returns inbounds (with node info) for a subscription.
func (s *SubscriptionStore) GetInboundsForSubscription(ctx context.Context, subID uuid.UUID) ([]InboundWithNode, error) {
	const q = `
		SELECT i.id, i.node_id, i.tag, i.protocol, i.port, i.settings, i.is_active,
		       n.public_host, n.name as node_name,
		       n.xray_api_addr, n.xray_api_tls,
		       n.hy2_api_url, n.hy2_api_secret
		FROM subscription_inbounds si
		JOIN inbounds i  ON i.id  = si.inbound_id
		JOIN nodes    n  ON n.id  = i.node_id
		WHERE si.subscription_id = $1
		  AND i.is_active = TRUE
		  AND n.is_active = TRUE`

	rows, err := s.pool.Query(ctx, q, subID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []InboundWithNode
	for rows.Next() {
		var r InboundWithNode
		err := rows.Scan(
			&r.ID, &r.NodeID, &r.Tag, &r.Protocol, &r.Port, &r.Settings, &r.IsActive,
			&r.PublicHost, &r.NodeName,
			&r.XrayAPIAddr, &r.XrayAPITLS,
			&r.Hy2APIURL, &r.Hy2APISecret,
		)
		if err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// AddInbound links an inbound to a subscription.
func (s *SubscriptionStore) AddInbound(ctx context.Context, subID, inboundID uuid.UUID) error {
	const q = `
		INSERT INTO subscription_inbounds (subscription_id, inbound_id)
		VALUES ($1, $2)
		ON CONFLICT DO NOTHING`
	_, err := s.pool.Exec(ctx, q, subID, inboundID)
	return err
}

// RemoveInbound unlinks an inbound from a subscription.
func (s *SubscriptionStore) RemoveInbound(ctx context.Context, subID, inboundID uuid.UUID) error {
	const q = `DELETE FROM subscription_inbounds WHERE subscription_id=$1 AND inbound_id=$2`
	_, err := s.pool.Exec(ctx, q, subID, inboundID)
	return err
}

// InboundWithNode is an inbound row joined with its node.
type InboundWithNode struct {
	ID          uuid.UUID
	NodeID      uuid.UUID
	Tag         string
	Protocol    string
	Port        int
	Settings    []byte
	IsActive    bool
	PublicHost  string
	NodeName    string
	XrayAPIAddr *string
	XrayAPITLS  bool
	Hy2APIURL   *string
	Hy2APISecret *string
}

// Exec runs a raw query against the pool. Used by service layer for complex updates.
func (s *SubscriptionStore) Exec(ctx context.Context, sql string, args ...any) error {
	_, err := s.pool.Exec(ctx, sql, args...)
	return err
}

// ResetTraffic zeroes traffic counters and clears the exceeded flag.
func (s *SubscriptionStore) ResetTraffic(ctx context.Context, id uuid.UUID) error {
	const q = `UPDATE subscriptions
		SET traffic_used_bytes = 0, is_traffic_exceeded = FALSE, updated_at = NOW()
		WHERE id = $1`
	_, err := s.pool.Exec(ctx, q, id)
	return err
}

// DeleteSubscription hard-deletes a subscription and cascades to subscription_inbounds.
// Caller must remove the user from xray/hy2 before calling this.
func (s *SubscriptionStore) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM subscriptions WHERE id = $1`, id)
	return err
}

// scanSubscription scans a pgx.Row or pgx.Rows into *Subscription.
// GetByChatID returns the subscription linked to a Telegram chat ID.
func (s *SubscriptionStore) GetByChatID(ctx context.Context, chatID int64) (*Subscription, error) {
	const q = `
		SELECT id, token, name, plan_id, uuid, hy2_password,
		       traffic_limit_bytes, traffic_used_bytes, expires_at,
		       is_enabled, is_traffic_exceeded, is_expired,
		       telegram_chat_id, created_by, created_at, updated_at, last_used_at
		FROM subscriptions WHERE telegram_chat_id = $1`
	row := s.pool.QueryRow(ctx, q, chatID)
	sub, err := scanSubscription(row)
	if err != nil {
		return nil, nil //nolint — no rows = no subscription
	}
	return sub, nil
}

// ListExpiringSoon returns enabled subscriptions expiring within the given duration.
func (s *SubscriptionStore) ListExpiringSoon(ctx context.Context, within time.Duration) ([]Subscription, error) {
	const q = `
		SELECT id, token, name, plan_id, uuid, hy2_password,
		       traffic_limit_bytes, traffic_used_bytes, expires_at,
		       is_enabled, is_traffic_exceeded, is_expired,
		       telegram_chat_id, created_by, created_at, updated_at, last_used_at
		FROM subscriptions
		WHERE is_enabled = TRUE AND is_expired = FALSE
		  AND expires_at IS NOT NULL
		  AND expires_at BETWEEN NOW() AND NOW() + $1::interval`
	rows, err := s.pool.Query(ctx, q, within.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var subs []Subscription
	for rows.Next() {
		sub, err := scanSubscription(rows)
		if err != nil {
			return nil, err
		}
		subs = append(subs, *sub)
	}
	return subs, rows.Err()
}

func scanSubscription(row interface {
	Scan(dest ...any) error
}) (*Subscription, error) {
	var sub Subscription
	err := row.Scan(
		&sub.ID, &sub.Token, &sub.Name, &sub.PlanID,
		&sub.UUID, &sub.Hy2Password,
		&sub.TrafficLimitBytes, &sub.TrafficUsedBytes, &sub.ExpiresAt,
		&sub.IsEnabled, &sub.IsTrafficExceeded, &sub.IsExpired,
		&sub.TelegramChatID, &sub.CreatedBy, &sub.CreatedAt, &sub.UpdatedAt, &sub.LastUsedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("scan subscription: %w", err)
	}
	return &sub, nil
}
