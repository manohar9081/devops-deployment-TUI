//go:build darwin

package term

import (
	"fmt"
	"syscall"
	"unsafe"
)

// The termios get/set ioctl request codes on darwin, from <sys/ioctl.h>.
// Darwin has no TCGETS/TCSETS (the linux names), so the BSD TIOCGETA and
// TIOCSETA requests are used instead; TIOCSETA applies the change
// immediately, like TCSANOW.
const (
	tiocgeta = 0x40487413
	tiocseta = 0x80487414
)

// makeRaw applies the `stty raw -echo` settings to fd and returns the
// termios it found there, for Restore. The get runs first: if fd is not a
// terminal the ioctl fails here, before any setting is touched.
func makeRaw(fd uintptr) (syscall.Termios, error) {
	var old syscall.Termios
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, tiocgeta, uintptr(unsafe.Pointer(&old)))
	if errno != 0 {
		return syscall.Termios{}, fmt.Errorf("TIOCGETA: %w", errno)
	}

	raw := old
	raw.Iflag &^= syscall.BRKINT | syscall.ICRNL | syscall.INPCK | syscall.ISTRIP | syscall.IXON
	raw.Oflag &^= syscall.OPOST
	raw.Lflag &^= syscall.ICANON | syscall.ECHO | syscall.ISIG | syscall.IEXTEN
	raw.Cc[syscall.VMIN] = 1
	raw.Cc[syscall.VTIME] = 0

	_, _, errno = syscall.Syscall(syscall.SYS_IOCTL, fd, tiocseta, uintptr(unsafe.Pointer(&raw)))
	if errno != 0 {
		return syscall.Termios{}, fmt.Errorf("TIOCSETA: %w", errno)
	}
	return old, nil
}

// restoreTermios writes saved back to fd with TIOCSETA.
func restoreTermios(fd uintptr, saved *syscall.Termios) error {
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, tiocseta, uintptr(unsafe.Pointer(saved)))
	if errno != 0 {
		return fmt.Errorf("TIOCSETA: %w", errno)
	}
	return nil
}
