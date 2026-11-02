package buypower

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const testSecret = "whsec_test"

const transferPaid = `{"id":"evt_1","type":"transfer.paid","version":"2","created":"2026-06-21T10:15:42.000Z","data":{"transactionId":88231,"transactionReference":"payout-4821","amount":15000,"stampDuty":50,"transactionFee":25,"totalAmount":15075,"transactionType":"payout","type":"OUTFLOW","status":"CONFIRMED"}}`

func TestConstructEvent(t *testing.T) {
	body := []byte(transferPaid)
	e, err := ConstructEvent(body, ComputeSignature(body, testSecret), testSecret)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := e.Transaction()
	if err != nil {
		t.Fatal(err)
	}
	if e.Type != EventTransferPaid || tx.TotalAmount != 15075 || tx.Type != Outflow {
		t.Fatalf("event=%+v tx=%+v", e, tx)
	}

	if _, err := ConstructEvent(body, ComputeSignature(body, "wrong"), testSecret); err != ErrInvalidSignature {
		t.Fatalf("want ErrInvalidSignature, got %v", err)
	}
	tampered := []byte(strings.Replace(transferPaid, "15000", "99999", 1))
	if _, err := ConstructEvent(tampered, ComputeSignature(body, testSecret), testSecret); err != ErrInvalidSignature {
		t.Fatalf("tampered body accepted: %v", err)
	}
}

func TestConstructEventRejectsV1(t *testing.T) {
	body := []byte(`{"event":"transfer.paid","data":{}}`)
	if _, err := ConstructEvent(body, ComputeSignature(body, testSecret), testSecret); err == nil {
		t.Fatal("v1 payload should be rejected")
	}
}

func TestWebhookHandler(t *testing.T) {
	var got string
	h := WebhookHandler(testSecret, func(ctx context.Context, e *WebhookEvent) error {
		got = e.ID
		return nil
	})

	req := httptest.NewRequest(http.MethodPost, "/wh", strings.NewReader(transferPaid))
	req.Header.Set(PanboxSignatureHeader, ComputeSignature([]byte(transferPaid), testSecret))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 || got != "evt_1" {
		t.Fatalf("code=%d id=%q", rec.Code, got)
	}

	failing := WebhookHandler(testSecret, func(ctx context.Context, e *WebhookEvent) error { return errors.New("db down") })
	req = httptest.NewRequest(http.MethodPost, "/wh", strings.NewReader(transferPaid))
	req.Header.Set(SignatureHeader, ComputeSignature([]byte(transferPaid), testSecret))
	rec = httptest.NewRecorder()
	failing.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("handler error must answer 500 for redelivery, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/wh", strings.NewReader(transferPaid))
	req.Header.Set(SignatureHeader, "deadbeef")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("bad signature: code=%d", rec.Code)
	}
}
