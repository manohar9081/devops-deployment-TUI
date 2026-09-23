// Tests for the tui_draw() port in draw.go, run with colors disabled
// (NewColors(false)) so every line is plain text and can be compared
// string-for-string: line count with and without the logo, the exact item
// rows and their %2d numbers, the ▶ marker on the selected row only, the
// padding of the label field to LabelW, the ━ separator width and the
// status/hints/footer strings. Nothing here touches a terminal: Draw
// writes into a bytes.Buffer, and no test runs the menu interactively.

package menu

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"
	"unicode/utf8"
)

// drawLines runs Draw with colors disabled and returns the content of each
// line written, with the "\r\x1b[2K" erase prefix stripped. It also checks
// that Draw's returned height matches the number of lines actually
// written, since the caller relies on it for erasing the block.
func drawLines(t *testing.T, sel, cols int, showLogo bool, osName, pkgMgr string) []string {
	t.Helper()
	var buf bytes.Buffer
	n := Draw(&buf, sel, cols, showLogo, osName, pkgMgr, NewColors(false))
	all := strings.Split(buf.String(), "\n")
	// The per-line format ends with "\n", so the split leaves one empty
	// trailing element.
	if all[len(all)-1] != "" {
		t.Fatalf("Draw output does not end with \\n: %q", buf.String())
	}
	lines := all[:len(all)-1]
	if len(lines) != n {
		t.Fatalf("Draw returned %d but wrote %d lines", n, len(lines))
	}
	for i, line := range lines {
		if !strings.HasPrefix(line, "\r\x1b[2K") {
			t.Fatalf("line %d missing \\r\\x1b[2K prefix: %q", i, line)
		}
		lines[i] = strings.TrimPrefix(line, "\r\x1b[2K")
	}
	return lines
}

// rowsNoLogo is the index of the first item row when the logo is hidden:
// the ⬢ header line, a blank, the status line and a blank come first. With
// the logo shown the 7-line banner+title block replaces that single header
// line, shifting the rows down by len(logoLines).
const rowsNoLogo = 4

// itemRows slices the 14 item rows out of a drawn block.
func itemRows(t *testing.T, showLogo bool, lines []string) []string {
	t.Helper()
	start := rowsNoLogo
	if showLogo {
		start = rowsNoLogo + len(logoLines)
	}
	if len(lines) < start+len(Items) {
		t.Fatalf("only %d lines, cannot hold %d item rows from %d", len(lines), len(Items), start)
	}
	return lines[start : start+len(Items)]
}

func TestDrawLineCount(t *testing.T) {
	for _, tc := range []struct {
		showLogo bool
		want     int
	}{
		// Logo block (6 banner + title), blank, status, blank, 14 item
		// rows, blank, separator, hints, footer.
		{true, len(logoLines) + 22},
		// One ⬢ header line instead of the logo block.
		{false, 22},
	} {
		lines := drawLines(t, 2, 78, tc.showLogo, "linux", "apt")
		if len(lines) != tc.want {
			t.Errorf("showLogo=%v: got %d lines, want %d", tc.showLogo, len(lines), tc.want)
		}
	}
}

func TestDrawItemRows(t *testing.T) {
	const sel = 2
	for _, showLogo := range []bool{false, true} {
		rows := itemRows(t, showLogo, drawLines(t, sel, 78, showLogo, "linux", "apt"))
		for i, item := range Items {
			marker := " "
			if i == sel {
				marker = "▶"
			}
			// marker, space, %2d number, two spaces, label.
			want := marker + " " + fmt.Sprintf("%2d", i+1) + "  " + item.Label
			if !strings.HasPrefix(rows[i], want) {
				t.Errorf("showLogo=%v row %d = %q, want prefix %q", showLogo, i, rows[i], want)
			}
		}
	}
}

func TestDrawLabelPadding(t *testing.T) {
	// sel = -1 selects nothing, so every row uses the plain " " marker
	// and the row layout is uniform.
	rows := itemRows(t, false, drawLines(t, -1, 78, false, "linux", "apt"))
	for i, item := range Items {
		prefix := " " + " " + fmt.Sprintf("%2d", i+1) + "  " + item.Label
		rest := strings.TrimPrefix(rows[i], prefix)
		pad := LabelW - utf8.RuneCountInString(item.Label)
		if pad < 1 {
			pad = 1
		}
		if !strings.HasPrefix(rest, strings.Repeat(" ", pad)) {
			t.Errorf("row %d: want %d spaces after %q, got %q", i, pad, item.Label, rest)
			continue
		}
		if got := rest[pad:]; got != item.Desc {
			t.Errorf("row %d: after padding got %q, want desc %q", i, got, item.Desc)
		}
	}
}

func TestDrawSeparator(t *testing.T) {
	for _, cols := range []int{78, 60, 40} {
		lines := drawLines(t, 0, cols, false, "linux", "apt")
		// Layout tail: blank, separator, hints, footer.
		sep := lines[len(lines)-3]
		if n := utf8.RuneCountInString(sep); n != cols {
			t.Errorf("cols=%d: separator is %d runes, want %d", cols, n, cols)
		}
		if stray := strings.Trim(sep, "━"); stray != "" {
			t.Errorf("cols=%d: separator has non-━ characters: %q", cols, stray)
		}
	}
}

func TestDrawHeaderStatusFooter(t *testing.T) {
	tall := drawLines(t, 0, 78, true, "linux", "apt")
	if got, want := tall[6], "D E P L O Y M E N T"; got != want {
		t.Errorf("logo title = %q, want %q", got, want)
	}

	lines := drawLines(t, 0, 78, false, "linux", "apt")
	if got, want := lines[0], "⬢ devops-deployment"; got != want {
		t.Errorf("compact header = %q, want %q", got, want)
	}
	if got, want := lines[2], "✓ prerequisites ready   os: linux · pkg: apt"; got != want {
		t.Errorf("status = %q, want %q", got, want)
	}
	// An empty PKG_MANAGER renders as "pkg: none", the bash default.
	none := drawLines(t, 0, 78, false, "darwin", "")
	if got, want := none[2], "✓ prerequisites ready   os: darwin · pkg: none"; got != want {
		t.Errorf("status = %q, want %q", got, want)
	}
	const hintsLine = "↑↓ navigate   ⏎ run   1-9 select   c config   u update   q quit"
	if got := lines[len(lines)-2]; got != hintsLine {
		t.Errorf("hints = %q, want %q", got, hintsLine)
	}
	if got, want := lines[len(lines)-1], "devops-deployment · interactive menu"; got != want {
		t.Errorf("footer = %q, want %q", got, want)
	}
}

func TestNewColors(t *testing.T) {
	// Disabled: every field empty, so Draw's formats degrade to text.
	if got := NewColors(false); got != (Colors{}) {
		t.Errorf("NewColors(false) = %+v, want the all-empty palette", got)
	}
	// Enabled: the exact sequences setup_colors() assigns.
	want := Colors{
		Reset:    "\x1b[0m",
		Bold:     "\x1b[1m",
		Dim:      "\x1b[2m",
		Magenta:  "\x1b[35m",
		BMagenta: "\x1b[1;35m",
		Green:    "\x1b[32m",
		Blue:     "\x1b[34m",
		Yellow:   "\x1b[33m",
		Cyan:     "\x1b[36m",
		Gray:     "\x1b[90m",
		White:    "\x1b[97m",
		BGreen:   "\x1b[1;32m",
		BRed:     "\x1b[1;31m",
		BYellow:  "\x1b[1;33m",
	}
	if got := NewColors(true); got != want {
		t.Errorf("NewColors(true) = %+v, want %+v", got, want)
	}
}

func TestColorsEnabled(t *testing.T) {
	// Not an *os.File: the bash analogue is a redirected stdout.
	if ColorsEnabled(&bytes.Buffer{}) {
		t.Error("bytes.Buffer: colors reported enabled")
	}
	// A pipe is an *os.File but never a terminal.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	if ColorsEnabled(w) {
		t.Error("pipe: colors reported enabled")
	}
	// NO_COLOR kills colors even for a terminal-like file, matching the
	// -z "${NO_COLOR:-}" half of setup_colors' gate.
	t.Setenv("NO_COLOR", "1")
	if ColorsEnabled(os.Stdout) {
		t.Error("NO_COLOR=1: colors reported enabled")
	}
}
