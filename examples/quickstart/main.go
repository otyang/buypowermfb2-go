// Quickstart: resolve an account, pay out to it, then reconcile.
//
//	BP_API_KEY=sk_test_... BP_BUSINESS_ID=4827 go run ./examples/quickstart
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	buypower "github.com/otyang/buypowermfb2"
)

func main() {
	client, err := buypower.NewClientFromEnv()
	if err != nil {
		log.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// 1. Resolve the beneficiary before sending money.
	acct, err := client.Banking.ResolveAccount(ctx, "058", "0123456789")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Paying:", acct.AccountName)

	// 2. Initiate the transfer with a unique reference (safe to retry).
	tr, err := client.Transfers.Create(ctx, &buypower.CreateTransferRequest{
		Source:      buypower.FromCollection(buypower.BalanceReserved),
		Destination: buypower.TransferDestination{BankCode: acct.BankCode, AccountNumber: acct.AccountNumber},
		Amount:      buypower.NGN(5000),
		Narration:   "Invoice #4821 payout",
		Reference:   "payout-4821",
	})
	var apiErr *buypower.APIError
	switch {
	case errors.As(err, &apiErr) && apiErr.Code == buypower.CodeValidation:
		log.Fatalf("fix the request: %v", apiErr.Errors)
	case err != nil:
		log.Fatal(err)
	}
	fmt.Printf("Transfer %s is %s (total debit ₦%.2f)\n", tr.Reference, tr.Status, tr.Amount.Total)

	// 3. The transfer is async: confirm the final state from the ledger.
	tx, err := client.Transactions.Get(ctx, tr.Reference)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Ledger status:", tx.Status)

	// Iterate every wallet across all pages.
	for w, err := range client.Wallets.ListAll(ctx, nil) {
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("%s  %s  ₦%.2f\n", w.AccountNumber, w.AccountName, w.Balance)
	}
}
