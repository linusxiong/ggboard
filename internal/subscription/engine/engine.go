// Package engine generates subscriptions: it loads the user's rule profile and nodes, applies the request's
// filters and overrides, resolves groups and the rule plan for the detected format and renders it. It is
// shared by the client subscription endpoint and the user preview endpoint.
package engine

import (
	"context"
	"crypto/sha1" //nolint:gosec // ETag content hash, not a security boundary (same as node config ETags)
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/settings"
	"github.com/linusxiong/ggboard/internal/subscription"
	"github.com/linusxiong/ggboard/internal/subscription/rulesets"

	// Register every output format.
	_ "github.com/linusxiong/ggboard/internal/subscription/formats"
)

// ProfileNamespace is the a.Cache namespace of stored rule profiles.
const ProfileNamespace = "rule_profile"

// ProfileTTL is how long a stored rule profile is cached (writes invalidate it immediately).
const ProfileTTL = time.Minute

// Request describes one subscription request.
type Request struct {
	Format    string     // format name; "" = detect from Flag and UserAgent
	Flag      string     // ?flag=
	UserAgent string     // User-Agent header
	Query     url.Values // types, filter, emoji, profile, source, cats, final, mirror, list
	Host      string     // request host without port (forced DIRECT in rules)
	SelfURL   string     // absolute URL of this subscription (Surge managed config)
	Now       time.Time  // zero = time.Now()
	Location  *time.Location
}

// Result is a rendered subscription.
type Result struct {
	Body        []byte
	ContentType string
	Format      string
	ETag        string // hex(sha1(body)), unquoted
	Profile     subscription.RuleProfile
	Proxies     int // number of proxies rendered (info nodes included)
}

// MaxQueryList bounds the entries of the types, filter and cats query lists (400 above).
const MaxQueryList = 64

// checkQueryLists rejects oversized query lists before any work: keywords are matched against every proxy name
// and tag (an unbounded list costs seconds of CPU), and every unknown category adds an error line to the answer.
func checkQueryLists(q url.Values) error {
	errs := rulesets.FieldErrors{}
	for _, key := range []string{"types", "filter", "cats"} {
		if len(subscription.ParseList(q.Get(key))) > MaxQueryList {
			errs[key] = []string{fmt.Sprintf("at most %d entries", MaxQueryList)}
		}
	}
	if len(errs) > 0 {
		return errs
	}
	return nil
}

// OverridesFromQuery reads profile, source, cats, final and mirror.
func OverridesFromQuery(q url.Values) rulesets.Overrides {
	ov := rulesets.Overrides{
		Template: strings.TrimSpace(q.Get("profile")),
		Source:   strings.TrimSpace(q.Get("source")),
		Final:    strings.TrimSpace(q.Get("final")),
		Mirror:   strings.TrimSpace(q.Get("mirror")),
	}
	if q.Has("cats") {
		ov.HasCats = true
		ov.Cats = subscription.ParseList(q.Get("cats"))
	}
	return ov
}

func queryBool(q url.Values, key string) (value, ok bool) {
	if !q.Has(key) {
		return false, false
	}
	b, err := strconv.ParseBool(strings.TrimSpace(q.Get(key)))
	return b && err == nil, err == nil
}

func profileNamespace(userID uint64) string {
	return ProfileNamespace + ":" + strconv.FormatUint(userID, 10)
}

// profileKey is versioned by both the shared namespace (InvalidateNamespace(ProfileNamespace) drops every
// profile) and the user's own generation (profile writes).
func profileKey(a *app.App, userID uint64) string {
	return a.Cache.Key(ProfileNamespace, a.Cache.Key(profileNamespace(userID), ""))
}

// Generate renders the subscription of u for req.
func Generate(ctx context.Context, a *app.App, u *model.User, req Request) (*Result, error) {
	if u == nil {
		return nil, errors.New("engine: nil user")
	}
	now := req.Now
	if now.IsZero() {
		now = time.Now()
	}
	loc := req.Location
	if loc == nil {
		loc = time.Local
	}
	q := req.Query
	if q == nil {
		q = url.Values{}
	}
	name := req.Format
	if name == "" {
		name = subscription.DetectFormat(req.Flag, req.UserAgent)
	}
	f, ok := subscription.GetFormat(name)
	if !ok {
		f, _ = subscription.GetFormat(subscription.FormatV2ray)
	}

	if err := checkQueryLists(q); err != nil {
		return nil, err
	}
	cat := rulesets.Current(a.Settings)
	profile, err := EffectiveProfile(ctx, a, cat, u, OverridesFromQuery(q))
	if err != nil {
		return nil, err
	}
	types, keywords := subscription.ParseList(q.Get("types")), subscription.ParseList(q.Get("filter"))

	locale := a.Settings.String("default_locale")
	var proxies []subscription.Proxy
	if a.Settings.Bool("show_info_to_server_enable") {
		proxies = subscription.InfoProxies(u, locale, now, loc)
	}
	nodeByID := map[uint64]*model.Node{}
	if u.IsAvailable(now) {
		nodes, err := subscription.AvailableNodes(ctx, a, u)
		if err != nil {
			return nil, err
		}
		for _, n := range nodes {
			nodeByID[n.Node.ID] = n.Node
		}
		emojiOn, emojiSet := queryBool(q, "emoji")
		opt := subscription.ProxyOptions{
			Types:        types,
			Keywords:     keywords,
			StripEmoji:   emojiSet && !emojiOn,
			ShowProtocol: a.Settings.Bool("show_protocol_to_server_enable"),
			Ops:          &profile.Nodes,
		}
		proxies = append(proxies, subscription.NodeProxies(u, nodes, opt)...)
	}

	listOnly, _ := queryBool(q, "list")
	return Render(f, Input{
		User: u, Proxies: proxies, Nodes: nodeByID, Profile: profile, Catalog: cat,
		Defaults: rulesets.DefaultsFrom(a.Settings), Settings: a.Settings,
		AppName: a.Settings.String("app_name"), Locale: locale, SelfURL: req.SelfURL, Host: req.Host,
		UserAgent: req.UserAgent, ListOnly: listOnly, UpdateHours: a.Settings.Int("subscribe_update_interval"),
		Now: now, Location: loc,
	})
}

// Input is everything Render needs; Generate fills it from the database and settings.
type Input struct {
	User        *model.User
	Proxies     []subscription.Proxy // info + node proxies, before format filtering
	Nodes       map[uint64]*model.Node
	Profile     subscription.RuleProfile // effective profile
	Catalog     *subscription.Catalog
	Defaults    rulesets.Defaults
	Settings    *settings.Service
	AppName     string
	Locale      string
	SelfURL     string
	Host        string
	UserAgent   string
	ListOnly    bool
	UpdateHours int
	Now         time.Time
	Location    *time.Location
}

// Render filters the proxies to what f supports, makes names unique, resolves groups and the rule plan and
// renders f. It does no I/O.
func Render(f subscription.Format, in Input) (*Result, error) {
	filter, _ := f.(subscription.ProxyFilter)
	kept := make([]subscription.Proxy, 0, len(in.Proxies))
	for i := range in.Proxies {
		p := &in.Proxies[i]
		if !f.Supports(p.Type) || (filter != nil && !filter.SupportsProxy(p)) {
			continue
		}
		kept = append(kept, *p)
	}
	if len(in.Profile.Groups) == 0 { // validated profiles always have groups; never render a config without Proxy
		in.Profile.Groups = []subscription.PolicyGroup{{Name: subscription.GroupProxy, Type: subscription.GroupSelect, Include: "*"}}
	}
	reserved := make(map[string]bool, len(in.Profile.Groups))
	for _, g := range in.Profile.Groups {
		reserved[g.Name] = true
	}
	subscription.UniqueNames(kept, reserved)
	var renderNodes []*model.Node
	for i := range kept {
		if n := in.Nodes[kept[i].NodeID]; n != nil && !kept[i].Info {
			renderNodes = append(renderNodes, n)
		}
	}
	family := ""
	if rc, ok := f.(subscription.RuleConsumer); ok {
		family = rc.RuleFamily()
	}
	mirror := rulesets.MirrorPrefix(in.Catalog, in.Profile.Mirror, in.Defaults)
	loc := in.Location
	if loc == nil {
		loc = time.Local
	}
	rc := &subscription.RenderContext{
		User:         in.User,
		Nodes:        renderNodes,
		Proxies:      kept,
		Groups:       subscription.ResolveGroups(in.Profile.Groups, kept),
		Rules:        rulesets.BuildPlan(in.Catalog, in.Profile, family, mirror),
		Profile:      in.Profile,
		Catalog:      in.Catalog,
		Settings:     in.Settings,
		SubscribeURL: in.SelfURL,
		RequestHost:  in.Host,
		AppName:      in.AppName,
		Locale:       in.Locale,
		Mirror:       mirror,
		UserAgent:    in.UserAgent,
		ListOnly:     in.ListOnly,
		UpdateHours:  in.UpdateHours,
		Now:          in.Now,
		Location:     loc,
	}
	body, ct, err := f.Render(rc)
	if err != nil {
		return nil, err
	}
	sum := sha1.Sum(body) //nolint:gosec // content hash
	return &Result{Body: body, ContentType: ct, Format: f.Name(), ETag: hex.EncodeToString(sum[:]), Profile: in.Profile, Proxies: len(kept)}, nil
}
