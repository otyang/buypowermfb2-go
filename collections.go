package buypower

import (
	"context"
	"net/http"
)

// Field names follow what the API returns, which differs from openapi.json
// (it documents "type" and "available").

// CollectionAccount is an account that virtual accounts of one type sweep into.
type CollectionAccount struct {
	AccountNumber string `json:"accountNumber"`
	Name          string `json:"name"`
	Description   string `json:"description"`
	AccountType   string `json:"accountType"` // e.g. collection.static, collection.invoice
	BankName      string `json:"bankName"`
	BankCode      string `json:"bankCode"`
}

// CollectionBalanceInfo is a pooled balance you can transfer out of.
type CollectionBalanceInfo struct {
	CollectionAccount
	Currency Currency `json:"currency"`
	Balance  struct {
		Balance       float64 `json:"balance"` // available to transfer
		BlockedAmount float64 `json:"blockedAmount"`
	} `json:"balance"`
}

// CollectionsService handles /v2/collections.
type CollectionsService struct{ c *Client }

// ListAccounts lists your collection accounts (GET /v2/collections/accounts).
func (s *CollectionsService) ListAccounts(ctx context.Context, opts ...RequestOption) ([]CollectionAccount, error) {
	page, err := callList[CollectionAccount](ctx, s.c, request{method: http.MethodGet, path: "/v2/collections/accounts"}, opts)
	if err != nil {
		return nil, err
	}
	return page.Data, nil
}

// Balances returns your collection balances (GET /v2/collections/balances).
func (s *CollectionsService) Balances(ctx context.Context, opts ...RequestOption) ([]CollectionBalanceInfo, error) {
	page, err := callList[CollectionBalanceInfo](ctx, s.c, request{method: http.MethodGet, path: "/v2/collections/balances"}, opts)
	if err != nil {
		return nil, err
	}
	return page.Data, nil
}
