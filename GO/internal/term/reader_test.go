package term

import (
	"errors"
	"io"
	"testing"
	"time"
)

// sliceSource is a synthetic KeySource over a fixed byte slice: ReadByte
// walks the bytes and reports io.EOF at the end, so a lone ESC resolves
// through the fast path (the follow-up read fails immediately, well inside
// the window) exactly like a tty where nothing follows the ESC.
type sliceSource struct {
	b []byte
	i int
}

func (s *sliceSource) ReadByte() (byte, error) {
	if s.i >= len(s.b) {
		return 0, io.EOF
	}
	b := s.b[s.i]
	s.i++
	return b, nil
}

// slowSource serves its first byte immediately and delays every later byte,
// standing in for a tty whose user pressed ESC and only pressed another key
// long after the disambiguation window had closed.
type slowSource struct {
	delay time.Duration
	b     []byte
	i     int
}

func (s *slowSource) ReadByte() (byte, error) {
	if s.i >= len(s.b) {
		return 0, io.EOF
	}
	if s.i > 0 {
		time.Sleep(s.delay)
	}
	b := s.b[s.i]
	s.i++
	return b, nil
}

func TestReadKey(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    KeyMsg
		wantErr error
	}{
		{"rune letter", "a", KeyMsg{Type: KeyRune, Rune: 'a'}, nil},
		{"rune digit", "5", KeyMsg{Type: KeyRune, Rune: '5'}, nil},
		{"enter CR", "\r", KeyMsg{Type: KeyEnter}, nil},
		{"enter LF", "\n", KeyMsg{Type: KeyEnter}, nil},
		{"space", " ", KeyMsg{Type: KeySpace}, nil},
		{"ctrl-d", "\x04", KeyMsg{Type: KeyCtrlD}, nil},
		{"ctrl-c", "\x03", KeyMsg{Type: KeyCtrlC}, nil},
		{"lone ESC at end of input", "\x1b", KeyMsg{Type: KeyEscape}, nil},
		{"up", "\x1b[A", KeyMsg{Type: KeyUp}, nil},
		{"down", "\x1b[B", KeyMsg{Type: KeyDown}, nil},
		{"home via [H", "\x1b[H", KeyMsg{Type: KeyHome}, nil},
		{"home via [1~", "\x1b[1~", KeyMsg{Type: KeyHome}, nil},
		{"home via [7~", "\x1b[7~", KeyMsg{Type: KeyHome}, nil},
		{"end via [F", "\x1b[F", KeyMsg{Type: KeyEnd}, nil},
		{"end via [4~", "\x1b[4~", KeyMsg{Type: KeyEnd}, nil},
		{"end via [8~", "\x1b[8~", KeyMsg{Type: KeyEnd}, nil},
		{"modified arrow degrades", "\x1b[1;5A", KeyMsg{Type: KeyOther}, nil},
		{"unknown CSI right arrow", "\x1b[C", KeyMsg{Type: KeyOther}, nil},
		{"unknown CSI page-up", "\x1b[5~", KeyMsg{Type: KeyOther}, nil},
		{"unknown CSI insert", "\x1b[2~", KeyMsg{Type: KeyOther}, nil},
		{"unknown CSI shift-tab", "\x1b[Z", KeyMsg{Type: KeyOther}, nil},
		{"unknown CSI long parameter", "\x1b[?25A", KeyMsg{Type: KeyOther}, nil},
		{"SS3 escape degrades", "\x1bOA", KeyMsg{Type: KeyOther}, nil},
		{"non-CSI follow-up byte degrades", "\x1bQ", KeyMsg{Type: KeyOther}, nil},
		{"empty input", "", KeyMsg{}, io.EOF},
		{"ESC then [ then EOF", "\x1b[", KeyMsg{}, io.EOF},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := NewReader(&sliceSource{b: []byte(tc.in)})
			got, err := r.ReadKey()
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("ReadKey() error = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ReadKey() unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("ReadKey() = %+v, want %+v", got, tc.want)
			}
		})
	}
}

// TestReadKeySequence drives several keystrokes through one Reader, which is
// how the menu loop consumes input, and checks that an unknown CSI sequence
// is swallowed whole instead of leaking stray runes into the next reads.
func TestReadKeySequence(t *testing.T) {
	r := NewReader(&sliceSource{b: []byte("\x1b[A \x1b[Bq\x1b[5~z")})
	want := []KeyMsg{
		{Type: KeyUp},
		{Type: KeySpace},
		{Type: KeyDown},
		{Type: KeyRune, Rune: 'q'},
		{Type: KeyOther}, // page-up: parsed, but bound to nothing
		{Type: KeyRune, Rune: 'z'},
	}
	for i, w := range want {
		got, err := r.ReadKey()
		if err != nil {
			t.Fatalf("key %d: unexpected error: %v", i, err)
		}
		if got != w {
			t.Fatalf("key %d = %+v, want %+v", i, got, w)
		}
	}
	if _, err := r.ReadKey(); !errors.Is(err, io.EOF) {
		t.Fatalf("after the sequence, error = %v, want io.EOF", err)
	}
}

// TestReadKeyEscapeWindowTimeout exercises the goroutine-plus-timer path: the
// follow-up byte arrives after the window closed, so the ESC resolves as a
// lone escape and the late byte is delivered to the next ReadKey rather than
// being lost.
func TestReadKeyEscapeWindowTimeout(t *testing.T) {
	// Shrink the window so the test stays quick while keeping a wide margin:
	// the follow-up byte arrives 20x the window later, so the timer wins.
	oldWindow := escWindow
	escWindow = 5 * time.Millisecond
	t.Cleanup(func() { escWindow = oldWindow })

	r := NewReader(&slowSource{delay: 100 * time.Millisecond, b: []byte("\x1bx")})

	got, err := r.ReadKey()
	if err != nil {
		t.Fatalf("ReadKey() unexpected error: %v", err)
	}
	wantEscape := KeyMsg{Type: KeyEscape}
	if got != wantEscape {
		t.Fatalf("ReadKey() = %+v, want %+v (timer should beat the follow-up byte)", got, wantEscape)
	}

	got, err = r.ReadKey()
	if err != nil {
		t.Fatalf("ReadKey() after the window: unexpected error: %v", err)
	}
	wantLate := KeyMsg{Type: KeyRune, Rune: 'x'}
	if got != wantLate {
		t.Fatalf("ReadKey() after the window = %+v, want %+v (late byte must not be lost)", got, wantLate)
	}
}

func TestDecodeByte(t *testing.T) {
	cases := []struct {
		name string
		b    byte
		want KeyMsg
	}{
		{"CR", '\r', KeyMsg{Type: KeyEnter}},
		{"LF", '\n', KeyMsg{Type: KeyEnter}},
		{"space", ' ', KeyMsg{Type: KeySpace}},
		{"ctrl-c", 0x03, KeyMsg{Type: KeyCtrlC}},
		{"ctrl-d", 0x04, KeyMsg{Type: KeyCtrlD}},
		{"letter", 'a', KeyMsg{Type: KeyRune, Rune: 'a'}},
		{"digit", '9', KeyMsg{Type: KeyRune, Rune: '9'}},
		{"unbound control byte", 0x01, KeyMsg{Type: KeyRune, Rune: 0x01}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := decodeByte(tc.b); got != tc.want {
				t.Fatalf("decodeByte(%#x) = %+v, want %+v", tc.b, got, tc.want)
			}
		})
	}
}

func TestCSIKey(t *testing.T) {
	cases := []struct {
		params string
		final  byte
		want   Key
		ok     bool
	}{
		{"", 'A', KeyUp, true},
		{"", 'B', KeyDown, true},
		{"", 'H', KeyHome, true},
		{"", 'F', KeyEnd, true},
		{"1", '~', KeyHome, true},
		{"7", '~', KeyHome, true},
		{"4", '~', KeyEnd, true},
		{"8", '~', KeyEnd, true},
		{"1;5", 'A', 0, false}, // modified arrow, not bound by the menu
		{"", 'C', 0, false},    // right arrow
		{"", 'D', 0, false},    // left arrow
		{"5", '~', 0, false},   // page-up
		{"6", '~', 0, false},   // page-down
		{"", '~', 0, false},    // bare tilde
		{"2", '~', 0, false},   // insert
		{"22", '~', 0, false},  // unsupported tilde variant
	}
	for _, tc := range cases {
		name := string(tc.params) + string(tc.final)
		t.Run(name, func(t *testing.T) {
			got, ok := csiKey([]byte(tc.params), tc.final)
			if ok != tc.ok || got != tc.want {
				t.Fatalf("csiKey(%q, %q) = (%v, %v), want (%v, %v)", tc.params, tc.final, got, ok, tc.want, tc.ok)
			}
		})
	}
}
