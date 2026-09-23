// colors.go ports setup_colors() from devops-deployment.sh: the C_* ANSI
// variables the TUI paints with, kept byte-for-byte, and the same
// `[[ -t 1 && -z "${NO_COLOR:-}" ]]` gate that decides whether they are
// set at all. With colors disabled every field is the empty string, so the
// printf formats of tui_draw() degrade to plain text exactly the way the
// bash ones do.

package menu

import (
	"io"
	"os"

	"devops-deployment-go/internal/term"
)

// Colors holds the escape sequences behind the bash script's C_* globals.
// The zero value is the all-empty "colors disabled" palette.
type Colors struct {
	Reset    string
	Bold     string
	Dim      string
	Magenta  string
	BMagenta string
	Green    string
	Blue     string
	Yellow   string
	Cyan     string
	Gray     string
	White    string
	BGreen   string
	BRed     string
	BYellow  string
}

// NewColors returns the palette with the exact sequences setup_colors()
// assigns, or the all-empty set when colors are off. enabled mirrors the
// bash gate `[[ -t 1 && -z "${NO_COLOR:-}" ]]`; use ColorsEnabled to
// compute it for a writer.
func NewColors(enabled bool) Colors {
	if !enabled {
		return Colors{}
	}
	return Colors{
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
}

// ColorsEnabled mirrors the setup_colors() gate for the writer the menu
// draws to: NO_COLOR must be unset or empty — an empty value counts as
// unset, like bash's -z "${NO_COLOR:-}" — and w must be a terminal. As
// with bash's [[ -t 1 ]], only a real file can be a terminal, so any other
// writer (a bytes.Buffer in the tests, say) disables colors the way a
// redirected stdout does in bash.
func ColorsEnabled(w io.Writer) bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	return term.IsTTY(f.Fd())
}
