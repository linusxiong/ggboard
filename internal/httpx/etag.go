package httpx

import "strings"

// ETagMatches reports whether the If-None-Match header value names etag (quoted, bare or weak, in a list, or "*").
// Tags are compared whole, never as substrings: when one resource has several representations (an encrypted
// subscription's "<sha1>-age-<key>" next to the plaintext "<sha1>"), a cached tag of one must not match another.
func ETagMatches(inm, etag string) bool {
	for t := range strings.SplitSeq(inm, ",") {
		t = strings.TrimSpace(t)
		if t == "*" {
			return true
		}
		t = strings.Trim(strings.TrimPrefix(t, "W/"), `"`)
		if t != "" && t == etag {
			return true
		}
	}
	return false
}
