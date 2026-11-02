package buypower

// Tests against the real BuyPower sandbox. They are skipped unless credentials
// are set, so `go test ./...` stays offline. See "Testing" in the README.
//
//	BP_API_KEY, BP_BUSINESS_ID   sandbox credentials (sk_test_ keys only)
//	BP_SANDBOX_WRITE=1           also create accounts, transfers and refunds
//	BP_WEBHOOK_URL               public URL (e.g. a cloudflared tunnel) that forwards to BP_WEBHOOK_PORT
//	BP_WEBHOOK_SECRET            webhook secret from the dashboard
//	BP_WEBHOOK_PORT              local port for the test receiver (default 9990)

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

func sandboxClient(t *testing.T) *Client {
	t.Helper()
	key, id := os.Getenv("BP_API_KEY"), os.Getenv("BP_BUSINESS_ID")
	if key == "" || id == "" {
		t.Skip("set BP_API_KEY and BP_BUSINESS_ID to run sandbox tests")
	}
	if !strings.HasPrefix(key, "sk_test_") {
		t.Fatal("sandbox tests only run with an sk_test_ key")
	}
	return NewClient(key, id, WithEnvironment(EnvironmentSandbox))
}

func needWrite(t *testing.T) {
	t.Helper()
	if os.Getenv("BP_SANDBOX_WRITE") != "1" {
		t.Skip("set BP_SANDBOX_WRITE=1 to run tests that create sandbox data")
	}
}

func sandboxCtx(t *testing.T, d time.Duration) context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), d)
	t.Cleanup(cancel)
	return ctx
}

// uniqueRef keeps runs from different people and machines apart.
func uniqueRef(prefix string) string {
	return fmt.Sprintf("sdk-test-%s-%d", prefix, time.Now().UnixNano())
}

// skipKnownBug skips when err matches a server bug we have reported, so the
// test starts passing by itself once BuyPower fixes it.
func skipKnownBug(t *testing.T, err error, substr, why string) {
	t.Helper()
	if err != nil && strings.Contains(err.Error(), substr) {
		t.Skip("known sandbox issue: " + why)
	}
}

func TestSandboxReads(t *testing.T) {
	c := sandboxClient(t)
	ctx := sandboxCtx(t, 2*time.Minute)

	t.Run("ListBanks", func(t *testing.T) {
		banks, err := c.Banking.ListBanks(ctx)
		if err != nil || len(banks) == 0 || banks[0].Code == "" {
			t.Fatalf("banks=%d err=%v", len(banks), err)
		}
	})
	t.Run("ResolveAccount", func(t *testing.T) {
		acc, err := c.Banking.ResolveAccount(ctx, "058", "0123456789")
		if err != nil || acc.AccountName == "" {
			t.Fatalf("acc=%+v err=%v", acc, err)
		}
	})
	t.Run("Collections", func(t *testing.T) {
		accounts, err := c.Collections.ListAccounts(ctx)
		if err != nil || len(accounts) == 0 || accounts[0].AccountType == "" {
			t.Fatalf("accounts=%+v err=%v", accounts, err)
		}
		bals, err := c.Collections.Balances(ctx)
		if err != nil || len(bals) == 0 || bals[0].AccountNumber == "" {
			t.Fatalf("balances=%+v err=%v", bals, err)
		}
	})
	t.Run("TransactionsListAll", func(t *testing.T) {
		first, err := c.Transactions.List(ctx, &ListParams{PageSize: 1})
		if err != nil || first.Pagination == nil {
			t.Fatalf("page=%+v err=%v", first, err)
		}
		size := 1 // one item per page exercises paging, but keep big accounts fast
		if first.Pagination.Total > 20 {
			size = 100
		}
		n := 0
		for _, err := range c.Transactions.ListAll(ctx, &ListParams{PageSize: size}) {
			if err != nil {
				t.Fatal(err)
			}
			n++
		}
		if n != first.Pagination.Total {
			t.Fatalf("walked %d, pagination total %d", n, first.Pagination.Total)
		}
	})
	t.Run("TransactionNotFound", func(t *testing.T) {
		if _, err := c.Transactions.Get(ctx, uniqueRef("missing")); !IsNotFound(err) {
			t.Fatalf("want 404, got %v", err)
		}
	})
	t.Run("Lists", func(t *testing.T) {
		if _, err := c.ReservedAccounts.List(ctx, nil); err != nil {
			t.Error("reserved:", err)
		}
		if _, err := c.InvoiceAccounts.List(ctx, nil); err != nil {
			t.Error("invoices:", err)
		}
		if _, err := c.Wallets.List(ctx, nil); err != nil {
			t.Error("wallets:", err)
		}
		if _, err := c.Refunds.List(ctx, nil); err != nil {
			t.Error("refunds:", err)
		}
	})
	t.Run("WebhookSubscriptions", func(t *testing.T) {
		subs, err := c.Webhooks.ListSubscriptions(ctx)
		if err != nil || len(subs) == 0 {
			t.Fatalf("subs=%+v err=%v", subs, err)
		}
	})
	t.Run("Revenue", func(t *testing.T) {
		now := time.Now()
		rev, err := c.Revenue.Get(ctx, RevenueParams{From: now.AddDate(0, 0, -7), To: now})
		if err != nil || rev.Period.Timezone != "Africa/Lagos" {
			t.Fatalf("rev=%+v err=%v", rev, err)
		}
	})
}

func TestSandboxWrites(t *testing.T) {
	c := sandboxClient(t)
	needWrite(t)
	ctx := sandboxCtx(t, 3*time.Minute)

	acct := createReservedAccount(t, ctx, c)

	t.Run("RenameReservedAccount", func(t *testing.T) {
		got, err := c.ReservedAccounts.Rename(ctx, acct.Reference, "SDK Test Renamed")
		if err != nil || got.AccountName != "SDK Test Renamed" {
			t.Fatalf("got=%+v err=%v", got, err)
		}
	})
	t.Run("ReservedAccountBalance", func(t *testing.T) {
		_, err := c.ReservedAccounts.Balance(ctx, acct.Reference)
		skipKnownBug(t, err, "not a wallet account", "balance endpoint rejects reserved accounts")
		if err != nil {
			t.Fatal(err)
		}
	})
	t.Run("InvoiceAccount", func(t *testing.T) {
		inv, err := c.InvoiceAccounts.Create(ctx, &CreateInvoiceAccountRequest{
			Reference: uniqueRef("inv"), Name: "SDK Invoice", Email: "sdk-test@example.com",
			Description: "sdk sandbox test", Amount: NGN(1500),
			ExpiresAt: time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339),
		})
		if err != nil || inv.Reference == "" {
			t.Fatalf("inv=%+v err=%v", inv, err)
		}
	})
	t.Run("Wallet", func(t *testing.T) {
		w, err := c.Wallets.Create(ctx, &CreateWalletRequest{Reference: uniqueRef("wal"), Name: "SDK Wallet", BVN: "12345678901"})
		skipKnownBug(t, err, "not enabled", "wallets are not enabled for this business")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := c.Wallets.Balance(ctx, w.Reference); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("TransferAndReconcile", func(t *testing.T) {
		ref := uniqueRef("tr")
		sandboxTransfer(t, ctx, c, ref)
		tx, err := c.Transactions.Get(ctx, ref)
		if err != nil || tx.Direction != Outflow || tx.Type != TxTypeTransfer {
			t.Fatalf("tx=%+v err=%v", tx, err)
		}
	})
	t.Run("RefundOwnCollection", func(t *testing.T) {
		txRef := uniqueRef("pay")
		simulatePayment(t, ctx, c, acct, txRef, 2500)
		waitForTransaction(t, ctx, c, txRef)
		rf, err := c.Refunds.Create(ctx, &CreateRefundRequest{TransactionReference: txRef, Amount: NGN(100), Reason: "sdk sandbox test"})
		if err != nil || rf.TransactionReference != txRef {
			t.Fatalf("rf=%+v err=%v", rf, err)
		}
		if _, err := c.Refunds.Get(ctx, rf.Reference); err != nil {
			t.Fatal(err)
		}
	})
}

// TestSandboxWebhooks points the business's sandbox webhook subscriptions at
// BP_WEBHOOK_URL, triggers a payment and a transfer, and waits for both signed
// events to reach a local receiver. It leaves the subscriptions pointing there.
func TestSandboxWebhooks(t *testing.T) {
	c := sandboxClient(t)
	needWrite(t)
	publicURL, secret := os.Getenv("BP_WEBHOOK_URL"), os.Getenv("BP_WEBHOOK_SECRET")
	if publicURL == "" || secret == "" {
		t.Skip("set BP_WEBHOOK_URL (a tunnel to BP_WEBHOOK_PORT) and BP_WEBHOOK_SECRET to run the webhook test")
	}
	port := os.Getenv("BP_WEBHOOK_PORT")
	if port == "" {
		port = "9990"
	}
	ctx := sandboxCtx(t, 4*time.Minute)

	// The tunnel forwards to one fixed port, so fail rather than pick another.
	ln, err := net.Listen("tcp", "127.0.0.1:"+port)
	if err != nil {
		t.Fatalf("port %s is busy (stop whatever uses it, or set BP_WEBHOOK_PORT): %v", port, err)
	}
	events := make(chan *WebhookEvent, 100)
	rejected := make(chan int, 100)
	handler := WebhookHandler(secret, func(ctx context.Context, e *WebhookEvent) error {
		events <- e
		return nil
	})
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &statusRecorder{ResponseWriter: w, code: http.StatusOK}
		handler.ServeHTTP(rec, r)
		if r.Method == http.MethodPost && rec.code != http.StatusOK {
			rejected <- rec.code
		}
	})}
	go srv.Serve(ln)
	t.Cleanup(func() { srv.Close() })

	if err := c.Webhooks.UpsertSubscriptions(ctx, []WebhookSubscriptionInput{
		{URL: publicURL, Topic: TopicCollections},
		{URL: publicURL, Topic: TopicTransfers},
		{URL: publicURL, Topic: TopicRefunds},
	}); err != nil {
		t.Fatal(err)
	}

	acct := createReservedAccount(t, ctx, c)

	t.Run("CollectionEvent", func(t *testing.T) {
		txRef := uniqueRef("whpay")
		simulatePayment(t, ctx, c, acct, txRef, 2500)
		e := waitForEvent(t, events, rejected, func(e *WebhookEvent) bool {
			tx, err := e.Transaction()
			return err == nil && tx.TransactionReference == txRef
		})
		tx, _ := e.Transaction()
		if e.Type != EventStaticAccountTransactionCreated || e.ID == "" || tx.Amount <= 0 || tx.Type != Inflow {
			t.Fatalf("event=%+v tx=%+v", e, tx)
		}
	})
	t.Run("TransferEvent", func(t *testing.T) {
		ref := uniqueRef("whtr")
		sandboxTransfer(t, ctx, c, ref)
		e := waitForEvent(t, events, rejected, func(e *WebhookEvent) bool {
			tx, err := e.Transaction()
			return err == nil && tx.TransactionReference == ref
		})
		if !strings.HasPrefix(e.Type, "transfer.") {
			t.Fatalf("event type %q", e.Type)
		}
	})
}

type statusRecorder struct {
	http.ResponseWriter
	code int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.code = code
	r.ResponseWriter.WriteHeader(code)
}

func waitForEvent(t *testing.T, events <-chan *WebhookEvent, rejected <-chan int, match func(*WebhookEvent) bool) *WebhookEvent {
	t.Helper()
	timeout := time.After(90 * time.Second)
	for {
		select {
		case e := <-events:
			if match(e) {
				return e
			}
		case code := <-rejected:
			if code == http.StatusUnauthorized {
				t.Fatal("a delivery failed signature verification: BP_WEBHOOK_SECRET does not match this business")
			}
			t.Fatalf("a delivery was rejected with HTTP %d", code)
		case <-timeout:
			t.Fatal("no matching webhook within 90s: check BP_WEBHOOK_URL forwards to BP_WEBHOOK_PORT")
		}
	}
}

func createReservedAccount(t *testing.T, ctx context.Context, c *Client) *BankAccount {
	t.Helper()
	acct, err := c.ReservedAccounts.Create(ctx, &CreateReservedAccountRequest{
		Reference: uniqueRef("res"), Name: "SDK Test", Description: "sdk sandbox test", BVN: "12345678901",
	})
	if err != nil || acct.AccountNumber == "" {
		t.Fatalf("create reserved account: acct=%+v err=%v", acct, err)
	}
	return acct
}

// sandboxTransfer sends ₦100. The sandbox simulates upstream 502s, so on a 502
// it reconciles first and retries once with the same reference, as the docs advise.
func sandboxTransfer(t *testing.T, ctx context.Context, c *Client, ref string) {
	t.Helper()
	req := &CreateTransferRequest{
		Source:      FromCollection(BalanceReserved),
		Destination: TransferDestination{BankCode: "058", AccountNumber: "0123456789"},
		Amount:      NGN(100), Narration: "sdk sandbox test", Reference: ref,
	}
	_, err := c.Transfers.Create(ctx, req)
	var apiErr *APIError
	if errors.As(err, &apiErr) && apiErr.Code == CodeGateway {
		if _, getErr := c.Transactions.Get(ctx, ref); IsNotFound(getErr) {
			_, err = c.Transfers.Create(ctx, req)
		} else {
			err = getErr // nil when the first attempt went through after all
		}
	}
	skipKnownBug(t, err, "Insufficient balance", "the collection balance is empty and simulated payments don't credit it, so transfers can't run on this business")
	if err != nil {
		t.Fatalf("transfer %s: %v", ref, err)
	}
}

// simulatePayment pays into acct via the sandbox-only simulator. It is a /v1
// endpoint, so it is reached through the internal request helper rather than
// the public (v2-only) API.
func simulatePayment(t *testing.T, ctx context.Context, c *Client, acct *BankAccount, txRef string, amount float64) {
	t.Helper()
	_, err := call[json.RawMessage](ctx, c, request{method: http.MethodPost, path: "/v1/webhooks/simulate", body: map[string]any{
		"reference":                acct.Reference,
		"destinationAccountNumber": acct.AccountNumber,
		"bankAccountType":          "Normal",
		"transactionStatus":        TxConfirmed,
		"amount":                   amount,
		"transactionReference":     txRef,
		"sessionId":                txRef,
		"sourceAccountName":        "SDK TEST PAYER",
		"narration":                "sdk sandbox test",
	}}, nil)
	if err != nil {
		t.Fatalf("simulate payment: %v", err)
	}
}

func waitForTransaction(t *testing.T, ctx context.Context, c *Client, ref string) {
	t.Helper()
	for i := 0; i < 15; i++ {
		_, err := c.Transactions.Get(ctx, ref)
		if err == nil {
			return
		}
		if !IsNotFound(err) {
			t.Fatal(err)
		}
		time.Sleep(2 * time.Second)
	}
	t.Fatalf("transaction %s did not appear within 30s", ref)
}
