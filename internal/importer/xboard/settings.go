package xboardimport

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"slices"
	"sort"
	"strings"

	"gorm.io/gorm/clause"

	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/settings"
)

// Settings translation (§4.13). Values are coerced to the type of the ggboard key and written straight into
// the target transaction (the settings service writes through its own transaction, which would deadlock on
// SQLite); Run reloads the settings snapshot after commit.

var (
	settingStrings = []string{
		"app_name", "app_description", "app_url", "logo", "tos_url", "currency", "currency_symbol",
		"email_host", "email_username", "email_password", "email_from_address", "frontend_background_url",
		"windows_version", "windows_download_url", "macos_version", "macos_download_url",
		"android_version", "android_download_url",
	}
	settingBools = []string{
		"stop_register", "email_verify", "email_whitelist_enable", "invite_force", "register_limit_by_ip_enable",
		"password_limit_enable", "plan_change_enable", "surplus_enable", "show_protocol_to_server_enable",
		"default_remind_expire", "default_remind_traffic", "commission_first_time_enable",
		"commission_auto_check_enable", "withdraw_close_enable", "commission_distribution_enable",
	}
	settingInts = []string{
		"register_limit_count", "register_limit_expire", "password_limit_count", "password_limit_expire",
		"server_pull_interval", "server_push_interval", "device_limit_mode", "email_port",
	}
	// int keys whose fractional values are rounded with a warning; NULL → omitted.
	settingRoundedInts = []string{
		"invite_commission", "invite_gen_limit", "commission_withdraw_limit",
		"commission_distribution_l1", "commission_distribution_l2", "commission_distribution_l3",
	}
	// explicitly dropped keys / prefixes (listed in the summary).
	settingDropped = []string{
		"force_https", "secure_path", "frontend_admin_path", "safe_mode_enable", "email_gmail_limit_enable",
		"email_template", "new_order_event_id", "renew_order_event_id", "change_order_event_id", "try_out_enable",
		"server_ws_enable", "server_ws_url", "ticket_must_wait_reply", "subscribe_path",
	}
	settingDroppedPrefixes = []string{"frontend_theme", "telegram_", "recaptcha_v3_", "subscribe_template_"}

	xboardWhitelist        = []string{"gmail.com", "qq.com", "163.com", "yahoo.com", "sina.com", "126.com", "outlook.com", "yeah.net", "foxmail.com"}
	xboardWithdrawMethods  = []string{"支付宝", "USDT", "Paypal"}
	subscribeURLPatternsRe = regexp.MustCompile(`\*|\[[^\[\]]+-[^\[\]]+\]|\[uuid\]|\{uuid\}`)
)

type settingSource struct {
	vals map[string]*string // lower-cased name → value (nil = SQL NULL); the highest id wins
	used map[string]bool
}

// get returns a present, non-NULL value and marks the key as translated.
func (s *settingSource) get(k string) (string, bool) {
	v, ok := s.vals[k]
	if !ok {
		return "", false
	}
	s.used[k] = true
	if v == nil {
		return "", false
	}
	return *v, true
}

// present reports whether the key exists (even NULL) and marks it as translated.
func (s *settingSource) present(k string) bool {
	_, ok := s.vals[k]
	if ok {
		s.used[k] = true
	}
	return ok
}

func (im *importer) importSettings() error {
	stat := im.rep.Table("settings")
	src := &settingSource{vals: map[string]*string{}, used: map[string]bool{}}
	err := im.src.each(im.ctx, "v2_settings", "", nil, func(r row) error {
		name := strings.ToLower(strings.TrimSpace(r.str("name")))
		if name == "" {
			return nil
		}
		if v, ok := flexStr(r["value"]); ok {
			src.vals[name] = &v
		} else {
			src.vals[name] = nil
		}
		return nil
	})
	if err != nil {
		return err
	}
	stat.Source = len(src.vals)
	out := im.translateSettings(src)
	im.validateSettings(out)

	// ignored / dropped keys
	var dropped, ignored []string
	for k := range src.vals {
		if src.used[k] {
			continue
		}
		if isDroppedSetting(k) {
			dropped = append(dropped, k)
		} else {
			ignored = append(ignored, k)
		}
	}
	sort.Strings(dropped)
	sort.Strings(ignored)
	if v, ok := src.vals["subscribe_path"]; ok && v != nil && strings.Trim(*v, "/ ") != "" && strings.Trim(*v, "/ ") != "s" {
		im.warn("settings", "subscribe_path %q is not supported: ggboard's short link is fixed at /s/<token>", *v)
	}
	if len(dropped) > 0 {
		im.warn("settings", "dropped (not supported by ggboard): %s", strings.Join(dropped, ", "))
	}
	if len(ignored) > 0 {
		im.warn("settings", "ignored settings: %s", strings.Join(ignored, ", "))
	}
	translated := 0
	for k := range src.vals {
		if src.used[k] {
			translated++
		}
	}
	stat.Imported = translated
	stat.Skipped = stat.Source - translated
	if len(dropped)+len(ignored) > 0 {
		all := append(append([]string{}, dropped...), ignored...)
		sort.Strings(all)
		if len(all) > 3 {
			all = append(all[:3], "…")
		}
		stat.Note = "ignored: " + strings.Join(all, ", ")
	}

	// effective values used by later steps
	im.resetMethod = model.ResetMethod(im.a.Settings.Int("reset_traffic_method"))
	if v, ok := out["reset_traffic_method"].(int); ok {
		im.resetMethod = model.ResetMethod(v)
	}
	im.currency = im.a.Settings.String("currency")
	if v, ok := out["currency"].(string); ok && v != "" {
		im.currency = v
	}
	im.appURL = im.a.Settings.String("app_url")
	if v, ok := out["app_url"].(string); ok && v != "" {
		im.appURL = v
	}
	_, im.rep.ServerTokenKept = out["server_token"]

	keys := make([]string, 0, len(out))
	for k := range out {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	rows := make([]model.Setting, 0, len(keys))
	for _, k := range keys {
		b, err := json.Marshal(out[k])
		if err != nil {
			return fmt.Errorf("encode setting %s: %w", k, err)
		}
		rows = append(rows, model.Setting{Key: k, Value: string(b), UpdatedAt: im.start})
	}
	if len(rows) == 0 {
		return nil
	}
	return im.tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value", "updated_at"}),
	}).CreateInBatches(&rows, batchSize).Error
}

func isDroppedSetting(k string) bool {
	if slices.Contains(settingDropped, k) {
		return true
	}
	for _, p := range settingDroppedPrefixes {
		if strings.HasPrefix(k, p) {
			return true
		}
	}
	return false
}

// translateSettings applies the table of §4.13 and returns ggboard key → typed value.
func (im *importer) translateSettings(src *settingSource) map[string]any {
	out := map[string]any{}
	for _, k := range settingStrings {
		if v, ok := src.get(k); ok {
			out[k] = strings.TrimSpace(v)
		}
	}
	for _, k := range settingBools {
		if v, ok := src.get(k); ok {
			out[k] = flexBool(v)
		}
	}
	for _, k := range settingInts {
		if v, ok := src.get(k); ok {
			if n, ok := flexInt(v); ok {
				out[k] = int(n)
			} else {
				im.warn("settings", "%s: %q is not a number; kept ggboard's default", k, v)
			}
		}
	}
	for _, k := range settingRoundedInts {
		if v, ok := src.get(k); ok {
			f, ok := flexFloat(v)
			if !ok {
				im.warn("settings", "%s: %q is not a number; kept ggboard's default", k, v)
				continue
			}
			if f != math.Trunc(f) {
				im.warn("settings", "%s: %v rounded to %d", k, f, int(math.Round(f)))
			}
			out[k] = int(math.Round(f))
		}
	}

	// try-out
	if v, ok := src.get("try_out_plan_id"); ok {
		n, _ := flexInt(v)
		if n != 0 && im.plans[uint64(max(n, 0))] == nil {
			im.warn("settings", "try_out_plan_id %d is not an imported plan → 0 (trial disabled)", n)
			n = 0
		}
		out["try_out_plan_id"] = int(n)
	}
	if v, ok := src.get("try_out_hour"); ok {
		if n, ok := flexInt(v); ok {
			out["try_out_hour"] = int(n)
		}
	} else {
		out["try_out_hour"] = 1 // Xboard's default
	}

	// lists
	if v, ok := src.get("email_whitelist_suffix"); ok {
		list, _, err := parseStringList(v)
		if err != nil {
			im.warn("settings", "email_whitelist_suffix: %v; kept ggboard's default", err)
		} else {
			out["email_whitelist_suffix"] = list
		}
	} else if b, _ := out["email_whitelist_enable"].(bool); b {
		out["email_whitelist_suffix"] = append([]string(nil), xboardWhitelist...)
	}
	if v, ok := src.get("subscribe_url"); ok {
		list, _, err := parseStringList(v)
		if err != nil {
			list = splitStringText(v)
		}
		urls := []string{}
		for _, u := range list {
			u = strings.TrimSpace(u)
			if u == "" {
				continue
			}
			if subscribeURLPatternsRe.MatchString(u) {
				im.warn("settings", "subscribe_url entry %q uses an Xboard pattern (wildcard, range or uuid) — dropped", u)
				continue
			}
			urls = append(urls, u)
		}
		out["subscribe_url"] = urls
	}
	if v, ok := src.get("commission_withdraw_method"); ok {
		list, _, err := parseStringList(v)
		if err != nil {
			im.warn("settings", "commission_withdraw_method: %v; kept ggboard's default", err)
		} else {
			out["commission_withdraw_method"] = list
		}
	} else {
		out["commission_withdraw_method"] = append([]string(nil), xboardWithdrawMethods...)
	}

	// defaults that differ between Xboard and ggboard
	if v, ok := src.get("show_info_to_server_enable"); ok {
		out["show_info_to_server_enable"] = flexBool(v)
	} else {
		out["show_info_to_server_enable"] = false
	}
	if v, ok := src.get("invite_never_expire"); ok {
		out["invite_never_expire"] = flexBool(v)
	} else {
		out["invite_never_expire"] = false
	}
	if v, ok := src.get("reset_traffic_method"); ok {
		n, ok := flexInt(v)
		if ok && n >= 0 && n <= 4 {
			out["reset_traffic_method"] = int(n)
		} else {
			im.warn("settings", "reset_traffic_method %q is not 0..4; kept ggboard's default", v)
		}
	} else {
		def := 0
		if im.flavor == FlavorUpstream {
			def = 1 // TrafficResetService falls back to RESET_TRAFFIC_MONTHLY although the admin form shows 0
		}
		out["reset_traffic_method"] = def
		im.warn("settings", "reset_traffic_method not set in Xboard → %d (the method Xboard %s actually applied)", def, im.flavor)
	}
	if im.flavor == FlavorUpstream || src.present("remind_mail_enable") {
		if v, ok := src.get("remind_mail_enable"); ok {
			out["remind_mail_enable"] = flexBool(v)
		} else {
			out["remind_mail_enable"] = false
		}
	}

	// server token
	if v, ok := src.get("server_token"); ok && strings.TrimSpace(v) != "" {
		out["server_token"] = strings.TrimSpace(v)
	} else {
		im.warn("settings", "server_token is empty in Xboard: kept ggboard's generated token — reconfigure every backend")
	}

	// mail
	if h, _ := out["email_host"].(string); h != "" {
		out["email_driver"] = "smtp"
	}
	if src.present("email_encryption") {
		v, _ := src.get("email_encryption")
		switch e := strings.ToLower(strings.TrimSpace(v)); e {
		case "ssl", "tls":
			out["email_encryption"] = e
		case "", "null", "none":
			out["email_encryption"] = "none"
		default:
			im.warn("settings", "email_encryption %q is not ssl/tls; kept ggboard's default", v)
		}
	}

	im.translateCaptcha(src, out)
	return out
}

// translateCaptcha maps L recaptcha_* and U captcha_* keys to captcha_provider/site_key/secret.
func (im *importer) translateCaptcha(src *settingSource, out map[string]any) {
	enabled, typ := false, "recaptcha"
	if src.present("captcha_enable") {
		v, _ := src.get("captcha_enable")
		enabled = flexBool(v)
		if t, ok := src.get("captcha_type"); ok && strings.TrimSpace(t) != "" {
			typ = strings.ToLower(strings.TrimSpace(t))
		}
	} else if src.present("recaptcha_enable") {
		v, _ := src.get("recaptcha_enable")
		enabled = flexBool(v)
	}
	src.present("captcha_type")
	recSite, _ := src.get("recaptcha_site_key")
	recKey, _ := src.get("recaptcha_key")
	tsSite, _ := src.get("turnstile_site_key")
	tsKey, _ := src.get("turnstile_secret_key")
	switch {
	case !enabled:
		out["captcha_provider"] = "none"
	case typ == "recaptcha":
		out["captcha_provider"] = "recaptcha"
		out["captcha_site_key"] = recSite
		out["captcha_secret"] = recKey
	case typ == "turnstile":
		out["captcha_provider"] = "turnstile"
		out["captcha_site_key"] = tsSite
		out["captcha_secret"] = tsKey
	default:
		out["captcha_provider"] = "none"
		im.warn("settings", "captcha type %q is not supported (reCAPTCHA v3) → captcha disabled; configure turnstile, recaptcha or hcaptcha", typ)
	}
}

// validateSettings drops values ggboard would reject (type, range, enum), with a warning each.
func (im *importer) validateSettings(out map[string]any) {
	keys := make([]string, 0, len(out))
	for k := range out {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		d, ok := settings.Lookup(k)
		if !ok {
			delete(out, k)
			continue
		}
		v := out[k]
		msg := ""
		switch d.Type {
		case settings.TypeInt:
			n, isInt := v.(int)
			switch {
			case !isInt:
				msg = "not an integer"
			case d.Min != nil && n < *d.Min:
				msg = fmt.Sprintf("%d is below the minimum %d", n, *d.Min)
			case d.Max != nil && n > *d.Max:
				msg = fmt.Sprintf("%d is above the maximum %d", n, *d.Max)
			}
		case settings.TypeString:
			s, isStr := v.(string)
			if !isStr {
				msg = "not a string"
			} else if len(d.Options) > 0 && !slices.Contains(d.Options, s) {
				msg = fmt.Sprintf("%q is not one of %s", s, strings.Join(d.Options, ", "))
			}
		case settings.TypeBool:
			if _, isBool := v.(bool); !isBool {
				msg = "not a boolean"
			}
		case settings.TypeStrings:
			if _, isList := v.([]string); !isList {
				msg = "not a list"
			}
		}
		if msg != "" {
			im.warn("settings", "%s: %s; kept ggboard's default", k, msg)
			delete(out, k)
		}
	}
}
