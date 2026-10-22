package gwutil

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/shopspring/decimal"
)

func TestParseCents(t *testing.T) {
	for in, want := range map[string]int64{"12.34": 1234, "12.3": 1230, "10": 1000, "100.0000": 10000, "0.005": 1, "0.004": 0, " 1.235 ": 124, "1e+06": 100000000} {
		got, err := ParseCents(in)
		if err != nil || got != want {
			t.Errorf("ParseCents(%q) = %d %v, want %d", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "abc", "1,00"} {
		if _, err := ParseCents(bad); err == nil {
			t.Errorf("ParseCents(%q) must fail", bad)
		}
	}
}

func TestAmountsAndRates(t *testing.T) {
	for cents, want := range map[int64]string{1234: "12.34", 1230: "12.3", 10000: "100", 5: "0.05"} {
		if got := TrimmedAmount(cents); got != want {
			t.Errorf("TrimmedAmount(%d) = %s", cents, got)
		}
	}
	r, err := ParseRate("")
	if err != nil || !r.Equal(decimal.NewFromInt(1)) {
		t.Fatal("empty rate = 1")
	}
	if _, err := ParseRate("0"); err == nil {
		t.Fatal("zero rate")
	}
	r, _ = ParseRate("0.14")
	if got := ConvertCents(1234, r); got != 173 {
		t.Fatalf("convert = %d", got)
	}
	if !CentsDecimal(1234).Equal(decimal.RequireFromString("12.34")) {
		t.Fatal("cents decimal")
	}
}

func TestURLsAndSigning(t *testing.T) {
	if s, err := BaseURL(" https://pay.example.com/ ", "x", false); err != nil || s != "https://pay.example.com" {
		t.Fatalf("base = %s %v", s, err)
	}
	if _, err := BaseURL("http://pay.example.com", "x", false); err == nil {
		t.Fatal("http must be rejected")
	}
	if _, err := BaseURL("pay.example.com", "x", true); err == nil {
		t.Fatal("relative must be rejected")
	}
	if got := PaymentUUID("https://h.example.com/api/v1/guest/payment/notify/epay/abc123"); got != "abc123" {
		t.Fatalf("uuid = %s", got)
	}
	if got := PaymentUUID("/api/v1/guest/payment/notify/stripe/def/"); got != "def" {
		t.Fatalf("uuid path = %s", got)
	}
	if PaymentUUID("") != "" {
		t.Fatal("empty uuid")
	}
	if got := SignString(map[string]string{"b": "2", "a": "1", "sign": "x", "e": ""}, "sign"); got != "a=1&b=2" {
		t.Fatalf("sign string = %s", got)
	}
	if !EqualHex("ABCD", "abcd") || EqualHex("", "") || EqualHex("ab", "abcd") {
		t.Fatal("EqualHex")
	}
	if MD5Hex("") != "d41d8cd98f00b204e9800998ecf8427e" {
		t.Fatal("md5")
	}
	if Truncate("abcdef", 3) != "abc" || Truncate("ab", 3) != "ab" {
		t.Fatal("truncate")
	}
	if RequireAmount("x", 0) == nil || RequireAmount("x", 1) != nil {
		t.Fatal("require amount")
	}
	if Required(map[string]string{"a": " "}, "x", "a") == nil || Required(map[string]string{"a": "1"}, "x", "a") != nil {
		t.Fatal("required")
	}
	ue := &UserError{Msg: "too small", Err: errors.New("inner")}
	if ue.UserMessage() != "too small" || !strings.Contains(ue.Error(), "inner") || errors.Unwrap(ue) == nil {
		t.Fatal("user error")
	}
}

func TestValuesAndBody(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/n?a=1&a=2&b=x", nil)
	v, err := Values(r)
	if err != nil || v["a"] != "1" || v["b"] != "x" {
		t.Fatalf("get = %v %v", v, err)
	}
	r = httptest.NewRequest(http.MethodPost, "/n?q=1", strings.NewReader("a=%2B1&c=3"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	v, _ = Values(r)
	if v["a"] != "+1" || v["c"] != "3" || v["q"] != "" {
		t.Fatalf("post = %v", v)
	}
	r = httptest.NewRequest(http.MethodPost, "/n?q=1", nil)
	if v, _ = Values(r); v["q"] != "1" {
		t.Fatalf("fallback = %v", v)
	}
	if _, err := ReadBody(strings.NewReader(strings.Repeat("x", MaxBody+1))); !errors.Is(err, ErrBodyTooLarge) {
		t.Fatal("body limit")
	}
	if b, err := ReadBody(nil); err != nil || b != nil {
		t.Fatal("nil body")
	}
	if resp := TextResponse(200, "ok"); resp.Body != "ok" || !strings.HasPrefix(resp.ContentType, "text/plain") {
		t.Fatal("text response")
	}
	if Client(nil) != DefaultClient {
		t.Fatal("default client")
	}
}
