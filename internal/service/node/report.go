package nodesvc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/linusxiong/ggboard/internal/app/online"
	"github.com/linusxiong/ggboard/internal/model"
)

// ErrBadReport is returned for report bodies that are not valid JSON of the expected top-level shape.
var ErrBadReport = errors.New("nodesvc: invalid report body")

// Traffic is the raw upload/download of one user in one report.
type Traffic struct{ Up, Down int64 }

// MaxReportCount is the largest value accepted for one counter in one report (1 PiB). Larger values cannot be
// real traffic since the last push; accepting them lets a single report overflow the int64 usage counters.
const MaxReportCount = 1 << 50

// parseCount parses a JSON number token as a non-negative int64 (fractions truncated). Strings, negatives,
// non-finite and out-of-range (> MaxReportCount) values are rejected.
func parseCount(raw json.RawMessage) (int64, bool) {
	s := strings.TrimSpace(string(raw))
	if s == "" || (s[0] != '-' && (s[0] < '0' || s[0] > '9')) {
		return 0, false
	}
	if i, err := strconv.ParseInt(s, 10, 64); err == nil {
		return i, i >= 0 && i <= MaxReportCount
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) || f < 0 || f > MaxReportCount {
		return 0, false
	}
	return int64(f), true
}

// ParseUserID parses a decimal user or node id: > 0 and ≤ MaxInt64 (ids are signed BIGINT in every dialect, and
// database/sql cannot bind a uint64 with the high bit set, which would fail the whole traffic flush).
func ParseUserID(s string) (uint64, bool) {
	id, err := strconv.ParseUint(strings.TrimSpace(s), 10, 63)
	return id, err == nil && id > 0
}

// parseID parses a JSON number token as a user id (> 0, ≤ MaxInt64, integral).
func parseID(raw json.RawMessage) (uint64, bool) { return ParseUserID(string(raw)) }

// topLevel returns the first non-space byte of body (0 when empty).
func topLevel(body []byte) byte {
	b := bytes.TrimSpace(body)
	if len(b) == 0 {
		return 0
	}
	return b[0]
}

func isEmptyArray(body []byte) bool {
	b := bytes.TrimSpace(body)
	if len(b) < 2 || b[0] != '[' {
		return false
	}
	return len(bytes.TrimSpace(b[1:len(b)-1])) == 0 && b[len(b)-1] == ']'
}

func isNull(body []byte) bool { return string(bytes.TrimSpace(body)) == "null" }

// ParseUniProxyPush parses a UniProxy push body {"<uid>":[up,down]} (the literal [] is empty). Entries whose
// key is not a uint64 > 0 or whose value is not a 2-element array of finite numbers ≥ 0 are dropped.
func ParseUniProxyPush(body []byte) (map[uint64]Traffic, error) {
	out := map[uint64]Traffic{}
	if isEmptyArray(body) {
		return out, nil
	}
	if topLevel(body) != '{' {
		return nil, ErrBadReport
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, ErrBadReport
	}
	for k, v := range m {
		uid, ok := ParseUserID(k)
		if !ok {
			continue
		}
		var pair []json.RawMessage
		if json.Unmarshal(v, &pair) != nil || len(pair) != 2 {
			continue
		}
		up, ok1 := parseCount(pair[0])
		down, ok2 := parseCount(pair[1])
		if !ok1 || !ok2 {
			continue
		}
		out[uid] = Traffic{Up: up, Down: down}
	}
	return out, nil
}

// ParseSogaTraffic parses a soga traffic body [{"id":uid,"u":bytes,"d":bytes}] (null and [] are empty).
// Entries with id ≤ 0 or invalid/negative values are dropped; duplicate ids are summed.
func ParseSogaTraffic(body []byte) (map[uint64]Traffic, error) {
	out := map[uint64]Traffic{}
	items, err := sogaArray(body)
	if err != nil {
		return nil, err
	}
	for _, raw := range items {
		var e struct {
			ID json.RawMessage `json:"id"`
			U  json.RawMessage `json:"u"`
			D  json.RawMessage `json:"d"`
		}
		if json.Unmarshal(raw, &e) != nil {
			continue
		}
		uid, ok := parseID(e.ID)
		if !ok {
			continue
		}
		up, down := int64(0), int64(0)
		if e.U != nil {
			if up, ok = parseCount(e.U); !ok {
				continue
			}
		}
		if e.D != nil {
			if down, ok = parseCount(e.D); !ok {
				continue
			}
		}
		t := out[uid]
		t.Up = satAdd(t.Up, up)
		t.Down = satAdd(t.Down, down)
		out[uid] = t
	}
	return out, nil
}

func satAdd(a, b int64) int64 {
	if a > math.MaxInt64-b {
		return math.MaxInt64
	}
	return a + b
}

// sogaArray decodes a soga POST body that must be a JSON array (null and [] are empty).
func sogaArray(body []byte) ([]json.RawMessage, error) {
	if isNull(body) {
		return nil, nil
	}
	if topLevel(body) != '[' {
		return nil, ErrBadReport
	}
	var items []json.RawMessage
	if err := json.Unmarshal(body, &items); err != nil {
		return nil, ErrBadReport
	}
	return items, nil
}

// NormalizeIP strips a port ("1.2.3.4:5", "[2001:db8::1]:443") and brackets from a reported address.
func NormalizeIP(s string) string {
	s = strings.TrimSpace(s)
	if host, _, err := net.SplitHostPort(s); err == nil {
		return host
	}
	return strings.TrimSuffix(strings.TrimPrefix(s, "["), "]")
}

// Bounds on alive reports: an address longer than MaxAliveIPLen is not an IP (the longest IPv6 text form with a
// zone is well below it), and one user cannot be online from more than MaxAliveIPsPerUser addresses on one node.
// Both keep a misbehaving node from growing the in-memory online store without limit.
const (
	MaxAliveIPLen      = 64
	MaxAliveIPsPerUser = 256
)

func normalizeIPs(ips []string) []string {
	seen := make(map[string]struct{}, min(len(ips), MaxAliveIPsPerUser))
	out := make([]string, 0, min(len(ips), MaxAliveIPsPerUser))
	for _, raw := range ips {
		if len(out) == MaxAliveIPsPerUser {
			break
		}
		ip := NormalizeIP(raw)
		if ip == "" || len(ip) > MaxAliveIPLen {
			continue
		}
		if _, dup := seen[ip]; !dup {
			seen[ip] = struct{}{}
			out = append(out, ip)
		}
	}
	return out
}

// stringsOf decodes a JSON array of strings, skipping non-string entries.
func stringsOf(raw json.RawMessage) []string {
	var items []json.RawMessage
	if json.Unmarshal(raw, &items) != nil {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, it := range items {
		var s string
		if json.Unmarshal(it, &s) == nil {
			out = append(out, s)
		}
	}
	return out
}

// ParseUniProxyAlive parses {"<uid>":["ip",...]} (the literal [] is empty). IPs are normalised and deduplicated.
func ParseUniProxyAlive(body []byte) (map[uint64][]string, error) {
	out := map[uint64][]string{}
	if isEmptyArray(body) {
		return out, nil
	}
	if topLevel(body) != '{' {
		return nil, ErrBadReport
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, ErrBadReport
	}
	for k, v := range m {
		uid, ok := ParseUserID(k)
		if !ok {
			continue
		}
		out[uid] = normalizeIPs(stringsOf(v))
	}
	return out, nil
}

// ParseSogaAlive parses [{"id":uid,"ips":["ip"]}] (null and [] are empty). Duplicate ids are merged.
func ParseSogaAlive(body []byte) (map[uint64][]string, error) {
	out := map[uint64][]string{}
	items, err := sogaArray(body)
	if err != nil {
		return nil, err
	}
	for _, raw := range items {
		var e struct {
			ID  json.RawMessage `json:"id"`
			IPs json.RawMessage `json:"ips"`
		}
		if json.Unmarshal(raw, &e) != nil {
			continue
		}
		uid, ok := parseID(e.ID)
		if !ok {
			continue
		}
		out[uid] = normalizeIPs(append(out[uid], stringsOf(e.IPs)...))
	}
	return out, nil
}

// StatusError reports a missing or invalid field of a status body.
type StatusError struct{ Field string }

func (e *StatusError) Error() string { return "invalid status field: " + e.Field }

// ParseStatus parses a node status body {"cpu":float,"mem":{"total","used"},"swap":{…},"disk":{…},"uptime":s}
// into an online.Load (mem/swap/disk as used/total percentages). With strict (UniProxy), cpu, mem, swap and disk
// are required; soga bodies may omit fields (treated as 0).
func ParseStatus(body []byte, strict bool) (online.Load, error) {
	if topLevel(body) != '{' {
		return online.Load{}, ErrBadReport
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(body, &m); err != nil {
		return online.Load{}, ErrBadReport
	}
	var load online.Load
	if raw, ok := m["cpu"]; ok && !isNull(raw) {
		var cpu float64
		if json.Unmarshal(raw, &cpu) != nil || math.IsNaN(cpu) || cpu < 0 {
			return load, &StatusError{Field: "cpu"}
		}
		load.CPU = min(cpu, 100)
	} else if strict {
		return load, &StatusError{Field: "cpu"}
	}
	for _, f := range []struct {
		name string
		dst  *float64
	}{{"mem", &load.Mem}, {"swap", &load.Swap}, {"disk", &load.Disk}} {
		raw, ok := m[f.name]
		if !ok || isNull(raw) {
			if strict {
				return load, &StatusError{Field: f.name}
			}
			continue
		}
		var u struct {
			Total *float64 `json:"total"`
			Used  *float64 `json:"used"`
		}
		if json.Unmarshal(raw, &u) != nil || (strict && (u.Total == nil || u.Used == nil)) {
			return load, &StatusError{Field: f.name}
		}
		if u.Total != nil && u.Used != nil {
			if *u.Total < 0 || *u.Used < 0 {
				return load, &StatusError{Field: f.name}
			}
			if *u.Total > 0 {
				*f.dst = math.Round(min(*u.Used / *u.Total * 100, 100)*100) / 100
			}
		}
	}
	if raw, ok := m["uptime"]; ok && !isNull(raw) {
		up, ok := parseCount(raw)
		if !ok {
			return load, &StatusError{Field: "uptime"}
		}
		load.Uptime = up
	}
	return load, nil
}

// RecordTraffic charges a report to P (docs/NODE_PROTOCOLS.md §2.3/§5.3): raw bytes go to the traffic
// accumulator at P.rate, LastPushAt(P) = now and Online(P) = number of kept entries (also for empty reports).
func (s *Service) RecordTraffic(r *Resolved, entries map[uint64]Traffic) {
	for uid, t := range entries {
		s.a.Traffic.Add(r.P.ID, uid, r.P.Rate, t.Up, t.Down)
	}
	s.a.Online.TouchPush(r.P.ID)
	s.a.Online.SetOnline(r.P.ID, len(entries))
}

// RecordAlive replaces the alive IPs of every reported user on P; users not in the report keep their entries
// until the alive TTL expires (docs/NODE_PROTOCOLS.md §2.4).
func (s *Service) RecordAlive(r *Resolved, alive map[uint64][]string) {
	for uid, ips := range alive {
		s.a.Online.SetAliveIPs(r.P.ID, uid, ips)
	}
}

// RecordLoad stores the machine load reported by P.
func (s *Service) RecordLoad(r *Resolved, load online.Load) { s.a.Online.SetLoad(r.P.ID, load) }

// AuditHit is one soga audit-rule hit kept in memory for the admin node view.
type AuditHit struct {
	At      time.Time `json:"at"`
	NodeID  uint64    `json:"node_id"`
	UserID  uint64    `json:"user_id"`
	AuditID uint64    `json:"audit_id"`
	RouteID uint64    `json:"route_id"`
	Rule    string    `json:"rule"`
}

// AuditRingSize is the number of audit hits kept in memory.
const AuditRingSize = 1000

type auditRing struct {
	mu   sync.Mutex
	buf  []AuditHit
	next int
}

func (r *auditRing) add(h AuditHit) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.buf) < AuditRingSize {
		r.buf = append(r.buf, h)
		return
	}
	r.buf[r.next] = h
	r.next = (r.next + 1) % AuditRingSize
}

// snapshot returns the hits newest first.
func (r *auditRing) snapshot() []AuditHit {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]AuditHit, 0, len(r.buf))
	for i := len(r.buf) - 1; i >= 0; i-- {
		out = append(out, r.buf[(r.next+i)%len(r.buf)])
	}
	return out
}

const auditRingKey = "nodesvc:audit_ring"

func (s *Service) ring() *auditRing {
	v := s.a.KV.Update(auditRingKey, -1, true, func(old any, ok bool) any {
		if r, isRing := old.(*auditRing); ok && isRing {
			return r
		}
		return &auditRing{}
	})
	return v.(*auditRing)
}

// AuditEntry is one element of a soga audit_log body.
type AuditEntry struct {
	UserID  uint64
	AuditID uint64
}

// ParseAuditLog parses [{"user_id":int,"audit_id":int}] (null and [] are empty); malformed entries are dropped.
func ParseAuditLog(body []byte) ([]AuditEntry, error) {
	items, err := sogaArray(body)
	if err != nil {
		return nil, err
	}
	out := make([]AuditEntry, 0, len(items))
	for _, raw := range items {
		var e struct {
			UserID  json.RawMessage `json:"user_id"`
			AuditID json.RawMessage `json:"audit_id"`
		}
		if json.Unmarshal(raw, &e) != nil {
			continue
		}
		uid, ok1 := parseID(e.UserID)
		aid, err := strconv.ParseUint(strings.TrimSpace(string(e.AuditID)), 10, 64)
		if !ok1 || err != nil {
			continue
		}
		out = append(out, AuditEntry{UserID: uid, AuditID: aid})
	}
	return out, nil
}

// RecordAudit logs every hit with slog.Warn and keeps it in the in-memory ring (docs/NODE_PROTOCOLS.md §3.7).
func (s *Service) RecordAudit(ctx context.Context, r *Resolved, entries []AuditEntry) error {
	if len(entries) == 0 {
		return nil
	}
	if len(entries) > AuditRingSize {
		// One report cannot flood the log: only the newest AuditRingSize hits would survive in the ring anyway.
		s.a.Log.Warn("audit log report truncated", "node_id", r.P.ID, "entries", len(entries), "kept", AuditRingSize)
		entries = entries[len(entries)-AuditRingSize:]
	}
	routes, err := s.Routes(ctx, r.P.RouteIDs)
	if err != nil {
		return err
	}
	rules := map[uint64]string{}
	for _, rule := range RenderAuditRules(routes) {
		rules[rule.ID] = rule.Rule
	}
	ring, now := s.ring(), time.Now()
	for _, e := range entries {
		h := AuditHit{At: now, NodeID: r.P.ID, UserID: e.UserID, AuditID: e.AuditID, RouteID: e.AuditID / 1000, Rule: rules[e.AuditID]}
		s.a.Log.Warn("audit rule hit", "node_id", h.NodeID, "user_id", h.UserID, "route_id", h.RouteID, "rule", h.Rule)
		ring.add(h)
	}
	return nil
}

// AuditHits returns the in-memory audit hits of node nodeID (0 = all nodes), newest first.
func (s *Service) AuditHits(nodeID uint64) []AuditHit {
	all := s.ring().snapshot()
	if nodeID == 0 {
		return all
	}
	out := []AuditHit{}
	for _, h := range all {
		if h.NodeID == nodeID {
			out = append(out, h)
		}
	}
	return out
}

// OnlineUser is a user currently alive on a node.
type OnlineUser struct {
	UserID uint64   `json:"user_id"`
	Email  string   `json:"email"`
	IPs    []string `json:"ips"`
}

// OnlineUsers returns the users alive on node id (pass EffectiveID for children) with their emails, by user id.
func (s *Service) OnlineUsers(ctx context.Context, id uint64) ([]OnlineUser, error) {
	alive := s.a.Online.NodeAlive(id)
	out := make([]OnlineUser, 0, len(alive))
	if len(alive) == 0 {
		return out, nil
	}
	ids := make([]uint64, 0, len(alive))
	for uid := range alive {
		ids = append(ids, uid)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	emails := make(map[uint64]string, len(ids))
	// Chunked: SQLite allows at most 32766 bound parameters per statement (PostgreSQL/MySQL 65535).
	const chunk = 1000
	for i := 0; i < len(ids); i += chunk {
		var rows []struct {
			ID    uint64
			Email string
		}
		part := ids[i:min(i+chunk, len(ids))]
		if err := s.a.DB.WithContext(ctx).Model(&model.User{}).Select("id", "email").Where("id IN ?", part).Scan(&rows).Error; err != nil {
			return nil, err
		}
		for _, r := range rows {
			emails[r.ID] = r.Email
		}
	}
	for _, uid := range ids {
		ips := alive[uid]
		sort.Strings(ips)
		out = append(out, OnlineUser{UserID: uid, Email: emails[uid], IPs: ips})
	}
	return out, nil
}
