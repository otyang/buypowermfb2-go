package buypower

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func newTestClient(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c := NewClient("sk_test_abc", "4827", WithBaseURL(srv.URL))
	c.retryBase = time.Millisecond
	return c
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func TestBaseURLInferredFromKey(t *testing.T) {
	if got := NewClient("sk_live_x", "1").baseURL; got != LiveURL {
		t.Fatalf("live key: got %s", got)
	}
	if got := NewClient("sk_test_x", "1").baseURL; got != SandboxURL {
		t.Fatalf("test key: got %s", got)
	}
	if got := NewClient("sk_live_x", "1", WithEnvironment(EnvironmentSandbox)).baseURL; got != SandboxURL {
		t.Fatalf("override: got %s", got)
	}
}

func TestCreateTransfer(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v2/transfers" {
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk_test_abc:4827" {
			t.Fatalf("auth header = %q", got)
		}
		body, _ := io.ReadAll(r.Body)
		want := `{"source":{"type":"collection","balance":"reserved"},"destination":{"bankCode":"058","accountNumber":"0123456789"},"amount":{"value":5000,"currency":"NGN"},"reference":"payout-1"}`
		if string(body) != want {
			t.Fatalf("body =\n%s\nwant\n%s", body, want)
		}
		writeJSON(w, 200, map[string]any{
			"status": "success", "message": "ok",
			"data": map[string]any{
				"reference": "payout-1", "status": "AUTHORIZED",
				"amount":      map[string]any{"value": 5000, "transactionFee": 0, "total": 5000, "currency": "NGN"},
				"destination": map[string]any{"accountNumber": "0123456789", "accountName": "JANE DOE"},
			},
		})
	})

	tr, err := c.Transfers.Create(context.Background(), &CreateTransferRequest{
		Source:      FromCollection(BalanceReserved),
		Destination: TransferDestination{BankCode: "058", AccountNumber: "0123456789"},
		Amount:      NGN(5000),
		Reference:   "payout-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if tr.Status != TxAuthorized || tr.Destination.AccountName != "JANE DOE" || tr.Amount.Total != 5000 {
		t.Fatalf("unexpected transfer: %+v", tr)
	}
}

func TestListTransactionsAndPaginate(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("pageSize") != "100" {
			t.Fatalf("pageSize = %q", r.URL.Query().Get("pageSize"))
		}
		page := r.URL.Query().Get("page")
		n, _ := strconv.Atoi(page)
		writeJSON(w, 200, map[string]any{
			"status": "success", "message": "ok",
			"data": []map[string]any{{"reference": "TX-" + page, "type": "COLLECTION", "status": "CONFIRMED",
				"amount": map[string]any{"value": 1, "fee": nil, "currency": "NGN"}}},
			"meta": map[string]any{"pagination": map[string]any{"page": n, "perPage": 100, "total": 3, "totalPages": 3}},
		})
	})

	var refs []string
	for tx, err := range c.Transactions.ListAll(context.Background(), nil) {
		if err != nil {
			t.Fatal(err)
		}
		if tx.Amount.Fee != nil {
			t.Fatal("null fee should decode to nil")
		}
		refs = append(refs, tx.Reference)
	}
	if strings.Join(refs, ",") != "TX-1,TX-2,TX-3" {
		t.Fatalf("refs = %v", refs)
	}
}

// The spec declares some list payloads as a single object; accept both.
func TestListAcceptsSingleObject(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		// Shape observed on the sandbox, not the one in openapi.json.
		writeJSON(w, 200, map[string]any{"status": "success", "message": "ok",
			"data": map[string]any{"accountNumber": "9716988283", "accountType": "collection.static", "currency": "NGN",
				"balance": map[string]any{"balance": 152000, "blockedAmount": 0}}})
	})
	bals, err := c.Collections.Balances(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(bals) != 1 || bals[0].Balance.Balance != 152000 || bals[0].AccountType != "collection.static" {
		t.Fatalf("balances = %+v", bals)
	}
}

func TestResolveAccountQuery(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "accountNumber=0123456789&bankCode=058" {
			t.Fatalf("query = %s", r.URL.RawQuery)
		}
		writeJSON(w, 200, map[string]any{"status": "success", "message": "ok",
			"data": map[string]any{"accountNumber": "0123456789", "accountName": "JANE DOE", "bankCode": "058"}})
	})
	acc, err := c.Banking.ResolveAccount(context.Background(), "058", "0123456789")
	if err != nil || acc.AccountName != "JANE DOE" {
		t.Fatalf("acc=%+v err=%v", acc, err)
	}
}

func TestValidationError(t *testing.T) {
	var calls atomic.Int32
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("x-request-id", "req_1")
		writeJSON(w, 400, map[string]any{"status": "validation-error", "message": "bad",
			"errors": []map[string]string{{"field": "amount.value", "message": "must be >= 1"}}})
	})
	_, err := c.Transactions.Get(context.Background(), "TX-1")
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("want APIError, got %v", err)
	}
	if apiErr.Code != CodeValidation || apiErr.RequestID != "req_1" || apiErr.Errors[0].Field != "amount.value" {
		t.Fatalf("unexpected error: %+v", apiErr)
	}
	if calls.Load() != 1 {
		t.Fatalf("4xx must not be retried, got %d calls", calls.Load())
	}
}

func TestRetriesGetOn5xx(t *testing.T) {
	var calls atomic.Int32
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) < 3 {
			writeJSON(w, 502, map[string]any{"status": "gateway-error", "message": "upstream"})
			return
		}
		writeJSON(w, 200, map[string]any{"status": "success", "message": "ok", "data": []map[string]string{{"code": "058", "name": "GTBank"}}})
	})
	banks, err := c.Banking.ListBanks(context.Background())
	if err != nil || len(banks) != 1 || calls.Load() != 3 {
		t.Fatalf("banks=%v err=%v calls=%d", banks, err, calls.Load())
	}
}

func TestTransferIsNeverRetried(t *testing.T) {
	var calls atomic.Int32
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		writeJSON(w, 502, map[string]any{"status": "gateway-error", "message": "upstream"})
	})
	_, err := c.Transfers.Create(context.Background(), &CreateTransferRequest{Amount: NGN(1), Reference: "payout-1"})
	if err == nil || calls.Load() != 1 {
		t.Fatalf("err=%v calls=%d", err, calls.Load())
	}
}

func TestRefundRetriesReuseIdempotencyKey(t *testing.T) {
	var keys []string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		keys = append(keys, r.Header.Get("Idempotency-Key"))
		if len(keys) == 1 {
			writeJSON(w, 500, map[string]any{"status": "server-error", "message": "boom"})
			return
		}
		writeJSON(w, 202, map[string]any{"status": "success", "message": "ok",
			"data": map[string]any{"reference": "RF-1", "transactionReference": "TX-1", "status": "processing"}})
	})
	rf, err := c.Refunds.Create(context.Background(), &CreateRefundRequest{TransactionReference: "TX-1", Amount: NGN(100), Reason: "returned"})
	if err != nil || rf.Reference != "RF-1" {
		t.Fatalf("rf=%+v err=%v", rf, err)
	}
	if len(keys) != 2 || keys[0] == "" || keys[0] != keys[1] {
		t.Fatalf("idempotency keys = %v", keys)
	}
}

func TestRefundListUsesSize(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "page=2&size=20&status=completed" {
			t.Fatalf("query = %s", r.URL.RawQuery)
		}
		writeJSON(w, 200, map[string]any{"status": "success", "message": "ok", "data": []any{}})
	})
	if _, err := c.Refunds.List(context.Background(), &RefundListParams{Page: 2, Size: 20, Status: "completed"}); err != nil {
		t.Fatal(err)
	}
}

func TestUpsertSubscriptionsDefaultsToV2(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if string(body) != `[{"url":"https://x.test/wh","topic":"collections","payloadVersion":"2"}]` {
			t.Fatalf("body = %s", body)
		}
		// Shape observed on the sandbox: [[row, created], ...].
		writeJSON(w, 200, map[string]any{"status": "success", "message": "ok",
			"data": []any{[]any{map[string]any{"webhookUrl": "https://x.test/wh", "topic": "collections"}, false}}})
	})
	if err := c.Webhooks.UpsertSubscriptions(context.Background(), []WebhookSubscriptionInput{{URL: "https://x.test/wh", Topic: TopicCollections}}); err != nil {
		t.Fatal(err)
	}
}

func TestRevenueDates(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "from=2026-06-01&to=2026-07-01" {
			t.Fatalf("query = %s", r.URL.RawQuery)
		}
		writeJSON(w, 200, map[string]any{"status": "success", "message": "ok", "data": map[string]any{"totalRevenue": 250}})
	})
	rev, err := c.Revenue.Get(context.Background(), RevenueParams{
		From: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		To:   time.Date(2026, 6, 30, 23, 30, 0, 0, time.UTC), // 00:30 on 1 July in Lagos
	})
	if err != nil || rev.TotalRevenue != 250 {
		t.Fatalf("rev=%+v err=%v", rev, err)
	}
}

func TestBackoffIsCapped(t *testing.T) {
	c := NewClient("sk_test_x", "1")
	for _, attempt := range []int{0, 10, 40, 100} {
		if d := c.backoff(attempt, errors.New("x")); d <= 0 || d > 40*time.Second {
			t.Fatalf("attempt %d: backoff %v", attempt, d)
		}
	}
}
