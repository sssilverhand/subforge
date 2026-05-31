package cryptomus

import (
	"bytes"
	"context"
	"crypto/md5" //nolint:gosec — required by Cryptomus API spec
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/sssilverhand/subforge/internal/payment"
)

const apiBase = "https://api.cryptomus.com/v1"

// Provider implements payment.Provider for Cryptomus.
// Docs: https://doc.cryptomus.com/payments/creating-invoice
type Provider struct {
	merchantID string
	apiKey     string
	httpClient *http.Client
}

func New(merchantID, apiKey string) *Provider {
	return &Provider{
		merchantID: merchantID,
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

func (p *Provider) Name() string { return "cryptomus" }

type createPayload struct {
	Amount   string `json:"amount"`
	Currency string `json:"currency"`
	OrderID  string `json:"order_id"`
	Network  string `json:"network,omitempty"`
	URLReturn string `json:"url_return,omitempty"`
	Lifetime  int   `json:"lifetime,omitempty"` // seconds
}

type createResponse struct {
	State  int `json:"state"`
	Result struct {
		UUID    string `json:"uuid"`
		URL     string `json:"url"`
		ExpiredAt int64 `json:"expired_at"`
	} `json:"result"`
	Message string `json:"message"`
}

func (p *Provider) CreateInvoice(ctx context.Context, req payment.CreateRequest) (*payment.CreateResult, error) {
	body := createPayload{
		Amount:    fmt.Sprintf("%.2f", req.Amount),
		Currency:  req.Currency,
		OrderID:   req.OrderID,
		URLReturn: req.ReturnURL,
		Lifetime:  3600, // 1 hour to pay
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	sig := p.sign(bodyBytes)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		apiBase+"/payment", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("merchant", p.merchantID)
	httpReq.Header.Set("sign", sig)

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("cryptomus create invoice: %w", err)
	}
	defer resp.Body.Close()

	var cr createResponse
	if err := json.NewDecoder(resp.Body).Decode(&cr); err != nil {
		return nil, fmt.Errorf("cryptomus decode response: %w", err)
	}
	if cr.State != 0 || cr.Result.UUID == "" {
		return nil, fmt.Errorf("cryptomus error: %s", cr.Message)
	}

	expiresAt := time.Unix(cr.Result.ExpiredAt, 0)
	return &payment.CreateResult{
		PaymentURL: cr.Result.URL,
		ExternalID: cr.Result.UUID,
		ExpiresAt:  expiresAt,
	}, nil
}

// webhookPayload is the structure Cryptomus sends to our webhook URL.
type webhookPayload struct {
	Type       string `json:"type"`
	UUID       string `json:"uuid"`
	OrderID    string `json:"order_id"`
	Amount     string `json:"amount"`
	Status     string `json:"status"`
	Sign       string `json:"sign"`
}

func (p *Provider) VerifyWebhook(payload []byte, _ string) (*payment.WebhookEvent, error) {
	var wh webhookPayload
	if err := json.Unmarshal(payload, &wh); err != nil {
		return nil, fmt.Errorf("cryptomus webhook decode: %w", err)
	}

	// Cryptomus embeds the sign in the payload body.
	// Verification: sign the payload with sign field set to null, compare.
	if err := p.verifySign(payload, wh.Sign); err != nil {
		return nil, fmt.Errorf("cryptomus webhook signature invalid: %w", err)
	}

	status := mapStatus(wh.Status)
	var amount float64
	fmt.Sscanf(wh.Amount, "%f", &amount)

	return &payment.WebhookEvent{
		OrderID:    wh.OrderID,
		ExternalID: wh.UUID,
		Status:     status,
		Amount:     amount,
	}, nil
}

// sign computes the Cryptomus request signature: MD5(base64(body) + api_key)
func (p *Provider) sign(body []byte) string {
	encoded := base64.StdEncoding.EncodeToString(body)
	//nolint:gosec
	h := md5.Sum([]byte(encoded + p.apiKey))
	return hex.EncodeToString(h[:])
}

// verifySign verifies the webhook signature by nullifying the sign field.
func (p *Provider) verifySign(payload []byte, receivedSign string) error {
	// Replace "sign":"<value>" with "sign":null in the raw JSON
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(payload, &raw); err != nil {
		return err
	}
	raw["sign"] = json.RawMessage("null")
	modified, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	expected := p.sign(modified)
	if !strings.EqualFold(expected, receivedSign) {
		return fmt.Errorf("signature mismatch")
	}
	return nil
}

func mapStatus(s string) payment.Status {
	switch s {
	case "paid", "paid_over":
		return payment.StatusPaid
	case "cancel":
		return payment.StatusCancelled
	case "system_fail", "wrong_amount", "process", "check":
		return payment.StatusPending
	default:
		return payment.StatusExpired
	}
}
