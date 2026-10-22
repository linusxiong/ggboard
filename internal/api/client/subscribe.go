package client

import (
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/httpx"
	"github.com/linusxiong/ggboard/internal/model"
	usersvc "github.com/linusxiong/ggboard/internal/service/user"
	"github.com/linusxiong/ggboard/internal/subscription/engine"
)

// maxTokenLen bounds tokens before they reach the database (ours are 32 hex characters; imported ones are
// similar).
const maxTokenLen = 64

func validToken(t string) bool {
	if t == "" || len(t) > maxTokenLen {
		return false
	}
	for i := 0; i < len(t); i++ {
		c := t[i]
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return false
		}
	}
	return true
}

// subscribe serves a subscription. Invalid tokens get a bare 403; the token never reaches the logs (the short
// link's path is redacted before the access log runs).
func subscribe(a *app.App, tokenOf func(*gin.Context) string, inPath bool) gin.HandlerFunc {
	users := usersvc.New(a)
	return func(c *gin.Context) {
		token := tokenOf(c)
		self := selfURL(c)
		if inPath {
			c.Request.URL.Path = "/s/[redacted]"
			c.Request.URL.RawPath = ""
		}
		if !validToken(token) {
			httpx.Fail(c, httpx.Forbidden("forbidden"))
			return
		}
		u, err := users.FindByToken(c.Request.Context(), token)
		if errors.Is(err, usersvc.ErrNotFound) {
			httpx.Fail(c, httpx.Forbidden("forbidden"))
			return
		}
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		q := c.Request.URL.Query()
		ua := c.GetHeader("User-Agent")
		res, err := engine.Generate(c.Request.Context(), a, u, engine.Request{
			Flag: q.Get("flag"), UserAgent: ua, Query: q, Host: requestHost(c), SelfURL: self,
		})
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		SetHeaders(c.Writer.Header(), a, u, res)
		httpx.Logger(c).Info("subscription served", "user_id", u.ID, "format", res.Format,
			"client", truncate(ua, 80), "proxies", res.Proxies)
		if inm := c.GetHeader("If-None-Match"); inm != "" && strings.Contains(inm, res.ETag) {
			c.Status(http.StatusNotModified)
			return
		}
		c.Data(http.StatusOK, res.ContentType, res.Body)
	}
}

// SetHeaders writes the subscription response headers of docs/SUBSCRIPTION.md.
func SetHeaders(h http.Header, a *app.App, u *model.User, res *engine.Result) {
	var expire int64
	if u.ExpiredAt != nil {
		expire = u.ExpiredAt.Unix()
	}
	h.Set("Subscription-Userinfo", "upload="+strconv.FormatInt(u.U, 10)+"; download="+strconv.FormatInt(u.D, 10)+
		"; total="+strconv.FormatInt(u.TransferEnable, 10)+"; expire="+strconv.FormatInt(expire, 10))
	h.Set("Profile-Update-Interval", strconv.Itoa(a.Settings.Int("subscribe_update_interval")))
	h.Set("Content-Disposition", ContentDisposition(a.Settings.String("app_name")))
	if u := a.Settings.String("app_url"); u != "" {
		h.Set("Profile-Web-Page-Url", u)
	}
	h.Set("ETag", `"`+res.ETag+`"`)
	h.Set("Cache-Control", "no-store")
}

// ContentDisposition returns `attachment; filename="<ASCII fallback>"; filename*=UTF-8”<RFC 5987 encoded>`.
func ContentDisposition(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "subscription"
	}
	var ascii, enc strings.Builder
	for _, r := range name {
		if r >= 0x20 && r < 0x7f && r != '"' && r != '\\' {
			ascii.WriteRune(r)
		} else {
			ascii.WriteByte('_')
		}
	}
	for _, b := range []byte(name) {
		if isAttrChar(b) {
			enc.WriteByte(b)
		} else {
			enc.WriteString("%" + strings.ToUpper(strconv.FormatInt(int64(b)>>4, 16)+strconv.FormatInt(int64(b)&15, 16)))
		}
	}
	return `attachment; filename="` + ascii.String() + `"; filename*=UTF-8''` + enc.String()
}

// isAttrChar reports RFC 5987 attr-char.
func isAttrChar(b byte) bool {
	switch {
	case b >= 'a' && b <= 'z', b >= 'A' && b <= 'Z', b >= '0' && b <= '9':
		return true
	}
	return strings.IndexByte("!#$&+-.^_`|~", b) >= 0
}

// requestHost returns the host the client used, without port.
func requestHost(c *gin.Context) string {
	host := c.Request.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	return strings.ToLower(strings.Trim(host, "[]"))
}

// selfURL is the absolute URL of the request (Surge managed configs refresh from it).
func selfURL(c *gin.Context) string {
	scheme := "http"
	if c.Request.TLS != nil {
		scheme = "https"
	}
	if p := strings.ToLower(c.GetHeader("X-Forwarded-Proto")); p == "https" || p == "http" {
		scheme = p
	}
	if c.Request.Host == "" {
		return ""
	}
	return scheme + "://" + c.Request.Host + c.Request.URL.RequestURI()
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}
