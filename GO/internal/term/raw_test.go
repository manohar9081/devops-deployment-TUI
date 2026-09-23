package term

import (
	"errors"
	"syscall"
	"testing"
)

// TestRestoreRejectsInvalidStates covers the zero-value handling the menu
// relies on: a nil State (MakeRaw failed, so the deferred Restore got nil)
// and the zero State (Restore called without MakeRaw) must both be
// rejected instead of writing a zeroed termios over a live terminal.
func TestRestoreRejectsInvalidStates(t *testing.T) {
	if err := Restore(nil); err == nil {
		t.Fatal("Restore(nil) should return an error")
	}
	if err := Restore(&State{}); err == nil {
		t.Fatal("Restore of the zero State should return an error")
	}
}

// TestMakeRawFailsOnNonTTY checks the fallback path on a regular file, notTTY
// being an ordinary file whose termios ioctl fails with ENOTTY on both
// target platforms. The get runs before any set, so nothing is mutated and
// the error — wrapped with %w — is one callers can test for when deciding to
// fall back to non-interactive mode.
func TestMakeRawFailsOnNonTTY(t *testing.T) {
	st, err := MakeRaw(notTTY(t))
	if err == nil {
		t.Fatal("MakeRaw on a regular file should fail")
	}
	if !errors.Is(err, syscall.ENOTTY) {
		t.Fatalf("MakeRaw on a regular file: err = %v, want ENOTTY", err)
	}
	if st != nil {
		t.Fatalf("MakeRaw on a regular file returned %+v, want nil", st)
	}
}

// The per-OS plumbing behind the shared API — makeRaw and restoreTermios in
// raw_darwin.go, raw_linux.go and raw_other.go — is pinned to these
// signatures at compile time. This file has no build tag, so the check runs
// on every platform raw.go builds on.
var (
	_ func(uintptr) (syscall.Termios, error) = makeRaw
	_ func(uintptr, *syscall.Termios) error  = restoreTermios
)
