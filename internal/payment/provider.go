package payment

import (
	"context"
	"time"
)

// Status represents the current state of a payment.
type Status string

const (
	StatusPending   Status = "pending"
	StatusPaid      Status = "paid"
	StatusExpired   Status = "expired"
	StatusCancelled Status = "cancelled"
)

// CreateRequest is the input for creating a payment invoice.
type CreateRequest struct {
	OrderID     string  // our DB invoice UUID
	Amount      float64 // in USD
	Currency    string  // USD, USDT, etc.
	Description string
	ReturnURL   string // redirect after payment
}

// CreateResult is returned after successfully creating a payment.
type CreateResult struct {
	PaymentURL string
	ExternalID string    // provider's own ID
	ExpiresAt  time.Time
}

// WebhookEvent is parsed from a provider's incoming webhook.
type WebhookEvent struct {
	OrderID    string
	ExternalID string
	Status     Status
	Amount     float64
}

// Provider is the interface every payment backend must implement.
type Provider interface {
	// Name returns the provider identifier (used in DB and routing).
	Name() string
	// CreateInvoice creates a payment and returns a URL for the user.
	CreateInvoice(ctx context.Context, req CreateRequest) (*CreateResult, error)
	// VerifyWebhook parses and verifies an incoming webhook payload.
	// Returns an error if the signature is invalid.
	VerifyWebhook(payload []byte, signature string) (*WebhookEvent, error)
}

// Registry holds registered payment providers keyed by name.
type Registry struct {
	providers map[string]Provider
}

func NewRegistry() *Registry {
	return &Registry{providers: make(map[string]Provider)}
}

func (r *Registry) Register(p Provider) {
	r.providers[p.Name()] = p
}

func (r *Registry) Get(name string) (Provider, bool) {
	p, ok := r.providers[name]
	return p, ok
}

func (r *Registry) Names() []string {
	names := make([]string, 0, len(r.providers))
	for n := range r.providers {
		names = append(names, n)
	}
	return names
}
