package buypower

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// Error codes returned in the "status" field of an error envelope.
const (
	CodeValidation   = "validation-error"
	CodeUnauthorized = "unauthorized-error"
	CodeForbidden    = "forbidden"
	CodeNotFound     = "not-found"
	CodeConflict     = "conflict-error"
	CodeServer       = "server-error"
	CodeDatabase     = "database-error"
	CodeGateway      = "gateway-error"
)

// FieldError points at an invalid request field.
type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// APIError is returned for any non-2xx response.
//
//	var apiErr *buypower.APIError
//	if errors.As(err, &apiErr) && apiErr.Code == buypower.CodeValidation { ... }
type APIError struct {
	StatusCode int          // HTTP status
	Code       string       // machine-readable code, e.g. "validation-error"
	Message    string       // human-readable message
	Errors     []FieldError // field-level details for validation failures
	RequestID  string       // x-request-id response header, if any
	Body       []byte       // raw response body

	retryAfter time.Duration
}

func (e *APIError) Error() string {
	msg := fmt.Sprintf("buypower: %d %s: %s", e.StatusCode, e.Code, e.Message)
	for _, f := range e.Errors {
		msg += fmt.Sprintf("; %s %s", f.Field, f.Message)
	}
	return msg
}

// Retryable reports whether the request may succeed if retried (429 / 5xx).
// For money-moving calls, reconcile via Transactions.Get before retrying a 502.
func (e *APIError) Retryable() bool {
	return e.StatusCode == http.StatusTooManyRequests || e.StatusCode >= 500
}

// IsNotFound reports whether err is a 404 from the API.
func IsNotFound(err error) bool {
	var e *APIError
	return errors.As(err, &e) && e.StatusCode == http.StatusNotFound
}

func newAPIError(resp *http.Response, body []byte) *APIError {
	e := &APIError{
		StatusCode: resp.StatusCode,
		RequestID:  resp.Header.Get("x-request-id"),
		Body:       body,
	}
	var payload struct {
		Status  string       `json:"status"`
		Message string       `json:"message"`
		Errors  []FieldError `json:"errors"`
	}
	if json.Unmarshal(body, &payload) == nil {
		e.Code, e.Message, e.Errors = payload.Status, payload.Message, payload.Errors
	}
	if e.Message == "" {
		e.Message = http.StatusText(resp.StatusCode)
	}
	if s, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && s > 0 {
		e.retryAfter = time.Duration(s) * time.Second
	}
	return e
}
