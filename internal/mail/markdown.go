package mail

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/renderer"
	gmhtml "github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
	xhtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// The custom-template pipeline (BACKEND.md §6): {{name}} placeholders become opaque alphanumeric tokens, goldmark
// renders the Markdown (GFM, raw HTML allowed: the author is an administrator, single line breaks kept), the AST
// gets the layout's inline e-mail styles, and the tokens are replaced by HTML-escaped values in one pass (so a value
// is never read as Markdown, HTML or another placeholder). The plain-text part is derived from the same AST with
// the raw values.
//
// Two conventions make the Markdown read like the built-in mails: a top-level paragraph that is nothing but a link
// is rendered as the layout's button, and a link whose address is empty once substituted (no url, app_url unset) is
// dropped (a button disappears, an inline link keeps its text), like the built-in {{if .URL}}.

// Inline styles, copied from templates/layout.html and the built-in content templates.
const (
	stylePara       = "margin:0 0 16px;font-size:16px;line-height:24px;color:#3C3C43;"
	styleParaQuote  = "margin:0 0 8px;font-size:15px;line-height:22px;color:#1C1C1E;"
	styleParaItem   = "margin:0 0 8px;font-size:16px;line-height:24px;color:#3C3C43;"
	styleH1         = "margin:0 0 16px;font-size:22px;line-height:28px;font-weight:700;letter-spacing:-0.3px;color:#1C1C1E;"
	styleH2         = "margin:0 0 12px;font-size:20px;line-height:25px;font-weight:600;color:#1C1C1E;"
	styleH3         = "margin:0 0 8px;font-size:17px;line-height:22px;font-weight:600;color:#1C1C1E;"
	styleH4         = "margin:0 0 8px;font-size:15px;line-height:20px;font-weight:600;color:#1C1C1E;"
	styleLink       = "color:#007AFF;text-decoration:none;"
	styleList       = "margin:0 0 16px;padding:0 0 0 24px;font-size:16px;line-height:24px;color:#3C3C43;"
	styleItem       = "margin:0 0 4px;font-size:16px;line-height:24px;color:#3C3C43;"
	styleQuote      = "margin:8px 0 16px;padding:14px 16px 6px;background:#F2F2F7;border-radius:12px;font-size:15px;line-height:22px;color:#1C1C1E;"
	styleCode       = "padding:2px 6px;background:#F2F2F7;border-radius:6px;font-size:14px;color:#1C1C1E;font-family:'SF Mono',ui-monospace,Menlo,Consolas,monospace;"
	stylePre        = "margin:0 0 16px;padding:14px 16px;background:#F2F2F7;border-radius:12px;overflow-x:auto;font-size:14px;line-height:20px;color:#1C1C1E;"
	stylePreCode    = "font-family:'SF Mono',ui-monospace,Menlo,Consolas,monospace;"
	styleRule       = "margin:24px 0;height:0;border:0;border-top:1px solid #E5E5EA;"
	styleImage      = "max-width:100%;height:auto;border:0;"
	styleTable      = "width:100%;margin:8px 0 16px;border-collapse:collapse;font-size:15px;line-height:20px;"
	styleTH         = "padding:10px 12px;border-bottom:1px solid #E5E5EA;text-align:left;font-weight:600;color:#1C1C1E" // goldmark appends ";text-align:…"
	styleTD         = "padding:10px 12px;border-bottom:1px solid #E5E5EA;color:#3C3C43"
	styleButtonLink = "display:inline-block;padding:12px 22px;font-size:16px;line-height:20px;font-weight:600;color:#FFFFFF;text-decoration:none;"
	styleTrusted    = "margin:0 0 16px;font-size:16px;line-height:24px;color:#3C3C43;"

	buttonOpen  = `<table role="presentation" cellpadding="0" cellspacing="0" border="0" style="margin:24px 0 8px;"><tr><td style="background:#007AFF;border-radius:12px;">`
	buttonClose = "</td></tr></table>\n"
)

var customMarkdown = goldmark.New(
	goldmark.WithExtensions(extension.GFM),
	goldmark.WithRendererOptions(
		gmhtml.WithUnsafe(),
		gmhtml.WithHardWraps(),
		// goldmark registers renderers from the highest priority value to the lowest, so the lowest wins: 100
		// replaces the code block renderers of goldmark's own (1000).
		renderer.WithNodeRenderers(util.Prioritized(mailNodeRenderer{}, 100)),
	),
)

var (
	kindMailButton = ast.NewNodeKind("MailButton")
	kindMailBlock  = ast.NewNodeKind("MailBlock")
)

// mailButton is a top-level paragraph that holds only a link: rendered as the layout's button.
type mailButton struct{ ast.BaseBlock }

// Kind implements ast.Node.
func (n *mailButton) Kind() ast.NodeKind { return kindMailButton }

// Dump implements ast.Node.
func (n *mailButton) Dump(source []byte, level int) { ast.DumpHelper(n, source, level, nil, nil) }

// mailBlock is a <div> standing in for a paragraph that holds only trusted block HTML (notify's ContentHTML).
type mailBlock struct{ ast.BaseBlock }

// Kind implements ast.Node.
func (n *mailBlock) Kind() ast.NodeKind { return kindMailBlock }

// Dump implements ast.Node.
func (n *mailBlock) Dump(source []byte, level int) { ast.DumpHelper(n, source, level, nil, nil) }

// mailNodeRenderer renders the two node kinds above and the code blocks (goldmark's renderers give <pre> no
// attributes).
type mailNodeRenderer struct{}

// RegisterFuncs implements renderer.NodeRenderer.
func (mailNodeRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(kindMailButton, func(w util.BufWriter, _ []byte, _ ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			_, _ = w.WriteString(buttonOpen)
		} else {
			_, _ = w.WriteString(buttonClose)
		}
		return ast.WalkContinue, nil
	})
	reg.Register(kindMailBlock, func(w util.BufWriter, _ []byte, _ ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			_, _ = w.WriteString(`<div style="` + styleTrusted + `">`)
		} else {
			_, _ = w.WriteString("</div>\n")
		}
		return ast.WalkContinue, nil
	})
	code := func(w util.BufWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			_, _ = w.WriteString("</code></pre>\n")
			return ast.WalkContinue, nil
		}
		_, _ = w.WriteString(`<pre style="` + stylePre + `"><code style="` + stylePreCode + `">`)
		lines := n.Lines()
		for i := range lines.Len() {
			seg := lines.At(i)
			gmhtml.DefaultWriter.RawWrite(w, seg.Value(source))
		}
		return ast.WalkContinue, nil
	}
	reg.Register(ast.KindCodeBlock, code)
	reg.Register(ast.KindFencedCodeBlock, code)
}

// tokens maps the placeholders of one rendering to opaque tokens. Every token has the same length and starts with
// letters, so none is a prefix of another and none can start a Markdown construct (lists, emphasis, entities).
type tokens struct {
	byName map[string]string
	order  []string               // tokens, sorted
	vals   map[string]customValue // by token
}

func newTokens(vals map[string]customValue) *tokens {
	nonce := strings.ToLower(rand.Text()[:12])
	names := make([]string, 0, len(vals))
	for n := range vals {
		names = append(names, n)
	}
	slices.Sort(names)
	t := &tokens{byName: make(map[string]string, len(vals)), vals: make(map[string]customValue, len(vals))}
	for i, n := range names {
		tok := fmt.Sprintf("mt%s%03dz", nonce, i)
		t.byName[n] = tok
		t.order = append(t.order, tok)
		t.vals[tok] = vals[n]
	}
	return t
}

// replacer substitutes every token in one pass with its HTML (html) or raw value.
func (t *tokens) replacer(html bool) *strings.Replacer {
	pairs := make([]string, 0, 2*len(t.order))
	for _, tok := range t.order {
		v := t.vals[tok]
		if html {
			pairs = append(pairs, tok, v.html)
		} else {
			pairs = append(pairs, tok, v.text)
		}
	}
	return strings.NewReplacer(pairs...)
}

// renderMarkdown renders a custom body with vals: the HTML of the card's content and the plain text.
func renderMarkdown(body string, vals map[string]customValue) (string, string, error) {
	tok := newTokens(vals)
	source := []byte(placeholderRe.ReplaceAllStringFunc(body, func(m string) string {
		if t, ok := tok.byName[m[2:len(m)-2]]; ok {
			return t
		}
		return m // unknown placeholders stay literal (preview)
	}))
	doc := customMarkdown.Parser().Parse(text.NewReader(source))
	raw := tok.replacer(false)
	prepareDoc(doc, source, tok, raw)
	var buf bytes.Buffer
	if err := customMarkdown.Renderer().Render(&buf, source, doc); err != nil {
		return "", "", err
	}
	d := textDeriver{src: source, raw: raw}
	return tok.replacer(true).Replace(buf.String()), raw.Replace(d.blocks(doc, "\n\n")), nil
}

// prepareDoc applies the two conventions (empty links, buttons), the trusted-HTML block and the inline styles.
func prepareDoc(doc ast.Node, src []byte, tok *tokens, raw *strings.Replacer) {
	var links []*ast.Link
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if l, ok := n.(*ast.Link); ok && entering {
			links = append(links, l)
		}
		return ast.WalkContinue, nil
	})
	for _, l := range links {
		if strings.TrimSpace(raw.Replace(string(markdownUnescape(l.Destination)))) != "" {
			continue
		}
		parent := l.Parent()
		if parent == nil {
			continue
		}
		if p, ok := parent.(*ast.Paragraph); ok && p.Parent() == doc && p.ChildCount() == 1 {
			doc.RemoveChild(doc, p)
			continue
		}
		for c := l.FirstChild(); c != nil; {
			next := c.NextSibling()
			parent.InsertBefore(parent, l, c)
			c = next
		}
		parent.RemoveChild(parent, l)
	}

	trusted := ""
	if v, ok := tok.vals[tok.byName["content"]]; ok && v.trusted {
		trusted = tok.byName["content"]
	}
	for c := doc.FirstChild(); c != nil; {
		next := c.NextSibling()
		if p, ok := c.(*ast.Paragraph); ok && p.ChildCount() == 1 {
			switch child := p.FirstChild().(type) {
			case *ast.Link:
				b := &mailButton{}
				doc.ReplaceChild(doc, p, b)
				b.AppendChild(b, child)
			case *ast.Text:
				if trusted != "" && strings.TrimSpace(string(child.Segment.Value(src))) == trusted {
					b := &mailBlock{}
					doc.ReplaceChild(doc, p, b)
					b.AppendChild(b, child)
				}
			}
		}
		c = next
	}

	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			if s := nodeStyle(n); s != "" {
				n.SetAttributeString("style", s)
			}
		}
		return ast.WalkContinue, nil
	})
}

// nodeStyle returns the inline style of a rendered node ("" = none).
func nodeStyle(n ast.Node) string {
	switch n := n.(type) {
	case *ast.Heading:
		switch n.Level {
		case 1:
			return styleH1
		case 2:
			return styleH2
		case 3:
			return styleH3
		}
		return styleH4
	case *ast.Paragraph:
		switch n.Parent().(type) {
		case *ast.Blockquote:
			return styleParaQuote
		case *ast.ListItem:
			return styleParaItem
		}
		return stylePara
	case *ast.Blockquote:
		return styleQuote
	case *ast.List:
		return styleList
	case *ast.ListItem:
		return styleItem
	case *ast.ThematicBreak:
		return styleRule
	case *ast.Link:
		if _, ok := n.Parent().(*mailButton); ok {
			return styleButtonLink
		}
		return styleLink
	case *ast.AutoLink:
		return styleLink
	case *ast.Image:
		return styleImage
	case *ast.CodeSpan:
		return styleCode
	case *east.Table:
		return styleTable
	case *east.TableCell:
		if n.Parent() != nil && n.Parent().Kind() == east.KindTableHeader {
			return styleTH
		}
		return styleTD
	}
	return ""
}

// markdownUnescape resolves backslash escapes and character references like goldmark's HTML writer.
func markdownUnescape(v []byte) []byte {
	return util.ResolveNumericReferences(util.ResolveEntityNames(util.UnescapePunctuations(v)))
}

// textDeriver writes the plain-text part from the prepared AST: blocks separated by blank lines, headings as
// text, links as "text (url)" (just the url when the text is the url), list items "- " / "1. ", raw HTML without
// tags (<br> → line break, <a href> → "text (url)", <img> → alt), images as their alt text. Values are substituted
// raw afterwards; raw only compares a link's text with its address.
type textDeriver struct {
	src []byte
	raw *strings.Replacer
}

func (d *textDeriver) blocks(n ast.Node, sep string) string {
	var parts []string
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if s := d.block(c); s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, sep)
}

func (d *textDeriver) block(n ast.Node) string {
	switch n := n.(type) {
	case *ast.Paragraph, *ast.Heading, *ast.TextBlock, *mailButton, *mailBlock:
		return strings.TrimSpace(d.inlineText(n))
	case *ast.Blockquote:
		return d.blocks(n, "\n\n")
	case *ast.List:
		var items []string
		i := 0
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			marker := "- "
			if n.IsOrdered() {
				marker = strconv.Itoa(n.Start+i) + ". "
			}
			i++
			body := d.blocks(c, "\n")
			items = append(items, marker+strings.ReplaceAll(body, "\n", "\n"+strings.Repeat(" ", len(marker))))
		}
		return strings.Join(items, "\n")
	case *ast.FencedCodeBlock, *ast.CodeBlock:
		var b strings.Builder
		lines := n.Lines()
		for i := range lines.Len() {
			seg := lines.At(i)
			b.Write(seg.Value(d.src))
		}
		return strings.Trim(b.String(), "\n")
	case *ast.HTMLBlock:
		var b strings.Builder
		lines := n.Lines()
		for i := range lines.Len() {
			seg := lines.At(i)
			b.Write(seg.Value(d.src))
		}
		if n.HasClosure() {
			b.Write(n.ClosureLine.Value(d.src))
		}
		var out strings.Builder
		h := htmlText{b: &out, raw: d.raw}
		h.feed(b.String())
		return tidyLines(out.String())
	case *ast.ThematicBreak:
		return "---"
	case *east.Table:
		var rows []string
		for r := n.FirstChild(); r != nil; r = r.NextSibling() {
			var cells []string
			for c := r.FirstChild(); c != nil; c = c.NextSibling() {
				cells = append(cells, strings.TrimSpace(d.inlineText(c)))
			}
			rows = append(rows, strings.Join(cells, " | "))
		}
		return strings.Join(rows, "\n")
	}
	if n.HasChildren() {
		return d.blocks(n, "\n\n")
	}
	return ""
}

func (d *textDeriver) inlineText(n ast.Node) string {
	var b strings.Builder
	h := htmlText{b: &b, raw: d.raw}
	d.inline(n, &b, &h)
	return b.String()
}

func (d *textDeriver) inline(n ast.Node, b *strings.Builder, h *htmlText) {
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		switch c := c.(type) {
		case *ast.Text:
			v := c.Segment.Value(d.src)
			if c.IsRaw() {
				b.Write(v)
			} else {
				b.Write(markdownUnescape(v))
			}
			if c.HardLineBreak() || c.SoftLineBreak() {
				b.WriteByte('\n')
			}
		case *ast.String:
			b.Write(c.Value)
		case *ast.CodeSpan:
			for t := c.FirstChild(); t != nil; t = t.NextSibling() {
				if tx, ok := t.(*ast.Text); ok {
					b.WriteString(strings.ReplaceAll(string(tx.Segment.Value(d.src)), "\n", " "))
				}
			}
		case *ast.Link:
			start := b.Len()
			d.inline(c, b, h)
			appendURL(b, d.raw, b.String()[start:], string(markdownUnescape(c.Destination)))
		case *ast.AutoLink:
			b.Write(c.Label(d.src))
		case *ast.RawHTML:
			for i := range c.Segments.Len() {
				seg := c.Segments.At(i)
				h.feed(string(seg.Value(d.src)))
			}
		case *east.TaskCheckBox:
			if c.IsChecked {
				b.WriteString("[x] ")
			} else {
				b.WriteString("[ ] ")
			}
		default: // emphasis, strikethrough, image (its alt text), …
			d.inline(c, b, h)
		}
	}
}

// appendURL writes " (url)" after a link's text, nothing when the text is the url, the url alone without text.
func appendURL(b *strings.Builder, raw *strings.Replacer, label, dest string) {
	u := strings.TrimSpace(raw.Replace(dest))
	l := strings.TrimSpace(raw.Replace(label))
	switch {
	case u == "", l == u:
	case l == "":
		b.WriteString(dest)
	default:
		b.WriteString(" (" + dest + ")")
	}
}

// htmlText strips raw HTML for the plain-text part. It keeps state across calls so that an inline <a href> and its
// </a> may arrive as separate raw HTML nodes.
type htmlText struct {
	b     *strings.Builder
	raw   *strings.Replacer // nil: compare link text and address as written
	links []pendingLink
	skip  int // inside <style>, <script>, <head> or <title>
}

type pendingLink struct {
	href string
	at   int
}

func (h *htmlText) feed(s string) {
	z := xhtml.NewTokenizer(strings.NewReader(s))
	for {
		tt := z.Next()
		switch tt {
		case xhtml.ErrorToken:
			return
		case xhtml.TextToken:
			if h.skip == 0 {
				h.b.Write(z.Text())
			}
		case xhtml.StartTagToken, xhtml.SelfClosingTagToken:
			name, hasAttr := z.TagName()
			attrs := map[string]string{}
			for hasAttr {
				var k, v []byte
				k, v, hasAttr = z.TagAttr()
				attrs[string(k)] = string(v)
			}
			switch a := atom.Lookup(name); a {
			case atom.Br:
				h.b.WriteByte('\n')
			case atom.Img:
				h.b.WriteString(attrs["alt"])
			case atom.A:
				if tt == xhtml.StartTagToken {
					h.links = append(h.links, pendingLink{href: attrs["href"], at: h.b.Len()})
				}
			case atom.Style, atom.Script, atom.Head, atom.Title:
				if tt == xhtml.StartTagToken {
					h.skip++
				}
			default:
				if blockTag(a) {
					h.b.WriteByte('\n')
				}
			}
		case xhtml.EndTagToken:
			name, _ := z.TagName()
			switch a := atom.Lookup(name); a {
			case atom.A:
				if n := len(h.links); n > 0 {
					l := h.links[n-1]
					h.links = h.links[:n-1]
					if l.at <= h.b.Len() {
						appendURL(h.b, h.replacer(), h.b.String()[l.at:], l.href)
					}
				}
			case atom.Style, atom.Script, atom.Head, atom.Title:
				if h.skip > 0 {
					h.skip--
				}
			case atom.Td, atom.Th:
				h.b.WriteByte(' ')
			default:
				if blockTag(a) {
					h.b.WriteByte('\n')
				}
			}
		}
	}
}

func (h *htmlText) replacer() *strings.Replacer {
	if h.raw == nil {
		return strings.NewReplacer()
	}
	return h.raw
}

func blockTag(a atom.Atom) bool {
	switch a {
	case atom.P, atom.Div, atom.Table, atom.Tr, atom.Li, atom.Ul, atom.Ol, atom.Blockquote, atom.Pre, atom.Hr,
		atom.H1, atom.H2, atom.H3, atom.H4, atom.H5, atom.H6, atom.Section, atom.Article, atom.Header, atom.Footer:
		return true
	}
	return false
}

// tidyLines collapses the whitespace of stripped HTML: spaces within a line, surrounding blank lines and runs of
// blank lines.
func tidyLines(s string) string {
	var out []string
	blank := false
	for line := range strings.SplitSeq(s, "\n") {
		line = strings.Join(strings.Fields(line), " ")
		if line == "" {
			blank = len(out) > 0
			continue
		}
		if blank {
			out = append(out, "")
			blank = false
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

// htmlToText is the plain text of trusted HTML (notify's ContentHTML without a Content).
func htmlToText(s string) string {
	var b strings.Builder
	h := htmlText{b: &b}
	h.feed(s)
	return tidyLines(b.String())
}
