# buypowermfb2

[![Go Reference](https://pkg.go.dev/badge/github.com/otyang/buypowermfb2.svg)](https://pkg.go.dev/github.com/otyang/buypowermfb2)
[![CI](https://github.com/otyang/buypowermfb2/actions/workflows/ci.yml/badge.svg)](https://github.com/otyang/buypowermfb2/actions/workflows/ci.yml)

Go SDK for the [BuyPower MFB](https://docs.buypowermfb.net/docs/introduction) payments API. Covers transfers, refunds, virtual accounts, wallets, collections, transactions, revenue and signed webhooks.

> [!CAUTION]
> **This SDK targets version 2 of the BuyPower MFB API only (`/v2/...`).**
>
> It does not support the deprecated `/v1` or `/api` endpoints, the `/admin` endpoints, or webhook payload version 1. If your integration still calls `/v1`, migrate first using BuyPower's [v1 to v2 guide](https://docs.buypowermfb.net/docs/migration/v1-to-v2). Webhook subscriptions must use `payloadVersion: "2"`; this SDK registers them that way by default.

> [!NOTE]
> This is a community SDK. It is not published or supported by BuyPower MFB. For account or API problems, contact BuyPower at developers@buypowermfb.net.

- Standard library only, no dependencies
- Go 1.24 or newer
- Tested against the live sandbox (see [Testing](#testing))

## Install

```sh
go get github.com/otyang/buypowermfb2
```

```go
import buypower "github.com/otyang/buypowermfb2"
```

## Quick start

```go
client := buypower.NewClient(os.Getenv("BP_API_KEY"), os.Getenv("BP_BUSINESS_ID"))
// sk_live_ keys go to https://api.buypowermfb.net, everything else to the sandbox.
// Override with buypower.WithEnvironment(buypower.EnvironmentLive) or WithBaseURL(...).

acct, err := client.Banking.ResolveAccount(ctx, "058", "0123456789")

tr, err := client.Transfers.Create(ctx, &buypower.CreateTransferRequest{
    Source:      buypower.FromCollection(buypower.BalanceReserved), // or buypower.FromWallet("9006990538")
    Destination: buypower.TransferDestination{BankCode: "058", AccountNumber: "0123456789"},
    Amount:      buypower.NGN(5000), // naira, not kobo
    Reference:   "payout-4821",      // your idempotent reference
})

for tx, err := range client.Transactions.ListAll(ctx, nil) { ... } // walks every page
```

Runnable programs are in [examples/quickstart](examples/quickstart) and [examples/webhooks](examples/webhooks).

## Endpoints

| Service | Method | Endpoint |
|---|---|---|
| `Transfers` | `Create` | `POST /v2/transfers` |
| `Refunds` | `Create` / `List` / `ListAll` / `Get` | `POST /v2/refunds`, `GET /v2/refunds`, `GET /v2/refunds/{ref}` |
| `Banking` | `ListBanks` / `ResolveAccount` | `GET /v2/banking/banks`, `GET /v2/banking/accounts/resolve` |
| `ReservedAccounts` | `Create` / `List` / `ListAll` / `Balance` / `Rename` | `/v2/accounts/reserved[/{ref}[/balance]]` |
| `InvoiceAccounts` | `Create` / `List` / `ListAll` | `/v2/accounts/invoices` |
| `Wallets` | `Create` / `List` / `ListAll` / `Get` / `Balance` | `/v2/wallets[/{ref}[/balance]]` |
| `Collections` | `ListAccounts` / `Balances` | `GET /v2/collections/accounts`, `GET /v2/collections/balances` |
| `Transactions` | `List` / `ListAll` / `Get` | `GET /v2/transactions[/{ref}]` |
| `Webhooks` | `ListSubscriptions` / `UpsertSubscriptions` | `GET`/`PUT /v2/webhooks/subscriptions` |
| `Revenue` | `Get` | `GET /v2/revenue` |

`GET /v2/collections/wallets` is left out because it is a deprecated alias of `GET /v2/wallets`.

Revenue dates are read in Lagos time (Africa/Lagos), whatever time zone you pass.

## Errors

Non-2xx responses return `*buypower.APIError` with `StatusCode`, `Code` (`validation-error`, `conflict-error`, `gateway-error`, …), `Message`, field-level `Errors` and `RequestID`.

```go
var apiErr *buypower.APIError
if errors.As(err, &apiErr) && apiErr.Code == buypower.CodeValidation { /* fix the request */ }
if buypower.IsNotFound(err) { ... }
```

## Retries and idempotency

Network errors, 429 and 5xx responses are retried with exponential backoff and jitter. The default is 2 retries; change it with `WithMaxRetries`. Retries happen only when they can't move money twice:

- `GET` requests, the subscription upsert, and account renames are always retried.
- `ReservedAccounts.Create`, `InvoiceAccounts.Create` and `Wallets.Create` are retried only when `Reference` is set.
- `Transfers.Create` is never retried. After a timeout or a 502 the money may still move, so check `Transactions.Get(ctx, reference)` first, then retry yourself with the same `Reference`.
- `Refunds.Create` sends an `Idempotency-Key` that it generates once per call and reuses on retries. To keep the same key across process restarts, pass your own with `buypower.WithIdempotencyKey(key)`.

Transfers and refunds are asynchronous. Confirm the final state with `Transactions.Get` or a webhook.

## Webhooks

```go
http.Handle("/webhooks/buypower", buypower.WebhookHandler(secret, func(ctx context.Context, e *buypower.WebhookEvent) error {
    switch e.Type {
    case buypower.EventTransferPaid:
        tx, _ := e.Transaction()
        ...
    case buypower.EventRefundCompleted:
        rf, _ := e.Refund()
        ...
    }
    return nil // non-nil → 500 → BuyPower redelivers
}))
```

The handler checks the HMAC-SHA256 signature over the raw body, compares it in constant time, and parses the v2 envelope (`id`, `type`, `version`, `created`, `data`). Retries of one delivery share the same `e.ID`, so de-duplicate on it. If you aren't using `net/http`, call `buypower.ConstructEvent(rawBody, signature, secret)` directly.

`UpsertSubscriptions` defaults to `payloadVersion: "2"` and returns only an error. Read the result back with `ListSubscriptions`, because the server's upsert reply echoes your webhook secret.

## Testing

There are three levels of tests. Each person uses their own sandbox credentials through environment variables; never commit them.

| Level | Needs | Changes sandbox data | Command |
|---|---|---|---|
| Offline | Go only | No | `go test ./...` |
| Sandbox reads | Sandbox API key and business ID | No | `go test -run Sandbox -v` |
| Sandbox writes | The above, plus `BP_SANDBOX_WRITE=1` | Yes | `BP_SANDBOX_WRITE=1 go test -run Sandbox -v -timeout 10m` |
| Webhooks | The above, plus a public tunnel and your webhook secret | Yes, and repoints your webhook subscriptions | see [Webhook test](#webhook-test) |

### Offline tests

```sh
go test ./...
```

These use a fake server, so they need no credentials and no network. CI runs them on every push and pull request.

### Sandbox tests

These run against the real BuyPower sandbox. They are skipped unless `BP_API_KEY` and `BP_BUSINESS_ID` are set, and they refuse anything other than an `sk_test_` key, so they can't touch live money.

```sh
export BP_API_KEY=sk_test_...          # the API key only, without the ":businessId" part
export BP_BUSINESS_ID=<your-business-id>

# Read-only checks: change nothing on the account
go test -run Sandbox -v

# Also create a reserved account, an invoice, a ₦100 transfer and a refund
BP_SANDBOX_WRITE=1 go test -run Sandbox -v -timeout 10m
```

Every run uses fresh references, so runs by different people never clash.

Known sandbox problems show as `SKIP` with the reason instead of failing:
- wallets not enabled for the business;
- the reserved-account balance endpoint answering "This is not a wallet account";
- an empty collection balance refusing the test transfer.

These tests start passing by themselves once the sandbox is fixed or the account is set up.

### Webhook test

This test makes the sandbox send real signed webhooks to your machine and checks they arrive and verify. It needs a public URL that forwards to a local port.

1. Start a tunnel to the test port (default `9990`) and copy the `https://….trycloudflare.com` URL it prints:
   ```sh
   cloudflared tunnel --url http://localhost:9990
   ```
2. Run the test with the tunnel URL and your webhook secret (dashboard, **API Keys** tab):
   ```sh
   BP_SANDBOX_WRITE=1 \
   BP_WEBHOOK_URL=https://<your-tunnel>.trycloudflare.com/webhooks/buypower \
   BP_WEBHOOK_SECRET=whsec_test_... \
   go test -run SandboxWebhooks -v -timeout 10m
   ```

What it does:
- It listens on `BP_WEBHOOK_PORT` (default `9990`). Nothing else may use that port while it runs.
- It **points your sandbox webhook subscriptions** (collections, transfers, refunds) at `BP_WEBHOOK_URL` and leaves them there.
- It sends a test payment and a ₦100 transfer, then waits up to 90 seconds for each signed event.

A wrong secret fails with a clear message.

To watch events by hand instead, run the example receiver on the same port:

```sh
BP_WEBHOOK_SECRET=whsec_test_... PORT=9990 go run ./examples/webhooks
```

### Environment variables

| Variable | Used by | Meaning |
|---|---|---|
| `BP_API_KEY` | sandbox tests, examples | Sandbox API key (`sk_test_…`) |
| `BP_BUSINESS_ID` | sandbox tests, examples | Your numeric business ID |
| `BP_SANDBOX_WRITE` | sandbox tests | Set to `1` to allow tests that create data |
| `BP_WEBHOOK_URL` | webhook test | Public URL that forwards to `BP_WEBHOOK_PORT` |
| `BP_WEBHOOK_SECRET` | webhook test, webhook example | Webhook secret (`whsec_…`) |
| `BP_WEBHOOK_PORT` | webhook test | Local port for the test receiver (default `9990`) |
| `PORT` | webhook example | Local port for the example receiver (default `9990`) |

## Where the API differs from its spec

Checked against the sandbox on 2 November 2026:

- List endpoints return arrays, even where `openapi.json` declares a single object. The SDK accepts either.
- Collection accounts and balances use `accountType` and a nested `balance.balance`, not the spec's `type` and `available`. The SDK follows the server.
- Transfer `status` uses the transaction statuses (`AUTHORIZED`, `CONFIRMED`, …), not the docs' `pending`/`paid`/`failed`. Compare against the `Tx*` constants.
- Validation errors list the bad fields inside `message`. The `errors` array is not sent.
- Insufficient balance on a transfer returns 502 `gateway-error`, not the documented 422 `conflict-error`.
- `ReservedAccounts.Balance` returns `json.RawMessage` because the spec doesn't type it. On the sandbox it currently fails with "This is not a wallet account", which is a server bug.

## License

[MIT](LICENSE)
