package buypower

import (
	"context"
	"net/http"
	"net/url"
	"time"
)

// lagos is Africa/Lagos (WAT, UTC+1, no daylight saving).
var lagos = time.FixedZone("WAT", 3600)

// RevenueParams selects an inclusive range of days in Africa/Lagos time.
type RevenueParams struct {
	From     time.Time // only the calendar date in Lagos is used
	To       time.Time
	Page     int
	PageSize int
}

// RevenueItem is one fee charge or reversal.
type RevenueItem struct {
	Reference       string  `json:"reference"`
	TransactionType string  `json:"transactionType"` // collection | payout
	EntryType       string  `json:"entryType"`       // CHARGE | REVERSAL
	PrincipalAmount float64 `json:"principalAmount"`
	TransactionFee  float64 `json:"transactionFee"`
	CreatedAt       string  `json:"createdAt"`
}

// MerchantRevenue is the fee revenue for a period.
type MerchantRevenue struct {
	Period struct {
		From     string `json:"from"`
		To       string `json:"to"`
		Timezone string `json:"timezone"`
	} `json:"period"`
	TotalRevenue      float64       `json:"totalRevenue"`
	CollectionRevenue float64       `json:"collectionRevenue"`
	PayoutRevenue     float64       `json:"payoutRevenue"`
	TransactionCount  int           `json:"transactionCount"`
	Items             []RevenueItem `json:"items"`
	Pagination        *Pagination   `json:"pagination"`
}

// RevenueService handles /v2/revenue.
type RevenueService struct{ c *Client }

// Get returns the revenue earned from the transaction fees you charge
// (GET /v2/revenue). Fees reversed on failed payouts are netted out.
func (s *RevenueService) Get(ctx context.Context, params RevenueParams, opts ...RequestOption) (*MerchantRevenue, error) {
	q := url.Values{
		"from": {params.From.In(lagos).Format(time.DateOnly)},
		"to":   {params.To.In(lagos).Format(time.DateOnly)},
	}
	setInt(q, "page", params.Page)
	setInt(q, "pageSize", params.PageSize)
	return call[MerchantRevenue](ctx, s.c, request{method: http.MethodGet, path: "/v2/revenue", query: q}, opts)
}
