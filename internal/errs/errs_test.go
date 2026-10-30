// SPDX-License-Identifier: MIT

package errs

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"testing"

	"github.com/otfabric/go-knx"
)

func TestExitCodes(t *testing.T) {
	for k, want := range map[Kind]int{
		KindUsage: ExitUsage, KindInvalidInput: ExitInvalidInput, KindTransport: ExitTransport,
		KindTimeout: ExitTimeout, KindProtocol: ExitProtocol, KindPermission: ExitPermission,
		KindUnsupported: ExitUnsupported, KindRejected: ExitRejected, KindOutput: ExitInternal,
		KindInternal:    ExitInternal,
		KindInterrupted: ExitInterrupted,
	} {
		if got := ExitCode(k); got != want {
			t.Errorf("ExitCode(%s) = %d, want %d", k, got, want)
		}
	}
	if ExitOK != 0 || ExitFindings != 1 || ExitInternal != 10 {
		t.Fatal("exit codes changed")
	}
}

func TestClassify(t *testing.T) {
	cases := []struct {
		err  error
		kind Kind
		code string
	}{
		{context.Canceled, KindInterrupted, CodeCanceled},
		{fmt.Errorf("x: %w", knx.ErrTimeout), KindTimeout, CodeTimeout},
		{context.DeadlineExceeded, KindTimeout, CodeTimeout},
		{knx.ErrBusy, KindRejected, CodeBusy},
		{knx.ErrRejected, KindRejected, CodeRejected},
		{knx.ErrNotConfirmed, KindProtocol, CodeNotConfirmed},
		{knx.ErrConnectionLost, KindTransport, CodeConnectionLost},
		{knx.ErrMalformed, KindProtocol, CodeMalformed},
		{knx.ErrSecurity, KindProtocol, CodeSecurity},
		{knx.ErrInvalidAddress, KindInvalidInput, CodeInvalidAddress},
		{knx.ErrInvalidValue, KindInvalidInput, CodeInvalidValue},
		{knx.ErrInvalidOption, KindInvalidInput, CodeInvalidInput},
		{knx.ErrUnsupported, KindUnsupported, CodeUnsupported},
		{fs.ErrPermission, KindPermission, CodePermission},
		{&net.OpError{Op: "read", Err: errors.New("refused")}, KindTransport, CodeNetwork},
		{knx.ErrClosed, KindTransport, CodeNetwork},
		{errors.New("other"), KindInternal, CodeInternalUnexpected},
	}
	for _, c := range cases {
		e := Classify(c.err)
		if e.Kind != c.kind || e.Code != c.code || e.Message != c.err.Error() || !errors.Is(e, c.err) {
			t.Errorf("Classify(%v) = %+v, want %s/%s", c.err, e, c.kind, c.code)
		}
	}
	typed := Invalid("bad %d", 1)
	if Classify(fmt.Errorf("wrapped: %w", typed)) != typed {
		t.Fatal("a typed error is not kept")
	}
}

func TestNew(t *testing.T) {
	cause := errors.New("cause")
	e := New(KindOutput, CodeOutputFailed, "write: %w", cause)
	if e.Error() != "write: cause" || !errors.Is(e, cause) || e.Unwrap() != cause {
		t.Fatalf("%+v", e)
	}
	if e := Invalid("plain"); e.Kind != KindInvalidInput || e.Code != CodeInvalidInput || e.Unwrap() != nil {
		t.Fatalf("%+v", e)
	}
}
