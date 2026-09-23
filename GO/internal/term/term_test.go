package term

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// notTTY returns the fd of an ordinary file, which must fail the
// TIOCGWINSZ ioctl on every platform the port targets.
func notTTY(t *testing.T) uintptr {
	t.Helper()
	f, err := os.Create(filepath.Join(t.TempDir(), "plain-file"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.Close() })
	return f.Fd()
}

func TestIsTTYFalseForRegularFile(t *testing.T) {
	if IsTTY(notTTY(t)) {
		t.Fatal("IsTTY reported a regular file as a terminal")
	}
}

func TestSizeFallsBackTo80x24(t *testing.T) {
	cols, lines, err := Size(notTTY(t))
	if err == nil {
		t.Fatal("Size on a regular file should return the ioctl error")
	}
	if cols != 80 || lines != 24 {
		t.Fatalf("Size fallback = %dx%d, want 80x24", cols, lines)
	}
}

func TestSequences(t *testing.T) {
	cases := []struct {
		name string
		fn   func(io.Writer)
		want string
	}{
		{"HideCursor", HideCursor, "\x1b[?25l"},
		{"ShowCursor", ShowCursor, "\x1b[?25h"},
		{"ClearScreen", ClearScreen, "\x1b[H\x1b[2J\x1b[H"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			tc.fn(&buf)
			if got := buf.String(); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestEraseBlock(t *testing.T) {
	var buf bytes.Buffer
	eraseBlock(&buf, 3)
	want := "\x1b[3A" + "\r\x1b[2K\n" + "\r\x1b[2K\n" + "\r\x1b[2K\n" + "\x1b[3A"
	if got := buf.String(); got != want {
		t.Fatalf("eraseBlock(3) = %q, want %q", got, want)
	}
}
