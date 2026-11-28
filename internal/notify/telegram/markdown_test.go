package telegram

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/linusxiong/ggboard/internal/notify"
)

func TestRenderLongMarkdown(t *testing.T) {
	body := strings.Join([]string{
		"# 维护通知",
		"",
		"今晚 **23:00** 起维护 *半小时*，~~不影响~~ 期间节点 `hk1` 不可用。",
		"第二行保留换行 & <尖括号>",
		"",
		"- 第一项",
		"- 第二项",
		"  1. 子项",
		"",
		"> 引用 [官网](https://example.com/a?b=1&c=2)",
		"",
		"```",
		"<code> & more",
		"```",
		"",
		"| 套餐 | 价格 |",
		"| --- | --- |",
		"| Pro | 10 |",
		"",
		"[坏链接](javascript:alert(1)) ![图](https://example.com/i.png) <b>raw</b> \\*星号\\*",
		"",
		"<div onclick=\"x\">块 <i>HTML</i></div>",
	}, "\n")
	msgs := RenderLong(notify.Broadcast(1, "公告 <1>", body, true))
	if len(msgs) != 1 {
		t.Fatalf("got %d messages: %q", len(msgs), msgs)
	}
	got := msgs[0]
	for _, want := range []string{
		"<b>公告 &lt;1&gt;</b>\n\n<b>维护通知</b>",
		"今晚 <b>23:00</b> 起维护 <i>半小时</i>，<s>不影响</s> 期间节点 <code>hk1</code> 不可用。\n第二行保留换行 &amp; &lt;尖括号&gt;",
		"• 第一项\n• 第二项\n   1. 子项",
		`<blockquote>引用 <a href="https://example.com/a?b=1&amp;c=2">官网</a></blockquote>`,
		"<pre>&lt;code&gt; &amp; more</pre>",
		"<b>套餐 | 价格</b>\nPro | 10",
		`坏链接 <a href="https://example.com/i.png">图</a> raw *星号*`,
		"块 HTML",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	for _, bad := range []string{"javascript:", "onclick", "<div", "<i>HTML"} {
		if strings.Contains(got, bad) {
			t.Errorf("%q leaked into:\n%s", bad, got)
		}
	}
}

func TestRenderLongPlainTextAndSplitting(t *testing.T) {
	msgs := RenderLong(notify.Broadcast(1, "T", "a *b* <c>\nline\n\n\nsecond", false))
	if len(msgs) != 1 || msgs[0] != "<b>T</b>\n\na *b* &lt;c&gt;\nline\n\nsecond" {
		t.Fatalf("plain = %q", msgs)
	}

	// Paragraphs are packed into messages under the limit; one oversized paragraph is cut into plain pieces.
	para := strings.Repeat("段落&", 400) // 1200 characters, 2400 when escaped
	huge := strings.Repeat("长", maxMessage*2)
	msgs = RenderLong(notify.Broadcast(1, "T", para+"\n\n"+para+"\n\n"+huge+"\n\n**end**", true))
	if len(msgs) < 4 {
		t.Fatalf("got %d messages", len(msgs))
	}
	for i, m := range msgs {
		if n := utf8.RuneCountInString(m); n > maxMessage {
			t.Fatalf("message %d has %d characters", i, n)
		}
		if strings.Contains(m, "&amp") && strings.HasSuffix(m, "&") {
			t.Fatalf("message %d splits an entity", i)
		}
	}
	if !strings.HasPrefix(msgs[0], "<b>T</b>\n\n段落&amp;") || msgs[len(msgs)-1] != "<b>end</b>" {
		t.Fatalf("first/last = %q … %q", msgs[0][:40], msgs[len(msgs)-1])
	}
	var total int
	for _, m := range msgs {
		total += strings.Count(m, "长")
	}
	if total != maxMessage*2 {
		t.Fatalf("lost text: %d of %d", total, maxMessage*2)
	}
}
