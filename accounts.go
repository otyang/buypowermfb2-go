package buypower

import (
	"context"
	"encoding/json"
	"iter"
	"net/http"
	"net/url"
)

// ReservedAccountType is the kind of reserved account.
type ReservedAccountType string

const (
	ReservedStatic       ReservedAccountType = "static"
	ReservedSavings      ReservedAccountType = "savings"
	ReservedStaticCustom ReservedAccountType = "static-custom"
)

// BankAccount is a reserved or invoice virtual account.
type BankAccount struct {
	Reference     string   `json:"reference"`
	AccountNumber string   `json:"accountNumber"`
	AccountName   string   `json:"accountName"`
	BankCode      string   `json:"bankCode"`
	BankName      string   `json:"bankName"`
	Type          string   `json:"type"`
	Description   string   `json:"description"`
	Currency      Currency `json:"currency"`
	Metadata      Metadata `json:"metadata"`
	CreatedAt     string   `json:"createdAt"`
}

// CreateReservedAccountRequest is the body of POST /v2/accounts/reserved.
// At least one of BVN or NIN is required.
type CreateReservedAccountRequest struct {
	Reference              string              `json:"reference"`
	Name                   string              `json:"name"`
	Description            string              `json:"description"`
	Type                   ReservedAccountType `json:"type,omitempty"` // default static
	BVN                    string              `json:"bvn,omitempty"`  // 11 digits
	NIN                    string              `json:"nin,omitempty"`  // 11 digits
	PreferredAccountNumber string              `json:"preferredAccountNumber,omitempty"`
	Metadata               Metadata            `json:"metadata,omitempty"`
}

// ReservedAccountsService handles /v2/accounts/reserved.
type ReservedAccountsService struct{ c *Client }

// Create creates a static virtual account (POST /v2/accounts/reserved).
func (s *ReservedAccountsService) Create(ctx context.Context, req *CreateReservedAccountRequest, opts ...RequestOption) (*BankAccount, error) {
	return call[BankAccount](ctx, s.c, request{
		method:    http.MethodPost,
		path:      "/v2/accounts/reserved",
		body:      req,
		retryable: req.Reference != "",
	}, opts)
}

// List returns one page of reserved accounts (GET /v2/accounts/reserved).
func (s *ReservedAccountsService) List(ctx context.Context, params *ListParams, opts ...RequestOption) (*Page[BankAccount], error) {
	return callList[BankAccount](ctx, s.c, request{method: http.MethodGet, path: "/v2/accounts/reserved", query: params.query()}, opts)
}

// ListAll iterates over every reserved account.
func (s *ReservedAccountsService) ListAll(ctx context.Context, params *ListParams, opts ...RequestOption) iter.Seq2[BankAccount, error] {
	return listAll(ctx, params, s.List, opts)
}

// Balance returns the balance of one reserved account
// (GET /v2/accounts/reserved/{reference}/balance). The spec leaves the
// payload untyped, so it is returned raw for you to decode.
func (s *ReservedAccountsService) Balance(ctx context.Context, reference string, opts ...RequestOption) (json.RawMessage, error) {
	raw, err := call[json.RawMessage](ctx, s.c, request{method: http.MethodGet, path: "/v2/accounts/reserved/" + url.PathEscape(reference) + "/balance"}, opts)
	if err != nil {
		return nil, err
	}
	return *raw, nil
}

// Rename changes a reserved account's name (PATCH /v2/accounts/reserved/{reference}).
func (s *ReservedAccountsService) Rename(ctx context.Context, reference, name string, opts ...RequestOption) (*BankAccount, error) {
	return call[BankAccount](ctx, s.c, request{
		method:    http.MethodPatch,
		path:      "/v2/accounts/reserved/" + url.PathEscape(reference),
		body:      map[string]string{"name": name},
		retryable: true,
	}, opts)
}

// CreateInvoiceAccountRequest is the body of POST /v2/accounts/invoices.
type CreateInvoiceAccountRequest struct {
	Reference   string   `json:"reference"`
	Name        string   `json:"name"`
	Email       string   `json:"email"`
	Description string   `json:"description"`
	Amount      Money    `json:"amount"`
	ExpiresAt   string   `json:"expiresAt"` // ISO-8601, e.g. time.Now().Add(24*time.Hour).UTC().Format(time.RFC3339)
	Metadata    Metadata `json:"metadata,omitempty"`
}

// InvoiceAccountsService handles /v2/accounts/invoices.
type InvoiceAccountsService struct{ c *Client }

// Create creates a one-time account for a fixed amount (POST /v2/accounts/invoices).
func (s *InvoiceAccountsService) Create(ctx context.Context, req *CreateInvoiceAccountRequest, opts ...RequestOption) (*BankAccount, error) {
	return call[BankAccount](ctx, s.c, request{
		method:    http.MethodPost,
		path:      "/v2/accounts/invoices",
		body:      req,
		retryable: req.Reference != "",
	}, opts)
}

// List returns one page of invoice accounts (GET /v2/accounts/invoices).
func (s *InvoiceAccountsService) List(ctx context.Context, params *ListParams, opts ...RequestOption) (*Page[BankAccount], error) {
	return callList[BankAccount](ctx, s.c, request{method: http.MethodGet, path: "/v2/accounts/invoices", query: params.query()}, opts)
}

// ListAll iterates over every invoice account.
func (s *InvoiceAccountsService) ListAll(ctx context.Context, params *ListParams, opts ...RequestOption) iter.Seq2[BankAccount, error] {
	return listAll(ctx, params, s.List, opts)
}
