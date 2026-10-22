// Package httpx contains the HTTP conventions shared by every API package: the JSON envelope, the Error
// type, request binding and validation, pagination, rate limiting and the base gin engine with middleware.
package httpx

import (
	"errors"
	"fmt"
	"net/http"
)

// Error codes (docs/ARCHITECTURE.md §HTTP conventions).
const (
	CodeValidation      = "validation_failed" // 400
	CodeUnauthorized    = "unauthorized"      // 401
	CodePaymentFailed   = "payment_failed"    // 402
	CodeForbidden       = "forbidden"         // 403
	CodeNotFound        = "not_found"         // 404
	CodeConflict        = "conflict"          // 409
	CodePayloadTooLarge = "payload_too_large" // 413
	CodeRateLimited     = "rate_limited"      // 429
	CodeInternal        = "internal"          // 500
)

// Error is an API error rendered as {"code","message","errors"} with HTTP status Status.
type Error struct {
	Status  int                 `json:"-"`
	Code    string              `json:"code"`
	Message string              `json:"message"`
	Errors  map[string][]string `json:"errors,omitempty"`
	cause   error
}

func (e *Error) Error() string {
	if e.cause != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.cause)
	}
	return e.Code + ": " + e.Message
}

// Unwrap returns the underlying cause (if any).
func (e *Error) Unwrap() error { return e.cause }

// WithField adds a field-level message and returns e (for chaining).
func (e *Error) WithField(field, msg string) *Error {
	if e.Errors == nil {
		e.Errors = map[string][]string{}
	}
	e.Errors[field] = append(e.Errors[field], msg)
	return e
}

// WithCause attaches an internal cause (logged, never sent to clients) and returns e.
func (e *Error) WithCause(err error) *Error {
	e.cause = err
	return e
}

// NewError builds an Error with an arbitrary status and code.
func NewError(status int, code, msg string) *Error {
	return &Error{Status: status, Code: code, Message: msg}
}

// BadRequest is a 400 validation_failed error.
func BadRequest(msg string) *Error { return NewError(http.StatusBadRequest, CodeValidation, msg) }

// FieldError is a 400 validation_failed error for a single field.
func FieldError(field, msg string) *Error {
	return BadRequest("validation failed").WithField(field, msg)
}

// Unauthorized is a 401 error.
func Unauthorized(msg string) *Error {
	return NewError(http.StatusUnauthorized, CodeUnauthorized, msg)
}

// PaymentFailed is a 402 error.
func PaymentFailed(msg string) *Error {
	return NewError(http.StatusPaymentRequired, CodePaymentFailed, msg)
}

// Forbidden is a 403 error.
func Forbidden(msg string) *Error { return NewError(http.StatusForbidden, CodeForbidden, msg) }

// NotFound is a 404 error.
func NotFound(msg string) *Error { return NewError(http.StatusNotFound, CodeNotFound, msg) }

// Conflict is a 409 error.
func Conflict(msg string) *Error { return NewError(http.StatusConflict, CodeConflict, msg) }

// PayloadTooLarge is a 413 error.
func PayloadTooLarge(msg string) *Error {
	return NewError(http.StatusRequestEntityTooLarge, CodePayloadTooLarge, msg)
}

// RateLimited is a 429 error.
func RateLimited(msg string) *Error {
	return NewError(http.StatusTooManyRequests, CodeRateLimited, msg)
}

// Internal is a 500 error; err is logged but never exposed.
func Internal(err error) *Error {
	return NewError(http.StatusInternalServerError, CodeInternal, "internal server error").WithCause(err)
}

// fieldErrorer is implemented by domain errors carrying per-field messages (e.g. settings.FieldErrors).
type fieldErrorer interface {
	FieldErrors() map[string][]string
}

// AsError converts any error to an *Error:
//   - *Error passes through
//   - gorm.ErrRecordNotFound -> 404, gorm.ErrDuplicatedKey -> 409
//   - *http.MaxBytesError -> 413
//   - validator / JSON binding errors and errors with FieldErrors() -> 400 validation_failed
//   - anything else -> 500 internal
func AsError(err error) *Error {
	if err == nil {
		return nil
	}
	var he *Error
	if errors.As(err, &he) {
		return he
	}
	if e := fromKnown(err); e != nil {
		return e
	}
	var fe fieldErrorer
	if errors.As(err, &fe) {
		e := BadRequest("validation failed")
		e.Errors = fe.FieldErrors()
		return e
	}
	return Internal(err)
}
