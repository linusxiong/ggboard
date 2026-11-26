package mail

import (
	"slices"
	"strings"
	"testing"

	xhtml "golang.org/x/net/html"
)

// markup lists the elements of an HTML document in order, each with its sorted attribute names, and collects the
// text of its title attributes. An attribute name made of character references (a value written where a name
// stands) is listed as "&#…": it is inert, and it differs with the value.
func markup(t *testing.T, doc string) (elems []string, titles []string) {
	t.Helper()
	root, err := xhtml.Parse(strings.NewReader(doc))
	if err != nil {
		t.Fatal(err)
	}
	for n := range root.Descendants() {
		if n.Type != xhtml.ElementNode {
			continue
		}
		var keys []string
		for _, a := range n.Attr {
			if strings.HasPrefix(a.Key, "&#") {
				keys = append(keys, "&#…")
				continue
			}
			keys = append(keys, a.Key)
			if a.Key == "title" {
				titles = append(titles, a.Val)
			}
		}
		slices.Sort(keys)
		elems = append(elems, n.Data+"["+strings.Join(keys, ",")+"]")
	}
	return elems, titles
}

// Security review 2026-11-25: a placeholder that an administrator's template writes inside a tag, where the value is
// not quoted, let a user's text (here a ticket subject) open a tag or add attributes: `<{{ticketSubject}}>` with
// the subject `img src=x onerror=alert(1)` rendered <img src=x onerror=alert(1)>. Such positions now get the value
// as character references, which still read as the same text in an attribute value; text and quoted attributes are
// unchanged.
func TestCustomValuesCannotOpenMarkup(t *testing.T) {
	r := customRenderer(t)
	render := func(body, subject string) Message {
		t.Helper()
		msg, err := r.RenderCustom(TplTicketReply, customBrand, Custom{Subject: "s", Body: body},
			Data{"Subject": subject, "Message": "m", "URL": "https://panel.example.com/tickets/1"})
		if err != nil {
			t.Fatal(err)
		}
		return msg
	}
	const evil = "img src=x onerror=alert(1)"
	for _, body := range []string{
		"<{{ticketSubject}}>",
		"<span title={{ticketSubject}}>x</span>",
		"<span {{ticketSubject}}>x</span>",
		"<span title='a' {{ticketSubject}}>x</span>",
		"<span title=\"a\"{{ticketSubject}}>x</span>",
		"<a href=\"https://example.com\" / {{ticketSubject}}>x</a>",
		"</{{ticketSubject}}>",
		"text <span title={{ticketSubject}}",
	} {
		benign, attacked := render(body, "hello"), render(body, evil)
		want, _ := markup(t, benign.HTML)
		got, _ := markup(t, attacked.HTML)
		if !slices.Equal(got, want) {
			t.Errorf("body %q: the subject changed the markup\n got %v\nwant %v\n%s", body, got, want, attacked.HTML)
		}
		if strings.Contains(attacked.HTML, "<img src=x") {
			t.Errorf("body %q: %s", body, attacked.HTML)
		}
	}

	// An unquoted attribute value still reads as the subject, whitespace included.
	_, titles := markup(t, render("<span title={{ticketSubject}}>x</span>", evil).HTML)
	if !slices.Contains(titles, evil) {
		t.Errorf("unquoted title = %q, want %q", titles, evil)
	}
	// The raw text of <style> (no character references there) cannot carry the subject's CSS.
	if msg := render("<div>\n<style>p{{{ticketSubject}}}</style>\n</div>", "color:red}body{display:none"); strings.Contains(msg.HTML, "display:none") {
		t.Errorf("subject reached a style sheet:\n%s", msg.HTML)
	}
	// Text and quoted attribute values are written as before: escaped, not character-referenced; the plain text
	// carries the raw subject.
	msg := render("{{ticketSubject}} <span title=\"{{ticketSubject}}\">x</span> <b title='{{ticketSubject}}'>y</b>", `Need help & "quotes"`)
	for _, want := range []string{
		"Need help &amp; &#34;quotes&#34; <span",
		`<span title="Need help &amp; &#34;quotes&#34;">x</span>`,
		`<b title='Need help &amp; &#34;quotes&#34;'>y</b>`,
	} {
		if !strings.Contains(msg.HTML, want) {
			t.Errorf("HTML lacks %s\n%s", want, msg.HTML)
		}
	}
	if !strings.Contains(msg.Text, `Need help & "quotes" x y`) {
		t.Errorf("plain text:\n%s", msg.Text)
	}
}

// The substitution pass writes every byte of the rendered HTML back unchanged apart from the tokens, whatever the
// markup (comments, doctype, raw text, an unterminated tag at the end), and picks each token's form by position.
func TestSubstituteHTMLForms(t *testing.T) {
	tok := newTokens(map[string]customValue{"v": {text: `a b"<`, html: "a b&#34;&lt;"}})
	v := tok.byName["v"]
	for _, s := range []string{
		"<!doctype html><p class=\"x\">plain &amp; <b>bold</b><br/></p><!-- note --><style>p{}</style><textarea>t</textarea>",
		"<table role=\"presentation\"><tr><td style='a:b'>x</td></tr></table>\n<div",
		"<p>unterminated <a href=\"x",
		"<plaintext><b>",
	} {
		if got := tok.substituteHTML(s); got != s {
			t.Errorf("markup changed:\n got %q\nwant %q", got, s)
		}
	}
	refs := charRefs(`a b"<`)
	for in, want := range map[string]string{
		"<p>" + v + "</p>":                         "<p>a b&#34;&lt;</p>",
		"<!-- " + v + " -->":                       "<!-- a b&#34;&lt; -->",
		`<p title="` + v + `" data-x='` + v + `'>`: `<p title="a b&#34;&lt;" data-x='a b&#34;&lt;'>`,
		"<p title=" + v + ">":                      "<p title=" + refs + ">",
		"<p " + v + ">":                            "<p " + refs + ">",
		"<" + v + ">":                              "<" + refs + ">",
		"</" + v + ">":                             "</" + refs + ">",
		`<p title="a"/` + v + `>`:                  `<p title="a"/` + refs + ">",
		"<style>" + v + "</style>":                 "<style>" + refs + "</style>",
		"<p>x</p><div title=" + v:                  "<p>x</p><div title=" + refs,
		"<p title=\"x\" " + v + ">":                "<p title=\"x\" " + refs + ">",
	} {
		if got := tok.substituteHTML(in); got != want {
			t.Errorf("%q:\n got %q\nwant %q", in, got, want)
		}
	}
}
