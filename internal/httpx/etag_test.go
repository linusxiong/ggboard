package httpx_test

import (
	"testing"

	"github.com/linusxiong/ggboard/internal/httpx"
)

func TestETagMatches(t *testing.T) {
	cases := []struct {
		inm, tag string
		want     bool
	}{
		{`"abc"`, "abc", true}, {`W/"abc"`, "abc", true}, {`abc`, "abc", true}, {`"x", W/"abc"`, "abc", true},
		{`*`, "abc", true}, {`"abc-age-1"`, "abc", false}, {`"abc"`, "abc-age-1", false}, {``, "abc", false},
		{` W/"abc" `, "abc", true}, {`"xabcx"`, "abc", false}, {`"abc"x`, "abc", false}, {`""`, "", false},
	}
	for _, c := range cases {
		if got := httpx.ETagMatches(c.inm, c.tag); got != c.want {
			t.Errorf("ETagMatches(%q, %q) = %v", c.inm, c.tag, got)
		}
	}
}
