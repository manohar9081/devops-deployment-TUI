//go:build linux

package term

import (
	"fmt"
	"syscall"
	"unsafe"
)

// The termios get/set ioctl request codes on linux, from
// <asm-generic/ioctls.h>. TCSETS applies the change immediately, like
// TCSANOW.
const (
	tcgets = 0x5401
	tcsets = 0x5402
)

// makeRaw applies the `stty raw -echo` settings to fd and returns the
// termios it found there, for Restore. The get runs first: if fd is not a
// terminal the ioctl fails here, before any setting is touched.
func makeRaw(fd uintptr) (syscall.Termios, error) {
	var old syscall.Termios
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, tcgets, uintptr(unsafe.Pointer(&old)))
	if errno != 0 {
		return syscall.Termios{}, fmt.Errorf("TCGETS: %w", errno)
	}

	raw := old
	raw.Iflag &^= syscall.BRKINT | syscall.ICRNL | syscall.INPCK | syscall.ISTRIP | syscall.IXON
	raw.Oflag &^= syscall.OPOST
	raw.Lflag &^= syscall.ICANON | syscall.ECHO | syscall.ISIG | syscall.IEXTEN
	raw.Cc[syscall.VMIN] = 1
	raw.Cc[syscall.VTIME] = 0

	_, _, errno = syscall.Syscall(syscall.SYS_IOCTL, fd, tcsets, uintptr(unsafe.Pointer(&raw)))
	if errno != 0 {
		return syscall.Termios{}, fmt.Errorf("TCSETS: %w", errno)
	}
	return old, nil
}

// restoreTermios writes saved back to fd with TCSETS.
func restoreTermios(fd uintptr, saved *syscall.Termios) error {
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, tcsets, uintptr(unsafe.Pointer(saved)))
	if errno != 0 {
		return fmt.Errorf("TCSETS: %w", errno)
	}
	return nil
}
