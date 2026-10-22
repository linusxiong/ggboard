// Package gwutil holds the helpers shared by the built-in payment gateways (docs/PAYMENTS_GATEWAYS.md §1):
// the HTTP client, base-URL normalization, bounded body reads, form/query parameter maps, sorted sign strings,
// constant-time hex comparison and exact decimal <-> cents conversion.
package gwutil

import (
	"context"
	"crypto/hmac"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/linusxiong/ggboard/internal/payment"
)

// Timeout is the per-request timeout of gateway HTTP calls.
const Timeout = 15 * time.Second

// MaxBody bounds every body read from a gateway (requests and responses).
const MaxBody = 1 << 20

// UserAgent is sent with every outgoing gateway request.
const UserAgent = "ggboard (+https://github.com/linusxiong/ggboard)"

// DefaultClient is the shared HTTP client (TLS verification on, HTTP/2 and HTTPS_PROXY support kept).
var DefaultClient = &http.Client{Timeout: Timeout}

// Client returns c, or DefaultClient when c is nil.
func Client(c *http.Client) *http.Client {
	if c != nil {
		return c
	}
	return DefaultClient
}

// ErrBodyTooLarge is returned by ReadBody when the body exceeds MaxBody.
var ErrBodyTooLarge = errors.New("gateway: body too large")

// ReadBody reads at most MaxBody bytes of r.
func ReadBody(r io.Reader) ([]byte, error) {
	if r == nil {
		return nil, nil
	}
	b, err := io.ReadAll(io.LimitReader(r, MaxBody+1))
	if err != nil {
		return nil, err
	}
	if len(b) > MaxBody {
		return nil, ErrBodyTooLarge
	}
	return b, nil
}

// BaseURL trims spaces and trailing slashes and requires an absolute http(s) URL. Plain http is accepted
// only when allowHTTP is set (self-hosted EPay/Epusdt) and then logged at WARN.
func BaseURL(raw, gateway string, allowHTTP bool) (string, error) {
	s := strings.TrimRight(strings.TrimSpace(raw), "/")
	u, err := url.Parse(s)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return "", fmt.Errorf("%s: invalid base URL", gateway)
	}
	if u.Scheme == "http" {
		if !allowHTTP {
			return "", fmt.Errorf("%s: base URL must use https", gateway)
		}
		slog.Warn("payment gateway base URL uses plain http", "gateway", gateway, "host", u.Host)
	}
	return s, nil
}

// Values returns the first value of every key: GET uses the query string; other methods use the parsed
// form body and fall back to the query string when the body carries no fields.
func Values(r *http.Request) (map[string]string, error) {
	src := r.URL.Query()
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		if r.Body != nil {
			r.Body = http.MaxBytesReader(nil, r.Body, MaxBody)
		}
		if err := r.ParseForm(); err != nil {
			return nil, err
		}
		if len(r.PostForm) > 0 {
			src = r.PostForm
		}
	}
	out := make(map[string]string, len(src))
	for k, v := range src {
		if len(v) > 0 {
			out[k] = v[0]
		}
	}
	return out, nil
}

// SignString joins the non-empty params (except the excluded keys) as k=v pairs sorted by key with "&",
// using raw (not URL-encoded) values.
func SignString(params map[string]string, exclude ...string) string {
	keys := make([]string, 0, len(params))
	for k, v := range params {
		if v == "" || contains(exclude, k) {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteByte('&')
		}
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(params[k])
	}
	return b.String()
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// MD5Hex returns the lowercase hex MD5 of s.
func MD5Hex(s string) string {
	sum := md5.Sum([]byte(s)) //nolint:gosec // the gateway protocols mandate MD5 signatures
	return hex.EncodeToString(sum[:])
}

// EqualHex compares two hex digests in constant time, ignoring case and surrounding spaces.
func EqualHex(a, b string) bool {
	a, b = strings.ToLower(strings.TrimSpace(a)), strings.ToLower(strings.TrimSpace(b))
	if a == "" || b == "" {
		return false
	}
	return hmac.Equal([]byte(a), []byte(b))
}

// ParseCents converts a decimal amount ("12.3", "12.34", "100.0000", "10") into cents, rounding half up
// beyond two places.
func ParseCents(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, errors.New("gateway: empty amount")
	}
	d, err := decimal.NewFromString(s)
	if err != nil {
		return 0, fmt.Errorf("gateway: invalid amount %q", s)
	}
	return d.Shift(2).Round(0).IntPart(), nil
}

// ParsePaidCents parses the amount of a verified payment notification. A zero or negative amount is an error:
// ordersvc treats Amount 0 as "not reported" and would skip the amount check, so a signed "0.00" must never
// reach it as a payment.
func ParsePaidCents(s string) (int64, error) {
	c, err := ParseCents(s)
	if err != nil {
		return 0, err
	}
	if c <= 0 {
		return 0, fmt.Errorf("gateway: non-positive paid amount %q", strings.TrimSpace(s))
	}
	return c, nil
}

// CentsDecimal returns cents as a decimal major-unit amount.
func CentsDecimal(cents int64) decimal.Decimal { return decimal.New(cents, -2) }

// TrimmedAmount renders cents without trailing zeros ("12.30" -> "12.3", "100.00" -> "100").
func TrimmedAmount(cents int64) string {
	s := payment.FormatAmount(cents)
	s = strings.TrimRight(s, "0")
	return strings.TrimSuffix(s, ".")
}

// ParseRate parses an exchange rate; empty means 1. Rates must be positive.
func ParseRate(s string) (decimal.Decimal, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return decimal.NewFromInt(1), nil
	}
	d, err := decimal.NewFromString(s)
	if err != nil || !d.IsPositive() {
		return decimal.Zero, fmt.Errorf("invalid exchange rate %q", s)
	}
	return d, nil
}

// ConvertCents returns roundHalfUp(cents x rate) (minor units of the target currency).
func ConvertCents(cents int64, rate decimal.Decimal) int64 {
	return decimal.NewFromInt(cents).Mul(rate).Round(0).IntPart()
}

// PaymentUUID returns the payment uuid encoded as the last path segment of a notify URL or request path
// (/api/v1/guest/payment/notify/<gateway>/<uuid>).
func PaymentUUID(notifyURLOrPath string) string {
	p := notifyURLOrPath
	if u, err := url.Parse(notifyURLOrPath); err == nil && u.Path != "" {
		p = u.Path
	}
	base := path.Base(strings.TrimRight(p, "/"))
	if base == "." || base == "/" {
		return ""
	}
	return base
}

// Do sends req with the User-Agent header and returns the status and a bounded body. Non-2xx responses are
// logged at WARN with at most 512 bytes of the body (never the request).
func Do(ctx context.Context, c *http.Client, req *http.Request, gateway string) (int, []byte, error) {
	req.Header.Set("User-Agent", UserAgent)
	start := time.Now()
	resp, err := Client(c).Do(req.WithContext(ctx)) //nolint:gosec // G704: gateway endpoints come from admin configuration by design
	if err != nil {
		return 0, nil, fmt.Errorf("%s: request failed: %w", gateway, err)
	}
	defer resp.Body.Close()
	body, err := ReadBody(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, fmt.Errorf("%s: read response: %w", gateway, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		slog.Warn("payment gateway returned an error status", "gateway", gateway, "status", resp.StatusCode,
			"latency", time.Since(start).String(), "body", Truncate(string(body), 512))
	}
	return resp.StatusCode, body, nil
}

// Truncate cuts s to at most n bytes.
func Truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// TextResponse builds a text/plain notify response.
func TextResponse(status int, body string) *payment.NotifyResponse {
	return &payment.NotifyResponse{Status: status, Body: body, ContentType: "text/plain; charset=utf-8"}
}

// RequireAmount rejects non-positive charges (the order service fulfils zero-amount orders itself).
func RequireAmount(gateway string, cents int64) error {
	if cents <= 0 {
		return fmt.Errorf("%s: amount must be positive", gateway)
	}
	return nil
}

// Required returns an error naming the first missing (blank) config key.
func Required(cfg map[string]string, gateway string, keys ...string) error {
	for _, k := range keys {
		if strings.TrimSpace(cfg[k]) == "" {
			return fmt.Errorf("%s: config %q is required", gateway, k)
		}
	}
	return nil
}

// UserError is a gateway error whose message may be shown to the paying user (e.g. "amount below the
// provider minimum"). The order service detects it through the UserMessage method.
type UserError struct {
	Msg string
	Err error
}

func (e *UserError) Error() string {
	if e.Err != nil {
		return e.Msg + ": " + e.Err.Error()
	}
	return e.Msg
}

// Unwrap returns the underlying error.
func (e *UserError) Unwrap() error { return e.Err }

// UserMessage is the user-facing message.
func (e *UserError) UserMessage() string { return e.Msg }
