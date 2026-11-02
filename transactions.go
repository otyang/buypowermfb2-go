package buypower

import (
	"context"
	"iter"
	"net/http"
	"net/url"
)

// Direction of a transaction.
type Direction string

const (
	Inflow  Direction = "INFLOW"
	Outflow Direction = "OUTFLOW"
)

// Transaction types.
const (
	TxTypeCollection = "COLLECTION"
	TxTypeTransfer   = "TRANSFER"
	TxTypeRefund     = "REFUND"
	TxTypeWithdrawal = "WITHDRAWAL"
	TxTypeAdjustment = "ADJUSTMENT"
)

// Transaction statuses. Confirmed and Unresolved are final.
const (
	TxPending           = "PENDING"
	TxAuthorized        = "AUTHORIZED"
	TxSubmitted         = "SUBMITTED"
	TxConfirmed         = "CONFIRMED"
	TxUnresolved        = "UNRESOLVED"
	TxRejected          = "REJECTED"
	TxRefundInitiated   = "REFUND_INITIATED"
	TxRefunded          = "REFUNDED"
	TxRefundedPartially = "REFUNDED_PARTIALLY"
)

// TransactionAmount breaks down a transaction's money fields.
type TransactionAmount struct {
	Value            float64  `json:"value"`
	Fee              *float64 `json:"fee"` // platform fee; nil when none applies
	StampDuty        float64  `json:"stampDuty"`
	TransactionFee   float64  `json:"transactionFee"`
	SettlementAmount float64  `json:"settlementAmount"`
	Total            float64  `json:"total"`
	Currency         Currency `json:"currency"`
}

// Transaction is a collection, transfer, refund, withdrawal or adjustment.
type Transaction struct {
	Reference                string            `json:"reference"`
	AccountExchangeReference string            `json:"accountExchangeReference"`
	SessionID                string            `json:"sessionId"`
	Type                     string            `json:"type"`
	Status                   string            `json:"status"`
	Direction                Direction         `json:"direction"`
	Amount                   TransactionAmount `json:"amount"`
	Source                   AccountParty      `json:"source"`
	Destination              AccountParty      `json:"destination"`
	Narration                string            `json:"narration"`
	CreatedAt                string            `json:"createdAt"`
	UpdatedAt                string            `json:"updatedAt"`
}

// TransactionsService handles /v2/transactions.
type TransactionsService struct{ c *Client }

// List returns one page of transactions (GET /v2/transactions).
func (s *TransactionsService) List(ctx context.Context, params *ListParams, opts ...RequestOption) (*Page[Transaction], error) {
	return callList[Transaction](ctx, s.c, request{method: http.MethodGet, path: "/v2/transactions", query: params.query()}, opts)
}

// ListAll iterates over every transaction.
func (s *TransactionsService) ListAll(ctx context.Context, params *ListParams, opts ...RequestOption) iter.Seq2[Transaction, error] {
	return listAll(ctx, params, s.List, opts)
}

// Get retrieves a transaction by reference or numeric id
// (GET /v2/transactions/{reference}). This is the source of truth for
// reconciliation.
func (s *TransactionsService) Get(ctx context.Context, reference string, opts ...RequestOption) (*Transaction, error) {
	return call[Transaction](ctx, s.c, request{method: http.MethodGet, path: "/v2/transactions/" + url.PathEscape(reference)}, opts)
}
