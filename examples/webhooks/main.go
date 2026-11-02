// Webhook receiver for v2 payloads.
//
//	BP_WEBHOOK_SECRET=whsec_... PORT=9990 go run ./examples/webhooks
package main

import (
	"cmp"
	"context"
	"log"
	"net/http"
	"os"

	buypower "github.com/otyang/buypowermfb2"
)

func main() {
	handler := buypower.WebhookHandler(os.Getenv("BP_WEBHOOK_SECRET"), func(ctx context.Context, e *buypower.WebhookEvent) error {
		// De-duplicate on e.ID: retries of one delivery share it.
		switch e.Type {
		case buypower.EventInvoicePaid, buypower.EventStaticAccountTransactionCreated:
			tx, err := e.Transaction()
			if err != nil {
				return err
			}
			log.Printf("collection %s: ₦%.2f settled (%s)", tx.TransactionReference, tx.SettlementAmount, tx.Status)
		case buypower.EventTransferPaid, buypower.EventTransferFailed:
			tx, err := e.Transaction()
			if err != nil {
				return err
			}
			log.Printf("transfer %s: %s", tx.TransactionReference, e.Type)
		case buypower.EventRefundCompleted, buypower.EventRefundPartiallyCompleted, buypower.EventRefundFailed:
			rf, err := e.Refund()
			if err != nil {
				return err
			}
			log.Printf("refund %s: %s", rf.Reference, rf.Status)
		default:
			log.Printf("ignoring %s", e.Type)
		}
		return nil
	})

	http.Handle("/webhooks/buypower", handler)
	addr := "127.0.0.1:" + cmp.Or(os.Getenv("PORT"), "9990")
	log.Println("listening on", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}
