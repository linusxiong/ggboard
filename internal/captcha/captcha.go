// Package captcha verifies captcha tokens with the provider selected by settings.captcha_provider
// (none, turnstile, recaptcha, hcaptcha). Providers are pluggable through Register.
package captcha

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/linusxiong/ggboard/internal/plugin"
	"github.com/linusxiong/ggboard/internal/settings"
)

// ErrFailed means the token was missing, invalid or rejected by the provider.
var ErrFailed = errors.New("captcha: verification failed")

// Verifier checks a captcha response token.
type Verifier interface {
	Verify(ctx context.Context, token, remoteIP string) error
}

// Factory builds a verifier from the provider secret.
type Factory func(secret string, client *http.Client) Verifier

var providers = plugin.NewRegistry[Factory]("captcha provider")

// Register adds a provider (call from init).
func Register(name string, f Factory) { providers.Register(name, f) }

// Providers returns the registered provider names.
func Providers() []string { return providers.Names() }

// Provider names.
const (
	None      = "none"
	Turnstile = "turnstile"
	ReCaptcha = "recaptcha"
	HCaptcha  = "hcaptcha"
)

// Default verification endpoints.
const (
	TurnstileURL = "https://challenges.cloudflare.com/turnstile/v0/siteverify"
	ReCaptchaURL = "https://www.google.com/recaptcha/api/siteverify"
	HCaptchaURL  = "https://api.hcaptcha.com/siteverify"
)

// Timeout bounds every verification request.
const Timeout = 10 * time.Second

func init() {
	Register(None, func(string, *http.Client) Verifier { return noneVerifier{} })
	Register(Turnstile, func(secret string, c *http.Client) Verifier {
		return &SiteVerify{Endpoint: TurnstileURL, Secret: secret, Client: c}
	})
	Register(ReCaptcha, func(secret string, c *http.Client) Verifier {
		return &SiteVerify{Endpoint: ReCaptchaURL, Secret: secret, Client: c}
	})
	Register(HCaptcha, func(secret string, c *http.Client) Verifier {
		return &SiteVerify{Endpoint: HCaptchaURL, Secret: secret, Client: c}
	})
}

// New returns the verifier for provider ("" = none).
func New(provider, secret string, client *http.Client) (Verifier, error) {
	if provider == "" {
		provider = None
	}
	f, ok := providers.Get(provider)
	if !ok {
		return nil, fmt.Errorf("captcha: unknown provider %q", provider)
	}
	if client == nil {
		client = &http.Client{Timeout: Timeout}
	}
	return f(secret, client), nil
}

// FromSettings returns the verifier configured in settings (falls back to none on unknown providers).
func FromSettings(st *settings.Service) Verifier {
	v, err := New(st.String("captcha_provider"), st.String("captcha_secret"), nil)
	if err != nil {
		return noneVerifier{}
	}
	return v
}

// Enabled reports whether a captcha provider other than none is configured.
func Enabled(st *settings.Service) bool {
	p := st.String("captcha_provider")
	return p != "" && p != None
}

type noneVerifier struct{}

func (noneVerifier) Verify(context.Context, string, string) error { return nil }

// SiteVerify implements the shared siteverify protocol of Turnstile, reCAPTCHA and hCaptcha:
// POST secret, response, remoteip (form) -> {"success": bool}.
type SiteVerify struct {
	Endpoint string
	Secret   string
	Client   *http.Client
}

// Verify posts the token to the provider.
func (s *SiteVerify) Verify(ctx context.Context, token, remoteIP string) error {
	if strings.TrimSpace(token) == "" {
		return ErrFailed
	}
	if s.Secret == "" {
		return errors.New("captcha: secret is not configured")
	}
	form := url.Values{"secret": {s.Secret}, "response": {token}}
	if remoteIP != "" {
		form.Set("remoteip", remoteIP)
	}
	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.Endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	client := s.Client
	if client == nil {
		client = &http.Client{Timeout: Timeout}
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("captcha: verify request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("captcha: verify status %d", resp.StatusCode)
	}
	var out struct {
		Success bool `json:"success"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&out); err != nil {
		return fmt.Errorf("captcha: decode response: %w", err)
	}
	if !out.Success {
		return ErrFailed
	}
	return nil
}
