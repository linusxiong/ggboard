package nodesvc

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"math"
	"net"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/linusxiong/ggboard/internal/model"
)

// ValidationError maps JSON field names to messages. httpx renders it as 400 validation_failed (FieldErrors).
type ValidationError map[string][]string

func (e ValidationError) Error() string {
	keys := make([]string, 0, len(e))
	for k := range e {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+": "+strings.Join(e[k], "; "))
	}
	return "validation failed: " + strings.Join(parts, ", ")
}

// FieldErrors implements the httpx field-error interface.
func (e ValidationError) FieldErrors() map[string][]string { return e }

// Add records msg for field.
func (e ValidationError) Add(field, msg string) { e[field] = append(e[field], msg) }

// Err returns e, or nil when it is empty.
func (e ValidationError) Err() error {
	if len(e) == 0 {
		return nil
	}
	return e
}

// Limits.
const (
	MaxRouteMatch = 999 // soga audit ids are route.id×1000 + index
	MaxNodeRate   = 100
	MaxNodeTags   = 32
)

var (
	v2Networks     = []string{"tcp", "ws", "grpc", "http", "h2", "httpupgrade", "xhttp"}
	trojanNetworks = []string{"tcp", "ws", "grpc"}
	ssCiphers      = []string{"aes-128-gcm", "aes-256-gcm", "chacha20-ietf-poly1305", "2022-blake3-aes-128-gcm", "2022-blake3-aes-256-gcm", "2022-blake3-chacha20-poly1305"}
	vlessFlows     = []string{"", "xtls-rprx-vision", "xtls-rprx-vision-udp443"}
	mieruMux       = []string{"", "MULTIPLEXING_OFF", "MULTIPLEXING_LOW", "MULTIPLEXING_MIDDLE", "MULTIPLEXING_HIGH"}
	ssrToken       = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)
)

// ValidPortSpec reports whether s is a client port: "443", a range "20000-30000", or a comma list of both.
func ValidPortSpec(s string) bool {
	if s == "" || len(s) > 32 {
		return false
	}
	for _, part := range strings.Split(s, ",") {
		lo, hi, isRange := strings.Cut(part, "-")
		a, ok := parsePort(lo)
		if !ok {
			return false
		}
		if isRange {
			b, ok := parsePort(hi)
			if !ok || b <= a {
				return false
			}
		}
	}
	return true
}

func parsePort(s string) (int, bool) {
	if s == "" || len(s) > 5 {
		return 0, false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, false
		}
	}
	p, err := strconv.Atoi(s)
	return p, err == nil && p >= 1 && p <= 65535
}

// NormalizeNode trims and de-duplicates the editable fields of n in place.
func NormalizeNode(n *model.Node) {
	n.Type = strings.ToLower(strings.TrimSpace(n.Type))
	n.Name = strings.TrimSpace(n.Name)
	n.Host = strings.TrimSpace(n.Host)
	n.Port = strings.ReplaceAll(strings.TrimSpace(n.Port), " ", "")
	tags := make([]string, 0, len(n.Tags))
	for _, t := range n.Tags {
		if t = strings.TrimSpace(t); t != "" && !slices.Contains(tags, t) {
			tags = append(tags, t)
		}
	}
	n.Tags = tags
	n.GroupIDs = uniqueIDs(n.GroupIDs)
	n.RouteIDs = uniqueIDs(n.RouteIDs)
	if n.ParentID != nil && *n.ParentID == 0 {
		n.ParentID = nil
	}
	n.Token = strings.TrimSpace(n.Token)
	if n.ParentID != nil {
		n.Token = "" // a child authenticates with its parent's token
	}
	if n.ProtocolSettings == nil {
		n.ProtocolSettings = model.JSONMap{}
	}
}

// Node token length bounds: the minimum matches server_token's, the maximum the column.
const (
	MinNodeTokenLen = 16
	MaxNodeTokenLen = 64
)

// ValidNodeToken reports whether t is "" (use server_token) or 16-64 visible ASCII characters: it travels in a query
// string (UniProxy) and a header (soga).
func ValidNodeToken(t string) bool {
	if t == "" {
		return true
	}
	if len(t) < MinNodeTokenLen || len(t) > MaxNodeTokenLen {
		return false
	}
	for i := 0; i < len(t); i++ {
		if t[i] <= 0x20 || t[i] >= 0x7f {
			return false
		}
	}
	return true
}

func uniqueIDs(ids []uint64) []uint64 {
	out := make([]uint64, 0, len(ids))
	for _, id := range ids {
		if id != 0 && !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	return out
}

// CheckNode normalises n and validates it against the database (groups, routes, parent, children) and the
// typed protocol settings. A child without server_port takes its parent's. n.ID is 0 for new nodes.
func (s *Service) CheckNode(ctx context.Context, n *model.Node) error {
	NormalizeNode(n)
	errs := ValidationError{}
	if !model.IsValidNodeType(n.Type) {
		errs.Add("type", "must be one of "+strings.Join(model.NodeTypes(), ", "))
	}
	if n.Name == "" {
		errs.Add("name", "is required")
	} else if utf8.RuneCountInString(n.Name) > 191 {
		errs.Add("name", "must be at most 191 characters")
	}
	if n.Host == "" {
		errs.Add("host", "is required")
	} else if len(n.Host) > 255 || strings.ContainsAny(n.Host, " \t/\\,#?@") || unsafeText(n.Host) {
		errs.Add("host", "must be a host name or IP address")
	}
	if unsafeText(n.Name) {
		errs.Add("name", "must not contain control characters or line breaks")
	}
	if !ValidPortSpec(n.Port) {
		errs.Add("port", `must be a port, a range "20000-30000" or a comma list of both`)
	}
	if math.IsNaN(n.Rate) || math.IsInf(n.Rate, 0) || n.Rate < 0 || n.Rate > MaxNodeRate {
		errs.Add("rate", fmt.Sprintf("must be between 0 and %d", MaxNodeRate))
	}
	if !ValidNodeToken(n.Token) {
		errs.Add("token", fmt.Sprintf("must be empty or %d-%d visible ASCII characters without spaces", MinNodeTokenLen, MaxNodeTokenLen))
	}
	if len(n.Tags) > MaxNodeTags {
		errs.Add("tags", fmt.Sprintf("at most %d tags", MaxNodeTags))
	}
	for _, t := range n.Tags {
		if utf8.RuneCountInString(t) > 64 || unsafeText(t) {
			errs.Add("tags", "each tag must be at most 64 characters without control characters")
			break
		}
	}
	db := s.a.DB.WithContext(ctx)
	if missing, err := s.missingIDs(ctx, &model.ServerGroup{}, n.GroupIDs); err != nil {
		return err
	} else if len(missing) > 0 {
		errs.Add("group_ids", "unknown group ids: "+joinIDs(missing))
	}
	if missing, err := s.missingIDs(ctx, &model.NodeRoute{}, n.RouteIDs); err != nil {
		return err
	} else if len(missing) > 0 {
		errs.Add("route_ids", "unknown route ids: "+joinIDs(missing))
	}
	var children []model.Node
	if n.ID != 0 {
		if err := db.Select("id", "type").Where("parent_id = ?", n.ID).Find(&children).Error; err != nil {
			return err
		}
	}
	var parent *model.Node
	if n.ParentID != nil {
		parent = new(model.Node)
		err := db.Where("id = ?", *n.ParentID).Limit(1).Find(parent).Error
		switch {
		case err != nil:
			return err
		case parent.ID == 0:
			errs.Add("parent_id", "parent node does not exist")
		case parent.ID == n.ID:
			errs.Add("parent_id", "a node cannot be its own parent")
		case parent.ParentID != nil:
			errs.Add("parent_id", "the parent must not be a child node itself")
		case parent.Type != n.Type:
			errs.Add("parent_id", fmt.Sprintf("the parent is %s; a child must have the same type", parent.Type))
		case len(children) > 0:
			errs.Add("parent_id", "a node with child nodes cannot have a parent")
		}
		if parent.ID != 0 && n.ServerPort == 0 {
			n.ServerPort = parent.ServerPort
		}
	}
	for _, c := range children {
		if c.Type != n.Type {
			errs.Add("type", "cannot change the type of a node that has child nodes")
			break
		}
	}
	if n.ServerPort < 1 || n.ServerPort > 65535 {
		errs.Add("server_port", "must be between 1 and 65535")
	}
	if n.ParentID == nil && model.IsValidNodeType(n.Type) {
		ValidateSettings(n.Type, n.ProtocolSettings, errs)
	}
	return errs.Err()
}

func (s *Service) missingIDs(ctx context.Context, table any, ids []uint64) ([]uint64, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var found []uint64
	if err := s.a.DB.WithContext(ctx).Model(table).Where("id IN ?", ids).Pluck("id", &found).Error; err != nil {
		return nil, err
	}
	var missing []uint64
	for _, id := range ids {
		if !slices.Contains(found, id) {
			missing = append(missing, id)
		}
	}
	return missing, nil
}

func joinIDs(ids []uint64) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = strconv.FormatUint(id, 10)
	}
	return strings.Join(parts, ", ")
}

func oneOf(errs ValidationError, field, v string, allowed []string) {
	if !slices.Contains(allowed, v) {
		shown := make([]string, 0, len(allowed))
		for _, a := range allowed {
			if a != "" {
				shown = append(shown, a)
			}
		}
		errs.Add(field, "must be one of "+strings.Join(shown, ", "))
	}
}

// isX25519Key reports whether k is a base64 (std or url, padded or not) encoding of 32 bytes.
func isX25519Key(k string) bool {
	for _, enc := range []*base64.Encoding{base64.RawURLEncoding, base64.URLEncoding, base64.RawStdEncoding, base64.StdEncoding} {
		if b, err := enc.DecodeString(k); err == nil && len(b) == 32 {
			return true
		}
	}
	return false
}

func validateReality(errs ValidationError, prefix string, ts model.TLSSettings, serverName string) {
	if !isX25519Key(ts.PrivateKey) {
		errs.Add(prefix+".private_key", "is required for reality (base64url X25519 key)")
	}
	if !isX25519Key(ts.PublicKey) {
		errs.Add(prefix+".public_key", "is required for reality (base64url X25519 key)")
	}
	if serverName == "" {
		errs.Add(prefix+".server_name", "is required for reality")
	}
	if ts.ShortID != "" {
		if _, err := hex.DecodeString(ts.ShortID); err != nil || len(ts.ShortID) > 16 {
			errs.Add(prefix+".short_id", "must be an even-length hex string of at most 16 characters")
		}
	}
	if d := strings.TrimSpace(ts.Dest); d != "" {
		if h, p, err := net.SplitHostPort(d); err == nil {
			if _, ok := parsePort(p); !ok || h == "" {
				errs.Add(prefix+".dest", `must be "host" or "host:port"`)
			}
		} else if strings.Contains(d, ":") && net.ParseIP(d) == nil {
			errs.Add(prefix+".dest", `must be "host" or "host:port"`)
		}
	}
	if ts.ServerPort != "" {
		if _, ok := parsePort(ts.ServerPort); !ok {
			errs.Add(prefix+".server_port", "must be a port between 1 and 65535")
		}
	}
}

// unsafeText reports strings that would break out of a line in the generated configs (Surge, Loon, Quantumult X
// lines, YAML scalars): control characters (newlines included), line/paragraph separators and noncharacters.
func unsafeText(s string) bool {
	for _, r := range s {
		if unicode.IsControl(r) || unicode.In(r, unicode.Zl, unicode.Zp, unicode.Noncharacter_Code_Point) {
			return true
		}
	}
	return false
}

// unsafeSetting returns the dotted path of the first string inside v that unsafeText rejects ("" when none).
func unsafeSetting(prefix string, v any) string {
	switch x := v.(type) {
	case string:
		if unsafeText(x) {
			return prefix
		}
	case map[string]any:
		for k, e := range x {
			if unsafeText(k) {
				return prefix
			}
			if f := unsafeSetting(prefix+"."+k, e); f != "" {
				return f
			}
		}
	case model.JSONMap:
		return unsafeSetting(prefix, map[string]any(x))
	case []any:
		for i, e := range x {
			if f := unsafeSetting(prefix+"["+strconv.Itoa(i)+"]", e); f != "" {
				return f
			}
		}
	}
	return ""
}

// ValidateSettings checks protocol_settings of a node type with the typed model settings; problems are added to
// errs under "protocol_settings[.field]".
func ValidateSettings(nodeType string, ps model.JSONMap, errs ValidationError) {
	const p = "protocol_settings"
	if f := unsafeSetting(p, ps); f != "" {
		errs.Add(f, "must not contain control characters or line breaks")
		return
	}
	st := model.NewSettingsFor(nodeType)
	if st == nil {
		return
	}
	if err := st.Decode(ps); err != nil {
		errs.Add(p, "invalid protocol settings: "+err.Error())
		return
	}
	switch v := st.(type) {
	case *model.VmessSettings:
		oneOf(errs, p+".network", v.Network, v2Networks)
		if v.TLS != model.TLSNone && v.TLS != model.TLSEnabled {
			errs.Add(p+".tls", "must be 0 or 1")
		}
	case *model.VlessSettings:
		oneOf(errs, p+".network", v.Network, v2Networks)
		oneOf(errs, p+".flow", v.Flow, vlessFlows)
		switch v.TLS {
		case model.TLSNone, model.TLSEnabled:
		case model.TLSReality:
			validateReality(errs, p+".tls_settings", v.TLSSettings, v.TLSSettings.ServerName)
		default:
			errs.Add(p+".tls", "must be 0, 1 or 2")
		}
		if d := strings.TrimSpace(v.Decryption); d != "" && d != "none" {
			seg := strings.Split(d, ".")
			if len(seg) < 4 || seg[0] != "mlkem768x25519plus" || seg[1] == "" || seg[2] == "" || seg[len(seg)-1] == "" {
				errs.Add(p+".decryption", `must be "mlkem768x25519plus.<mode>.<ticket>[.<padding>].<private key>"`)
			}
		}
	case *model.ShadowsocksSettings:
		oneOf(errs, p+".cipher", v.Cipher, ssCiphers)
		oneOf(errs, p+".obfs", v.Obfs, []string{"", "plain", "http", "tls"})
		if size := model.SS2022KeySize(v.Cipher); size > 0 && v.ServerKey != "" {
			if b, err := base64.StdEncoding.DecodeString(v.ServerKey); err != nil || len(b) != size {
				errs.Add(p+".server_key", fmt.Sprintf("must be the base64 encoding of %d bytes", size))
			}
		}
	case *model.ShadowsocksRSettings:
		for field, val := range map[string]string{"cipher": v.Cipher, "protocol": v.Protocol, "obfs": v.Obfs} {
			if !ssrToken.MatchString(val) {
				errs.Add(p+"."+field, "is required")
			}
		}
		oneOf(errs, p+".single_port_type", v.SinglePortType, []string{"", "protocol", "obfs"})
	case *model.TrojanSettings:
		oneOf(errs, p+".network", v.Network, trojanNetworks)
	case *model.HysteriaSettings:
		if v.Version != 1 && v.Version != 2 {
			errs.Add(p+".version", "must be 1 or 2")
		}
		if v.UpMbps < 0 || v.DownMbps < 0 || v.UpMbps > 100000 || v.DownMbps > 100000 {
			errs.Add(p+".up_mbps", "bandwidth must be between 0 and 100000 Mbps")
		}
		oneOf(errs, p+".obfs", v.Obfs, []string{"", "plain", "salamander"})
		if v.Obfs == "salamander" && v.ObfsPassword == "" {
			errs.Add(p+".obfs_password", "is required with salamander")
		}
		if v.HopInterval < 0 {
			errs.Add(p+".hop_interval", "must not be negative")
		}
	case *model.AnyTLSSettings:
		switch v.TLS {
		case model.TLSEnabled:
		case model.TLSReality:
			validateReality(errs, p+".tls_settings", v.TLSSettings, anyTLSRealityName(ps))
		default:
			errs.Add(p+".tls", "must be 1 or 2")
		}
	case *model.MieruSettings:
		oneOf(errs, p+".transport", v.Transport, []string{"TCP", "UDP"})
		oneOf(errs, p+".multiplexing", v.Multiplexing, mieruMux)
	case *model.TuicSettings:
		oneOf(errs, p+".congestion_control", v.CongestionControl, []string{"cubic", "bbr", "new_reno"})
		oneOf(errs, p+".udp_relay_mode", v.UDPRelayMode, []string{"native", "quic"})
	}
}

// NormalizeRoute trims a node route in place (empty match lines are dropped).
func NormalizeRoute(r *model.NodeRoute) {
	r.Remarks = strings.TrimSpace(r.Remarks)
	r.Action = strings.ToLower(strings.TrimSpace(r.Action))
	r.ActionValue = strings.TrimSpace(r.ActionValue)
	match := make([]string, 0, len(r.Match))
	for _, m := range r.Match {
		if m = strings.TrimSpace(m); m != "" {
			match = append(match, m)
		}
	}
	r.Match = match
}

// ValidateRoute checks a (normalised) node route: action block|dns, 1..999 match lines, block patterns that
// compile as one Go regexp when joined with "|" (XrayR panics otherwise), dns routes with a server address.
func ValidateRoute(r *model.NodeRoute) error {
	errs := ValidationError{}
	if utf8.RuneCountInString(r.Remarks) > 191 {
		errs.Add("remarks", "must be at most 191 characters")
	}
	oneOf(errs, "action", r.Action, []string{model.RouteActionBlock, model.RouteActionDNS})
	switch {
	case len(r.Match) == 0:
		errs.Add("match", "at least one entry is required")
	case len(r.Match) > MaxRouteMatch:
		errs.Add("match", fmt.Sprintf("at most %d entries", MaxRouteMatch))
	case r.Action == model.RouteActionBlock:
		if _, err := regexp.Compile(strings.Join(r.Match, "|")); err != nil {
			errs.Add("match", "entries joined with | must form a valid regular expression: "+err.Error())
		}
	}
	if r.Action == model.RouteActionDNS && r.ActionValue == "" && (len(r.Match) == 0 || r.Match[0] != "main") {
		errs.Add("action_value", "a DNS server address is required")
	}
	if len(r.ActionValue) > 255 {
		errs.Add("action_value", "must be at most 255 characters")
	}
	return errs.Err()
}
