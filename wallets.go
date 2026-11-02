package buypower

import (
	"context"
	"iter"
	"net/http"
	"net/url"
)

// Wallet is an account that keeps its own balance.
type Wallet struct {
	Reference     string   `json:"reference"`
	AccountNumber string   `json:"accountNumber"`
	AccountName   string   `json:"accountName"`
	BankCode      string   `json:"bankCode"`
	BankName      string   `json:"bankName"`
	Type          string   `json:"type"`
	Currency      Currency `json:"currency"`
	Balance       float64  `json:"balance"`
	CreatedAt     string   `json:"createdAt"`
}

// WalletBalance is the balance of one wallet.
type WalletBalance struct {
	Reference     string   `json:"reference"`
	AccountNumber string   `json:"accountNumber"`
	Balance       float64  `json:"balance"`
	Currency      Currency `json:"currency"`
}

// CreateWalletRequest is the body of POST /v2/wallets.
// At least one of BVN or NIN is required.
type CreateWalletRequest struct {
	Name          string   `json:"name"`
	Reference     string   `json:"reference,omitempty"` // generated if omitted
	Description   string   `json:"description,omitempty"`
	BVN           string   `json:"bvn,omitempty"`
	NIN           string   `json:"nin,omitempty"`
	AccountNumber string   `json:"accountNumber,omitempty"` // 10 digits, optional
	Metadata      Metadata `json:"metadata,omitempty"`
}

// WalletsService handles /v2/wallets.
type WalletsService struct{ c *Client }

// Create creates a wallet account (POST /v2/wallets). Creates are idempotent
// on Reference, so the SDK retries them only when Reference is set.
func (s *WalletsService) Create(ctx context.Context, req *CreateWalletRequest, opts ...RequestOption) (*Wallet, error) {
	return call[Wallet](ctx, s.c, request{
		method:    http.MethodPost,
		path:      "/v2/wallets",
		body:      req,
		retryable: req.Reference != "",
	}, opts)
}

// List returns one page of wallets with balances (GET /v2/wallets).
func (s *WalletsService) List(ctx context.Context, params *ListParams, opts ...RequestOption) (*Page[Wallet], error) {
	return callList[Wallet](ctx, s.c, request{method: http.MethodGet, path: "/v2/wallets", query: params.query()}, opts)
}

// ListAll iterates over every wallet.
func (s *WalletsService) ListAll(ctx context.Context, params *ListParams, opts ...RequestOption) iter.Seq2[Wallet, error] {
	return listAll(ctx, params, s.List, opts)
}

// Get fetches a wallet by your reference or its account number (GET /v2/wallets/{reference}).
func (s *WalletsService) Get(ctx context.Context, reference string, opts ...RequestOption) (*Wallet, error) {
	return call[Wallet](ctx, s.c, request{method: http.MethodGet, path: "/v2/wallets/" + url.PathEscape(reference)}, opts)
}

// Balance fetches a wallet's balance (GET /v2/wallets/{reference}/balance).
func (s *WalletsService) Balance(ctx context.Context, reference string, opts ...RequestOption) (*WalletBalance, error) {
	return call[WalletBalance](ctx, s.c, request{method: http.MethodGet, path: "/v2/wallets/" + url.PathEscape(reference) + "/balance"}, opts)
}
