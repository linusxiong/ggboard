package guest

import (
	"github.com/gin-gonic/gin"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/captcha"
	"github.com/linusxiong/ggboard/internal/httpx"
	mfasvc "github.com/linusxiong/ggboard/internal/service/mfa"
	"github.com/linusxiong/ggboard/internal/settings"
)

// Locales are the UI languages shipped by the frontend.
var Locales = []string{"zh-CN", "en"}

// CaptchaConfig is the public captcha configuration (never the secret).
type CaptchaConfig struct {
	Provider string `json:"provider"`
	SiteKey  string `json:"site_key"`
}

// Download is one client download link.
type Download struct {
	URL     string `json:"url"`
	Version string `json:"version"`
}

// Downloads are the client downloads of the "app" settings group.
type Downloads struct {
	Windows Download `json:"windows"`
	MacOS   Download `json:"macos"`
	Android Download `json:"android"`
	IOS     Download `json:"ios"`
}

// Config is the response of GET /guest/config (docs/API.md §Guest).
type Config struct {
	AppName              string        `json:"app_name"`
	AppDescription       string        `json:"app_description"`
	AppURL               string        `json:"app_url"`
	Logo                 string        `json:"logo"`
	TosURL               string        `json:"tos_url"`
	Currency             string        `json:"currency"`
	CurrencySymbol       string        `json:"currency_symbol"`
	RegisterEnabled      bool          `json:"register_enabled"`
	EmailVerify          bool          `json:"email_verify"`
	EmailWhitelistSuffix []string      `json:"email_whitelist_suffix"`
	InviteForce          bool          `json:"invite_force"`
	Captcha              CaptchaConfig `json:"captcha"`
	Downloads            Downloads     `json:"downloads"`
	Locales              []string      `json:"locales"`
	PasskeyEnabled       bool          `json:"passkey_enabled"`         // a WebAuthn RP can be derived (app_url or passkey_rp_id)
	AllowUserRuleProfile bool          `json:"allow_user_rule_profile"` // users may customize their subscription rule profile
}

// PublicConfig builds the public site configuration from settings. email_whitelist_suffix is empty unless
// the whitelist is enabled; the captcha provider is "none" (and site_key empty) unless enabled.
func PublicConfig(st *settings.Service) Config {
	whitelist := []string{}
	if st.Bool("email_whitelist_enable") {
		whitelist = append(whitelist, st.Strings("email_whitelist_suffix")...)
	}
	cc := CaptchaConfig{Provider: captcha.None}
	if captcha.Enabled(st) {
		cc = CaptchaConfig{Provider: st.String("captcha_provider"), SiteKey: st.String("captcha_site_key")}
	}
	dl := func(name string) Download {
		return Download{URL: st.String(name + "_download_url"), Version: st.String(name + "_version")}
	}
	return Config{
		AppName:              st.String("app_name"),
		AppDescription:       st.String("app_description"),
		AppURL:               st.String("app_url"),
		Logo:                 st.String("logo"),
		TosURL:               st.String("tos_url"),
		Currency:             st.String("currency"),
		CurrencySymbol:       st.String("currency_symbol"),
		RegisterEnabled:      !st.Bool("stop_register"),
		EmailVerify:          st.Bool("email_verify"),
		EmailWhitelistSuffix: whitelist,
		InviteForce:          st.Bool("invite_force"),
		Captcha:              cc,
		Downloads:            Downloads{Windows: dl("windows"), MacOS: dl("macos"), Android: dl("android"), IOS: dl("ios")},
		Locales:              append([]string(nil), Locales...),
		PasskeyEnabled:       mfasvc.PasskeyEnabled(st),
		AllowUserRuleProfile: st.Bool("allow_user_rule_profile"),
	}
}

// registerConfig mounts GET /guest/config.
func registerConfig(g *gin.RouterGroup, a *app.App) {
	g.GET("/config", func(c *gin.Context) {
		httpx.OK(c, PublicConfig(a.Settings))
	})
}
