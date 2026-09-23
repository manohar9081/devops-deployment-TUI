// Raw mode for the interactive menu: MakeRaw switches a tty to the
// byte-at-a-time, no-echo state that bash gets per-read from the script's
// `read -rsn1` (the -s silences echo and no line editing happens because
// bash reads one char at a time), and which Reader.ReadKey needs when its
// source is a real terminal. Restore puts the original settings back, the
// way the script's EXIT trap hands back a sane terminal.

package term

import (
	"errors"
	"syscall"
)

// State carries the terminal settings MakeRaw captured, plus the fd they
// belong to, so Restore can put them back. The zero State is invalid and
// Restore rejects it instead of writing a zeroed termios over a live
// terminal (fd 0 would be the victim of exactly that). The saved termios is
// kept by value rather than behind a pointer stuffed into a uintptr, so the
// garbage collector sees it as an ordinary field and only the ioctl
// argument itself ever goes through unsafe.
type State struct {
	fd      uintptr
	termios syscall.Termios
	valid   bool
}

// MakeRaw puts fd into raw mode, mirroring `stty raw -echo`: the input
// flags brkint, icrnl, inpck, istrip and ixon are cleared, output
// post-processing (opost) is turned off, the line-discipline flags echo,
// icanon, isig and iexten are cleared, and VMIN=1/VTIME=0 make reads
// return as soon as one byte arrives. It returns the captured state for
// Restore.
//
// Callers must defer Restore for the same fd, and wire SIGINT/SIGTERM to
// Restore plus ShowCursor, so an interrupt during the menu cannot leave
// the terminal in raw mode with a hidden cursor. When fd is not a terminal
// — or the platform has no termios ioctls — MakeRaw returns a clear error
// and no state, and callers should fall back to non-interactive mode.
func MakeRaw(fd uintptr) (*State, error) {
	old, err := makeRaw(fd)
	if err != nil {
		return nil, err
	}
	return &State{fd: fd, termios: old, valid: true}, nil
}

// Restore writes the settings MakeRaw captured back to the terminal. It is
// safe to call more than once — e.g. from both a signal handler and the
// deferred cleanup — because restoring a saved state is idempotent, and it
// rejects a nil or zero State rather than saving zeroes over a live
// terminal.
func Restore(s *State) error {
	if s == nil || !s.valid {
		return errors.New("term: Restore of a State that MakeRaw did not return")
	}
	return restoreTermios(s.fd, &s.termios)
}
