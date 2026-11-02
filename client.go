// Package buypower is a Go client for the BuyPower MFB (PanBox) v2 Integration API.
//
//	client := buypower.NewClient(os.Getenv("BP_API_KEY"), os.Getenv("BP_BUSINESS_ID"))
//	banks, err := client.Banking.ListBanks(ctx)
//
// See https://docs.buypowermfb.net/docs/introduction.
package buypower

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	// Version is the SDK version, sent in the User-Agent header.
	Version = "0.1.0"

	LiveURL    = "https://api.buypowermfb.net"
	SandboxURL = "https://api-sandbox.buypowermfb.net"

	defaultTimeout    = 30 * time.Second
	defaultMaxRetries = 2
	maxResponseBytes  = 10 << 20
)

// Environment selects live or sandbox.
type Environment string

const (
	EnvironmentLive    Environment = "live"
	EnvironmentSandbox Environment = "sandbox"
)

// Client is the v2 API client. It is safe for concurrent use.
type Client struct {
	Transfers        *TransfersService
	Refunds          *RefundsService
	Banking          *BankingService
	ReservedAccounts *ReservedAccountsService
	InvoiceAccounts  *InvoiceAccountsService
	Wallets          *WalletsService
	Collections      *CollectionsService
	Transactions     *TransactionsService
	Webhooks         *WebhooksService
	Revenue          *RevenueService

	baseURL    string
	token      string
	env        Environment
	httpClient *http.Client
	userAgent  string
	maxRetries int
	retryBase  time.Duration
}

// Option configures a Client.
type Option func(*Client)

// WithEnvironment points the client at live or sandbox and sends the
// x-api-environment header so the server resolves the same environment.
func WithEnvironment(env Environment) Option {
	return func(c *Client) {
		c.env = env
		if env == EnvironmentLive {
			c.baseURL = LiveURL
		} else {
			c.baseURL = SandboxURL
		}
	}
}

// WithBaseURL overrides the API base URL (e.g. for a proxy or tests).
func WithBaseURL(u string) Option {
	return func(c *Client) { c.baseURL = strings.TrimRight(u, "/") }
}

// WithHTTPClient sets the underlying *http.Client. Default timeout is 30s.
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) { c.httpClient = h }
}

// WithMaxRetries sets how many times a retryable request is retried on
// network errors, 429 and 5xx. Default 2; 0 disables retries.
func WithMaxRetries(n int) Option {
	return func(c *Client) { c.maxRetries = n }
}

// WithUserAgent appends a product token to the User-Agent header.
func WithUserAgent(ua string) Option {
	return func(c *Client) { c.userAgent += " " + ua }
}

// NewClient creates a client authenticated with your API key and Business ID.
// The base URL is inferred from the key prefix (sk_live_ → live, otherwise
// sandbox) unless WithEnvironment or WithBaseURL is given.
func NewClient(apiKey, businessID string, opts ...Option) *Client {
	c := &Client{
		baseURL:    SandboxURL,
		token:      apiKey + ":" + businessID,
		httpClient: &http.Client{Timeout: defaultTimeout},
		userAgent:  "buypowermfb2-go/" + Version,
		maxRetries: defaultMaxRetries,
		retryBase:  500 * time.Millisecond,
	}
	if strings.HasPrefix(apiKey, "sk_live_") {
		c.baseURL = LiveURL
	}
	for _, o := range opts {
		o(c)
	}

	c.Transfers = &TransfersService{c}
	c.Refunds = &RefundsService{c}
	c.Banking = &BankingService{c}
	c.ReservedAccounts = &ReservedAccountsService{c}
	c.InvoiceAccounts = &InvoiceAccountsService{c}
	c.Wallets = &WalletsService{c}
	c.Collections = &CollectionsService{c}
	c.Transactions = &TransactionsService{c}
	c.Webhooks = &WebhooksService{c}
	c.Revenue = &RevenueService{c}
	return c
}

// NewClientFromEnv reads BP_API_KEY and BP_BUSINESS_ID.
func NewClientFromEnv(opts ...Option) (*Client, error) {
	key, id := os.Getenv("BP_API_KEY"), os.Getenv("BP_BUSINESS_ID")
	if key == "" || id == "" {
		return nil, errors.New("buypower: BP_API_KEY and BP_BUSINESS_ID must be set")
	}
	return NewClient(key, id, opts...), nil
}

// RequestOption customises a single API call.
type RequestOption func(http.Header)

// WithIdempotencyKey sets the Idempotency-Key header (honoured on refunds).
func WithIdempotencyKey(key string) RequestOption {
	return func(h http.Header) { h.Set("Idempotency-Key", key) }
}

// WithRequestID sets x-request-id, which support can use to trace a call.
func WithRequestID(id string) RequestOption {
	return func(h http.Header) { h.Set("x-request-id", id) }
}

// WithHeader sets an arbitrary request header.
func WithHeader(key, value string) RequestOption {
	return func(h http.Header) { h.Set(key, value) }
}

type request struct {
	method string
	path   string
	query  url.Values
	body   any
	// retryable marks a non-GET request as safe to retry, e.g. a POST that
	// carries a client reference or an Idempotency-Key.
	retryable bool
}

type envelope struct {
	Status  string          `json:"status"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
	Meta    struct {
		Pagination *Pagination `json:"pagination"`
	} `json:"meta"`
}

func (c *Client) do(ctx context.Context, r request, opts []RequestOption) (*envelope, error) {
	var body []byte
	if r.body != nil {
		var err error
		if body, err = json.Marshal(r.body); err != nil {
			return nil, fmt.Errorf("buypower: encode request: %w", err)
		}
	}
	u := c.baseURL + r.path
	if len(r.query) > 0 {
		u += "?" + r.query.Encode()
	}
	headers := http.Header{}
	for _, o := range opts {
		o(headers)
	}

	retryable := r.method == http.MethodGet || r.retryable
	for attempt := 0; ; attempt++ {
		env, err := c.once(ctx, r.method, u, body, headers)
		if err == nil {
			return env, nil
		}
		if !retryable || attempt >= c.maxRetries || !shouldRetry(ctx, err) {
			return nil, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(c.backoff(attempt, err)):
		}
	}
}

func (c *Client) once(ctx context.Context, method, u string, body []byte, extra http.Header) (*envelope, error) {
	var rdr io.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rdr)
	if err != nil {
		return nil, fmt.Errorf("buypower: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.env != "" {
		req.Header.Set("x-api-environment", string(c.env))
	}
	for k, v := range extra {
		req.Header[k] = v
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("buypower: read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, newAPIError(resp, raw)
	}
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("buypower: decode response (HTTP %d): %w", resp.StatusCode, err)
	}
	return &env, nil
}

func shouldRetry(ctx context.Context, err error) bool {
	if ctx.Err() != nil {
		return false
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.Retryable()
	}
	var urlErr *url.Error
	return errors.As(err, &urlErr) // transport-level failure
}

func (c *Client) backoff(attempt int, err error) time.Duration {
	var apiErr *APIError
	if errors.As(err, &apiErr) && apiErr.retryAfter > 0 {
		return apiErr.retryAfter
	}
	d := min(c.retryBase<<min(attempt, 10), 30*time.Second)
	return d + time.Duration(rand.Int64N(int64(d/4)+1))
}

// call performs a request and decodes envelope.data into T.
func call[T any](ctx context.Context, c *Client, r request, opts []RequestOption) (*T, error) {
	env, err := c.do(ctx, r, opts)
	if err != nil {
		return nil, err
	}
	var out T
	if len(env.Data) > 0 && string(env.Data) != "null" {
		if err := json.Unmarshal(env.Data, &out); err != nil {
			return nil, fmt.Errorf("buypower: decode %s %s: %w", r.method, r.path, err)
		}
	}
	return &out, nil
}

// callList performs a request whose data is a list. The spec declares some
// list payloads as a single object, so both shapes are accepted.
func callList[T any](ctx context.Context, c *Client, r request, opts []RequestOption) (*Page[T], error) {
	env, err := c.do(ctx, r, opts)
	if err != nil {
		return nil, err
	}
	items, err := decodeList[T](env.Data)
	if err != nil {
		return nil, fmt.Errorf("buypower: decode %s %s: %w", r.method, r.path, err)
	}
	return &Page[T]{Data: items, Pagination: env.Meta.Pagination}, nil
}

func decodeList[T any](raw json.RawMessage) ([]T, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	if raw[0] == '[' {
		var out []T
		return out, json.Unmarshal(raw, &out)
	}
	var one T
	if err := json.Unmarshal(raw, &one); err != nil {
		return nil, err
	}
	return []T{one}, nil
}

func setInt(q url.Values, key string, v int) {
	if v > 0 {
		q.Set(key, strconv.Itoa(v))
	}
}
