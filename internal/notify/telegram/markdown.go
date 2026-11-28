package telegram

import (
	"html"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	east "github.com/yuin/goldmark/extension/ast"
	gmtext "github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
	xhtml "golang.org/x/net/html"

	"github.com/linusxiong/ggboard/internal/notify"
)

// Long messages (broadcasts). The Bot API's HTML mode knows only b, i, u, s, a, code, pre and blockquote (which
// cannot be nested), so an administrator's Markdown is converted block by block: headings become bold lines, list
// items get "• " or "1. ", tables become "a | b" lines, images become links and raw HTML keeps only its text. Single
// line breaks are kept, as in the e-mail (internal/mail/markdown.go). Plain text keeps its paragraphs and lines.
// The bold title and the blocks are packed in order into messages of at most maxMessage characters; a block that
// alone is longer is sent as plain text in pieces.

var tgMarkdown = goldmark.New(goldmark.WithExtensions(extension.GFM))

// RenderLong renders ev (a broadcast) as one or more Telegram HTML messages.
func RenderLong(ev notify.Event) []string {
	var blocks []string
	if ev.Markdown {
		blocks = markdownBlocks(ev.Body)
	} else {
		blocks = plainBlocks(ev.Body)
	}
	var (
		msgs []string
		cur  string
	)
	if title := strings.TrimSpace(ev.Title); title != "" {
		cur = "<b>" + escTruncate(title, maxTitle) + "</b>"
	}
	flush := func() {
		if strings.TrimSpace(cur) != "" {
			msgs = append(msgs, cur)
		}
		cur = ""
	}
	for _, b := range blocks {
		sep := "\n\n"
		if cur == "" {
			sep = ""
		}
		if runes(cur)+runes(sep)+runes(b) <= maxMessage {
			cur += sep + b
			continue
		}
		flush()
		if runes(b) <= maxMessage {
			cur = b
			continue
		}
		msgs = append(msgs, splitEscaped(htmlText(b), maxMessage)...)
	}
	flush()
	return msgs
}

func runes(s string) int { return utf8.RuneCountInString(s) }

// plainBlocks splits plain text into escaped paragraphs (blank lines separate them; single line breaks stay).
func plainBlocks(s string) []string {
	s = strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n")
	var out []string
	for _, p := range strings.Split(s, "\n\n") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, esc(p))
		}
	}
	return out
}

// markdownBlocks converts Markdown into Telegram HTML, one balanced string per top-level block.
func markdownBlocks(src string) []string {
	source := []byte(strings.ReplaceAll(src, "\r\n", "\n"))
	doc := tgMarkdown.Parser().Parse(gmtext.NewReader(source))
	r := tgRenderer{src: source}
	var out []string
	for c := doc.FirstChild(); c != nil; c = c.NextSibling() {
		if b := strings.Trim(r.block(c, ""), "\n "); b != "" {
			out = append(out, b)
		}
	}
	return out
}

type tgRenderer struct {
	src   []byte
	quote bool // inside a blockquote: Telegram cannot nest one, and code blocks stay plain
}

func (r *tgRenderer) children(n ast.Node, indent string) []string {
	var parts []string
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if s := strings.Trim(r.block(c, indent), "\n"); s != "" {
			parts = append(parts, s)
		}
	}
	return parts
}

func (r *tgRenderer) block(n ast.Node, indent string) string {
	switch n := n.(type) {
	case *ast.Heading:
		return "<b>" + r.inlines(n) + "</b>"
	case *ast.Paragraph, *ast.TextBlock:
		return r.inlines(n)
	case *ast.ThematicBreak:
		return "——————"
	case *ast.FencedCodeBlock, *ast.CodeBlock:
		var b strings.Builder
		lines := n.Lines()
		for i := range lines.Len() {
			seg := lines.At(i)
			b.Write(seg.Value(r.src))
		}
		code := esc(strings.TrimRight(b.String(), "\n"))
		if r.quote {
			return code
		}
		return "<pre>" + code + "</pre>"
	case *ast.Blockquote:
		if r.quote {
			return strings.Join(r.children(n, indent), "\n")
		}
		r.quote = true
		parts := r.children(n, indent)
		r.quote = false
		return "<blockquote>" + strings.Join(parts, "\n") + "</blockquote>"
	case *ast.List:
		num := n.Start
		if num == 0 {
			num = 1
		}
		var lines []string
		for item := n.FirstChild(); item != nil; item = item.NextSibling() {
			marker := "• "
			if n.IsOrdered() {
				marker = strconv.Itoa(num) + ". "
				num++
			}
			lines = append(lines, indent+marker+strings.Join(r.children(item, indent+"   "), "\n"))
		}
		return strings.Join(lines, "\n")
	case *east.Table:
		var lines []string
		for row := n.FirstChild(); row != nil; row = row.NextSibling() {
			var cells []string
			for cell := row.FirstChild(); cell != nil; cell = cell.NextSibling() {
				cells = append(cells, strings.TrimSpace(r.inlines(cell)))
			}
			line := strings.Join(cells, " | ")
			if _, header := row.(*east.TableHeader); header {
				line = "<b>" + line + "</b>"
			}
			lines = append(lines, line)
		}
		return strings.Join(lines, "\n")
	case *ast.HTMLBlock:
		var b strings.Builder
		lines := n.Lines()
		for i := range lines.Len() {
			seg := lines.At(i)
			b.Write(seg.Value(r.src))
		}
		if n.HasClosure() {
			b.Write(n.ClosureLine.Value(r.src))
		}
		return esc(htmlText(b.String()))
	}
	return strings.Join(r.children(n, indent), "\n")
}

func (r *tgRenderer) inlines(n ast.Node) string {
	var b strings.Builder
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		r.inline(c, &b)
	}
	return b.String()
}

func (r *tgRenderer) inline(n ast.Node, b *strings.Builder) {
	switch n := n.(type) {
	case *ast.Text:
		b.WriteString(esc(string(unescape(n.Segment.Value(r.src)))))
		if n.SoftLineBreak() || n.HardLineBreak() {
			b.WriteString("\n")
		}
	case *ast.String:
		b.WriteString(esc(string(n.Value)))
	case *ast.CodeSpan:
		var code strings.Builder
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			if t, ok := c.(*ast.Text); ok {
				code.Write(t.Segment.Value(r.src))
			}
		}
		b.WriteString("<code>" + esc(code.String()) + "</code>")
	case *ast.Emphasis:
		tag := "i"
		if n.Level >= 2 {
			tag = "b"
		}
		b.WriteString("<" + tag + ">" + r.inlines(n) + "</" + tag + ">")
	case *east.Strikethrough:
		b.WriteString("<s>" + r.inlines(n) + "</s>")
	case *ast.Link:
		writeLink(b, string(unescape(n.Destination)), r.inlines(n))
	case *ast.AutoLink:
		url, label := string(n.URL(r.src)), esc(string(n.Label(r.src)))
		if n.AutoLinkType == ast.AutoLinkEmail && !strings.HasPrefix(strings.ToLower(url), "mailto:") {
			url = "mailto:" + url
		}
		writeLink(b, url, label)
	case *ast.Image:
		label := r.inlines(n)
		if strings.TrimSpace(label) == "" {
			label = esc(string(unescape(n.Destination)))
		}
		writeLink(b, string(unescape(n.Destination)), label)
	case *ast.RawHTML:
		// An inline tag is dropped; the text between tags arrives as its own nodes.
	case *east.TaskCheckBox:
		if n.IsChecked {
			b.WriteString("☑ ")
		} else {
			b.WriteString("☐ ")
		}
	default:
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			r.inline(c, b)
		}
	}
}

// writeLink writes an <a> for http(s), tg and mailto addresses; any other address keeps only its label.
func writeLink(b *strings.Builder, dest, label string) {
	lower := strings.ToLower(strings.TrimSpace(dest))
	for _, scheme := range []string{"https://", "http://", "tg://", "mailto:"} {
		if strings.HasPrefix(lower, scheme) {
			b.WriteString(`<a href="` + html.EscapeString(strings.TrimSpace(dest)) + `">` + label + "</a>")
			return
		}
	}
	b.WriteString(label)
}

// unescape resolves Markdown backslash escapes and character references, as goldmark's HTML writer does.
func unescape(v []byte) []byte {
	return util.ResolveEntityNames(util.ResolveNumericReferences(util.UnescapePunctuations(v)))
}

// htmlText is the text of an HTML fragment: tags dropped, entities decoded, a line break for block elements.
func htmlText(s string) string {
	z := xhtml.NewTokenizer(strings.NewReader(s))
	var b strings.Builder
	for {
		switch z.Next() {
		case xhtml.ErrorToken:
			lines := strings.Split(b.String(), "\n")
			out := lines[:0]
			for _, l := range lines {
				if l = strings.TrimSpace(l); l != "" {
					out = append(out, l)
				}
			}
			return strings.Join(out, "\n")
		case xhtml.TextToken:
			b.Write(z.Text())
		case xhtml.StartTagToken, xhtml.EndTagToken, xhtml.SelfClosingTagToken:
			name, _ := z.TagName()
			switch string(name) {
			case "br", "p", "div", "li", "tr", "h1", "h2", "h3", "h4", "h5", "h6", "blockquote", "pre", "table", "ul", "ol":
				b.WriteString("\n")
			}
		}
	}
}

// splitEscaped cuts plain text into escaped pieces of at most n characters, at line breaks when it can.
func splitEscaped(s string, n int) []string {
	var (
		out  []string
		cur  strings.Builder
		size int
	)
	for _, line := range strings.SplitAfter(s, "\n") {
		e := esc(line)
		if size+runes(e) <= n {
			cur.WriteString(e)
			size += runes(e)
			continue
		}
		if size > 0 {
			out = append(out, strings.TrimRight(cur.String(), "\n"))
			cur.Reset()
			size = 0
		}
		for _, r := range line {
			piece := esc(string(r))
			if size+runes(piece) > n {
				out = append(out, cur.String())
				cur.Reset()
				size = 0
			}
			cur.WriteString(piece)
			size += runes(piece)
		}
	}
	if strings.TrimSpace(cur.String()) != "" {
		out = append(out, strings.TrimRight(cur.String(), "\n"))
	}
	return out
}
