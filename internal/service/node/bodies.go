package nodesvc

import (
	"context"
	"strconv"

	"github.com/linusxiong/ggboard/internal/cache"
	"github.com/linusxiong/ggboard/internal/model"
	usersvc "github.com/linusxiong/ggboard/internal/service/user"
)

// UniProxyConfig returns the encoded UniProxy config of r.P (cached per P and intervals; node/route writes
// invalidate it through Invalidate).
func (s *Service) UniProxyConfig(ctx context.Context, r *Resolved) (*Body, error) {
	iv := s.Intervals()
	k := s.key("uniproxy", u64(r.P.ID), strconv.Itoa(iv.Push), strconv.Itoa(iv.Pull))
	return cache.GetOrLoad(s.a.Cache, k, CacheTTL, func() (*Body, error) {
		routes, err := s.Routes(ctx, r.P.RouteIDs)
		if err != nil {
			return nil, err
		}
		return NewBody(RenderUniProxyConfig(r.P, routes, iv))
	})
}

// SogaNode returns the encoded soga /node body of r.P (cached like UniProxyConfig).
func (s *Service) SogaNode(_ context.Context, r *Resolved) (*Body, error) {
	iv := s.Intervals()
	k := s.key("soga", u64(r.P.ID), strconv.Itoa(iv.Push), strconv.Itoa(iv.Pull))
	return cache.GetOrLoad(s.a.Cache, k, CacheTTL, func() (*Body, error) {
		if Network(r.P) == "httpupgrade" {
			s.a.Log.Warn("soga has no httpupgrade transport; stream_type sent verbatim", "node_id", r.P.ID)
		}
		return NewBody(RenderSogaNode(r.P, iv))
	})
}

// AuditRules returns the encoded soga /audit_rules body of r.P (cached).
func (s *Service) AuditRules(ctx context.Context, r *Resolved) (*Body, error) {
	return cache.GetOrLoad(s.a.Cache, s.key("audit", u64(r.P.ID)), CacheTTL, func() (*Body, error) {
		routes, err := s.Routes(ctx, r.P.RouteIDs)
		if err != nil {
			return nil, err
		}
		return NewBody(RenderAuditRules(routes))
	})
}

// WhiteList returns the encoded soga /white_list body (settings server_white_list, [] when unset).
func (s *Service) WhiteList() (*Body, error) {
	list := s.a.Settings.Strings("server_white_list")
	if list == nil {
		list = []string{}
	}
	return NewBody(list)
}

// usersBody caches an encoded user list together with the usersvc slice it was built from: usersvc returns the
// same backing array while its 60 s cache entry lives, so an identical slice means an identical body.
type usersBody struct {
	src  []usersvc.NodeUser
	body *Body
}

func sameUsers(a, b []usersvc.NodeUser) bool {
	return len(a) == len(b) && (len(a) == 0 || &a[0] == &b[0])
}

func (s *Service) usersBody(k string, users []usersvc.NodeUser, render func() any) (*Body, error) {
	if c, ok := cache.GetAs[*usersBody](s.a.Cache, k); ok && sameUsers(c.src, users) {
		return c.body, nil
	}
	b, err := NewBody(render())
	if err != nil {
		return nil, err
	}
	s.a.Cache.Set(k, &usersBody{src: users, body: b}, CacheTTL)
	return b, nil
}

// AvailableUsers returns the users P serves: available users in groups(P) ∪ groups(children of P), by id.
// The slice is shared with the usersvc cache: do not mutate it.
func (s *Service) AvailableUsers(ctx context.Context, r *Resolved) ([]usersvc.NodeUser, error) {
	return usersvc.New(s.a).AvailableUsersForGroups(ctx, r.Groups)
}

// UniProxyUsers returns the encoded UniProxy /user body {"users":[{id,uuid,speed_limit,device_limit}]}.
func (s *Service) UniProxyUsers(ctx context.Context, r *Resolved) (*Body, error) {
	users, err := s.AvailableUsers(ctx, r)
	if err != nil {
		return nil, err
	}
	return s.usersBody(s.key("users_uni", u64(r.P.ID)), users, func() any {
		return struct {
			Users []usersvc.NodeUser `json:"users"`
		}{Users: users}
	})
}

// SogaUser is one element of the soga /users array. Exactly one credential shape is filled per node type.
type SogaUser struct {
	ID          uint64 `json:"id"`
	UUID        string `json:"uuid,omitempty"`
	Username    string `json:"username,omitempty"`
	Password    string `json:"password,omitempty"`
	SpeedLimit  int    `json:"speed_limit"`
	DeviceLimit int    `json:"device_limit"`
	TCPLimit    int    `json:"tcp_limit"`
}

// SogaUserList converts node users to the soga shape for node p (docs/NODE_PROTOCOLS.md §3.2).
func SogaUserList(p *model.Node, users []usersvc.NodeUser) []SogaUser {
	out := make([]SogaUser, 0, len(users))
	for _, nu := range users {
		su := SogaUser{ID: nu.ID, SpeedLimit: nu.SpeedLimit, DeviceLimit: nu.DeviceLimit}
		creds := model.UserCredentials(&model.User{ID: nu.ID, UUID: nu.UUID}, p)
		switch p.Type {
		case model.NodeVmess, model.NodeVless:
			su.UUID = creds.UUID
		case model.NodeMieru:
			su.Username, su.Password = creds.MieruUser, creds.MieruPass
		case model.NodeShadowsocks:
			su.Password = creds.UUID
			if creds.SS2022Key != "" {
				su.Password = creds.SS2022Key
			}
		default:
			su.Password = creds.Password
		}
		out = append(out, su)
	}
	return out
}

// SogaUsers returns the encoded soga /users body for r.P.
func (s *Service) SogaUsers(ctx context.Context, r *Resolved) (*Body, error) {
	users, err := s.AvailableUsers(ctx, r)
	if err != nil {
		return nil, err
	}
	return s.usersBody(s.key("users_soga", u64(r.P.ID)), users, func() any { return SogaUserList(r.P, users) })
}

// AliveList returns uid → alive device count for users eligible on P with device_limit > 0 and count > 0
// (UniProxy /alivelist, docs/NODE_PROTOCOLS.md §2.5). Counting follows the device_limit_mode setting.
func (s *Service) AliveList(ctx context.Context, r *Resolved) (map[string]int, error) {
	users, err := s.AvailableUsers(ctx, r)
	if err != nil {
		return nil, err
	}
	counts := s.a.Online.AliveCountsMode(s.a.Settings.Int("device_limit_mode"))
	out := map[string]int{}
	for _, u := range users {
		if u.DeviceLimit <= 0 {
			continue
		}
		if n := counts[u.ID]; n > 0 {
			out[u64(u.ID)] = n
		}
	}
	return out, nil
}
