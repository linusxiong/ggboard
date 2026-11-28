package httpx

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestParseLocale(t *testing.T) {
	for header, want := range map[string]string{
		"":                       LocaleEN,
		"en":                     LocaleEN,
		"en-US,zh-CN;q=0.9":      LocaleEN,
		"zh-CN":                  LocaleZH,
		"zh":                     LocaleZH,
		"ZH-tw":                  LocaleZH,
		"zh-Hans-CN,en;q=0.8":    LocaleZH,
		" zh_CN ;q=1, en;q=0.5 ": LocaleZH,
		"zhx":                    LocaleEN,
	} {
		if got := ParseLocale(header); got != want {
			t.Errorf("ParseLocale(%q) = %q, want %q", header, got, want)
		}
	}
}

func TestTranslate(t *testing.T) {
	cases := map[string]string{
		"you already have the maximum number of unused invite codes": "未使用的邀请码已达上限",
		"must be at most 64 characters":                              "最多 64 个字符",
		"must be at most 3 items":                                    "最多 3 项",
		"must be at most 12":                                         "不能大于 12",
		"must be 0":                                                  "必须为 0",
		"must be one of: 0, 1, 3":                                    "必须为以下之一：0, 1, 3",
		"too many failed password attempts, try again in 15 minutes": "密码错误次数过多，请 15 分钟后再试",
		"group is still used by 3 users, 0 plans and 12 nodes":       "该权限组仍被 3 个用户、0 个套餐和 12 个节点使用",
		"epay: config \"pid\" is required":                           "epay：必须填写配置项 \"pid\"",
		// A captured piece that is itself a message is translated too.
		"invalid catalog: is required": "规则集目录无效：必填",
		// Unknown messages and other locales stay as they are.
		"something nobody wrote down": "something nobody wrote down",
		"":                            "",
	}
	for in, want := range cases {
		if got := Translate(LocaleZH, in); got != want {
			t.Errorf("Translate(%q) = %q, want %q", in, got, want)
		}
	}
	if got := Translate(LocaleEN, "is required"); got != "is required" {
		t.Errorf("en changed the message: %q", got)
	}
	// "{n}" only takes numbers: a word does not turn "must be a string" into a half translation.
	if got := Translate(LocaleZH, "must be blue"); got != "must be blue" {
		t.Errorf("{n} matched a word: %q", got)
	}
}

func TestCatalogPatternsAreWellFormed(t *testing.T) {
	for key, tr := range messagesZH {
		if strings.TrimSpace(tr) == "" {
			t.Errorf("%q has an empty translation", key)
		}
		if got, want := strings.Count(tr, "{}"), len(placeholderRe.FindAllString(key, -1)); got != want {
			t.Errorf("%q: translation has %d {} marks, key has %d placeholders", key, got, want)
		}
	}
}

func TestLocalizedCopiesTheError(t *testing.T) {
	shared := Conflict("you have an unpaid order; pay or cancel it first").WithField("trade_no", "2026112812345").
		WithField("plan_id", "is required")
	got := shared.Localized(LocaleZH)
	if got.Message != "你有一个未支付的订单，请先支付或取消" || got.Errors["plan_id"][0] != "必填" ||
		got.Errors["trade_no"][0] != "2026112812345" || got.Status != http.StatusConflict || got.Code != CodeConflict {
		t.Fatalf("localized = %+v", got)
	}
	if shared.Message != "you have an unpaid order; pay or cancel it first" || shared.Errors["plan_id"][0] != "is required" {
		t.Fatalf("the shared error was changed: %+v", shared)
	}
	if shared.Localized(LocaleEN) != shared {
		t.Fatal("en must return the error itself")
	}
}

func TestFailUsesTheRequestLanguage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/x", func(c *gin.Context) {
		var in struct {
			Name string `json:"name" binding:"required,max=4"`
		}
		if err := Bind(c, &in); err != nil {
			Fail(c, err)
			return
		}
		Fail(c, BadRequest("you already have the maximum number of unused invite codes"))
	})
	call := func(lang, body string) Error {
		req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if lang != "" {
			req.Header.Set("Accept-Language", lang)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		var e Error
		if err := json.Unmarshal(w.Body.Bytes(), &e); err != nil {
			t.Fatal(err)
		}
		return e
	}
	if e := call("zh-CN", `{"name":"ok"}`); e.Message != "未使用的邀请码已达上限" {
		t.Fatalf("zh-CN message = %q", e.Message)
	}
	if e := call("en", `{"name":"ok"}`); e.Message != "you already have the maximum number of unused invite codes" {
		t.Fatalf("en message = %q", e.Message)
	}
	if e := call("", `{"name":"ok"}`); !strings.HasPrefix(e.Message, "you already") {
		t.Fatalf("no header message = %q", e.Message)
	}
	e := call("zh-CN", `{"name":"toolong"}`)
	if e.Message != "请检查填写的内容" || e.Errors["name"][0] != "最多 4 个字符" {
		t.Fatalf("zh-CN validation = %+v", e)
	}
	if e := call("zh-CN", `{}`); e.Errors["name"][0] != "必填" {
		t.Fatalf("zh-CN required = %+v", e)
	}
}
