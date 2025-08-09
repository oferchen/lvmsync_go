package common

import (
	"errors"
	"strings"
	"testing"
)

type mockCloser struct{ err error }

func (m mockCloser) Close() error { return m.err }

func TestCloseWithErrSetsError(t *testing.T) {
	closeErr := errors.New("close fail")
	var err error
	CloseWithErr(mockCloser{closeErr}, &err, "context")
	if err == nil || !errors.Is(err, closeErr) {
		t.Fatalf("expected wrapped close error, got %v", err)
	}
	if !strings.Contains(err.Error(), "context") {
		t.Fatalf("expected context message in error, got %v", err)
	}
}

func TestCloseWithErrAppendsError(t *testing.T) {
	baseErr := errors.New("base")
	err := baseErr
	closeErr := errors.New("close fail")
	CloseWithErr(mockCloser{closeErr}, &err, "closing")
	if err == nil || !errors.Is(err, closeErr) {
		t.Fatalf("expected to wrap close error, got %v", err)
	}
	if !strings.Contains(err.Error(), baseErr.Error()) {
		t.Fatalf("expected original error in message, got %v", err)
	}
}

func TestCloseWithErrNoError(t *testing.T) {
	var err error
	CloseWithErr(mockCloser{nil}, &err, "context")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}
