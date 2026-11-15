package gwutiltest

import (
	"reflect"
	"testing"
)

func TestReparses(t *testing.T) {
	for signed, want := range map[string][]map[string]string{
		"a=1&b=2":    {{"a": "1&b=2"}, {"a": "1", "b": "2"}},
		"b=1&a=2":    {{"b": "1&a=2"}},                         // split, the keys sort to a=2&b=1
		"a=1&a=2":    {{"a": "1&a=2"}},                         // split, the second a overwrites the first
		"a=1&sign=x": {{"a": "1&sign=x"}},                      // split, the excluded key drops out of the string
		"a=1&&b=2":   {{"a": "1&&b=2"}, {"a": "1&", "b": "2"}}, // the empty piece has no value, so it is never a field
		"a=x=y":      {{"a": "x=y"}},                           // keys split at the first '='
	} {
		got := Reparses(signed, "sign")
		if len(got) != len(want) {
			t.Fatalf("%s: got %v, want %v", signed, got, want)
		}
		for _, w := range want {
			found := false
			for _, g := range got {
				found = found || reflect.DeepEqual(g, w)
			}
			if !found {
				t.Fatalf("%s: missing %v in %v", signed, w, got)
			}
		}
	}
}
