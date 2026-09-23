// Package term provides the small terminal helpers the interactive menu
// needs: TTY detection and window size through the TIOCGWINSZ ioctl, the
// ANSI cursor/clear/erase sequences used by tui_erase, clear and the
// \e[?25l/\e[?25h cursor toggles in devops-deployment.sh, and the
// keystroke decoding of reader.go, whose Reader.ReadKey mirrors the
// script's `read -rsn1` plus `read -rsn2 -t 0.05` escape disambiguation,
// and the raw-mode toggle of raw.go, whose MakeRaw/Restore give a Go
// reader the silent, char-at-a-time tty that bash gets per-read from -rsn.
//
// Everything is stdlib-only: the ioctl runs through syscall.Syscall, with
// the per-OS TIOCGWINSZ request code defined in term_darwin.go and
// term_linux.go (the two platforms the port targets), as are the termios
// get/set requests in raw_darwin.go and raw_linux.go.
package term

import (
	"fmt"
	"io"
	"os"
	"syscall"
	"unsafe"
)

// Fallback dimensions matching devops-deployment.sh's
// `tput cols || echo 80` / `tput lines || echo 24` defaults.
const (
	defaultCols  = 80
	defaultLines = 24
)

// winsize mirrors struct winsize from <sys/ioctl.h>; like the C struct on
// both darwin and linux it starts with the two uint16 counts the ioctl
// fills in, followed by the unused pixel fields.
type winsize struct {
	rows, cols, xpixel, ypixel uint16
}

// IsTTY reports whether fd refers to a terminal, like bash's [[ -t fd ]].
// Asking for the window size is the cheapest portable probe: the ioctl
// succeeds exactly on terminals.
func IsTTY(fd uintptr) bool {
	var ws winsize
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, tiocgwinsz, uintptr(unsafe.Pointer(&ws)))
	return errno == 0
}

// Size returns the terminal's dimensions for fd via TIOCGWINSZ. When the
// ioctl fails (e.g. fd is not a terminal) it returns the same 80x24
// fallback as the bash script's tput defaults, plus the error so callers
// can tell a real size from the fallback.
func Size(fd uintptr) (cols, lines int, err error) {
	var ws winsize
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, tiocgwinsz, uintptr(unsafe.Pointer(&ws)))
	if errno != 0 {
		return defaultCols, defaultLines, fmt.Errorf("TIOCGWINSZ: %w", errno)
	}
	return int(ws.cols), int(ws.rows), nil
}

// The ANSI sequences shared with devops-deployment.sh's TUI.
const (
	hideCursorSeq = "\x1b[?25l"
	showCursorSeq = "\x1b[?25h"
	// clearSeq is what bash's `clear` prints: cursor home, erase the
	// whole display, cursor home again.
	clearSeq = "\x1b[H\x1b[2J" + "\x1b[H"
)

// HideCursor writes the "hide cursor" sequence; the bash script prints it
// before the menu loop and restores the cursor via an EXIT trap.
func HideCursor(w io.Writer) {
	fmt.Fprint(w, hideCursorSeq)
}

// ShowCursor writes the "show cursor" sequence, e.g. before handing the
// terminal to an installer's prompts.
func ShowCursor(w io.Writer) {
	fmt.Fprint(w, showCursorSeq)
}

// ClearScreen clears the terminal like bash's `clear`.
func ClearScreen(w io.Writer) {
	fmt.Fprint(w, clearSeq)
}

// EraseBlock ports tui_erase() from devops-deployment.sh for a block of
// n lines: move the cursor up n lines, wipe each of the n lines (\r then
// \e[2K, stepping down again with \n), then move back up n lines so the
// cursor sits on the block's first line once more. Like the bash printf
// calls it writes to stdout. Each wipe starts with \r because the caller
// leaves the cursor at the previous block's last column and raw mode's \n
// never returns the carriage.
func EraseBlock(n int) {
	eraseBlock(os.Stdout, n)
}

// eraseBlock is EraseBlock against an arbitrary writer, so tests can
// check the exact byte sequence.
func eraseBlock(w io.Writer, n int) {
	fmt.Fprintf(w, "\x1b[%dA", n)
	for i := 0; i < n; i++ {
		fmt.Fprint(w, "\r\x1b[2K\n")
	}
	fmt.Fprintf(w, "\x1b[%dA", n)
}
