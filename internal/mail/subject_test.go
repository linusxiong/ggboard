package mail

import (
	"strings"
	"testing"
)

// A caller-supplied Subject is template data (the ticket subject of ticket_reply); the e-mail subject must not
// replace it in the body, and the HTML <title> must still carry the e-mail subject.
func TestRenderKeepsDataSubject(t *testing.T) {
	r, err := NewRenderer()
	if err != nil {
		t.Fatal(err)
	}
	for _, locale := range []string{"en", "zh-CN"} {
		msg, err := r.Render(TplTicketReply, Brand{AppName: "Example Panel", Locale: locale}, Data{"Subject": "Cannot connect", "Message": "Fixed."})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(msg.Text, "Cannot connect") || !strings.Contains(msg.HTML, "Cannot connect") {
			t.Fatalf("%s: ticket subject missing from body:\n%s", locale, msg.Text)
		}
		if strings.Contains(msg.Text, msg.Subject) {
			t.Fatalf("%s: e-mail subject %q leaked into the body:\n%s", locale, msg.Subject, msg.Text)
		}
		if !strings.Contains(msg.HTML, "<title>"+msg.Subject+"</title>") {
			t.Fatalf("%s: <title> should be the e-mail subject %q", locale, msg.Subject)
		}
	}
	// Templates without a data Subject keep falling back to the e-mail subject.
	msg, err := r.Render(TplNotify, Brand{AppName: "Example Panel", Locale: "en"}, Data{"Content": "Hello"})
	if err != nil {
		t.Fatal(err)
	}
	if msg.Subject != "Example Panel notification" || !strings.Contains(msg.Text, "Example Panel notification") {
		t.Fatalf("notify fallback title: subject=%q text=%q", msg.Subject, msg.Text)
	}
}
