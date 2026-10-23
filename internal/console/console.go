// Package console decides, once per output stream, how ggboard writes to it, and renders the CLI's
// human-oriented output (command results, the serve startup summary, the Xboard import report) with lipgloss.
//
// Every stream gets one of three styles, chosen at startup by Detect:
//
//   - plain:  not a terminal (systemd, docker, a pipe or a file). Script-friendly lines without ANSI escapes;
//     text logs stay in slog's logfmt (key=value) format.
//   - pretty: a terminal with colours disabled (NO_COLOR, TERM=dumb or --color=never). Aligned layout and
//     symbols, no ANSI escapes.
//   - color:  a terminal, or --color=always. Pretty plus ANSI colours.
//
// Only the 16 basic ANSI colours are used, the same ones as the log handler (github.com/lmittmann/tint), so the
// output follows the terminal's own theme in light and dark mode.
package console

import (
	"io"
	"os"
	"strings"
	"unicode/utf8"

	"golang.org/x/term"
)

// Values of --color / log.color / GGBOARD_LOG_COLOR. An empty value means ColorAuto.
const (
	ColorAuto   = "auto"   // colours on a terminal unless NO_COLOR is set or TERM=dumb
	ColorAlways = "always" // colours and the pretty layout even when writing to a pipe or a file (overrides NO_COLOR)
	ColorNever  = "never"  // never colours; a terminal still gets the pretty layout
)

// EnvNoColor disables colours in ColorAuto when set to any non-empty value (https://no-color.org).
const EnvNoColor = "NO_COLOR"

// ValidColor reports whether mode is a known --color value ("" counts as auto).
func ValidColor(mode string) bool {
	switch normalize(mode) {
	case ColorAuto, ColorAlways, ColorNever:
		return true
	}
	return false
}

func normalize(mode string) string {
	if m := strings.ToLower(strings.TrimSpace(mode)); m != "" {
		return m
	}
	return ColorAuto
}

// Style is how one output stream is rendered.
type Style struct {
	// Pretty selects the human-oriented layout (aligned blocks, symbols, tables). False means plain,
	// script-friendly lines.
	Pretty bool
	// Color adds ANSI colours. It is only ever set together with Pretty.
	Color bool
}

// terminal is replaced in tests.
var terminal = func(w io.Writer) bool {
	f, ok := w.(interface{ Fd() uintptr })
	return ok && term.IsTerminal(int(f.Fd()))
}

// IsTerminal reports whether w is a terminal (an *os.File connected to a TTY).
func IsTerminal(w io.Writer) bool { return terminal(w) }

// Detect returns the style for output written to w under mode (see the Color constants). It costs a system call
// and two environment lookups: call it once per stream when the process starts, never per write.
func Detect(mode string, w io.Writer) Style {
	switch normalize(mode) {
	case ColorAlways:
		return Style{Pretty: true, Color: true}
	case ColorNever:
		return Style{Pretty: IsTerminal(w)}
	}
	if !IsTerminal(w) {
		return Style{}
	}
	return Style{Pretty: true, Color: os.Getenv(EnvNoColor) == "" && os.Getenv("TERM") != "dumb"}
}

// Clean makes untrusted text safe to print on a terminal: C0 control characters other than tab and newline, DEL,
// the C1 controls and invalid UTF-8 become U+FFFD, so data from requests or an imported database cannot move the
// cursor, retitle the window or write the clipboard through escape sequences.
func Clean(s string) string {
	if !needsClean(s) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if isControl(r) || r == utf8.RuneError {
			b.WriteRune(utf8.RuneError)
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func needsClean(s string) bool {
	for i := 0; i < len(s); i++ {
		if c := s[i]; c < 0x20 && c != '\t' && c != '\n' || c >= 0x7f {
			// Bytes >= 0x80 start multi-byte runes; only C1 controls and invalid sequences need work.
			if c < 0x80 {
				return true
			}
			return !utf8.ValidString(s[i:]) || strings.ContainsFunc(s[i:], isControl)
		}
	}
	return false
}

func isControl(r rune) bool {
	return r < 0x20 && r != '\t' && r != '\n' || r >= 0x7f && r < 0xa0
}
