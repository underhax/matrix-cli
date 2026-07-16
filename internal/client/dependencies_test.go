package client

import (
	"bytes"
	"errors"
	"testing"

	"go.mau.fi/util/dbutil"
)

func TestWrapErr(t *testing.T) {
	if err := wrapErr(nil, "test"); err != nil {
		t.Errorf("expected nil, got %v", err)
	}

	baseErr := errors.New("base error")
	wrapped := wrapErr(baseErr, "wrapped: %w")
	if wrapped == nil || wrapped.Error() != "wrapped: base error" {
		t.Errorf("unexpected wrap result: %v", wrapped)
	}
}

func TestDefaultGetOlmMachine(t *testing.T) {
	c := &Client{}
	mach := defaultGetOlmMachine(c)
	if mach != nil {
		t.Errorf("expected nil machine, got %v", mach)
	}
}

type mockRows struct {
	dbutil.Rows
	closeErr error
}

func (m *mockRows) Close() error {
	return m.closeErr
}

func TestDefaultRowsClose(t *testing.T) {
	if err := defaultRowsClose(nil); err != nil {
		t.Errorf("expected nil error for nil rows, got %v", err)
	}

	successRows := &mockRows{closeErr: nil}
	if err := defaultRowsClose(successRows); err != nil {
		t.Errorf("expected nil error, got %v", err)
	}

	failRows := &mockRows{closeErr: errors.New("close fail")}
	if err := defaultRowsClose(failRows); err == nil || err.Error() != "failed to close rows: close fail" {
		t.Errorf("unexpected error result: %v", err)
	}
}

type errWriter struct{}

func (errWriter) Write(_ []byte) (n int, err error) {
	return 0, errors.New("write error")
}

func TestStderrWrappers(t *testing.T) {
	oldStderr := stderr
	defer func() { stderr = oldStderr }()

	var buf bytes.Buffer
	stderr = &buf

	fprintfStderr("test %d", 1)
	if buf.String() != "test 1" {
		t.Errorf("fprintfStderr output mismatch: got %q", buf.String())
	}

	buf.Reset()
	fprintlnStderr("test")
	if buf.String() != "test\n" {
		t.Errorf("fprintlnStderr output mismatch: got %q", buf.String())
	}

	stderr = errWriter{}
	fprintfStderr("test")
	fprintlnStderr("test")
}
