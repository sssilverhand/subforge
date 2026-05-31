package stars

import (
	"context"
	"fmt"
	"time"

	"github.com/sssilverhand/subforge/internal/payment"
)

// Provider implements payment.Provider for Telegram Stars.
//
// Telegram Stars payments happen entirely inside the bot:
// 1. Bot sends sendInvoice → user taps Pay
// 2. Bot handles pre_checkout_query (must answer in 10s)
// 3. Bot handles successful_payment message
//
// This provider's CreateInvoice is called by the bot handler to build
// the invoice parameters. The actual Telegram API call is done in the bot
// because it requires the user's chat_id.
type Provider struct {
	botToken string
}

func New(botToken string) *Provider {
	return &Provider{botToken: botToken}
}

func (p *Provider) Name() string { return "telegram_stars" }

// StarInvoiceParams contains what the bot needs to call sendInvoice.
type StarInvoiceParams struct {
	Title       string
	Description string
	Payload     string // our order ID
	StarAmount  int    // XTR amount (1 Star = ~0.013 USD at launch)
}

// PriceUSDToStars converts a USD price to Telegram Stars amount.
// Rate is approximate — update as market changes.
// Minimum is 1 Star, maximum is 2500 Stars per transaction.
func PriceUSDToStars(usd float64) int {
	stars := int(usd / 0.013)
	if stars < 1 {
		stars = 1
	}
	if stars > 2500 {
		stars = 2500
	}
	return stars
}

// BuildInvoiceParams builds the parameters for a Telegram Stars sendInvoice call.
func BuildInvoiceParams(orderID, planName string, priceUSD float64) StarInvoiceParams {
	return StarInvoiceParams{
		Title:       "SubForge: " + planName,
		Description: fmt.Sprintf("VPN подписка — %s", planName),
		Payload:     orderID,
		StarAmount:  PriceUSDToStars(priceUSD),
	}
}

// CreateInvoice for Stars is a no-op at the HTTP level — the invoice is
// created via the Telegram Bot API in the bot handler.
// We return a placeholder so the DB invoice record can be created before the bot call.
func (p *Provider) CreateInvoice(_ context.Context, req payment.CreateRequest) (*payment.CreateResult, error) {
	return &payment.CreateResult{
		PaymentURL: "telegram://stars", // bot will use sendInvoice directly
		ExternalID: req.OrderID,        // use our order ID as external ID
		ExpiresAt:  time.Now().Add(24 * time.Hour),
	}, nil
}

// VerifyWebhook for Stars: the Telegram bot receives successful_payment
// messages; we don't use a traditional HTTP webhook.
// The bot handler calls this after receiving the payment to validate the payload.
func (p *Provider) VerifyWebhook(payload []byte, _ string) (*payment.WebhookEvent, error) {
	// payload is the order ID sent as the invoice payload
	orderID := string(payload)
	if orderID == "" {
		return nil, fmt.Errorf("empty order ID in stars payment")
	}
	return &payment.WebhookEvent{
		OrderID:    orderID,
		ExternalID: orderID,
		Status:     payment.StatusPaid,
	}, nil
}
