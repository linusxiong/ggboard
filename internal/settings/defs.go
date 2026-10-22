package settings

// Type is the value type of a setting.
type Type string

// Setting value types.
const (
	TypeString  Type = "string"
	TypeInt     Type = "int"
	TypeBool    Type = "bool"
	TypeStrings Type = "strings" // []string
	TypeJSON    Type = "json"    // arbitrary JSON document
)

// Def describes one setting key.
type Def struct {
	Key     string   `json:"key"`
	Group   string   `json:"group"`
	Type    Type     `json:"type"`
	Default any      `json:"default"`
	Secret  bool     `json:"secret,omitempty"`  // returned to admins only, masked in logs
	Hidden  bool     `json:"hidden,omitempty"`  // managed by a dedicated endpoint; not in AllGrouped
	Options []string `json:"options,omitempty"` // allowed values for strings
	Min     *int     `json:"min,omitempty"`     // inclusive bounds for ints
	Max     *int     `json:"max,omitempty"`
}

// Group is the admin-UI metadata of a settings group.
type Group struct {
	Name string `json:"name"`
	Keys []Def  `json:"keys"`
}

func ip(v int) *int { return &v }

func s(group, key, def string, opts ...string) Def {
	return Def{Key: key, Group: group, Type: TypeString, Default: def, Options: opts}
}
func secret(group, key string) Def {
	return Def{Key: key, Group: group, Type: TypeString, Default: "", Secret: true}
}
func i(group, key string, def, min, max int) Def {
	return Def{Key: key, Group: group, Type: TypeInt, Default: def, Min: ip(min), Max: ip(max)}
}
func b(group, key string, def bool) Def {
	return Def{Key: key, Group: group, Type: TypeBool, Default: def}
}
func ss(group, key string, def ...string) Def {
	if def == nil {
		def = []string{}
	}
	return Def{Key: key, Group: group, Type: TypeStrings, Default: def}
}

const maxInt = 1<<31 - 1

// Group names in display order.
const (
	GroupSite      = "site"
	GroupRegister  = "register"
	GroupSubscribe = "subscribe"
	GroupOrder     = "order"
	GroupInvite    = "invite"
	GroupServer    = "server"
	GroupEmail     = "email"
	GroupFrontend  = "frontend"
	GroupApp       = "app"
	GroupSecurity  = "security"
)

// defs is the authoritative list of settings keys (docs/API.md "Settings keys").
var defs = []Def{
	s(GroupSite, "app_name", "GG Bond"),
	s(GroupSite, "app_description", ""),
	s(GroupSite, "app_url", ""),
	s(GroupSite, "logo", ""),
	s(GroupSite, "tos_url", ""),
	s(GroupSite, "currency", "CNY"),
	s(GroupSite, "currency_symbol", "¥"),
	s(GroupSite, "default_locale", "zh-CN", "zh-CN", "en"),
	b(GroupSite, "show_info_to_user", true),

	b(GroupRegister, "stop_register", false),
	b(GroupRegister, "email_verify", false),
	b(GroupRegister, "email_whitelist_enable", false),
	ss(GroupRegister, "email_whitelist_suffix"),
	b(GroupRegister, "invite_force", false),
	b(GroupRegister, "register_limit_by_ip_enable", false),
	i(GroupRegister, "register_limit_count", 3, 1, maxInt),
	i(GroupRegister, "register_limit_expire", 60, 1, maxInt),
	b(GroupRegister, "password_limit_enable", true),
	i(GroupRegister, "password_limit_count", 5, 1, maxInt),
	i(GroupRegister, "password_limit_expire", 60, 1, maxInt),
	i(GroupRegister, "try_out_plan_id", 0, 0, maxInt),
	i(GroupRegister, "try_out_hour", 0, 0, maxInt),
	s(GroupRegister, "captcha_provider", "none", "none", "turnstile", "recaptcha", "hcaptcha"),
	s(GroupRegister, "captcha_site_key", ""),
	secret(GroupRegister, "captcha_secret"),

	ss(GroupSubscribe, "subscribe_url"),
	b(GroupSubscribe, "plan_change_enable", true),
	i(GroupSubscribe, "reset_traffic_method", 0, 0, 4),
	b(GroupSubscribe, "surplus_enable", true),
	b(GroupSubscribe, "show_info_to_server_enable", true),
	b(GroupSubscribe, "show_protocol_to_server_enable", false),
	b(GroupSubscribe, "default_remind_expire", true),
	b(GroupSubscribe, "default_remind_traffic", true),
	s(GroupSubscribe, "ruleset_default_template", "standard"),
	s(GroupSubscribe, "ruleset_default_source", "blackmatrix7"),
	s(GroupSubscribe, "ruleset_mirror", ""),
	i(GroupSubscribe, "subscribe_update_interval", 24, 1, 720),
	b(GroupSubscribe, "allow_user_rule_profile", true),
	{Key: "rulesets_custom", Group: GroupSubscribe, Type: TypeJSON, Default: map[string]any{}, Hidden: true},

	i(GroupOrder, "order_timeout_minutes", 30, 1, 10080),

	i(GroupInvite, "invite_commission", 10, 0, 100),
	i(GroupInvite, "invite_gen_limit", 5, 0, maxInt),
	b(GroupInvite, "invite_never_expire", true),
	b(GroupInvite, "commission_first_time_enable", true),
	b(GroupInvite, "commission_auto_check_enable", true),
	i(GroupInvite, "commission_withdraw_limit", 100, 0, maxInt),
	ss(GroupInvite, "commission_withdraw_method", "USDT", "Alipay"),
	b(GroupInvite, "withdraw_close_enable", false),
	b(GroupInvite, "commission_distribution_enable", false),
	i(GroupInvite, "commission_distribution_l1", 0, 0, 100),
	i(GroupInvite, "commission_distribution_l2", 0, 0, 100),
	i(GroupInvite, "commission_distribution_l3", 0, 0, 100),

	secret(GroupServer, "server_token"),
	i(GroupServer, "server_pull_interval", 60, 10, 86400),
	i(GroupServer, "server_push_interval", 60, 10, 86400),
	ss(GroupServer, "server_white_list"),
	i(GroupServer, "device_limit_mode", 0, 0, 1),

	s(GroupEmail, "email_driver", "log", "log", "smtp"),
	s(GroupEmail, "email_host", ""),
	i(GroupEmail, "email_port", 465, 1, 65535),
	s(GroupEmail, "email_encryption", "ssl", "ssl", "tls", "none"),
	s(GroupEmail, "email_username", ""),
	secret(GroupEmail, "email_password"),
	s(GroupEmail, "email_from_address", ""),
	s(GroupEmail, "email_from_name", ""),
	s(GroupEmail, "email_template", "default"),
	b(GroupEmail, "remind_mail_enable", true),

	s(GroupFrontend, "frontend_accent", "#007AFF"),
	s(GroupFrontend, "frontend_background_url", ""),
	s(GroupFrontend, "frontend_customer_service_url", ""),
	s(GroupFrontend, "frontend_footer_html", ""),

	s(GroupApp, "windows_download_url", ""),
	s(GroupApp, "windows_version", ""),
	s(GroupApp, "macos_download_url", ""),
	s(GroupApp, "macos_version", ""),
	s(GroupApp, "android_download_url", ""),
	s(GroupApp, "android_version", ""),
	s(GroupApp, "ios_download_url", ""),
	s(GroupApp, "ios_version", ""),

	// Two-factor authentication and passkeys (internal/service/mfa). Empty passkey_rp_id = host of app_url;
	// the app_url origin is always allowed in addition to passkey_origins; empty mfa_issuer = app_name.
	s(GroupSecurity, "passkey_rp_id", ""),
	ss(GroupSecurity, "passkey_origins"),
	b(GroupSecurity, "admin_mfa_required", false),
	s(GroupSecurity, "mfa_issuer", ""),
}

var groupOrder = []string{GroupSite, GroupRegister, GroupSubscribe, GroupOrder, GroupInvite, GroupServer, GroupEmail, GroupFrontend, GroupApp, GroupSecurity}

var defIndex = func() map[string]Def {
	m := make(map[string]Def, len(defs))
	for _, d := range defs {
		if _, dup := m[d.Key]; dup {
			panic("settings: duplicate key " + d.Key)
		}
		m[d.Key] = d
	}
	return m
}()

// Defaults maps every known key to its default value (string, int, bool, []string or map[string]any).
var Defaults = func() map[string]any {
	m := make(map[string]any, len(defs))
	for _, d := range defs {
		m[d.Key] = d.Default
	}
	return m
}()

// Lookup returns the definition of key.
func Lookup(key string) (Def, bool) {
	d, ok := defIndex[key]
	return d, ok
}

// IsSecret reports whether key holds a secret (captcha_secret, email_password, server_token).
func IsSecret(key string) bool { return defIndex[key].Secret }

// Groups returns the admin-UI metadata for every visible key, grouped and ordered.
func Groups() []Group {
	out := make([]Group, 0, len(groupOrder))
	for _, g := range groupOrder {
		grp := Group{Name: g}
		for _, d := range defs {
			if d.Group == g && !d.Hidden {
				grp.Keys = append(grp.Keys, d)
			}
		}
		out = append(out, grp)
	}
	return out
}
