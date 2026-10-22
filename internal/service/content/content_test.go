package content

import "testing"

func TestRenderBody(t *testing.T) {
	const sub = "https://panel.example.com/api/v1/client/subscribe?token=abc"
	body := "A {{siteName}}\n<!--access start-->\nline1\n{{subscribeUrl}}\n<!--access end-->B<!--access start-->x<!--access end-->C {{urlEncodeSubscribeUrl}} {{safeBase64SubscribeUrl}}"
	got := RenderBody(body, "en-US", "Example", sub, true)
	want := "A Example\n<!--access start-->\nline1\n" + sub + "\n<!--access end-->B<!--access start-->x<!--access end-->C " +
		"https%3A%2F%2Fpanel.example.com%2Fapi%2Fv1%2Fclient%2Fsubscribe%3Ftoken%3Dabc " +
		"aHR0cHM6Ly9wYW5lbC5leGFtcGxlLmNvbS9hcGkvdjEvY2xpZW50L3N1YnNjcmliZT90b2tlbj1hYmM"
	if got != want {
		t.Fatalf("with plan:\n%q\nwant\n%q", got, want)
	}
	gated := RenderBody(body, "en-US", "Example", sub, false)
	notice := `<div class="knowledge-no-access">You must have a valid subscription to view content in this area</div>`
	wantGated := "A Example\n" + notice + "B" + notice + "C " +
		"https%3A%2F%2Fpanel.example.com%2Fapi%2Fv1%2Fclient%2Fsubscribe%3Ftoken%3Dabc " +
		"aHR0cHM6Ly9wYW5lbC5leGFtcGxlLmNvbS9hcGkvdjEvY2xpZW50L3N1YnNjcmliZT90b2tlbj1hYmM"
	if gated != wantGated {
		t.Fatalf("gated:\n%q\nwant\n%q", gated, wantGated)
	}
	// An unterminated block is left alone (Xboard's regex needs both markers).
	if got := RenderBody("<!--access start-->open", "zh-CN", "", sub, false); got != "<!--access start-->open" {
		t.Fatalf("unterminated = %q", got)
	}
	// Replacement text is literal ($1 is not expanded).
	if got := RenderBody("{{siteName}}", "en", "$1 & Co", sub, false); got != "$1 & Co" {
		t.Fatalf("literal = %q", got)
	}
}

func TestLikeEscape(t *testing.T) {
	for in, want := range map[string]string{"100%": "100!%", "a_b": "a!_b", "x!y": "x!!y", "plain": "plain"} {
		if got := likeEscape(in); got != want {
			t.Fatalf("likeEscape(%q) = %q, want %q", in, got, want)
		}
	}
}
