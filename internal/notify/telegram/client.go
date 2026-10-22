package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// DefaultAPIBase is the Bot API endpoint.
const DefaultAPIBase = "https://api.telegram.org"

// RequestTimeout bounds every Bot API request.
const RequestTimeout = 10 * time.Second

// maxResponse bounds the Bot API answers read into memory.
const maxResponse = 1 << 20

var httpClient = &http.Client{
	Timeout: RequestTimeout,
	// The token is part of the URL: never follow a redirect to another host.
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}

// Client calls the Telegram Bot API (https://core.telegram.org/bots/api) with JSON requests. Its errors never
// contain the token.
type Client struct {
	token string
	base  string
	hc    *http.Client
}

// NewClient returns a client for token; base "" means DefaultAPIBase (tests point it at an httptest server).
func NewClient(token, base string) *Client {
	if base = strings.TrimRight(strings.TrimSpace(base), "/"); base == "" {
		base = DefaultAPIBase
	}
	return &Client{token: strings.TrimSpace(token), base: base, hc: httpClient}
}

// Response is a raw Bot API answer.
type Response struct {
	OK          bool            `json:"ok"`
	Result      json.RawMessage `json:"result,omitempty"`
	Description string          `json:"description,omitempty"`
	ErrorCode   int             `json:"error_code,omitempty"`
	Parameters  *struct {
		RetryAfter      int   `json:"retry_after,omitempty"`
		MigrateToChatID int64 `json:"migrate_to_chat_id,omitempty"`
	} `json:"parameters,omitempty"`

	retryAfterHeader time.Duration
}

// APIError is an answer with ok=false.
type APIError struct {
	Method      string
	Code        int // error_code (the HTTP status when Telegram sent none)
	Description string
	After       time.Duration // parameters.retry_after of a 429
}

func (e *APIError) Error() string {
	msg := fmt.Sprintf("telegram %s: %d %s", e.Method, e.Code, e.Description)
	if e.After > 0 {
		msg += fmt.Sprintf(" (retry after %s)", e.After)
	}
	return msg
}

// RetryAfter is how long Telegram asked to wait (429 Too Many Requests); 0 otherwise.
func (e *APIError) RetryAfter() time.Duration { return e.After }

// Temporary reports whether retrying may succeed (429 and 5xx).
func (e *APIError) Temporary() bool { return e.Code == http.StatusTooManyRequests || e.Code >= 500 }

// Raw calls method with params (JSON) and returns the decoded answer whether ok or not. The error is only for
// transport failures and undecodable answers.
func (c *Client) Raw(ctx context.Context, method string, params any) (*Response, error) {
	body := []byte("{}")
	if params != nil {
		var err error
		if body, err = json.Marshal(params); err != nil {
			return nil, fmt.Errorf("telegram %s: encode: %w", method, err)
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/bot"+c.token+"/"+method, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("telegram %s: build request: %w", method, redact(err))
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.hc.Do(req) //nolint:gosec // G704: the Bot API host is fixed (tests use a local server)
	if err != nil {
		return nil, fmt.Errorf("telegram %s: %w", method, redact(err))
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponse))
	if err != nil {
		return nil, fmt.Errorf("telegram %s: read answer: %w", method, redact(err))
	}
	var r Response
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, &APIError{Method: method, Code: resp.StatusCode, Description: "unexpected answer (" + http.StatusText(resp.StatusCode) + ")"}
	}
	if !r.OK && r.ErrorCode == 0 {
		r.ErrorCode = resp.StatusCode
	}
	if s := resp.Header.Get("Retry-After"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			r.retryAfterHeader = time.Duration(n) * time.Second
		}
	}
	return &r, nil
}

// Call calls method and decodes result (may be nil). An answer with ok=false is an *APIError.
func (c *Client) Call(ctx context.Context, method string, params, result any) error {
	r, err := c.Raw(ctx, method, params)
	if err != nil {
		return err
	}
	if !r.OK {
		e := &APIError{Method: method, Code: r.ErrorCode, Description: r.Description}
		if r.Parameters != nil && r.Parameters.RetryAfter > 0 {
			e.After = time.Duration(r.Parameters.RetryAfter) * time.Second
		} else if e.Code == http.StatusTooManyRequests {
			e.After = r.retryAfterHeader
		}
		return e
	}
	if result != nil && len(r.Result) > 0 {
		if err := json.Unmarshal(r.Result, result); err != nil {
			return fmt.Errorf("telegram %s: decode result: %w", method, err)
		}
	}
	return nil
}

// redact strips the request URL (which embeds the token) from transport errors.
func redact(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		return ue.Err
	}
	return err
}

// chatID encodes a chat reference: numeric ids as numbers, "@channel" names as strings.
func chatID(ref string) any {
	ref = strings.TrimSpace(ref)
	if n, err := strconv.ParseInt(ref, 10, 64); err == nil {
		return n
	}
	return ref
}

// SendMessage sends an HTML-formatted message (link previews off) to chat.
func (c *Client) SendMessage(ctx context.Context, chat, html string) error {
	return c.Call(ctx, "sendMessage", map[string]any{
		"chat_id":              chatID(chat),
		"text":                 html,
		"parse_mode":           "HTML",
		"link_preview_options": map[string]bool{"is_disabled": true},
	}, nil)
}

// BotUser is the result of getMe.
type BotUser struct {
	ID        int64  `json:"id"`
	IsBot     bool   `json:"is_bot"`
	FirstName string `json:"first_name"`
	Username  string `json:"username"`
}

// GetMe returns the bot account.
func (c *Client) GetMe(ctx context.Context) (BotUser, error) {
	var u BotUser
	err := c.Call(ctx, "getMe", nil, &u)
	return u, err
}

// SetWebhook registers url (updates: messages only); secretToken is echoed by Telegram in the
// X-Telegram-Bot-Api-Secret-Token header. It returns Telegram's answer.
func (c *Client) SetWebhook(ctx context.Context, url, secretToken string) (*Response, error) {
	params := map[string]any{"url": url, "allowed_updates": []string{"message"}}
	if secretToken != "" {
		params["secret_token"] = secretToken
	}
	return c.Raw(ctx, "setWebhook", params)
}
