// SPDX-License-Identifier: MIT

// Package errs holds knxctl's typed errors, their stable codes and the
// process exit codes they map to (docs/exitcodes.md).
package errs

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net"

	"github.com/otfabric/go-knx"
)

// Kind is a failure category; it selects the exit code.
type Kind string

// Kinds.
const (
	KindUsage        Kind = "usage"
	KindInvalidInput Kind = "invalid_input"
	KindTransport    Kind = "transport"
	KindTimeout      Kind = "timeout"
	KindProtocol     Kind = "protocol"
	KindRejected     Kind = "rejected"
	KindUnsupported  Kind = "unsupported"
	KindPermission   Kind = "permission"
	KindOutput       Kind = "output"
	KindInternal     Kind = "internal"
	// KindInterrupted is a command stopped by SIGINT or SIGTERM before
	// it finished.
	KindInterrupted Kind = "interrupted"
)

// Process exit codes.
const (
	ExitOK           = 0
	ExitFindings     = 1 // a result was written and it reports problems (diagnose)
	ExitUsage        = 2
	ExitInvalidInput = 3
	ExitTransport    = 4
	ExitTimeout      = 5
	ExitProtocol     = 6
	ExitPermission   = 7
	ExitUnsupported  = 8
	ExitRejected     = 9
	ExitInternal     = 10
	ExitInterrupted  = 130 // 128 + SIGINT, as a shell reports it
)

// Stable codes for scripts (the JSON error's "code").
const (
	CodeInvalidInput       = "invalid_input"
	CodeInvalidAddress     = "invalid_address"
	CodeInvalidValue       = "invalid_value"
	CodeMalformed          = "malformed_frame"
	CodeNetwork            = "network_failed"
	CodeConnectionLost     = "connection_lost"
	CodeTimeout            = "timeout"
	CodeNoResponse         = "no_response"
	CodeRejected           = "rejected"
	CodeBusy               = "busy"
	CodeNotConfirmed       = "not_confirmed"
	CodeSecurity           = "security_failed"
	CodeUnsupported        = "unsupported"
	CodePermission         = "permission_denied"
	CodeOutputFailed       = "output_failed"
	CodeCanceled           = "canceled"
	CodeInternalUnexpected = "internal_unexpected"
)

// Error is a typed failure.
type Error struct {
	Kind    Kind   `json:"kind"`
	Code    string `json:"code"`
	Message string `json:"message"`
	Cause   error  `json:"-"`
}

func (e *Error) Error() string { return e.Message }

// Unwrap returns the cause.
func (e *Error) Unwrap() error { return e.Cause }

// New returns a typed error.
func New(kind Kind, code, format string, args ...any) *Error {
	err := fmt.Errorf(format, args...)
	return &Error{Kind: kind, Code: code, Message: err.Error(), Cause: errors.Unwrap(err)}
}

// Invalid is an invalid input error.
func Invalid(format string, args ...any) *Error {
	return New(KindInvalidInput, CodeInvalidInput, format, args...)
}

// ExitCode maps a kind to its exit code.
func ExitCode(k Kind) int {
	switch k {
	case KindUsage:
		return ExitUsage
	case KindInvalidInput:
		return ExitInvalidInput
	case KindTransport:
		return ExitTransport
	case KindTimeout:
		return ExitTimeout
	case KindProtocol:
		return ExitProtocol
	case KindPermission:
		return ExitPermission
	case KindUnsupported:
		return ExitUnsupported
	case KindRejected:
		return ExitRejected
	case KindInterrupted:
		return ExitInterrupted
	default:
		return ExitInternal
	}
}

// Classify turns any error into a typed one, recognising go-knx
// sentinels.
func Classify(err error) *Error {
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	mk := func(k Kind, code string) *Error { return &Error{Kind: k, Code: code, Message: err.Error(), Cause: err} }
	var ne net.Error
	switch {
	case errors.Is(err, context.Canceled):
		return mk(KindInterrupted, CodeCanceled)
	case errors.Is(err, knx.ErrTimeout), errors.Is(err, context.DeadlineExceeded):
		return mk(KindTimeout, CodeTimeout)
	case errors.Is(err, knx.ErrBusy):
		return mk(KindRejected, CodeBusy)
	case errors.Is(err, knx.ErrRejected):
		return mk(KindRejected, CodeRejected)
	case errors.Is(err, knx.ErrNotConfirmed):
		return mk(KindProtocol, CodeNotConfirmed)
	case errors.Is(err, knx.ErrConnectionLost):
		return mk(KindTransport, CodeConnectionLost)
	case errors.Is(err, knx.ErrMalformed):
		return mk(KindProtocol, CodeMalformed)
	case errors.Is(err, knx.ErrSecurity):
		return mk(KindProtocol, CodeSecurity)
	case errors.Is(err, knx.ErrInvalidAddress):
		return mk(KindInvalidInput, CodeInvalidAddress)
	case errors.Is(err, knx.ErrInvalidValue):
		return mk(KindInvalidInput, CodeInvalidValue)
	case errors.Is(err, knx.ErrInvalidOption):
		return mk(KindInvalidInput, CodeInvalidInput)
	case errors.Is(err, knx.ErrUnsupported):
		return mk(KindUnsupported, CodeUnsupported)
	case errors.Is(err, fs.ErrPermission):
		return mk(KindPermission, CodePermission)
	case errors.As(err, &ne), errors.Is(err, knx.ErrClosed):
		return mk(KindTransport, CodeNetwork)
	}
	return mk(KindInternal, CodeInternalUnexpected)
}

// ErrFindings ends a command whose result was written and reports
// problems: exit code 1, no error document.
var ErrFindings = errors.New("findings reported")
