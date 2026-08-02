// Package ui provides the presentation-layer bits shared by internal/cli:
// TTY-aware color styling and the interactive fuzzy-finder picker.
package ui

import (
	"io"
	"os"

	"golang.org/x/term"
)

const (
	ansiReset      = "\x1b[0m"
	ansiBold       = "\x1b[1m"
	ansiBoldYellow = "\x1b[1;33m"
)

// Style controls how the active context name is decorated in list output.
type Style struct {
	bold  bool
	color bool // yellow, implies bold
}

// Active decorates name as the active context according to s: yellow+bold,
// bold only, or unchanged, depending on what s allows.
func (s Style) Active(name string) string {
	switch {
	case s.color:
		return ansiBoldYellow + name + ansiReset
	case s.bold:
		return ansiBold + name + ansiReset
	default:
		return name
	}
}

// DetectStyle decides the color style for output written to w.
//
//   - FORCE_COLOR (any non-empty value) always enables full color, regardless
//     of whether w is a terminal.
//   - Otherwise, color requires w to be a terminal.
//   - NO_COLOR (any non-empty value, per no-color.org) then suppresses color
//     but keeps bold — it says "no color", not "no emphasis".
func DetectStyle(w io.Writer) Style {
	if os.Getenv("FORCE_COLOR") != "" {
		return Style{bold: true, color: true}
	}
	if !IsTerminal(w) {
		return Style{}
	}
	if os.Getenv("NO_COLOR") != "" {
		return Style{bold: true}
	}
	return Style{bold: true, color: true}
}

// IsTerminal reports whether w refers to a terminal. Writers that aren't
// *os.File (e.g. buffers used by tests) are never terminals.
func IsTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(f.Fd()))
}

// IsTerminalReader reports whether r refers to a terminal. Readers that
// aren't *os.File (e.g. buffers used by tests) are never terminals.
func IsTerminalReader(r io.Reader) bool {
	f, ok := r.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(f.Fd()))
}
