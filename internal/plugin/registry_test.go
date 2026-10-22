package plugin

import (
	"slices"
	"testing"
)

func TestRegistry(t *testing.T) {
	r := NewRegistry[int]("thing")
	r.Register("b", 2)
	r.Register("a", 1)
	if v, ok := r.Get("a"); !ok || v != 1 {
		t.Fatal("get")
	}
	if !r.Has("b") || r.Has("c") || r.Len() != 2 {
		t.Fatal("has/len")
	}
	if !slices.Equal(r.Names(), []string{"a", "b"}) {
		t.Fatalf("names = %v", r.Names())
	}
	var order []string
	r.Each(func(n string, _ int) { order = append(order, n) })
	if !slices.Equal(order, []string{"a", "b"}) {
		t.Fatalf("each = %v", order)
	}
	for _, name := range []string{"a", ""} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("Register(%q) must panic", name)
				}
			}()
			r.Register(name, 3)
		}()
	}
}
