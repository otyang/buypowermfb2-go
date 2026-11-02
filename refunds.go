package buypower

import (
	"context"
	crand "crypto/rand"
	"iter"
	"net/http"
	"net/url"
)

// CreateRefundRequest is the body of POST /v2/refunds.
type CreateRefundRequest struct {
	TransactionReference string   `json:"transactionReference"` // original confirmed collection
	Amount               Money    `json:"amount"`
	Reason               string   `json:"reason"`         // 3–255 chars
	Note                 string   `json:"note,omitempty"` // up to 500 chars
	Metadata             Metadata `json:"metadata,omitempty"`
}

// Refund reverses all or part of a collection.
type Refund struct {
	Reference            string `json:"reference"`
	TransactionReference string `json:"transactionReference"`
	Amount               Money  `json:"amount"`
	Status               string `json:"status"`
	Reason               string `json:"reason"`
	Note                 string `json:"note"`
	Type                 string `json:"type"`
	Strategy             string `json:"strategy"`
	Comment              string `json:"comment"`
	CompletedAt          string `json:"completedAt"`
	CreatedAt            string `json:"createdAt"`
}

// RefundListParams filters GET /v2/refunds. Note refunds page with "size".
type RefundListParams struct {
	Page                 int
	Size                 int
	Status               string
	TransactionReference string
}

func (p *RefundListParams) query() url.Values {
	q := url.Values{}
	if p == nil {
		return q
	}
	setInt(q, "page", p.Page)
	setInt(q, "size", p.Size)
	if p.Status != "" {
		q.Set("status", p.Status)
	}
	if p.TransactionReference != "" {
		q.Set("transactionReference", p.TransactionReference)
	}
	return q
}

// RefundsService handles /v2/refunds.
type RefundsService struct{ c *Client }

// Create initiates a refund (POST /v2/refunds, 202 Accepted). An
// Idempotency-Key is generated per call unless you pass WithIdempotencyKey;
// persist your own key if you need retries across process restarts.
func (s *RefundsService) Create(ctx context.Context, req *CreateRefundRequest, opts ...RequestOption) (*Refund, error) {
	opts = append([]RequestOption{WithIdempotencyKey(crand.Text())}, opts...)
	return call[Refund](ctx, s.c, request{
		method:    http.MethodPost,
		path:      "/v2/refunds",
		body:      req,
		retryable: true,
	}, opts)
}

// List returns one page of refunds (GET /v2/refunds).
func (s *RefundsService) List(ctx context.Context, params *RefundListParams, opts ...RequestOption) (*Page[Refund], error) {
	return callList[Refund](ctx, s.c, request{method: http.MethodGet, path: "/v2/refunds", query: params.query()}, opts)
}

// ListAll iterates over every refund matching params.
func (s *RefundsService) ListAll(ctx context.Context, params *RefundListParams, opts ...RequestOption) iter.Seq2[Refund, error] {
	p := RefundListParams{Size: 100}
	if params != nil {
		p = *params
		if p.Size == 0 {
			p.Size = 100
		}
	}
	return paginate(ctx, func(ctx context.Context, page int) (*Page[Refund], error) {
		p.Page = page
		return s.List(ctx, &p, opts...)
	})
}

// Get retrieves a refund by its reference (GET /v2/refunds/{refundReference}).
func (s *RefundsService) Get(ctx context.Context, refundReference string, opts ...RequestOption) (*Refund, error) {
	return call[Refund](ctx, s.c, request{method: http.MethodGet, path: "/v2/refunds/" + url.PathEscape(refundReference)}, opts)
}
