// Keystroke decoding for the interactive menu: Reader.ReadKey turns the raw
// bytes a terminal delivers into KeyMsg values, mirroring tui_loop in
// devops-deployment.sh, which reads one char with `read -rsn1` and
// disambiguates ESC-prefixed sequences with `read -rsn2 -t 0.05`. A terminal
// sends a whole escape sequence (e.g. ESC [ A) in a single burst while a lone
// ESC keypress sends nothing more, and both the script's timeout and the
// escWindow select below rely on exactly that timing — so a real tty source
// gets precisely the bash behavior.

package term

import (
	"errors"
	"io"
	"os"
	"time"
)

// Key classifies a decoded keystroke.
type Key int

// The keystrokes the menu acts on. KeyRune carries the printable character
// in KeyMsg.Rune; KeyOther covers escape sequences that parse but are bound
// to no action (right arrow, PageUp, ...), so the menu can swallow them
// instead of seeing stray runes.
const (
	KeyRune Key = iota
	KeyEnter
	KeyEscape
	KeyUp
	KeyDown
	KeyHome
	KeyEnd
	KeySpace
	KeyCtrlD
	KeyCtrlC
	KeyOther
)

// KeyMsg is one decoded keystroke. Rune is only meaningful for KeyRune.
type KeyMsg struct {
	Type Key
	Rune rune
}

// KeySource supplies the raw input bytes one at a time. It matches io's
// ByteReader, so *bytes.Reader, *bufio.Reader and anything wrapping a tty
// in raw mode can serve as a source; tests feed synthetic byte slices so
// the parsing is table-testable without a terminal.
type KeySource interface {
	ReadByte() (byte, error)
}

// escWindow is how long ReadKey waits after ESC for the rest of an escape
// sequence before reporting a lone KeyEscape, mirroring the script's
// `read -rsn2 -t 0.05`. It is a variable only so tests can shrink it.
var escWindow = 50 * time.Millisecond

// escByte starts every escape sequence.
const escByte = 0x1b

// maxCSIParams bounds how many parameter bytes are swallowed for one CSI
// sequence before giving up; the sequences the menu knows are at most
// three ("1;5").
const maxCSIParams = 16

// readResult is what the escape-window goroutine delivers: the byte read
// after ESC, or the error if the source ended or failed first.
type readResult struct {
	b   byte
	err error
}

// Reader decodes keystrokes from a KeySource. It is not safe for concurrent
// use: one goroutine must own the Reader and nothing else may read from src
// while it is active, because ReadKey temporarily spawns a helper goroutine
// that reads src itself (see readEscape).
//
// A Reader over a terminal is also meant to be owned sequentially across
// the whole program — one Reader per input stream, handed down from phase
// to phase (ask picker → menu loop → any-key pauses). The escape-window
// peek stays consistent only because the same instance continues: a window
// left open when one phase ends parks its follow-up byte on this Reader's
// peek channel, and the next phase's first read drains it. An abandoned
// Reader instead strands that byte — its helper goroutine still holds a
// read on the stream and would swallow the next keystroke from whoever
// reads next. See NewReaderStdin.
type Reader struct {
	src KeySource

	// peek holds the result channel of an escape-window read that lost the
	// race with the timer: its goroutine is still waiting for the byte that
	// would have disambiguated the ESC. The next ReadKey consumes that byte
	// first, so nothing typed is lost — the same place bash finds the
	// follow-up input after `read -t 0.05` fails, still buffered on the tty.
	peek chan readResult
}

// NewReader returns a Reader decoding keystrokes from src.
func NewReader(src KeySource) *Reader {
	return &Reader{src: src}
}

// NewReaderStdin returns a Reader decoding keystrokes from os.Stdin, the
// program's single terminal input stream — the port of bash having exactly
// one input stream to read. Construct it once and hand it down
// sequentially to every interactive phase (menu loop, stack/cloud pickers,
// any-key pauses): each keeps reading the same instance, so an escape
// window left open when a phase ends parks its follow-up byte on the peek
// channel the next phase's read finds — nothing typed is lost between
// phases. Constructing additional Readers over stdin instead strands such
// a byte on whichever Reader was abandoned mid-window, and its helper
// goroutine then swallows the next keystroke from the remaining reader.
func NewReaderStdin() *Reader {
	return NewReader(stdinSource{os.Stdin})
}

// stdinSource adapts *os.File to the one-byte reader KeySource wants
// (os.File has Read, not ReadByte). It is the shared adapter for the
// program's terminal input, used by NewReaderStdin. Reading a single byte
// per call is what raw mode delivers anyway — VMIN=1 — and unlike bufio it
// buffers nothing ahead: the escape window sees an arrow key's bytes one at
// a time, so a lone ESC stays distinguishable from a sequence, and
// type-ahead stays on the tty for the next reader along the line (an
// installer's confirm prompt), the way bash's per-read `read -rsn1` never
// over-consumes either.
type stdinSource struct{ f *os.File }

// ReadByte returns the next byte from the file, reporting io.EOF when the
// read comes back empty.
func (s stdinSource) ReadByte() (byte, error) {
	var b [1]byte
	n, err := s.f.Read(b[:])
	if n == 1 {
		return b[0], nil
	}
	if err == nil {
		err = io.EOF
	}
	return 0, err
}

// ReadKey blocks until one keystroke has been decoded, then returns it.
// It returns the source's error (e.g. io.EOF) once the input is exhausted.
func (r *Reader) ReadKey() (KeyMsg, error) {
	b, err := r.next()
	if err != nil {
		return KeyMsg{}, err
	}
	if b == escByte {
		return r.readEscape()
	}
	return decodeByte(b), nil
}

// next returns the next input byte, first draining a leftover escape-window
// read if one is still in flight (see Reader.peek).
func (r *Reader) next() (byte, error) {
	if r.peek != nil {
		res := <-r.peek
		r.peek = nil
		if res.err != nil {
			return 0, res.err
		}
		return res.b, nil
	}
	return r.src.ReadByte()
}

// decodeByte maps a single non-ESC byte to its keystroke, the way the
// script's case statement matches $'\n', $'\r', ' ' and $'\004'. Anything
// else becomes KeyRune with the byte as its rune, which is all the menu
// needs since its bindings are printable ASCII (j, k, g, G, digits, q).
func decodeByte(b byte) KeyMsg {
	switch b {
	case '\r', '\n':
		return KeyMsg{Type: KeyEnter}
	case ' ':
		return KeyMsg{Type: KeySpace}
	case 0x03:
		return KeyMsg{Type: KeyCtrlC}
	case 0x04:
		return KeyMsg{Type: KeyCtrlD}
	default:
		return KeyMsg{Type: KeyRune, Rune: rune(b)}
	}
}

// readEscape finishes decoding a keystroke whose first byte was ESC: it
// waits up to escWindow for a follow-up byte and either parses the CSI
// sequence that arrived or reports a lone KeyEscape.
//
// The follow-up byte is read by a helper goroutine and selected against a
// timer instead of read directly, because a blocking source (a real tty in
// raw mode) would otherwise hold up the lone-ESC verdict forever. If the
// timer wins, the goroutine keeps waiting and parks its result on r.peek
// for the next ReadKey, so a byte typed just after the window closed is
// still delivered — like bash, which leaves it buffered on the tty.
func (r *Reader) readEscape() (KeyMsg, error) {
	ch := make(chan readResult, 1)
	go func() {
		b, err := r.src.ReadByte()
		ch <- readResult{b, err} // buffered: never blocks, even if abandoned
	}()

	var nb byte
	select {
	case res := <-ch:
		if res.err != nil {
			// EOF right after ESC: the script's `read -rsn2 -t 0.05` fails
			// the same way — immediately, well inside the window — and its
			// `|| true` leaves an empty seq, i.e. a lone ESC. Other errors
			// are real and propagate.
			if errors.Is(res.err, io.EOF) {
				return KeyMsg{Type: KeyEscape}, nil
			}
			return KeyMsg{}, res.err
		}
		nb = res.b
	case <-time.After(escWindow):
		r.peek = ch
		return KeyMsg{Type: KeyEscape}, nil
	}

	if nb != '[' {
		// Not a CSI sequence (e.g. the SS3 forms some terminals send, like
		// ESC O A): as in the script, where the follow-up bytes match no
		// case, degrade to KeyOther and act on nothing.
		return KeyMsg{Type: KeyOther}, nil
	}
	return r.readCSI()
}

// readCSI reads the rest of a CSI sequence (ESC [ params final) and maps it
// to a keystroke. Everything after the windowed byte is read blocking: a
// terminal delivers the whole sequence in one burst, so once the '[' made
// it inside the window the remaining bytes are already there.
func (r *Reader) readCSI() (KeyMsg, error) {
	var params []byte
	for {
		b, err := r.next()
		if err != nil {
			return KeyMsg{}, err
		}
		if b >= 0x40 && b <= 0x7e { // final byte terminates the sequence
			if key, ok := csiKey(params, b); ok {
				return KeyMsg{Type: key}, nil
			}
			return KeyMsg{Type: KeyOther}, nil
		}
		params = append(params, b)
		if len(params) > maxCSIParams {
			// Runaway parameter bytes: give up rather than read forever.
			return KeyMsg{Type: KeyOther}, nil
		}
	}
}

// csiKey maps a parsed CSI sequence to a Key, given the parameter bytes
// between `ESC [` and the final byte. ok is false for sequences the menu
// has no binding for, which ReadKey reports as KeyOther. The recognized
// set mirrors the script's case statement exactly: '[A' up, '[B' down,
// '[H'|'1~'|'7~' home, '[F'|'4~'|'8~' end — bare finals and single-digit
// tilde variants only, so anything else (right arrow, PageUp, modified
// arrows like 1;5A) degrades to KeyOther instead of firing.
func csiKey(params []byte, final byte) (key Key, ok bool) {
	if len(params) == 0 {
		switch final {
		case 'A':
			return KeyUp, true
		case 'B':
			return KeyDown, true
		case 'H':
			return KeyHome, true
		case 'F':
			return KeyEnd, true
		}
		return 0, false
	}
	if final == '~' {
		switch string(params) {
		case "1", "7":
			return KeyHome, true
		case "4", "8":
			return KeyEnd, true
		}
	}
	return 0, false
}
