package buypower

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"math"
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

// MarshalJSON sends Value rounded to the kobo. Floating-point noise such as
// 300.29999999999995 (from 100.10*3) is rounded away; a real third decimal
// place is an error, because BuyPower would otherwise round it silently.
func (m Money) MarshalJSON() ([]byte, error) {
	v, err := toKobo(m.Value)
	if err != nil {
		return nil, err
	}
	type plain Money // no MarshalJSON method, so no recursion
	return json.Marshal(plain{Value: v, Currency: m.Currency})
}

// toKobo rounds v to two decimal places, allowing only floating-point noise
// (a few units in the last place) to be rounded away.
func toKobo(v float64) (float64, error) {
	kobo := v * 100
	k := math.Round(kobo)
	if math.Abs(kobo-k) > math.Max(1e-6, math.Abs(kobo)*1e-13) {
		return 0, fmt.Errorf("buypower: amount %v has more than two decimal places; BuyPower works in kobo, so round it first", v)
	}
	return k / 100, nil
}

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
