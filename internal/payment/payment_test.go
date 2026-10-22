package payment

import (
	"context"
	"net/http"
	"testing"
)

type fakeGateway struct{ name string }

func (f fakeGateway) Name() string  { return f.name }
func (f fakeGateway) Label() string { return "Fake" }
func (f fakeGateway) Fields() []Field {
	return []Field{{Key: "key", Label: "Key", Type: FieldPassword, Required: true}}
}
func (f fakeGateway) Pay(context.Context, map[string]string, PayRequest) (*PayResult, error) {
	return &PayResult{Type: PayTypeURL, Data: "https://pay.example.com"}, nil
}
func (f fakeGateway) Notify(context.Context, map[string]string, *http.Request) (*NotifyResult, *NotifyResponse, error) {
	return &NotifyResult{TradeNo: "t"}, &NotifyResponse{Status: 200, Body: "success", ContentType: "text/plain"}, nil
}

func TestRegistry(t *testing.T) {
	Register(fakeGateway{"zzz_fake_b"})
	Register(fakeGateway{"zzz_fake_a"})
	gw, ok := Get("zzz_fake_a")
	if !ok || gw.Label() != "Fake" {
		t.Fatal("get")
	}
	names := Names()
	if names[len(names)-2] != "zzz_fake_a" || names[len(names)-1] != "zzz_fake_b" {
		t.Fatalf("names = %v", names)
	}
	if l := List(); l[len(l)-1].Name() != "zzz_fake_b" {
		t.Fatal("list order")
	}
	defer func() {
		if recover() == nil {
			t.Fatal("duplicate must panic")
		}
	}()
	Register(fakeGateway{"zzz_fake_a"})
}

func TestAmounts(t *testing.T) {
	for cents, want := range map[int64]string{0: "0.00", 5: "0.05", 1234: "12.34", 100: "1.00", -5: "-0.05", -1234: "-12.34"} {
		if got := FormatAmount(cents); got != want {
			t.Errorf("FormatAmount(%d) = %s, want %s", cents, got, want)
		}
	}
	for in, want := range map[string]int64{"12.34": 1234, "12.3": 1230, "12": 1200, "0.05": 5, ".5": 50, "-1.25": -125, "+3": 300} {
		got, err := ParseAmount(in)
		if err != nil || got != want {
			t.Errorf("ParseAmount(%q) = %d %v, want %d", in, got, err, want)
		}
	}
	for _, bad := range []string{"1.234", "abc", "1.x", "1.-5"} {
		if _, err := ParseAmount(bad); err == nil {
			t.Errorf("ParseAmount(%q) must fail", bad)
		}
	}
}
