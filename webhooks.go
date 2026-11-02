package buypower

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// WebhookTopic groups webhook events.
type WebhookTopic string

const (
	TopicGeneral     WebhookTopic = "general"
	TopicCollections WebhookTopic = "collections"
	TopicTransfers   WebhookTopic = "transfers"
	TopicSettlements WebhookTopic = "settlements"
	TopicRefunds     WebhookTopic = "refunds"
)

// Event types delivered in WebhookEvent.Type.
const (
	EventInvoicePaid                     = "invoice.paid"
	EventStaticAccountTransactionCreated = "static_account.transaction.created"
	EventTransferPending                 = "transfer.pending"
	EventTransferPaid                    = "transfer.paid"
	EventTransferFailed                  = "transfer.failed"
	EventRefundInitiated                 = "refund.initiated"
	EventRefundCompleted                 = "refund.completed"
	EventRefundPartiallyCompleted        = "refund.partially_completed"
	EventRefundFailed                    = "refund.failed"
)

// Webhook delivery headers.
const (
	SignatureHeader       = "x-buypower-signature"
	PanboxSignatureHeader = "x-panbox-signature" // identical value, alias
	VersionHeader         = "x-webhook-version"
)

// WebhookSubscription is a topic → URL mapping.
type WebhookSubscription struct {
	Topic          WebhookTopic `json:"topic"`
	URL            string       `json:"url"`
	PayloadVersion string       `json:"payloadVersion"`
	Enabled        bool         `json:"enabled"`
}

// WebhookSubscriptionInput registers one endpoint for a topic.
type WebhookSubscriptionInput struct {
	URL   string       `json:"url"`
	Topic WebhookTopic `json:"topic,omitempty"`
	// PayloadVersion defaults to "2" in this SDK (the server default is "1").
	PayloadVersion string `json:"payloadVersion,omitempty"`
}

// WebhooksService handles /v2/webhooks.
type WebhooksService struct{ c *Client }

// ListSubscriptions lists webhook subscriptions (GET /v2/webhooks/subscriptions).
func (s *WebhooksService) ListSubscriptions(ctx context.Context, opts ...RequestOption) ([]WebhookSubscription, error) {
	page, err := callList[WebhookSubscription](ctx, s.c, request{method: http.MethodGet, path: "/v2/webhooks/subscriptions"}, opts)
	if err != nil {
		return nil, err
	}
	return page.Data, nil
}

// UpsertSubscriptions creates or updates subscriptions by topic
// (PUT /v2/webhooks/subscriptions). Subscriptions are per environment.
// The response is discarded: the server returns raw database rows that echo
// the webhook secret. Read the result back with ListSubscriptions.
func (s *WebhooksService) UpsertSubscriptions(ctx context.Context, subs []WebhookSubscriptionInput, opts ...RequestOption) error {
	if len(subs) == 0 {
		return errors.New("buypower: at least one subscription is required")
	}
	body := make([]WebhookSubscriptionInput, len(subs))
	for i, sub := range subs {
		if sub.PayloadVersion == "" {
			sub.PayloadVersion = "2"
		}
		body[i] = sub
	}
	_, err := s.c.do(ctx, request{
		method:    http.MethodPut,
		path:      "/v2/webhooks/subscriptions",
		body:      body,
		retryable: true,
	}, opts)
	return err
}

// ErrInvalidSignature is returned when a webhook signature does not match.
var ErrInvalidSignature = errors.New("buypower: invalid webhook signature")

// ComputeSignature returns the hex HMAC-SHA256 of body keyed by secret.
func ComputeSignature(body []byte, secret string) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write(body)
	return hex.EncodeToString(m.Sum(nil))
}

// VerifySignature checks signature against the raw request body in constant time.
func VerifySignature(body []byte, signature, secret string) bool {
	got, err := hex.DecodeString(strings.TrimSpace(signature))
	if err != nil || secret == "" {
		return false
	}
	m := hmac.New(sha256.New, []byte(secret))
	m.Write(body)
	return hmac.Equal(got, m.Sum(nil))
}

// WebhookEvent is the v2 webhook envelope.
type WebhookEvent struct {
	ID      string          `json:"id"`      // evt_…, stable across retries: de-duplicate on it
	Type    string          `json:"type"`    // e.g. transfer.paid
	Version string          `json:"version"` // always "2"
	Created string          `json:"created"`
	Data    json.RawMessage `json:"data"`
}

// ConstructEvent verifies the signature over the raw body and parses the
// v2 envelope. Always pass the unmodified bytes you received.
func ConstructEvent(body []byte, signature, secret string) (*WebhookEvent, error) {
	if !VerifySignature(body, signature, secret) {
		return nil, ErrInvalidSignature
	}
	var e WebhookEvent
	if err := json.Unmarshal(body, &e); err != nil {
		return nil, fmt.Errorf("buypower: decode webhook: %w", err)
	}
	if e.Version != "2" {
		return nil, fmt.Errorf("buypower: unsupported webhook payload version %q; subscribe with payloadVersion \"2\"", e.Version)
	}
	return &e, nil
}

// WebhookTransaction is the data of collections and transfers events.
type WebhookTransaction struct {
	TransactionID            int64     `json:"transactionId"`
	TransactionReference     string    `json:"transactionReference"`
	AccountExchangeReference string    `json:"accountExchangeReference"`
	SessionID                string    `json:"sessionId"`
	AccountNumber            string    `json:"accountNumber"`
	AccountType              string    `json:"accountType"`
	Amount                   float64   `json:"amount"`
	Fee                      float64   `json:"fee"`
	StampDuty                float64   `json:"stampDuty"`
	SettlementAmount         float64   `json:"settlementAmount"`
	TransactionFee           float64   `json:"transactionFee"`
	TotalAmount              float64   `json:"totalAmount"`
	TransactionType          string    `json:"transactionType"` // collection | payout
	SourceAccountName        string    `json:"sourceAccountName"`
	SourceAccountNumber      string    `json:"sourceAccountNumber"`
	SourceBankName           string    `json:"sourceBankName"`
	SourceBankCode           string    `json:"sourceBankCode"`
	DestinationAccountNumber string    `json:"destinationAccountNumber"`
	DestinationAccountName   string    `json:"destinationAccountName"`
	DestinationBankName      string    `json:"destinationBankName"`
	DestinationBankCode      string    `json:"destinationBankCode"`
	Type                     Direction `json:"type"`
	Status                   string    `json:"status"` // CONFIRMED / UNRESOLVED are final
	Narration                string    `json:"narration"`
	Name                     string    `json:"name"`
	Metadata                 Metadata  `json:"metadata"`
	CreatedAt                string    `json:"createdAt"`
	UpdatedAt                string    `json:"updatedAt"`
}

// WebhookRefund is the data of refunds events.
type WebhookRefund struct {
	Event                string   `json:"event"`
	Reference            string   `json:"reference"`
	TransactionReference string   `json:"transactionReference"`
	Amount               float64  `json:"amount"`
	Currency             Currency `json:"currency"`
	Status               string   `json:"status"`
	Reason               string   `json:"reason"`
	CustomerNote         string   `json:"customerNote"`
	CreatedAt            string   `json:"createdAt"`
	CompletedAt          string   `json:"completedAt"`
}

// Transaction decodes Data for invoice.*, static_account.* and transfer.* events.
func (e *WebhookEvent) Transaction() (*WebhookTransaction, error) {
	var t WebhookTransaction
	return &t, json.Unmarshal(e.Data, &t)
}

// Refund decodes Data for refund.* events.
func (e *WebhookEvent) Refund() (*WebhookRefund, error) {
	var r WebhookRefund
	return &r, json.Unmarshal(e.Data, &r)
}

// WebhookHandler returns an http.Handler that verifies and parses each
// delivery, then calls fn. It answers 401 on a bad signature, 500 if fn
// returns an error (so the delivery is retried) and 200 otherwise. Keep fn
// fast; queue heavy work.
func WebhookHandler(secret string, fn func(ctx context.Context, e *WebhookEvent) error) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		sig := r.Header.Get(SignatureHeader)
		if sig == "" {
			sig = r.Header.Get(PanboxSignatureHeader)
		}
		event, err := ConstructEvent(body, sig, secret)
		if errors.Is(err, ErrInvalidSignature) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest) // shows in BuyPower's delivery log
			return
		}
		if err := fn(r.Context(), event); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
}
