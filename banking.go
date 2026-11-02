package buypower

import (
	"context"
	"net/http"
	"net/url"
)

// Bank is a bank supported for transfers and account resolution.
type Bank struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

// ResolvedAccount is the holder of a bank account.
type ResolvedAccount struct {
	AccountNumber string `json:"accountNumber"`
	AccountName   string `json:"accountName"`
	BankCode      string `json:"bankCode"`
	BankName      string `json:"bankName"`
}

// BankingService handles /v2/banking.
type BankingService struct{ c *Client }

// ListBanks returns supported banks (GET /v2/banking/banks). Cache the result;
// it changes rarely.
func (s *BankingService) ListBanks(ctx context.Context, opts ...RequestOption) ([]Bank, error) {
	page, err := callList[Bank](ctx, s.c, request{method: http.MethodGet, path: "/v2/banking/banks"}, opts)
	if err != nil {
		return nil, err
	}
	return page.Data, nil
}

// ResolveAccount looks up the account name for a bank code and NUBAN
// (GET /v2/banking/accounts/resolve). Call this before every transfer.
func (s *BankingService) ResolveAccount(ctx context.Context, bankCode, accountNumber string, opts ...RequestOption) (*ResolvedAccount, error) {
	q := url.Values{"bankCode": {bankCode}, "accountNumber": {accountNumber}}
	return call[ResolvedAccount](ctx, s.c, request{method: http.MethodGet, path: "/v2/banking/accounts/resolve", query: q}, opts)
}
