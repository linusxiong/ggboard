package console

import (
	"fmt"
	"io"
	"strings"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"
	"github.com/charmbracelet/colorprofile"
)

// Symbols that start pretty result lines (kept without colours; plain output has none).
const (
	SymbolSuccess = "✓"
	SymbolWarning = "!"
	SymbolFailure = "✗"
	SymbolArrow   = "➜"
)

// Semantic styles. Basic ANSI colours only, matching the tint log handler (bright green / yellow / red levels).
var (
	styleBold    = lipgloss.NewStyle().Bold(true)
	styleFaint   = lipgloss.NewStyle().Faint(true)
	styleAccent  = lipgloss.NewStyle().Foreground(lipgloss.Cyan)
	styleGood    = lipgloss.NewStyle().Foreground(lipgloss.BrightGreen)
	styleCaution = lipgloss.NewStyle().Foreground(lipgloss.BrightYellow)
	styleBad     = lipgloss.NewStyle().Foreground(lipgloss.BrightRed)
)

// Printer writes human-oriented output to one stream in its Style.
//
// The text helpers (Bold, Faint, Accent, …, Text) take untrusted text and Clean it, so build lines by
// concatenating constants with helper results and never nest helpers. The line methods (Success, Warning, …)
// do not clean again: they expect such composed text.
type Printer struct {
	w      io.Writer
	style  Style
	indent string // prefix of the line methods (Indented)
}

// New returns a printer for w. Pretty output goes through a colorprofile writer: the ANSI profile keeps the 16
// basic colours and the NoTTY profile strips every escape sequence (pretty without colours). Plain output is
// written as is.
func New(w io.Writer, s Style) *Printer {
	if s.Pretty {
		profile := colorprofile.NoTTY
		if s.Color {
			profile = colorprofile.ANSI
		}
		w = &colorprofile.Writer{Forward: w, Profile: profile}
	}
	return &Printer{w: w, style: s}
}

// Indented returns a printer whose line methods (Success, Warning, Failure) start with n more spaces.
func (p *Printer) Indented(n int) *Printer {
	q := *p
	q.indent += strings.Repeat(" ", n)
	return &q
}

// Style returns the printer's style.
func (p *Printer) Style() Style { return p.style }

// Pretty reports whether the printer uses the human-oriented layout.
func (p *Printer) Pretty() bool { return p.style.Pretty }

// Writer returns the destination (wrapped by the colour filter in the pretty styles).
func (p *Printer) Writer() io.Writer { return p.w }

// Println writes the operands followed by a newline.
func (p *Printer) Println(a ...any) { _, _ = fmt.Fprintln(p.w, a...) }

// Printf writes a formatted string.
func (p *Printer) Printf(format string, a ...any) { _, _ = fmt.Fprintf(p.w, format, a...) }

func (p *Printer) render(st lipgloss.Style, s string) string {
	s = Clean(s)
	if !p.style.Pretty || s == "" {
		return s
	}
	return st.Render(s)
}

// Text returns s cleaned and unstyled.
func (p *Printer) Text(s string) string { return Clean(s) }

// Bold renders s in bold (headings, the values that matter).
func (p *Printer) Bold(s string) string { return p.render(styleBold, s) }

// Faint renders s dimmed (labels, secondary details).
func (p *Printer) Faint(s string) string { return p.render(styleFaint, s) }

// Accent renders s in the accent colour (versions, URLs, commands to run).
func (p *Printer) Accent(s string) string { return p.render(styleAccent, s) }

// Good renders s in green.
func (p *Printer) Good(s string) string { return p.render(styleGood, s) }

// Caution renders s in yellow.
func (p *Printer) Caution(s string) string { return p.render(styleCaution, s) }

// Bad renders s in red.
func (p *Printer) Bad(s string) string { return p.render(styleBad, s) }

func (p *Printer) line(symbol string, st lipgloss.Style, msg string) {
	if p.style.Pretty {
		msg = st.Render(symbol) + " " + msg
	}
	p.Println(p.indent + msg)
}

// Success prints a completed action: "✓ msg" when pretty, msg alone when plain.
func (p *Printer) Success(msg string) { p.line(SymbolSuccess, styleGood, msg) }

// Warning prints something the operator should act on: "! msg" when pretty, msg alone when plain.
func (p *Printer) Warning(msg string) { p.line(SymbolWarning, styleCaution.Bold(true), msg) }

// Failure prints an error: "✗ msg" when pretty, msg alone when plain. Continuation lines of a multi-line msg are
// indented under the first one.
func (p *Printer) Failure(msg string) {
	if p.style.Pretty {
		msg = strings.ReplaceAll(msg, "\n", "\n  "+p.indent)
	}
	p.line(SymbolFailure, styleBad.Bold(true), msg)
}

// Field is one line of a key/value block. Value is composed text (see Printer); Mark, when set, is a symbol
// printed before the key in pretty output (for example SymbolArrow on the lines worth clicking).
type Field struct {
	Key   string
	Value string
	Mark  string
}

// Fields prints an aligned key/value block indented by indent spaces. Pretty keys are faint and marked lines get
// their symbol in the accent colour; plain lines are "key  value".
func (p *Printer) Fields(indent int, fields []Field) {
	width, marks := 0, false
	for _, f := range fields {
		width = max(width, lipgloss.Width(f.Key))
		marks = marks || f.Mark != ""
	}
	pad := strings.Repeat(" ", indent)
	for _, f := range fields {
		key := f.Key + strings.Repeat(" ", width-lipgloss.Width(f.Key))
		if !p.style.Pretty {
			p.Println(strings.TrimRight(pad+key+"  "+f.Value, " "))
			continue
		}
		mark := ""
		if marks {
			mark = "   "
			if f.Mark != "" {
				mark = styleAccent.Render(f.Mark) + "  "
			}
		}
		p.Println(strings.TrimRight(pad+mark+styleFaint.Render(key)+"  "+f.Value, " "))
	}
}

// TableOptions tunes Table.
type TableOptions struct {
	Indent int   // spaces before every line
	Right  []int // right-aligned columns (numbers)
	// Notes, when set, holds one free-text note per row, printed after the row outside the columns, so a long note
	// neither widens the table nor its header rule.
	Notes []string
}

// Table prints rows under headers as a borderless table with a rule below the header. Cells and notes are composed
// text (see Printer). Plain output uses an ASCII rule and no styles.
func (p *Printer) Table(headers []string, rows [][]string, o TableOptions) {
	alignRight := make(map[int]bool, len(o.Right))
	for _, c := range o.Right {
		alignRight[c] = true
	}
	border, rule, header := lipgloss.NormalBorder(), styleFaint, styleFaint
	if !p.style.Pretty {
		border, rule, header = lipgloss.ASCIIBorder(), lipgloss.NewStyle(), lipgloss.NewStyle()
	}
	t := table.New().
		Border(border).
		BorderTop(false).BorderBottom(false).BorderLeft(false).BorderRight(false).BorderColumn(false).
		BorderHeader(true).
		BorderStyle(rule).
		Wrap(false).
		Headers(headers...).
		Rows(rows...).
		StyleFunc(func(row, col int) lipgloss.Style {
			st := lipgloss.NewStyle()
			if row == table.HeaderRow {
				st = header
			}
			if col > 0 {
				st = st.PaddingLeft(2)
			}
			if alignRight[col] {
				st = st.Align(lipgloss.Right)
			}
			return st
		})
	pad := strings.Repeat(" ", o.Indent)
	const bodyStart = 2 // header line, rule line
	i := 0
	for line := range strings.SplitSeq(t.Render(), "\n") {
		if row := i - bodyStart; row >= 0 && row < len(o.Notes) && o.Notes[row] != "" {
			line += "  " + o.Notes[row]
		}
		p.Println(strings.TrimRight(pad+line, " "))
		i++
	}
}
