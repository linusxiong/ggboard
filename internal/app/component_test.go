package app_test

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/logx"
)

type stopRecorder struct {
	name  string
	order *[]string
	mu    *sync.Mutex
}

func (s *stopRecorder) Stop(context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	*s.order = append(*s.order, s.name)
	return nil
}

func TestComponents(t *testing.T) {
	a, err := app.New(testConfig(t), app.WithLogger(logx.Discard(), nil))
	if err != nil {
		t.Fatal(err)
	}
	var (
		mu     sync.Mutex
		order  []string
		builds int
	)
	build := func(name string) func() any {
		return func() any {
			builds++
			return &stopRecorder{name: name, order: &order, mu: &mu}
		}
	}
	first := a.Component("one", build("one"))
	if again := a.Component("one", build("one")); again != first || builds != 1 {
		t.Fatal("Component must build once and return the same value")
	}
	if a.Component("missing", nil) != nil {
		t.Fatal("nil build returns nil for a missing component")
	}
	a.Component("two", build("two"))
	a.Component("plain", func() any { return 42 }) // not a Stopper
	if err := a.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if strings.Join(order, ",") != "two,one" {
		t.Fatalf("stop order = %v", order)
	}
	if a.Component("late", build("late")) != nil || builds != 2 {
		t.Fatal("no component is built after Close")
	}
	if a.Component("one", nil) != first {
		t.Fatal("existing components stay reachable after Close")
	}
}
