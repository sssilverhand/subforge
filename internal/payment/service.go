package payment

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sssilverhand/subforge/internal/db"
)

// Service handles invoice creation and payment processing.
type Service struct {
	pool     *pgxpool.Pool
	subs     *db.SubscriptionStore
	registry *Registry
}

func NewService(pool *pgxpool.Pool, subs *db.SubscriptionStore, registry *Registry) *Service {
	return &Service{pool: pool, subs: subs, registry: registry}
}

// CreateInvoiceParams describes what's needed to start a payment flow.
type CreateInvoiceParams struct {
	SubscriptionID *uuid.UUID // nil = new subscription will be created on payment
	PlanID         uuid.UUID
	PlanName       string
	PriceUSD       float64
	Provider       string
	ReturnURL      string
	TelegramChatID *int64 // for bot notifications
}

// CreateInvoiceResult contains the DB invoice and payment URL.
type CreateInvoiceResult struct {
	Invoice    *db.Invoice
	PaymentURL string
	// For Telegram Stars: use StarParams instead of URL
	IsStars    bool
	StarParams any // *stars.StarInvoiceParams
}

func (s *Service) CreateInvoice(ctx context.Context, p CreateInvoiceParams) (*CreateInvoiceResult, error) {
	provider, ok := s.registry.Get(p.Provider)
	if !ok {
		return nil, fmt.Errorf("unknown payment provider: %s", p.Provider)
	}

	orderID := uuid.New()

	result, err := provider.CreateInvoice(ctx, CreateRequest{
		OrderID:     orderID.String(),
		Amount:      p.PriceUSD,
		Currency:    "USD",
		Description: "SubForge: " + p.PlanName,
		ReturnURL:   p.ReturnURL,
	})
	if err != nil {
		return nil, fmt.Errorf("create invoice with %s: %w", p.Provider, err)
	}

	// Persist invoice to DB
	inv, err := s.createInvoiceDB(ctx, createInvoiceDBParams{
		ID:             orderID,
		SubscriptionID: p.SubscriptionID,
		PlanID:         &p.PlanID,
		AmountUSD:      p.PriceUSD,
		Provider:       p.Provider,
		ExternalID:     result.ExternalID,
		ExpiresAt:      result.ExpiresAt,
	})
	if err != nil {
		return nil, fmt.Errorf("persist invoice: %w", err)
	}

	return &CreateInvoiceResult{
		Invoice:    inv,
		PaymentURL: result.PaymentURL,
		IsStars:    p.Provider == "telegram_stars",
	}, nil
}

// HandleWebhook processes an incoming payment notification.
// Returns the updated invoice and whether a subscription was activated.
func (s *Service) HandleWebhook(ctx context.Context, providerName string, payload []byte, sig string) (*db.Invoice, bool, error) {
	provider, ok := s.registry.Get(providerName)
	if !ok {
		return nil, false, fmt.Errorf("unknown provider: %s", providerName)
	}

	event, err := provider.VerifyWebhook(payload, sig)
	if err != nil {
		return nil, false, fmt.Errorf("webhook verify: %w", err)
	}

	inv, err := s.getInvoiceByOrderID(ctx, event.OrderID)
	if err != nil || inv == nil {
		return nil, false, fmt.Errorf("invoice not found: %s", event.OrderID)
	}

	if inv.Status == "paid" {
		// Already processed — idempotent
		return inv, false, nil
	}

	switch event.Status {
	case StatusPaid:
		inv, err = s.markPaid(ctx, inv.ID)
		if err != nil {
			return nil, false, err
		}
		// If no subscription yet, we'll create one — caller handles this
		return inv, true, nil

	case StatusExpired, StatusCancelled:
		inv, err = s.updateStatus(ctx, inv.ID, string(event.Status))
		return inv, false, err
	}

	return inv, false, nil
}

// ─── DB operations ───────────────────────────────────────────────────────────

type createInvoiceDBParams struct {
	ID             uuid.UUID
	SubscriptionID *uuid.UUID
	PlanID         *uuid.UUID
	AmountUSD      float64
	Provider       string
	ExternalID     string
	ExpiresAt      time.Time
}

func (s *Service) createInvoiceDB(ctx context.Context, p createInvoiceDBParams) (*db.Invoice, error) {
	const q = `
		INSERT INTO invoices
		    (id, subscription_id, plan_id, amount_usd, currency, status,
		     payment_provider, external_id, expires_at)
		VALUES ($1, $2, $3, $4, 'USD', 'pending', $5, $6, $7)
		RETURNING id, subscription_id, plan_id, amount_usd, currency, status,
		          payment_provider, external_id, paid_at, expires_at, created_at`

	var inv db.Invoice
	err := s.pool.QueryRow(ctx, q,
		p.ID, p.SubscriptionID, p.PlanID, p.AmountUSD,
		p.Provider, p.ExternalID, p.ExpiresAt,
	).Scan(
		&inv.ID, &inv.SubscriptionID, &inv.PlanID,
		&inv.AmountUSD, &inv.Currency, &inv.Status,
		&inv.PaymentProvider, &inv.ExternalID,
		&inv.PaidAt, &inv.ExpiresAt, &inv.CreatedAt,
	)
	return &inv, err
}

func (s *Service) getInvoiceByOrderID(ctx context.Context, orderID string) (*db.Invoice, error) {
	id, err := uuid.Parse(orderID)
	if err != nil {
		return nil, fmt.Errorf("invalid order ID: %w", err)
	}
	const q = `SELECT id, subscription_id, plan_id, amount_usd, currency, status,
		payment_provider, external_id, paid_at, expires_at, created_at
		FROM invoices WHERE id = $1`
	var inv db.Invoice
	err = s.pool.QueryRow(ctx, q, id).Scan(
		&inv.ID, &inv.SubscriptionID, &inv.PlanID,
		&inv.AmountUSD, &inv.Currency, &inv.Status,
		&inv.PaymentProvider, &inv.ExternalID,
		&inv.PaidAt, &inv.ExpiresAt, &inv.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &inv, nil
}

func (s *Service) markPaid(ctx context.Context, id uuid.UUID) (*db.Invoice, error) {
	return s.updateStatus(ctx, id, "paid")
}

func (s *Service) updateStatus(ctx context.Context, id uuid.UUID, status string) (*db.Invoice, error) {
	const q = `UPDATE invoices SET status = $2,
		paid_at = CASE WHEN $2 = 'paid' THEN NOW() ELSE paid_at END
		WHERE id = $1
		RETURNING id, subscription_id, plan_id, amount_usd, currency, status,
		          payment_provider, external_id, paid_at, expires_at, created_at`
	var inv db.Invoice
	err := s.pool.QueryRow(ctx, q, id, status).Scan(
		&inv.ID, &inv.SubscriptionID, &inv.PlanID,
		&inv.AmountUSD, &inv.Currency, &inv.Status,
		&inv.PaymentProvider, &inv.ExternalID,
		&inv.PaidAt, &inv.ExpiresAt, &inv.CreatedAt,
	)
	return &inv, err
}
