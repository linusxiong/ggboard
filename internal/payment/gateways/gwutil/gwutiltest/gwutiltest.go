// Package gwutiltest holds test helpers for gateways that sign raw k=v strings (gwutil.SignString). Only tests
// import it.
package gwutiltest

import (
	"strings"

	"github.com/linusxiong/ggboard/internal/payment/gateways/gwutil"
)

// maxFields bounds the enumeration (2^(n-1) splits).
const maxFields = 20

// Reparses returns every parameter map whose gwutil.SignString(params, exclude...) is exactly signed. The sign
// string does not escape values, so each '&' in signed is either a field separator or part of a value, and a
// forged callback that reuses the signature over signed must be one of these maps (apart from fields with empty
// values, which the sign string drops). Keys are split at their first '='. Folding a field into the value before
// it always parses again, so a string with n fields has at least 2^(n-1) parses; what Pay can guarantee is that
// none of its own values contains '&'. Feed every map to Notify to prove that a signature the merchant made for a
// request can never verify as a callback (docs/PAYMENTS_GATEWAYS.md §8, MISTAKES #246 and #302).
func Reparses(signed string, exclude ...string) []map[string]string {
	pieces := strings.Split(signed, "&")
	if len(pieces) > maxFields {
		panic("gwutiltest: too many fields to enumerate")
	}
	var out []map[string]string
	for mask := 0; mask < 1<<(len(pieces)-1); mask++ {
		params := map[string]string{}
		field := pieces[0]
		for i := 1; i <= len(pieces); i++ {
			if i < len(pieces) && mask&(1<<(i-1)) == 0 {
				field += "&" + pieces[i]
				continue
			}
			k, v, _ := strings.Cut(field, "=")
			params[k] = v
			if i < len(pieces) {
				field = pieces[i]
			}
		}
		// Duplicate keys, empty values, excluded keys and unsorted splits all change the string and drop out here.
		if gwutil.SignString(params, exclude...) == signed {
			out = append(out, params)
		}
	}
	return out
}
