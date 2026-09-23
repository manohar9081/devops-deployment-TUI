//go:build !darwin && !linux

package term

import (
	"errors"
	"syscall"
)

// errRawUnsupported is the clear error every raw-mode path returns on
// platforms whose termios ioctls are not wired up (the port targets
// darwin and linux only), so callers can fall back to non-interactive
// mode.
var errRawUnsupported = errors.New("term: raw mode is not supported on this platform")

// makeRaw never touches fd: without the termios ioctls there is nothing to
// apply and nothing to save.
func makeRaw(fd uintptr) (syscall.Termios, error) {
	return syscall.Termios{}, errRawUnsupported
}

// restoreTermios never touches fd; see makeRaw.
func restoreTermios(fd uintptr, saved *syscall.Termios) error {
	return errRawUnsupported
}
