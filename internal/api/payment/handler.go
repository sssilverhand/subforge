package payment

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/sssilverhand/subforge/internal/core/subscription"
	"github.com/sssilverhand/subforge/internal/db"
	paymentsvc "github.com/sssilverhand/subforge/internal/payment"
)

// Notifier sends Telegram notifications (implemented by bot.Bot).
type Notifier interface {
	NotifyPaymentSuccess(ctx context.Context, chatID int64, subToken string)
}

type Handler struct {
	svc    *paymentsvc.Service
	subSvc *subscription.Service
	plans  *db.PlanStore
	notify Notifier
}

func NewHandler(
	svc *paymentsvc.Service,
	subSvc *subscription.Service,
	plans *db.PlanStore,
	notify Notifier,
) *Handler {
	return &Handler{svc: svc, subSvc: subSvc, plans: plans, notify: notify}
}

func (h *Handler) Routes() http.Handler {
	r := chi.NewRouter()
	// Webhook endpoints — called by payment providers
	r.Post("/webhook/{provider}", h.webhook)
	// Invoice creation — called by bot or admin API
	r.Post("/invoice", h.createInvoice)
	return r
}

// webhook handles incoming payment notifications from providers.
func (h *Handler) webhook(w http.ResponseWriter, r *http.Request) {
	provider := chi.URLParam(r, "provider")

	body, err := io.ReadAll(io.LimitReader(r.Body, 64*1024))
	if err != nil {
		http.Error(w, "read body", http.StatusBadRequest)
		return
	}

	sig := r.Header.Get("X-Signature")

	inv, paid, err := h.svc.HandleWebhook(r.Context(), provider, body, sig)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if paid && inv.SubscriptionID != nil {
		h.activateSubscription(r.Context(), inv)
	}

	w.WriteHeader(http.StatusOK)
}

// createInvoice creates a payment invoice (called by bot after plan selection).
func (h *Handler) createInvoice(w http.ResponseWriter, r *http.Request) {
	var body struct {
		PlanID         uuid.UUID  `json:"plan_id"`
		Provider       string     `json:"provider"`
		TelegramChatID *int64     `json:"telegram_chat_id"`
		ReturnURL      string     `json:"return_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, "invalid request", http.StatusBadRequest)
		return
	}

	plan, err := h.plans.GetByID(r.Context(), body.PlanID)
	if err != nil || plan == nil || plan.PriceUSD == nil {
		writeError(w, "plan not found or free", http.StatusBadRequest)
		return
	}

	result, err := h.svc.CreateInvoice(r.Context(), paymentsvc.CreateInvoiceParams{
		PlanID:         body.PlanID,
		PlanName:       plan.Name,
		PriceUSD:       *plan.PriceUSD,
		Provider:       body.Provider,
		ReturnURL:      body.ReturnURL,
		TelegramChatID: body.TelegramChatID,
	})
	if err != nil {
		writeError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"invoice_id":  result.Invoice.ID,
		"payment_url": result.PaymentURL,
		"expires_at":  result.Invoice.ExpiresAt,
		"is_stars":    result.IsStars,
	})
}

// activateSubscription creates or extends a subscription after successful payment.
func (h *Handler) activateSubscription(ctx context.Context, inv *db.Invoice) {
	if inv.SubscriptionID == nil || inv.PlanID == nil {
		return
	}

	plan, err := h.plans.GetByID(ctx, *inv.PlanID)
	if err != nil || plan == nil {
		return
	}

	sub, err := h.subSvc.GetByID(ctx, *inv.SubscriptionID)
	if err != nil || sub == nil {
		return
	}

	// Calculate new expiry
	var expiresAt *time.Time
	if plan.DurationDays != nil {
		base := time.Now()
		// If already active and not expired, extend from current expiry
		if sub.ExpiresAt != nil && sub.ExpiresAt.After(base) {
			base = *sub.ExpiresAt
		}
		t := base.AddDate(0, 0, *plan.DurationDays)
		expiresAt = &t
	}

	// Reset traffic if plan specifies a limit
	if err := h.subSvc.RenewAfterPayment(ctx, *inv.SubscriptionID, plan.TrafficLimitBytes, expiresAt); err != nil {
		return
	}

	// Notify user via bot
	if h.notify != nil && sub.TelegramChatID != nil {
		h.notify.NotifyPaymentSuccess(ctx, *sub.TelegramChatID, sub.Token)
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, msg string, code int) {
	writeJSON(w, code, map[string]string{"error": msg})
}
