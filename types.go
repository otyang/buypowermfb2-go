package buypower

import (
	"context"
	"iter"
	"net/url"
)

// Currency is an ISO currency code. Only NGN is honoured today.
type Currency string

const CurrencyNGN Currency = "NGN"

// Money is an amount in major units (naira, not kobo).
type Money struct {
	Value    float64  `json:"value"`
	Currency Currency `json:"currency,omitempty"`
}

// NGN returns a naira Money value: buypower.NGN(5000) is ₦5,000.00.
func NGN(value float64) Money { return Money{Value: value, Currency: CurrencyNGN} }

// AccountParty describes a counterparty bank account.
type AccountParty struct {
	AccountName   string `json:"accountName,omitempty"`
	AccountNumber string `json:"accountNumber,omitempty"`
	BankName      string `json:"bankName,omitempty"`
	BankCode      string `json:"bankCode,omitempty"`
}

// Metadata is arbitrary key/value data stored with a resource.
type Metadata map[string]any

// Pagination is returned in meta.pagination on list endpoints.
type Pagination struct {
	Page       int `json:"page"`
	PerPage    int `json:"perPage"`
	Total      int `json:"total"`
	TotalPages int `json:"totalPages"`
}

// Page is one page of a list response.
type Page[T any] struct {
	Data       []T
	Pagination *Pagination // nil when the endpoint does not paginate
}

// HasMore reports whether further pages exist.
func (p *Page[T]) HasMore() bool {
	return p.Pagination != nil && p.Pagination.Page < p.Pagination.TotalPages
}

// ListParams are the paging parameters shared by v2 list endpoints.
// Zero values use the server defaults (page 1, 15 per page, max 100).
type ListParams struct {
	Page     int
	PageSize int
}

func (p *ListParams) query() url.Values {
	q := url.Values{}
	if p != nil {
		setInt(q, "page", p.Page)
		setInt(q, "pageSize", p.PageSize)
	}
	return q
}

// paginate walks every page starting at page 1, yielding each item.
func paginate[T any](ctx context.Context, fetch func(ctx context.Context, page int) (*Page[T], error)) iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		for page := 1; ; page++ {
			p, err := fetch(ctx, page)
			if err != nil {
				var zero T
				yield(zero, err)
				return
			}
			for _, item := range p.Data {
				if !yield(item, nil) {
					return
				}
			}
			if len(p.Data) == 0 || !p.HasMore() {
				return
			}
		}
	}
}

// listAll walks every page of a ListParams-paged endpoint, 100 per page by default.
func listAll[T any](ctx context.Context, params *ListParams, list func(context.Context, *ListParams, ...RequestOption) (*Page[T], error), opts []RequestOption) iter.Seq2[T, error] {
	size := 100
	if params != nil && params.PageSize > 0 {
		size = params.PageSize
	}
	return paginate(ctx, func(ctx context.Context, page int) (*Page[T], error) {
		return list(ctx, &ListParams{Page: page, PageSize: size}, opts...)
	})
}
