// draw.go is the Go port of the TUI's drawing half in devops-deployment.sh:
// tui_geometry()'s size probing (the tput 80x24 fallback and the 78-column
// cap) and tui_draw()'s menu block — the ASCII-art logo or the compact ⬢
// header, the status line, the 14 group-colored item rows, the ━
// separator, the key hints and the footer. Like the bash version Draw
// writes every line as "\r\x1b[2K" + content + "\n" and reports the block
// height so the caller can erase and redraw in place (term.EraseBlock).

package menu

import (
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"

	"devops-deployment-go/internal/term"
)

const (
	// fallbackCols/fallbackLines are tui_geometry()'s `tput cols || echo
	// 80` and `tput lines || echo 24` defaults, the same pair term.Size
	// returns when its ioctl fails.
	fallbackCols  = 80
	fallbackLines = 24
	// maxCols is tui_geometry()'s `if (( TUI_COLS > 78 ))` cap.
	maxCols = 78
	// logoMinLines is the height below which the ASCII-art logo is
	// hidden: tui_geometry()'s `if (( TUI_LINES < 27 ))`.
	logoMinLines = 27
	// LabelW is the script's LABEL_W: the width each item label field is
	// padded to so the descriptions line up; labels longer than this keep
	// a single separating space.
	LabelW = 22
	// erasePrefix is what the bash draw loop and Draw both put ahead of
	// every line: carriage return plus \e[2K, wiping the line before it
	// is (re)drawn.
	erasePrefix = "\r\x1b[2K"
)

// logoLines are the six banner lines tui_draw() paints above the
// "D E P L O Y M E N T" title, copied byte-for-byte from its OUT+= lines,
// box-drawing characters and edge spaces included.
var logoLines = []string{
	" ██████╗ ███████╗██████╗ ██████╗  ██████╗ ██╗  ██╗",
	" ██╔══██╗██╔════╝██╔══██╗██╔══██╗██╔═══██╗╚██╗██╔╝",
	" ██║  ██║███████╗██████╔╝██║  ██║██║   ██║ ╚███╔╝ ",
	" ██║  ██║╚════██║██╔══██╗██║  ██║██║   ██║ ██╔██╗ ",
	" ██████╔╝███████║██║  ██║██████╔╝╚██████╔╝██╔╝ ██╗",
	" ╚═════╝ ╚══════╝╚═╝  ╚═╝╚═════╝ ╚═════╝ ╚═╝  ╚═╝",
}

// Geometry ports tui_geometry(): it reports the terminal size for w —
// falling back to 80x24 when w is not a file or the ioctl fails, like the
// script's tput defaults — caps the column count at 78 so the layout
// survives very wide terminals, and says whether the terminal is tall
// enough (27+ lines) for the full ASCII-art logo.
func Geometry(w io.Writer) (cols, lines int, showLogo bool) {
	cols, lines = fallbackCols, fallbackLines
	if f, ok := w.(*os.File); ok {
		if c, l, err := term.Size(f.Fd()); err == nil {
			cols, lines = c, l
		}
	}
	if cols > maxCols {
		cols = maxCols
	}
	return cols, lines, lines >= logoMinLines
}

// Draw ports tui_draw(): it paints the whole menu block to w and returns
// its height — the bash BLOCK_HEIGHT — so the caller can erase it and draw
// the next frame. sel is the 0-based index of the selected entry (out of
// range selects nothing, like an unmatched `(( i == sel ))`); cols is the
// already capped separator width from Geometry; showLogo picks the tall
// ASCII-art header over the one-line ⬢ header; osName and pkgMgr fill the
// status line, an empty pkgMgr rendering as "pkg: none". Every line is
// written as erasePrefix + content + "\n", matching the bash printf loop.
func Draw(w io.Writer, sel int, cols int, showLogo bool, osName, pkgMgr string, c Colors) int {
	var out []string

	if showLogo {
		for _, line := range logoLines {
			out = append(out, c.BMagenta+line+c.Reset)
		}
		out = append(out, c.Magenta+"D E P L O Y M E N T"+c.Reset)
	} else {
		out = append(out, c.BMagenta+"⬢ devops-deployment"+c.Reset)
	}
	out = append(out, "")

	// Status line: prerequisites + os + package manager, with the bash
	// `pkg_part="pkg: none"` default for an unset PKG_MANAGER.
	pkgPart := "pkg: none"
	if pkgMgr != "" {
		pkgPart = "pkg: " + pkgMgr
	}
	out = append(out, c.BGreen+"✓ prerequisites ready"+c.Reset+"   "+
		c.Dim+"os: "+osName+" · "+pkgPart+c.Reset)
	out = append(out, "")

	for i, item := range Items {
		numstr := fmt.Sprintf("%2d", i+1)
		var marker, label, desc string
		if i == sel {
			marker = c.BMagenta + "▶" + c.Reset
			label = c.Bold + item.Label + c.Reset
			// NB: the bash selected desc deliberately omits the trailing
			// reset — the next line's \e[2K wipes the color anyway.
			desc = c.Gray + item.Desc
			numstr = c.BMagenta + numstr + c.Reset
		} else {
			marker = " "
			label = groupColor(c, item.Group) + item.Label + c.Reset
			desc = c.Dim + item.Desc + c.Reset
			numstr = c.Dim + numstr + c.Reset
		}
		pad := LabelW - utf8.RuneCountInString(item.Label)
		if pad < 1 {
			pad = 1
		}
		out = append(out, fmt.Sprintf("%s %s  %s%*s%s", marker, numstr, label, pad, "", desc))
	}

	out = append(out, "")
	out = append(out, c.Magenta+strings.Repeat("━", cols)+c.Reset)
	out = append(out, hints(c))
	out = append(out, c.Dim+"devops-deployment · interactive menu"+c.Reset)

	for _, line := range out {
		fmt.Fprint(w, erasePrefix+line+"\n")
	}
	return len(out)
}

// groupColor ports group_color(): the color a group's item labels are
// painted with when they are not selected. Unknown groups fall through to
// the empty string, like the bash case with no matching pattern.
func groupColor(c Colors, group string) string {
	switch group {
	case "k8s":
		return c.Green
	case "cloud":
		return c.Blue
	case "app":
		return c.Yellow
	case "setup":
		return c.Magenta
	case "bulk":
		return c.Cyan
	default:
		return ""
	}
}

// hints builds the key-hints line, byte-for-byte the single bash printf
// with the key glyphs painted C_BMAGENTA and the words left plain.
func hints(c Colors) string {
	return c.BMagenta + "↑↓" + c.Reset + " navigate   " +
		c.BMagenta + "⏎" + c.Reset + " run   " +
		c.BMagenta + "1-9" + c.Reset + " select   " +
		c.BMagenta + "c" + c.Reset + " config   " +
		c.BMagenta + "u" + c.Reset + " update   " +
		c.BMagenta + "q" + c.Reset + " quit"
}
