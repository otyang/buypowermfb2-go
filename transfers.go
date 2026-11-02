package buypower

import (
	"context"
	"net/http"
)

// TransferSourceType is where a transfer draws money from.
type TransferSourceType string

const (
	SourceCollection TransferSourceType = "collection"
	SourceWallet     TransferSourceType = "wallet"
)

// CollectionBalance names a pooled collection balance.
type CollectionBalance string

const (
	BalanceReserved       CollectionBalance = "reserved"
	BalanceReservedCustom CollectionBalance = "reserved-custom"
	BalanceInvoice        CollectionBalance = "invoice"
)

// TransferSource is either a collection balance or a wallet account.
// Use FromCollection or FromWallet to build one.
type TransferSource struct {
	Type          TransferSourceType `json:"type"`
	Balance       CollectionBalance  `json:"balance,omitempty"`
	AccountNumber string             `json:"accountNumber,omitempty"`
}

// FromCollection draws from a pooled collection balance.
func FromCollection(balance CollectionBalance) TransferSource {
	return TransferSource{Type: SourceCollection, Balance: balance}
}

// FromWallet draws from a specific wallet account.
func FromWallet(accountNumber string) TransferSource {
	return TransferSource{Type: SourceWallet, AccountNumber: accountNumber}
}

// TransferDestination is an external bank account.
type TransferDestination struct {
	BankCode      string `json:"bankCode"`
	AccountNumber string `json:"accountNumber"`
}

// CreateTransferRequest is the body of POST /v2/transfers.
type CreateTransferRequest struct {
	Source      TransferSource      `json:"source"`
	Destination TransferDestination `json:"destination"`
	Amount      Money               `json:"amount"`
	Narration   string              `json:"narration,omitempty"` // max 50 chars
	// Reference is your idempotent client reference. Strongly recommended:
	// reuse it when you retry after reconciling.
	Reference string   `json:"reference,omitempty"`
	Metadata  Metadata `json:"metadata,omitempty"`
}

// TransferAmount breaks down what a transfer debits.
type TransferAmount struct {
	Value          float64  `json:"value"`          // sent to the beneficiary
	TransactionFee float64  `json:"transactionFee"` // your own fee, if any
	Total          float64  `json:"total"`          // value + stampDuty + transactionFee
	Currency       Currency `json:"currency"`
}

// Transfer is an outbound payout.
type Transfer struct {
	Reference                string         `json:"reference"`
	AccountExchangeReference string         `json:"accountExchangeReference"`
	SessionID                string         `json:"sessionId"`
	Status                   string         `json:"status"` // a transaction status, e.g. TxAuthorized
	Amount                   TransferAmount `json:"amount"`
	StampDuty                float64        `json:"stampDuty"`
	Source                   *AccountParty  `json:"source"`
	Destination              AccountParty   `json:"destination"`
	Narration                string         `json:"narration"`
	InitiatedAt              string         `json:"initiatedAt"`
}

// TransfersService handles /v2/transfers.
type TransfersService struct{ c *Client }

// Create initiates a transfer (POST /v2/transfers). The returned status is
// usually pending; wait for the transfer.* webhook or poll Transactions.Get.
//
// Create is never retried by the SDK: on a timeout or 502 the money may still
// move, so check Transactions.Get(ctx, req.Reference) before retrying yourself.
func (s *TransfersService) Create(ctx context.Context, req *CreateTransferRequest, opts ...RequestOption) (*Transfer, error) {
	return call[Transfer](ctx, s.c, request{method: http.MethodPost, path: "/v2/transfers", body: req}, opts)
}
