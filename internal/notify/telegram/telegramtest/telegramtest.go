// Package telegramtest is a fake Telegram Bot API for tests:
//
//	srv := telegramtest.Enable(t, a)            // token + admin chats set, client pointed at the fake
//	srv.Respond("sendMessage", func(c telegramtest.Call) (int, string) { ... })
//	calls := srv.Calls("sendMessage")
package telegramtest

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/notify/telegram"
)

// Token is the fake bot token used by Enable.
const Token = "123456:TEST-fake-token-for-tests"

// BotUsername is the username answered by getMe.
const BotUsername = "ggbond_test_bot"

// Call is one recorded Bot API request.
type Call struct {
	Method string
	Token  string
	Params map[string]any
}

// Text returns the "text" parameter.
func (c Call) Text() string { s, _ := c.Params["text"].(string); return s }

// ChatID returns the "chat_id" parameter as a string.
func (c Call) ChatID() string {
	switch v := c.Params["chat_id"].(type) {
	case string:
		return v
	case float64:
		return strconv.FormatFloat(v, 'f', 0, 64)
	}
	return ""
}

// Handler answers one call with an HTTP status and a JSON body.
type Handler func(c Call) (int, string)

// Server is the fake Bot API.
type Server struct {
	*httptest.Server
	mu       sync.Mutex
	calls    []Call
	handlers map[string]Handler
}

// OK is a successful answer with result.
func OK(result string) (int, string) { return http.StatusOK, `{"ok":true,"result":` + result + `}` }

// Fail is an error answer.
func Fail(status int, description string) (int, string) {
	b, _ := json.Marshal(map[string]any{"ok": false, "error_code": status, "description": description})
	return status, string(b)
}

// NewServer starts a fake Bot API: getMe answers BotUsername, sendMessage and setWebhook succeed.
func NewServer(t testing.TB) *Server {
	t.Helper()
	s := &Server{handlers: map[string]Handler{}}
	s.handlers["getMe"] = func(Call) (int, string) {
		return OK(`{"id":123456,"is_bot":true,"first_name":"GG Bond","username":"` + BotUsername + `"}`)
	}
	s.handlers["sendMessage"] = func(Call) (int, string) { return OK(`{"message_id":1}`) }
	s.handlers["setWebhook"] = func(Call) (int, string) {
		return http.StatusOK, `{"ok":true,"result":true,"description":"Webhook was set"}`
	}
	s.Server = httptest.NewServer(http.HandlerFunc(s.serve))
	t.Cleanup(s.Close)
	return s
}

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	// /bot<token>/<method>
	path := strings.TrimPrefix(r.URL.Path, "/bot")
	token, method, _ := strings.Cut(path, "/")
	body, _ := io.ReadAll(r.Body)
	c := Call{Method: method, Token: token, Params: map[string]any{}}
	_ = json.Unmarshal(body, &c.Params)
	s.mu.Lock()
	s.calls = append(s.calls, c)
	h := s.handlers[method]
	s.mu.Unlock()
	status, out := Fail(http.StatusNotFound, "Not Found")
	if h != nil {
		status, out = h(c)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, out)
}

// Respond replaces the handler of method.
func (s *Server) Respond(method string, h Handler) {
	s.mu.Lock()
	s.handlers[method] = h
	s.mu.Unlock()
}

// Calls returns the recorded calls of method (every call when method is "").
func (s *Server) Calls(method string) []Call {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []Call
	for _, c := range s.calls {
		if method == "" || c.Method == method {
			out = append(out, c)
		}
	}
	return out
}

// Reset forgets the recorded calls.
func (s *Server) Reset() {
	s.mu.Lock()
	s.calls = nil
	s.mu.Unlock()
}

// Enable points a's Telegram client at a new fake server and saves the bot token plus settings (e.g.
// telegram_admin_chat_ids).
func Enable(t testing.TB, a *app.App, settings ...map[string]any) *Server {
	t.Helper()
	s := NewServer(t)
	telegram.SetAPIBase(a, s.URL)
	values := map[string]any{telegram.KeyToken: Token}
	for _, m := range settings {
		for k, v := range m {
			values[k] = v
		}
	}
	if err := a.Settings.Set(values); err != nil {
		t.Fatalf("telegramtest: settings: %v", err)
	}
	return s
}
