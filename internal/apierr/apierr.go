// Package apierr defines typed errors that map to HTTP responses.
package apierr

import (
	"errors"
	"fmt"
	"net/http"
)

// Error is an error with an HTTP status and a stable machine-readable code.
type Error struct {
	Status  int
	Code    string
	Message string
	Err     error
}

func (e *Error) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	}
	return e.Message
}

func (e *Error) Unwrap() error { return e.Err }

func New(status int, code, msg string) *Error {
	return &Error{Status: status, Code: code, Message: msg}
}

func BadRequest(msg string, err error) *Error {
	return &Error{Status: http.StatusBadRequest, Code: "BAD_REQUEST", Message: msg, Err: err}
}

func NotFound(code, msg string) *Error {
	return &Error{Status: http.StatusNotFound, Code: code, Message: msg}
}

var (
	ErrUnauthorized = New(http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
	ErrForbidden    = New(http.StatusForbidden, "FORBIDDEN", "insufficient permissions")
	ErrReadOnly     = New(http.StatusForbidden, "CLUSTER_READ_ONLY", "cluster is read-only")
	ErrClusterNF    = NotFound("CLUSTER_NOT_FOUND", "cluster not found")
	ErrCSRF         = New(http.StatusForbidden, "CSRF", "missing X-Requested-With header")
	ErrOffline      = New(http.StatusServiceUnavailable, "CLUSTER_OFFLINE", "cluster is unreachable")
)

// From converts any error into an *Error, defaulting to 500.
func From(err error) *Error {
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	return &Error{Status: http.StatusInternalServerError, Code: "INTERNAL", Message: "internal error", Err: err}
}
